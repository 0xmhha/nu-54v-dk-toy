"""Exercise the public market runner with a local build and emulator fixture."""

import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import time
import unittest

FAKE_ADB = '''#!/usr/bin/env python3
import hashlib
import os
from pathlib import Path
import sys
import time

assert sys.argv[1:3] == ["-s", "emulator-5554"]
assert os.environ["ANDROID_USER_HOME"] == os.environ["CALLER_ANDROID_USER_HOME"]
root = Path(os.environ["DEVICE_STATE"])
package_file = root / "package"
apk_file = root / "base.apk"
tunnel_file = root / "reverse"
args = sys.argv[3:]
mode = os.environ["BUILD_MODE"]
if args == ["shell", "getprop", "ro.kernel.qemu"]:
    print("1")
elif args == ["reverse", "--list"]:
    if tunnel_file.exists():
        print("host tcp:9443 " + tunnel_file.read_text())
    print("host tcp:9000 tcp:9001")
elif args[:4] == ["shell", "pm", "list", "packages"]:
    if mode == "collision":
        print("package:" + args[4])
    elif package_file.exists():
        print("package:" + package_file.read_text())
elif args[0] == "install":
    assert args[1:4] == ["--streaming", "-R", "--pkg"]
    assert not package_file.exists()
    package_file.write_text(args[4])
    apk_file.write_bytes(Path(args[5]).read_bytes())
    if mode == "install-block":
        (root / "install-started").touch()
        time.sleep(0.6)
    print("Performing Streamed Install")
    print("Success")
elif args == ["reverse", "--no-rebind", "tcp:9443", "tcp:45123"]:
    assert not tunnel_file.exists()
    if mode == "foreign-reverse":
        tunnel_file.write_text("tcp:1024")
        sys.exit(1)
    tunnel_file.write_text("tcp:45123")
elif args == ["reverse", "--remove", "tcp:9443"]:
    assert tunnel_file.read_text() == "tcp:45123"
    tunnel_file.unlink()
elif args[:3] == ["shell", "pm", "path"]:
    assert args[3] == package_file.read_text()
    print("package:/data/app/base.apk")
elif args == ["shell", "sha256sum", "/data/app/base.apk"]:
    print(hashlib.sha256(apk_file.read_bytes()).hexdigest() + "  /data/app/base.apk")
elif args[0] == "uninstall":
    assert args[1:] == [package_file.read_text()]
    apk_file.unlink()
    package_file.unlink()
    print("Success")
else:
    raise RuntimeError(args)
'''

SCRIPTS = Path(__file__).resolve().parents[2]


