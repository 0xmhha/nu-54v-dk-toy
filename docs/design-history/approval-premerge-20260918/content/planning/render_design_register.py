"""Validate the design register and render its reading view; no product tests.

Run: python3 content/planning/render_design_register.py [--check]
"""
import json
import os
import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]


def read(path):
    return json.loads((ROOT / path).read_text())


data = read('content/planning/design-integration-register.json')
wbs = read('content/planning/work-breakdown.json')
tech = read('content/planning/technology-selection.json')
validation = read('content/planning/technology-validation-plan.json')
recovery = read('content/specifications/return-recovery-contract.json')
tasks = {t['id']: t for t in wbs['tasks']}
decisions = {d['id']: d for d in wbs['decisions']}
bundles = {b['id']: b for b in data['integrationBundles']}
tech_ids = {t['id'] for t in tech['choices']}
assert len(bundles) == len(data['integrationBundles'])
assert len({d['id'] for d in data['decisions']}) == len(data['decisions'])
assert {d['id'] for d in data['decisions']} == decisions.keys()
assert {r for t in tasks.values() for r in t['requirements']} == set(range(1, 16))
assert data['phase'] == wbs['phaseControl']['currentPhase'] == 'detailed_design'
assert data['implementation'] == wbs['phaseControl']['implementation'] == 'deferred_by_user'
assert data['runtimeVerified'] is False
for d in data['decisions']:
    source = decisions[d['id']]
    assert d['sourceStatus'] == source['status']
    assert d['taskRefs'] == source['affectedTasks']
    assert d['technologyRefs'] == source['technologyRefs']
    assert set(d['taskRefs']) <= tasks.keys()
    assert set(d['technologyRefs']) <= tech_ids
    assert d['resolved'] is False
for entry in data['currentLayers'] + data['integrationBundles']:
    for path in entry['sourceFiles']:
        assert (ROOT / path).is_file(), path
for design in data.get('relatedDesigns', []):
    assert (ROOT / design['document']).is_file()
    candidate = read(design['source'])
    assert candidate['canonicalMerged'] is design['canonicalMerged'] is False
    assert candidate['runtimeVerified'] is design['runtimeVerified'] is False
    assert len(candidate['policyCandidates']) == design['policyCandidateCount']
    assert len(candidate['contractImpacts']) == design['contractImpactCount']
    assert len(candidate['cases']) == design['reviewCaseCount']
for b in bundles.values():
    assert set(b['decisionRefs']) <= decisions.keys()
    assert set(b['finishDependsOn']) <= bundles.keys()
    assert b['runtimeVerified'] is False
for difference in data['differences']:
    assert set(difference['bundleRefs']) <= bundles.keys()
for item in data['designQueue']:
    assert set(item['decisionRefs']) <= decisions.keys()
active, done = set(), set()


def visit(key):
    assert key not in active, f'Design dependency cycle: {key}'
    if key in done:
        return
    active.add(key)
    for dep in bundles[key]['finishDependsOn']:
        visit(dep)
    active.remove(key)
    done.add(key)


for key in bundles:
    visit(key)
question = data['pendingUserQuestion']
source_question = next(d for d in recovery['openDecisions'] if d['id'] == question['sourceEntry'])
assert source_question['selection'] is question['selection'] is None
assert source_question['status'] == 'awaiting_user_preference'
assert question['status'] == 'awaiting_answer'
assert question['reaskThisTurn'] is False and question['doNotInferApprovalFromContinuation']
assert (ROOT / data['scopeSource']).is_file()


def cell(value):
    if isinstance(value, list):
        value = '；'.join(map(str, value)) if value else '—'
    return str(value).replace('|', '\\|').replace('\n', ' ')


def link(path):
    return f'[{Path(path).name}]({os.path.relpath(ROOT / path, HERE)})'


def table(headers, rows):
    return ['| ' + ' | '.join(headers) + ' |', '| ' + ' | '.join(['---'] * len(headers)) + ' |'] + [
        '| ' + ' | '.join(cell(c) for c in row) + ' |' for row in rows
    ] + ['']


