// Build the proof-backed v5 direct-sale candidate only after the current fast
// guest, sealed synthetic receipt, and pinned runtime pass their release gates.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { copyFile, mkdir, mkdtemp, readFile, rename, rm, stat, writeFile } from "node:fs/promises";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { sourceInventory } from "./source-snapshot.mjs";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const contract = join(root, "contracts/direct-escrow-v5");
const harness = join(root, "spikes/harness");
const artifact = join(root, "release/artifacts/fod_direct_escrow_v5.wasm");
const manifestPath = join(root, "release/DIRECT_V5_MANIFEST.json");
// The existing focused harness test deliberately reads this lowered module.
const testedWasm = join(contract, "target/fod_direct_escrow_v5.lowered.wasm");
const targetWasm = join(contract, "target/wasm32-unknown-unknown/release/fod_direct_escrow_v5.wasm");
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
const run = (command, args, cwd = root, env = process.env) => execFileSync(command, args,
  { cwd, env, encoding: "utf8", maxBuffer: 64 * 1024 * 1024 }).trim();
const inputPaths = {
  contract: "contracts/direct-escrow-v5/src/lib.rs",
  state: "contracts/direct-escrow-v5/src/state.rs",
  manifest: "contracts/direct-escrow-v5/Cargo.toml",
  lockfile: "contracts/direct-escrow-v5/Cargo.lock",
  verifier: "contracts/escrow/src/verifier.rs",
  fastContentReceipt: "proofs/fast-content/dist/synthetic-v5-one-flip.json",
  syntheticSession: "proofs/fast-content/dist/synthetic-session.json",
  syntheticSecrets: "proofs/registration/vectors/blob-flip-v1.json",
  cliImagePin: "tools/fod/direct_v5_start.go",
  cliCodePin: "tools/fod/direct_commands.go",
  runtimeTest: "spikes/harness/direct_escrow_v5_test.go",
};
const inputHashes = async () => Object.fromEntries(await Promise.all(
  Object.entries(inputPaths).map(async ([label, path]) => [label, hash(await readFile(join(root, path)))])));
const harnessEnv = { ...process.env, GOFLAGS: "-modfile=go.local.mod", GOTOOLCHAIN: "auto", GOPROXY: "off", GOWORK: "off" };

const contractNumber = async (name) => {
  const source = await readFile(join(root, inputPaths.contract), "utf8");
  const match = source.match(new RegExp(`const ${name}: u(?:64|128) = ([0-9_]+);`));
  if (!match) throw new Error(`missing ${name} in v5 contract`);
  const number = Number(match[1].replaceAll("_", ""));
  if (!Number.isSafeInteger(number) || number <= 0) throw new Error(`invalid ${name} in v5 contract`);
  return number;
};

const fastContentImage = async () => {
  const verifier = await readFile(join(root, inputPaths.contract), "utf8");
  const cli = await readFile(join(root, inputPaths.cliImagePin), "utf8");
  const receipt = JSON.parse(await readFile(join(root, inputPaths.fastContentReceipt), "utf8"));
  const declaration = verifier.match(/pub const FAST_CONTENT_V5_IMAGE_ID: \[u8; 32\] = \[([\s\S]*?)\]/);
  const contractImage = [...(declaration?.[1] ?? "").matchAll(/0x([0-9a-fA-F]{2})/g)]
    .map((match) => match[1].toLowerCase()).join("");
  const cliImage = cli.match(/fastContentV5ImageIDHex\s*=\s*"([0-9a-fA-F]{64})"/)?.[1]?.toLowerCase();
  if (receipt.format !== "fod:fast-content-sealed:v1" || receipt.statement !== "fod:fast-content:v1" ||
      contractImage.length !== 64 || contractImage !== cliImage || contractImage !== receipt.image_id ||
      !/^[0-9a-f]{546}$/i.test(receipt.journal) || !/^[0-9a-f]{512}$/i.test(receipt.seal)) {
    throw new Error("v5 fast-content fixture, verifier, and CLI image pins differ or the sealed fixture is incomplete");
  }
  return contractImage;
};

