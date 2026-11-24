import os
from pathlib import Path
import signal
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import test_release_workspace as release


class AndroidReleaseWorkspace(unittest.TestCase):
    def setUp(self):
        self.storage = tempfile.TemporaryDirectory()
        self.addCleanup(self.storage.cleanup)
        self.outer = Path(self.storage.name)
        self.root, self.home = self.outer / "repo", self.outer / "borrowed-gradle"
        self.project = self.root / "products/p02-user-app"
        for folder in ("node_modules/cli/build", "node_modules/fast-glob/out",
                       "node_modules/@react-native/gradle-plugin/react-native-gradle-plugin/build",
                       "packages", "scripts/android", "docs/content/specifications", "docs/content/commercial/legal",
                       "products/p01-device-firmware", "products/p02-user-app/android/app/build",
                       "products/p02-user-app/android/app/.cxx", "products/p02-user-app/android/gradle/wrapper"):
            (self.root / folder).mkdir(parents=True)
        (self.project / "android/app/debug.keystore").write_text("known fixture")
        (self.project / "android/app/build/old.apk").write_text("deployment must not be touched or copied")
        (self.project / "android/app/.cxx/built").write_text("old native output")
        (self.project / "android/gradle/wrapper/gradle-wrapper.properties").write_text(
            "distributionUrl=https\\://services.gradle.org/distributions/gradle-9.4.1-bin.zip\n")
        (self.root / "node_modules/cli/build/run.js").write_text("packaged JavaScript")
        (self.root / "node_modules/fast-glob/out/index.js").write_text("packaged JavaScript")
        (self.home / "caches/modules-2").mkdir(parents=True)
        self.dist = self.home / "wrapper/dists/gradle-9.4.1-bin/hash"
        self.dist.mkdir(parents=True)
        (self.dist / "download.zip.ok").touch()
        (self.dist / "download.zip.lck").touch()
        self.evidence = self.outer / "caller-evidence"
        self.evidence.mkdir()
        (self.evidence / "caller.txt").write_text("caller receipt")
        for name in release.RECEIPT_FILES:
            (self.evidence / name).write_text("previous receipt")
        self.env = {"GRADLE_USER_HOME": str(self.home), "NU54_ANDROID_EVIDENCE_DIR": str(self.evidence),
                    "NPM_CONFIG_CACHE": str(self.home / "caller-npm")}

    def check_run(self, command, *, cwd, env, **_):
        owned = cwd.parent
        self.assertIn("nu54-android-release-p02-", owned.name)
        self.assertEqual(command, ["bash", cwd / "scripts/android/test_release_checks.sh", "p02"])
        android = cwd / "products/p02-user-app/android"
        self.assertFalse((android / "app/build").exists())
        self.assertFalse((android / "app/.cxx").exists())
        self.assertEqual((android / "app/debug.keystore").read_text(), "known fixture")
        self.assertTrue((cwd / "node_modules/cli/build/run.js").is_file())
        self.assertTrue((cwd / "node_modules/fast-glob/out/index.js").is_file())
        self.assertFalse((cwd / "node_modules/@react-native/gradle-plugin/react-native-gradle-plugin/build").exists())
        gradle = Path(env["GRADLE_USER_HOME"])
        self.assertEqual(gradle.parent, owned)
        self.assertEqual(env["GRADLE_RO_DEP_CACHE"], str((self.home / "caches").resolve()))
        self.assertEqual(env["npm_config_cache"], str(owned / "npm-cache"))
        self.assertNotIn("NPM_CONFIG_CACHE", env)
        self.assertEqual(Path(env["NU54_ANDROID_EVIDENCE_DIR"]), owned / "evidence")
        (owned / "evidence").mkdir()
        for name in release.RECEIPT_FILES:
            (owned / "evidence" / name).write_text("approved receipt")
        self.assertTrue((gradle / "wrapper/dists/gradle-9.4.1-bin/hash/download.zip.ok").is_file())
        self.assertFalse((gradle / "wrapper/dists/gradle-9.4.1-bin/hash/download.zip.lck").exists())
        (android / "app/build").mkdir()
        return owned

    def test_success_failure_and_term_remove_build_and_gradle_state(self):
        for outcome in ("failure", "term", "success"):
            visited = []
            def commands(*args, **kwargs):
                visited.append(self.check_run(*args, **kwargs))
                if outcome == "failure":
                    raise subprocess.CalledProcessError(7, args)
                if outcome == "term":
                    os.kill(os.getpid(), signal.SIGTERM)
            with self.subTest(outcome=outcome), patch.object(release, "run_command", commands):
                if outcome == "success":
                    release.run("p02", self.root, self.env)
                elif outcome == "failure":
                    with self.assertRaises(subprocess.CalledProcessError):
                        release.run("p02", self.root, self.env)
                else:
                    with self.assertRaises(SystemExit) as stopped:
                        release.run("p02", self.root, self.env)
                    self.assertEqual(stopped.exception.code, 143)
            self.assertTrue(visited and not visited[0].exists())
            self.assertEqual((self.project / "android/app/build/old.apk").read_text(), "deployment must not be touched or copied")
            self.assertTrue((self.dist / "download.zip.lck").exists())
            self.assertEqual((self.evidence / "caller.txt").read_text(), "caller receipt")
            expected = "approved receipt" if outcome == "success" else "previous receipt"
            for name in release.RECEIPT_FILES:
                self.assertEqual((self.evidence / name).read_text(), expected)
            self.assertFalse(list(self.evidence.glob(".android-evidence-*")))

    def test_external_source_symlink_fails_before_build(self):
        external_file = self.home / "external"
        external_file.write_text("caller-owned")
        for link in (self.root / "node_modules/external", self.root / "package.json", self.project / "android/app/debug.keystore"):
            if link.exists():
                link.unlink()
            link.symlink_to(external_file)
            with self.subTest(link=link), patch.object(release, "run_command") as command:
                with self.assertRaisesRegex(ValueError, "symlink escapes its workspace"):
                    release.run("p02", self.root, self.env)
                command.assert_not_called()
            link.unlink()
        self.assertEqual(external_file.read_text(), "caller-owned")

    def test_hangup_uses_workspace_cleanup(self):
        owned = []

        def interrupted(*args, **kwargs):
            owned.append(self.check_run(*args, **kwargs))
            signal.raise_signal(signal.SIGHUP)

        with (patch.object(release, "ROOT", self.root),
              patch.object(release.sys, "argv", ["runner", "p02"]),
              patch.dict(release.os.environ, self.env, clear=True),
              patch.object(release, "run_command", interrupted),
              self.assertRaises(SystemExit) as stopped):
            release.main()
        self.assertEqual(stopped.exception.code, 129)
        self.assertTrue(owned and not owned[0].exists())

    def test_failed_publication_restores_previous_receipts(self):
        replace = Path.replace

        def fail_second(source, destination):
            if source.name == release.RECEIPT_FILES[1] and source.parent.name.startswith(".android-evidence-"):
                raise OSError("cannot publish second file")
            return replace(source, destination)

        current = dict.fromkeys(release.RECEIPT_FILES, b"new receipt")
        with patch.object(Path, "replace", fail_second), self.assertRaisesRegex(OSError, "second file"):
            release.publish_evidence(self.evidence, current)
        for name in release.RECEIPT_FILES:
            self.assertEqual((self.evidence / name).read_text(), "previous receipt")
        self.assertFalse(list(self.evidence.glob(".android-evidence-*")))
