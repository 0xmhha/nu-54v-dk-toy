"""Exercise exact allowance scope and legacy hash recovery on mobile-fixture's disposable chain."""

import json
import os
from pathlib import Path
import resource
import time
import subprocess
import sys
from urllib.error import HTTPError
from urllib.request import Request, urlopen


def request(origin, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    try:
        with urlopen(Request(origin + path, data=data, headers={"Content-Type": "application/json"}), timeout=10) as response:
            return response.status, json.load(response)
    except HTTPError as error:
        return error.code, json.load(error)


def rpc(method, *params):
    status, reply = request("http://127.0.0.1:8285", "", {"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
    assert status == 200 and "result" in reply, f"fixture RPC failed: {method}"
    return reply["result"]


def market(path, body=None):
    return request("http://127.0.0.1:8284", "/v1/" + path, body)


def words(*values):
    return "".join(f"{value:064x}" for value in values)


def main():
    resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
    profile = json.loads(Path(os.environ["P08_PROFILE"]).read_text())
    assert rpc("eth_chainId") == hex(8283) and "anvil" in rpc("web3_clientVersion").lower()
    assert rpc("eth_getBlockByHash", profile["deploymentHash"], False)["hash"] == profile["deploymentHash"]
    accounts = rpc("eth_accounts")
    venue, asset = profile["venue"]["address"], profile["quote"]["address"]
    mobile_signer, board_signer = os.environ["P08_TEST_MOBILE_SIGNER"], os.environ["P08_TEST_BOARD_SIGNER"]
    user_key = f"{106:064x}"  # Matches the checked-in disposable mobile signer adapter.
    user = subprocess.check_output(["cast", "wallet", "address", "--private-key", user_key], text=True).strip().lower()
    rpc("anvil_setBalance", user, "0x3635c9adc5dea00000")
    block = rpc("eth_getBlockByNumber", "latest", False)
    for operator in accounts[2:4]:
        rpc("eth_sendTransaction", {"from": operator, "to": venue,
                                   "data": "0x07640c1f" + words(1_200_000, int(block["timestamp"], 16))})
    rpc("anvil_mine", "0x3")

    terms = {"user": user, "action": "open-short", "a": "12000000", "b": "100000000", "minA": "0", "minB": "0"}
    status, first = market("quotes", terms)
    assert status == 200
    status, substitute = market("quotes", terms)
    assert status == 200 and substitute["digest"] != first["digest"]
    nonce = rpc("eth_getTransactionCount", user, "pending")
    approval = "0x095ea7b3" + venue[2:].rjust(64, "0") + words(int(terms["a"]))

    domain = "".join(profile[key]["address"][2:] for key in ("venue", "base", "quote"))
    domain += profile["quoteSigner"][2:] + profile["venue"]["codeHash"][2:]
    gas_price = int(rpc("eth_gasPrice"), 16)
    now = int(rpc("eth_getBlockByNumber", "latest", False)["timestamp"], 16)

    def mobile(data):
        return subprocess.run([mobile_signer, domain, asset[2:], str(int(nonce, 16)), str(gas_price),
                               data[2:], first["data"][2:], str(now)],
                              capture_output=True, text=True, check=False)

    def board(data, mode=2):
        board_id = bytes([8]) + bytes(30) + bytes([9])
        user_bytes = bytes.fromhex(user[2:])
        wire = (b"\x15" + first["deadline"].to_bytes(8, "big") + (1).to_bytes(8, "big") +
                bytes.fromhex(profile["genesis"][2:] + profile["deploymentHash"][2:] + domain + asset[2:]) +
                int(nonce, 16).to_bytes(8, "big") + gas_price.to_bytes(8, "big") +
                bytes([mode]) + bytes.fromhex(first["data"][2:] + (data[2:] if mode else "")))
        proof = bytes([8]) * 64
        statement = b"N54J" + board_id + (1).to_bytes(8, "big") + user_bytes + wire
        return subprocess.run([board_signer, (wire + proof).hex(), statement.hex(), proof.hex(),
                               board_id.hex(), user_bytes.hex(), user_key, str(now), "1"],
                              capture_output=True, text=True, check=False)

    def allowance():
        data = "0xdd62ed3e" + user[2:].rjust(64, "0") + venue[2:].rjust(64, "0")
        return int(rpc("eth_call", {"to": asset, "data": data}, "latest"), 16)

    approved_data = approval + first["digest"][2:]
    wide_mobile, wide_board = mobile(approved_data), board(approved_data)
    assert wide_mobile.returncode == 0 and wide_board.returncode == 0, "bound host signature failed"
    bound = wide_mobile.stdout.strip()
    assert bound == "0x" + wide_board.stdout.strip(), "board and handset signed different bytes"
    legacy_sign = mobile(approval)
    legacy_board = board(approval, mode=1)
    assert legacy_sign.returncode == 0 and legacy_board.returncode == 0
    legacy = legacy_sign.stdout.strip()
    assert legacy == "0x" + legacy_board.stdout.strip()
    assert mobile(approval + substitute["digest"][2:]).returncode != 0
    assert board(approval + substitute["digest"][2:]).returncode != 0
    assert market("transactions", {"raw": bound, "offer": substitute["data"]})[0] == 400
    assert market("transactions", {"raw": bound})[0] == 400
    assert market("transactions", {"raw": legacy})[0] == 400
    assert market("transactions", {"raw": legacy, "offer": first["data"]})[0] == 400
    assert allowance() == 0 and rpc("eth_getTransactionCount", user, "pending") == nonce

    saved = rpc("evm_snapshot")
    block = rpc("eth_getBlockByNumber", "latest", False)
    rpc("evm_setNextBlockTimestamp", int(block["timestamp"], 16) + 121)
    rpc("evm_mine")
    assert market("profile")[1]["state"] == "price-unavailable"
    assert market("transactions", {"raw": bound, "offer": first["data"]})[0] == 400
    assert allowance() == 0 and rpc("eth_getTransactionCount", user, "pending") == nonce
    assert rpc("evm_revert", saved)

    status, accepted = market("transactions", {"raw": bound, "offer": first["data"]})
    assert status == 202 and accepted["state"] == "submitted", (status, accepted.get("state"))
    rpc("anvil_mine", "0x5")
    assert allowance() == 12_000_000
    assert rpc("eth_getTransactionReceipt", accepted["txHash"])["status"] == "0x1"
    # Reconstruct a historical, already node-known approval on a different
    # disposable actor. The updated relay must only read back its exact hash.
    historical = accounts[6]
    historical_raw = rpc("eth_signTransaction", {
        "from": historical, "to": asset, "nonce": rpc("eth_getTransactionCount", historical, "pending"),
        "gas": hex(65_000), "gasPrice": rpc("eth_gasPrice"), "value": "0x0", "data": approval,
    })
    historical_raw = historical_raw["raw"] if isinstance(historical_raw, dict) else historical_raw
    historical_hash = rpc("eth_sendRawTransaction", historical_raw)
    rpc("anvil_mine", "0x5")
    assert rpc("eth_getTransactionReceipt", historical_hash)["status"] == "0x1"
    sent_nonce = rpc("eth_getTransactionCount", user, "pending")
    assert int(sent_nonce, 16) == int(nonce, 16) + 1
    settled = rpc("evm_snapshot")
    block = rpc("eth_getBlockByNumber", "latest", False)
    rpc("evm_setNextBlockTimestamp", int(block["timestamp"], 16) + 121)
    rpc("evm_mine")
    status, replay = market("transactions", {"raw": bound})
    assert status == 202 and replay == accepted
    status, recovered = market("transactions", {"raw": historical_raw})
    assert status == 202 and recovered == {"state": "submitted", "txHash": historical_hash}
    assert market("approvals/" + historical_hash + "/" + historical)[1]["state"] == "confirmed"
    query = "approvals/" + accepted["txHash"] + "/" + user
    for _ in range(6):
        status, observed = market(query)
        if status == 200 and observed["state"] == "confirmed":
            break
        time.sleep(1)
    assert status == 200 and observed["state"] == "confirmed" and observed["txHash"] == accepted["txHash"]
    assert allowance() == 12_000_000 and rpc("eth_getTransactionCount", user, "pending") == sent_nonce
    assert rpc("evm_revert", settled)

    base = profile["base"]["address"]
    quote_asset = profile["quote"]["address"]
    for token, minted in ((base, 15_000_000_000), (quote_asset, 150_000_000)):
        rpc("eth_sendTransaction", {"from": accounts[0], "to": token,
                                   "data": "0x40c10f19" + user[2:].rjust(64, "0") + words(minted)})
    block = rpc("eth_getBlockByNumber", "latest", False)
    for operator in accounts[2:4]:
        rpc("eth_sendTransaction", {"from": operator, "to": venue,
                                   "data": "0x07640c1f" + words(1_200_000, int(block["timestamp"], 16))})
    rpc("anvil_mine", "0x3")
    for action, a, b, min_a, approvals in (
        ("lp-add", "10000000000", "120000000", "10000000000", [(base, 10_000_000_000), (quote_asset, 120_000_000)]),
        ("swap-base", "100000000", "0", "1000000", [(base, 100_000_000)]),
    ):
        status, first = market("quotes", {**terms, "action": action, "a": a, "b": b, "minA": min_a})
        assert status == 200, action
        for asset, amount in approvals + [(venue, 0)]:
            nonce = rpc("eth_getTransactionCount", user, "pending")
            now = int(rpc("eth_getBlockByNumber", "latest", False)["timestamp"], 16)
            quoted_call = first["data"] if asset == venue else (
                "0x095ea7b3" + venue[2:].rjust(64, "0") + words(amount) + first["digest"][2:]
            )
            signed_mobile = mobile(quoted_call)
            signed_board = board(quoted_call, mode=0 if asset == venue else 2)
            # Execution mode carries the offer once, without an allowance tail.
            assert signed_mobile.returncode == 0 and signed_board.returncode == 0, action
            raw_bytes = signed_mobile.stdout.strip()
            assert raw_bytes == "0x" + signed_board.stdout.strip(), action
            status, accepted_order = market("transactions", {"raw": raw_bytes, "offer": first["data"]})
            assert status == 202 and accepted_order["state"] == "submitted", action
            rpc("evm_mine")
            assert rpc("eth_getTransactionReceipt", accepted_order["txHash"])["status"] == "0x1", action
            if amount:
                assert allowance() == amount, action
    print("L05_RELAY_EXISTING swap=confirmed lp_add=confirmed bound_approvals=3")
    print("L05_RELAY_BOUND allowance=12000000 invalid_context=held stale_first=held known_bound_and_legacy_hashes=confirmed board=identical handset=identical nonce_delta=1")


def run_fixture():
    root = Path(__file__).resolve().parents[3]
    sys.path.insert(0, str(root / "scripts"))
    from integration_workspace import run_command, run_environment, scoped_workspace

    with scoped_workspace("nu54-market-relay-") as work:
        environment = run_environment(work, os.environ)
        firmware = root / "products/p01-device-firmware/test"
        mobile = root / "products/p02-user-app/native/market"
        build = work / "board"
        signer = work / "market-sign"
        run_command(["cmake", "-S", firmware, "-B", build, "-DCMAKE_BUILD_TYPE=Debug"], env=environment, check=True)
        run_command(["cmake", "--build", build, "--target", "test_device_evm"], env=environment, check=True)
        run_command(["cc", "-O2", "-std=c11", "-o", signer,
                     *(mobile / name for name in ("wallet.c", "crypto.c", "test_sign.c"))],
                    env=environment, check=True)
        environment.update(P08_TEST_MOBILE_SIGNER=str(signer), P08_TEST_BOARD_SIGNER=str(build / "test_device_evm"))
        run_command(["bash", Path(__file__).with_name("mobile-fixture.sh"), work / "chain",
                     sys.executable, "-B", Path(__file__).resolve(), "--live"], env=environment, check=True)


if __name__ == "__main__":
    if sys.argv[1:] == ["--live"]:
        main()
    elif not sys.argv[1:]:
        run_fixture()
    else:
        raise SystemExit("unexpected relay fixture arguments")
