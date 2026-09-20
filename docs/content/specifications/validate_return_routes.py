"""Validate design shapes, references and finite invariants, not runtime security."""
import copy, hashlib, json
from pathlib import Path
from jsonschema import Draft202012Validator, FormatChecker
P=Path(__file__).resolve().parent

def read(n): return json.loads((P/n).read_text())
def require(ok,msg):
    if not ok: raise ValueError(msg)

d=read('return-route-contracts.json'); s=read(d['schemaFile']); e=read(d['examplesFile'])
require(d['implementation']=='deferred_by_user' and not any(d[k] for k in ['canonicalMerged','runtimeVerified','sqlApplied']), 'design only')
Draft202012Validator.check_schema(s)
def refs(v):
    if isinstance(v,dict):
        if '$ref' in v: require(v['$ref'].startswith('#/$defs/') and v['$ref'].split('/')[-1] in s['$defs'], 'broken ref')
        for x in v.values(): refs(x)
    elif isinstance(v,list):
        for x in v: refs(x)
refs(s)
for group,expected in [('valid',True),('invalid',False)]:
    for x in e[group]:
        errors=list(Draft202012Validator({'$ref':'#/$defs/'+x['schema'],'$defs':s['$defs']},format_checker=FormatChecker()).iter_errors(x['value']))
        require((not errors)==expected, x['name']+': '+str([er.message for er in errors[:2]]))
for n,h in d['sourceHashes'].items(): require(hashlib.sha256((P/n).read_bytes()).hexdigest()==h,'source drift '+n)
a={x['apiId']:x for x in read('api-access-transactions.json')['operations']}
require(len(a)==d['baselineApiCount']==110,'canonical count')
for op in d['operations']+d['bleCommands']:
    require(all(op[k] in s['$defs'] for k in ['requestSchema','responseSchema']),'schema mapping')
for op in d['operations']:
    if op['id'].startswith('API-'): require(all(op[k]==a[op['id']][k] for k in ['method','path']),'route drift')
    else: require(op['registration']=='alias_not_catalogued','alias registration')
require(len(d['operations'])==10 and len(d['bleCommands'])==6, 'route count')
require(len(d['rpMapping'])==8 and len(d['screenActions'])==12,'coverage')
require(len(d['runtimeCases'])==18 and all(x['status']=='not_run' for x in d['runtimeCases']),'runtime status')
q=next(x for x in read('return-recovery-contract.json')['openDecisions'] if x['id']=='RR-DEC-01')
require(q['selection'] is None and q['status']=='awaiting_user_preference','policy changed')
require('holderKey' in s['$defs']['RB03Request']['properties']['payload']['required'], 'holder public key missing')
require(s['$defs']['API091Request']['properties']['query']['properties']['projection']['const']=='return', 'opt-in projection missing')
require({'type':'null'} in s['$defs']['API091Response']['properties']['body']['properties']['data']['properties']['view']['anyOf'], 'no-job branch missing')
require(s['$defs']['API046Request']['properties']['body']['properties']['externalAccessProof']['anyOf'][0]['$ref']=='#/$defs/ProofReference', 'proof reference drift')
require({'RR-G15','RR-G16','RR-G17'} <= {x['id'] for x in d['guards']}, 'review guards missing')
# Finite logical oracle: no claims about atomic reads or real authorization enforcement.
def eligible(v):
    r=v['readiness']; p=v['progress']
    return bool(v['consistency']['currentness']=='current' and r and
        r['jobId']==p['jobId'] and r['jobRevision']==p['jobRevision'] and
        r['reuseGate']==p['reuseGate']=='eligible' and p['phase']=='completed' and
        r['serverReturnCompleted'] and r['cleanupVerified'] and r['currentDeviceEpochMatches'] and
        not r['hasActiveBinding'] and r['enrollmentState']=='unprovisioned_ready')
v={'consistency':{'currentness':'current'},'progress':{'jobId':'j','jobRevision':3,'reuseGate':'eligible','phase':'completed'},'readiness':{'jobId':'j','jobRevision':3,'reuseGate':'eligible','serverReturnCompleted':True,'cleanupVerified':True,'currentDeviceEpochMatches':True,'hasActiveBinding':False,'enrollmentState':'unprovisioned_ready'}}
require(eligible(v),'eligible fixture')
mutations=[('consistency','currentness','historical'),('progress','phase','evidence_pending'),('readiness','jobId','another'),('readiness','jobRevision',2),('readiness','reuseGate','awaiting_cleanup'),('readiness','serverReturnCompleted',False),('readiness','cleanupVerified',False),('readiness','currentDeviceEpochMatches',False),('readiness','hasActiveBinding',True),('readiness','enrollmentState','quarantined')]
for part,key,value in mutations:
    x=copy.deepcopy(v);x[part][key]=value;require(not eligible(x), 'invalid eligible '+key)
print(json.dumps({'scope':'design_only','httpRoutes':10,'blePairs':6,'validShapes':len(e['valid']),'invalidShapes':len(e['invalid']),'readinessLogicExamples':1+len(mutations),'runtimeCasesNotRun':18,'unchangedInputs':len(d['sourceHashes']),'canonicalMerged':False},ensure_ascii=False))
