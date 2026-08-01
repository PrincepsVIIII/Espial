import { randomBytes } from "node:crypto";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const core = path.join(root, "core");
const suffix = randomBytes(4).toString("hex");
const containerName = `espial-phase2-load-${suffix}`;
const networkName = `espial-phase2-load-${suffix}`;
const password = `phase2-load-${randomBytes(12).toString("base64url")}`;

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
  throw new Error("timed out waiting for the Phase 2 load database");
}

run("docker", ["network", "create", networkName]);
try {
  run("docker", [
    "run",
    "--detach",
    "--rm",
    "--name",
    containerName,
    "--network",
    networkName,
    "-e",
    "POSTGRES_DB=espial",
    "-e",
    "POSTGRES_USER=espial",
    "-e",
    `POSTGRES_PASSWORD=${password}`,
    "postgres:17-alpine",
  ]);
  await waitForPostgres();
  const dsn = `postgres://espial:${encodeURIComponent(password)}@${containerName}:5432/espial?sslmode=disable`;
  run("docker", [
    "run",
    "--rm",
    "--network",
    networkName,
    "-e",
    `ESPIAL_TEST_DATABASE_URL=${dsn}`,
    "-e",
    "ESPIAL_PHASE2_LOAD_TEST=1",
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
    "-v",
    "-count=1",
    "-timeout=5m",
    "-run",
    "^TestPhase2BacklogProfiles$",
    "./internal/operations",
  ]);
} finally {
  run("docker", ["stop", "--time", "5", containerName], {
    capture: true,
    allowFailure: true,
  });
  run("docker", ["network", "rm", networkName], {
    capture: true,
    allowFailure: true,
  });
}
