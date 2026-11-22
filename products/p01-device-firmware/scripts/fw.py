#!/usr/bin/env python3
"""Build, flash and reset the P01 firmware ([D05][N25]).

Runs west and pyOCD inside the NCS toolchain environment through nrfutil, so no
global PATH setup is needed. The vendored board package is passed as BOARD_ROOT
because sysbuild resolves the board before the application CMakeLists runs.

Usage:
    python3 -B scripts/fw.py build [--pristine] [--test-capability-cache] [--rental | --release [--approtect] [--security-counter=N] [--official]]
    python3 scripts/fw.py keygen PATH
    python3 scripts/fw.py reject-images OUT_DIR
    python3 scripts/fw.py flash
    python3 scripts/fw.py reset

--test-capability-cache uses a temporary Zephyr compiler capability cache. It requires
an explicit, fresh absolute P01_BUILD_DIR for test output. Use a separate tree for product builds.

--rental adds app/rental.conf (no week-7 fixed setup). --release builds MCUboot with serial
recovery and the signed release app (app/sysbuild-release.conf, app/release.conf); it needs
NU54_SIGNING_KEY. --approtect configures both images for debug lock (app/approtect.conf). Switching between
variants needs --pristine or a separate P01_BUILD_DIR.
Signed builds record the verified bootloader and application outputs in release-manifest.json.
--official requires --approtect, a monotonic security counter, a pinned signing key, and a clean source tree.

reject-images writes, from a release build, the two images MCUboot must refuse (W12-11): one
with no signature and one signed with a throwaway key that is deleted afterwards.

keygen makes an ed25519 signing key with MCUboot's imgtool. Keep it outside the repository and
offline; anyone with it can make images the board boots.

Environment:
    NCS_VERSION   default v3.4.2
    NCS_DIR       Nordic SDK workspace (defaults to /opt/nordic/ncs/$NCS_VERSION)
    NRFUTIL       default nrfutil on PATH, then ~/.local/bin/nrfutil
    P01_BUILD_DIR default products/p01-device-firmware/build
    NU54_SIGNING_KEY  release signing key (PEM), outside the repository
    NU54_MBEDTLS_ARCHIVE  optional offline copy of the pinned Mbed TLS release archive
    NU54_INSTALL_AUTHORITY_HEX  pinned P-256 public point for board enrollment
    NU54_RECOVERY_OVERLAY  public devicetree overlay mapping a dedicated accessory UART
    NU54_COMMISSIONED_WALLET_CHAINS=1  enable commissioned testnet signing on an enrolled custom image
    NU54_LAB_WALLET_CHAINS=1  enable the bounded multichain profiles on a custom laboratory image
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

import mbedtls_input
import mcuboot_input

HERE = Path(__file__).resolve().parent
PRODUCT = HERE.parent
APP = PRODUCT / "app"
BOARD = "nu54v_dk/nrf54l15/cpuapp"
PYOCD_TARGET = "nrf54l"
NCS_VERSION = os.environ.get("NCS_VERSION", "v3.4.2")
NCS_DIR = Path(os.environ.get("NCS_DIR", f"/opt/nordic/ncs/{NCS_VERSION}"))
REPO = PRODUCT.parents[1]
BUILD_DIR = Path(os.environ.get("P01_BUILD_DIR", PRODUCT / "build"))
RELEASE_POLICY = HERE / "release-policy.json"


def nrfutil() -> str:
    found = os.environ.get("NRFUTIL") or shutil.which("nrfutil")
    if found:
        return found
    fallback = Path.home() / ".local/bin/nrfutil"
    if fallback.exists():
        return str(fallback)
    sys.exit("nrfutil not found; install it and run `nrfutil install sdk-manager`")


def in_toolchain(*cmd: str, runner=None) -> int:
    """Run a command inside the NCS toolchain environment."""
    full = [nrfutil(), "sdk-manager", "toolchain", "launch", "--ncs-version", NCS_VERSION, "--", *cmd]
    return (runner or subprocess.run)(full, cwd=NCS_DIR).returncode


def outside_repo(path: Path) -> bool:
    return not path.resolve().is_relative_to(REPO)


def signing_key() -> Path:
    raw = os.environ.get("NU54_SIGNING_KEY")
    if not raw:
        sys.exit("--release needs NU54_SIGNING_KEY (a PEM made with `fw.py keygen`)")
    key = Path(raw).expanduser()
    if not key.is_file():
        sys.exit(f"signing key not found: {key}")
    if not outside_repo(key):
        sys.exit("the signing key must live outside the repository")
    return key.resolve()


def digest(path: Path) -> str:
    with path.open("rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def release_signer(key: Path, official_pin: str | None, official: bool, temp: Path) -> tuple[Path, str]:
    public = temp / "public.pem"
    raw = temp / "public.bin"
    if in_toolchain("python3", imgtool(), "getpub", "-e", "pem", "-k", str(key), "-o", str(public)):
        sys.exit("Unable to derive MCUboot public key")
    if in_toolchain("python3", imgtool(), "getpub", "-e", "raw", "-k", str(public), "-o", str(raw)):
        sys.exit("Unable to encode MCUboot public key")
    fingerprint = digest(raw)
    sample_key = NCS_DIR / "bootloader/mcuboot/root-ed25519.pem"
    if sample_key.is_file():
        sample_raw = temp / "sample.bin"
        if in_toolchain("python3", imgtool(), "getpub", "-e", "raw", "-k", str(sample_key), "-o", str(sample_raw)):
            sys.exit("Unable to check MCUboot sample key")
        if fingerprint == digest(sample_raw):
            sys.exit("The public MCUboot sample key cannot sign a release build")
    if official != (fingerprint == official_pin):
        sys.exit("MCUboot signing key does not match the requested release channel")
    return public, fingerprint


def release_debug_configuration(configs: dict[str, Path], approtect: bool) -> dict[str, str]:
    expected = "locked" if approtect else "diagnostic"
    modes = {}
    for image, config in configs.items():
        options = set(re.findall(
            r"^CONFIG_NRF_((?:SECURE_)?APPROTECT_(?:LOCK|DISABLE|USER_HANDLING))=y$",
            config.read_text(), re.MULTILINE,
        ))
        if options == {"APPROTECT_LOCK", "SECURE_APPROTECT_LOCK"}:
            mode = "locked"
        elif options == {"APPROTECT_DISABLE", "SECURE_APPROTECT_DISABLE"}:
            mode = "diagnostic"
        else:
            sys.exit(f"{image} has invalid or mixed generated APPROTECT options")
        if mode != expected:
            sys.exit(f"{image} generated APPROTECT mode is {mode}; requested {expected}")
        modes[image] = mode
    return modes


def release_manifest(official: bool, approtect: bool, public: Path, fingerprint: str, counter: int | None,
                     enrollment_pin: str | None, mbedtls: Path, mcuboot: Path) -> None:
    boot = BUILD_DIR / "mcuboot/zephyr"
    app = BUILD_DIR / "app/zephyr"
    debug_configuration = release_debug_configuration({"app": app / ".config", "mcuboot": boot / ".config"}, approtect)
    app_config = (app / ".config").read_text()
    compiled_pin = re.search(r'^CONFIG_NU54_INSTALL_AUTHORITY_HEX="(.*?)"$', app_config, re.MULTILINE)
    if not compiled_pin or compiled_pin[1] != (enrollment_pin or "").lower():
        sys.exit("Built P01 enrollment pin does not match the release configuration")
    boot_bytes = bytes(int(value, 16) for value in re.findall(r"0x([a-fA-F0-9]{2})\b", (boot / "autogen-pubkey.c").read_text()))
    if hashlib.sha256(boot_bytes).hexdigest() != fingerprint:
        sys.exit("Built MCUboot trust key does not match the selected signer")
    if in_toolchain("python3", str(HERE / "verify_release_image.py"), imgtool(), str(public),
                    str(app / "zephyr.signed.bin"), str(app / "zephyr.signed.hex")):
        sys.exit("Built firmware signature verification failed")
    if in_toolchain("python3", str(HERE / "verify_recovery_layout.py"), imgtool(), str(public), str(BUILD_DIR), str(counter or 0)):
        sys.exit("Built firmware recovery layout or interruption verification failed")
    west_manifest = BUILD_DIR / "west-frozen.yml"
    if in_toolchain("west", "manifest", "--freeze", "--active-only", "-o", str(west_manifest)):
        sys.exit("Unable to freeze the firmware SDK module manifest")
    source_commit = subprocess.run(
        ["git", "rev-parse", "HEAD"], cwd=REPO, capture_output=True, text=True,
    ).stdout.strip() or None
    source_status = subprocess.run(
        ["git", "status", "--porcelain"], cwd=REPO, capture_output=True, text=True,
    )
    if official and (not source_commit or source_status.returncode != 0 or source_status.stdout):
        sys.exit("Official firmware source changed before release verification")
    nrf_commit = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=NCS_DIR / "nrf", text=True).strip()
    cache = (BUILD_DIR / "app/CMakeCache.txt").read_text()
    compiler_path = re.search(r"^CMAKE_C_COMPILER:(?:FILEPATH|STRING)=(.+)$", cache, re.MULTILINE)
    if not compiler_path:
        sys.exit("Cannot identify the selected firmware compiler")
    compiler = Path(compiler_path[1])
    if not compiler.is_file() or not os.access(compiler, os.X_OK):
        sys.exit("Selected firmware compiler is not an executable file")
    nm_path = re.search(r"^CMAKE_NM:(?:FILEPATH|STRING)=(.+)$", cache, re.MULTILINE)
    if not nm_path:
        sys.exit("Cannot identify the selected firmware symbol reader")
    tls_component = mbedtls_input.evidence(mbedtls, BUILD_DIR, Path(nm_path[1]))
    mcuboot_component = mcuboot_input.evidence(mcuboot, BUILD_DIR)
    psa_path = re.search(r'^CONFIG_TF_PSA_CRYPTO_MODULE_DIR="(.+)"$', app_config, re.MULTILINE)
    if not psa_path or Path(psa_path[1]).resolve() != (NCS_DIR / "modules/crypto/oberon-psa-crypto").resolve():
        sys.exit("Built firmware did not select the Nordic Oberon PSA provider")
    tls_sbom = BUILD_DIR / "tls-stack.cdx.json"
    mbedtls_input.write_sbom(tls_sbom, tls_component, Path(psa_path[1]))
    public_copy = BUILD_DIR / "release-public.pem"
    shutil.copyfile(public, public_copy)
    artifacts = [
        boot / "zephyr.elf", boot / "zephyr.hex", boot / "zephyr.map", boot / ".config",
        app / "zephyr.elf", app / "zephyr.bin", app / "zephyr.signed.bin", app / "zephyr.signed.hex", app / "zephyr.map", app / ".config",
        west_manifest, public_copy, tls_sbom,
    ]
    if counter is not None:
        artifacts.append(BUILD_DIR / "app_provision.hex")
    provenance = {
        "channel": "official" if official else "custom",
        "board": BOARD,
        "ncs": NCS_VERSION,
        "nrfCommit": nrf_commit,
        "tlsComponent": tls_component,
        "mcubootComponent": mcuboot_component,
        "compilerSha256": digest(compiler),
        "sourceCommit": source_commit,
        "sourceDirty": bool(source_status.stdout) if source_status.returncode == 0 else None,
        "debugProtection": all(mode == "locked" for mode in debug_configuration.values()),
        "debugProtectionBuildConfiguration": debug_configuration,
        "debugProtectionDeviceReadback": None,
        "serialRecovery": {
            "layoutVerified": True, "signedApplicationSlotOnly": True,
            "staging": "application-secondary" if counter is not None else "bootloader-primary",
            "offlineInterruptedPrefixesRejected": 4, "onDeviceRecoveryVerified": None,
            "hardwareSecurityCounter": counter, "hardwareCounterReadback": None,
        },
        "mcubootPublicKeySha256": fingerprint,
        "enrollmentAuthorityPointSha256": hashlib.sha256(bytes.fromhex(enrollment_pin)).hexdigest() if enrollment_pin else None,
        "artifacts": {str(path.relative_to(BUILD_DIR)): digest(path) for path in artifacts},
    }
    manifest = BUILD_DIR / "release-manifest.json"
    manifest.write_text(json.dumps(provenance, sort_keys=True, indent=2) + "\n")
    print(f"Verified firmware build evidence: {manifest}")


def build(pristine: bool, rental: bool, release: bool, approtect: bool, official: bool = False,
          counter: int | None = None, *, test_cache: Path | None = None,
          runner=None) -> int:
    if approtect and not release:
        sys.exit("--approtect only goes with --release")
    if official and not release:
        sys.exit("--official only goes with --release")
    if official and not approtect:
        sys.exit("--official requires --approtect")
    if counter is not None and (not release or not 1 <= counter <= 65535):
        sys.exit("--security-counter requires --release and a counter from 1 to 65535")
    args = ["west", "build", "-b", BOARD, str(APP), "-d", str(BUILD_DIR)]
    if pristine:
        args += ["-p", "always"]
    cmake = [f"-DBOARD_ROOT={PRODUCT}"]
    def west_build(*modules: Path) -> int:
        options = cmake.copy()
        if test_cache is not None:
            modules = (*modules, HERE / "capability-cache")
            options.append(f"-DUSER_CACHE_DIR:PATH={test_cache}")
        if modules:
            options.append("-DEXTRA_ZEPHYR_MODULES=" + ";".join(str(module) for module in modules))
        command = (*args, "--", *options)
        return in_toolchain(*command) if runner is None else in_toolchain(*command, runner=runner)

    # Sysbuild passes image options by image name: "app" and "mcuboot".
    if rental:
        cmake.append(f"-Dapp_EXTRA_CONF_FILE={APP / 'rental.conf'}")
    if not release:
        return west_build()
    policy = json.loads(RELEASE_POLICY.read_text())
    official_pin = policy["officialPublicKeySha256"]
    if official:
        if not official_pin or not re.fullmatch("[a-f0-9]{64}", official_pin):
            sys.exit("Official MCUboot public key is not enrolled")
        if counter is None:
            sys.exit("Official firmware requires --security-counter")
        status = subprocess.run(["git", "status", "--porcelain"], cwd=REPO, capture_output=True, text=True, check=True)
        if status.stdout:
            sys.exit("Official firmware requires a clean source tree")
    key = signing_key()
    (BUILD_DIR / "release-manifest.json").unlink(missing_ok=True)
    with tempfile.TemporaryDirectory() as tmp:
        public, fingerprint = release_signer(key, official_pin, official, Path(tmp))
        mbedtls = mbedtls_input.prepare(Path(tmp))
        mcuboot = mcuboot_input.prepare(NCS_DIR / "bootloader/mcuboot", Path(tmp))
        cmake.extend(f"-D{image}_EXTRA_ZEPHYR_MODULES={mbedtls};{mcuboot}" for image in ("app", "mcuboot"))
        app_conf = [APP / "release.conf", APP / "pilot-tls.conf", APP / "hd-recovery.conf"] + ([APP / "approtect.conf"] if approtect else [])
        if os.environ.get("NU54_LAB_WALLET_CHAINS") == "1":
            if official or not os.environ.get("NU54_RECOVERY_OVERLAY"):
                sys.exit("Laboratory wallet chains require a custom build with the recovery accessory UART")
            app_conf.append(APP / "wallet-chains.conf")
        if os.environ.get("NU54_COMMISSIONED_WALLET_CHAINS") == "1":
            if official or not os.environ.get("NU54_RECOVERY_OVERLAY") or not os.environ.get("NU54_INSTALL_AUTHORITY_HEX"):
                sys.exit("Commissioned testnets require a custom image with an enrolled authority and recovery accessory")
            if APP / "wallet-chains.conf" not in app_conf:
                app_conf.append(APP / "wallet-chains.conf")
            app_conf.append(APP / "commissioned-wallet-chains.conf")
        enrollment_pin = policy.get("officialEnrollmentAuthorityPoint") if official else os.environ.get("NU54_INSTALL_AUTHORITY_HEX")
        if official and not enrollment_pin:
            sys.exit("Official board enrollment authority is not enrolled")
        if enrollment_pin:
            if not re.fullmatch(r"04[0-9a-fA-F]{128}", enrollment_pin):
                sys.exit("NU54_INSTALL_AUTHORITY_HEX must be an uncompressed P-256 public point")
            pin_conf = Path(tmp) / "enrollment-pin.conf"
            pin_conf.write_text(f'CONFIG_NU54_INSTALL_AUTHORITY_HEX="{enrollment_pin.lower()}"\n')
            app_conf.append(pin_conf)
        if counter is not None:
            app_conf.append(APP / "counted-update.conf")
        cmake += [
            f"-DSB_CONF_FILE={APP / 'sysbuild-release.conf'}",
            f"-DSB_CONFIG_BOOT_SIGNATURE_KEY_FILE=\"{key}\"",
            "-Dapp_EXTRA_CONF_FILE=" + ";".join(str(p) for p in app_conf),
        ]
        recovery_overlay = os.environ.get("NU54_RECOVERY_OVERLAY")
        if recovery_overlay:
            if official:
                sys.exit("Official recovery hardware must come from the reviewed board configuration")
            overlay = Path(recovery_overlay).expanduser().resolve()
            if not overlay.is_file() or ";" in str(overlay):
                sys.exit("NU54_RECOVERY_OVERLAY must name one devicetree file")
            cmake.append(f"-Dapp_EXTRA_DTC_OVERLAY_FILE={overlay}")
        if counter is not None:
            # The Nordic single-slot loader does not advance the hardware rollback counter.
            cmake.extend(["-DSB_CONFIG_MCUBOOT_MODE_SINGLE_APP=n", "-DSB_CONFIG_MCUBOOT_MODE_SWAP_USING_MOVE=y",
                          "-DSB_CONFIG_MCUBOOT_HARDWARE_DOWNGRADE_PREVENTION=y",
                          f"-DSB_CONFIG_MCUBOOT_HW_DOWNGRADE_PREVENTION_COUNTER_VALUE={counter}"])
        if approtect or counter is not None:
            # Setting it replaces app/sysbuild/mcuboot.conf, so name that file as well.
            boot_conf = [APP / "counted-mcuboot.conf" if counter is not None else APP / "sysbuild" / "mcuboot.conf"]
            if approtect:
                boot_conf.append(APP / "approtect.conf")
            cmake.append("-Dmcuboot_EXTRA_CONF_FILE=" + ";".join(str(p) for p in boot_conf))
        rc = west_build(mcuboot)
        if rc == 0:
            release_manifest(official, approtect, public, fingerprint, counter, enrollment_pin, mbedtls, mcuboot)
        return rc


def keygen(path: str) -> int:
    key = Path(path).expanduser()
    if not outside_repo(key):
        sys.exit("the signing key must live outside the repository")
    if key.exists():
        sys.exit(f"{key} exists; not overwriting a signing key")
    return in_toolchain("python3", imgtool(), "keygen", "-k", str(key.resolve()), "-t", "ed25519")


def flash() -> int:
    # A release build has MCUboot and the signed app as two images; other builds have one.
    boot = BUILD_DIR / "mcuboot" / "zephyr" / "zephyr.hex"
    if boot.exists():
        config = boot.parent / ".config"
        if not config.is_file():
            sys.exit("MCUboot configuration missing; rebuild before flashing")
        if "CONFIG_MCUBOOT_HW_DOWNGRADE_PREVENTION=y" in config.read_text().splitlines():
            sys.exit("Hardware-counter firmware requires separately authorized manufacturing provisioning")
        images = [boot, BUILD_DIR / "app" / "zephyr" / "zephyr.signed.hex"]
    else:
        merged = BUILD_DIR / "merged.hex"
        images = [merged if merged.exists() else BUILD_DIR / "app" / "zephyr" / "zephyr.hex"]
    for hex_file in images:
        if not hex_file.exists():
            sys.exit(f"{hex_file} missing; run build first")
    for hex_file in images:
        rc = in_toolchain("pyocd", "flash", "-t", PYOCD_TARGET, str(hex_file))
        if rc:
            return rc
    return 0


def reset() -> int:
    return in_toolchain("pyocd", "reset", "-t", PYOCD_TARGET)


def imgtool() -> str:
    return str(NCS_DIR / "bootloader/mcuboot/scripts/imgtool.py")


def reject_images(out_dir: str) -> int:
    ninja = BUILD_DIR / "app" / "build.ninja"
    found = re.search(r"imgtool\.py sign (.+?) -k \S+ (.+?) \S+zephyr\.bin ", ninja.read_text()) if ninja.exists() else None
    if not found:
        sys.exit(f"no signed release build under {BUILD_DIR}; run build --release first")
    # The same layout options the build used (version, slot and header size, alignment, hash).
    options = shlex.split(found.group(1)) + shlex.split(found.group(2))
    unsigned_in = BUILD_DIR / "app" / "zephyr" / "zephyr.bin"
    out = Path(out_dir).resolve()
    out.mkdir(parents=True, exist_ok=True)
    rc = in_toolchain("python3", imgtool(), "sign", *options, str(unsigned_in), str(out / "unsigned.bin"))
    with tempfile.TemporaryDirectory() as tmp:
        other = Path(tmp) / "other.pem"
        rc = rc or in_toolchain("python3", imgtool(), "keygen", "-k", str(other), "-t", "ed25519")
        rc = rc or in_toolchain("python3", imgtool(), "sign", *options, "-k", str(other), str(unsigned_in),
                                str(out / "other-key.bin"))
    if not rc:
        print(f"{out}/unsigned.bin and {out}/other-key.bin: upload each with serial recovery; the board must not boot them")
    return rc


def main(argv: list[str]) -> int:
    if not argv or argv[0] not in {"build", "flash", "reset", "keygen", "reject-images"}:
        print(__doc__)
        return 2
    if argv[0] == "build":
        counter_flags = [arg.split("=", 1)[-1] for arg in argv[1:] if arg.startswith("--security-counter")]
        if counter_flags and (len(counter_flags) != 1 or not re.fullmatch(r"[0-9]{1,5}", counter_flags[0])):
            sys.exit("Use --security-counter=N for exactly one decimal counter")
        options = ("--pristine" in argv, "--rental" in argv, "--release" in argv,
                   "--approtect" in argv, "--official" in argv, int(counter_flags[0]) if counter_flags else None)
        if "--test-capability-cache" in argv:
            if (not os.environ.get("P01_BUILD_DIR") or not BUILD_DIR.is_absolute()
                    or (BUILD_DIR.exists() and (not BUILD_DIR.is_dir() or any(BUILD_DIR.iterdir())))):
                sys.exit("--test-capability-cache requires a fresh, explicit absolute P01_BUILD_DIR")
            sys.path.insert(0, str(REPO / "scripts"))
            try:
                from integration_workspace import run_command, scoped_workspace
            finally:
                sys.path.pop(0)
            with scoped_workspace("nu54-p01-capability-") as cache:
                return build(*options, test_cache=cache, runner=run_command)
        return build(*options)
    if argv[0] in {"keygen", "reject-images"}:
        if len(argv) != 2:
            print(__doc__)
            return 2
        return keygen(argv[1]) if argv[0] == "keygen" else reject_images(argv[1])
    return flash() if argv[0] == "flash" else reset()


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
