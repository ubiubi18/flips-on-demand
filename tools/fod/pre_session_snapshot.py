#!/usr/bin/env python3
"""Make an author-specific, owner-only content witness before validation.

This runs beside the nodes. It uses the read-only watcher's CID-checked archive,
but only this separate process opens the author's node key. It never signs or
broadcasts a transaction. A local prover may fetch the witness by digest.
"""

import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import time

import session_indexer as indexer


os.umask(0o077)
HEX64 = re.compile(r"[0-9a-f]{64}\Z")
HEX40 = re.compile(r"[0-9a-f]{40}\Z")


class NotReady(Exception):
    pass


def private_dir(path):
    path.mkdir(mode=0o700, parents=True, exist_ok=True)
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise indexer.CaptureError("unsafe-private-directory")


def private_file(path, limit):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077 or info.st_size > limit:
        raise indexer.CaptureError("unsafe-private-file")
    return path.read_bytes()


def checked_nodekey(path):
    # The node container owns this 0600 file, while this helper runs as root.
    # Its expected address is checked by the witness command before writing.
    info = path.lstat()
    if (not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077 or
            not 0 < info.st_size <= 4096):
        raise indexer.CaptureError("unsafe-node-key")


def normalized_cids(value):
    if (not isinstance(value, list) or not 1 <= len(value) <= 32 or
            any(not isinstance(cid, str) or not indexer.CID.fullmatch(cid) for cid in value) or
            len(value) != len(set(value))):
        raise NotReady("invalid-or-empty-inventory")
    return sorted(value)


def current_inventory(nodes, archive, name):
    epoch, _, _, _ = indexer.canonical_view(nodes)
    watch = indexer.read_json(archive / "watch.json")
    if watch.get("epoch") != epoch:
        raise NotReady("watcher-epoch-behind")
    authors = watch.get("authors", {})
    if set(authors) != {node["name"] for node in nodes} or name not in authors:
        raise indexer.CaptureError("watcher-identity-map-invalid")
    normalized_authors = {label: indexer.address(author) for label, author in authors.items()}
    if len(set(normalized_authors.values())) != len(nodes):
        raise indexer.CaptureError("identities-not-separated")
    manifests = []
    for node in nodes:
        label = node["name"]
        coinbase = indexer.address(indexer.rpc(node, "dna_getCoinbaseAddr"))
        if coinbase != normalized_authors[label]:
            raise indexer.CaptureError("node-identity-changed")
        state = indexer.rpc(node, "dna_epoch")
        if int(state["epoch"]) != epoch or state.get("currentPeriod") not in (None, "None", 0):
            raise NotReady("validation-period-active")
        identity = indexer.rpc(node, "dna_identity", ["0x" + normalized_authors[name]])
        if not isinstance(identity, dict):
            raise NotReady("identity-state-unavailable")
        manifests.append(normalized_cids(identity.get("flips")))
    if any(manifest != manifests[0] for manifest in manifests[1:]):
        raise NotReady("nodes-disagree-on-flips")
    return epoch, normalized_authors[name], manifests[0]


def checked_archive(archive, epoch, name, manifest):
    root = archive / f"epoch-{epoch}"
    status = indexer.read_json(root / "encrypted-blob-status.json", 8_000_000)
    if status.get("epoch") != epoch or status.get("phase") != "prefetch":
        raise NotReady("blob-capture-not-current")
    records = status.get("blobs", {}).get(name)
    if not isinstance(records, dict) or sorted(records) != manifest:
        raise NotReady("blob-capture-incomplete")
    directory = root / "encrypted-blobs" / name
    for cid in manifest:
        saved = records[cid]
        if not isinstance(saved, dict) or saved.get("cid_verified") is not True:
            raise NotReady("blob-capture-incomplete")
        body = indexer.checked_blob(directory / cid, cid)
        if body is None or hashlib.sha256(body).hexdigest() != saved.get("sha256"):
            raise NotReady("blob-capture-mismatch")
    return directory


def verify_witness(path, epoch, author, manifest):
    witness = json.loads(private_file(path, 64 * 1024 * 1024))
    if witness.get("epoch") != epoch or indexer.address(witness.get("author")) != author:
        raise indexer.CaptureError("witness-identity-mismatch")
    if "blobs" in witness:
        pairs = witness["blobs"]
        cids = [item.get("blobCid") for item in pairs] if isinstance(pairs, list) else []
    else:
        cids = [witness.get("blobCid")]
    if sorted(cids) != manifest or len(cids) != len(manifest):
        raise indexer.CaptureError("witness-inventory-mismatch")
    return hashlib.sha256(path.read_bytes()).hexdigest()


