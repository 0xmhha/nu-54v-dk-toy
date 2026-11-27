#!/usr/bin/env python3
"""Loopback-only Bitcoin regtest, Solana and XRP Ledger wallet authority and review."""
import argparse
import base64
import hashlib
import hmac
import json
import os
import re
import ssl
import struct
import subprocess
import time
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from decimal import Decimal
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

ALPHABET = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
RIPPLE = "rpshnaf39wBUDNEGHJKLM4PQRST7VWXYZ2bcdeCg65jkm8oFqi1tuvAxyz"
TOKENS = bytes.fromhex("06ddf6e1d765a193d9cbe146ceeb79ac1cb485ed5f5b37913a8cf5857eff00a9")
MAX_PACKET = 65_536
LOCAL = {"127.0.0.1", "localhost", "::1"}
METHODS = {
    1: {"getblockhash", "getblockchaininfo", "getrawtransaction", "testmempoolaccept", "sendrawtransaction", "getaddressinfo", "listunspent"},
    2: {"getGenesisHash", "getSignatureStatuses", "isBlockhashValid", "getBlockHeight", "sendTransaction", "getBalance"},
    3: {"server_info", "tx", "ledger_current", "submit", "account_info", "ledger"},
}


def checked(condition, description):
    if not condition:
        raise ValueError(description)


def h256(raw):
    return hashlib.sha256(raw).digest()


def hash160(raw):
    return hashlib.new("ripemd160", h256(raw)).digest()


def d256(raw):
    return h256(h256(raw))


def hexn(value, length):
    checked(isinstance(value, str) and bool(re.fullmatch(f"[a-fA-F0-9]{{{length * 2}}}", value)), "invalid hex field")
    return bytes.fromhex(value)


def number(value, high, low=0):
    checked(type(value) is int and low <= value <= high, "integer outside profile")
    return value


def b58decode(address, alphabet=ALPHABET, max_length=60):
    checked(isinstance(address, str) and 1 <= len(address) <= max_length, "invalid base58 length")
    value = 0
    for symbol in address:
        value = 58 * value + alphabet.index(symbol)
    count = len(address) - len(address.lstrip(alphabet[0]))
    return bytes(count) + value.to_bytes((value.bit_length() + 7) // 8, "big")


def xrpl_id(address):
    raw = b58decode(address, RIPPLE)
    checked(len(raw) == 25 and raw[0] == 0 and bool(any(raw[1:21])) and
            d256(raw[:21])[:4] == raw[21:], "invalid XRPL classic address")
    return raw[1:21]


def xrpl_destination(address, tag, test):
    if tag is not None:
        number(tag, 2**32 - 1)
    if address.startswith("r"):
        return xrpl_id(address), address, tag
    raw = b58decode(address, RIPPLE)
    prefix = bytes((4, 147) if test else (5, 68))
    checked(len(raw) == 35 and raw[:2] == prefix, "XRPL X-address network mismatch")
    checked(d256(raw[:31])[:4] == raw[31:] and any(raw[2:22]), "XRPL X-address checksum")
    flag = raw[22]
    checked(flag in (0, 1) and not any(raw[27 if flag else 23:31]), "XRPL X-address tag encoding")
    embedded = int.from_bytes(raw[23:27], "little") if flag else None
    checked(embedded is None or tag is None or embedded == tag, "XRPL destination tag conflict")
    from wallet_chain_review import classic
    return raw[2:22], classic(raw[2:22]), embedded if embedded is not None else tag


def decimals(source, places, max_units):
    checked(isinstance(source, str) and bool(re.fullmatch(r"[0-9]{1,16}(?:\.[0-9]{1,16})?", source)),
            "invalid positive decimal amount")
    units = Decimal(source) * (10 ** places)
    checked(0 < units <= max_units and int(units) == units, "unsupported amount or precision")
    return int(units)


def var(raw):
    length = len(raw)
    checked(length <= 65535, "oversized field")
    return (bytes([length]) if length < 253 else b"\xfd" + struct.pack("<H", length)) + raw


def kv(key, value):
    return var(key) + var(value)


def txout(value, script):
    return struct.pack("<Q", value) + var(script)


def strip_witness(raw):
    """Return txid preimage for the signer and fail closed on unexpected field sizes."""
    checked(len(raw) >= 10 and len(raw) <= 4000, "unsupported previous transaction size")
    if raw[4:6] != b"\x00\x01":
        return raw
    cursor = 6

    def compact():
        nonlocal cursor
        checked(cursor < len(raw), "truncated previous transaction")
        value = raw[cursor]
        cursor += 1
        if value == 253:
            checked(cursor + 2 <= len(raw), "truncated previous transaction")
            value = int.from_bytes(raw[cursor:cursor + 2], "little")
            cursor += 2
            checked(value >= 253, "noncanonical compact size")
        elif value > 253:
            raise ValueError("unsupported compact size")
        return value

    def take(size):
        nonlocal cursor
        cursor += size
        checked(cursor <= len(raw), "truncated previous transaction")

    inputs = number(compact(), 4, 1)
    for _ in range(inputs):
        take(36)
        take(compact())
        take(4)
    for _ in range(number(compact(), 4, 1)):
        take(8)
        take(compact())
    outputs_end = cursor
    for _ in range(inputs):
        for _ in range(number(compact(), 100)):
            take(compact())
    checked(cursor + 4 == len(raw), "previous transaction trailing data")
    return raw[:4] + raw[6:outputs_end] + raw[-4:]


class LocalOnlyRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *_args):
        raise ValueError("local node redirected the operation")


def rpc_client(endpoint, rpc_login, timeout=8):
    url = urllib.parse.urlsplit(endpoint)
    checked(url.scheme in {"http", "https"} and url.hostname in LOCAL and not url.username and not url.fragment
            and not url.query, "a loopback node is required")
    transport = urllib.request.build_opener(urllib.request.ProxyHandler({}), LocalOnlyRedirect())
    basic = base64.b64encode(rpc_login.encode()).decode() if rpc_login else None

    def call(method, params, relay=False):
        packet = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params}).encode()
        headers = {"Content-Type": "application/json"}
        if basic:
            headers["Authorization"] = "Basic " + basic
        request = urllib.request.Request(endpoint, data=packet, headers=headers)
        try:
            with transport.open(request, timeout=timeout) as response:
                raw = response.read(256_001)
        except urllib.error.HTTPError as response:
            raw = response.read(256_001)
        checked(len(raw) <= 256_000, "oversized local node response")
        reply = json.loads(raw, parse_float=Decimal)
        checked(isinstance(reply, dict) and reply.get("id") == 1, "invalid local node reply")
        if relay:
            return reply
        checked(not reply.get("error") and "result" in reply, "local node did not validate request")
        return reply["result"]

    return call


