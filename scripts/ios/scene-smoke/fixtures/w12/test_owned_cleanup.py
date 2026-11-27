"""Exercise the actual W12 launcher with local command stubs."""

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


HERE = Path(__file__).resolve()
SCENE = HERE.parents[2]
ID = "11111111-1111-1111-1111-111111111111"
OTHER = "22222222-2222-2222-2222-222222222222"


class W12OwnedCleanup(unittest.TestCase):
    def test_early_boot_failure_cleans_device_and_scratch(self):
        for fault in ("signal", "delete"):
            with self.subTest(fault=fault), tempfile.TemporaryDirectory(prefix="nu54-w12-cleanup-") as temp:
                self.run_fixture(Path(temp), fault)

    def test_permanent_delete_or_listing_failure_reports_only_owned_device(self):
        for fault in ("delete-permanent", "list-permanent"):
            with self.subTest(fault=fault), tempfile.TemporaryDirectory(prefix="nu54-w12-cleanup-") as temp:
                self.run_fixture(Path(temp), fault)

    def run_fixture(self, root, fault):
        work = root / "work"
        work.mkdir()
        (work / "caller").touch()
        scene = root / "scripts/ios/scene-smoke"
        spec = scene / "fixtures/w12"
        spec.mkdir(parents=True)
        for source, destination in (
            (HERE.with_name("run-owned.sh"), spec / "run-owned.sh"),
            (SCENE / "private-build.sh", scene / "private-build.sh"),
            (SCENE / "run-w12-installed.sh", scene / "run-w12-installed.sh"),
            (SCENE.parent / "test-environment.sh", scene.parent / "test-environment.sh"),
        ):
            shutil.copy2(source, destination)
        app = root / "products/p02-user-app"
        for rel in ("ios/NU54Renter.xcworkspace/contents.xcworkspacedata", "ios/Pods/Manifest.lock",
                    "src/wallet/RenterReturnStatus.tsx", "app.json", "babel.config.js", "index.js",
                    "metro.config.js", "package.json", "build-tools/.keep", "native/.keep"):
            path = app / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            path.touch()
        for path in (scene / "SceneSmoke.xcodeproj/project.pbxproj", scene / "SceneSmoke.swift",
                     root / "products/p07-indexer/test/generation-v1.py"):
            path.parent.mkdir(parents=True, exist_ok=True)
            path.touch()
        generated = "build/generated/ios/ReactCodegen"
        (app / "ios" / generated / "NU54RenterSpec").mkdir(parents=True)
        tools = root / "bin"
        tools.mkdir()
        other = root / OTHER
        other.touch()

        def tool(name, script="exit 0"):
            path = tools / name
            path.write_text("#!/bin/bash\nset -eu\n" + script + "\n")
            path.chmod(0o700)

        for name in ("postgres", "psql", "pg_ctl", "initdb", "createdb", "xcodebuild", "codesign", "go", "cmake"):
            tool(name)
        for name in ("forge", "anvil"):
            tool(name, f"echo '{name} Version: 1.5.1'")
        tool("solc", "echo 'Version: 0.8.39+commit.ce47f2c8.fixture'")
        tool("uuidgen", f"echo '{ID}'")
        tool("cp", '[[ $1 == -cR ]] && shift && exec /bin/cp -R "$@"; exec /bin/cp "$@"')
        tool("node", f'mkdir -p "$5/{generated}/NU54RenterSpec"; touch "$5/{generated}"/{{NU54RenterSpec/NU54RenterSpec.h,NU54RenterSpec/NU54RenterSpec-generated.mm,NU54RenterSpecJSI.h}}')
        tool("xcrun", f'''
[[ $1 == simctl ]]
case "$2" in
  create) touch "$STUB_ROOT/{ID}"; echo '{ID}';;
  boot)
    if [[ $FAULT == *-permanent ]]; then kill -HUP "$PPID"; sleep 1; fi
    exit 73;;
  shutdown)
    [[ $3 == '{ID}' ]]
    if [[ $FAULT == signal ]]; then kill -TERM "$PPID"; sleep .1; fi;;
  delete)
    [[ $3 == '{ID}' ]]
    if [[ $FAULT == delete-permanent ]]; then exit 74; fi
    if [[ $FAULT == delete && ! -f "$STUB_ROOT/once" ]]; then
      touch "$STUB_ROOT/once"; exit 74
    fi
    rm "$STUB_ROOT/{ID}";;
  list)
    if [[ $FAULT == list-permanent ]]; then exit 75; fi
    if [[ -f "$STUB_ROOT/{ID}" ]]; then echo '{ID}'; fi
    echo '{OTHER}';;
  *) exit 3;;
esac
''')
        env = {**os.environ, "PATH": f"{tools}:{os.defpath}", "TMPDIR": str(work), "STUB_ROOT": str(root),
               "FAULT": fault, "W12_PG_BIN": str(tools), "W12_SOLC": str(tools / "solc"),
               "W12_FORGE": str(tools / "forge"), "W12_ANVIL": str(tools / "anvil")}
        result = subprocess.run(["bash", spec / "run-owned.sh", "--install-slot"], env=env,
                                capture_output=True, text=True, timeout=15)
        if fault.endswith("-permanent"):
            self.assertEqual(result.returncode, 129, result.stderr)
            self.assertIn(f"could not confirm removal of owned W12 simulator {ID}", result.stderr)
            self.assertNotIn(OTHER, result.stderr)
            self.assertEqual((root / ID).exists(), fault == "delete-permanent")
        else:
            self.assertIn("W12 boot failed", result.stderr)
            self.assertFalse((root / ID).exists(), result.stderr)
            self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(list(work.iterdir()), [work / "caller"])
        self.assertTrue(other.exists())


if __name__ == "__main__":
    unittest.main()
