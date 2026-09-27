#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["bleak>=0.22"]
# ///
"""Connect to the board over BLE and run CLI commands through the Nordic UART Service.

Writes each command to RX (terminated by CR) and prints what arrives on TX notifications.
Needs the reference `ble_nus` example firmware.

Usage:
    uv run ble_nus_cli.py "ble info"
    uv run ble_nus_cli.py --name NU54V-DK --wait 3 "ble info"
"""

import argparse
import asyncio

from bleak import BleakClient, BleakScanner

NUS_RX = "6e400002-b5a3-f393-e0a9-e50e24dcca9e"  # central -> device (write)
NUS_TX = "6e400003-b5a3-f393-e0a9-e50e24dcca9e"  # device -> central (notify)


async def run(name: str, commands: list[str], wait: float) -> int:
    dev = await BleakScanner.find_device_by_name(name, timeout=10)
    if dev is None:
        print(f"{name} not found")
        return 1
    buf = bytearray()
    async with BleakClient(dev) as client:
        print(f"connected, ATT MTU {client.mtu_size}")
        await client.start_notify(NUS_TX, lambda _h, data: buf.extend(data))
        await asyncio.sleep(1.5)
        if buf:
            print("on subscribe:", bytes(buf).decode(errors="replace"))
            buf.clear()
        for cmd in commands:
            await client.write_gatt_char(NUS_RX, (cmd + "\r").encode(), response=True)
            await asyncio.sleep(wait)
            print(bytes(buf).decode(errors="replace"))
            buf.clear()
        await client.stop_notify(NUS_TX)
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("commands", nargs="+")
    ap.add_argument("--name", default="NU54V-DK", help="advertised device name")
    ap.add_argument("--wait", type=float, default=2.0, help="seconds to collect each reply")
    args = ap.parse_args()
    return asyncio.run(run(args.name, args.commands, args.wait))


if __name__ == "__main__":
    raise SystemExit(main())
