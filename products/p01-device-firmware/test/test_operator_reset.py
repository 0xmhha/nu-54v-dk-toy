"""enroll_board.py for the spending profile against the board's own checks, with throwaway keys."""

import hashlib
import json
from pathlib import Path
import re
import secrets
import subprocess
import sys
import tempfile
import time
import unittest

from cryptography import x509
from cryptography.hazmat.primitives import serialization
from cryptography.x509.oid import NameOID

from test_runtime_enrollment import SCRIPTS, cert, key
from extract_c import definition
from enroll_board import point, private_key, reset_request, signed, spki

ROOT = Path(__file__).resolve().parents[3]
TOOL = [sys.executable, str(SCRIPTS / "enroll_board.py")]


def board(*arguments):
    return subprocess.run([RUNNER, *map(str, arguments)], check=True, capture_output=True, text=True).stdout.strip()


def save(path, value):
    path.touch(mode=0o600)
    path.write_bytes(value.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8,
                                         serialization.NoEncryption()))
    return path


def public(path, value):
    path.write_bytes(value.public_key().public_bytes(serialization.Encoding.PEM,
                                                     serialization.PublicFormat.SubjectPublicKeyInfo))
    return path


class Operator(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix="nu54-operator-")
        self.addCleanup(directory.cleanup)
        self.paths = Path(directory.name)
        authority = key()
        self.authority = save(self.paths / "authority.pem", authority)
        self.pin = point(authority.public_key()).hex()


