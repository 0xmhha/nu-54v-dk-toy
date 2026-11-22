"""Check the identity and patched dependency versions of the isolated QA peer."""
import json

VERSION = "2.21.8"
TARBALL = "https://registry.npmjs.org/@walletconnect/sign-client/-/sign-client-2.21.8.tgz"
INTEGRITY = "sha512-lTcUbMjQ0YUZ5wzCLhpBeS9OkWYgLLly6BddEp2+pm4QxiwCCU2Nao0nBJXgzKbZYQOgrEGqtdm/7ze67gjzRA=="
EXPECTED = {
    "@walletconnect/core": "2.21.8",
    "@walletconnect/relay-auth": "1.1.0",
    "@walletconnect/sign-client": VERSION,
    "@walletconnect/types": "2.21.8",
    "@walletconnect/utils": "2.21.8",
    "decode-uri-component": "0.5.1",
    "query-string": "7.1.3",
    "ws": "8.21.0",
    "@walletconnect/jsonrpc-ws-connection/node_modules/ws": "7.5.14",
}


def validate_lock(path):
    lock = json.loads(path.read_text())["packages"]
    peer = lock.get("node_modules/@walletconnect/sign-client", {})
    if (peer.get("version"), peer.get("resolved"), peer.get("integrity"), peer.get("license")) != (
        VERSION, TARBALL, INTEGRITY, "Apache-2.0"
    ):
        raise RuntimeError("Reference package pin mismatch")
    for name, item in lock.items():
        if not name:
            continue
        if not item.get("resolved", "").startswith("https://registry.npmjs.org/") or not item.get(
            "integrity", ""
        ).startswith("sha512-") or item.get("hasInstallScript", False) or item.get("link", False):
            raise RuntimeError("Unpinned or executable reference dependency")
        package = name.removeprefix("node_modules/")
        leaf = package.rsplit("/node_modules/", 1)[-1]
        if leaf in {"elliptic", "@stablelib/ed25519"}:
            raise RuntimeError("Removed vulnerable dependency restored")
        if leaf in EXPECTED and EXPECTED.get(package) != item.get("version"):
            raise RuntimeError("Unexpected version or duplicate of patched dependency")
    if any(lock.get("node_modules/" + name, {}).get("version") != version for name, version in EXPECTED.items()):
        raise RuntimeError("Patched reference dependency pin mismatch")
