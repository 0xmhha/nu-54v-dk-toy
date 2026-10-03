#!/usr/bin/env python3
"""Build, flash and reset the P01 firmware ([D05][N25]).

Runs west and pyOCD inside the NCS toolchain environment through nrfutil, so no
global PATH setup is needed. The vendored board package is passed as BOARD_ROOT
because sysbuild resolves the board before the application CMakeLists runs.

Usage:
    python3 scripts/fw.py build [--pristine] [--rental | --release [--approtect]]
    python3 scripts/fw.py keygen PATH
    python3 scripts/fw.py reject-images OUT_DIR
    python3 scripts/fw.py flash
    python3 scripts/fw.py reset

--rental adds app/rental.conf (no week-7 fixed setup). --release builds MCUboot with serial
recovery and the signed release app (app/sysbuild-release.conf, app/release.conf); it needs
NU54_SIGNING_KEY. --approtect also locks the debug port (app/approtect.conf). Switching between
variants needs --pristine or a separate P01_BUILD_DIR.

reject-images writes, from a release build, the two images MCUboot must refuse (W12-11): one
with no signature and one signed with a throwaway key that is deleted afterwards.

keygen makes an ed25519 signing key with MCUboot's imgtool. Keep it outside the repository and
offline; anyone with it can make images the board boots.

Environment:
    NCS_VERSION   default v3.4.1
    NRFUTIL       default nrfutil on PATH, then ~/.local/bin/nrfutil
    P01_BUILD_DIR default products/p01-device-firmware/build
    NU54_SIGNING_KEY  release signing key (PEM), outside the repository
"""

from __future__ import annotations

import os
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
PRODUCT = HERE.parent
APP = PRODUCT / "app"
BOARD = "nu54v_dk/nrf54l15/cpuapp"
PYOCD_TARGET = "nrf54l"
NCS_VERSION = os.environ.get("NCS_VERSION", "v3.4.1")
NCS_DIR = Path(f"/opt/nordic/ncs/{NCS_VERSION}")
REPO = PRODUCT.parents[1]
BUILD_DIR = Path(os.environ.get("P01_BUILD_DIR", PRODUCT / "build"))


def nrfutil() -> str:
    found = os.environ.get("NRFUTIL") or shutil.which("nrfutil")
    if found:
        return found
    fallback = Path.home() / ".local/bin/nrfutil"
    if fallback.exists():
        return str(fallback)
    sys.exit("nrfutil not found; install it and run `nrfutil install sdk-manager`")


def in_toolchain(*cmd: str) -> int:
    """Run a command inside the NCS toolchain environment."""
    full = [nrfutil(), "sdk-manager", "toolchain", "launch", "--ncs-version", NCS_VERSION, "--", *cmd]
    return subprocess.run(full, cwd=NCS_DIR).returncode


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


def build(pristine: bool, rental: bool, release: bool, approtect: bool) -> int:
    if approtect and not release:
        sys.exit("--approtect only goes with --release")
    args = ["west", "build", "-b", BOARD, str(APP), "-d", str(BUILD_DIR)]
    if pristine:
        args += ["-p", "always"]
    cmake = [f"-DBOARD_ROOT={PRODUCT}"]
    # Sysbuild passes image options by image name: "app" and "mcuboot".
    if rental:
        cmake.append(f"-Dapp_EXTRA_CONF_FILE={APP / 'rental.conf'}")
    if release:
        app_conf = [APP / "release.conf"] + ([APP / "approtect.conf"] if approtect else [])
        cmake += [
            f"-DSB_CONF_FILE={APP / 'sysbuild-release.conf'}",
            f"-DSB_CONFIG_BOOT_SIGNATURE_KEY_FILE=\"{signing_key()}\"",
            "-Dapp_EXTRA_CONF_FILE=" + ";".join(str(p) for p in app_conf),
        ]
        if approtect:
            # Setting it replaces app/sysbuild/mcuboot.conf, so name that file as well.
            boot_conf = [APP / "sysbuild" / "mcuboot.conf", APP / "approtect.conf"]
            cmake.append("-Dmcuboot_EXTRA_CONF_FILE=" + ";".join(str(p) for p in boot_conf))
    return in_toolchain(*args, "--", *cmake)


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
        return build("--pristine" in argv, "--rental" in argv, "--release" in argv, "--approtect" in argv)
    if argv[0] in {"keygen", "reject-images"}:
        if len(argv) != 2:
            print(__doc__)
            return 2
        return keygen(argv[1]) if argv[0] == "keygen" else reject_images(argv[1])
    return flash() if argv[0] == "flash" else reset()


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
