"""Run with the independently installed protocol quote-v2 extra."""

import hashlib
import os
import resource
from pathlib import Path
import subprocess
import tempfile
import unittest

from payment_presentation import DOMAIN, reference, verify_presentation
from coincurve import PrivateKey
from Crypto.Hash import keccak
from nu54_protocol.quote_v2 import QuotePolicy, quote_digest, verify_quote


class PresentationTest(unittest.TestCase):
    def test_valid_pair_from_other_order_is_refused(self):
        vectors = Path(__file__).resolve().parents[3] / "packages/protocol/v2/testdata/quote.vectors"
        _, signed, *unused = vectors.read_text().splitlines()[1].split("|")
        original = bytes.fromhex(signed)
        replacement = bytearray(original)
        replacement[64] ^= 1
        replacement[160] ^= 1
        branch = "0fbc54ef-2827-4ab5-bfcd-3a27611c39ea"
        customer = "1728d4b7-f136-4b87-89cd-26baec26804e"
        sale = "dc624ff0-dbd4-4d8c-9815-5edf72b71443"
        other = "a62c3820-95e4-4559-9cce-218707b51478"
        origin = "22572997-f880-4c99-adbb-7dea0db07cfa"
        with tempfile.TemporaryDirectory() as scratch:
            directory = Path(scratch)
            secret = directory / "test.pem"
            subprocess.run(["openssl", "genpkey", "-algorithm", "ED25519", "-out", secret], check=True)
            public_der = subprocess.check_output(["openssl", "pkey", "-in", secret, "-pubout", "-outform", "DER"])
            public = public_der[-32:].hex()

            def projection(quote, sale_ref, invoice=None):
                digest = quote_digest(quote[:672])
                order_key = quote[160:192]
                generation = int.from_bytes(quote[512:544], "big")
                deadline = int.from_bytes(quote[576:608], "big")
                body = DOMAIN + b"".join(map(reference, (branch, sale_ref, customer, origin)))
                body += b"b" * 32 + order_key + digest + generation.to_bytes(8, "big") + deadline.to_bytes(8, "big")
                payload = directory / "message"
                def sign(value):
                    payload.write_bytes(value)
                    return subprocess.check_output(["openssl", "pkeyutl", "-sign", "-inkey", secret,
                                                    "-rawin", "-in", payload]).hex()
                proof = {"customer": customer, "origin": origin, "claim": (b"b" * 32).hex(),
                         "signature": sign(body)}
                if invoice is not None:
                    cycle = invoice["cycle"].encode()
                    terms = invoice["termsVersion"].encode()
                    extension = b"\x01" + cycle + len(terms).to_bytes(2, "big") + terms + hashlib.sha256(quote).digest()
                    proof["invoice"] = {**invoice, "signature": sign(body + extension)}
                return {"branchRef": branch, "saleRef": sale_ref, "quoteDigest": digest.hex(),
                        "orderKey": order_key.hex(), "expiresAt": deadline, "presentation": proof}

            first = projection(original, sale)
            second = projection(replacement, other)
            clock = first["expiresAt"] - 1
            verify_presentation(original, first, public, branch, customer, sale=sale, now=clock)
            verify_presentation(replacement, second, public, branch, customer, sale=other, now=clock)
            with self.assertRaisesRegex(ValueError, "different checkout"):
                verify_presentation(replacement, second, public, branch, customer, sale=sale, now=clock)
            with self.assertRaisesRegex(ValueError, "Unauthenticated"):
                verify_presentation(replacement, {**second, "saleRef": sale}, public, branch, customer, sale=sale, now=clock)

            invoice = {"cycle": "2026-11-07", "termsVersion": "v2"}
            renewal = projection(original, sale, invoice)
            self.assertEqual(verify_presentation(original, renewal, public, branch, customer,
                                               origin=origin, now=clock, invoice=invoice), invoice)
            # Both quotes carry valid signatures made with fresh synthetic merchant and presenter keys.
            # The only changed unsigned field is the recipient address.
            merchant, payer = PrivateKey(), PrivateKey()

            def address(key):
                return keccak.new(digest_bits=256, data=key.public_key.format(compressed=False)[1:]).digest()[-20:]

            def sign_quote(raw):
                sig = merchant.sign_recoverable(quote_digest(raw), hasher=None)
                return raw + sig[:64] + bytes([sig[64] + 27])

            commissioned = bytearray(original[:672])
            commissioned[300:320] = address(payer)
            commissioned[160:192] = os.urandom(32)
            for offset in (448, 480, 512):
                commissioned[offset:offset + 32] = (1).to_bytes(32, "big")
            good_raw = bytes(commissioned)
            commissioned[371] ^= 1
            wrong_raw = bytes(commissioned)
            self.assertEqual(good_raw[:364], wrong_raw[:364])
            self.assertEqual(good_raw[384:], wrong_raw[384:])
            good, wrong = sign_quote(good_raw), sign_quote(wrong_raw)
            for raw, signed in ((good_raw, good), (wrong_raw, wrong)):
                self.assertEqual(verify_quote(signed, QuotePolicy(raw, address(merchant), clock, False)), 0)
            beneficiary = good_raw[364:384].hex()
            for signed in (good, wrong):
                read = lambda: verify_presentation(signed, projection(signed, sale, invoice), public,
                    branch, customer, origin=origin, now=clock, invoice=invoice, beneficiary=beneficiary)
                if signed == good:
                    self.assertEqual(read(), invoice)
                else:
                    with self.assertRaisesRegex(ValueError, "different beneficiary"):
                        read()

            board = os.getenv("NU54_RECIPIENT_TEST_BOARD")
            if board:
                self.assert_board_rejects_replaced_recipient(directory, board, good, wrong, address(merchant), payer.secret)

            for field, changed in (("cycle", "2026-11-14"), ("termsVersion", "v3")):
                altered = {**invoice, field: changed}
                forged = {**renewal, "presentation": {**renewal["presentation"], "invoice": {**altered, "signature": renewal["presentation"]["invoice"]["signature"]}}}
                with self.assertRaisesRegex(ValueError, "Unauthenticated"):
                    verify_presentation(original, forged, public, branch, customer, origin=origin,
                                        now=clock, invoice=altered)
            with self.assertRaisesRegex(ValueError, "different checkout"):
                verify_presentation(original, renewal, public, branch, other, origin=origin,
                                    now=clock, invoice=invoice)
            with self.assertRaisesRegex(ValueError, "different checkout"):
                verify_presentation(original, renewal, public, branch, customer, origin=other,
                                    now=clock, invoice=invoice)
            with self.assertRaisesRegex(ValueError, "expired"):
                verify_presentation(original, renewal, public, branch, customer, origin=origin,
                                    now=first["expiresAt"], invoice=invoice)
            changed_signature = original[:-1] + bytes([original[-1] ^ 1])
            with self.assertRaisesRegex(ValueError, "Unauthenticated"):
                verify_presentation(changed_signature, renewal, public, branch, customer,
                                    origin=origin, now=clock, invoice=invoice)
            with self.assertRaisesRegex(ValueError, "different invoice"):
                verify_presentation(original, first, public, branch, customer, origin=origin,
                                    now=clock, invoice=invoice)

    def assert_board_rejects_replaced_recipient(self, directory, board, good, wrong, issuer, payer):
        # The display key is generated for this process and deleted with its directory.
        display, public = directory / "display.pem", directory / "display.pub"
        challenge, proof = directory / "challenge", directory / "proof"
        subprocess.run(["openssl", "genpkey", "-algorithm", "EC", "-pkeyopt", "ec_paramgen_curve:P-256",
                        "-out", display], check=True)
        subprocess.run(["openssl", "pkey", "-in", display, "-pubout", "-out", public], check=True)
        challenge.write_bytes(os.urandom(32))
        # Sign the actual display challenge and the approved original wire. The same proof is replayed against the substituted wire.
        statement = b"N54D" + challenge.read_bytes() + good
        der = subprocess.check_output(["openssl", "dgst", "-sha256", "-sign", display], input=statement)
        from coincurve.ecdsa import der_to_cdata, serialize_compact
        proof.write_bytes(serialize_compact(der_to_cdata(der)))
        env = {**os.environ, "NU54_INSTALL_DISPLAY_PROOF": str(proof),
               "NU54_INSTALL_DISPLAY_KEY": str(public), "NU54_INSTALL_DISPLAY_CHALLENGE": str(challenge)}
        for label, wire in (("approved", good), ("substituted", wrong)):
            path = directory / label
            path.write_bytes(wire + issuer + payer)
            attempt = subprocess.run([board, "--approve-one", "management", path], env=env,
                                     capture_output=True, check=False)
            if label == "approved":
                self.assertEqual(attempt.returncode, 0, attempt.stderr)
                self.assertEqual(len(attempt.stdout), 65)
            else:
                self.assertNotEqual(attempt.returncode, 0)
                self.assertIn(b"nu54_pilot_display", attempt.stderr)
                self.assertEqual(attempt.stdout, b"")


if __name__ == "__main__":
    resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
    unittest.main()
