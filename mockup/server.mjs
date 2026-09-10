import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = fileURLToPath(new URL(".", import.meta.url));
const port = Number(process.env.PORT || 4175);
const mime = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".png": "image/png",
};
const server = createServer(async (req, res) => {
  try {
    const pathname = decodeURIComponent(
      new URL(req.url, "http://localhost").pathname,
    );
    const relative = pathname === "/" ? "index.html" : pathname.slice(1);
    const resolved = path.resolve(root, relative);
    if (!resolved.startsWith(root) || !mime[path.extname(resolved)]) {
      res.writeHead(404).end("Not found");
      return;
    }
    const data = await readFile(resolved);
    res.writeHead(200, {
      "Content-Type": mime[path.extname(resolved)],
      "Cache-Control": "no-store",
      "X-Content-Type-Options": "nosniff",
    });
    res.end(data);
  } catch {
    res.writeHead(404).end("Not found");
  }
});
server.listen(port, "127.0.0.1", () =>
  console.log(`Espial design preview: http://localhost:${port}`),
);
