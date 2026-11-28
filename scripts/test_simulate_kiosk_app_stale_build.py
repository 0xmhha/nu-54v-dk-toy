#!/usr/bin/env python3
"""The kiosk app simulation refuses an app that was not built from the current sources.

Runs on the small repository of the user app's test with the kiosk beside the user app, and with that test's stand-in
Gradle wrapper and stand-in adb; no emulator, adb or Android build is used. What app_build_record.py does with any
app's inputs is held there; here is what the kiosk's inputs are and how the kiosk simulation acts on them.
Run it with `python3 -B scripts/test_simulate_kiosk_app_stale_build.py`.
"""

from contextlib import nullcontext, redirect_stderr, redirect_stdout
import io
import os
from pathlib import Path
import shutil
import sys
import time
import types
import unittest
from unittest import mock

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
import app_build_record as build_record
import simulate_kiosk_app as simulation
import test_simulate_user_app_stale_build as user_app  # as a module: its test cases are not run again from here

ROOT = Path(__file__).resolve().parents[1]
KIOSK = "products/p04-merchant-kiosk"
MANIFEST = f"{KIOSK}/android/app/src/main/AndroidManifest.xml"
KOTLIN = f"{KIOSK}/android/app/src/main/java/com/nu54kiosk/ble/NearbyModule.kt"
SPEC = f"{KIOSK}/src/specs/NativeNearby.ts"
PACKAGE_JSON = f"{KIOSK}/package.json"
SHARED, CONSTRAINTS, LOCK, INSTALLED_LOCK = user_app.SHARED, user_app.CONSTRAINTS, user_app.LOCK, user_app.INSTALLED_LOCK
MARK = " (STALE APP ALLOWED: not a result for the current sources)"
ADB = user_app.ADB.replace("com.nu54renter", simulation.PACKAGE)  # the same emulator, holding the kiosk app
READS = ("shell getprop", "shell pm path", "shell sha256sum", "exec-out cat")  # the adb commands that change nothing


class Repository(user_app.Repository):
    """The user app's small repository with the kiosk app beside it."""

    def setUp(self):
        super().setUp()
        self.user_apk, self.apk = self.apk, self.root / simulation.APK
        for name, text in {
                MANIFEST: "<manifest/>\n", KOTLIN: "class NearbyModule\n", SPEC: "export interface Spec {}\n", PACKAGE_JSON: '{"codegenConfig": {}}\n',
                f"{KIOSK}/README.md": "# Kiosk\n", f"{KIOSK}/src/kiosk/Menu.tsx": "export {};\n", f"{KIOSK}/test/menu.test.ts": "test('menu', () => {});\n",
                f"{KIOSK}/ios/NU54Kiosk/AppDelegate.swift": "// ios\n", f"{KIOSK}/scripts/provision-dev.ts": "export {};\n",
                f"{KIOSK}/dev/merchant-profile.json": "{}\n", f"{KIOSK}/Gemfile": "source 'https://rubygems.org'\n"}.items():
            self.write(name, text)
        self.write(f"{KIOSK}/android/gradlew", user_app.GRADLEW)
        (self.root / KIOSK / "android/gradlew").chmod(0o755)

    def build(self):
        build_record.build(self.apk, self.root, build_record.KIOSK_APP, ["./gradlew", ":app:assembleDebug"], self.root / KIOSK / "android")

    def stale(self, installed=None):
        return build_record.stale(self.apk, self.root, build_record.KIOSK_APP, installed)


