#!/usr/bin/env python3
"""Create an offline-signed FOTA descriptor for an independently verified MCUboot build."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import struct
import tempfile

from evidence import openssl, read, require, verify


def descriptor(args):
    build = read(args.build_manifest)
    require(build["channel"] == args.channel and build["debugProtection"], "source build channel/protection differs")
    require(re.fullmatch(r"[a-z0-9][a-z0-9_-]{0,31}", args.board_revision), "invalid enclosed board revision")
    wire = args.image.read_bytes()
    archive = hashlib.sha256(wire).hexdigest()
    require(build["artifacts"]["app/zephyr/zephyr.signed.bin"] == archive, "image differs from verified build")
    counter = build["serialRecovery"]["hardwareSecurityCounter"]
    require(type(counter) is int and 1 <= counter <= 65535, "missing counted release")
    if args.channel == "official":
        require(all((args.release_manifest, args.release_signature, args.release_public_key, args.release_trust)),
                "official rollout requires an independently verified release evidence bundle")
        verify(args.release_manifest, args.release_signature, args.release_public_key, read(args.release_trust), args.channel)
        evidence = read(args.release_manifest)
        require(evidence["sourceCommit"] == build["sourceCommit"] and build["sourceDirty"] is False
                and any(artifact["id"] == "firmware" and artifact["target"] == args.board_revision
                        and artifact["payload"]["sha256"] == archive for artifact in evidence["artifacts"]),
                "official artifact, revision or source differs from the build")
    magic, _, header, protected, size = struct.unpack_from("<IIHHI", wire)
    require(magic == 0x96F3B83D and header >= 32 and size and protected >= 8 and header + size + protected < len(wire),
            "invalid counted image")
    major, minor, patch, build_number = struct.unpack_from("<BBHI", wire, 20)
    version = f"{major}.{minor}.{patch}" + (f"+{build_number}" if build_number else "")
    fields = {"version": version, "boardRevision": args.board_revision, "counter": counter, "channel": args.channel,
              "measurement": hashlib.sha256(wire[:header + size + protected]).hexdigest(), "imageSha256": archive}
    if args.release_manifest:
        fields["releaseEvidence"] = hashlib.sha256(args.release_manifest.read_bytes()).hexdigest()
    with tempfile.TemporaryDirectory(prefix="nu54-fota-") as work:
        path = Path(work) / "descriptor.json"
        path.write_text(json.dumps(fields, sort_keys=True, separators=(",", ":")) + "\n")
        signature = openssl(["pkeyutl", "-sign", "-rawin", "-inkey", args.publisher_key, "-in", path])
        args.output.write_bytes(path.read_bytes())
        args.signature.write_bytes(signature)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--board-revision", required=True)
    parser.add_argument("--channel", required=True, choices=["official", "custom"])
    for name in ("build-manifest", "image", "publisher-key", "output", "signature"):
        parser.add_argument(f"--{name}", type=Path, required=True)
    for name in ("release-manifest", "release-signature", "release-public-key", "release-trust"):
        parser.add_argument(f"--{name}", type=Path)
    args = parser.parse_args()
    try:
        descriptor(args)
    except (ValueError, KeyError, OSError, struct.error) as exc:
        parser.exit(1, f"{exc}\n")


if __name__ == "__main__":
    main()
