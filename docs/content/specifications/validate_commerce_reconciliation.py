"""Validate current-compatible design envelopes and finite semantic examples.
No DB, product service, signature verification, event delivery or concurrency is run.
"""
from pathlib import Path
import hashlib
import json
from jsonschema import Draft202012Validator, FormatChecker
P=Path(__file__).resolve().parent

def read(name):return json.loads((P/name).read_text())
def require(ok,message):
    if not ok:raise ValueError(message)

d=read('commerce-reconciliation-integration.json')
s=read(d['schemaFile']); f=read(d['examplesFile'])
require(not any(d[k] for k in ['canonicalMerged','runtimeVerified','sqlApplied']), 'unsupported completion claim')
require(d['implementation']=='deferred_by_user','implementation gate changed')
Draft202012Validator.check_schema(s)
def walk(v):
    if isinstance(v,dict):
        if '$ref' in v:
            r=v['$ref'];require(r.startswith('#/$defs/') and r.split('/')[-1] in s['$defs'],'external or broken schema ref')
        for x in v.values():walk(x)
    elif isinstance(v,list):
        for x in v:walk(x)
walk(s)
for group,expected in [('valid',True),('invalid',False)]:
    for x in f[group]:
        checker=Draft202012Validator({'$ref':'#/$defs/'+x['schema'],'$defs':s['$defs']},format_checker=FormatChecker())
        errors=list(checker.iter_errors(x['value']))
        require((not errors)==expected, f"{group}: {x['name']}: " + '; '.join(e.message for e in errors[:2]))
for name,h in d['sourceHashes'].items():
    require(hashlib.sha256((P/name).read_bytes()).hexdigest()==h,'source drift: '+name)
# API037 must still accept exactly the copied current contract, with rewritten local refs only.
a=read('approval-baseline.schema.json')['$defs']
def prefix(v):
    if isinstance(v,list):return [prefix(x) for x in v]
    if isinstance(v,dict):return {k:('#/$defs/A_'+x.split('/')[-1] if k=='$ref' else prefix(x)) for k,x in v.items()}
    return v
for name,val in a.items():
    if 'A_'+name in s['$defs']:require(s['$defs']['A_'+name]==prefix(val),'approval definition changed: '+name)
for side in ['Request','Response']:
    require(s['$defs']['API037'+side]=={'$ref':'#/$defs/A_AB_integration_API037'+side}, 'API037 diverged')
# Small semantic model: these examples establish arithmetic/link predicates only.
def evaluate(rule,v):
    if rule=='refund_source_scope':return v['pathOrder']==v['allocationOrder'] and v['authorizedStore']==v['allocationStore'] and v['requestAsset']==v['allocationAsset']
    if rule=='new_reservation':return v['expectedRevision']==v['currentRevision'] and not v['held'] and 0<v['amount']<=v['cap']-v['reserved']-v['confirmed']
    if rule=='reorg_preserves_exposure':
        b=v['before'];a=v['after'];q=v['orphanedRefundAmount']
        return 0<q<=b['confirmed'] and a['cap']==b['cap'] and a['reserved']==b['reserved']+q and a['confirmed']==b['confirmed']-q and a['reserved']+a['confirmed']<=a['cap']
    if rule=='receipt_allocation_scope':return v['refundPayment']==v['receiptPayment']
    if rule=='event_identity':return v['aggregateId']==v['payloadId'] and (v['observationId'] is None)==(v['observationRevision'] is None)
    if rule=='source_cursor':return all(v[k] for k in ['principalMatches','storeMatches','orderMatches','canRequestRefund']) and v['cursorRevision']==v['sourceListRevision']
    raise ValueError('unknown predicate '+rule)
for x in f['semanticCases']:require(evaluate(x['rule'],x['facts'])==x['expected'],'semantic case: '+x['id'])
require({x['apiId'] for x in d['operations']}=={'API-030','API-035','API-036','API-037','API-038'},'route coverage')
for x in d['operations']:
    require(all(x[k] in s['$defs'] for k in ['requestSchema','responseSchema','errorSchema']),'route schema linkage')
require(all(c['status']=='not_run' for c in d['runtimeCases']),'runtime scenario status')
require(len(d['storage'])==4 and len(d['transactions'])==4 and len(d['events'])==2,'integration coverage')
print(json.dumps({'scope':'design_only','httpRoutes':len(d['operations']),'schemaDefinitions':len(s['$defs']),'validShapes':len(f['valid']),'invalidShapes':len(f['invalid']),'semanticExamples':len(f['semanticCases']),'runtimeCasesNotRun':len(d['runtimeCases']),'pinnedSources':len(d['sourceHashes']),'canonicalMerged':False,'sqlApplied':False},ensure_ascii=False))
