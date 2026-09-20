"""Finite design-rule examples for LG06~08; no service or device execution."""
import json
from pathlib import Path
R=Path(__file__).resolve().parents[3]
read=lambda p:json.loads((R/p).read_text())
s='content/specifications/'
c=read(s+'credential-paid-resource-design.json');o=read(s+'preimplementation-contract-overlay.json');p=read(s+'recording-travel-ai-design.json')
preds={r['id']:r for r in c['paidResourcePredicates']}
def requirements(id):
 return set(preds[id].get('allOf',[]))|set().union(*(requirements(x) for x in preds[id].get('includes',[])))
def allowed(id,**overrides):
 facts={k:True for k in requirements(id)};facts.update(overrides)
 return all(facts[k] is True for k in requirements(id))
cases=[]
def check(id,observed,expected,meaning):
 assert observed==expected,(id,observed,expected)
 cases.append(dict(id=id,meaning=meaning,observed=observed,expected=expected,status='passed_design_example'))
for id in ['PR-GRANT','PR-GENERATE','PR-PUBLISH']:
 assert 'payment_currently_confirmed' in requirements(id)
check('LG06-normal',allowed('PR-GENERATE'),True,'현재 지급 확정과 모든 조건 충족')
check('LG06-reorg-before-generation',allowed('PR-GENERATE',payment_currently_confirmed=False),False,'entitlement active여도 지급 불확실이면 시작 금지')
check('LG06-reorg-before-publish',allowed('PR-PUBLISH',payment_currently_confirmed=False),False,'이미 생성된 결과도 현재 지급 불확실이면 공개 금지')
check('LG06-restore-with-refund',allowed('PR-PUBLISH',refund_clear=False),False,'지급 복구가 환불 제한을 해제하지 않음')
check('LG06-restore-with-privacy',allowed('PR-PUBLISH',privacy_allowed=False),False,'지급 복구가 삭제/동의 제한을 해제하지 않음')
check('LG06-revision-race',allowed('PR-GENERATE',request_revision_matches=False),False,'먼저 본 지급 상태가 맞아도 CAS 세대 불일치 거절')
assert c['paidSourceRecovery']['releaseOnlyOwnReason'] and not c['paidSourceRecovery']['deliveryResponseLossChangesPayment']
# Exhaust the contract's disclosure flags. This validates the specified token gate only.
rules=o['refreshResultPolicy'];assert rules['priority']==['caller_bound_proof','family_session_epoch','result_generation','result_material_lifetime']
assert next(r for r in rules['rules'] if r['id']=='RR-03')['state']=='superseded'
assert [r['id'] for r in rules['rules'] if r['returnToken']]==['RR-06']
# rule table: flags correspond to the explicit four ordered checks.
def refresh(proof,epoch,generation_relation,live):
 if not proof:return 'forbidden',False
 if not epoch:return 'revoked',False
 if generation_relation<0:return 'superseded',False
 if generation_relation>0:return 'held',False
 if not live:return 'restart_required',False
 return 'rotated',True
for name,inputs,expected in [('normal',(True,True,0,True),('rotated',True)),('old-generation',(True,True,-1,True),('superseded',False)),('future-generation',(True,True,1,True),('held',False)),('logout-race',(True,False,0,True),('revoked',False)),('lost-sender',(False,True,0,True),('forbidden',False)),('expired-result',(True,True,0,False),('restart_required',False))]:
 check('LG07-'+name,refresh(*inputs),expected,'동일 요청 결과의 현재 공개 정책 예제')
trs={r['id']:r for r in p['privacyTransitions']}
assert trs['PV-02']['predicateRefs']==['PV-DELETE']
conditions=set(p['privacyPredicates'][0]['allOf'])
assert conditions=={'deletion_plan_authorized','target_within_plan_scope','owner_and_policy_revision_current','privacy_revision_matches'}
def delete(**kw):
 facts={k:True for k in conditions};facts.update(kw);return all(facts[k] is True for k in conditions)
check('LG08-withdraw-only',delete(deletion_plan_authorized=False),False,'동의 철회만으로 파일 삭제 불가')
check('LG08-outside-plan',delete(target_within_plan_scope=False),False,'요약 대상 삭제 권한으로 개인 원음까지 확대 금지')
check('LG08-authorized-scope',delete(),True,'명시 삭제 계획과 현재 권한·범위·revision 일치')
check('LG08-stale-plan',delete(privacy_revision_matches=False),False,'이전 scope/revision으로 파괴적 삭제 금지')
assert trs['PV-06']['toState']=='purpose_blocked' and trs['PV-07']['fromState']=='purpose_blocked'
assert not any(r['toState']=='active' and set(r['fromState'].split('|')) & {'deleting','partially_deleted','completed'} for r in p['privacyTransitions'])
assert all(r['status']=='open' for r in read('content/planning/work-breakdown.json')['decisions'])
print(json.dumps(dict(scope='finite_design_examples_only',casesPassed=len(cases),cases=cases,runtimeVerified=False),ensure_ascii=False))
