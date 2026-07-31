import { readFile, readdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const schemaDirectory = path.join(root, "api", "schemas", "v1");
const fixtureDirectory = path.join(root, "api", "fixtures", "v1");

const ajv = new Ajv2020({
  allErrors: true,
  allowUnionTypes: true,
  strict: true,
});
addFormats(ajv);

for (const name of await jsonFiles(schemaDirectory)) {
  const schema = await readJSON(path.join(schemaDirectory, name));
  ajv.addSchema(schema, name.replace(".schema.json", ""));
}

let checked = 0;
for (const expectation of ["valid", "invalid"]) {
  for (const name of await jsonFiles(
    path.join(fixtureDirectory, expectation),
  )) {
    const fixture = await readJSON(
      path.join(fixtureDirectory, expectation, name),
    );
    const validate = ajv.getSchema(fixture.schema);

    if (!validate) {
      throw new Error(`${name}: unknown schema ${fixture.schema}`);
    }

    const valid = validate(fixture.data);
    const expected = expectation === "valid";
    if (valid !== expected) {
      const details = ajv.errorsText(validate.errors, { separator: "\n  " });
      throw new Error(`${name}: expected ${expectation}\n  ${details}`);
    }

    checked += 1;
  }
}

console.log(`Validated ${checked} contract fixtures.`);

async function jsonFiles(directory) {
  return (await readdir(directory))
    .filter((name) => name.endsWith(".json"))
    .sort();
}

async function readJSON(file) {
  return JSON.parse(await readFile(file, "utf8"));
}
