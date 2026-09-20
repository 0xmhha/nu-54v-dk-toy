"""Validate a review candidate's local shapes/references and illustrative invariants.
No network, product implementation, database mutation, or runtime acceptance tests.
"""
import hashlib
import json
from pathlib import Path
from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource

ROOT = Path(__file__).resolve().parent
load = lambda name: json.loads((ROOT / name).read_text())
proposal = load('commerce-contract-candidate.json')
schema = load(proposal['schemaFile'])
examples = load('commerce-contract-examples.json')
assert proposal['status'] == 'review_candidate_not_merged'
assert proposal['runtimeVerified'] is False
for file, digest in proposal['baselineFiles'].items():
    assert hashlib.sha256((ROOT / file).read_bytes()).hexdigest() == digest, f'Baseline changed: {file}'
registry = Registry()
for uri, path in proposal['externalSchemaBindings'].items():
    baseline = load(path)
    baseline['$id'] = uri
    registry = registry.with_resource(uri, Resource.from_contents(baseline))
Draft202012Validator.check_schema(schema)

def validator(name):
    return Draft202012Validator(dict(schema, **{'$ref': '#/$defs/'+name}), registry=registry, format_checker=FormatChecker())

for category in ('valid', 'invalid'):
    for item in examples[category]:
        errors = list(validator(item['schema']).iter_errors(item['value']))
        assert bool(errors) == (category == 'invalid'), (item['name'], [e.message for e in errors])

# These finite design assertions do not check permissions, signatures, concurrency,
# observation truth, state-machine execution, or complete business acceptance.
def source_valid(source):
    fields = ['policyRefundable', 'reserved', 'confirmed', 'available']
    if any(source[f]['assetId'] != source['assetId'] for f in fields):
        return False
    cap, reserved, confirmed, available = [int(source[f]['atomicAmount']) for f in fields]
    return cap == reserved + confirmed + available and (not source['authorizationHeld'] or bool(source['holdReasons']))

def semantic_valid(kind, value):
    if kind == 'API030Data':
        summary = value['paymentSummary']
        if summary['canStartPayment'] and (value['order']['state'] != 'awaiting_payment' or summary['state'] != 'unpaid'):
            return False
        sources = value.get('refundSources', [])
        return all(source_valid(s) for s in sources) and len({s['paymentId'] for s in sources}) == len(sources)
    if kind == 'API036Body':
        return int(value['amount']['atomicAmount']) > 0
    if kind == 'API038Data':
        refund, recon = value['refund'], value['reconciliation']
        if recon['revision'] != refund['revision']:
            return False
        if refund['state'] == 'confirmed' and recon['reservationState'] != 'consumed':
            return False
        if refund['state'] in ('requested','authorized','signing','submitted','unknown') and recon['reservationState'] != 'active':
            return False
        if 'refundSource' in value:
            source = value['refundSource']
            return source_valid(source) and source['paymentId'] == refund['paymentId'] and source['assetId'] == refund['assetId']
    if kind in ('PaymentAcceptanceEvent','RefundStateEvent'):
        payload = value['payload']
        identifier = 'paymentId' if kind == 'PaymentAcceptanceEvent' else 'refundId'
        return value['aggregateId'] == payload[identifier] and ((payload['sourceObservationId'] is None) == (payload['sourceObservationRevision'] is None))
    return True

for item in examples['valid']:
    assert semantic_valid(item['schema'], item['value']), item['name']
for item in examples['semanticCases']:
    assert validator(item['schema']).is_valid(item['value']), 'Semantic case fails shape: '+item['name']
    assert semantic_valid(item['schema'], item['value']) == item['expectedValid'], item['name']
for item in examples['accountingCases']:
    def available(row):
        return row['cap'] - row['reserved'] - row['confirmed']
    assert (available(item['before']) == available(item['after'])) == item['expectedSameAvailable'], item['name']

api = {x['id']: x for x in load('api-catalog.json')['operations']}
events = {x['type'] for x in load('event-catalog.json')['events']}
wbs = json.loads((ROOT.parent/'planning/work-breakdown.json').read_text())
tasks = {t['id'] for t in wbs['tasks']}
for item in proposal['operations'] + proposal['events']:
    assert item['candidateSchema'] in schema['$defs']
    assert set(item['taskRefs']) <= tasks
for item in proposal['operations']:
    assert item['apiId'] in api
for item in proposal['events']:
    assert item['type'] in events
assert len(api) == 107 and len(events) == 10
assert len(proposal['logicalResources']) == 4
assert wbs['phaseControl']['implementation'] == 'deferred_by_user'
print(json.dumps({'candidateApis':len(proposal['operations']), 'candidateEvents':len(proposal['events']), 'logicalResources':4, 'validShapes':len(examples['valid']), 'rejectedShapes':len(examples['invalid']), 'semanticExamples':len(examples['semanticCases']), 'accountingExamples':len(examples['accountingCases']), 'baselineHashes':'matched', 'canonicalMerged':False, 'runtimeTests':'not_run'}, ensure_ascii=False))
