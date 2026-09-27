#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["pyocd>=0.36"]
# ///
"""Watch real button presses: compare the pin level with what the firmware reports.

Polls the GPIO input register through the debugger and `button info` over VCOM, and
prints a line whenever either changes. Press the buttons on the board while it runs.
Needs the reference `button` example firmware.

Usage:
    uv run button_watch.py --seconds 45
"""

import argparse
import re
import time

from board import BUTTONS, GPIO, GPIO_IN, Vcom, probe_session


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--port", help="VCOM device (default: auto-detect)")
    ap.add_argument("--seconds", type=float, default=45.0)
    args = ap.parse_args()
    t0 = time.time()
    previous = None
    with Vcom(args.port) as vcom, probe_session() as session:
        vcom.read(0.3)
        target = session.target
        while time.time() - t0 < args.seconds:
            levels = {port: target.read32(GPIO[port] + GPIO_IN) for port in GPIO}
            pins = " ".join(f"{n}={(levels[p] >> pin) & 1}" for n, (p, pin) in sorted(BUTTONS.items()))
            reported = dict(re.findall(r"(BTN\d)\s+: P\d\.\d+, (\w+)", vcom.command("button info", 0.15)))
            driver = " ".join(f"{n}={reported.get(n, '?')}" for n in sorted(BUTTONS))
            current = f"pin {pins} | firmware {driver}"
            if current != previous:
                print(f"{time.time() - t0:5.2f}s {current}", flush=True)
                previous = current
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
