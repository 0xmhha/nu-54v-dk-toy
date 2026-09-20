"""Render the resolved review without rewriting the pre-fix evidence."""
import json,hashlib
from pathlib import Path
O=Path(__file__).resolve().parent;R=O.parents[2]
read=lambda p:json.loads(p.read_text())
f=read(O/'findings.json');a=read(O/'structural-audit.json');g=read(O/'graph.json')
changes={'LG-01':'unknown의 원 allowance/실행 관측별 복구 전이 6개와 실패 근거 reorg 재검증 전이를 추가했다. 실행 노출이 있는 상태에서 review_ready로 돌아가 재서명할 수 없도록 제한했다.', 'LG-02':'CredentialStatus, PresentationSession, 불변 VerificationRecord를 분리했다. 같은 active credential을 서로 다른 challenge에 독립 제시하며, challenge 단회 소비와 철회 검사는 유지한다.', 'LG-03':'PR-COMMON / PR-GRANT / PR-GENERATE / PR-PUBLISH를 정의하고 PX05/06/07/11에서 참조한다. 최초 실행·재시도·공개가 동일 refund/authority/privacy/source/revision 조건을 검사한다.', 'LG-04':'OC17~20으로 refresh 요청/결과조회와 identity unlink 요청/결과조회를 추가했다. 회전 단일 successor, 이전 bearer 재전달 제한, 병렬 마지막 로그인 수단 제거 방지를 명시했다.', 'LG-05':'8개 DS와 전체 설계·인계서에 declaredScopeRequirementRefs와 transitiveTaskImpactRequirementRefs를 분리했다. WBS 104개 작업의 원 요구사항은 보존했다.'}
lines=['# 문서 관계 그래프와 논리 보완 결과','','**LG-01~05 모두 원본 설계에 수정 반영했다.** 기존 반례와 수정 전 파일은 보존했다. 논리 설계 회귀 검사와 문서 참조 검증을 수행했으며 제품의 보안·실기·체인 실행 검증은 아니다.','','x-theory 정의는 아직 미확인이다. 아래 분류와 관계는 중립적인 provenance 분석이며 x-theory 적용 완료를 주장하지 않는다.','',f'목록: {g["counts"]["document"]}개 문서/스키마/참조 SQL·GraphQL. 상세 의미 검토는 12개 구조화 핵심 문서 중심이다. 그래프는 {len(g["nodes"])}개 노드·{len(g["edges"])}개 관계이며, 과거 보존 디렉터리와 이 분석 폴더의 수정 전 스냅샷은 입력에서 제외한다.','','## 반영한 수정','']
for x in f['findings']:
 lines += [f'### {x["id"]} · 수정 반영','',changes[x['id']],'','수정 전 문제: '+x['claim'],'','회귀 기준: '+x['acceptance'],'']
 for e in x['resolutionEvidence']:
  assert hashlib.sha256((R/e['file']).read_bytes()).hexdigest()==e['sha256']
  lines += [f'- 현재 근거: [{Path(e["file"]).name}]({R/e["file"]}) — `{e["pointer"]}`']
 lines+=['']
lines+=['## 수정된 관계','', '```mermaid','flowchart LR',' R["요구 15"] --> T["작업 104 · 단계 320"]',' T --> P["DS01–08"]',' P --> S["주요 선언 범위"]',' T --> I["작업에서 계산한 영향 범위"]',' P --> C["API110 · 후보 계약20"]',' C --> G["공통 guard · 상태별 복구"]',' G --> V["설계 회귀 검사"]',' V -. "실제 실행 증거는 별도" .-> E["기기·앱·체인 검증 미실행"]','```','', '```mermaid','stateDiagram-v2',' execution_pending --> unknown: 응답 유실',' unknown --> execution_pending: 원 실행 pending 확인',' unknown --> result_observed: 원 receipt와 실제 효과 확인',' result_observed --> completed: 확정 정책과 현재성 검사',' unknown --> failed_confirmed: 확정 실패 및 노출 해소',' failed_confirmed --> unknown: 실패 근거 reorg','```','', '```mermaid','flowchart LR',' C["Credential active"] --> A["Presentation A / challenge A"]',' C --> B["Presentation B / challenge B"]',' A --> VA["불변 Verification A"]',' B --> VB["불변 Verification B"]',' RE["현재 철회·신뢰·만료 검사"] --> A',' RE --> B','```','', '## 검증 결과와 경계','',f'- WBS 의존 관계 {a["wbsDependencyEdges"]}개: 순환 {len(a["wbsCycles"])}개.',f'- 원본 source hash {a["sourceHashChecks"]}개: 현재 참조 불일치 {len(a["hashMismatches"])}개.', '- `validate_logic_fixes.py`: 수정 전 결함 재현과 수정 후 전이·객체·guard·계약 연결·범위 구분을 검사한다. 형식 모델/판정표 검사이며 실제 경쟁 상황이나 암호 구현을 증명하지 않는다.', '- 후보 API는 논리 계약으로 추가했다. 기존 카탈로그110개를 런타임 API가 구현된 것으로 변경하지 않았다.', '- 정책19개와 RR-DEC-01은 미선택 상태를 유지한다.','', '## 산출물과 재현','', '- [전체 그래프 JSON](graph.json) · [DOT](graph.dot) · [검토 주석 포함 그래프](review-graph.json)', '- [수정 전 근거와 수정 상태](findings.json) · [해시 변경 이력](source-pin-rebase.json) · [검사 결과](correction-validation.json)','', '```sh','python3 content/analysis/document-logic/build_graph.py','python3 content/analysis/document-logic/validate_logic_fixes.py','python3 content/analysis/document-logic/render_review.py','```']
(O/'review.md').write_text('\n'.join(lines)+'\n')
for x in f['findings']:
 id='finding:'+x['id'];g['nodes'].append(dict(id=id,kind='finding',label=x['id']+' 수정 반영',status=x['status'],source='content/analysis/document-logic/findings.json'))
 for e in x['resolutionEvidence']:g['edges'].append(dict(source=id,target='doc:'+e['file'],relation='resolved_by_design_change',provenance=e,basis='validated_design_correction'))
g['counts']['finding']=len(f['findings']);(O/'review-graph.json').write_text(json.dumps(g,ensure_ascii=False,indent=2)+'\n')
print('Updated resolved findings, graph annotations and Mermaid views.')