def wire_value(value):
    if isinstance(value, Decimal):
        return int(value) if value == int(value) else str(value)
    if isinstance(value, list):
        return [wire_value(item) for item in value]
    if isinstance(value, dict):
        return {key: wire_value(item) for key, item in value.items()}
    return value


def key_prefix(key):
    checked(key[0] in (2, 3), "compressed secp256k1 key required")
    return key


def bitcoin_predecessor(rpc, txid):
    hexn(txid, 32)
    packet = rpc("getrawtransaction", [txid, True], relay=True)
    tx = packet.get("result")
    if not tx:
        error = packet.get("error", {}).get("code") if packet.get("error") else None
        checked(error == -5, "original transaction lookup is unavailable")
        return {"id": txid, "state": "absent"}
    checked(tx["txid"] == txid, "original transaction ID did not match")
    confirmations = tx.get("confirmations", 0)
    checked(type(confirmations) is int, "invalid original confirmations")
    if confirmations > 0:
        return {"id": txid, "state": "confirmed"}
    if confirmations < 0 or tx.get("in_active_chain") is False:
        return {"id": txid, "state": "conflicted"}
    packet = rpc("getmempoolentry", [txid], relay=True)
    status = packet.get("result")
    if not status:
        error = packet.get("error", {}).get("code") if packet.get("error") else None
        checked(error == -5, "original mempool lookup is unavailable")
        return {"id": txid, "state": "absent"}
    eligible = (status["descendantcount"] == 1 and tx["version"] == 2 and tx["locktime"] == 0 and
                1 <= len(tx["vin"]) <= 4 and len(tx["vout"]) == 2 and
                all(row.get("sequence") == 0xfffffffd for row in tx["vin"]))
    return {"id": txid, "state": "live" if eligible else "ineligible"}


def submit_bitcoin_replacement(args, rpc, request):
    checked(set(request) == {"original", "id", "signed", "grant", "anchor"}, "invalid replacement submission")
    original = request["original"]
    txid = request["id"]
    digest = hexn(request["grant"], 32)
    hexn(original, 32)
    hexn(txid, 32)
    checked(rpc("getblockchaininfo", [])["chain"] == "regtest" and
            rpc("getblockhash", [0]) == request["anchor"], "wrong Bitcoin genesis or network")
    path = args.review_dir / (digest.hex() + ".json")
    checked(path.stat().st_size <= 12000, "oversized replacement grant")
    grant = bytes.fromhex(json.loads(path.read_text())["grant"])
    checked(217 < len(grant) <= 1800 and h256(grant) == digest and grant[:3] == bytes([11, 1, 1]) and
            grant[23] == 1 and grant[24:56].hex() == original and
            grant[113:145] == h256(request["anchor"].encode("ascii")) and
            int.from_bytes(grant[145:153], "big") == args.lease_revision,
            "replacement does not match the approved grant")
    txt = request["signed"]
    checked(isinstance(txt, str) and bool(re.fullmatch(r"(?:[a-f0-9]{2}){1,1232}", txt)), "invalid signed transaction")
    from wallet_chain_review import Reader, psbt_map
    psbt = Reader(grant[217:])
    psbt.expect(b"psbt\xff")
    unsigned = psbt_map(psbt)[b"\x00"]
    checked(strip_witness(bytes.fromhex(txt)) == unsigned and d256(unsigned)[::-1].hex() == txid,
            "signed transaction does not match the approved PSBT")
    checked(bitcoin_predecessor(rpc, original)["state"] == "live", "original is not a live replaceable mempool transaction")
    accepted, = rpc("testmempoolaccept", [[txt]])
    checked(accepted["allowed"] and accepted["txid"] == txid, "Core rejected replacement preflight")
    checked(trusted_wall(args) < int.from_bytes(grant[7:15], "big"), "replacement authorization expired")
    # Core has no atomic compare-and-submit RPC. Repeat the lookup immediately
    # before the final node call so even preflight-to-submit evictions fail closed.
    checked(bitcoin_predecessor(rpc, original)["state"] == "live", "original left the mempool during replacement preflight")
    checked(trusted_wall(args) < int.from_bytes(grant[7:15], "big"), "replacement authorization expired")
    observed = rpc("sendrawtransaction", [txt])
    checked(observed == txid, "Core returned an unexpected replacement transaction ID")
    return observed


COMMISSIONED = {
    1: "00000008819873e925422c1ff0f99f7cc9bbb232af63a077a480a3633bee1ef6:"
       "00000002a5e0ba0498f1e9f4591af0b66b63c654665efe65206fd0ae7bbaf923",
    2: "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG",
    3: "1:60691D3CA08E04064EBB014D0ADE8DA2CB2BFE0AE9D820D88502FB3A9A8F7149",
}


def validate_commissioned_profiles(rows):
    checked(isinstance(rows, list) and 1 <= len(rows) <= 3, "expected commissioned chain profiles")
    profiles = {1: ("bitcoin-signet", "/bitcoin"), 2: ("solana-devnet", "/solana"), 3: ("xrpl-testnet", "/xrpl")}
    seen = set()
    for row in rows:
        checked(isinstance(row, dict) and set(row) == {"chain", "profile", "anchor", "rpc"},
                "invalid commissioned profile")
        chain = number(row["chain"], 3, 1)
        profile, route = profiles[chain]
        checked(chain not in seen and row["profile"] == profile and row["anchor"] == COMMISSIONED[chain],
                "commissioned profile scope mismatch")
        seen.add(chain)
        endpoint = row["rpc"]
        checked(isinstance(endpoint, str) and len(endpoint) <= 320 and
                re.fullmatch(r"https://(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}(?::[1-9][0-9]{0,4})?" + route, endpoint),
                "commissioned endpoint must be canonical HTTPS DNS")
        components = urllib.parse.urlsplit(endpoint)
        checked(not components.hostname.endswith(".localhost"), "reserved loopback DNS cannot be commissioned")
        checked(components.port != 443, "omit the default HTTPS port")

