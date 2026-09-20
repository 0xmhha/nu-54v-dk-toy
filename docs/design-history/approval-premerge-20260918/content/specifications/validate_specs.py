"""Validate planning references and synthetic schema fixtures, not runtime code.

Run: python3 content/specifications/validate_specs.py
Requires the jsonschema Python package already present in this workspace runtime.
"""
import json
import re
import hashlib
from datetime import datetime
from pathlib import Path
from jsonschema import Draft202012Validator, FormatChecker

root = Path(__file__).resolve().parent
load = lambda name: json.loads((root / name).read_text())
wbs = json.loads((root.parent / 'planning/work-breakdown.json').read_text())
task_ids = {t['id'] for t in wbs['tasks']}
api = load('api-catalog.json')['operations']
ble = load('ble-catalog.json')['commands']
events = load('event-catalog.json')['events']
assert len({(a['method'], a['path']) for a in api}) == len(api)
assert len({a['id'] for a in api}) == len(api)
assert len({b['name'] for b in ble}) == len(ble)
assert len({e['type'] for e in events}) == len(events)
for entry in api + ble + events:
    assert set(entry['taskRefs']) <= task_ids, entry
for a in api:
    assert a['authorization']
    if a['method'] != 'GET':
        assert a['idempotency'] == 'required'

schema = load('core.schema.json')
Draft202012Validator.check_schema(schema)
fixtures = load('examples.json')
for verdict in ('valid', 'invalid'):
    for sample in fixtures[verdict]:
        target = dict(schema, **{'$ref': '#/$defs/' + sample['schema']})
        validator = Draft202012Validator(target, format_checker=FormatChecker())
        errors = list(validator.iter_errors(sample['value']))
        assert bool(errors) == (verdict == 'invalid'), (sample['name'], [e.message for e in errors])

# The JSON schema role filters and the logical BLE inventory must agree.
base = fixtures['valid'][5]['value']
commands_by_name = {b['name']: b for b in ble}
for role in ('owner', 'payment_terminal', 'ranging_peer'):
    for cmd in schema['$defs']['BleControlMessage']['properties']['command']['enum']:
        sample = dict(base, sessionRole=role, command=cmd)
        validator = Draft202012Validator(dict(schema, **{'$ref': '#/$defs/BleControlMessage'}))
        permitted = role in commands_by_name[cmd]['roles']
        assert validator.is_valid(sample) == permitted, (role, cmd)

screens = load('screen-flows.json')['screens']
flows = load('common-flows.json')['flows']
api_ids = {a['id'] for a in api}
ble_names = {b['name'] for b in ble}
flow_ids = {f['id'] for f in flows}
assert len({s['id'] for s in screens}) == len(screens)
for screen in screens:
    assert set(screen['taskRefs']) <= task_ids
    assert set(screen['apiRefs']) <= api_ids
    assert set(screen['bleCommands']) <= ble_names
    assert all(screen['states'].get(k) for k in ('submitting', 'success', 'errorOrEmpty', 'resume'))
    if 'signerFlowRef' in screen:
        assert screen['signerFlowRef'] in flow_ids
for flow in flows:
    assert set(flow['apiRefs']) <= api_ids
    assert set(flow['bleCommands']) <= ble_names

critical = load('critical-dtos.json')
critical_schema = load('critical-dtos.schema.json')
assert critical['sharedSourceSha256'] == hashlib.sha256((root / 'core.schema.json').read_bytes()).hexdigest()
Draft202012Validator.check_schema(critical_schema)
for contract in critical['contracts']:
    assert contract['apiId'] in api_ids
    assert contract['requestSchema'] in critical_schema['$defs']
    assert contract['responseSchema'] in critical_schema['$defs']
for verdict in ('valid', 'invalid'):
    for sample in critical[verdict]:
        target = dict(critical_schema, **{'$ref': '#/$defs/' + sample['schema']})
        validator = Draft202012Validator(target, format_checker=FormatChecker())
        errors = list(validator.iter_errors(sample['value']))
        assert bool(errors) == (verdict == 'invalid'), (sample['name'], [e.message for e in errors])
