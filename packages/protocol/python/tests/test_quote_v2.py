from pathlib import Path

import pytest

from nu54_protocol._quote_v2_layout import WIDTHS
from nu54_protocol.quote_v2 import QuotePolicy, encode_quote, quote_digest, verify_quote

FIXTURE = Path(__file__).resolve().parents[2] / "v2/testdata/quote.vectors"
VECTORS = [
    line.split("|") for line in FIXTURE.read_text().splitlines()
    if line and not line.startswith("#")
]


@pytest.mark.parametrize(
    "raw,expected,unsigned,signer,clock,consumed,digest,status",
    [vector[1:] for vector in VECTORS],
    ids=[vector[0] for vector in VECTORS],
)
def test_merchant_quote(raw, expected, unsigned, signer, clock, consumed, digest, status):
    if digest:
        payload = bytes.fromhex(unsigned)
        fields = [payload[(index + 1) * 32 - width:(index + 1) * 32] for index, width in enumerate(WIDTHS)]
        assert encode_quote(fields) == payload
        assert quote_digest(payload).hex() == digest
    else:
        with pytest.raises(ValueError, match="malformed"):
            quote_digest(bytes.fromhex(unsigned))
    policy = QuotePolicy(bytes.fromhex(expected), bytes.fromhex(signer), int(clock), consumed == "true")
    assert verify_quote(bytes.fromhex(raw), policy) == int(status)


def test_malformed_runtime_trust_policy_is_rejected():
    from dataclasses import replace

    from nu54_protocol._quote_v2_layout import AMOUNT_OFFSET, STATUS_MALFORMED

    _, raw, expected, _, signer, clock, *_ = VECTORS[0]
    signed = bytes.fromhex(raw)
    valid = QuotePolicy(bytes.fromhex(expected), bytes.fromhex(signer), int(clock), False)
    zero_amount = valid.expected[:AMOUNT_OFFSET] + bytes(32) + valid.expected[AMOUNT_OFFSET + 32:]
    policies = (
        None, {}, replace(valid, now=None), replace(valid, now=float("nan")),
        replace(valid, now=float(clock)), replace(valid, now=True), replace(valid, now=clock),
        replace(valid, now=-1), replace(valid, now=1 << 64), replace(valid, consumed=None),
        replace(valid, consumed=0), replace(valid, consumed="false"), replace(valid, expected=zero_amount),
        replace(valid, expected=b""), replace(valid, merchant_signer=bytes(20)),
    )
    for policy in policies:
        assert verify_quote(signed, policy) == STATUS_MALFORMED
