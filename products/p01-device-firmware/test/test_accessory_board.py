"""W05 host test with distinct durable P01 C slots and the independent accessory executable."""
import hashlib
import os
from pathlib import Path
import json
import struct
import subprocess
import sys
import tempfile
import time

from test_accessory_transport import exchange, identities


def context(identity, generation):
    return (b"\x05" + os.urandom(32) + os.urandom(16) + identity + os.urandom(32) + os.urandom(16)
            + struct.pack(">QQ", generation, int(time.time()) + 180))


def frame(packet):
    return struct.pack(">H", len(packet)) + packet


def exercise(board, accessory, folder, enrolled=None, opened=None, retain=False):
    identities(folder, enrolled)
    pins = [folder / f"{role}.pem" for role in ("source", "target", "accessory")]
    source_id = opened[49:81] if opened is not None else hashlib.sha256(pins[0].read_bytes()).digest()
    target_id = hashlib.sha256(pins[1].read_bytes()).digest()
    assert source_id != target_id
    seal_key = os.urandom(32)
    if retain:
        (folder / "seal.key").write_bytes(seal_key)
    paths = {"source": enrolled or (folder / "source-vault"), "target": folder / "target-vault"}
    for path in paths.values():
        path.mkdir(exist_ok=True)
    source, target = opened or context(source_id, 1), context(target_id, 2)

    def device(role, operation, *packets, succeeds=True):
        command = [board, paths[role], operation]
        if operation != "inspect":
            command.extend([role, *pins, folder / f"{role}.key"])
        process = subprocess.run(command, input=b"".join(map(frame, packets)), capture_output=True, timeout=6)
        assert process.returncode == (0 if succeeds else 1), (role, operation, process.returncode, process.stderr)
        return process.stdout

    def accessory_session(role, action, key=seal_key):
        return exchange(accessory, folder, role, action, source_id=source_id, target_id=target_id, seal_key=key)

    def select_backup(peer, confirm, *, approve=True, mark=False):
        peer.packet(b"\x06" + source[1:])
        confirm(approve)
        if not approve:
            return
        assert peer.packet() == b"\x0a"
        capsule = device("source", "capsule", source)
        peer.packet(capsule)
        status = peer.packet()
        if mark:
            assert status[0] == 8
            # An invalid accessory signature must leave the source address gated.
            damaged = bytearray(status[1:])
            accessory_signature = 318 + 1 + damaged[318] + 1
            damaged[accessory_signature] ^= 1
            device("source", "mark", target, bytes(damaged), succeeds=False)
            assert device("source", "inspect") == b"LOCKED\n"
            device("source", "mark", target, status[1:])
        else:
            assert status == b"\x07"

    assert device("source", "inspect") == device("target", "inspect") == b"LOCKED\n"
    assert accessory_session("source", lambda peer, confirm: select_backup(peer, confirm, approve=False))
    assert not (folder / "capsule.bin").exists() and not (paths["source"] / "hd-seed").exists()
    assert accessory_session("swapped", None) != 0
    assert accessory_session("source", select_backup) == 0
    saved = (folder / "capsule.bin").read_bytes()
    original_seed = (paths["source"] / "hd-seed").read_bytes()
    assert original_seed not in saved
    assert device("source", "inspect") == b"LOCKED\n"

    def restore(peer, confirm, *, fail_seal=False, wrong_root=False, drop_before_commit=False):
        peer.packet(target)
        confirm()
        if fail_seal:
            return
        capsule = peer.packet()
        if wrong_root:
            # Keep the source-signed claim and change only the candidate seed in the disposable target.
            rejected = capsule[:-1] + bytes((capsule[-1] ^ 1,))
            assert device("target", "partial", target, rejected, succeeds=False) == b""
            # Tell the panel this target session ended without a proof.
            peer.packet(b"\x00")
            return
        partial = device("target", "partial", target, capsule)
        peer.packet(partial)
        proof = peer.packet()
        assert proof.startswith(partial)
        assert not (folder / "proof.bin").exists()
        assert device("target", "inspect") == b"LOCKED\n"
        if drop_before_commit:
            peer.packet(b"\x09" + os.urandom(32))
            return
        ack = device("target", "commit", target, capsule, proof)
        assert ack == b"\x09" + hashlib.sha256(proof).digest()
        peer.packet(ack)

    assert accessory_session("target", lambda peer, confirm: restore(peer, confirm, fail_seal=True), os.urandom(32)) != 0
    assert device("target", "inspect") == b"LOCKED\n"
    assert accessory_session("target", lambda peer, confirm: restore(peer, confirm, wrong_root=True)) != 0
    assert not (paths["target"] / "hd-seed").exists() and not (folder / "proof.bin").exists()
    assert device("source", "inspect") == device("target", "inspect") == b"LOCKED\n"
    assert (paths["source"] / "hd-seed").read_bytes() == original_seed
    assert accessory_session("target", lambda peer, confirm: restore(peer, confirm, drop_before_commit=True)) != 0
    assert not (folder / "proof.bin").exists()
    assert device("source", "inspect") == device("target", "inspect") == b"LOCKED\n"
    assert accessory_session("target", restore) == 0
    assert (folder / "proof.bin").exists()
    assert original_seed not in (folder / "proof.bin").read_bytes()
    assert device("source", "inspect") == b"LOCKED\n"
    assert accessory_session("source", lambda peer, confirm: select_backup(peer, confirm, mark=True)) == 0
    first_receive = device("source", "inspect")
    assert len(first_receive) == 20 and first_receive == device("target", "inspect")
    assert (paths["source"] / "hd-seed").read_bytes() == (paths["target"] / "hd-seed").read_bytes()
    return "0x" + first_receive.hex()


