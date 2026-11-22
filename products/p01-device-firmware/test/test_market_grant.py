"""Independent provisioning, typed quote and display checks for lab board approval."""

import tempfile
import sys
from pathlib import Path
import unittest

from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, utils

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
from market_grant import APPROVE, EXECUTE, S_LIMIT, digest, display, issue, keccak256  # noqa: E402


def portal_fixture():
    clock = 1800000001
    public = lambda key: key.public_key().public_bytes(serialization.Encoding.X962, serialization.PublicFormat.UncompressedPoint)
    oracle = ec.derive_private_key(17, ec.SECP256K1())
    authority = ec.derive_private_key(19, ec.SECP256R1())
    board, code, genesis, deployment, user = (bytes([i]) * size for i, size in zip(range(1, 6), (32,32,32,32,20)))
    venue, base, quote_token = (bytes([i]) * 20 for i in range(6, 9))
    quote_key=public(oracle)
    signer=keccak256(quote_key[1:])[12:]
    scope=venue+base+quote_token+signer+code
    word=lambda n: n.to_bytes(32,"big")
    q=EXECUTE+user.rjust(32,b"\0")+b"".join(word(n) for n in (0,100_000_000,0,1_000_000,0,2,9,clock+35,320,65))
    raw=oracle.sign(digest(scope,q),ec.ECDSA(utils.Prehashed(hashes.SHA256())))
    r,s=utils.decode_dss_signature(raw)
    q+=r.to_bytes(32,"big")+min(s,2*S_LIMIT+1-s).to_bytes(32,"big")+bytes([27])+bytes(31)
    params={"token":"a"*64,"venue":venue.hex(),"genesis":genesis.hex(),"deployment":deployment.hex(),"user":user.hex(),
            "to":base.hex(),"nonce":"2","gasPrice":"1000000000","quote":q.hex(),
            "allowance":(APPROVE+venue.rjust(32,b"\0")+word(100_000_000)).hex()}
    profile={"leaseRevision":4,"expires":clock+200,"device":board.hex(),"generation":1,"market":{
        "sender":user.hex(),"venue":venue.hex(),"base":base.hex(),"quoteToken":quote_token.hex(),"quoteSigner":signer.hex(),
        "codeHash":code.hex(),"genesis":genesis.hex(),"deployment":deployment.hex(),"quotePublicKey":quote_key.hex(),"feeLimit":10**15}}
    return clock, board, user, authority, profile, params


class TestIsolatedBoardGrant(unittest.TestCase):
    def test_report_exact_allowance_then_reject_unreviewed_terms(self):
        clock, board, user, authority, profile, params=portal_fixture()
        with tempfile.TemporaryDirectory() as d:
            key=Path(d)/"authority.pem"
            key.write_bytes(authority.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption()))
            key.chmod(0o600)
            self.check_grant(clock,board,user,authority,key,profile,params)

    def check_grant(self,clock,board,user,authority,key,profile,params):
        expires, encoding=issue(profile, params, clock, key)
        wire=bytes.fromhex(encoding)
        terms,fee,_,allowance=display(wire, profile, authority.public_key(), board, user, 1, clock)
        self.assertEqual((expires, len(wire), terms[0], fee, allowance),
                         (clock+35,814,"Swap LAB to LAQ",65_000*10**9,100_000_000))
        market = profile["market"]
        scope = bytes.fromhex("".join(market[name] for name in ("venue", "base", "quoteToken", "quoteSigner", "codeHash")))
        offer_digest = digest(scope, bytes.fromhex(params["quote"]))
        scoped = {**params, "allowance": params["allowance"] + offer_digest.hex()}
        expires, wire_hex = issue(profile, scoped, clock, key)
        scoped_wire = bytes.fromhex(wire_hex)
        self.assertEqual((len(scoped_wire), scoped_wire[229]), (846, 2))
        self.assertEqual(display(scoped_wire, profile, authority.public_key(), board, user, 1, clock),
                         (terms, fee, expires, allowance))
        with self.assertRaisesRegex(ValueError, "not bound"):
            issue(profile, {**scoped, "allowance": params["allowance"] + ("cc" * 32)}, clock, key)
        for changes in ({"deployment":"aa"*32},{"to":"ff"*20},{"gasPrice":"100000000001"},
                        {"allowance":params["allowance"][:-2]+"aa"}):
            with self.assertRaises(ValueError):
                issue(profile,{**params,**changes},clock,key)
        for instant in (clock+35,clock-26):
            with self.assertRaises(ValueError):
                issue(profile,params,instant,key)
        with self.assertRaises(InvalidSignature):
            changed=wire[:212]+bytes([wire[212]^1])+wire[213:]
            display(changed,profile,authority.public_key(),board,user,1,clock)


if __name__ == "__main__":
    unittest.main()
