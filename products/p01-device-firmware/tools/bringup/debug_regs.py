#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["pyocd>=0.36"]
# ///
"""Read debug and GPIO registers from the running board (pyOCD attach, no halt).

Commands:
    core     print DHCSR three times: halted / sleeping / pyOCD state
    gpiote   print GPIOTE port-event and interrupt enables, NVIC enable bits,
             and GPIO DETECTMODE / LATCH / IN for both ports

Usage:
    uv run debug_regs.py core
    uv run debug_regs.py gpiote
"""

import argparse
import time

from board import DHCSR, GPIO, GPIO_DETECTMODE, GPIO_IN, GPIO_LATCH, GPIOTE, NVIC_ISER, probe_session


def core(target) -> None:
    for _ in range(3):
        d = target.read32(DHCSR)
        print(f"DHCSR 0x{d:08x} halted={(d >> 17) & 1} sleep={(d >> 18) & 1} state={target.get_state()}")
        time.sleep(0.5)


def gpiote(target) -> None:
    for port, (base, irq) in sorted(GPIOTE.items()):
        r = lambda off: target.read32(base + off)  # noqa: E731
        enabled = (target.read32(NVIC_ISER + 4 * (irq // 32)) >> (irq % 32)) & 1
        print(f"GPIOTE for P{port}: EVENTS_PORT ns=0x{r(0x140):x} s=0x{r(0x144):x} "
              f"INTEN0=0x{r(0x300):08x} INTEN1=0x{r(0x310):08x} NVIC enabled={enabled}")
    for port, base in sorted(GPIO.items()):
        print(f"P{port}: DETECTMODE=0x{target.read32(base + GPIO_DETECTMODE):x} "
              f"LATCH=0x{target.read32(base + GPIO_LATCH):x} IN=0x{target.read32(base + GPIO_IN):08x}")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("command", choices=["core", "gpiote"])
    args = ap.parse_args()
    with probe_session() as session:
        {"core": core, "gpiote": gpiote}[args.command](session.target)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
