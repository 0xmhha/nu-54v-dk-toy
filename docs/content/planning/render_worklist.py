"""Render planning documents from work-breakdown.json using only the stdlib.

Run: python3 content/planning/render_worklist.py [--check]
--check validates references and confirms that committed/generated views match.
This validates planning data, not product functionality.
"""
import csv
import io
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
data = json.loads((ROOT / 'work-breakdown.json').read_text())
tasks = data['tasks']
lookup = {t['id']: t for t in tasks}
assert len(lookup) == len(tasks) == data['taskCount']
epic_ids = {e['id'] for e in data['epics']}
decision_ids = {d['id'] for d in data['decisions']}
assert len(epic_ids) == len(data['epics'])
assert len(decision_ids) == len(data['decisions'])
assert {r for t in tasks for r in t['requirements']} == set(range(1, 16))
step_ids = set()
for t in tasks:
    assert t['epic'] in epic_ids
    assert set(t['dependsOn']) <= lookup.keys(), t['id']
    assert set(t['decisionInputs']) <= decision_ids, t['id']
    assert t['scenarioRefs']
    for step in t['implementationSteps']:
        assert step['id'].startswith(t['id'] + '.')
        assert step['id'] not in step_ids
        assert step['action'] and step['output']
        step_ids.add(step['id'])
assert len(step_ids) == data['subtaskCount']
done, active = set(), set()


def visit(task_id):
    assert task_id not in active, f'Dependency cycle: {task_id}'
    if task_id in done:
        return
    active.add(task_id)
    for dependency in lookup[task_id]['dependsOn']:
        visit(dependency)
    active.remove(task_id)
    done.add(task_id)


for task_id in lookup:
    visit(task_id)


def joined(value):
    if value is None:
        return ''
    if isinstance(value, list):
        return '; '.join(map(str, value))
    return str(value)


def csv_content(fields, rows):
    stream = io.StringIO(newline='')
    writer = csv.DictWriter(stream, fieldnames=fields, lineterminator='\n')
    writer.writeheader()
    for row in rows:
        writer.writerow({f: joined(row.get(f)) for f in fields})
    return '\ufeff' + stream.getvalue()


cards = [
    '# 상세 작업 카드 — 세부 실행 작업', '',
    f"{data['date']}. {len(data['epics'])}개 작업군, {len(tasks)}개 작업 패키지, "
    f"{len(step_ids)}개 세부 작업. 전체 범위와 앱 연동을 12주 안에 완료하는 계획 전제다. "
    '담당자·공수·마감일은 비워 두었다.', '',
    '[읽는 방법](../work-breakdown-plan.md) · [실행 시나리오](functional-execution-spec.md) · '
    '[JSON 원본](work-breakdown.json) · [패키지 CSV](work-breakdown.csv) · [세부 작업 CSV](implementation-steps.csv)', '',
    '작업 분해·상세 수용 기준·설계 제안은 검토용이다. 결정 입력은 구현 완료 전에 해결할 선택이며, '
    '조사·설계 착수를 금지하지 않는다. 선행 작업은 최종 통합 완료 관계다. '
    '세부 작업 번호는 식별자이며 모든 하위 작업의 순차 실행을 강제하지 않는다. '
    '시나리오 연결은 직접 실행 또는 해당 패키지의 제품 간 검증 맥락을 뜻한다. '
    '현재 제품 실행 증거는 비어 있으며, 기존 코드의 존재를 완료로 표시하지 않았다.', ''
]
for epic in data['epics']:
    cards += [f"## {epic['id']} — {epic['title']}", '']
    for t in tasks:
        if t['epic'] != epic['id']:
            continue
        cards += [f"### {t['id']} · {t['title']}", '']
        fields = [
            ('원문 연결', ', '.join(map(str, t['requirements'])) + '번'),
            ('산출물', t['deliverable']),
            ('완료 기준', '; '.join(t['acceptanceCriteria'])),
            ('선행 작업', ', '.join(t['dependsOn']) or '없음'),
            ('결정 입력', ', '.join(t['decisionInputs']) or '없음'),
            ('앱/제품 연결', t['integrationSurface']),
            ('재사용 기준', t['reuseBasis']),
            ('검증 환경', t['validationEnvironment']),
            ('연결 시나리오', ', '.join(t['scenarioRefs'])),
            ('상태', t['status']),
            ('담당자 / 공수 / 마감일', '미배정 / 미산정 / 미지정'),
        ]
        cards += [f'- {label}: {value}' for label, value in fields]
        if t.get('interfaceRefs'):
            refs = ', '.join(f"{r['entry']}" for r in t['interfaceRefs'])
            cards += [f'- 연결 명세: {refs}. [규칙과 카탈로그](../specifications/interface-contracts.md)']
        if t.get('screenRefs'):
            cards += [f"- 화면 연결: {', '.join(t['screenRefs'])}. [상태/복구 흐름](../specifications/screen-flows.md)"]
        if t.get('storageTables'):
            cards += [f"- 저장 연결: {', '.join(t['storageTables'])}. [참조 DDL](../specifications/database/README.md)"]
        if t.get('accessApiRefs'):
            cards += [f"- 권한/트랜잭션 연결: {', '.join(t['accessApiRefs'])}. [API 접근 표](../specifications/api-access-transactions.md)"]
        if t.get('securityIntegrationInputs'):
            cards += [f"- 남은 보안 연결 입력: {', '.join(t['securityIntegrationInputs'])}. [설계 결과](../specifications/security-integration-inputs.json)"]
        if t.get('securityAcceptanceRefs'):
            cards += [f"- 보안 수용 시나리오: {', '.join(t['securityAcceptanceRefs'])}. [흐름·완료 조건](../specifications/security-integration-design.md)"]
        if t.get('securityStorageRefs'):
            cards += [f"- 보안 논리 저장 계약: {', '.join(t['securityStorageRefs'])}. [Adapter 자원](../specifications/security-storage-contracts.json)"]
        if t.get('technologyRefs'):
            cards += [f"- 기술 선택: {', '.join(t['technologyRefs'])}. [선택안](technology-selection.md)"]
        if t.get('technologyValidationRefs'):
            cards += [f"- 구현 단계 기술 검증: {', '.join(t['technologyValidationRefs'])}. [검증 카드](technology-validation-plan.md)"]
        if t.get('implementationInterfaceRefs'):
            cards += [f"- 구현 인터페이스: {', '.join(t['implementationInterfaceRefs'])}. [구성요소 경계](../specifications/implementation-interfaces.md)"]
        if t.get('compatibilityRefs'):
            cards += [f"- 버전 호환 판정: {', '.join(t['compatibilityRefs'])}. [호환 행렬](../specifications/compatibility-matrix.md)"]
        if t.get('executionReadinessRef'):
            assert t['executionReadinessRef'] == t['id']
            cards += [f"- 착수·완료 증거: {t['executionReadinessRef']}. [구현 순서](implementation-sequence.md) · [증거 카드](execution-readiness.md)"]
        if t.get('lifecycleCaseRefs'):
            cards += [f"- 결제·환불·반납 설계 사례: {', '.join(t['lifecycleCaseRefs'])}. [책임·상태 전이 설계](../specifications/commerce-lifecycle-design.md)"]
        cards += ['', '| 세부 ID | 실행할 작업 | 검토할 산출물 |', '|---|---|---|']
        cards += [f"| {s['id']} | {s['action']} | {s['output']} |" for s in t['implementationSteps']]
        cards += ['']

