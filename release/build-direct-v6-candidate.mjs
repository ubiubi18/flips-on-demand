// Produce an offline, synthetic-only v6 candidate. This does not qualify a
// real seller proof, a funded trade, or deployment on any network.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { copyFile, mkdir, mkdtemp, readFile, rename, rm, stat, writeFile } from "node:fs/promises";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { sourceInventory } from "./source-snapshot.mjs";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const contract = join(root, "contracts/direct-escrow-v6");
const harness = join(root, "spikes/harness");
const artifact = join(root, "release/artifacts/fod_direct_escrow_v6.wasm");
const manifestPath = join(root, "release/DIRECT_V6_MANIFEST.json");
// The focused harness loads these exact lowered bytes.
const testedWasm = join(contract, "target/fod_direct_escrow_v6.lowered.wasm");
const sha256 = (bytes) => createHash("sha256").update(bytes).digest("hex");
const offline = { ...process.env, CARGO_NET_OFFLINE: "true", GOPROXY: "off", GOWORK: "off" };
const run = (command, args, cwd = root, env = offline) => execFileSync(command, args,
  { cwd, env, encoding: "utf8", maxBuffer: 64 * 1024 * 1024 }).trim();

const inputs = {
  contract: "contracts/direct-escrow-v6/src/lib.rs",
  curve: "contracts/direct-escrow-v6/src/curve.rs",
  state: "contracts/direct-escrow-v6/src/state.rs",
  cargoManifest: "contracts/direct-escrow-v6/Cargo.toml",
  cargoLock: "contracts/direct-escrow-v6/Cargo.lock",
  verifier: "contracts/escrow/src/verifier.rs",
  fastGuest: "proofs/fast-content/methods/guest/src/main.rs",
  fastReceipt: "proofs/fast-content/dist/synthetic-v5-one-flip.json",
  syntheticSession: "proofs/fast-content/dist/synthetic-session.json",
  syntheticFlipVector: "proofs/registration/vectors/blob-flip-v1.json",
  v5CliImagePin: "tools/fod/direct_v5_start.go",
  runtimeTest: "spikes/harness/direct_escrow_v6_test.go",
  runtimeModule: "spikes/harness/go.mod",
  runtimeSums: "spikes/harness/go.sum",
  proofBuilder: "release/check-proof-builds.mjs",
  builder: "release/build-direct-v6-candidate.mjs",
};
const inputHashes = async () => Object.fromEntries(await Promise.all(Object.entries(inputs)
  .map(async ([name, path]) => [name, sha256(await readFile(join(root, path)))])));

const contractNumber = async (name) => {
  const source = await readFile(join(root, inputs.contract), "utf8");
  const match = source.match(new RegExp(`const ${name}: u(?:64|128) = ([0-9_]+);`));
  if (!match) throw new Error(`missing v6 ${name}`);
  const value = Number(match[1].replaceAll("_", ""));
  if (!Number.isSafeInteger(value) || value <= 0) throw new Error(`invalid v6 ${name}`);
  return value;
};

const imageId = async () => {
  const source = await readFile(join(root, inputs.contract), "utf8");
  const harnessSource = await readFile(join(root, inputs.runtimeTest), "utf8");
  const cli = await readFile(join(root, inputs.v5CliImagePin), "utf8");
  const receipt = JSON.parse(await readFile(join(root, inputs.fastReceipt), "utf8"));
  const declaration = source.match(/pub const FAST_CONTENT_V6_IMAGE_ID: \[u8; 32\] = \[([\s\S]*?)\]/)?.[1] ?? "";
  const bytes = [...declaration.matchAll(/0x([0-9a-fA-F]{2})/g)];
  const contractPin = bytes.map((entry) => entry[1].toLowerCase()).join("");
  const harnessPin = harnessSource.match(/directV6FastContentImageID\s*=\s*"([0-9a-fA-F]{64})"/)?.[1]?.toLowerCase();
  const cliPin = cli.match(/fastContentV5ImageIDHex\s*=\s*"([0-9a-fA-F]{64})"/)?.[1]?.toLowerCase();
  if (bytes.length !== 32 || contractPin !== harnessPin || contractPin !== cliPin ||
      receipt.format !== "fod:fast-content-sealed:v1" || receipt.statement !== "fod:fast-content:v1" ||
      contractPin !== receipt.image_id || !/^[0-9a-f]{546}$/i.test(receipt.journal) ||
      !/^[0-9a-f]{512}$/i.test(receipt.seal)) {
    throw new Error("v6 image pin or sealed synthetic fixture is missing or inconsistent");
  }
  return contractPin;
};

