#!/usr/bin/env python3
"""Issue a short-lived direct-transfer authorization from a configured HTTPS node."""

import argparse
import json
import re
import struct
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

from Crypto.Hash import keccak
from enroll_board import exact_hex, public_file, signed
from cryptography.hazmat.primitives.asymmetric import ec, utils
from cryptography.hazmat.primitives import hashes

CHAINS = {11155111: "Ethereum Sepolia", 84532: "Base Sepolia", 8283: "StableNet"}
ORACLE = "0x420000000000000000000000000000000000000f"


def keccak256(data):
    return keccak.new(digest_bits=256, data=data).digest()


def fields(data):
    out = []
    index = 0
    while index < len(data):
        tag = data[index]
        group = tag >= 0xC0
        index += 1
        size = tag - (0xC0 if group else 0x80)
        if tag < 0x80:
            index -= 1
            size = 1
        elif size >= 56:
            if size != 56 or index == len(data) or data[index] < 56:
                raise ValueError("Invalid RLP length")
            size = data[index]
            index += 1
        end = index + size
        if end > len(data) or (tag == 0x81 and data[index] < 0x80):
            raise ValueError("Invalid RLP field")
        out.append((data[index:end], group))
        index = end
    return out


def decode(raw, token, revoke=False, allowance=None):
    if len(raw) > 256 or not raw.startswith(b"\x02"):
        raise ValueError("Only bounded EIP-1559 transfers are supported")
    outer = fields(raw[1:])
    if len(outer) != 1 or not outer[0][1]:
        raise ValueError("Expected a transaction RLP list")
    items = fields(outer[0][0])
    if len(items) != 9 or items[8] != (b"", True) or any(group for _, group in items[:8]):
        raise ValueError("Unexpected list or transaction field")
    n = [items[i][0] for i in (0, 1, 2, 3, 4, 6)]
    if any(len(value) > (32 if index == 5 else 8) or value.startswith(b"\x00") for index, value in enumerate(n)):
        raise ValueError("Noncanonical number")
    chain, nonce, tip, max_price, gas, value = (int.from_bytes(x, "big") for x in n)
    recipient, data = items[5][0], items[7][0]
    if chain not in CHAINS or not 0 <= tip <= max_price or not max_price or gas < 21000 or len(recipient) != 20 or not any(recipient):
        raise ValueError("Unsupported network, recipient or gas")
    amount = value
    if any(token):
        if gas > 150000 or value or recipient != token or len(data) != 68 or data[:16] != bytes.fromhex(("095ea7b3" if revoke or allowance else "a9059cbb") + "00" * 12):
            raise ValueError("Only the authorized ERC20 transfer is supported")
        recipient = data[16:36]
        amount = int.from_bytes(data[36:], "big")
    elif revoke or allowance or gas != 21000 or data:
        raise ValueError("Only a native transfer is supported")
    if not any(recipient) or bool(amount) == revoke:
        raise ValueError("Recipient and amount must be present")
    if allowance and (revoke or recipient != allowance[0] or not 0 < amount <= allowance[1] < 2**256 - 1):
        raise ValueError("Approval exceeds the commissioned spender or amount")
    return chain, nonce, max_price, gas, amount, recipient


def dapp_context(wire, start, browser=False):
    prefix = start + 97
    if len(wire) <= prefix or not 9 <= wire[prefix-1] <= 104 or len(wire) <= prefix + wire[prefix-1]:
        raise ValueError("Malformed external DApp context")
    identities = tuple(wire[start+i:start+i+32] for i in (0,32,64))
    first, second, third = identities
    if not any(first) or (browser and (any(second) or any(third))) or (
            not browser and (not any(second) or not any(third) or first == second)):
        raise ValueError("Invalid external DApp topics")
    end = prefix + wire[prefix-1]
    origin = wire[prefix:end].decode("ascii")
    if not re.fullmatch(r"https://[a-z0-9.-]+(?::[0-9]{1,5})?",origin):
        raise ValueError("Invalid external DApp origin")
    return end, (origin, *identities)


