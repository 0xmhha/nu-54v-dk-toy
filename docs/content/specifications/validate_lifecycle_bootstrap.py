"""Validate local draft shapes/references; not consent, cryptography or device tests."""
import hashlib,json
from pathlib import Path
from jsonschema import Draft202012Validator,FormatChecker
P=Path(__file__).resolve().parent

def rd(n):return json.loads((P/n).read_text())
def need(ok,msg):
    if not ok:raise ValueError(msg)

d=rd('lifecycle-bootstrap-contracts.json');s=rd(d['schemaFile']);e=rd(d['examplesFile'])
need(d['implementation']=='deferred_by_user' and not any(d[k]for k in ['canonicalMerged','runtimeVerified','sqlApplied']),'design status')
Draft202012Validator.check_schema(s)
def refs(x):
    if isinstance(x,dict):
        if '$ref'in x:need(x['$ref'].startswith('#/$defs/') and x['$ref'].split('/')[-1]in s['$defs'],'local ref')
        for v in x.values():refs(v)
    elif isinstance(x,list):
        for v in x:refs(v)
refs(s)
for group,expected in [('valid',True),('invalid',False)]:
    for x in e[group]:
        errors=list(Draft202012Validator({'$ref':'#/$defs/'+x['schema'],'$defs':s['$defs']},format_checker=FormatChecker()).iter_errors(x['value']))
        need((not errors)==expected,x['name']+str([er.message for er in errors[:2]]))
for name,h in d['sourceHashes'].items():need(hashlib.sha256((P/name).read_bytes()).hexdigest()==h,'source drift '+name)
for op in d['operations']+d['bleCommands']:
    need(op['registration']=='alias_not_catalogued','alias adopted')
    need(all(op[k]in s['$defs']for k in ['requestSchema','responseSchema']),'schema mapping')
known={x['id']for x in d['operations']}
for f in ['rental-admission-cancel.json','return-route-contracts.json']:known|={x['id']for x in rd(f)['operations']}
need(all(x['routeId']in known for x in d['pendingRouteOverlays']),'pending route')
need('enrollmentProofRef'in s['$defs']['API045RequestDelta']['required'],'verified enrollment reference')
need(len(s['$defs']['LW01Request']['properties']['headers']['oneOf'])==2,'typed LW auth')
need(len(d['runtimeCases'])==22 and all(x['status']=='not_run'for x in d['runtimeCases']),'runtime claims')
q=next(x for x in rd('return-recovery-contract.json')['openDecisions']if x['id']=='RR-DEC-01');need(q['selection']is None and q['status']=='awaiting_user_preference','recovery decision changed')
print(json.dumps({'scope':'design_shapes_and_references_only','httpAliases':len(d['operations']),'bleAliases':len(d['bleCommands']),'pendingRouteOverlays':len(d['pendingRouteOverlays']),'validShapes':len(e['valid']),'invalidShapes':len(e['invalid']),'unchangedInputs':len(d['sourceHashes']),'runtimeCasesNotRun':len(d['runtimeCases']),'canonicalMerged':False},ensure_ascii=False))
