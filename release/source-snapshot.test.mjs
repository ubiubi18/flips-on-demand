import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { exportSource, sourceInventory, verifySource } from "./source-snapshot.mjs";

test("release source refuses runtime credentials even under a custom filename", () => {
  const root = mkdtempSync(join(tmpdir(), "fod-source-secret-test-"));
  execFileSync("git", ["init", "-q", root]);
  try {
    for (const name of ["custom-runtime.json", "custom-runtime.JSON", "no-extension"]) {
      const path = join(root, name);
      for (const data of [
        { format: "fod.provider-application-key", privateKeyPem: "synthetic-private-key" },
        { kind: "fod.epoch-keys", publicPartPrivateKey: "synthetic-secret" },
        { kind: "fod.delivery-key", privateKey: "synthetic-secret" },
        { format: "fod:fast-content-seller-private:v1", blind: "synthetic-secret" },
        { publicPartSecret: "synthetic-secret", privatePartSecret: "synthetic-secret", blobs: [] },
      ]) {
        writeFileSync(path, JSON.stringify(data));
        execFileSync("git", ["add", "--", name], { cwd: root });
        assert.throws(() => sourceInventory(root), /private runtime file/);
      }
      rmSync(path);
      execFileSync("git", ["rm", "--cached", "-q", "--", name], { cwd: root });
    }
    writeFileSync(join(root, "custom.pem"), "-----BEGIN PRIVATE KEY-----\nsynthetic-test-only\n-----END PRIVATE KEY-----\n");
    execFileSync("git", ["add", "--", "custom.pem"], { cwd: root });
    assert.throws(() => sourceInventory(root), /private runtime file/);
    rmSync(join(root, "custom.pem"));
    execFileSync("git", ["rm", "--cached", "-q", "--", "custom.pem"], { cwd: root });
    writeFileSync(join(root, "output.private"), "synthetic decrypted private part");
    execFileSync("git", ["add", "--", "output.private"], { cwd: root });
    assert.throws(() => sourceInventory(root), /private runtime file/);
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test("uncommitted source copies exactly; ignored keys stay out; drift and symlinks refuse", () => {
  const base = mkdtempSync(join(tmpdir(), "fod-source-test-"));
  const root = join(base, "repo");
  execFileSync("git", ["init", "-q", root]);
  try {
    writeFileSync(join(root, ".gitignore"), "*.key\n");
    writeFileSync(join(root, "app.mjs"), "original source\n");
    writeFileSync(join(root, "test.key"), "synthetic ignored secret");
    assert.throws(() => sourceInventory(root), /untracked files/);
    execFileSync("git", ["add", "--", ".gitignore", "app.mjs"], { cwd: root });
    const source = sourceInventory(root);
    assert.equal(source.files.length, 2);
    const snapshot = join(base, "snapshot");
    exportSource(root, snapshot, source);
    assert.equal(verifySource(snapshot, source), source.sha256);
    assert.equal(readFileSync(join(snapshot, "app.mjs"), "utf8"), "original source\n");
    assert.throws(() => exportSource(root, snapshot, source));
    writeFileSync(join(root, "app.mjs"), "altered source\n");
    assert.throws(() => verifySource(root, source), /source drift/);
    rmSync(join(root, "app.mjs"));
    symlinkSync(join(snapshot, "app.mjs"), join(root, "app.mjs"));
    assert.throws(() => sourceInventory(root), /symlink refused/);
    assert.throws(() => verifySource(root, { ...source, files: [{ path: "../escape" }] }), /invalid source inventory/);
  } finally {
    rmSync(base, { recursive: true, force: true });
  }
});
