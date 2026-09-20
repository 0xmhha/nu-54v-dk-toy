"""Validate source-approval candidate shapes/references and finite field matching.
No proof validation, device/MPC execution, real authorization or concurrency tests.
"""
import hashlib
import json
from pathlib import Path
from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource

ROOT = Path(__file__).resolve().parent
load = lambda p: json.loads((ROOT / p).read_text())
p = load('source-approval-candidate.json')
s = load(p['schemaFile'])
e = load(p['examplesFile'])
assert p['canonicalMerged'] is p['runtimeVerified'] is False
assert p['policySelectionUnchanged'] is True
for name, digest in p['baselineFiles'].items():
    assert hashlib.sha256((ROOT / name).read_bytes()).hexdigest() == digest, name
registry = Registry()
for uri, path in p['externalSchemaBindings'].items():
    baseline = load(path)
    baseline['$id'] = uri
    registry = registry.with_resource(uri, Resource.from_contents(baseline))
Draft202012Validator.check_schema(s)


def validator(name):
    return Draft202012Validator(dict(s, **{'$ref': '#/$defs/'+name}), registry=registry,
                                format_checker=FormatChecker())


for category in ('valid', 'invalid'):
    for x in e[category]:
        errors = list(validator(x['schema']).iter_errors(x['value']))
        assert bool(errors) == (category == 'invalid'), (x['name'], [v.message for v in errors])


def consistent(value):
    data = value['body']['data']
    snapshot = data['snapshot']
    context, intent = snapshot['context'], snapshot['intent']
    if any(context[k] != intent[k] for k in ('intentId','chainId','payerAddress','payloadDigest','expiresAt')):
        return False
    if context['sourceKind'] != snapshot['sourceKind'] or context['storeId'] != intent['merchantDisplay']['storeId']:
        return False
    if context['profileId'] != intent['encodingProfile']:
        return False
    source_key = 'attemptId' if context['sourceKind'] == 'payment' else 'refundId'
    if context[source_key] != intent[source_key]:
        return False
    if 'refund' in data:
        refund = data['refund']
        if (context['authorizedRefundRevision'],context['paymentId'],context['refundId']) != (
                refund['revision'],refund['paymentId'],refund['refundId']):
            return False
        if (refund['assetId'],refund['atomicAmount'],refund['destinationAddress']) != (
                intent['asset']['assetId'],intent['atomicAmount'],intent['recipientAddress']):
            return False
    return True


for x in e['consistencyExamples']:
    assert validator(x['schema']).is_valid(x['value']), x['name']
    assert consistent(x['value']) == x['expectedConsistent'], x['name']
apis = {a['id']: a for a in load('api-catalog.json')['operations']}
commands = {b['name']: b for b in load('ble-catalog.json')['commands']}
w = json.loads((ROOT.parent/'planning/work-breakdown.json').read_text())
for op in p['operations']:
    a = apis[op['apiId']]
    assert (op['method'], op['path'], op['authorization']) == (a['method'], a['path'], a['authorization'])
    assert set(op['taskRefs']) <= {t['id'] for t in w['tasks']}
    for k in ('requestSchema', 'responseSchema', 'errorSchema'):
        assert op[k] in s['$defs'] and any(x['schema'] == op[k] for x in e['valid'])
for b in p['bleMessages']:
    assert b['role'] in commands[b['command']]['roles']
    for k in ('requestSchema', 'responseSchema'):
        assert b[k] in s['$defs'] and any(x['schema'] == b[k] for x in e['valid'])
assert len({x['apiId'] for x in p['operations']}) == len(p['operations']) == 5
assert len({x['command'] for x in p['bleMessages']}) == len(p['bleMessages']) == 5
assert all(x['status'] == 'not_run' for x in e['reviewCases'])
assert len({x['id'] for x in e['reviewCases']}) == len(e['reviewCases'])
assert w['phaseControl']['implementation'] == 'deferred_by_user'
pending = next(x for x in load('return-recovery-contract.json')['openDecisions'] if x['id'] == 'RR-DEC-01')
assert pending['selection'] is None and pending['status'] == 'awaiting_user_preference'
print(json.dumps({'httpCandidates':len(p['operations']),'logicalBlePairs':len(p['bleMessages']),
                  'validShapes':len(e['valid']),'invalidShapes':len(e['invalid']),
                  'fieldConsistencyExamples':len(e['consistencyExamples']),
                  'unexecutedReviewCases':len(e['reviewCases']),
                  'baselineHashes':'matched','canonicalMerged':False,'runtimeVerified':False}))
