import { access, readFile, readdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const markdownFiles = await walk(root);
const missing = [];

for (const file of markdownFiles) {
  const content = await readFile(file, "utf8");
  for (const match of content.matchAll(/\]\(([^)]+)\)/g)) {
    const target = match[1];
    if (/^(?:https?:|mailto:|#)/.test(target)) continue;

    const withoutAnchor = target.replace(/#.*$/, "").replace(/^<|>$/g, "");
    try {
      await access(path.resolve(path.dirname(file), withoutAnchor));
    } catch {
      missing.push(`${path.relative(root, file)}: ${target}`);
    }
  }
}

if (missing.length > 0) {
  throw new Error(`Missing local documentation links:\n${missing.join("\n")}`);
}

console.log(`Checked local links in ${markdownFiles.length} Markdown files.`);

async function walk(directory) {
  const files = [];
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    if (entry.name === ".git" || entry.name === "node_modules") continue;
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) files.push(...(await walk(entryPath)));
    if (entry.isFile() && entry.name.endsWith(".md")) files.push(entryPath);
  }
  return files;
}
