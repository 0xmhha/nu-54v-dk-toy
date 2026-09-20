"""Render the logical evidence contract; does not create keys or wire schemas."""
import argparse
import hashlib
import json
from pathlib import Path

P = Path(__file__).resolve().parent
R = P.parents[1]
parser = argparse.ArgumentParser()
parser.add_argument('--check', action='store_true')
args = parser.parse_args()
d = json.loads((P/'management-trust-evidence.json').read_text())
parent = json.loads((P/'management-trust-lifecycle.json').read_text())
assert d['implementation'] == 'deferred_by_user'
assert not any(d[k] for k in ['canonicalMerged', 'runtimeVerified', 'sqlApplied', 'activeTrustChanges'])
for f, h in d['sourceHashes'].items():
    assert hashlib.sha256((R/f).read_bytes()).hexdigest() == h, f
ids = {r['id'] for r in d['records']}
assert len(ids) == len(d['records']) == 7
parent_ids = {r['id'] for r in parent['records']}
policies = {r['id'] for r in parent['policyInputs']}
commands = {r['id'] for r in parent['commands']}
for r in d['records']:
    assert set(r['recordRefs']) <= parent_ids
    assert r['fields'] and all(len(f) == 2 for f in r['fields'])
for r in d['rules']:
    assert set(r['recordRefs']) <= ids
for r in d['scenarios']:
    assert set(r['commandRefs']) <= commands
for r in d['retention'] + d['unresolved']:
    assert r['policyRef'] in policies
assert all(r['selection'] is None for r in d['unresolved'])
lines = ['# 관리 승인·복구 증거의 필드와 수명', '', d['purpose'], '', '## 설계 경계', '']
lines += ['- '+s for s in d['boundaries']]
lines += ['', '## 증거가 연결되는 흐름', '', '```mermaid', 'flowchart LR',
          ' A["정확한 변경 본문"] --> B["목적·수신자에 결합한 승인"]',
          ' B --> C["현재 권한·독립성·시각 확인"]',
          ' C --> D["원 요청 예약·commit 재검사"]',
          ' D --> E["단회 효과·소비·결과"]',
          ' D --> U["불명확: 원 요청 조정"]',
          ' U --> E',
          ' E --> Q["현재 읽기 권한으로 원 결과 조회"]',
          ' E --> T["본문 축약·재사용 방지 보존"]', '```', '',
          '도표는 논리 흐름이다. 외부 저장소를 포함한 분산 원자성이나 자동 복구가 구현됐다는 의미가 아니다.', '',
          '## 필수 논리 필드', '',
          '조건부 필드는 적용되는 연산에서 필수다. 선택하지 않은 suite·wire 인코딩·시간/보존 수치를 임의 기본값으로 채우지 않는다.']
for r in d['records']:
    lines += ['', f"### {r['id']} · {r['name']}", '',
              '기존 기록: '+', '.join(r['recordRefs']), '',
              '| 필드 묶음 | 의미·조건 |', '|---|---|']
    lines += [f'| {name} | {meaning} |' for name, meaning in r['fields']]
    lines += ['', r['rule']]
lines += ['', '## 유효성·소비 규칙', '']
for r in d['rules']:
    lines += [f"- **{r['id']} · {r['name']}**: {r['rule']}"]
lines += ['', '## 주요 시간 순서', '', '| 사례 | 흐름과 결과 | 연산 |', '|---|---|---|']
for r in d['scenarios']:
    lines += [f"| {r['id']} · {r['name']} | {r['steps']} | {', '.join(r['commandRefs'])} |"]
lines += ['', '## 보존과 삭제', '', '| 기록 | 규칙 | 기존 정책 입력 |', '|---|---|---|']
for r in d['retention']:
    lines += [f"| {r['id']} · {r['name']} | {r['rule']} | {r['policyRef']} |"]
lines += ['', '## 미선택 입력', '']
lines += [f"- {r['policyRef']}: {r['needed']} — 미선택." for r in d['unresolved']]
lines += ['', '## 검증 범위', '',
          '유한 설계 모델은 만료 경계·현재 권한 변경·독립 승인·중복 소비·응답 유실·보관 종료·복원 불일치를 검사한다. 암호 검증과 독립 신원/시계/저장소 증거는 합성 입력이다. 단일 순서 모델은 실서비스 동시성이나 분산 commit을 입증하지 않는다. 테스트의 승인 수와 시각은 예제이며 정책 선택이 아니다.', '',
          '[검증 기록](trust-evidence-validation.json) · [구조화 명세](management-trust-evidence.json) · [상위 신뢰 설계](management-trust-lifecycle.md)', '', d['nextDesign']]
rendered = '\n'.join(lines)+'\n'
if args.check:
    assert (P/'management-trust-evidence.md').read_text() == rendered
else:
    (P/'management-trust-evidence.md').write_text(rendered)
print(json.dumps(dict(status='passed', scope='evidence_document_structure', records=len(ids),
                     rules=len(d['rules']), scenarios=len(d['scenarios']), runtimeVerified=False)))
