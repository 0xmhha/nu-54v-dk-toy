#!/usr/bin/env python3
"""Validate the baseline pins and fail closed on contradictory archive evidence."""

import copy
import hashlib
import json
import tempfile
import unittest
from pathlib import Path

import verify


class BaselineTests(unittest.TestCase):
    def setUp(self):
        self.baseline = verify.load(verify.HERE / "v1.json")

    def test_committed_source_and_deployment_pins(self):
        verify.check(self.baseline)

    def test_changed_source_pin_fails(self):
        self.baseline["source"]["objects"]["pnpm-lock.yaml"] = "0" * 40
        with self.assertRaisesRegex(ValueError, "pnpm-lock.yaml"):
            verify.check(self.baseline)

    def test_different_archived_block_fails_even_when_checksum_matches(self):
        data = copy.deepcopy(self.baseline["confirmedPayment"])
        body = json.dumps(dict(blockNumber=hex(data["block"]), blockHash="0x" + "0" * 64, status="0x1"))
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "receipt.json"
            path.write_text(body)
            data.update(receiptPath="receipt.json", receiptSha256=hashlib.sha256(body.encode()).hexdigest())
            with self.assertRaisesRegex(ValueError, "receipt block"):
                verify.check_receipt(data, Path(directory))

    def test_receipt_checksum_mismatch_fails(self):
        data = self.baseline["confirmedPayment"]
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "receipt.json"
            path.write_text("{}")
            data["receiptPath"] = "receipt.json"
            with self.assertRaisesRegex(ValueError, "receipt SHA-256"):
                verify.check_receipt(data, Path(directory))

    def test_partial_manifest_fails(self):
        for entry, key in (("source", "objects"), ("apps", "P02"), ("deployment", "contracts")):
            with self.subTest(entry=entry):
                manifest = copy.deepcopy(self.baseline)
                if entry == "deployment":
                    del manifest[entry][key]["PaymentSettlement"]
                elif entry == "source":
                    del manifest[entry][key]["packages/protocol"]
                else:
                    del manifest[entry][key]
                with self.assertRaisesRegex(ValueError, "coverage"):
                    verify.check(manifest)

    def test_abi_mismatch_fails(self):
        self.baseline["deployment"]["abiSha256"]["TestUSDC"] = "0" * 64
        with self.assertRaisesRegex(ValueError, "ABI"):
            verify.check(self.baseline)


if __name__ == "__main__":
    unittest.main()
