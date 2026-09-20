"""Validate design coverage and pinned inputs, never live-chain compatibility."""
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DOC = ROOT / 'content/specifications/stablenet-compatibility-design.json'
read = lambda p: json.loads((ROOT / p).read_text())
d = json.loads(DOC.read_text())
assert d['implementation'] == 'deferred_by_user'
assert d['status'] == 'design_proposal_not_merged'
assert not d['canonicalMerged'] and not d['runtimeVerified']
assert d['policySelectionUnchanged']
assert len(d['selections']) == 8 and all(x is None for x in d['selections'].values())
for path, digest in d['sourceHashes'].items():
    assert hashlib.sha256((ROOT / path).read_bytes()).hexdigest() == digest, path
wbs = read('content/planning/work-breakdown.json')
pkg = next(p for p in read('content/planning/full-scope-design-review.json')['designPackages'] if p['id'] == d['packageRef'])
assert set(pkg['primaryTaskRefs']) <= set(d['taskRefs']) <= {t['id'] for t in wbs['tasks']}
assert d['requirementRefs'] == pkg['requirementRefs']
assert set(d['decisionRefs']) <= {t['id'] for t in wbs['decisions']}
assert wbs['taskCount'] == 104 and wbs['subtaskCount'] == 320
assert len(wbs['decisions']) == 19 and all(x['status'] == 'open' for x in wbs['decisions'])
network = d['network']
assert network['historicallyObservedChainId'] == 8283
assert network['observationDate'] == '2026-09-17' and not network['liveVerifiedThisTurn']
assert all(network[k] is None for k in ['environmentId','genesisHash','checkpointHash','indexerEndpoint'])
counts = dict(manifestComponents=8, assets=4, domainCoverage=9, indexerStages=8, accountTransitions=7, runtimeCases=34)
md = DOC.with_suffix('.md').read_text()
for key, count in counts.items():
    rows = d[key]
    assert len(rows) == len({x['id'] for x in rows}) == count
    assert all(x['id'] in md for x in rows)
for m in d['manifestComponents']:
    assert set(m['requiredFields']) == set(m['values'])
    assert all(x is None for x in m['values'].values())
    assert m['compatibility'] == 'unverified' and m['evidenceRefs'] == []
assert {a['kind'] for a in d['assets']} == {'native','erc20'}
assert len([a for a in d['assets'] if a['kind'] == 'native']) == 1
assert all(a['address'] is None and a['decimals'] is None and a['status'] == 'unverified' for a in d['assets'])
assert all(x['status'] == 'design_mapping_not_deployed' for x in d['domainCoverage'])
states = {'eoa_active','plan_ready','authorized','execution_pending','account_verified','migration_pending','ready','held'}
for t in d['accountTransitions']:
    assert set(t['fromState'].split('|')) <= states and t['toState'] in states
    assert t['guard'] in md and t['record'] in md
assert all(set(t['fromState'].split('|')) <= {'account_verified','migration_pending'} for t in d['accountTransitions'] if t['toState'] == 'ready')
assert all(c['status'] == 'not_run' and c['evidenceRefs'] == [] for c in d['runtimeCases'])
assert len(d['externalReferences']) == 3 and all(r['url'] in md for r in d['externalReferences'])
dispatch = read('content/specifications/approval-source-dispatch.json')
aa = next(e for e in dispatch['entries'] if e['sourceKind'] == 'smart_account')
assert aa['status'] == 'adapter_pending_execution_blocked' and not aa['executionVerified']
print(json.dumps(dict(scope='design_inventory_and_pinned_references_only', **counts, preservedSources=len(d['sourceHashes']), liveRpcVerified=False, runtimeVerified=False)))
