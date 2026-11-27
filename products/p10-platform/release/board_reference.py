"""Validate frozen board references separately from installed hardware."""

import argparse
from pathlib import Path
import re
import subprocess

from evidence import digest, read, require

HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[2]
REFERENCE=HERE/"board-reference.json"


def reference():
    record=read(REFERENCE)
    require(record["format"]=="nu54-board-reference-v1","unknown board reference")
    return record


def inspect_checkout(upstream):
    record=reference()
    revision=subprocess.check_output(["git","-C",upstream,"rev-parse","HEAD"],text=True).strip()
    status=subprocess.check_output(["git","-C",upstream,"status","--porcelain"],text=True)
    require(revision==record["commit"] and not status,"frozen clean board checkout differs")
    vendored=ROOT/"products/p01-device-firmware/boards/nucode/nu54v_dk"
    for kind,item in record["files"].items():
        path=Path(item["file"])
        require(digest(upstream/path)==item["sha256"],"upstream board reference bytes differ")
        if kind!="schematic":
            require(digest(vendored/path.name)==item["sha256"],"vendored build input differs from board reference")
    return record


def comparison(record, rows, channel, selection):
    if not selection:
        return {"state":"pending-input"}
    require(len(selection)<=5,"too many board comparison fields")
    if selection.get("board")!=record["board"] or selection.get("channel")!=channel:
        return {"state":"mismatch"}
    image, root=selection.get("image",""),selection.get("bootRoot","")
    if not re.fullmatch(r"[a-f0-9]{64}",image) or not re.fullmatch(r"[a-f0-9]{64}",root):
        return {"state":"mismatch"}
    matched=[row for row in rows if row["payload"]==image and row["boot_key"]==root and
             row["dimensions"]["board"]==record["board"]]
    if len(matched)!=1:
        return {"state":"mismatch"}
    return {"state":"fingerprints-match-evidence-pending","version":matched[0]["version"]}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("checkout",type=Path)
    args=parser.parse_args()
    item=inspect_checkout(args.checkout)
    print(item["commit"],item["referenceVariant"],"source references match; installed revision and manufacturing BOM require separate evidence")


if __name__=="__main__":
    main()
