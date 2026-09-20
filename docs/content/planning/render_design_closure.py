"""Validate and render the current handoff addendum. No product readiness inference."""
import argparse, collections, hashlib, json
from pathlib import Path
P=Path(__file__).resolve().parent;R=P.parents[1]
args=argparse.ArgumentParser();args.add_argument('--check',action='store_true');a=args.parse_args()
def load(p):return json.loads((R/p).read_text())
d=load('content/planning/design-closure-review.json')
h=load('content/planning/preimplementation-handoff.json')
w=load('content/planning/work-breakdown.json')
full=load('content/planning/full-scope-design-review.json')
assert d['implementation']=='deferred_by_user'
assert not any(d[k] for k in ['canonicalMerged','runtimeVerified','allImplementationReady'])
assert all(d[k] is None for k in ['ownerAssignment','effortEstimates','completionPercentage'])
for p,v in d['sourceHashes'].items():assert hashlib.sha256((R/p).read_bytes()).hexdigest()==v,p
tasks={t['id']:t for t in w['tasks']};decisions={r['id'] for r in w['decisions']}
assert len(tasks)==104 and sum(len(t['implementationSteps']) for t in tasks.values())==320
assert len(decisions)==19 and all(r['status']=='open' for r in w['decisions'])
assert all(t['owner'] is None and t['effortEstimate'] is None for t in tasks.values())
rr=load('content/specifications/return-recovery-contract.json')
assert next(x for x in rr['openDecisions'] if x['id']=='RR-DEC-01')['selection'] is None
profiles=load('content/specifications/selection-profile-contract.json')
assert all(f['value'] is None for p in profiles['profiles'] for f in p['fields'])
packages={p['id'] for p in full['designPackages']}
for section in ['products','classes','adoptionFamilies','closureSequence','reviewCorrections']:
 assert len(d[section])==len({r['id'] for r in d[section]})
assert len(d['products'])==10
assert {r for p in d['products'] for r in p['requirementRefs']}==set(range(1,16))
assert {t for p in d['products'] for t in p['taskImpactRefs']}==set(tasks)
classes={c['id'] for c in d['classes']};residuals={r['id'] for r in h['designResidualRegister']}
gates={r['id'] for r in h['phaseGates']}
for c in d['classes']:assert c['residualRef'] in residuals and c['gateRef'] in gates
for p in d['products']:
 expected={t['id'] for t in tasks.values() if set(t['requirements'])&set(p['requirementRefs'])}
 assert set(p['taskImpactRefs'])==expected
 assert set(p['taskImpactDecisionRefs'])=={x for t in expected for x in tasks[t]['decisionInputs']}
 assert set(p['focusDecisionRefs'])<=decisions|{'RR-DEC-01'}
 assert set(p['packageRefs'])<=packages and set(p['classRefs'])<=classes
 assert not p['implementationReady'] and not p['runtimeVerified']
 for k in ['preparedLogicalDesign','selectionRequired','afterSelectionDesign','runtimeEvidence','boundary']:assert p[k]
assert d['compoundFacets']==h['compoundRequirementFacets'] and len(d['compoundFacets'])==16
assert d['acceptanceJourneys']==h['crossProductAcceptance'] and len(d['acceptanceJourneys'])==12
assert len(d['runtimeRequirements'])==15
for r in d['runtimeRequirements']:
 original=next(x for x in h['requirements'] if x['requirement']==r['requirement'])
 assert r['runtimeEvidenceNeeded']==original['runtimeEvidenceNeeded'] and r['status']=='not_run'
done=set()
for s in d['closureSequence']:
 assert set(s['dependsOn'])<=done and set(s['classRefs'])<=classes;done.add(s['id'])
for f in d['adoptionFamilies']:assert f['source'] in d['sourceHashes'] and f['status']=='not_adopted'
assert len(load('content/planning/integration-adoption-matrix.json')['bundles'])==8
assert len(load('content/specifications/preimplementation-contract-overlay.json')['routeContracts'])==26
assert len(profiles['profiles'])==20 and sum(len(p['fields']) for p in profiles['profiles'])==82
assert len(load('content/specifications/selection-action-gates.json')['actions'])==53
trust=load('content/specifications/management-trust-lifecycle.json');ops=load('content/specifications/trust-operations-design.json')
assert len(trust['commands'])==8 and len(ops['roles'])==10 and len(ops['panels'])==7
assert len(load('content/specifications/management-trust-evidence.json')['records'])==7
control=load('content/specifications/profile-control-adoption-map.json')
assert len(control['operations'])==7 and len(control['policyCandidates'])==7 and len(control['bleCandidates'])==3

lines=['# 제품별 구현 전 설계 종료·인계 조건 재점검','',d['date']+' · 설계 보완판 · 제품 구현 보류','',d['purpose'],'','## 이번 판정','']
lines+=['- '+s for s in d['conclusions']]
lines+=['','[원 인계서](preimplementation-handoff.md) · [104개 작업 인계](preimplementation-task-handoffs.md) · [선택 결정 자료](decision-briefing.md)','',
 '## 남은 작업 분류','','| 분류 | 닫는 데 필요한 증거 | 완료 근거가 아닌 것 | 기존 항목 |','|---|---|---|']
