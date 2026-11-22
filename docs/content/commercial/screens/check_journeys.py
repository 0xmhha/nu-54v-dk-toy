"""Check critical UX transitions against reviewed business invariants."""
from pathlib import Path
from urllib.parse import unquote
import json
import re
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parent
LINK = re.compile(r"\[([^\[\]]+)\]\(([^()\s]+)\)")


def canonical(ref):
    return ref if "#" in ref else ref + "#start"


def check_runtime_coverage(pages, markdown):
    """Check that each design state appears exactly once in its ledger row."""
    catalog = {}
    for page in pages:
        pid = page["id"]
        ids = [state["id"] for state in page["states"]]
        if pid in catalog or len(ids) != len(set(ids)):
            raise ValueError(f"duplicate catalog page or state: {pid}")
        catalog[pid] = (page["title"], set(ids))

    seen_pages = set()
    observed = pending = 0
    for number, line in enumerate(markdown.splitlines(), start=1):
        line = line.strip()
        if not line.startswith("|"):
            continue
        cells = re.split(r"(?<!\\)\|", line)
        if len(cells) != 6 or cells[-1]:
            raise ValueError(f"runtime ledger line {number}: expected four columns")
        row = [cell.strip() for cell in cells[1:-1]]
        if row == ["페이지", "제품 UI에서 관측", "증거 연결 대기", "실행 근거"] or all(
            re.fullmatch(r":?-{3,}:?", cell) for cell in row
        ):
            continue
        heading = LINK.fullmatch(row[0])
        if not heading or not heading[2].startswith("book.html#"):
            raise ValueError(f"runtime ledger line {number}: invalid page link")
        pid = heading[2].removeprefix("book.html#")
        if pid not in catalog:
            raise ValueError(f"runtime ledger line {number}: unknown page {pid}")
        title, expected = catalog[pid]
        if heading[1] != f"{pid} {title}":
            raise ValueError(f"runtime ledger line {number}: wrong label for page {pid}")
        if pid in seen_pages:
            raise ValueError(f"runtime ledger line {number}: duplicate page {pid}")
        seen_pages.add(pid)

        covered = set()
        counts = []
        for cell in row[1:3]:
            entries = [] if cell == "—" else cell.split(" · ")
            for entry in entries:
                state = LINK.fullmatch(entry)
                if not state or state[1] not in expected or state[2] != f"book.html#{pid}/{state[1]}":
                    raise ValueError(f"runtime ledger line {number}: invalid state link for {pid}: {entry}")
                if state[1] in covered:
                    raise ValueError(f"runtime ledger line {number}: duplicate state {pid}/{state[1]}")
                covered.add(state[1])
            counts.append(len(entries))
        if covered != expected:
            raise ValueError(f"runtime ledger line {number}: missing {pid} states {sorted(expected - covered)}")
        if counts[0] and not any(
            match[1].strip()
            and match[2].partition("#")[0] not in {"", "book.html", "runtime-states.md"}
            for match in LINK.finditer(row[3])
        ):
            raise ValueError(f"runtime ledger line {number}: observed page {pid} needs an evidence link")
        observed += counts[0]
        pending += counts[1]
    if seen_pages != catalog.keys():
        raise ValueError(f"runtime ledger misses catalog pages {sorted(catalog.keys() - seen_pages)}")
    return len(catalog), observed + pending, observed, pending


def check_evidence_anchors(ledger, evidence):
    references = set()
    for link in LINK.finditer(ledger):
        path, separator, fragment = link[2].partition("#")
        if path.removeprefix("./") == "runtime-acceptance.md" and separator:
            references.add(unquote(fragment))

    anchors = set()
    fence = ""
    for line in evidence.splitlines():
        marker = re.match(r"^ {0,3}(`{3,}|~{3,})(.*)$", line)
        if fence:
            if marker and marker[1].startswith(fence) and not marker[2].strip():
                fence = ""
            continue
        if marker:
            fence = marker[1]
            continue
        heading = re.match(r"^ {0,3}#{1,6}[ \t]+(.+)$", line)
        if not heading:
            continue
        title = re.sub(r"[ \t]+#+[ \t]*$", "", heading[1]).strip().lower()
        anchor = re.sub(r"[^\w\s-]", "", title)
        anchor = re.sub(r"\s", "-", anchor)
        if anchor in anchors:
            raise ValueError(f"duplicate runtime acceptance anchor: {anchor}")
        anchors.add(anchor)
    if missing := references - anchors:
        raise ValueError(f"runtime acceptance missing heading anchors {sorted(missing)}")
    return len(references)


