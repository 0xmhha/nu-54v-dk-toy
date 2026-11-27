"""Run the K05 clerk and customer fixture with private builds and simulators."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time

root = Path(__file__).resolve().parents[4]
sys.path[:0] = [str(root / "scripts"), str(root / "products/p10-platform/database")]
from integration_workspace import integration_workspace, OwnedProcess, run_command, start_postgres, _defer_interruptions, _ignore_interruptions
import migrate
from cafe_fixture import TESTS, read_pair, rendered_test_passed, staged_app_inputs
from cafe_simulators import SIMULATOR_ID, recover_owned_allocations, retire_owned_simulators, simulator_inventory

cli = argparse.ArgumentParser(description=__doc__)
cli.add_argument("--nearby", action="store_true", help="exercise a separate nearby order in both installed apps")
nearby = cli.parse_args().nearby
variant = "nearby" if nearby else "unknown"
go_test = "TestLoopbackK05NearbyCafeC2" if nearby else "TestLoopbackCustomerOrderReceipt"
pg_override = os.environ.get("POSTGRES_BIN")
pg_ctl = str(Path(pg_override) / "pg_ctl") if pg_override else shutil.which("pg_ctl")
if not pg_ctl or not Path(pg_ctl).is_file():
    raise SystemExit("Supply PostgreSQL tools on PATH or set POSTGRES_BIN.")
pg = Path(pg_ctl).resolve().parent
env = {**os.environ, "PATH": str(pg) + os.pathsep + os.environ["PATH"],
       "PYTHONDONTWRITEBYTECODE": "1", "LC_ALL": "C"}
runtime = os.environ.get("NU54_K05_SIM_RUNTIME", "com.apple.CoreSimulator.SimRuntime.iOS-27-0")
device_type = os.environ.get("NU54_K05_SIM_DEVICE", "com.apple.CoreSimulator.SimDeviceType.iPhone-17-Pro")
started = time.monotonic()
phase = "prepare"
simulators = []
report = {}
scratch_path = None
staged_inputs = None
scope = [root / ("products/" + product + "/ios/build/generated")
         for product in ("p02-user-app", "p04-merchant-kiosk")]

def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def source_fingerprints():
    return {str(file.relative_to(root)): digest(file) for base in scope for file in base.rglob("*") if file.is_file()}

def simctl(*args, check=False):
    return run_command(["xcrun", "simctl", *args], env=env, check=check, text=True, capture_output=True, timeout=20)


remaining_simulators = []
uncertain_labels = []
unconfirmed_allocations = {}
original_fingerprints = source_fingerprints()
result = 1
ledger = OwnedProcess()
ui_runner = OwnedProcess()
try:
    with integration_workspace("nu54-k05-native-", str(pg / "pg_ctl"), "db", env) as (work, runenv):
        scratch_path = work
        lab = work / "lab"
        lab.mkdir(mode=0o700)
        rawlogs = work / "logs"
        rawlogs.mkdir(mode=0o700)
        def run(args, name, timeout=300):
            with (rawlogs / (name + ".log")).open("w") as log:
                return run_command(args, cwd=root, env=runenv, stdout=log, stderr=log, check=True, timeout=timeout)
        try:
            apps = []
            phase = "signed-app-builds"
            staged = work / "source"
            runenv["NU54_PICKUP_READONLY_ROOT"] = str(root)
            run(["bash", "-c", """
