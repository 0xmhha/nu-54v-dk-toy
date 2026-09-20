"""Current approval design validation; never a runtime/security certification."""
import copy
import hashlib
import json
from pathlib import Path
from jsonschema import Draft202012Validator, FormatChecker

P = Path(__file__).resolve().parent
R = P.parents[1]
load = lambda f: json.loads((P / f).read_text())
m = load('approval-baseline.json')
s = load(m['schemaFile'])
e = load(m['examplesFile'])
assert m['canonicalMerged'] is m['archiveCreated'] is True
assert m['runtimeVerified'] is m['physicalSqlChanged'] is False
assert m['implementation'] == 'deferred_by_user' and m['policySelectionUnchanged']
Draft202012Validator.check_schema(s)


def refs(node):
    if isinstance(node, dict):
        if '$ref' in node:
            assert node['$ref'].startswith('#/$defs/'), node['$ref']
            assert node['$ref'].split('/')[-1] in s['$defs']
        for value in node.values():
            refs(value)
    elif isinstance(node, list):
        for value in node:
            refs(value)


refs(s)
def valid(name, value):
    return Draft202012Validator(dict(s, **{'$ref':'#/$defs/'+name}), format_checker=FormatChecker()).is_valid(value)


for category in ('valid', 'invalid'):
    for x in e[category]:
        assert valid(x['schema'], x['value']) == (category == 'valid'), x['name']
apis = {x['id']:x for x in load('api-catalog.json')['operations']}
access = {x['apiId']:x for x in load('api-access-transactions.json')['operations']}
assert len(apis) == len(access) == m['apiCount'] == 110
schemas = [load('critical-dtos.schema.json'), load('extended-dtos.schema.json'), load('core.schema.json')]
for doc in schemas:
    for name, definition in s['$defs'].items():
        assert doc['$defs'][name] == definition, name
for aid, bindings in m['httpSchemas'].items():
    assert apis[aid]['requestSchema'] == bindings['requestSchema']
    assert apis[aid]['responseSchema'] == bindings['responseSchema']
    for field in ('requestSchema','responseSchema'):
        assert any(x['schema'] == bindings[field] for x in e['valid']), (aid, field)
for alias, aid in m['aliasMapping'].items():
    assert apis[aid]['originAlias'] == alias
    assert access[aid]['authorization'] == apis[aid]['authorization']
for aid in ('API-017','API-034','API-037'):
    assert {'transaction_intents','operations'} <= set(access[aid]['writeTables'])
    assert 'ApprovalBinding' in access[aid]['adapterWrites']
for aid in ('API-019','API-020'):
    assert not access[aid]['writeTables'] and not access[aid]['adapterWrites']
    assert access[aid]['authorizationMetadataWrites'] == ['SenderProofReplay']
ble = {x['name']:x for x in load('ble-catalog.json')['commands']}
core = schemas[2]
for pair in m['bleMessages']:
    for key in ('requestSchema','responseSchema'):
        assert ble[pair['command']][key] == pair[key]
        fixture = next(x['value'] for x in e['valid'] if x['schema'] == pair[key])
        checker = Draft202012Validator(dict(core, **{'$ref':'#/$defs/BleControlMessage'}))
        assert checker.is_valid(fixture)
        old = copy.deepcopy(fixture)
        old['protocolVersion'] = 'draft-1'
        assert not checker.is_valid(old), pair['command']
dispatch = load('approval-source-dispatch.json')
assert dispatch['noLegacyFallback']
assert {x['sourceKind'] for x in dispatch['entries'] if x['submissionSchema']} == {'personal_transfer','payment','merchant_refund'}
assert all(x['status'] == 'adapter_pending_execution_blocked' for x in dispatch['entries'] if not x['submissionSchema'])
mapping = load('approval-storage-mapping.json')
resources = {x['id'] for x in load('security-storage-contracts.json')['resources']}
tables = {x['name'] for x in load('database/schema-catalog.json')['tables']}
assert len(resources) == m['logicalStorageCount'] == 28
assert len(mapping['resources']) == 13 and not mapping['sqlApplied']
for x in mapping['resources']:
    assert x['resourceId'] in resources and set(x['existingTables']) <= tables
    assert x['sqlConstraintVerified'] is False
checkpoint = R / 'design-history/approval-premerge-20260918'
assert hashlib.sha256((checkpoint/'checkpoint.json').read_bytes()).hexdigest() == m['checkpointManifestSha256']
assert hashlib.sha256((checkpoint/'validation-report.json').read_bytes()).hexdigest() == m['checkpointReportSha256']
manifest = json.loads((checkpoint/'checkpoint.json').read_text())
for name, digest in manifest['files'].items():
    assert hashlib.sha256((checkpoint/name).read_bytes()).hexdigest() == digest, name
report = json.loads((checkpoint/'validation-report.json').read_text())
assert len(report['results']) == 10 and all(x['exitCode'] == 0 for x in report['results'])
for name in manifest['files']:
    if name.startswith('content/specifications/database/') and name.endswith('.sql'):
        assert hashlib.sha256((R/name).read_bytes()).hexdigest() == manifest['files'][name]
pending = next(x for x in load('return-recovery-contract.json')['openDecisions'] if x['id']=='RR-DEC-01')
assert pending['selection'] is None
assert all(x['status']=='unverified' and not x['testEvidenceRefs'] for x in load('compatibility-matrix.json')['rows'])
print(json.dumps({'scope':'current_canonical_design_only','api':len(apis),'approvalHttpRoutes':len(m['routes']),
                  'blePairs':len(m['bleMessages']),'validShapes':len(e['valid']),'invalidShapes':len(e['invalid']),
                  'checkpointFilesVerified':len(manifest['files']),'historicalChecks':len(report['results']),
                  'storageMappings':len(mapping['resources']),'canonicalMerged':True,
                  'sqlApplied':False,'runtimeVerified':False}))
