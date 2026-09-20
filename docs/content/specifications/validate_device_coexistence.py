"""Validate design inventory and unchanged source references, not firmware behavior."""
import hashlib
import itertools
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DOC = ROOT / 'content/specifications/device-coexistence-design.json'
d = json.loads(DOC.read_text())
assert d['implementation'] == 'deferred_by_user'
assert not d['canonicalMerged'] and not d['runtimeVerified']
assert d['policySelectionUnchanged'] and d['firmwareBase'] == 'Zephyr'
assert all(d[k] is None for k in ['selectedSdk', 'selectedBoardTarget', 'selectedProfile'])
for path, digest in d['sourceHashes'].items():
    assert hashlib.sha256((ROOT / path).read_bytes()).hexdigest() == digest, path
wbs = json.loads((ROOT / 'content/planning/work-breakdown.json').read_text())
assert set(d['taskRefs']) <= {t['id'] for t in wbs['tasks']}
assert set(d['decisionRefs']) <= {t['id'] for t in wbs['decisions']}
assert wbs['taskCount'] == 104 and wbs['subtaskCount'] == 320
assert len(wbs['decisions']) == 19
scope = json.loads((ROOT / 'content/planning/full-scope-design-review.json').read_text())
package = next(x for x in scope['designPackages'] if x['id'] == d['packageRef'])
assert set(package['primaryTaskRefs']) <= set(d['taskRefs'])
assert d['requirementRefs'] == package['requirementRefs']
ops = {o['id'] for o in d['operations']}
resources = {r['id'] for r in d['resources']}
assert len(ops) == len(d['operations']) == 10
assert len(resources) == len(d['resources'])
for op in d['operations']:
    assert set(op['resourceRefs']) <= resources
    assert op['interfaceRef'] in {'IF-02', 'IF-03', 'IF-04', 'IF-05', 'IF-06', 'IF-07'}
matrix = d['admissionMatrix']
pairs = {(r['active'], r['incoming']) for r in matrix}
assert pairs == set(itertools.product(ops, repeat=2))
assert len(pairs) == len(matrix)
assert all(r['action'] in {'busy', 'deduplicate_or_busy', 'yield_at_checkpoint', 'profile_conditional'} and r['rule'] for r in matrix)
assert all(r['action'] == 'deduplicate_or_busy' for r in matrix if r['active'] == r['incoming'])
assert all(r['action'] == 'busy' for r in matrix if r['active'] in {'apply','lifecycle'} and r['incoming'] != r['active'])
assert all(r['active'] in {'find','upload'} for r in matrix if r['action'] == 'yield_at_checkpoint')
assert {x['id'] for x in d['admissionExceptions']} == {'DC-X01', 'DC-X02'}
assert {x['kind'] for x in d['admissionExceptions']} == {'approval_child_observation', 'apply_prepare_control'}
states = {'idle','receiving','verified','draining','boot_pending','trial','confirmed','recovery_hold','cancelled'}
transitions = d['fotaTransitions']
assert len({t['id'] for t in transitions}) == len(transitions) == 10
for t in transitions:
    assert set(t['fromState'].split('|')) <= states and t['toState'] in states
    assert all(t[k] for k in ['guard','durableBoundary','uiLabel'])
assert all(t['fromState'] in {'trial','recovery_hold'} for t in transitions if t['toState'] == 'confirmed')
assert all(set(t['fromState'].split('|')) <= {'receiving','verified','draining'} for t in transitions if t['toState'] == 'cancelled')
cases = d['runtimeCases']
assert len({c['id'] for c in cases}) == len(cases) == 28
assert all(c['status'] == 'not_run' and c['evidenceRefs'] == [] for c in cases)
assert all(v['selectedValue'] is None and v['evidenceRefs'] == [] for v in d['measurementVariables'])
md = DOC.with_suffix('.md').read_text()
assert all(c['id'] in md for c in cases)
assert all(t['id'] in md for t in transitions)
assert all(x['id'] in md for x in d['admissionExceptions'])
assert all(t['guard'] in md for t in transitions)
print(json.dumps(dict(scope='design_inventory_and_reference_checks_only', operations=len(ops), directionalPairs=len(matrix), fotaTransitions=len(transitions), resources=len(resources), runtimeCasesNotRun=len(cases), sourceFilesPreserved=len(d['sourceHashes']), runtimeVerified=False)))
