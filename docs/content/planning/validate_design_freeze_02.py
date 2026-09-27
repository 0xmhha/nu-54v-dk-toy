#!/usr/bin/env python3
"""Validate DF-20260925-02 and every document it governs.

Facts come from the register (design-freeze-checkpoint-02.json) and the protocol
schema, never from this script. Every check group reads the committed tree at
HEAD and refuses to run on a dirty worktree, so a pass always describes a commit.

Usage:
    validate_design_freeze_02.py --check <group>   # print "OK <group>" or "FAIL <assertion>: why"
    validate_design_freeze_02.py --check all
    validate_design_freeze_02.py --self-test       # every assertion must reject its known-bad fixture
    validate_design_freeze_02.py --list            # groups and assertion names
    --allow-dirty  read the working tree instead of HEAD (authoring aid; prints DEV-PASS, never gate evidence)

Groups: register, precedence+banners, products, protocol+vectors, wbs,
acceptance, conflicts, redaction.

Fixtures: docs/content/planning/fixtures/df02/<group>/<assertion>/fixture.json
    {"assertion": "<name>", "ops": [{"op": "json_set", "path": ..., "pointer": "/a/0/b", "value": ...},
                                     {"op": "replace", "path": ..., "old": ..., "new": ...},
                                     {"op": "write", "path": ..., "content": ...},
                                     {"op": "delete", "path": ...}]}
"""

from __future__ import annotations

import csv
import hashlib
import io
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Callable

ROOT = Path(__file__).resolve().parents[3]
REGISTER = "docs/content/planning/design-freeze-checkpoint-02.json"
RENDERED = "docs/content/planning/design-freeze-checkpoint-02.md"
BUILD = "docs/content/planning/build_design_freeze_02.py"
DF01_BUILD = "docs/content/planning/build_design_freeze.py"
DF01_REGISTER = "docs/content/planning/design-freeze-checkpoint.json"
PROTOCOL_DIR = "docs/content/specifications/protocol"
PROTOCOL_MD = f"{PROTOCOL_DIR}/payment-protocol.md"
SCHEMA = f"{PROTOCOL_DIR}/payment-protocol.schema.json"
VECTORS = f"{PROTOCOL_DIR}/eip712-vectors.json"
ACCEPTANCE = "docs/content/acceptance/week12-log.md"
WBS_CSV = "docs/content/planning/product-worklist-and-12week-wbs-02.csv"
WBS_MD = "docs/content/planning/product-worklist-and-12week-wbs-02.md"
FIXTURES = "docs/content/planning/fixtures/df02"
DIRTY_SCOPE = ["docs/content", "REPOSITORY-CHECKPOINT.md", "docs/README.md", "products"]
DOC_KINDS = ["plan", "srs", "use-cases", "design"]
CITATION = re.compile(r"\[(D\d\d|RR-DEC-01|N\d\d)\]")
HEX_LEAK = re.compile(r"(?<![0-9a-fA-F])0x(?:[0-9a-fA-F]{64}|[0-9a-fA-F]{40})(?![0-9a-fA-F])")
BARE_SHA256 = re.compile(r"(?<![0-9a-fA-Fx:])[0-9a-f]{64}(?![0-9a-fA-F])")


class Failure(Exception):
    def __init__(self, assertion: str, message: str):
        super().__init__(f"{assertion}: {message}")
        self.assertion = assertion


def git(*args: str) -> str:
    return subprocess.run(["git", *args], cwd=ROOT, capture_output=True, text=True, check=True).stdout


# ---------------------------------------------------------------- tree access


