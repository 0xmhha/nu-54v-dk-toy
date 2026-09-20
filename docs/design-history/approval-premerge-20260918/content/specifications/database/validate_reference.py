"""Validate only in a fresh disposable PostgreSQL cluster; no existing DB access.

python3 content/specifications/database/validate_reference.py --pg-bin /path/to/bin
Single-user mode opens no application TCP/Unix service and does not test concurrency.
"""
import argparse
import hashlib
import json
import os
import re
import subprocess
import tempfile
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument('--pg-bin', required=True)
args = parser.parse_args()
pg = Path(args.pg_bin)
root = Path(__file__).resolve().parent
catalog = json.loads((root / 'schema-catalog.json').read_text())
for entry in catalog['migrations']:
    assert hashlib.sha256((root / entry['file']).read_bytes()).hexdigest() == entry['sha256']
env = {'PATH': os.environ.get('PATH', '/usr/bin:/bin'), 'LC_ALL': 'C'}
checks = []
with tempfile.TemporaryDirectory(prefix='nu-schema-review-', dir='/private/tmp') as tmp:
    data = Path(tmp) / 'cluster'
    run = subprocess.run([str(pg / 'initdb'), '-D', str(data), '--no-locale', '--encoding=UTF8',
                          '--auth=trust', '--username=nu_schema_review'], env=env,
                         text=True, capture_output=True, timeout=60)
    if run.returncode:
        raise RuntimeError(run.stderr[-4000:])

    def sql(source, label):
        result = subprocess.run([str(pg / 'postgres'), '--single', '-D', str(data), '-j', 'postgres'],
                                env=env, input=source.rstrip()+'\n\n', text=True,
                                capture_output=True, timeout=30)
        output = result.stdout + result.stderr
        if result.returncode or re.search(r'\b(ERROR|FATAL|PANIC):', output):
            raise RuntimeError(label + '\n' + output[-5000:])
        return output

    for migration in catalog['migrations']:
        sql((root / migration['file']).read_text(), migration['file'])
    output = sql("SELECT count(*) AS table_count FROM information_schema.tables WHERE table_schema='wallet_platform';", 'table_count')
    assert re.search(r'table_count = "62"', output), output
    sql((root / 'constraint-fixtures.sql').read_text(), 'synthetic_fixtures')
    cases = json.loads((root / 'constraint-cases.json').read_text())
    for case in cases:
        if case['expected'] == 'accept':
            statement = 'SET search_path=wallet_platform,pg_catalog;\n'+case['sql']
        else:
            assert case['sqlstate'] in ('23503','23505','23514')
            statement = f"""SET search_path=wallet_platform,pg_catalog;
DO $test$
BEGIN
  BEGIN
    {case['sql']}
    RAISE EXCEPTION 'Expected rejection did not occur: {case['id']}';
  EXCEPTION WHEN SQLSTATE '{case['sqlstate']}' THEN
    NULL;
  END;
END
$test$;
"""
        sql(statement, case['id'])
        checks.append(dict(id=case['id'],expected=case['expected'],result='pass'))
    version = subprocess.run([str(pg / 'postgres'), '--version'],env=env,text=True,capture_output=True,check=True).stdout.strip()
report = dict(mode='fresh_temporary_cluster_single_user',serverVersion=version,
              businessTables=61,migrationsApplied=7,constraintCases=checks,
              scope='DDL execution and sequential database constraints only; no multi-session locking, authorization or product tests',
              existingDatabaseTouched=False,networkListenerStarted=False,temporaryClusterRemoved=True,
              migrationHashes={m['file']:m['sha256'] for m in catalog['migrations']})
(root / 'validation-result.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
print(json.dumps(dict(serverVersion=version,migrations=7,businessTables=61,passedCases=len(checks),
                     temporaryClusterRemoved=True,existingDatabaseTouched=False),ensure_ascii=False))
