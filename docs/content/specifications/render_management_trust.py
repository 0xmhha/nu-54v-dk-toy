"""Render/check management trust design; never provisions credentials."""
import argparse,hashlib,json
from pathlib import Path
P=Path(__file__).resolve().parent;R=P.parents[1]
a=argparse.ArgumentParser();a.add_argument('--check',action='store_true');args=a.parse_args()
load=lambda f:json.loads((R/f).read_text());d=load('content/specifications/management-trust-lifecycle.json')
assert d['implementation']=='deferred_by_user' and not any(d[k] for k in ['canonicalMerged','runtimeVerified','sqlApplied','activeTrustChanges'])
for f,h in d['sourceHashes'].items():assert hashlib.sha256((R/f).read_bytes()).hexdigest()==h,f
for key,count in [('keyDomains',6),('policyInputs',6),('records',7),('invariants',10),('commands',8),('transitions',11),('crossMappings',4)]:
 assert len(d[key])==len({r['id'] for r in d[key]})==count
fields={f['id'] for p in load('content/specifications/selection-profile-contract.json')['profiles'] for f in p['fields']}
records={r['id'] for r in d['records']};cmds={c['id'] for c in d['commands']}
for p in d['policyInputs']:assert p['selection'] is None and set(p['fieldRefs'])<=fields
for c in d['commands']:assert set(c['recordRefs'])<=records and c['registration']=='privileged_service_command_candidate_not_public_api'
for t in d['transitions']:assert t['commandRef'] in cmds
for i in d['invariants']:assert set(i['recordRefs'])<=records
policies={p['id'] for p in load('content/specifications/profile-control-adoption-map.json')['policyCandidates']}
assert {p for r in d['crossMappings'] for p in r['policyCandidateRefs']}==policies
assert set(d['existingTrustResourceRefs'])<={r['id'] for r in load('content/specifications/security-storage-contracts.json')['resources']}
assert set(d['taskRefs'])<={r['id'] for r in load('content/planning/work-breakdown.json')['tasks']}
assert set(d['decisionRefs'])<={r['id'] for r in load('content/planning/work-breakdown.json')['decisions']}
assert set(d['uiProjection']['screenRefs'])<={s['id'] for s in load('content/specifications/screen-flows.json')['screens']}
lines=['# 관리 신뢰의 최초 등록·키 교체·복구','',d['purpose'],'','## 신뢰와 키의 범위','']+['- '+x for x in d['boundaries']]
lines+=['','| 용도 | 역할 | 부여하지 않는 권한 |','|---|---|---|']
for k in d['keyDomains']:lines.append(f"| {k['id']} · {k['name']} | {k['purpose']} | {k['notGranted']} |")
lines+=['','## 최초 등록·정상 교체·복구의 분리','',
 '```mermaid','flowchart LR',' O["독립 OOB bootstrap 근거"] --> B["초기 anchor + 설치 checkpoint"]',
 ' B --> BR["current restricted"]',' BR --> V["별도 release 검토"]',' V --> A["현재 관리 신뢰"]',
 ' A --> P["정상 교체: 현재 승인 + 새 키 증거"]',' P --> N["다음 anchor epoch"]',
 ' A --> X["유실·침해 의심: scope 제한"]',' X --> R["사전 독립 recovery 근거"]',
 ' R --> RR["복구 commit: 제한 유지"]',' RR --> C["사건 종료·소비자 재검증"]',' C --> A',
 ' X --> Q["독립 근거 없음: 격리"]','```','',
 '교체 준비 상태는 기존 anchor의 활성 상태와 별개다. 대기 교체의 승인만 만료됐고 기존 신뢰가 유효하면 원 상태를 유지한다. 반면 침해·복구 실패는 제한을 유지한다.','',
 '## 최초 관리 신뢰 등록','']+[f'{i+1}. {s}' for i,s in enumerate(d['bootstrapProtocol'])]