def verify_commissioned_node(chain, rpc):
    anchor = COMMISSIONED[chain]
    if chain == 1:
        genesis, checkpoint = anchor.split(":")
        valid = (rpc("getblockchaininfo", [])["chain"] == "signet" and
                 rpc("getblockhash", [0]) == genesis and rpc("getblockhash", [321295]) == checkpoint)
    elif chain == 2:
        valid = rpc("getGenesisHash", []) == anchor
    else:
        _, checkpoint = anchor.split(":")
        info = rpc("server_info", [{}])["info"]
        ledger = rpc("ledger", [{"ledger_hash": checkpoint, "transactions": False}])
        valid = (type(info["network_id"]) is int and info["network_id"] == 1 and ledger["validated"] is True and
                 ledger["ledger_hash"] == checkpoint and str(ledger["ledger_index"]) == "22470370")
    checked(valid, "commissioned node identity mismatch")
    return anchor


def bitcoin(rpc, request, account):
    commissioned = request.get("network") == 2
    if commissioned:
        checked(verify_commissioned_node(1, rpc) == request["anchor"], "wrong Signet anchor")
    else:
        checked(rpc("getblockchaininfo", [])["chain"] == "regtest" and
                rpc("getblockhash", [0]) == request["anchor"], "wrong Bitcoin genesis or network")
    checked(not request.get("asset") and request.get("tag") is None, "Bitcoin profile fields")
    public = key_prefix(hexn(request["publicKey"], 33))
    change = key_prefix(hexn(request["change"], 33))
    fingerprint = hexn(request["fingerprint"], 4)
    destination = request["destination"]
    validated = rpc("validateaddress", [destination])
    checked(validated["isvalid"] and destination.startswith("tb1q" if commissioned else "bcrt1q"), "native v0 address on selected Bitcoin network required")
    script = bytes.fromhex(validated["scriptPubKey"])
    checked(len(script) in (22, 34) and script[:2] == bytes([0, len(script) - 2]), "unsupported Bitcoin program")
    own_script = b"\x00\x14" + hash160(public)
    change_script = b"\x00\x14" + hash160(change)
    value = decimals(request["amount"], 8, 2_100_000_000_000_000)
    fee = number(request.get("fee", 1000), 100_000, 100)
    selector = request.get("outpoint")
    if selector is not None:
        checked(isinstance(selector, str) and bool(re.fullmatch(r"[0-9a-f]{64}:[0-9]{1,8}", selector)),
                "invalid selected UTXO")
    replacement, old_fee, previous_change = request.get("replaces"), 0, None
    sequence = 0xfffffffd if request.get("replaceable") is True else 0xffffffff
    if replacement:
        hexn(replacement, 32)
        checked(not selector and sequence == 0xfffffffd, "invalid replacement options")
        mempool = rpc("getmempoolentry", [replacement])
        original = rpc("getrawtransaction", [replacement, True])
        checked(original["txid"] == replacement and not original.get("confirmations") and
                mempool["descendantcount"] == 1 and original["version"] == 2 and original["locktime"] == 0 and
                1 <= len(original["vin"]) <= 4 and len(original["vout"]) == 2 and
                all(item["sequence"] == sequence for item in original["vin"]),
                "original is no longer an eligible replaceable transaction")
        external, returned = original["vout"]
        checked(external["scriptPubKey"]["hex"] == script.hex() and
                Decimal(external["value"]) * 10**8 == value and
                returned["scriptPubKey"]["hex"] == change_script.hex(), "replacement changes original recipient")
        old_fee = int(Decimal(mempool["fees"]["base"]) * 10**8)
        previous_change = int(Decimal(returned["value"]) * 10**8)
        checked(fee > old_fee and previous_change - fee + old_fee >= 330, "replacement fee leaves dust or does not increase")
        coins = []
        for item in original["vin"]:
            parent = rpc("getrawtransaction", [item["txid"], True])
            checked(parent.get("confirmations", 0) > 0, "replacement parent is not confirmed")
            coin = parent["vout"][item["vout"]]
            checked(coin["scriptPubKey"]["hex"] == own_script.hex(), "replacement input belongs to another account")
            coins.append({"txid": item["txid"], "vout": item["vout"], "amount": coin["value"]})
    else:
        available = sorted(rpc("listunspent", [1, 9_999_999]),
                           key=lambda coin: (-Decimal(coin["amount"]), coin["txid"], coin["vout"]))
        coins = []
        selected_value = 0
        for coin in available:
            if coin["scriptPubKey"] != own_script.hex() or (selector and selector != f'{coin["txid"]}:{coin["vout"]}'):
                continue
            coins.append(coin)
            selected_value += int(Decimal(coin["amount"]) * 10**8)
            if selected_value >= value + fee or len(coins) == 4:
                break
        checked(selected_value >= value + fee, "confirmed watch-only funds required")
    total = sum(int(Decimal(coin["amount"]) * 10**8) for coin in coins)
    remainder = total - value - fee
    checked(remainder == 0 or remainder >= 330, "dust change would exceed the approved fee")
    checked(previous_change is None or remainder == previous_change - fee + old_fee, "replacement does not spend exact original inputs")
    path = (0x80000054, 0x80000001, 0x80000000 | account)
    metadata = fingerprint + struct.pack("<IIIII", *path, 0, 0)
    tx_inputs, input_maps, outpoints = [], [], []
    for coin in coins:
        prev = strip_witness(bytes.fromhex(rpc("getrawtransaction", [coin["txid"], False])))
        outpoint = d256(prev)
        checked(outpoint[::-1].hex() == coin["txid"], "unverifiable previous transaction")
        amount = int(Decimal(coin["amount"]) * 10**8)
        tx_inputs.append(outpoint + struct.pack("<I", coin["vout"]) + b"\x00" + struct.pack("<I", sequence))
        input_maps.append(kv(b"\x00", prev) + kv(b"\x01", txout(amount, own_script)) +
                          kv(b"\x03", b"\x01\x00\x00\x00") + kv(b"\x06" + public, metadata) + b"\x00")
        outpoints.append(f'{coin["txid"]}:{coin["vout"]}')
    checked(len(outpoints) == len(set(outpoints)), "duplicate funding outpoint")
    outputs = txout(value, script) + (txout(remainder, change_script) if remainder else b"")
    unsigned = (b"\x02\x00\x00\x00" + bytes([len(coins)]) + b"".join(tx_inputs) +
                bytes([2 if remainder else 1]) + outputs + bytes(4))
    psbt = b"psbt\xff" + kv(b"\x00", unsigned) + b"\x00" + b"".join(input_maps) + b"\x00"
    if remainder:
        psbt += kv(b"\x02" + change, fingerprint + struct.pack("<IIIII", *path, 1, 0)) + b"\x00"
    context = b"\x01" + bytes.fromhex(replacement) + struct.pack(">Q", old_fee) + bytes(49) if replacement else bytes(90)
    return psbt, fee, context, {
        "destination": destination, "amountSats": value, "feeSats": fee, "replaceable": sequence == 0xfffffffd,
        "utxo": outpoints[0], "inputs": outpoints, "replaces": replacement, "allOutputs": [
            {"scriptHex": script.hex(), "amountSats": value},
            *([{"scriptHex": change_script.hex(), "amountSats": remainder, "change": True}] if remainder else [])],
    }


