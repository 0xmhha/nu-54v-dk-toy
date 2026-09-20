"""Render/check the candidate mapping; does not register API/BLE or apply SQL."""
import argparse,hashlib,json
from pathlib import Path
P=Path(__file__).resolve().parent;R=P.parents[1]
pa=argparse.ArgumentParser();pa.add_argument('--check',action='store_true');a=pa.parse_args()
load=lambda f:json.loads((R/f).read_text())
d=load('content/specifications/profile-control-adoption-map.json');parent=load('content/specifications/profile-adoption-protocol.json')
for f,h in d['sourceHashes'].items():assert hashlib.sha256((R/f).read_bytes()).hexdigest()==h,f
assert d['implementation']=='deferred_by_user'
assert not any(d[k] for k in ['canonicalMerged','runtimeVerified','sqlApplied','registeredNewApis','registeredNewBleCommands'])
apis={r['apiId'] for r in load('content/specifications/api-access-transactions.json')['operations']}
policies={r['id'] for r in load('content/specifications/authorization-policies.json')['policies']}
commands={r['name'] for r in load('content/specifications/ble-catalog.json')['commands']}
tables={r['name'] for r in load('content/specifications/database/schema-catalog.json')['tables']}
resources={r['id'] for r in load('content/specifications/security-storage-contracts.json')['resources']}
assert (len(apis),len(policies),len(commands),len(tables))==(110,60,34,61)
assert len(load('content/specifications/database/schema-catalog.json')['migrations'])==7
assert len(load('content/specifications/preimplementation-contract-overlay.json')['routeContracts'])==26
ops={r['id'] for r in d['operations']};ps={r['id'] for r in d['policyCandidates']};sr={r['id'] for r in d['storageMappings']}
assert len(ops)==len(d['operations'])==7 and ops=={r['id'] for r in parent['controlOperations']}
assert len(ps)==len(d['policyCandidates'])==7 and len(sr)==len(d['storageMappings'])==10
assert not policies.intersection(p['name'] for p in d['policyCandidates'])
assert len(d['bleCandidates'])==3 and not commands.intersection(p['proposedCommand'] for p in d['bleCandidates'])
assert len({(o['candidateHttp']['method'],o['candidateHttp']['path']) for o in d['operations']})==7
for o in d['operations']:
 assert o['policyRef'] in ps and set(o['readResourceRefs']+o['writeResourceRefs'])<=sr
 assert o['candidateHttp']['status']=='proposed_not_registered'
 if o['id']=='PRC-04':assert not o['writeResourceRefs'] and o['idempotencyMode']=='read_only'
for p in d['policyCandidates']:
 assert p['operationRef'] in ops and p['defaultDecision']=='deny'
 assert set(p['trustResourceRefs'])<=resources
for b in d['bleCandidates']:assert set(b['existingCommandRefs'])<=commands
for b in d['existingBindings']:assert b['apiRef'] in apis and not b['adopted']
for r in d['storageMappings']:
 assert set(r['reuseCandidates'])<=tables
 assert r['physicalStatus']=='pending_not_in_reference_sql'
 if r['sourceRecordRef']:assert r['sourceRecordRef'] in {r['id'] for r in parent['records']}
assert set(d['storageInfrastructure']['existingTableRefs'])<=tables
assert set(d['storageInfrastructure']['externalTrustRefs'])<=resources
assert len({r['code'] for r in d['errorRules']})==len(d['errorRules'])==11
lines=['# 管理連結'.replace('管理連結','관리 연산의 권한·오류·중복 요청·저장 연결'),'',d['purpose'],'',
 '관리 권한을 기존 점주·펌웨어 배포·감사 조회 권한에서 자동으로 유도하지 않는다. 원 결과 조회와 새로운 변경 실행도 분리한다. 모든 경로·권한·BLE 명령은 후보이며 실제 등록은 0개다.','',
 '## 권한 경계','']+['- '+s for s in d['authorityRules']]
lines+=['','## 관리 권한 후보','','| ID | 권한 이름 | 주체와 범위 | 허용 경계 |','|---|---|---|---|']
for p in d['policyCandidates']:lines.append(f"| {p['id']} | {p['name']} | {p['principal']} | {p['boundary']} |")
lines+=['','## 연산별 연결','','| 관리 연산 | 후보 HTTP | 권한 | 저장 경계 |','|---|---|---|---|']
for o in d['operations']:lines.append(f"| {o['id']} | {o['candidateHttp']['method']} {o['candidateHttp']['path']} | {o['policyRef']} | {o['transactionRef']} |")
for o in d['operations']:
 lines+=['',f"### {o['id']}",'','- 입력: '+', '.join(o['requestSpecificFields']),'- 결과: '+', '.join(o['responseSpecificFields']),
 '- 읽기: '+', '.join(o['readResourceRefs']),'- 쓰기: '+(', '.join(o['writeResourceRefs']) or '도메인 쓰기 없음'),'- 조건: '+o['guard']]
