"""Render catalog, extended DTO, access and screen views; no product execution."""
import json,sys
from pathlib import Path
P=Path(__file__).resolve().parent
L=lambda n:json.loads((P/n).read_text())
views={}
a=L('api-catalog.json')['operations'];c=L('extended-dtos.json')['contracts'];m=L('api-access-transactions.json')['operations'];screens=L('screen-flows.json')['screens']
s=['# API 작업 계약 목록','','설계 제안. [공통 규칙](interface-contracts.md) · [핵심 8개 DTO](critical-dtos.md) · [확장 DTO](extended-dtos.md) · [보안 API 통합](security-api-integration.md). Path 변수는 상세 request.path를 따른다.','', '| ID | 요청 | 권한 | 입력 | 결과 | 작업 |','|---|---|---|---|---|---|']
for x in a:s.append('| '+' | '.join([x['id'],x['method']+' '+x['path'],x['authorization'],', '.join(x['inputFields']) or '—',', '.join(x['outputFields']),', '.join(x['taskRefs'])])+' |')
views['api-catalog.md']='\n'.join(s)+'\n'
s=[f'# 확장 {len(c)}개 API 요청·응답 타입','','설계 제안. [기본 규칙](extended-contracts.md) · [보안 통합과 binary 전달](security-api-integration.md). `?` 타입은 null 허용, 선택은 필드 생략 허용. path 변수는 body/query와 분리한다. ProfileInput 내부는 선정한 엄격한 profile 검증기가 필요하다.','']
for x in c:
 s += [f"## {x['apiId']} · {x['method']} {x['path']}",'',f"권한: {x['authorization']}. 정상 응답: {x['successStatus']}. 작업: {', '.join(x['taskRefs'])}.",'','| 입력 필드 | 타입 | 필수 |','|---|---|---|']
 s += [f"| {f['name']} | {f['type']} | {'필수' if f['required'] else '선택'} |" for f in x['inputFields']] or ['| 없음 | — | — |']
 s += ['','| 응답 data 필드 | 타입 |','|---|---|']+[f"| {f['name']} | {f['type']} |" for f in x['outputFields']]
 s += ['',f"개별 오류: {', '.join(x['specificErrors']) or '공통 오류'}. 인증/권한/형태/멱등성 공통 오류도 적용.",'']
 if x.get('semanticRules'):s += ['- '+r for r in x['semanticRules']]+['']
 if x.get('binaryResponseContract'):s += [f"JSON 외 binary 응답: [전달 계약]({x['binaryResponseContract']}).",'']
views['extended-dtos.md']='\n'.join(s)+'\n'
s=['# API별 읽기·쓰기·트랜잭션 연결','','[권한 규칙](access-transactions.md) · [논리 adapter 자원](security-storage-contracts.json). SQL 테이블과 adapter 자원을 구분한다. 런타임 권한/SQL GRANT가 아니다.','', '| API | 요청 | 권한 | SQL 읽기 | SQL 쓰기 | Adapter 읽기 | Adapter 쓰기 | 트랜잭션 |','|---|---|---|---|---|---|---|---|']
for x in m:s.append('| '+' | '.join([x['apiId'],x['method']+' '+x['path'],x['authorization']]+[', '.join(x[k]) or '—' for k in ['readTables','writeTables','adapterReads','adapterWrites']]+[x['transactionFamily']])+' |')
s+=['','## 반영 조건','']+['- **'+x['apiId']+'**: '+x['commitRule'] for x in m];views['api-access-transactions.md']='\n'.join(s)+'\n'
# Preserve the shared manually authored screen rules, regenerate per-screen records.
s=(P/'screen-flows.md').read_text().split('## U01')[0].rstrip().splitlines()+['']
for x in screens:
 s += [f"## {x['id']} · {x['title']} ({x['product']})",'', '- 입력/시작: '+x['input']]
 for key,label in [('submitting','로딩/처리 중'),('success','성공/결과'),('errorOrEmpty','빈 상태/실패'),('resume','닫기/재실행/복구')]:s+=['- '+label+': '+x['states'][key]]
 s+=['- 연결: '+', '.join(x['taskRefs'])+'; API '+(', '.join(x['apiRefs']) or '직접 호출 없음')+'; BLE '+(', '.join(x['bleCommands']) or '직접 명령 없음')]
 if x.get('signerFlowRef'):s+=['- 공통 signer 흐름: '+x['signerFlowRef']]
 if x.get('returnDesignRef'):s+=['- 반납 상세 후보: [상태·문구·복구 동작]('+x['returnDesignRef']+'). 후보 프로토콜의 제품 구현은 미완료.']
 s+=['']
views['screen-flows.md']='\n'.join(s)+'\n'
for name,value in views.items():
 if '--check' in sys.argv:assert (P/name).read_text()==value,name
 else:(P/name).write_text(value)
print(json.dumps({'api':len(a),'extendedDto':len(c),'access':len(m),'screens':len(screens),'mode':'check' if '--check' in sys.argv else 'render'}))