class Tree:
    """Read-only view of HEAD (default) or the working tree, with an optional fixture overlay."""

    def __init__(self, worktree: bool, overlay: dict[str, bytes | None] | None = None):
        self.worktree = worktree
        self.overlay = overlay or {}
        self._files: set[str] | None = None

    def with_overlay(self, overlay: dict[str, bytes | None]) -> "Tree":
        return Tree(self.worktree, {**self.overlay, **overlay})

    def files(self) -> set[str]:
        if self._files is None:
            if self.worktree:
                listed = git("ls-files").split("\n") + git("ls-files", "--others", "--exclude-standard").split("\n")
                base = {f for f in listed if f and (ROOT / f).is_file()}
            else:
                base = set(filter(None, git("ls-tree", "-r", "--name-only", "HEAD").split("\n")))
            for path, data in self.overlay.items():
                (base.add if data is not None else base.discard)(path)
            self._files = base
        return self._files

    def exists(self, path: str) -> bool:
        return path in self.files()

    def dir_exists(self, prefix: str) -> bool:
        prefix = prefix.rstrip("/") + "/"
        return any(f.startswith(prefix) for f in self.files())

    def read_bytes(self, path: str) -> bytes:
        if path in self.overlay:
            data = self.overlay[path]
            if data is None:
                raise FileNotFoundError(path)
            return data
        if self.worktree:
            return (ROOT / path).read_bytes()
        return subprocess.run(["git", "show", f"HEAD:{path}"], cwd=ROOT, capture_output=True, check=True).stdout

    def read(self, path: str) -> str:
        return self.read_bytes(path).decode("utf-8")

    def json(self, path: str):
        return json.loads(self.read(path))

    def changed_since(self, base: str) -> set[str]:
        changed = set(filter(None, git("diff", "--name-only", base, "HEAD").split("\n")))
        if self.worktree:
            changed |= set(filter(None, git("diff", "--name-only", "HEAD").split("\n")))
            changed |= set(filter(None, git("ls-files", "--others", "--exclude-standard").split("\n")))
        changed |= set(self.overlay)
        return {f for f in changed if self.exists(f)}


def dirty_paths() -> list[str]:
    return [l for l in git("status", "--porcelain", "--", *DIRTY_SCOPE).split("\n") if l]


# ---------------------------------------------------------------- helpers


def require(cond: bool, assertion: str, message: str) -> None:
    if not cond:
        raise Failure(assertion, message)


def load_register(t: Tree) -> dict:
    return t.json(REGISTER)


def exempt_paths(t: Tree, reg: dict) -> list[str]:
    """DF-01 generated artifacts and inputs (derived from the DF-01 generator) plus the fixed list."""
    derived = {DF01_BUILD, "docs/content/planning/validate_design_freeze.py"}
    for rel in re.findall(r'ROOT / "(content/[^"]+)"', t.read(DF01_BUILD)):
        derived.add(f"docs/{rel}")
    for rel in t.json(DF01_REGISTER)["sourceHashes"]:
        derived.add(f"docs/{rel}")
    return sorted(derived) + list(reg["freezeScope"]["exemptPathsFixed"])


def is_under(path: str, prefixes: list[str]) -> bool:
    return any(path == p or (p.endswith("/") and path.startswith(p)) for p in prefixes)


def product_doc(pid: str, kind: str) -> str:
    return f"docs/content/products/{pid.lower()}/{kind}.md"


def product_readme(t: Tree, pid: str) -> str:
    prefix = f"products/{pid.lower()}-"
    for f in sorted(t.files()):
        if f.startswith(prefix) and f.count("/") == 2 and f.endswith("/README.md"):
            return f
    raise Failure("products.readme", f"no products/{pid.lower()}-*/README.md")


def rendered_from(t: Tree) -> str:
    """Run the committed renderer against the tree's register without touching disk."""
    namespace: dict = {"__name__": "df02_build", "__file__": str(ROOT / BUILD)}
    exec(compile(t.read(BUILD), BUILD, "exec"), namespace)
    return namespace["render"](load_register(t))


# ---------------------------------------------------------------- group: register

EXPECTED_DF01 = [f"D{i:02}" for i in range(1, 20)] + ["RR-DEC-01"]
PARAMETER_KEYS = ["perPaymentCap", "dailyCap", "withdrawalDelay", "payoutChangeDelay",
                  "authorizationExpiry", "attestationValidity", "kioskMinGasBalance",
                  "pinMaxRetries", "anchorClockSkew"]


