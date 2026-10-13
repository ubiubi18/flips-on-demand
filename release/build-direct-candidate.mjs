// Rebuild the proof-backed direct-sale module without touching deployed
// artifacts or manifests. A failed build leaves the previous v4 pair intact.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFile, rename, rm, writeFile } from "node:fs/promises";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const contract = join(root, "contracts/direct-escrow");
const harness = join(root, "spikes/harness");
const artifact = join(root, "release/artifacts/fod_direct_escrow_v4.wasm");
const staged = `${artifact}.staged`;
const manifestPath = join(root, "release/DIRECT_V4_MANIFEST.json");
const stagedManifest = `${manifestPath}.staged`;
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
const run = (command, args, cwd = root, env = process.env) => execFileSync(command, args,
  { cwd, env, encoding: "utf8", maxBuffer: 64 * 1024 * 1024 }).trim();
const inputPaths = {
  contract: "contracts/direct-escrow/src/lib.rs",
  state: "contracts/direct-escrow/src/state.rs",
  manifest: "contracts/direct-escrow/Cargo.toml",
  lockfile: "contracts/direct-escrow/Cargo.lock",
  registrationVerifier: "contracts/escrow/src/verifier.rs",
  cliProofPin: "tools/fod/receipt_claim.go",
  syntheticProof: "proofs/registration-v2/dist/direct-v2-synthetic.json",
};
const inputHashes = async () => Object.fromEntries(await Promise.all(
  Object.entries(inputPaths).map(async ([label, path]) => [label, hash(await readFile(join(root, path)))])));
const harnessEnv = { ...process.env, GOPROXY: "off", GOWORK: "off" };

const contractBondPolicy = async () => {
  const source = await readFile(join(root, inputPaths.contract), "utf8");
  const read = (name) => {
    const match = source.match(new RegExp(`const ${name}: u128 = ([0-9_]+);`));
    if (!match) throw new Error(`missing ${name} in direct contract`);
    return Number(match[1].replaceAll("_", ""));
  };
  return { gasBudget: read("BOND_GAS_BUDGET"), feeMargin: read("BOND_FEE_MARGIN"),
    reimbursementGasBudget: read("FALLBACK_REIMBURSE_GAS_BUDGET"),
    sellerStakeGasBudget: read("SELLER_STAKE_GAS_BUDGET") };
};

const directProofImage = async () => {
  const contractSource = await readFile(join(root, inputPaths.contract), "utf8");
  const goSource = await readFile(join(root, "tools/fod/receipt_claim.go"), "utf8");
  const fixture = JSON.parse(await readFile(join(root, "proofs/registration-v2/dist/direct-v2-synthetic.json"), "utf8"));
  const declaration = contractSource.match(/const DIRECT_V2_IMAGE_ID: \[u8; 32\] = \[([\s\S]*?)\]/);
  const contractImage = [...(declaration?.[1] ?? "").matchAll(/0x([0-9a-fA-F]{2})/g)]
    .map((match) => match[1].toLowerCase()).join("");
  const cliImage = (goSource.match(/const directV2ImageIDHex = "([0-9a-f]{64})"/) ?? [])[1];
  if (contractImage.length !== 64 || contractImage !== cliImage || contractImage !== fixture.image_id) {
    throw new Error("direct proof fixture, contract, and CLI image pins differ");
  }
  return contractImage;
};

