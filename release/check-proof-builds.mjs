// Reproduce the published guests, the fast candidate, or the separate direct
// candidate in two fresh source/target locations. Docker compiles
// source-only contexts at /src.
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { createWriteStream, readFileSync, readdirSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { sourceInventory, exportSource } from "./source-snapshot.mjs";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const fastOnly = process.argv[2] === "--fast-only";
const fastV5Only = process.argv[2] === "--fast-v5-only";
const directOnly = process.argv[2] === "--direct-only";
const workArg = process.argv[fastOnly || fastV5Only || directOnly ? 3 : 2];
const work = workArg ? resolve(workArg) : mkdtempSync(join(tmpdir(), "fod-proof-builds-"));
if (workArg) mkdirSync(work, { mode: 0o700 }); // refuse overwriting evidence
const source = sourceInventory(root);
const programs = fastOnly || fastV5Only ? [
  { directory: "fast-content", package: "fast-content-methods", name: "FAST_CONTENT_GUEST",
    artifact: fastV5Only ? "fast-content/dist/synthetic-v5-one-flip.json" : "fast-content/dist/synthetic-one-flip.json" },
] : directOnly ? [
  { directory: "registration-v2", package: "v2-methods", name: "REGISTRATION_V2_GUEST", artifact: "registration-v2/dist/direct-v2-synthetic.json" },
] : [
  { directory: "delivery", package: "methods", name: "DELIVERY_GUEST", artifact: "delivery/dist/delivery-v2.json" },
  { directory: "registration-v2", package: "v2-methods", name: "REGISTRATION_V2_GUEST", artifact: "registration-v2/dist/registration-v2.json" },
];
const commandEnv = { ...process.env, CARGO_BUILD_JOBS: "4", RISC0_DEV_MODE: "0" };
delete commandEnv.BONSAI_API_KEY;
delete commandEnv.BONSAI_API_URL;
const sha256 = (bytes) => createHash("sha256").update(bytes).digest("hex");

function run(command, args, cwd, logName, env = commandEnv) {
  console.log(`${command} ${args.join(" ")} (${cwd}); log: ${join(work, logName)}`);
  return new Promise((done, reject) => {
    const log = createWriteStream(join(work, logName), { flags: "wx", mode: 0o600 });
    const child = spawn(command, args, { cwd, env, stdio: ["ignore", "pipe", "pipe"] });
    child.stdout.pipe(log, { end: false });
    child.stderr.pipe(log, { end: false });
    child.once("error", (error) => { log.end(); reject(error); });
    child.once("close", (code) => log.end(() => code === 0 ? done() : reject(new Error(`${logName}: exit ${code}`))));
  });
}

function methodFiles(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    return entry.isDirectory() ? methodFiles(path) : entry.name === "methods.rs" ? [path] : [];
  });
}

function guest(target, name) {
  const matches = methodFiles(join(target, "release", "build"))
    .map((path) => readFileSync(path, "utf8"))
    .filter((text) => text.includes(`pub const ${name}_ID:`));
  if (matches.length !== 1) throw new Error(`expected exactly one ${name} build`);
  const text = matches[0];
  const words = text.match(new RegExp(`pub const ${name}_ID: \\[u32; 8\\] = \\[([^\\]]+)\\]`));
  const binaryPath = text.match(new RegExp(`pub const ${name}_ELF: &\\[u8\\] = include_bytes!\\(("[^\\n]+")\\)`));
  if (!words || !binaryPath) throw new Error(`missing built image or binary: ${name}`);
  const values = words[1].split(",").map((word) => Number(word.trim()));
  if (values.length !== 8 || values.some((v) => !Number.isInteger(v) || v < 0 || v > 0xffffffff)) throw new Error("invalid image words");
  const image = Buffer.alloc(32);
  values.forEach((value, i) => image.writeUInt32LE(value, i * 4));
  const binary = readFileSync(JSON.parse(binaryPath[1]));
  return { imageId: image.toString("hex"), binarySha256: sha256(binary), bytes: binary.length };
}