for sample in critical['semanticCases']:
    assert sample['rule'] == 'intent_expiry_within_quote_and_session'
    times = [datetime.fromisoformat(sample[k].replace('Z', '+00:00'))
             for k in ('intentExpiry', 'quoteExpiry', 'sessionExpiry')]
    accepted = times[0] <= min(times[1:])
    assert accepted == (sample['expected'] == 'accept'), sample['name']

samples = {s['name']: s['value'] for s in critical['valid']}
quote = samples['API-032-response']['body']['data']['quote']
intent = samples['API-034-response']['body']['data']['intent']
assert intent['expiresAt'] <= quote['expiresAt']
assert intent['atomicAmount'] == quote['atomicAmount']
assert intent['recipientAddress'] == quote['recipientAddress']
refund_intent = samples['API-037-response']['body']['data']['signingIntent']
refund = samples['API-036-response']['body']['data']['refund']
assert refund_intent['refundId'] == refund['refundId']
assert refund_intent['atomicAmount'] == refund['atomicAmount']
assert refund_intent['recipientAddress'] == refund['destinationAddress']
assert 'attemptId' not in refund_intent

extended = load('extended-dtos.json')
extended_schema = load('extended-dtos.schema.json')
extended_cases = load('extended-examples.json')
assert extended['sourceSha256'] == hashlib.sha256((root / 'critical-dtos.schema.json').read_bytes()).hexdigest()
Draft202012Validator.check_schema(extended_schema)
critical_ids = {c['apiId'] for c in critical['contracts']}
extended_ids = {c['apiId'] for c in extended['contracts']}
assert len(extended_ids) == len(extended['contracts'])
assert not critical_ids & extended_ids
assert critical_ids | extended_ids == api_ids
catalog_by_id = {a['id']: a for a in api}
for contract in extended['contracts']:
    source = catalog_by_id[contract['apiId']]
    assert contract['method'] == source['method'] and contract['path'] == source['path']
    assert {f['name'] for f in contract['inputFields']} == set(source['inputFields'])
    assert {f['name'] for f in contract['outputFields']} == set(source['outputFields'])
    for key in ('requestSchema', 'responseSchema'):
        assert contract[key] in extended_schema['$defs']
for verdict in ('valid', 'invalid'):
    for sample in extended_cases[verdict]:
        target = dict(extended_schema, **{'$ref': '#/$defs/' + sample['schema']})
        validator = Draft202012Validator(target, format_checker=FormatChecker())
        errors = list(validator.iter_errors(sample['value']))
        assert bool(errors) == (verdict == 'invalid'), (sample['name'], [e.message for e in errors])

for case in extended_cases.get('semanticCases', []):
    assert case['apiId'] in api_ids
    assert case['rule'] == 'pending_and_materialized_sources_disjoint'
    disjoint = not set(case['pendingSourceIds']) & set(case['materializedSourceIds'])
    assert disjoint == (case['expected'] == 'accept')

db = json.loads((root / 'database/schema-catalog.json').read_text())
db_result = json.loads((root / 'database/validation-result.json').read_text())
assert len({t['name'] for t in db['tables']}) == len(db['tables']) == 61
for table in db['tables']:
    assert set(table['taskRefs']) <= task_ids
for migration in db['migrations']:
    actual = hashlib.sha256((root / 'database' / migration['file']).read_bytes()).hexdigest()
    assert actual == migration['sha256'] == db_result['migrationHashes'][migration['file']]
assert len(db_result['constraintCases']) == 19
assert all(c['result'] == 'pass' for c in db_result['constraintCases'])

access = load('api-access-transactions.json')['operations']
policies = load('authorization-policies.json')['policies']
authorization_cases = load('authorization-scenarios.json')['scenarios']
security_inputs = load('security-integration-inputs.json')['inputs']
tables = {t['name'] for t in db['tables']}
assert len(access) == len({a['apiId'] for a in access}) == 107
assert {a['apiId'] for a in access} == api_ids
policy_ids = {p['id'] for p in policies}
assert len(policies) == len(policy_ids) == 57
assert policy_ids == {a['authorization'] for a in api}
families = {f'TX-{i:02}' for i in range(1, 17)} | {
    'READ', 'SECURITY_STORE', 'INTENT_CREATION', 'REQUEST_SUBMISSION',
    'STORE_CONFIGURATION', 'QUOTE_CREATION', 'RECEIPT_CLAIM'}
