"""Validate and render integration adoption plan. This never merges product contracts."""
import hashlib,json,sys
from pathlib import Path
P=Path(__file__).resolve().parent;R=P.parents[1]
d=json.loads((P/'integration-adoption-matrix.json').read_text())
def read(p):return json.loads((R/p).read_text())
def require(ok,msg):
    if not ok:raise ValueError(msg)
require(d['implementation']=='deferred_by_user','implementation gate')
require(not any(d[k]for k in ['canonicalMergedThisTurn','runtimeVerified','sqlApplied']),'unsupported completion status')
for name,h in d['sourceHashes'].items():require(hashlib.sha256((R/name).read_bytes()).hexdigest()==h,'source drift: '+name)
bundles={x['id']:x for x in d['bundles']};require(len(bundles)==8,'bundle inventory')
visiting=set();done=set()
def visit(i):
    require(i not in visiting,'dependency cycle')
    if i in done:return
    visiting.add(i)
    for dep in bundles[i]['finishDependsOn']:require(dep in bundles,'unknown dependency');visit(dep)
    visiting.remove(i);done.add(i)
for i in bundles:visit(i)
checks=[x for b in bundles.values()for x in b['acceptanceChecks']]
require(len({x['id']for x in checks})==24,'acceptance IDs')
for b in bundles.values():
    require({x['layer']for x in b['acceptanceChecks']}=={'contract','decision','runtime'},'acceptance layers')
    for f in b['sources']+b['targets']:require((R/'content/specifications'/f).exists(),'missing artifact: '+f)
    require(all(x['status']=='not_run'for x in b['acceptanceChecks']if x['layer']=='runtime'),'runtime status')
apis={x['id']for x in read('content/specifications/api-catalog.json')['operations']}
policies=read('content/specifications/authorization-policies.json')['policies']
require(len(apis)==d['baseline']['apis']==110 and len(policies)==60,'canonical counts')
for b in bundles.values():require(set(b['existingApis']+b['preserveApis'])<=apis,'API mapping')
require(len([x for x in d['dispositions']if x['kind']=='existing_http'])==10,'existing routes')
require({x['id']for x in d['dispositions']if x['kind']=='uncatalogued_http_alias'}=={'SR-01','SR-02'},'alias inventory')
rr=read('content/specifications/return-recovery-contract.json')
require(next(x for x in rr['openDecisions']if x['id']=='RR-DEC-01')['selection'] is None,'RR policy changed')
w=read('content/planning/work-breakdown.json');require(len(w['tasks'])==104 and w['subtaskCount']==320,'WBS changed')
require([x['number']for x in d['requirements']]==list(range(1,16)),'requirement coverage')
for x in d['requirements']:require(x['taskRefs']==[t['id']for t in w['tasks']if x['number']in t['requirements']],'requirement/WBS drift')
require({x['id']for x in d['decisionRouting']}=={x['id']for x in w['decisions']} and all(x['resolved'] is False for x in d['decisionRouting']),'decision status')
for inv in d['existingRuntimeCaseInventories']:require(len(read('content/specifications/'+inv['source'])[inv['collection']])==inv['count'],'case inventory changed')

