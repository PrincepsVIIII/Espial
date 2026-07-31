import { existsSync } from "node:fs";
import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const environmentFile = existsSync(path.join(root, ".env"))
  ? ".env"
  : ".env.example";
const args = [
  "compose",
  "--env-file",
  environmentFile,
  "-f",
  "deployments/local/compose.yml",
  ...process.argv.slice(2),
];
const result = spawnSync("docker", args, { cwd: root, stdio: "inherit" });
if (result.error) throw result.error;
process.exit(result.status ?? 1);