def external_context(wire):
    return dapp_context(wire, 165) if wire[0] == 20 else (165, None)


def allowance_context(wire):
    if len(wire) <= 219 or wire[165] not in (0, 1) or wire[218] > 24:
        raise ValueError("Malformed allowance policy")
    end, channel = dapp_context(wire, 219, wire[165] == 0)
    spender, maximum = wire[166:186], int.from_bytes(wire[186:218], "big")
    if not any(spender) or not 0 < maximum < 2**256 - 1:
        raise ValueError("Invalid allowance policy")
    return end, (spender, maximum, wire[218]), channel


def parse(wire, authority, board, address, generation, now):
    if not 167 <= len(wire) <= 676 or wire[0] not in (9,14,20,36):
        raise ValueError("Malformed direct transfer envelope")
    expiry, scope, fee_limit, base_data = struct.unpack_from(">QQQQ", wire, 1)
    start, context = external_context(wire)
    allowance = None
    if wire[0] == 36:
        start, allowance, _ = allowance_context(wire)
    token, genesis, raw = wire[33:53], wire[53:85], wire[start:]
    if context and any(token):
        raise ValueError("External spend only supports native currency")
    profile_revision, lease_revision = struct.unpack_from(">QQ", wire, 85)
    if not any(genesis) or profile_revision != 1 or not lease_revision:
        raise ValueError("Invalid signed network profile")
    chain, nonce, max_price, gas, amount, recipient = decode(raw, token, wire[0] == 14, allowance)
    fee = max_price * gas + base_data
    if chain != scope or not now < expiry <= now + 120 or fee > fee_limit or bool(base_data) != (chain == 84532):
        raise ValueError("Invalid transfer fees, network or expiry")
    signature = wire[101:165]
    signature = utils.encode_dss_signature(int.from_bytes(signature[:32], "big"), int.from_bytes(signature[32:], "big"))
    body = b"N54V" + board + struct.pack(">Q", generation) + address + wire[:101] + keccak256(raw) + wire[165:start]
    authority.verify(signature, body, ec.ECDSA(hashes.SHA256()))
    return chain, nonce, token, amount, recipient, fee, base_data, genesis


def parse_siwe_capsule(wire, authority, board, address, generation, now):
    if not 124 <= len(wire) <= 820 or wire[0] != 15:
        raise ValueError("Malformed login authorization")
    expiry, chain = struct.unpack_from(">QQ",wire,1)
    genesis, revision, host_length = wire[17:49], int.from_bytes(wire[49:57],"big"), wire[57]
    length_at=58+host_length
    size=int.from_bytes(wire[length_at:length_at+2],"big")
    if (not now < expiry <= now+120 or chain not in CHAINS or not any(genesis) or not revision or
        not 1 <= host_length <= 96 or not 1 <= size <= 600 or length_at+2+size+64 != len(wire)):
        raise ValueError("Malformed login scope")
    host=wire[58:length_at].decode("ascii")
    if not re.fullmatch(r"[a-z0-9.-]+(?::[0-9]{1,5})?",host):
        raise ValueError("Invalid login hostname")
    message=wire[length_at+2:-64].decode("ascii").split("\n")
    if (len(message)!=10 or message[0]!=host+" wants you to sign in with your Ethereum account:" or
        exact_hex(message[1],20)!=address or not message[4].startswith("URI: https://"+host+"/") or
        message[6]!="Chain ID: "+str(chain) or not re.fullmatch(r"Nonce: [a-zA-Z0-9]{8,32}",message[7])):
        raise ValueError("Login scope differs from the signed message")
    signature=wire[-64:]
    der=utils.encode_dss_signature(int.from_bytes(signature[:32],"big"),int.from_bytes(signature[32:],"big"))
    statement=b"N54S"+board+struct.pack(">Q",generation)+address+wire[:-64]
    authority.verify(der,statement,ec.ECDSA(hashes.SHA256()))
    return chain,host,message[7][7:],genesis


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args):
        raise ValueError("RPC redirects are not allowed")


