#!/usr/bin/env python3
"""Validate the implementation-entry design freeze without activating runtime work."""

import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def read(path: str):
    return json.loads((ROOT / path).read_text())


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


freeze = read("content/planning/design-freeze-checkpoint.json")
contract = read("content/specifications/design-baseline-contract.json")
pins = read("content/environment/toolchain-pins.json")
providers = read("content/environment/provider-accounts.template.json")
chain = read("content/environment/stablenet-8283.manifest.json")
indexer = read("content/environment/indexer.manifest.json")
trust = read("content/environment/trust-bootstrap.template.json")
fixtures = read("content/environment/test-fixtures.manifest.json")

require(freeze["checkpointId"] == "DF-20260920-01", "checkpoint id")
require(freeze["counts"] == {
    "selectedDecisions": 20,
    "detailedProductAreas": 10,
    "adoptedContractBundles": 8,
    "environmentPreparationItems": 7,
}, "checkpoint counts")
require({d["id"] for d in freeze["decisions"]} == {f"D{i:02}" for i in range(1, 20)} | {"RR-DEC-01"}, "decision ids")
require(all(d["status"] == "selected_for_design_baseline" and d["selection"] and d["fallback"] for d in freeze["decisions"]), "decision completion")
require({p["id"] for p in freeze["productDesigns"]} == {f"B-{i:02}" for i in range(1, 11)}, "product ids")
require(all(set(["scope", "modules", "interfaces", "persistence", "state", "failures", "acceptance"]) <= set(p) for p in freeze["productDesigns"]), "product detail fields")
require({a["id"] for a in freeze["contractAdoptions"]} == {f"UA-{i:02}" for i in range(1, 9)}, "adoption ids")
require(all(a["status"] == "adopted_design_baseline" and not a["runtimeActive"] for a in freeze["contractAdoptions"]), "adoption boundary")
require({e["id"] for e in freeze["environmentPreparation"]} == {f"E-{i:02}" for i in range(1, 8)}, "environment ids")
require(not freeze["implementationStarted"] and not freeze["runtimeVerified"] and not freeze["sqlApplied"] and not freeze["contractsDeployedByThisCheckpoint"], "implementation boundary")

for path, expected in freeze["sourceHashes"].items():
    actual = hashlib.sha256((ROOT / path).read_bytes()).hexdigest()
    require(actual == expected, f"source hash drift: {path}")

require(contract["checkpointId"] == freeze["checkpointId"], "contract checkpoint")
require({b["id"] for b in contract["bundles"]} == {a["id"] for a in freeze["contractAdoptions"]}, "contract bundle coverage")
require(not any([contract["runtimeActive"], contract["databaseApplied"], contract["firmwareApplied"], contract["chainApplied"]]), "contract activation boundary")

require(pins["mobile"]["reactNative"] == "0.87.x" and pins["firmware"]["tag"] == "v3.4.0", "toolchain pins")
require(providers["status"] == "template_only_no_credentials", "provider template status")
require(all("secretRef" in p and p["secretRef"] for p in providers["providers"]), "provider secret references")
require(chain["chain"]["chainId"] == 8283 and chain["chain"]["chainIdHex"] == "0x205b", "chain identity")
require(all(not c["enabled"] for c in chain["contracts"]), "undeployed contract must be disabled")
require(indexer["status"] == "revisions_pinned_runtime_not_deployed" and indexer["policy"]["startBlock"] is None, "indexer activation boundary")
require(trust["status"] == "template_only_no_real_identity_or_key", "trust bootstrap boundary")
require(fixtures["status"] == "prepared_synthetic_only", "fixture classification")

rendered = (ROOT / "content/planning/design-freeze-checkpoint.md").read_text()
require("20개 선택 완료 / 10개 제품 상세 설계 완료 / 8개 계약 묶음 설계 기준 채택 / 7개 환경 준비" in rendered, "rendered summary")

print(json.dumps({
    "checkpoint": freeze["checkpointId"],
    "decisions": 20,
    "products": 10,
    "adoptions": 8,
    "environments": 7,
    "runtimeActive": False,
    "validated": True,
}, ensure_ascii=False))