def solana(rpc, request, policy):
    checked(rpc("getGenesisHash", []) == request["anchor"], "wrong Solana genesis")
    payer = hexn(request["publicKey"], 32)
    destination = b58decode(request["destination"])
    checked(len(destination) == 32 and any(destination) and destination != payer, "invalid Solana destination")
    checked(request.get("tag") is None, "Solana destination tags are unsupported")
    block = rpc("getLatestBlockhash", [{"commitment": "confirmed"}])["value"]
    key_hash = b58decode(block["blockhash"])
    last = number(block["lastValidBlockHeight"], 2**53 - 1, 1)
    height = number(rpc("getBlockHeight", [{"commitment": "confirmed"}]), 2**53 - 1)
    checked(len(key_hash) == 32 and 80 <= last - height <= 150, "stale Solana blockhash")
    asset = request.get("asset")
    mint = bytes(32)
    precision = 0
    review = {"feePayerHex": payer.hex(), "destination": request["destination"], "blockhash": block["blockhash"],
              "lastValidBlockHeight": last}
    if asset:
        checked(policy.spl_mint and asset == policy.spl_mint and request.get("source"), "unlisted SPL mint or source")
        mint = b58decode(asset)
        source = b58decode(request["source"])
        checked(len(source) == 32 and len(mint) == 32 and len({payer, source, destination, mint, TOKENS}) == 5,
                "invalid SPL accounts")
        for address in [request["source"], request["destination"]]:
            token_account = rpc("getAccountInfo", [address, {"encoding": "jsonParsed", "commitment": "confirmed"}])["value"]
            checked(token_account["owner"] == "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA" and
                    token_account["data"]["parsed"]["info"]["mint"] == asset, "invalid SPL token account or mint")
            info = token_account["data"]["parsed"]["info"]
            checked(info["tokenAmount"]["decimals"] == policy.spl_decimals and info["state"] == "initialized",
                    "SPL decimals or token account state")
            if address == request["source"]:
                checked(b58decode(info["owner"]) == payer, "owner mismatch on SPL source")
        precision = policy.spl_decimals
        value = decimals(request["amount"], precision, 2**64 - 1)
        message = (b"\x01\x00\x02\x05" + payer + source + destination + mint + TOKENS + key_hash +
                   b"\x01\x04\x04\x01\x03\x02\x00\x0a\x0c" + struct.pack("<QB", value, precision))
        review.update({"mint": asset, "source": request["source"], "tokenPrecision": precision})
    else:
        value = decimals(request["amount"], 9, 2**64 - 1)
        message = (b"\x01\x00\x01\x03" + payer + destination + bytes(32) + key_hash +
                   b"\x01\x02\x02\x00\x01\x0c\x02\x00\x00\x00" + struct.pack("<Q", value))
    quote = rpc("getFeeForMessage", [base64.b64encode(message).decode(), {"commitment": "confirmed"}])
    fee = number(quote["value"], 1_000_000, 1)
    return message, fee, key_hash + struct.pack(">QQQ", height, last, fee) + mint + bytes([precision, bool(asset)]), {
        **review, "amountBaseUnits": value, "feeLamports": fee,
    }


def issued_word(amount):
    value = Decimal(amount)
    checked(value > 0, "issued amount must be positive")
    digits = value.normalize().as_tuple()
    checked(len(digits.digits) <= 16, "XRPL 16 significant digits exceeded")
    coefficient = int("".join(map(str, digits.digits)).ljust(16, "0"))
    exponent = digits.exponent + len(digits.digits) - 16
    checked(-96 <= exponent <= 80, "XRPL decimal exponent outside range")
    return struct.pack(">Q", (3 << 62) | ((exponent + 97) << 54) | coefficient)


def xrp_drops(value):
    return struct.pack(">Q", (1 << 62) | value)


def xrpl(rpc, request, policy):
    if request.get("network") == 2:
        checked(verify_commissioned_node(3, rpc) == request["anchor"], "wrong XRP Ledger Testnet anchor")
    else:
        checked(request["anchor"] == str(policy.xrpl_network) and
                rpc("server_info", [{}])["info"]["network_id"] == policy.xrpl_network, "wrong XRP Ledger network")
    public = key_prefix(hexn(request["publicKey"], 33))
    sender = xrpl_id(request["sender"])
    dest, destination, tag = xrpl_destination(request["destination"], request.get("tag"), True)
    checked(hash160(public) == sender and sender != dest, "XRPL account or destination invalid")
    info = rpc("account_info", [{"account": request["sender"], "ledger_index": "validated"}])["account_data"]
    sequence = number(info["Sequence"], 2**32 - 1, 1)
    head = number(rpc("ledger_current", [{}])["ledger_current_index"], 2**32 - 21, 1)
    last = head + 20
    fee = number(int(rpc("fee", [{}])["drops"]["open_ledger_fee"]), 200_000, 1)
    asset = request.get("asset")
    currency, issuer, sendmax = bytes(20), bytes(20), b""
    if asset:
        checked(policy.xrpl_currency and policy.xrpl_issuer and asset == policy.xrpl_currency, "unlisted XRPL currency")
        checked(isinstance(request["amount"], str) and bool(re.fullmatch(r"[0-9]{1,16}(?:\.[0-9]{1,16})?",
                request["amount"])), "invalid issued amount")
        currency = bytes(12) + asset.encode("ascii") + bytes(5)
        issuer = xrpl_id(policy.xrpl_issuer)
        checked(issuer != dest, "issuer-as-destination wildcard is unsupported")
        amount = issued_word(request["amount"]) + currency + issuer
        if issuer != sender:
            sendmax = b"\x69" + amount
        lines = rpc("account_lines", [{"account": destination, "peer": policy.xrpl_issuer,
                                       "ledger_index": "validated"}])["lines"]
        checked(any(line["currency"] == asset and line["account"] == policy.xrpl_issuer and
                    Decimal(line["limit"]) - Decimal(line["balance"]) >= Decimal(request["amount"])
                    for line in lines), "recipient has insufficient trusted issuer allowance")
    else:
        drops = decimals(request["amount"], 6, 100_000_000_000_000_000)
        amount = xrp_drops(drops)
        ledger = rpc("server_state", [{}])["state"]["validated_ledger"]
        reserve = int(ledger["reserve_base"]) + int(info["OwnerCount"]) * int(ledger["reserve_inc"])
        checked(int(info["Balance"]) - drops - fee >= reserve, "XRP reserve would be exceeded")
    raw = (b"\x12\x00\x00\x22\x80\x00\x00\x00\x24" + struct.pack(">I", sequence) +
           (b"\x2e" + struct.pack(">I", tag) if tag is not None else b"") + b"\x20\x1b" +
           struct.pack(">I", last) + b"\x61" + amount + b"\x68" + xrp_drops(fee) + sendmax +
           b"\x73\x21" + public + b"\x81\x14" + sender + b"\x83\x14" + dest)
    context = struct.pack(">IIII", head, 25, sequence, 0) + currency + issuer + bytes([bool(asset)]) + bytes(33)
    return raw, fee, context, {
        "destination": destination, "amount": request["amount"], "currency": asset or "XRP",
        "issuer": policy.xrpl_issuer if asset else None, "feeDrops": fee, "destinationTag": tag,
        "sequence": sequence, "lastLedgerSequence": last,
    }


