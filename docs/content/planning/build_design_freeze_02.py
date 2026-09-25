#!/usr/bin/env python3
"""Render DF-20260925-02 from its JSON register.

The JSON register is the single source of truth. This script only renders it to
Markdown, so re-running it on the committed JSON must reproduce the committed
Markdown byte for byte (checked by validate_design_freeze_02.py --check register).

Usage:
    python3 docs/content/planning/build_design_freeze_02.py          # write the md
    python3 docs/content/planning/build_design_freeze_02.py --stdout # print only
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
REGISTER = "docs/content/planning/design-freeze-checkpoint-02.json"
RENDERED = "docs/content/planning/design-freeze-checkpoint-02.md"

DISPOSITION_KO = {"kept": "유지", "amended": "변경", "withdrawn": "철회"}


def link(from_md: str, target: str) -> str:
    """Relative Markdown link from one repo path to another."""
    import os

    return os.path.relpath(target, str(Path(from_md).parent))


def cell(text: str) -> str:
    return text.replace("|", "\\|").replace("\n", " ")


def render(reg: dict) -> str:
    scope = reg["freezeScope"]
    out: list[str] = []
    out.append(f"# 설계 동결 {reg['checkpointId']}\n")
    out.append(
        f"{reg['date']} · `{reg['checkpointId']}` · `{reg['supersedes']}`를 대체한다 · 문서 기준선, 구현 이전\n"
    )
    out.append(
        "이 문서는 `design-freeze-checkpoint-02.json`을 `build_design_freeze_02.py`로 렌더링한 결과다. "
        "값을 바꿀 때는 JSON을 고치고 다시 렌더링한다. 제품 문서와 프로토콜 문서는 값을 다시 적지 않고 "
        "`[N03]`처럼 결정 ID를 인용한다.\n"
    )
    out.append(
        f"`{reg['supersedes']}`와 그 생성 산출물은 바이트 단위로 그대로 두고, 우선순위는 "
        "저장소 읽기 순서와 아래 배너로 바뀐다. baseCommit 이전 문서 전체는 충돌하는 부분에서 이 동결에 따른다.\n"
    )

    out.append("## 1. 새 결정\n")
    out.append("| ID | 제목 | 결정 | 인용해야 하는 문서 | 해소하는 충돌 |")
    out.append("|---|---|---|---|---|")
    for d in reg["newDecisions"]:
        docs = ", ".join(f"[{Path(p).parent.name}/{Path(p).name}]({link(RENDERED, p)})" for p in d["requiredIn"])
        res = ", ".join(d["resolves"]) or "-"
        out.append(f"| **{d['id']}** | {cell(d['title'])} | {cell(d['value'])} | {docs} | {res} |")
    out.append("")

    out.append(f"## 2. {reg['supersedes']} 결정의 처분\n")
    out.append("| ID | 처분 | 이전 값 | 새 값 | 이유 |")
    out.append("|---|---|---|---|---|")
    for d in reg["dispositions"]:
        out.append(
            f"| **{d['id']}** | {DISPOSITION_KO[d['disposition']]} | {cell(d['old'])} | {cell(d['new'])} | {cell(d['reason'])} |"
        )
    out.append("")

    out.append("## 3. 파라미터\n")
    out.append("값은 여기 한 곳에만 둔다. 다른 문서는 [N13]·[N10]·[N05]·[N06]·[N11]을 인용한다.\n")
    out.append("| 이름 | 값 | 단위 | 범위 | 근거 |")
    out.append("|---|---:|---|---|---|")
    for name, p in reg["parameters"].items():
        out.append(f"| `{name}` | {p['value']} | {p['unit']} | {cell(p['bound'])} | {cell(p['rationale'])} |")
    out.append("")

    out.append("## 4. 제품 범위\n")
    out.append("| DF-01 ID | 제품 | 이번 사이클 |")
    out.append("|---|---|---|")
    for m in reg["productIdMap"]:
        status = "만든다" if m["status"] == "buildable" else "설계만 한다"
        out.append(f"| {m['df01']} | {m['product']} | {status} |")
    out.append("")

    out.append("## 5. 우선순위와 배너\n")
    out.append("다음 파일의 읽기 순서에서 이 동결이 이전 동결보다 먼저 나온다.\n")
    for p in reg["precedence"]:
        out.append(f"- [{p['file']}]({link(RENDERED, p['file'])})")
    out.append("")
    out.append("다음 문서는 첫 줄에 대체 배너가 있고 나머지는 baseCommit 원문과 같다.\n")
    for b in reg["bannerRecord"]:
        out.append(f"- [{b['path'].removeprefix('docs/content/')}]({link(RENDERED, b['path'])}) `{b['baseSha256']}`")
    out.append("")

    out.append("## 6. 검증 범위\n")
    out.append(f"- baseCommit: `{scope['baseCommit'][:12]}`")
    out.append("- stale literal: " + ", ".join(f"{s['id']}({cell(s['why'])})" for s in scope["staleLiterals"]))
    out.append("- 예외 경로: DF-01 산출물과 입력, " + ", ".join(f"`{p}`" for p in scope["exemptPathsFixed"]))
    out.append(
        "- redaction 예외: "
        + ", ".join(f"`{e['path'].removeprefix('docs/content/')}`({e['reason']})" for e in scope["redactionExemptions"])
    )
    out.append("- waiver: " + ", ".join(reg["waivers"]))
    out.append("")
    if reg.get("amendments"):
        out.append("## 7. 사이클 중 변경\n")
        out.append("| 날짜 | 결정 | 변경 | 이유 | 유지한 합의 | 비용 |")
        out.append("|---|---|---|---|---|---|")
        for a in reg["amendments"]:
            out.append(
                f"| {a['date']} | {', '.join('[' + d + ']' for d in a['decisions'])} | {cell(a['change'])} | "
                f"{cell(a['reason'])} | {cell(a['preserves'])} | {cell(a['cost'])} |"
            )
        out.append("")
    out.append("검증: `python3 docs/content/planning/validate_design_freeze_02.py --check <group>` 와 `--self-test`.")
    return "\n".join(out) + "\n"


def main() -> int:
    reg = json.loads((ROOT / REGISTER).read_text(encoding="utf-8"))
    text = render(reg)
    if "--stdout" in sys.argv:
        sys.stdout.write(text)
    else:
        (ROOT / RENDERED).write_text(text, encoding="utf-8")
        print(RENDERED)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
