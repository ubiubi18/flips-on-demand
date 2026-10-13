// Build a separately named, ownerless fast-contract candidate. A real author
// still needs a contract-bound proof and canonical post-session witness.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { sourceInventory } from "./source-snapshot.mjs";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const escrow = join(root, "contracts", "escrow");
const artifact = join(root, "release", "artifacts", "fod_fast_escrow.wasm");
const staged = `${artifact}.staged`;
const manifestPath = join(root, "release", "FAST_MANIFEST.json");
const stagedManifest = `${manifestPath}.staged`;
const harnessWasm = join(root, "spikes", "fast-escrow-candidate", "dist", "fod_fast_escrow.testhooks.wasm");
const targetWasm = join(escrow, "target", "wasm32-unknown-unknown", "release", "fod_escrow.wasm");
const sha256 = (bytes) => createHash("sha256").update(bytes).digest("hex");

function run(command, args, cwd = root, env = process.env) {
  return execFileSync(command, args, { cwd, env, encoding: "utf8", maxBuffer: 64 * 1024 * 1024 }).trim();
}

function exportsOf(bytes) {
  let at = 8;
  const u32 = () => {
    let value = 0, shift = 0, byte;
    do { byte = bytes[at++]; value |= (byte & 127) << shift; shift += 7; } while (byte & 128);
    return value >>> 0;
  };
  while (at < bytes.length) {
    const id = bytes[at++];
    const size = u32();
    const end = at + size;
    if (id !== 7) { at = end; continue; }
    const names = [];
    for (let count = u32(); count > 0; count -= 1) {
      const length = u32();
      names.push(bytes.subarray(at, at + length).toString("utf8"));
      at += length + 1;
      u32();
    }
    return names;
  }
  return [];
}

const expectedExports = [
  "memory", "allocate", "deploy", "addReserve", "withdrawReserve", "donateUnit",
  "sellUnit", "beacon", "poolView", "buy", "refund", "orderPoolView",
  "withdraw", "accounting", "registerFast", "fulfillFast", "fastBundleView", "fastDeliveryView",
];
const harness = join(root, "spikes", "harness");
const harnessEnv = { ...process.env, GOFLAGS: "-modfile=go.local.mod", GOTOOLCHAIN: "auto", GOPROXY: "off", GOWORK: "off" };
const canonicalMod = JSON.parse(run("go", ["mod", "edit", "-json"], harness, { ...harnessEnv, GOFLAGS: "" }));
const localMod = JSON.parse(run("go", ["mod", "edit", "-json", "-modfile=go.local.mod"], harness, { ...harnessEnv, GOFLAGS: "" }));
const replacements = localMod.Replace ?? [];
localMod.Replace = null;
if (canonicalMod.Replace !== null || replacements.length !== 1 ||
    replacements[0].Old.Path !== "github.com/idena-network/idena-wasm-binding" ||
    replacements[0].Old.Version || replacements[0].New.Version ||
    JSON.stringify(localMod) !== JSON.stringify(canonicalMod)) {
  throw new Error("local harness module graph differs from the tracked graph");
}
const binding = run("go", ["list", "-m", "-f", "{{.Dir}}", "github.com/idena-network/idena-wasm-binding"], harness, harnessEnv);
if (replacements[0].New.Path !== binding) throw new Error("local harness binding replacement differs from the resolved runtime");
const bindingRevision = run("git", ["-C", binding, "rev-parse", "HEAD"]);
if (bindingRevision !== "f7c71ce32d86c8ed26e74935265c95768b0230d0" ||
    run("git", ["-C", binding, "status", "--porcelain=v1", "--untracked-files=all"]) !== "") {
  throw new Error("pinned Idena WASM runtime binding is unavailable or modified");
}
const source = sourceInventory(root);
const revision = run("git", ["rev-parse", "HEAD"]);
const dirty = run("git", ["status", "--porcelain=v1", "--untracked-files=no", "--",
  ":(exclude)release/FAST_MANIFEST.json", ":(exclude)release/artifacts"]).length > 0;