counts = [(title, len(read(path)[key])) for title, path, key in [
    ('API', 'content/specifications/api-catalog.json', 'operations'),
    ('BLE 논리 명령', 'content/specifications/ble-catalog.json', 'commands'),
    ('이벤트', 'content/specifications/event-catalog.json', 'events'),
    ('화면', 'content/specifications/screen-flows.json', 'screens'),
]]
lines = [
    '# 설계 통합 현황과 남은 결정', '',
    f"{data['date']} · 상세 설계 중 · 제품 구현 보류 · 개인 배정/공수 산정 보류", '',
    '**반납 화면의 새 규칙은 반영됐지만, 결제·반납 API/BLE/저장 변경안은 아직 기준 명세에 병합되지 않았다.** '
    '이 문서는 그 차이와 전체 제품의 남은 결정을 한곳에서 추적한다. 명세 수나 예제 검증 수는 개발 진척률이 아니다.', '',
    '[기준 데이터](design-integration-register.json) · [전체 작업](../work-breakdown-plan.md) · '
    '[기존 결정 카드](decisions.md) · [기술 선택](technology-selection.md)', '',
    '## 1. 유지하는 범위와 현재 상태', '',
] + ['- ' + c for c in data['confirmedConstraints']] + ['',
    f"현재 작업은 {len(wbs['epics'])}개 작업군·{len(tasks)}개 패키지·"
    f"{sum(len(t['implementationSteps']) for t in tasks.values())}개 세부 작업이다. "
    f"{len(decisions)}개 결정, {len(tech_ids)}개 기술 선택, {len(validation['experiments'])}개 향후 검증 카드를 유지한다. "
    '이번 통합 정리로 작업이나 기능을 추가하지 않았다.', '',
    '기준 설계 카탈로그: ' + ', '.join(f'{n}개 {name}' for name, n in counts) + '. '
    'DB 참조 DDL과 보안·결제·반납의 논리 저장 자원은 서로 다른 수준이다. '
    '후보 자원 수를 기존 물리 테이블 수에 더해 신규 테이블로 확정하지 않는다.', '',
]
lines += table(['설계 계층', '기준 파일', '해석'], [
    (l['label'], ' · '.join(link(p) for p in l['sourceFiles']), l['meaning']) for l in data['currentLayers']
])
for design in data.get('relatedDesigns', []):
    lines += [f"후속 상세 설계: {link(design['document'])} — {design['note']}", '']
lines += ['## 2. 먼저 해소할 명세 차이', '']
for d in data['differences']:
    lines += [f"### {d['id']} · {d['title']}", '',
              f"- 기준 명세: {d['baseline']}", f"- 후보: {d['candidate']}",
              f"- 통합 기준: {d['rule']}", f"- 연결 묶음: {', '.join(d['bundleRefs'])}", '']
lines += ['## 3. 설계 반영 묶음과 완료 기준', '',
          'DI 번호는 새 개발 작업이 아니라 기존 작업에 연결할 명세 변경 묶음이다. '
          '선행 관계는 설계 병합 완료에 필요한 결과를 뜻하며, 조사나 초안 작성을 순서대로만 하라는 뜻은 아니다. '
          '완료 기준을 만족해도 제품 실행 검증을 대신하지 않는다.', '']
for b in bundles.values():
    lines += [f"### {b['id']} · {b['title']}", '',
              f"상태: `{b['status']}` · 결정: {', '.join(b['decisionRefs'])} · "
              f"선행 설계 결과: {', '.join(b['finishDependsOn']) or '없음'}", '',
              '원본: ' + ' · '.join(link(p) for p in b['sourceFiles']), '', '반영 대상:', '']
    lines += ['- ' + v for v in b['targetContracts']] + ['', '설계 완료로 확인할 결과:', '']
    lines += ['- ' + v for v in b['completionEvidence']] + ['']
