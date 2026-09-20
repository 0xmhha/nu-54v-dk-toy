"""Validate and render a design adoption/compatibility plan. No runtime acceptance."""
import hashlib
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
load = lambda f: json.loads((ROOT / f).read_text())
p = load('approval-adoption-plan.json')
assert p['implementation'] == 'deferred_by_user'
assert p['canonicalMerged'] is p['archiveCreated'] is p['runtimeVerified'] is False
assert p['policySelectionUnchanged']
for path, digest in p['baselineFiles'].items():
    assert hashlib.sha256((ROOT / path).read_bytes()).hexdigest() == digest, path
comp = {x['id']: x for x in load('compatibility-matrix.json')['rows']}
vals = {x['id'] for x in json.loads((ROOT.parent / 'planning/technology-validation-plan.json').read_text())['experiments']}
stages = {x['id']: x for x in p['adoptionStages']}
profiles = {x['id']: x for x in p['profiles']}
assert len(stages) == len(p['adoptionStages']) == 6
assert len(profiles) == len(p['profiles']) == 11
assert len({x['id'] for x in p['acceptanceRows']}) == len(p['acceptanceRows']) == 24
assert {r for x in p['acceptanceRows'] for r in x['compatibilityRefs']} == set(comp)
active, done = set(), set()


def visit(key):
    assert key not in active, key
    if key in done:
        return
    active.add(key)
    stage = stages[key]
    assert stage['status'] == 'not_applied' and not stage['evidenceRefs']
    for path in stage['targetFiles']:
        assert (ROOT / path).is_file(), path
    for dep in stage['dependsOn']:
        assert dep in stages
        visit(dep)
    active.remove(key)
    done.add(key)


for key in stages:
    visit(key)
for row in p['profiles']:
    assert set(row['compatibilityRefs']) <= comp.keys()
    assert row['selectedRuntimeProfile'] is None and not row['runtimeEvidenceRefs']
for row in p['acceptanceRows']:
    assert set(row['profileRefs']) <= profiles.keys()
    assert set(row['compatibilityRefs']) <= comp.keys()
    assert set(row['validationRefs']) <= vals
    assert set(row['requiredPins']) == row['pinValues'].keys()
    assert len(row['requiredPins']) == len(set(row['requiredPins']))
    assert all(value is None for value in row['pinValues'].values())
    assert row['status'] == 'unverified' and not row['runtimeEvidenceRefs']
    assert {v for c in row['compatibilityRefs'] for v in comp[c]['requiredPins']} <= set(row['requiredPins'])
for x in p['validatorTransition']:
    assert (ROOT / x['validator']).is_file() and (ROOT / x['candidate']).is_file()
    assert x['status'] == 'planned_not_moved'
assert all(x['status'] == 'not_evaluated' for x in p['gates'])
apis = load('api-catalog.json')['operations']
assert len(apis) == p['catalogRegistration']['beforeApiCount'] == 107
assert p['catalogRegistration']['afterApiCountIfOnlyTheseRegistered'] == len(apis) + 3
assert p['catalogRegistration']['assignedApiIds'] is None
assert len(load('ble-catalog.json')['commands']) == 34
assert len(load('screen-flows.json')['screens']) == 37
wbs = json.loads((ROOT.parent / 'planning/work-breakdown.json').read_text())
assert len(wbs['tasks']) == p['designCounters']['tasks'] == 104
assert wbs['subtaskCount'] == p['designCounters']['subtasks'] == 320
assert len(wbs['decisions']) == p['designCounters']['decisions'] == 19
assert {r for t in wbs['tasks'] for r in t['requirements']} == set(range(1, 16))
pending = next(x for x in load('return-recovery-contract.json')['openDecisions'] if x['id'] == 'RR-DEC-01')
assert pending['selection'] is p['pendingUserDecision']['selection'] is None
assert pending['status'] == p['pendingUserDecision']['status'] == 'awaiting_user_preference'


def cell(value):
    if isinstance(value, list):
        value = ', '.join(value)
    return str(value).replace('|', '\\|').replace('\n', ' ')


def table(headers, rows):
    return '\n'.join(['| ' + ' | '.join(headers) + ' |', '| ' + ' | '.join(['---'] * len(headers)) + ' |'] + ['| ' + ' | '.join(cell(c) for c in row) + ' |' for row in rows]) + '\n\n'


f = ROOT / p['document']
text = f.read_text().split('<!-- GENERATED_ADOPTION -->')[0] + '<!-- GENERATED_ADOPTION -->\n\n'
text += '## 8. 기준 반영 순서 — 모두 미적용\n\n'
text += table(['단계', '선행', '반영 내용', '완료 확인'], [[x['id'] + ' ' + x['title'], x['dependsOn'] or '없음', x['action'], x['completionEvidence']] for x in p['adoptionStages']])
text += '## 9. 프로파일 영역 — 실행 profile 미선정\n\n'
text += table(['영역', '초안 계약 이름', '기존 호환 참조', '정의·고정할 내용'], [[x['id'] + ' ' + x['title'], x['draftContractLabel'] or '미선정', x['compatibilityRefs'], x['requiredDefinition']] for x in p['profiles']])
text += '## 10. 승인 수용표 — 24개 모두 실행 미검증\n\n'
text += '아래 조건을 문서화했을 뿐 실제 조합의 호환을 판정하지 않았다. 각 행의 null pins와 빈 실행 증거 목록은 원본 JSON에 있다.\n\n'
for x in p['acceptanceRows']:
    text += f"### {x['id']} · {x['title']}\n\n"
    text += f"- 연결: {', '.join(x['profileRefs'])} / {', '.join(x['compatibilityRefs'])} / {', '.join(x['validationRefs'])}\n"
    text += f"- 수용에 필요한 것: {x['acceptanceCondition']}\n- 거절할 것: {x['rejectionCondition']}\n- 조회·복구: {x['recoveryBehavior']}\n- 현재: unverified, 실제 pin 미기입, 실행 증거 없음.\n\n"
text += '## 11. 판정 단계 — 서로 대체하지 않음\n\n'
text += table(['판정', '조건', '의미'], [[x['id'] + ' ' + x['title'], x['condition'], x['meaning']] for x in p['gates']])
if '--check' in sys.argv:
    assert f.read_text() == text, 'Generated adoption view differs'
else:
    f.write_text(text)
print(json.dumps({'adoptionStages': len(stages), 'profileAreas': len(profiles),
                  'acceptanceRows': len(p['acceptanceRows']), 'existingCompatibilityRowsReferenced': len(comp),
                  'adoptionGraph': 'acyclic', 'baselineHashes': 'matched',
                  'archiveCreated': False, 'canonicalMerged': False, 'runtimeVerified': False,
                  'mode': 'check' if '--check' in sys.argv else 'render'}))
