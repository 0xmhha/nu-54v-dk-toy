"""Shared helpers for the board bring-up tools.

The NU-54V-DK exposes two USB interfaces through its DAPLink debugger:
- a CMSIS-DAP probe, used here through pyOCD (target "nrf54l");
- a virtual COM port (VCOM) at 115200 baud, where the reference firmware runs a CLI.
  The CLI runs a command only when it receives CR (0x0D).
"""

from __future__ import annotations

import glob
import os
import select
import sys
import termios
import time

PYOCD_TARGET = "nrf54l"
BAUD = termios.B115200

# nRF54L15 GPIO and GPIOTE instances (non-secure addresses).
GPIO = {0: 0x5010A000, 1: 0x500D8200}
GPIOTE = {0: (0x5010C000, 269), 1: (0x500DA000, 219)}  # port -> (base, IRQ number)
GPIO_IN, GPIO_LATCH, GPIO_DETECTMODE = 0x0C, 0x20, 0x24
GPIOTE_EVENTS_PORT_SEC = 0x144
NVIC_ISER, NVIC_ISPR = 0xE000E100, 0xE000E200
DHCSR = 0xE000EDF0

# Buttons on the board: name -> (GPIO port, pin). Active low.
BUTTONS = {"BTN1": (1, 13), "BTN2": (1, 9), "BTN3": (1, 8), "BTN4": (0, 4)}


def find_vcom(port: str | None = None) -> str:
    """Return the VCOM device: the argument, $NU54_VCOM, or the only /dev/cu.usbmodem*04.

    The name changes with the USB port, but the DAPLink VCOM interface always ends in 04.
    """
    port = port or os.environ.get("NU54_VCOM")
    if port:
        return port
    found = sorted(glob.glob("/dev/cu.usbmodem*04"))
    if len(found) != 1:
        sys.exit(f"cannot pick the board VCOM automatically (found {found or 'none'}); "
                 "pass --port or set NU54_VCOM")
    return found[0]


class Vcom:
    """Raw, non-blocking access to the board VCOM."""

    def __init__(self, port: str | None = None):
        self.path = find_vcom(port)
        self.fd = os.open(self.path, os.O_RDWR | os.O_NOCTTY | os.O_NONBLOCK)
        attr = termios.tcgetattr(self.fd)
        attr[0] = 0                                   # iflag: no input translation
        attr[1] = 0                                   # oflag: raw output
        attr[2] |= termios.CREAD | termios.CLOCAL     # cflag
        attr[3] = 0                                   # lflag: no echo, no canonical mode
        attr[4] = attr[5] = BAUD
        termios.tcsetattr(self.fd, termios.TCSANOW, attr)

    def read(self, seconds: float) -> str:
        buf = b""
        end = time.time() + seconds
        while time.time() < end:
            ready, _, _ = select.select([self.fd], [], [], 0.05)
            if ready:
                try:
                    buf += os.read(self.fd, 4096)
                except BlockingIOError:
                    pass
        return buf.decode(errors="replace")

    def command(self, line: str, wait: float = 1.0) -> str:
        """Send one CLI command terminated by CR and return what arrives within `wait` seconds."""
        os.write(self.fd, (line + "\r").encode())
        return self.read(wait)

    def close(self) -> None:
        os.close(self.fd)

    def __enter__(self) -> "Vcom":
        return self

    def __exit__(self, *exc) -> None:
        self.close()


def probe_session():
    """Attach to the running target without halting or resetting it."""
    from pyocd.core.helpers import ConnectHelper

    return ConnectHelper.session_with_chosen_probe(
        target_override=PYOCD_TARGET, connect_mode="attach", options={"frequency": 4_000_000})


def pin_cnf(port: int, pin: int) -> int:
    return GPIO[port] + 0x80 + 4 * pin


def kick_gpiote(target, port: int) -> None:
    """Raise the port event and pend its IRQ, so the driver re-reads the pin.

    Changing PIN_CNF from the debugger does not always latch a DETECT event.
    """
    base, irq = GPIOTE[port]
    target.write32(base + GPIOTE_EVENTS_PORT_SEC, 1)
    target.write32(NVIC_ISPR + 4 * (irq // 32), 1 << (irq % 32))