const runtimeBinding = () => {
  const canonical = JSON.parse(run("go", ["mod", "edit", "-json"], harness, { ...harnessEnv, GOFLAGS: "" }));
  const local = JSON.parse(run("go", ["mod", "edit", "-json", "-modfile=go.local.mod"], harness,
    { ...harnessEnv, GOFLAGS: "" }));
  const replacements = local.Replace ?? [];
  const dependencies = (graph) => (graph.Require ?? [])
    .map(({ Path, Version }) => `${Path}@${Version}`).sort();
  if (JSON.stringify(dependencies(local)) !== JSON.stringify(dependencies(canonical))) {
    throw new Error("local harness dependency versions differ from the tracked graph");
  }
  local.Require = null;
  canonical.Require = null;
  local.Replace = null;
  if (canonical.Replace !== null || replacements.length !== 1 ||
      replacements[0].Old.Path !== "github.com/idena-network/idena-wasm-binding" ||
      replacements[0].Old.Version ||
      replacements[0].New.Path !== "github.com/ubiubi18/idena-wasm-binding" ||
      replacements[0].New.Version !== "v0.0.0-20260710092321-f7c71ce32d86" ||
      JSON.stringify(local) !== JSON.stringify(canonical)) {
    throw new Error("local harness module graph differs from the tracked graph");
  }
  const resolved = JSON.parse(run("go", ["list", "-m", "-json", "github.com/idena-network/idena-wasm-binding"],
    harness, harnessEnv));
  if (resolved.Replace?.Path !== replacements[0].New.Path ||
      resolved.Replace?.Version !== replacements[0].New.Version ||
      resolved.Replace?.Sum !== "h1:QCLJkNtKcHiZ6UGeMCsSEgrzQ4UbfGsQSFlrX3On6fs=") {
    throw new Error("pinned Idena WASM runtime binding is unavailable or modified");
  }
  if (run("go", ["mod", "verify"], harness, harnessEnv) !== "all modules verified") {
    throw new Error("pinned Idena WASM runtime modules did not verify");
  }
  return "f7c71ce32d86c8ed26e74935265c95768b0230d0";
};

const measurement = (output, name) => {
  const match = output.match(new RegExp(`v5 ${name}: wasm gas=(\\d+) (?:wasm|(?:exact )?payload) bytes=(\\d+) conservative tx gas=(\\d+)`));
  if (!match) throw new Error(`missing v5 ${name} runtime measurement`);
  const [wasmGas, payloadBytes, conservativeTransactionGas] = match.slice(1).map(Number);
  const calculated = Math.ceil(wasmGas / 100) + (payloadBytes + 2048) * 10;
  if (![wasmGas, payloadBytes, conservativeTransactionGas, calculated].every(Number.isSafeInteger) ||
      wasmGas <= 0 || payloadBytes <= 0 || conservativeTransactionGas !== calculated) {
    throw new Error(`invalid v5 ${name} runtime measurement`);
  }
  return { wasmGas, payloadBytes, conservativeTransactionGas };
};

const exists = async (path) => {
  try { return (await stat(path)).isFile(); }
  catch (error) { if (error.code === "ENOENT") return false; throw error; }
};

async function promotePair(stagedArtifact, stagedManifest, stageDir) {
  const oldArtifact = await exists(artifact);
  const oldManifest = await exists(manifestPath);
  if (oldArtifact !== oldManifest) throw new Error("existing v5 artifact and manifest are incomplete");
  const backup = join(stageDir, "previous.wasm");
  if (oldArtifact) await copyFile(artifact, backup);
  await rename(stagedArtifact, artifact);
  try {
    await rename(stagedManifest, manifestPath);
  } catch (error) {
    if (oldArtifact) await rename(backup, artifact);
    else await rm(artifact, { force: true });
    throw error;
  }
}

