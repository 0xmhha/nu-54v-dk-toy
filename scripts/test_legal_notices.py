#!/usr/bin/env python3
from contextlib import redirect_stderr, redirect_stdout
import io
import json
from pathlib import Path
import shutil
import sys
import tempfile
import unittest
from zipfile import ZipFile

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
import legal_notices as legal

APP = "p02-user-app"
KIOSK = "p04-merchant-kiosk"


class LegalNotices(unittest.TestCase):
    def setUp(self):
        storage = tempfile.TemporaryDirectory()
        self.addCleanup(storage.cleanup)
        self.root = Path(storage.name).resolve() / "repo"
        for app, folder in legal.APPS.items():
            for name in legal.DOCUMENTS:
                for language in legal.LANGUAGES:
                    self.document(folder, name, language)
            self.write(f"products/{app}/package.json", {"name": app, "version": "1.0.0", "dependencies": {"alpha": "1", "react-native": "1"}})
            self.write(f"products/{app}/android/app/build.gradle", 'dependencies {\n  implementation("com.example:core:1.0")\n}\n')
            self.write(f"products/{app}/ios/Podfile.lock", "PODS:\n  - glog (0.3.5):\n    - Base\n  - React (0.1.0)\n\nDEPENDENCIES:\n")
        self.write("scripts/android/runtime-constraints.gradle", "")
        self.write("packages/protocol/platform/android/nu54-mobile.gradle", "")
        self.publisher(None)
        self.package("alpha", {"version": "1.2.0", "license": "MIT", "dependencies": {"beta": "1"}}, {"LICENSE": "Copyright (c) Alpha\nPermission is hereby granted\n"})
        self.package("beta", {"version": "2.0.0", "license": "MIT", "peerDependencies": {"absent": "1"},
                              "peerDependenciesMeta": {"absent": {"optional": True}}}, {"index.js": "/*\n * Copyright (c) Beta Authors.\n */\n"})
        self.package("react-native", {"version": "0.1.0", "license": "MIT"}, {"LICENSE.md": "Permission is hereby granted\n"})
        texts = self.root / legal.LICENSES / "texts"
        shutil.copytree(legal.ROOT / legal.LICENSES / "texts", texts)
        self.native = {
            "corrections": {}, "podPackages": {}, "moduleOverrides": {},
            "bundled": [{"name": "glog", "apps": list(legal.APPS), "pod": "glog", "license": "BSD-3-Clause", "texts": ["glog"]}],
            "android": {}, "modules": {"com.example:core:1.0": {"license": "Apache-2.0", "copyright": ["Example"], "origin": "Its Maven POM"}}}
        for app in legal.APPS:
            self.native["android"][app] = {"inputs": legal.gradle_inputs(self.root, app), "modules": ["com.example:core:1.0"]}
        self.save_native()
        self.generate()

    def write(self, name, content):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content if isinstance(content, str) else json.dumps(content), encoding="utf-8")
        return path

    def document(self, folder, name, language, status="draft", effective="null", edition="1"):
        note = "> This is an unreviewed draft.\n" if status == "draft" else ""
        self.write(legal.SOURCES / folder / f"{name}.{language}.md",
                   f"---\nid: {name}\nlanguage: {language}\ntitle: {name}\nedition: {edition}\nstatus: {status}\neffective: {effective}\n---\n\n"
                   f"{note}\n## Contact\n\n{{{{publisher.name}}}}: {{{{publisher.contact}}}}, {{{{retention.orders}}}}\n")

    def publisher(self, value):
        self.write(legal.SOURCES / "publisher.json",
                   {"publisher": {"name": value, "contact": value}, "retention": {"orders": value and {"ko": "하루", "en": "one day"}}})

    def package(self, name, manifest, files, inside="node_modules"):
        self.write(f"{inside}/{name}/package.json", {"name": name, **manifest})
        for file, content in files.items():
            self.write(f"{inside}/{name}/{file}", content)

    def save_native(self):
        self.write(legal.LICENSES / "native.json", self.native)

    def run_script(self, *arguments):
        out, err = io.StringIO(), io.StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            status = legal.main(list(arguments), self.root)
        return status, out.getvalue(), err.getvalue()

    def generate(self):
        status, _, error = self.run_script()
        self.assertEqual(0, status, error)

    def generated(self, app=APP):
        return json.loads(legal.output_path(self.root, app).read_text(encoding="utf-8"))

    def finish(self):
        self.publisher("Example")
        for folder in legal.APPS.values():
            for name in legal.DOCUMENTS:
                for language in legal.LANGUAGES:
                    self.document(folder, name, language, "final", "2030-01-01")
        self.generate()

    def refused(self, pattern, *arguments):
        status, _, error = self.run_script(*(arguments or ("--release", "--app", APP)))
        self.assertEqual(1, status)
        self.assertRegex(error, pattern)

    def test_shape(self):
        data = self.generated()
        self.assertEqual(["version", "publisher", "documents", "licenses", "texts"], list(data))
        self.assertEqual({"name": None, "contact": None}, data["publisher"])
        self.assertEqual({(name, language) for name in legal.DOCUMENTS for language in legal.LANGUAGES},
                         {(item["id"], item["language"]) for item in data["documents"]})
        self.assertEqual(["alpha", "beta", "com.example:core", "glog", "react-native"], [item["name"] for item in data["licenses"]])
        self.assertEqual(len(data["texts"]), len(set(data["texts"])))
        self.assertTrue(all(data["texts"][item["text"]].strip() for item in data["licenses"]))
        self.assertIn(legal.BLANK["en"], data["documents"][-1]["sections"][0]["body"])

    def test_drafts_and_blanks_stop_a_release_but_not_the_check(self):
        self.assertEqual(0, self.run_script("--check")[0])
        for point in ("publisher.name is blank", "publisher.contact is blank", "retention.orders is blank",
                      "terms.ko.md: status is draft", "privacy.en.md: effective date is blank"):
            self.refused(point)

    def test_complete_notices_allow_a_release(self):
        self.finish()
        for app in legal.APPS:
            self.assertEqual(0, self.run_script("--release", "--app", app)[0])
        self.assertEqual("Example: Example, one day", self.generated()["documents"][-1]["sections"][0]["body"])
        self.assertEqual("2030-01-01", self.generated()["documents"][0]["effective"])

    def test_one_missing_decision_stops_a_release(self):
        self.finish()
        for change, pattern in (
            ({"publisher": {"name": " ", "contact": "c"}, "retention": {"orders": "o"}}, "publisher.name is blank"),
            ({"publisher": {"name": "n", "contact": None}, "retention": {"orders": "o"}}, "publisher.contact is blank"),
            ({"publisher": {"name": "n", "contact": "c"}, "retention": {"orders": {"ko": "하루"}}}, "retention.orders is blank")):
            with self.subTest(pattern):
                self.write(legal.SOURCES / "publisher.json", change)
                self.generate()
                self.refused(pattern)
        self.finish()
        for language in legal.LANGUAGES:
            self.document("user-app", "terms", language, "final", "null")
        self.generate()
        self.refused("terms.ko.md: effective date is blank")
        self.assertEqual(0, self.run_script("--release", "--app", KIOSK)[0])

    def test_languages_must_be_the_same_edition(self):
        self.finish()
        self.document("user-app", "terms", "en", "final", "2030-01-02")
        self.refused("the languages differ", "--check")

    def test_document_errors(self):
        cases = (("status: draft", "status: ready", "draft or final"), ("effective: null", "effective: 2030-1-1", "YYYY-MM-DD"),
                 ("edition: 1", "edition: " + "x" * 33, "edition needs"), ("language: ko", "language: en", "another document"),
                 ("> This is an unreviewed draft.\n", "", "unreviewed draft"), ("publisher.name", "publisher.nobody", "unknown placeholder"))
        for old, new, pattern in cases:
            with self.subTest(pattern):
                self.document("user-app", "terms", "ko")
                path = self.root / legal.SOURCES / "user-app/terms.ko.md"
                path.write_text(path.read_text().replace(old, new, 1))
                self.refused(pattern, "--check")
        (self.root / legal.SOURCES / "user-app/terms.ko.md").unlink()
        self.refused("terms.ko.md", "--check")

    def test_outdated_file_stops_the_check_and_a_release(self):
        self.finish()
        path = legal.output_path(self.root, APP)
        path.write_text(path.read_text() + " ")
        self.refused("is outdated", "--check")
        self.refused("is outdated")
        path.unlink()
        self.refused("is outdated")

    def test_a_dependency_change_makes_the_file_outdated(self):
        self.package("alpha", {"version": "1.3.0", "license": "MIT"}, {})
        self.refused("is outdated", "--check")

    def test_missing_license_text_fails(self):
        for manifest, files, pattern in (
            ({"version": "1", "license": "Custom-1.0"}, {}, "no standard text"),
            ({"version": "1", "license": "SEE LICENSE IN other"}, {}, "no standard text"),
            ({"version": "1"}, {}, "declares no licence"),
            ({"version": "1", "license": "MIT"}, {}, "no author or copyright holder"),
            ({"version": "1", "license": "MIT"}, {"LICENSE": " \n"}, "is empty"),
            ({"version": "1", "license": "MIT"}, {"LICENSE": "Apache License\nVersion 2.0"}, "another licence")):
            with self.subTest(pattern):
                shutil.rmtree(self.root / "node_modules/beta")
                self.package("beta", manifest, files)
                self.refused(f"beta 1.*{pattern}", "--check")
                self.assertEqual(1, self.run_script()[0])
        shutil.rmtree(self.root / "node_modules/beta")
        self.refused("dependency beta is not installed", "--check")

    def test_a_standard_text_is_recorded_and_names_the_holder(self):
        entry = next(item for item in self.generated()["licenses"] if item["name"] == "beta")
        text = self.generated()["texts"][entry["text"]]
        self.assertTrue(entry["substituted"])
        self.assertIn("carries no licence file", text)
        self.assertIn("Copyright (c) Beta Authors", text)
        self.assertIn("Permission is hereby granted", text)
        self.assertNotIn("substituted", next(item for item in self.generated()["licenses"] if item["name"] == "alpha"))

    def test_author_choice_and_correction(self):
        self.package("beta", {"version": "3", "license": "(MIT OR Apache-2.0)", "author": "Someone <a@b.c> (https://x)"}, {"index.js": "0"})
        self.package("alpha", {"version": "1.2.0", "license": "MIT", "dependencies": {"beta": "1"}}, {"LICENSE": "Apache License\nVersion 2.0\n"})
        self.native["corrections"]["alpha"] = {"declared": "MIT", "license": "Apache-2.0"}
        self.save_native()
        self.generate()
        rows = {item["name"]: item for item in self.generated()["licenses"]}
        self.assertEqual("Apache-2.0", rows["alpha"]["license"])
        self.assertIn("Copyright (c) Someone\n", self.generated()["texts"][rows["beta"]["text"]])

    def test_native_changes_are_noticed(self):
        gradle = f"products/{APP}/android/app/build.gradle"
        before = (self.root / gradle).read_text()
        self.write(gradle, before + 'implementation("com.example:new:2.0")\n')
        self.refused("Android build inputs changed.*com.example:new", "--check")
        self.write(gradle, before)
        lock = f"products/{APP}/ios/Podfile.lock"
        self.write(lock, "PODS:\n  - glog (0.3.5)\n  - Stranger (1.0)\n\nDEPENDENCIES:\n")
        self.refused("iOS pod Stranger 1.0: no recorded licence", "--check")
        self.write(lock, "PODS:\n  - React (0.1.0)\n\nDEPENDENCIES:\n")
        self.refused("glog: its version is no longer found", "--check")
        self.write(lock, "PODS:\n  - glog (0.3.5)\n\nDEPENDENCIES:\n")
        self.native["modules"].clear()
        self.native["bundled"][0]["texts"] = ["nothing"]
        self.save_native()
        self.refused("com.example:core:1.0: no recorded licence\n.*licence text nothing is missing", "--check")

    def test_resolved_runtime_must_match_the_notices(self):
        self.finish()
        runtime = self.write("runtime.txt", "com.example:core:1.0=releaseRuntimeClasspath\n")
        arguments = ("--app", APP, "--resolved-runtime", str(runtime))
        self.assertEqual(0, self.run_script("--release", *arguments)[0])
        runtime.write_text("com.example:core:1.1=releaseRuntimeClasspath\n")
        self.refused("core:1.1: in the Android build, not in the notices\n.*core:1.0: in the notices, no longer", "--release", *arguments)
        self.refused("core:1.1", "--check", *arguments)
        runtime.write_text("")
        self.refused("not a resolved Android runtime", "--release", *arguments)

    def test_refresh_reads_poms_and_needs_a_hand_written_row_for_the_rest(self):
        cache = self.root / "cache"
        pom = "<project><licenses><license><name>{}</name></license></licenses><developers><developer><name>Maker</name></developer></developers></project>"
        self.write("cache/com.new/lib/2.0/hash/lib-2.0.pom", pom.format("The Apache License, Version 2.0"))
        runtime = self.write("runtime.txt", "com.new:lib:2.0=releaseRuntimeClasspath\n")
        legal.refresh_android(self.root, APP, runtime, cache)
        native = legal.read_json(self.root / legal.LICENSES / "native.json")
        self.assertEqual({"license": "Apache-2.0", "copyright": ["Maker"], "origin": "Its Maven POM"}, native["modules"]["com.new:lib:2.0"])
        self.assertEqual(["com.new:lib:2.0"], native["android"][APP]["modules"])
        with ZipFile(self.root / "cache/com.new/lib/2.0/hash/lib-2.0.jar", "w") as archive:
            archive.writestr("META-INF/NOTICE", "attribution")
        with self.assertRaisesRegex(legal.LegalError, "need a moduleOverrides entry"):
            legal.refresh_android(self.root, APP, runtime, cache)

    def test_output_is_the_same_every_time(self):
        first = {app: legal.output_path(self.root, app).read_bytes() for app in legal.APPS}
        for app in legal.APPS:
            legal.output_path(self.root, app).unlink()
        self.generate()
        self.assertEqual(first, {app: legal.output_path(self.root, app).read_bytes() for app in legal.APPS})
        moved = self.root.parent / "elsewhere"
        shutil.copytree(self.root, moved)
        self.root = moved
        self.assertEqual(0, self.run_script("--check")[0])

    def test_there_is_no_way_to_skip_the_release_check(self):
        for flag in ("--force", "--skip", "--allow-draft", "--root"):
            with self.assertRaises(SystemExit), redirect_stderr(io.StringIO()):
                legal.main(["--release", flag], self.root)
        with self.assertRaises(SystemExit), redirect_stderr(io.StringIO()):
            legal.main(["--release", "--check"], self.root)


