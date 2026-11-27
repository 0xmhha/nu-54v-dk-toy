#!/usr/bin/env python3
"""Install and print the path to the verified P06/P08 Solidity compiler."""

import argparse
import hashlib
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile
from urllib.request import urlopen

VERSION = "0.8.39+commit.ce47f2c8"
BASE_URL = "https://raw.githubusercontent.com/ethereum/solc-bin/gh-pages"
# SHA-256 from the Solidity project binary distribution lists.
BUILDS = {
    ("Linux", "x86_64"): (
        "linux-amd64", "b8b0642d46838575d3177628b685564b52f51f2aee546cb4dce781951d23c666",
    ),
    ("Darwin", "x86_64"): (
        "macosx-amd64", "6134d77c433e3b714b64b4101a6147cacff93d50137c0b66ab80825fb274ef51",
    ),
}
BUILDS["Darwin", "arm64"] = BUILDS["Darwin", "x86_64"]


def verify(binary, expected):
    with binary.open("rb") as data:
        if hashlib.file_digest(data, "sha256").hexdigest() != expected:
            raise ValueError(f"SHA-256 mismatch for {binary}")
    version = subprocess.check_output([str(binary), "--version"], text=True).splitlines()
    if not any(line.startswith(f"Version: {VERSION}.") for line in version):
        raise ValueError(f"Unexpected compiler version in {binary}: {version}")


def install(destination, distribution, expected):
    destination = destination.resolve()
    if destination.exists():
        verify(destination, expected)
        return destination

    destination.parent.mkdir(parents=True, exist_ok=True)
    url = f"{BASE_URL}/{distribution}/solc-{distribution}-v{VERSION}"
    with tempfile.TemporaryDirectory(prefix=".solc-", dir=destination.parent) as work:
        binary = Path(work) / destination.name
        with urlopen(url, timeout=60) as remote, binary.open("wb") as output:
            shutil.copyfileobj(remote, output)
        binary.chmod(0o755)
        verify(binary, expected)
        binary.replace(destination)
    return destination


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("destination", type=Path)
    args = parser.parse_args()
    host = platform.system(), platform.machine()
    if host not in BUILDS:
        parser.error(f"no pinned Solidity executable for {host}")
    try:
        print(install(args.destination, *BUILDS[host]))
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"{error}\n")


if __name__ == "__main__":
    main()
