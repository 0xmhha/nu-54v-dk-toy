"""Check that genuine Foundry failures do not escape the test workspace."""

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import contract_forge as guard
from integration_workspace import scoped_workspace


class FoundryFailurePathsTest(unittest.TestCase):
    def test_real_failure_paths_and_explicit_overrides(self):
        if not shutil.which("forge") or not os.environ.get("FOUNDRY_SOLC"):
            self.skipTest("requires pinned Foundry and FOUNDRY_SOLC")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            project = root / "products" / guard.PRODUCTS["p08"]
            (project / "src").mkdir(parents=True)
            (project / "test").mkdir()
            (project / "remappings.txt").touch()
            (project / "foundry.toml").write_text(f"""
[profile.default]
solc_version = "{guard.RELEASE}"
optimizer = true
optimizer_runs = 200
evm_version = "prague"
auto_detect_remappings = false
""")
            (project / "test/Fail.t.sol").write_text("""
pragma solidity ^0.8.0;
contract Counter {
    uint public n;
    function advance() public { n++; }
}
contract FailTest {
    Counter target = new Counter();
    function targetContracts() public view returns (address[] memory result) {
        result = new address[](1);
        result[0] = address(target);
    }
    function testPassing() public pure {}
    function testDeterministicFails() public pure { require(false); }
    function testFuzzFails(uint n) public pure { require(n == 0); }
    function invariant_Fails() public view { require(target.n() == 0); }
}
""")
            environment = {k: v for k, v in os.environ.items() if k not in {
                "FOUNDRY_OUT", "FOUNDRY_CACHE_PATH", "FOUNDRY_CONFIG",
                *(variable for variable, _, _, _ in guard.FAILURE_ARTIFACTS),
            }}
            def expect_failures(work, env):
                for name in ("testDeterministicFails", "testFuzzFails", "invariant_Fails"):
                    with self.subTest(name=name), self.assertRaises(subprocess.CalledProcessError) as error:
                        guard.run_guarded_test("p08", ["test", "--offline", "--match-test", name], work, env)
                    self.assertEqual(error.exception.returncode, 1)
            with patch.object(guard, "ROOT", root):
                base_cache = project / "cache"
                def snapshot():
                    return {item.relative_to(base_cache): item.read_bytes() if item.is_file() else None
                            for item in base_cache.rglob("*")}
                for preexisting in (False, True):
                    with self.subTest(preexisting=preexisting):
                        if preexisting:
                            for _, _, default, _ in guard.FAILURE_ARTIFACTS:
                                file = project / default
                                if file.name != "test-failures":
                                    file = file / "sentinel"
                                file.parent.mkdir(parents=True, exist_ok=True)
                                file.write_text("old")
                        before = snapshot()
                        with scoped_workspace("nu54-forge-fail-test-") as work:
                            env = {**environment, "FOUNDRY_OUT": str(work/"out"), "FOUNDRY_CACHE_PATH": str(work/"compile")}
                            expect_failures(work, env)
                            for path in (work/"test-failures", work/"fuzz/failures", work/"invariant/failures"):
                                self.assertTrue(path.exists(), str(path))
                            guard.run_guarded_test("p08", ["test", "--offline", "--match-test", "testPassing"], work, env)
                        self.assertFalse(work.exists())
                        after = snapshot()
                        self.assertEqual(before, after)
                        self.assertEqual(base_cache.exists(), preexisting)
                explicit = project / "caller"
                with scoped_workspace("nu54-forge-fail-test-") as work:
                    env = dict(environment, FOUNDRY_OUT=str(work/"out"), FOUNDRY_CACHE_PATH=str(work/"compile"))
                    env.update({variable: str(explicit / basename) for variable, _, _, basename in guard.FAILURE_ARTIFACTS})
                    expect_failures(work, env)
                self.assertTrue((explicit / "test-failures").is_file())
                self.assertTrue((explicit / "fuzz/failures").is_dir())
                self.assertTrue((explicit / "invariant/failures").is_dir())
