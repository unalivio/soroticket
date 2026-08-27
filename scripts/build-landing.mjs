import { cp, copyFile, mkdir, rm } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const output = resolve(root, "dist");

await rm(output, { recursive: true, force: true });
await mkdir(resolve(output, "assets"), { recursive: true });

await copyFile(
  resolve(root, "soroticket-index.html"),
  resolve(output, "index.html"),
);
await cp(
  resolve(root, "assets", "soroticket"),
  resolve(output, "assets", "soroticket"),
  { recursive: true },
);

console.log("Soroticket landing built in dist/");
