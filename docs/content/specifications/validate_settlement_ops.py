"""Design envelopes/links only. No server, database or approval workflow execution."""
import hashlib,json
from pathlib import Path
from jsonschema import Draft202012Validator,FormatChecker
P=Path(__file__).resolve().parent

def read(n):return json.loads((P/n).read_text())
def require(ok,msg):
    if not ok:raise ValueError(msg)

d=read('settlement-ops-contracts.json');s=read(d['schemaFile']);e=read(d['examplesFile'])
require(d['implementation']=='deferred_by_user' and not any(d[k] for k in ['canonicalMerged','runtimeVerified','sqlApplied']),'implementation status')
Draft202012Validator.check_schema(s)
def refs(v):
    if isinstance(v,dict):
        if '$ref'in v:require(v['$ref'].startswith('#/$defs/') and v['$ref'].split('/')[-1]in s['$defs'],'broken/external ref')
        for x in v.values():refs(x)
    elif isinstance(v,list):
        for x in v:refs(x)
refs(s)
for group,expected in [('valid',True),('invalid',False)]:
    for x in e[group]:
        validator=Draft202012Validator({'$ref':'#/$defs/'+x['schema'],'$defs':s['$defs']},format_checker=FormatChecker())
        errors=list(validator.iter_errors(x['value']))
        require((not errors)==expected,x['name']+': '+str([er.message for er in errors[:2]]))
# Cross-field arithmetic of synthetic response metrics, not a runtime calculator.
def metrics(v):
    if isinstance(v,dict):
        if 'netSalesAtomic'in v:
            require(int(v['netSalesAtomic'])==int(v['grossAcceptedAtomic'])-int(v['confirmedSalesRefundAtomic']),'sales arithmetic')
            require(int(v['netAttributedCashAtomic'])==int(v['canonicalAttributedReceiptsAtomic'])-int(v['allCanonicalRefundOutflowAtomic']),'cash arithmetic')
        for x in v.values():metrics(x)
    elif isinstance(v,list):
        for x in v:metrics(x)
for x in e['valid']:metrics(x['value'])
for name,h in d['sourceHashes'].items():require(hashlib.sha256((P/name).read_bytes()).hexdigest()==h,'source drift '+name)
a={x['apiId']:x for x in read('api-access-transactions.json')['operations']}
require(len(a)==d['baselineApiCount']==110,'baseline count')
require(d['registeredNewApiCount']==0,'alias cannot be registered claim')
for op in d['operations']:
    require(all(op[k]in s['$defs'] for k in ['requestSchema','responseSchema','errorSchema']),'schema route link')
    if op['id'].startswith('API-'):
        require(op['method']==a[op['id']]['method'] and op['path']==a[op['id']]['path'],'existing path mismatch')
    else:require(op['registration']=='alias_only_not_catalogued','alias status')
require(d['publicationScope']['cardinality']=='one_consumer_one_projection_per_repair','publication scope')
for node in [s['$defs']['Review'],s['$defs']['RepairView']]:
    require({'consumer','projectionKey'}<=set(node['required']),'missing consumer/projection binding')
    require('consumers' not in node['properties'],'multiple consumers under single fence')
require('consumer' in s['$defs']['API087Request']['properties']['body']['required'],'consumer selector missing')
require(len(d['runtimeCases'])==18 and all(x['status']=='not_run'for x in d['runtimeCases']),'runtime cases')
print(json.dumps({'scope':'design_only','existingRoutes':5,'proposedUnregisteredRoutes':2,'validShapes':len(e['valid']),'invalidShapes':len(e['invalid']),'runtimeCasesNotRun':18,'unchangedInputs':len(d['sourceHashes']),'canonicalMerged':False},ensure_ascii=False))