class KioskInputs(Repository):
    def test_a_build_from_the_current_sources_is_current(self):
        self.build()
        self.assertIsNone(self.stale())
        self.assertIsNone(self.stale(lambda: build_record.file_digest(self.apk)))

    def test_every_kind_of_build_input_counts(self):
        new = f"{KIOSK}/android/app/src/main/java/com/nu54kiosk/ble/NewModule.kt"
        for name in (MANIFEST, KOTLIN, SPEC, PACKAGE_JSON, SHARED, CONSTRAINTS, LOCK, f"{KIOSK}/android/gradlew", f"{KIOSK}/android/gradle.properties", new,
                     f"{KIOSK}/react-native.config.js", f"{KIOSK}/native/printer/escpos.c"):
            with self.subTest(name):
                self.install()
                self.build()
                self.write(name, "changed\n" if name != f"{KIOSK}/android/gradlew" else user_app.GRADLEW + "# changed\n")
                self.assertIn(name, reason := self.stale())
                self.assertIn("older than its sources: 1 changed", reason)

    def test_changes_outside_the_build_keep_it_current(self):
        self.build()
        for name in ("docs/content/commercial/traveller-services.md", f"{KIOSK}/README.md", f"{KIOSK}/MERCHANT-ORDER-V2.md", f"{KIOSK}/src/kiosk/Menu.tsx",
                     f"{KIOSK}/src/payment/New.tsx", f"{KIOSK}/test/menu.test.ts", f"{KIOSK}/test/android-pos/Input.java", f"{KIOSK}/ios/NU54Kiosk/AppDelegate.swift",
                     f"{KIOSK}/scripts/provision-dev.ts", f"{KIOSK}/dev/merchant-profile.json", f"{KIOSK}/.env.example", f"{KIOSK}/Gemfile", f"{KIOSK}/Gemfile.lock",
                     f"{KIOSK}/.bundle/config", f"{KIOSK}/Makefile", f"{KIOSK}/metro.config.js", f"{KIOSK}/index.js", f"{KIOSK}/app.json",
                     "packages/protocol/platform/test/frames.test.ts", "packages/protocol/platform/ios/Frames.swift", "scripts/simulate_kiosk_app.py"):
            self.write(name, "changed\n")
            self.assertIsNone(self.stale(), name)

    def test_what_a_build_or_a_tool_writes_is_no_input(self):
        self.build()
        for name in (f"{KIOSK}/android/app/build/generated/source/codegen/Spec.java", f"{KIOSK}/android/build/reports/problems.html", f"{KIOSK}/android/.gradle/8.14/fileHashes.bin",
                     f"{KIOSK}/android/.kotlin/sessions/kotlin-compiler.salive", f"{KIOSK}/android/app/.cxx/Debug/x86_64/build.ninja", f"{KIOSK}/node_modules/react-native/index.js",
                     f"{KIOSK}/vendor/bundle/ruby/3.3.0/gems/cocoapods-1.16.2/lib/cocoapods.rb", f"{KIOSK}/android/local.properties", f"{KIOSK}/android/.idea/workspace.xml",
                     f"{KIOSK}/android/app/app.iml", f"{KIOSK}/android/app/src/main/.DS_Store", f"{KIOSK}/.metro-health-check-4242", f"{KIOSK}/coverage/lcov.info",
                     f"{KIOSK}/.env", f"{KIOSK}/android/app/upload.keystore"):
            self.write(name, "written\n")
            self.assertIsNone(self.stale(), name)

    def test_a_new_source_in_a_directory_named_like_build_output_counts(self):
        self.build()
        new = f"{KIOSK}/android/app/src/main/java/com/nu54kiosk/build/Entry.kt"
        self.write(new, "class Entry\n")
        self.assertIn(new, self.stale())

    def test_a_link_counts_by_what_lies_where_it_leads(self):
        link = f"{KIOSK}/native/printer/frames.h"
        self.write("packages/protocol/c/frames.h", "#define FRAME 64\n")
        (self.root / link).parent.mkdir(parents=True)
        (self.root / link).symlink_to("../../../../packages/protocol/c/frames.h")
        self.build()
        self.assertIsNone(self.stale())
        self.write("packages/protocol/c/frames.h", "#define FRAME 128\n")
        self.assertIn(link, self.stale())

    def test_a_file_that_cannot_be_read_is_a_reason_and_not_a_traceback(self):
        self.build()
        digest = build_record.file_digest
        def refuse(path):
            if Path(path) == self.root / KOTLIN:
                raise PermissionError(13, "Permission denied", str(path))
            return digest(path)
        with mock.patch.object(build_record, "file_digest", refuse):
            self.assertIn(f"{KOTLIN} cannot be read (Permission denied)", self.stale())
            with self.assertRaises(SystemExit) as stopped:
                self.build()
            self.assertIn(f"{KOTLIN} cannot be read", str(stopped.exception))

    def test_a_failed_build_keeps_the_record_of_the_app_it_left_untouched(self):
        self.build()
        record = build_record.record_path(self.apk).read_bytes()
        self.write(f"{KIOSK}/android/fail-the-build", "")
        with self.assertRaises(SystemExit) as stopped:
            self.build()
        self.assertIn("did not build", str(stopped.exception))
        self.assertEqual(build_record.record_path(self.apk).read_bytes(), record)
        (self.root / KIOSK / "android/fail-the-build").unlink()
        self.assertIsNone(self.stale())

    def test_gradle_properties_of_the_environment_do_not_reach_the_build(self):
        self.build()
        plain = self.apk.read_bytes()
        with mock.patch.dict(os.environ, {"ORG_GRADLE_PROJECT_reactNativeArchitectures": "x86_64", "ORG_GRADLE_PROJECT_nu54P03Libs": "/lab/libs"}):
            self.build()
            self.assertEqual(self.apk.read_bytes(), plain)
            self.assertIsNone(self.stale())

    def test_node_modules_that_the_lockfile_does_not_name_refuse_the_build_and_the_run(self):
        self.build()
        self.write(INSTALLED_LOCK, "lockfileVersion: '9.0'\n# installed on another branch\n")
        self.assertIn("pnpm install", reason := self.stale())
        self.assertIn(INSTALLED_LOCK, reason)
        with self.assertRaises(SystemExit) as stopped:
            self.build()
        self.assertIn("pnpm install", str(stopped.exception))
        self.install()
        self.assertIsNone(self.stale())

    def test_a_change_of_one_app_leaves_the_other_current(self):
        def user_stale():
            return build_record.stale(self.user_apk, self.root, build_record.USER_APP)
        self.build()
        build_record.build(self.user_apk, self.root, build_record.USER_APP, ["./gradlew", ":app:assembleDebug"], self.root / user_app.APP / "android")
        self.assertIsNone(self.stale())
        self.assertIsNone(user_stale())
        for name in (user_app.KOTLIN, user_app.MANIFEST, user_app.SPEC, f"{user_app.APP}/package.json", f"{user_app.APP}/native/market/wallet.c"):
            self.write(name, "changed\n")
            self.assertIsNone(self.stale(), name)
            self.assertIn(name, user_stale())
        build_record.build(self.user_apk, self.root, build_record.USER_APP, ["./gradlew", ":app:assembleDebug"], self.root / user_app.APP / "android")
        for name in (KOTLIN, MANIFEST, SPEC, PACKAGE_JSON):
            self.write(name, "changed\n")
            self.assertIsNone(user_stale(), name)
            self.assertIn(name, self.stale())
        self.build()
        self.write(SHARED, "// shared, changed\n")  # the protocol package's Android sources go into both apps
        self.assertIn(SHARED, self.stale())
        self.assertIn(SHARED, user_stale())


