"""Test the pinned dependency inventory's per-artifact provenance."""

import unittest

import inventory


class InventoryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.artifacts = {item["id"]: item for item in inventory.generate()["artifacts"]}

    def test_firmware_vendor_and_contract_test_vendor_are_separate(self):
        firmware = self.artifacts["p01-firmware"]
        self.assertIn("secp256k1", {item["name"] for item in firmware["components"]})
        settlement = self.artifacts["p06-contract-deployment"]
        self.assertNotIn("forge-std", {item["name"] for item in settlement["components"]})
        self.assertIn("forge-std", {item["name"] for item in self.artifacts["p06-contract-tests"]["components"]})

    def test_app_graph_contains_direct_transitive_and_development_components(self):
        renter = self.artifacts["p02-android"]
        kiosk = self.artifacts["p04-android"]
        for artifact in (renter, kiosk):
            graph = artifact["components"]
            self.assertTrue(any(item["name"] == "react-native" and item["relation"] == "direct" for item in graph))
            self.assertTrue(any(item["relation"] == "transitive" for item in graph))
            self.assertTrue(any(item["scope"] == "development" for item in graph))
        self.assertIn("qrcode-generator", {item["name"] for item in kiosk["components"]})
        self.assertNotIn("qrcode-generator", {item["name"] for item in renter["components"]})

    def test_go_module_graph_retains_checksums_and_scope(self):
        modules = {item["name"]: item for item in self.artifacts["p07-indexer"]["components"]}
        self.assertEqual(modules["github.com/jackc/pgx/v5"]["relation"], "direct")
        self.assertEqual(modules["github.com/jackc/puddle/v2"]["relation"], "transitive")
        self.assertTrue(modules["github.com/jackc/puddle/v2"]["checksum"].startswith("h1:"))

    def test_patch_ledger_keeps_every_artifact_open_until_release_evidence(self):
        ledger = inventory.patch_ledger()
        inventory.check_ledger(ledger, set(self.artifacts))
        self.assertTrue(all(item["status"] == "open" for item in ledger["entries"]))
        ledger["entries"].pop()
        with self.assertRaisesRegex(ValueError, "coverage"):
            inventory.check_ledger(ledger, set(self.artifacts))


if __name__ == "__main__":
    unittest.main()