class ResetTest(Operator):
    def setUp(self):
        super().setUp()
        self.identity = secrets.token_hex(20)
        self.generation = 1
        # The phone builds its request from the owner status answer, exactly as readStatus does.
        status = bytes.fromhex(board("status", self.identity, self.generation))
        self.assertEqual(self.generation, int.from_bytes(status[5:13], "big"))
        self.device = status[13:45].hex()
        self.nonce = secrets.token_hex(32)

    def request(self, **changed):
        parts = {"device": self.device, "generation": self.generation, "nonce": self.nonce, **changed}
        return f"NU54-RESET1:{parts['device']}:{parts['generation']}:{parts['nonce']}"

    def sign(self, request=None, *extra, authority=None, typed=None):
        return subprocess.run([*TOOL, "operator-reset", request or self.request(),
                               "--authority", str(authority or self.authority), *extra],
                              capture_output=True, text=True, input=typed or "")

    def answer(self, request=None, **options):
        done = self.sign(request, typed="RESET\n", **options)
        self.assertEqual(0, done.returncode, done.stderr)
        self.assertRegex(done.stdout, r"\ANU54-RESET1-OK:[0-9a-f]{64}:[0-9a-f]{128}\n\Z")
        return done.stdout.strip().split(":")[1:]

    def verdict(self, answer, identity=None, generation=None, live=None, pin=None):
        return board("reset", pin or self.pin, identity or self.identity, generation or self.generation,
                     live or self.nonce, *answer)

    def test_authority_pin_is_the_key_the_board_imports(self):
        path = public(self.paths / "authority.pub", private_key(self.authority))
        self.assertEqual(self.pin, subprocess.run([*TOOL, "authority-pin", str(path)], check=True,
                                                  capture_output=True, text=True).stdout.strip())

    def test_the_phone_sees_the_board_id_in_the_statement(self):
        self.assertEqual(hashlib.sha256(bytes.fromhex(self.identity)).hexdigest(), self.device)

    def test_the_board_accepts_the_signature(self):
        answer = self.answer()
        self.assertEqual(self.nonce, answer[0])
        self.assertEqual("accepted", self.verdict(answer))

    def test_the_board_accepts_the_largest_generation(self):
        generation = (1 << 64) - 1
        answer = self.answer(self.request(generation=generation))
        self.assertEqual("accepted", self.verdict(answer, generation=generation))

    def test_the_board_refuses_anything_else(self):
        answer = self.answer()
        self.assertEqual("refused", self.verdict(answer, identity=secrets.token_hex(20)))
        self.assertEqual("refused", self.verdict(answer, generation=2))
        self.assertEqual("refused", self.verdict(answer, live=secrets.token_hex(32)))
        other = secrets.token_hex(32)
        self.assertEqual("refused", self.verdict([other, answer[1]], live=other))
        self.assertEqual("refused", self.verdict(answer, pin=point(key().public_key()).hex()))
        wrong = self.answer(authority=save(self.paths / "other.pem", key()))
        self.assertEqual("refused", self.verdict(wrong))
        damaged = answer[1][:-1] + ("0" if answer[1][-1] != "0" else "1")
        self.assertEqual("refused", self.verdict([answer[0], damaged]))

    def test_the_one_time_value_outlives_a_session_but_not_a_restart(self):
        source = (ROOT / "products/p01-device-firmware/app/src/device_pilot.c").read_text()
        self.assertNotIn("reset_nonce", definition(source, "reset_link"))
        uses = re.findall(r"^.*\breset_nonce\w*.*$", source, re.MULTILINE)
        self.assertFalse([line for line in uses if "psa_" in line or "settings_" in line])

    def test_a_signature_for_another_purpose_is_refused(self):
        statement = bytes.fromhex(self.device) + (1).to_bytes(8, "big") + bytes.fromhex(self.nonce)
        for body in (statement, b"NU54IT01" + statement, b"NU54 operator reset v2".ljust(24, b"\0") + statement):
            self.assertEqual("refused", self.verdict([self.nonce, signed(self.authority, body).hex()]))

    def test_confirmation(self):
        for typed in ("", "\n", "reset\n", "yes\n", "RESET now\n"):
            with self.subTest(typed=typed):
                done = self.sign(typed=typed)
                self.assertNotEqual(0, done.returncode)
                self.assertEqual("", done.stdout)
        done = self.sign(typed="RESET\n")
        self.assertIn(self.device, done.stderr)
        self.assertIn("Generation: 1\n", done.stderr)

    def test_without_a_question(self):
        done = self.sign(None, "--yes", "--device", self.device)
        self.assertEqual("accepted", self.verdict(done.stdout.strip().split(":")[1:]))
        for extra in (("--yes",), ("--yes", "--device", "11" * 32), ("--device", "11" * 32)):
            with self.subTest(extra=extra):
                done = self.sign(None, *extra, typed="RESET\n")
                self.assertNotEqual(0, done.returncode)
                self.assertEqual("", done.stdout)

    def test_strict_parsing(self):
        self.assertEqual((bytes.fromhex(self.device), 1, bytes.fromhex(self.nonce)), reset_request(self.request()))
        good = self.request()
        bad = [good + "\n", " " + good, good + ":", good.upper(), good.replace("RESET1", "RESET2"),
               good.replace(":", ";"), "", good[:-1], good + "0", "x" + good,
               self.request(generation="01"), self.request(generation="0"), self.request(generation="-1"),
               self.request(generation="+1"), self.request(generation="0x1"), self.request(generation=1 << 64),
               self.request(generation="١"), self.request(generation="1_0"),
               self.request(device=self.device.upper()[:-1] + "A"), self.request(device="00" * 32),
               self.request(nonce="00" * 32), self.request(nonce=self.nonce[:-2])]
        for text in bad:
            with self.subTest(text=text), self.assertRaises(ValueError):
                reset_request(text)

    def test_keys_inside_the_checkout_are_refused(self):
        with tempfile.TemporaryDirectory(dir=ROOT / "products/p01-device-firmware/test", prefix=".no-key-") as inside:
            done = self.sign(authority=Path(inside) / "missing.pem", typed="RESET\n")
        self.assertNotEqual(0, done.returncode)
        self.assertIn("outside the checkout", done.stderr)
        self.assertEqual("", done.stdout)

    def test_other_key_types_are_refused(self):
        from cryptography.hazmat.primitives.asymmetric import ec
        done = self.sign(authority=save(self.paths / "k1.pem", ec.generate_private_key(ec.SECP256K1())), typed="RESET\n")
        self.assertNotEqual(0, done.returncode)
        self.assertEqual("", done.stdout)


