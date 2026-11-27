"""Build the offline page design book and its requirement-to-screen index."""
from pathlib import Path
import argparse
import hashlib
import json
import re

HERE = Path(__file__).resolve().parent
PRODUCTS = HERE.parent / "products"
KINDS = {"home", "form", "review", "receipt", "list", "monitor"}
TONES = {"normal", "success", "warning", "blocked"}
# The board has no screen: any state text that asks for a board action must name its board step.
BOARD_ACTION = re.compile(r"SW[1-4]|PIN|보드에서|보드 버튼|물리 확인|보드 승인|실물 승인|버튼으로 승인|기기 승인|기기에서|"
                          r"(?:기기|보드)(?:로|에서)? ?(?:승인|확인|대조|전달)|보드 주소")
# The phone is not the reference for what is approved (security.md TM-002): approval copy that
# says "if it matches" must point at the independent display.
PHONE_AS_REFERENCE = re.compile(r"(?:맞으면|같으면|일치하면)[^.。]*(?:SW1|승인)")
AREAS = {"W": "지갑·보드·클라우드", "K": "매장·키오스크", "T": "여행·예약",
         "O": "가맹점 운영", "L": "테스트 시장", "X": "체인 영수증", "D": "SDK·릴리스"}


def sources():
    result = {}
    for folder in sorted(PRODUCTS.glob("p[0-9][0-9]-*")):
        pid = folder.name[:3].upper()
        prd = (folder / "prd.md").read_text()
        use_cases = (folder / "use-cases.md").read_text()
        for number, name, behavior, test in re.findall(
            r"^\| (R\d+) \| (.*?) \| (.*?) \| (.*?) \|$", prd, re.M
        ):
            result[f"{pid}.{number}"] = {
                "title": name, "criteria": test, "behavior": behavior,
                "source": f"../products/{folder.name}/prd.md"
            }
        for case in re.split(r"(?m)(?=^### UC\d+ )", use_cases)[1:]:
            number, name = re.match(r"### (UC\d+) · (.+)", case).groups()
            fields = dict(re.findall(r"^- \*\*(.*?)\*\* (.*)$", case, re.M))
            result[f"{pid}.{number}"] = {
                "title": name.split(" (R")[0],
                "criteria": fields["합격 증거"], "behavior": fields["정상 흐름"],
                "source": f"../products/{folder.name}/use-cases.md"
            }
    return result


def validate(pages, refs):
    known = {}
    for p in pages:
        key = p["id"]
        assert re.fullmatch(r"[A-Z]\d{2,3}", key) and key not in known, key
        assert all(p.get(k) for k in ("title", "area", "role", "entry", "accept", "refs")), key
        assert p["kind"] in KINDS and p["states"][0]["id"] == "start", key
        known[key] = {s["id"] for s in p["states"]}
        assert len(known[key]) == len(p["states"]), key
        assert all(r in refs for r in p["refs"]), (key, p["refs"])
        for s in p["states"]:
            assert re.fullmatch(r"[a-z][a-zA-Z_0-9-]*", s["id"]), (key, s["id"])
            assert all(s.get(k) for k in ("label", "headline", "note", "rows")), (key, s)
            assert s.get("tone", "normal") in TONES, (key, s)
            for field in ("rows", "actions", "events", "inputs"):
                for pair in s.get(field, []):
                    assert len(pair) == 2 and all(isinstance(v, str) and v.strip() for v in pair), (key, field, pair)
    for p in pages:
        for s in p["states"]:
            for _, link in s.get("actions", []) + s.get("events", []):
                page, _, state = link.partition("#")
                assert page in known and (state or "start") in known[page], (p["id"], s["id"], link)
    mapped = {r for p in pages for r in p["refs"]}
    assert mapped == set(refs), "Unmapped requirements/use cases: " + str(set(refs) - mapped)
    edges = {}
    for p in pages:
        for s in p["states"]:
            edges[(p["id"], s["id"])] = [
                tuple((t + "#start").split("#")[:2])
                for _, t in s.get("actions", []) + s.get("events", [])
            ]
    pending = [(p["id"], "start") for p in pages]
    seen = set(pending)
    while pending:
        for target in edges[pending.pop()]:
            if target not in seen:
                seen.add(target)
                pending.append(target)
    assert set(edges) == seen, f"States accessible only from the review picker: {set(edges) - seen}"


def validate_products(pages, products, board):
    ids = {p["id"] for p in pages}
    owner = {}
    for product in products["products"]:
        assert product["surface"] in {"mobile", "kiosk", "desk"}, product["id"]
        for key in product["pages"]:
            assert key in ids and key not in owner, (product["id"], key)
            owner[key] = product["id"]
    assert set(owner) == ids, f"Pages without a product: {ids - set(owner)}"
    steps, flows = board["steps"], board["flows"]
    for name, flow in flows.items():
        assert flow["steps"] and all(step in steps for step in flow["steps"]), name
    states = {(p["id"], s["id"]): s for p in pages for s in p["states"]}
    for target, (flow, step) in board["states"].items():
        page, _, state = target.partition("#")
        assert (page, state) in states, target
        assert owner[page] == "mobile", f"{target}: board steps are shown by the mobile app"
        assert flow in flows and (step in flows[flow]["steps"] or step == "failed"), target
    for target in board["notBoardSteps"]:
        assert tuple(target.split("#")) in states and target not in board["states"], target
    for (page, state), s in states.items():
        target = f"{page}#{state}"
        text = json.dumps(s, ensure_ascii=False)
        if owner[page] == "mobile" and BOARD_ACTION.search(text):
            assert target in board["states"] or target in board["notBoardSteps"], \
                f"{target} asks for a board action but names no board step"
        if target in board["states"] and "독립 표시" in text and board["states"][target][1] != "failed":
            assert "display" in flows[board["states"][target][0]]["steps"], \
                f"{target} asks for the independent display but its board flow has no display step"
        for sentence in re.split(r"[.。\"]", text):
            assert not PHONE_AS_REFERENCE.search(sentence) or "독립" in sentence, f"{target}: approval copy must name the independent display: {sentence}"
    for info in steps.values():
        assert not PHONE_AS_REFERENCE.search(info["do"]) or "독립" in info["do"], info["do"]
    return owner


