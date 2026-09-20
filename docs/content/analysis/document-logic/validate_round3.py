"""Finite design examples for LG09–11, not executable product or concurrency tests."""
import json
from pathlib import Path

R = Path(__file__).resolve().parents[3]
O = Path(__file__).resolve().parent
read = lambda p: json.loads((R / p).read_text())
base = 'content/specifications/'
paid = read(base + 'credential-paid-resource-design.json')
privacy = read(base + 'recording-travel-ai-design.json')
overlay = read(base + 'preimplementation-contract-overlay.json')
pa = {r['id']: r for r in paid['paymentAttemptTransitions']}
rf = {r['id']: r for r in paid['refundAttemptTransitions']}
cases = []


def check(name, actual, expected, meaning):
    assert actual == expected, (name, actual, expected)
    cases.append(dict(id=name, meaning=meaning, observed=actual, expected=expected,
                      status='passed_design_example'))


def permits(table, transition, state, **facts):
    row = table[transition]
    return state in row['fromState'].split('|') and all(
        facts.get(key) is True for key in row['requires'])


for name, transition, state, facts, expected in [
    ('invalid-unexposed', 'PA-04', 'proof_received', dict(proof_invalid=True, attempt_exposure_absent=True), True),
    ('invalid-exposed', 'PA-04', 'proof_received', dict(proof_invalid=True, attempt_exposure_absent=False), False),
    ('expiry-no-exposure', 'PA-05', 'required', dict(deadline_expired=True, attempt_exposure_absent=True), True),
    ('timeout-exposure-unknown', 'PA-10', 'unknown', dict(deadline_expired=True), False),
    ('failure-confirmed', 'PA-08', 'unknown', dict(failure_final=True, no_partial_payment=True, exposure_resolved=True), True),
    ('partial-payment', 'PA-08', 'unknown', dict(failure_final=True, no_partial_payment=False, exposure_resolved=True), False),
    ('failure-no-exposure-proof', 'PA-08', 'unknown', dict(failure_final=True, no_partial_payment=True), False),
    ('already-paid-cannot-expire', 'PA-05', 'paid', dict(deadline_expired=True, attempt_exposure_absent=True), False),
]:
    check('LG09-' + name, permits(pa, transition, state, **facts), expected,
          '설계의 requires와 원 상태를 통한 지급 종료 판정')
for id, source, target in [('PA-03', 'unknown', 'paid'), ('PA-07', 'unknown', 'settlement_pending'), ('PA-09', 'failed_confirmed', 'unknown')]:
    check('LG09-' + id, (source in pa[id]['fromState'].split('|'), pa[id]['toState']), (True, target),
          '원 attempt 성공·pending·실패근거 reorg의 복구 경로')
assert paid['paymentAttemptContract']['failureDoesNotAuthorizeRetry']
assert paid['paymentAttemptContract']['unknownExposureRetained']

# Arithmetic illustration of the explicit ledgerRule; amounts are one asset in atomic units.
# This does not simulate signing, a database transaction, or real chain finality.
assert paid['paidRefundContract']['unknownKeepsReservation']
assert 'allocatedPaidAtomic - confirmedRefundAtomic - outstandingRefundExposureAtomic' in paid['paidRefundContract']['ledgerRule']
attempts = {'a': {'state': 'reserved', 'amount': 60}}


def available(funding):
    return funding - sum(a['amount'] for a in attempts.values()
                         if a['state'] in {'reserved', 'submitted', 'unknown', 'confirmed'})


def transition(id, key='a'):
    row = rf[id]
    assert attempts[key]['state'] in row['fromState'].split('|')
    attempts[key]['state'] = row['toState']


check('LG10-reservation', available(100), 40, '60 예약 후 잔액 40')
transition('RF-02')
transition('RF-04')
check('LG10-response-loss', available(100), 40, '응답 유실 후 unknown 노출 60 유지')
check('LG10-repeat-query', [available(100) for _ in range(3)], [40, 40, 40], '동일 attempt 조회는 예약을 추가하지 않음')
check('LG10-second-refund-budget', available(100) >= 50, False, '미확정 환불이 있는 동안 잔액보다 큰 추가 환불 금지')
transition('RF-03')
check('LG10-confirmed-single-charge', available(100), 40, '확정 시 예약에서 확정 반환으로 이동하며 한 번만 차감')
transition('RF-04')
check('LG10-refund-reorg', available(100), 40, '환불 reorg도 동일 attempt 예약으로 노출 유지')
check('LG10-partial-not-failed', permits(rf, 'RF-06', 'unknown', failure_final=True,
      no_partial_refund=False, exposure_resolved=True), False, '부분 반환을 확정 실패로 처리해 전액 예약 해제 금지')
check('LG10-cancel-exposed', permits(rf, 'RF-07', 'reserved', attempt_exposure_absent=False),
      False, '서명/전송 노출이 있으면 미노출 취소 불가')
