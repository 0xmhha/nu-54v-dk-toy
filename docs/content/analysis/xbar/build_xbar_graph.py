#!/usr/bin/env python3
"""Build the X-bar document graph from records/*.jsonl.

Usage:
    python3 docs/content/analysis/xbar/build_xbar_graph.py

Inputs:  records/*.jsonl (one X-bar record per document, see METHOD.md)
Outputs: graph.json     nodes and typed edges
         findings.json  broken or stale complements, claim conflicts,
                        orphans, open questions by product
         FINDINGS.md    human-readable summary with per-product Mermaid graphs
         graph.html     interactive force-directed view (d3 from cdnjs)
"""

import collections
import json
import re
from pathlib import Path


HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
RECORDS = HERE / "records"
PRODUCTS = [f"P{n:02d}" for n in range(1, 11)]
PRODUCT_NAMES = {
    "P01": "NU-54V-DK firmware", "P02": "User app", "P03": "Cloud MPC wallet",
    "P04": "Merchant kiosk", "P05": "Operations backoffice",
    "P06": "StableNet contracts", "P07": "Indexer", "P08": "Market services",
    "P09": "Travel AI", "P10": "Platform",
}
LIVE = {"current", "draft"}


def load_records():
    records = []
    for path in sorted(RECORDS.glob("*.jsonl")):
        for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
            if line.strip():
                record = json.loads(line)
                record["_source"] = f"{path.name}:{number}"
                records.append(record)
    return records


def norm_target(target):
    target = (target or "").strip()
    if not target or target.startswith("concept:"):
        return target
    return target.split("#")[0].lstrip("./")


def norm_value(value):
    return re.sub(r"\s+", " ", str(value)).strip().lower()


def build(records):
    # A later batch may refresh an earlier record for the same path; keep the last one.
    records = list({r["path"]: r for r in records}.values())
    by_path = {r["path"]: r for r in records}
    nodes, edges, broken = {}, [], []

    for product in PRODUCTS:
        nodes[product] = {"id": product, "kind": "product", "label": f"{product} {PRODUCT_NAMES[product]}"}

    for r in records:
        spec, head = r.get("specifier", {}), r.get("head", {})
        nodes[r["path"]] = {
            "id": r["path"], "kind": "document", "label": r.get("title") or r["path"],
            "type": head.get("type"), "proposition": head.get("proposition"),
            "authority": spec.get("authority"), "products": spec.get("products", []),
            "as_of": spec.get("as_of"),
        }

    def link(source, target, relation, note=None):
        target = norm_target(target)
        if not target:
            return
        if target.startswith("concept:"):
            nodes.setdefault(target, {"id": target, "kind": "concept", "label": target[8:]})
        elif target not in nodes:
            if (ROOT / target).exists():
                nodes[target] = {"id": target, "kind": "artifact", "label": Path(target).name}
            else:
                broken.append({"source": source, "target": target, "relation": relation})
                return
        edges.append({"source": source, "target": target, "relation": relation, "note": note})

    for r in records:
        spec = r.get("specifier", {})
        for c in r.get("complements", []):
            link(r["path"], c.get("target"), "requires", c.get("role"))
        for a in r.get("adjuncts", []):
            link(r["path"], a.get("target"), "elaborates", a.get("kind"))
        for newer in spec.get("superseded_by", []):
            link(newer, r["path"], "supersedes")
        for product in spec.get("products", []):
            targets = PRODUCTS if product == "ALL" else [product]
            for p in targets:
                if p in nodes:
                    edges.append({"source": r["path"], "target": p, "relation": "scopes", "note": None})

    return by_path, nodes, edges, broken


