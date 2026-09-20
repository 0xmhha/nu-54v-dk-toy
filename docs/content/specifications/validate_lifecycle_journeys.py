"""Traceability and source preservation checks, not an end-to-end product test."""
import hashlib,json
from pathlib import Path
P=Path(__file__).resolve().parent
ROOT=P.parents[1]

def read(n):return json.loads((P/n).read_text())
def need(ok,msg):
    if not ok:raise ValueError(msg)

d=read('lifecycle-journey-acceptance.json')
need(d['implementation']=='deferred_by_user' and not any(d[k]for k in ['canonicalMerged','runtimeVerified','sqlApplied']),'design status')
for name,h in d['sourceHashes'].items():need(hashlib.sha256((P/name).read_bytes()).hexdigest()==h,'source drift '+name)
p=ROOT/'content/planning/work-breakdown.json';need(hashlib.sha256(p.read_bytes()).hexdigest()==d['sourceWbsHash'],'WBS drift');w=json.loads(p.read_text());tasks={x['id']for x in w['tasks']}
need(len(tasks)==d['baseline']['wbsTasks']==104,'scope tasks');need(w['subtaskCount']==d['baseline']['wbsSteps']==320,'scope steps')
srcs=['return-route-contracts.json','rental-admission-cancel.json','lifecycle-bootstrap-contracts.json','lifecycle-storage-design.json','enrollment-continuity-adoption.json']
cases={};routes=set();atoms=set()
for f in srcs:
 x=read(f)
 for c in x.get('runtimeCases',x.get('recoveryCases',[])):
  need(c['id']not in cases,'duplicate case');cases[c['id']]=(f,c)
 routes|={y['id']for y in x.get('operations',[])+x.get('bleCommands',[])}
 atoms|={y['id']for y in x.get('atomicUnits',[])}
jj={j['id']:j for j in d['journeys']};need(len(jj)==len(d['journeys'])==16,'journey count')
for j in jj.values():
 need(j['caseRefs'] and set(j['caseRefs'])<=cases.keys(),'case ref '+j['id'])
 need(set(j['routeRefs'])<=routes,'route ref '+j['id']);need(set(j['atomicRefs'])<=atoms,'atomic ref')
 need(set(j['taskRefs'])<=tasks,'WBS task ref')
 need(j['runtimeStatus']=='not_run' and not j['evidenceRefs'],'invented execution')
 need(j['normalControlExpected'] and j['verdictRule'] and j['forbiddenOutcomes'],'weak verdict')
 need(j['testPassWhen'] and j['uiOracle']['userVisibleSuccessWhen'] and 'successOnlyWhen'not in j['uiOracle'],'test/UI conflation')
coverage={x['caseId']:x for x in d['caseCoverage']};need(len(coverage)==107 and coverage.keys()==cases.keys(),'107-case source coverage')
for k,x in coverage.items():
 need(x['sourceFile']==cases[k][0] and x['sourceStatus']==cases[k][1]['status']=='not_run','case provenance')
 need(set(x['journeyRefs'])=={j['id']for j in jj.values()if k in j['caseRefs']},'coverage backref')
finished=set()
for a in d['adoptionPackages']:
 need(set(a['dependsOn'])<=finished,'adoption dependency');finished.add(a['id'])
 need(a['runtimeStatus']=='not_run' and not a['implementationAuthorized'],'unearned adoption')
 need(all((P/f).is_file()for f in a['sources']),'adoption source')
need(len(finished)==5 and len(d['openGaps'])==5,'adoption/gaps')
m=d['runManifestTemplate'];need(m['result']=='not_run' and m['runId']is None and not m['evidenceRefs'],'fake manifest')
q=next(x for x in read('return-recovery-contract.json')['openDecisions']if x['id']=='RR-DEC-01');need(q['selection']is None and q['status']=='awaiting_user_preference','policy changed')
print(json.dumps({'scope':'journey_traceability_only','journeys':16,'uniqueSourceCasesLinked':107,'adoptionPackages':5,'openGaps':5,'sourceFilesPreserved':8,'wbsTasks':104,'wbsSteps':320,'runtimeRuns':0,'canonicalMerged':False},ensure_ascii=False))
