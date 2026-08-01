import { randomBytes } from "node:crypto";
import net from "node:net";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const core = path.join(root, "core");
const suffix = randomBytes(4).toString("hex");
const containerName = `espial-phase2-db-${suffix}`;
const password = `phase2-db-${randomBytes(12).toString("base64url")}`;

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? root,
    encoding: "utf8",
    stdio: options.capture ? ["ignore", "pipe", "pipe"] : "inherit",
  });
  if (result.error) throw result.error;
  if (result.status !== 0 && !options.allowFailure) {
    throw new Error(
      `${command} ${args.join(" ")} failed with ${result.status}${options.capture ? `\n${result.stderr}` : ""}`,
    );
  }
  return result;
}

function availablePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      server.close((error) => (error ? reject(error) : resolve(address.port)));
    });
  });
}

async function waitForPostgres() {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const result = run(
      "docker",
      ["exec", containerName, "pg_isready", "-U", "espial", "-d", "espial"],
      { capture: true, allowFailure: true },
    );
    if (result.status === 0) return;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error("timed out waiting for the Phase 2 race database");
}

const port = await availablePort();
try {
  run("docker", [
    "run",
    "--detach",
    "--rm",
    "--name",
    containerName,
    "-e",
    "POSTGRES_DB=espial",
    "-e",
    "POSTGRES_USER=espial",
    "-e",
    `POSTGRES_PASSWORD=${password}`,
    "-p",
    `127.0.0.1:${port}:5432`,
    "postgres:17-alpine",
  ]);
  await waitForPostgres();
  const dsn = `postgres://espial:${encodeURIComponent(password)}@host.docker.internal:${port}/espial?sslmode=disable`;
  run("docker", [
    "run",
    "--rm",
    "--add-host",
    "host.docker.internal:host-gateway",
    "-e",
    `ESPIAL_TEST_DATABASE_URL=${dsn}`,
    "-v",
    `${core}:/workspace`,
    "-v",
    "espial-go-mod:/go/pkg/mod",
    "-v",
    "espial-go-build:/root/.cache/go-build",
    "-w",
    "/workspace",
    "golang:1.26",
    "go",
    "test",
    "-race",
    "-count=1",
    "-timeout=8m",
    "./...",
  ]);
} finally {
  run("docker", ["stop", "--time", "5", containerName], {
    capture: true,
    allowFailure: true,
  });
}