def reg_parse(t: Tree) -> None:
    try:
        reg = load_register(t)
    except (FileNotFoundError, json.JSONDecodeError, subprocess.CalledProcessError) as exc:
        raise Failure("register.parse", f"cannot read {REGISTER}: {exc}")
    require(reg.get("checkpointId") == "DF-20260925-02", "register.parse", "checkpointId must be DF-20260925-02")
    require(reg.get("supersedes") == "DF-20260920-01", "register.parse", "supersedes must be DF-20260920-01")
    require(re.fullmatch(r"[0-9a-f]{40}", reg.get("freezeScope", {}).get("baseCommit", "")) is not None,
            "register.parse", "freezeScope.baseCommit must be a full commit id")


def reg_dispositions(t: Tree) -> None:
    reg = load_register(t)
    ids = [d["id"] for d in reg["dispositions"]]
    require(ids == EXPECTED_DF01, "register.dispositions", f"expected {EXPECTED_DF01}, got {ids}")
    for d in reg["dispositions"]:
        require(d.get("disposition") in {"kept", "amended", "withdrawn"}, "register.dispositions", f"{d['id']} disposition")
        for field in ("old", "new", "reason"):
            require(bool(str(d.get(field, "")).strip()), "register.dispositions", f"{d['id']} missing {field}")
        require(bool(d.get("affectedFiles")), "register.dispositions", f"{d['id']} has no affectedFiles")
        for f in d["affectedFiles"]:
            require(t.exists(f), "register.dispositions", f"{d['id']} affectedFile missing: {f}")


def reg_new_decisions(t: Tree) -> None:
    reg = load_register(t)
    ids = [d["id"] for d in reg["newDecisions"]]
    require(ids == [f"N{i:02}" for i in range(1, len(ids) + 1)] and ids, "register.new_decisions",
            f"new decision ids must run N01..Nxx without gaps, got {ids}")
    for d in reg["newDecisions"]:
        require(bool(d.get("value", "").strip()), "register.new_decisions", f"{d['id']} has no value")
        require(bool(d.get("requiredIn")) and bool(d.get("mustContain")), "register.new_decisions",
                f"{d['id']} needs requiredIn and mustContain")
        require(isinstance(d.get("resolves"), list), "register.new_decisions", f"{d['id']} resolves must be a list")


def reg_required_in(t: Tree) -> None:
    reg = load_register(t)
    for d in reg["newDecisions"]:
        for path in d["requiredIn"]:
            require(t.exists(path), "register.required_in", f"{d['id']} requiredIn missing: {path}")
            text = t.read(path)
            require(f"[{d['id']}]" in text, "register.required_in", f"{path} does not cite [{d['id']}]")
            for token in d["mustContain"]:
                require(token in text, "register.required_in", f"{path} lacks '{token}' required by {d['id']}")


def reg_citations_resolve(t: Tree) -> None:
    reg = load_register(t)
    known = {d["id"] for d in reg["dispositions"]} | {d["id"] for d in reg["newDecisions"]}
    for f in sorted(t.files()):
        if f.endswith(".md") and is_under(f, reg["citationScope"]):
            for cited in CITATION.findall(t.read(f)):
                require(cited in known, "register.citations_resolve", f"{f} cites unknown [{cited}]")


def reg_product_map(t: Tree) -> None:
    reg = load_register(t)
    df01 = [p["id"] for p in t.json(DF01_REGISTER)["productDesigns"]]
    mapping = {m["df01"]: m for m in reg["productIdMap"]}
    require(sorted(mapping) == sorted(df01), "register.product_map", "every DF-01 product id must be mapped once")
    require(sorted(m["product"] for m in mapping.values()) == [f"P{i:02}" for i in range(1, 11)],
            "register.product_map", "mapping must cover P01..P10 one-to-one")
    for b, m in mapping.items():
        require(m["product"] == "P" + b.split("-")[1], "register.product_map", f"{b} must map to P{b[2:]}")
        expected = "buildable" if m["product"] in reg["buildableProducts"] else "out_of_cycle"
        require(m["status"] == expected, "register.product_map", f"{b} status {m['status']} != {expected}")
    require(set(reg["buildableProducts"]).isdisjoint(reg["outOfCycleProducts"]) and
            len(reg["buildableProducts"]) + len(reg["outOfCycleProducts"]) == 10,
            "register.product_map", "buildable and out-of-cycle lists must partition P01..P10")