for entry in access:
    source = catalog_by_id[entry['apiId']]
    for key in ('method', 'path', 'authorization'):
        assert entry[key] == source[key], (entry['apiId'], key)
    assert entry['transactionFamily'] in families, entry['apiId']
    for key in ('readTables', 'writeTables', 'commonInfrastructureWrites'):
        assert set(entry[key]) <= tables, (entry['apiId'], key)
        assert len(entry[key]) == len(set(entry[key]))
    assert set(entry['taskRefs']) == set(source['taskRefs'])
    if entry['method'] == 'GET':
        assert not entry['writeTables'], entry['apiId']
for policy in policies:
    assert policy['defaultDecision'] == 'deny' and policy['predicate']
    assert set(policy['apiRefs']) == {
        a['id'] for a in api if a['authorization'] == policy['id']}
assert len({s['id'] for s in authorization_cases}) == len(authorization_cases)
for case in authorization_cases:
    assert case['policyId'] == catalog_by_id[case['apiId']]['authorization']
    assert case['expectedDecision'] in ('allow', 'deny')
    assert case['verificationStatus'] == 'not_run'
    assert case['given'] and case['expectedEffects'] and case['forbiddenEffects']
assert len({g['id'] for g in security_inputs}) == len(security_inputs) == 5
for gap in security_inputs:
    assert set(gap['taskRefs']) <= task_ids
    assert gap['status'] == 'design_specified_pending_adapter' and not gap['runtimeVerified']
for task in wbs['tasks']:
    assert set(task['accessApiRefs']) == {
        a['apiId'] for a in access if task['id'] in a['taskRefs']}
    assert set(task['securityIntegrationInputs']) == {
        g['id'] for g in security_inputs if task['id'] in g['taskRefs']}

security_design = load('security-integration-design.json')['modules']
security_cases = load('security-integration-cases.json')['cases']
security_addendum = load('security-api-addendum.json')
extra_apis = security_addendum['operations']
extra_ids = {a['id'] for a in extra_apis}
gap_ids = {g['id'] for g in security_inputs}
assert len(security_design) == len({m['gapId'] for m in security_design}) == 5
assert {m['gapId'] for m in security_design} == gap_ids
assert len(extra_apis) == len(extra_ids) == security_addendum['supplementalApiCount'] == 5
assert security_addendum['baselineApiCount'] == 102
assert security_addendum['canonicalApiCount'] == len(api) == 107
assert security_addendum['totalProposedPaths'] == len(api) == 107
all_paths = [(a['method'], a['path']) for a in api]
assert len(all_paths) == len(set(all_paths))
assert len(security_cases) == len({c['id'] for c in security_cases}) == 30
for case in security_cases:
    assert case['gapId'] in gap_ids
    assert case['verificationStatus'] == 'not_run'
    assert case['when'] and case['expected'] and case['evidenceRequired']
for entry in extra_apis:
    assert entry['gapId'] in gap_ids and entry['authorization']
    assert entry['status'] == 'merged_into_canonical_design' and not entry['runtimeVerified']
    assert entry['idempotency'] == ('not_applicable' if entry['method'] == 'GET' else 'required')
    for key in ('inputFields', 'outputFields'):
        fields = entry[key]
        assert len(fields) == len({f['name'] for f in fields})
        assert all(f['type'] and f['meaning'] and isinstance(f['required'], bool) for f in fields)
    assert set(re.findall(r'\{([^}]+)\}', entry['path'])) <= {f['name'] for f in entry['inputFields']}
