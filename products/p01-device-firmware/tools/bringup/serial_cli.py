#!/usr/bin/env python3
"""Send CLI commands to the board over VCOM and print the replies.

Needs firmware with the reference CLI (for example the reference `ble_nus` or `button` example).

Usage:
    python3 serial_cli.py "ble info" "button info"
    python3 serial_cli.py --port /dev/cu.usbmodem213104 --listen 60 "ble info"
"""

import argparse
import sys

from board import Vcom


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("commands", nargs="*", help="CLI commands, sent in order")
    ap.add_argument("--port", help="VCOM device (default: auto-detect)")
    ap.add_argument("--wait", type=float, default=1.0, help="seconds to collect each reply")
    ap.add_argument("--listen", type=float, default=0.0, help="seconds to keep printing output at the end")
    args = ap.parse_args()
    with Vcom(args.port) as vcom:
        vcom.read(0.5)  # drop a stale prompt
        for cmd in args.commands:
            sys.stdout.write(vcom.command(cmd, args.wait))
        if args.listen:
            sys.stdout.write(vcom.read(args.listen))
    sys.stdout.flush()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
