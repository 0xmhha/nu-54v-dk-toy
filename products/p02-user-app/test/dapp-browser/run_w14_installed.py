#!/usr/bin/env python3
"""Run and publish one isolated W14 installed-iOS acceptance journey."""
from contextlib import contextmanager
import errno
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time
import uuid

HERE = Path(__file__).resolve().parent
APP = HERE.parents[1]
ROOT = APP.parents[1]
PORT = 18483
PORT_LEASE = Path("/private/tmp/nu54-w14-port-18483.lock")
SELECTOR = "SceneSmoke/WalletConnectNativeJourney/testInstalledWalletGrantsAndRestoresBoundedAllowance"
EXPECTED = {"reference": "2.21.8", "boardApprovals": 1, "submissions": 1,
            "samePeerHash": True, "connectionViewRetiredReview": True, "finalizedAllowance": 5000000}
W15_TEST = "testInstalledWalletManagesTwoApprovedSessionsWithoutChangingAllowance"
W15_EXPECTED = {"reference": "2.21.8", "authorizedSessions": 2, "selectedClosed": True,
                "unselectedRemainedAuthorized": True, "allClosed": True, "signingAttempts": 0,
                "submissions": 0, "finalizedAllowance": 5000000}
W13_TEST = "testInstalledWalletRequiresFreshApprovalAfterExpiryAndRestart"
W13_EXPECTED = {"reference": "2.21.8", "expiredSessionDenied": True, "freshScopeApproved": True,
                "freshResumedAfterRestart": True, "signingAttempts": 0, "submissions": 0}
JOURNEYS = {
    "w14-browser-login": ("testInstalledBrowserReturnsOriginalLoginAndRejectsReplay", "--native-browser-login",
                          "Native W14 browser login fixture ready",
                          {"nonce":"8abc0023","accountVerified":True,"accountScopeUnchanged":True,"replayRefused":True,
                           "boardApprovals":1,"submissions":0,"onChainApprovals":0}),
    "w14-login": ("testInstalledWalletReviewsAndReturnsLogin", "--native-login", "Native W14 login fixture ready",
                  {"nonce": "8abc0023", "accountVerified": True, "replayRefused": True, "boardApprovals": 1,
                   "submissions": 0, "onChainApprovals": 0}),
    "w13-start": ("testInstalledExploreReviewsBrowserAndExternalConnection", "--native-explore",
                  "Native W13 explore fixture ready",
                  {"reference": "2.21.8", "externalApproved": True, "signingAttempts": 0, "submissions": 0}),
    "w13": (W13_TEST, "--native-expiry", "Native W13 expiry fixture ready", W13_EXPECTED),
    "w14": (SELECTOR.rsplit("/", 1)[-1], "--native-positive-allowance", "Native W14 allowance fixture ready", EXPECTED),
    "w15": (W15_TEST, "--native-session-management", "Native W15 management fixture ready", W15_EXPECTED),
}
OFFLINE_LOGIN_JOURNEYS = {"w14-login","w14-browser-login"}
BINDINGS = ("NU54RenterSpec/NU54RenterSpec.h", "NU54RenterSpec/NU54RenterSpec-generated.mm",
            "NU54RenterSpecJSI.h")


