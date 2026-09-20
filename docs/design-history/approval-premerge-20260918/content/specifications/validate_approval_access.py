"""Design checks only: schema shapes, finite field matching and baseline pins.

No authentication, sender proof, cryptography, BLE, MPC or runtime verification.
"""
import copy
import hashlib
import json
from pathlib import Path

from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource

ROOT = Path(__file__).resolve().parent
load = lambda p: json.loads((ROOT / p).read_text())
manifest = load('approval-access-candidate.json')
schema = load(manifest['schemaFile'])
examples = load(manifest['examplesFile'])
assert manifest['canonicalMerged'] is manifest['runtimeVerified'] is False
assert manifest['implementation'] == 'deferred_by_user'
assert manifest['policySelectionUnchanged'] is True
for name, digest in manifest['baselineFiles'].items():
    assert hashlib.sha256((ROOT / name).read_bytes()).hexdigest() == digest, name

registry = Registry()
for uri, file in manifest['externalSchemaBindings'].items():
    external = load(file)
    external['$id'] = uri
    registry = registry.with_resource(uri, Resource.from_contents(external))
Draft202012Validator.check_schema(schema)


def validator(name):
    return Draft202012Validator(dict(schema, **{'$ref': '#/$defs/' + name}),
                                registry=registry, format_checker=FormatChecker())


# Resolve every local and external reference, including types not in fixtures.
def check_refs(node):
    if isinstance(node, dict):
        if '$ref' in node:
            uri, fragment = node['$ref'].split('#', 1)
            doc = schema if not uri else load(manifest['externalSchemaBindings'][uri])
            for part in fragment.lstrip('/').split('/'):
                doc = doc[part.replace('~1', '/').replace('~0', '~')]
        for value in node.values():
            check_refs(value)
    elif isinstance(node, list):
        for value in node:
            check_refs(value)


check_refs(schema)
for kind in ('valid', 'invalid'):
    for example in examples[kind]:
        errors = list(validator(example['schema']).iter_errors(example['value']))
        assert bool(errors) == (kind == 'invalid'), (example['name'], [e.message for e in errors])
for route in manifest['proposedAccessRoutes']:
    assert route['catalogStatus'] == 'proposed_not_registered'
    for key in ('requestSchema', 'responseSchema', 'errorSchema'):
        assert route[key] in schema['$defs']
        if key != 'errorSchema':
            assert any(e['schema'] == route[key] for e in examples['valid'])


def snapshot_matches(value):
    context, intent = value['context'], value['intent']
    if value['sourceKind'] != context['sourceKind']:
        return False
    if any(context[k] != intent[k] for k in ('intentId', 'chainId', 'payerAddress', 'payloadDigest', 'expiresAt')):
        return False
    if context['profileId'] != intent['encodingProfile']:
        return False
    source_key = {'personal_transfer': 'sourceId', 'payment': 'attemptId', 'merchant_refund': 'refundId'}[value['sourceKind']]
    if context[source_key] != intent[source_key]:
        return False
    if value['sourceKind'] != 'personal_transfer' and context['storeId'] != intent['merchantDisplay']['storeId']:
        return False
    return True


consistency_count = 0
for example in examples['valid']:
    if example['schema'] == 'Snapshot':
        value = example['value']
        assert snapshot_matches(value), example['name']
        consistency_count += 1
        altered = copy.deepcopy(value)
        altered['context']['intentId'] = 'another_intent'
        assert validator('Snapshot').is_valid(altered)
        assert not snapshot_matches(altered)
        consistency_count += 1

pending = next(x for x in load('return-recovery-contract.json')['openDecisions'] if x['id'] == 'RR-DEC-01')
assert pending['selection'] is None and pending['status'] == 'awaiting_user_preference'
assert all(x['status'] == 'not_run' for x in manifest['reviewCases'])
assert len({x['id'] for x in manifest['reviewCases']}) == len(manifest['reviewCases'])
wbs = json.loads((ROOT.parent / 'planning/work-breakdown.json').read_text())
assert wbs['phaseControl']['implementation'] == 'deferred_by_user'
assert len(wbs['tasks']) == 104
print(json.dumps({'schemaDefinitions': len(schema['$defs']),
                  'proposedAccessRoutes': len(manifest['proposedAccessRoutes']),
                  'validShapes': len(examples['valid']), 'invalidShapes': len(examples['invalid']),
                  'finiteSnapshotConsistencyChecks': consistency_count,
                  'unexecutedReviewCases': len(manifest['reviewCases']),
                  'baselineHashes': 'matched', 'canonicalMerged': False, 'runtimeVerified': False}))
