"""Standalone wheel consumer. Runs in a venv with the installed quote-v2 extra."""

import importlib.metadata
import importlib.resources
import json

from nu54_protocol import _generated
from nu54_protocol.quote_v2 import QuotePolicy, quote_digest, verify_quote

assert "PaymentAuthorization" in _generated.EIP712_TYPES
root = importlib.resources.files("nu54_protocol") / "spec_v2"
assert json.loads((root / "schema.json").read_text())["sdkVersion"] == "0.2.0"
vectors = [
    line.split("|") for line in (root / "testdata/quote.vectors").read_text().splitlines()
    if line and not line.startswith("#")
]
assert len(vectors) >= 31
for name, signed, expected, unsigned, signer, clock, consumed, digest, status in vectors:
    if digest:
        assert quote_digest(bytes.fromhex(unsigned)).hex() == digest, name
    else:
        try:
            quote_digest(bytes.fromhex(unsigned))
        except ValueError:
            pass
        else:
            raise AssertionError(name)
    policy = QuotePolicy(bytes.fromhex(expected), bytes.fromhex(signer), int(clock), consumed == "true")
    assert verify_quote(bytes.fromhex(signed), policy) == int(status), name
actual = {
    distribution.metadata["Name"].lower(): distribution.version
    for distribution in importlib.metadata.distributions()
}
assert actual["nu54-protocol"] == "0.2.0"
assert {"coincurve", "pycryptodome"} <= actual.keys()
print(json.dumps({"quoteVectors": len(vectors), "installed": actual}, sort_keys=True))