try {
  const buildAt = async (location) => {
    const clean = join(work, `source-${location}`);
    exportSource(root, clean, source);
    const target = join(clean, "target-proofs");
    const results = {};
    for (const program of programs) {
      await run("cargo", ["build", "--locked", "--offline", "--release", "-p", program.package],
        join(clean, "proofs", program.directory), `${program.directory}-${location}.log`, { ...commandEnv, CARGO_TARGET_DIR: target });
      results[program.directory] = guest(target, program.name);
    }
    return results;
  };
  const builds = directOnly
    ? [await buildAt("a"), await buildAt("b")]
    : await Promise.all(["a", "b"].map(buildAt));
  for (const program of programs) {
    const a = builds[0][program.directory], b = builds[1][program.directory];
    const artifact = JSON.parse(readFileSync(join(root, "proofs", program.artifact)));
    if (a.imageId !== b.imageId || a.binarySha256 !== b.binarySha256 || a.imageId !== artifact.image_id) {
      throw new Error(`${program.directory}: clean programs or active receipt image drift (${a.imageId}, ${b.imageId}, ${artifact.image_id})`);
    }
    console.log(`${program.directory}: two identical programs, image ${a.imageId}`);
  }
  if (fastOnly || fastV5Only) {
    const verifier = readFileSync(fastV5Only
      ? join(root, "contracts", "direct-escrow-v5", "src", "lib.rs")
      : join(root, "contracts", "escrow", "src", "verifier.rs"), "utf8");
    const declaration = verifier.match(fastV5Only
      ? /pub const FAST_CONTENT_V5_IMAGE_ID: \[u8; 32\] = \[([\s\S]*?)\]/
      : /pub const FAST_CONTENT_IMAGE_ID: \[u8; 32\] = \[([\s\S]*?)\]/);
    const pin = [...(declaration?.[1] ?? "").matchAll(/0x([0-9a-fA-F]{2})/g)].map((match) => match[1].toLowerCase()).join("");
    if (pin.length !== 64 || pin !== builds[0]["fast-content"].imageId) {
      throw new Error("fast content contract image pin differs from the reproduced guest");
    }
    const fastCli = readFileSync(fastV5Only
      ? join(root, "tools", "fod", "direct_v5_start.go")
      : join(root, "tools", "fod", "prepare_fast_registration.go"), "utf8");
    const cliPin = fastCli.match(fastV5Only
      ? /fastContentV5ImageIDHex\s*=\s*"([0-9a-fA-F]{64})"/
      : /fastContentImageIDHex\s*=\s*"([0-9a-fA-F]{64})"/)?.[1]?.toLowerCase();
    if (cliPin !== pin) {
      throw new Error("fast content CLI image pin differs from the reproduced guest");
    }
    // The separate Idena WASM harness verifies this fixture's Groth16 seal.
  } else if (directOnly) {
    await run("cargo", ["test", "--locked", "--offline", "--release", "-p", "registration-v2-host", "--test", "sealed_artefact"],
      join(work, "source-a", "proofs", "registration-v2"), "direct-receipt.log",
      { ...commandEnv, CARGO_TARGET_DIR: join(work, "source-a", "target-proofs") });
  } else {
    // Matching labels alone are insufficient: both genuine Groth16 seals must
    // verify against a host built with this same pinned guest procedure.
    await run("cargo", ["run", "--locked", "--offline", "--release", "--bin", "delivery-host", "--", "--verify", "dist/delivery-v2.json"],
      join(root, "proofs", "delivery"), "delivery-receipt.log");
    await run("cargo", ["test", "--locked", "--offline", "--release", "-p", "registration-v2-host", "--test", "sealed_artefact"],
      join(root, "proofs", "registration-v2"), "registration-receipt.log");
  }
  if (sourceInventory(root).sha256 !== source.sha256) throw new Error("source changed during proof build verification");
  const builder = readFileSync(join(root, "proofs", "guest-build.rs"), "utf8").match(/pub const BUILDER: &str =\s*"([^"]+)"/)[1];
  writeFileSync(join(work, "EVIDENCE.json"), `${JSON.stringify({ source: source.sha256, builder, builds, receiptsVerified: !fastOnly && !fastV5Only }, null, 2)}\n`, { flag: "wx", mode: 0o600 });
  if (fastOnly || fastV5Only || directOnly) {
    for (const location of ["a", "b"]) rmSync(join(work, `source-${location}`), { recursive: true });
    console.log(fastOnly || fastV5Only
      ? `portable fast guest and image pin verified; seal still requires Idena WASM harness: ${join(work, "EVIDENCE.json")}`
      : `portable direct guest and genuine seal verified; Idena WASM harness is a separate gate: ${join(work, "EVIDENCE.json")}`);
  } else {
    console.log(`portable proof programs and receipts verified; evidence: ${join(work, "EVIDENCE.json")}`);
  }
} catch (error) {
  console.error(`proof reproducibility refused: ${error.message}; build evidence: ${work}`);
  process.exitCode = 1;
}
