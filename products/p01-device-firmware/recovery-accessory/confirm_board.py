#!/usr/bin/env python3
"""Run on the independent display or registered recovery device, with its own protected key."""

import argparse
import hashlib
import getpass
import json
import re
import asyncio
import sys
import time
import warnings
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
from enroll_board import GATT, exact_hex, public_file, signed


def require_confirmation():
    if not sys.stdin.isatty() or not sys.stdout.isatty() or input("Type AUTHORIZE to confirm: ") != "AUTHORIZE":
        raise ValueError("Independent confirmation refused")


def policy_body(statement, expected, now):
    """Validate every displayed policy field against an independently supplied record."""
    if len(statement) != 232 or statement[:4] not in (b"N54I", b"N54P"):
        raise ValueError("Malformed policy statement")
    if not isinstance(expected, dict) or set(expected) != {
        "board", "generation", "chain", "asset", "destination", "per_transfer", "daily",
        "previous_destination", "previous_per_transfer", "previous_daily", "revision", "not_before", "expires", "kind",
    }:
        raise ValueError("Invalid independent policy record")
    integer = lambda start: int.from_bytes(statement[start:start + 8], "big")
    kind = "initialize" if statement[:4] == b"N54I" else "change"
    board, generation, revision, chain = statement[4:36], integer(100), integer(108), integer(116)
    asset, old_target, target = statement[124:144], statement[144:164], statement[164:184]
    old_per, old_daily, per, daily = integer(184), integer(192), integer(200), integer(208)
    try:
        expected_old_target = bytes.fromhex(expected["previous_destination"].removeprefix("0x"))
    except (TypeError, ValueError, AttributeError) as error:
        raise ValueError("Invalid previous destination") from error
    if len(expected_old_target) != 20:
        raise ValueError("Invalid previous destination")
    not_before, expires = integer(216), integer(224)
    if (board != exact_hex(expected["board"], 32) or generation != expected["generation"] or
        chain != expected["chain"] or asset != exact_hex(expected["asset"], 20) or
        target != exact_hex(expected["destination"], 20) or per != expected["per_transfer"] or
        daily != expected["daily"] or old_target != expected_old_target or
        old_per != expected["previous_per_transfer"] or old_daily != expected["previous_daily"] or
        kind != expected["kind"] or revision != expected["revision"] or not_before != expected["not_before"] or
        expires != expected["expires"] or not revision or not generation or not chain or not per or per > daily or
        not any(asset) or not any(target) or not any(statement[36:52]) or not any(statement[52:68]) or
        not any(statement[68:100]) or any(type(expected[k]) is not int for k in
          ("generation", "chain", "per_transfer", "daily", "previous_per_transfer", "previous_daily", "revision", "not_before", "expires")) or
        expires <= now or expires - now > 600):
        raise ValueError("Policy differs from independent record or is expired")
    if kind == "initialize":
        if revision != 1 or old_per or old_daily or any(old_target) or not_before:
            raise ValueError("Invalid initial policy")
    elif not old_per or old_per > old_daily or not any(old_target) or not_before < expires + 86400:
        raise ValueError("Invalid prior policy or delay")
    print("Board", board.hex(), "generation", generation, "revision", revision)
    print("Chain", chain, "asset", "0x" + asset.hex())
    print("Previous destination", "0x" + old_target.hex(), "limits", old_per, "/", old_daily, "base units")
    print("Proposed destination", "0x" + target.hex(), "limits", per, "/", daily, "base units")
    print("Policy", kind, "earliest activation", not_before, "request expiry", expires)
    return statement