decisions = [
    '# 결정 목록 — 작업별 제안과 확정할 결과', '',
    '12주 전체 완료·전 항목 앱 연동·총 3명·RN 키오스크·EOA 우선·고객 가스·StableNet 8283은 기존 결정이다. '
    '아래 19개는 그 범위 안의 세부 선택이다. 질문만 남기지 않고 작업용 제안과 결정 산출물을 적었다. '
    '제안값은 사용자가 확정한 값으로 취급하지 않는다. 구현 조사를 진행하며 구체화할 수 있다.', '',
    '[설계 통합 현황·결정 성격 구분](design-integration-register.md) · '
    '[실행 시나리오](functional-execution-spec.md) · [작업 카드](work-cards.md)', ''
]
for d in data['decisions']:
    assert set(d['affectedTasks']) <= lookup.keys()
    if d.get('technologyRefs'):
        decisions += [f"기술 비교/검증 연결 ({d['id']}): {', '.join(d['technologyRefs'])}. [선택 카드](technology-selection.md)", '']
    decisions += [f"## {d['id']} · {d['title']}", '', d['question'] + '.', '',
                  f"- 작업용 제안: {d['workingProposal']}",
                  f"- 확정할 결과: {d['resolutionDeliverable']}",
                  f"- 영향 작업: {', '.join(d['affectedTasks'])}",
                  '- 상태: 미정. 제안의 채택·변경 이유와 적용 작업을 기록한다.', '']

package_fields = ['id', 'epic', 'title', 'requirements', 'phase', 'deliverable',
                  'acceptanceCriteria', 'dependsOn', 'decisionInputs', 'integrationSurface',
                  'reuseBasis', 'validationEnvironment', 'scenarioRefs', 'status',
                  'owner', 'effortEstimate', 'dueDate', 'evidence', 'estimationReady']
step_fields = ['id', 'parentId', 'epic', 'requirements', 'action', 'output',
               'parentDependencies', 'decisionInputs', 'scenarioRefs', 'status',
               'owner', 'effortEstimate', 'dueDate']
step_rows = []
for t in tasks:
    for step in t['implementationSteps']:
        step_rows.append(dict(step, parentId=t['id'], epic=t['epic'], requirements=t['requirements'],
                              parentDependencies=t['dependsOn'], decisionInputs=t['decisionInputs'],
                              scenarioRefs=t['scenarioRefs']))
outputs = {
    'work-cards.md': '\n'.join(cards),
    'decisions.md': '\n'.join(decisions),
    'work-breakdown.csv': csv_content(package_fields, tasks),
    'implementation-steps.csv': csv_content(step_fields, step_rows),
}
for name, content in outputs.items():
    path = ROOT / name
    if '--check' in sys.argv:
        assert path.exists() and path.read_text() == content, f'Out of date: {name}'
    else:
        path.write_text(content)
print(json.dumps(dict(packages=len(tasks), subtasks=len(step_ids), epics=len(epic_ids),
                      decisions=len(decision_ids), dependencyGraph='acyclic',
                      mode='check' if '--check' in sys.argv else 'render'), ensure_ascii=False))