def unique_keys(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("Duplicate fixture JSON key")
        result[key] = value
    return result


def parse_fixture(log, expected_result=EXPECTED):
    records = [json.loads(line, object_pairs_hook=unique_keys)
               for line in log.splitlines() if line.lstrip().startswith("{")]
    if len(records) != 1:
        raise ValueError("Expected one W14 fixture result")
    record = records[0]
    if type(record) is not dict or record.keys() != expected_result.keys() or any(
            type(record[key]) is not type(expected) or record[key] != expected
            for key, expected in expected_result.items()):
        raise ValueError("Invalid W14 fixture result")
    return json.dumps(record, sort_keys=True) + "\n"


def publish(evidence, run_id, contents, journey="w14"):
    evidence.mkdir(parents=True, exist_ok=True)
    destination = evidence / f"{journey}-{run_id}"
    with tempfile.TemporaryDirectory(prefix=".w14-stage-", dir=evidence) as storage:
        stage = Path(storage)
        staged_run = stage / "receipt"
        staged_run.mkdir()
        for filename, value in contents.items():
            (staged_run / filename).write_text(value, encoding="utf-8")
        # Immutable per-run receipts avoid overwrites even across checkouts.
        # A failed validation never reaches publication.
        staged_run.rename(destination)
    return destination


def acquire_port(path=PORT_LEASE, port=PORT):
    # Keep the coordination inode. Unlinking it permits two independent locks.
    fd = os.open(path, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    try:
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        with socket.socket() as probe:
            probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            probe.bind(("127.0.0.1", port))
    except OSError:
        os.close(fd)
        raise RuntimeError(f"W14 loopback port {port} is busy or its host lease is unavailable") from None
    return fd


def sha(data):
    return hashlib.sha256(data).hexdigest()


class Runner:
    def __init__(self, scratch, run_id, journey="w14"):
        self.scratch = scratch
        self.run_id = run_id
        self.journey = journey
        self.sim_name = f"NU54 {journey.upper()} installed QA {run_id}"
        self.sim = None
        self.fixture = None
        self.interrupted = False
        self.creation_attempted = False
        self.generated = APP / "ios/build/generated/ios/ReactCodegen"
        self.backups = {}
        self.env = dict(os.environ, TMPDIR=str(scratch / "temp"), UV_CACHE_DIR=str(scratch / "uv-cache"),
                        PYTHONPYCACHEPREFIX=str(scratch / "pycache"), NU54_W15_QA="1",
                        PATH=f"{Path.home()}/.foundry/bin:/opt/homebrew/bin:{os.environ['PATH']}")
        (scratch / "temp").mkdir()

    def interrupt(self, *_):
        # Do not raise inside lock, Popen or simctl ownership transitions.
        self.interrupted = True

    def check_interrupt(self):
        if self.interrupted:
            raise InterruptedError("W14 acceptance interrupted")

    @staticmethod
    def stop(proc, graceful=False):
        if graceful and proc.poll() is None:
            proc.terminate()
            try:
                proc.wait(timeout=12)
            except subprocess.TimeoutExpired:
                pass
        try:
            os.killpg(proc.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            pass
        # On interrupt Xcode's leader may exit before its workers.
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        proc.wait()

    def command(self, args, log=None, owner_transition=False, cleaning=False, capture=False):
        if not cleaning:
            self.check_interrupt()
        output = None
        try:
            if log:
                output = (self.scratch / log).open("wb")
            child = subprocess.Popen(args, cwd=ROOT, env=self.env, start_new_session=True,
                                     stdout=subprocess.PIPE if capture else output,
                                     stderr=subprocess.PIPE if capture else output)
            max_wait = time.monotonic() + 30 if owner_transition or cleaning else None
            while True:
                try:
                    stdout, stderr = child.communicate(timeout=.25)
                    break
                except subprocess.TimeoutExpired:
                    if self.interrupted and not (cleaning or owner_transition):
                        self.stop(child)
                        self.check_interrupt()
                    if max_wait is not None and time.monotonic() >= max_wait:
                        self.stop(child)
                        raise RuntimeError(f"Timed out running {args[0]} {' '.join(args[1:3])}") from None
            if child.returncode:
                if log:
                    print(f"Failed stage log: {log}", file=sys.stderr)
                    for line in (self.scratch / log).read_text(errors="replace").splitlines()[-30:]:
                        print(line, file=sys.stderr)
                raise RuntimeError(f"{args[0]} {' '.join(args[1:3])} exited {child.returncode}")
            return stdout.decode() if capture else None
        finally:
            if output:
                output.close()

    def create_simulator(self):
        self.creation_attempted = True
        self.sim = self.command(["xcrun", "simctl", "create", self.sim_name,
                                 self.env.get("W14_SIM_DEVICE", "com.apple.CoreSimulator.SimDeviceType.iPhone-17-Pro"),
                                 self.env.get("W14_SIM_RUNTIME", "com.apple.CoreSimulator.SimRuntime.iOS-27-0")],
                                capture=True, owner_transition=True).strip()
        if self.sim != str(uuid.UUID(self.sim)).upper():
            raise RuntimeError("Expected exact simulator identifier")
        self.check_interrupt()

    def owned_simulators(self):
        report = json.loads(self.command(["xcrun", "simctl", "list", "devices", "-j"], cleaning=True, capture=True))
        return {device["udid"]: device["state"] for group in report["devices"].values()
                for device in group if device["name"] == self.sim_name}

    def delete_simulator(self):
        if not self.creation_attempted:
            return
        for sim, state in self.owned_simulators().items():
            if state not in ("Shutdown", "Shutting Down"):
                try:
                    self.command(["xcrun", "simctl", "shutdown", sim], cleaning=True, capture=True)
                except RuntimeError as error:
                    print(f"Simulator shutdown reported {error}; attempting deletion", file=sys.stderr)
            self.command(["xcrun", "simctl", "delete", sim], cleaning=True, capture=True)
        if self.owned_simulators():
            raise RuntimeError("Owned W14 simulator still present after cleanup")
        print("Owned W14 simulator absent", flush=True)

    def restore_bindings(self):
        for filename, saved in self.backups.items():
            destination = self.generated / filename
            shutil.copy2(saved, destination)
            if sha(saved.read_bytes()) != sha(destination.read_bytes()):
                raise RuntimeError("iOS binding did not restore byte-for-byte")
        if self.backups:
            print("Original iOS bindings restored byte-for-byte", flush=True)
            self.backups.clear()

    def clean(self):
        failures = []
        for action in (self.stop_fixture, self.restore_bindings, self.delete_simulator):
            try:
                action()
            except Exception as error:
                failures.append(f"{action.__name__}: {error}")
        if failures:
            raise RuntimeError("; ".join(failures))

    def stop_fixture(self):
        if self.fixture is not None:
            self.stop(self.fixture, graceful=True)
            # Allow the kernel to drop the listening descriptor before testing.
            for _ in range(20):
                with socket.socket() as probe:
                    probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
                    try:
                        probe.bind(("127.0.0.1", PORT))
                        print("W14 loopback listener absent", flush=True)
                        return
                    except OSError:
                        time.sleep(.1)
            raise RuntimeError(f"Loopback port {PORT} remains in use")

    def start_fixture(self, python):
        self.check_interrupt()
        fixture_path = self.scratch / "fixture.log"
        with fixture_path.open("wb") as log:
            self.fixture = subprocess.Popen(
                [python, "-u", HERE / "wallet_connect_reference.py", JOURNEYS[self.journey][1],
                 self.scratch / "cert.pem", self.scratch / "cert.key", "--gateway", self.scratch / "cmake/test_device_evm"],
                cwd=ROOT, env=self.env, stdout=log, stderr=log, start_new_session=True)
        self.check_interrupt()
        for _ in range(360):
            self.check_interrupt()
            if JOURNEYS[self.journey][2] in fixture_path.read_text(errors="replace"):
                print("W14 loopback fixture ready", flush=True)
                return
            if self.fixture.poll() is not None:
                print(fixture_path.read_text(errors="replace").splitlines()[-12:], file=sys.stderr)
                raise RuntimeError("W14 loopback fixture exited before readiness")
            time.sleep(.25)
        raise RuntimeError("Timed out waiting for W14 loopback fixture")

    def build(self, python):
        scratch = self.scratch
        self.command(["xcrun", "simctl", "boot", self.sim])
        self.command(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
                      "-subj", "/CN=localhost", "-addext", "subjectAltName=IP:127.0.0.1", "-keyout",
                      scratch / "cert.key", "-out", scratch / "cert.pem"], log="cert.log")
        if self.journey in OFFLINE_LOGIN_JOURNEYS:
            self.command([python, "-c", "import cryptography, websockets, Crypto"])
        else:
            self.command(["uv", "venv", "--python", sys.executable, scratch / "venv"])
            wheel = self.env.get("W14_PYCRYPTODOME_WHEEL", "pycryptodome")
            self.command(["uv", "pip", "install", "--python", python, "cryptography", "websockets", wheel])
        self.command(["cmake", "-S", "products/p01-device-firmware/test", "-B", scratch / "cmake"], log="cmake.log")
        self.command(["cmake", "--build", scratch / "cmake", "--target", "test_device_evm", "-j4"], log="c-build.log")
        app_source = self.private_app_source() if self.journey in OFFLINE_LOGIN_JOURNEYS else APP
        if self.journey not in OFFLINE_LOGIN_JOURNEYS:
            self.refresh_bindings()
        self.command(["xcodebuild", "-workspace", app_source / "ios/NU54Renter.xcworkspace", "-scheme", "NU54Renter",
                      "-configuration", "Release", "-sdk", "iphonesimulator", "-destination", f"id={self.sim}",
                      "-derivedDataPath", scratch / "app", "CODE_SIGN_IDENTITY=-",
                      "PRODUCT_BUNDLE_IDENTIFIER=com.nu54renter.wallet.w15qa",
                      f"BUNDLE_CONFIG={app_source / 'test/dapp-browser/wallet-native.metro.config.js'}", "build"], log="ios-build.log")
        self.restore_bindings()
        app = scratch / "app/Build/Products/Release-iphonesimulator/NU54Renter.app"
        self.command(["codesign", "--verify", "--strict", app])
        self.command(["xcrun", "simctl", "bootstatus", self.sim, "-b"])
        self.command(["xcrun", "simctl", "keychain", self.sim, "add-root-cert", scratch / "cert.pem"])
        self.command(["xcrun", "simctl", "install", self.sim, app])
        return app

    def private_app_source(self):
        mirror = self.scratch / "source"
        app = mirror / "products" / APP.name
        app.mkdir(parents=True)
        for name in ("node_modules", "packages", "docs", "products/p01-device-firmware"):
            (mirror / name).symlink_to(ROOT / name, target_is_directory=True)
        for source in APP.iterdir():
            if source.name in ("android", "node_modules") or source.name.startswith((".git", ".env")):
                continue
            if source.is_dir():
                self.command(["/bin/cp", "-cR", source, app / source.name])
            elif source.is_file():
                shutil.copy2(source, app / source.name)
        (app / "node_modules").symlink_to(APP / "node_modules", target_is_directory=True)
        with (app / "metro.config.js").open("a") as config:
            config.write(f"\nmodule.exports.watchFolders.push({str(ROOT)!r});\n")
        for folder in (app / "ios/Pods", app / "ios/build"):
            if not folder.resolve().is_relative_to(app):
                raise RuntimeError("iOS build folder escapes disposable source")
            for directory, dirs, files in os.walk(folder):
                for name in dirs + files:
                    candidate = Path(directory) / name
                    if candidate.is_symlink() and any(candidate.resolve().is_relative_to(APP / original)
                        for original in ("ios/Pods", "ios/build")):
                        raise RuntimeError("iOS write path resolves to the working checkout")
        cache = self.scratch / "xcode-cache"
        config = self.scratch / "ScopedCaches.xcconfig"
        config.write_text(f"SDK_STAT_CACHE_DIR = {cache}/sdk\nCLANG_MODULE_CACHE_PATH = {cache}/clang\n")
        self.env.update(XCODE_XCCONFIG_FILE=str(config), NODE_COMPILE_CACHE=str(self.scratch / "node-cache"))
        return app

    def refresh_bindings(self):
        scratch = self.scratch
        self.command(["node", "node_modules/react-native/scripts/generate-codegen-artifacts.js",
                      "-p", APP, "-o", scratch / "codegen", "-t", "ios"], log="codegen.log")
        backup = scratch / "backup"
        backup.mkdir()
        fresh = scratch / "codegen/build/generated/ios/ReactCodegen"
        for filename in BINDINGS:
            saved = backup / Path(filename).name
            shutil.copy2(self.generated / filename, saved)
            self.backups[filename] = saved
            shutil.copy2(fresh / filename, self.generated / filename)

    def run(self):
        self.create_simulator()
        print(f"Disposable simulator {self.sim}; live logs {self.scratch}", flush=True)
        python = sys.executable if self.journey in OFFLINE_LOGIN_JOURNEYS else self.scratch / "venv/bin/python"
        source_manifest = fingerprint()
        app = self.build(python)
        self.start_fixture(python)
        test_name, _, _, expected = JOURNEYS[self.journey]
        selector = SELECTOR.rsplit("/", 1)[0] + "/" + test_name
        self.command(["xcodebuild", "test", "-project", HERE / "reference/SceneSmoke.xcodeproj",
                      "-scheme", "SceneSmoke", "-destination", f"id={self.sim}",
                      "-derivedDataPath", self.scratch / "ui", "-resultBundlePath",
                      self.scratch / "w14.xcresult", "-parallel-testing-enabled", "NO",
                      f"-only-testing:{selector}"], log="xctest.log")
        raw_test = (self.scratch / "xctest.log").read_text()
        result = re.findall(r"^Test Case '[^\n]*" + re.escape(test_name) + r"\]' passed \([\d.]+ seconds\)\.$",
                            raw_test, flags=re.MULTILINE)
        if len(result) != 1 or "** TEST SUCCEEDED **" not in raw_test:
            raise RuntimeError("Missing W14 XCTest pass")
        fixture = parse_fixture((self.scratch / "fixture.log").read_text(), expected)
        if fingerprint() != source_manifest:
            raise RuntimeError("W14 source changed during acceptance")
        # codesign writes its description to stderr.
        sig = subprocess.check_output(["codesign", "--display", "--verbose=4", app], stderr=subprocess.STDOUT, text=True)
        return {"xctest.txt": result[0] + "\n** TEST SUCCEEDED **\n",
                "relay-anvil.json": fixture,
                "ios-signature.txt": "\n".join(line for line in sig.splitlines()
                                             if line.startswith(("Identifier=", "CDHash=", "Signature="))) + "\n",
                "ios-js-sha256.txt": sha((app / "main.jsbundle").read_bytes()) + "\n",
                "source-sha256.txt": source_manifest,
                "source-manifest-sha256.txt": sha(source_manifest.encode()) + "\n"}


def fingerprint():
    paths = ["products/p01-device-firmware/" + path for path in (
        "app/src/device_evm.c", "app/src/device_evm.h", "scripts/evm_service.py", "scripts/evm_transfer.py",
        "test/test_device_evm.c", "test/test_revoke_anvil.py", "test/test_evm_service.py")]
    paths += ["products/p02-user-app/" + path for path in (
        "src/App.tsx", "ios/NU54Renter/NearbyBle.mm", "src/merchant/CustomerOrders.tsx", "test/w04W06OwnerUi.test.tsx",
        "src/specs/NativeDappBrowser.ts", "src/wallet/WalletHome.tsx", "src/wallet/evmTransfer.ts",
        "src/wallet/walletEnglish.ts", "src/wallet/profile.ts", "src/wallet/evmRegistry.ts", "src/wallet/dapp/DappBrowserScreen.tsx",
        "src/wallet/dapp/WalletConnectScreen.tsx", "src/wallet/dapp/walletConnectSession.ts", "src/wallet/dapp/loginWire.ts",
        "ios/NU54Renter/DappBrowser.mm", "ios/NU54Renter/OwnerWallet.mm",
        "android/app/src/main/java/com/nu54renter/ble/DappBrowserModule.kt",
        "test/w14AllowanceTracking.test.ts", "test/w15AllowanceUi.test.tsx", "test/w26BrowserRefusalUi.test.tsx",
        "test/walletConnectPolicy.test.ts", "test/walletConnectUi.test.tsx")]
    paths += [str((HERE / name).relative_to(ROOT)) for name in (
        "WalletConnectNative.swift", "walletNativeOwner.js", "wallet_connect_reference.py", "browser-siwe.html",
        "wallet_connect_spend.py", "reference/peer.cjs", "reference/package-lock.json",
        "run_w14_installed.sh", "run_w14_installed.py")]
    return "".join(f"{sha((ROOT / item).read_bytes())}  {item}\n" for item in sorted(paths))


@contextmanager
def private_scratch():
    directory = Path(tempfile.mkdtemp(prefix="nu54-w14-view-installed-", dir="/private/tmp"))
    try:
        yield directory
    finally:
        for attempt in range(20):
            try:
                shutil.rmtree(directory)
                break
            except FileNotFoundError:
                if not directory.exists():
                    break
                if attempt==19:
                    raise
            except OSError as error:
                if error.errno!=errno.ENOTEMPTY or attempt==19:
                    raise
            time.sleep(.1)


def main(evidence, journey="w14"):
    run_id = uuid.uuid4().hex
    runner = None
    interrupted = False

    def interrupt(*_):
        nonlocal interrupted
        interrupted = True
        if runner is not None:
            runner.interrupt()

    for kind in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
        signal.signal(kind, interrupt)
    with private_scratch() as directory:
        runner = Runner(directory, run_id, journey)
        runner.interrupted = interrupted
        runner.check_interrupt()
        lease = acquire_port()
        try:
            runner.check_interrupt()
            try:
                contents = runner.run()
            finally:
                try:
                    runner.clean()
                except RuntimeError:
                    if runner.backups:
                        restored = {path.replace("/", "-"): saved.read_text() for path, saved in runner.backups.items()}
                        print("Generated binding recovery:", publish(evidence, run_id + "-recovery", restored, journey),
                              file=sys.stderr)
                    raise
        finally:
            os.close(lease)
    runner.check_interrupt()
    print("W14 installed test scratch removed and host lease released", flush=True)
    print(contents["xctest.txt"], end="")
    print(contents["relay-anvil.json"], end="")
    print("Published acceptance receipt", publish(evidence, run_id, contents, journey), flush=True)


if __name__ == "__main__":
    journey = sys.argv[2] if len(sys.argv) == 3 else "w14"
    if len(sys.argv) not in (2, 3) or journey not in JOURNEYS:
        raise SystemExit(f"Usage: {sys.argv[0]} /absolute/path/to/evidence-directory [{ '|'.join(JOURNEYS) }]")
    try:
        main(Path(sys.argv[1]).resolve(), journey)
    except InterruptedError as error:
        print(error, file=sys.stderr)
        raise SystemExit(130) from None
    except (RuntimeError, ValueError, OSError) as error:
        print(f"W14/W15 runner: {error}", file=sys.stderr)
        raise SystemExit(1) from None
