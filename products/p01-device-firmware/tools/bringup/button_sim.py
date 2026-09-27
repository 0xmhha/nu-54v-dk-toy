#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["pyocd>=0.36"]
# ///
"""Simulate button presses from the debugger, without touching the board.

The buttons are active low with pull-ups. While a button is "held", this tool switches
its pin to pull-down (the pin stays an input), then restores the original PIN_CNF.
The firmware keeps running (pyOCD attach, no halt).

It turns on the reference CLI's `button event` log over VCOM and prints the log next
to each simulated press. With --query it also runs `button info` while the button is held.
Needs the reference `button` example firmware.

Usage:
    uv run button_sim.py                          # BTN1..BTN4, short and long presses
    uv run button_sim.py --button BTN2 --hold 1.8
    uv run button_sim.py --button BTN1 --hold 0.3 --query
"""

import argparse
import threading
import time

from board import BUTTONS, GPIO, GPIO_IN, GPIO_LATCH, Vcom, kick_gpiote, pin_cnf, probe_session

DEFAULT_PLAN = [("BTN1", 0.3), ("BTN2", 1.8), ("BTN3", 0.3), ("BTN4", 1.5)]
PULL_MASK, PULL_DOWN = 0xC, 1 << 2


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--port", help="VCOM device (default: auto-detect)")
    ap.add_argument("--button", choices=sorted(BUTTONS), help="press only this button")
    ap.add_argument("--hold", type=float, default=0.3, help="seconds to hold (with --button)")
    ap.add_argument("--query", action="store_true", help="run `button info` while held and after release")
    args = ap.parse_args()
    plan = [(args.button, args.hold)] if args.button else DEFAULT_PLAN

    log: list[str] = []
    t0 = time.time()
    stop = threading.Event()
    with Vcom(args.port) as vcom:
        def reader() -> None:
            while not stop.is_set():
                for line in vcom.read(0.1).splitlines():
                    if line.strip():
                        log.append(f"{time.time() - t0:6.2f}s VCOM {line.strip()}")

        if not args.query:
            vcom.command("button event", 0.8)
            thread = threading.Thread(target=reader)
            thread.start()
        with probe_session() as session:
            target = session.target
            for name, hold in plan:
                port, pin = BUTTONS[name]
                addr = pin_cnf(port, pin)
                cnf = target.read32(addr)
                target.write32(addr, (cnf & ~PULL_MASK) | PULL_DOWN)
                time.sleep(0.005)
                kick_gpiote(target, port)
                log.append(f"{time.time() - t0:6.2f}s SIM  {name} P{port}.{pin:02d} pressed for {hold}s")
                if args.query:
                    time.sleep(0.3)
                    log.append("held:\n" + vcom.command("button info", 0.8))
                    time.sleep(max(0.0, hold - 1.1))
                else:
                    time.sleep(hold)
                level = (target.read32(GPIO[port] + GPIO_IN) >> pin) & 1
                target.write32(addr, cnf)
                time.sleep(0.005)
                kick_gpiote(target, port)
                log.append(f"{time.time() - t0:6.2f}s SIM  {name} released (pin level while held: {level})")
                if args.query:
                    time.sleep(0.3)
                    log.append("released:\n" + vcom.command("button info", 0.8))
                    log.append(f"LATCH P{port} 0x{target.read32(GPIO[port] + GPIO_LATCH):08x}")
                time.sleep(1.0)
        if not args.query:
            time.sleep(0.5)
            stop.set()
            thread.join()
            vcom.command("", 0.3)  # a bare CR ends the event log
    print("\n".join(log))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
