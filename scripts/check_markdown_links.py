#!/usr/bin/env python3
"""Check local Markdown links in active repository documentation."""

from __future__ import annotations

import re
import sys
from pathlib import Path
from urllib.parse import unquote


ROOT = Path(__file__).resolve().parents[1]
EXCLUDED_PARTS = {".git", "node_modules", "design-history"}
SNAPSHOT_PREFIXES = ("before-",)
LINK_PATTERN = re.compile(r"!?\[[^\]]*\]\(([^)]+)\)")


def is_active_markdown(path: Path) -> bool:
    relative = path.relative_to(ROOT)
    if any(part in EXCLUDED_PARTS for part in relative.parts):
        return False
    return not any(part.startswith(SNAPSHOT_PREFIXES) for part in relative.parts)


def local_target(raw_target: str) -> str | None:
    target = raw_target.strip().strip("<>")
    if not target or target.startswith("#"):
        return None
    if re.match(r"^[a-zA-Z][a-zA-Z0-9+.-]*:", target):
        return None
    return unquote(target.split("#", 1)[0].split("?", 1)[0])


def main() -> int:
    broken: list[str] = []
    checked = 0
    for markdown in sorted(ROOT.rglob("*.md")):
        if not is_active_markdown(markdown):
            continue
        text = markdown.read_text(encoding="utf-8")
        for line_number, line in enumerate(text.splitlines(), start=1):
            for match in LINK_PATTERN.finditer(line):
                target = local_target(match.group(1))
                if target is None:
                    continue
                checked += 1
                resolved = (markdown.parent / target).resolve()
                if not resolved.exists():
                    broken.append(
                        f"{markdown.relative_to(ROOT)}:{line_number}: {target}"
                    )

    if broken:
        print("Broken local Markdown links:")
        print("\n".join(f"- {item}" for item in broken))
        return 1

    print(f"Checked {checked} local links; all targets exist.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
