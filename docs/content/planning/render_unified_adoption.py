"""Validate/render a prospective adoption checkpoint. Never edits target contracts."""
import argparse,collections,hashlib,json
from pathlib import Path
P=Path(__file__).resolve().parent;R=P.parents[1]
ap=argparse.ArgumentParser();ap.add_argument('--check',action='store_true');args=ap.parse_args()
def load(p):return json.loads((R/p).read_text())
d=load('content/planning/unified-adoption-plan.json');old=load('content/planning/integration-adoption-matrix.json')
assert d['implementation']=='deferred_by_user' and not any(d[k] for k in ['canonicalMerged','runtimeVerified','sqlApplied'])
assert all(d[k] is None for k in ['selectedCheckpoint','ownerAssignment','effortEstimates'])
assert d['baseline']==old['baseline'] and d['legacyDispositions']==old['dispositions']
assert d['legacyDependencies']==[dict(id=b['id'],finishDependsOn=b['finishDependsOn']) for b in old['bundles']]
for p,h in d['sourceHashes'].items():assert hashlib.sha256((R/p).read_bytes()).hexdigest()==h,p
for section in ['units','targetInventory','candidateRegistry','checkpointFields','compatibilityRules','reviewStages','stopRecoveryRules']:
 assert len(d[section])==len({r['id'] for r in d[section]})
units={u['id']:u for u in d['units']};di={b['id'] for b in old['bundles']}
oc=load('content/specifications/preimplementation-contract-overlay.json')
pc=load('content/specifications/profile-control-adoption-map.json')
mt=load('content/specifications/management-trust-lifecycle.json')
assert {x for u in units.values() for x in u['diRefs']}==di
assert collections.Counter(x for u in units.values() for x in u['ocRefs'])==collections.Counter(r['id'] for r in oc['routeContracts'])
targets={r['path']:r for r in d['targetInventory']}
assert len(targets)==len(d['targetInventory'])
assert set(targets)=={p for u in units.values() for p in u['targetPaths']}
assert {'content/specifications/'+p for b in old['bundles'] for p in b['targets']}<=set(targets)
for p,r in targets.items():
 assert r['baselineHash']==d['sourceHashes'][p] and not r['changed']
 assert set(r['unitRefs'])=={u['id'] for u in units.values() if p in u['targetPaths']}
 assert set(r['diRefs'])=={b['id'] for b in old['bundles'] if p.removeprefix('content/specifications/') in b['targets']}
 assert r['sharedReviewRequired']==(len(r['unitRefs'])>1)
 if p.endswith('.sql'):assert r['mode']=='reference_only_append_future_migration'
for u in units.values():assert u['state']=='planned_not_adopted' and set(u['diRefs'])<=di
catalog={r['id'] for r in load('content/specifications/api-catalog.json')['operations']}
expected=dict(OC={r['id'] for r in oc['routeContracts']},PRC={r['id'] for r in pc['operations']},
 PCP={r['id'] for r in pc['policyCandidates']},PCB={r['id'] for r in pc['bleCandidates']},TMC={r['id'] for r in mt['commands']})
for ns,ids in expected.items():assert {r['candidateRef'] for r in d['candidateRegistry'] if r['namespace']==ns}==ids
assert len(d['candidateRegistry'])==sum(map(len,expected.values()))
for r in d['candidateRegistry']:
 assert r['unitRef'] in units and r['source'] in d['sourceHashes']
 assert set(r['existingApiRefs'])<=catalog and r['adoptedIdentifier'] is None and r['disposition']=='candidate_not_registered'
 if r['namespace']=='OC':assert r['candidateRef'] in units[r['unitRef']]['ocRefs']
 if r['namespace']=='TMC':assert r['kind']=='privileged_internal_command_not_public_api'
assert all(f['value'] is None for f in d['checkpointFields'])
done=set()
for s in d['reviewStages']:
 assert not s['executed'] and set(s['dependsOn'])<=done;done.add(s['id'])
assert all(c['status']=='specified_not_runtime_verified' for c in d['compatibilityRules'])
w=load('content/planning/work-breakdown.json')
assert len(w['tasks'])==104 and w['subtaskCount']==320 and all(r['status']=='open' for r in w['decisions'])
assert len(catalog)==110 and len(load('content/specifications/authorization-policies.json')['policies'])==60

lines=['# 통합 계약 채택 checkpoint 계획','',d['date']+' · 후보 계획 · 기준 계약 병합·제품 구현 없음','',d['purpose'],'','## 계획의 범위','']
lines+=['- '+s for s in d['boundaries']]
lines+=['','## 검토 묶음','','아래 파일 목록은 공동 검토 범위다. 모든 파일을 반드시 수정한다는 뜻이 아니며, 실제 disposition에서 유지/수정/보류를 확인해야 한다.','',
 '| 묶음 | 기존 DI | OC 후보 | 변경 의도 | 보존할 의미 |','|---|---|---|---|']
