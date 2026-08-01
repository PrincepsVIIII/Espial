import { access, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const name = path.join(root, "api", "openapi", "v1.json");
const document = JSON.parse(await readFile(name, "utf8"));

if (
  document.openapi !== "3.1.0" ||
  !document.info?.title ||
  !document.info?.version
) {
  throw new Error("OpenAPI v1 must be a titled OpenAPI 3.1 document");
}

const requiredOperations = new Map([
  ["/health/live", ["get"]],
  ["/health/ready", ["get"]],
  ["/auth/capabilities", ["get"]],
  ["/auth/local/login", ["post"]],
  ["/auth/session", ["get"]],
  ["/auth/logout", ["post"]],
  ["/admin/ping", ["get"]],
  ["/overview", ["get"]],
  ["/resources", ["get"]],
  ["/resources/{id}", ["get"]],
  ["/integrations", ["get", "post"]],
  ["/integrations/{id}", ["get"]],
  ["/integrations/{id}/configuration", ["put"]],
  ["/audit", ["get"]],
  ["/incidents", ["get"]],
  ["/incidents/{id}", ["get"]],
  ["/incidents/{id}/timeline", ["get"]],
  ["/incidents/{id}/acknowledge", ["post"]],
  ["/incidents/{id}/investigate", ["post"]],
  ["/incidents/{id}/owner", ["put"]],
  ["/incidents/{id}/notes", ["post"]],
  ["/incidents/{id}/resolve", ["post"]],
  ["/incident-assignees", ["get"]],
  ["/events/stream", ["get"]],
]);

const operationIDs = new Set();
for (const [route, methods] of requiredOperations) {
  for (const method of methods) {
    const operation = document.paths?.[route]?.[method];
    if (!operation?.operationId || !operation.responses) {
      throw new Error(`OpenAPI is missing ${method.toUpperCase()} ${route}`);
    }
    if (operationIDs.has(operation.operationId)) {
      throw new Error(`duplicate operationId ${operation.operationId}`);
    }
    operationIDs.add(operation.operationId);
  }
}

let references = 0;
await visit(document, name);
if (references < 20) {
  throw new Error(
    "OpenAPI v1 is not reusing enough shared schema/components references",
  );
}

console.log(
  `Validated OpenAPI v1 with ${operationIDs.size} operations and ${references} references.`,
);

async function visit(value, owner) {
  if (Array.isArray(value)) {
    for (const item of value) await visit(item, owner);
    return;
  }
  if (!value || typeof value !== "object") return;
  if (typeof value.$ref === "string") {
    references += 1;
    if (!value.$ref.startsWith("#")) {
      const relative = value.$ref.split("#", 1)[0];
      await access(path.resolve(path.dirname(owner), relative));
    }
  }
  for (const child of Object.values(value)) await visit(child, owner);
}
