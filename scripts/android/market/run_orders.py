"""Keep the market fixture and its signed Android build in private scratch space."""

import hashlib
from pathlib import Path
import os
import re
import secrets
import shlex
import subprocess
import sys
import time

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from test_release_workspace import PRODUCTS, ROOT, isolated_environment, mirror_product
from integration_workspace import (
    OwnedProcess, WorkspaceCleanupError, _defer_interruptions, cleanup_stack, run_command,
    run_environment, scoped_workspace,
)

HERE = Path(__file__).resolve().parent
MARKET = ROOT / "products/p08-market-services"
TUNNEL = "tcp:9443"
APK = Path("app/build/outputs/apk/custom/app-custom.apk")
INIT_SCRIPT = """gradle.beforeProject { project ->
    if (project.path != ':app' || project.rootProject.name != 'NU54Renter') return
    project.plugins.withId('com.android.application') {
        project.androidComponents.finalizeDsl { app ->
            app.buildTypes.getByName('custom').applicationIdSuffix = System.getenv('NU54_MARKET_FIXTURE_SUFFIX')
        }
    }
}
"""


class DeviceError(RuntimeError):
    """The test could not confirm its expected device state."""


class TestInstall:
    def __init__(self, serial, bundle, output):
        self.command = [str(Path(os.environ["ANDROID_HOME"]) / "platform-tools/adb"), "-s", serial]
        self.bundle = bundle
        self.output = output
        self.apk_digest = None
        self.install_pending = False
        self.tunnel = False
        self.host_port = None
        self.uncertain_tunnel = False

    def adb(self, *args):
        command = [*self.command, *args]
        try:
            return run_command(command, capture_output=True, text=True, timeout=45, check=True).stdout
        except (OSError, subprocess.SubprocessError) as error:
            raise DeviceError(f"adb {' '.join(map(str, args[:3]))} failed: {error}") from None

    def installed(self):
        packages = self.adb("shell", "pm", "list", "packages", self.bundle).splitlines()
        return f"package:{self.bundle}" in packages

    def reverse_targets(self):
        return [line.split()[-1] for line in self.adb("reverse", "--list").splitlines()
                if len(line.split()) >= 2 and line.split()[-2] == TUNNEL]

    def start(self, apk, host_port):
        if not 1 <= host_port <= 65535 or host_port == 9443:
            raise ValueError("Expected a private HTTPS host port for the market tunnel")
        self.host_port = f"tcp:{host_port}"
        if self.installed():
            raise DeviceError(f"Test package already exists on the emulator: {self.bundle}")
        if self.reverse_targets():
            raise DeviceError("The dedicated emulator already has a 9443 tunnel")

        # ADB can finish on the device after its client is interrupted. Keep the
        # parent alive through the bounded setup handshake and verify ownership on exit.
        with apk.open("rb") as source:
            self.apk_digest = hashlib.file_digest(source, "sha256").hexdigest()
        with _defer_interruptions():
            self.install_pending = True
            if "Success" not in self.adb("install", "--streaming", "-R", "--pkg", self.bundle, apk).splitlines():
                raise DeviceError("Could not confirm Android test APK installation")
            self.install_pending = False
        with _defer_interruptions():
            self.uncertain_tunnel = True
            self.adb("reverse", "--no-rebind", TUNNEL, self.host_port)
            self.tunnel = True
            self.uncertain_tunnel = False
        print(f"Installed {self.bundle} with SHA-256 {self.apk_digest}; ADB reverse {TUNNEL} → {self.host_port}",
              file=sys.stderr, flush=True)

    def remove_tunnel(self):
        if self.tunnel:
            targets = self.reverse_targets()
            if targets == [self.host_port]:
                self.adb("reverse", "--remove", TUNNEL)
                if self.reverse_targets():
                    raise DeviceError("Could not confirm removal of the market reverse tunnel")
            elif targets:
                raise DeviceError("The market reverse tunnel changed ownership")
        elif self.uncertain_tunnel and self.reverse_targets():
            raise DeviceError("Could not confirm ownership of the emulator's 9443 tunnel")

    def remove_package(self):
        if self.apk_digest is None:
            return
        if not self.installed():
            if self.install_pending:
                raise DeviceError(f"Could not confirm installation state for {self.bundle}")
            return
        paths = self.adb("shell", "pm", "path", self.bundle).splitlines()
        if len(paths) != 1 or not paths[0].startswith("package:"):
            raise DeviceError(f"Could not identify the installed test APK for {self.bundle}")
        filename = paths[0].removeprefix("package:")
        checksum = self.adb("shell", "sha256sum", shlex.quote(filename)).split()
        if not checksum or checksum[0] != self.apk_digest:
            raise DeviceError(f"The installed APK for {self.bundle} does not match this run")
        if self.adb("uninstall", self.bundle).strip() != "Success" or self.installed():
            raise DeviceError(f"Could not confirm removal of {self.bundle}")

    def cleanup(self):
        errors = []
        for remove in (self.remove_tunnel, self.remove_package):
            try:
                remove()
            except DeviceError as error:
                errors.append(str(error))
        if errors:
            report = f"{self.command[-1]} {self.bundle}\n" + "\n".join(errors)
            (self.output / "device-cleanup.log").write_text(report + "\n")
            raise DeviceError(report)
        if self.tunnel:
            print(f"Confirmed test package {self.bundle} and ADB reverse {TUNNEL} removed",
                  file=sys.stderr, flush=True)