def reg_parameters(t: Tree) -> None:
    params = load_register(t)["parameters"]
    a = "register.parameters"
    require(list(params) == PARAMETER_KEYS, a, f"parameters must be exactly {PARAMETER_KEYS}")
    for k, p in params.items():
        require(isinstance(p.get("value"), (int, float)) and p["value"] > 0, a, f"{k} value must be > 0")
        require(bool(p.get("unit")), a, f"{k} needs a unit")
        require(bool(p.get("rationale", "").strip()), a, f"{k} needs a rationale")
    v = {k: p["value"] for k, p in params.items()}
    require(params["withdrawalDelay"]["unit"] == params["authorizationExpiry"]["unit"] == "s", a, "delays are seconds")
    require(v["perPaymentCap"] <= v["dailyCap"], a, "perPaymentCap must be <= dailyCap")
    require(params["perPaymentCap"]["unit"] == params["dailyCap"]["unit"], a, "caps share a unit")
    require(30 <= v["authorizationExpiry"] <= 300, a, "authorizationExpiry must be 30..300 s")
    require(600 <= v["withdrawalDelay"] <= 604800, a, "withdrawalDelay must be 600..604800 s")
    require(v["withdrawalDelay"] >= v["authorizationExpiry"], a, "withdrawalDelay must be >= authorizationExpiry")
    require(v["payoutChangeDelay"] >= v["attestationValidity"], a, "payoutChangeDelay must be >= attestationValidity")
    require(3 <= v["pinMaxRetries"] <= 10, a, "pinMaxRetries must be 3..10")
    require(0 < v["anchorClockSkew"] <= 300, a, "anchorClockSkew must be 1..300 s")
    require(v["attestationValidity"] == 86400 and params["attestationValidity"]["unit"] == "s", a,
            "attestationValidity is fixed at 86400 s")
    prov = params["kioskMinGasBalance"].get("provenance", {})
    require(prov.get("kind") == "estimate" and prov.get("remeasureAt") and prov.get("gasPriceGwei"), a,
            "kioskMinGasBalance must carry its estimate provenance and a re-measurement week")


def reg_rendered(t: Tree) -> None:
    require(t.exists(RENDERED), "register.rendered", f"{RENDERED} missing")
    require(rendered_from(t) == t.read(RENDERED), "register.rendered",
            "rendered md differs from build_design_freeze_02.py output; re-run the build")


# ---------------------------------------------------------------- group: precedence+banners


def prec_reading_order(t: Tree) -> None:
    reg = load_register(t)
    for p in reg["precedence"]:
        text = t.read(p["file"])
        new_pos = text.find("design-freeze-checkpoint-02.md")
        old_pos = text.find("design-freeze-checkpoint.md")
        require(new_pos >= 0 and p["governing"] in text, "precedence.reading_order",
                f"{p['file']} does not list {p['governing']}")
        require(old_pos < 0 or new_pos < old_pos, "precedence.reading_order",
                f"{p['file']} lists {p['before']} before {p['governing']}")


def banner_first_line(t: Tree) -> None:
    for b in load_register(t)["bannerRecord"]:
        first = t.read(b["path"]).split("\n", 1)[0]
        require(first == b["bannerLine"], "banners.first_line", f"{b['path']} first line is not its bannerLine")
        require("DF-20260925-02" in first, "banners.first_line", f"{b['path']} banner must name DF-20260925-02")


def banner_remainder_hash(t: Tree) -> None:
    for b in load_register(t)["bannerRecord"]:
        data = t.read_bytes(b["path"])
        rest = data.split(b"\n", 1)[1] if b"\n" in data else b""
        actual = "sha256:" + hashlib.sha256(rest).hexdigest()
        require(actual == b["baseSha256"], "banners.remainder_hash", f"{b['path']} body changed since baseCommit")


def stale_literals(t: Tree) -> None:
    reg = load_register(t)
    scope = reg["freezeScope"]
    exempt = exempt_paths(t, reg)
    bannered = {b["path"] for b in reg["bannerRecord"]}
    patterns = [(s["id"], re.compile(s["regex"])) for s in scope["staleLiterals"]]
    for f in sorted(t.changed_since(scope["baseCommit"])):
        if not (f.startswith("docs/content/") and f.endswith(".md")) or f in bannered or is_under(f, exempt):
            continue
        for n, line in enumerate(t.read(f).split("\n"), 1):
            for sid, rx in patterns:
                require(not rx.search(line), "stale.literals", f"{f}:{n} matches {sid}")