lines += ['## 4. 19개 결정의 성격 구분', '',
          'D 번호는 기존 결정 ID다. 모든 항목이 열려 있다는 이유로 확정된 하위 조건까지 미정으로 되돌리지 않는다. '
          '아래 정책 입력은 앞으로 확인할 선택의 목록이며 이번에 19개 질문을 보냈다는 뜻이 아니다. '
          '기술 설계는 현재 단계에서 진행하고, 실기·서비스 검증은 개발 단계에 수행한다.', '']
lines += table(['결정', '사용자/운영 정책 입력', '현재 진행할 기술 설계', '개발 단계 검증'], [
    (d['id'] + ' ' + d['title'], d['policyInputs'], d['engineeringDesign'], d['implementationVerification'])
    for d in data['decisions']
])
lines += ['D02·D10은 새 사용자 질문 없이 기술 명세를 더 진행할 수 있다. '
          'D03·D09의 복구 정책은 같은 대기 질문을 공유한다. '
          '그 외 정책 입력은 관련 제품을 구체화할 때 제안과 선택 영향을 준비한다. '
          '상세 작업·TECH 연결은 JSON과 기존 결정 카드에 보존했다.', '',
          '## 5. 전체 15개 요구사항의 추적', '',
          '관련 작업 수는 중복을 포함한다. 결정 ID는 해당 요구 작업의 기존 decisionInputs 합집합으로, '
          '모든 결정이 모든 하위 기능을 차단한다는 의미가 아니다. 완료율이나 공수로 사용하지 않는다.', '']
scope = (ROOT / data['scopeSource']).read_text()
section = scope.split('## 2. 사용자 원문 요구사항', 1)[1].split('## 3.', 1)[0]
requirements = [(int(n), title.strip()) for n, title in re.findall(r'^(\d+)\. (.+)$', section, re.M)]
assert [n for n, _ in requirements] == list(range(1, 16))
rows = []
for number, title in requirements:
    related = [t for t in tasks.values() if number in t['requirements']]
    refs = sorted({d for t in related for d in t['decisionInputs']})
    rows.append((number, title, len(related), ', '.join(refs)))
lines += table(['원문 번호', '요구사항', '관련 작업 수', '결정 연결 맥락'], rows)
lines += ['## 6. 다음 설계 순서', '']
for item in data['designQueue']:
    lines += [f"- **{item['id']} · {item['focus']}** ({', '.join(item['decisionRefs'])}): "
              f"{item['output']}. {item['note']}."]
    if item.get('status'):
        lines += [f"  현재 상태: `{item['status']}`" +
                  (f" · {link(item['document'])}" if item.get('document') else '')]
lines += ['', '**이미 질문한 복구 정책은 답변 대기 중이다.** 신규 여행 지갑에 본인 전용 암호화 복구 백업을 추가할지, '
          '복구 수단이 없으면 초기화를 보류할지는 선택되지 않았다. '
          '이 문서에서는 질문을 반복하거나 일반적인 진행 요청을 동의로 해석하지 않는다. '
          '답변 전에도 두 지갑의 사용·권한·복구 비교 설계를 진행할 수 있다. '
          '필수 복구 조건이 충족되지 않은 상태에서 새 파괴적 초기화 허가를 발급하는 것으로 설계하지 않는다.', '',
          '## 7. 이 문서의 검증 범위', '',
          '렌더러는 원본 파일, WBS의 19개 결정·작업·기술 참조, 15개 요구 추적, 설계 묶음 의존 관계, '
          '미답변 복구 정책과 구현 보류 상태, Markdown 동기화를 확인한다. '
          '프로토콜 선택이나 보안·성능·법적 적합성·실제 기기 동작을 검증한 결과는 아니다.', '']
lines += ['- ' + n for n in data['nonGoalsThisTurn']] + ['']
output = '\n'.join(lines)
target = HERE / 'design-integration-register.md'
if '--check' in sys.argv:
    assert target.read_text() == output, 'Design register view is stale; run renderer'
else:
    target.write_text(output)
print(f'Design register OK: {len(decisions)} decisions, 15 requirements, {len(bundles)} integration bundles, '
      f"{len(data['differences'])} differences; design only")