class RealRepository(unittest.TestCase):
    def test_the_rules_name_the_real_build_inputs(self):
        files = set(build_record.input_files(ROOT, build_record.KIOSK_APP))
        for name in (MANIFEST, KOTLIN, SPEC, PACKAGE_JSON, f"{KIOSK}/android/app/build.gradle", f"{KIOSK}/android/build.gradle", f"{KIOSK}/android/settings.gradle",
                     f"{KIOSK}/android/gradle.properties", f"{KIOSK}/android/gradle/wrapper/gradle-wrapper.properties", f"{KIOSK}/android/app/debug.keystore",
                     SHARED, CONSTRAINTS, "scripts/android/release-signing.gradle", "scripts/android/release-signers.json", "scripts/android/legal-notices.gradle",
                     LOCK, ".npmrc"):
            self.assertIn(name, files)
        self.assertTrue(any(name.startswith("packages/protocol/platform/android/com/nu54/protocol/") for name in files))
        for name in files:
            self.assertFalse(name.startswith(("docs/", f"{KIOSK}/test/", f"{KIOSK}/ios/", f"{KIOSK}/src/kiosk/", f"{KIOSK}/scripts/", f"{KIOSK}/dev/", f"{KIOSK}/Gemfile",
                                              f"{KIOSK}/Makefile", f"{KIOSK}/metro.config.js", "packages/protocol/platform/ios/", "scripts/test_", "scripts/simulate_"))
                             or name.endswith(".md"), name)
            self.assertNotIn("/build/", name)

    def test_every_named_path_exists(self):
        """A renamed directory must not silently drop out of the rules."""
        for name in (*build_record.KIOSK_APP.take, *build_record.KIOSK_APP.leave, *build_record.KIOSK_APP.keep):
            self.assertTrue((ROOT / name).exists(), name)

    def test_app_build_gradle_applies_what_the_rules_take_from_outside_the_app(self):
        """The inputs outside the kiosk's directory are read off app/build.gradle: a file it starts to apply must be added to the rules."""
        gradle = (ROOT / KIOSK / "android/app/build.gradle").read_text()
        applied = sorted(line.strip() for line in gradle.splitlines() if line.startswith("apply from:"))
        self.assertEqual(applied, ['apply from: new File(new File(nu54MobilePackage).parentFile, "android/nu54-mobile.gradle")',
                                   'apply from: rootProject.file("../../../scripts/android/legal-notices.gradle")',
                                   'apply from: rootProject.file("../../../scripts/android/release-signing.gradle")',
                                   'apply from: rootProject.file("../../../scripts/android/runtime-constraints.gradle")'])
        self.assertIn("@nu54/protocol-mobile", (ROOT / "packages/protocol/platform/package.json").read_text())
        self.assertIn('rootProject.file("../../../scripts/android/release-signers.json")', (ROOT / "scripts/android/release-signing.gradle").read_text())

    def test_neither_app_is_an_input_of_the_other(self):
        kiosk, user = (set(build_record.input_files(ROOT, inputs)) for inputs in (build_record.KIOSK_APP, build_record.USER_APP))
        self.assertEqual([name for name in kiosk if name.startswith(user_app.APP + "/")], [])
        self.assertEqual([name for name in user if name.startswith(KIOSK + "/")], [])
        # Both: the protocol package's Android sources, the shared Gradle files and what names the dependencies.
        self.assertEqual(kiosk & user, {name for name in kiosk | user if not name.startswith("products/")})
        self.assertEqual({name.split("/")[1] for name in kiosk - user}, {"p04-merchant-kiosk"})
        self.assertEqual({name.split("/")[1] for name in user - kiosk}, {"p02-user-app"})