# ---------------------------------------------------------------- group: products


def products_docs_present(t: Tree) -> None:
    for pid in load_register(t)["buildableProducts"]:
        for kind in DOC_KINDS:
            path = product_doc(pid, kind)
            require(t.exists(path), "products.docs_present", f"missing {path}")
            require(len(t.read_bytes(path)) >= 400, "products.docs_present", f"{path} is under 400 bytes")


def products_cite_ids(t: Tree) -> None:
    for pid in load_register(t)["buildableProducts"]:
        for kind in DOC_KINDS:
            path = product_doc(pid, kind)
            require(bool(CITATION.search(t.read(path))), "products.cite_ids", f"{path} cites no decision id")


def products_readme_links(t: Tree) -> None:
    for pid in load_register(t)["buildableProducts"]:
        readme = product_readme(t, pid)
        text = t.read(readme)
        for kind in DOC_KINDS:
            target = os.path.relpath(product_doc(pid, kind), str(Path(readme).parent))
            require(f"]({target})" in text, "products.readme_links", f"{readme} does not link {target}")


def products_out_of_cycle(t: Tree) -> None:
    for pid in load_register(t)["outOfCycleProducts"]:
        require(not t.dir_exists(f"docs/content/products/{pid.lower()}"), "products.out_of_cycle",
                f"{pid} is out of cycle but has a docs directory")
        require("DF-20260925-02" in t.read(product_readme(t, pid)), "products.out_of_cycle",
                f"{pid} README must name DF-20260925-02")


# ---------------------------------------------------------------- group: protocol+vectors

PAYMENT_FIELDS = ["chainId", "contract", "merchant", "payout", "token", "amount", "orderId", "nonce", "expiry"]


def proto_schema_types(t: Tree) -> None:
    schema = t.json(SCHEMA)
    types = schema.get("eip712Types", {})
    require(sorted(types) == ["LimitChange", "PaymentAuthorization"], "protocol.schema_types",
            f"eip712Types must hold exactly PaymentAuthorization and LimitChange, got {sorted(types)}")


def proto_payment_fields(t: Tree) -> None:
    fields = [f["name"] for f in t.json(SCHEMA)["eip712Types"]["PaymentAuthorization"]]
    require(fields == PAYMENT_FIELDS, "protocol.payment_fields", f"PaymentAuthorization fields {fields}")


def proto_vectors(t: Tree) -> None:
    types = t.json(SCHEMA)["eip712Types"]
    vectors = t.json(VECTORS)["vectors"]
    for name, fields in types.items():
        mine = [v for v in vectors if v.get("primaryType") == name]
        require(bool(mine), "protocol.vectors", f"no vector for {name}")
        for v in mine:
            require(sorted(v["message"]) == sorted(f["name"] for f in fields), "protocol.vectors",
                    f"vector {v.get('id')} keys differ from schema {name}")
            require(re.fullmatch(r"0x[0-9a-f]{64}", v.get("digest", "")) is not None, "protocol.vectors",
                    f"vector {v.get('id')} needs a 32-byte digest")


def proto_md_tokens(t: Tree) -> None:
    text = t.read(PROTOCOL_MD)
    for token in load_register(t)["protocolRequiredTokens"]:
        require(token in text, "protocol.md_tokens", f"payment-protocol.md lacks '{token}'")
    require(any("proximityRef" in l and ("removed" in l or "제거" in l) for l in text.split("\n")),
            "protocol.md_tokens", "proximityRef must be recorded as removed")


# ---------------------------------------------------------------- group: wbs

OWNERS = ["user", "role A", "role B"]


def wbs_rows(t: Tree) -> list[dict]:
    return list(csv.DictReader(io.StringIO(t.read(WBS_CSV))))


def wbs_files(t: Tree) -> None:
    require(t.exists(WBS_CSV) and t.exists(WBS_MD), "wbs.files", "WBS-02 csv and md must both exist")
    require(bool(wbs_rows(t)), "wbs.files", "WBS-02 csv has no rows")


