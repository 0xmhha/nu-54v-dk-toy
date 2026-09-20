"""Finite itinerary edit/apply design examples, not AI, HTTP, or storage execution."""
import json
from copy import deepcopy
from pathlib import Path

R = Path(__file__).resolve().parents[3]
O = Path(__file__).resolve().parent
source = 'content/specifications/recording-travel-ai-design.json'
read = lambda p: json.loads((R / p).read_text())
d = read(source)
preds = {p['id']: p for p in d['itineraryPredicates']}
routes = {r['id']: r for r in read('content/specifications/preimplementation-contract-overlay.json')['routeContracts']}
cases = []


def check(name, actual, expected, meaning):
    assert actual == expected, (name, actual, expected)
    cases.append(dict(id=name, meaning=meaning, observed=deepcopy(actual), expected=deepcopy(expected),
                      status='passed_design_example'))


def allowed(predicate, **overrides):
    facts = {key: True for key in preds[predicate]['allOf']}
    facts.update(overrides)
    return all(facts[key] is True for key in preds[predicate]['allOf'])


for predicate in ['ITINERARY-APPLY', 'ITINERARY-EDIT']:
    check(predicate + '-normal', allowed(predicate), True, '현재 적용/편집 조건 충족')
    for key in preds[predicate]['allOf']:
        check(predicate + '-deny-' + key, allowed(predicate, **{key: False}), False, '각 조건이 빠지면 현재 코스 변경 거절')

# Content revision and generation-selection revision are deliberately independent.
state = dict(head=5, selected_plan='manual-v5', generation_revision=2, generation=None, candidates={}, receipts={})


def generate(id, expected_head, expected_selection):
    if expected_head != state['head'] or expected_selection != state['generation_revision']:
        return False
    state['generation'] = id
    state['generation_revision'] += 1
    return True


def observe(id, base):
    state['candidates'][id] = dict(base=base, valid=True, applied=False)


def apply(id, request, expected_head, read_authority=True, **flags):
    if request in state['receipts']:
        return state['receipts'][request] if read_authority else 'forbidden'
    candidate = state['candidates'][id]
    permitted = allowed('ITINERARY-APPLY', base_revision_matches=candidate['base'] == state['head'],
                        selected_generation_matches=id == state['generation'],
                        expected_head_revision_matches=expected_head == state['head'],
                        candidate_not_applied_or_discarded=not candidate['applied'], **flags)
    if not permitted:
        return 'conflict_or_held'
    state['head'] += 1
    state['selected_plan'] = id
    candidate['applied'] = True
    state['receipts'][request] = dict(candidate=id, appliedAtRevision=state['head'])
    return state['receipts'][request]


check('LG18-start-generation', generate('gen-A', 5, 2), True, '현재 내용과 선택 세대에서 원 생성 예약')
check('LG18-generation-does-not-edit-content', (state['head'], state['selected_plan'], state['generation_revision']),
      (5, 'manual-v5', 3), '생성 예약이 자기 base revision을 무효화하거나 기존 코스를 덮지 않음')
check('LG18-concurrent-stale-selection', generate('gen-B', 5, 2), False, '별도 선택 CAS로 동시 생성의 무조건 덮어쓰기 방지')
state['head'] = 6  # A manual draft was saved; the previous selected plan is still intact.
observe('gen-A', 5)
check('LG18-late-worker-only-candidate', (state['head'], state['selected_plan']), (6, 'manual-v5'),
      '수동 초안 저장 뒤 늦은 worker는 candidate만 기록')
check('LG18-stale-base-cannot-apply', apply('gen-A', 'apply-A', 6), 'conflict_or_held',
      '현재 expectedRevision을 보내도 원 base가 다르면 자동 적용 불가')
check('LG18-explicit-new-generation', generate('gen-C', 6, 3), True, '사용자가 현재 base로 새 생성 명시 요청')
observe('gen-C', 6)
check('LG18-source-changed-after-generation', apply('gen-C', 'apply-C', 6, source_vector_current_and_complete=False),
      'conflict_or_held', '후보 검증 이후 원천 변경도 적용 시 재검사')
check('LG18-consent-revoked-after-generation', apply('gen-C', 'apply-C', 6, current_privacy_and_consent=False),
      'conflict_or_held', '과거 생성 동의가 현재 적용 권한을 대신하지 않음')
check('LG18-user-apply-current-candidate', apply('gen-C', 'apply-C', 6), dict(candidate='gen-C', appliedAtRevision=7),
      '현재 후보·base·검토·권한 조건을 충족해 한 번 적용')
check('LG18-competing-second-apply', apply('gen-C', 'apply-D', 6), 'conflict_or_held', '경쟁한 두 번째 적용이 새 version을 만들지 않음')
state.update(head=8, selected_plan='manual-v8')
check('LG18-lost-response-retry', apply('gen-C', 'apply-C', 6), dict(candidate='gen-C', appliedAtRevision=7),
      '응답 유실 재시도는 과거 적용 receipt 조회')
check('LG18-retry-preserves-new-edit', (state['head'], state['selected_plan']), (8, 'manual-v8'),
      '과거 성공 응답이 현재 수동 코스를 되돌리지 않음')
check('LG18-retry-current-read-denied', apply('gen-C', 'apply-C', 6, read_authority=False), 'forbidden',
      '멱등 결과 조회도 현재 읽기권이 필요')
check('LG18-superseded-valid-candidate', allowed('ITINERARY-APPLY', selected_generation_matches=False), False,
      'valid 후보라도 더 이상 선택된 generation이 아니면 적용 거절')
check('LG18-unknown-validation', allowed('ITINERARY-APPLY', validation_current_and_valid=False), False,
      '영업시간 등 unknown을 AI 설명만으로 valid 승격하지 않음')
check('LG18-no-explicit-apply', allowed('ITINERARY-APPLY', explicit_user_apply=False), False,
      'worker 생성 성공은 사용자 적용 승인이 아님')
actions = {r['id']: r for r in d['itineraryWriteActions']}
assert actions['IW-03']['predicateRefs'] == ['ITINERARY-EDIT']
assert actions['IW-04']['predicateRefs'] == ['ITINERARY-APPLY']
assert 'current head/selected plan 변경 없음' in actions['IW-02']['effect']
assert '새 job·candidate 적용·과금을 생성하지 않는다' in routes['OC-26']['recovery']
assert 'expectedGenerationSelectionRevision' in routes['OC-25']['requestFields']
assert d['itineraryGenerationContract']['routes'] == ['OC-25', 'OC-26']
assert d['itineraryApplyContract']['candidateModes'] == ['manual_edit', 'apply_candidate']
old = json.loads((O / 'before-round8' / source).read_text())
check('before-LG18-no-generation-contract', 'itineraryGenerationContract' in old, False, '기존 수동 revision 검사는 있었지만 비동기 적용 계약이 없었음')
assert all(x['status'] == 'open' for x in read('content/planning/work-breakdown.json')['decisions'])
print(json.dumps(dict(scope='finite_design_examples_only', casesPassed=len(cases), cases=cases,
                     runtimeVerified=False, implementationReady=False), ensure_ascii=False))
