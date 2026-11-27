"""Separate display test peer for the signed W27 simulator flow."""
import importlib.util
import json
import os
from pathlib import Path
import sys
import time

path = Path(__file__).resolve().parents[1] / "recovery-accessory" / "confirm_board.py"
spec = importlib.util.spec_from_file_location("confirm_board", path)
display = importlib.util.module_from_spec(spec)
spec.loader.exec_module(display)

root = Path(sys.argv[1])
challenge = (root / "policy-challenge").read_bytes()
operation = sys.argv[2]
initial = operation == "initialize"
path = root / "policy-private-input"
for _ in range(450):
    if path.is_file():
        break
    time.sleep(0.1)
else:
    raise SystemExit("No private display input received")
raw = path.read_text().strip()
path.unlink()
binding = display.parse_owner_display_code(raw)
target = "05" * 20
expected = {
    "board": (root / "policy-board").read_text().strip(),
    "generation": 1,
    "chain": 8283,
    "asset": (root / "policy-asset").read_text().strip(),
    "destination": target,
    "per_transfer": 10_000_000 if initial else 12_000_000,
    "daily": 30_000_000,
    "previous_destination": "00" * 20 if initial else target,
    "previous_per_transfer": 0 if initial else 10_000_000,
    "previous_daily": 0 if initial else 30_000_000,
    "revision": 1,
    "kind": "initialize" if initial else "change",
}
if operation in {"recipients", "caps"}:
    expected = json.loads((root / "policy-set-expect.json").read_text())
    if operation == os.environ.get("NU54_W27_PANEL_REJECT_ONCE") and not (root / "policy-wrong-statement-tested").exists():
        (root / "policy-wrong-statement-tested").touch()
        expected["recipients"] = expected["recipients"][:-1]
        print("DISPLAY TEST signs a different proposed set", flush=True)
try:
    clock = int(os.environ.get("NU54_W27_PANEL_CLOCK", time.time()))
    body = display.owner_display_body("policy", binding, challenge, expected, clock)
except ValueError as error:
    print(f"DISPLAY DECLINED {error}", flush=True)
    raise SystemExit(1) from error

proof = display.signed(root / "policy-display.key", body)
if operation == "change" and not (root / "policy-bad-proof-tested").exists():
    (root / "policy-bad-proof-tested").touch()
    proof = bytes([proof[0] ^ 1]) + proof[1:]
    print("DISPLAY TEST corrupted signature", flush=True)
(root / "policy-display-proof").write_bytes(proof)
