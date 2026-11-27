"""Drive the independent replacement display from a disposable PTY."""
import os
from pathlib import Path
import pty
import select
import subprocess
import sys
import time

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "recovery-accessory"))
from confirm_board import recovery_body, require_confirmation, signed


def panel(challenge, device, replacement, private_key, result):
    message = recovery_body(bytes.fromhex(challenge), bytes.fromhex(device), bytes.fromhex(replacement))
    require_confirmation()
    proof = signed(Path(private_key), message)
    assert len(proof) == 64
    data = Path(result)
    staged = data.with_suffix(".tmp")
    staged.write_bytes(proof)
    staged.replace(data)


def sign_on_separate_panel(context, board, phone, key, proof):
    master, slave = pty.openpty()
    process = subprocess.Popen(
        [sys.executable, __file__, "panel", context.hex(), board, phone, str(key), str(proof)],
        stdin=slave, stdout=slave, stderr=slave,
    )
    os.close(slave)
    try:
        displayed = b""
        deadline = time.monotonic() + 7
        while b"Type AUTHORIZE to confirm" not in displayed:
            if time.monotonic() >= deadline:
                raise TimeoutError("independent replacement panel did not prompt")
            if select.select([master], [], [], 1)[0]:
                displayed += os.read(master, 4096)
        assert board.encode() in displayed and phone.encode() in displayed
        os.write(master, b"AUTHORIZE\n")
        assert process.wait(timeout=5) == 0
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()
        os.close(master)


def controller(folder, board, phone):
    challenge = folder / "replacement.context"
    proof = folder / "replacement.proof"
    last = b""
    for key in ("wrong-verifier.key", "replacement-verifier.key"):
        deadline = time.monotonic() + 200
        while time.monotonic() < deadline:
            try:
                context = challenge.read_bytes()
            except FileNotFoundError:
                context = b""
            if context and context != last:
                assert len(context) == 130
                sign_on_separate_panel(context, board, phone, folder / key, proof)
                print(f"Independent panel completed {key.removesuffix('.key')} proof", flush=True)
                last = context
                break
            time.sleep(0.1)
        else:
            raise TimeoutError("board did not request independent replacement confirmation")


if __name__ == "__main__":
    os.umask(0o077)
    if sys.argv[1] == "panel":
        panel(*sys.argv[2:])
    else:
        controller(Path(sys.argv[1]), *sys.argv[2:])