class ReleaseWiring(unittest.TestCase):
    """The checks only help where a build runs them."""

    def test_android_release_variant_of_both_apps(self):
        for app in legal.APPS:
            gradle = (legal.ROOT / "products" / app / "android/app/build.gradle").read_text()
            self.assertEqual(1, gradle.count('apply from: rootProject.file("../../../scripts/android/legal-notices.gradle")'))
        gate = (legal.ROOT / "scripts/android/legal-notices.gradle").read_text()
        self.assertIn('"preReleaseBuild"', gate)
        self.assertNotIn("environmentVariable", gate)
        self.assertNotIn("getenv", gate)

    def test_official_ios_bundle_of_both_apps(self):
        policies = {APP: ("NU54Renter", "renter-release-signers.json"), KIOSK: ("NU54Kiosk", "release-signers.json")}
        for app, (project, policy) in policies.items():
            bundle = json.loads((legal.ROOT / "scripts/ios" / policy).read_text())["bundle"]
            source = (legal.ROOT / f"products/{app}/ios/{project}.xcodeproj/project.pbxproj").read_text()
            self.assertIn(f'\\"$PRODUCT_BUNDLE_IDENTIFIER\\" = \\"{bundle}\\"', source)
            self.assertIn(f"legal_notices.py\\\" --release --app {app}", source)
            self.assertEqual(2, source.count("/* Check legal notices of the official app */"))


if __name__ == "__main__":
    unittest.main()
