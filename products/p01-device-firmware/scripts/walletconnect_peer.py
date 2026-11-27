#!/usr/bin/env python3
"""Issue a short-lived peer proof after authenticating the commissioned DApp operator."""
import argparse
import json
from pathlib import Path
import re
import time

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, utils
from urllib.parse import urlsplit

from enroll_board import exact_hex, signed


def statement(proof):
    return ("N54-walletconnect-v1\n"+proof["topic"]+"\n"+proof["publicKey"]+"\n"+
            proof["origin"]+"\n"+str(proof["expires"])).encode("ascii")


def authenticate(enrollment, authority, origin, topic, peer, now):
    if not isinstance(enrollment, dict) or set(enrollment) != {"proof","signature"}:
        raise ValueError("WalletConnect peer has not been commissioned")
    proof = enrollment["proof"]
    if (proof["origin"] != origin or proof["topic"] != exact_hex(topic,32).hex() or
        proof["publicKey"] != exact_hex(peer,32).hex() or type(proof["expires"]) is not int or
        not now < proof["expires"] <= now+300):
        raise ValueError("WalletConnect peer scope differs from commissioning")
    raw = exact_hex(enrollment["signature"],64)
    signature = utils.encode_dss_signature(int.from_bytes(raw[:32],"big"),int.from_bytes(raw[32:],"big"))
    key = serialization.load_pem_private_key(authority.read_bytes(), None).public_key()
    key.verify(signature,statement(proof),ec.ECDSA(hashes.SHA256()))
    return enrollment


def issue(profile, authority, topic, peer, now):
    settings = profile.get("dapp", {})
    origin = settings.get("origin", "")
    if not re.fullmatch(r"https://[a-z0-9.-]+(?::[0-9]{1,5})?", origin) or urlsplit(origin).netloc == "":
        raise ValueError("Profile does not commission an exact HTTPS origin")
    if not profile.get("walletConnect", {}).get("relay") or profile["expires"] <= now:
        raise ValueError("WalletConnect profile is disabled or expired")
    if settings.get("chain") not in [net["chain"] for net in profile["networks"]]:
        raise ValueError("DApp chain does not belong to the wallet profile")
    topic, peer = exact_hex(topic,32).hex(), exact_hex(peer,32).hex()
    expires = min(now+300, profile["expires"])
    proof = {"origin":origin,"topic":topic,"publicKey":peer,"expires":expires}
    return {"proof":proof,"signature":signed(authority,statement(proof)).hex()}


if __name__ == "__main__":
    argument = argparse.ArgumentParser(description=__doc__)
    argument.add_argument("--profile", required=True, type=Path)
    argument.add_argument("--authority", required=True, type=Path)
    argument.add_argument("--pairing-topic", required=True)
    argument.add_argument("--peer-public-key", required=True)
    args = argument.parse_args()
    print(json.dumps(issue(json.loads(args.profile.read_text()),args.authority,args.pairing_topic,args.peer_public_key,int(time.time()))))
