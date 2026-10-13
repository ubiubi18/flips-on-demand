import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { spawn } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

test("browser verification refuses an occupied service port without contacting its occupant", async () => {
  let requests = 0;
  const occupant = createServer((request, response) => {
    requests++;
    response.writeHead(200, { "content-type": "application/json" });
    response.end("{}");
  });
  await new Promise((resolve, reject) => {
    occupant.once("error", reject);
    occupant.listen(8787, "127.0.0.1", resolve);
  });
  try {
    const script = fileURLToPath(new URL("../app/verify.mjs", import.meta.url));
    const child = spawn(process.execPath, [script], { cwd: resolve(fileURLToPath(new URL("..", import.meta.url))), stdio: ["ignore", "pipe", "pipe"] });
    let output = "";
    child.stdout.on("data", (chunk) => { output += String(chunk); });
    child.stderr.on("data", (chunk) => { output += String(chunk); });
    const timeout = setTimeout(() => child.kill(), 10_000);
    const code = await new Promise((resolve, reject) => {
      child.once("error", reject);
      child.once("close", resolve);
    }).finally(() => clearTimeout(timeout));
    assert.equal(code, 1, output.slice(-1000));
    assert.match(output, /FAIL.*example gateway starts/);
    assert.equal(requests, 0, "verification touched an existing service");
    const response = await fetch("http://127.0.0.1:8787");
    assert.equal(response.status, 200, "verification terminated the existing service");
  } finally { await new Promise((resolve) => occupant.close(resolve)); }
});
