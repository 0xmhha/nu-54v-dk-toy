#!/usr/bin/env python3
"""Check the Android runtime's selected HTTP/IO coordinates."""

import argparse
from pathlib import Path

EXPECTED = {
    ("com.squareup.okhttp3", "okhttp"): "4.12.0",
    ("com.squareup.okhttp3", "okhttp-urlconnection"): "4.12.0",
    ("com.squareup.okio", "okio"): "3.6.0",
    ("com.squareup.okio", "okio-jvm"): "3.6.0",
}


def verify(path, variant):
    seen = {}
    for row in path.read_text().splitlines():
        coordinates, sep, config = row.partition("=")
        parts = coordinates.split(":")
        if not sep or config != variant or len(parts) != 3 or not all(parts):
            raise ValueError("invalid resolved runtime lock entry")
        module = tuple(parts[:2])
        if module in seen:
            raise ValueError(f"duplicate runtime module {'/'.join(module)}")
        seen[module] = parts[2]
    for module, version in EXPECTED.items():
        if seen.get(module) != version:
            raise ValueError(f"{':'.join(module)} selected {seen.get(module)}, expected {version}")
    return seen


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("lockfile", type=Path)
    parser.add_argument("--variant", default="customRuntimeClasspath",
                        choices=("customRuntimeClasspath", "releaseRuntimeClasspath"))
    args = parser.parse_args()
    try:
        selected = verify(args.lockfile, args.variant)
        print(f"Verified {args.variant}: {len(selected)} resolved modules; OkHttp 4.12.0 and Okio 3.6.0")
    except (ValueError, OSError) as error:
        parser.exit(1, f"FAIL {error}\n")


if __name__ == "__main__":
    main()
