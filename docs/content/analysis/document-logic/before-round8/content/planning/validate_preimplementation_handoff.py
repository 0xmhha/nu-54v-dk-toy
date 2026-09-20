"""Audit design inventories, source pins and state boundaries; no product execution.

Run from any directory. No files or external systems are mutated.
"""
import hashlib
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
def read(path):
    return json.loads((ROOT / path).read_text())

def check_pins(doc):
    for path, digest in doc['sourceHashes'].items():
        assert hashlib.sha256((ROOT / path).read_bytes()).hexdigest() == digest, path

def unique(rows):
    ids = [x['id'] for x in rows]
    assert len(ids) == len(set(ids)), ids

def unchanged_design(doc):
    assert doc['implementation'] == 'deferred_by_user'
    assert doc['canonicalMerged'] is False
    assert doc['runtimeVerified'] is False
    check_pins(doc)

wbs = read('content/planning/work-breakdown.json')
full = read('content/planning/full-scope-design-review.json')
tasks = {t['id']: t for t in wbs['tasks']}
packages = {p['id']: p for p in full['designPackages']}
apis = {x['id'] for x in read('content/specifications/api-catalog.json')['operations']}
screens = {x['id'] for x in read('content/specifications/screen-flows.json')['screens']}
decisions = {d['id'] for d in wbs['decisions']}
assert len(tasks) == 104 and sum(len(t['implementationSteps']) for t in tasks.values()) == 320
assert len(apis) == 110 and len(screens) == 37
assert len(decisions) == 19 and all(d['status'] == 'open' for d in wbs['decisions'])

spec_paths = ['credential-paid-resource-design', 'recording-travel-ai-design', 'operations-release-acceptance-design']
counts = {}
for name in spec_paths:
    path = 'content/specifications/' + name + '.json'
    doc = read(path)
    unchanged_design(doc)
    pkg = packages[doc['packageRef']]
    assert doc['requirementRefs'] == pkg['requirementRefs']
    assert doc['taskRefs'] == pkg['primaryTaskRefs']
    assert doc['decisionRefs'] == pkg['decisionRefs']
    assert doc['policySelectionUnchanged'] is True
    md = (ROOT / path).with_suffix('.md').read_text()
    for key, value in doc.items():
        if isinstance(value, list) and value and isinstance(value[0], dict) and 'id' in value[0]:
            unique(value)
            for row in value:
                assert row['id'] in md, (name, key, row['id'])
                assert set(row.get('apiRefs', row.get('existingApiRefs', []))) <= apis
                assert set(row.get('screenRefs', [])) <= screens
    for row in doc['policyInputs']:
        assert row['decisionRef'] in decisions and row['selection'] is None
    for row in doc['runtimeCases']:
        assert row['status'] == 'not_run' and row['evidenceRefs'] == []
    for row in doc['externalReferences']:
        assert row['url'] in md
    counts[doc['packageRef']] = dict(tasks=len(doc['taskRefs']), runtimeCasesNotRun=len(doc['runtimeCases']), policyInputsUnselected=len(doc['policyInputs']))

cred = read('content/specifications/credential-paid-resource-design.json')
assert set(cred['independentStateAxes']) == {'payment', 'refund', 'entitlement', 'delivery', 'rule'}
assert any(t['id'] == 'PX-09' and 'payment 축만' in t['effect'] for t in cred['paidResourceTransitions'])
assert any(t['id'] == 'PX-12' and 'refund fence' in t['effect'] for t in cred['paidResourceTransitions'])
assert all('refund fence' in t['guard'] for t in cred['paidResourceTransitions'] if t['id'] in ['PX-06', 'PX-07'])
rec = read('content/specifications/recording-travel-ai-design.json')
assert any(t['id'] == 'PV-05' and '대상 추가' in t['effect'] and 'completed' in t['fromState'] for t in rec['privacyTransitions'])
# The vocabularies are design constraints; this checks typos/unknown states, not runtime concurrency.
vocabularies = [
    (cred, 'credentialTransitions', {'draft','active','revoked','expired','unknown'}),
    (cred, 'paidResourceTransitions', {'created','payment_required','proof_received','settlement_pending','paid','entitled','delivering','delivered','unknown','delivery_failed','refund_pending','refunded','cancelled'}),
    (rec, 'privacyTransitions', {'active','deny_effective','deleting','partially_deleted','completed','purpose_blocked'}),
]
for doc, section, states in vocabularies:
    for row in doc[section]:
        assert set(row['fromState'].split('|')) <= states
        assert row['toState'] in states and row['guard'] and row['effect']