// Build a temporary version of the tracked harness graph with the exact fork
// whose WASM runtime was measured for v5. No checkout-local replace is trusted.
async function pinnedRuntime(stageDir) {
  const modfile = join(stageDir, "harness.mod");
  const mod = await readFile(join(harness, "go.mod"), "utf8");
  const sum = await readFile(join(harness, "go.sum"), "utf8");
  const required = "github.com/idena-network/idena-wasm-binding";
  const version = "v0.0.0-20260710092321-f7c71ce32d86";
  const replacement = "github.com/ubiubi18/idena-wasm-binding";
  const moduleSum = "h1:QCLJkNtKcHiZ6UGeMCsSEgrzQ4UbfGsQSFlrX3On6fs=";
  const goModSum = "h1:Gaqs/PYOTw9Jr1VQPlR0Be+CcgsuP4sdGmTzzQ5hvQ0=";
  if (/^replace\s/m.test(mod) || !mod.includes(`${required} ${version}`)) {
    throw new Error("tracked v6 harness runtime graph changed");
  }
  await writeFile(modfile, `${mod.trimEnd()}\n\nreplace ${required} => ${replacement} ${version}\n`, { flag: "wx" });
  await writeFile(join(stageDir, "harness.sum"),
    `${sum.trimEnd()}\n${replacement} ${version} ${moduleSum}\n${replacement} ${version}/go.mod ${goModSum}\n`,
    { flag: "wx" });
  const canonical = JSON.parse(run("go", ["mod", "edit", "-json"], harness));
  const pinned = JSON.parse(run("go", ["mod", "edit", "-json", `-modfile=${modfile}`], harness));
  const dependencies = (graph) => (graph.Require ?? [])
    .map(({ Path, Version }) => `${Path}@${Version}`).sort();
  const replaces = pinned.Replace ?? [];
  if ((canonical.Replace ?? []).length !== 0 ||
      JSON.stringify(dependencies(pinned)) !== JSON.stringify(dependencies(canonical)) ||
      replaces.length !== 1 || replaces[0].Old.Path !== required || replaces[0].Old.Version ||
      replaces[0].New.Path !== replacement || replaces[0].New.Version !== version) {
    throw new Error("temporary v6 harness graph differs from its tracked pin");
  }
  const resolved = JSON.parse(run("go", ["list", "-m", "-json", `-modfile=${modfile}`, required], harness));
  if (resolved.Replace?.Path !== replacement || resolved.Replace?.Version !== version ||
      resolved.Replace?.Sum !== moduleSum ||
      run("go", ["mod", "verify", `-modfile=${modfile}`], harness) !== "all modules verified") {
    throw new Error("pinned Idena WASM runtime is unavailable or modified");
  }
  return { modfile, revision: "f7c71ce32d86c8ed26e74935265c95768b0230d0", moduleSum };
}

const measurement = (output, method) => {
  const match = output.match(new RegExp(`v6 ${method}: wasm gas=(\\d+) payload bytes=(\\d+) conservative tx gas=(\\d+)`));
  if (!match) throw new Error(`missing v6 ${method} runtime measurement`);
  const [wasmGas, payloadBytes, conservativeTransactionGas] = match.slice(1).map(Number);
  const calculated = Math.ceil(wasmGas / 100) + (payloadBytes + 2048) * 10;
  if (![wasmGas, payloadBytes, conservativeTransactionGas, calculated].every(Number.isSafeInteger) ||
      wasmGas <= 0 || payloadBytes <= 0 || conservativeTransactionGas !== calculated) {
    throw new Error(`invalid v6 ${method} runtime measurement`);
  }
  return { wasmGas, payloadBytes, conservativeTransactionGas };
};

async function fileExists(path) {
  try { return (await stat(path)).isFile(); }
  catch (error) { if (error.code === "ENOENT") return false; throw error; }
}

async function promotePair(stagedArtifact, stagedManifest, stageDir) {
  const oldArtifact = await fileExists(artifact);
  const oldManifest = await fileExists(manifestPath);
  if (oldArtifact !== oldManifest) throw new Error("existing v6 artifact and manifest are incomplete");
  const backup = join(stageDir, "previous.wasm");
  if (oldArtifact) await copyFile(artifact, backup);
  await rename(stagedArtifact, artifact);
  try { await rename(stagedManifest, manifestPath); }
  catch (error) {
    if (oldArtifact) await rename(backup, artifact);
    else await rm(artifact, { force: true });
    throw error;
  }
}

