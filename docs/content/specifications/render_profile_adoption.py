"""Render/check an unexecuted design protocol for profile adoption."""
import argparse,hashlib,json
from pathlib import Path
P=Path(__file__).resolve().parent;R=P.parents[1]
arg=argparse.ArgumentParser();arg.add_argument('--check',action='store_true');args=arg.parse_args()
load=lambda f:json.loads((R/f).read_text())
d=load('content/specifications/profile-adoption-protocol.json')
assert d['implementation']=='deferred_by_user' and not any(d[k] for k in ['canonicalMerged','runtimeVerified','sqlApplied','activeRollouts'])
for path,h in d['sourceHashes'].items():assert hashlib.sha256((R/path).read_bytes()).hexdigest()==h,path
for key,total in [('records',7),('transitions',12),('controlOperations',7),('adoptionSteps',7)]:
 rows=d[key];assert len(rows)==len({r['id'] for r in rows})==total
assert all(not r['executed'] for r in d['adoptionSteps'])
assert all(r['registration']=='logical_alias_no_http_ble_registration' for r in d['controlOperations'])
assert set(d['taskRefs'])<={t['id'] for t in load('content/planning/work-breakdown.json')['tasks']}
assert set(d['relatedGateRefs'])<={a['id'] for a in load('content/specifications/selection-action-gates.json')['actions']}
assert set(d['uiProjection']['screenRefs'])<={s['id'] for s in load('content/specifications/screen-flows.json')['screens']}
assert set(d['decisionRefs'])<={x['id'] for x in load('content/planning/work-breakdown.json')['decisions']}
assert set(d['integrationBundleRefs'])=={x['id'] for x in load('content/planning/integration-adoption-matrix.json')['bundles']}
lines=['# 설정 적용·부분 갱신·중단·복구 계약','',d['purpose'],'','## 핵심 경계','']+['- '+s for s in d['boundary']]
lines += ['', '## 준비와 활성화의 차이','',
 '```mermaid','flowchart LR',' M["불변 변경 manifest"] --> P["peer 준비·호환 증거"]',
 ' P --> R["현재 readiness revision"]',' R --> A["서버 head/fence/registry CAS"]',
 ' A --> C["서버 activation commit"]',' C --> N["앱·기기별 통지와 적용"]',
 ' N --> G["매 요청: 같은 digest·현재 권한 확인"]',' N --> H["미적용 peer: 해당 동작 보류"]',
 ' LOST["응답·통지 유실"] --> Q["원 변경/activation 결과 조회"]',' Q --> N',
 ' OLD["기존 노출 operation"] --> OR["원 binding·현재 정책으로 대사/복구"]','```','',
 '관리 화면의 준비 완료, 서버 활성화 완료, 기기 적용 완료, 특정 사용자 행위 허용은 서로 다른 상태다. 상태 한 개를 enabled=true로 합치지 않는다.','',
 '## 채택 manifest','', '| 영역 | 고정할 내용 |','|---|---|']
for k in ['identity','contractBundle','selection','activation','cohort']:
 lines.append(f"| {k} | {', '.join(d['manifest'][k])} |")
lines += ['',d['manifest']['digestRule'],'',d['manifest']['dataHandling'],'','## 저장할 논리 기록','',
 '| ID | 기록 | 논리 키 |','|---|---|---|']
for r in d['records']:lines.append(f"| {r['id']} | {r['name']} | {r['key']} |")
for r in d['records']:lines += ['',f"### {r['id']} · {r['name']}",'',r['rule'],'','필드: '+', '.join(r['fields'])]
lines += ['', '실제 테이블·DDL·인덱스·저장 엔진은 이 문서에서 적용하지 않는다. 아래 CAS/원자성은 구현에서 입증해야 하는 요구사항이다.','',
 '## 필수 참여자와 준비 증거','']+['- '+s for s in d['cohortRules']]