for u in d['units']:lines.append(f"| {u['id']} · {u['name']} | {', '.join(u['diRefs']) or '교차 보완'} | {', '.join(u['ocRefs']) or '별도 후보/공유 기반'} | {u['changeIntent']} | {u['preserve']} |")
lines+=['','## 변경 대상별 공동 검토','','공유 파일은 한 묶음의 검토만으로 완료 처리하지 않는다. 원본 SQL은 참조 대상으로 남기고, 실제 채택 시 새 migration 계획과 검증을 별도로 작성한다.','',
 '| 대상 | 파일 | 함께 검토할 묶음 | 취급 |','|---|---|---|---|']
for t in d['targetInventory']:
 rel='../'+t['path'].removeprefix('content/')
 lines.append(f"| {t['id']} | [{Path(t['path']).name}]({rel}) | {', '.join(t['unitRefs'])} | {t['mode']} |")
lines+=['','## 후보 이름·등록 상태','','기존 API 110개·정책 60개·BLE 34개·SQL 61개 테이블/7개 migration 기준을 유지한다. 아래는 정식 등록 수에 더하지 않는다.','',d['registryBoundary'],'',
 '| namespace | 후보 수 | 현재 취급 |','|---|---:|---|']
for ns,ids in expected.items():lines.append(f"| {ns} | {len(ids)} | {'보호된 내부 관리 명령; 공개 API 아님' if ns=='TMC' else '개별 등록 ID/채택 위치 미확정'} |")
lines+=['','개별 후보→UA·기존 API·원 설계의 연결은 구조화 파일의 candidateRegistry에 있다. 원 DI의 20개 분류와 완료 의존 관계도 원형 그대로 보존했다.','',
 '## checkpoint 필수 항목','','| 항목 | 필요한 내용 | 해석 경계 |','|---|---|---|']
for c in d['checkpointFields']:lines.append(f"| {c['id']} · {c['name']} | {c['required']} | {c['rule']} |")
lines+=['','현재 CP 값과 selectedCheckpoint는 null이다. 계획 문서의 hash와 실제 채택 후 target hash를 혼동하지 않는다.','',
 '## 호환·기존 작업 보존','','| 조합 | 조건 |','|---|---|']
for c in d['compatibilityRules']:lines.append(f"| {c['id']} · {c['name']} | {c['rule']} |")
lines+=['','## 반영 순서','','```mermaid','flowchart LR',
 ' A["기준·공유 대상 보존"] --> B["관련 선택·호환 입력 확인"]',
 ' B --> C["공동 계약 diff 검토"]',' C --> D["후보별 disposition·검사"]',
 ' D --> E["문서 checkpoint 채택"]',
 ' E -. "명시적 구현 전환 후" .-> F["실환경 준비·활성화·peer 적용"]','```','',
 '| 단계 | 산출물 | 중단 조건·한계 |','|---|---|---|']
for s in d['reviewStages']:lines.append(f"| {s['id']} · {s['name']} | {s['output']} | {s['guard']} |")
lines+=['','UP는 검토 완료 순서다. 기존 DI finishDependsOn을 삭제하거나 배포 순서를 재정의하지 않는다. 일부 기능만 채택할 때에는 그 기능에 필요한 공유 계약·권한·reader·저장 의존성을 닫고 제외된 기능을 명시해야 한다. 전체 15개 요구의 수용 목표는 유지한다.','',
 '## 중단·실패·복구','','| 시점 | 조치 |','|---|---|']
for r in d['stopRecoveryRules']:lines.append(f"| {r['id']} · {r['name']} | {r['rule']} |")
lines+=['','## 검증과 다음 단계','',
 '기존 DI 8개·OC 26개의 누락/중복, PRC/PCP/PCB/TMC 후보 연결, 공유 파일·원본 hash·검토 순서와 미채택 상태를 검사했다. 문서 구조/참조 검사이며 실제 호환성·동시성·DB migration·BLE·신뢰 복구의 통과 증거가 아니다.', '',
 '[구조화 계획](unified-adoption-plan.json) · [검증 기록](unified-adoption-validation.json) · [제품별 인계 보완판](design-closure-review.md) · [기존 DI 채택표](integration-adoption-matrix.md)', '',d['nextAction']]
rendered='\n'.join(lines)+'\n'
if args.check:assert (P/'unified-adoption-plan.md').read_text()==rendered
else:(P/'unified-adoption-plan.md').write_text(rendered)
report=dict(status='passed',scope='unified_adoption_plan_structure_only',units=len(units),targetFiles=len(targets),
 sharedTargets=sum(t['sharedReviewRequired'] for t in targets.values()),legacyBundles=len(di),legacyDispositions=len(d['legacyDispositions']),
 candidateCounts={ns:len(ids) for ns,ids in expected.items()},sourcePinChecks=len(d['sourceHashes']),canonicalMerged=False,runtimeVerified=False,implementationAuthorized=False,
 sourceHashes={'content/planning/unified-adoption-plan.json':hashlib.sha256((P/'unified-adoption-plan.json').read_bytes()).hexdigest()})
if not args.check:(P/'unified-adoption-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k!='sourceHashes'}))
