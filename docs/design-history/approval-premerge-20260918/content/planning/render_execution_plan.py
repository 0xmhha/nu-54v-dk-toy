"""Generate task entry/evidence cards and audit completion dependencies (stdlib only).
--check checks planning consistency/generated views, not implementation readiness.
"""
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[1]
wbs = json.loads((ROOT / 'work-breakdown.json').read_text())
plan = json.loads((ROOT / 'implementation-sequence.json').read_text())
tasks = {t['id']: t for t in wbs['tasks']}
steps = {s['id']: s for t in tasks.values() for s in t['implementationSteps']}
partials = {p['id']: p for p in plan['partialDeliverables']}
assert len(tasks) == 104 and len(steps) == 320
assert len(partials) == len(plan['partialDeliverables'])
assert plan['firmwareBase'] == wbs['confirmedFirmwareBase']['firmwareBase'] == 'Zephyr'
assert plan['readinessStatus'] == 'not_assessed'
assert set(plan['specialConditions']) <= tasks.keys()
assert set(plan['designEvidenceTasks']) <= tasks.keys()
assert set(plan['additionalAcceptanceTasks']) <= tasks.keys()
for ids in plan['additionalAcceptanceTasks'].values():
    assert set(ids) <= tasks.keys()
for p in partials.values():
    assert set(p['producerSteps']) <= steps.keys()
    assert set(p['requiredTaskOutcomes']) <= tasks.keys()
    assert set(p['consumers']) <= tasks.keys()
    assert p['evidenceRefs'] == [] and p['status'] == 'planned'
    assert p['acceptanceCriteria']

partial_refs = {i: [p['id'] for p in partials.values() if i in p['consumers']] for i in tasks}
original = {i: t['dependsOn'] for i, t in tasks.items()}
effective = {i: sorted(set(t['dependsOn'] + plan['additionalAcceptanceTasks'].get(i, []) + partial_refs[i])) for i, t in tasks.items()}
effective.update({i: p['requiredTaskOutcomes'] for i, p in partials.items()})


def levels(graph):
    done, result = set(), []
    assert all(set(ds) <= graph.keys() for ds in graph.values())
    while len(done) < len(graph):
        batch = sorted(i for i, ds in graph.items() if i not in done and set(ds) <= done)
        assert batch, 'Completion dependency cycle: ' + ', '.join(sorted(set(graph) - done))
        result.append(batch)
        done.update(batch)
    return result


def closure(ids):
    result = set()
    def visit(i):
        if i in result:
            return
        result.add(i)
        for dependency in effective[i]:
            visit(dependency)
    for i in ids:
        visit(i)
    return sorted(result)


original_layers = levels(original)
effective_layers = levels(effective)
assert set(tasks) <= set(closure(['RELEASE-03'])), 'Release leaves out required packages'
assert {'HW-04', 'MPC-02'} <= set(effective['APP-02'])
assert {'AUTH-02', 'AUTH-03'} <= set(effective['MPC-02'])
assert {'PART-OTA-POLICY', 'PART-DEVICE-ARBITRATION'} <= set(effective['OTA-03'])
# Early index subset must not inherit complete expanded-product prerequisites.
assert not {'STO-02', 'X402-02', 'INDEX-04', 'INDEX-05'} & set(closure(['PART-CORE-INDEX']))
assert 'OTA-03' not in closure(['PART-OTA-POLICY', 'PART-DEVICE-ARBITRATION'])