def finality_enrollment_body(statement, expected, now):
    """Compare the verifier key and all policy values to an independent profile."""
    required = {"board", "generation", "chain", "signer", "genesis", "asset", "destination", "per_transfer", "daily", "finality_key", "expires"}
    if len(statement)!=297 or statement[:4]!=b"N54G" or not isinstance(expected,dict) or set(expected)!=required:
        raise ValueError("Malformed independent finality enrollment")
    number=lambda offset:int.from_bytes(statement[offset:offset+8],"big")
    generation,chain,per,daily,expires=number(36),number(108),number(208),number(216),number(289)
    key=statement[224:289]
    if (statement[4:36]!=exact_hex(expected["board"],32) or generation!=expected["generation"] or
        chain!=8283 or chain!=expected["chain"] or statement[116:136]!=exact_hex(expected["signer"],20) or
        statement[136:168]!=exact_hex(expected["genesis"],32) or statement[168:188]!=exact_hex(expected["asset"],20) or
        statement[188:208]!=exact_hex(expected["destination"],20) or per!=expected["per_transfer"] or
        daily!=expected["daily"] or key!=exact_hex(expected["finality_key"],65) or expires!=expected["expires"] or
        any(type(expected[k]) is not int for k in ("generation","chain","per_transfer","daily","expires")) or
        not generation or not per or per>daily or expires<=now or expires-now>600 or key[0]!=4 or
        not all(any(statement[a:b]) for a,b in ((44,60),(60,76),(76,108),(116,136),(136,168),(168,188),(188,208),(225,289)))):
        raise ValueError("Finality enrollment differs from independent profile or is expired")
    print("Board",statement[4:36].hex(),"generation",generation,"chain",chain)
    print("Signer",statement[116:136].hex(),"genesis",statement[136:168].hex())
    print("Asset",statement[168:188].hex(),"destination",statement[188:208].hex(),"limits",per,"/",daily,"base units")
    print("Finality P-256 point SHA-256",hashlib.sha256(key).hexdigest(),"expiry",expires)
    return statement



def display_binding(binding, challenge, operations, now):
    if (not isinstance(binding, dict) or set(binding) != {"type", "operation", "session", "request", "expires"} or
        binding["type"] != "nu54-owner-display-v1" or type(binding["operation"]) is not int or
        binding["operation"] not in operations or len(challenge) != 32 or not any(challenge)):
        raise ValueError("Invalid private owner request or board challenge")
    expiry = binding["expires"]
    if not isinstance(expiry, str) or not expiry.isascii() or not expiry.isdecimal() or len(expiry) > 20:
        raise ValueError("Invalid request expiry")
    expiry = int(expiry)
    if not now < expiry <= now + 110:
        raise ValueError("Owner request expired or lifetime exceeds the approval window")
    session = exact_hex(binding["session"], 16)
    request = exact_hex(binding["request"], 16)
    return session + request + challenge, expiry


