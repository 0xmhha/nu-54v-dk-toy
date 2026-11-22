#!/usr/bin/env python3
"""Tenant-local EVM registry and authorization service. It never broadcasts."""

import argparse
import base64
import datetime
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import re
import secrets
import ssl
import struct
import threading
import time
from urllib.parse import urlsplit

from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import ec, utils

from enroll_board import exact_hex, signed
from walletconnect_peer import authenticate as authenticate_peer
from evm_transfer import CHAINS, decode, keccak256, rpc_client, rpc_quote

ZERO = "0x" + "00" * 20


def rlp(value, group=False):
    if group:
        body = b"".join(value)
    elif isinstance(value, int):
        body = value.to_bytes((value.bit_length() + 7) // 8, "big")
    else:
        body = value
    if not group and len(body) == 1 and body[0] < 128:
        return body
    lead = 192 if group else 128
    if len(body) < 56:
        return bytes([lead + len(body)]) + body
    if len(body) >= 256:
        raise ValueError("Transaction too large")
    return bytes([lead + 56, len(body)]) + body


def transfer_raw(chain, owner, recipient, asset, amount, rpc, genesis, revoke=False, allowance=None):
    if int(rpc("eth_chainId", []), 16) != chain or rpc("eth_getBlockByNumber", ["0x0", False])["hash"] != genesis:
        raise ValueError("Provider network mismatch")
    signer, address = exact_hex(owner, 20), exact_hex(recipient, 20)
    nonce = int(rpc("eth_getTransactionCount", [owner, "pending"]), 16)
    block = rpc("eth_getBlockByNumber", ["latest", False])
    base = int(block["baseFeePerGas"], 16)
    priority = int(rpc("eth_maxPriorityFeePerGas", []), 16)
    cap = (2 * base) + priority
    if asset == ZERO:
        to, value, data = address, amount, b""
    else:
        to, value = exact_hex(asset, 20), 0
        data = bytes.fromhex("095ea7b3" if revoke or allowance else "a9059cbb") + address.rjust(32, b"\0") + amount.to_bytes(32, "big")
    request = {"from": owner, "to": "0x" + to.hex(), "value": hex(value), "data": "0x" + data.hex()}
    gas = int(rpc("eth_estimateGas", [request]), 16)
    if asset != ZERO:
        gas = (gas * 11 + 9) // 10
    raw = bytes([2]) + rlp([rlp(x) for x in (chain, nonce, priority, cap, gas, to, value, data)] + [rlp([], True)], True)
    token = bytes(20) if asset == ZERO else to
    decode(raw, token, revoke, allowance)
    l1 = rpc_quote(rpc, genesis, raw, signer, token, revoke, allowance)
    return raw, cap * gas + l1, l1, token


class WalletAuthority:
    def __init__(self, profile, key, token_hash, rpc_factory=rpc_client, clock=time.time, display_token_hash=None, peer_path=None):
        self.profile = profile
        self.key = key
        self.token_hash = token_hash
        self.rpc = rpc_factory
        self.clock = clock
        self.display_token_hash = display_token_hash
        self.peer_path = peer_path
        self.display_quote = None
        self.display_lock = threading.Lock()
        self.login_lock = threading.Lock()
        self.login_challenges = {}
        self.login_reviewed = set()

    def _dapp_scope(self):
        dapp = self.profile.get("dapp")
        if not dapp or dapp.get("chain") not in ("11155111", "84532", "8283"):
            raise ValueError("DApp login is disabled")
        origin = dapp["origin"]
        host = urlsplit(origin)
        if not re.fullmatch(r"https://[a-z0-9.-]+(?::[0-9]{1,5})?", origin) or len(host.netloc) > 96:
            raise ValueError("Invalid commissioned DApp origin")
        network = next(entry for entry in self.profile["networks"] if entry["chain"] == dapp["chain"])
        return origin, host.netloc, network

    def _siwe(self, method, request, transport_origin, now):
        origin, host, network = self._dapp_scope()
        if transport_origin != origin or request.get("account") != self.profile["sender"]:
            raise ValueError("DApp session origin or account mismatch")
        with self.login_lock:
            self.login_challenges = {key:value for key,value in self.login_challenges.items() if value[0]>now}
            self.login_reviewed.intersection_update(self.login_challenges)
            if method == "nu54_siweChallenge":
                if len(self.login_challenges) >= 12:
                    raise ValueError("Too many active login challenges")
                nonce = secrets.token_hex(16)
                expires = min(now + 115, self.profile["expires"])
                def stamp(value):
                    return datetime.datetime.fromtimestamp(value,datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
                message = "\n".join((host + " wants you to sign in with your Ethereum account:", self.profile["sender"], "", "",
                                     "URI: " + origin + "/sign-in", "Version: 1", "Chain ID: " + network["chain"],
                                     "Nonce: " + nonce, "Issued At: " + stamp(now), "Expiration Time: " + stamp(expires)))
                self.login_challenges[nonce] = (expires,message)
                return {"nonce":nonce,"message":message,"expires":expires}
            nonce = request.get("nonce")
            entry = self.login_challenges.get(nonce)
            if not entry or entry[1] != request.get("message"):
                raise ValueError("Login expired or changed")
            signature = exact_hex(request["signature"],65)
            if signature[-1] not in (27,28):
                raise ValueError("Invalid EVM login signature")
            public = exact_hex(self.profile["dapp"]["publicKey"],65)
            if keccak256(public[1:])[12:] != exact_hex(self.profile["sender"],20):
                raise ValueError("Commissioned login key differs from the wallet")
            key = ec.EllipticCurvePublicKey.from_encoded_point(ec.SECP256K1(),public)
            raw = entry[1].encode("ascii")
            digest = keccak256(b"\x19Ethereum Signed Message:\n"+str(len(raw)).encode()+raw)
            der = utils.encode_dss_signature(int.from_bytes(signature[:32],"big"),int.from_bytes(signature[32:64],"big"))
            key.verify(der,digest,ec.ECDSA(utils.Prehashed(hashes.SHA256())))
            del self.login_challenges[nonce]
            self.login_reviewed.discard(nonce)
            return {"verified":True}

    def _walletconnect_peer(self, request, now):
        origin, _, network = self._dapp_scope()
        configuration = self.profile.get("walletConnect")
        if not configuration or request.get("origin") != origin or request.get("chain") != network["chain"]:
            raise ValueError("WalletConnect origin or chain is not commissioned")
        enrolled = configuration.get("peer")
        if self.peer_path:
            with self.peer_path.open("r",encoding="ascii") as file:
                enrolled = json.loads(file.read(2049))
        return authenticate_peer(enrolled,self.key,origin,request["pairingTopic"],request["peerKey"],now)

    def _allowance_scope(self, request, network, asset, now):
        origin, _, authorized_network = self._dapp_scope()
        policy = self.profile["dapp"].get("allowance", {})
        if (network != authorized_network or request["origin"] != origin or asset["address"] == ZERO or
            policy.get("token") != asset["address"] or policy.get("spender") != request["recipient"]):
            raise ValueError("DApp allowance scope mismatch")
        maximum = policy.get("maximum")
        decimals = asset.get("decimals")
        if (not isinstance(maximum, str) or not re.fullmatch(r"[1-9][0-9]{0,77}", maximum) or
            not 0 < int(maximum) < 2**256 - 1 or type(decimals) is not int or not 0 <= decimals <= 24):
            raise ValueError("Invalid DApp allowance configuration")
        expires = request["sessionExpires"]
        if type(expires) is not int or not now < expires <= now + 3600:
            raise ValueError("DApp session expired")
        session = exact_hex(request["sessionTopic"], 32)
        if not any(session):
            raise ValueError("Invalid DApp session")
        spender = exact_hex(policy["spender"], 20)
        if not any(spender):
            raise ValueError("Invalid DApp spender")
        wc = request["transport"] == "walletconnect"
        if wc:
            enrolled = self._walletconnect_peer(request, now)["proof"]
            expires = min(expires, enrolled["expires"])
            pairing = exact_hex(request["pairingTopic"], 32)
            if pairing == session:
                raise ValueError("Pairing topic cannot carry session spending")
            identities = pairing + session + exact_hex(request["peerKey"], 32)
        else:
            identities = session + bytes(64)
        hostname = origin.encode("ascii")
        context = (bytes([wc]) + spender + int(maximum).to_bytes(32, "big") + bytes([decimals]) +
                   identities + bytes([len(hostname)]) + hostname)
        return context, expires, (spender, int(maximum))

    def handle(self, packet, transport_origin=None):
        if packet.get("jsonrpc") != "2.0" or packet.get("id") != 1 or not isinstance(packet.get("params"), list) or len(packet["params"]) != 1:
            raise ValueError("Invalid RPC envelope")
        request = packet["params"][0]
        now = int(self.clock())
        method = packet.get("method")
        if self.profile["expires"] <= now:
            raise ValueError("Wallet authority enrollment expired")
        if method in ("nu54_siweChallenge","nu54_siweVerify"):
            return self._siwe(method,request,transport_origin,now)
        token = request.get("token", "")
        permission = self.display_token_hash if method == "nu54_walletDisplay" else self.token_hash
        if (not permission or not isinstance(token, str) or len(token) != 64 or
            not secrets.compare_digest(hashlib.sha256(token.encode()).hexdigest(), permission)):
            raise ValueError("Wallet authority access denied")
        if method == "nu54_walletDisplay":
            with self.display_lock:
                if not self.display_quote or self.display_quote[0] <= now:
                    raise ValueError("No current transfer to review")
                return self.display_quote[1]
        if method == "nu54_walletConnectPeer":
            return self._walletconnect_peer(request,now)
        if method == "nu54_marketBoardGrant":
            from market_grant import issue
            expires, wire = issue(self.profile, request, now, self.key)
            with self.display_lock:
                self.display_quote = (expires, wire)
            return wire
        if method == "nu54_walletRegistry":
            body = json.dumps(self.profile, separators=(",", ":"), sort_keys=True).encode()
            return {"payload": base64.b64encode(body).decode(), "proof": signed(self.key, b"N54G" + body).hex()}
        if method == "nu54_walletLogin":
            origin, host, network = self._dapp_scope()
            message = request.get("message")
            if request.get("origin") != origin or not isinstance(message,str):
                raise ValueError("Login scope mismatch")
            with self.login_lock:
                match = next(((nonce,expires) for nonce,(expires,original) in self.login_challenges.items()
                              if original==message and expires>now),None)
                if not match:
                    raise ValueError("No active login challenge")
                nonce,expires=match
                if nonce in self.login_reviewed:
                    raise ValueError("Login challenge was already reviewed")
                self.login_reviewed.add(nonce)
            raw=message.encode("ascii")
            host_bytes=host.encode("ascii")
            wire=(bytes([15])+struct.pack(">QQ",expires,int(network["chain"]))+exact_hex(network["genesis"],32)+
                  struct.pack(">Q",int(self.profile["leaseRevision"]))+bytes([len(host_bytes)])+host_bytes+
                  struct.pack(">H",len(raw))+raw)
            statement=(b"N54S"+exact_hex(self.profile["device"],32)+int(self.profile["generation"]).to_bytes(8,"big")+
                       exact_hex(self.profile["sender"],20)+wire)
            wire_hex=(wire+signed(self.key,statement)).hex()
            with self.display_lock:
                self.display_quote=(expires,wire_hex)
            return {"wire":wire_hex,"nonce":nonce,"expires":expires}
        if method not in ("nu54_walletQuote","nu54_walletRevokeAllowance","nu54_walletExternalSpend","nu54_walletDappAllowance"):
            raise ValueError("Unsupported wallet authority operation")
        revoke = method == "nu54_walletRevokeAllowance"
        external = method == "nu54_walletExternalSpend"
        approval = method == "nu54_walletDappAllowance"
        expected = {"token", "chain", "asset", "recipient"}
        if not revoke:
            expected.add("amount")
        if external:
            expected.update(("origin", "pairingTopic", "sessionTopic", "peerKey"))
        if approval:
            if request.get("transport") not in ("browser", "walletconnect"):
                raise ValueError("Unknown DApp transport")
            expected.update(("origin", "sessionTopic", "sessionExpires", "transport"))
            if request["transport"] == "walletconnect":
                expected.update(("pairingTopic", "peerKey"))
        if set(request) != expected:
            raise ValueError("Unexpected transaction fields")
        context = b""
        expires = min(now + 100, self.profile["expires"])
        chain = request["chain"]
        network = next((entry for entry in self.profile["networks"] if entry["chain"] == chain), None)
        if network is None:
            raise ValueError("Unregistered wallet network")
        asset = request["asset"]
        asset_profile = next((entry for entry in network["assets"] if entry["address"] == asset), None)
        if asset_profile is None:
            raise ValueError("Unregistered wallet asset")
        allowance = None
        if approval:
            context, session_expiry, allowance = self._allowance_scope(request, network, asset_profile, now)
            expires = min(expires, session_expiry)
        if external:
            enrolled = self._walletconnect_peer(request,now)["proof"]
            origin = enrolled["origin"]
            if asset != ZERO:
                raise ValueError("External spend supports only the enrolled native asset")
            pairing = exact_hex(request["pairingTopic"],32)
            session = exact_hex(request["sessionTopic"],32)
            if session == pairing:
                raise ValueError("Pairing topic cannot carry session spending")
            origin_bytes = origin.encode("ascii")
            context = pairing + session + exact_hex(request["peerKey"],32) + bytes([len(origin_bytes)]) + origin_bytes
            expires = min(expires,enrolled["expires"])
        amount = request.get("amount")
        if revoke:
            if asset == ZERO:
                raise ValueError("Revocation requires an enrolled token and a zero allowance")
            amount = "0"
        elif not isinstance(amount, str) or not re.fullmatch(r"[1-9][0-9]{0,77}", amount) or not 0 < int(amount) < 2**256:
            raise ValueError("Invalid transfer amount")
        if allowance and int(amount) > allowance[1]:
            raise ValueError("DApp allowance exceeds the commissioned cap")
        raw, total, base, token = transfer_raw(int(chain), self.profile["sender"], request["recipient"],
                                               asset, int(amount), self.rpc(network["rpc"]), network["genesis"], revoke, allowance)
        limit = int(network["feeLimit"])
        if total > limit:
            raise ValueError("Authority fee limit exceeded")
        header = (bytes([36 if approval else 20 if external else 14 if revoke else 9]) + struct.pack(">QQQQ", expires, int(chain), limit, base) +
                  token + exact_hex(network["genesis"], 32) + struct.pack(">QQ", 1, int(self.profile["leaseRevision"])))
        statement = (b"N54V" + exact_hex(self.profile["device"], 32) + int(self.profile["generation"]).to_bytes(8,"big") +
                     exact_hex(self.profile["sender"], 20) + header + keccak256(raw) + context)
        wire = (header + signed(self.key, statement) + context + raw).hex()
        with self.display_lock:
            self.display_quote = (expires, wire)
        return wire


def serve():
    parser = argparse.ArgumentParser(description=__doc__)
    for path in ("profile", "authority", "certificate", "tls-key"):
        parser.add_argument("--" + path, required=True, type=Path)
    parser.add_argument("--token-sha256", required=True)
    parser.add_argument("--display-token-sha256", required=True)
    parser.add_argument("--wallet-peer-proof", type=Path)
    parser.add_argument("--port", default=8447, type=int)
    args = parser.parse_args()
    profile = json.loads(args.profile.read_text())
    exact_hex(args.token_sha256, 32)
    exact_hex(args.display_token_sha256, 32)
    if args.display_token_sha256 == args.token_sha256:
        raise ValueError("Use a separate credential on the independent display")
    if "chainNetworks" in profile:
        from wallet_chain_lab import validate_commissioned_profiles
        validate_commissioned_profiles(profile["chainNetworks"])
    if not isinstance(profile.get("networks"), list) or not (profile["networks"] or profile.get("chainNetworks")) \
            or any(int(entry["chain"]) not in CHAINS for entry in profile["networks"]):
        raise ValueError("Expected a commissioned wallet profile")
    oracle = WalletAuthority(profile, args.authority, args.token_sha256, display_token_hash=args.display_token_sha256, peer_path=args.wallet_peer_proof)

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            try:
                length = int(self.headers.get("Content-Length", "0"))
                if self.path != "/wallet" or not 1 <= length <= 2048:
                    raise ValueError("Invalid request length")
                result = oracle.handle(json.loads(self.rfile.read(length)), self.headers.get("Origin"))
                body = json.dumps({"jsonrpc":"2.0","id":1,"result":result}).encode()
                status = 200
            except Exception:
                body = b'{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"Wallet authority refused request"}}'
                status = 400
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *ignored):
            pass

    server = ThreadingHTTPServer(("127.0.0.1", args.port), Handler)
    tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    tls.minimum_version = ssl.TLSVersion.TLSv1_3
    tls.load_cert_chain(args.certificate, args.tls_key)
    server.socket = tls.wrap_socket(server.socket, server_side=True)
    server.serve_forever()


if __name__ == "__main__":
    serve()
