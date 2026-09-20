"""Finite MPC design examples, not cryptographic, device, or storage execution."""
import json
from pathlib import Path

R = Path(__file__).resolve().parents[3]
O = Path(__file__).resolve().parent
read = lambda p: json.loads((R / p).read_text())
source = 'content/specifications/social-wallet-recovery-design.json'
d = read(source)
overlay = read('content/specifications/preimplementation-contract-overlay.json')
predicates = {r['id']: r for r in d['mpcPredicates']}
rows = d['mpcResumeTransitions']
cases = []


def check(name, actual, expected, meaning):
    assert actual == expected, (name, actual, expected)
    cases.append(dict(id=name, meaning=meaning, observed=actual, expected=expected,
                      status='passed_design_example'))


def allowed(predicate, **overrides):
    facts = {k: True for k in predicates[predicate]['allOf']}
    facts.update(overrides)
    return all(facts[k] is True for k in predicates[predicate]['allOf'])


def resume(phase, commit, original_epoch=4, committed_epoch=5, current_epoch=4, **facts):
    # Input phase is the validated server checkpoint, never a client-provided destination.
    if not allowed('MPC-RESUME', **facts):
        return 'recovery_hold'
    for row in rows:
        if row['durableResumePhase'] != phase or row['commitDisposition'] != commit:
            continue
        target_epoch = committed_epoch if row['epochRule'] == 'exact_committed_epoch' else original_epoch
        if current_epoch != target_epoch:
            return 'recovery_hold'
        return row['toState']
    return 'recovery_hold'


assert len(rows) == 7 and len({r['id'] for r in rows}) == 7
assert all(r['fromState'] == 'recovery_hold' and r['predicateRefs'] == ['MPC-RESUME'] for r in rows)
assert d['holdResumeRule']['directActiveAllowed'] is False
assert d['holdResumeRule']['unknownCommitAction'] == 'remain_recovery_hold'
for row in rows:
    phase = row['durableResumePhase']
    commit = row['commitDisposition']
    epoch = 5 if commit == 'committed' else 4
    check('LG12-' + row['id'] + '-resume', resume(phase, commit, current_epoch=epoch), phase,
          '원 operation의 증명된 단계와 일치하는 epoch로만 재개')
    check('LG12-' + row['id'] + '-unknown-commit', resume(phase, 'unknown', current_epoch=epoch),
          'recovery_hold', 'commit 여부를 모르면 추정하지 않음')
    check('LG12-' + row['id'] + '-opposite-commit',
          resume(phase, 'not_committed' if commit == 'committed' else 'committed', current_epoch=epoch),
          'recovery_hold', '단계와 commit 증거가 모순이면 재개 불가')
    check('LG12-' + row['id'] + '-wrong-epoch', resume(phase, commit, current_epoch=epoch + 1),
          'recovery_hold', '다른 participant epoch의 응답 혼입 차단')
for flag in predicates['MPC-RESUME']['allOf']:
    check('LG12-deny-' + flag, resume('epoch_committed', 'committed', current_epoch=5, **{flag: False}),
          'recovery_hold', '현재 권한·원 작업·checkpoint·제한·material 조건을 각각 검사')
for phase in ['active', 'absent', 'unknown', 'client_selected_phase']:
    check('LG12-no-target-' + phase, resume(phase, 'not_committed'), 'recovery_hold',
          '전이표 밖 단계 또는 active로 바로 이동 금지')

mp = {r['id']: r for r in d['mpcTransitions']}
check('LG13-activation-wired', mp['MP-03']['predicateRefs'], ['MPC-ACTIVATE'], 'MP03이 활성화 공통 guard를 직접 참조')
check('LG13-normal-activation', allowed('MPC-ACTIVATE'), True, 'DKG 완료와 현재 활성화 조건 모두 충족')
for flag in predicates['MPC-ACTIVATE']['allOf']:
    check('LG13-deny-' + flag, allowed('MPC-ACTIVATE', **{flag: False}), False,
          '프로토콜 완료만으로 현재 권한·상태 검사를 생략하지 않음')
contract = d['mpcActivationContract']
assert contract['observationDoesNotAuthorizeActivation']
assert not contract['addressUsableBeforeActivation']
assert contract['currentReadReleaseRequired'] and not contract['readCredentialMayMutate']

# Serial ordering illustrations: evidence persistence is independent of active/address projection.
state = dict(phase='dkg_pending', revision=2, restricted=False, evidence=False, activated=False)
captured_revision = state['revision']
state.update(phase='recovery_hold', revision=3, restricted=True)
state['evidence'] = True  # Verified late completion remains attached to the original operation.
activate = allowed('MPC-ACTIVATE', expected_dkg_phase_current=state['phase'] == 'dkg_pending',
                   operation_revision_matches=captured_revision == state['revision'],
                   security_holds_clear=not state['restricted'])
state['activated'] = activate
check('LG13-restriction-before-completion', (state['evidence'], state['activated']), (True, False),
      '늦은 완료 사실은 보존하되 보류 상태를 활성화로 덮지 않음')
state = dict(activated=True, restricted=True)
check('LG13-restriction-after-activation', state['activated'] and not state['restricted'], False,
      '과거 활성화 성공 이력이 현재 공개 권한을 보장하지 않음')
check('LG13-relogin-without-mutation-proof',
      resume('dkg_pending', 'not_committed', current_mutation_authority=False), 'recovery_hold',
      '재로그인 사실만으로 원 enrollment 재개 권한을 만들지 않음')
authority = {r['action']: r for r in overlay['walletLifecycleAuthority']}
for action in ['create', 'recover', 'resume_or_activate']:
    check('LG13-read-proof-cannot-' + action, authority[action]['readCredentialAllowed'], False,
          '결과 조회 증명과 변경 권한 분리')
check('LG13-get-no-side-effect', authority['read_result']['mutation'], False, '조회가 재개·서명·활성화를 만들지 않음')
check('LG13-scoped-read-retained', authority['read_result']['readCredentialAllowed'], True,
      '허용된 원 operation 결과 조회 경로는 유지')
old = json.loads((O / 'before-round4' / source).read_text())
check('before-LG12-no-resume-table', 'mpcResumeTransitions' in old, False, '기존 원칙만 있던 상태표 공백 재현')
check('before-LG13-no-activation-predicate',
      'predicateRefs' in next(r for r in old['mpcTransitions'] if r['id'] == 'MP-03'), False,
      '수정 전 최초 활성화의 명시 현재성 조건 누락 확인')
assert all(row['status'] == 'open' for row in read('content/planning/work-breakdown.json')['decisions'])
print(json.dumps(dict(scope='finite_design_examples_only', casesPassed=len(cases), cases=cases,
                     runtimeVerified=False, implementationReady=False), ensure_ascii=False))