def wbs_products(t: Tree) -> None:
    got = {r["product"] for r in wbs_rows(t)}
    want = set(load_register(t)["buildableProducts"])
    require(got == want, "wbs.products", f"WBS-02 products {sorted(got)} != buildable {sorted(want)}")


def wbs_owners(t: Tree) -> None:
    for r in wbs_rows(t):
        require(r["owner"] in OWNERS, "wbs.owners", f"{r['id']} owner '{r['owner']}' not one of {OWNERS}")


def wbs_capacity(t: Tree) -> None:
    available: dict[int, dict[str, float]] = {}
    for line in t.read(WBS_MD).split("\n"):
        m = re.match(r"\|\s*W(\d+)\s*\|\s*([\d.]+)\s*\|\s*([\d.]+)\s*\|\s*([\d.]+)\s*\|", line)
        if m:
            available[int(m.group(1))] = dict(zip(OWNERS, map(float, m.group(2, 3, 4))))
    require(sorted(available) == list(range(1, 13)), "wbs.capacity", "capacity table must list W1..W12")
    load: dict[tuple[int, str], float] = {}
    for r in wbs_rows(t):
        start, end, effort = int(r["startWeek"]), int(r["endWeek"]), float(r["effortDays"])
        require(1 <= start <= end <= 12, "wbs.capacity", f"{r['id']} weeks out of range")
        for w in range(start, end + 1):
            load[(w, r["owner"])] = load.get((w, r["owner"]), 0) + effort / (end - start + 1)
    for (w, owner), days in sorted(load.items()):
        require(days <= available[w][owner] + 1e-9, "wbs.capacity",
                f"W{w} {owner} load {days:.2f} d exceeds available {available[w][owner]} d")
    require(available[4]["user"] < available[5]["user"], "wbs.capacity",
            "W4 must be recorded shorter than W5 (Chuseok)")


def wbs_dependencies(t: Tree) -> None:
    rows = {r["id"]: r for r in wbs_rows(t)}
    for r in rows.values():
        for dep in r["dependsOn"].split():
            if dep == "-":
                continue
            require(dep in rows, "wbs.dependencies", f"{r['id']} depends on unknown {dep}")
            require(int(rows[dep]["endWeek"]) <= int(r["startWeek"]), "wbs.dependencies",
                    f"{r['id']} starts W{r['startWeek']} before {dep} ends W{rows[dep]['endWeek']}")


def wbs_gates(t: Tree) -> None:
    for r in wbs_rows(t):
        if r["gate"] != "-":
            gate = int(r["gate"].lstrip("W"))
            require(int(r["endWeek"]) <= gate, "wbs.gates",
                    f"{r['id']} ends W{r['endWeek']} after its gate {r['gate']}")


def wbs_md_tokens(t: Tree) -> None:
    text = t.read(WBS_MD)
    for token in load_register(t)["wbsRequiredTokens"]:
        require(token in text, "wbs.md_tokens", f"WBS-02 md lacks '{token}'")


# ---------------------------------------------------------------- group: acceptance

REASON_CODES = ["ATTESTATION_EXPIRED", "MERCHANT_REVOKED", "OVER_CAP", "NONCE_REPLAYED", "MERCHANT_FORGED"]


def acc_items(t: Tree) -> None:
    text = t.read(ACCEPTANCE)
    for i in range(1, 13):
        require(f"W12-{i:02}" in text, "acceptance.items", f"W12-{i:02} missing")


def acc_reason_codes(t: Tree) -> None:
    text = t.read(ACCEPTANCE)
    codes = load_register(t)["acceptanceRefusalCodes"]
    for code in codes["agreed"] + codes["deviceAdded"]:
        require(f"| {code} |" in text, "acceptance.reason_codes", f"{code} has no refusal demo row")
    require(set(codes["agreed"]) == set(REASON_CODES), "acceptance.reason_codes",
            "the agreed refusal set must stay the five interview codes")


