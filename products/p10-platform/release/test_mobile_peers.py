import copy
import unittest

from build_mobile_sdk import runtime_peers, verify_peer_inventory


class MobilePeerInventoryTest(unittest.TestCase):
    def setUp(self):
        self.package = {"name": "@nu54/protocol-mobile", "version": "0.3.0", "peerDependencies": {
            "react": "19.2.3", "react-native": "0.87.1",
        }}
        self.peers = self.package["peerDependencies"]
        root = "pkg:npm/%40nu54/protocol-mobile@0.3.0"
        self.sbom = {
            "metadata": {"component": {"bom-ref": root, "name": self.package["name"], "version": self.package["version"]}},
            "components": [
                {"name": name, "type": "library", "version": version, "bom-ref": f"pkg:npm/{name}@{version}",
                 "purl": f"pkg:npm/{name}@{version}"}
                for name, version in self.peers.items()
            ],
            "dependencies": [{"ref": root, "dependsOn": [
                f"pkg:npm/{name}@{version}" for name, version in self.peers.items()
            ]}],
        }

    def test_matching_runtime_and_sbom(self):
        self.assertEqual(runtime_peers(self.package), self.peers)
        verify_peer_inventory(self.package, self.sbom, self.peers)

    def test_missing_swapped_and_unlinked_peers(self):
        for altered in ("missing", "wrong-version", "unlinked", "wrong-runtime"):
            with self.subTest(altered=altered):
                sbom = copy.deepcopy(self.sbom)
                actual = dict(self.peers)
                if altered == "missing":
                    sbom["components"].pop()
                elif altered == "wrong-version":
                    sbom["components"][0]["version"] = "19.3.0"
                elif altered == "unlinked":
                    sbom["dependencies"][0]["dependsOn"].pop()
                else:
                    actual["react"] = "19.3.0"
                with self.assertRaises(ValueError):
                    verify_peer_inventory(self.package, sbom, actual)


if __name__ == "__main__":
    unittest.main()