def owner_display_body(command, binding, challenge, expected, now):
    def number(key):
        value = expected.get(key)
        if type(value) is not int or not 0 <= value < 2**64:
            raise ValueError("Invalid independent " + key)
        return value.to_bytes(8, "big")

    if not isinstance(expected, dict):
        raise ValueError("Invalid independent approval record")
    if command == "finality-enroll":
        context, expires = display_binding(binding, challenge, (28,), now)
        grant_expiry = number("expires")
        if expires > int.from_bytes(grant_expiry, "big"):
            raise ValueError("Request outlives the independent issuer grant")
        body = (b"N54G" + exact_hex(expected["board"], 32) + number("generation") + context + number("chain") +
                exact_hex(expected["signer"], 20) + exact_hex(expected["genesis"], 32) +
                exact_hex(expected["asset"], 20) + exact_hex(expected["destination"], 20) +
                number("per_transfer") + number("daily") + exact_hex(expected["finality_key"], 65) + grant_expiry)
        return finality_enrollment_body(body, expected, now)
    if command != "policy":
        raise ValueError("Invalid display action")
    if isinstance(binding, dict) and binding.get("operation") in (33, 34):
        context, expires = display_binding(binding, challenge, (33, 34), now)
        fields = {"board", "generation", "revision", "chain", "asset", "previous_recipients",
                  "recipients", "previous_per_transfer", "previous_daily", "per_transfer", "daily"}
        if set(expected) != fields:
            raise ValueError("Invalid independent recipient policy record")

        def addresses(field):
            values = expected[field]
            if not isinstance(values, list) or len(values) > 8 or any(not isinstance(value, str) for value in values):
                raise ValueError("Expected up to eight recipient addresses")
            entries = [exact_hex(value, 20) for value in values]
            if bytes(20) in entries or entries != sorted(set(entries)):
                raise ValueError("Expected a canonical recipient set")
            return entries

        active, proposed = addresses("previous_recipients"), addresses("recipients")
        generation, revision, chain, previous_per, previous_daily, per, daily = (
            number(key) for key in ("generation", "revision", "chain", "previous_per_transfer", "previous_daily",
                                    "per_transfer", "daily")
        )
        caps = binding["operation"] == 34
        if (not all(map(any, (generation, revision, chain, previous_per, per))) or
            previous_per > previous_daily or per > daily or
            ((active != proposed or (per, daily) == (previous_per, previous_daily)) if caps else
             (active == proposed or (per, daily) != (previous_per, previous_daily)))):
            raise ValueError("Recipient or cap change differs from the requested operation")
        board, asset = exact_hex(expected["board"], 32), exact_hex(expected["asset"], 20)
        not_before = expires + 86400
        body = ((b"N54M" if caps else b"N54R") + board + context + generation + revision + chain + asset +
                previous_per + previous_daily + not_before.to_bytes(8, "big") + expires.to_bytes(8, "big"))
        for entries in (active, proposed):
            body += bytes([len(entries)]) + b"".join(entries) + bytes(20 * (8 - len(entries)))
        if caps:
            body += per + daily
        print("Board", board.hex(), "generation", expected["generation"], "revision", expected["revision"])
        print("Chain", expected["chain"], "asset", "0x" + asset.hex())
        print("Current recipients", *("0x" + item.hex() for item in active), sep="\n")
        print("Proposed recipients", *("0x" + item.hex() for item in proposed), sep="\n")
        print("Removed", *("0x" + item.hex() for item in active if item not in proposed), sep="\n")
        print("Added", *("0x" + item.hex() for item in proposed if item not in active), sep="\n")
        print("Current limits", expected["previous_per_transfer"], "/", expected["previous_daily"], "base units")
        print("Proposed limits", expected["per_transfer"], "/", expected["daily"], "base units")
        print("Earliest activation", not_before, "request expiry", expires)
        return body
    context, expires = display_binding(binding, challenge, (24, 26), now)
    initial = binding["operation"] == 26
    not_before = 0 if initial else expires + 86400
    expected = {"expires": expires, "not_before": not_before, **expected}
    old_destination = bytes.fromhex(expected["previous_destination"].removeprefix("0x"))
    if len(old_destination) != 20:
        raise ValueError("Invalid previous destination")
    body = ((b"N54I" if initial else b"N54P") + exact_hex(expected["board"], 32) + context +
            number("generation") + number("revision") + number("chain") + exact_hex(expected["asset"], 20) +
            old_destination + exact_hex(expected["destination"], 20) + number("previous_per_transfer") +
            number("previous_daily") + number("per_transfer") + number("daily") +
            not_before.to_bytes(8, "big") + expires.to_bytes(8, "big"))
    return policy_body(body, expected, now)


def recovery_body(challenge, device, replacement):
    if len(challenge) != 130 or challenge[:2] != b"\x01\x05" or challenge[2:34] != device or challenge[42:74] != replacement:
        raise ValueError("Challenge differs from independently verified board or phone")
    print("Replace the owner of board", device.hex())
    print("Current owner generation", int.from_bytes(challenge[34:42], "big"))
    print("New phone key fingerprint", replacement.hex())
    return b"N54R" + challenge


def parse_owner_display_code(raw):
    if not re.fullmatch(r"(?:24|26|28|33|34)/[0-9a-f]{32}/[0-9a-f]{32}/[1-9][0-9]{0,19}", raw):
        raise ValueError("Invalid owner request code")
    operation, session, request, expires = raw.split("/")
    return {"type": "nu54-owner-display-v1", "operation": int(operation), "session": session,
            "request": request, "expires": expires}


def local_binding():
    if not sys.stdin.isatty():
        raise ValueError("Scan or enter the current owner request directly at the registered display")
    with warnings.catch_warnings():
        warnings.simplefilter("error", getpass.GetPassWarning)
        raw = getpass.getpass("Scan the owner's one-time QR or enter the code (hidden): ")
    return parse_owner_display_code(raw)