def acc_conditional(t: Tree) -> None:
    lines = [l for l in t.read(ACCEPTANCE).split("\n") if re.search(r"W12-\d\d", l) and "P07" in l]
    require(bool(lines), "acceptance.conditional", "no W12 item for the P07 receipt")
    require(all("conditional" in l and "PaymentSettled" in l for l in lines), "acceptance.conditional",
            "the P07 item must be marked conditional with PaymentSettled substitute evidence")
    for token in load_register(t)["acceptanceRequiredTokens"]:
        require(token in t.read(ACCEPTANCE), "acceptance.conditional", f"week12-log lacks '{token}'")


# ---------------------------------------------------------------- group: conflicts


def conflicts_resolved(t: Tree) -> None:
    reg = load_register(t)
    for f in reg["xbarConflicts"]:
        require(any(f in d["resolves"] for d in reg["newDecisions"]), "conflicts.resolved", f"{f} unresolved")


# ---------------------------------------------------------------- group: redaction


def redaction_exemptions(t: Tree) -> None:
    for e in load_register(t)["freezeScope"]["redactionExemptions"]:
        require(bool(e.get("reason", "").strip()), "redaction.exemptions", f"{e['path']} exemption has no reason")
        require(t.exists(e["path"]), "redaction.exemptions", f"{e['path']} exemption does not exist")


def redaction_scan(t: Tree) -> list[str]:
    reg = load_register(t)
    skip = [e["path"] for e in reg["freezeScope"]["redactionExemptions"]] + list(reg["freezeScope"]["exemptPathsFixed"])
    out = []
    for f in sorted(t.changed_since(reg["freezeScope"]["baseCommit"])):
        if f.startswith("docs/content/") and not is_under(f, skip):
            try:
                out.append((f, t.read(f)))
            except UnicodeDecodeError:
                continue
    return out


def redaction_hex(t: Tree) -> None:
    for f, text in redaction_scan(t):
        m = HEX_LEAK.search(text)
        require(m is None, "redaction.hex", f"{f} contains a full 0x address or hash")


def redaction_sha_prefix(t: Tree) -> None:
    for f, text in redaction_scan(t):
        for m in BARE_SHA256.finditer(text):
            require(text[max(0, m.start() - 7):m.start()] == "sha256:", "redaction.sha_prefix",
                    f"{f} has a 64-hex checksum without the sha256: prefix")


# ---------------------------------------------------------------- registry

GROUPS: dict[str, list[tuple[str, Callable[[Tree], None]]]] = {
    "register": [
        ("register.parse", reg_parse),
        ("register.dispositions", reg_dispositions),
        ("register.new_decisions", reg_new_decisions),
        ("register.product_map", reg_product_map),
        ("register.parameters", reg_parameters),
        ("register.rendered", reg_rendered),
        ("register.required_in", reg_required_in),
        ("register.citations_resolve", reg_citations_resolve),
    ],
    "precedence+banners": [
        ("precedence.reading_order", prec_reading_order),
        ("banners.first_line", banner_first_line),
        ("banners.remainder_hash", banner_remainder_hash),
        ("stale.literals", stale_literals),
    ],
    "products": [
        ("products.docs_present", products_docs_present),
        ("products.cite_ids", products_cite_ids),
        ("products.readme_links", products_readme_links),
        ("products.out_of_cycle", products_out_of_cycle),
    ],
    "protocol+vectors": [
        ("protocol.schema_types", proto_schema_types),
        ("protocol.payment_fields", proto_payment_fields),
        ("protocol.vectors", proto_vectors),
        ("protocol.md_tokens", proto_md_tokens),
    ],
    "wbs": [
        ("wbs.files", wbs_files),
        ("wbs.products", wbs_products),
        ("wbs.owners", wbs_owners),
        ("wbs.capacity", wbs_capacity),
        ("wbs.dependencies", wbs_dependencies),
        ("wbs.gates", wbs_gates),
        ("wbs.md_tokens", wbs_md_tokens),
    ],
    "acceptance": [
        ("acceptance.items", acc_items),
        ("acceptance.reason_codes", acc_reason_codes),
        ("acceptance.conditional", acc_conditional),
    ],
    "conflicts": [("conflicts.resolved", conflicts_resolved)],
    "redaction": [
        ("redaction.exemptions", redaction_exemptions),
        ("redaction.hex", redaction_hex),
        ("redaction.sha_prefix", redaction_sha_prefix),
    ],
}


