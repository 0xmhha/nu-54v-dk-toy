#!/usr/bin/env python3
"""The user app simulation refuses an app that was not built from the current sources.

Runs on a small repository with a stand-in Gradle wrapper and a stand-in adb; no emulator, adb or Android build is used.
Run it with `python3 -B scripts/test_simulate_user_app_stale_build.py`.
"""

from contextlib import redirect_stderr, redirect_stdout
import hashlib
import io
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
import app_build_record as build_record
import simulate_user_app as simulation

ROOT = Path(__file__).resolve().parents[1]
APP = "products/p02-user-app"
MANIFEST = f"{APP}/android/app/src/main/AndroidManifest.xml"
KOTLIN = f"{APP}/android/app/src/main/java/com/nu54renter/ble/SubscriptionAlertsModule.kt"
SPEC = f"{APP}/src/specs/NativeSubscriptionAlerts.ts"
SHARED = "packages/protocol/platform/android/nu54-mobile.gradle"
CONSTRAINTS = "scripts/android/runtime-constraints.gradle"
LOCK = "pnpm-lock.yaml"
INSTALLED_LOCK = "node_modules/.pnpm/lock.yaml"
REMEDY = "rebuild with --build, or pass --allow-stale-app"

# Packs the native sources into the "APK", so a build from other sources is another file.
GRADLEW = f'''#!{sys.executable}
from pathlib import Path
import sys
assert sys.argv[1:] == [":app:assembleDebug"], sys.argv
here = Path(__file__).resolve().parent
if (here / "fail-the-build").exists():
    sys.exit(1)
out = here / "app/build/outputs/apk/debug"
out.mkdir(parents=True, exist_ok=True)
(here / "app/build/records-seen").write_text("".join(path.name + "\\n" for path in out.glob("*.json")))
if (here / "fail-after-packaging").exists():
    (out / "app-debug.apk").write_bytes(b"half a build")
    sys.exit(1)
(out / "app-debug.apk").write_bytes(b"apk\\n" + b"".join(path.read_bytes() for path in sorted((here / "app/src").rglob("*")) if path.is_file()))
if (here / "edit-while-building").exists():
    (here / "app/src/main/AndroidManifest.xml").write_text("<manifest edited='while building'/>\\n")
'''

ADB = f'''#!{sys.executable}
import hashlib
import os
from pathlib import Path
import shutil
import sys
assert sys.argv[1:3] == ["-s", "emulator-5554"], sys.argv
state, args = Path(os.environ["DEVICE_STATE"]), sys.argv[3:]
with (state / "calls").open("a") as calls:
    calls.write(" ".join(args) + "\\n")
installed, path = state / "base.apk", "/data/app/~~kq/com.nu54renter-1/base.apk"
if args == ["shell", "getprop", "sys.boot_completed"]:
    print("1")
elif args == ["shell", "pm", "path", "com.nu54renter"]:
    if not installed.exists():
        sys.exit(1)
    print("package:" + path)
elif args == ["shell", "sha256sum", path]:
    if (state / "no-sha256sum").exists():
        sys.exit("/system/bin/sh: sha256sum: inaccessible or not found")
    print(hashlib.sha256(installed.read_bytes()).hexdigest() + "  " + path)
elif args == ["exec-out", "cat", path]:
    sys.stdout.buffer.write(installed.read_bytes())
elif args[:2] == ["install", "-r"]:
    shutil.copyfile(args[2], installed)
    print("Success")
elif args[:4] == ["shell", "pm", "grant", "com.nu54renter"]:
    if (state / "undeclared").exists() and args[4] in (state / "undeclared").read_text().split():
        sys.exit("java.lang.SecurityException: Package com.nu54renter has not requested permission " + args[4])
'''


