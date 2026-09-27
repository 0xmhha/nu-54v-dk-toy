#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["pyocd>=0.36"]
# ///
"""Simulate button presses from the debugger, without touching the board.

The buttons are active low with pull-ups. While a button is "held", this tool switches
its pin to pull-down (the pin stays an input), then restores the original pull setting.
The firmware keeps running (pyOCD attach, no halt). The pull change alone makes the
GPIO SENSE logic fire. `--kick` also raises the port event by hand; it is off by default
because pending the interrupt by hand can confuse a firmware's sleep and timers.

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
    ap.add_argument("--kick", action="store_true",
                    help="also raise the GPIOTE port event by hand (needed only if a firmware misses the pull change)")
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
        # The debugger is attached only for the moment a pin is changed. While a
        # debug session stays open the core's sleep and kernel timers can stall,
        # which hides releases from firmware that polls the button while it is held.
        for name, hold in plan:
            port, pin = BUTTONS[name]
            addr = pin_cnf(port, pin)
            with probe_session() as session:
                target = session.target
                cnf = target.read32(addr)
                target.write32(addr, (cnf & ~PULL_MASK) | PULL_DOWN)
                if args.kick:
                    time.sleep(0.005)
                    kick_gpiote(target, port)
            pressed_at = time.time()
            log.append(f"{pressed_at - t0:6.2f}s SIM  {name} P{port}.{pin:02d} pressed for {hold}s")
            if args.query:
                time.sleep(0.3)
                log.append("held:\n" + vcom.command("button info", 0.8))
            time.sleep(max(0.0, hold - (time.time() - pressed_at)))
            with probe_session() as session:
                target = session.target
                level = (target.read32(GPIO[port] + GPIO_IN) >> pin) & 1
                # Restore only the pull bits. The GPIO driver may have flipped the
                # SENSE field during the press; writing the whole saved PIN_CNF back
                # would undo that and hide the release.
                target.write32(addr, (target.read32(addr) & ~PULL_MASK) | (cnf & PULL_MASK))
                if args.kick:
                    time.sleep(0.005)
                    kick_gpiote(target, port)
                released_at = time.time()
                latch = target.read32(GPIO[port] + GPIO_LATCH)
            log.append(f"{released_at - t0:6.2f}s SIM  {name} released after {released_at - pressed_at:.2f}s "
                       f"(pin level while held: {level})")
            if args.query:
                time.sleep(0.3)
                log.append("released:\n" + vcom.command("button info", 0.8))
                log.append(f"LATCH P{port} 0x{latch:08x}")
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
