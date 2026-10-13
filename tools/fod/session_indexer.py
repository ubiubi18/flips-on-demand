#!/usr/bin/env python3
"""Optional read-only archive of portable Idena session evidence.

Run beside one or more ordinary nodes. The archive holds public proofs and
CID-checked encrypted flip bytes, never node keys or epoch secrets. A buyer
still authenticates the roots against their own chain view; this process is
neither an oracle nor a protocol operator.
"""

import argparse
import base64
import errno
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import tempfile
import time
from urllib.request import Request, urlopen


os.umask(0o077)
CID = re.compile(r"b[a-z2-7]{20,120}\Z")
ADDRESS = re.compile(r"(?:0x)?[0-9a-fA-F]{40}\Z")
TABLE = re.compile(r"[0-9]+\.(?:ldb|sst)\Z")
META = re.compile(r"(?:[0-9]+\.log|MANIFEST-[0-9]+|CURRENT)\Z")
MAX_FLIP_BYTES = 614_400
UNIXFS_CHUNK = 262_144


class CaptureError(Exception):
    pass


def private_json(path, value):
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix=".write-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as stream:
            json.dump(value, stream, sort_keys=True, indent=2)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def read_json(path, limit=1_000_000):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_size > limit:
        raise CaptureError("oversized-or-linked-archive-file")
    return json.loads(path.read_text())


def address(value):
    if not isinstance(value, str) or not ADDRESS.fullmatch(value):
        raise CaptureError("invalid-author-address")
    return value.removeprefix("0x").lower()


def node_arg(value):
    parts = value.split(":", 2)
    if len(parts) != 3 or not re.fullmatch(r"[a-z][a-z0-9_-]{0,31}", parts[0]):
        raise argparse.ArgumentTypeError("--node must be NAME:PORT:DATADIR")
    try:
        port = int(parts[1])
    except ValueError as exc:
        raise argparse.ArgumentTypeError("node port must be an integer") from exc
    if not 1 <= port <= 65535 or not Path(parts[2]).is_absolute():
        raise argparse.ArgumentTypeError("node port or datadir is invalid")
    return {"name": parts[0], "port": port, "datadir": parts[2]}


def rpc(node, method, params=None, limit=1_000_000):
    # The credential never appears in process arguments, files or error text.
    key = (Path(node["datadir"]) / "api.key").read_text().strip()
    if not key:
        raise CaptureError("node-rpc-key-unavailable")
    body = json.dumps({"id": 1, "method": method, "params": params or [], "key": key}).encode()
    request = Request(f"http://127.0.0.1:{node['port']}", data=body,
                      headers={"Content-Type": "application/json"})
    try:
        with urlopen(request, timeout=6) as response:
            raw = response.read(limit + 1)
    except Exception as exc:
        raise CaptureError("node-rpc-unavailable") from exc
    if len(raw) > limit:
        raise CaptureError("node-rpc-oversized")
    try:
        result = json.loads(raw)
    except (ValueError, UnicodeError) as exc:
        raise CaptureError("node-rpc-invalid-json") from exc
    if not isinstance(result, dict) or result.get("error") or result.get("result") is None:
        raise CaptureError("node-rpc-refused")
    return result["result"]


def varint(value):
    output = bytearray()
    while value >= 128:
        output.append((value & 127) | 128)
        value >>= 7
    output.append(value)
    return bytes(output)


def cid_bytes(codec, data):
    return bytes((1, codec, 0x12, 0x20)) + hashlib.sha256(data).digest()


def cid_text(value):
    return "b" + base64.b32encode(value).decode("ascii").lower().rstrip("=")


