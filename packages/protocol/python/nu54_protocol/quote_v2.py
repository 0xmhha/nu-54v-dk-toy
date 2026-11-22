"""Canonical v2 merchant quote verification using the optional quote-v2 extra."""
from dataclasses import dataclass
from collections.abc import Sequence

from coincurve import PublicKey
from Crypto.Hash import keccak

from . import _quote_v2_layout as layout

_CURVE_ORDER = 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141


@dataclass(frozen=True)
class QuotePolicy:
    """The independently authenticated order bytes and merchant signer."""

    expected: bytes
    merchant_signer: bytes
    now: int
    consumed: bool


def _keccak(data: bytes) -> bytes:
    return keccak.new(digest_bits=256, data=data).digest()


_DOMAIN_TYPE = _keccak(layout.DOMAIN_TYPE.encode("ascii"))
_DOMAIN_NAME = _keccak(layout.DOMAIN_NAME.encode("ascii"))
_DOMAIN_VERSION = _keccak(layout.DOMAIN_VERSION.encode("ascii"))
_QUOTE_TYPE = _keccak(layout.QUOTE_TYPE.encode("ascii"))


def _u(raw: bytes, offset: int) -> int:
    return int.from_bytes(raw[offset:offset + 32], "big")


def _valid(raw: bytes) -> bool:
    if len(raw) != layout.UNSIGNED_SIZE:
        return False
    for index, width in enumerate(layout.WIDTHS):
        field = raw[index * 32:(index + 1) * 32]
        pad = 32 - width
        if any(field[:pad]) or not any(field[pad:]):
            return False
    mode = raw[layout.MODE_OFFSET + 31]
    role = raw[layout.PAYER_ROLE_OFFSET + 31]
    return (
        mode in (layout.ESCROW_MODE, layout.DIRECT_MODE)
        and role in (layout.OWNER_ROLE, layout.RENTER_ROLE)
        and (mode != layout.DIRECT_MODE or role == layout.OWNER_ROLE)
        and _u(raw, layout.EXPIRES_AT_OFFSET) > _u(raw, layout.ISSUED_AT_OFFSET)
    )


def encode_quote(fields: Sequence[bytes]) -> bytes:
    """Encode the ordered fixed-width v2 fields without padding in the inputs."""
    if len(fields) != layout.FIELD_COUNT or any(len(f) != width for f, width in zip(fields, layout.WIDTHS)):
        raise ValueError("malformed v2 quote")
    raw = b"".join(field.rjust(32, b"\x00") for field in fields)
    if not _valid(raw):
        raise ValueError("malformed v2 quote")
    return raw


def quote_digest(raw: bytes) -> bytes:
    """Hash the canonical unsigned bytes under the EIP-712 v2 domain."""
    if not _valid(raw):
        raise ValueError("malformed v2 quote")
    domain = _keccak(_DOMAIN_TYPE + _DOMAIN_NAME + _DOMAIN_VERSION + raw[:layout.QUOTE_ID_OFFSET])
    quote = _keccak(_QUOTE_TYPE + raw[layout.QUOTE_ID_OFFSET:])
    return _keccak(b"\x19\x01" + domain + quote)


def verify_quote(signed: bytes, policy: QuotePolicy) -> int:
    """Validate the received signature and the independently sourced binding."""
    if (
        type(signed) is not bytes
        or len(signed) != layout.UNSIGNED_SIZE + layout.SIGNATURE_SIZE
        or type(policy) is not QuotePolicy
        or type(policy.expected) is not bytes
        or not _valid(policy.expected)
        or type(policy.merchant_signer) is not bytes
        or len(policy.merchant_signer) != 20
        or not any(policy.merchant_signer)
        or type(policy.now) is not int
        or not 0 <= policy.now < 1 << 64
        or type(policy.consumed) is not bool
    ):
        return layout.STATUS_MALFORMED
    raw = signed[:layout.UNSIGNED_SIZE]
    if not _valid(raw):
        return layout.STATUS_MALFORMED
    if _u(raw, layout.CHAIN_ID_OFFSET) != layout.PILOT_CHAIN or raw[layout.MODE_OFFSET + 31] != layout.ESCROW_MODE:
        return layout.STATUS_UNSUPPORTED_PROFILE
    if raw != policy.expected:
        return layout.STATUS_WRONG_BINDING
    if not _u(raw, layout.ISSUED_AT_OFFSET) <= policy.now < _u(raw, layout.EXPIRES_AT_OFFSET):
        return layout.STATUS_EXPIRED
    signature = signed[layout.UNSIGNED_SIZE:]
    r = int.from_bytes(signature[:32], "big")
    s = int.from_bytes(signature[32:64], "big")
    v = signature[64]
    if not (0 < r < _CURVE_ORDER and 0 < s <= _CURVE_ORDER // 2 and v in (27, 28)):
        return layout.STATUS_UNTRUSTED_SIGNER
    try:
        compact = signature[:64] + bytes([v - 27])
        public = PublicKey.from_signature_and_message(compact, quote_digest(raw), hasher=None)
        recovered = _keccak(public.format(compressed=False)[1:])[-20:]
    except ValueError:
        return layout.STATUS_UNTRUSTED_SIGNER
    if recovered != policy.merchant_signer:
        return layout.STATUS_UNTRUSTED_SIGNER
    return layout.STATUS_CONSUMED if policy.consumed else layout.STATUS_OK