for c in d['classes']:lines.append(f"| {c['id']} · {c['name']} | {c['doneWhen']} | {c['notEnough']} | {c['residualRef']} / {c['gateRef']} |")
lines+=['','DC-06은 실제 자산·상용 운영으로 전환할 때 적용하는 별도 출시 판단이다. 테스트넷 기반 12주 기능 수용에 상용 출시 완료를 추가하지 않는다. DC-04의 계정/주소 등록도 설계 문서 작성 완료와 구분한다.','',
 '## 제품별 인계 카드','',
 '요구 기반 작업 영향은 제품 사이에 겹칠 수 있다. 아래 연결은 작업 소유권·담당 배정이 아니다. 결정 참조는 주요 검토 범위이며 모든 행위가 그 결정의 모든 필드를 요구한다는 뜻도 아니다. 실제 행위별 조건은 selection-action-gates를 따른다.']
for p in d['products']:
 links=[]
 for ref in p['packageRefs']:
  pkg=next(x for x in h['designPackages'] if x['id']==ref)
  links.append(f"[{ref}](../specifications/{Path(pkg['document']).name})")
 lines += ['',f"### {p['id']} · {p['name']}",'',
           '요구 '+', '.join(map(str,p['requirementRefs']))+' · '+', '.join(links), '',
           '- 작성된 논리 설계: '+p['preparedLogicalDesign'],
           '- 선택 대기: '+p['selectionRequired']+' ('+', '.join(p['focusDecisionRefs'])+')',
           '- 선택 후 구체화: '+p['afterSelectionDesign'],
           '- 구현 단계 검증: '+p['runtimeEvidence'],
           '- 판정 경계: '+p['boundary']]
lines+=['','## 기준 채택에서 빠뜨리지 않을 묶음','','HG-03 채택 범위의 최신 보완 목록이다. 기존 DI/OC 수량에 새 명령 수를 합쳐 정식 API 수량으로 표기하지 않는다.','',
 '| 묶음 | 범위 | 함께 대조할 대상 | 종료 증거 |','|---|---|---|---|']
for f in d['adoptionFamilies']:
 rel='../'+f['source'].removeprefix('content/')
 lines.append(f"| [{f['id']} · {f['name']}]({rel}) | {f['scope']} | {f['targets']} | {f['acceptance']} |")
lines+=['','## 설계 종료와 실행의 순서','','```mermaid','flowchart LR',
 ' A["관련 정책·profile 선택"] --> B["선택 후 구체 설계"]',
 ' B --> C["공통 채택 checkpoint"]',
 ' C --> D["명시적 구현 전환 + task 입력"]',
 ' D --> E["제품 구현·실환경 연결"]',
 ' E --> F["15요구 기능 수용"]',
 ' F -. "실자산 운영 시 별도" .-> G["출시 판단"]','```','',d['sequenceBoundary'],'',
 '| 순서 | 종료 산출물 | 현재 상태 |','|---|---|---|']
for s in d['closureSequence']:lines.append(f"| {s['id']} · {s['name']} | {s['output']} | {s['state']} |")
lines+=['','## 복합 요구 누락 점검','','| 요구 | 세부 기능 | 기준 |','|---|---|---|']
for f in d['compoundFacets']:lines.append(f"| {f['requirement']} | {f['facet']} | {f['completion']} |")
lines+=['','## 통합 수용 여정','','다음 12개 여정은 모두 실행 전이다. 실제 제품·환경 증거는 원 인계서의 기준을 유지한다.','',
 '| ID | 여정 | 실패·복구 조건 |','|---|---|---|']
for e in d['acceptanceJourneys']:lines.append(f"| {e['id']} | {e['flow']} | {e['failureRecovery']} |")
lines+=['','## 보완 사항','']
for c in d['reviewCorrections']:lines.append(f"- **{c['id']}**: {c['issue']} → {c['resolution']}.")
lines+=['','## 검사 결과와 한계','',
 '10개 제품 관점에서 15개 요구·104개 작업의 영향 연결, 16개 복합 기능·12개 통합 여정, 미선택 상태와 참조 해시를 검사한다. 새 제품 코드·실환경 호출·실기 시험·공급자/법규 재조사는 수행하지 않았다. 해당 실검증 항목은 완료 처리하지 않는다. 완료율이나 12주 일정 타당성을 이 검사로 산출하지 않는다.', '',
 '[구조화 보완판](design-closure-review.json) · [검증 기록](design-closure-validation.json) · [운영 책임·승인 화면](../specifications/trust-operations-design.md)', '',d['nextAction']]
text='\n'.join(lines)+'\n'
if a.check:assert (P/'design-closure-review.md').read_text()==text
else:(P/'design-closure-review.md').write_text(text)
report=dict(status='passed',scope='design_closure_traceability_only',products=10,requirements=15,tasks=104,steps=320,
 compoundFacets=16,acceptanceJourneys=12,closureClasses=6,adoptionFamilies=4,openDecisions=19,returnDecisionSelected=False,
 sourcePinChecks=len(d['sourceHashes']),runtimeVerified=False,allImplementationReady=False,implementationAuthorized=False,
 sourceHashes={'content/planning/design-closure-review.json':hashlib.sha256((P/'design-closure-review.json').read_bytes()).hexdigest()},
 limitations=['Requirement-based task impact is not task ownership or a universal policy gate.',
 'This is a document structure/source audit, not runtime tests, schedule estimation or full prose correctness proof.'])
if not a.check:(P/'design-closure-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k not in ['sourceHashes','limitations']}))
