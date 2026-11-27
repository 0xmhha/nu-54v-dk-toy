"""Copy named functions and one-line macros or arrays out of an app source, so a host test compiles the exact text the board builds.

Usage: extract_c.py OUTPUT SOURCE NAME... [SOURCE NAME...]
"""

from pathlib import Path
import re
import sys


def definition(source, name):
    line = re.findall(rf"^(?:#define {name} |static \w+ {name}\[).*\n", source, re.MULTILINE)
    if len(line) == 1:
        return line[0]
    found = re.findall(rf"^(?:static )?(?:const )?\w+ \*?{name}\([^;{{]*\)\n\{{\n.*?^\}}\n", source, re.MULTILINE | re.DOTALL)
    if len(found) != 1:
        raise SystemExit(f"expected exactly one definition of {name}")
    return found[0]


def main(output, *arguments):
    source, parts = "", []
    for argument in arguments:
        if argument.endswith(".c"):
            source = Path(argument).read_text()
        else:
            parts.append(definition(source, argument))
    Path(output).write_text("\n".join(parts))


if __name__ == "__main__":
    main(*sys.argv[1:])
