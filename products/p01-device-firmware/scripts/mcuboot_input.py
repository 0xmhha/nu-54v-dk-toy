"""Stage and verify the upstream TLV bounds fix on the pinned Nordic MCUboot."""

import json
import re
from pathlib import Path
import subprocess

from mbedtls_input import digest

VENDOR_COMMIT = "5d8736c1fd98959f9a607f59fe1a2d82c7e37bde"
UPSTREAM_COMMIT = "90f4e7c3ebe4cf9a5eb7d981220388ffdefd6d19"
PATCH = Path(__file__).with_name("mcuboot-tlv-bounds.patch")
PATCH_SHA256 = "02977b3489beaf5f47aad743ae840f2bb6291b9b72ca66013cd73263cb52e876"
TLV = Path("boot/bootutil/src/tlv.c")
PATCHED_TLV_SHA256 = "0f7c8e66efbc5afb9a1b5616fc2a5dd0b389bce880d3e81f1af233d35a7546e2"


def prepare(source: Path, scratch: Path) -> Path:
    """Apply the exact patch to a private checkout without modifying the SDK."""
    source = source.resolve(strict=True)
    if subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=source, text=True).strip() != VENDOR_COMMIT:
        raise SystemExit("MCUboot source does not match the pinned Nordic patch base")
    if digest(PATCH) != PATCH_SHA256:
        raise SystemExit("MCUboot patch digest mismatch")
    target = scratch / "mcuboot"
    subprocess.run(["git", "clone", "--quiet", "--shared", "--no-checkout", "--", str(source), str(target)], check=True)
    subprocess.run(["git", "checkout", "--quiet", "--detach", VENDOR_COMMIT], cwd=target, check=True)
    subprocess.run(["git", "apply", "--check", str(PATCH)], cwd=target, check=True)
    subprocess.run(["git", "apply", str(PATCH)], cwd=target, check=True)
    if digest(target / TLV) != PATCHED_TLV_SHA256:
        raise SystemExit("Patched MCUboot TLV source digest mismatch")
    return target.resolve()


def evidence(module: Path, build: Path) -> dict:
    """Require the bootloader compiler and linker to have used the staged module."""
    boot = build / "mcuboot"
    entries = json.loads((boot / "compile_commands.json").read_text())
    inputs = {
        (Path(entry["directory"]) / entry["file"]).resolve()
        for entry in entries
        if Path(entry["file"]).as_posix().endswith(f"/{TLV.as_posix()}")
    }
    expected = (module / TLV).resolve(strict=True)
    if inputs != {expected} or digest(expected) != PATCHED_TLV_SHA256:
        raise SystemExit("Built MCUboot did not select the pinned TLV source")
    cache = (boot / "CMakeCache.txt").read_text()
    readelf = re.search(r"^CMAKE_READELF:(?:FILEPATH|STRING)=(.+)$", cache, re.MULTILINE)
    if not readelf:
        raise SystemExit("Cannot identify the MCUboot ELF reader")
    lines = subprocess.check_output(
        [readelf[1], "--debug-dump=decodedline", str(boot / "zephyr/zephyr.elf")], text=True)
    # Nordic enables LTO so the iterator can exist solely as inlined code.
    linked_lines = set()
    in_tlv = False
    for line in lines.splitlines():
        if line.endswith(":"):
            in_tlv = line == f"{expected}:"
        elif in_tlv and (row := re.match(r"^tlv[.]c\s+(\d+)\s+0x[0-9a-f]+\b", line)):
            linked_lines.add(int(row[1]))
    # These fixed line numbers follow from the enforced patched-source digest.
    if not (linked_lines & {181, 182}) or 196 not in linked_lines:
        raise SystemExit("Built MCUboot is missing the linked TLV boundary checks")
    return {
        "advisory": "GHSA-4fp3-3qw2-25jv",
        "vendorCommit": VENDOR_COMMIT,
        "upstreamCommit": UPSTREAM_COMMIT,
        "patchSha256": PATCH_SHA256,
        "tlvSourceSha256": PATCHED_TLV_SHA256,
        "linkedTlvBoundaryLines": sorted(linked_lines & {181, 182, 196}),
    }