class Device(Repository, user_app.Device):
    """The stand-in adb and what its emulator holds: the user app test's, answering for the kiosk's package."""

    def setUp(self):
        super().setUp()  # the kiosk's repository on top of the user app test's device and repository
        self.assertIn(simulation.PACKAGE, ADB)
        (self.root.parent / "tools/adb").write_text(ADB)

    def put(self, apk=None):
        """Install on the emulator, as --build does: the build that lies in the repository, or these bytes."""
        if apk is None:
            shutil.copyfile(self.apk, self.device / "base.apk")
        else:
            (self.device / "base.apk").write_bytes(apk)

    def changes(self):
        """The adb commands that changed something on the emulator."""
        return [call for call in self.calls() if not call.startswith(READS)]


class Emulator(Device):
    """Screen.app() against the stand-in adb."""

    def app(self, **options):
        """What app() returned or the message it stopped with, and what it printed."""
        (self.device / "calls").write_text("")
        out, err = io.StringIO(), io.StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            try:
                result = simulation.Screen("emulator-5554").app(root=self.root, **options)
            except SystemExit as stopped:
                result = stopped
        return result, out.getvalue() + err.getvalue()

    def assertStopped(self, result, word):
        self.assertIsInstance(result, SystemExit)
        self.assertIn(word, str(result))
        self.assertIn("rebuild and install it with --build", str(result))
        self.assertIn("--allow-stale-app", str(result))
        self.assertEqual(self.changes(), [])  # neither the app nor the emulator was touched

    def test_build_installs_and_records_and_a_current_app_passes_untouched(self):
        result, printed = self.app(build=True)
        self.assertEqual(result, "", printed)
        self.assertEqual(printed, "Building the debug app\n")
        self.assertEqual((self.device / "base.apk").read_bytes(), self.apk.read_bytes())
        self.assertEqual(self.changes(), [f"install -r {self.apk}"])
        result, printed = self.app()
        self.assertEqual((result, printed), ("", ""))
        self.assertEqual(self.changes(), [])

    def test_an_image_without_sha256sum_is_read_whole(self):
        (self.device / "no-sha256sum").write_text("")
        self.assertEqual(self.app(build=True)[0], "")
        self.assertTrue([call for call in self.calls() if call.startswith("exec-out cat")])

    def test_a_native_source_changed_after_the_build_stops_the_run(self):
        self.app(build=True)
        self.write(KOTLIN, "class NearbyModule { fun ready() {} }\n")
        result, _ = self.app()
        self.assertStopped(result, "the debug app is older than its sources: 1 changed since it was built")
        self.assertIn(KOTLIN, str(result))

    def test_a_lockfile_changed_after_the_build_stops_the_run(self):
        self.app(build=True)
        self.write(LOCK, "lockfileVersion: '9.0'\n# pulled: another native dependency\n")
        result, _ = self.app()
        self.assertStopped(result, "older than its sources")
        self.assertIn(LOCK, str(result))
        self.install()  # installing what the new lockfile names does not make the old build current
        self.assertStopped(self.app()[0], LOCK)

    def test_node_modules_that_do_not_match_the_lockfile_stop_the_run(self):
        self.app(build=True)
        self.write(INSTALLED_LOCK, "lockfileVersion: '9.0'\n# installed on another branch\n")
        self.assertStopped(self.app()[0], "node_modules does not hold what pnpm-lock.yaml names")
        (self.root / INSTALLED_LOCK).unlink()
        self.assertStopped(self.app()[0], "run `pnpm install` first")
        result, _ = self.app(build=True)
        self.assertIn("run `pnpm install` first", str(result))
        self.assertEqual(self.changes(), [])

    def test_an_emulator_with_another_build_stops_the_run(self):
        self.app(build=True)
        self.put(b"built on 5 October")
        self.assertStopped(self.app()[0], "the app installed on the emulator is not the build at")

    def test_an_app_without_a_record_stops_the_run_and_names_the_build_option(self):
        self.put(b"built on 5 October")
        self.assertStopped(self.app()[0], "there is no debug app")
        self.apk.parent.mkdir(parents=True)
        self.apk.write_bytes(b"built on 5 October")  # by ./gradlew :app:assembleDebug, which records nothing
        result, _ = self.app()
        self.assertStopped(result, "nothing records which sources")
        self.assertIn("app-debug.apk.inputs.json is missing", str(result))

    def test_an_emulator_without_the_app_gets_the_current_build_only(self):
        result, _ = self.app()
        self.assertEqual(str(result), "install the debug app first: run this again with --build")
        self.assertEqual(self.changes(), [])
        self.build()
        self.assertEqual(self.app()[0], "")
        self.assertEqual(self.changes(), [f"install -r {self.apk}"])
        (self.device / "base.apk").unlink()
        self.write(KOTLIN, "class NearbyModule { fun ready() {} }\n")
        self.assertStopped(self.app()[0], "older than its sources")
        self.assertFalse((self.device / "base.apk").exists())
        result, printed = self.app(allow_stale=True)
        self.assertIn("older than its sources", result)
        self.assertEqual(printed.count("STALE APP ALLOWED"), 1)
        self.assertEqual(self.changes(), [f"install -r {self.apk}"])

    def test_the_override_runs_a_stale_app_and_says_so_loudly(self):
        self.app(build=True)
        self.write(KOTLIN, "class NearbyModule { fun ready() {} }\n")
        result, printed = self.app(allow_stale=True)
        self.assertIn("older than its sources", result)
        self.assertIn("STALE APP ALLOWED: the debug app is older than its sources", printed)
        self.assertIn("This run does not show that the current sources work.", printed)
        self.assertEqual(self.changes(), [])

    def test_a_build_that_fails_installs_nothing_and_keeps_the_last_record(self):
        self.app(build=True)
        record = build_record.record_path(self.apk).read_bytes()
        self.write(f"{KIOSK}/android/fail-the-build", "")
        result, _ = self.app(build=True)
        self.assertIn("the debug app did not build", str(result))
        self.assertEqual(self.changes(), [])
        self.assertEqual(build_record.record_path(self.apk).read_bytes(), record)
        (self.root / KIOSK / "android/fail-the-build").unlink()
        self.assertEqual(self.app(), ("", ""))  # the app installed before is still the recorded build

    def test_changes_that_do_not_reach_the_app_do_not_stop_the_run(self):
        self.app(build=True)
        for name in ("docs/content/commercial/traveller-services.md", f"{KIOSK}/src/kiosk/Menu.tsx", user_app.KOTLIN):
            self.write(name, "changed\n")
        self.assertEqual(self.app(), ("", ""))

    def test_an_emulator_that_is_not_running_or_no_emulator_stops_before_the_build(self):
        with self.assertRaises(SystemExit) as stopped:
            simulation.Screen("R58M123ABC")
        self.assertIn("only an emulator can run this", str(stopped.exception))
        with mock.patch.object(simulation.Screen, "shell", lambda screen, *args: ""):
            result, _ = self.app(build=True)
        self.assertEqual(str(result), "emulator-5554 is not running")
        self.assertFalse(self.apk.exists())


