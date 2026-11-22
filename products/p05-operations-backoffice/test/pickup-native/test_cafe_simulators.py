import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

from cafe_fixture import TESTS, read_pair, rendered_test_passed
from cafe_simulators import retire_owned_simulators

CLERK = "a100a100-0000-4000-8000-a100a100a100"
CUSTOMER = "b100b100-0000-4000-8000-b100b100b100"
PREEXISTING = "c100c100-0000-4000-8000-c100c100c100"
SIBLING = "d100d100-0000-4000-8000-d100d100d100"


class Simctl:
    def __init__(self):
        self.devices = {CLERK, CUSTOMER, PREEXISTING}
        self.transient = set()
        self.persistent = set()
        self.malformed = False
        self.deleted = []

    def __call__(self, action, *args, check=False):
        if action == "delete":
            identifier, = args
            self.deleted.append(identifier)
            # A different test adds a simulator while cleanup runs.
            self.devices.add(SIBLING)
            if identifier in self.transient:
                self.transient.remove(identifier)
                return subprocess.CompletedProcess(args, 37)
            if identifier in self.persistent:
                raise subprocess.TimeoutExpired(args, 20)
            self.devices.discard(identifier)
        if action == "list":
            assert check
            devices = [{"udid": identifier} for identifier in self.devices]
            if self.malformed:
                devices.append({"udid": "partial list"})
            return subprocess.CompletedProcess(args, 0, json.dumps({"devices": {"runtime": devices}}))
        return subprocess.CompletedProcess(args, 0)