async def approve(args):
    from bleak import BleakClient

    binding = await asyncio.to_thread(local_binding) if args.command in ("policy", "finality-enroll") else None
    async with BleakClient(args.address) as client:
        if args.command in ("policy", "finality-enroll"):
            challenge = bytes(await client.read_gatt_char(GATT + "5"))
            body = owner_display_body(args.command, binding,
                                      challenge, json.loads(args.expect.read_text()), int(time.time()))
        elif args.command == "pay":
            challenge = bytes(await client.read_gatt_char(GATT + "5"))
            from payment_presentation import payment_body, read_private_projection, verify_presentation

            quote = args.quote.read_bytes()
            if (args.cycle is None) != (args.terms_version is None):
                raise ValueError("Provide both independently known subscription terms and cycle")
            expected_invoice = None if args.cycle is None else {"cycle": args.cycle, "termsVersion": args.terms_version}
            if expected_invoice is not None and (args.merchant_signer is None or args.beneficiary is None):
                raise ValueError("The independently registered invoice issuer and beneficiary are required")
            beneficiary = exact_hex(args.beneficiary, 20).hex() if args.beneficiary else None
            projection = read_private_projection(args.projection)
            invoice = verify_presentation(quote, projection, args.presenter_key.read_text().strip(),
                                          args.branch, args.customer, sale=args.sale, origin=args.origin,
                                          invoice=expected_invoice, beneficiary=beneficiary)
            merchant_signer = exact_hex(args.merchant_signer, 20) if args.merchant_signer else None
            body = payment_body(challenge, quote, merchant_signer)
            if invoice is not None:
                print("Invoice", args.origin, "cycle", invoice["cycle"], "terms", json.dumps(invoice["termsVersion"]))
            print("Quote", quote[64:96].hex(), "digest", projection["quoteDigest"])
        elif args.command == "evm":
            from evm_transfer import CHAINS, allowance_context, external_context, parse, parse_siwe_capsule, rpc_client

            if args.wallet_service:
                if not args.display_token_file or args.transfer:
                    raise ValueError("Use an independent display credential with the wallet service")
                token = args.display_token_file.read_text().strip()
                if len(token) != 64:
                    raise ValueError("Invalid independent display credential")
                wire = bytes.fromhex(rpc_client(args.wallet_service)("nu54_walletDisplay", [{"token": token}]))
            elif args.transfer and not args.display_token_file:
                wire = bytes.fromhex(args.transfer.read_text().strip())
            else:
                raise ValueError("Expected the exact signed request file or the independent wallet service")
            board = exact_hex(args.device, 32)
            sender = exact_hex(args.sender, 20)
            authority = public_file(args.authority)
            if wire and wire[0] == 21:
                from market_grant import display
                if not args.market_profile:
                    raise ValueError("Independent market deployment profile is required")
                profile = json.loads(args.market_profile.read_text())
                terms, fee, expiry, allowance = display(wire, profile, authority, board, sender, args.generation, int(time.time()))
                action, first, second, min_first, min_second, price_version = terms
                print("StableNet isolated test market 8283", action)
                print("Input A", first, "input B", second, "min A", min_first, "min B", min_second, "(atomic units)")
                print("Oracle version", price_version, "maximum fee", fee, "deadline", expiry)
                if allowance is not None:
                    print("Exact token allowance", allowance, "spender", "0x" + wire[81:101].hex())
                domain = b"N54Z"
            elif wire and wire[0] == 15:
                chain, host, nonce, genesis = parse_siwe_capsule(
                    wire, authority, board, sender, args.generation, int(time.time()))
                if not args.origin or args.origin != "https://" + host or genesis != exact_hex(args.genesis, 32):
                    raise ValueError("Login origin or network differs from the independent enrollment")
                print(CHAINS[chain], "sign-in only")
                print("Site", "https://" + host)
                print("Account", "0x" + sender.hex())
                print("Login nonce", nonce)
                domain = b"N54I"
            else:
                chain, nonce, token, amount, recipient, fee, base, genesis = parse(
                    wire, authority, board, sender, args.generation, int(time.time()))
                if genesis != exact_hex(args.genesis, 32):
                    raise ValueError("Transfer genesis differs from the independent network profile")
                if wire[0] == 20:
                    _, (origin, pairing, session, peer) = external_context(wire)
                    if args.origin != origin:
                        raise ValueError("External DApp differs from the independent origin enrollment")
                    print("WalletConnect site", origin)
                    print("Pairing", pairing.hex(), "Session", session.hex(), "Peer key", peer.hex())
                if wire[0] == 36:
                    _, (_, maximum, decimals), (origin, first, second, peer) = allowance_context(wire)
                    if not args.dapp_allowance_profile or args.origin != origin:
                        raise ValueError("Independent DApp allowance enrollment is required")
                    expected = json.loads(args.dapp_allowance_profile.read_text())
                    expected_policy = {"chain":str(chain), "token":"0x"+token.hex(),
                                       "spender":"0x"+recipient.hex(), "maximum":str(maximum),
                                       "decimals":decimals, "feeLimit":str(int.from_bytes(wire[17:25], "big"))}
                    if expected != expected_policy:
                        raise ValueError("Allowance differs from the independent enrollment")
                    print("WalletConnect site" if wire[165] else "Browser site", origin)
                    if wire[165]:
                        print("Pairing", first.hex(), "Session", second.hex(), "Peer key", peer.hex())
                    else:
                        print("Browser session", first.hex())
                    whole, fraction = divmod(amount, 10**decimals)
                    reviewed = f"{whole}.{fraction:0{decimals}d}" if decimals else str(whole)
                    print(CHAINS[chain], "grant token allowance")
                    print("Requested allowance", reviewed, f"({amount} base units)")
                    print("Commissioned allowance cap", maximum, "base units")
                    print("Request expires at Unix time", int.from_bytes(wire[1:9], "big"))
                else:
                    print(CHAINS[chain], "revoke token allowance" if wire[0] == 14 else "direct transfer")
                    print("New allowance" if wire[0] == 14 else "Amount in base units", amount)
                print("Genesis", "0x" + genesis.hex())
                print("Sender", "0x" + sender.hex(), "nonce", nonce)
                print("Spender" if wire[0] in (14, 36) else "Recipient", "0x" + recipient.hex())
                print("Asset", "0x" + token.hex() if any(token) else "native")
                print("Maximum fee in wei", fee, "including Base data and operator estimate", base)
                domain = b"N54X"
            challenge = bytes(await client.read_gatt_char(GATT + "5"))
            if len(challenge) != 32:
                raise ValueError("Missing board challenge")
            body = domain + challenge + wire
        else:
            body = recovery_body(bytes.fromhex(args.challenge), exact_hex(args.device, 32), exact_hex(args.replacement, 32))
        await asyncio.to_thread(require_confirmation)
        if binding is not None and int(time.time()) >= int(binding["expires"]):
            raise ValueError("Owner request expired while awaiting confirmation")
        signature = signed(args.key, body)
        for offset in range(0, len(signature), 19):
            await client.write_gatt_char(GATT + "5", bytes([offset]) + signature[offset:offset + 19], response=True)


