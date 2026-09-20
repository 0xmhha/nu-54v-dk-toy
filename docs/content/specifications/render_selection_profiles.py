"""Render and structurally check logical selection profiles. No product execution."""
import argparse,hashlib,json
from pathlib import Path
P=Path(__file__).resolve().parent;R=P.parents[1]
parser=argparse.ArgumentParser();parser.add_argument('--check',action='store_true');args=parser.parse_args()
load=lambda p:json.loads((R/p).read_text())
p=load('content/specifications/selection-profile-contract.json');m=load('content/specifications/selection-action-gates.json');b=load('content/planning/decision-briefing.json')
fields={f['id']:f for c in p['profiles'] for f in c['fields']};actions={a['id']:a for a in m['actions']}
assert len(p['profiles'])==20 and {c['decisionRef'] for c in p['profiles']}=={c['id'] for c in b['cards']}
assert len(fields)==sum(len(c['fields']) for c in p['profiles'])
assert len(actions)==len(m['actions'])==53
assert len(p['crossConstraints'])==16
screens={s['id'] for s in load('content/specifications/screen-flows.json')['screens']}
apis={a['id'] for a in load('content/specifications/api-catalog.json')['operations']}
ocs={a['id'] for a in load('content/specifications/preimplementation-contract-overlay.json')['routeContracts']}
for d in (p,m):
 assert d['implementation']=='deferred_by_user' and not d['runtimeVerified'] and not d['canonicalMerged']
 for f,h in d['sourceHashes'].items():assert hashlib.sha256((R/f).read_bytes()).hexdigest()==h,f
for c in p['profiles']:
 assert c['selection'] is None
 for f in c['fields']:
  assert f['value'] is None and f['requiredMembers']
  names=[x['name'] for x in f['requiredMembers']];assert len(names)==len(set(names))
  assert {x['type'] for x in f['requiredMembers']} <= {'string','array','object','enum','positiveInteger','nonnegativeInteger','boolean'}
  obj=b
  for k in f['sourcePointer'].strip('/').split('/'):obj=obj[int(k)] if isinstance(obj,list) else obj[k]
  assert obj==f['title']
for c in p['crossConstraints']:assert set(c['fieldRefs'])<=fields.keys() and c['rule'] and c['failure']
for a in actions.values():
 assert a['class'] in m['actionClasses']
 assert set(a['requiredFieldRefs'])<=fields.keys()
 assert set(a['apiRefs'])<=apis and set(a['candidateContractRefs'])<=ocs and set(a['screenRefs'])<=screens
 assert a['domainRule'] and a['whenUnselected'] and not a['runtimeVerified']
 if a['class'] in {'read_observation','safety_restriction'}:assert not a['requiredFieldRefs']
 if a.get('branches'):
  assert len({r['when'] for r in a['branches']})==len(a['branches'])
  assert {f for r in a['branches'] for f in r['requiredFieldRefs']}==set(a['requiredFieldRefs'])
assert {s for a in actions.values() for s in a['screenRefs']}==screens
assert {o for a in actions.values() for o in a['candidateContractRefs']}==ocs
assert not p['profileRecord']['selectionRecords']
assert all(x['selection'] is None for x in load('content/planning/preimplementation-handoff.json')['decisionLedger'])
assert all(x['selectedVersion'] is None for x in load('content/planning/technology-selection.json')['choices'])
assert next(x for x in load('content/specifications/return-recovery-contract.json')['openDecisions'] if x['id']=='RR-DEC-01')['selection'] is None

lines=['# 選択値 profile・適用条件'.replace('選択値','선택값').replace('適用条件','적용 조건'),'',
 f"20개 선택 영역을 {len(fields)}개 설정 객체로 나누고 16개 교차 제약을 정의했다. 모두 값이 비어 있는 논리 명세이며 제품 설정·wire schema·실제 활성 profile은 아니다.",'',
 '## 적용 원칙','']+['- '+s for s in p['valueRules']]
lines += ['', '## 불변 profile과 현재 제한','',
 '```mermaid','flowchart LR',' D["draft: 미선택"] --> S["selected: 선택 기록"]',
 ' S --> C["contract validated: 계약 대조"]',' C --> A["active: 기능별 실제 증거 + 활성화 CAS"]',
 ' A --> R["retired: 신규 사용 중지"]',' F["현재 권한·동의·보안 제한"] --> G["행위별 판정"]',
 ' A --> G',' R --> O["원 profile·원 operation"]',' O --> G',
 ' G --> READ["허용된 조회·제한·원 작업 복구"]','```','',
 '현재 활성 profile은 0개다. lifecycle enum에 active가 있다는 이유로 활성화했다고 해석하지 않는다. 제안한 목표값이나 문서 검사를 실기 증거로 사용하지 않는다.','',
 '### profile 레코드','']
for k in ['identityFields','scopeSelectorFields','provenanceFields','effectiveFields']:
 lines += ['- '+k+': '+', '.join(p['profileRecord'][k])]
lines += ['', '각 값은 선택 기록·scope·version·evidence 참조에 묶인다. 비밀 자체를 profile에 넣지 않는다.','',
 '### 활성화·교체·실패 복구','']+['- '+s for s in p['activationRules']]
lines += ['', p['profileRecovery'],'', '**원 작업에 고정할 식별자** — '+', '.join(p['operationBinding']), '',
 '## 필드 명세','', '객체 내부의 enum 값·배열 원소·정규화·상한은 선정 profile에서 고정한다. 아래 목록만으로 네트워크 입력을 검증하거나 런타임 사용 가능으로 판정하지 않는다.','']