class GrantTest(Operator):
    """One board over several boots, an operator reset and a new owner."""

    def setUp(self):
        super().setUp()
        ca = key()
        issuer = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "P01 local test CA")])
        self.ca = self.paths / "ca.pem"
        self.ca.write_bytes(cert(issuer, issuer, ca.public_key(), ca, root=True))
        self.board = key()
        self.board_key = save(self.paths / "board.key", self.board)
        self.leaf = self.paths / "board.pem"
        self.leaf.write_bytes(cert(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "P01 test board")]), issuer,
                                   self.board.public_key(), ca))
        self.keys = {role: public(self.paths / f"{role}.pub", key()) for role in ("display", "recovery")}
        self.payer, self.device = secrets.token_bytes(20), secrets.token_bytes(32)
        self.boots = 0

    def boot(self, *extra, check=True, keys=None, **changed):
        """A new boot offers a new challenge, and the issuer answers it with a grant."""
        self.boots += 1
        offer, grant, policy = (self.paths / f"{name}{self.boots}" for name in ("offer", "grant", "policy"))
        body = b"N54E" + secrets.token_bytes(32) + point(self.board.public_key()) + self.payer + self.device
        offer.write_bytes(body + signed(self.board_key, body))
        policy.write_text(json.dumps({
            "revision": 1, "payer_generation": 1, "not_after": int(time.time()) + 1800, "max_amount": 30_000_000,
            "payer_role": 1, "merchant_key_version": 1, "beneficiary_version": 1, "contract": "11" * 20,
            "token": "22" * 20, "beneficiary": "33" * 20, "merchant_signer": "44" * 20, "merchant": "55" * 32,
            "owner": "66" * 32, "terminal": "77" * 32, **changed}))
        keys = {**self.keys, **(keys or {})}
        done = subprocess.run([*TOOL, "issue", "--commercial-testnet", *extra, "--offer", str(offer),
                               "--policy", str(policy), "--ca", str(self.ca), "--board-certificate", str(self.leaf),
                               "--display-public", str(keys["display"]), "--recovery-public", str(keys["recovery"]),
                               "--board-pin", hashlib.sha256(spki(self.board.public_key())).hexdigest(),
                               "--device", self.device.hex(), "--authority", str(self.authority), "--out", str(grant)],
                              check=check, capture_output=True, text=True)
        return (offer, grant) if check else done.returncode

    def installed(self, *pairs):
        return board("grants", self.pin, *(path for pair in pairs for path in pair)).splitlines()

    def test_grants_over_a_reset(self):
        # The registration number the board itself gives: first owner 1, reset 2, next owner 3.
        self.assertEqual(["1", "2", "3", "4"], board("generations").split())
        first = self.boot("--spending")
        self.assertEqual(b"N54Q", first[1].read_bytes()[:4])
        second_boot = self.boot("--spending", revision=2)
        after_reset = self.boot("--spending", revision=3, payer_generation=3, owner="88" * 32)
        third_boot = self.boot("--spending", revision=4, payer_generation=3, owner="88" * 32)
        line = "role=1 testnet=1 generation={} cap=30000000".format
        self.assertEqual([line(1), line(1), line(3), line(3)],
                         self.installed(first, second_boot, after_reset, third_boot))

    def test_a_grant_is_for_one_boot(self):
        first, again = self.boot("--spending"), self.boot("--spending", revision=2)
        self.assertEqual("refused", self.installed(first, (again[0], first[1]))[1])

    def test_no_return_to_the_previous_owner(self):
        kept = self.boot("--spending", revision=3, payer_generation=3)
        for changed in ({"revision": 4, "payer_generation": 1}, {"revision": 2, "payer_generation": 3}):
            with self.subTest(changed=changed):
                self.assertEqual("refused", self.installed(kept, self.boot("--spending", **changed))[1])

    def test_a_changed_policy_needs_a_new_revision(self):
        first = self.boot("--spending")
        self.assertEqual("refused", self.installed(first, self.boot("--spending", payer_generation=3))[1])

    def test_the_recovery_key_outlives_a_reset(self):
        first = self.boot("--spending")
        other = {"recovery": public(self.paths / "recovery2.pub", key())}
        changed = self.boot("--spending", revision=2, payer_generation=3, keys=other)
        self.assertEqual("refused", self.installed(first, changed)[1])

    def test_policies_the_profile_cannot_use_are_refused(self):
        self.assertNotEqual(0, self.boot("--spending", check=False, payer_role=2))
        self.assertNotEqual(0, self.boot("--spending", check=False, max_amount=30_000_001))
        self.assertEqual(0, self.boot("--spending", check=False))

    def test_the_spending_check_needs_the_testnet_grant(self):
        offer, _ = self.boot("--spending")
        done = subprocess.run([*TOOL, "issue", "--spending", "--offer", str(offer), "--policy", str(self.paths / "policy1"),
                               "--ca", str(self.ca), "--board-certificate", str(self.leaf),
                               "--display-public", str(self.keys["display"]), "--recovery-public", str(self.keys["recovery"]),
                               "--board-pin", hashlib.sha256(spki(self.board.public_key())).hexdigest(),
                               "--device", self.device.hex(), "--authority", str(self.authority),
                               "--out", str(self.paths / "none")], capture_output=True, text=True)
        self.assertIn("--commercial-testnet", done.stderr)
        self.assertFalse((self.paths / "none").exists())


if __name__ == "__main__":
    RUNNER = str(Path(sys.argv.pop(1)).resolve())
    unittest.main()