def rpc_client(url):
    origin = urllib.parse.urlsplit(url)
    if origin.scheme != "https" or not origin.hostname or origin.username or origin.password or origin.fragment:
        raise ValueError("Use an independently configured HTTPS RPC endpoint")
    client = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())

    def call(method, params):
        body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params}).encode()
        with client.open(urllib.request.Request(url, data=body, headers={"Content-Type": "application/json"}), timeout=8) as response:
            raw = response.read(256_001)
        if len(raw) > 256_000:
            raise ValueError("Oversized RPC response")
        packet = json.loads(raw)
        if packet.get("jsonrpc") != "2.0" or packet.get("id") != 1 or "error" in packet:
            raise ValueError("Invalid RPC response")
        return packet["result"]

    return call


def rpc_quote(rpc, genesis, raw, sender, token, revoke=False, allowance=None):
    chain, nonce, _, gas, _, recipient = decode(raw, token, revoke, allowance)
    if int(rpc("eth_chainId", []), 16) != chain or rpc("eth_getBlockByNumber", ["0x0", False])["hash"] != genesis:
        raise ValueError("Provider does not match the expected network/genesis")
    if int(rpc("eth_getTransactionCount", ["0x" + sender.hex(), "pending"]), 16) != nonce:
        raise ValueError("Unsigned transfer nonce does not match the provider")
    if not any(token) and rpc("eth_getCode", ["0x" + recipient.hex(), "latest"]) != "0x":
        raise ValueError("Native recipient must be an externally owned account")
    if chain != 84532:
        return 0
    latest = rpc("eth_getBlockByNumber", ["latest", False])
    block_hash = latest["hash"]
    if not re.fullmatch(r"0x[a-f0-9]{64}", block_hash):
        raise ValueError("Missing Base oracle block")
    scope = {"blockHash": block_hash, "requireCanonical": True}
    estimates = []
    for method, value in (("getL1FeeUpperBound(uint256)", len(raw)), ("getOperatorFee(uint256)", gas)):
        call = "0x" + keccak256(method.encode())[:4].hex() + value.to_bytes(32, "big").hex()
        returned = rpc("eth_call", [{"to": ORACLE, "data": call}, scope])
        if not re.fullmatch(r"0x[a-f0-9]{64}", returned):
            raise ValueError("Unsupported Base fee oracle")
        estimates.append(int(returned, 16))
    result = sum(estimates)
    if not 0 < result < 2**64:
        raise ValueError("Invalid Base fee estimate")
    return result


def issue(args):
    raw = args.unsigned.read_bytes()
    board, sender = exact_hex(args.board, 32), exact_hex(args.sender, 20)
    token = exact_hex(args.token, 20) if args.token else bytes(20)
    chain, _, price, gas, _, _ = decode(raw, token)
    if not 0 < args.generation < 2**64 or not 0 < args.fee_limit < 2**64 or not re.fullmatch(r"0x[a-f0-9]{64}", args.genesis):
        raise ValueError("Invalid chain authority policy")
    l1 = rpc_quote(rpc_client(args.rpc), args.genesis, raw, sender, token)
    if price * gas + l1 > args.fee_limit:
        raise ValueError("Transaction exceeds the authority fee budget")
    if not 0 < args.lease_revision < 2**64:
        raise ValueError("Current board lease revision is required")
    header = (bytes([9]) + struct.pack(">QQQQ", int(time.time()) + 100, chain, args.fee_limit, l1) + token +
              exact_hex(args.genesis, 32) + struct.pack(">QQ", 1, args.lease_revision))
    statement = b"N54V" + board + struct.pack(">Q", args.generation) + sender + header + keccak256(raw)
    args.out.write_text((header + signed(args.authority, statement) + raw).hex() + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for arg in ("unsigned", "authority", "out"):
        parser.add_argument("--" + arg, required=True, type=Path)
    for arg in ("board", "sender", "rpc", "genesis"):
        parser.add_argument("--" + arg, required=True)
    for arg in ("generation", "fee-limit", "lease-revision"):
        parser.add_argument("--" + arg, required=True, type=int)
    parser.add_argument("--token", help="Independently enrolled ERC-20 contract")
    issue(parser.parse_args())