def snapshot(cli, archive, nodes, name, output, contract, code_hash, stable_seconds=600):
    if len(nodes) != 2 or len({node["name"] for node in nodes}) != 2:
        raise indexer.CaptureError("two-distinct-nodes-required")
    if not HEX40.fullmatch(contract) or not HEX64.fullmatch(code_hash):
        raise indexer.CaptureError("invalid-contract-pin")
    private_dir(output)
    lock = output / ".lock"
    with lock.open("a+b") as held:
        fcntl.flock(held, fcntl.LOCK_EX)
        epoch, author, manifest = current_inventory(nodes, archive, name)
        for node in nodes:
            stake = indexer.rpc(node, "contract_getStake", ["0x" + contract])
            if (not isinstance(stake, dict) or
                    str(stake.get("Hash", "")).removeprefix("0x").lower() != code_hash):
                raise indexer.CaptureError("deployed-contract-code-changed")
        blob_dir = checked_archive(archive, epoch, name, manifest)
        payload = {"format": "fod.pre-session-inventory:v1", "author": author,
                   "epoch": epoch, "manifest": manifest}
        digest = hashlib.sha256(json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
        parent = output / f"epoch-{epoch}" / name
        private_dir(parent)
        candidate_file = parent / "candidate.json"
        candidate = indexer.read_json(candidate_file) if candidate_file.exists() else {}
        if candidate.get("digest") != digest:
            indexer.private_json(candidate_file, {"digest": digest, "firstSeen": time.time()})
            raise NotReady("inventory-settling")
        if time.time() - candidate.get("firstSeen", 0) < stable_seconds:
            raise NotReady("inventory-settling")
        final = parent / digest
        if not final.exists():
            work = Path(tempfile.mkdtemp(prefix=".snapshot-", dir=parent))
            try:
                inventory_file = work / "inventory.json"
                indexer.private_json(inventory_file, payload)
                key = Path(next(node["datadir"] for node in nodes if node["name"] == name)) / "keystore" / "nodekey"
                checked_nodekey(key)
                run = subprocess.run([str(cli), "pre-session-witness",
                                      "-pre-session-inventory", str(inventory_file),
                                      "-nodekey", str(key), "-blob-dir", str(blob_dir),
                                      "-out", str(work / "witness.json")],
                                     capture_output=True, timeout=120, check=False)
                if run.returncode:
                    raise indexer.CaptureError("witness-cli-refused")
                verify_witness(work / "witness.json", epoch, author, manifest)
                os.rename(work, final)
            finally:
                if work.exists():
                    shutil.rmtree(work)
        else:
            private_dir(final)
            saved = indexer.read_json(final / "inventory.json")
            if saved != payload:
                raise indexer.CaptureError("saved-inventory-changed")
        witness_hash = verify_witness(final / "witness.json", epoch, author, manifest)
        return {"state": "ready", "epoch": epoch, "identity": name,
                "author": author, "manifest": manifest, "inventoryDigest": digest,
                "witnessSha256": witness_hash}


def fetch(output, epoch, name, digest, expected):
    if not re.fullmatch(r"[a-z][a-z0-9_-]{0,31}", name) or not HEX64.fullmatch(digest) or not HEX64.fullmatch(expected):
        raise indexer.CaptureError("invalid-fetch-selector")
    private_dir(output)
    parent = output / f"epoch-{epoch}" / name / digest
    if not parent.is_dir() or parent.is_symlink():
        raise indexer.CaptureError("snapshot-unavailable")
    raw = private_file(parent / "witness.json", 64 * 1024 * 1024)
    if hashlib.sha256(raw).hexdigest() != expected:
        raise indexer.CaptureError("witness-digest-mismatch")
    return raw


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=("snapshot", "fetch"))
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--cli", type=Path)
    parser.add_argument("--archive", type=Path)
    parser.add_argument("--node", action="append", type=indexer.node_arg)
    parser.add_argument("--identity")
    parser.add_argument("--epoch", type=int)
    parser.add_argument("--digest")
    parser.add_argument("--sha256")
    parser.add_argument("--contract")
    parser.add_argument("--code-hash")
    parser.add_argument("--stable-seconds", type=int, default=600)
    args = parser.parse_args()
    try:
        if not args.output.is_absolute():
            raise indexer.CaptureError("output-must-be-absolute")
        if args.operation == "snapshot":
            if (not args.cli or not args.cli.is_file() or not args.archive or not args.node or
                    not args.identity or not args.contract or not args.code_hash):
                raise indexer.CaptureError("snapshot-arguments-incomplete")
            if not 0 <= args.stable_seconds <= 86_400:
                raise indexer.CaptureError("invalid-stability-period")
            result = snapshot(args.cli, args.archive, args.node, args.identity, args.output,
                              args.contract.removeprefix("0x").lower(),
                              args.code_hash.removeprefix("0x").lower(), args.stable_seconds)
            print(json.dumps(result, separators=(",", ":")))
        else:
            if args.epoch is None or args.epoch < 1 or not args.identity or not args.digest or not args.sha256:
                raise indexer.CaptureError("fetch-arguments-incomplete")
            sys.stdout.buffer.write(fetch(args.output, args.epoch, args.identity, args.digest, args.sha256))
        return 0
    except NotReady as problem:
        print(json.dumps({"state": "waiting", "reason": str(problem)}))
        return 0
    except Exception as problem:
        # Never print a CLI's stdout/stderr, witness, epoch secret or key path.
        code = str(problem) if isinstance(problem, indexer.CaptureError) else type(problem).__name__
        print(f"pre_session_snapshot_failed:{code}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
