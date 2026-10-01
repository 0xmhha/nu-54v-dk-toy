#!/usr/bin/env python3
"""Observe finality on the StableNet testnet for a fixed time (WBS2-P06-05).

Every second it reads the `latest` and `finalized` block headers and records the first hash
seen for each block number. Once a minute it re-reads the watched block (the contract-gate
payment). At the end it re-reads every block it saw and reports any hash that changed, so a
reorg of a block already reported as finalized shows up as a mismatch.

Standard library only. Writes a JSON-lines log and a summary JSON.

Usage:
  tools/observe_finality.py --seconds 3600 --watch-block 21129166 \
      --watch-hash 0x80bf5d...c45e --out ../../evidence/finality
"""

from __future__ import annotations

import argparse
import json
import statistics
import time
import urllib.request
from pathlib import Path

RPC = "https://api.test.stablenet.network/"


def rpc(method: str, params: list, retries: int = 3):
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params}).encode()
    for attempt in range(retries):
        try:
            req = urllib.request.Request(RPC, body, {"Content-Type": "application/json"})
            with urllib.request.urlopen(req, timeout=10) as r:
                out = json.load(r)
            if "error" in out:
                raise RuntimeError(out["error"])
            return out["result"]
        except Exception:  # network hiccups are counted, not fatal
            if attempt == retries - 1:
                raise
            time.sleep(0.5)


def header(tag: str) -> tuple[int, str, int]:
    b = rpc("eth_getBlockByNumber", [tag, False])
    return int(b["number"], 16), b["hash"], int(b["timestamp"], 16)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--seconds", type=int, default=3600)
    ap.add_argument("--watch-block", type=int, required=True)
    ap.add_argument("--watch-hash", required=True)
    ap.add_argument("--out", type=Path, required=True)
    a = ap.parse_args()
    a.out.mkdir(parents=True, exist_ok=True)
    stamp = time.strftime("%Y%m%dT%H%M%SZ", time.gmtime())
    log_path = a.out / f"finality-{stamp}.jsonl"

    chain_id = int(rpc("eth_chainId", []), 16)
    seen: dict[int, str] = {}
    lags, errors, watch_checks, watch_changed = [], 0, 0, 0
    block_times: dict[int, int] = {}
    start = time.time()
    next_watch = start
    with log_path.open("w") as log:
        while time.time() - start < a.seconds:
            t = time.time()
            try:
                ln, lh, lt = header("latest")
                fn, fh, _ = header("finalized")
            except Exception as e:
                errors += 1
                log.write(json.dumps({"t": round(t, 3), "error": str(e)[:200]}) + "\n")
                time.sleep(1)
                continue
            for n, h in ((ln, lh), (fn, fh)):
                if n in seen and seen[n] != h:
                    log.write(json.dumps({"t": round(t, 3), "hashChanged": n, "was": seen[n], "now": h}) + "\n")
                seen.setdefault(n, h)
            block_times[ln] = lt
            lags.append(ln - fn)
            rec = {"t": round(t, 3), "latest": ln, "finalized": fn, "lag": ln - fn, "latestHash": lh, "finalizedHash": fh}
            if t >= next_watch:
                wh = rpc("eth_getBlockByNumber", [hex(a.watch_block), False])["hash"]
                watch_checks += 1
                watch_changed += wh.lower() != a.watch_hash.lower()
                rec["watchHash"] = wh
                next_watch = t + 60
            log.write(json.dumps(rec) + "\n")
            log.flush()
            time.sleep(max(0.0, 1 - (time.time() - t)))

        # Re-read every block seen: a changed hash means a reorg after it was reported.
        changed = []
        for n, h in sorted(seen.items()):
            now = rpc("eth_getBlockByNumber", [hex(n), False])["hash"]
            if now != h:
                changed.append({"block": n, "was": h, "now": now})
        final_watch = rpc("eth_getBlockByNumber", [hex(a.watch_block), False])["hash"]

    nums = sorted(block_times)
    intervals = [block_times[b] - block_times[p] for p, b in zip(nums, nums[1:]) if b == p + 1]
    summary = {
        "chainId": chain_id,
        "startedAt": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(start)),
        "seconds": round(time.time() - start),
        "samples": len(lags),
        "rpcErrors": errors,
        "blocksSeen": len(seen),
        "firstBlock": min(seen) if seen else None,
        "lastBlock": max(seen) if seen else None,
        # latest and finalized are two separate calls, so a block produced between them shows up
        # as a lag of +1 or -1 even when finalized equals latest; read the distribution, not the max.
        "finalizedLag": {"distribution": {str(k): lags.count(k) for k in sorted(set(lags))},
                         "zeroShare": round(lags.count(0) / len(lags), 4) if lags else None},
        "blockIntervalSeconds": {
            "median": statistics.median(intervals) if intervals else None,
            "max": max(intervals, default=None),
        },
        "hashesChangedOnRecheck": changed,
        "watchBlock": {"number": a.watch_block, "expectedHash": a.watch_hash, "checks": watch_checks,
                       "changedDuringRun": watch_changed, "finalHash": final_watch,
                       "unchanged": watch_changed == 0 and final_watch.lower() == a.watch_hash.lower()},
        "log": log_path.name,
    }
    (a.out / f"finality-{stamp}.summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    print(json.dumps(summary, indent=2))
    return 0 if summary["watchBlock"]["unchanged"] and not changed else 1


if __name__ == "__main__":
    raise SystemExit(main())
