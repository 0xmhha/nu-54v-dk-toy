#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["bleak>=0.22"]
# ///
"""Scan for the board over BLE and print matching advertisements.

Matches the Nordic UART Service UUID or names starting with the given prefixes.

Usage:
    uv run ble_scan.py
    uv run ble_scan.py --timeout 20 --prefix NU54
"""

import argparse
import asyncio

from bleak import BleakScanner

NUS_SERVICE = "6e400001-b5a3-f393-e0a9-e50e24dcca9e"


async def scan(timeout: float, prefixes: tuple[str, ...]) -> None:
    found = await BleakScanner.discover(timeout=timeout, return_adv=True)
    print("devices seen:", len(found))
    for addr, (dev, adv) in found.items():
        uuids = [u.lower() for u in adv.service_uuids]
        name = adv.local_name or dev.name or ""
        if NUS_SERVICE in uuids or name.upper().startswith(prefixes):
            mfg = {k: v.hex() for k, v in adv.manufacturer_data.items()}
            print(addr, f"name={name!r}", f"rssi={adv.rssi}", f"uuids={uuids}", f"mfg={mfg}")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--timeout", type=float, default=12.0)
    ap.add_argument("--prefix", action="append", default=None, help="name prefix (default: NU54)")
    args = ap.parse_args()
    asyncio.run(scan(args.timeout, tuple(p.upper() for p in (args.prefix or ["NU54"]))))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
