"""Focused failure/recovery checks for the optional capture watcher."""

import errno
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import session_indexer as indexer


FAKE_CLI = """#!/usr/bin/env python3
import json, os, sys
args = sys.argv[1:]
source = args[args.index('-datadir') + 1]
name = 'b' if '.lottery-b' in source else 'a'
mode = os.environ.get('FOD_TEST_LOTTERY', 'ready')
if mode == 'fail-b' and name == 'b':
    raise SystemExit(1)
seed = '0' * 64 if mode != 'mismatch' or name == 'a' else '1' * 64
value = {
    'dataDir': source, 'epoch': 229, 'seedHex': seed,
    'seedBytes': 0 if mode == 'not-ready' else 32,
    'candidates': 0 if mode == 'not-ready' else 1,
    'identities': [] if mode == 'not-ready' else [{'address': '0x' + '1' * 40}],
}
with open(args[args.index('-out') + 1], 'w') as output:
    json.dump(value, output)
"""

PUBLIC_CLI = """#!/usr/bin/env python3
import json, os, sys
args = sys.argv[1:]
mode = args[0]
author = args[args.index('-author') + 1]
cids = json.loads(os.environ['FOD_TEST_PUBLIC_CIDS'])
if author not in cids:
    raise SystemExit(2)
if mode == 'session-anchor':
    value = {'height': int(args[args.index('-state-height') + 1]),
             'rootHex': 'a' * 64, 'identity': {'keyHex': author}}
elif mode == 'session-bundle':
    with open(args[args.index('-inventory-anchor') + 1]) as stream:
        inventory = json.load(stream)
    with open(args[args.index('-verdict-anchor') + 1]) as stream:
        verdict = json.load(stream)
    if inventory['identity']['keyHex'] != author or verdict['identity']['keyHex'] != author:
        raise SystemExit(3)
    value = {'epoch': 229, 'author': author, 'manifest': [cids[author]],
             'inventory': inventory, 'verdict': verdict}
else:
    raise SystemExit(4)
with open(args[args.index('-out') + 1], 'w') as output:
    json.dump(value, output)
"""


class SessionIndexerLotteryTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        base = Path(self.temporary.name)
        self.archive = base / "archive"
        self.archive.mkdir(mode=0o700)
        self.cli = base / "fake-fod"
        self.cli.write_text(FAKE_CLI)
        self.cli.chmod(0o700)
        self.nodes = []
        for name in ("a", "b"):
            data = base / name
            db = data / "idenachain.db"
            db.mkdir(parents=True)
            (db / "CURRENT").write_text("MANIFEST-000001\n")
            (db / "MANIFEST-000001").write_text("fixture")
            (db / "000001.ldb").write_text("fixture")
            self.nodes.append({"name": name, "port": 1, "datadir": str(data)})

    def test_snapshot_copies_tables_when_hard_link_crosses_mounts(self):
        table = Path(self.nodes[0]["datadir"]) / "idenachain.db" / "000001.ldb"
        with patch.object(indexer.os, "link", side_effect=OSError(errno.EXDEV, "cross-device link")) as link:
            kept = indexer.snapshot(self.nodes[0], self.archive)

        link.assert_called_once()
        self.assertEqual(link.call_args.args[0], table)
        self.assertEqual((kept / "idenachain.db" / table.name).read_bytes(), table.read_bytes())
        self.assertNotEqual((kept / "idenachain.db" / table.name).stat().st_ino, table.stat().st_ino)
        self.assertEqual((kept / "idenachain.db" / "CURRENT").read_text(), "MANIFEST-000001\n")
        self.assertEqual(list(self.archive.glob(".state-staging-*")), [])

    def test_snapshot_preserves_other_link_errors_and_cleans_staging(self):
        with patch.object(indexer.os, "link", side_effect=OSError(errno.EACCES, "permission denied")):
            with self.assertRaises(OSError) as raised:
                indexer.snapshot(self.nodes[0], self.archive)

        self.assertEqual(raised.exception.errno, errno.EACCES)
        self.assertFalse((self.archive / ".state-snapshot").exists())
        self.assertEqual(list(self.archive.glob(".state-staging-*")), [])

    def test_snapshot_refuses_to_spend_identity_disk_reserve(self):
        with patch.object(indexer.shutil, "disk_usage") as usage:
            usage.return_value.free = 11 * 1024**3
            with self.assertRaisesRegex(indexer.CaptureError, "insufficient-free-disk"):
                indexer.snapshot(self.nodes[0], self.archive)
        self.assertFalse((self.archive / ".state-snapshot").exists())
        self.assertEqual(list(self.archive.glob(".state-staging-*")), [])

    def test_snapshot_aborts_and_cleans_if_other_writer_consumes_reserve(self):
        from types import SimpleNamespace
        with patch.object(indexer.shutil, "disk_usage", side_effect=[
            SimpleNamespace(free=13 * 1024**3),
            SimpleNamespace(free=indexer.MIN_FREE_DISK_BYTES),
        ]):
            with self.assertRaisesRegex(indexer.CaptureError, "insufficient-free-disk"):
                indexer.snapshot(self.nodes[0], self.archive)
        self.assertFalse((self.archive / ".state-snapshot").exists())
        self.assertEqual(list(self.archive.glob(".state-staging-*")), [])

    def test_cross_device_table_copy_checks_reserve_for_every_chunk(self):
        from types import SimpleNamespace
        source = Path(self.nodes[0]["datadir"]) / "idenachain.db" / "000001.ldb"
        source.write_bytes(b"x" * (indexer.COPY_CHUNK_BYTES + 1))
        target = self.archive / "copy.ldb"
        with patch.object(indexer.shutil, "disk_usage", side_effect=[
            SimpleNamespace(free=indexer.MIN_FREE_DISK_BYTES + indexer.COPY_CHUNK_BYTES),
            SimpleNamespace(free=indexer.MIN_FREE_DISK_BYTES),
        ]):
            with self.assertRaisesRegex(indexer.CaptureError, "insufficient-free-disk"):
                indexer.copy_with_disk_reserve(source, target)
        self.assertEqual(source.stat().st_size, indexer.COPY_CHUNK_BYTES + 1)
        self.assertEqual(target.stat().st_size, indexer.COPY_CHUNK_BYTES)

    def test_not_ready_retains_no_early_lottery_claim(self):
        with patch.dict(os.environ, {"FOD_TEST_LOTTERY": "not-ready"}):
            self.assertFalse(indexer.capture_lottery(self.cli, self.nodes, self.archive, 229))
        epoch = self.archive / "epoch-229"
        self.assertFalse((epoch / "lottery-ready.json").exists())
        self.assertFalse((epoch / ".lottery-a").exists())

    def test_consistent_records_discard_snapshots_and_redact_host_path(self):
        self.assertTrue(indexer.capture_lottery(self.cli, self.nodes, self.archive, 229))
        epoch = self.archive / "epoch-229"
        ready = json.loads((epoch / "lottery-ready.json").read_text())
        self.assertEqual(ready["status"], "consistent_local_node_records")
        for name in ("a", "b"):
            record = json.loads((epoch / f"{name}-lottery.json").read_text())
            self.assertEqual(record["dataDir"], "local-state-capture")
            self.assertFalse((epoch / f".lottery-{name}").exists())

    def test_disagreement_and_failure_never_publish_ready(self):
        for mode in ("mismatch", "fail-b"):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as target:
                with patch.dict(os.environ, {"FOD_TEST_LOTTERY": mode}):
                    with self.assertRaises(indexer.CaptureError):
                        indexer.capture_lottery(self.cli, self.nodes, Path(target), 229)
                epoch = Path(target) / "epoch-229"
                self.assertFalse((epoch / "lottery-ready.json").exists())
                self.assertEqual((epoch / ".lottery-b").exists(), mode == "fail-b")

    def test_common_finalized_height_accepts_lag_but_refuses_a_fork(self):
        def answer(node, method, params=None):
            if method == "dna_epoch":
                return {"epoch": 230, "startBlock": 100}
            if method == "bcn_lastBlock":
                return {"height": 105 if node["name"] == "a" else 103}
            if method == "bcn_blockAt":
                self.assertEqual(params, [103])
                return {"hash": "0xabc", "root": "0xdef"}
            raise AssertionError(method)

        with patch.object(indexer, "rpc", side_effect=answer):
            self.assertEqual(indexer.canonical_view(self.nodes), (230, 100, 103, "0xabc"))

        def forked(node, method, params=None):
            result = answer(node, method, params)
            if method == "bcn_blockAt" and node["name"] == "b":
                result["hash"] = "0xfork"
            return result

        with patch.object(indexer, "rpc", side_effect=forked):
            with self.assertRaisesRegex(indexer.CaptureError, "common-block"):
                indexer.canonical_view(self.nodes)

    def test_encrypted_blobs_cover_all_flips_and_reuse_verified_bytes(self):
        small = b"encrypted-small-flip"
        large = b"e" * 300_000
        small_cid, large_cid = indexer.flip_cid(small), indexer.flip_cid(large)
        manifests = {"a": [small_cid, large_cid], "b": [large_cid]}
        blobs = {small_cid: small, large_cid: large}

        def fetch(node, method, params=None, limit=1_000_000):
            self.assertEqual(method, "ipfs_get")
            self.assertEqual(limit, 1_300_000)
            return "0x" + blobs[params[0]].hex()

        with patch.object(indexer, "rpc", side_effect=fetch) as remote:
            prefetched = indexer.retain_blobs(self.nodes, self.archive, 230, manifests)
            self.assertEqual(remote.call_count, 3)
        self.assertFalse(prefetched["ready"])
        self.assertEqual(prefetched["counts"], {
            "a": {"retained": 2, "required": 2}, "b": {"retained": 1, "required": 1},
        })
        for name, cids in manifests.items():
            for cid in cids:
                saved = self.archive / "epoch-230" / "encrypted-blobs" / name / cid
                self.assertEqual(saved.read_bytes(), blobs[cid])
                self.assertEqual(saved.stat().st_mode & 0o777, 0o600)
        with patch.object(indexer, "rpc", side_effect=AssertionError("unneeded fetch")):
            final = indexer.retain_blobs(self.nodes, self.archive, 230, manifests, final=True)
        self.assertTrue(final["ready"])
        self.assertEqual(final["phase"], "final")

    def test_wrong_encrypted_bytes_never_become_ready(self):
        cid = indexer.flip_cid(b"expected ciphertext")
        manifests = {"a": [cid], "b": [cid]}
        with patch.object(indexer, "rpc", return_value="0x" + b"wrong ciphertext".hex()):
            result = indexer.retain_blobs(self.nodes, self.archive, 230, manifests, final=True)
        self.assertFalse(result["ready"])
        self.assertEqual(result["counts"]["a"], {"retained": 0, "required": 1})
        self.assertFalse((self.archive / "epoch-230" / "encrypted-blobs" / "a" / cid).exists())

    def test_public_third_party_authors_keep_distinct_identity_and_immutable_evidence(self):
        self.cli.write_text(PUBLIC_CLI)
        blobs = {"third": b"original-third-party-ciphertext",
                 "fourth": b"original-fourth-party-ciphertext"}
        authors = {"third": "3" * 40, "fourth": "4" * 40}
        cids = {authors[label]: indexer.flip_cid(blob) for label, blob in blobs.items()}
        by_cid = {indexer.flip_cid(blob): blob for blob in blobs.values()}
        public = self.archive / "public-authors"

        def answer(node, method, params=None, limit=1_000_000):
            if method == "dna_identity":
                self.assertIn(params[0][2:], authors.values())
                return {"flips": [cids[params[0][2:]]]}
            if method == "ipfs_get":
                return "0x" + by_cid[params[0]].hex()
            if method == "bcn_blockAt":
                return {"root": "0x" + "a" * 64, "hash": "0x" + "b" * 64}
            raise AssertionError(method)

        with patch.dict(os.environ, {"FOD_TEST_PUBLIC_CIDS": json.dumps(cids)}):
            with patch.object(indexer, "rpc", side_effect=answer):
                current = indexer.public_manifests(self.nodes, public, 229, authors)
                self.assertEqual(current, {label: [cids[author]] for label, author in authors.items()})
                indexer.retain_public_blobs(self.nodes, public, 229, authors, current)
                indexer.capture_public_boundary(self.cli, self.nodes, public, 229, 101, authors)
                completed = indexer.finish_public_epoch(self.cli, self.nodes, public, 229, authors)
                self.assertTrue(all(result["ready"] for result in completed.values()))

                saved = {}
                for label, author in authors.items():
                    home = public / "epoch-229" / "authors" / label
                    self.assertEqual((home / "encrypted-blobs" / cids[author]).read_bytes(), blobs[label])
                    bundle = json.loads((home / "session-bundle.json").read_text())
                    self.assertEqual(bundle["author"], author)
                    self.assertEqual(bundle["inventory"]["identity"]["keyHex"], author)
                    marker = json.loads((home / "ready.json").read_text())
                    self.assertEqual(marker["format"], "fod.shared-evidence-ready:v1")
                    self.assertEqual(marker["manifest"], [cids[author]])
                    self.assertEqual(marker["blobs"][cids[author]]["bytes"], len(blobs[label]))
                    self.assertEqual(marker["headersCheckedAgainst"], ["a", "b"])
                    saved[label] = {name: ((home / name).read_bytes(), (home / name).stat().st_ino)
                                    for name in ("inventory.json", "verdict.json",
                                                 "session-bundle.json", "ready.json")}

                indexer.capture_public_boundary(self.cli, self.nodes, public, 229, 101, authors)
                indexer.finish_public_epoch(self.cli, self.nodes, public, 229, authors)
                for label in authors:
                    home = public / "epoch-229" / "authors" / label
                    for name, evidence in saved[label].items():
                        self.assertEqual(((home / name).read_bytes(), (home / name).stat().st_ino),
                                         evidence)

        changed = dict(cids)
        changed[authors["third"]] = indexer.flip_cid(b"substituted")
        with patch.dict(os.environ, {"FOD_TEST_PUBLIC_CIDS": json.dumps(changed)}):
            with patch.object(indexer, "rpc", side_effect=answer):
                with self.assertRaisesRegex(indexer.CaptureError, "saved-public-evidence-disagrees"):
                    indexer.capture_public_boundary(self.cli, self.nodes, public, 229, 101, authors)
        home = public / "epoch-229" / "authors" / "third"
        self.assertEqual((home / "session-bundle.json").read_bytes(),
                         saved["third"]["session-bundle.json"][0])

    def test_public_final_requires_two_headers_and_all_cid_checked_bytes(self):
        self.cli.write_text(PUBLIC_CLI)
        author = "3" * 40
        authors = {"third": author}
        cid = indexer.flip_cid(b"expected ciphertext")
        public = self.archive / "public-authors"

        def answer(node, method, params=None, limit=1_000_000):
            if method == "bcn_blockAt":
                return {"root": "0x" + "a" * 64, "hash": "0x" + "b" * 64}
            if method == "ipfs_get":
                return "0x" + b"wrong ciphertext".hex()
            raise AssertionError(method)

        with patch.dict(os.environ, {"FOD_TEST_PUBLIC_CIDS": json.dumps({author: cid})}):
            with patch.object(indexer, "rpc", side_effect=answer):
                with patch.object(indexer, "canonical_view", side_effect=[
                        (229, 10, 99, "0x" + "b" * 64),
                        (230, 101, 101, "0x" + "b" * 64)]):
                    self.assertEqual(indexer.observe_public(self.cli, self.nodes[:1], public, authors),
                                     ("armed", 229))
                    self.assertEqual(indexer.observe_public(self.cli, self.nodes[:1], public, authors),
                                     ("anchored", 229))
                home = public / "epoch-229" / "authors" / "third"
                self.assertTrue((home / "inventory.json").exists())
                self.assertFalse((home / "session-bundle.json").exists())
                self.assertEqual(json.loads((public / "watch.json").read_text())["pending"], [229])

                def forked(node, method, params=None, limit=1_000_000):
                    if method == "bcn_blockAt" and node["name"] == "b":
                        return {"root": "0x" + "c" * 64, "hash": "0x" + "d" * 64}
                    return answer(node, method, params, limit)

                with patch.object(indexer, "rpc", side_effect=forked):
                    with self.assertRaisesRegex(indexer.CaptureError, "anchor-header-root-mismatch"):
                        indexer.capture_public_boundary(self.cli, self.nodes, public, 229, 101, authors)
                self.assertFalse((home / "session-bundle.json").exists())
                indexer.capture_public_boundary(self.cli, self.nodes, public, 229, 101, authors)
                result = indexer.finish_public_epoch(self.cli, self.nodes, public, 229, authors)
        self.assertFalse(result["third"]["ready"])
        self.assertFalse((public / "epoch-229" / "authors" / "third" / "ready.json").exists())

    def test_public_keeps_one_observer_when_the_other_node_is_unavailable(self):
        def unavailable(node, method, params=None, limit=1_000_000):
            if node["name"] == "b":
                raise indexer.CaptureError("node-rpc-unavailable")
            return {"epoch": 229} if method == "dna_epoch" else {"height": 100}

        with patch.object(indexer, "rpc", side_effect=unavailable):
            self.assertEqual(indexer.available_public_nodes(self.nodes), self.nodes[:1])

        with patch.object(indexer, "rpc", return_value={"height": 100}):
            self.assertEqual(indexer.available_public_nodes(self.nodes), self.nodes)


if __name__ == "__main__":
    unittest.main()
