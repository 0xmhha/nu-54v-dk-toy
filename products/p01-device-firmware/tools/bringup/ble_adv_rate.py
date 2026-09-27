#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["bleak>=0.22"]
# ///
"""Measure how often the board's advertisements reach this computer.

The host OS drops and merges reports, so the median gap is an upper bound on the
advertising interval, not the exact value.

Usage:
    uv run ble_adv_rate.py --seconds 15
"""

import argparse
import asyncio
import time

from bleak import BleakScanner

NUS_SERVICE = "6e400001-b5a3-f393-e0a9-e50e24dcca9e"


async def measure(name: str, seconds: float) -> None:
    seen: list[tuple[float, int]] = []

    def on_adv(dev, adv):
        if (adv.local_name or dev.name) == name or NUS_SERVICE in [u.lower() for u in adv.service_uuids]:
            seen.append((time.time(), adv.rssi))

    scanner = BleakScanner(on_adv)
    await scanner.start()
    await asyncio.sleep(seconds)
    await scanner.stop()
    if not seen:
        print("not seen")
        return
    times = [t for t, _ in seen]
    gaps = sorted(b - a for a, b in zip(times, times[1:]))
    rssi = [r for _, r in seen]
    line = f"{len(seen)} reports in {seconds:.0f}s, rssi {min(rssi)}..{max(rssi)}"
    if gaps:
        line += f", median gap {gaps[len(gaps) // 2]:.2f}s"
    print(line)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--name", default="NU54V-DK")
    ap.add_argument("--seconds", type=float, default=15.0)
    args = ap.parse_args()
    asyncio.run(measure(args.name, args.seconds))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