set -euo pipefail
source "$1/scripts/ios/test-environment.sh"
ios_test_prepare_environment "$3"
source "$1/scripts/ios/stage-project.sh"
scene_stage_ios_source "$1" "$2"
scene_stage_ios_project "$1" "$2" "$3" "$3/logs" p04-merchant-kiosk NU54KioskSpec
scene_stage_ios_project "$1" "$2" "$3" "$3/logs" p02-user-app NU54RenterSpec
""", "stage-projects", root, staged, work], "clones")
            staged_inputs = staged_app_inputs(staged)
            report["stagedAppInputs"] = staged_inputs
            projects = (
                ("p04-merchant-kiosk", "NU54Kiosk", "com.nu54kiosk.cafepickupqa"),
                ("p02-user-app", "NU54Renter", "com.nu54renter.cafepickupqa"),
            )
            for product, app, bundle in projects:
                derived = work / app
                app_source = staged / "products" / product
                for directory in ("ios", "ios/Pods", "ios/build"):
                    path = app_source / directory
                    if path.is_symlink() or not path.is_dir() or not path.resolve().is_relative_to(staged.resolve()):
                        raise ValueError("Xcode project contains a non-private build or Pods path")
                run(["xcodebuild", "-workspace", app_source / "ios" / (app + ".xcworkspace"),
                     "BUNDLE_CONFIG=" + str(app_source / "metro.config.js"), "-scheme", app,
                     "-configuration", "Release", "-destination", "generic/platform=iOS Simulator",
                     "-derivedDataPath", derived, "-jobs", "2", "ARCHS=arm64", "NU54_IOS_BUNDLE=" + bundle,
                     "CODE_SIGNING_ALLOWED=YES", "CODE_SIGN_IDENTITY=-", "build"], app, timeout=900)
                path = derived / "Build/Products/Release-iphonesimulator" / (app + ".app")
                run(["codesign", "--verify", "--strict", path], app + "-signature")
                report.setdefault("signedAppSHA256", {})[app] = [digest(path / app), digest(path / "main.jsbundle")]
                apps.append(path)
            phase = "simulators"
            original_simulators = set(simulator_inventory(simctl))
            for role in ("clerk", "customer"):
                with _defer_interruptions():
                    label = "NU54 K05 " + role + " " + work.name
                    unconfirmed_allocations[label] = original_simulators.copy()
                    cmd = run_command(["xcrun", "simctl", "create", label, device_type, runtime], cwd=root,
                                      env=runenv, text=True, capture_output=True, check=True, timeout=45)
                    simulator = cmd.stdout.strip()
                    if not SIMULATOR_ID.fullmatch(simulator) or simulator.lower() in original_simulators:
                        raise ValueError("simulator allocation did not publish a new ID")
                    original_simulators.add(simulator.lower())
                    simulators.append(simulator)
                    del unconfirmed_allocations[label]
                run(["xcrun", "simctl", "boot", simulator], role + "-boot")
            for role, simulator in zip(("clerk", "customer"), simulators):
                run(["xcrun", "simctl", "bootstatus", simulator, "-b"], role + "-boot-status")
            staff, customer = simulators
            phase = "registration"
            harness = "products/p05-operations-backoffice/test/pickup-native/run-cafe-ios.sh"
            run(["bash", harness, "prepare", lab, staff, customer, *apps], "registrations")
            phase = "database"
            sock = work / "s"
            sock.mkdir(mode=0o700)
            runenv.update(PGHOST=str(sock), PGUSER="qa", PGPORT="5432", PGSSLMODE="disable")
            run([pg / "initdb", "-D", work / "db", "-A", "trust", "--no-locale", "--encoding=UTF8", "-U", "qa"], "init")
            start_postgres(pg / "pg_ctl", work / "db", work / "pg.log",
                           f"-c listen_addresses='' -c unix_socket_directories={sock} -c unix_socket_permissions=0700",
                           runenv, cwd=work, stdout=subprocess.DEVNULL)
            db = "nu54_settlement_k05_native"
            run([pg / "createdb", db], "createdb")
            for owner in ("p05", "p07"):
                local = {**runenv, f"{owner.upper()}_MIGRATION_DATABASE_URL": db}
                with (rawlogs / f"migrate-{owner}.log").open("w") as log:
                    run_command(["python3", "-B", "products/p10-platform/database/migrate.py", owner],
                                cwd=root, env=local, stdout=log, stderr=log, check=True)
            runenv.update(P05_SETTLEMENT_TEST_DATABASE_URL=f"host={sock} dbname={db} user=qa sslmode=disable",
                          NU54_CAFE_NATIVE_LAB=str(lab), NU54_K05_NATIVE_VARIANT=variant)
            phase = "settlement-and-installed-apps"
            with (lab / "ledger.log").open("w") as ledgerlog:
                worker = ledger.start(["bash", "products/p05-operations-backoffice/test/settlement/anvil.sh", "-v",
                                       "-run", f"^{go_test}$"], cwd=root, env=runenv, stdout=ledgerlog, stderr=ledgerlog)
                deadline = time.monotonic() + 180
                while not all((lab / (role + ".json")).is_file() for role in ("staff", "customer")):
                    if worker.poll() is not None or time.monotonic() >= deadline:
                        raise ValueError("Go verifier did not publish enrolled customer and staff scope")
                    time.sleep(.25)
                phase = "paired-fixture-scope"
                read_pair(lab, variant)
                phase = "settlement-and-installed-apps"
                with (rawlogs / "verification.log").open("w") as verification:
                    ui = ui_runner.start(["bash", harness, "verify", lab, staff, customer, *apps], cwd=root, env=runenv,
                                         stdout=verification, stderr=verification)
                    deadline = time.monotonic() + (330 if nearby else 260)
                    while True:
                        report["uiProcessExit"], report["ledgerProcessExit"] = ui.poll(), worker.poll()
                        if (ui.returncode or 0) != 0 or (worker.returncode or 0) != 0:
                            raise ValueError("one installed verifier stopped before both completed")
                        if ui.returncode == worker.returncode == 0:
                            break
                        if time.monotonic() >= deadline:
                            raise TimeoutError("both installed verifiers did not finish")
                        time.sleep(.25)
            for role, method in zip(("staff", "customer"), TESTS[variant]):
                output = run_command(["xcrun", "xcresulttool", "get", "test-results", "summary", "--path", lab / role / "ui.xcresult"],
                                     cwd=root, env=runenv, check=True, text=True, capture_output=True)
                data = json.loads(output.stdout)
                report[role] = {k: data[k] for k in ("totalTestCount", "passedTests", "failedTests", "skippedTests")}
                if report[role] != {"totalTestCount": 1, "passedTests": 1, "failedTests": 0, "skippedTests": 0}:
                    raise ValueError("installed test source did not run and pass exactly once")
                if not rendered_test_passed((lab / role / "ui.log").read_text(errors="replace"), method):
                    raise ValueError("the expected installed test name did not pass")
                report[role]["test"] = method
            go = (lab / "ledger.log").read_text(errors="replace")
            lifecycle = ("paired signed P04 and P02 reopened the same ready nearby original" if nearby
                         else "installed P04 staff moved the acknowledged original")
            go_case = "KioskNearbyPickupSeparateOrder" if nearby else "KioskCafeAfterFulfillment"
            report["originalLifecycleVerified"] = lifecycle in go and "one payment and consumer effect" in go
            cases = [go_case]
            if nearby:
                cases.append("KioskNearbyPaidSeparateOriginal")
            report["goCasePassed"] = all(
                re.search(rf"^ +--- PASS: {go_test}/{name} \(", go, re.MULTILINE)
                for name in cases
            )
            if not report["originalLifecycleVerified"] or not report["goCasePassed"] or ui.returncode or worker.returncode:
                raise ValueError("K05 original scope did not complete")
            phase = "complete"
            result = 0
        finally:
            with _ignore_interruptions():
                try:
                    ui_runner.stop()
                finally:
                    try:
                        ledger.stop()
                    finally:
                        recovered, uncertain_labels = recover_owned_allocations(simctl, unconfirmed_allocations)
                        remaining_simulators = retire_owned_simulators(simctl, [*simulators, *recovered])
                        if staged_inputs is not None:
                            try:
                                report["stagedAppInputsUnchanged"] = staged_app_inputs(staged) == staged_inputs
                            except (OSError, ValueError, KeyError):
                                report["stagedAppInputsUnchanged"] = False
                for role in ("staff", "customer"):
                    log = lab / role / "ui.log"
                    if log.is_file():
                        found = re.findall(r"SceneSmoke[.]swift:(\d+): error:", log.read_text(errors="replace"))
                        if found:
                            report[role + "AssertionLines"] = list(map(int, found[:12]))
                log = lab / "ledger.log"
                if log.is_file():
                    safe_log = log.read_text(errors="replace")
                    report["goFixedDiagnostics"] = re.findall(r"anvil_test[.]go:\d+: (?:original relay missing with quote TTL -?[0-9msh.]+: no rows in result set|no rows in result set|installed cafe fixture reported failure|installed cafe fixture never reached [a-z-]+)", safe_log)
                    report["goResultMarkers"] = [line.strip() for line in safe_log.splitlines()
                                                 if line.lstrip().startswith((f"--- PASS: {go_test}", f"--- FAIL: {go_test}"))]
except subprocess.CalledProcessError as failure:
    result = failure.returncode
    report["failure"] = "external command"
except SystemExit as failure:
    if isinstance(failure.code, int):
        result = failure.code
    report["failure"] = "interrupted"
except Exception as failure:
    report["failure"] = type(failure).__name__
    result = result or 1
finally:
    report.update(phase=phase, seconds=round(time.monotonic() - started, 3), ownedSimulatorsRemoved=not remaining_simulators and not uncertain_labels,
                  uncertainOwnedSimulatorLabels=uncertain_labels,
                  remainingOwnedSimulators=remaining_simulators,
                  sharedCodegenUnchanged=source_fingerprints() == original_fingerprints,
                  sharedCodegenFiles=len(original_fingerprints), scratchRemoved=scratch_path is not None and not scratch_path.exists())
    if not result and not all(report.get(k) for k in (
        "ownedSimulatorsRemoved", "sharedCodegenUnchanged", "scratchRemoved", "stagedAppInputsUnchanged")):
        result = 1
    report["exit"] = result
    print(json.dumps(report, sort_keys=True))
sys.exit(result)