for c in p['profiles']:
 lines += [f"### {c['id']} · {c['title']}",'','| 필드 ID | 선택 내용 | 최소 객체 멤버 |','|---|---|---|']
 for f in c['fields']:
  members=', '.join(f"{x['name']}:{x['type']}" for x in f['requiredMembers'])
  lines.append(f"| {f['id']} | {f['title']} | {members} |")
 lines += ['']
lines += ['## 교차 제약','', '참조 필드는 검토 영향 범위다. 특정 행위와 무관한 상품·제공자 설정까지 전역 필수값으로 요구하지 않는다.','']
for c in p['crossConstraints']:
 lines += [f"### {c['id']}",'',c['rule'],'','- 실패 처리: '+c['failure'],'- 연결: '+', '.join(c['fieldRefs']),'']
lines += ['## 다음 계약 채택에 필요한 결과','',
 '- 선택 필드별 실제 값·버전·근거와 scope를 기록한다.',
 '- action/branch별 schema·권한·원자 저장·호환 reader를 함께 고정한다.',
 '- 원 profile을 보존하는 재조회·재개와 현재 제한 검사를 검증한다.',
 '- 구현 전환 뒤 대상 기기/앱/체인에서 실행 증거를 수집한다.','',
 '[행위별 gate·화면 연결](selection-action-gates.md) · [원 선택 카드](../planning/decision-briefing.md) · [구조화 명세](selection-profile-contract.json) · [설계 검사](selection-profile-validation.json)']
profile_md='\n'.join(lines)+'\n'
lines=['# 미선택 상태의 행위·화면·계약 연결','',m['coverage'],'',
 '조회가 가능하다는 말은 무인증 접근을 뜻하지 않는다. 현재 읽기권·동의 범위와 해당 버전을 이해하는 신뢰된 reader가 필요하다. 새 profile 선택이 없어도 기존의 검증된 reader를 쓸 수 있다는 설계다. 현재 제품 reader가 구현됐다는 뜻은 아니다.','',
 '## 판정 순서','']+[f'{i+1}. {s}' for i,s in enumerate(m['classificationOrder'])]
lines += ['', '## 행위 유형','', '| 유형 | 의미 |','|---|---|']
for k,v in m['actionClasses'].items():lines.append(f'| {k} | {v} |')
lines += ['', '### 모든 행위에 필요한 조건','']+['- '+s for s in m['alwaysRequired']]
lines += ['', '### 기존 작업 재개','']+['- '+s for s in m['continuationPolicy']]
lines += ['', '## 대표 행위별 연결','', '| ID | 행위 | 유형 | 화면 | 계약 |','|---|---|---|---|---|']
for a in actions.values():lines.append(f"| {a['id']} | {a['name']} | {a['class']} | {', '.join(a['screenRefs'])} | {', '.join(a['apiRefs']+a['candidateContractRefs']) or '기기/native 계약 후보'} |")
lines += ['', '## 입력과 보류·복구 기준','']
for a in actions.values():
 lines += [f"### {a['id']} · {a['name']}",'',
           '- 필요한 선택: '+(', '.join(a['requiredFieldRefs']) or '새 업무 선택값 없음; 공통 권한·reader/writer 조건은 필수'),
           '- 도메인 조건: '+a['domainRule'],'- 미선택 때: '+a['whenUnselected']]
 if a.get('additionalGate'):lines += ['- 결합 gate: '+a['additionalGate']]
 if a.get('branches'):
  lines += ['- 분기: '+a['branchSemantics']]
  for br in a['branches']:lines += ['  - '+br['when']+': '+', '.join(br['requiredFieldRefs'])+(' · '+br['domainRequirement'] if br.get('domainRequirement') else '')]
 lines+=['']
lines += ['## 화면 응답 계약','', '**필드** — '+', '.join(m['uiContract']['resultFields']), '',
          '**표시 상태** — '+', '.join(m['uiContract']['availability']), '',
          m['uiContract']['trust'], '',m['uiContract']['refresh'],'', '화면 문구 후보:','']+['- '+s for s in m['uiContract']['copyExamples']]
lines += ['', 'API 참조는 제안 동작의 연결이다. 예를 들어 동일 API-084의 동의 부여와 철회는 서로 다른 gate를 쓰며, POST 여부로 새 효과·조회·제한을 판정하지 않는다. 현재 wire에 없는 subtype은 해당 계약을 고정하기 전 실행하지 않는다.','',
          '[필드·교차 제약](selection-profile-contract.md) · [구조화 행위표](selection-action-gates.json) · [설계 검사](selection-profile-validation.json)']
action_md='\n'.join(lines)+'\n'
if args.check:
 assert (P/'selection-profile-contract.md').read_text()==profile_md
 assert (P/'selection-action-gates.md').read_text()==action_md
else:
 (P/'selection-profile-contract.md').write_text(profile_md);(P/'selection-action-gates.md').write_text(action_md)
print(json.dumps(dict(scope='logical_profile_structure_only',profiles=len(p['profiles']),fields=len(fields),crossConstraints=len(p['crossConstraints']),actions=len(actions),screenRefs=len(screens),candidateContractRefs=len(ocs),selectedValues=0,runtimeVerified=False),ensure_ascii=False))
