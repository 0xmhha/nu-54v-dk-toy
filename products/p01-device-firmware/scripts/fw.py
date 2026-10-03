#!/usr/bin/env python3
"""Build, flash and reset the P01 firmware ([D05][N25]).

Runs west and pyOCD inside the NCS toolchain environment through nrfutil, so no
global PATH setup is needed. The vendored board package is passed as BOARD_ROOT
because sysbuild resolves the board before the application CMakeLists runs.

Usage:
    python3 scripts/fw.py build [--pristine] [--rental]
    python3 scripts/fw.py flash
    python3 scripts/fw.py reset

--rental adds app/rental.conf (no week-7 fixed setup). Switching between the two needs
--pristine or a separate P01_BUILD_DIR.

Environment:
    NCS_VERSION   default v3.4.1
    NRFUTIL       default nrfutil on PATH, then ~/.local/bin/nrfutil
    P01_BUILD_DIR default products/p01-device-firmware/build
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
PRODUCT = HERE.parent
APP = PRODUCT / "app"
BOARD = "nu54v_dk/nrf54l15/cpuapp"
PYOCD_TARGET = "nrf54l"
NCS_VERSION = os.environ.get("NCS_VERSION", "v3.4.1")
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
    return subprocess.run(full, cwd=f"/opt/nordic/ncs/{NCS_VERSION}").returncode


def build(pristine: bool, rental: bool) -> int:
    args = ["west", "build", "-b", BOARD, str(APP), "-d", str(BUILD_DIR)]
    if pristine:
        args += ["-p", "always"]
    cmake = [f"-DBOARD_ROOT={PRODUCT}"]
    if rental:
        # Sysbuild passes image options by image name; the application image is "app".
        cmake.append(f"-Dapp_EXTRA_CONF_FILE={APP / 'rental.conf'}")
    return in_toolchain(*args, "--", *cmake)


def flash() -> int:
    hex_file = BUILD_DIR / "merged.hex"
    if not hex_file.exists():
        hex_file = BUILD_DIR / "app" / "zephyr" / "zephyr.hex"
    if not hex_file.exists():
        sys.exit(f"no firmware image under {BUILD_DIR}; run build first")
    return in_toolchain("pyocd", "flash", "-t", PYOCD_TARGET, str(hex_file))


def reset() -> int:
    return in_toolchain("pyocd", "reset", "-t", PYOCD_TARGET)


def main(argv: list[str]) -> int:
    if not argv or argv[0] not in {"build", "flash", "reset"}:
        print(__doc__)
        return 2
    if argv[0] == "build":
        return build("--pristine" in argv, "--rental" in argv)
    return flash() if argv[0] == "flash" else reset()


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
