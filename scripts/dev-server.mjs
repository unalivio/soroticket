import { createReadStream, existsSync, statSync } from "node:fs";
import { createServer } from "node:http";
import { extname, resolve, sep } from "node:path";

const projectRoot = resolve(import.meta.dirname, "..");
const host = process.env.HOST || "127.0.0.1";
const port = Number(process.env.PORT || 5173);

const contentTypes = {
  ".css": "text/css; charset=utf-8",
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".svg": "image/svg+xml",
  ".ttf": "font/ttf",
  ".woff": "font/woff",
  ".woff2": "font/woff2"
};

const server = createServer((request, response) => {
  const requestUrl = new URL(request.url || "/", `http://${request.headers.host || host}`);
  const pathname = requestUrl.pathname === "/" ? "/soroticket-index.html" : requestUrl.pathname;
  const decodedPath = decodeURIComponent(pathname);
  const filePath = resolve(projectRoot, `.${decodedPath}`);

  if (filePath !== projectRoot && !filePath.startsWith(`${projectRoot}${sep}`)) {
    response.writeHead(403, { "Content-Type": "text/plain; charset=utf-8" });
    response.end("Forbidden");
    return;
  }

  if (!existsSync(filePath) || !statSync(filePath).isFile()) {
    response.writeHead(404, { "Content-Type": "text/plain; charset=utf-8" });
    response.end("Not found");
    return;
  }

  response.writeHead(200, {
    "Cache-Control": "no-store",
    "Content-Type": contentTypes[extname(filePath).toLowerCase()] || "application/octet-stream"
  });
  createReadStream(filePath).pipe(response);
});

server.listen(port, host, () => {
  console.log(`Soroticket disponible en http://${host}:${port}`);
});

const shutdown = () => server.close(() => process.exit(0));
process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);