def main():
    cli = argparse.ArgumentParser(description=__doc__)
    sub = cli.add_subparsers(dest="command", required=True)
    for name in ("pay", "replace-phone", "evm", "policy", "finality-enroll"):
        command = sub.add_parser(name)
        command.add_argument("address")
        command.add_argument("key", type=Path)
        if name in ("policy", "finality-enroll"):
            command.add_argument("--expect", type=Path, required=True,
                                 help="policy values verified independently of the phone")
        elif name == "pay":
            command.add_argument("quote", type=Path)
            command.add_argument("--projection", type=Path, required=True)
            command.add_argument("--presenter-key", type=Path, required=True)
            command.add_argument("--branch", required=True)
            command.add_argument("--customer", required=True)
            expected = command.add_mutually_exclusive_group(required=True)
            expected.add_argument("--sale")
            expected.add_argument("--origin")
            command.add_argument("--cycle", help="Independently known subscription cycle (YYYY-MM-DD)")
            command.add_argument("--terms-version", help="Independently known original terms version")
            command.add_argument("--merchant-signer", help="Independently registered issuer address")
            command.add_argument("--beneficiary", help="Independently registered recipient address")
        elif name == "evm":
            command.add_argument("transfer", type=Path, nargs="?")
            command.add_argument("--wallet-service")
            command.add_argument("--display-token-file", type=Path)
            command.add_argument("--market-profile", type=Path)
            command.add_argument("--dapp-allowance-profile", type=Path)
            command.add_argument("--authority", type=Path, required=True)
            command.add_argument("--device", required=True)
            command.add_argument("--sender", required=True)
            command.add_argument("--generation", type=int, required=True)
            command.add_argument("--genesis", required=True)
            command.add_argument("--origin", help="Independently enrolled HTTPS sign-in origin")
        else:
            command.add_argument("challenge")
            command.add_argument("--device", required=True)
            command.add_argument("--replacement", required=True)
    asyncio.run(approve(cli.parse_args()))


if __name__ == "__main__":
    main()