def stop_gradle(work, android):
    # Gradle may still fork a detached, single-use daemon with --no-daemon.
    # Only this run's private registry is visible to the stop command.
    home = work / "gradle"
    daemon = home / "daemon"
    if not daemon.is_dir():
        return
    env = {**os.environ, "GRADLE_USER_HOME": str(home), "ANDROID_USER_HOME": str(work / "android-user")}
    env["JAVA_TOOL_OPTIONS"] = f'{env.get("JAVA_TOOL_OPTIONS", "")} -Djava.io.tmpdir="{work / "tmp"}"'.strip()
    try:
        run_command(["./gradlew", "--offline", "--stop"], cwd=android, env=env,
                    capture_output=True, timeout=15, check=True)
        deadline = time.monotonic() + 10
        while True:
            for log in daemon.glob("*/daemon-*.out.log"):
                pid = re.fullmatch(r"daemon-([1-9][0-9]*)\.out\.log", log.name)
                if pid:
                    try:
                        os.getpgid(int(pid[1]))
                    except ProcessLookupError:
                        continue
                    break
            else:
                return
            if time.monotonic() >= deadline:
                raise WorkspaceCleanupError("The market Gradle daemon did not stop")
            time.sleep(0.1)
    except (OSError, subprocess.SubprocessError) as error:
        raise WorkspaceCleanupError(f"Could not confirm market Gradle shutdown: {error}") from error
    finally:
        (work / "rehearsal.p12").unlink(missing_ok=True)


def build_custom_apk(work, android, bundle):
    env = isolated_environment(work, android, os.environ)
    password = secrets.token_hex(24)
    env.update(
        NU54_MARKET_FIXTURE_SUFFIX=bundle.removeprefix("com.nu54renter"),
        NU54_P02_CUSTOM_KEYSTORE=str(work / "rehearsal.p12"),
        NU54_P02_CUSTOM_KEY_ALIAS="market",
        NU54_P02_CUSTOM_STORE_PASSWORD=password,
        NU54_P02_CUSTOM_KEY_PASSWORD=password,
    )
    subprocess.run([
        Path(env["JAVA_HOME"]) / "bin/keytool", "-genkeypair", "-alias", "market",
        "-keyalg", "RSA", "-keysize", "2048", "-validity", "2",
        "-dname", "CN=NU54 Android market test", "-keystore", env["NU54_P02_CUSTOM_KEYSTORE"],
        "-storepass:env", "NU54_P02_CUSTOM_STORE_PASSWORD",
        "-keypass:env", "NU54_P02_CUSTOM_KEY_PASSWORD",
    ], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)

    init = work / "market.gradle"
    init.write_text(INIT_SCRIPT)
    with (work / "build.log").open("wb") as log:
        result = subprocess.run([
            "./gradlew", "--offline", "--no-daemon", "-Pkotlin.compiler.execution.strategy=in-process",
            "-I", init, "-PreactNativeArchitectures=arm64-v8a",
            f"-Pnu54MarketProfile={env['NU54_MOBILE_QA_PROFILE']}", ":app:assembleCustom",
        ], cwd=android, env=env, stdout=log, stderr=subprocess.STDOUT)
    if result.returncode:
        raise SystemExit("Android custom release build failed")



