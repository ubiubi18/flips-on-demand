// Builds the release artefacts and writes release/MANIFEST.json.
//
//   node release/build-manifest.mjs
//
// The manifest is a record of what exists right now: the exact revision, the
// bytes of each artefact with their SHA-256, the proof-program identifiers the
// escrow verifies against, and the commands that produced them. It is
// regenerable, so a reader can rebuild and compare rather than take our word.
//
// It does not deploy anything. First guest builds may fetch the immutable
// local Docker builder and locked public dependencies; no hosted prover is used.

import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { sourceInventory } from "./source-snapshot.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..");
const artifactsDir = join(here, "artifacts");

function run(command, args, cwd) {
  return execFileSync(command, args, { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
}

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

async function describeArtifact(path) {
  const bytes = await readFile(path);
  return {
    path: relative(root, path).split("\\").join("/"),
    bytes: bytes.length,
    sha256: sha256(bytes),
  };
}

const source = sourceInventory(root);
const revision = run("git", ["rev-parse", "HEAD"], root);
// The manifest itself is written after this check, so it is excluded: what
// matters is whether the sources behind the artefacts were committed.
const dirty =
  run("git", ["status", "--porcelain=v1", "--", ":(exclude)release/MANIFEST.json"], root).length > 0;

// 1. The release escrow module: built without the test hooks, then lowered so
//    the gatekeeper accepts it.
const escrowDir = join(root, "contracts", "escrow");
const targetWasm = join(escrowDir, "target", "wasm32-unknown-unknown", "release", "fod_escrow.wasm");
run("cargo", ["build", "--locked", "--offline", "--release", "--target", "wasm32-unknown-unknown"], escrowDir);
await mkdir(artifactsDir, { recursive: true });
const releaseWasm = join(artifactsDir, "fod_escrow.wasm");
// Validate before replacing release outputs: the candidate module is staged
// next to the released one and is only renamed into place after every check
// below has passed. A failed check leaves the previous artefact untouched.
const stagedWasm = `${releaseWasm}.staged`;
const stagedHtml = join(artifactsDir, "flips-on-demand.html.staged");
const stagedManifest = join(here, "MANIFEST.json.staged");
run(
  "wasm-opt",
  ["--llvm-memory-copy-fill-lowering", "-O3", targetWasm, "-o", stagedWasm],
  root,
);

async function fail(message) {
  console.error(message);
  await rm(stagedWasm, { force: true });
  await rm(stagedHtml, { force: true });
  await rm(stagedManifest, { force: true });
  process.exit(1);
}

// 2. Gatekeeper constraints, checked rather than assumed.
const disassembler = spawnSync("wasm-objdump", ["-d", stagedWasm], {
  cwd: root,
  encoding: "utf8",
  // The disassembly of a 350 KB module runs to megabytes.
  maxBuffer: 64 * 1024 * 1024,
});
if (disassembler.error !== undefined || disassembler.status !== 0 || typeof disassembler.stdout !== "string") {
  await fail(
    "wasm-objdump is required to check the gatekeeper constraints; install wabt (Homebrew: wabt) and re-run",
  );
}
const forbiddenMatches = disassembler.stdout.match(/memory\.(copy|fill)|f32\.|f64\./g);
const forbiddenOperators = forbiddenMatches === null ? 0 : forbiddenMatches.length;
if (forbiddenOperators !== 0) {
  await fail(
    `the candidate module uses ${forbiddenOperators} operator(s) the gatekeeper refuses; refusing to replace the released artefact`,
  );
}

// 2b. The artefact that may be deployed must be the release build: the harness
// build adds seed hooks that can mint units and settle seeded orders, so the
// staged module's export section is checked before it can replace the released
// file. The full published surface is pinned by
// `spikes/harness/escrow_no_admin_test.go`; this gate keeps a wrong artefact
// from ever being promoted (`AH-23`).
function wasmExportNames(bytes) {
  let at = 8; // magic (4) + version (4)
  const readU32 = () => {
    let value = 0;
    let shift = 0;
    let byte;
    do {
      byte = bytes[at++];
      value |= (byte & 0x7f) << shift;
      shift += 7;
    } while (byte & 0x80);
    return value >>> 0;
  };
  const readName = () => {
    const length = readU32();
    const name = bytes.subarray(at, at + length).toString("utf8");
    at += length;
    return name;
  };
  while (at < bytes.length) {
    const id = bytes[at++];
    const size = readU32();
    const end = at + size;
    if (id === 7) {
      const count = readU32();
      const names = [];
      for (let index = 0; index < count; index += 1) {
        names.push(readName());
        at += 1; // external-kind byte
        readU32(); // export index
      }
      return names;
    }
    at = end;
  }
  return [];
}
const publishedExports = [
  "memory", "allocate", "deploy", "registerBundle", "addReserve", "withdrawReserve",
  "donateUnit", "sellUnit", "beacon", "poolView", "buy", "deliver", "confirm",
  "fulfill", "refund", "reveal", "orderPoolView", "withdraw", "accounting",
];
const stagedExports = wasmExportNames(await readFile(stagedWasm));
const testHookExports = stagedExports.filter((name) => name.startsWith("seed"));
const missingExports = publishedExports.filter((name) => !stagedExports.includes(name));
const unexpectedExports = stagedExports.filter((name) => !publishedExports.includes(name));
if (testHookExports.length !== 0 || missingExports.length !== 0 || unexpectedExports.length !== 0) {
  await fail(
    `the staged release module's export surface is wrong (test hooks: ${testHookExports.join(", ") || "none"}; missing: ${missingExports.join(", ") || "none"}; unexpected: ${unexpectedExports.join(", ") || "none"}); refusing to replace the released artefact`,
  );
}
console.log(`  release exports: ${stagedExports.length} published names, no test hooks`);

// 3. Proof-program identifiers: what the escrow verifies against.
const delivery = JSON.parse(
  await readFile(join(root, "proofs", "delivery", "dist", "delivery-v2.json"), "utf8"),
);
const registration = JSON.parse(
  await readFile(join(root, "proofs", "registration-v2", "dist", "registration-v2.json"), "utf8"),
);

// 3b. The identifiers the contract pins must be the ones the proved runs carry.
// A drift here is a receipt that can never verify, and finding it while building
// the manifest is cheaper than finding it on mainnet.
function pinnedImageId(source, name) {
  const declaration = source.match(
    new RegExp(`pub const ${name}: \\[u8; 32\\] = \\[([\\s\\S]*?)\\]`),
  );
  if (declaration === null) {
    return null;
  }
  const bytes = [...declaration[1].matchAll(/0x([0-9a-fA-F]{2})/g)].map((m) => m[1].toLowerCase());
  return bytes.length === 32 ? bytes.join("") : null;
}

const verifierSource = await readFile(
  join(root, "contracts", "escrow", "src", "verifier.rs"),
  "utf8",
);
const cliClaimSource = await readFile(join(root, "tools", "fod", "receipt_claim.go"), "utf8");
const cliRegistrationImageId =
  (cliClaimSource.match(/registrationImageIDHex = "([0-9a-f]{64})"/) ?? [])[1] ?? null;
const cliDeliveryImageId = (cliClaimSource.match(/deliveryImageIDHex = "([0-9a-f]{64})"/) ?? [])[1] ?? null;
const pinnedIdentifiers = [
  { program: "delivery (cli)", pinned: cliDeliveryImageId, proved: delivery.image_id },
  { program: "delivery", pinned: pinnedImageId(verifierSource, "DELIVERY_IMAGE_ID"), proved: delivery.image_id },
  {
    program: "registration",
    pinned: pinnedImageId(verifierSource, "REGISTRATION_IMAGE_ID"),
    proved: registration.image_id,
  },
  {
    // The seller's CLI refuses a receipt that was not proved by this program,
    // so its copy of the id has to be the proved one too.
    program: "registration (cli)",
    pinned: cliRegistrationImageId,
    proved: registration.image_id,
  },
];
const pinnedMismatch = pinnedIdentifiers.filter(
  ({ pinned, proved }) => pinned === null || pinned !== String(proved).toLowerCase(),
);
if (pinnedMismatch.length > 0) {
  for (const { program, pinned, proved } of pinnedMismatch) {
    console.error(
      `the contract pins ${program} image id ${pinned ?? "unreadable"}, the artefact carries ${proved}`,
    );
  }
  await fail("refusing to write a manifest whose pinned identifiers drift from the artefacts");
}

// Both clean relocated guests must reproduce the active images and binaries,
// and both genuine receipts must verify before any release output is promoted.
try {
  run("node", ["release/check-proof-builds.mjs"], root);
} catch (error) {
  console.error(error.stdout?.toString() ?? "");
  console.error(error.stderr?.toString() ?? "");
  await fail("the portable proof tuple could not be verified; refusing promotion");
}

// 4. The interface must stay self-contained: no remote script or stylesheet.
const appHtml = join(root, "app", "dist", "flips-on-demand.html");
try {
  run("npm", ["run", "build", "--", stagedHtml], join(root, "app"));
} catch {
  await fail("the interface build failed; refusing promotion");
}
const appBytes = await readFile(stagedHtml, "utf8");
const remoteTags = (appBytes.match(/<script[^>]+src=|<link[^>]+rel="stylesheet"/g) ?? []).length;
if (remoteTags !== 0) {
  await fail(
    `the interface references ${remoteTags} remote script or stylesheet tag(s); the offline client must stay self-contained`,
  );
}

if (sourceInventory(root).sha256 !== source.sha256) await fail("release source changed while building");

// Every check passed. Describe the staged bytes under their final paths.

const manifest = {
  format: "fod.release-manifest",
  version: 1,
  generatedAt: new Date().toISOString(),
  revision,
  worktreeDirty: dirty,
  source,
  artifacts: [
    { ...await describeArtifact(stagedHtml), path: relative(root, appHtml).split("\\").join("/") },
    { ...await describeArtifact(stagedWasm), path: relative(root, releaseWasm).split("\\").join("/") },
  ],
  build: {
    interface: "cd app && npm ci --offline --ignore-scripts && npm run build",
    escrow:
      "cd contracts/escrow && cargo build --locked --offline --release --target wasm32-unknown-unknown && " +
      "wasm-opt --llvm-memory-copy-fill-lowering -O3 target/wasm32-unknown-unknown/release/fod_escrow.wasm -o ../../release/artifacts/fod_escrow.wasm",
    escrowHarness:
      "cd contracts/escrow && cargo build --release --target wasm32-unknown-unknown --features test-hooks && " +
      "wasm-opt --llvm-memory-copy-fill-lowering -O3 target/wasm32-unknown-unknown/release/fod_escrow.wasm -o dist/fod_escrow.testhooks.wasm",
    registrationProof: "cd proofs/registration-v2/host && cargo run --release -- --prove",
    proofReproducibility: "node release/check-proof-builds.mjs",
    guestBuilder: "risczero/risc0-guest-builder:" +
      (await readFile(join(root, "proofs", "guest-build.rs"), "utf8")).match(/pub const BUILDER: &str =\s*"([^"]+)"/)[1],
    deliveryProof: "cd proofs/delivery && RISC0_PROVER=ipc RISC0_DEV_MODE=0 cargo run --locked --offline --release --bin delivery-host -- --no-witness --out dist/delivery-v2.json",
  },
  proofPrograms: {
    delivery: {
      statement: delivery.statement,
      imageId: delivery.image_id,
      claimDigest: delivery.claim_digest,
      journalBytes: delivery.journal.length / 2,
      sealBytes: delivery.seal_bytes,
      totalCycles: delivery.total_cycles,
    },
    registration: {
      statement: registration.statement,
      imageId: registration.image_id,
      claimDigest: registration.claim_digest,
      journalBytes: registration.journal.length / 2,
      // The registration host writes camelCase where the delivery host writes
      // snake_case; both carry the same facts.
      sealBytes: registration.sealBytes ?? registration.seal_bytes,
      totalCycles: registration.totalCycles ?? registration.total_cycles,
      segments: registration.segments,
      proveSeconds: registration.proveSeconds,
    },
  },
  checks: {
    gatekeeperForbiddenOperators: forbiddenOperators,
    releaseExportSurface: `published names only; no test hooks (${stagedExports.length} exports)`,
    interfaceRemoteTags: remoteTags,
    pinnedIdentifiers: "agree with the proved artefacts",
    portableProofTuple: "identical images and programs from two clean source locations; both genuine receipts verified",
  },
  deployment: {
    network: null,
    contract: null,
    note: "This build deploys nothing. Historical lab receipts are recorded separately in docs/PROGRESS.md; they do not qualify this changed dirty-source candidate. Mainnet and existing-node actions require separate authorization.",
  },
};

await writeFile(stagedManifest, `${JSON.stringify(manifest, null, 2)}\n`);
await mkdir(dirname(appHtml), { recursive: true });
await rename(stagedHtml, appHtml);
await rename(stagedWasm, releaseWasm);
await rename(stagedManifest, join(here, "MANIFEST.json"));
console.log(`manifest written for ${revision.slice(0, 12)}${dirty ? " (dirty worktree)" : ""}`);
for (const artifact of manifest.artifacts) {
  console.log(`  ${artifact.path} ${artifact.bytes} bytes ${artifact.sha256.slice(0, 16)}...`);
}
console.log(`  forbidden operators: ${forbiddenOperators ?? "not checked (wasm-objdump missing)"}`);
console.log(`  interface remote tags: ${remoteTags}`);
