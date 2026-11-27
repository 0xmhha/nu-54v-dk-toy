"""Verify the app's public watch export using a disconnected Bitcoin Core node."""
import json
from pathlib import Path
import sys

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parents[3] / "scripts"))
from bitcoin_core_workspace import bitcoin_core_workspace

tools = Path(sys.argv[1]).resolve()
export = json.load(sys.stdin)
chain = "signet" if "--signet" in sys.argv else "regtest"
with bitcoin_core_workspace(tools, "nu54-watch-export.", chain) as core:
    def rpc(*args, wallet=None):
        target = ["-rpcwallet=" + wallet] if wallet else []
        result = core.cli(*target, *args)
        return result.strip() if args[0] in ("getnewaddress", "getrawchangeaddress") else json.loads(result)

    core.start()
    wallet = "descriptor-only"
    rpc("createwallet", wallet, "true", "true")
    records = []
    for name, first, internal in [("receive", "firstReceive", False), ("change", "firstChange", True)]:
        descriptor = export[name]
        proof = rpc("getdescriptorinfo", descriptor)
        if proof["checksum"] != descriptor.split("#")[1] or proof["hasprivatekeys"]:
            raise ValueError("Unexpected descriptor checksum or key type")
        if rpc("deriveaddresses", descriptor, "[0,0]") != [export[first]]:
            raise ValueError("Derived address differs")
        records.append(dict(desc=descriptor, timestamp="now", range=[0, 2], active=True, internal=internal))
    if not all(row["success"] for row in rpc("importdescriptors", json.dumps(records), wallet=wallet)):
        raise ValueError("Bitcoin Core rejected the watch descriptors")
    assert not rpc("getwalletinfo", wallet=wallet)["private_keys_enabled"]
    core.stop()
    core.start()
    rpc("loadwallet", wallet)
    assert not rpc("getwalletinfo", wallet=wallet)["private_keys_enabled"]
    restored = rpc("listdescriptors", wallet=wallet)["descriptors"]
    for candidate, internal in [("receive", False), ("change", True)]:
        assert any(row["desc"] == export[candidate] and row["internal"] == internal and row["active"]
                   for row in restored)
    assert rpc("getnewaddress", "", "bech32", wallet=wallet) == export["firstReceive"]
    assert rpc("getrawchangeaddress", "bech32", wallet=wallet) == export["firstChange"]
    print("PASS Bitcoin Core checksums, receive/change, watch-only import and restart")