def sign(key, message):
    checked(not key.resolve().is_relative_to(Path(__file__).resolve().parents[3]),
            "store laboratory keys outside the checkout")
    returncode = subprocess.run(["openssl", "pkey", "-in", str(key), "-text_pub", "-noout"],
                                capture_output=True, check=True)
    checked(b"prime256v1" in returncode.stdout, "P-256 signing key required")
    der = subprocess.run(["openssl", "dgst", "-sha256", "-sign", str(key)], input=message,
                         capture_output=True, check=True).stdout
    checked(der[0] == 0x30 and der[1] == len(der) - 2 and der[2] == 2, "invalid authority signature")
    first = der[3]
    checked(der[4 + first] == 2 and der[5 + first] == len(der) - 6 - first, "invalid authority signature")
    return int.from_bytes(der[4:4 + first], "big").to_bytes(32, "big") + int.from_bytes(
        der[6 + first:], "big").to_bytes(32, "big")


def submit_commissioned(args, chain, rpc, request):
    checked(set(request) == {"grant", "id", "signed"}, "invalid commissioned submission")
    digest = hexn(request["grant"], 32)
    path = args.review_dir / (digest.hex() + ".json")
    checked(path.stat().st_size <= 12000, "oversized grant")
    grant = bytes.fromhex(json.loads(path.read_text())["grant"])
    checked(217 < len(grant) <= 1800 and h256(grant) == digest and grant[:3] == bytes([11, chain, 2]) and
            grant[113:145] == h256(COMMISSIONED[chain].encode()) and
            int.from_bytes(grant[145:153], "big") == args.lease_revision, "commissioned grant scope mismatch")
    encoded = request["signed"]
    checked(isinstance(encoded, str) and re.fullmatch(r"(?:[a-f0-9]{2}){1,1232}", encoded), "invalid signed bytes")
    signed, unsigned = bytes.fromhex(encoded), grant[217:]
    txid = request["id"]
    expiry = int.from_bytes(grant[7:15], "big")

    def current():
        checked(trusted_wall(args) < min(expiry, args.commissioned_expires), "commissioned grant expired")

    if chain == 1:
        from wallet_chain_review import Reader, psbt_map
        psbt = Reader(unsigned)
        psbt.expect(b"psbt\xff")
        template = psbt_map(psbt)[b"\x00"]
        checked(strip_witness(signed) == template and d256(template)[::-1].hex() == txid, "transaction differs from PSBT")
        current()
        [accept] = rpc("testmempoolaccept", [[encoded]])
        checked(accept["allowed"] is True and accept["txid"] == txid, "Signet preflight refused")
        current()
        result = rpc("sendrawtransaction", [encoded])
    elif chain == 2:
        checked(signed[:1] == b"\x01" and signed[65:] == unsigned and
                isinstance(txid, str) and b58decode(txid, max_length=90) == signed[1:65], "Solana message differs from grant")
        current()
        result = rpc("sendTransaction", [base64.b64encode(signed).decode(),
                     {"encoding": "base64", "skipPreflight": False, "preflightCommitment": "confirmed"}])
    else:
        boundary = len(unsigned) - 44
        length = signed[boundary + 1]
        checked(signed[boundary] == 0x74 and 8 <= length <= 72 and
                signed[:boundary] + signed[boundary + 2 + length:] == unsigned, "XRP Ledger bytes differ from grant")
        computed = hashlib.sha512(b"TXN\x00" + signed).hexdigest()[:64].upper()
        checked(computed == txid, "XRP Ledger transaction ID mismatch")
        current()
        result = rpc("submit", [{"tx_blob": encoded.upper(), "fail_hard": True}])["tx_json"]["hash"]
    checked(result == txid, "commissioned transaction ID mismatch")
    return result