lines += ['', '## 활성화 조건','']+[f'{i+1}. {s}' for i,s in enumerate(d['activationConditions'])]
lines += ['',d['activationRulesNote'],'','## 부분 적용과 요청 수락','']
for k,v in d['partialApplication'].items():lines += ['- **'+k+'**: '+v]
lines += ['', '### 오프라인과 철회 한계','']
for m in d['offlineModes']:lines += ['- **'+m['mode']+'**: '+m['rule']+' '+m['limits']]
lines += ['', '외부 체인에 유효하게 서명·전파된 거래나 외부로 공개된 데이터는 서버의 설정 변경만으로 회수할 수 없다. 앱 내부의 새 요청 차단과 원 거래의 외부 실행 가능성을 분리해 표시한다.','',
 '## 상태와 전이','',d['stateAxes']['rule'],'']
for k in ['coordinatorPhase','participantPhase','requestOutcome','securityState']:lines += ['- '+k+': '+', '.join(d['stateAxes'][k])]
for t in d['transitions']:
 lines += ['',f"### {t['id']} · {t['from']} → {t['to']}",'',
           '- 계기: '+t['trigger'],'- 조건: '+t['guard'],'- 효과: '+t['effect']]
lines += ['', '## 관리용 논리 연산','',
 'PRC 별칭은 후보 명칭이다. 기존 110개 API/26개 OC 또는 BLE catalog에 새 경로를 등록하지 않았다. 모든 요청은 기존 공통 envelope·현재 관리 scope·canonical digest·오류/멱등 정책을 사용하도록 후속 wire 설계에 연결한다.','']
for op in d['controlOperations']:
 lines += [f"### {op['id']} · {op['name']} ({op['mode']})",'',
           '- 입력: '+', '.join(op['requestFields']),'- 결과: '+', '.join(op['responseFields']),'- 조건: '+op['rule'],'']
lines += ['## 되돌리기와 보존','']+['- '+s for s in d['rollbackRules']]
lines += ['', '## 계약 채택 순서','', '| 단계 | 입력 | 결과 |','|---|---|---|']
for s in d['adoptionSteps']:lines.append(f"| {s['id']} · {s['name']} | {s['inputs']} | {s['output']} |")
lines += ['', '모든 단계는 미실행 계획이다. 선택된 schema·ACL·reader·저장 매핑을 함께 바꾸는 채택 변경표를 먼저 만든 뒤 실제 기준 반영 여부를 별도 기록한다.','',
 '## 화면 표시','', '응답 필드: '+', '.join(d['uiProjection']['fields']), '', '| 관측 상태 | 문구 후보 |','|---|---|']
for s in d['uiProjection']['copy']:lines.append(f"| {s['state']} | {s['text']} |")
lines += ['',d['uiProjection']['privacy'],'','화면 연결: '+', '.join(d['uiProjection']['screenRefs']), '',
          '작업 연결: '+', '.join(d['taskRefs']), '', '행위 조건 연결: '+', '.join(d['relatedGateRefs']), '',
          '## 남은 고정값','']+['- '+s for s in d['remainingProfileInputs']]
lines += ['', '## 검증과 범위','',
 '설계 판정표는 필수 peer 누락·오래된 ACK·경합·응답 유실·부분 적용·보안 철회·후속 복귀 반례를 검사한다. 실제 분산 합의·암호·기기 재부팅 탐지·DB 원자성을 실행 검증한 결과가 아니다.','',
 '[검증 기록](profile-adoption-validation.json) · [구조화 계약](profile-adoption-protocol.json) · [설정 필드](selection-profile-contract.md) · [행위별 조건](selection-action-gates.md) · [기존 통합 묶음](../planning/integration-adoption-matrix.md)','',
 '재현: `python3 content/specifications/render_profile_adoption.py --check`, `python3 content/specifications/validate_profile_adoption.py`.','',
 '다음은 각 관리 연산의 권한·오류·멱등 결과를 기존 API/BLE/저장 매핑에 연결한 채택 변경표를 작성하는 것이다. 구현과 실제 활성화는 계속 보류한다.']
text='\n'.join(lines)+'\n'
if args.check:assert (P/'profile-adoption-protocol.md').read_text()==text
else:(P/'profile-adoption-protocol.md').write_text(text)
print(json.dumps(dict(scope='profile_adoption_design_only',records=7,transitions=12,controlOperations=7,adoptionSteps=7,activeRollouts=0,runtimeVerified=False)))
