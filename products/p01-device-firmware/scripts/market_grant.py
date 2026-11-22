"""Protocol boundary for one lab order approved by the onboard signer and independent display."""

import struct

from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import ec, utils

from enroll_board import exact_hex, signed
from evm_transfer import keccak256

EXECUTE = keccak256(b"execute((address,uint8,uint256,uint256,uint256,uint256,uint256,uint256,uint256),bytes)")[:4]
APPROVE = bytes.fromhex("095ea7b3")
OPERATIONS = ("Swap LAB to LAQ", "Swap LAQ to LAB", "Add liquidity", "Remove liquidity",
              "Simulate LAB to LAQ", "Simulate LAQ to LAB", "Open long", "Open short", "Close position")
ORDER = b"Quote(address user,uint8 action,uint256 a,uint256 b,uint256 minA,uint256 minB,uint256 priceVersion,uint256 nonce,uint256 deadline)"
DOMAIN = b"EIP712Domain(string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)"
S_LIMIT = 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0


def num(value):
    return int.from_bytes(value, "big")


def digest(scope, quote):
    venue, code_hash = scope[:20], scope[80:112]
    domain = keccak256(keccak256(DOMAIN) + keccak256(b"NU54 Isolated Market") + keccak256(b"1") +
                       (8283).to_bytes(32, "big") + venue.rjust(32, b"\0") + code_hash)
    return keccak256(b"\x19\x01" + domain + keccak256(keccak256(ORDER) + quote[4:292]))


def review(scope, target, sender, quote, allowance, now, quote_key):
    if (len(scope) != 112 or len(quote) != 452 or quote[:4] != EXECUTE or quote[4:16] != bytes(12) or
        quote[16:36] != sender or num(quote[36:68]) >= len(OPERATIONS) or num(quote[292:324]) != 320 or
        num(quote[324:356]) != 65 or quote[420] not in (27, 28) or any(quote[421:]) or
        not 0 < num(quote[260:292]) - now <= 60 or not 0 < num(quote[68:100]) <= 10**24 or
        num(quote[100:132]) > 10**24 or not 0 < num(quote[388:420]) <= S_LIMIT):
        raise ValueError("Market quote scope or deadline invalid")
    if keccak256(quote_key[1:])[12:] != scope[60:80]:
        raise ValueError("Market signer differs from the commissioned role")
    key = ec.EllipticCurvePublicKey.from_encoded_point(ec.SECP256K1(), quote_key)
    der = utils.encode_dss_signature(num(quote[356:388]), num(quote[388:420]))
    offer_digest = digest(scope, quote)
    key.verify(der, offer_digest, ec.ECDSA(utils.Prehashed(hashes.SHA256())))
    action = quote[67]
    a, b, minimum_a, minimum_b = (num(quote[i:i+32]) for i in (68,100,132,164))
    if not allowance:
        if target != scope[:20]:
            raise ValueError("Execution target differs from the venue")
    else:
        if len(allowance) not in (68, 100) or allowance[:4] != APPROVE or allowance[4:36] != scope[:20].rjust(32, b"\0"):
            raise ValueError("Allowance differs from commissioned venue")
        if len(allowance) == 100 and allowance[68:] != offer_digest:
            raise ValueError("Allowance is not bound to the original quote")
        if target == scope[20:40] and action in (0,2):
            expected = a
        elif target == scope[40:60] and action in (1,2,6,7):
            expected = b if action == 2 else a
        else:
            raise ValueError("Token allowance differs from the quote direction")
        if not expected or num(allowance[36:68]) != expected:
            raise ValueError("Allowance exceeds or changes the quoted quantity")
    return (OPERATIONS[action], a, b, minimum_a, minimum_b, num(quote[196:228]))