class Run(Device):
    """simulate_kiosk_app.main() against the stand-in adb. The store is not started, and the kiosk's screen shows the order button or nothing."""

    def setUp(self):
        super().setUp()
        self.evidence = self.root.parent / "evidence"
        self.log = self.root.parent / "host.log"
        self.log.write_text("=== RUN   TestOrderFlow\nKIOSK menu: 7 products\n")

    def run_simulation(self, *arguments, seen=True):
        """What main() ended with (None when it returned), what it printed, and the mark of the screen each store was started with."""
        (self.device / "calls").write_text("")
        printed, worlds, app, tap = io.StringIO(), [], simulation.Screen.app, simulation.Screen.tap

        def world(args, screen, tools):
            worlds.append(screen.mark)
            return nullcontext((mock.Mock(), self.log))

        with mock.patch.object(simulation.Screen, "app", lambda screen, *given: app(screen, *given, root=self.root)), \
                mock.patch.object(simulation.Screen, "nodes", lambda screen: [("새 고객 주문", "", [0, 0, 200, 80])] if seen else []), \
                mock.patch.object(simulation.Screen, "tap", lambda screen, label, seconds: tap(screen, label, seconds if seen else 0)), \
                mock.patch.object(simulation, "world", world), mock.patch.object(simulation, "step", lambda name: None), \
                mock.patch.object(simulation.flow, "postgres_tools", lambda given: Path("unused")), \
                mock.patch.object(simulation, "time", types.SimpleNamespace(sleep=lambda seconds: None, monotonic=time.monotonic)), \
                redirect_stdout(printed), redirect_stderr(printed):
            try:
                ended = simulation.main(["emulator-5554", str(self.evidence), *arguments])
            except SystemExit as stopped:
                ended = stopped.code
            except AssertionError as failed:  # how a kiosk that does not show what it should ends the run
                ended = failed
        return ended, printed.getvalue(), worlds

    def make_current(self):
        self.build()
        self.put()

    def make_stale(self):
        self.make_current()
        self.write(KOTLIN, "class NearbyModule { fun ready() {} }\n")

    def test_a_run_of_a_current_app_ends_as_before(self):
        self.make_current()
        ended, printed, worlds = self.run_simulation()
        self.assertIsNone(ended)
        self.assertEqual(printed, f"KIOSK menu: 7 products\nScreenshots in {self.evidence}\n")
        self.assertEqual(sorted(path.name for path in self.evidence.iterdir()), ["kiosk-menu.png"])
        self.assertEqual(worlds, [""])
        self.assertFalse([call for call in self.calls() if call.startswith("install")])

    def test_build_builds_installs_and_runs(self):
        ended, printed, worlds = self.run_simulation("--build")
        self.assertIsNone(ended, printed)
        self.assertEqual(printed.splitlines(), ["Building the debug app", "KIOSK menu: 7 products", f"Screenshots in {self.evidence}"])
        self.assertEqual((self.device / "base.apk").read_bytes(), self.apk.read_bytes())
        self.assertEqual(worlds, [""])

    def test_a_stale_app_stops_the_run_before_the_emulator_or_the_app_is_touched(self):
        self.make_stale()
        ended, printed, worlds = self.run_simulation()
        self.assertIn("the debug app is older than its sources: 1 changed since it was built", ended)
        self.assertIn(KOTLIN, ended)
        self.assertIn("rebuild and install it with --build, or pass --allow-stale-app", ended)
        self.assertEqual((printed, worlds), ("", []))
        self.assertEqual(self.changes(), [])  # the screen was not turned either
        self.assertFalse(self.evidence.exists())

    def test_a_refused_run_can_be_repeated_with_the_same_directory(self):
        self.make_stale()
        self.assertIn("older than its sources", self.run_simulation()[0])
        ended, printed, _ = self.run_simulation("--build")
        self.assertIsNone(ended, printed)
        self.assertEqual(printed.splitlines()[-1], f"Screenshots in {self.evidence}")

    def test_an_allowed_stale_app_is_marked_on_its_result_line_and_in_its_directory_and_ends_with_its_own_code(self):
        self.make_stale()
        ended, printed, worlds = self.run_simulation("--allow-stale-app")
        self.assertEqual(ended, 3)
        self.assertEqual(printed.splitlines()[-1], f"Screenshots in {self.evidence}{MARK}")
        self.assertIn("STALE APP ALLOWED: the debug app is older than its sources", printed)
        self.assertIn("older than its sources: 1 changed", marker := (self.evidence / "STALE-APP.txt").read_text())
        self.assertIn(KOTLIN, marker)
        self.assertEqual(worlds, [MARK])

    def test_a_store_kept_running_for_an_allowed_stale_app_is_marked_too(self):
        self.make_stale()
        ended, printed, _ = self.run_simulation("--stay", "--allow-stale-app")
        self.assertEqual(ended, 3)
        self.assertEqual(printed.splitlines()[-1], f"STORE-READY: drive the kiosk with adb; interrupt to end.{MARK}")
        self.assertTrue((self.evidence / "STALE-APP.txt").is_file())

    def test_a_store_kept_running_for_a_current_app_says_what_it_said(self):
        self.make_current()
        ended, printed, _ = self.run_simulation("--stay")
        self.assertIsNone(ended)
        self.assertEqual(printed, "STORE-READY: drive the kiosk with adb; interrupt to end.\n")
        self.assertEqual(list(self.evidence.iterdir()), [])

    def test_a_failed_run_of_an_allowed_stale_app_is_marked_and_stays_a_failure(self):
        self.make_stale()
        ended, printed, _ = self.run_simulation("--allow-stale-app", seen=False)
        self.assertIsInstance(ended, AssertionError)
        self.assertEqual(str(ended), f"no 새 고객 주문 on the kiosk{MARK}")
        self.assertNotIn("Screenshots in", printed)
        self.assertIn(KOTLIN, (self.evidence / "STALE-APP.txt").read_text())

    def test_a_failed_run_of_a_current_app_fails_without_a_mark(self):
        self.make_current()
        ended, printed, _ = self.run_simulation(seen=False)
        self.assertEqual(str(ended), "no 새 고객 주문 on the kiosk")
        self.assertEqual(printed, "KIOSK menu: 7 products\n")

    def test_a_current_app_ends_with_zero_also_when_a_stale_one_was_allowed(self):
        self.make_current()
        ended, printed, worlds = self.run_simulation("--allow-stale-app")
        self.assertIsNone(ended)
        self.assertEqual(printed.splitlines()[-1], f"Screenshots in {self.evidence}")
        self.assertNotIn("STALE", printed)
        self.assertEqual(sorted(path.name for path in self.evidence.iterdir()), ["kiosk-menu.png"])
        self.assertEqual(worlds, [""])

    def test_help_names_the_options_and_the_code(self):
        ended, printed, _ = self.run_simulation("--help")
        self.assertEqual(ended, 0)
        self.assertRegex(" ".join(printed.split()), r"--build build the debug app from the current sources")
        self.assertRegex(" ".join(printed.split()), r"--allow-stale-app .*ends with 3")

    def test_a_directory_that_is_there_already_stops_the_run_before_anything_is_built(self):
        self.evidence.mkdir()
        ended, _, worlds = self.run_simulation("--build")
        self.assertEqual(ended, f"{self.evidence} is there already: the screenshots go into a new directory")
        self.assertEqual(self.calls(), [])
        self.assertFalse(self.apk.exists())
        self.assertEqual(worlds, [])

    def test_the_wording_and_the_code_are_the_user_app_simulation_s(self):
        self.assertIs(simulation.REMEDY, user_app.simulation.REMEDY)
        self.assertEqual((simulation.STALE_MARK, simulation.STALE_EXIT), (MARK, 3))
        self.assertEqual((user_app.simulation.STALE_MARK, user_app.simulation.STALE_EXIT), (MARK, 3))


if __name__ == "__main__":
    unittest.main()
