#!/usr/bin/env python3
"""Evidence collection for the week-12 acceptance log (WBS2-P10-03, [N16][N18]).

Raw evidence (logs, photos, receipts, dumps) stays in evidence/w12/ (git-ignored). The log
(docs/content/acceptance/week12-log.md) gets only shortened addresses and tx hashes (0x + 6 ... 4)
and full 64-hex checksums written as `sha256:<hex>`, which is what the register validator's
redaction group checks.

    w12.py env [--firmware IMG] [--apk APK] [--write]   environment table (section 1)
    w12.py add ITEM FILE...                              store evidence files for an item
    w12.py tx ITEM TXHASH                                store a receipt; print the PaymentSettled summary
    w12.py timings ITEM CSV                              W12-04: 20 runs, each <= 10 s
    w12.py record ITEM RESULT [--note TEXT]              write the result and evidence cells of a row
    w12.py check                                         evidence hashes still match; every row has a result

ITEM is a row key: W12-01 .. W12-12, or a refusal code row (ATTESTATION_EXPIRED, ...).
RPC: --rpc or NU54_RPC (default the StableNet testnet).
"""

from __future__ import annotations

import argparse
import csv
import datetime as dt
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
LOG = ROOT / "docs/content/acceptance/week12-log.md"
EVIDENCE = ROOT / "evidence/w12"
DEPLOYMENT = ROOT / "products/p06-stablenet-contracts/deployments/8283.json"
REGISTER = ROOT / "docs/content/planning/design-freeze-checkpoint-02.json"
DEFAULT_RPC = "https://api.test.stablenet.network/"
# keccak256("PaymentSettled(address,bytes32,address,uint256,uint256)")
PAYMENT_SETTLED = "0xeef4300dbd9217414481ce2ade0c4791c3b47c75b0c0d4b6bbe5fbb05260195f"
BUDGET_MS = 10_000  # W12-04: request delivered -> approved
RESULTS = ("통과", "실패", "조건부 통과", "미실행")


# ---------------------------------------------------------------- helpers

def short(h: str) -> str:
    """0x + first 6 + ... + last 4 [N16]."""
    return f"{h[:8]}…{h[-4:]}" if h.startswith("0x") and len(h) > 14 else h


def sha256_file(p: Path) -> str:
    d = hashlib.sha256()
    with p.open("rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            d.update(chunk)
    return d.hexdigest()


def rpc(method: str, params: list, url: str) -> object:
    req = urllib.request.Request(url, json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params}).encode(),
                                 {"content-type": "application/json"})
    with urllib.request.urlopen(req, timeout=20) as r:
        body = json.load(r)
    if "error" in body:
        raise SystemExit(f"rpc {method}: {body['error']}")
    return body["result"]


def git(*args: str) -> str:
    return subprocess.run(["git", *args], cwd=ROOT, capture_output=True, text=True).stdout.strip()


def version_of(cmd: list[str]) -> str:
    try:
        out = subprocess.run(cmd, capture_output=True, text=True, timeout=20)
        return (out.stdout or out.stderr).strip().splitlines()[0]
    except (OSError, subprocess.TimeoutExpired, IndexError):
        return "없음"


def firmware_toolchain(fw_dir: Path) -> str:
    """NCS and Zephyr versions the firmware was built with, from the build's generated headers."""
    gen = fw_dir / "build/app/zephyr/include/generated"
    found = {}
    for name, header, macro in (("NCS", "ncs_version.h", "NCS_VERSION_STRING"), ("Zephyr", "zephyr/version.h", "KERNEL_VERSION_STRING")):
        p = gen / header
        m = re.search(rf'#define {macro}\s+"([^"]+)"', p.read_text()) if p.exists() else None
        found[name] = m.group(1) if m else "?"
    return f"NCS {found['NCS']}, Zephyr {found['Zephyr']}"


