import { randomBytes } from "node:crypto";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const environmentPath = path.join(root, ".env");
const secretPath = path.join(root, "deployments/local/database_dsn.local");

if (!existsSync(environmentPath)) {
  const password = randomBytes(24).toString("base64url");
  const template = readFileSync(path.join(root, ".env.example"), "utf8")
    .replace(/^POSTGRES_PASSWORD=.*$/m, `POSTGRES_PASSWORD=${password}`)
    .replace(
      /^ESPIAL_DATABASE_DSN_SECRET_FILE=.*$/m,
      "ESPIAL_DATABASE_DSN_SECRET_FILE=./database_dsn.local",
    );
  writeFileSync(environmentPath, template, { mode: 0o600, flag: "wx" });
  const dsn = `postgres://espial:${encodeURIComponent(password)}@postgres:5432/espial?sslmode=disable\n`;
  writeFileSync(secretPath, dsn, { mode: 0o600, flag: "wx" });
  process.stdout.write(
    "Created .env and a random local PostgreSQL credential.\n",
  );
} else if (!existsSync(secretPath)) {
  const environment = readFileSync(environmentPath, "utf8");
  if (
    environment.includes("ESPIAL_DATABASE_DSN_SECRET_FILE=./database_dsn.local")
  ) {
    throw new Error(
      "deployments/local/database_dsn.local is missing; restore it or choose a fresh local database before initialization",
    );
  }
}

const compose = [
  "compose",
  "--env-file",
  ".env",
  "-f",
  "deployments/local/compose.yml",
];

function docker(args) {
  const result = spawnSync("docker", [...compose, ...args], {
    cwd: root,
    stdio: "inherit",
  });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}

docker(["up", "-d", "--build", "postgres"]);
docker(["run", "--rm", "core", "migrate"]);
docker(["run", "--rm", "core", "admin", "bootstrap", "--username", "admin"]);
docker(["up", "--build"]);