class Repository(unittest.TestCase):
    def setUp(self):
        storage = tempfile.TemporaryDirectory()
        self.addCleanup(storage.cleanup)
        self.root = Path(storage.name).resolve() / "repo"
        self.apk = self.root / simulation.APK
        for name, text in {
                MANIFEST: "<manifest><uses-permission android:name='android.permission.POST_NOTIFICATIONS'/></manifest>\n",
                KOTLIN: "class SubscriptionAlertsModule\n", SPEC: "export interface Spec {}\n", SHARED: "// shared\n", CONSTRAINTS: "// constraints\n",
                LOCK: "lockfileVersion: '9.0'\n", ".gitignore": "node_modules/\nbuild/\nout/\ncache/\ndist/\nbin/\n.gradle/\nlocal.properties\n.DS_Store\n",
                f"{APP}/.gitignore": "build/\n.cxx/\n.kotlin/\n.idea\n*.iml\n", f"{APP}/README.md": "# App\n", f"{APP}/src/wallet/Home.tsx": "export {};\n",
                f"{APP}/test/home.test.tsx": "test('home', () => {});\n", f"{APP}/ios/NU54Renter/AppDelegate.swift": "// ios\n",
                f"{APP}/android/app/src/androidTest/java/com/nu54renter/ble/OwnerTlsLab.kt": "class OwnerTlsLab\n",
                "docs/content/commercial/traveller-services.md": "# Services\n", "scripts/android/market/run-orders.sh": "true\n"}.items():
            self.write(name, text)
        self.write(f"{APP}/android/gradlew", GRADLEW)
        (self.root / APP / "android/gradlew").chmod(0o755)
        self.install()
        # Not the developer's git settings: a hook or signing set up there has no place in this repository.
        env = {**os.environ, "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_NOSYSTEM": "1"}
        for command in (["init", "-q"], ["add", "-A"], ["-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "-qm", "sources"]):
            subprocess.run(["git", "-C", str(self.root), *command], check=True, env=env, capture_output=True)

    def write(self, name, text):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)

    def install(self):
        """What `pnpm install` leaves behind: its copy of the lockfile that node_modules was made from."""
        self.write(INSTALLED_LOCK, (self.root / LOCK).read_text())

    def build(self):
        build_record.build(self.apk, self.root, build_record.USER_APP, ["./gradlew", ":app:assembleDebug"], self.root / APP / "android")

    def stale(self, installed=None):
        return build_record.stale(self.apk, self.root, build_record.USER_APP, installed)


class BuildRecord(Repository):
    def test_a_build_from_the_current_sources_is_current(self):
        self.build()
        self.assertIsNone(self.stale())
        self.assertIsNone(self.stale(lambda: build_record.file_digest(self.apk)))

    def test_a_changed_native_source_makes_it_stale_and_is_named(self):
        self.build()
        self.write(KOTLIN, "class SubscriptionAlertsModule { fun ready() {} }\n")
        reason = self.stale()
        self.assertIn("older than its sources: 1 changed", reason)
        self.assertIn(KOTLIN, reason)

    def test_every_kind_of_build_input_counts(self):
        new = f"{APP}/android/app/src/main/java/com/nu54renter/ble/NewModule.kt"
        for name in (MANIFEST, SPEC, SHARED, CONSTRAINTS, LOCK, f"{APP}/android/gradlew", new, f"{APP}/native/market/wallet.c"):
            with self.subTest(name):
                self.install()
                self.build()
                self.write(name, "changed\n" if name != f"{APP}/android/gradlew" else GRADLEW + "# changed\n")
                self.assertIn(name, self.stale())

    def test_a_deleted_input_and_an_edit_put_back_are_told_apart(self):
        self.build()
        (self.root / KOTLIN).unlink()
        self.assertIn(KOTLIN, self.stale())
        self.write(KOTLIN, "class SubscriptionAlertsModule\n")
        self.assertIsNone(self.stale())

    def test_only_the_first_changed_files_are_listed(self):
        self.build()
        for number in range(8):
            self.write(f"{APP}/native/market/file{number}.c", "int x;\n")
        self.assertIn("8 changed since it was built", reason := self.stale())
        self.assertIn("and 3 more", reason)

    def test_an_apk_without_a_record_is_stale(self):
        self.build()
        build_record.record_path(self.apk).unlink()
        self.assertIn("nothing records which sources", reason := self.stale())
        self.assertIn("app-debug.apk.inputs.json is missing", reason)

    def test_a_missing_apk_and_a_broken_record_are_stale(self):
        self.assertIn("there is no debug app", self.stale())
        self.build()
        for text in ("{", "[]", '{"version": 2, "apk": "", "files": {}}', '{"version": 1, "apk": ""}'):
            build_record.record_path(self.apk).write_text(text)
            self.assertIn("cannot be read", self.stale(), text)

    def test_an_apk_built_again_outside_the_script_is_stale(self):
        self.build()
        self.apk.write_bytes(b"built by ./gradlew :app:installDebug")
        self.assertIn("built again without its record", self.stale())

    def test_changes_outside_the_build_keep_it_current(self):
        self.build()
        for name in ("docs/content/commercial/traveller-services.md", "docs/new.md", f"{APP}/README.md", f"{APP}/android/NOTES.md", f"{APP}/src/wallet/Home.tsx",
                     f"{APP}/src/travel/New.tsx", f"{APP}/test/home.test.tsx", f"{APP}/ios/NU54Renter/AppDelegate.swift",
                     f"{APP}/android/app/src/androidTest/java/com/nu54renter/ble/OwnerTlsLab.kt", f"{APP}/android/app/build/intermediates/x.bin",
                     f"{APP}/Makefile", f"{APP}/metro.config.js", f"{APP}/index.js", f"{APP}/build-tools/public-travel-origin.js",
                     "scripts/android/market/run-orders.sh", "scripts/simulate_user_app.py", "products/p04-merchant-kiosk/android/app/build.gradle"):
            self.write(name, "changed\n")
            self.assertIsNone(self.stale(), name)

    def test_a_new_file_of_the_app_that_is_not_named_as_left_out_counts(self):
        self.build()
        self.write(f"{APP}/react-native.config.js", "module.exports = {};\n")
        self.assertIn(f"{APP}/react-native.config.js", self.stale())

    def test_a_new_source_in_a_directory_named_like_build_output_counts(self):
        """The ignore rules of the repository name build, out, cache, dist and bin wherever they lie."""
        for directory in ("build", "out", "cache", "dist", "bin"):
            with self.subTest(directory):
                self.build()
                new = f"{APP}/android/app/src/main/java/com/nu54renter/{directory}/Entry.kt"
                self.write(new, "class Entry\n")
                self.assertIn(new, self.stale())
        self.build()
        self.write("packages/protocol/platform/android/com/nu54/protocol/cache/Frames.kt", "class Frames\n")
        self.assertIn("packages/protocol/platform/android/com/nu54/protocol/cache/Frames.kt", self.stale())

    def test_what_a_build_or_an_editor_writes_is_no_input(self):
        self.build()
        for name in (f"{APP}/android/app/build/generated/source/codegen/Spec.java", f"{APP}/android/build/reports/problems.html", f"{APP}/android/.gradle/8.14/fileHashes.bin",
                     f"{APP}/android/.kotlin/sessions/kotlin-compiler.salive", f"{APP}/android/app/.cxx/Debug/x86_64/build.ninja", f"{APP}/node_modules/react-native/index.js",
                     "packages/protocol/platform/node_modules/typescript/index.js", f"{APP}/android/local.properties", f"{APP}/android/.idea/workspace.xml",
                     f"{APP}/android/app/app.iml", f"{APP}/android/app/src/main/.DS_Store", f"{APP}/android/java_pid1.hprof"):
            self.write(name, "written\n")
            self.assertIsNone(self.stale(), name)

    def link(self, name, target):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.unlink(missing_ok=True)
        path.symlink_to(target)

    def test_a_link_counts_by_where_it_leads_and_by_what_lies_there(self):
        """The header lies outside the files the build is known to read, so only the link tells of it."""
        link, up = f"{APP}/native/market/frames.h", "../../../../"
        self.write("packages/protocol/c/frames.h", "#define FRAME 64\n")
        self.write("packages/protocol/c/frames-copy.h", "#define FRAME 64\n")
        self.link(link, up + "packages/protocol/c/frames.h")
        self.build()
        self.assertIsNone(self.stale())
        self.write("packages/protocol/c/frames.h", "#define FRAME 128\n")
        self.assertIn(link, self.stale())
        self.write("packages/protocol/c/frames.h", "#define FRAME 64\n")
        self.assertIsNone(self.stale())
        self.link(link, up + "packages/protocol/c/frames-copy.h")  # the same content elsewhere
        self.assertIn(link, self.stale())
        self.link(link, up + "packages/protocol/c/frames.h")
        (self.root / "packages/protocol/c/frames.h").unlink()
        self.assertIn(link, self.stale())

    def test_a_link_to_a_directory_counts_by_every_file_below_it(self):
        link = f"{APP}/native/protocol"
        self.write("packages/protocol/c/include/frames.h", "#define FRAME 64\n")
        self.link(f"{APP}/native/loop", "loop")
        self.link("packages/protocol/c/include/self", ".")
        self.link(link, "../../../packages/protocol/c")
        self.build()
        self.assertIsNone(self.stale())
        self.write("packages/protocol/c/include/frames.h", "#define FRAME 128\n")
        self.assertIn(link, self.stale())
        self.write("packages/protocol/c/include/frames.h", "#define FRAME 64\n")
        self.assertIsNone(self.stale())
        self.write("packages/protocol/c/include/new.h", "")
        self.assertIn(link, self.stale())

    def unreadable(self, name):
        """Reading this file fails as it does without the right to read it; a changed mode would not stop a test run as root."""
        digest = build_record.file_digest
        def refuse(path):
            if Path(path) == self.root / name:
                raise PermissionError(13, "Permission denied", str(path))
            return digest(path)
        return mock.patch.object(build_record, "file_digest", refuse)

    def test_a_file_that_cannot_be_read_is_a_reason_and_not_a_traceback(self):
        self.build()
        with self.unreadable(KOTLIN):
            self.assertIn(KOTLIN, reason := self.stale())
            self.assertIn("cannot be read", reason)
            self.assertIn("Permission denied", reason)
            with self.assertRaises(SystemExit) as stopped:
                self.build()
            self.assertIn(KOTLIN, str(stopped.exception))
            self.assertIn("cannot be read", str(stopped.exception))
        with self.unreadable(simulation.APK):
            self.assertIn("app-debug.apk cannot be read", self.stale())
            with self.assertRaises(SystemExit) as stopped:
                self.build()
            self.assertIn("the build cannot be recorded", str(stopped.exception))
            self.assertIn("app-debug.apk", str(stopped.exception))

    def test_modification_times_play_no_part(self):
        self.build()
        os.utime(self.root / KOTLIN, (2_000_000_000, 2_000_000_000))  # newer than the APK
        self.assertIsNone(self.stale())
        self.write(KOTLIN, "class Changed\n")
        os.utime(self.root / KOTLIN, (1_000_000_000, 1_000_000_000))  # older than the APK
        self.assertIn(KOTLIN, self.stale())

    def test_the_installed_app_must_be_the_build(self):
        self.build()
        self.assertIn("is not the build at", self.stale(lambda: "0" * 64))
        self.assertIn("cannot be read", self.stale(lambda: None))

    def test_the_emulator_is_not_asked_about_a_stale_build(self):
        self.build()
        self.write(MANIFEST, "<manifest/>\n")
        self.assertIn(MANIFEST, self.stale(lambda: self.fail("asked the emulator")))

    def test_sources_that_cannot_be_listed_are_stale(self):
        self.build()
        outside = self.root.parent / "copy"
        (self.root / ".git").rename(self.root.parent / "elsewhere.git")
        self.root.rename(outside)
        with mock.patch.dict(os.environ, {"GIT_CEILING_DIRECTORIES": str(outside.parent)}):
            self.assertIn("git cannot list the app's sources", build_record.stale(outside / simulation.APK, outside, build_record.USER_APP))

    def test_a_failed_first_build_leaves_no_record(self):
        self.write(f"{APP}/android/fail-the-build", "")
        with self.assertRaises(SystemExit) as stopped:
            self.build()
        self.assertIn("did not build", str(stopped.exception))
        self.assertFalse(build_record.record_path(self.apk).exists())

    def test_a_failed_build_keeps_the_record_of_the_app_it_left_untouched(self):
        self.build()
        record = build_record.record_path(self.apk).read_bytes()
        self.write(f"{APP}/android/fail-the-build", "")
        with self.assertRaises(SystemExit) as stopped:
            self.build()
        self.assertIn("did not build", str(stopped.exception))
        self.assertEqual(build_record.record_path(self.apk).read_bytes(), record)
        (self.root / APP / "android/fail-the-build").unlink()
        self.assertIsNone(self.stale())

    def test_a_build_that_fails_after_it_replaced_the_app_is_stale(self):
        self.build()
        self.write(f"{APP}/android/fail-after-packaging", "")
        with self.assertRaises(SystemExit):
            self.build()
        (self.root / APP / "android/fail-after-packaging").unlink()
        self.assertIn("built again without its record", self.stale())

    def test_the_build_does_not_find_the_last_record_among_its_outputs(self):
        self.build()
        self.build()
        self.assertEqual((self.root / APP / "android/app/build/records-seen").read_text(), "")
        self.assertIsNone(self.stale())

    def test_sources_edited_during_the_build_leave_no_record(self):
        self.write(f"{APP}/android/edit-while-building", "")
        with self.assertRaises(SystemExit) as stopped:
            self.build()
        self.assertIn("changed while it was building", str(stopped.exception))
        self.assertIn("nothing records which sources", self.stale())

    def test_sources_edited_during_a_later_build_are_seen_against_the_record_kept(self):
        self.build()
        self.write(f"{APP}/android/edit-while-building", "")
        with self.assertRaises(SystemExit) as stopped:
            self.build()
        self.assertIn("changed while it was building", str(stopped.exception))
        self.assertIn(MANIFEST, self.stale())

    def test_node_modules_that_the_lockfile_does_not_name_refuse_the_build(self):
        self.write(LOCK, "lockfileVersion: '9.0'\n# pulled: another native dependency\n")
        for missing in (False, True):
            if missing:
                (self.root / INSTALLED_LOCK).unlink()
            with self.assertRaises(SystemExit) as stopped:
                self.build()
            self.assertIn("pnpm install", str(stopped.exception))
            self.assertIn(INSTALLED_LOCK, str(stopped.exception))
            self.assertFalse(self.apk.exists(), "the build ran")
        self.install()
        self.build()
        self.assertIsNone(self.stale())

    def test_node_modules_installed_from_another_lockfile_since_the_build_are_stale(self):
        self.build()
        self.write(INSTALLED_LOCK, "lockfileVersion: '9.0'\n# installed on another branch\n")
        self.assertIn("pnpm install", reason := self.stale())
        self.assertIn(INSTALLED_LOCK, reason)
        (self.root / INSTALLED_LOCK).unlink()
        self.assertIn("pnpm install", self.stale())
        self.install()
        self.assertIsNone(self.stale())

    def test_the_override_lets_a_stale_app_through_with_a_warning(self):
        out = io.StringIO()
        self.assertEqual(build_record.admit("the debug app is older than its sources", True, REMEDY, out), "the debug app is older than its sources")
        self.assertIn("STALE APP ALLOWED: the debug app is older than its sources", out.getvalue())
        self.assertIn("does not show that the current sources work", out.getvalue())

    def test_without_the_override_a_stale_app_stops_the_run_and_says_what_to_do(self):
        with self.assertRaises(SystemExit) as stopped:
            build_record.admit("the debug app is older than its sources", False, REMEDY)
        self.assertEqual(str(stopped.exception), "the debug app is older than its sources\n" + REMEDY)

    def test_a_current_app_is_admitted_silently(self):
        out = io.StringIO()
        self.assertEqual(build_record.admit(None, False, REMEDY, out), "")
        self.assertEqual(out.getvalue(), "")


class InstalledApp(unittest.TestCase):
    def adb(self, answers):
        def run(*args):
            code, output = answers[args]
            return subprocess.CompletedProcess(args, code, output, b"")
        return run

    def test_digest_from_the_emulator(self):
        path, digest = "/data/app/~~kq/com.nu54renter-1/base.apk", hashlib.sha256(b"apk").hexdigest()
        asked = ("shell", "pm", "path", "com.nu54renter")
        self.assertEqual(build_record.installed_digest(self.adb({asked: (0, f"package:{path}\n".encode()), ("shell", "sha256sum", path): (0, f"{digest}  {path}\n".encode())}),
                                                       "com.nu54renter"), digest)
        self.assertEqual(build_record.installed_digest(self.adb({asked: (0, f"package:{path}\r\n".encode()), ("shell", "sha256sum", path): (127, b"not found"),
                                                                 ("exec-out", "cat", path): (0, b"apk")}), "com.nu54renter"), digest)
        self.assertIsNone(build_record.installed_digest(self.adb({asked: (0, f"package:{path}\n".encode()), ("shell", "sha256sum", path): (1, b""),
                                                                  ("exec-out", "cat", path): (1, b"")}), "com.nu54renter"))
        self.assertIsNone(build_record.installed_digest(self.adb({asked: (0, b"package:/a/base.apk\npackage:/a/split_config.x86_64.apk\n")}), "com.nu54renter"))
        self.assertIsNone(build_record.installed_digest(self.adb({asked: (1, b"")}), "com.nu54renter"))

    def test_only_a_missing_declaration_counts_as_an_old_app(self):
        def granted(code, error):
            return subprocess.CompletedProcess([], code, b"", error)
        self.assertTrue(build_record.undeclared(granted(255, b"java.lang.SecurityException: Package com.nu54renter has not requested permission android.permission.POST_NOTIFICATIONS")))
        self.assertFalse(build_record.undeclared(granted(255, b"java.lang.IllegalArgumentException: Unknown permission: android.permission.POST_NOTIFICATIONS")))
        self.assertFalse(build_record.undeclared(granted(0, b"")))


class RealRepository(unittest.TestCase):
    def test_the_rules_name_the_real_build_inputs(self):
        files = set(build_record.input_files(ROOT, build_record.USER_APP))
        for name in (MANIFEST, f"{APP}/android/app/build.gradle", f"{APP}/android/gradle.properties", f"{APP}/native/market/CMakeLists.txt", f"{APP}/package.json",
                     SHARED, CONSTRAINTS, LOCK, ".npmrc"):
            self.assertIn(name, files)
        self.assertTrue(any(name.startswith(f"{APP}/src/specs/Native") for name in files))
        self.assertTrue(any(name.startswith("packages/protocol/platform/android/com/nu54/protocol/") for name in files))
        for name in files:
            self.assertFalse(name.startswith(("docs/", f"{APP}/test/", f"{APP}/ios/", f"{APP}/src/wallet/", f"{APP}/Makefile", f"{APP}/metro.config.js",
                                              "scripts/test_", "scripts/simulate_")) or name.endswith(".md"), name)
            self.assertNotIn("/build/", name)

    def test_every_named_path_exists(self):
        """A renamed directory must not silently drop out of the rules."""
        for name in (*build_record.USER_APP.take, *build_record.USER_APP.leave, *build_record.USER_APP.keep):
            self.assertTrue((ROOT / name).exists(), name)


class Emulator(Repository):
    """simulate_user_app.emulator() against the stand-in adb."""

    def setUp(self):
        super().setUp()
        self.device = self.root.parent / "device"
        tools = self.root.parent / "tools"
        self.device.mkdir()
        tools.mkdir()
        (tools / "adb").write_text(ADB)
        (tools / "adb").chmod(0o755)
        patched = mock.patch.dict(os.environ, {"PATH": f"{tools}{os.pathsep}{os.environ['PATH']}", "DEVICE_STATE": str(self.device)})
        patched.start()
        self.addCleanup(patched.stop)

    def emulator(self, **options):
        """What emulator() returned or the message it stopped with, and what it printed."""
        (self.device / "calls").write_text("")
        out, err = io.StringIO(), io.StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            try:
                result = simulation.emulator("emulator-5554", root=self.root, **options)[1]
            except SystemExit as stopped:
                result = stopped
        return result, out.getvalue() + err.getvalue()

    def calls(self):
        return (self.device / "calls").read_text().splitlines()

    def assertStopped(self, result, word):
        self.assertIsInstance(result, SystemExit)
        self.assertIn(word, str(result))
        self.assertIn("--build", str(result))
        self.assertIn("--allow-stale-app", str(result))
        # The app was not touched.
        self.assertFalse([call for call in self.calls() if "force-stop" in call or "run-as" in call or call.startswith("reverse")], self.calls())

    def test_build_installs_records_and_runs(self):
        result, printed = self.emulator(build=True)
        self.assertEqual(result, "", printed)
        self.assertEqual((self.device / "base.apk").read_bytes(), self.apk.read_bytes())
        self.assertIn("shell pm grant com.nu54renter android.permission.POST_NOTIFICATIONS", self.calls())
        result, printed = self.emulator()
        self.assertEqual(result, "", printed)
        self.assertNotIn("STALE", printed)
        self.assertFalse([call for call in self.calls() if call.startswith("install")])

    def test_an_image_without_sha256sum_is_read_whole(self):
        (self.device / "no-sha256sum").write_text("")
        self.assertEqual(self.emulator(build=True)[0], "")
        self.assertTrue([call for call in self.calls() if call.startswith("exec-out cat")])

    def test_an_app_installed_from_an_unrecorded_build_stops_the_run(self):
        (self.device / "base.apk").write_bytes(b"built on 5 October")
        self.assertStopped(self.emulator()[0], "there is no debug app")
        self.apk.parent.mkdir(parents=True)
        self.apk.write_bytes(b"built on 5 October")
        self.assertStopped(self.emulator()[0], "nothing records which sources")

    def test_a_source_changed_after_the_build_stops_the_run(self):
        self.emulator(build=True)
        self.write(KOTLIN, "class SubscriptionAlertsModule { fun ready() {} }\n")
        self.assertStopped(self.emulator()[0], "older than its sources")

    def test_an_emulator_with_another_build_stops_the_run(self):
        self.emulator(build=True)
        (self.device / "base.apk").write_bytes(b"built on 5 October")
        self.assertStopped(self.emulator()[0], "installed on the emulator is not the build")

    def test_a_document_changed_after_the_build_does_not_stop_the_run(self):
        self.emulator(build=True)
        self.write("docs/content/commercial/traveller-services.md", "# Services, revised\n")
        self.write(f"{APP}/src/wallet/Home.tsx", "export const revised = true;\n")
        self.assertEqual(self.emulator()[0], "")

    def test_the_override_runs_a_stale_app_and_says_so_loudly(self):
        self.emulator(build=True)
        self.write(KOTLIN, "class SubscriptionAlertsModule { fun ready() {} }\n")
        result, printed = self.emulator(allow_stale=True)
        self.assertIn("older than its sources", result)
        self.assertIn("STALE APP ALLOWED: the debug app is older than its sources", printed)
        self.assertIn("shell am force-stop com.nu54renter", self.calls())

    def test_a_permission_the_installed_app_lacks_stops_the_run(self):
        self.emulator(build=True)
        (self.device / "undeclared").write_text("android.permission.POST_NOTIFICATIONS\n")
        result, _ = self.emulator()
        self.assertIsInstance(result, SystemExit)
        self.assertIn("does not declare POST_NOTIFICATIONS", str(result))
        self.assertIn("--build", str(result))
        result, printed = self.emulator(allow_stale=True)
        self.assertIn("does not declare POST_NOTIFICATIONS", result)
        self.assertIn("STALE APP ALLOWED", printed)

    def test_a_build_that_fails_installs_nothing(self):
        self.write(f"{APP}/android/fail-the-build", "")
        result, _ = self.emulator(build=True)
        self.assertIn("did not build", str(result))
        self.assertFalse([call for call in self.calls() if call.startswith("install")])


if __name__ == "__main__":
    unittest.main()