class Manifest:
    """evidence/w12/manifest.json: every stored file with its checksum, per item."""

    def __init__(self, base: Path | None = None):
        self.base = base or EVIDENCE  # read at call time, so tests can point it elsewhere
        self.path = self.base / "manifest.json"
        self.data: dict = json.loads(self.path.read_text()) if self.path.exists() else {"items": {}}

    def add(self, item: str, src: Path, name: str | None = None) -> dict:
        dest_dir = self.base / item
        dest_dir.mkdir(parents=True, exist_ok=True)
        dest = dest_dir / (name or src.name)
        if src.resolve() != dest.resolve():
            shutil.copy2(src, dest)
        entry = {"file": str(dest.relative_to(self.base)), "sha256": sha256_file(dest), "bytes": dest.stat().st_size,
                 "added": dt.datetime.now(dt.timezone.utc).isoformat(timespec="seconds")}
        files = self.data["items"].setdefault(item, {"files": [], "summary": []})["files"]
        files[:] = [f for f in files if f["file"] != entry["file"]] + [entry]
        self.save()
        return entry

    def note(self, item: str, text: str) -> None:
        self.data["items"].setdefault(item, {"files": [], "summary": []})["summary"].append(text)
        self.save()

    def save(self) -> None:
        self.base.mkdir(parents=True, exist_ok=True)
        self.path.write_text(json.dumps(self.data, ensure_ascii=False, indent=2) + "\n")


# ---------------------------------------------------------------- the log table

def row_index(lines: list[str], item: str) -> int:
    """The table row whose first cell is `item`."""
    for i, line in enumerate(lines):
        cells = [c.strip() for c in line.strip().strip("|").split("|")]
        if line.startswith("|") and cells and cells[0] == item:
            return i
    raise SystemExit(f"no row for {item} in {LOG.name}")


def set_cells(text: str, item: str, values: dict[int, str]) -> str:
    """Replaces cells (by index, negative from the end) of `item`'s row; '|' in values is escaped."""
    lines = text.split("\n")
    i = row_index(lines, item)
    cells = lines[i].strip().strip("|").split("|")
    for k, v in values.items():
        cells[k] = f" {v.replace('|', '/')} "
    lines[i] = "|" + "|".join(cells) + "|"
    return "\n".join(lines)


def evidence_cell(entry: dict) -> str:
    files = "; ".join(f"`{Path(f['file']).name}` sha256:{f['sha256']}" for f in entry.get("files", []))
    summary = "; ".join(entry.get("summary", []))
    return "; ".join(x for x in (summary, files) if x)


# ---------------------------------------------------------------- commands

