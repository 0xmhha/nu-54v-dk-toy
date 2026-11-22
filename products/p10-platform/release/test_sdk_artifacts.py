"""Protocol distribution archive acceptance tests."""

import io
import json
import unittest
import zipfile
from pathlib import Path

from build_sdk import PROTOCOL, go_proxy_files, schemas


class SDKArtifactTests(unittest.TestCase):
    def test_proxy_reproducible_and_standard_layout(self):
        module = "github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go"
        version = "v0.2.0"
        files = go_proxy_files(version)
        self.assertEqual(files, go_proxy_files(version))
        base = f"{module}/@v"
        self.assertEqual(json.loads(files[f"{base}/{version}.info"])["Version"], version)
        self.assertEqual(files[f"{base}/list"], f"{version}\n".encode())
        self.assertEqual(files[f"{base}/{version}.mod"], (PROTOCOL / "go/go.mod").read_bytes())
        with zipfile.ZipFile(io.BytesIO(files[f"{base}/{version}.zip"])) as archive:
            root = f"{module}@{version}"
            self.assertEqual(archive.read(f"{root}/go.sum"), (PROTOCOL / "go/go.sum").read_bytes())
            self.assertEqual(
                archive.read(f"{root}/spec-v2/testdata/quote.vectors"),
                schemas(root)[f"{root}/spec-v2/testdata/quote.vectors"],
            )
            self.assertFalse(any(name.endswith("_test.go") for name in archive.namelist()))


if __name__ == "__main__":
    unittest.main()
