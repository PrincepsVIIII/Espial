import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const core = path.join(root, "core");
const args = process.argv.slice(2);

if (args.length === 0) {
  throw new Error("run-go requires a Go command");
}

const local = spawnSync("go", args, { cwd: core, stdio: "inherit" });
if (!local.error) {
  process.exit(local.status ?? 1);
}
if (local.error.code !== "ENOENT") {
  throw local.error;
}

const dockerArgs = [
  "run",
  "--rm",
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
  ...args,
];
const container = spawnSync("docker", dockerArgs, { stdio: "inherit" });
if (container.error) throw container.error;
process.exit(container.status ?? 1);