try {
  const before = await inputHashes();
  run("node", ["release/check-proof-builds.mjs", "--direct-only"]);
  const registrationImageId = await directProofImage();
  const binding = run("go", ["list", "-modfile=go.local.mod", "-m", "-f", "{{.Dir}}",
    "github.com/idena-network/idena-wasm-binding"], harness, harnessEnv);
  const bindingRevision = run("git", ["-C", binding, "rev-parse", "HEAD"]);
  if (bindingRevision !== "f7c71ce32d86c8ed26e74935265c95768b0230d0" ||
      run("git", ["-C", binding, "status", "--porcelain=v1", "--untracked-files=all"]) !== "") {
    throw new Error("pinned Idena WASM runtime binding is unavailable or modified");
  }
  run("cargo", ["build", "--locked", "--offline", "--release", "--target", "wasm32-unknown-unknown"], contract);
  run("wasm-opt", ["--llvm-memory-copy-fill-lowering", "-O2",
    join(contract, "target/wasm32-unknown-unknown/release/fod_direct_escrow.wasm"), "-o", staged]);
  const code = await readFile(staged);
  const exports = WebAssembly.Module.exports(new WebAssembly.Module(code)).map((entry) => entry.name).sort();
  const expected = ["memory", "allocate", "deploy", "list", "buy", "confirm", "startFallback", "proveAndReveal", "refund"].sort();
  if (JSON.stringify(exports) !== JSON.stringify(expected)) throw new Error("direct contract export surface changed");
  if (/memory\.(copy|fill)|f32\.|f64\./.test(run("wasm-objdump", ["-d", staged]))) {
    throw new Error("direct contract uses forbidden WASM operators");
  }
  const tests = run("go", ["test", "-mod=readonly", "-modfile=go.local.mod", "-run", "^TestDirect",
    "-count=1", "-v", "."], harness, { ...harnessEnv, FOD_DIRECT_PRODUCTION_WASM: staged });
  const measure = (pattern, label) => {
    const value = tests.match(pattern);
    if (!value) throw new Error(`missing ${label} runtime measurement`);
    return Number(value[1]);
  };
  const confirm = measure(/direct confirm wasm gas=(\d+)/, "cooperative confirmation");
  const fallbackStart = measure(/direct sealed fallback start wasm gas=(\d+)/, "proof-backed fallback start");
  const keyReveal = measure(/direct key reveal wasm gas=(\d+)/, "public key reveal");
  const deploy = measure(/direct deploy wasm gas=(\d+) bytes=\d+/, "deployment");
  const deployBytes = measure(/direct deploy wasm gas=\d+ bytes=(\d+)/, "deployed bytes");
  if (deployBytes !== code.length) throw new Error("runtime tested different direct module bytes");
  const blockGasLimit = 5_120_000;
  const conservativeTransactionGas = Math.ceil(deploy / 100) + (code.length + 2048) * 10;
  const conservativeFallbackStartGas = Math.ceil(fallbackStart / 100) + (8 + 326 + 256 + 512) * 10;
  const conservativeKeyRevealGas = Math.ceil(keyReveal / 100) + (8 + 106 + 512) * 10;
  const conservativeFallbackGas = conservativeFallbackStartGas + conservativeKeyRevealGas;
  const buyerBond = await contractBondPolicy();
  if (!Number.isSafeInteger(buyerBond.gasBudget) || !Number.isSafeInteger(buyerBond.feeMargin) ||
      !Number.isSafeInteger(buyerBond.reimbursementGasBudget) || buyerBond.sellerStakeGasBudget !== 100_000 ||
      buyerBond.reimbursementGasBudget < conservativeFallbackGas ||
      buyerBond.gasBudget <= buyerBond.reimbursementGasBudget || buyerBond.feeMargin < 2) {
    throw new Error("the capped seller reimbursement no longer covers the measured direct fallback with a buyer bond margin");
  }
  if (conservativeTransactionGas >= blockGasLimit ||
      conservativeFallbackStartGas >= blockGasLimit || conservativeKeyRevealGas >= blockGasLimit) {
    throw new Error("a direct contract transaction exceeds the one-block budget");
  }
  if (JSON.stringify(await inputHashes()) !== JSON.stringify(before)) throw new Error("direct source changed during build");
  const manifest = {
    format: "fod.direct-candidate-manifest", version: 4,
    artifact: { path: relative(root, artifact), bytes: code.length, sha256: hash(code) },
    source: before,
    build: {
      rust: run("cargo", ["--version"]), binaryen: run("wasm-opt", ["--version"]),
      command: "cargo build --locked --offline --release --target wasm32-unknown-unknown; wasm-opt --llvm-memory-copy-fill-lowering -O2 target/wasm32-unknown-unknown/release/fod_direct_escrow.wasm -o ../../release/artifacts/fod_direct_escrow_v4.wasm",
    },
    checks: {
      runtime: `pinned idena-wasm-binding ${bindingRevision}`,
      cooperativeConfirmation: { wasmGas: confirm },
      syntheticSealedFallbackStart: { wasmGas: fallbackStart, conservativeTransactionGas: conservativeFallbackStartGas, genuineRegistrationV2: true },
      publicKeyReveal: { wasmGas: keyReveal, conservativeTransactionGas: conservativeKeyRevealGas },
      registrationImageId,
      buyerBond,
      refundBoundaryBlocks: 8640,
      deploy: { wasmGas: deploy, conservativeTransactionGas },
      conservativeFallbackGas, blockGasLimit,
    },
    deployment: { network: null, contract: null,
      note: "Experimental direct sale. Local synthetic proof gas and behavior do not establish live proof availability, chain inclusion, or guaranteed payment after a censored public fallback." },
  };
  await writeFile(stagedManifest, `${JSON.stringify(manifest, null, 2)}\n`);
  await rename(staged, artifact);
  await rename(stagedManifest, manifestPath);
  console.log(`direct v4 candidate built: ${code.length} bytes; deploy ${conservativeTransactionGas} conservative gas; proof-backed start ${conservativeFallbackStartGas}, key reveal ${conservativeKeyRevealGas}, combined proof/reveal ${conservativeFallbackGas} conservative gas`);
} catch (error) {
  await rm(staged, { force: true });
  await rm(stagedManifest, { force: true });
  throw error;
}