def build_and_drive(work, android, serial, bundle):
    build_custom_apk(work, android, bundle)
    (work / "apk-ready").touch()
    while not (work / "device-ready").exists():
        time.sleep(0.1)
    driver_env = {key: value for key, value in os.environ.items() if not key.startswith("NU54_P02_CUSTOM_")}
    os.execve(sys.executable, [
        sys.executable, "-B", str(HERE / "orders.py"), serial, bundle, os.environ["P08_PROFILE"],
    ], driver_env)


def run(serial, output):
    with scoped_workspace("nu54-android-market-") as work, cleanup_stack() as cleanup:
        env = run_environment(work, os.environ)
        env.setdefault("FOUNDRY_OUT", str(work / "forge-out"))
        env.setdefault("FOUNDRY_CACHE_PATH", str(work / "forge-cache"))
        if not env.get("P08_ARTIFACT") and env["FOUNDRY_OUT"] == str(work / "forge-out"):
            env["P08_ARTIFACT"] = str(work / "forge-out/Market.sol/Market.json")
        build = work / "android-build"
        build.mkdir()
        mirror = build / "source"
        project = PRODUCTS["p02"]
        mirror_product(ROOT, project, mirror)
        android = mirror / "products" / project / "android"
        bundle = f"com.nu54renter.marketqa.r{secrets.token_hex(6)}"

        cleanup.callback(stop_gradle, build, android)
        install = TestInstall(serial, bundle, output)
        cleanup.callback(install.cleanup)
        logger = OwnedProcess()
        cleanup.callback(logger.stop)
        fixture = OwnedProcess()
        cleanup.callback(fixture.stop)
        https_port = work / "https-port"
        home_only = env.get("NU54_ANDROID_MARKET_HOME_ONLY") == "1"
        private_scenario = env.get("NU54_ANDROID_MARKET_SCENARIO") in {"liquidity-pool", "position-review"}
        chain = work / "chain" if home_only or private_scenario else output / "chain"
        if home_only:
            env["NU54_ANDROID_MARKET_HOME_SCREEN"] = str(output / "learning-home.png")
        env["NU54_MOBILE_QA_PORT_FILE"] = str(https_port)
        log = logger.start(["tee", output / "result.log"], stdin=subprocess.PIPE)
        # The fixture must lead its owned group so TERM can finish stopping its
        # private worker before the outer launcher reclaims the build.
        with log.stdin:
            process = fixture.start([
                "bash", MARKET / "test/mobile-fixture.sh", chain,
                "bash", MARKET / "test/mobile-https-fixture.sh",
                sys.executable, "-B", __file__, "--build", build, android, serial, bundle,
            ], env=env, stdout=log.stdin)
        while process.poll() is None:
            if (build / "apk-ready").exists():
                install.start(android / APK, int(https_port.read_text()))
                (build / "device-ready").touch()
                break
            try:
                process.wait(timeout=0.1)
            except subprocess.TimeoutExpired:
                continue
        fixture_status = process.wait()
        return log.wait() or fixture_status


def main():
    try:
        if sys.argv[1] == "--build":
            _, _, work, android, serial, bundle = sys.argv
            build_and_drive(Path(work), Path(android), serial, bundle)
        else:
            status = run(sys.argv[1], Path(sys.argv[2]))
            raise SystemExit(status if status >= 0 else 128 - status)
    except subprocess.CalledProcessError as error:
        raise SystemExit(error.returncode if error.returncode >= 0 else 128 - error.returncode) from None
    except (DeviceError, OSError, ValueError) as error:
        raise SystemExit(str(error)) from None


if __name__ == "__main__":
    main()