def link(f):return f'[{f}](../specifications/{f})'
lines=['# 미병합 설계 통합 묶음과 수용 기준','',
'2026-09-18 · **기존 상세 설계의 통합표. 이번에 기준 카탈로그 병합·제품 구현·DB 실행은 하지 않았다.**','',
'최근 결제·환불·정산·소비자 보정 설계를 기존 DI-01~08에 다시 연결했다. 새로운 WBS 작업이나 개인별 배정을 만든 것이 아니다. 설계가 작성됐다는 사실과 기준 명세에 채택됐다는 사실, 실제 동작 검증을 분리한다.','',
'[구조 원본](integration-adoption-matrix.json) · [검사/렌더러](render_integration_adoption.py) · [검증 실행 기록](integration-adoption-validation.json) · [전체 설계 등록부](design-integration-register.md)','',
'## 1. 현재 상태','',
'| 영역 | 현재 판정 | 의미 |','|---|---|---|',
'| 승인 HTTP/BLE/권한/화면 | 기준에 이미 통합 | API110·권한60·논리자원28. 암호·실기·새SQL 검증은 미완료 |',
'| 승인 물리 저장 | 상세 후보 작성 | 테이블 후보16·이행8단계. 기존 SQL61/7에는 미반영 |',
'| 결제·환불/매출·정산/운영 | 전체 계약 후보 작성 | 기존 HTTP10개 중 API037은 보존 검토, 나머지9개는 변경안. SR2개는 미등록 |',
'| 이벤트·소비자 보정 | 상세 후보 작성 | 이벤트2개 v2, 소비자5개·논리자원6개·재구축7단계. 카탈로그 미병합 |',
'| 반납 | 논리 프로토콜·상태/화면 후보 | RP8개는 HTTP/BLE 전체 경로 8개라는 뜻이 아님. 호출 방향·권한·조회 조합 보완 필요 |',
'| 전체 제품 | 범위 유지, 설계 진행 | 15요구/104작업/320세부작업/19결정. 3명·12주·앱 연동 목표 유지 |','',
'예전 후보의 API107/보안자원15 표기는 작성 당시 이력이다. 현재 기준과 더하거나 최신 숫자로 과거 파일을 덮어쓰지 않는다. logical resource·table candidate·실제 reference SQL table 수 역시 서로 다른 분류다.','',
'## 2. 통합 묶음 8개','',
'각 묶음의 파일은 **같은 설계 변경으로 함께 검토할 대상**이다. 여러 파일을 동시에 수정한다는 뜻이지 서비스 간 분산 transaction을 보장한다는 뜻이 아니다. 소스 후보는 원본 hash로 고정하고, 실제 채택 단계에서 전후 checkpoint와 변경 처분을 새로 남긴다.','']
for b in d['bundles']:
    lines += [f"### {b['id']} · {b['focus']}",'',f"완료 선행: {', '.join(b['finishDependsOn']) or '없음'} · 결정 연결: {', '.join(b['decisionRefs'])}",'',
      '근거: '+' · '.join(link(f)for f in b['sources']),'',
      '반영 대상: '+' · '.join(link(f)for f in b['targets']),'',
      '**남은 구체 작업**','']+['- '+x for x in b['remaining']]+['',
      '| 수용 ID | 계층 | 판정 기준 | 현재 |','|---|---|---|---|']
    for x in b['acceptanceChecks']:lines.append(f"| {x['id']} | {dict(contract='설계 채택',decision='결정/미정 경계',runtime='실제 실행')[x['layer']]} | {x['criterion']} | {'미실행' if x['layer']=='runtime' else '기준 정의·채택 미완료'} |")
    lines+=['']