def analyse(by_path, nodes, edges, broken):
    stale = []
    for e in edges:
        if e["relation"] != "requires":
            continue
        src, dst = by_path.get(e["source"]), by_path.get(e["target"])
        if src and dst and src["specifier"].get("authority") in LIVE \
                and dst["specifier"].get("authority") not in LIVE | {"reference"}:
            stale.append({"source": e["source"], "target": e["target"],
                          "target_authority": dst["specifier"].get("authority")})

    claims = collections.defaultdict(list)
    for path, r in by_path.items():
        if r["specifier"].get("authority") in LIVE:
            for c in r.get("claims", []):
                claims[c.get("subject", "").strip().lower()].append((path, c.get("value")))
    conflicts = []
    for subject, values in sorted(claims.items()):
        distinct = {norm_value(v) for _, v in values if v not in (None, "")}
        if subject and len(distinct) > 1:
            conflicts.append({"subject": subject, "values": [{"path": p, "value": v} for p, v in values]})

    incoming = collections.Counter(e["target"] for e in edges if e["relation"] in {"requires", "elaborates"})
    orphans = sorted(p for p, r in by_path.items()
                     if r["specifier"].get("authority") in LIVE and incoming[p] == 0)

    questions = collections.defaultdict(list)
    for path, r in by_path.items():
        if r["specifier"].get("authority") not in LIVE:
            continue
        scope = r["specifier"].get("products") or ["CROSS"]
        for q in r.get("open_questions", []):
            for p in scope:
                questions[p].append({"path": path, "question": q})

    authority = collections.Counter(r["specifier"].get("authority") for r in by_path.values())
    head_types = collections.Counter(r["head"].get("type") for r in by_path.values())
    missing = [p for p in expected_paths() if p not in by_path]
    return {
        "counts": {"documents": len(by_path), "nodes": len(nodes), "edges": len(edges),
                   "authority": dict(authority), "head_types": dict(head_types)},
        "missing_records": missing,
        "broken_targets": broken,
        "stale_complements": stale,
        "claim_conflicts": conflicts,
        "orphans": orphans,
        "open_questions": {k: v for k, v in sorted(questions.items())},
    }


def expected_paths():
    paths = ["README.md", "REPOSITORY-CHECKPOINT.md", "CONTRIBUTING.md", "packages/README.md",
             "products/README.md"]
    paths += sorted(str(p.relative_to(ROOT)) for p in ROOT.glob("products/*/README.md"))
    for p in sorted((ROOT / "docs").rglob("*.md")):
        rel = str(p.relative_to(ROOT))
        if rel.startswith("docs/design-history/") or "/document-logic/before-" in rel:
            continue
        if rel.startswith("docs/content/analysis/xbar/"):
            continue
        paths.append(rel)
    return [p for p in dict.fromkeys(paths) if (ROOT / p).exists()]


def mermaid_for(product, by_path, edges):
    docs = [p for p, r in by_path.items()
            if product in (r["specifier"].get("products") or []) and r["specifier"].get("authority") in LIVE]
    if not docs:
        return None
    ids = {p: f"d{i}" for i, p in enumerate(docs)}
    lines = ["```mermaid", "graph LR", f'  {product}(["{product} {PRODUCT_NAMES[product]}"])']
    for p in docs:
        r = by_path[p]
        label = f'{r["head"].get("type", "?")}: {Path(p).stem}'
        lines.append(f'  {ids[p]}["{label}"] --> {product}')
    for e in edges:
        if e["relation"] == "requires" and e["source"] in ids and e["target"] in ids:
            lines.append(f'  {ids[e["source"]]} -. requires .-> {ids[e["target"]]}')
    lines.append("```")
    return "\n".join(lines)


def write_markdown(findings, by_path, edges):
    c = findings["counts"]
    out = ["# X-bar document graph findings", "",
           "Generated by `build_xbar_graph.py`; do not edit by hand. Method: [METHOD.md](METHOD.md).", "",
           f"- Documents: {c['documents']} · nodes: {c['nodes']} · edges: {c['edges']}",
           f"- Authority: {', '.join(f'{k}={v}' for k, v in sorted(c['authority'].items(), key=str))}",
           f"- Head types: {', '.join(f'{k}={v}' for k, v in sorted(c['head_types'].items(), key=str))}",
           f"- Missing records: {len(findings['missing_records'])} · broken targets: "
           f"{len(findings['broken_targets'])} · stale complements: {len(findings['stale_complements'])} · "
           f"claim conflicts: {len(findings['claim_conflicts'])} · orphans: {len(findings['orphans'])}", ""]

    out += ["## Claim conflicts among live documents", ""]
    for conflict in findings["claim_conflicts"]:
        out.append(f"- `{conflict['subject']}`")
        for v in conflict["values"]:
            out.append(f"  - {v['value']} — `{v['path']}`")
    out += ["", "## Live documents that require superseded or withdrawn documents", ""]
    for s in findings["stale_complements"]:
        out.append(f"- `{s['source']}` requires `{s['target']}` ({s['target_authority']})")
    out += ["", "## Broken targets", ""]
    for b in findings["broken_targets"]:
        out.append(f"- `{b['source']}` {b['relation']} `{b['target']}`")
    out += ["", "## Live documents nothing depends on", ""]
    out += [f"- `{p}`" for p in findings["orphans"]]
    out += ["", "## Open questions by product", ""]
    for product, items in findings["open_questions"].items():
        out.append(f"### {product}")
        out += [f"- {q['question']} — `{q['path']}`" for q in items]
        out.append("")
    out += ["## Product graphs (live documents)", ""]
    for product in PRODUCTS:
        block = mermaid_for(product, by_path, edges)
        if block:
            out += [f"### {product} {PRODUCT_NAMES[product]}", "", block, ""]
    (HERE / "FINDINGS.md").write_text("\n".join(out) + "\n", encoding="utf-8")


