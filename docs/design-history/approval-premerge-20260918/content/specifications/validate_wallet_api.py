"""Check candidate document references and synthetic HTTP envelope shapes only.
Does not implement/test authorization, cryptography, chain calls, or concurrency.
"""
import hashlib
import json
from pathlib import Path
from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource

ROOT = Path(__file__).resolve().parent
load = lambda name: json.loads((ROOT / name).read_text())
p = load('wallet-api-candidate.json')
s = load(p['schemaFile'])
examples = load(p['caseFile'])
assert p['canonicalMerged'] is p['runtimeVerified'] is False
assert p['policySelectionUnchanged'] is True
for name, digest in p['baselineFiles'].items():
    assert hashlib.sha256((ROOT / name).read_bytes()).hexdigest() == digest, name
registry = Registry()
for uri, name in p['externalSchemaBindings'].items():
    baseline = load(name)
    baseline['$id'] = uri
    registry = registry.with_resource(uri, Resource.from_contents(baseline))
Draft202012Validator.check_schema(s)
for category in ('valid', 'invalid'):
    for x in examples[category]:
        v = Draft202012Validator(dict(s, **{'$ref': '#/$defs/'+x['schema']}),
                                 registry=registry, format_checker=FormatChecker())
        errors = list(v.iter_errors(x['value']))
        assert bool(errors) == (category == 'invalid'), (x['name'], [e.message for e in errors])
api = {x['id']: x for x in load('api-catalog.json')['operations']}
wbs = json.loads((ROOT.parent/'planning/work-breakdown.json').read_text())
tasks = {x['id'] for x in wbs['tasks']}
assert len({x['apiId'] for x in p['operations']}) == len(p['operations']) == 8
for op in p['operations']:
    base = api[op['apiId']]
    assert (op['method'], op['path'], op['authorization']) == (base['method'], base['path'], base['authorization'])
    assert set(op['taskRefs']) <= tasks
    for key in ('requestSchema', 'successResponseSchema', 'errorResponseSchema'):
        assert op[key] in s['$defs']
        assert any(x['schema'] == op[key] for x in examples['valid']), op[key]
assert len({x['id'] for x in examples['reviewCases']}) == len(examples['reviewCases'])
assert all(x['status'] == 'not_run' for x in examples['reviewCases'])
assert set(p['decisionRefs']) <= {d['id'] for d in wbs['decisions']}
assert wbs['phaseControl']['implementation'] == 'deferred_by_user'
pending = next(x for x in load('return-recovery-contract.json')['openDecisions'] if x['id'] == 'RR-DEC-01')
assert pending['selection'] is None and pending['status'] == 'awaiting_user_preference'
print(json.dumps({'candidateEndpoints': len(p['operations']), 'validShapes': len(examples['valid']),
                  'invalidShapes': len(examples['invalid']), 'unexecutedReviewCases': len(examples['reviewCases']),
                  'baselineHashes': 'matched', 'canonicalMerged': False, 'runtimeVerified': False}))