def run_group(t: Tree, group: str) -> None:
    for name, fn in GROUPS[group]:
        try:
            fn(t)
        except Failure:
            raise
        except (OSError, KeyError, json.JSONDecodeError, subprocess.CalledProcessError, ValueError) as exc:
            raise Failure(name, f"{type(exc).__name__}: {exc}")


# ---------------------------------------------------------------- self-test


def apply_ops(t: Tree, ops: list[dict]) -> dict[str, bytes | None]:
    overlay: dict[str, bytes | None] = {}

    def current(path: str) -> str:
        if path in overlay:
            return (overlay[path] or b"").decode("utf-8")
        return t.read(path)

    for op in ops:
        path = op["path"]
        if op["op"] == "delete":
            overlay[path] = None
        elif op["op"] == "write":
            overlay[path] = op["content"].encode("utf-8")
        elif op["op"] == "replace":
            text = current(path)
            if op["old"] not in text:
                raise ValueError(f"fixture replace target not found in {path}: {op['old'][:40]!r}")
            overlay[path] = text.replace(op["old"], op["new"], 1).encode("utf-8")
        elif op["op"] == "json_set":
            doc = json.loads(current(path))
            node = doc
            keys = [k for k in op["pointer"].split("/") if k]
            for k in keys[:-1]:
                node = node[int(k)] if isinstance(node, list) else node[k]
            last = keys[-1]
            if isinstance(node, list):
                node[int(last)] = op["value"]
            else:
                node[last] = op["value"]
            overlay[path] = (json.dumps(doc, ensure_ascii=False, indent=2) + "\n").encode("utf-8")
        else:
            raise ValueError(f"unknown fixture op {op['op']}")
    return overlay


def self_test(t: Tree) -> list[str]:
    problems: list[str] = []
    for group in GROUPS:
        try:
            run_group(t, group)
        except Failure as exc:
            problems.append(f"baseline must pass before self-test: {group}: {exc}")
    if problems:
        return problems
    for group, assertions in GROUPS.items():
        for name, _ in assertions:
            fixture_path = f"{FIXTURES}/{group}/{name}/fixture.json"
            if not t.exists(fixture_path):
                problems.append(f"{name}: no fixture at {fixture_path}")
                continue
            fixture = t.json(fixture_path)
            if fixture.get("assertion") != name:
                problems.append(f"{fixture_path}: declares {fixture.get('assertion')}, expected {name}")
                continue
            try:
                bad = t.with_overlay(apply_ops(t, fixture["ops"]))
                run_group(bad, group)
                problems.append(f"{name}: fixture was accepted")
            except Failure as exc:
                if exc.assertion != name:
                    problems.append(f"{name}: fixture rejected by {exc.assertion} instead")
            except ValueError as exc:
                problems.append(f"{name}: {exc}")
    return problems


# ---------------------------------------------------------------- main


def main(argv: list[str]) -> int:
    worktree = "--allow-dirty" in argv
    passed = "DEV-PASS (worktree, not gate evidence)" if worktree else "OK"
    if "--list" in argv:
        for group, assertions in GROUPS.items():
            print(group + ": " + ", ".join(n for n, _ in assertions))
        return 0
    if not worktree:
        dirty = dirty_paths()
        if dirty:
            print("FAIL dirty-worktree: commit or stash changes first:\n  " + "\n  ".join(dirty[:20]))
            return 2
    t = Tree(worktree)
    if "--self-test" in argv:
        problems = self_test(t)
        if problems:
            print("FAIL self-test:\n  " + "\n  ".join(problems))
            return 1
        count = sum(len(a) for a in GROUPS.values())
        print(f"{passed} self-test ({count} assertions rejected their fixtures)")
        return 0
    if "--check" not in argv:
        print(__doc__)
        return 2
    group = argv[argv.index("--check") + 1]
    groups = list(GROUPS) if group == "all" else [group]
    for g in groups:
        if g not in GROUPS:
            print(f"FAIL unknown-group: {g}; choose from {', '.join(GROUPS)}")
            return 2
        try:
            run_group(t, g)
        except Failure as exc:
            print(f"FAIL {exc}")
            return 1
        print(f"{passed} {g}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