REF_FIELDS = ['decisionInputs', 'scenarioRefs', 'interfaceRefs', 'screenRefs', 'technologyRefs', 'technologyValidationRefs', 'implementationInterfaceRefs', 'compatibilityRefs', 'securityAcceptanceRefs', 'securityIntegrationInputs', 'storageTables', 'securityStorageRefs']
rows = []
for i, t in tasks.items():
    assert t['executionReadinessRef'] == i
    assert t['status'] == 'planned' and not t['evidence']
    assert t['owner'] is None and t['effortEstimate'] is None and t['dueDate'] is None
    design = i in plan['designEvidenceTasks']
    dependencies = [dict(taskId=d, requiredOutput=tasks[d]['deliverable']) for d in t['dependsOn']]
    row = dict(taskId=i, title=t['title'], readiness='not_assessed', inputs={f:t.get(f, []) for f in REF_FIELDS}, dependencyInputs=dependencies,
        designEntry=['연결된 요구사항·시나리오·결정 입력을 읽고 이번 작업 경계를 명시한다. 미선택 기술은 가정으로 표시한다.'],
        componentEntry=['사용할 인터페이스/DTO와 실패 상태를 정하고 대상 기술 검증 카드를 확인한다.', '독립 구현은 합의한 계약/fixture로 시작 가능하다. 선행 작업 전체 완료를 착수 조건으로 오해하지 않는다.', '빌드·실행이 필요한 단계 전에는 그 단계에 필요한 SDK/board/provider/chain profile을 선택·기록한다.'],
        integrationEntry=['실제로 호출하는 선행 산출물·앱/기기 build·endpoint·권한·데이터 준비 증거를 확인한다.', '호환 판정 항목의 실제 조합을 고정하고 정상·거절·복구 절차를 준비한다.'],
        preApplyPartialRefs=partial_refs[i] if i == 'OTA-03' else [],
        fullCompletionDependencies=t['dependsOn'], additionalAcceptanceTasks=plan['additionalAcceptanceTasks'].get(i, []), completionPartialRefs=partial_refs[i],
        acceptanceCriteria=t['acceptanceCriteria'], deliverable=t['deliverable'], validationEnvironment=t['validationEnvironment'],
        evidenceProfile='design' if design else 'integration', evidenceRules=plan['evidenceRules']['design' if design else 'integration'] + plan['evidenceRules']['acceptance'],
        specialConditions=plan['specialConditions'].get(i, []),
        stepEvidence=[dict(stepId=s['id'], action=s['action'], expectedArtifact=s['output'], status='not_run', evidenceRefs=[]) for s in t['implementationSteps']], evidenceRefs=[])
    rows.append(row)
assert sum(len(r['stepEvidence']) for r in rows) == 320
checkpoints=[]
for c in plan['checkpoints']:
    assert set(c['anchorTasks']) <= tasks.keys()
    checkpoints.append(dict(**c, requiredOutcomes=closure(c['anchorTasks']), status='not_assessed', evidenceRefs=[]))
assert set(tasks) <= set(checkpoints[-1]['requiredOutcomes'])
result = dict(schemaVersion='1.0', status='planning_only', taskCount=len(rows), stepCount=len(steps), partialCount=len(partials), originalDependencyEdges=sum(map(len, original.values())), originalCompletionLayers=original_layers, effectiveCompletionLayers=effective_layers, checkpoints=checkpoints, tasks=rows)

md = ['# 구현 순서·착수 조건·완료 증거', '',
'104개 패키지·320개 세부 작업의 기존 범위를 유지한다. **15개 요구사항 모두 앱 연동까지 12주 완료**, 참여자 3명, 개인 배정·공수·마감일 산정은 보류다. 펌웨어는 **Zephyr**이며 SDK 배포판·버전·NU board target은 미선정이다.', '',
'현재는 계획 검증만 수행했다. 모든 작업의 구현 착수 가능 여부는 `not_assessed`, 실행 증거는 비어 있다. 문서가 존재한다는 이유로 작업을 완료로 판정하지 않는다.', '',
'[104개 착수/증거 카드](execution-readiness.md) · [기계 판독 데이터](execution-readiness.json) · [순서/예외 원본](implementation-sequence.json) · [증거 양식](execution-evidence.template.json)', '',
'## 진행 방식', '',
'1. 설계: 기존 요구·화면·DTO·결정 입력을 확인한다. 미해결 선택은 가정과 변경 영향을 기록한다.',
'2. 독립 구현: 해당 인터페이스와 검증 환경이 정해지면 시작한다. 필요한 선행 산출물의 일부를 사용해도 된다. mock/fixture 사용은 증거에 표시한다.',
'3. 통합: 실제로 호출할 선행 기능과 버전·권한·데이터가 준비되어야 한다. 전체 패키지가 끝나기 전 준비된 일부를 연결할 수 있다.',
'4. 완료: 원래 `dependsOn`, 추가 완료 작업, 필수 부분 산출물, 기존 수용 기준을 모두 확인한다. 실기·앱·체인 통합이 필요한 작업은 모의 결과로 완료할 수 없다.', '',
'기존 `dependsOn`은 최종 통합 완료 관계다. 부분 산출물은 기존 세부 작업의 범위만 나눈 것으로 104개 패키지나 320개 세부 작업을 늘리지 않는다. 생산 작업 전체의 완료를 요구하지 않으며, 지정된 선행 산출물과 부분 수용 증거를 요구한다. 부분 통합에 필요한 화면/조회 probe는 생산자와 소비자 구현 중 함께 검증하고, 소비자 전체 완료와 구분한다.', '',
'## 통합 확인 순서', '',
'아래는 확인 지점이며 주차·스프린트·인력 배치표가 아니다. CHECK 번호가 다음 작업의 일괄 착수 금지를 의미하지 않는다. 준비된 계약군·기기 기능·여행 기능은 동시에 개발할 수 있고, 각 카드의 실제 입력 조건을 따른다. EOA 이후 스마트 계정 순서는 유지한다.', '',
'| 확인 지점 | 대표 완료 작업 | 확인할 사용자 결과 |', '|---|---|---|']
for c in checkpoints:
    md.append(f"| {c['id']} {c['title']} | {', '.join(c['anchorTasks'])} | {c['outcome']} |")
