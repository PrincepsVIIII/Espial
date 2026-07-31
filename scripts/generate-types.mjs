import { mkdir, readdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { compileFromFile } from "json-schema-to-typescript";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const schemaDirectory = path.join(root, "api", "schemas", "v1");
const outputFile = path.join(root, "web", "src", "lib", "api", "generated.ts");
const names = (await readdir(schemaDirectory))
  .filter((name) => name.endsWith(".schema.json"))
  .sort();

const sections = [];
for (const name of names) {
  sections.push(
    await compileFromFile(path.join(schemaDirectory, name), {
      bannerComment: "",
      style: { singleQuote: false },
    }),
  );
}

const banner = "// Generated from api/schemas/v1. Do not edit by hand.\n\n";
await mkdir(path.dirname(outputFile), { recursive: true });
await writeFile(outputFile, banner + sections.join("\n"), "utf8");
console.log(
  `Generated ${path.relative(root, outputFile)} from ${names.length} schemas.`,
);
