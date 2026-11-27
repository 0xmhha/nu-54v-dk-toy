"""Independent display rejects substitutions before signing a policy statement."""
import contextlib
import importlib.util
import io
from pathlib import Path
import unittest
import tempfile
from cryptography.hazmat.primitives.asymmetric import ec, utils
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.exceptions import InvalidSignature

path = Path(__file__).resolve().parents[1] / 'recovery-accessory' / 'confirm_board.py'
spec = importlib.util.spec_from_file_location('policy_display', path)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

def binding(operation, expires=87000):
    return {"type": "nu54-owner-display-v1", "operation": operation, "session": "02"*16,
            "request": "03"*16, "expires": str(expires)}


class PolicyDisplayTest(unittest.TestCase):
    def test_exact_independent_policy_and_mutations(self):
        num = lambda value: value.to_bytes(8, 'big')
        board, asset, target = b'\x01'*32, b'\x04'*20, b'\x05'*20
        body = (b'N54P' + board + b'\x02'*16 + b'\x03'*16 + b'\x04'*32 + num(7) + num(1) + num(8283)
                + asset + target + target + num(10000000) + num(30000000) + num(12000000)
                + num(30000000) + num(173400) + num(87000))
        expected = dict(board=board.hex(), generation=7, chain=8283, asset=asset.hex(), destination=target.hex(),
                        per_transfer=12000000, daily=30000000, previous_destination=target.hex(),
                        previous_per_transfer=10000000, previous_daily=30000000, revision=1, not_before=173400, expires=87000, kind='change')
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(module.policy_body(body, expected, 86410), body)
            self.assertEqual(module.owner_display_body("policy", binding(24), b"\x04"*32, expected, 86900), body)
            for wrong in (body, bytes(32), b"\x04"*31):
                with self.assertRaises(ValueError):
                    module.owner_display_body("policy", binding(24), wrong, expected, 86900)
            with self.assertRaises(ValueError):
                module.owner_display_body("policy", binding(24), b"\x04"*32, expected, 87000)
            with self.assertRaises(ValueError):
                module.owner_display_body("policy", binding(26), b"\x04"*32, expected, 86900)

            with tempfile.TemporaryDirectory() as directory:
                key=ec.generate_private_key(ec.SECP256R1())
                key_path=Path(directory)/'display.pem'
                key_path.write_bytes(key.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption()))
                signature=module.signed(key_path,body)
                der=utils.encode_dss_signature(int.from_bytes(signature[:32],'big'),int.from_bytes(signature[32:],'big'))
                key.public_key().verify(der,body,ec.ECDSA(hashes.SHA256()))
                with self.assertRaises(InvalidSignature): key.public_key().verify(der,body[:-1]+bytes([body[-1]^1]),ec.ECDSA(hashes.SHA256()))
            for offset in (4,100,116,124,164,184,200,216):
                tampered = bytearray(body); tampered[offset] ^= 1
                with self.assertRaises(ValueError): module.policy_body(bytes(tampered), expected, 86410)
            with self.assertRaises(ValueError): module.policy_body(body, expected, 87000)


    def test_recipient_and_cap_statements_preserve_independent_current_values(self):
        num = lambda value: value.to_bytes(8, 'big')
        challenge = b"\x04" * 32
        addresses = [bytes([index]) * 20 for index in range(1, 9)]
        encoded = [address.hex() for address in addresses]
        board, asset = b"\x01" * 32, b"\x04" * 20
        prefix = (board + b"\x02"*16 + b"\x03"*16 + challenge + num(7) + num(4) + num(8283) +
                  asset + num(10) + num(30) + num(173400) + num(87000))
        singleton = b"\x01" + addresses[0] + bytes(140)
        full = b"\x08" + b"".join(addresses)
        original = dict(board=board.hex(), asset=asset.hex(), generation=7, revision=4, chain=8283,
                        previous_recipients=encoded[:1], recipients=encoded, previous_per_transfer=10,
                        previous_daily=30, per_transfer=10, daily=30)
        cap = dict(original, previous_recipients=encoded, per_transfer=8, daily=20)
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(module.owner_display_body("policy", binding(33), challenge, original, 86900),
                             b"N54R" + prefix + singleton + full)
            self.assertEqual(module.owner_display_body("policy", binding(34), challenge, cap, 86900),
                             b"N54M" + prefix + full + full + num(8) + num(20))
            for invalid in (dict(original, recipients=encoded[::-1]),
                            dict(original, recipients=encoded[:7] + encoded[:1]),
                            dict(original, daily=21), dict(original, asset="04"),
                            dict(original, revision=0)):
                with self.assertRaises(ValueError):
                    module.owner_display_body("policy", binding(33), challenge, invalid, 86900)
            for invalid in (dict(cap, recipients=encoded[1:]), dict(cap, per_transfer=10, daily=30)):
                with self.assertRaises(ValueError):
                    module.owner_display_body("policy", binding(34), challenge, invalid, 86900)
            with self.assertRaises(ValueError):
                module.owner_display_body("policy", binding(34), challenge, cap, 87000)

    def test_initial_policy_is_explicit_and_has_no_previous_values(self):
        num = lambda value: value.to_bytes(8, 'big')
        board, asset, target = b'\x01'*32, b'\x04'*20, b'\x05'*20
        body = (b'N54I' + board + b'\x02'*16 + b'\x03'*16 + b'\x04'*32 + num(7) + num(1) + num(8283)
                + asset + bytes(20) + target + num(0) + num(0) + num(10000000)
                + num(30000000) + num(0) + num(87000))
        expected = dict(board=board.hex(), generation=7, chain=8283, asset=asset.hex(), destination=target.hex(),
                        per_transfer=10000000, daily=30000000, previous_destination=bytes(20).hex(),
                        previous_per_transfer=0, previous_daily=0, revision=1, not_before=0, expires=87000, kind='initialize')
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(module.policy_body(body, expected, 86410), body)
            self.assertEqual(module.owner_display_body("policy", binding(26), b"\x04"*32, expected, 86900), body)
            tampered = bytearray(body); tampered[151] = 1
            with self.assertRaises(ValueError): module.policy_body(bytes(tampered), expected, 86410)

