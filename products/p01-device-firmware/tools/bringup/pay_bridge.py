#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["bleak>=0.22"]
# ///
"""BLE transport between a central program and the payment device (payment-protocol.md 3, 4).

The central program (for example packages/device-sim/scripts/rehearse.ts) speaks JSON lines on
stdin/stdout and handles only CBOR message bodies; this bridge does the BLE part: scan for the
payment service, connect, envelope and fragment each body for the negotiated ATT_MTU, and
reassemble the device's notifications.

  stdin   {"send": "<cbor body hex>"}   {"close": true}
  stdout  {"event": "connected", "mtu": 185, "name": "NU54 Signer"}
          {"recv": "<cbor body hex>"}   {"error": "..."}

Usage: uv run pay_bridge.py [--timeout 30]
"""

import argparse
import asyncio
import hashlib
import json
import sys

from bleak import BleakClient, BleakScanner

SERVICE = "6e753534-7061-7900-8000-00805f9b0001"
RX = "6e753534-7061-7900-8000-00805f9b0002"
TX = "6e753534-7061-7900-8000-00805f9b0003"


def out(obj: dict) -> None:
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()


class Reassembler:
    """The receiver rules of payment-protocol.md 4 (same as the shared fragment vectors)."""

    def __init__(self) -> None:
        self.reset()

    def reset(self) -> None:
        self.buf, self.seq, self.index, self.want = b"", None, 0, 0

    def feed(self, frag: bytes) -> bytes | None:
        if len(frag) <= 2:
            raise ValueError("fragment without data")
        if self.seq is None:
            if frag[1] != 0:
                raise ValueError("message does not start at index 0")
            self.seq = frag[0]
        elif frag[0] != self.seq or frag[1] != self.index:
            raise ValueError("fragment out of order")
        self.buf += frag[2:]
        self.index += 1
        if not self.want and len(self.buf) >= 2:
            self.want = int.from_bytes(self.buf[:2], "big")
        if self.want and len(self.buf) >= self.want:
            env = self.buf
            self.reset()
            body = env[10:]
            if len(env) != int.from_bytes(env[:2], "big") or hashlib.sha256(body).digest()[:8] != env[2:10]:
                raise ValueError("bad envelope")
            return body
        return None


async def main(timeout: float) -> int:
    dev = await BleakScanner.find_device_by_filter(
        lambda d, adv: SERVICE in [u.lower() for u in adv.service_uuids], timeout=timeout)
    if dev is None:
        out({"error": "no device advertising the payment service (long-press SW4 for payment mode)"})
        return 1
    rx = Reassembler()
    seq = 0
    async with BleakClient(dev) as client:
        def on_notify(_h, data: bytearray) -> None:
            try:
                body = rx.feed(bytes(data))
            except ValueError as e:
                out({"error": f"BAD_FRAME from device: {e}"})
                return
            if body is not None:
                out({"recv": body.hex()})

        await client.start_notify(TX, on_notify)
        await asyncio.sleep(0.5)  # let the device see the notification subscription first
        mtu = client.mtu_size
        out({"event": "connected", "mtu": mtu, "name": dev.name})
        loop = asyncio.get_running_loop()
        reader = asyncio.StreamReader()
        await loop.connect_read_pipe(lambda: asyncio.StreamReaderProtocol(reader), sys.stdin)
        while True:
            line = await reader.readline()
            if not line:
                break
            cmd = json.loads(line)
            if cmd.get("close"):
                break
            body = bytes.fromhex(cmd["send"])
            env = (10 + len(body)).to_bytes(2, "big") + hashlib.sha256(body).digest()[:8] + body
            size = mtu - 5
            for i, off in enumerate(range(0, len(env), size)):
                await client.write_gatt_char(RX, bytes([seq, i]) + env[off:off + size], response=True)
            seq = (seq + 1) & 0xFF
        await client.stop_notify(TX)
    return 0


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("--timeout", type=float, default=30.0, help="seconds to scan for the device")
    raise SystemExit(asyncio.run(main(ap.parse_args().timeout)))