def prepare(args, rpc, request):
    allowed = {"chain", "network", "account", "generation", "board", "publicKey", "sender", "destination",
               "amount", "anchor", "asset", "source", "tag", "change", "fingerprint", "fee", "outpoint", "replaceable", "replaces", "leaseRevision"}
    checked(isinstance(request, dict) and set(request) <= allowed, "unsupported wallet transfer field")
    chain = number(request["chain"], 3, 1)
    network = 2 if getattr(args, "commissioned_token_file", None) else 1
    if network == 2:
        checked(request["anchor"] == COMMISSIONED[chain] and not request.get("asset") and
                not request.get("replaces") and not request.get("replaceable"), "commissioned native transfer scope")
    checked(request["network"] == network and request["board"] == args.board and
            request["generation"] == str(args.generation) and request["leaseRevision"] == str(args.lease_revision),
            "wallet account generation, lease or network mismatch")
    account = number(request["account"], 99)
    checked(chain == 1 or not request.get("replaces"), "replacement is Bitcoin only")
    if chain == 1:
        raw, fee, context, review = bitcoin(rpc, request, account)
    elif chain == 2:
        raw, fee, context, review = solana(rpc, request, args)
    else:
        raw, fee, context, review = xrpl(rpc, request, args)
    header = (struct.pack(">BBBIQQ", 11, chain, network, account, trusted_wall(args) + 55, fee) +
              context + h256(request["anchor"].encode("ascii")) + struct.pack(">Q", args.lease_revision))
    checked(len(header) == 153 and 0 < len(raw) <= 1583, "grant exceeds authenticated transport capacity")
    statement = b"N54M" + hexn(args.board, 32) + struct.pack(">Q", args.generation) + header + h256(raw)
    grant = header + sign(args.authority, statement) + raw
    details = {"chain": chain, "account": account, "network": request["anchor"], "unsignedHex": raw.hex(), **review}
    digest = h256(grant).hex()
    with open(args.review_dir / (digest + ".json"), "x", opener=private_file) as output:
        json.dump({"grant": grant.hex(), "review": details}, output, indent=2)
    print("Independent approval awaiting review " + digest, flush=True)
    return {"grant": grant.hex()}


def private_file(path, flags):
    return os.open(path, flags, 0o600)


def bip48_configuration(path, network=1):
    document = json.loads(path.read_text())
    descriptors = document["descriptors"]
    fee, owner = number(document["maxFee"], 1_000_000, 1), number(document["owner"], 2)
    account = number(document["account"], 99)
    roots = []
    for branch, descriptor in enumerate(descriptors):
        prefix = "wsh(sortedmulti(2,"
        wrapper, *_ = descriptor.split("#")
        checked(wrapper.startswith(prefix) and wrapper.endswith("))"), "BIP48 sortedmulti required")
        expressions = wrapper[len(prefix):-2].split(",")
        checked(len(expressions) == 3, "2-of-3 descriptor required")
        children = []
        for expression in expressions:
            pattern = rf"\[([a-f0-9]{{8}})/48h/1h/{account}h/2h\](tpub[1-9A-HJ-NP-Za-km-z]+)/{branch}/\*"
            match = re.fullmatch(pattern, expression)
            checked(match is not None, "unsupported BIP48 origin/network/branch")
            raw = b58decode(match[2], max_length=112)
            checked(len(raw) == 82 and raw[:5].hex() == "043587cf04" and
                    raw[9:13].hex() == "80000002" and d256(raw[:78])[:4] == raw[78:] and
                    raw[45] in (2, 3), "invalid BIP48 tpub")
            children.append(bytes.fromhex(match[1]) + raw[13:78])
        roots.append(children)
    checked(len(roots) == 2 and roots[0] == roots[1] and len(set(roots[0])) == 3, "descriptor roles differ")
    wire = struct.pack(">BBBIQBB", 17, 1, network, account, fee, owner, 2) + b"".join(roots[0])
    return wire, descriptors


def review_record(args, grant, details):
    digest = h256(grant).hex()
    path = args.review_dir / (digest + ".json")
    if not path.exists():
        with open(path, "x", opener=private_file) as output:
            json.dump({"grant": grant.hex(), "review": details}, output, indent=2)
    print("Independent approval awaiting review " + digest, flush=True)


def bip48_request(args, rpc, request, prepare_psbt):
    checked(args.bip48_policy is not None, "no BIP48 policy configured")
    checked(set(request) <= {"board", "generation", "leaseRevision", "anchor", "recipient", "amount", "counterpart", "network", "account"} and
            request["board"] == args.board and request["generation"] == str(args.generation) and
            request["leaseRevision"] == str(args.lease_revision), "unverified wallet session")
    commissioned = bool(getattr(args, "commissioned_token_file", None))
    network = 2 if commissioned else 1
    checked(request.get("network", 1) == network, "wrong BIP48 profile network")
    if commissioned:
        checked(verify_commissioned_node(1, rpc) == request["anchor"], "wrong Signet anchor")
    else:
        checked(rpc("getblockchaininfo", [])["chain"] == "regtest" and
                rpc("getblockhash", [0]) == request["anchor"], "wrong Bitcoin genesis or network")
    policy, descriptors = bip48_configuration(args.bip48_policy, network)
    checked(request.get("account", policy[6]) == policy[6], "wrong BIP48 account")
    pin = h256(policy)
    if not prepare_psbt:
        review_record(args, policy, {"bitcoinRegistry": pin.hex()})
        return {"policy": policy.hex(), "descriptors": descriptors}
    received = request.get("counterpart")
    if received:
        checked(isinstance(received, str) and len(received) <= 2376, "invalid counterparty PSBT")
        psbt = received
    else:
        destination = request["recipient"]
        value = decimals(request["amount"], 8, 2_100_000_000_000_000)
        validated = rpc("validateaddress", [destination])
        checked(destination.startswith("tb1q" if commissioned else "bcrt1q") and validated["isvalid"], "native regtest SegWit required")
        change = rpc("deriveaddresses", [descriptors[1], [0, 0]])[0]
        psbt = rpc("walletcreatefundedpsbt", [[], [{destination: str(Decimal(value) / 10**8)}], 0,
                                             {"fee_rate": 2, "changeAddress": change, "changePosition": 1}, True])["psbt"]
    raw = base64.b64decode(psbt, validate=True)
    checked(0 < len(raw) <= 1780, f"PSBT of {len(raw)} bytes exceeds authenticated transport")
    from wallet_chain_review import Reader, psbt_map, bip48_input_count, bip48_prevout
    source = Reader(raw)
    source.expect(b"psbt\xff")
    global_map = psbt_map(source, 15)
    seen = set()
    for _ in range(bip48_input_count(global_map)):
        txid, vout, amount, script = bip48_prevout(psbt_map(source, 15))
        checked((txid,vout) not in seen, "duplicate BIP48 prevout")
        seen.add((txid,vout))
        coin = rpc("gettxout", [txid, vout, True])
        checked(coin and coin["confirmations"] > 0 and coin["scriptPubKey"]["hex"] == script.hex() and
                Decimal(coin["value"]) * 10**8 == amount, "PSBT input is absent from the selected network")
    fee_limit = int.from_bytes(policy[7:15], "big")
    header = (struct.pack(">BBBIQQ", 18, 1, network, int.from_bytes(policy[3:7], "big"), trusted_wall(args) + 55, fee_limit) +
              pin + bytes(58) + h256(request["anchor"].encode("ascii")) + struct.pack(">Q", args.lease_revision))
    grant = header + sign(args.authority, b"N54M" + hexn(args.board, 32) + struct.pack(">Q", args.generation) +
                          header + h256(raw)) + raw
    review_record(args, grant, {"unsignedHex": raw.hex(), "bitcoinRegistry": pin.hex()})
    return {"grant": grant.hex()}


