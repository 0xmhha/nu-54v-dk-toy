"""Finite capture boundary and completeness examples, not firmware/BLE/file-system tests."""
import json
from pathlib import Path

R = Path(__file__).resolve().parents[3]
O = Path(__file__).resolve().parent
source = 'content/specifications/recording-travel-ai-design.json'
d = json.loads((R / source).read_text())
predicates = {p['id']: p for p in d['recordingPredicates']}
stages = {r['id']: r for r in d['recordingStages']}
assert stages['AR-02']['predicateRefs'] == ['REC-CHUNK']
assert stages['AR-03']['predicateRefs'] == ['REC-COMPLETE']
assert d['recordingCompletionContract']['uploadCannotUpgradeCapture']
cases = []


def check(name, actual, expected, meaning):
    assert actual == expected, (name, actual, expected)
    cases.append(dict(id=name, meaning=meaning, observed=actual, expected=expected,
                      status='passed_design_example'))


def allowed(predicate, **overrides):
    facts = {key: True for key in predicates[predicate]['allOf']}
    facts.update(overrides)
    return all(facts[key] is True for key in predicates[predicate]['allOf'])


original = {key: key + ':A' for key in d['recordingCaptureContract']['immutableBinding']}


def matching(binding):
    return all(binding.get(key) == original[key] for key in original)


check('LG14-original-binding', matching(original), True, '원 녹음의 소유자·대여·기기·세대 일치')
for key in original:
    changed = {**original, key: key + ':B'}
    check('LG14-other-' + key, allowed('REC-CHUNK', capture_binding_matches=matching(changed)), False,
          '한 binding 성분만 달라도 다른 녹음으로 수용하지 않음')
for key in predicates['REC-CHUNK']['allOf']:
    check('LG14-deny-' + key, allowed('REC-CHUNK', **{key: False}), False,
          '시작 시 인증과 별개로 durable 수신 commit 시 각 조건 검사')

# Serial ordering examples for the specified native receive gate, not real concurrent IO.
state = dict(generation=1, owner_namespace='A', lease=True, durable={}, acknowledgements=[])


def commit(frame, generation, namespace):
    consistent = frame['sequence'] not in state['durable'] or state['durable'][frame['sequence']] == frame
    permit = allowed('REC-CHUNK', receive_lease_current=state['lease'],
                     capture_generation_matches=generation == state['generation'],
                     local_owner_namespace_matches=namespace == state['owner_namespace'],
                     frame_identity_consistent=consistent)
    if not permit:
        return False
    state['durable'][frame['sequence']] = dict(frame)
    state['acknowledgements'].append((frame['sequence'], frame['digest']))
    return True


frame = dict(sequence=0, offset=0, count=100, digest='original')
check('LG14-first-commit', commit(frame, 1, 'A'), True, '원 namespace에 저장 후 해당 digest ACK')
check('LG14-identical-retransmit', (commit(frame, 1, 'A'), len(state['durable'])), (True, 1),
      '동일 frame 재전송은 저장된 한 건만 유지')
check('LG14-conflicting-payload', commit({**frame, 'digest': 'different'}, 1, 'A'), False,
      '같은 sequence의 다른 payload는 기존 파일을 덮지 않음')
state.update(generation=2, lease=False)
ack_count = len(state['acknowledgements'])
check('LG14-switch-before-delayed-commit', commit(dict(sequence=1, offset=100, count=100, digest='late'), 1, 'A'),
      False, '차단 이후 old worker가 원 namespace에도 새 수신 commit 불가')
check('LG14-switch-no-late-ack', len(state['acknowledgements']), ack_count, '저장하지 않은 늦은 청크의 durable ACK 없음')
state['lease'] = True
check('LG14-no-reroute-to-new-account', commit(dict(sequence=1, offset=100, count=100, digest='late'), 2, 'B'),
      False, '새 계정 화면으로 전환해도 기존 녹음 owner는 A')
check('LG14-prior-file-preserved', (state['owner_namespace'], len(state['durable'])), ('A', 1),
      '계정 전환을 원 파일 삭제나 소유권 이전으로 해석하지 않음')
assert 'currentReadAuthority' in d['recordingCompletionContract']['separateAxes']
assert '로컬 unlock' in next(r['rule'] for r in d['recordingBoundaryRules'] if r['id'] == 'RB-04')

# Frame sequence and sample coverage simulation. Each tuple is (sequence, start, exclusive end).
def full_coverage(frames, end_sample, end_sequence):
    unique = sorted(set(frames))
    if not unique or [f[0] for f in unique] != list(range(end_sequence + 1)):
        return False
    cursor = 0
    for sequence, start, end in unique:
        if start != cursor or end <= start or end > end_sample:
            return False
        cursor = end
    return cursor == end_sample


frames = [(0, 0, 100), (1, 100, 200), (2, 200, 300)]
check('LG15-complete-capture', allowed('REC-COMPLETE', exact_durable_interval_coverage=full_coverage(frames, 300, 2)),
      True, '인증 종료 범위와 durable frame 구간이 정확히 일치')
for name, value in [('tail-missing', frames[:2]), ('middle-missing', [frames[0], frames[2]]),
                    ('overlap', [(0, 0, 120), (1, 100, 200), (2, 200, 300)]),
                    ('extra-tail', frames + [(3, 300, 400)]),
                    ('wrong-sequence', [(0, 0, 100), (2, 100, 200), (3, 200, 300)])]:
    check('LG15-' + name, full_coverage(value, 300, 2), False, '재생 가능 여부만으로 전체 범위 수신을 추정하지 않음')
check('LG15-out-of-order-arrival', full_coverage([frames[2], frames[0], frames[1]], 300, 2), True,
      '동일 binding 아래 순서만 바뀐 수신은 정렬 후 구간 대조')
check('LG15-identical-duplicate', full_coverage(frames + [frames[1]], 300, 2), True,
      '동일 frame 재전송을 두 번 수신한 길이로 계산하지 않음')
for key in predicates['REC-COMPLETE']['allOf']:
    check('LG15-deny-' + key, allowed('REC-COMPLETE', **{key: False}), False,
          '종료 표식·binding·구간·파일·revision 중 어느 하나라도 불충족이면 complete 금지')
check('LG15-playable-truncation', allowed('REC-COMPLETE', exact_durable_interval_coverage=False, decode_verified=True),
      False, '정상 decode된 후미 유실 파일도 partial')
capture = 'partial'
upload = 'complete'
check('LG15-upload-does-not-upgrade', (capture, upload), ('partial', 'complete'),
      '부분 녹음 파일 전체가 업로드되어도 capture의 partial 표시 유지')
old = json.loads((O / 'before-round5' / source).read_text())
for key in ['recordingCaptureContract', 'recordingCompletionContract']:
    check('before-' + key, key in old, False, '수정 전 상세 경계 계약 누락을 보존한 원본에서 확인')
assert all(x['status'] == 'open' for x in json.loads((R / 'content/planning/work-breakdown.json').read_text())['decisions'])
print(json.dumps(dict(scope='finite_design_examples_only', casesPassed=len(cases), cases=cases,
                     runtimeVerified=False, implementationReady=False), ensure_ascii=False))
