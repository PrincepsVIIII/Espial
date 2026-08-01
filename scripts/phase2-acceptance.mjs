import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

function run(label, command, args) {
  process.stdout.write(`\n[Phase 2] ${label}\n`);
  const result = spawnSync(command, args, { cwd: root, stdio: "inherit" });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${label} failed with ${result.status}`);
  }
}

run("Contract, documentation, and generated-type compatibility", "npm", [
  "run",
  "check:repo",
]);
run("Core static analysis", "npm", ["run", "core:lint"]);
run("Race and PostgreSQL integration gate", "node", [
  "scripts/phase2-db-gate.mjs",
]);
run("Representative and oversized backlog profiles", "node", [
  "scripts/phase2-load.mjs",
]);
run("Clean-stack lifecycle, SSE, shutdown, and restore drill", "node", [
  "scripts/phase1-acceptance.mjs",
]);
run("Frontend diagnostics and unit contracts", "npm", [
  "--prefix",
  "web",
  "run",
  "check",
]);
run("Frontend unit suite", "npm", ["--prefix", "web", "run", "test"]);
run("Cross-route browser acceptance", "npm", [
  "--prefix",
  "web",
  "run",
  "test:browser",
]);
run("Production Web build", "npm", ["--prefix", "web", "run", "build"]);

process.stdout.write("\nPhase 2 automated acceptance passed.\n");