def cmd_env(a: argparse.Namespace) -> dict:
    dep = json.loads(DEPLOYMENT.read_text())
    fw_dir = ROOT / "products/p01-device-firmware"
    fw_version = re.search(r'#define FIRMWARE_VERSION "([^"]+)"', (fw_dir / "app/src/pay_link.c").read_text())
    kiosk_pkg = json.loads((ROOT / "products/p04-merchant-kiosk/package.json").read_text())
    commit = git("rev-parse", "--short", "HEAD")
    fw_img = Path(a.firmware) if a.firmware else fw_dir / "build/app/zephyr/zephyr.hex"
    apk = Path(a.apk) if a.apk else ROOT / "products/p04-merchant-kiosk/android/app/build/outputs/apk/debug/app-debug.apk"
    chain = int(str(rpc("eth_chainId", [], a.rpc)), 16)
    gas = int(str(rpc("eth_getBalance", [dep["roles"]["kiosk"], "latest"], a.rpc)), 16)
    reg_commit = git("log", "-1", "--format=%h", "--", str(REGISTER.relative_to(ROOT)))
    rows = {
        "기기 펌웨어": (f"{fw_version.group(1) if fw_version else '?'} (`{commit}`)",
                     f"`{fw_img.name}` sha256:{sha256_file(fw_img)}" if fw_img.exists() else f"이미지 없음: {fw_img.name}"),
        "키오스크 빌드": (f"{kiosk_pkg['version']} (`{commit}`)",
                     f"`{apk.name}` sha256:{sha256_file(apk)}" if apk.exists() else f"APK 없음: {apk.name}"),
        "정산 컨트랙트": (f"`{short(dep['contracts']['PaymentSettlement']['address'])}`", f"deployments/8283.json, 배포 블록 {dep['contracts']['PaymentSettlement']['block']}"),
        "체인": (f"StableNet testnet {chain}", f"`eth_chainId` = {hex(chain)}"),
        "파라미터": ("DF-20260925-02 parameters [N13]", f"register `{reg_commit}`"),
        "도구·의존성 버전": ("; ".join([firmware_toolchain(fw_dir), version_of(["forge", "--version"]),
                                  f"React Native {kiosk_pkg['dependencies'].get('react-native', '?')}", version_of(["go", "version"])]),
                       "의존성 pinning은 waiver [N14]"),
        "키오스크 시작 가스 잔액": (f"{gas / 1e18:.2f} WKRC", f"`eth_getBalance` 키오스크 `{short(dep['roles']['kiosk'])}`"),
    }
    for k, (value, ev) in rows.items():
        print(f"| {k} | {value} | {ev} |")
    if chain != dep["chainId"]:
        print(f"warning: RPC chain {chain} differs from the deployment's {dep['chainId']}", file=sys.stderr)
    if a.write:
        text = LOG.read_text()
        for k, (value, ev) in rows.items():
            text = set_cells(text, k, {1: value, -1: ev})
        LOG.write_text(text)
        print(f"wrote section 1 of {LOG.relative_to(ROOT)}", file=sys.stderr)
    return rows


def cmd_add(a: argparse.Namespace) -> None:
    m = Manifest()
    for f in a.files:
        e = m.add(a.item, Path(f))
        print(f"{a.item}: {e['file']} sha256:{e['sha256']}")


def decode_settled(receipt: dict, settlement: str) -> dict | None:
    for log in receipt.get("logs", []):
        if log["address"].lower() == settlement.lower() and log["topics"][0].lower() == PAYMENT_SETTLED:
            data = log["data"][2:]
            return {"merchant": "0x" + log["topics"][1][-40:], "orderId": log["topics"][2], "device": "0x" + log["topics"][3][-40:],
                    "amount": int(data[:64], 16), "nonce": int(data[64:128], 16)}
    return None


def cmd_tx(a: argparse.Namespace) -> dict:
    dep = json.loads(DEPLOYMENT.read_text())
    receipt = rpc("eth_getTransactionReceipt", [a.txhash], a.rpc)
    if not receipt:
        raise SystemExit("no receipt (not mined, or another chain)")
    finalized = int(str(rpc("eth_getBlockByNumber", ["finalized", False], a.rpc)["number"]), 16)
    block = int(receipt["blockNumber"], 16)
    event = decode_settled(receipt, dep["contracts"]["PaymentSettlement"]["address"])
    raw = EVIDENCE / a.item / f"receipt-{a.txhash[2:10]}.json"
    raw.parent.mkdir(parents=True, exist_ok=True)
    raw.write_text(json.dumps(receipt, indent=2) + "\n")
    m = Manifest()
    m.add(a.item, raw)
    status = "성공" if receipt["status"] == "0x1" else "실패(status 0)"
    summary = f"tx `{short(a.txhash)}` 블록 {block} {status}" + (" finalized" if block <= finalized else " (finalized 전)")
    if event:
        summary += f", PaymentSettled 기기 `{short(event['device'])}` 금액 {event['amount']} nonce {short(hex(event['nonce']))}"
    m.note(a.item, summary)
    print(summary)
    return {"summary": summary, "event": event}