class CafeSimulatorCleanup(unittest.TestCase):
    def test_retries_only_remaining_owned_id_despite_sibling_change(self):
        simctl = Simctl()
        simctl.transient.add(CLERK)
        self.assertEqual(retire_owned_simulators(simctl, [CLERK.upper(), CUSTOMER]), [])
        self.assertEqual(simctl.deleted, [CLERK, CUSTOMER, CLERK])
        self.assertEqual(simctl.devices, {PREEXISTING, SIBLING})

    def test_persistent_failure_returns_exact_residual(self):
        simctl = Simctl()
        simctl.persistent.add(CUSTOMER)
        self.assertEqual(retire_owned_simulators(simctl, [CLERK, CUSTOMER]), [CUSTOMER])
        self.assertEqual(simctl.deleted, [CLERK, CUSTOMER, CUSTOMER, CUSTOMER])
        self.assertEqual(simctl.devices, {PREEXISTING, SIBLING, CUSTOMER})

    def test_invalid_successful_inventory_cannot_certify_absence(self):
        simctl = Simctl()
        simctl.malformed = True
        self.assertEqual(retire_owned_simulators(simctl, [CUSTOMER, CLERK]), [CLERK, CUSTOMER])
        self.assertEqual(simctl.devices, {PREEXISTING, SIBLING})


    def test_real_runner_preserves_boot_error_and_reports_owned_residue(self):
        self.runner_scenario("boot", 37, [CUSTOMER], {PREEXISTING, SIBLING, CUSTOMER},
                             [CLERK, CUSTOMER, CLERK, CUSTOMER, CUSTOMER])

    def test_staged_mutation_is_reported_with_original_failure(self):
        self.runner_scenario("stage-drift", 37, [CUSTOMER], {PREEXISTING, SIBLING, CUSTOMER},
                             [CLERK, CUSTOMER, CLERK, CUSTOMER, CUSTOMER])

    def test_real_runner_recovers_creation_without_stdout(self):
        for mode, status in (("create-failed", 41), ("create-invalid-id", 1)):
            with self.subTest(mode=mode):
                self.runner_scenario(mode, status, [], {PREEXISTING, SIBLING}, [CLERK, CUSTOMER, CLERK])

    def test_real_runner_allocates_before_deferred_interrupt_cleanup(self):
        self.runner_scenario("create-interrupted", 143, [], {PREEXISTING, SIBLING},
                             [CLERK, CUSTOMER, CLERK])

    def test_real_runner_preserves_ambiguous_creation_candidates(self):
        self.runner_scenario("create-ambiguous", 41, [], {PREEXISTING, SIBLING, CUSTOMER}, [CLERK, CLERK],
                             uncertain=True)

    def runner_scenario(self, scenario, exit_code, residual, devices_after, deletion_order, *, uncertain=False):
        source = Path(__file__).resolve().parent
        repo = source.parents[3]
        with tempfile.TemporaryDirectory(prefix="nu54-cafe-cleanup-") as suffix:
            root = Path(suffix)
            harness = root / source.relative_to(repo)
            harness.mkdir(parents=True)
            for name in ("run-cafe-owned.py", "cafe_simulators.py", "cafe_fixture.py"):
                shutil.copyfile(source / name, harness / name)
            for name in ("scripts/integration_workspace.py", "products/p10-platform/database/migrate.py"):
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(repo / name, target)
            ios = root / "scripts/ios"
            ios.mkdir()
            (ios / "test-environment.sh").write_text("ios_test_prepare_environment() { :; }\n")
            (ios / "stage-project.sh").write_text(
                'scene_stage_ios_source() { :; }\n'
                'scene_stage_ios_project() { '
                'local app="$2/products/$5"; '
                'mkdir -p "$app/ios/Pods" "$app/ios/build" "$app/ios/${6%Spec}"; '
                'cp -R "$1/products/$5/src" "$app/"; '
                'for f in app.json babel.config.js index.js metro.config.js package.json; do '
                'printf "%s" "$f" >"$app/$f"; done; }\n'
            )
            refs = {"p02-user-app": ("merchant/CustomerOrders.tsx",),
                    "p04-merchant-kiosk": ("kiosk/MerchantScreen.tsx", "kiosk/PickupKitchen.tsx")}
            for product, names in refs.items():
                for name in names:
                    target = root / "products" / product / "src" / name
                    target.parent.mkdir(parents=True, exist_ok=True)
                    target.write_text(name)
            tools = root / "bin"
            tools.mkdir()
            stub = tools / "stub"
            stub.write_text("#!" + sys.executable + "\n" + """
import json, os, pathlib, signal, sys
tool = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
work = pathlib.Path(os.environ["K05_STUB_DIR"])
file = work / "devices.json"
devices, deletions = json.loads(file.read_text())
scenario = os.environ["K05_STUB_SCENARIO"]
clerk, customer, existing, sibling = json.loads(os.environ["K05_STUB_IDS"])
if tool == "codesign":
    raise SystemExit(0)
if tool == "xcodebuild":
    product = pathlib.Path(args[args.index("-derivedDataPath")+1])
    name = args[args.index("-scheme")+1]
    app = product / "Build/Products/Release-iphonesimulator" / (name + ".app")
    app.mkdir(parents=True)
    (app / name).write_text("signed")
    (app / "main.jsbundle").write_text("bundle")
    if name == "NU54Renter":
        ios = pathlib.Path(args[args.index("-workspace")+1]).parent
        app_root = ios.parent
        tree = "src/merchant/CustomerOrders.tsx"
        upstream = work / "products/p02-user-app" / tree
        upstream.write_text("changed outside private build")
        if scenario == "stage-drift":
            (app_root / tree).write_text("changed inside private build")
    raise SystemExit(0)
assert tool == "xcrun" and args[0] == "simctl"
action = args[1]
status = 0
if action == "list":
    print(json.dumps({"devices": {"runtime": [{"udid": u, "name": label} for u, label in devices.items()]}}))
elif action == "create":
    u = clerk if "clerk" in args[2] else customer
    devices[u] = args[2]
    if u == clerk or scenario in ("boot", "stage-drift"):
        print(u)
    elif scenario == "create-invalid-id":
        print(existing)
    elif scenario == "create-interrupted":
        file.write_text(json.dumps([devices, deletions]))
        os.kill(os.getppid(), signal.SIGTERM)
        print(u)
    else:
        status = 41
        if scenario == "create-ambiguous":
            devices[sibling] = args[2]
elif action == "boot":
    status = 37 if args[2] == customer and scenario in ("boot", "stage-drift") else 0
elif action == "delete":
    u = args[2]
    deletions.append(u)
    devices.setdefault(sibling, "another caller's device")
    if (u == customer and scenario in ("boot", "stage-drift")) or (u == clerk and deletions.count(clerk) == 1):
        status = 1
    elif u in devices:
        del devices[u]
else:
    assert action == "shutdown"
file.write_text(json.dumps([devices, deletions]))
raise SystemExit(status)
""")
            stub.chmod(0o700)
            for name in ("xcrun", "codesign", "xcodebuild", "pg_ctl"):
                (tools / name).symlink_to(stub)
            state = root / "devices.json"
            state.write_text(json.dumps([{PREEXISTING: "preexisting"}, []]))
            env = dict(os.environ, PATH=str(tools) + os.pathsep + os.environ["PATH"], POSTGRES_BIN=str(tools),
                       TMPDIR=str(root), K05_STUB_DIR=str(root), K05_STUB_SCENARIO=scenario,
                       K05_STUB_IDS=json.dumps([CLERK, CUSTOMER, PREEXISTING, SIBLING]))
            result = subprocess.run([sys.executable, "-B", harness / "run-cafe-owned.py"], env=env,
                                    check=False, capture_output=True, text=True, timeout=30)
            self.assertEqual(result.returncode, exit_code, result.stderr)
            devices, deletions = json.loads(state.read_text())
            self.assertEqual(set(devices), devices_after)
            self.assertEqual(deletions, deletion_order)
            report = json.loads(result.stdout)
            self.assertEqual(report["remainingOwnedSimulators"], residual)
            labels = report["uncertainOwnedSimulatorLabels"]
            if uncertain:
                self.assertEqual(len(labels), 1)
                self.assertTrue(labels[0].startswith("NU54 K05 customer nu54-k05-native-"))
            else:
                self.assertEqual(labels, [])
            self.assertEqual(report["ownedSimulatorsRemoved"], not residual and not uncertain)
            self.assertTrue(report["scratchRemoved"])
            self.assertTrue(report["sharedCodegenUnchanged"])
            self.assertEqual(report["stagedAppInputsUnchanged"], scenario != "stage-drift")
            for product, names in refs.items():
                manifest = report["stagedAppInputs"][product]
                self.assertEqual(manifest["files"], 5 + len(names))
                self.assertRegex(manifest["sha256"], r"^[a-f0-9]{64}$")
                self.assertEqual(manifest["references"], {
                    "src/" + name: hashlib.sha256(name.encode()).hexdigest() for name in names})
            self.assertEqual((root / "products/p02-user-app/src/merchant/CustomerOrders.tsx").read_text(),
                             "changed outside private build")
            self.assertEqual(list(root.glob("nu54-k05-native-*")), [])


