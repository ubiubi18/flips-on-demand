// Exact source provenance for staged or modified candidates. Untracked files
// must be reviewed and added before they can enter a release snapshot.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, lstatSync, mkdirSync, readFileSync, realpathSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");
const omitted = (path) => path === "release/MANIFEST.json" || path === "release/MANIFEST.json.staged" ||
  path === "release/FAST_MANIFEST.json" || path === "release/FAST_MANIFEST.json.staged" ||
  path === "release/DIRECT_MANIFEST.json" || path === "release/DIRECT_MANIFEST.json.staged" ||
  path === "release/DIRECT_V3_MANIFEST.json" || path === "release/DIRECT_V3_MANIFEST.json.staged" ||
  path === "release/DIRECT_V4_MANIFEST.json" || path === "release/DIRECT_V4_MANIFEST.json.staged" ||
  path === "release/DIRECT_V5_MANIFEST.json" || path === "release/DIRECT_V5_MANIFEST.json.staged" ||
  path.startsWith("release/artifacts/") || path.startsWith("app/dist/") ||
  /^contracts\/[^/]+\/dist\//.test(path);
const publicFlipVectors = new Map([
  ["proofs/registration/vectors/blob-flip-v1.json", "230ee51b364bdd3c84907c35ad5da2ac6a0c761a03a8087bf601d8b52250801e"],
  ["proofs/registration/vectors/blob-v1.json", "f7aa27dd6b61b9ff022d6b5c4c723a184057f1bb4677644b9e57d976e82a243b"],
]);
// This published toy fixture predates the runtime-private suffix rule.
const publicPrivatePartFixture = new Map([
  ["gateway/fixtures/session-export/flips/bafkreiavv7q7hx64363khxeg573qekzm7ip4acparfb35w4xv6iovg7xi4.private", "cd1eb2757ba3e48d6b1996c5f2a6f759e9e73caab05eb08f0412c71274b6d19b"],
]);
const gitFiles = (root, args) => execFileSync("git", ["ls-files", ...args, "-z"],
  { cwd: root, maxBuffer: 32 * 1024 * 1024 }).toString().split("\0").filter(Boolean);

function sourceBytes(root, path) {
  if (path === "" || path.startsWith("/") || path.includes("\\") || path.split("/").some((p) => p === ".." || p === "." || p === "")) {
    throw new Error("invalid source path");
  }
  if (/\.(age|key)$/.test(path) || /^nodekey/i.test(path.split("/").at(-1))) {
    throw new Error(`private runtime file is not a release input: ${path}`);
  }
  let current = realpathSync(root);
  for (const part of path.split("/")) {
    current = join(current, part);
    if (lstatSync(current).isSymbolicLink()) throw new Error(`source symlink refused: ${path}`);
  }
  const stat = lstatSync(current);
  if (!stat.isFile()) throw new Error(`source is not a regular file: ${path}`);
  const bytes = readFileSync(current);
  if (/\.private$/.test(path) && publicPrivatePartFixture.get(path) !== digest(bytes)) {
    throw new Error(`private runtime file is not a release input: ${path}`);
  }
  // Runtime keys may have a custom filename that the ignore rules do not
  // cover. Their known payload types must never become contributor sources.
  const text = bytes.toString("utf8");
  let privateRuntime = /^-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----/m.test(text);
  if (text.trimStart().startsWith("{")) {
    let record;
    try { record = JSON.parse(text); } catch { /* Not a key payload. */ }
    if (record && typeof record === "object") {
      privateRuntime ||= record.format === "fod.provider-application-key" ||
        record.format === "fod:fast-content-seller-private:v1" ||
        record.kind === "fod.epoch-keys" || record.kind === "fod.delivery-key" ||
        typeof record.privateKeyPem === "string";
      if (typeof record.publicPartSecret === "string" && typeof record.privatePartSecret === "string") {
        privateRuntime ||= publicFlipVectors.get(path) !== digest(bytes);
      }
    }
  }
  if (privateRuntime) throw new Error(`private runtime file is not a release input: ${path}`);
  return { bytes, executable: (stat.mode & 0o111) !== 0 };
}

export function sourceInventory(root) {
  const untracked = gitFiles(root, ["--others", "--exclude-standard"])
    .filter((path) => !omitted(path));
  if (untracked.length > 0) throw new Error("untracked files are not release inputs; review and add them first");
  const paths = [...new Set(gitFiles(root, ["--cached"]))]
    .filter((p) => !omitted(p) && existsSync(join(root, p))).sort();
  const files = paths.map((path) => {
    const file = sourceBytes(root, path);
    return { path, bytes: file.bytes.length, sha256: digest(file.bytes), executable: file.executable };
  });
  return { format: "fod.source-snapshot", version: 1, sha256: digest(JSON.stringify(files)), files };
}

export function verifySource(root, source) {
  if (source?.format !== "fod.source-snapshot" || source.version !== 1 || !Array.isArray(source.files) ||
      digest(JSON.stringify(source.files)) !== source.sha256) throw new Error("invalid source inventory");
  const names = new Set();
  for (const entry of source.files) {
    if (names.has(entry.path)) throw new Error("duplicate source path");
    names.add(entry.path);
    const file = sourceBytes(root, entry.path);
    if (file.bytes.length !== entry.bytes || digest(file.bytes) !== entry.sha256 || file.executable !== entry.executable) {
      throw new Error(`source drift: ${entry.path}`);
    }
  }
  return source.sha256;
}

export function exportSource(root, destination, source) {
  verifySource(root, source);
  mkdirSync(destination, { mode: 0o700 }); // deliberately refuses an existing destination
  for (const entry of source.files) {
    const file = sourceBytes(root, entry.path);
    if (digest(file.bytes) !== entry.sha256) throw new Error(`source changed during snapshot: ${entry.path}`);
    const target = join(destination, entry.path);
    mkdirSync(dirname(target), { recursive: true, mode: 0o700 });
    writeFileSync(target, file.bytes, { flag: "wx", mode: entry.executable ? 0o700 : 0o600 });
  }
  verifySource(destination, source);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
  const manifest = JSON.parse(readFileSync(join(root, "release/MANIFEST.json")));
  verifySource(root, manifest.source);
  // Also refuse extra/removed non-ignored files in the current checkout.
  if (sourceInventory(root).sha256 !== manifest.source.sha256) throw new Error("source file set drift");
  if (process.argv[2]) {
    exportSource(root, resolve(process.argv[2]), manifest.source);
    writeFileSync(join(resolve(process.argv[2]), "release/MANIFEST.json"), `${JSON.stringify(manifest, null, 2)}\n`, { flag: "wx", mode: 0o600 });
  }
  console.log(`source verified: ${manifest.source.files.length} files, ${manifest.source.sha256}`);
}
