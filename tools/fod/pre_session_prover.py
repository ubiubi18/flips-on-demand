#!/usr/bin/env python3
"""Prove a pre-session flip bundle locally from a checked node-host snapshot.

The node host derives the author-specific witness. Only the resulting private
witness crosses SSH to this prover. No transaction is signed or broadcast.
Run this periodically; a process lock and create-new outputs prevent duplicates.
"""

import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import signal
import stat
import subprocess
import time


os.umask(0o077)
HEX40 = re.compile(r"(?:0x)?[0-9a-fA-F]{40}\Z")
SSH_HOST = re.compile(r"[a-zA-Z0-9][a-zA-Z0-9@.:-]*\Z")
MIN_FREE = 8 * 1024**3


class JobError(Exception):
    pass


def unique_json_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise JobError("ambiguous-json-field")
        result[key] = value
    return result


def private_dir(path):
    path.mkdir(mode=0o700, parents=True, exist_ok=True)
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise JobError("unsafe-private-directory")


def private_json(path, value):
    private_dir(path.parent)
    temporary = path.with_name("." + path.name + ".tmp")
    fd = os.open(temporary, os.O_CREAT | os.O_WRONLY | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as stream:
        json.dump(value, stream, sort_keys=True)
        stream.write("\n")
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temporary, path)


def read_private(path, limit):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077 or info.st_size > limit:
        raise JobError("unsafe-private-file")
    return path.read_bytes()


def write_new(path, raw):
    private_dir(path.parent)
    fd = os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    with os.fdopen(fd, "wb") as stream:
        stream.write(raw)
        stream.flush()
        os.fsync(stream.fileno())


def ssh(config, remote_args):
    command = shlex.join(remote_args)
    run = subprocess.run(["ssh", "-o", "StrictHostKeyChecking=yes", "-o", "BatchMode=yes",
                          "-o", "ConnectTimeout=10", "--", config["remoteHost"], command],
                         capture_output=True, timeout=180, check=False)
    if run.returncode:
        raise JobError("node-host-command-failed")
    return run.stdout


def remote_base(config, operation, name):
    return ["python3", "-B", config["remoteScript"], operation,
            "--output", config["remoteOutput"], "--identity", name]


def remote_snapshot(config, name):
    args = remote_base(config, "snapshot", name) + [
        "--cli", config["remoteCli"], "--archive", config["remoteArchive"],
        "--contract", config["contract"], "--code-hash", config["contractCodeHash"]]
    for node in config["nodes"]:
        args.extend(("--node", node))
    raw = ssh(config, args)
    if len(raw) > 16 * 1024:
        raise JobError("oversized-snapshot-reply")
    result = json.loads(raw)
    if result.get("state") not in ("ready", "waiting"):
        raise JobError("invalid-snapshot-reply")
    return result


def remote_witness(config, snap):
    args = remote_base(config, "fetch", snap["identity"]) + [
        "--epoch", str(snap["epoch"]), "--digest", snap["inventoryDigest"],
        "--sha256", snap["witnessSha256"]]
    raw = ssh(config, args)
    if len(raw) > 64 * 1024 * 1024 or hashlib.sha256(raw).hexdigest() != snap["witnessSha256"]:
        raise JobError("witness-transfer-mismatch")
    return raw


def content_fields(proof, snap, contract, image):
    if proof.get("format") != "fod:fast-content-sealed:v1" or proof.get("image_id") != image:
        raise JobError("wrong-content-proof-image")
    journal = bytes.fromhex(proof["journal"])
    if (len(journal) != 273 or journal[36:56].hex() != contract or
            journal[56:76].hex() != snap["author"].removeprefix("0x").lower() or
            int.from_bytes(journal[76:78], "big") != snap["epoch"] or
            int.from_bytes(journal[78:80], "big") != len(snap["manifest"])):
        raise JobError("content-proof-context-mismatch")
    if (len(bytes.fromhex(proof["seal"])) != 256 or proof.get("sealBytes") != 256 or
            sorted(item["originalCid"] for item in proof["items"]) != snap["manifest"]):
        raise JobError("content-proof-inventory-mismatch")


def proof_host_image(config):
    try:
        result = subprocess.run([config["proveBinary"], "--image-id"],
                                capture_output=True, timeout=30, check=False)
    except (OSError, subprocess.TimeoutExpired) as problem:
        raise JobError("proof-host-image-unavailable") from problem
    image = result.stdout.decode("ascii", errors="replace").strip()
    if result.returncode or len(result.stdout) > 128 or not re.fullmatch(r"[0-9a-f]{64}", image):
        raise JobError("proof-host-image-unavailable")
    return image


def verify_content_proof(config, path, contract):
    try:
        result = subprocess.run([config["proveBinary"], "--verify", str(path),
                                 "--contract", "0x" + contract],
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                timeout=180, check=False)
    except (OSError, subprocess.TimeoutExpired) as problem:
        raise JobError("content-proof-seal-unverified") from problem
    if result.returncode:
        raise JobError("content-proof-seal-unverified")