class CafeInstalledPair(unittest.TestCase):
    def test_early_public_files_cannot_select_nearby_tests(self):
        source = Path(__file__).resolve().parent
        with tempfile.TemporaryDirectory(prefix="nu54-cafe-pair-") as folder:
            lab = Path(folder)
            profile = {"control": "http://127.0.0.1:1234", "sale": CLERK}
            staff = lab / "staff.json"
            buyer = lab / "customer.json"
            staff.write_text(json.dumps(profile))
            buyer.write_text(json.dumps(profile))
            self.assertEqual(read_pair(lab, "unknown")["sale"], CLERK)
            command = [sys.executable, "-B", source / "cafe_fixture.py",
                       lab, source / "CafePickupSmoke.swift.template", "nearby"]
            result = subprocess.run(command, check=False, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("unexpected café enrollment variant", result.stderr)
            self.assertFalse((lab / "SceneSmoke.swift").exists())
            nearby = dict(profile, variant="nearby", publicSale=CUSTOMER, publicEvent=PREEXISTING, publicNumber=7)
            staff.write_text(json.dumps(nearby))
            with self.assertRaisesRegex(ValueError, "unexpected café enrollment variant"):
                read_pair(lab, "nearby")
            buyer.write_text(json.dumps(dict(nearby, sale=SIBLING)))
            with self.assertRaisesRegex(ValueError, "different originals"):
                read_pair(lab, "nearby")
            buyer.write_text(json.dumps(nearby))
            result = subprocess.run(command, check=True, capture_output=True, text=True)
            self.assertEqual(tuple(result.stdout.splitlines()), TESTS["nearby"])
            self.assertIn('private let publicNumber = "7"', (lab / "SceneSmoke.swift").read_text())

    def test_success_requires_expected_xctest_name_once(self):
        def passed(case):
            return f"Test Case '-[SceneSmoke.CafePickupInstalledApps {case}]' passed (1.000 seconds).\n"
        for public, nearby in zip(TESTS["unknown"], TESTS["nearby"]):
            self.assertFalse(rendered_test_passed(passed(public), nearby))
            self.assertFalse(rendered_test_passed(passed(nearby) * 2, nearby))
            self.assertTrue(rendered_test_passed(passed(nearby), nearby))


if __name__ == "__main__":
    unittest.main()