for module in security_design:
    gap = next(g for g in security_inputs if g['id'] == module['gapId'])
    assert module['status'] == 'design_specified' and not module['runtimeVerified']
    assert set(module['taskRefs']) == set(gap['taskRefs'])
    assert set(module['existingApiRefs']) <= api_ids
    assert set(module['supplementalAliases']) == {
        a['id'] for a in extra_apis if a['gapId'] == module['gapId']}
    assert set(module['acceptanceRefs']) == set(gap['acceptanceRefs']) == {
        c['id'] for c in security_cases if c['gapId'] == module['gapId']}
    assert module['openSelections'] and module['logicalRecords']
    for transition in module['transitions']:
        assert transition['source'] in module['states'] and transition['target'] in module['states']
        assert transition['guard']
    assert len(module['commands']) == len({c['name'] for c in module['commands']})
    assert all(c['input'] and c['output'] and c['invariant'] for c in module['commands'])
for change in security_addendum['integrationChanges']:
    assert set(change['apiRefs']) <= api_ids
for task in wbs['tasks']:
    assert set(task['securityAcceptanceRefs']) == {
        c['id'] for c in security_cases
        if any(m['gapId'] == c['gapId'] and task['id'] in m['taskRefs'] for m in security_design)}

storage_resources = load('security-storage-contracts.json')['resources']
resource_ids = {r['id'] for r in storage_resources}
assert len(storage_resources) == len(resource_ids) == 15
assert not resource_ids & tables
for resource in storage_resources:
    assert resource['kind'] == 'logical_adapter_resource' and not resource['runtimeVerified']
    assert set(resource['taskRefs']) <= task_ids and resource['invariant']
for row in access:
    assert set(row['adapterReads'] + row['adapterWrites']) <= resource_ids
    if row['method'] == 'GET':
        assert not row['adapterWrites']
for task in wbs['tasks']:
    assert set(task['securityStorageRefs']) == {r['id'] for r in storage_resources if task['id'] in r['taskRefs']}
for old in extra_apis:
    canonical = catalog_by_id[old['canonicalApiId']]
    assert canonical['originAlias'] == old['id']
    assert canonical['path'] == old['path'] and canonical['method'] == old['method']
    assert any(canonical['id'] in s['apiRefs'] for s in screens)
for module in security_design:
    assert set(module['integratedApiRefs']) == {a['canonicalApiId'] for a in extra_apis if a['gapId'] == module['gapId']}
for contract in extended['contracts']:
    assert contract['authorization'] == catalog_by_id[contract['apiId']]['authorization']

for doc in root.rglob('*.md'):
    for link in re.findall(r'\]\(([^)]+)\)', doc.read_text()):
        if '://' in link or link.startswith('#'):
            continue
        assert (doc.parent / link.split('#')[0]).exists(), (doc.name, link)

print(json.dumps(dict(apiOperations=len(api), bleCommands=len(ble), events=len(events),
                      schemas=len(schema['$defs']), acceptedExamples=len(fixtures['valid']),
                      rejectedExamples=len(fixtures['invalid']),
                      screenFlows=len(screens), detailedApiContracts=len(critical['contracts']),
                      detailedAcceptedExamples=len(critical['valid']), detailedRejectedExamples=len(critical['invalid']),
                      semanticCases=len(critical['semanticCases']),
                      extendedContracts=len(extended['contracts']),
                      extendedAcceptedExamples=len(extended_cases['valid']),
                      extendedRejectedExamples=len(extended_cases['invalid']),
                      extendedSemanticExamples=len(extended_cases.get('semanticCases', [])),
                      apiTypeCoverage=f'{len(api)}/{len(api)} JSON envelopes; profile verifiers and binary transport require implementation',
                      roleCommandMatrix='consistent', taskReferences='valid',
                      accessMappings=len(access), authorizationPolicies=len(policies),
                      authorizationScenarioSpecifications=len(authorization_cases),
                      authorizationRuntimeTests='not_run', pendingSecurityInputs=len(security_inputs),
                      securityDesignModules=len(security_design), securityAcceptanceSpecifications=len(security_cases),
                      supplementalApiProposals=len(extra_apis), supplementalSchemaMerge='integrated_design_runtime_unverified', logicalAdapterResources=len(storage_resources),
                      validationScope='design_shapes_and_references_only'), ensure_ascii=False))
