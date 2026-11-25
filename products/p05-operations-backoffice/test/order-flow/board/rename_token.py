#!/usr/bin/env python3
"""Copy one firmware source for another token. The firmware itself is never changed.

Usage: rename_token.py SOURCE COPY NAME [UNIT]. With a unit, the copy signs that many base units for each unit it counts.
"""

import sys
from pathlib import Path

NAME = '"NU54 Test USD"'
# What changes for a token whose amounts do not fit the 8 bytes a board counts in. See token_unit.h.
UNIT = {
    "nu54_order.c": [],
    "nu54_spending.c": [("word64(fields + 96, amount);", "nu54_token_word(fields + 96, amount);")],
    "nu54_pilot.c": [("memcmp(quote + NU54_V2_AMOUNT_OFFSET, zero, sizeof(zero)) ||\n"
                      "	    word_u64(quote + NU54_V2_AMOUNT_OFFSET) > allowlist->max_amount ||\n",
                      "!nu54_token_units(quote + NU54_V2_AMOUNT_OFFSET, &units) || units > allowlist->max_amount ||\n"),
                     ("	uint64_t cap = allowlist->commercial_testnet", "	uint64_t units;\n	uint64_t cap = allowlist->commercial_testnet")],
    "device_pilot.c": [("	order_amount = sys_get_be64(quote + NU54_V2_AMOUNT_OFFSET + 24);\n",
                        "	if (!nu54_token_units(quote + NU54_V2_AMOUNT_OFFSET, &order_amount)) return -1;\n")],
}
# The firmware's own payment test counts in base units at one place.
UNIT["test_order.c"] = [("small[NU54_V2_AMOUNT_OFFSET + 31] = 100;", "nu54_token_word(small + NU54_V2_AMOUNT_OFFSET, 100);")]
NAMED = {"nu54_order.c", "nu54_spending.c"}


def once(text, old, new, source):
    if text.count(old) != 1:
        raise SystemExit(f"{source}: expected {old!r} once, found {text.count(old)}")
    return text.replace(old, new)


def copy(text, name, source, unit=""):
    if not name or any(not 0x20 <= ord(c) <= 0x7E or c in '"\\' for c in name):
        raise SystemExit(f"not a token name: {name!r}")
    file = Path(source).name
    if file in NAMED:
        text = once(text, NAME, f'"{name}"', source)
    if unit:
        for old, new in UNIT[file]:
            text = once(text, old, new, source)
        text = '#include "token_unit.h"\n' + text
    return text


if __name__ == "__main__":
    source, target, name, *unit = sys.argv[1:]
    Path(target).write_text(copy(Path(source).read_text(), name, source, *unit))