def flip_cid(blob):
    """Reproduce the pinned node's raw-leaf UnixFS CID without trusting RPC."""
    if not blob or len(blob) > MAX_FLIP_BYTES:
        raise CaptureError("unsupported-flip-size")
    if len(blob) <= UNIXFS_CHUNK:
        return cid_text(cid_bytes(0x55, blob))
    root = bytearray()
    sizes = []
    for start in range(0, len(blob), UNIXFS_CHUNK):
        part = blob[start:start + UNIXFS_CHUNK]
        leaf = cid_bytes(0x55, part)
        link = b"\x0a" + varint(len(leaf)) + leaf + b"\x12\x00\x18" + varint(len(part))
        root.extend(b"\x12" + varint(len(link)) + link)
        sizes.append(len(part))
    file_data = b"\x08\x02\x18" + varint(len(blob))
    for size in sizes:
        file_data += b"\x20" + varint(size)
    root.extend(b"\x0a" + varint(len(file_data)) + file_data)
    return cid_text(cid_bytes(0x70, root))


def checked_blob(path, cid):
    try:
        info = path.lstat()
    except FileNotFoundError:
        return None
    if not stat.S_ISREG(info.st_mode) or info.st_size > MAX_FLIP_BYTES:
        raise CaptureError("unsafe-saved-encrypted-blob")
    blob = path.read_bytes()
    if flip_cid(blob) != cid:
        raise CaptureError("saved-encrypted-blob-cid-mismatch")
    return blob


def retain_blobs(nodes, archive, epoch, manifests, final=False):
    """Keep opaque ciphertext for every CID without touching identity keys."""
    destination = archive / f"epoch-{epoch}" / "encrypted-blobs"
    destination.mkdir(mode=0o700, parents=True, exist_ok=True)
    records = {}
    complete = True
    for node in nodes:
        name = node["name"]
        cids = manifests[name]
        if (not isinstance(cids, list) or len(cids) > 32
                or any(not isinstance(cid, str) or not CID.fullmatch(cid) for cid in cids)
                or len(cids) != len(set(cids))):
            raise CaptureError("invalid-encrypted-inventory")
        node_dir = destination / name
        node_dir.mkdir(mode=0o700, exist_ok=True)
        items = {}
        for cid in cids:
            path = node_dir / cid
            try:
                blob = checked_blob(path, cid)
                if blob is None:
                    response = rpc(node, "ipfs_get", [cid], limit=1_300_000)
                    if not isinstance(response, str) or not response.startswith("0x"):
                        raise CaptureError("invalid-encrypted-blob-response")
                    blob = bytes.fromhex(response[2:])
                    if flip_cid(blob) != cid:
                        raise CaptureError("encrypted-blob-cid-mismatch")
                    fd, temporary = tempfile.mkstemp(prefix=".blob-", dir=node_dir)
                    try:
                        with os.fdopen(fd, "wb") as stream:
                            stream.write(blob)
                            stream.flush()
                            os.fsync(stream.fileno())
                        os.replace(temporary, path)
                    finally:
                        if os.path.exists(temporary):
                            os.unlink(temporary)
                items[cid] = {"bytes": len(blob), "sha256": hashlib.sha256(blob).hexdigest(),
                              "cid_verified": True}
            except (CaptureError, ValueError) as exc:
                complete = False
                items[cid] = {"missing": True,
                              "reason": str(exc) if isinstance(exc, CaptureError) else "invalid-hex"}
        records[name] = items
    summary = {"epoch": epoch, "phase": "final" if final else "prefetch",
               "ready": final and complete and all(manifests[node["name"]] for node in nodes),
               "counts": {name: {"retained": sum(not item.get("missing", False) for item in items.values()),
                                  "required": len(items)} for name, items in records.items()},
               "blobs": records}
    private_json(archive / f"epoch-{epoch}" / "encrypted-blob-status.json", summary)
    return summary


def live_manifests(nodes, authors):
    manifests = {}
    for node in nodes:
        name = node["name"]
        identity = rpc(node, "dna_identity", ["0x" + address(authors[name])])
        if not isinstance(identity, dict):
            raise CaptureError("identity-inventory-unavailable")
        manifests[name] = identity.get("flips")
    return manifests


def final_manifests(nodes, archive, epoch):
    return {node["name"]: read_json(archive / f"epoch-{epoch}" /
                                     f"{node['name']}-session-bundle.json")["manifest"]
            for node in nodes}


