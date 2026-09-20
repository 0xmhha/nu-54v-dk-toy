"""Finite terminal handoff design examples; no kiosk, server, or device execution."""
import json
from pathlib import Path

R = Path(__file__).resolve().parents[3]
O = Path(__file__).resolve().parent
source = 'content/specifications/kiosk-commerce-journey-design.json'
read = lambda p: json.loads((R / p).read_text())
d = read(source)
overlay = read('content/specifications/preimplementation-contract-overlay.json')
predicates = {r['id']: r for r in d['terminalPredicates']}
transitions = {r['id']: r for r in d['terminalHandoffTransitions']}
routes = {r['id']: r for r in overlay['routeContracts']}
cases = []


def check(name, actual, expected, meaning):
    assert actual == expected, (name, actual, expected)
    cases.append(dict(id=name, meaning=meaning, observed=actual, expected=expected,
                      status='passed_design_example'))


def allowed(predicate, **overrides):
    facts = {key: True for key in predicates[predicate]['allOf']}
    facts.update(overrides)
    return all(facts[key] is True for key in predicates[predicate]['allOf'])


for predicate in ['TERM-CLEAR', 'TERM-NEXT', 'TERM-RENDER']:
    check(predicate + '-normal', allowed(predicate), True, '각 단계의 명시 조건을 모두 충족')
    for key in predicates[predicate]['allOf']:
        check(predicate + '-deny-' + key, allowed(predicate, **{key: False}), False,
              '원 작업·현재 세대·권한·revision 조건 중 하나라도 없으면 거절')
assert transitions['KT-03']['predicateRefs'] == ['TERM-CLEAR']
assert transitions['KT-04']['predicateRefs'] == ['TERM-NEXT']
assert d['terminalResultIsolation']['predicateRef'] == 'TERM-RENDER'
assert d['terminalClearanceContract']['clearanceConsumesOnce']
assert d['terminalClearanceContract']['receiptDoesNotAuthorizePayment']
assert d['terminalClearanceContract']['routes'] == ['OC-04', 'OC-23', 'OC-24']

# Serial finite example of one terminal head and one clearance receipt.
state = dict(phase='active', revision=10, boot='boot-A', receipt=None, consumed=False, new_sessions=0)


def step(id):
    row = transitions[id]
    assert state['phase'] in row['fromState'].split('|')
    state['phase'] = row['toState']


step('KT-01')
step('KT-02')
check('LG16-clearance-pending-not-ready', allowed('TERM-NEXT', clearance_current=state['receipt'] is not None),
      False, '서버 종료 수락만으로 다음 고객 진입 불가')
step('KT-03')
state.update(receipt='receipt-A', revision=11)
captured_revision = state['revision']


def admit(revision):
    permit = allowed('TERM-NEXT', clearance_current=state['phase'] == 'cleared',
                     clearance_unconsumed=not state['consumed'],
                     expected_revisions_match=revision == state['revision'])
    if not permit:
        return False
    step('KT-04')
    state.update(consumed=True, revision=state['revision'] + 1, new_sessions=state['new_sessions'] + 1)
    return True


check('LG16-first-admission', admit(captured_revision), True, '현재 clearance 소비와 단일 새 session 발급')
check('LG16-parallel-second-admission', admit(captured_revision), False, '같은 revision으로 경쟁한 두 번째 admission 거절')
check('LG16-one-successor', state['new_sessions'], 1, '하나의 clearance에서 새 session 하나만 생성')
check('LG16-old-cleared-receipt', allowed('TERM-NEXT', clearance_current=False, clearance_unconsumed=False), False,
      '과거 receipt가 남아 있어도 현재 재사용 허가로 간주하지 않음')
state.update(phase='cleared', consumed=False)
step('KT-05')
state.update(boot='boot-B', revision=14)
check('LG16-previous-boot-ack', allowed('TERM-CLEAR', current_boot_matches='boot-A' == state['boot']), False,
      '재시작 전 정리 ACK가 새 boot의 정리 완료가 될 수 없음')
step('KT-06')
check('LG16-held-resume', state['phase'], 'clearance_pending', '원 종료 작업의 새 challenge로 정리를 다시 확인')
check('LG16-superseded-challenge', allowed('TERM-CLEAR', clearance_challenge_current=False), False,
      '같은 endRequest라도 이전 challenge의 ACK 거절')
check('LG16-ack-replay-current-head-changed', allowed('TERM-CLEAR', expected_revisions_match=False), False,
      'ACK 재전송의 과거 성공은 변경된 현재 head를 덮지 않음')
check('LG16-authority-revoked-after-clear', allowed('TERM-NEXT', current_store_terminal_authority=False), False,
      '정리 이후 terminal 권한 철회가 발생하면 새 고객 발급 차단')

server_order = dict(id='order-A', status='paid', revision=22)
render = allowed('TERM-RENDER', current_local_customer_context=False, view_generation_matches=False)
check('LG16-late-paid-new-customer', (server_order['status'], render), ('paid', False),
      '원 거래 대사는 유지하되 다음 고객 화면에 결과를 적용하지 않음')
check('LG16-old-poll-current-customer', allowed('TERM-RENDER', response_revision_not_older=False), False,
      '같은 고객이라도 오래된 poll이 새 결과를 덮지 않음')
check('LG16-reorg-higher-revision', allowed('TERM-RENDER', response_revision_not_older=True), True,
      '같은 ancestry의 최신 reorg 상태는 이전 paid보다 낮은 단계여도 수용 가능')
assert '종료된 terminal/customer credential은 결과 조회로 부활하지 않는다' in d['terminalResultIsolation']['readAfterEnd']
assert 'API104~106' in d['terminalResultIsolation']['claimSeparation']
assert '조회는 정리 완료·새 세션·거래를 생성하지 않는다' in routes['OC-24']['recovery']
assert routes['OC-23']['existingApiRefs'] == routes['OC-24']['existingApiRefs'] == []
assert 'read proof 미발급·상실이면 임의 재발급 금지' in routes['OC-24']['recovery']
check('LG16-claim-profile-unselected', next(r for r in d['policyInputs'] if r['id'] == 'KP-05')['selection'], None,
      '조회 증명 전달 profile을 선택 완료로 간주하지 않음')
old = json.loads((O / 'before-round6' / source).read_text())
check('before-LG16-no-handoff-table', 'terminalHandoffTransitions' in old, False,
      '수정 전 원칙과 ACK 필요 문구는 있었지만 상세 전이표가 없었음을 확인')
assert len(overlay['routeContracts']) == len(routes) and {'OC-23', 'OC-24'} <= routes.keys()
assert all(x['status'] == 'open' for x in read('content/planning/work-breakdown.json')['decisions'])
print(json.dumps(dict(scope='finite_design_examples_only', casesPassed=len(cases), cases=cases,
                     runtimeVerified=False, implementationReady=False), ensure_ascii=False))