def check():
    pages = [p for f in (ROOT / "catalog").glob("*.json") for p in json.loads(f.read_text())]
    states = {p["id"] + "#" + s["id"]: s for p in pages for s in p["states"]}
    ledger = (ROOT / "runtime-states.md").read_text()
    page_count, state_count, observed, pending = check_runtime_coverage(pages, ledger)
    print(f"Runtime ledger covers {page_count} pages and {state_count} states ({observed} observed, {pending} awaiting evidence).")
    anchors = check_evidence_anchors(ledger, (ROOT / "runtime-acceptance.md").read_text())
    print(f"Validated {anchors} distinct runtime acceptance heading references.")

    def targets(key, edge_type):
        return {canonical(target) for _, target in states[canonical(key)].get(edge_type, [])}

    def edge(source, edge_type, dest):
        assert canonical(dest) in targets(source, edge_type), (source, edge_type, dest)

    def only_queries(source, allowed):
        actual = targets(source, "actions")
        assert actual <= {canonical(i) for i in allowed}, (source, actual, allowed)

    def fixture(source, expected):
        text = json.dumps(states[canonical(source)], ensure_ascii=False)
        assert all(value in text for value in expected), (source, expected)

    # Accessibility exercises end in the same practice decision as the tutorial.
    reader = states["W27#reader"]
    assert [label for label, _ in reader["rows"]] == [
        "요청 출처", "금액", "받는 곳", "체인", "수수료", "확인 방법", "마지막 행동",
    ], "W27 practice reading order"
    edge("W27#reader", "actions", "W27#reader")
    edge("W27#reader", "actions", "W02#reject")
    edge("W27#font200", "actions", "W02#reject")
    edge("W02#reject", "actions", "W02#practice")
    edge("W27#notifyoff", "actions", "W09")

    # A return review is distinct from the ledger inquiry and from money movement.
    edge("O09#return", "actions", "O15")
    edge("O09#returnReview", "actions", "O09#return")
    edge("O09#returnReview", "actions", "O09#returnPending")

    # Board approval, transaction finality and pickup notifications have distinct authorities.
    edge("W10", "events", "W11")
    only_queries("W10", ["W10#details", "W10#cancel"])
    edge("W11", "events", "W11#paid")
    edge("W11#paid", "events", "W11#pickup")
    edge("K05#ready", "events", "W11#pickup")
    edge("W11#pickup", "events", "W11#collected")
    fixture("W10", ["S-0142", "6.50", "8283"])
    fixture("X01", ["S-0142", "6.50", "{{address:0x62c9…8e41}}"])

    # Independent phone order and a repriced order cannot inherit the original payer approval.
    edge("K10#request", "events", "W10#bleReview")
    fixture("W10#bleReview", ["S-0151", "8.00"])
    # A confirmed checkout claim can exist before the selected phone requests a quote.
    assert targets("K08#binding", "events") == {"K08#linked", "K08#unknown"}
    edge("K08#unknown", "events", "K08#linked")
    edge("K08#linked", "events", "W10")
    edge("K08#repriced", "actions", "K08#repriceBinding")
    fixture("K08#repriced", ["S-0144", "7.00"])

    # A device decision alone does not submit either leg of a booking.
    for source, review, pending in [
        ("T04#board", "T04#start", "T05#pending"),
        ("T05#balance", "T05#balance", "T05#balance_pending"),
    ]:
        assert pending not in targets(source, "actions"), source
        outcomes = states[source]["events"]
        assert any("거절" in label and target == review for label, target in outcomes), source
        assert [target for label, target in outcomes if "명시적 제출" in label] == [pending], source
        assert all("명시적 제출" in label for label, target in outcomes if target == pending), source
    only_queries("T05#pending", ["T05#pending", "T01"])

    # Expiry is observed at the chain boundary, and reservation refunds have distinct principals.
    only_queries("W19#reconcile", ["W19#reconcile"])
    edge("W19#reconcile", "events", "W19#expired")
    edge("W19#reconcile", "events", "W19#paid")
    edge("T07#paid_pending", "events", "T07#paid_partial")
    edge("T07#paid_partial", "events", "T07#paid_done")
    fixture("T07#paid_partial", ["10", "20", "확인 중"])

    # Submission is not durable reservation, and operational requests do not certify their own result.
    edge("O05#review", "actions", "O05#submitPending")
    only_queries("O05#submitPending", ["O05#submitPending"])
    edge("O05#submitPending", "events", "O05#submitted")
    edge("X05#matched", "actions", "X05#switching")
    only_queries("X05#switching", ["X05#switching"])
    edge("X05#switching", "events", "X05#switched")
    only_queries("D06#await", ["D06#await"])
    edge("D06#await", "events", "D06#done")
    edge("L06#auth", "events", "L06#denied")
    edge("L06#auth", "events", "L06")
    assert "L06#start" not in targets("L01", "actions")

    # Prebuilt QR assets and the corresponding review page must use the same mock address.
    for state, file in [("qr","evm"),("ethshare","evm"),("baseshare","evm"),
                        ("btcshare","btc"),("solshare","sol")]:
        value = states["W07#"+state]["rows"][0][1]
        svg = ET.parse(ROOT / f"receive-{file}.svg")
        assert svg.find("{http://www.w3.org/2000/svg}title").text == value, state
    # A receiving address with a required XRP destination tag is displayed as an address/tag pair.
    fixture("W07#xrpshare", ["DestinationTag", "142"])
    edge("W07#xrpshare", "actions", "W07#xrpcard")
    edge("W07#xrpcard", "actions", "W07#xrpsheet")
    edge("W07#xrpsheet", "actions", "W07#xrpcard")
    tag_card = dict(states["W07#xrpcard"]["rows"])
    payload = ET.parse(ROOT / "receive-xrp.svg").find("{http://www.w3.org/2000/svg}title").text
    assert payload == f"{tag_card['네트워크']}\nAddress: {tag_card['주소']}\nDestinationTag: {tag_card['DestinationTag']}"
    print("Passed reviewed wallet, concurrency, expiry, refund, operations, permissions and QR fixture journeys.")


if __name__ == "__main__":
    check()
