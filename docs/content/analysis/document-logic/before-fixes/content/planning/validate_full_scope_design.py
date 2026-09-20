"""Validate scope traceability; not runtime readiness, schedule feasibility or effort."""
import hashlib,json
from pathlib import Path
P=Path(__file__).resolve().parent;R=P.parents[1]

def read(path):return json.loads((R/path).read_text())
def need(ok,msg):
    if not ok:raise ValueError(msg)

d=read('content/planning/full-scope-design-review.json');w=read('content/planning/work-breakdown.json')
need(d['implementation']=='deferred_by_user' and not any(d[k]for k in ['canonicalMerged','runtimeVerified','sqlApplied']),'design only')
for path,h in d['sourceHashes'].items():need(hashlib.sha256((R/path).read_bytes()).hexdigest()==h,'source drift '+path)
tasks={t['id']:t for t in w['tasks']};reqs={x['requirement']:x for x in d['requirements']};need(set(reqs)==set(range(1,16)),'requirements')
need(len(tasks)==104 and w['subtaskCount']==320,'WBS preserved')
tech={x['id']for x in read('content/planning/technology-selection.json')['choices']};ifs={x['id']for x in read('content/specifications/implementation-interfaces.json')['interfaces']};dec={x['id']:x for x in w['decisions']}
for n,x in reqs.items():
 expected={t['id']for t in tasks.values()if n in t['requirements']and t['epic']!='VERIFY'}
 verify={t['id']for t in tasks.values()if n in t['requirements']and t['epic']=='VERIFY'}
 need(set(x['taskRefs'])==expected and set(x['verificationTaskRefs'])==verify,'task mapping '+str(n))
 need(set(x['interfaceRefs'])<=ifs and set(x['technologyRefs'])<=tech,'interface/tech refs')
 need(set(x['taskDecisionRefs'])=={dd for t in expected for dd in tasks[t]['decisionInputs']},'task decision mapping')
 need(set(x['decisionRefs'])==set(x['taskDecisionRefs']+x['policyDecisionRefs']) and set(x['decisionRefs'])<=dec.keys(),'policy decision mapping')
 need(x['completionPercentage']is None and not x['runtimeVerified'],'false maturity score')
 need(all((R/p).is_file()for p in x['sourceFiles']),'source path')
for n,count in [(3,4),(11,9),(13,3)]:
 fs=[x['facet']for x in d['compoundRequirementFacets']if x['requirement']==n]
 need(len(fs)==len(set(fs))==count and set(fs)==set(reqs[n]['facets']),'compound requirement missing '+str(n))
packages={x['id']:x for x in d['designPackages']};need(len(packages)==8,'package count')
done=set()
for x in d['designPackages']:
 need(set(x['designDependsOn'])<=done,'dependency/order');done.add(x['id'])
 need(set(x['focusTaskRefs']+x['primaryTaskRefs'])<=tasks.keys(),'package task ref')
 need(set(x['decisionRefs']+x['taskDecisionRefs'])<=dec.keys(),'package decision ref')
 need(x['owner']is None and x['effortEstimate']is None and not x['implementationAuthorized'],'staffing/implementation inferred')
routes={x['taskId']:x for x in d['taskRouting']};need(len(routes)==len(d['taskRouting'])==104 and routes.keys()==tasks.keys(),'all task coverage')
for tid,x in routes.items():
 need(x['primaryDesignPackage']in packages and tid in packages[x['primaryDesignPackage']]['primaryTaskRefs'],'task reverse mapping')
 need(x['requirements']==tasks[tid]['requirements'] and x['owner']is None and x['effortEstimate']is None,'task modification')
need(len(d['decisions'])==19,'decisions')
for x in d['decisions']:need(x['id']in dec and x['sourceStatus']==dec[x['id']]['status'] and not x['resolved'],'policy selected')
q=next(x for x in read('content/specifications/return-recovery-contract.json')['openDecisions']if x['id']=='RR-DEC-01');need(q['selection']is None,'recovery policy')
need(d['lifecycleResiduals']==read('content/specifications/lifecycle-journey-acceptance.json')['openGaps'],'residuals lost')
print(json.dumps({'scope':'full_scope_design_traceability_only','requirements':15,'compoundFacets':16,'designPackages':8,'wbsTasksMappedOnce':104,'wbsStepsPreserved':320,'openDecisions':19,'sourceFilesPreserved':len(d['sourceHashes']),'runtimeVerified':False},ensure_ascii=False))
