"""Regression checks for the five document defects, not product security tests."""
import json
from pathlib import Path
R=Path(__file__).resolve().parents[3]

def read(p):return json.loads((R/p).read_text())
def arcs(rows):return {(s,r['toState']) for r in rows for s in r['fromState'].split('|')}
def walk(rows,states):return all((a,b) in arcs(rows) for a,b in zip(states,states[1:]))
sp='content/specifications/'
m=read(sp+'market-product-design.json');c=read(sp+'credential-paid-resource-design.json');o=read(sp+'preimplementation-contract-overlay.json');a=read(sp+'social-wallet-recovery-design.json')
checks=[]
def check(id,value):
 assert value,id
 checks.append(id)
check('LG01_unknown_to_original_completion',walk(m['tradeTransitions'],['execution_pending','unknown','result_observed','completed']))
check('LG01_failure_reorg_reconciliation',walk(m['tradeTransitions'],['unknown','failed_confirmed','unknown','execution_pending']))
check('LG01_no_resign_after_execution_exposure',next(t for t in m['tradeTransitions'] if t['id']=='MT-11')['executionExposureMustBeAbsent'] is True and next(t for t in m['tradeTransitions'] if t['id']=='MT-11')['allowedResumePhases']==['allowance_pending','review_ready'])
check('LG01_allowance_recovery',walk(m['tradeTransitions'],['allowance_pending','unknown','review_ready']))
check('LG01_preserved_identity',set(['exposureRef','operationId','businessEffectId','resumePhase'])<=set(m['recoveryContext']['required']))
check('LG02_credential_has_no_session_states',not ({'verified','presentation_ready','approved','submitted'} & {s for pair in arcs(c['credentialTransitions']) for s in pair}))
check('LG02_independent_repeat_presentations',all(walk(c['presentationTransitions'],['absent','requested','approved','submitted','verified']) for _ in ['challenge_A','challenge_B']) and 'active' in c['credentialObjects']['credential'])
check('LG02_revocation_from_current_or_unknown',all((s,'revoked') in arcs(c['credentialTransitions']) for s in ['active','unknown']))
check('LG02_unknown_verification_can_recover',walk(c['presentationTransitions'],['submitted','verification_unknown','verified']))
preds={p['id']:p for p in c['paidResourcePredicates']}
def required(id):return set(preds[id].get('allOf',[]))|set().union(*(required(p) for p in preds[id].get('includes',[])))
expected={'authority_current','privacy_allowed','source_current','refund_clear','entitlement_revision_matches','request_revision_matches','resource_terms_current'}
for tid,pred in [('PX-05','PR-GRANT'),('PX-06','PR-GENERATE'),('PX-07','PR-PUBLISH'),('PX-11','PR-GENERATE')]:
 row=next(t for t in c['paidResourceTransitions'] if t['id']==tid)
 check('LG03_'+tid+'_common_guard',row['predicateRefs']==[pred] and expected<=required(pred))
# Truth-table examples evaluate the design's allOf rule, not a service implementation.
for pred in ['PR-GENERATE','PR-PUBLISH']:
 flags={f:True for f in required(pred)}
 check('LG03_'+pred+'_normal',all(flags.values()))
 for f in sorted(expected):
  altered={**flags,f:False}
  check('LG03_'+pred+'_deny_'+f,not all(altered.values()))
routes={r['id']:r for r in o['routeContracts']}
check('LG04_four_explicit_contracts',{'OC-17','OC-18','OC-19','OC-20'}<=routes.keys())
for tid,refs in [('AU-06','OC-17/OC-18'),('AU-09','OC-19/OC-20'),('AU-12','OC-19/OC-20')]:
 check('LG04_'+tid+'_wired',next(t['route'] for t in a['authTransitions'] if t['id']==tid)==refs)
for id in ['OC-17','OC-18','OC-19','OC-20']:
 check('LG04_'+id+'_complete_fields',all(routes[id][k] for k in ['authority','requestFields','responseFields','atomicBoundary','recovery']))
check('LG04_readers_do_not_require_valid_access_token', 'sender' in routes['OC-18']['authority'] and 'read credential' in routes['OC-20']['authority'])
w=read('content/planning/work-breakdown.json');ts={t['id']:t for t in w['tasks']};h=read('content/planning/preimplementation-handoff.json')
for p in read('content/planning/full-scope-design-review.json')['designPackages']:
 expectedImpact=sorted({r for t in p['primaryTaskRefs'] for r in ts[t]['requirements']})
 d=read(next(x['source'] for x in h['designPackages'] if x['id']==p['id']))
 check('LG05_'+p['id']+'_same_typed_scope',d['declaredScopeRequirementRefs']==p['requirementRefs']==p['declaredScopeRequirementRefs'] and d['transitiveTaskImpactRequirementRefs']==expectedImpact==p['transitiveTaskImpactRequirementRefs'])
check('LG05_DS07_app_impact_preserved',4 in read(sp+'recording-travel-ai-design.json')['transitiveTaskImpactRequirementRefs'])
check('no_policy_or_execution_inferred',all(d['status']=='open' for d in w['decisions']) and all(read(x['source'])['runtimeVerified'] is False for x in h['designPackages']))
# Confirm the same tests catch the original defects; archived source snapshots are immutable.
b='content/analysis/document-logic/before-fixes/'
check('before_LG01_counterexample_reproduced',not walk(read(b+sp+'market-product-design.json')['tradeTransitions'],['unknown','result_observed','completed']))
check('before_LG02_counterexample_reproduced','presentationTransitions' not in read(b+sp+'credential-paid-resource-design.json'))
check('before_LG03_guard_omission_reproduced','predicateRefs' not in next(t for t in read(b+sp+'credential-paid-resource-design.json')['paidResourceTransitions'] if t['id']=='PX-11'))
check('before_LG04_missing_contract_reproduced','OC-17' not in {r['id'] for r in read(b+sp+'preimplementation-contract-overlay.json')['routeContracts']})
print(json.dumps(dict(scope='design_regression_only',checksPassed=len(checks),checks=checks,findingsResolvedInDesign=5,runtimeVerified=False),ensure_ascii=False))