def canonical_view(nodes):
    views = []
    for node in nodes:
        epoch = rpc(node, "dna_epoch")
        head = rpc(node, "bcn_lastBlock")
        views.append((int(epoch["epoch"]), int(epoch["startBlock"]),
                      int(head["height"])))
    if len({(v[0], v[1]) for v in views}) != 1:
        raise CaptureError("nodes-disagree-on-epoch")
    common_height = min(v[2] for v in views)
    blocks = [rpc(node, "bcn_blockAt", [common_height]) for node in nodes]
    hashes = {str(block.get("hash", "")).lower() for block in blocks}
    roots = {str(block.get("root", "")).lower() for block in blocks}
    if (len(hashes) != 1 or "" in hashes or len(roots) != 1 or "" in roots):
        raise CaptureError("nodes-do-not-share-common-block")
    return views[0][0], views[0][1], common_height, next(iter(hashes))


def verify_header(nodes, anchor):
    if not isinstance(anchor, dict) or not isinstance(anchor.get("height"), int):
        raise CaptureError("invalid-saved-anchor")
    root = str(anchor.get("rootHex", "")).removeprefix("0x").lower()
    if not re.fullmatch(r"[0-9a-f]{64}", root):
        raise CaptureError("invalid-saved-root")
    hashes = set()
    for node in nodes:
        block = rpc(node, "bcn_blockAt", [anchor["height"]])
        if str(block.get("root", "")).removeprefix("0x").lower() != root:
            raise CaptureError("anchor-header-root-mismatch")
        hashes.add(str(block.get("hash", "")).lower())
    if len(hashes) != 1 or not next(iter(hashes)):
        raise CaptureError("nodes-disagree-on-anchor-block")


def snapshot(node, destination):
    source = Path(node["datadir"]) / "idenachain.db"
    kept = destination / ".state-snapshot"
    if kept.is_dir() and (kept / "idenachain.db" / "CURRENT").is_file():
        return kept
    if kept.exists():
        raise CaptureError("incomplete-kept-state-snapshot")
    if shutil.disk_usage(destination).free < 2 * 1024**3:
        raise CaptureError("insufficient-free-disk")
    work = Path(tempfile.mkdtemp(prefix=".state-staging-", dir=destination))
    target = work / "idenachain.db"
    target.mkdir(mode=0o700)
    try:
        for entry in source.iterdir():
            if TABLE.fullmatch(entry.name):
                try:
                    os.link(entry, target / entry.name)
                except OSError as exc:
                    if exc.errno != errno.EXDEV:
                        raise
                    shutil.copyfile(entry, target / entry.name)
            elif META.fullmatch(entry.name):
                shutil.copyfile(entry, target / entry.name)
        if not (target / "CURRENT").exists() or not list(target.glob("MANIFEST-*")):
            raise CaptureError("incomplete-state-snapshot")
        os.replace(work, kept)
        return kept
    except Exception:
        shutil.rmtree(work)
        raise