def wait_log(transcript, marker, timeout=220):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if transcript.exists() and marker in transcript.read_text():
            return
        time.sleep(0.25)
    raise TimeoutError(f"application did not reach {marker}")


def live_app(board, accessory, enrolled, transcript, evidence, kit=None):
    wait_log(transcript, "BACKUP_PENDING ")
    opened = (enrolled / "backup.context").read_bytes()
    assert len(opened) == 145 and opened[0] == 5
    if kit:
        kit.mkdir(mode=0o700)
        address = exercise(board, accessory, kit, enrolled, opened, retain=True)
    else:
        with tempfile.TemporaryDirectory(prefix="nu54-accessory-live-") as directory:
            address = exercise(board, accessory, Path(directory), enrolled, opened)
    evidence.write_text(json.dumps({"address": address, "request": opened[33:49].hex()}))


def restore_app(board, accessory, kit, target, transcript, source_id, expected_address):
    wait_log(transcript, "RESTORE_PENDING ")
    opened = (target / "restore.context").read_bytes()
    assert len(opened) == 145 and opened[0] == 5
    pins = [kit / f"{role}.pem" for role in ("source", "target", "accessory")]
    receipt = b""

    def run_board(operation, *packets):
        command = [board, target, operation]
        if operation != "inspect":
            command.extend(["target", *pins, target / "board.key"])
        return subprocess.run(command, input=b"".join(map(frame, packets)), capture_output=True,
                              timeout=6, check=True).stdout

    def restore(peer, confirm):
        nonlocal receipt
        assert run_board("inspect") == b"LOCKED\n"
        peer.packet(opened)
        confirm()
        capsule = peer.packet()
        peer.packet(run_board("partial", opened, capsule))
        receipt = peer.packet()
        assert (target / "restore.context").read_bytes() == opened
        ack = run_board("commit", opened, capsule, receipt)
        assert ack == b"\x09" + hashlib.sha256(receipt).digest()
        peer.packet(ack)

    assert exchange(accessory, kit, "target", restore, source_id=bytes.fromhex(source_id),
                    target_id=opened[49:81], seal_key=(kit / "seal.key").read_bytes(),
                    board_key=target / "board.key") == 0
    assert (kit / "proof.bin").exists()
    assert run_board("inspect").hex() == expected_address.removeprefix("0x")

    def publish(frame):
        staged = target / "restore.tmp"
        staged.write_bytes(frame)
        staged.replace(target / "restore.receipt")

    # Deliver a deliberate corrupted copy first. The accessory and target kept the authentic receipt.
    damaged = bytearray(receipt)
    damaged[319] ^= 1
    publish(damaged)
    wait_log(transcript, "RESTORE_RECEIPT ", timeout=100)
    publish(receipt)


if __name__ == "__main__":
    os.umask(0o077)
    board, accessory = (Path(arg).resolve() for arg in sys.argv[1:3])
    if len(sys.argv) == 6 or len(sys.argv) == 7:
        live_app(board, accessory, *(Path(arg).resolve() for arg in sys.argv[3:]))
    elif len(sys.argv) == 9 and sys.argv[3] == "restore":
        restore_app(board, accessory, *(Path(arg).resolve() for arg in sys.argv[4:7]), *sys.argv[7:])
    else:
        with tempfile.TemporaryDirectory(prefix="nu54-accessory-board-") as directory:
            exercise(board, accessory, Path(directory))
