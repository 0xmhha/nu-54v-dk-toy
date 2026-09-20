"""Finite benefit writer/identity/correction examples; no real ledger or payout execution."""
import json
from copy import deepcopy
from pathlib import Path

R = Path(__file__).resolve().parents[3]
O = Path(__file__).resolve().parent
source = 'content/specifications/commerce-consumer-repair-design.json'
read = lambda p: json.loads((R / p).read_text())
d = read(source)
contract = d['benefitEffectOwnership']
predicate = d['benefitHandoffPredicates'][0]
cases = []


def check(name, actual, expected, meaning):
    assert actual == expected, (name, actual, expected)
    cases.append(dict(id=name, meaning=meaning, observed=deepcopy(actual), expected=deepcopy(expected),
                      status='passed_design_example'))


def allowed(**overrides):
    facts = {key: True for key in predicate['allOf']}
    facts.update(overrides)
    return all(facts[key] is True for key in predicate['allOf'])


assert contract['effectWriter'] == 'CP-BENEFITS'
assert contract['assessmentWriter'] == 'CP-TRAVEL'
assert contract['transportAuthority'] is False
assert contract['scope'] == 'internal_nontransferable_stamp_or_benefit_ledger_only'
check('LG17-normal', allowed(), True, '현재 원천과 원 entitlement 조건 모두 충족')
for key in predicate['allOf']:
    check('LG17-deny-' + key, allowed(**{key: False}), False, '각 적용 조건 누락 시 효과 갱신 거절')


def effect_key(record):
    return tuple(record[key] for key in contract['effectKeyFields'])


purchase = dict(environmentId='test', benefitProgramId='cafe-stamp', entitlementSourceKind='purchase_eligibility',
                entitlementSourceId='elig-1')
challenge = dict(environmentId='test', benefitProgramId='travel-challenge', entitlementSourceKind='challenge_entitlement',
                 entitlementSourceId='challenge-v1/participant-1/slot-1/occurrence-1')
for excluded in contract['excludedFromEffectKey']:
    check('LG17-key-excludes-' + excluded, effect_key({**purchase, excluded: 'changed'}), effect_key(purchase),
          '전달 이벤트·재구축·연결 계정·mutable revision 변경으로 새 효과를 만들지 않음')
check('LG17-legitimate-programs-separated', effect_key(purchase) != effect_key(challenge), True,
      '같은 지급에서 유래해도 선정된 구매 스탬프와 챌린지 프로그램은 별도 혜택')
check('LG17-typed-source-collision', effect_key({**purchase, 'entitlementSourceKind': 'challenge_entitlement'}) != effect_key(purchase),
      True, '문자열 sourceId가 같더라도 purchase/challenge namespace 혼합 금지')
check('LG17-cross-environment', effect_key({**purchase, 'environmentId': 'other'}) != effect_key(purchase), True,
      '다른 환경의 효과 원장을 혼합하지 않음')
check('LG17-registered-source-variants', {x['kind'] for x in contract['sourceVariants']},
      {'purchase_eligibility', 'challenge_entitlement'}, 'challenge에 가짜 purchase eligibility를 요구하지 않음')
# Make results JSON-serializable after checking the exact set relation.
cases[-1]['observed'] = sorted(cases[-1]['observed'])
cases[-1]['expected'] = sorted(cases[-1]['expected'])

# Serial target-delta illustration with immutable consumed quantity and current effect revision.
ledger = dict(target=1, consumed=1, revision=3, corrections=[], held=False)


def apply(writer, target, expected_revision, source_current=True, policy_current=True):
    if target is None or not allowed(writer_is_benefits=writer == contract['effectWriter'],
                                    source_cut_complete_and_current=source_current,
                                    original_rule_binding_current=policy_current,
                                    expected_effect_revision_matches=expected_revision == ledger['revision']):
        ledger['held'] = True
        return False
    delta = target - ledger['target']
    if delta:
        ledger['corrections'].append(delta)
    ledger.update(target=target, revision=ledger['revision'] + 1, held=False)
    return True


check('LG17-travel-cannot-write-ledger', apply('CP-TRAVEL', 0, 3), False,
      '여행 assessment writer가 grant/correction을 직접 적용하지 않음')
check('LG17-single-writer-refund-correction', apply('CP-BENEFITS', 0, 3), True, '현재 선정 규칙이 목표 0인 경우 단일 보정')
check('LG17-consumed-history-deficit', (ledger['target'] - ledger['consumed'], ledger['consumed']), (-1, 1),
      '사용 이력을 지우지 않고 deficit를 표시; 자산 인출 없음')
check('LG17-second-consumer-replay', apply('CP-BENEFITS', 0, 3), False, '동일 이전 revision 재적용은 거절')
check('LG17-current-duplicate-zero-delta', apply('CP-BENEFITS', 0, 4), True, '현 상태 재조회 후 같은 목표는 추가 보정 없음')
check('LG17-only-one-negative-delta', ledger['corrections'], [-1], '중복/재전달로 두 번 차감하지 않음')
check('LG17-source-reconfirmed', apply('CP-BENEFITS', 1, 5), True, '재확정 목표를 원 효과에 복구 보정')
check('LG17-no-new-grant-on-rebuild', (ledger['corrections'], ledger['target'] - ledger['consumed']), ([-1, 1], 0),
      '복구 후에도 기존 consume 보존, 새 grant로 이중 적립하지 않음')
saved = (ledger['target'], ledger['consumed'], list(ledger['corrections']))
check('LG17-unknown-not-zero', apply('CP-BENEFITS', None, 6), False, '알 수 없는 목표는 보류')
check('LG17-unknown-preserves-ledger', (ledger['target'], ledger['consumed'], ledger['corrections']), saved,
      'unknown을 0으로 처리해 과거 적립을 차감하지 않음')
check('LG17-late-assessment-source-stale', apply('CP-BENEFITS', 2, 6, source_current=False), False,
      '늦은 평가 메시지도 현재 원천이 바뀌면 적용 차단')
check('LG17-marketing-rule-change', apply('CP-BENEFITS', 2, 6, policy_current=False), False,
      '새 마케팅 규칙으로 기존 entitlement를 자동 재발급하지 않음')
assert '낮은 revision' in d['benefitAssessmentContract']['ordering']
assert '동일 revision의 다른 digest는 quarantine' in d['benefitAssessmentContract']['ordering']
assert d['benefitAssessmentContract']['catalogStatus'] == 'logical_outbox_inbox_adapter_not_registered_event'
boundary = read('content/specifications/recording-travel-ai-design.json')['benefitEffectBoundary']
check('LG17-completion-not-reward', boundary['completionIsReward'], False, '챌린지 완료와 원장 효과 적용 분리')
check('LG17-visit-review-not-deleted', boundary['originalVisitOrReviewDeletionAllowed'], False,
      '보상 보정이 독립 방문/후기 삭제 권한을 부여하지 않음')
old = json.loads((O / 'before-round7' / source).read_text())
check('before-LG17-writer-contract-absent', 'benefitEffectOwnership' in old, False,
      '수정 전 consumer별 규칙과 구분해 writer/tagged identity 계약 공백 확인')
assert all(x['status'] == 'open' for x in read('content/planning/work-breakdown.json')['decisions'])
print(json.dumps(dict(scope='finite_design_examples_only', casesPassed=len(cases), cases=cases,
                     runtimeVerified=False, implementationReady=False), ensure_ascii=False))
