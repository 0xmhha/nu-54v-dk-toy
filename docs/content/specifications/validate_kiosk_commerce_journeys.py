"""Check journey references, coverage and preserved baselines; no product execution."""
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DOC = ROOT / 'content/specifications/kiosk-commerce-journey-design.json'
read = lambda p: json.loads((ROOT / p).read_text())
d = json.loads(DOC.read_text())
assert d['implementation'] == 'deferred_by_user'
assert d['status'] == 'design_proposal_not_merged'
assert not any(d[k] for k in ['canonicalMerged', 'runtimeVerified', 'sqlApplied'])
assert d['policySelectionUnchanged']
for path, digest in d['sourceHashes'].items():
    assert hashlib.sha256((ROOT / path).read_bytes()).hexdigest() == digest, path
wbs = read('content/planning/work-breakdown.json')
package = next(p for p in read('content/planning/full-scope-design-review.json')['designPackages'] if p['id'] == d['packageRef'])
assert set(package['primaryTaskRefs']) <= set(d['taskRefs']) <= {t['id'] for t in wbs['tasks']}
decision_ids = {r['id'] for r in wbs['decisions']}
assert set(d['decisionRefs']) <= decision_ids
assert d['requirementRefs'] == package['requirementRefs']
assert wbs['taskCount'] == 104 and wbs['subtaskCount'] == 320
assert len(decision_ids) == 19 and all(x['status'] == 'open' for x in wbs['decisions'])
apis = read('content/specifications/api-catalog.json')['operations']
screens = read('content/specifications/screen-flows.json')['screens']
api_ids, screen_ids = {x['id'] for x in apis}, {x['id'] for x in screens}
assert len(api_ids) == 110 and len(screen_ids) == 37
api037 = next(a for a in apis if a['id'] == 'API-037')
assert {'expectedRevision','expectedFundingRevision','merchantSigner'} <= set(api037['inputFields'])
md = DOC.with_suffix('.md').read_text()
counts = {'journeys':12,'paymentDisplayStates':10,'policyInputs':7,'contractDeltas':7,'runtimeCases':34}
for name, count in counts.items():
    rows = d[name]
    assert len(rows) == len({r['id'] for r in rows}) == count
    assert all(r['id'] in md for r in rows)
case_ids = {c['id'] for c in d['runtimeCases']}
by_journey = {j['id']: j for j in d['journeys']}
for j in d['journeys']:
    assert set(j['screenRefs']) <= screen_ids and set(j['apiRefs']) <= api_ids
    assert j['userVisibleSuccessWhen'] in md and j['recovery'] in md
    assert not j['runtimeVerified'] and j['caseRefs']
    assert j['acceptance'] == dict(normalControlRequired=True, negativeBranchPassIsJourneyComplete=False, status='not_run', evidenceRefs=[])
    assert set(j['caseRefs']) == {c['id'] for c in d['runtimeCases'] if c['journeyRef'] == j['id']}
assert set().union(*(set(j['caseRefs']) for j in d['journeys'])) == case_ids
for c in d['runtimeCases']:
    assert c['journeyRef'] in by_journey
    assert c['status'] == 'not_run' and c['evidenceRefs'] == []
for p in d['policyInputs']:
    assert p['selection'] is None and set(p['decisionRefs']) <= decision_ids
    assert p['whenUnselected'] in md
for delta in d['contractDeltas']:
    assert set(delta['apiRefs']) <= api_ids
assert d['baseline'] == dict(tasks=104,steps=320,decisions=19,apis=110,screens=37)
print(json.dumps(dict(scope='journey_inventory_and_reference_checks_only', **counts, sourceFilesPreserved=len(d['sourceHashes']), runtimeVerified=False)))
