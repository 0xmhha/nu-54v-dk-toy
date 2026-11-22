"""CI pinning recognizes YAML syntax rather than just bare action tokens."""

import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[3] / "scripts"))

import verify_ci_inputs as inputs


class InputTests(unittest.TestCase):
    def test_quoted_unpinned_action_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            workflows = root / ".github/workflows"
            workflows.mkdir(parents=True)
            (workflows / "ci.yml").write_text("jobs: {one: {steps: [{uses: 'actions/checkout@v4'}]}}")
            with self.assertRaisesRegex(ValueError, "unpinned Action"):
                inputs.check_actions(root)


if __name__ == "__main__":
    unittest.main()
