#!/usr/bin/env python3
"""Copy one firmware source with another token name. The firmware itself is never changed."""

import sys
from pathlib import Path

NAME = '"NU54 Test USD"'


def renamed(text, name, source):
    if not name or any(not 0x20 <= ord(c) <= 0x7E or c in '"\\' for c in name):
        raise SystemExit(f"not a token name: {name!r}")
    if text.count(NAME) != 1:
        raise SystemExit(f"{source}: expected {NAME} once, found {text.count(NAME)}")
    return text.replace(NAME, f'"{name}"')


if __name__ == "__main__":
    source, target, name = sys.argv[1:]
    Path(target).write_text(renamed(Path(source).read_text(), name, source))
