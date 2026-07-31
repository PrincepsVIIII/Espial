import { mkdir, readFile, readdir, writeFile } from "node:fs/promises";
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
      ignoreMinAndMaxItems: true,
      style: { singleQuote: false },
    }),
  );
}

const banner = "// Generated from api/schemas/v1. Do not edit by hand.\n\n";
const generated = banner + sections.join("\n");
const relativeOutput = path.relative(root, outputFile);
if (process.argv.includes("--check")) {
  let existing = "";
  try {
    existing = await readFile(outputFile, "utf8");
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  if (existing !== generated) {
    throw new Error(`${relativeOutput} is stale; run npm run generate`);
  }
  console.log(`Checked ${relativeOutput} against ${names.length} schemas.`);
} else {
  await mkdir(path.dirname(outputFile), { recursive: true });
  await writeFile(outputFile, generated, "utf8");
  console.log(`Generated ${relativeOutput} from ${names.length} schemas.`);
}