HTML = """<!doctype html><meta charset="utf-8"><title>X-bar document graph</title>
<style>body{margin:0;font:13px system-ui,sans-serif}#info{position:fixed;top:8px;left:8px;max-width:420px;
background:#fff;border:1px solid #ccc;padding:8px;border-radius:6px}svg{width:100vw;height:100vh}</style>
<div id="info">Drag nodes; hover for the head proposition. Edge colors: requires=red, elaborates=grey,
scopes=blue, supersedes=orange.</div><svg></svg>
<script src="https://cdnjs.cloudflare.com/ajax/libs/d3/7.9.0/d3.min.js"></script>
<script>const G=__GRAPH__;
const color={product:"#1f4fbf",document:"#2a9d8f",concept:"#999",artifact:"#bbb"};
const ecolor={requires:"#d62828",elaborates:"#ccc",scopes:"#90b4ff",supersedes:"#f4a261"};
const svg=d3.select("svg"),g=svg.append("g");svg.call(d3.zoom().on("zoom",e=>g.attr("transform",e.transform)));
const links=G.edges.map(e=>({...e})),nodes=G.nodes.map(n=>({...n}));
const sim=d3.forceSimulation(nodes).force("link",d3.forceLink(links).id(d=>d.id).distance(60))
.force("charge",d3.forceManyBody().strength(-60)).force("center",d3.forceCenter(innerWidth/2,innerHeight/2));
const l=g.selectAll("line").data(links).join("line").attr("stroke",d=>ecolor[d.relation]);
const n=g.selectAll("circle").data(nodes).join("circle").attr("r",d=>d.kind==="product"?10:5)
.attr("fill",d=>d.authority==="superseded"||d.authority==="withdrawn"?"#e9c46a":color[d.kind])
.call(d3.drag().on("start",(e,d)=>{if(!e.active)sim.alphaTarget(.3).restart();d.fx=d.x;d.fy=d.y})
.on("drag",(e,d)=>{d.fx=e.x;d.fy=e.y}).on("end",(e,d)=>{if(!e.active)sim.alphaTarget(0);d.fx=null;d.fy=null}));
n.append("title").text(d=>d.label+(d.proposition?"\\n"+d.proposition:"")+(d.authority?"\\n["+d.authority+"]":""));
sim.on("tick",()=>{l.attr("x1",d=>d.source.x).attr("y1",d=>d.source.y).attr("x2",d=>d.target.x).attr("y2",d=>d.target.y);
n.attr("cx",d=>d.x).attr("cy",d=>d.y)});</script>
"""


def main():
    records = load_records()
    by_path, nodes, edges, broken = build(records)
    findings = analyse(by_path, nodes, edges, broken)
    graph = {"method": "METHOD.md", "nodes": list(nodes.values()), "edges": edges}
    (HERE / "graph.json").write_text(json.dumps(graph, ensure_ascii=False, indent=1) + "\n", encoding="utf-8")
    (HERE / "findings.json").write_text(json.dumps(findings, ensure_ascii=False, indent=1) + "\n", encoding="utf-8")
    write_markdown(findings, by_path, edges)
    (HERE / "graph.html").write_text(HTML.replace("__GRAPH__", json.dumps(graph, ensure_ascii=False)), encoding="utf-8")
    print(json.dumps({k: (len(v) if isinstance(v, (list, dict)) else v)
                      for k, v in findings.items() if k != "counts"} | findings["counts"], ensure_ascii=False))


if __name__ == "__main__":
    main()