lines+=['','## 중복·권한 변경·응답 유실','']
for k,v in d['idempotency'].items():lines+=['- **'+k+'**: '+(', '.join(v) if isinstance(v,list) else v)]
lines+=['','```mermaid','flowchart LR',' R["원 request identity + body digest"] --> A["현재 인증·scope"]',
 ' A --> I["영속 outcome + 도메인 unique identity"]',' I --> SAME["같은 본문: 원 결과를 현재 권한으로 투영"]',
 ' I --> CONFLICT["다른 본문: 충돌"]',' I --> NEW["처음 요청: 현재 의미·CAS 검사"]',
 ' NEW --> COMMIT["업무 + outcome + outbox + 감사"]',' LOST["응답 유실"] --> READ["PRC04 원 결과 조회"]',
 ' READ --> META["원 commit과 현재 head를 구분"]','```','',
 '### 결과 투영','']
for k,v in d['projection'].items():lines+=['- **'+k+'**: '+(', '.join(v) if isinstance(v,list) else v)]
lines+=['','## 기존 API에서 연결할 부분','','| 기존 API | 재사용 의도 | 채택 전에 필요한 조건 |','|---|---|---|']
for b in d['existingBindings']:lines.append(f"| {b['apiRef']} | {b['reuseIntent']} | {b['requiredChangeOrRestriction']} |")
lines+=['','API-020/107이 있다는 사실만으로 관리 결과 조회가 지원되는 것은 아니다. 새로운 operationClass와 typed reader·현재 ACL·투영이 함께 채택되기 전에는 기존 경로로 우회하지 않는다.','',
 '## BLE 준비·적용·상태 후보','']
for b in d['bleCandidates']:
 lines += [f"### {b['id']} · {b['proposedCommand']}",'','- 기존 검토 대상: '+', '.join(b['existingCommandRefs']),
 '- 입력: '+', '.join(b['requestFields']),'- 결과: '+', '.join(b['responseFields']),'- 조건: '+b['guard'],'']
lines+=['앱이 서버에 전달하는 relay와 원 기기 증거는 별도다.','']+['- '+s for s in d['bleRelayRules']]
lines+=['','## 논리 저장 매핑','','| ID | 자원 | 기존 참조 후보 | 추가로 필요한 물리 계약 |','|---|---|---|']
for r in d['storageMappings']:lines.append(f"| {r['id']} | {r['logicalName']} | {', '.join(r['reuseCandidates']) or '새 매핑 필요'} | {r['missingPhysicalContract']} |")
lines+=['','공통 기반: '+', '.join(d['storageInfrastructure']['existingTableRefs'])+'. 이 표의 참조는 실제 컬럼·제약조건·트랜잭션 구현을 뜻하지 않는다.','']+['- '+s for s in d['storageInfrastructure']['atomicityRules']]
lines+=['','## 오류와 재시도','','| 오류 후보 | 판정 단계 | 재시도 처리 | 규칙 |','|---|---|---|---|']
for e in d['errorRules']:lines.append(f"| {e['code']} | {e['phase']} | {e['retry']} | {e['rule']} |")
lines+=['',d['errorPrecedence'],'','## 함께 채택할 묶음','']+['- '+s for s in d['adoptionBundle']['mustChangeTogether']]
lines+=['','보존하는 기준:','']+['- '+s for s in d['adoptionBundle']['preserve']]
lines+=['','아직 기준 채택 전인 이유:','']+['- '+s for s in d['adoptionBundle']['notReadyToMerge']]
lines+=['','## 검증과 다음 설계','',
 '정상·권한 변경·중복/충돌·보고 relay 변경·결과 cache 만료·읽기 분리를 유한 설계 예제로 검사한다. 실제 인증/서명/DB 원자성/기기 통신 시험은 아니다.','',
 '[검증 결과](profile-control-validation.json) · [구조화 매핑](profile-control-adoption-map.json) · [부분 적용·복구 원칙](profile-adoption-protocol.md) · [행위 조건](selection-action-gates.md)','',d['nextDesign']]
text='\n'.join(lines)+'\n'
if a.check:assert (P/'profile-control-adoption-map.md').read_text()==text
else:(P/'profile-control-adoption-map.md').write_text(text)
print(json.dumps(dict(scope='profile_control_mapping_only',operations=7,policyCandidates=7,bleCandidates=3,storageMappings=10,errorMappings=11,canonicalApis=110,canonicalPolicies=60,canonicalBleCommands=34,referenceTables=61,migrations=7,runtimeVerified=False)))