def trusted_submission_time(args, request):
    checked(set(request) == {"nonce", "board", "generation", "leaseRevision"} and
            request["board"] == args.board and request["generation"] == str(args.generation) and
            request["leaseRevision"] == str(args.lease_revision), "unexpected time proof scope")
    nonce = hexn(request["nonce"], 32)
    checked(any(nonce), "invalid time proof challenge")
    now = trusted_wall(args)
    scope = struct.pack(">QQ", args.generation, args.lease_revision)
    proof = sign(args.authority, b"N54C" + hexn(args.board, 32) + scope + nonce + struct.pack(">Q", now))
    return {"time": now, "proof": proof.hex()}


def trusted_wall(args):
    now = int(time.time())
    projected = args.wall_start + int(time.monotonic() - args.mono_start)
    floor_path = args.review_dir / ".submission-time-floor"
    floor = int(floor_path.read_text()) if floor_path.exists() else 0
    checked(now > 0 and now >= floor and now >= projected - 1, "local authority time moved backward")
    with tempfile.NamedTemporaryFile(mode="w", dir=args.review_dir, delete=False) as temporary:
        temporary.write(str(now))
        temporary.flush()
        os.fsync(temporary.fileno())
    os.replace(temporary.name, floor_path)
    return now


def serve(args):
    checked(args.generation > 0 and args.generation < 2**64 and len(hexn(args.board, 32)) == 32,
            "invalid board enrollment binding")
    checked(args.spl_mint is None or len(b58decode(args.spl_mint)) == 32, "invalid allowed mint")
    checked(args.xrpl_currency is None or bool(re.fullmatch(r"[A-Z0-9]{3}", args.xrpl_currency)),
            "invalid allowed issued currency")
    checked(args.xrpl_issuer is None or len(xrpl_id(args.xrpl_issuer)) == 20, "invalid allowed XRPL issuer")
    args.review_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
    args.wall_start, args.mono_start = int(time.time()), time.monotonic()
    credentials = args.bitcoin_cookie.read_text().strip() if args.bitcoin_cookie else None
    nodes = {i: rpc_client(value, credentials if i == 1 else None) for i, value in
             enumerate([args.bitcoin, args.solana, args.xrpl], 1) if value}
    checked(bool(nodes), "configure a local node")
    token_hashes = None
    if args.commissioned_token_file:
        checked(args.commissioned_token_file.stat().st_mode & 0o077 == 0, "private credential file required")
        token_hashes = json.loads(args.commissioned_token_file.read_text())
        checked(isinstance(token_hashes, dict) and set(token_hashes) == {str(chain) for chain in nodes} and
                all(isinstance(value, str) and re.fullmatch(r"[0-9a-f]{64}", value) for value in token_hashes.values()),
                "route-scoped SHA256 credentials required")
        checked(args.commissioned_expires and args.commissioned_expires > int(time.time()),
                "commissioned credential expiration required")
        for chain, rpc in nodes.items():
            verify_commissioned_node(chain, rpc)

    class Gateway(BaseHTTPRequestHandler):
        def do_POST(self):
            try:
                chain = {"/bitcoin": 1, "/solana": 2, "/xrpl": 3}[self.path]
                rpc = nodes[chain]
                size = number(int(self.headers["Content-Length"]), MAX_PACKET, 2)
                packet = json.loads(self.rfile.read(size))
                checked(packet["jsonrpc"] == "2.0" and packet["id"] == 1, "invalid JSON-RPC envelope")
                method = packet["method"]
                if token_hashes is not None:
                    checked(trusted_wall(args) < args.commissioned_expires, "commissioned credential expired")
                    token = packet.get("walletToken")
                    checked(isinstance(token, str) and re.fullmatch(r"[0-9a-f]{64}", token) and
                            hmac.compare_digest(h256(token.encode()).hex(), token_hashes[str(chain)]), "wallet authorization required")
                    checked(method in (METHODS[chain] - {"sendrawtransaction", "sendTransaction", "submit"}) or
                            method in {"nu54_walletTime", "nu54_prepareWalletTransfer", "nu54_submitCommissionedTransfer"} or
                            (chain == 1 and method in {"nu54_bip48Policy", "nu54_bip48Prepare"}),
                            "method outside commissioned allowlist")
                    verify_commissioned_node(chain, rpc)
                if token_hashes is not None and method == "nu54_submitCommissionedTransfer":
                    request, = packet["params"]
                    reply = {"result": submit_commissioned(args, chain, rpc, request)}
                elif chain == 1 and method in {"nu54_bip48Policy", "nu54_bip48Prepare"}:
                    request, = packet["params"]
                    reply = {"result": bip48_request(args, rpc, request, method == "nu54_bip48Prepare")}
                elif chain == 1 and method == "nu54_bitcoinReplacementStatus":
                    txid, = packet["params"]
                    checked(rpc("getblockchaininfo", [])["chain"] == "regtest", "Bitcoin regtest required")
                    reply = {"result": bitcoin_predecessor(rpc, txid)}
                elif chain == 1 and method == "nu54_submitBitcoinReplacement":
                    request, = packet["params"]
                    reply = {"result": submit_bitcoin_replacement(args, rpc, request)}
                elif method == "nu54_walletTime":
                    request, = packet["params"]
                    reply = {"result": trusted_submission_time(args, request)}
                elif method == "nu54_prepareWalletTransfer":
                    request, = packet["params"]
                    checked(chain == request["chain"], "path and chain mismatch")
                    reply = {"result": prepare(args, rpc, request)}
                elif method in METHODS[chain]:
                    if chain == 1 and token_hashes is None:
                        checked(rpc("getblockchaininfo", [])["chain"] == "regtest", "Bitcoin regtest required")
                    reply = rpc(method, packet["params"], relay=True)
                else:
                    raise ValueError("method outside laboratory allowlist")
                result = {"id": 1, "jsonrpc": "2.0", **reply}
            except (ValueError, KeyError, IndexError, TypeError, OSError, subprocess.SubprocessError) as error:
                result = {"id": 1, "jsonrpc": "2.0", "result": None, "error": {"code": -32602, "message": "Commissioned request refused" if token_hashes is not None else str(error)}}
            data = json.dumps(wire_value(result), allow_nan=False).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

    server = HTTPServer(("127.0.0.1", args.port), Gateway)
    tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    tls.load_cert_chain(args.tls_cert, args.tls_key)
    server.socket = tls.wrap_socket(server.socket, server_side=True)
    server.serve_forever()


