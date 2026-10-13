"""Focused pre-session snapshot and identity-separation checks."""

import hashlib
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import pre_session_snapshot as snap
import session_indexer as indexer


class PreSessionSnapshotTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        base = Path(self.temporary.name)
        self.archive = base / "archive"
        self.archive.mkdir(mode=0o700)
        self.output = base / "private"
        self.contract = "1" * 40
        self.code_hash = "2" * 64
        self.nodes = []
        self.authors = {"seller2": "a" * 40, "seller3": "b" * 40}
        for name in self.authors:
            datadir = base / name
            (datadir / "keystore").mkdir(parents=True)
            (datadir / "keystore" / "nodekey").write_bytes(b"private-test-key")
            (datadir / "keystore" / "nodekey").chmod(0o600)
            self.nodes.append({"name": name, "port": 1, "datadir": str(datadir)})
        indexer.private_json(self.archive / "watch.json", {"epoch": 231, "authors": self.authors})
        self.blobs = [b"first submitted flip", b"second submitted flip"]
        self.manifest = sorted(indexer.flip_cid(blob) for blob in self.blobs)
        directory = self.archive / "epoch-231" / "encrypted-blobs" / "seller2"
        directory.mkdir(parents=True, mode=0o700)
        records = {}
        for blob in self.blobs:
            cid = indexer.flip_cid(blob)
            (directory / cid).write_bytes(blob)
            (directory / cid).chmod(0o600)
            records[cid] = {"cid_verified": True, "sha256": hashlib.sha256(blob).hexdigest()}
        indexer.private_json(self.archive / "epoch-231" / "encrypted-blob-status.json",
                             {"epoch": 231, "phase": "prefetch", "blobs": {"seller2": records}})
        self.cli = base / "fake-fod"
        self.cli.write_text("""#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
args = sys.argv
inventory = json.loads(Path(args[args.index('-pre-session-inventory') + 1]).read_text())
directory = Path(args[args.index('-blob-dir') + 1])
output = Path(args[args.index('-out') + 1])
with open(output.parent / 'calls', 'a') as stream:
    stream.write('called\\n')
blobs = [{'blobCid': cid, 'blob': (directory / cid).read_bytes().hex()} for cid in inventory['manifest']]
output.write_text(json.dumps({'author': inventory['author'], 'epoch': inventory['epoch'],
                              'blobs': blobs, 'publicPartSecret': '0' * 64,
                              'privatePartSecret': '1' * 64}))
os.chmod(output, 0o600)
""")
        self.cli.chmod(0o700)

    def answer(self, node, method, params=None):
        if method == "dna_getCoinbaseAddr":
            return "0x" + self.authors[node["name"]]
        if method == "dna_epoch":
            return {"epoch": 231, "currentPeriod": None}
        if method == "dna_identity":
            self.assertEqual(params, ["0x" + self.authors["seller2"]])
            return {"flips": list(reversed(self.manifest))}
        if method == "contract_getStake":
            self.assertEqual(params, ["0x" + self.contract])
            return {"Hash": "0x" + self.code_hash}
        raise AssertionError(method)

    def test_snapshot_reuses_exact_manifest_without_exposing_witness(self):
        with patch.object(indexer, "canonical_view", return_value=(231, 1, 10, "hash")), \
                patch.object(indexer, "rpc", side_effect=self.answer):
            with self.assertRaisesRegex(snap.NotReady, "inventory-settling"):
                snap.snapshot(self.cli, self.archive, self.nodes, "seller2", self.output,
                              self.contract, self.code_hash, 0)
            first = snap.snapshot(self.cli, self.archive, self.nodes, "seller2", self.output,
                                  self.contract, self.code_hash, 0)
            again = snap.snapshot(self.cli, self.archive, self.nodes, "seller2", self.output,
                                  self.contract, self.code_hash, 0)
        self.assertEqual(first, again)
        self.assertEqual(first["state"], "ready")
        self.assertEqual(first["manifest"], self.manifest)
        folder = self.output / "epoch-231" / "seller2" / first["inventoryDigest"]
        self.assertEqual((folder / "calls").read_text().count("called"), 1)
        witness = snap.fetch(self.output, 231, "seller2", first["inventoryDigest"], first["witnessSha256"])
        self.assertIn(b"publicPartSecret", witness)
        self.assertNotIn("publicPartSecret", json.dumps(first))

    def test_changed_identity_or_incomplete_blob_refuses_before_key_use(self):
        def wrong_coinbase(node, method, params=None):
            if method == "dna_getCoinbaseAddr" and node["name"] == "seller2":
                return "0x" + self.authors["seller3"]
            return self.answer(node, method, params)

        with patch.object(indexer, "canonical_view", return_value=(231, 1, 10, "hash")), \
                patch.object(indexer, "rpc", side_effect=wrong_coinbase):
            with self.assertRaisesRegex(indexer.CaptureError, "node-identity-changed"):
                snap.snapshot(self.cli, self.archive, self.nodes, "seller2", self.output,
                              self.contract, self.code_hash, 0)
        status_path = self.archive / "epoch-231" / "encrypted-blob-status.json"
        status = json.loads(status_path.read_text())
        status["blobs"]["seller2"].pop(self.manifest[0])
        indexer.private_json(status_path, status)
        with patch.object(indexer, "canonical_view", return_value=(231, 1, 10, "hash")), \
                patch.object(indexer, "rpc", side_effect=self.answer):
            with self.assertRaisesRegex(snap.NotReady, "blob-capture-incomplete"):
                snap.snapshot(self.cli, self.archive, self.nodes, "seller2", self.output,
                              self.contract, self.code_hash, 0)
        self.assertFalse(list(self.output.glob("epoch-*/seller2/*/witness.json")))

    def test_new_submission_resets_stability_and_cannot_reuse_old_witness(self):
        with patch.object(indexer, "canonical_view", return_value=(231, 1, 10, "hash")), \
                patch.object(indexer, "rpc", side_effect=self.answer):
            with self.assertRaises(snap.NotReady):
                snap.snapshot(self.cli, self.archive, self.nodes, "seller2", self.output,
                              self.contract, self.code_hash, 0)
            old = snap.snapshot(self.cli, self.archive, self.nodes, "seller2", self.output,
                                self.contract, self.code_hash, 0)
        added = b"third submitted flip"
        cid = indexer.flip_cid(added)
        self.manifest = sorted([*self.manifest, cid])
        directory = self.archive / "epoch-231" / "encrypted-blobs" / "seller2"
        (directory / cid).write_bytes(added)
        (directory / cid).chmod(0o600)
        status_path = self.archive / "epoch-231" / "encrypted-blob-status.json"
        status = json.loads(status_path.read_text())
        status["blobs"]["seller2"][cid] = {
            "cid_verified": True, "sha256": hashlib.sha256(added).hexdigest()}
        indexer.private_json(status_path, status)
        with patch.object(indexer, "canonical_view", return_value=(231, 1, 10, "hash")), \
                patch.object(indexer, "rpc", side_effect=self.answer):
            with self.assertRaisesRegex(snap.NotReady, "inventory-settling"):
                snap.snapshot(self.cli, self.archive, self.nodes, "seller2", self.output,
                              self.contract, self.code_hash, 0)
            new = snap.snapshot(self.cli, self.archive, self.nodes, "seller2", self.output,
                                self.contract, self.code_hash, 0)
        self.assertNotEqual(new["inventoryDigest"], old["inventoryDigest"])
        self.assertEqual(len(new["manifest"]), 3)

    def test_node_key_must_be_private_regular_file(self):
        key = Path(self.nodes[0]["datadir"]) / "keystore" / "nodekey"
        snap.checked_nodekey(key)
        key.chmod(0o640)
        with self.assertRaisesRegex(indexer.CaptureError, "unsafe-node-key"):
            snap.checked_nodekey(key)
        key.unlink()
        key.symlink_to(Path(self.nodes[1]["datadir"]) / "keystore" / "nodekey")
        with self.assertRaisesRegex(indexer.CaptureError, "unsafe-node-key"):
            snap.checked_nodekey(key)


if __name__ == "__main__":
    unittest.main()
