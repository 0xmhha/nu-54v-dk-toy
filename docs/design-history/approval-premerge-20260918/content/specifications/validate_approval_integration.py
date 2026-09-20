"""Validate design-only HTTP envelopes, cross references and finite projections.

No running API, current ACL, sender proof, cryptography or storage race tests.
"""
import copy
import hashlib
import json
from pathlib import Path

from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource

ROOT = Path(__file__).resolve().parent
load = lambda p: json.loads((ROOT / p).read_text())
m = load('approval-integration-candidate.json')
s = load(m['schemaFile'])
e = load(m['examplesFile'])
assert m['canonicalMerged'] is m['runtimeVerified'] is False
assert m['implementation'] == 'deferred_by_user' and m['policySelectionUnchanged']
for name, digest in m['baselineFiles'].items():
    assert hashlib.sha256((ROOT / name).read_bytes()).hexdigest() == digest, name
registry = Registry()
documents = {s['$id']: s}
for uri, file in m['externalSchemaBindings'].items():
    doc = load(file)
    doc['$id'] = uri
    documents[uri] = doc
    registry = registry.with_resource(uri, Resource.from_contents(doc))


def check_refs(node, current):
    if isinstance(node, dict):
        if '$ref' in node:
            uri, fragment = node['$ref'].split('#', 1)
            doc = documents[uri or current]
            for part in fragment.lstrip('/').split('/'):
                doc = doc[part.replace('~1', '/').replace('~0', '~')]
        for value in node.values():
            check_refs(value, current)
    elif isinstance(node, list):
        for value in node:
            check_refs(value, current)


for uri, doc in documents.items():
    Draft202012Validator.check_schema(doc)
    check_refs(doc, uri)


def validator(name):
    return Draft202012Validator(dict(s, **{'$ref': '#/$defs/' + name}), registry=registry,
                                format_checker=FormatChecker())


for category in ('valid', 'invalid'):
    for fixture in e[category]:
        errors = list(validator(fixture['schema']).iter_errors(fixture['value']))
        assert bool(errors) == (category == 'invalid'), (fixture['name'], [x.message for x in errors])

apis = {x['id']: x for x in load('api-catalog.json')['operations']}
screens = {x['id']: x for x in load('screen-flows.json')['screens']}
ble = {x['name'] for x in load('ble-catalog.json')['commands']}
assert len(apis) == 107 and len(screens) == 37 and len(ble) == 34
route_ids = {x['id'] for x in m['routes']}
assert len(route_ids) == len(m['routes']) == 13
for route in m['routes']:
    if route['id'].startswith('API-'):
        a = apis[route['id']]
        assert (route['method'], route['path'], route['baselineAuthorization']) == (a['method'], a['path'], a['authorization'])
    else:
        assert route['path'] not in {a['path'] for a in apis.values()}
    for key in ('requestSchema', 'responseSchema', 'errorSchema'):
        assert route[key] in s['$defs']
        if key != 'errorSchema':
            assert any(x['schema'] == route[key] for x in e['valid']), route['id']
assert {x['routeId'] for x in m['authorizationMappings']} == route_ids
assert {x['screenId'] for x in m['screenMappings']} == {x['screenId'] for x in load('approval-contract-merge-plan.json')['directlyReferencingScreens']}
for x in m['screenMappings']:
    assert x['title'] == screens[x['screenId']]['title'] and x['appliedToBaseline'] is False
    assert set(x['routeRefs']) <= route_ids and set(x['bleRefs']) <= ble
resource_ids = {x['id'] for x in m['resources']}
for x in m['atomicUnits']:
    assert set(x['resourceRefs']) <= resource_ids and x['runtimeVerified'] is False
assert all(x['status'] == 'not_run' for x in m['reviewCases'])


def projection_matches(request, response):
    """Finite fixture field consistency only; not authorization or cryptography."""
    data = response['body']['data']
    if request['path']['operationId'] != data['operation']['operationId']:
        return False
    projection = data['approval']
    if projection is None:
        return not any(request['query'].values())
    if not request['query'].get('includeSnapshot', False) and projection['snapshot'] is not None:
        return False
    if not request['query'].get('includeSigningResult', False) and projection['signingResult'] is not None:
        return False
    snapshot = projection['snapshot']
    if snapshot:
        context = snapshot['context']
        if context['operationId'] != data['operation']['operationId']:
            return False
        if any(projection['progress'][k] != context[k] for k in ('intentId', 'approvalContextId')):
            return False
    return True


get = lambda name: copy.deepcopy(next(x['value'] for x in e['valid'] if x['name'] == name))
req, res = get('request_020'), get('response_020_projection_personal')
assert projection_matches(req, res)
bad = copy.deepcopy(req)
bad['query'] = {}
assert not projection_matches(bad, res)
bad = copy.deepcopy(res)
bad['body']['data']['operation']['operationId'] = 'other_parent'
assert not projection_matches(req, bad)
bad = copy.deepcopy(res)
bad['body']['data']['approval']['progress']['intentId'] = 'other_intent'
assert not projection_matches(req, bad)
pending = next(x for x in load('return-recovery-contract.json')['openDecisions'] if x['id'] == 'RR-DEC-01')
assert pending['selection'] is None and pending['status'] == 'awaiting_user_preference'
print(json.dumps({'httpRoutes': len(m['routes']), 'validShapes': len(e['valid']),
                  'invalidShapes': len(e['invalid']), 'finiteProjectionChecks': 4,
                  'screenMappings': len(m['screenMappings']), 'storageResources': len(m['resources']),
                  'atomicUnits': len(m['atomicUnits']), 'unexecutedReviewCases': len(m['reviewCases']),
                  'baselineHashes': 'matched', 'canonicalMerged': False, 'runtimeVerified': False}))