def cli_json(cli, args, path, validate, limit=1_000_000):
    fd, temporary = tempfile.mkstemp(prefix=".capture-", suffix=".json", dir=path.parent)
    os.close(fd)
    try:
        run = subprocess.run([str(cli), *args, "-out", temporary], capture_output=True,
                             timeout=60, check=False)
        # CLI output can contain author information; it is never logged.
        if run.returncode or os.stat(temporary).st_size > limit:
            raise CaptureError("evidence-cli-refused")
        result = read_json(Path(temporary), limit)
        validate(result)
        private_json(path, result)
        return result
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def capture_lottery(cli, nodes, archive, epoch, source_datadir=None):
    """Keep the public lottery order before the node clears its epoch store.

    The records are local node observations, not a chain-membership proof.
    Both configured nodes must agree before the status says they are consistent.
    """
    destination = archive / f"epoch-{epoch}"
    destination.mkdir(mode=0o700, exist_ok=True)
    records = []
    for node in nodes:
        path = destination / f"{node['name']}-lottery.json"
        if path.exists():
            record = read_json(path, 8_000_000)
        else:
            scratch = destination / f".lottery-{node['name']}"
            scratch.mkdir(mode=0o700, exist_ok=True)
            source = dict(node)
            if source_datadir is not None:
                source["datadir"] = str(source_datadir)
            work = snapshot(source, scratch)
            try:
                def check_lottery(value):
                    if value.get("seedBytes") == 0:
                        raise CaptureError("lottery-not-ready")
                    if (value.get("epoch") != epoch or value.get("seedBytes") != 32
                            or not isinstance(value.get("candidates"), int)
                            or value["candidates"] <= 0
                            or not isinstance(value.get("identities"), list)
                            or len(value["identities"]) != value["candidates"]):
                        raise CaptureError("invalid-lottery-record")
                    # Never publish the temporary host path as evidence.
                    value["dataDir"] = "local-state-capture"

                record = cli_json(cli, ["epoch-db", "-datadir", str(work),
                                         "-inventory-epoch", str(epoch)], path,
                                  check_lottery, 8_000_000)
            except CaptureError as exc:
                if str(exc) == "lottery-not-ready":
                    shutil.rmtree(scratch)
                    return False
                # A failed extraction retains its state copy for repair.
                raise
            finally:
                if path.exists():
                    shutil.rmtree(scratch)
        if (record.get("epoch") != epoch or record.get("seedBytes") != 32
                or not isinstance(record.get("identities"), list)
                or len(record["identities"]) != record.get("candidates")):
            raise CaptureError("saved-lottery-record-invalid")
        records.append(record)
    serialized = [json.dumps(record, sort_keys=True, separators=(",", ":")) for record in records]
    if len(set(serialized)) != 1:
        raise CaptureError("lottery-node-records-disagree")
    private_json(destination / "lottery-ready.json", {
        "status": "consistent_local_node_records", "epoch": epoch,
        "nodes": [node["name"] for node in nodes],
        "seed_bytes": 32, "candidates": records[0]["candidates"],
        "sha256": hashlib.sha256(serialized[0].encode()).hexdigest(),
    })
    return True


def capture_boundary(cli, nodes, archive, epoch, start_block, authors, source_datadir=None):
    if start_block <= 1:
        raise CaptureError("invalid-opening-block")
    destination = archive / f"epoch-{epoch}"
    destination.mkdir(mode=0o700, exist_ok=True)
    anchors = {}
    missing = []
    for node in nodes:
        for phase, height in (("inventory", start_block - 1), ("verdict", start_block)):
            path = destination / f"{node['name']}-{phase}.json"
            if path.exists():
                anchor = read_json(path)
                if anchor.get("height") != height:
                    raise CaptureError("saved-anchor-wrong-height")
                verify_header(nodes, anchor)
                anchors[(node["name"], phase)] = anchor
            else:
                missing.append((node, phase, height, path))
    if missing:
        source = dict(nodes[0])
        if source_datadir is not None:
            source["datadir"] = str(source_datadir)
        work = snapshot(source, destination)
        captured_all_anchors = False
        try:
            for node, phase, height, path in missing:
                def check_anchor(anchor):
                    if anchor.get("height") != height:
                        raise CaptureError("extracted-anchor-wrong-height")
                    verify_header(nodes, anchor)

                anchor = cli_json(cli, ["session-anchor", "-datadir", str(work),
                                        "-state-height", str(height), "-author", authors[node["name"]]],
                                  path, check_anchor)
                anchors[(node["name"], phase)] = anchor
            captured_all_anchors = True
        finally:
            # Keep the boundary version if extraction failed. A later retry or
            # manual repair can still recover it after the live node prunes.
            if captured_all_anchors:
                shutil.rmtree(work)
    records = {}
    for node in nodes:
        name = node["name"]
        output = destination / f"{name}-session-bundle.json"
        def check_bundle(bundle):
            if (bundle.get("epoch") != epoch or address(bundle.get("author")) != authors[name]
                    or not isinstance(bundle.get("manifest"), list) or not bundle["manifest"]
                    or len(bundle["manifest"]) != len(set(bundle["manifest"]))
                    or any(not isinstance(cid, str) or not CID.fullmatch(cid) for cid in bundle["manifest"])):
                raise CaptureError("bundle-identity-or-inventory-mismatch")
            for phase in ("inventory", "verdict"):
                if bundle.get(phase) != anchors[(name, phase)]:
                    raise CaptureError("bundle-anchor-mismatch")

        bundle = cli_json(cli, ["session-bundle", "-inventory-anchor",
                                str(destination / f"{name}-inventory.json"),
                                "-verdict-anchor", str(destination / f"{name}-verdict.json"),
                                "-author", authors[name]], output, check_bundle)
        records[name] = {"bundle_sha256": hashlib.sha256(output.read_bytes()).hexdigest(),
                         "manifest_count": len(bundle["manifest"]),
                         "inventory_height": start_block - 1, "verdict_height": start_block}
    done = {"epoch": epoch, "status": "verified_public_anchors",
            "lottery_record_captured": (destination / "lottery-ready.json").exists(),
            "headers_checked_against": [node["name"] for node in nodes], "bundles": records}
    private_json(destination / "done.json", done)
    return done


