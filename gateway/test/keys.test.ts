import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, stat, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { loadOrCreateApplicationKey, signResult, verifyResult } from "../src/keys.ts";

test("concurrent first starts preserve one complete application key", async () => {
  const dir = await mkdtemp(join(tmpdir(), "fod-key-race-"));
  try {
    const path = join(dir, "key.json");
    const keys = await Promise.all(Array.from({ length: 12 }, () => loadOrCreateApplicationKey(path, "provider")));
    const stored = JSON.parse(await readFile(path, "utf8"));
    for (const key of keys) {
      assert.equal(key.publicKeyHex, stored.publicKeyHex);
      assert.equal(verifyResult(stored.publicKeyHex, "test", signResult(key.privateKey, "test")), true);
    }
  } finally { await rm(dir, { recursive: true, force: true }); }
});

test("key loading refuses aliases, another provider and a substituted public key", async () => {
  const dir = await mkdtemp(join(tmpdir(), "fod-key-validation-"));
  try {
    const path = join(dir, "key.json");
    await loadOrCreateApplicationKey(path, "provider");
    await symlink(path, join(dir, "alias.json"));
    await assert.rejects(loadOrCreateApplicationKey(join(dir, "alias.json"), "provider"));
    await assert.rejects(loadOrCreateApplicationKey(path, "another-provider"));
    const stored = JSON.parse(await readFile(path, "utf8"));
    stored.publicKeyHex = "ab".repeat(44);
    await writeFile(path, JSON.stringify(stored));
    await assert.rejects(loadOrCreateApplicationKey(path, "provider"));
  } finally { await rm(dir, { recursive: true, force: true }); }
});

test("loading an existing key restores private file permissions", async () => {
  const dir = await mkdtemp(join(tmpdir(), "fod-key-mode-"));
  try {
    const path = join(dir, "key.json");
    const key = await loadOrCreateApplicationKey(path, "provider");
    const saved = await readFile(path);
    await rm(path);
    await writeFile(path, saved, { mode: 0o644 });
    assert.equal((await loadOrCreateApplicationKey(path, "provider")).publicKeyHex, key.publicKeyHex);
    assert.equal((await stat(path)).mode & 0o777, 0o600);
    assert.deepEqual(await readFile(path), saved);
  } finally { await rm(dir, { recursive: true, force: true }); }
});