def run_step(args, log):
    with open(log, "xb") as stream:
        run = subprocess.run(args, stdout=stream, stderr=subprocess.STDOUT, timeout=180,
                             check=False)
    if run.returncode:
        raise JobError("preparation-or-preflight-failed")


def prove(config, args, log, status):
    if shutil.disk_usage(log.parent).free < MIN_FREE:
        raise JobError("insufficient-free-disk")
    with open(log, "xb") as stream:
        process = subprocess.Popen(args, stdout=stream, stderr=subprocess.STDOUT,
                                   start_new_session=True)
        private_json(status, {"state": "proving", "pid": process.pid, "startedAt": time.time()})
        if shutil.which("caffeinate"):
            subprocess.Popen(["caffeinate", "-i", "-w", str(process.pid)],
                             stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        while process.poll() is None:
            time.sleep(30)
            if shutil.disk_usage(log.parent).free < MIN_FREE:
                os.killpg(process.pid, signal.SIGTERM)
                process.wait(timeout=30)
                raise JobError("disk-safety-threshold-reached")
        if process.returncode:
            raise JobError("content-proof-failed")


def check_previous_attempts(output):
    # A proof child survives a scheduler crash because it runs in its own
    # session. Check every digest before starting another proof.
    for status_path in output.glob("epoch-*/*/*/status.json"):
        private_dir(status_path.parent)
        state = json.loads(read_private(status_path, 4096)).get("state")
        if state not in ("complete", "stale"):
            raise JobError("previous-attempt-needs-review")


def run_job(config, name, author, contract, image):
    snap = remote_snapshot(config, name)
    if snap["state"] == "waiting":
        return {"identity": name, "state": "waiting", "reason": snap.get("reason")}
    if snap["identity"] != name or snap["author"].removeprefix("0x").lower() != author:
        raise JobError("node-identity-mismatch")
    if snap["epoch"] < config["minEpoch"]:
        return {"identity": name, "state": "waiting", "reason": "before-minimum-epoch"}
    if (not isinstance(snap.get("manifest"), list) or not 1 <= len(snap["manifest"]) <= 32 or
            snap["manifest"] != sorted(set(snap["manifest"]))):
        raise JobError("invalid-snapshot-inventory")
    digest = hashlib.sha256((snap["inventoryDigest"] + contract + image).encode()).hexdigest()
    folder = Path(config["output"]) / f"epoch-{snap['epoch']}" / name / digest
    proof_path = folder / "content-proof.json"
    status_path = folder / "status.json"
    if folder.exists():
        private_dir(folder)
        status = json.loads(read_private(status_path, 4096))
        if status.get("state") != "complete":
            raise JobError("previous-attempt-needs-review")
        raw = read_private(proof_path, 64 * 1024 * 1024)
        if hashlib.sha256(raw).hexdigest() != status.get("proofSha256"):
            raise JobError("saved-content-proof-changed")
        content_fields(json.loads(raw), snap, contract, image)
        verify_content_proof(config, proof_path, contract)
        return {"identity": name, "epoch": snap["epoch"], "state": "ready"}
    witness = remote_witness(config, snap)
    parsed = json.loads(witness)
    if parsed.get("epoch") != snap["epoch"] or parsed.get("author", "").removeprefix("0x").lower() != author:
        raise JobError("transferred-witness-identity-mismatch")
    records = parsed.get("blobs")
    cids = [item["blobCid"] for item in records] if isinstance(records, list) else [parsed.get("blobCid")]
    if sorted(cids) != snap["manifest"] or len(cids) != len(snap["manifest"]):
        raise JobError("transferred-witness-inventory-mismatch")
    private_dir(folder)
    try:
        write_new(folder / "witness.json", witness)
        private_json(status_path, {"state": "preparing", "inventoryDigest": snap["inventoryDigest"],
                                   "witnessSha256": snap["witnessSha256"]})
        run_step([config["prepareBinary"], "--input", str(folder / "witness.json"),
                  "--contract", "0x" + contract, "--out-dir", str(folder / "prepared")],
                 folder / "prepare.log")
        host_args = [config["proveBinary"], "--input", str(folder / "witness.json"),
                     "--prepared", str(folder / "prepared"), "--contract", "0x" + contract,
                     "--out", str(proof_path)]
        run_step(host_args, folder / "preflight.log")
        prove(config, host_args + ["--prove"], folder / "proof.log", status_path)
        raw = read_private(proof_path, 64 * 1024 * 1024)
        content_fields(json.loads(raw), snap, contract, image)
        verify_content_proof(config, proof_path, contract)
        current = remote_snapshot(config, name)
        if (current.get("state") == "ready" and current.get("epoch") == snap["epoch"] and
                current.get("inventoryDigest") != snap["inventoryDigest"]):
            private_json(status_path, {"state": "stale", "proofSha256": hashlib.sha256(raw).hexdigest()})
            return {"identity": name, "epoch": snap["epoch"], "state": "stale"}
        if current.get("state") == "waiting" or current.get("epoch") != snap["epoch"]:
            private_json(status_path, {"state": "complete", "proofSha256": hashlib.sha256(raw).hexdigest(),
                                       "inventoryDigest": snap["inventoryDigest"], "sessionCheck": "pending"})
            return {"identity": name, "epoch": snap["epoch"], "state": "ready_for_session_check"}
        private_json(status_path, {"state": "complete", "proofSha256": hashlib.sha256(raw).hexdigest(),
                                   "inventoryDigest": snap["inventoryDigest"]})
        return {"identity": name, "epoch": snap["epoch"], "state": "ready"}
    except Exception as problem:
        private_json(status_path, {"state": "failed", "reason": str(problem) if isinstance(problem, JobError) else type(problem).__name__})
        raise


def held_proof_running(status):
    if status.get("state") not in ("running", "starting", "identity_complete"):
        return False
    pid = status.get("pid")
    if isinstance(pid, bool) or not isinstance(pid, int) or pid <= 0:
        return True
    # The detached prover child stays in the host's process group if the host exits.
    try:
        os.killpg(pid, 0)
    except ProcessLookupError:
        return False
    except OSError:
        return True
    return True


def run(config):
    output = Path(config["output"])
    private_dir(output)
    with (output / ".lock").open("a+b") as held:
        try:
            fcntl.flock(held, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            return [{"state": "waiting", "reason": "prover-already-running"}]
        hold = config.get("holdStatus")
        if hold and Path(hold).exists():
            status = json.loads(read_private(Path(hold), 4096))
            if held_proof_running(status):
                return [{"state": "waiting", "reason": "earlier-proof-running"}]
        check_previous_attempts(output)
        manifest = json.loads(Path(config["manifest"]).read_text(), object_pairs_hook=unique_json_object)
        if manifest.get("format") == "fod.fast-candidate-manifest" and manifest.get("version") == 1:
            if "fastContentImageId" in manifest.get("checks", {}):
                raise JobError("ambiguous-proof-image-pin")
            image_pin = manifest["proofProgram"]["imageId"]
            artifact_code_hash = manifest["artifact"].get("codeHash")
        elif manifest.get("format") == "fod.direct-candidate-manifest" and manifest.get("version") == 5:
            if "proofProgram" in manifest:
                raise JobError("ambiguous-proof-image-pin")
            image_pin = manifest["checks"]["fastContentImageId"]
            artifact_code_hash = manifest["artifact"].get("codeHash")
            if artifact_code_hash is None:
                raise JobError("deployed-artifact-pin-mismatch")
        else:
            raise JobError("unsupported-proof-manifest")
        image = image_pin.removeprefix("0x").lower()
        contract = config["contract"].removeprefix("0x").lower()
        code_hash = config["contractCodeHash"].removeprefix("0x").lower()
        contract_sha256 = config["contractSha256"].removeprefix("0x").lower()
        artifact_sha256 = manifest["artifact"]["sha256"].removeprefix("0x").lower()
        if not re.fullmatch(r"[0-9a-f]{64}", image) or not re.fullmatch(r"[0-9a-f]{40}", contract):
            raise JobError("invalid-proof-or-contract-pin")
        if (not re.fullmatch(r"[0-9a-f]{64}", code_hash) or
                not re.fullmatch(r"[0-9a-f]{64}", contract_sha256) or
                contract_sha256 != artifact_sha256 or
                (artifact_code_hash is not None and
                 code_hash != artifact_code_hash.removeprefix("0x").lower())):
            raise JobError("deployed-artifact-pin-mismatch")
        if proof_host_image(config) != image:
            raise JobError("proof-host-image-mismatch")
        results = []
        for identity in config["identities"]:
            results.append(run_job(config, identity["name"], identity["author"].removeprefix("0x").lower(),
                                   contract, image))
        return results


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", required=True, type=Path)
    args = parser.parse_args()
    try:
        config = json.loads(read_private(args.config, 16 * 1024), object_pairs_hook=unique_json_object)
        if not SSH_HOST.fullmatch(config["remoteHost"]) or len(config["nodes"]) != 2 or len(config["identities"]) != 2:
            raise JobError("invalid-prover-configuration")
        if len({identity["name"] for identity in config["identities"]}) != 2 or any(
            not HEX40.fullmatch(identity["author"]) for identity in config["identities"]
        ):
            raise JobError("invalid-identity-configuration")
        print(json.dumps(run(config), separators=(",", ":")))
        return 0
    except Exception as problem:
        code = str(problem) if isinstance(problem, JobError) else type(problem).__name__
        print(f"pre_session_prover_failed:{code}")
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