md += ['', 'CHECK-07의 의존성 폐쇄에는 104개 작업 모두가 포함된다. 각 확인 지점의 전체 선행 집합은 execution-readiness.json의 `checkpoints[].requiredOutcomes`에 있다.', '', '## 먼저 준비할 부분 산출물', '']
for p in partials.values():
    md += [f"### {p['id']} — {p['title']}", '', f"- 기존 세부 작업: {', '.join(p['producerSteps'])}", f"- 필요한 완료 산출물: {', '.join(p['requiredTaskOutcomes'])}", f"- 소비 작업: {', '.join(p['consumers'])}"]
    md += ['- 수용 증거: ' + s for s in p['acceptanceCriteria']]
    md += ['']
md += ['## 추가 완료 조건', '',
'APP-02는 실제 HW-04/MPC-02 지갑을 연결해야 끝난다. MPC-02는 Google과 Apple 실제 로그인에 따른 생성·재조회까지 확인한다. 이 조건은 독립 화면/DKG 개발의 착수를 막지 않는다.', '',
'OTA-03의 실제 이미지 적용 전에 OTA-04.01 호환/복귀 정책과 HW-07.01 중재 정책을 준비한다. 정책이 존재해도 OTA-04 호환/마이그레이션 시험이나 HW-07 동시 동작 실기 시험이 완료된 것은 아니다.', '',
'PART-CORE-INDEX는 초기 EOA 입금·거래 관측에 필요한 범위로 먼저 준비한다. INDEX-04/05 전체 완료는 모든 확장 상품과 운영 연동 증거를 요구한다. DEX/FX/Perp/STO/DID/x402/스마트 계정 앱은 해당 상품의 부분 조회 증거를 제출해야 끝난다.', '',
'## 증거 기록 규칙', '',
'증거 양식은 실행 시 taskId, stepIds, stage(design/component/integration/acceptance), runAt, sourceRevision, build/env를 채운다. acceptanceResults에는 기준 ID 또는 원문, expected, observed, result(pass/fail/not_run), artifactRefs를 기록한다. artifacts에는 접근 가능한 경로와 hash를 넣는다. 실제 실행 명령/수동 절차와 실패/복구 결과도 남긴다. 해당하지 않는 필드는 이유를 기록한다.', '',
'컴포넌트 시험과 실제 통합 시험을 구분한다. 스마트 계정·EVM·MPC·passkey 등 다른 서명 방식의 성공을 서로 대신하지 않는다. 지갑/체인 증거는 실제 signer·chainId·주소·거래/업무 식별자를 연결한다. 비밀·민감한 원문은 로그에 남기지 않는다. 증거 JSON 양식은 자동 합격 판정기가 아니며 검토자가 수용 기준을 대조해야 한다.', '',
'## 의존성 검산', '',
f"기존 전체 완료 관계: {sum(map(len, original.values()))}개 간선, {len(original_layers)}개 위상 계층. 추가 조건·부분 산출물을 포함한 그래프도 순환이 없다. 계층 수는 주차나 공수, 임계 경로 길이 추정이 아니다.", '',
'| 기존 완료 계층 | 작업 |', '|---|---|']
for index, layer in enumerate(original_layers, 1):
    md.append(f"| {index} | {', '.join(layer)} |")