def board_line(board, target):
    if target not in board["states"]:
        return []
    flow, step = board["states"][target]
    order = board["flows"][flow]["steps"]
    where = f"{order.index(step) + 1}/{len(order)}" if step in order else "종료"
    info = board["steps"][step]
    buttons = " · ".join(f"{b} {how}" for b, how in info["buttons"]) or "버튼 없음"
    return [f"하드월렛 단계 — {board['flows'][flow]['label']} {where} {info['label']}: {info['do']} ({buttons})"]


def page_specs(prefix, pages, refs, owner, products, board):
    def cell(text):
        shown = re.sub(r"\{\{(?:address|hash):(0x[\da-f]{4}…[\da-f]{4})\}\}", r"\1 (화면에서 합성값 전체 표시)", text)
        return shown.replace("|", "&#124;").replace("\n", " ")

    def target_link(label, target):
        return f"[{cell(label)}](../book.html#{target.replace('#', '/')})"

    def links(state, key):
        return "<br>".join(target_link(label, dest) for label, dest in state.get(key, [])) or "—"

    lines = [f"# {AREAS[prefix]} · 페이지별 설계", "",
             "[전체 화면](../book.html) · [원 요구사항 대조표](../coverage.md) · [통합 감사](../audit.md)", "",
             "제품에서 누르는 동작과 상태를 바꾸는 외부 사건을 열로 구분했다. 입력은 예시 데이터이며 실제 제품의 인수에서는 진입 조건과 검증 결과까지 함께 확인한다.", ""]
    for p in pages:
        if p["id"][0] != prefix:
            continue
        product = next(x["name"] for x in products["products"] if x["id"] == owner[p["id"]])
        lines += [f"## {p['id']} · {p['title']}", "",
                  f"**제품** {product}　 **사용자** {p['role']}　 **진입** {p['entry']}", "",
                  f"**완료 기준** {p['accept']}", "",
                  " · ".join(f"[{k}](../{refs[k]['source']})" for k in p['refs']), "",
                  "| 화면 상태 | 사용자에게 전달하는 정보 | 입력 예시 · 표시 데이터 | 사용자 동작과 목적지 | 외부 사건과 결과 |",
                  "|---|---|---|---|---|"]
        for s in p["states"]:
            facts = [f"입력 · {k} — {v}" for k, v in s.get("inputs", [])]
            facts.extend(f"{k} — {v}" for k, v in s["rows"])
            facts.extend(board_line(board, p["id"] + "#" + s["id"]))
            row = [target_link(s["label"], p["id"] + "#" + s["id"]),
                   f"**{cell(s['headline'])}**<br>{cell(s['note'])}",
                   "<br>".join(map(cell, facts)), links(s, "actions"), links(s, "events")]
            lines.append("| " + " | ".join(row) + " |")
        lines.append("")
    return "\n".join(lines) + "\n"


def matrix(pages, refs):
    lines = ["# 기능·유즈케이스와 화면 연결표", "",
             "요구사항이나 유즈케이스 한 행에서 해당 페이지를 연다. 페이지의 ‘설계 검토’에서 정상·예외 상태와 외부 사건을 바꾸고 원문 인수 조건을 대조한다. [전체 페이지 설계](book.html) · [검토 결과와 기획 변경](audit.md)", ""]
    for folder in sorted(PRODUCTS.glob("p[0-9][0-9]-*")):
        pid = folder.name[:3].upper()
        lines += [f"## {pid} · {folder.name[4:]}", "", "| 기준 | 기능 또는 사용자 목표 | 대응 페이지 |",
                  "|---|---|---|"]
        for key, ref in refs.items():
            if key.startswith(pid + "."):
                links = " · ".join(f"[{p['id']} {p['title']}](book.html#{p['id']})" for p in pages if key in p["refs"])
                lines.append(f"| [{key}]({ref['source']}) | {ref['title']} | {links} |")
        lines += [""]
    return "\n".join(lines) + "\n"


def build(check):
    pages = [p for file in sorted((HERE / "catalog").glob("*.json"))
             for p in json.loads(file.read_text())]
    pages.sort(key=lambda p: ("WKTOLXD".index(p["id"][0]), p["id"]))
    refs = sources()
    validate(pages, refs)
    products = json.loads((HERE / "products.json").read_text())
    board = json.loads((HERE / "board.json").read_text())
    owner = validate_products(pages, products, board)
    data = json.dumps({"pages": pages, "refs": refs, "products": products, "board": board},
                      ensure_ascii=False, separators=(",", ":"))
    data = data.replace("<", "\\u003c")
    html = (HERE / "book.template.html").read_text().replace("<!--PAGE_DATA-->", data)
    outputs = {"coverage.md": matrix(pages, refs), "book.html": html}
    outputs.update({f"specs/{prefix}.md": page_specs(prefix, pages, refs, owner, products, board) for prefix in AREAS})
    for file, content in outputs.items():
        dest = HERE / file
        if check:
            assert dest.read_text() == content, f"{file} no longer matches the design sources"
        else:
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_text(content)
    print(f"{len(pages)} pages, {sum(len(p['states']) for p in pages)} states, "
          f"{len(refs)} mapped source entries, {hashlib.sha256(data.encode()).hexdigest()[:12]}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    build(parser.parse_args().check)