class MarketRunnerTest(unittest.TestCase):
    def setUp(self):
        storage = tempfile.TemporaryDirectory()
        self.addCleanup(storage.cleanup)
        self.root = Path(storage.name).resolve()
        self.tmp = self.root / "tmp"
        self.tmp.mkdir()
        self.output = self.root / "evidence"
        self.original = self.root / "products/p02-user-app/android"
        self.runner = self.root / "scripts/android/market/run-orders.sh"
        self.runner.parent.mkdir(parents=True)
        for name in ("run-orders.sh", "run_orders.py"):
            source = Path(__file__).with_name(name)
            if source.exists():
                shutil.copy2(source, self.runner.with_name(name))
        for name in ("integration_workspace.py", "android/test_release_workspace.py"):
            shutil.copy2(SCRIPTS / name, self.root / "scripts" / name)
        for folder in ("node_modules", "packages", "docs/content/specifications",
                       "products/p01-device-firmware"):
            (self.root / folder).mkdir(parents=True)
        self.file("products/p02-user-app/android/app/debug.keystore")
        self.file("products/p02-user-app/android/gradle/wrapper/gradle-wrapper.properties",
                  "distributionUrl=https://services.gradle.org/distributions/gradle-fixture-bin.zip\n")
        self.file("bin/go", "#!/bin/sh\nprintf '%s\\n' '{\"GOENV\":\"off\",\"GOCACHEPROG\":\"\"}'\n", True)
        self.device_state = self.root / "device-state"
        self.device_state.mkdir()
        self.file("sdk/platform-tools/adb", FAKE_ADB, True)
        self.file("jdk/bin/keytool", '#!/bin/sh\ntouch "$NU54_P02_CUSTOM_KEYSTORE"\n', True)
        self.file("products/p08-market-services/test/mobile-fixture.sh", '''#!/bin/bash
set -eu
mkdir -p "$1"
test -d "$GOCACHE"
test "$P08_ARTIFACT" = "$FOUNDRY_OUT/Market.sol/Market.json"
mkdir -p "$FOUNDRY_OUT" "$FOUNDRY_CACHE_PATH"
touch "$GOCACHE/generated" "$FOUNDRY_CACHE_PATH/generated"
export P08_PROFILE="$1/profile.json"
echo '{}' > "$P08_PROFILE"
shift
"$@"
''')
        self.file("products/p08-market-services/test/mobile-https-fixture.sh", '''#!/bin/bash
export NU54_MOBILE_QA_PROFILE=fixture
printf "%s\n" 45123 > "$NU54_MOBILE_QA_PORT_FILE"
"$@"
''')
        self.file("scripts/android/market/orders.py", '''import os, pathlib, re, sys, time
assert sys.argv[1] == "emulator-5554"
assert re.fullmatch(r"com\\.nu54renter\\.marketqa\\.r[0-9a-f]{12}", sys.argv[2])
assert pathlib.Path(sys.argv[3]).is_file()
root = pathlib.Path(os.environ["DEVICE_STATE"])
assert (root / "base.apk").read_text().strip() == "signed APK"
assert (root / "package").read_text() == sys.argv[2]
assert (root / "reverse").read_text() == "tcp:45123"
if os.environ["BUILD_MODE"] == "driver-fail":
    sys.exit(23)
if os.environ["BUILD_MODE"] == "driver-block":
    (root / "driver-started").touch()
    time.sleep(60)
if os.environ["BUILD_MODE"] == "replaced-reverse":
    (root / "reverse").write_text("tcp:9443")
if os.environ["BUILD_MODE"] == "replaced-apk":
    (root / "base.apk").write_text("different APK")
assert os.environ["ANDROID_USER_HOME"] == os.environ["CALLER_ANDROID_USER_HOME"]
assert not os.environ.get("NU54_P02_CUSTOM_KEY_PASSWORD")
print("orders passed")
''')
        self.file("products/p02-user-app/android/gradlew", '''#!/bin/bash
set -eu
if [[ "${2:-}" == --stop ]]; then
    test -d "$GRADLE_USER_HOME/daemon"
    touch "$DEVICE_STATE/gradle-stopped"
    exit 0
fi
mkdir -p "$GRADLE_USER_HOME/daemon"
mkdir -p .gradle ../node_modules/dep/android/build
touch .gradle/generated ../node_modules/dep/android/build/generated
printf '%s\\n' "$PWD" "$GRADLE_USER_HOME" "$NU54_P02_CUSTOM_KEYSTORE" >"$MARKER"
test -f "$NU54_P02_CUSTOM_KEYSTORE"
test "$ANDROID_USER_HOME" != "$CALLER_ANDROID_USER_HOME"
test "$1" = --offline
test "$2" = --no-daemon
test "${NU54_MARKET_FIXTURE_SUFFIX#.marketqa.r}" != "$NU54_MARKET_FIXTURE_SUFFIX"
printf '%s\\n' "$@" | grep -Fx -- '-Pnu54MarketProfile=fixture'
case "$BUILD_MODE" in
   block) exec sleep 60 ;;
   fail) exit 17 ;;
esac
mkdir -p app/build/outputs/apk/custom
echo signed APK > app/build/outputs/apk/custom/app-custom.apk
''', True)

    def file(self, relative, contents="", executable=False):
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(contents)
        if executable:
            path.chmod(0o755)

    def run_fixture(self, mode, signum=None, ready=None):
        marker = self.root / "build-paths"
        device_home = self.root / "device-home"
        env = {**os.environ, "TMPDIR": str(self.tmp), "ANDROID_HOME": str(self.root / "sdk"),
               "JAVA_HOME": str(self.root / "jdk"), "GRADLE_USER_HOME": str(self.root / "caller-gradle"),
               "ANDROID_USER_HOME": str(device_home), "CALLER_ANDROID_USER_HOME": str(device_home),
               "MARKER": str(marker), "BUILD_MODE": mode, "DEVICE_STATE": str(self.device_state), "PATH": f"{self.root / 'bin'}:{os.environ['PATH']}"}
        with subprocess.Popen(["bash", self.runner, "emulator-5554", self.output],
                              env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                              start_new_session=True, text=True) as child:
            try:
                if signum:
                    checkpoint = (self.device_state / ready) if ready else marker
                    for _ in range(1000):
                        if checkpoint.is_file() or child.poll() is not None:
                            break
                        time.sleep(0.01)
                    self.assertTrue(checkpoint.is_file())
                    child.send_signal(signum)
                stdout, stderr = child.communicate(timeout=15)
            finally:
                if child.poll() is None:
                    os.killpg(child.pid, signal.SIGKILL)
                    child.wait()
        self.assertTrue(marker.is_file(), stderr)
        for path in marker.read_text().splitlines():
            self.assertTrue(Path(path).is_relative_to(self.tmp), path)
            self.assertFalse(Path(path).exists(), f"{path}\n{stderr}")
        self.assertEqual(list(self.tmp.iterdir()), [])
        self.assertTrue((self.device_state / "gradle-stopped").is_file())
        self.assertFalse((self.original / ".gradle").exists())
        self.assertFalse((self.original.parent / "node_modules").exists())
        self.assertTrue((self.output / "chain/profile.json").is_file())
        self.assertEqual((self.device_state / "package").exists(), mode == "replaced-apk")
        self.assertEqual((self.device_state / "base.apk").exists(), mode == "replaced-apk")
        self.assertEqual((self.device_state / "reverse").exists(), mode in ("foreign-reverse", "replaced-reverse"))
        return child.returncode, stdout, stderr

    def test_success_preserves_order_evidence(self):
        code, stdout, stderr = self.run_fixture("success")
        self.assertEqual(code, 0, stderr)
        self.assertEqual(stdout, "orders passed\n")
        self.assertEqual((self.output / "result.log").read_text(), stdout)

    def test_failed_build_removes_workspace(self):
        code, _, stderr = self.run_fixture("fail")
        self.assertEqual(code, 1)
        self.assertIn("Android custom release build failed", stderr)

    def test_term_removes_workspace(self):
        self.assertEqual(self.run_fixture("block", signal.SIGTERM)[0], 143)

    def test_hup_removes_workspace(self):
        self.assertEqual(self.run_fixture("block", signal.SIGHUP)[0], 129)


    def test_failed_orders_remove_owned_installation(self):
        self.assertEqual(self.run_fixture("driver-fail")[0], 23)

    def test_signals_remove_owned_installation(self):
        for name, signum in (("TERM", signal.SIGTERM), ("HUP", signal.SIGHUP)):
            with self.subTest(name=name):
                self.output = self.root / name
                self.assertEqual(self.run_fixture("driver-block", signum, "driver-started")[0], 128 + signum)
                (self.device_state / "driver-started").unlink()

    def test_term_during_installation(self):
        self.assertEqual(self.run_fixture("install-block", signal.SIGTERM, "install-started")[0], 143)

    def test_existing_package_prevents_installation(self):
        code, _, stderr = self.run_fixture("collision")
        self.assertEqual(code, 1)
        self.assertIn("Test package already exists", stderr)

    def test_tunnel_claim_race_preserves_the_other_mapping(self):
        code, _, stderr = self.run_fixture("foreign-reverse")
        self.assertEqual(code, 1)
        self.assertIn("Could not confirm ownership", stderr)
        self.assertEqual((self.device_state / "reverse").read_text(), "tcp:1024")
        self.assertTrue((self.output / "device-cleanup.log").is_file())

    def test_recreated_conventional_tunnel_is_preserved(self):
        code, _, stderr = self.run_fixture("replaced-reverse")
        self.assertEqual(code, 1)
        self.assertIn("changed ownership", stderr)
        self.assertEqual((self.device_state / "reverse").read_text(), "tcp:9443")
        self.assertTrue((self.output / "device-cleanup.log").is_file())

    def test_different_installed_apk_is_preserved_and_reported(self):
        code, _, stderr = self.run_fixture("replaced-apk")
        self.assertEqual(code, 1)
        self.assertIn("does not match this run", stderr)
        self.assertEqual((self.device_state / "base.apk").read_text(), "different APK")
        self.assertTrue((self.output / "device-cleanup.log").is_file())


if __name__ == "__main__":
    unittest.main()
