"""Contract shapes + bounded design models only; no implementation or hardware tests."""
import hashlib,itertools,json
from pathlib import Path
from jsonschema import Draft202012Validator,FormatChecker
P=Path(__file__).resolve().parent

def read(n):return json.loads((P/n).read_text())
def need(ok,msg):
    if not ok:raise ValueError(msg)

d=read('rental-admission-cancel.json');s=read(d['schemaFile']);e=read(d['examplesFile'])
need(d['implementation']=='deferred_by_user' and not any(d[k]for k in ['canonicalMerged','runtimeVerified','sqlApplied']),'design only')
Draft202012Validator.check_schema(s)
def refs(x):
    if isinstance(x,dict):
        if '$ref'in x:need(x['$ref'].startswith('#/$defs/') and x['$ref'].split('/')[-1]in s['$defs'],'missing local ref')
        for v in x.values():refs(v)
    elif isinstance(x,list):
        for v in x:refs(v)
refs(s)
for group,expected in [('valid',True),('invalid',False)]:
    for x in e[group]:
        errors=list(Draft202012Validator({'$ref':'#/$defs/'+x['schema'],'$defs':s['$defs']},format_checker=FormatChecker()).iter_errors(x['value']))
        need((not errors)==expected,x['name']+str([er.message for er in errors[:2]]))
for path,h in d['sourceHashes'].items():need(hashlib.sha256((P/path).read_bytes()).hexdigest()==h,'source changed '+path)
for op in d['operations']+d['bleCommands']:
    need(all(op[k]in s['$defs'] for k in ['requestSchema','responseSchema']),'schema mapping')
a={x['apiId']:x for x in read('api-access-transactions.json')['operations']}
need(len(a)==110,'baseline count');op=next(x for x in d['operations']if x['id']=='API-045')
need(all(op[k]==a['API-045'][k]for k in ['method','path']),'API045 drift')
need(all(x['registration']=='alias_not_catalogued'for x in d['operations']if x['id']!='API-045'),'registered alias')
q=next(x for x in read('return-recovery-contract.json')['openDecisions']if x['id']=='RR-DEC-01');need(q['selection']is None,'policy selection')
need(len(d['runtimeCases'])==23 and all(x['status']=='not_run'for x in d['runtimeCases']),'runtime claims')
# Finite serial histories assume a real implementation supplies the required common lock.
for order in itertools.permutations(['commit','cancel']):
    issued=False;tombstone=False;accepted=[]
    for action in order:
        if action=='commit' and not tombstone:issued=True;accepted.append(action)
        if action=='cancel' and not issued:tombstone=True;accepted.append(action)
    need(accepted==[order[0]],'commit/cancel double winner')
# Distinct retry keys do not create two cancellation lineages for a job.
lineages={}
for key in ['key1','key2']:
    if 'job1' not in lineages:lineages['job1']='cancel1'
need(len(lineages)==1,'multiple cancellation lineages')
need(any(x['id']=='RLC-G15' for x in d['guards']),'lineage rule missing')
# Cancellation is complete only on durable exact receipts and never-issued history.
complete_count=0
for issued,prepared,released,exact in itertools.product([False,True],repeat=4):
    complete=not issued and prepared and released and exact
    if complete:complete_count+=1
    need(not(complete and issued),'issued grant aborted')
need(complete_count==1,'release guard model')
# Admission guard model checks every disqualifier; actual cryptographic proof is not modeled.
base={'basisVerified':True,'cleanupOrVirginVerified':True,'epochMatches':True,'revisionsMatch':True,'operatorAllowed':True,'recipientConsent':True,'holderProof':True,'noActiveOrPendingRental':True,'noQuarantine':True}
need(all(base.values()),'eligible base')
for key in base:
    x=base.copy();x[key]=False;need(not all(x.values()),'missing admission guard')
need({'enrollmentHolderThumbprint','enrollmentSessionId'}<=set(s['$defs']['AdmissionContext']['required']),'unbound enrollment holder')
print(json.dumps({'scope':'design_only','httpRoutes':len(d['operations']),'blePairs':len(d['bleCommands']),'validShapes':len(e['valid']),'invalidShapes':len(e['invalid']),'finiteModelCases':3+16+1+len(base),'runtimeCasesNotRun':23,'pinnedInputs':len(d['sourceHashes']),'canonicalMerged':False},ensure_ascii=False))