let stageDir;
try {
  const source = sourceInventory(root);
  const before = await inputHashes();
  const fastContentImageId = await imageId();
  await mkdir(dirname(artifact), { recursive: true });
  stageDir = await mkdtemp(join(dirname(artifact), ".direct-v6-"));
  const runtime = await pinnedRuntime(stageDir);
  // Two clean, source-only guest builds must reproduce the image in the
  // sealed fixture; the focused v6 runtime test verifies its Groth16 seal.
  run("node", ["release/check-proof-builds.mjs", "--fast-v5-only"]);
  if (sourceInventory(root).sha256 !== source.sha256) throw new Error("source changed during fast guest build");

  const contractTarget = join(stageDir, "contract-target");
  const targetWasm = join(contractTarget, "wasm32-unknown-unknown/release/fod_direct_escrow_v6.wasm");
  run("cargo", ["build", "--locked", "--offline", "--release", "--target", "wasm32-unknown-unknown"],
    contract, { ...offline, CARGO_TARGET_DIR: contractTarget });
  await mkdir(dirname(testedWasm), { recursive: true });
  run("wasm-opt", ["--llvm-memory-copy-fill-lowering", "-O2", targetWasm, "-o", testedWasm]);
  const code = await readFile(testedWasm);
  const codeHash = run("openssl", ["dgst", "-keccak-256", testedWasm])
    .match(/=\s*([0-9a-fA-F]{64})$/)?.[1]?.toLowerCase();
  if (!codeHash) throw new Error("cannot measure v6 Keccak-256 code hash");
  const exports = WebAssembly.Module.exports(new WebAssembly.Module(code)).map((entry) => entry.name).sort();
  const expected = ["memory", "allocate", "deploy", "activate", "qualify", "proveBundle", "slashUnproved",
    "seedBundle", "list", "relist", "cancel", "topUpBond", "buy", "buyPublicSeed", "challenge",
    "confirm", "startFallback", "proveAndReveal", "refund"].sort();
  if (JSON.stringify(exports) !== JSON.stringify(expected)) throw new Error("v6 contract export surface changed");
  if (/memory\.(copy|fill)|f32\.|f64\./.test(run("wasm-objdump", ["-d", testedWasm]))) {
    throw new Error("v6 contract uses forbidden WASM operators");
  }

  const stagedArtifact = join(stageDir, "fod_direct_escrow_v6.wasm");
  const stagedManifest = join(stageDir, "DIRECT_V6_MANIFEST.json");
  await copyFile(testedWasm, stagedArtifact);
  const tests = run("go", ["test", `-modfile=${runtime.modfile}`, "-mod=readonly", "-count=1",
    "-run", "^TestDirectV6SealedSellerFallbackGas$", "-v", "."], harness);
  if (!tests.includes("--- PASS: TestDirectV6SealedSellerFallbackGas")) {
    throw new Error("focused v6 sealed runtime test did not report a pass");
  }
  const measured = Object.fromEntries(["deploy", "activate", "qualify", "list", "buy", "startFallback", "proveAndReveal"]
    .map((method) => [method, measurement(tests, method)]));
  if (measured.deploy.payloadBytes !== code.length || sha256(await readFile(testedWasm)) !== sha256(code) ||
      sha256(await readFile(stagedArtifact)) !== sha256(code)) {
    throw new Error("v6 runtime tested different bytes from the staged module");
  }
  const blockGasLimit = 5_120_000;
  if (Object.values(measured).some((entry) => entry.conservativeTransactionGas >= blockGasLimit)) {
    throw new Error("a v6 transaction exceeds the one-block gas budget");
  }
  const conservativeFallbackGas = measured.startFallback.conservativeTransactionGas +
    measured.proveAndReveal.conservativeTransactionGas;
  const buyerBond = {
    gasBudget: await contractNumber("BOND_GAS_BUDGET"),
    feeMargin: await contractNumber("BOND_FEE_MARGIN"),
    reimbursementGasBudget: await contractNumber("FALLBACK_REIMBURSE_GAS_BUDGET"),
  };
  const sellerBond = {
    proofGasBudget: await contractNumber("SELLER_PROOF_GAS_BUDGET"),
    feeMargin: await contractNumber("SELLER_BOND_FEE_MARGIN"),
  };
  if (buyerBond.gasBudget < buyerBond.reimbursementGasBudget || buyerBond.feeMargin < 2 ||
      sellerBond.proofGasBudget < buyerBond.reimbursementGasBudget || sellerBond.feeMargin < 2 ||
      !Number.isSafeInteger(conservativeFallbackGas) || conservativeFallbackGas > buyerBond.reimbursementGasBudget) {
    throw new Error("v6 bonds do not cover the measured sealed fallback gas allowance");
  }
  const refundBoundaryBlocks = await contractNumber("PROOF_WINDOW_BLOCKS");
  const confirmGraceBlocks = await contractNumber("BUYER_CONFIRM_GRACE_BLOCKS");
  const deadlineMarginBlocks = await contractNumber("PROOF_DEADLINE_MARGIN_BLOCKS");
  if (confirmGraceBlocks + deadlineMarginBlocks >= refundBoundaryBlocks) {
    throw new Error("v6 fallback grace and deadline margin leave no proof window");
  }
  const manifest = {
    format: "fod.direct-candidate-manifest", version: 6, status: "candidate-only",
    artifact: { path: relative(root, artifact), bytes: code.length, sha256: sha256(code), codeHash },
    source: { inventorySha256: source.sha256, files: before },
    build: {
      rust: run("cargo", ["--version"]), binaryen: run("wasm-opt", ["--version"]),
      go: run("go", ["version"], harness),
      command: "CARGO_TARGET_DIR=<empty temporary directory> cargo build --locked --offline --release --target wasm32-unknown-unknown; wasm-opt --llvm-memory-copy-fill-lowering -O2 <target directory>/wasm32-unknown-unknown/release/fod_direct_escrow_v6.wasm -o target/fod_direct_escrow_v6.lowered.wasm",
    },
    checks: {
      sourceOnlyGuest: "two clean source builds with identical image and ELF; image matches v6 contract, v6 harness, v5 CLI and sealed fixture",
      runtime: `pinned idena-wasm-binding ${runtime.revision}`,
      runtimeModuleSum: runtime.moduleSum,
      sealedSyntheticFallback: "genuine fast-content receipt and four-leaf session accepted in the pinned Idena WASM runtime",
      fastContentImageId, exports, forbiddenOperators: 0, ...measured, conservativeFallbackGas,
      buyerBond, sellerBond, confirmGraceBlocks, deadlineMarginBlocks, refundBoundaryBlocks, blockGasLimit,
    },
    deployment: { network: null, contract: null,
      note: "Experimental candidate only. Synthetic measurements do not establish a real seller proof, live inclusion, deployment qualification, or funded-trade qualification." },
  };
  await writeFile(stagedManifest, `${JSON.stringify(manifest, null, 2)}\n`, { flag: "wx" });
  await pinnedRuntimeVerification(runtime.modfile);
  if (sourceInventory(root).sha256 !== source.sha256 ||
      JSON.stringify(await inputHashes()) !== JSON.stringify(before)) {
    throw new Error("v6 source changed during candidate build");
  }
  await promotePair(stagedArtifact, stagedManifest, stageDir);
  console.log(`direct v6 candidate built: ${code.length} bytes; deploy ${measured.deploy.conservativeTransactionGas}, activate ${measured.activate.conservativeTransactionGas}, combined sealed fallback ${conservativeFallbackGas} conservative gas`);
} finally {
  if (stageDir) await rm(stageDir, { recursive: true, force: true });
}

async function pinnedRuntimeVerification(modfile) {
  const required = "github.com/idena-network/idena-wasm-binding";
  const resolved = JSON.parse(run("go", ["list", "-m", "-json", `-modfile=${modfile}`, required], harness));
  if (resolved.Replace?.Path !== "github.com/ubiubi18/idena-wasm-binding" ||
      resolved.Replace?.Version !== "v0.0.0-20260710092321-f7c71ce32d86" ||
      resolved.Replace?.Sum !== "h1:QCLJkNtKcHiZ6UGeMCsSEgrzQ4UbfGsQSFlrX3On6fs=" ||
      run("go", ["mod", "verify", `-modfile=${modfile}`], harness) !== "all modules verified") {
    throw new Error("pinned Idena WASM runtime changed during candidate build");
  }
}