try {
  run("node", ["release/check-proof-builds.mjs", "--fast-only"]);
  run("cargo", ["build", "--locked", "--offline", "--release", "--target", "wasm32-unknown-unknown", "--features", "fast-candidate"], escrow);
  await mkdir(dirname(artifact), { recursive: true });
  run("wasm-opt", ["--llvm-memory-copy-fill-lowering", "-O3", targetWasm, "-o", staged]);
  const code = await readFile(staged);
  const operators = run("wasm-objdump", ["-d", staged]);
  if (/memory\.(copy|fill)|f32\.|f64\./.test(operators)) throw new Error("fast candidate uses forbidden WASM operators");
  const exports = exportsOf(code);
  if (new Set(exports).size !== expectedExports.length || exports.some((name) => !expectedExports.includes(name))) {
    throw new Error("fast candidate export surface differs from the production surface");
  }

  run("cargo", ["build", "--locked", "--offline", "--release", "--target", "wasm32-unknown-unknown", "--features", "fast-candidate,test-hooks"], escrow);
  await mkdir(dirname(harnessWasm), { recursive: true });
  run("wasm-opt", ["--llvm-memory-copy-fill-lowering", "-O3", targetWasm, "-o", harnessWasm]);
  const tests = run("go", ["test", "-count=1", "-run", "^TestFast(ContentSealVerifiesInPinnedRuntime|SyntheticRegistrationMeasuresCombinedGas|CandidateOmitsLegacySettlement|EscrowSettlesOnlyTheStoredFundedOrder|ProductionDeployFitsBlock)$", "-v", "."],
    harness, { ...harnessEnv, FOD_FAST_PRODUCTION_WASM: staged });
  const registration = tests.match(/successful synthetic registerFast: wasm gas=(\d+), call bytes=(\d+)/);
  const deployment = tests.match(/production fast deploy: wasm gas=(\d+), wasm bytes=(\d+), conservative transaction gas=(\d+)/);
  if (!registration || !deployment || Number(deployment[2]) !== code.length) {
    throw new Error("fast candidate gas evidence is incomplete");
  }
  const registrationGas = Math.ceil(Number(registration[1]) / 100) + (Number(registration[2]) + 512) * 10;
  if (registrationGas > 5_120_000 || Number(deployment[3]) > 5_120_000) {
    throw new Error("fast candidate exceeds one block in the local runtime");
  }
  if (sourceInventory(root).sha256 !== source.sha256) throw new Error("source changed during fast candidate build");

  const fixture = JSON.parse(await readFile(join(root, "proofs", "fast-content", "dist", "synthetic-one-flip.json")));
  const manifest = {
    format: "fod.fast-candidate-manifest", version: 1, generatedAt: new Date().toISOString(),
    revision, worktreeDirty: dirty, source, runtimeBindingRevision: bindingRevision,
    artifact: { path: relative(root, artifact), bytes: code.length, sha256: sha256(code) },
    proofProgram: { statement: fixture.statement, imageId: fixture.image_id, journalBytes: fixture.journal.length / 2, sealBytes: fixture.seal.length / 2 },
    checks: {
      sourceOnlyGuest: "two clean builds with identical image and ELF; image matches contract pin and sealed fixture",
      seal: "genuine receipt accepted in pinned Idena WASM runtime; altered journal and seal refused",
      exports, forbiddenOperators: 0,
      syntheticRegistration: { wasmGas: Number(registration[1]), callBytes: Number(registration[2]), conservativeTransactionGas: registrationGas },
      deploy: { wasmGas: Number(deployment[1]), conservativeTransactionGas: Number(deployment[3]) },
    },
    deployment: { network: null, contract: null, note: "Experimental candidate only. No live author proof, real-session registration gas or paid mainnet qualification is implied." },
  };
  await writeFile(stagedManifest, `${JSON.stringify(manifest, null, 2)}\n`);
  await rename(staged, artifact);
  await rename(stagedManifest, manifestPath);
  console.log(`fast candidate built: ${code.length} bytes; synthetic registration ${registrationGas} conservative gas; deploy ${deployment[3]} conservative gas`);
} catch (error) {
  await rm(staged, { force: true });
  await rm(stagedManifest, { force: true });
  throw error;
}