md += ['', '## 현재 단계와 다음 설계 작업', '',
'사용자 확인에 따라 현재 단계는 상세 설계이며 구현은 보류한다. 이 문서의 착수 조건과 구현 순서는 향후 실행 계획으로, 지금 구현을 시작한다는 의미가 아니다. 이후 일반적인 “다음 진행” 요청도 설계 작업의 연속으로 해석한다. 제품 코드 작성·개발환경 설치·배포로의 전환은 사용자가 구현 착수를 지시한 뒤 진행한다.', '',
'다음 설계는 기존 명세를 기준으로 미결정 사항과 설계 간 불일치를 먼저 점검한다. 제품별 책임·신뢰 경계, 핵심 여정의 상태 전이와 실패/복구, 지갑·서명·키 수명주기, 앱 화면별 동작을 구체화한다. 새 문서를 중복 생성하기보다 기존 작업·인터페이스·수용 기준에 결정을 반영한다. 개인별 배정과 공수 산정은 계속 보류한다.', '']

cards = ['# 작업별 착수 조건·완료 증거 카드', '', '[진행 방식과 부분 산출물](implementation-sequence.md) · [JSON](execution-readiness.json)', '', '모든 카드: 착수 가능 여부 미평가, 실행 증거 없음. 설계/독립 구현과 최종 통합 완료를 구분한다.', '']
for r in rows:
    cards += [f"## {r['taskId']} — {r['title']}", '', '- 산출물: '+r['deliverable'], '- 검증 환경: '+r['validationEnvironment']]
    for label, key in [('설계 착수', 'designEntry'), ('독립 구현 착수', 'componentEntry'), ('통합 착수', 'integrationEntry')]:
        cards += [f'- {label}: '+ ' '.join(r[key])]
    if r['dependencyInputs']:
        cards += ['- 선행 입력: '+ '; '.join(d['taskId']+' → '+d['requiredOutput'] for d in r['dependencyInputs'])]
    cards += ['- 기존 전체 완료 의존: '+(', '.join(r['fullCompletionDependencies']) or '없음'), '- 추가 완료 작업: '+(', '.join(r['additionalAcceptanceTasks']) or '없음'), '- 필수 부분 증거: '+(', '.join(r['completionPartialRefs']) or '없음')]
    if r['preApplyPartialRefs']:
        cards += ['- 실제 FOTA 적용 전 준비 필수: '+', '.join(r['preApplyPartialRefs'])]
    for key, label in [('decisionInputs','결정'),('technologyValidationRefs','기술 검증'),('implementationInterfaceRefs','구현 인터페이스'),('compatibilityRefs','호환 판정'),('scenarioRefs','종단 시나리오')]:
        if r['inputs'][key]:cards += [f"- {label}: "+ ', '.join(r['inputs'][key])]
    cards += ['- 추가 조건: '+s for s in r['specialConditions']]
    cards += ['', '완료 기준:', '']+['- '+s for s in r['acceptanceCriteria']]
    cards += ['', '필수 증거:', '']+['- '+s for s in r['evidenceRules']]
    cards += ['', '| 세부 작업 | 실행 내용 | 제출할 산출물 | 실행 상태 |', '|---|---|---|---|']
    for s in r['stepEvidence']:
        vals=[s['stepId'],s['action'],s['expectedArtifact'],'미실행']
        cards.append('| '+' | '.join(v.replace('|', '\\|').replace('\n',' ') for v in vals)+' |')
    cards += ['']
outputs = {'execution-readiness.json': json.dumps(result, ensure_ascii=False, indent=2)+'\n', 'implementation-sequence.md': '\n'.join(md), 'execution-readiness.md': '\n'.join(cards)}
for name, value in outputs.items():
    target = ROOT/name
    if '--check' in sys.argv:
        assert target.exists() and target.read_text() == value, f'Stale generated view: {target}'
    else:
        target.write_text(value)
print(f'Planning OK: {len(tasks)} packages, {len(steps)} step evidence slots, {len(partials)} partial deliverables; {sum(map(len, original.values()))} original edges; original/effective DAG valid; release covers all 104. No runtime evidence evaluated.')