lines+=['','## 정상 키 교체','']+['- '+s for s in d['normalRotation']]
r=d['externalReference'];lines+=['',f"참고: [{r['title']}]({r['url']}) — {r['supports']} {r['application']} {r['notClaimed']}을 주장하지 않는다.",'',
 '## 유실·침해 후 복구','']+[f'{i+1}. {s}' for i,s in enumerate(d['incidentRecovery'])]
lines+=['','## 소비자·오프라인·저장소 복구','']+['- '+s for s in d['consumerRules']]
lines+=['','## 논리 기록','','| ID | 기록 | 키 | 내용 |','|---|---|---|---|']
for r in d['records']:lines.append(f"| {r['id']} | {r['name']} | {r['logicalKey']} | {r['contents']} |")
lines+=['','기존 TrustPrincipal·TrustGrantRevision·TrustChangeRequest와 연결할 추가 논리 기록이다. SQL 테이블·기기 보호 저장소를 구현하거나 검증한 것으로 보지 않는다.','',
 '## 반드시 유지할 조건','']
for i in d['invariants']:lines+=['- **'+i['id']+'**: '+i['rule']+' ('+', '.join(i['recordRefs'])+')']
lines+=['','## 보호된 운영 명령 후보','', '등록된 공개 HTTP/BLE 명령이 아니다. 실제 관리 채널과 키 보관 방식은 선정 전이다.','']
for c in d['commands']:lines += [f"### {c['id']} · {c['name']}",'','- 현재 권한: '+c['authority'],'- 조건: '+c['guard'],'- 기록: '+', '.join(c['recordRefs']),'']
lines+=['## 상태 전이','','| ID | 전이 | 조건 | 연산 |','|---|---|---|---|']
for t in d['transitions']:lines.append(f"| {t['id']} | {t['from'].replace('|',' / ')} → {t['to']} | {t['rule']} | {t['commandRef']} |")
lines+=['','원 요청의 pending/committed/unknown outcome과 현재 trust admission·개별 key 상태는 별도 축이다. 응답 유실 때문에 anchor를 되돌리거나 원 case를 재소비하지 않는다.','',
 '## 원자성·중복 요청','']+['- '+s for s in d['atomicity']]
lines+=['','## 기존 관리 연산과의 연결','']
for m in d['crossMappings']:lines+=['- **'+m['id']+'** ('+', '.join(m['policyCandidateRefs'])+'): '+m['rule']]
lines+=['','## 화면 표시','','| 상태 | 문구 후보 |','|---|---|']
for u in d['uiProjection']['copy']:lines.append(f"| {u['state']} | {u['text']} |")
lines+=['','공개 필드: '+', '.join(d['uiProjection']['fields']), '',d['uiProjection']['privateProjection'],'',
 '## 아직 선택할 입력','','| ID | 선택 내용 | 기존 선택 필드 |','|---|---|---|']
for p in d['policyInputs']:lines.append(f"| {p['id']} | {p['needed']} | {', '.join(p['fieldRefs'])} |")
lines+=['','모든 selection은 null이다. 독립 승인·복구 정책은 설계 후보이며 실제 관리자 배정이나 approval threshold를 확정하지 않았다.','',
 '## 검증과 다음 단계','',
 '유한 설계 예제로 자기 신뢰 등록·중복 승인·본문 변경·stale 권한·복구 replay·fence rollback·기기 checkpoint 불일치를 검사한다. 실제 암호 서명, HSM/기기 보호 저장, 외부 recovery 채널, 운영자 신원이나 분산 commit을 검증한 결과는 아니다.','',
 '[검증 기록](management-trust-validation.json) · [구조화 설계](management-trust-lifecycle.json) · [관리 연산 매핑](profile-control-adoption-map.md) · [기존 신뢰 registry 설계](security-integration-design.md)','',d['nextDesign']]
rendered='\n'.join(lines)+'\n'
if args.check:assert (P/'management-trust-lifecycle.md').read_text()==rendered
else:(P/'management-trust-lifecycle.md').write_text(rendered)
print(json.dumps(dict(scope='management_trust_design_structure_only',keyDomains=6,unselectedInputs=6,records=7,invariants=10,commands=8,transitions=11,activeTrustChanges=0,runtimeVerified=False)))