ov = read('content/specifications/preimplementation-contract-overlay.json')
unchanged_design(ov)
assert ov['wireSchemasFrozen'] is False
assert len(ov['routeContracts']) == 24 and len(ov['actionVariants']) == 11
assert len(ov['storageAndAtomicity']) == 8 and len(ov['finiteReviewCases']) == 12
omd = (ROOT / 'content/specifications/preimplementation-contract-overlay.md').read_text()
for name in ['routeContracts', 'actionVariants', 'storageAndAtomicity', 'finiteReviewCases']:
    unique(ov[name])
    assert all(x['id'] in omd for x in ov[name])
for r in ov['routeContracts']:
    assert set(r['existingApiRefs']) <= apis
    for k in ['authority', 'requestFields', 'responseFields', 'atomicBoundary', 'recovery']:
        assert r[k]

h = read('content/planning/preimplementation-handoff.json')
unchanged_design(h)
assert not h['allImplementationReady']
assert h['ownerAssignment'] is None and h['effortEstimates'] is None
assert len(h['designPackages']) == 8 and {p['id'] for p in h['designPackages']} == packages.keys()
assert len(h['taskHandoffs']) == len(tasks)
assert {t['taskId'] for t in h['taskHandoffs']} == tasks.keys()
step_ids = []
for row in h['taskHandoffs']:
    t = tasks[row['taskId']]
    assert row['requirementRefs'] == t['requirements']
    assert row['stepRefs'] == [s['id'] for s in t['implementationSteps']]
    assert row['dependsOn'] == t['dependsOn']
    assert row['decisionRefs'] == t['decisionInputs']
    assert row['acceptanceCriteria'] == t['acceptanceCriteria']
    assert row['apiRefs'] == t['interfaceRefs'] and row['screenRefs'] == t['screenRefs']
    assert row['taskId'] in packages[row['packageRef']]['primaryTaskRefs']
    assert row['executionStatus'] == 'not_run' and row['owner'] is None and row['effortEstimate'] is None
    for path in row['designInputs'] + [row['evidenceTemplate']]:
        assert (ROOT / path).is_file(), path
    for ref in row['apiRefs']:
        assert (ROOT / ref['file']).is_file()
        if ref['file'].endswith('api-catalog.json'):
            assert ref['entry'] in apis
    step_ids += row['stepRefs']
assert len(step_ids) == len(set(step_ids)) == 320
assert {r['requirement'] for r in h['requirements']} == set(range(1, 16))
assert h['compoundRequirementFacets'] == full['compoundRequirementFacets']
assert len(h['compoundRequirementFacets']) == 16
assert {d['id'] for d in h['decisionLedger']} == decisions
assert all(d['selection'] is None and d['status'] == 'open' for d in h['decisionLedger'])
source_q = next(q for q in read(h['returnPolicy']['source'])['openDecisions'] if q['id'] == 'RR-DEC-01')
assert source_q['selection'] is h['returnPolicy']['selection'] is None
assert source_q['status'] == h['returnPolicy']['status'] == 'awaiting_user_preference'
assert h['returnPolicy']['reask'] is False
for entry in read('content/specifications/approval-source-dispatch.json')['entries']:
    if entry['sourceKind'] in ['smart_account', 'market_action', 'credential_proof', 'paid_resource']:
        assert entry['status'] == 'adapter_pending_execution_blocked'
        assert entry['executionVerified'] is False
# Only check the new local link set; historical source references remain immutable.
for path in ['content/planning/preimplementation-handoff.md', 'content/planning/preimplementation-task-handoffs.md']:
    p = ROOT / path
    for link in re.findall(r'\]\(([^)]+)\)', p.read_text()):
        if not link.startswith(('http:', 'https:')):
            assert (p.parent / link.split('#')[0]).exists(), (path, link)
result = dict(scope='design_inventory_and_source_integrity_only', packages=counts, handoffTasks=104, handoffSteps=320, requirements=15, compoundFacets=16, openDecisions=19, overlayRoutes=24, actionVariants=11, runtimeCasesAddedNotRun=sum(x['runtimeCasesNotRun'] for x in counts.values()), runtimeVerified=False, implementationReady=False)
print(json.dumps(result, ensure_ascii=False))