def issue(profile, request, now, key):
    market = profile.get("market")
    if not market or set(request) != {"token","venue","genesis","deployment","user","to","nonce","gasPrice","quote","allowance"}:
        raise ValueError("Isolated market capability unavailable")
    genesis = exact_hex(market["genesis"], 32)
    deployment = exact_hex(market["deployment"], 32)
    sender = exact_hex(market["sender"], 20)
    venue = exact_hex(market["venue"], 20)
    if (exact_hex(request["genesis"],32)!=genesis or exact_hex(request["deployment"],32)!=deployment or
        exact_hex(request["user"],20)!=sender or exact_hex(request["venue"],20)!=venue):
        raise ValueError("Market order differs from the commissioned deployment and account")
    scope = b"".join(exact_hex(market[field], 20) for field in ("venue","base","quoteToken","quoteSigner"))
    scope += exact_hex(market["codeHash"],32)
    target = exact_hex(request["to"],20)
    fee, nonce = int(request["gasPrice"]), int(request["nonce"])
    quote = exact_hex(request["quote"],452)
    encoded_allowance = request["allowance"]
    allowance = bytes.fromhex(encoded_allowance.removeprefix("0x")) if encoded_allowance else b""
    mode = {0: 0, 68: 1, 100: 2}.get(len(allowance))
    if mode is None or (encoded_allowance and exact_hex(encoded_allowance, len(allowance)) != allowance):
        raise ValueError("Invalid market allowance encoding")
    quote_key = exact_hex(market["quotePublicKey"],65)
    review(scope,target,sender,quote,allowance,now,quote_key)
    if not 0 <= nonce < 2**64 or not 0 < fee <= 100_000_000_000 or fee*(65000 if allowance else 600000) > int(market["feeLimit"]):
        raise ValueError("Market gas exceeds the commissioned limit")
    expires = min(num(quote[260:292]),profile["expires"])
    if expires<=now:
        raise ValueError("Market grant expired")
    body = bytes([21])+struct.pack(">QQ",expires,int(profile["leaseRevision"]))+genesis+deployment+scope+target
    body += struct.pack(">QQB",nonce,fee,mode)+quote+allowance
    statement = b"N54J"+exact_hex(profile["device"],32)+struct.pack(">Q",int(profile["generation"]))+sender+body
    return expires,(body+signed(key,statement)).hex()


def display(wire, profile, authority, device, sender, generation, now):
    if len(wire) not in (746, 814, 846) or wire[0] != 21 or {0: 746, 1: 814, 2: 846}.get(wire[229]) != len(wire):
        raise ValueError("Invalid market grant")
    expires, revision = struct.unpack(">QQ",wire[1:17])
    if not 0 < expires-now <= 60 or revision != int(profile["leaseRevision"]):
        raise ValueError("Market approval expired or no longer commissioned")
    market=profile["market"]
    if (wire[17:49]!=exact_hex(market["genesis"],32) or wire[49:81]!=exact_hex(market["deployment"],32) or
        sender!=exact_hex(market["sender"],20)):
        raise ValueError("Wrong market deployment or account")
    scope=b"".join(exact_hex(market[field],20) for field in ("venue","base","quoteToken","quoteSigner"))+exact_hex(market["codeHash"],32)
    if wire[81:193]!=scope:
        raise ValueError("Different venue or bytecode")
    statement=b"N54J"+device+struct.pack(">Q",generation)+sender+wire[:-64]
    der=utils.encode_dss_signature(num(wire[-64:-32]),num(wire[-32:]))
    authority.verify(der,statement,ec.ECDSA(hashes.SHA256()))
    quote, allowance=wire[230:682],wire[682:-64]
    result=review(scope,wire[193:213],sender,quote,allowance,now,exact_hex(market["quotePublicKey"],65))
    gas=65000 if allowance else 600000
    fee=gas*num(wire[221:229])
    if fee>int(market["feeLimit"]) or expires>num(quote[260:292]):
        raise ValueError("Market fee or deadline differs from independent policy")
    return result, fee, expires, num(allowance[36:68]) if allowance else None