let stageDir;
try {
  const source = sourceInventory(root);
  const before = await inputHashes();
  const imageId = await fastContentImage();
  const bindingRevision = runtimeBinding();
  // The fast gate reproduces the guest twice from clean source locations and
  // requires its image to match the sealed fixture, verifier, and Go CLI pin.
  run("node", ["release/check-proof-builds.mjs", "--fast-v5-only"]);
  if (sourceInventory(root).sha256 !== source.sha256) throw new Error("source changed during fast guest build");

  run("cargo", ["build", "--locked", "--offline", "--release", "--target", "wasm32-unknown-unknown"], contract);
  await mkdir(dirname(testedWasm), { recursive: true });
  run("wasm-opt", ["--llvm-memory-copy-fill-lowering", "-O2", targetWasm, "-o", testedWasm]);
  const code = await readFile(testedWasm);
  const codePinSource = await readFile(join(root, inputPaths.cliCodePin), "utf8");
  const pinnedCodeHash = codePinSource.match(/directV5CodeHashHex\s*=\s*"([0-9a-fA-F]{64})"/)?.[1]?.toLowerCase();
  const measuredCodeHash = run("openssl", ["dgst", "-keccak-256", testedWasm])
    .match(/=\s*([0-9a-fA-F]{64})$/)?.[1]?.toLowerCase();
  if (!pinnedCodeHash || pinnedCodeHash !== measuredCodeHash) {
    throw new Error("v5 CLI code hash pin differs from the lowered candidate module");
  }
  const exports = WebAssembly.Module.exports(new WebAssembly.Module(code)).map((entry) => entry.name).sort();
  const expected = ["memory", "allocate", "deploy", "list", "buy", "confirm", "startFallback", "proveAndReveal", "refund"].sort();
  if (JSON.stringify(exports) !== JSON.stringify(expected)) throw new Error("v5 contract export surface changed");
  if (/memory\.(copy|fill)|f32\.|f64\./.test(run("wasm-objdump", ["-d", testedWasm]))) {
    throw new Error("v5 contract uses forbidden WASM operators");
  }

  await mkdir(dirname(artifact), { recursive: true });
  stageDir = await mkdtemp(join(dirname(artifact), ".direct-v5-"));
  const stagedArtifact = join(stageDir, "fod_direct_escrow_v5.wasm");
  const stagedManifest = join(stageDir, "DIRECT_V5_MANIFEST.json");
  await copyFile(testedWasm, stagedArtifact);
  const tests = run("go", ["test", "-count=1", "-run", "^TestDirectV5SyntheticFallbackRuntimeAndGas$", "-v", "."],
    harness, harnessEnv);
  if (!tests.includes("--- PASS: TestDirectV5SyntheticFallbackRuntimeAndGas")) {
    throw new Error("focused v5 runtime test did not report a pass");
  }
  const deploy = measurement(tests, "deploy");
  const list = measurement(tests, "list");
  const buy = measurement(tests, "buy");
  const confirm = measurement(tests, "confirm");
  const startFallback = measurement(tests, "startFallback");
  const proveAndReveal = measurement(tests, "proveAndReveal");
  const refund = measurement(tests, "refund");
  if (deploy.payloadBytes !== code.length || hash(await readFile(testedWasm)) !== hash(code) ||
      hash(await readFile(stagedArtifact)) !== hash(code)) {
    throw new Error("v5 runtime tested different module bytes from the staged artifact");
  }
  const blockGasLimit = 5_120_000;
  if ([deploy, list, buy, confirm, startFallback, proveAndReveal, refund]
    .some((entry) => entry.conservativeTransactionGas >= blockGasLimit)) {
    throw new Error("a v5 transaction exceeds the one-block gas budget");
  }
  const conservativeFallbackGas = startFallback.conservativeTransactionGas + proveAndReveal.conservativeTransactionGas;
  const buyerBond = {
    gasBudget: await contractNumber("BOND_GAS_BUDGET"),
    feeMargin: await contractNumber("BOND_FEE_MARGIN"),
    reimbursementGasBudget: await contractNumber("FALLBACK_REIMBURSE_GAS_BUDGET"),
    sellerStakeGasBudget: await contractNumber("SELLER_STAKE_GAS_BUDGET"),
  };
  if (buyerBond.gasBudget < buyerBond.reimbursementGasBudget || buyerBond.feeMargin < 2 ||
      buyerBond.sellerStakeGasBudget !== 100_000 ||
      !Number.isSafeInteger(conservativeFallbackGas) || conservativeFallbackGas > buyerBond.reimbursementGasBudget) {
    throw new Error("v5 buyer bond does not cover the measured proof-backed start and reveal");
  }
  const refundBoundaryBlocks = await contractNumber("PROOF_WINDOW_BLOCKS");
  const confirmGraceBlocks = await contractNumber("BUYER_CONFIRM_GRACE_BLOCKS");
  const deadlineMarginBlocks = await contractNumber("PROOF_DEADLINE_MARGIN_BLOCKS");
  if (confirmGraceBlocks + deadlineMarginBlocks >= refundBoundaryBlocks) {
    throw new Error("v5 fallback grace and deadline margin leave no proof window");
  }
  const manifest = {
    format: "fod.direct-candidate-manifest", version: 5,
    artifact: { path: relative(root, artifact), bytes: code.length, sha256: hash(code), codeHash: measuredCodeHash },
    source: before,
    build: {
      rust: run("cargo", ["--version"]), binaryen: run("wasm-opt", ["--version"]),
      command: "cargo build --locked --offline --release --target wasm32-unknown-unknown; wasm-opt --llvm-memory-copy-fill-lowering -O2 target/wasm32-unknown-unknown/release/fod_direct_escrow_v5.wasm -o target/fod_direct_escrow_v5.lowered.wasm",
    },
    checks: {
      sourceOnlyGuest: "two clean builds with identical image and ELF; image matches verifier, CLI, and sealed fixture",
      runtime: `pinned idena-wasm-binding ${bindingRevision}`,
      syntheticSealedFallback: "genuine fast-content receipt and four-leaf session accepted in the pinned Idena WASM runtime",
      fastContentImageId: imageId, exports, forbiddenOperators: 0,
      deploy, list, buy, confirm, startFallback, proveAndReveal, refund, conservativeFallbackGas, buyerBond,
      confirmGraceBlocks, deadlineMarginBlocks, refundBoundaryBlocks, blockGasLimit,
    },
    deployment: { network: null, contract: null,
      note: "Experimental candidate only. Gas measurements use synthetic confirmation, proof-backed reveal, and pending-order refund paths in a pinned runtime. They do not establish a real seller proof, live inclusion, or funded-trade qualification." },
  };
  await writeFile(stagedManifest, `${JSON.stringify(manifest, null, 2)}\n`, { flag: "wx" });
  if (runtimeBinding() !== bindingRevision || sourceInventory(root).sha256 !== source.sha256 ||
      JSON.stringify(await inputHashes()) !== JSON.stringify(before)) {
    throw new Error("v5 source or pinned runtime changed during candidate build");
  }
  await promotePair(stagedArtifact, stagedManifest, stageDir);
  console.log(`direct v5 candidate built: ${code.length} bytes; deploy ${deploy.conservativeTransactionGas}, combined proof-backed fallback ${conservativeFallbackGas} conservative gas`);
} finally {
  if (stageDir) await rm(stageDir, { recursive: true, force: true });
}