def display(args):
    checked(args.review.stat().st_size <= 12_000, "oversized review document")
    record = json.loads(args.review.read_text())
    grant = bytes.fromhex(record["grant"])
    hashed = h256(grant)
    checked(args.review.name == hashed.hex() + ".json", "review and grant mismatch")
    registration = len(grant) == 224 and grant[0] == 17
    from wallet_chain_review import decode, review_bip48
    if registration:
        print(json.dumps({"register": "Bitcoin Signet BIP48 native SegWit" if grant[2] == 2 else "Bitcoin regtest BIP48 native SegWit", "threshold": grant[16],
            "account": int.from_bytes(grant[3:7], "big"), "feeCap": int.from_bytes(grant[7:15], "big"),
            "ownSlot": grant[15], "policy": hashed.hex(),
            "cosigners": [{"fingerprint": grant[i:i+4].hex(), "xpubMaterial": grant[i+4:i+69].hex()}
                         for i in (17, 86, 155)]}, indent=2))
    elif grant and grant[0] == 18:
        checked(217 < len(grant) <= 1997 and grant[1] == 1 and grant[2] in (1, 2) and
                int.from_bytes(grant[7:15], "big") > int(time.time()) and
                record["review"]["unsignedHex"] == grant[217:].hex(),
                "expired or unsupported BIP48 grant")
        checked(args.bip48_policy and args.bitcoin, "independent BIP48 policy and loopback descriptor engine required")
        policy, descriptors = bip48_configuration(args.bip48_policy, grant[2])
        checked(hmac.compare_digest(h256(policy), grant[23:55]) and grant[3:7] == policy[3:7] and
                0 < int.from_bytes(grant[15:23], "big") <= int.from_bytes(policy[7:15], "big"),
                "grant differs from independently configured BIP48 policy")
        rpc = rpc_client(args.bitcoin, args.bitcoin_cookie.read_text().strip() if args.bitcoin_cookie else None)
        node = rpc("getblockchaininfo", [])["chain"]
        checked(node == ("signet" if grant[2] == 2 else "regtest"), "wrong independent descriptor engine network")
        anchor = COMMISSIONED[1] if grant[2] == 2 else rpc("getblockhash", [0])
        checked(hmac.compare_digest(h256(anchor.encode("ascii")), grant[113:145]), "wrong BIP48 grant network")

        def script(branch, index):
            derived, = rpc("deriveaddresses", [descriptors[branch], [index, index]])
            details = rpc("validateaddress", [derived])
            checked(details["isvalid"] and details["isscript"], "unexpected multisig descriptor output")
            return hexn(details["scriptPubKey"], 34)

        print(json.dumps(review_bip48(grant, policy, script), indent=2))
    else:
        checked(217 < len(grant) <= 1800 and grant[0] == 11 and grant[2] == 1 and
                int.from_bytes(grant[7:15], "big") > int(time.time()) and
                record["review"]["unsignedHex"] == grant[217:].hex(), "expired or unsupported grant")
        print(json.dumps(decode(grant), indent=2))
    checked(input("Type APPROVE after reviewing the exact request: ") == "APPROVE", "approval declined")
    if not registration:
        checked(int.from_bytes(grant[7:15], "big") > int(time.time()), "approval expired")
    proof = sign(args.display_key, b"N54D" + hexn(args.challenge, 32) + hashed)
    print(proof.hex())


def display_export(args):
    body = struct.pack(">BBBI", 13, 1, 2 if args.signet else 1, args.account)
    if args.bip48:
        body += bytes([48])
    description = "BIP48 cosigner public key" if args.bip48 else "BIP84 public watch-only descriptors"
    network = "Signet" if args.signet else "regtest"
    print(f"Bitcoin {network}, account index {args.account}. Export {description}.")
    if not args.bip48:
        print("Native SegWit receiving and change branches. This export links past and future addresses.")
    checked(input("Type APPROVE after verifying the requested account: ") == "APPROVE", "approval declined")
    print(sign(args.display_key, b"N54D" + hexn(args.challenge, 32) + h256(body)).hex())


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    server = commands.add_parser("serve")
    server.set_defaults(run=serve)
    for name in ("authority", "tls-cert", "tls-key", "review-dir"):
        server.add_argument("--" + name, required=True, type=Path)
    server.add_argument("--board", required=True)
    server.add_argument("--generation", type=int, required=True)
    server.add_argument("--lease-revision", type=lambda value: number(int(value), 2**64 - 1, 1), required=True)
    server.add_argument("--port", type=int, default=8443)
    server.add_argument("--commissioned-token-file", type=Path)
    server.add_argument("--commissioned-expires", type=int)
    server.add_argument("--bitcoin")
    server.add_argument("--bitcoin-cookie", type=Path)
    server.add_argument("--bip48-policy", type=Path)
    server.add_argument("--solana")
    server.add_argument("--xrpl")
    server.add_argument("--spl-mint")
    server.add_argument("--spl-decimals", type=lambda value: number(int(value), 18), default=6)
    server.add_argument("--xrpl-currency")
    server.add_argument("--xrpl-issuer")
    server.add_argument("--xrpl-network", type=lambda value: number(int(value), 1024, 1), default=1)
    review = commands.add_parser("display")
    review.set_defaults(run=display)
    review.add_argument("--review", type=Path, required=True)
    review.add_argument("--challenge", required=True)
    review.add_argument("--display-key", type=Path, required=True)
    review.add_argument("--bip48-policy", type=Path)
    review.add_argument("--bitcoin")
    review.add_argument("--bitcoin-cookie", type=Path)
    export = commands.add_parser("display-export")
    export.set_defaults(run=display_export)
    export.add_argument("--bip48", action="store_true")
    export.add_argument("--signet", action="store_true")
    export.add_argument("--account", type=lambda value: number(int(value), 99), required=True)
    export.add_argument("--challenge", required=True)
    export.add_argument("--display-key", type=Path, required=True)
    args = parser.parse_args()
    args.run(args)
