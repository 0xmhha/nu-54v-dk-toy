"""Check design references/coverage and preserved SQL bytes; never connect to DB.

This is not a SQL, concurrency, cryptography or recovery test.
"""
import hashlib
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent

def read(name):
    return json.loads((HERE / name).read_text())

def require(condition, message):
    if not condition:
        raise ValueError(message)

spec = read('approval-physical-storage-design.json')
mapping = read('approval-storage-mapping.json')
require(spec['implementation'] == 'deferred_by_user', 'implementation must remain deferred')
require(spec['runtimeVerified'] is False and spec['sqlApplied'] is False, 'unearned execution status')
tables = {t['name']: t for t in spec['tables']}
require(len(tables) == len(spec['tables']) == 16, 'duplicate/missing table candidate')
resource_ids = {r['resourceId'] for r in mapping['resources']}
covered = set()
for table in tables.values():
    names = [c['name'] for c in table['columns']]
    require(len(names) == len(set(names)), f"duplicate column in {table['name']}")
    require(all(type(c['nullable']) is bool and c['type'] for c in table['columns']), 'missing type/nullability')
    require(table['databaseConstraints'] and table['transactionResponsibilities'], 'missing responsibility split')
    require(set(table['resourceRefs']) <= resource_ids, 'unknown logical resource')
    covered.update(table['resourceRefs'])
require(covered == resource_ids, 'logical resource not covered')
for r in mapping['resources']:
    actual = {t['name'] for t in tables.values() if r['resourceId'] in t['resourceRefs']}
    require(set(r['physicalDetailTables']) == actual, 'mapping drift')
    require(r['sqlConstraintVerified'] is False, 'SQL verification not performed')
seen = set()
for stage in spec['migrations']:
    require(stage['id'] not in seen and set(stage['dependsOn']) <= seen, 'migration duplicate/forward cycle')
    require(stage['procedure'] and stage['exitGate'], 'missing migration action/gate')
    seen.add(stage['id'])
require(len(seen) == 8, 'missing migration phase')
require(len({x['id'] for x in spec['recoveryCases']}) == 17, 'recovery case IDs')
require(all(x['status'] == 'not_run' and x['trigger'] and x['expected'] for x in spec['recoveryCases']), 'invalid recovery evidence status')
files = {p.name for p in (HERE / 'database').glob('[0-9]*.sql')}
require(files == set(spec['referenceSqlHashes']) and len(files) == 7, 'reference SQL inventory changed')
for name, expected in spec['referenceSqlHashes'].items():
    require(hashlib.sha256((HERE / 'database' / name).read_bytes()).hexdigest() == expected, 'reference SQL bytes changed: ' + name)
# Catch two material contract compatibility regressions found by independent review.
release_cols = {c['name']: c for c in tables['approval_release_decisions']['columns']}
require(release_cols['sender_key_digest']['nullable'] is True, 'primary read must not invent sender key')
require({'auth_path', 'primary_session_ref'} <= release_cols.keys(), 'primary/grant variant missing')
binding_cols = {c['name'] for c in tables['approval_bindings']['columns']}
require({'context_digest','review_digest','snapshot_object_id','snapshot_object_version'} <= binding_cols, 'immutable snapshot storage missing')
text = (HERE / 'approval-physical-storage-design.md').read_text()
require(all(t in text for t in tables), 'table missing from reading view')
require(all(x['id'] in text for x in spec['migrations'] + spec['recoveryCases']), 'case/stage missing from reading view')
require(read('approval-baseline.json')['physicalStorageDesignFile'] == 'approval-physical-storage-design.json', 'baseline pointer drift')
print('Approval physical design OK: 13 resources, 16 table proposals, 8 migration phases, 17 NOT RUN cases; 7 SQL hashes unchanged. No DB/runtime verification.')
