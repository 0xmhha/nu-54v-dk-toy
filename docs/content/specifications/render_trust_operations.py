"""Render/check proposed operating responsibilities, not an IAM configuration."""
import argparse, hashlib, json
from pathlib import Path
P=Path(__file__).resolve().parent; R=P.parents[1]
a=argparse.ArgumentParser(); a.add_argument('--check',action='store_true'); args=a.parse_args()
load=lambda f:json.loads((P/f).read_text())
d=load('trust-operations-design.json'); trust=load('management-trust-lifecycle.json')
assert d['implementation']=='deferred_by_user'
assert not any(d[k] for k in ['canonicalMerged','runtimeVerified','sqlApplied','assignedPeople','activeTrustChanges'])
for f,h in d['sourceHashes'].items():assert hashlib.sha256((R/f).read_bytes()).hexdigest()==h,f
roles={r['id'] for r in d['roles']}; evidence={r['id'] for r in load('management-trust-evidence.json')['records']}
commands={r['id'] for r in trust['commands']}; screens={r['id'] for r in load('screen-flows.json')['screens']}
for section in ['roles','evidenceResponsibilities','panels','actions','handoffs','invariants']:
 assert len(d[section])==len({r['id'] for r in d[section]})
assert all(r['assignedPerson'] is None for r in d['roles'])
assert {r['evidenceRef'] for r in d['evidenceResponsibilities']}==evidence
for r in d['evidenceResponsibilities']:
 assert set(r['issuerRoleRefs']+r['verifierRoleRefs']+r['readerRoleRefs']+[r['custodianRoleRef']])<=roles
for r in d['actions']:
 assert set(r['roleRefs'])<=roles and set(r['commandRefs'])<=commands and set(r['evidenceRefs'])<=evidence
assert {c for r in d['actions'] for c in r['commandRefs']}==commands
assert all(r['screenRef'] in screens and r['status']=='candidate_extension_not_adopted' for r in d['panels'])
assert all(r['selection'] is None and r['policyRef'] in {p['id'] for p in trust['policyInputs']} for r in d['unresolved'])
lines=['# 관리 증거의 운영 책임과 백오피스 승인 흐름','',d['purpose'],'','## 기존 설계와의 경계','']+['- '+s for s in d['boundaries']]
lines+=['','## 역할과 권한의 구분','','| 역할 | 수행 책임 | 자동 부여하지 않는 권한 |','|---|---|---|']
for r in d['roles']:lines.append(f"| {r['id']} · {r['name']} | {r['responsibility']} | {r['notGranted']} |")
lines+=['','이 표는 기능 책임이다. 사람 수·인력 배치·공수와 승인 수를 연결하지 않았다.','',
 '## 증거별 발급·검증·보관','','| 증거 | 발급/작성 역할 | 검증 역할 | 보관 | 읽기 |','|---|---|---|---|']
for r in d['evidenceResponsibilities']:lines.append(f"| {r['evidenceRef']} | {', '.join(r['issuerRoleRefs'])} | {', '.join(r['verifierRoleRefs'])} | {r['custodianRoleRef']} | {', '.join(r['readerRoleRefs'])} |")
for r in d['evidenceResponsibilities']:lines+=['',f"**{r['evidenceRef']}**: {r['rule']} {r['readBoundary']}"]
lines+=['','## 화면 흐름','','```mermaid','flowchart LR',
 ' A["O04 변경안 작성"] --> B["정확한 본문 검토"]',
 ' B --> C["승인 기록: 아직 미적용"]',' B --> D["수정 요청: 자동 취소 아님"]',
 ' D --> E["새 본문·새 검토"]',' E --> B',
 ' C --> F["현재 권한 재검사 후 확정"]',' F --> G["원 결과 + 현재 제한 표시"]',
 ' F --> U["불명확: 원 요청 조회·조정"]',' U --> G',
 ' I["O03 사건·제한"] --> R["독립 복구 채널"]',
 ' R --> K["복구 완료: 제한 유지"]',' K --> S["O04 별도 해제 검토"]','```', '',
 '선은 논리적 인계다. 아직 등록하지 않은 명령·API·실제 인증 채널을 구현한 것으로 보지 않는다.', '',
 '## 기존 화면에 추가할 패널','']
for r in d['panels']:
 lines += [f"### {r['id']} · {r['screenRef']} {r['name']}",'',
           '- 표시: '+r['visible'],'- 동작: '+r['actions'],'- 실패/권한 경계: '+r['onFailure'],'']
lines+=['## 행위별 검토·실행 조건','']
for r in d['actions']:
 lines += [f"### {r['id']} · {r['name']}",'',
           '- 책임 역할: '+', '.join(r['roleRefs']),'- 명령 후보: '+(', '.join(r['commandRefs']) or '별도 보존 작업 계약 미등록'),
           '- 증거: '+', '.join(r['evidenceRefs']),'- 조건: '+r['guard'],'- 결과: '+r['result']+' — '+r['limit'],'']
lines+=['## 인계·재접속·예외 처리','','| 흐름 | 전달 주체 | 전달 근거 | 실패/복구 |','|---|---|---|---|']
for r in d['handoffs']:lines.append(f"| {r['id']} · {r['name']} | {r['actors']} | {r['payload']} | {r['onFailure']} |")
lines+=['','## 반드시 지킬 조건','']+['- '+r['id']+': '+r['rule'] for r in d['invariants']]
lines+=['','## 확정 전 필요한 정책','']+[f"- {r['policyRef']}: {r['needed']} — 미선택." for r in d['unresolved']]
lines+=['','## 검토 범위','',
 '문서 참조와 증거/역할/화면 연결을 확인하고, 합성 모델에서 조회 범위·민감 필드 차단·독립 승인·본문 변경·현재 권한 변경·응답 유실을 점검한다. 실제 IAM·암호·브라우저·서버 동시성·독립 복구 채널의 검증 결과는 아니다.', '',
 '[검증 기록](trust-operations-validation.json) · [구조화 설계](trust-operations-design.json) · [승인 증거](management-trust-evidence.md) · [상위 관리 신뢰](management-trust-lifecycle.md)', '',d['nextDesign']]
text='\n'.join(lines)+'\n'
if args.check:assert (P/'trust-operations-design.md').read_text()==text
else:(P/'trust-operations-design.md').write_text(text)
print(json.dumps(dict(status='passed',roles=len(roles),evidenceMappings=len(evidence),panels=len(d['panels']),actions=len(d['actions']),canonicalMerged=False)))