lines += ['## 3. HTTP/프로토콜 처분표','',
'| 분류 | ID | 처리 | 근거 |','|---|---|---|---|']
for x in d['dispositions']:lines.append(f"| {x['kind']} | {x['id']} | {x['disposition']} | {link(x['source'])} |")
lines += ['',
'기존 HTTP10개는 commerce5개와 settlement/ops5개의 합집합이다. API037은 이미 채택된 전체 승인 계약을 유지하므로 신규 변경 건수로 다시 계산하지 않는다. API018/020도 이번 후보를 이유로 광범위한 union이나 결과 URL 경로로 변경하지 않는다. SR2개와 RP8개를 합쳐 미래 정식 API 개수를 예측하지 않는다.','',
'## 4. 덮어쓰면 안 되는 기준','']+['- **'+x['id']+'**: '+x['rule'] for x in d['preservedInvariants']]+['',
'## 5. 채택 순서와 실행 경계','',
'여기서 순서는 문서 검토 단계다. W01은 DI08의 원본 보호 작업만 선행 수행하며 DI08 전체 완료를 뜻하지 않는다. 각 DI의 완료 의존성은 2절을 따른다.','']
for x in d['waves']:lines += [f"### {x['id']} · {x['name']}",'',f"연결: {', '.join(x['bundleRefs'])}",'','진입: '+x['entry'],'','출구: '+x['exit'],'']
lines += ['정책이 미정이어도 선택지를 명시하고 미지원 실행을 차단하는 **초안 명세 통합**은 진행할 수 있다. 운영 정책의 선택이나 실제 서비스 활성화까지 완료됐다고 표시할 수는 없다. 반대로 실제 하드웨어 시험을 아직 하지 않았다는 이유로 모든 문서 통합을 무기한 막지도 않는다. 구현 착수는 사용자가 아직 보류한 상태다.','',
'실제 채택에서는 alias 정식 ID와 권한·버전 협상을 함께 등록하고, 후보별 상태를 보존/부분반영/전체반영으로 기록한다. 카탈로그 개수를 먼저 바꾸고 미완성 schema나 검증되지 않은 alias를 완료로 채우지 않는다.','',
'## 6. 정책 결정과 엔지니어링 작업 분리','',
'RR-DEC-01은 이미 질문한 복구 정책이며 selection=null을 유지한다. 이번 작업에서 재질문하거나 새 backup/export/reset 정책을 선택하지 않는다. 아래 나머지 정책도 새로 승인받은 것으로 처리하지 않는다. D02/D10 같은 엔지니어링 항목은 사용자 취향 질문으로 넘기지 않고 문서·코드·프로필 근거를 모아 계속 구체화할 수 있다.','',
'| 결정 | 사용자 판단 항목 | 계속 설계할 항목 | 실제 검증 대상 |','|---|---|---|---|']
for x in d['decisionRouting']:lines.append(f"| {x['id']} {x['title']} | {'; '.join(x['policyInputs']) or '별도 취향 질문 없음'} | {'; '.join(x['engineeringDesign'])} | {'; '.join(x['runtimeEvidence'])} |")
lines += ['',
'## 7. 15개 요구사항 범위 확인','',
'아래는 기존 WBS에서 다시 읽은 연결이다. 최근 문서가 결제·운영에 집중돼도 FOTA·패스키·녹음·MPC·온체인 상품·여행 기능을 범위에서 제거하지 않는다. 작업은 여러 요구사항에 겹칠 수 있으며 행별 작업 수를 합산해 총 작업량/진행률로 사용하지 않는다. 역할·공수·일정 배정은 이번에 하지 않았다.','',
'| 요구 | 기존 WBS 연결 수 | 상태 |','|---|---:|---|']
for x in d['requirements']:lines.append(f"| {x['number']}. {x['title']} | {x['taskCount']} | 범위 유지·완료 판정 아님 |")
lines += ['',
'현재 source dispatch에서 smart_account/market_action/credential_proof/paid_resource는 adapter_pending_execution_blocked다. 이들은 12주 범위에 남아 있으며 개인 EOA 승인 adapter로 우회하지 않는다. passkey 실연동, BLE 녹음, MPC 참여자/키 통제, FOTA board target 등은 각 D/IF/COMP 검증 작업을 유지한다.','',
'## 8. 증거 종류와 실행 사례 재사용','',
'| 증거 | 입증하는 것 | 입증하지 않는 것 |','|---|---|---|',
'| source hash·파일/참조 검사 | 어느 설계를 비교했는지, 참조가 존재하는지 | 보안·정책 승인·실제 호출 가능성 |',
'| 합성 schema·산술 예제 | 정해진 입력 형태와 유한 불변식 | 인증 진위·DB 경쟁·메시지 장애 복구 |',
'| 기존 reference SQL 시험 | 당시 61개 테이블에 한정한 제약 결과 | 새 companion/adapter 제약 |',
'| 향후 실환경 증거 | 실제 버전·입력·관측 결과가 기록된 범위 | 모든 환경/미실행 기능에 대한 일반 보장 |','',
'기존 실행 수용 사례 inventory(모두 미실행):','',
'| 근거 | 항목 수 | 상태 |','|---|---:|---|']
for x in d['existingRuntimeCaseInventories']:lines.append(f"| {link(x['source'])} · {x['collection']} | {x['count']} | not_run |")
lines += ['',
'위 사례는 중복될 수 있어 하나의 고유 테스트 총수로 합치지 않는다. 이번 DI 수용 기준24개도 기존 runtime 사례 개수에 더해 완료율을 만들지 않는다. 실제 실행 기록에는 artifact/profile/SDK/board/chain/권한 상태, 입력, 기대 결과, 관측 결과, 복구 결과가 필요하다.','',
'## 9. 다음 작업','',
'다음은 **반납 RP-01~08의 전송 방향·HTTP/BLE 경로·전체 envelope·현재 재대여 gate 조회 계약**이다. 먼저 빈 호출 경로와 상태 조회 부족을 해소한다. RR-DEC-01 미답변이어도 기존 증거 복구·권한 분리·조회 설계는 진행하고, 신규 파괴적 reset 승인 정책은 선택하지 않는다.','']
output='\n'.join(lines)
p=P/'integration-adoption-matrix.md'
if '--check'in sys.argv:require(p.read_text()==output,'stale reading view')
else:p.write_text(output)
print(json.dumps({'scope':'design_integration_inventory_only','bundles':8,'acceptanceCriteria':24,'dispositions':20,'originalRequirements':15,'openDecisions':19,'pinnedSources':len(d['sourceHashes']),'canonicalMergedThisTurn':False,'runtimeVerified':False},ensure_ascii=False))
