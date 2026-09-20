"""Inventory/reference checks only. No wire-schema, cryptography, SQL or device test."""
import hashlib,json
from pathlib import Path
P=Path(__file__).resolve().parent

def read(n):return json.loads((P/n).read_text())
def need(ok,msg):
    if not ok:raise ValueError(msg)

d=read('enrollment-continuity-adoption.json')
need(d['implementation']=='deferred_by_user' and not any(d[k]for k in ['canonicalMerged','runtimeVerified','sqlApplied']),'design status')
for n,h in d['sourceHashes'].items():need(hashlib.sha256((P/n).read_bytes()).hexdigest()==h,'source drift '+n)
need(len(d['operations'])==5 and len(d['bleCommands'])==3,'route coverage')
for x in d['operations']+d['bleCommands']:need(x['registration']=='alias_not_catalogued','alias registration')
messages={m['name']:m for m in d['messages']};need(len(messages)==6,'signed payload coverage')
for m in messages.values():need(len(m['requiredPayloadFields'])==len(set(m['requiredPayloadFields'])) and m['profileStatus']=='unselected','message fields/profile')
for name in ['ResumeChallenge','ResumePrepared','ContinuationLease','ContinuationApplied']:need({'admissionId','originalContextDigest'}<=set(messages[name]['requiredPayloadFields']),'lost root context')
for name in ['HoldInstruction','HoldApplied']:need('consentAuthorizationRevision'in messages[name]['requiredPayloadFields'],'missing hold revision')
registry={x['table']for x in d['tables']};need(len(registry)==7,'registry tables')
need(set(read('lifecycle-bootstrap-contracts.json')['logicalResources'])<={x['resource']for x in d['tables']},'unmapped prior logical resource')
prior={x['name']for x in read('lifecycle-storage-design.json')['tables']}
shared=prior|registry|{'approval_authorization_gates','audit_events','outbox'}
known={x['id']for x in d['operations']}
for f in ['lifecycle-bootstrap-contracts.json','rental-admission-cancel.json']:known|={x['id']for x in read(f)['operations']}
units={u['id']:u for u in d['atomicUnits']};need(len(units)==8,'units')
for u in units.values():
 need(set(u['routes'])<=known,'route ref')
 need(set(u['registryWrites'])<=registry,'unknown registry')
 need(set(u['existingCandidateWrites'])<=prior,'unknown existing candidate')
 need(set(u['sharedWrites'])<=shared,'unknown shared table')
 need(set(u['existingSqlWrites'])<={'rentals','devices','device_bindings'},'unknown baseline effect')
 need(not u['runtimeVerified'],'runtime claim')
need({'lifecycle_admissions','device_lifecycle_heads'}<=set(units['ECT03']['existingCandidateWrites']),'generation commit incomplete')
need({'rentals','device_bindings','devices'}<=set(units['ECT07']['existingSqlWrites']),'reservation baseline missing')
need(len(d['evidenceParentMapping'])==3 and all(x['message']in messages and x['ownerTable']in registry for x in d['evidenceParentMapping']),'typed evidence mapping')
need(len(d['acceptance'])==12 and all(x['runtimeStatus']=='not_run'for x in d['acceptance']),'acceptance claims')
need(len(d['runtimeCases'])==18 and all(x['status']=='not_run'for x in d['runtimeCases']),'runtime cases')
q=next(x for x in read('return-recovery-contract.json')['openDecisions']if x['id']=='RR-DEC-01');need(q['selection']is None and q['status']=='awaiting_user_preference','policy drift')
print(json.dumps({'scope':'design_inventory_and_references_only','httpAliases':5,'bleAliases':3,'signedMessageSpecifications':6,'registryTableProposals':7,'atomicMappings':8,'adoptionCriteria':12,'runtimeCasesNotRun':18,'unchangedInputs':8,'canonicalMerged':False},ensure_ascii=False))
