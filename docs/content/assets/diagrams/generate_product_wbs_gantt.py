#!/usr/bin/env python3
"""Generate the 12-week WBS Gantt chart HTML from the planning CSV.

Usage:
    python3 docs/content/assets/diagrams/generate_product_wbs_gantt.py

The PNG files are browser captures of the generated HTML (Playwright,
viewport 1320px):
    product-wbs-gantt.png  - whole page (.wrap), deviceScaleFactor 2
    product-wbs.png        - chart only (.chart), deviceScaleFactor 1
"""

import csv
import json
from pathlib import Path


HERE = Path(__file__).resolve().parent
SOURCE = HERE.parent.parent / "planning" / "product-worklist-and-12week-wbs.csv"
TEMPLATE = HERE / "product-wbs-gantt.template.html"
OUTPUT = HERE / "product-wbs-gantt.html"


def load_rows():
    with SOURCE.open(encoding="utf-8-sig") as handle:
        return [
            {
                "id": row["wbs_id"],
                "p": row["product_id"],
                "pn": row["product"],
                "n": row["work_package"],
                "d": row["detail"],
                "pr": row["priority"],
                "s": int(row["start_week"]),
                "e": int(row["end_week"]),
                "pre": row["predecessors"],
                "m": row["milestone"],
                "ev": row["completion_evidence"],
                "st": row["status"],
                "t": row["task_refs"],
            }
            for row in csv.DictReader(handle)
        ]


def main():
    rows = load_rows()
    template = TEMPLATE.read_text(encoding="utf-8")
    OUTPUT.write_text(
        template.replace("__DATA__", json.dumps(rows, ensure_ascii=False)),
        encoding="utf-8",
    )
    print(f"wrote {OUTPUT.name}: {len(rows)} work packages")


if __name__ == "__main__":
    main()
