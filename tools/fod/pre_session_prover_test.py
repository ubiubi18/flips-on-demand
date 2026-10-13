"""Focused no-duplicate and identity guards for the local proof scheduler."""

import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import pre_session_prover as prover


class PreSessionProverTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.output = Path(self.temporary.name) / "private"
        self.contract = "1" * 40
        self.image = "2" * 64
        self.author = "3" * 40
        self.snapshot = {"state": "ready", "identity": "seller2", "epoch": 231,
                         "author": self.author, "manifest": ["cid-a"],
                         "inventoryDigest": "4" * 64, "witnessSha256": "5" * 64}
        self.config = {"output": str(self.output), "minEpoch": 231, "prepareBinary": "prepare",
                       "proveBinary": "prove"}

    def fake_prove(self, config, args, log, status):
        journal = bytearray(273)
        journal[36:56] = bytes.fromhex(self.contract)
        journal[56:76] = bytes.fromhex(self.author)
        journal[76:78] = (231).to_bytes(2, "big")
        journal[78:80] = (1).to_bytes(2, "big")
        proof = {"format": "fod:fast-content-sealed:v1", "image_id": self.image,
                 "journal": journal.hex(), "seal": "00" * 256, "sealBytes": 256,
                 "items": [{"originalCid": "cid-a"}]}
        prover.write_new(Path(args[args.index("--out") + 1]), json.dumps(proof).encode())

    def test_proof_runs_once_for_one_author_inventory(self):
        witness = json.dumps({"author": self.author, "epoch": 231, "blobCid": "cid-a"}).encode()
        self.snapshot["witnessSha256"] = hashlib.sha256(witness).hexdigest()
        with patch.object(prover, "remote_snapshot", return_value=self.snapshot), \
                patch.object(prover, "remote_witness", return_value=witness), \
                patch.object(prover, "run_step"), \
                patch.object(prover, "verify_content_proof") as verify, \
                patch.object(prover, "prove", side_effect=self.fake_prove) as prove:
            first = prover.run_job(self.config, "seller2", self.author, self.contract, self.image)
            second = prover.run_job(self.config, "seller2", self.author, self.contract, self.image)
        self.assertEqual(first["state"], "ready")
        self.assertEqual(second["state"], "ready")
        prove.assert_called_once()
        self.assertEqual(verify.call_count, 2)

    def test_failed_seal_blocks_reusing_or_restarting_proof(self):
        witness = json.dumps({"author": self.author, "epoch": 231, "blobCid": "cid-a"}).encode()
        self.snapshot["witnessSha256"] = hashlib.sha256(witness).hexdigest()
        with patch.object(prover, "remote_snapshot", return_value=self.snapshot), \
                patch.object(prover, "remote_witness", return_value=witness), \
                patch.object(prover, "run_step"), \
                patch.object(prover, "verify_content_proof", side_effect=prover.JobError("content-proof-seal-unverified")), \
                patch.object(prover, "prove", side_effect=self.fake_prove) as prove:
            with self.assertRaisesRegex(prover.JobError, "content-proof-seal-unverified"):
                prover.run_job(self.config, "seller2", self.author, self.contract, self.image)
            with self.assertRaisesRegex(prover.JobError, "previous-attempt-needs-review"):
                prover.run_job(self.config, "seller2", self.author, self.contract, self.image)
        prove.assert_called_once()

    def test_other_identity_is_rejected_before_witness_transfer(self):
        with patch.object(prover, "remote_snapshot", return_value=self.snapshot), \
                patch.object(prover, "remote_witness") as fetch:
            with self.assertRaisesRegex(prover.JobError, "node-identity-mismatch"):
                prover.run_job(self.config, "seller2", "4" * 40, self.contract, self.image)
        fetch.assert_not_called()

    def test_host_image_mismatch_stops_before_node_access(self):
        manifest = Path(self.temporary.name) / "manifest.json"
        manifest.write_text(json.dumps({"format": "fod.fast-candidate-manifest", "version": 1,
                                        "proofProgram": {"imageId": self.image},
                                        "artifact": {"sha256": "4" * 64}}))
        config = dict(self.config, manifest=str(manifest), contract=self.contract,
                      contractCodeHash="5" * 64, contractSha256="4" * 64)
        with patch.object(prover, "proof_host_image", return_value="0" * 64), \
                patch.object(prover, "remote_snapshot") as snapshot:
            with self.assertRaisesRegex(prover.JobError, "proof-host-image-mismatch"):
                prover.run(config)
        snapshot.assert_not_called()

    def test_release_manifest_pins_allow_fast_and_direct_v5(self):
        release = Path(__file__).resolve().parents[2] / "release"
        cases = (
            ("FAST_MANIFEST.json", "b8586d40d7d5b912a1ce3863009c9ccb88eb836d2b441a3a08ce32ecc85bd7d2", "5" * 64),
            ("DIRECT_V5_MANIFEST.json", "4fd124c8cb91370dab60d3e1d7c4e666e79485e210b5f20ed36cc2104883726c",
             "45d1d0ebf1140e0f9a7d0383878a6e6e00c424ec12c12de78cacb92ec92f268a"),
        )
        for filename, image, code_hash in cases:
            with self.subTest(filename=filename):
                manifest_path = release / filename
                manifest = json.loads(manifest_path.read_text())
                config = dict(self.config, manifest=str(manifest_path), contract=self.contract,
                              contractCodeHash=code_hash, contractSha256=manifest["artifact"]["sha256"],
                              identities=[{"name": "seller2", "author": self.author}])
                with patch.object(prover, "proof_host_image", return_value=image) as host, \
                        patch.object(prover, "remote_snapshot", return_value={"state": "waiting"}) as snapshot:
                    self.assertEqual(prover.run(config), [{"identity": "seller2", "state": "waiting", "reason": None}])
                host.assert_called_once()
                snapshot.assert_called_once()

    def test_direct_v5_wrong_or_ambiguous_pins_stop_before_host_and_node_access(self):
        base = {"format": "fod.direct-candidate-manifest", "version": 5,
                "artifact": {"sha256": "4" * 64, "codeHash": "5" * 64},
                "checks": {"fastContentImageId": self.image}}
        for case in ("wrong-sha256", "wrong-keccak", "missing-keccak", "mixed-image-fields", "duplicate-json-key"):
            with self.subTest(case=case):
                manifest = json.loads(json.dumps(base))
                config = dict(self.config, contract=self.contract, contractCodeHash="5" * 64,
                              contractSha256="4" * 64, identities=[])
                if case == "wrong-sha256":
                    config["contractSha256"] = "6" * 64
                elif case == "wrong-keccak":
                    config["contractCodeHash"] = "6" * 64
                elif case == "missing-keccak":
                    del manifest["artifact"]["codeHash"]
                elif case == "mixed-image-fields":
                    manifest["proofProgram"] = {"imageId": self.image}
                path = Path(self.temporary.name) / f"{case}.json"
                if case == "duplicate-json-key":
                    path.write_text(json.dumps(base).replace('"sha256":', '"sha256": "0", "sha256":', 1))
                else:
                    path.write_text(json.dumps(manifest))
                config["manifest"] = str(path)
                with patch.object(prover, "proof_host_image") as host, \
                        patch.object(prover, "remote_snapshot") as snapshot:
                    with self.assertRaises(prover.JobError):
                        prover.run(config)
                host.assert_not_called()
                snapshot.assert_not_called()

    def test_stale_hold_releases_only_after_process_group_exits(self):
        hold = {"state": "running", "pid": 1234}
        with patch.object(prover.os, "killpg") as probe:
            self.assertTrue(prover.held_proof_running(hold))
            probe.assert_called_once_with(1234, 0)
        with patch.object(prover.os, "killpg", side_effect=ProcessLookupError):
            self.assertFalse(prover.held_proof_running(hold))
        self.assertTrue(prover.held_proof_running({"state": "starting"}))
        self.assertFalse(prover.held_proof_running({"state": "complete", "pid": 1234}))

    def test_previous_proof_attempt_blocks_a_new_digest_after_scheduler_crash(self):
        earlier = self.output / "epoch-231" / "seller2" / ("a" * 64) / "status.json"
        prover.private_json(earlier, {"state": "proving", "pid": 1234})
        with patch.object(prover, "remote_snapshot") as snapshot:
            with self.assertRaisesRegex(prover.JobError, "previous-attempt-needs-review"):
                prover.run({"output": str(self.output)})
        snapshot.assert_not_called()


if __name__ == "__main__":
    unittest.main()
