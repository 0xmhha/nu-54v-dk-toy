"""Check proposal inventories/references; no authentication or MPC execution."""
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DOC = ROOT / 'content/specifications/social-wallet-recovery-design.json'
d = json.loads(DOC.read_text())
assert d['implementation'] == 'deferred_by_user'
assert d['status'] == 'design_proposal_not_merged'
assert not d['canonicalMerged'] and not d['runtimeVerified']
assert d['policySelectionUnchanged']
assert len(d['selections']) == 8 and all(v is None for v in d['selections'].values())
for path, digest in d['sourceHashes'].items():
    assert hashlib.sha256((ROOT / path).read_bytes()).hexdigest() == digest, path
wbs = json.loads((ROOT / 'content/planning/work-breakdown.json').read_text())
scope = json.loads((ROOT / 'content/planning/full-scope-design-review.json').read_text())
package = next(p for p in scope['designPackages'] if p['id'] == d['packageRef'])
assert set(package['primaryTaskRefs']) <= set(d['taskRefs']) <= {t['id'] for t in wbs['tasks']}
assert set(d['decisionRefs']) <= {x['id'] for x in wbs['decisions']}
assert d['requirementRefs'] == package['requirementRefs']
assert wbs['taskCount'] == 104 and wbs['subtaskCount'] == 320
assert len(wbs['decisions']) == 19 and all(x['status'] == 'open' for x in wbs['decisions'])
api = json.loads((ROOT / 'content/specifications/api-catalog.json').read_text())
api_ids = {x['id'] for x in api['operations']}
ifs = json.loads((ROOT / 'content/specifications/implementation-interfaces.json').read_text())
if_ids = {x['id'] for x in ifs['interfaces']}
aliases = {'local_app', 'unregistered_refresh_adapter', 'unregistered_identity_unlink', 'unregistered_security_event'}
md = DOC.with_suffix('.md').read_text()
counts = {'authTransitions':11, 'mpcTransitions':11, 'authorityImpacts':8, 'logicalResources':8, 'contractDeltas':6, 'runtimeCases':32}
for key, expected in counts.items():
    rows = d[key]
    assert len(rows) == len({r['id'] for r in rows}) == expected, key
    assert all(r['id'] in md for r in rows), key
for t in d['authTransitions'] + d['mpcTransitions']:
    assert set(t['route'].split('/')) <= api_ids | if_ids | aliases
    assert t['guard'] in md and t['durableBoundary'] in md
for t in d['contractDeltas']:
    assert set(t['apiRefs']) <= api_ids
transitions = d['mpcTransitions']
states = {'absent', 'enrolling', 'dkg_pending', 'active', 'signing', 'recovery_proving', 'replacement_prepared', 'epoch_committed', 'verifying', 'recovery_hold'}
for t in transitions:
    assert set(t['fromState'].split('|')) <= states and t['toState'] in states
active_sources = {t['fromState'] for t in transitions if t['toState'] == 'active'}
assert active_sources == {'dkg_pending', 'signing', 'verifying'}
assert not any(t['toState'] == 'active' and t['fromState'] in {'recovery_proving','replacement_prepared','epoch_committed'} for t in transitions)
assert d['holdResumeRule']['directActiveAllowed'] is False
assert d['holdResumeRule']['authorityRecheck'] is True
assert d['holdResumeRule']['afterCommit'] == 'same_committed_epoch_only'
assert all(c['status'] == 'not_run' and c['evidenceRefs'] == [] for c in d['runtimeCases'])
assert len(d['externalReferences']) == 3
assert all(r['url'] in md for r in d['externalReferences'])
print(json.dumps(dict(scope='design_inventory_and_source_references_only', **counts, sourceFilesPreserved=len(d['sourceHashes']), runtimeVerified=False)))