def observe(cli, nodes, archive):
    epoch, start, head, _ = canonical_view(nodes)
    watch_file = archive / "watch.json"
    if not watch_file.exists():
        authors = {node["name"]: address(rpc(node, "dna_getCoinbaseAddr")) for node in nodes}
        if len(set(authors.values())) != len(authors):
            raise CaptureError("duplicate-authors")
        private_json(watch_file, {"epoch": epoch, "authors": authors})
        private_json(archive / "status.json", {"status": "armed", "epoch": epoch})
        return "armed", epoch
    watch = read_json(watch_file)
    previous = int(watch["epoch"])
    if epoch == previous:
        return "waiting", epoch
    if epoch < previous:
        raise CaptureError("node-epoch-regressed")
    if epoch > previous + 1:
        private_json(archive / f"missed-after-{previous}.json",
                     {"status": "missed_epoch", "from": previous, "to": epoch})
        authors = {node["name"]: address(rpc(node, "dna_getCoinbaseAddr")) for node in nodes}
        private_json(watch_file, {"epoch": epoch, "authors": authors})
        private_json(archive / "status.json", {"status": "missed_epoch",
                                                "from": previous, "epoch": epoch})
        return "missed", previous
    try:
        result = capture_boundary(cli, nodes, archive, previous, start, watch["authors"])
    except Exception as exc:
        code = str(exc) if isinstance(exc, CaptureError) else type(exc).__name__
        private_json(archive / "status.json", {"status": "capture_failed", "epoch": previous,
                                                "head": head, "reason": code})
        if head >= start + 100:
            private_json(archive / f"missed-epoch-{previous}.json",
                         {"status": "state_pruned_before_capture", "epoch": previous,
                          "reason": code})
            authors = {node["name"]: address(rpc(node, "dna_getCoinbaseAddr")) for node in nodes}
            private_json(watch_file, {"epoch": epoch, "authors": authors})
        raise
    authors = {node["name"]: address(rpc(node, "dna_getCoinbaseAddr")) for node in nodes}
    private_json(watch_file, {"epoch": epoch, "authors": authors})
    private_json(archive / "status.json", {"status": "ready", "epoch": previous,
                                            "next_epoch": epoch, "bundles": result["bundles"]})
    return "captured", previous


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cli", type=Path, required=True)
    parser.add_argument("--archive", type=Path, required=True)
    parser.add_argument("--node", action="append", type=node_arg, required=True)
    parser.add_argument("--once", action="store_true", help="check once, then exit")
    parser.add_argument("--prefetch-blobs", action="store_true",
                        help="retain the current authors' CID-checked encrypted flips, then exit")
    args = parser.parse_args()
    if not args.archive.is_absolute() or not args.cli.is_file() or not os.access(args.cli, os.X_OK):
        parser.error("--archive must be absolute and --cli must be executable")
    nodes = args.node
    if len({node["name"] for node in nodes}) != len(nodes):
        parser.error("node names must be unique")
    if args.once and args.prefetch_blobs:
        parser.error("--once and --prefetch-blobs are separate checks")
    args.archive.mkdir(mode=0o700, parents=True, exist_ok=True)
    os.chmod(args.archive, 0o700)
    if args.prefetch_blobs:
        try:
            epoch, _, _, _ = canonical_view(nodes)
            watch = read_json(args.archive / "watch.json")
            if epoch != watch["epoch"]:
                raise CaptureError("watch-epoch-does-not-match-nodes")
            manifests = live_manifests(nodes, watch["authors"])
            result = retain_blobs(nodes, args.archive, epoch, manifests)
            counts = result["counts"]
            print(f"indexer_encrypted_prefetched_epoch={epoch} " +
                  " ".join(f"{name}={count['retained']}/{count['required']}"
                           for name, count in counts.items()), flush=True)
            return 0 if all(count["retained"] == count["required"] and count["required"]
                            for count in counts.values()) else 1
        except Exception as exc:
            code = str(exc) if isinstance(exc, CaptureError) else type(exc).__name__
            print(f"indexer_encrypted_prefetch_failed:{code}", flush=True)
            return 1
    last_error = None
    last_report = 0
    last_lottery_attempt = 0
    last_blob_attempt = 0
    last_blob_report = None
    while True:
        try:
            status, epoch = observe(args.cli, nodes, args.archive)
            last_error = None
            if status != "waiting":
                print(f"indexer_{status}_epoch={epoch}", flush=True)
            active_epoch = read_json(args.archive / "watch.json")["epoch"]
            lottery_status = args.archive / f"epoch-{active_epoch}" / "lottery-ready.json"
            if not lottery_status.exists() and time.monotonic() - last_lottery_attempt >= 30:
                period = rpc(nodes[0], "dna_epoch").get("currentPeriod")
                if period not in (None, "None", 0):
                    last_lottery_attempt = time.monotonic()
                    if capture_lottery(args.cli, nodes, args.archive, active_epoch):
                        print(f"indexer_lottery_captured_epoch={active_epoch}", flush=True)
            if not args.once and (status == "captured" or
                                  time.monotonic() - last_blob_attempt >= 300):
                last_blob_attempt = time.monotonic()
                previous = active_epoch - 1
                done = args.archive / f"epoch-{previous}" / "done.json"
                if done.exists():
                    path = args.archive / f"epoch-{previous}" / "encrypted-blob-status.json"
                    if not path.exists() or not read_json(path, 8_000_000).get("ready"):
                        final = retain_blobs(nodes, args.archive, previous,
                                             final_manifests(nodes, args.archive, previous),
                                             final=True)
                        if final["ready"]:
                            print(f"indexer_encrypted_ready_epoch={previous}", flush=True)
                manifests = live_manifests(nodes, read_json(args.archive / "watch.json")["authors"])
                if any(manifests.values()):
                    current = retain_blobs(nodes, args.archive, active_epoch, manifests)
                    report = (active_epoch, tuple((name, count["retained"], count["required"])
                                                  for name, count in current["counts"].items()))
                    if report != last_blob_report:
                        print(f"indexer_encrypted_prefetched_epoch={active_epoch} " +
                              " ".join(f"{name}={count['retained']}/{count['required']}"
                                       for name, count in current["counts"].items()), flush=True)
                        last_blob_report = report
        except Exception as exc:
            code = str(exc) if isinstance(exc, CaptureError) else type(exc).__name__
            if code != last_error or time.monotonic() - last_report > 1800:
                print(f"indexer_waiting_after:{code}", flush=True)
                last_error = code
                last_report = time.monotonic()
            if args.once:
                return 1
        if args.once:
            return 0
        time.sleep(5)


if __name__ == "__main__":
    raise SystemExit(main())