class FinalityDisplayTest(unittest.TestCase):
    def test_requires_exact_independent_verifier_profile(self):
        num=lambda value:value.to_bytes(8,"big")
        board,signer,genesis,asset,destination,key=(b"\x01"*32,b"\x05"*20,b"\x06"*32,b"\x07"*20,b"\x08"*20,b"\x04"+b"\x09"*64)
        body=(b"N54G"+board+num(7)+b"\x02"*16+b"\x03"*16+b"\x04"*32+num(8283)+signer+genesis+asset+destination+num(10)+num(30)+key+num(87000))
        expected=dict(board=board.hex(),generation=7,chain=8283,signer=signer.hex(),genesis=genesis.hex(),asset=asset.hex(),destination=destination.hex(),per_transfer=10,daily=30,finality_key=key.hex(),expires=87000)
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(module.finality_enrollment_body(body,expected,86410),body)
            self.assertEqual(module.owner_display_body("finality-enroll", binding(28, 86999), b"\x04"*32, expected, 86900), body)
            with self.assertRaises(ValueError):
                module.owner_display_body("finality-enroll", binding(28, 87001), b"\x04"*32, expected, 86900)

            for offset in (4,36,108,116,136,168,188,208,216,224,289):
                altered=bytearray(body);altered[offset]^=1
                with self.assertRaises(ValueError):module.finality_enrollment_body(bytes(altered),expected,86410)
            with self.assertRaises(ValueError):module.finality_enrollment_body(body,expected,87000)

if __name__ == '__main__': unittest.main()
