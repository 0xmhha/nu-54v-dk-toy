"""Validate design links and finite arithmetic/ordering examples only.
No product implementation, DB, messaging or device runtime is exercised.
"""
import hashlib
import json
import re
from pathlib import Path
P=Path(__file__).resolve().parent

def read(name):return json.loads((P/name).read_text())
def require(ok,msg):
    if not ok:raise ValueError(msg)

d=read('commerce-consumer-repair-design.json');e=read('commerce-consumer-repair-examples.json')
require(d['implementation']=='deferred_by_user','implementation gate')
require(not any(d[k] for k in ['canonicalMerged','runtimeVerified','sqlApplied']),'unearned execution status')
for name,h in d['sourceFiles'].items():require(hashlib.sha256((P/name).read_bytes()).hexdigest()==h,'source changed: '+name)
require({x['consumer'] for x in d['consumers']}=={'sales','settlements','benefits','receipts','travel'},'consumer coverage')
require(len(d['resources'])==6 and len(d['rebuildSteps'])==7,'resource or rebuild coverage')
known_events={x['type']:set(x['consumers']) for x in read('event-catalog.json')['events']}
for s in d['subscriptionDeltas']:
    require(set(s['existingConsumers'])==known_events[s['event']],'subscription baseline drift')
known_apis={x['apiId'] for x in read('api-access-transactions.json')['operations']}
for c in d['consumers']:
    require(set(c['apiRefs'])<=known_apis,'unknown API')
    require(set(c['eventTypes'])<=known_events.keys(),'unknown event')
seen=set()
for step in d['rebuildSteps']:
    require(step['id'] not in seen and set(step['dependsOn'])<=seen,'rebuild ordering')
    seen.add(step['id'])
p=d['publicationAtomicUnit']
require(p['preparationMayPublish'] is False and p['commitStep']=='CP-B06','premature publication')
require(set(p['sameCommit'])=={'activeGenerationPointer','newFence','sourceContributions','correctionRecords','consumerApplications','checkpoint','outbox'},'partial publication design')
# Synthetic arithmetic: positive source amounts, signed correction/net outputs.
def integer(s):
    require(isinstance(s,str) and re.fullmatch(r'0|-?[1-9][0-9]*',s) is not None,'noncanonical integer')
    return int(s)
for x in e['contributionCases']:
    b={k:integer(v) for k,v in x['before'].items()};t={k:integer(v) for k,v in x['target'].items()}
    require(all(v>=0 for v in [*b.values(),*t.values()]),'negative raw contribution')
    require({k:str(t[k]-b[k]) for k in t}==x['expectedDelta'],x['id']+' delta')
    require(str(t['gross']-t['salesRefund'])==x['expectedNetSales'],x['id']+' sales net')
    require(str(t['receipts']-t['refundOutflow'])==x['expectedNetCash'],x['id']+' cash net')
for x in e['benefitCases']:
    require(x['newTargetEarned']-x['oldTargetEarned']==x['expectedCorrection'],x['id']+' correction')
    bal=x['newTargetEarned']-x['consumed']
    require(bal==x['expectedBalance'] and (bal<0)==x['expectedDeficitHold'],x['id']+' preserved consume')
for x in e['publicationCases']:
    allowed=x['complete'] and x['captured']==x['current'] and x['expectedFence']==x['currentFence'] and not x['deny']
    require(allowed==x['expected'],x['id']+' publication condition')
for x in e['eventCases']:
    a,i=x['appliedRevision'],x['incomingRevision']
    actual=('duplicate' if x['sameDigest'] else 'quarantine') if i==a else ('stale_reconcile' if i<a else ('gap_reconcile' if i>a+1 else 'capture_current_target'))
    require(actual==x['expected'],x['id']+' ordering')
examples=[x for k in ['contributionCases','benefitCases','publicationCases','eventCases'] for x in e[k]]
require(len({x['id'] for x in examples})==len(examples),'duplicate example ID')
require(len(d['runtimeCases'])==22 and all(x['status']=='not_run' for x in d['runtimeCases']),'runtime status')
text=(P/'commerce-consumer-repair-design.md').read_text()
require(all(x['id'] in text for k in ['consumers','resources','rebuildSteps','runtimeCases'] for x in d[k]),'reading view missing section')
print(json.dumps({'scope':'design_links_and_finite_examples_only','consumers':len(d['consumers']),'logicalResources':len(d['resources']),'rebuildSteps':len(d['rebuildSteps']),'syntheticExamples':len(examples),'runtimeCasesNotRun':len(d['runtimeCases']),'unchangedSourceFiles':len(d['sourceFiles']),'canonicalMerged':False,'sqlApplied':False},ensure_ascii=False))