check('LG10-final-failure-evidence', permits(rf, 'RF-06', 'unknown', failure_final=True,
      no_partial_refund=True, exposure_resolved=True), True, '세 근거가 갖춰진 경우에만 실패 종료')
transition('RF-06')
check('LG10-failure-releases-own', available(100), 100, '확정 실패한 원 attempt의 예약 해제')
attempts['b'] = {'state': 'confirmed', 'amount': 80}
transition('RF-08')
check('LG10-failure-proof-reorg-deficit', available(100), -40, '실패 근거 reorg 시 노출 복원; 과거 환불을 지우지 않고 deficit 표시')
check('LG10-source-reorg-deficit', available(50), -90, '원 지급 감소도 음수 한도를 숨기지 않음')
routes = {r['id']: r for r in overlay['routeContracts']}
assert paid['paidRefundContract']['adapterStatus'] == 'unregistered_execution_blocked'
assert paid['paidRefundContract']['routes'] == ['OC-21', 'OC-22']
check('LG10-create-binds-parent-and-ledger', all(k in routes['OC-21']['requestFields'] for k in
      ['originalPaymentEffectId', 'expectedFundingRevision', 'expectedRefundLedgerRevision',
       'merchantSignerRef', 'assetRef', 'destinationProofRef', 'refundPolicyRevision']), True,
      '환불 생성은 원 funding·signer·목적지·정책·현재 원장과 결합')
assert '고객 request owner만으로 판매자 자금 서명 불가' in routes['OC-21']['authority']
assert '현재 request owner' in routes['OC-22']['authority'] and 'operator' in routes['OC-22']['authority']
assert '새서명/환불 예약/외부전송을 생성하지 않음' in routes['OC-22']['atomicBoundary']

pred = next(p for p in privacy['privacyPredicates'] if p['id'] == 'PV-COMPLETE')
assert set(pred['allOf']) == {'all_targets_terminal_with_evidence', 'target_set_revision_matches',
                            'deletion_revision_matches', 'discovery_queue_empty', 'producer_closure_accounted'}
assert privacy['deletionCompletionContract']['casFields'] == ['targetSetRevision', 'deletionRevision']
assert set(privacy['deletionCompletionContract']['lateTargetAtomicWrites']) == {
    'register_target_with_source', 'increment_target_set_revision', 'mark_completion_pending', 'increment_deletion_revision'}


def complete(**overrides):
    facts = {key: True for key in pred['allOf']}
    facts.update(overrides)
    return all(facts[key] is True for key in pred['allOf'])


check('LG11-current-complete', complete(), True, '현재 대상 집합·증거·원천 종료가 모두 확인됨')
for key in pred['allOf']:
    check('LG11-deny-' + key, complete(**{key: False}), False, '완료에 필요한 각 조건이 빠지면 완료 금지')
# Two serial interleavings model the required CAS contract; not real multithreaded storage.
state = dict(targetRevision=3, deletionRevision=8, status='pending', targets={'old'}, receipts=[])


def late_target(identity):
    if identity in state['targets']:
        return
    state['targets'].add(identity)
    state['targetRevision'] += 1
    state['deletionRevision'] += 1
    state['status'] = 'pending'


def finish(read_revisions, terminal):
    allowed = complete(target_set_revision_matches=read_revisions[0] == state['targetRevision'],
                       deletion_revision_matches=read_revisions[1] == state['deletionRevision'],
                       all_targets_terminal_with_evidence=terminal)
    if allowed:
        state['status'] = 'completed'
        state['receipts'].append(read_revisions[0])
    return allowed


late_target('late')
check('LG11-add-before-complete', finish((3, 8), True), False, 'rev4 대상 추가 후 rev3 근거로 완료 처리 불가')
check('LG11-fresh-complete', finish((4, 9), True), True, '추가 대상까지 삭제 후 현재 revision으로 완료')
late_target('later')
check('LG11-complete-before-add', (state['status'], state['receipts']), ('pending', [4]),
      '완료 후 대상 추가는 현재 상태를 pending으로 바꾸고 과거 receipt 보존')
before = (state['targetRevision'], state['deletionRevision'])
late_target('later')
check('LG11-duplicate-artifact', (state['targetRevision'], state['deletionRevision']), before,
      '같은 artifact 재통지는 대상 집합 revision을 중복 증가시키지 않음')

for filename, absent in [('credential-paid-resource-design.json', 'paymentAttemptTransitions'),
                         ('credential-paid-resource-design.json', 'refundAttemptTransitions'),
                         ('recording-travel-ai-design.json', 'deletionCompletionContract')]:
    old = json.loads((O / 'before-round3' / base / filename).read_text())
    check('before-' + absent, absent in old, False, '수정 전 계약 누락을 보존한 원본에서 확인')
assert all(d['status'] == 'open' for d in read('content/planning/work-breakdown.json')['decisions'])
print(json.dumps(dict(scope='finite_design_examples_only', casesPassed=len(cases), cases=cases,
                     runtimeVerified=False, implementationReady=False), ensure_ascii=False))
