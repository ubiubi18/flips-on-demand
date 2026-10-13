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


if __name__ == "__main__":
    unittest.main()