def judge_timings(rows: list[dict]) -> tuple[bool, str]:
    ms = [int(r["ms"]) for r in rows]
    ok = len(ms) == 20 and all(x <= BUDGET_MS for x in ms) and all(r.get("outcome", "approved") == "approved" for r in rows)
    text = f"{len(ms)}회, 최대 {max(ms) / 1000:.1f} s, 평균 {sum(ms) / len(ms) / 1000:.1f} s" if ms else "기록 없음"
    return ok, text


def cmd_timings(a: argparse.Namespace) -> bool:
    with open(a.csv, newline="") as f:
        rows = list(csv.DictReader(f))
    if rows and "ms" not in rows[0]:
        raise SystemExit("the CSV needs a column `ms` (request delivered -> approved), optionally `outcome`")
    ok, text = judge_timings(rows)
    m = Manifest()
    m.add(a.item, Path(a.csv), "timings.csv")
    m.note(a.item, text + ("" if ok else " (기준 미달)"))
    print(("통과: " if ok else "실패: ") + text)
    return ok


def cmd_record(a: argparse.Namespace) -> None:
    if a.result not in RESULTS:
        raise SystemExit(f"result is one of {', '.join(RESULTS)}")
    m = Manifest()
    if a.note:
        m.note(a.item, a.note)
    entry = m.data["items"].get(a.item, {"files": [], "summary": []})
    if a.result != "미실행" and not entry["files"]:
        raise SystemExit(f"{a.item} has no evidence file yet: add one first (w12.py add/tx/timings)")
    LOG.write_text(set_cells(LOG.read_text(), a.item, {-2: a.result, -1: evidence_cell(entry)}))
    print(f"{a.item}: {a.result}")


def item_rows(text: str) -> list[tuple[str, str]]:
    """(key, result) of every W12 item and refusal-code row."""
    out = []
    for line in text.split("\n"):
        cells = [c.strip() for c in line.strip().strip("|").split("|")]
        if line.startswith("|") and cells and (re.fullmatch(r"W12-\d\d", cells[0]) or re.fullmatch(r"[A-Z_]{6,}", cells[0])) and len(cells) >= 4:
            out.append((cells[0], cells[-2]))
    return out


def cmd_check(_: argparse.Namespace) -> bool:
    ok = True
    m = Manifest()
    for item, entry in m.data["items"].items():
        for f in entry["files"]:
            p = m.base / f["file"]
            if not p.exists() or sha256_file(p) != f["sha256"]:
                print(f"FAIL {item}: {f['file']} is missing or changed")
                ok = False
    pending = [k for k, r in item_rows(LOG.read_text()) if r not in RESULTS[:3]]
    if pending:
        print(f"pending ({len(pending)}): {', '.join(pending)}")
        ok = False
    print("ok: evidence matches and every row has a result" if ok else "not complete")
    return ok


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--rpc", default=os.environ.get("NU54_RPC", DEFAULT_RPC))
    sub = p.add_subparsers(dest="cmd", required=True)
    e = sub.add_parser("env")
    e.add_argument("--firmware")
    e.add_argument("--apk")
    e.add_argument("--write", action="store_true")
    ad = sub.add_parser("add")
    ad.add_argument("item")
    ad.add_argument("files", nargs="+")
    t = sub.add_parser("tx")
    t.add_argument("item")
    t.add_argument("txhash")
    ti = sub.add_parser("timings")
    ti.add_argument("item")
    ti.add_argument("csv")
    r = sub.add_parser("record")
    r.add_argument("item")
    r.add_argument("result")
    r.add_argument("--note")
    sub.add_parser("check")
    a = p.parse_args(argv)
    result = {"env": cmd_env, "add": cmd_add, "tx": cmd_tx, "timings": cmd_timings, "record": cmd_record, "check": cmd_check}[a.cmd](a)
    return 1 if result is False else 0


if __name__ == "__main__":
    raise SystemExit(main())
