import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";

type Request = { dir: string; paths: string[] };

const request: Request = JSON.parse(await Bun.stdin.text());
if (!request.dir) throw new Error("compress: dir is required");
if (!Array.isArray(request.paths)) throw new Error("compress: paths must be an array");

for (const relative of request.paths) {
  const file = path.join(request.dir, relative);
  const body = readFileSync(file);
  writeFileSync(
    `${file}.br`,
    brotliCompressSync(body, { params: { [constants.BROTLI_PARAM_QUALITY]: 11 } }),
  );
  writeFileSync(`${file}.gz`, gzipSync(body, { level: 9 }));
}
