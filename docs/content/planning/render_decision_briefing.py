"""Render/check decision material; never selects policy or implements a product."""
import argparse
import hashlib
import json
from pathlib import Path

P = Path(__file__).resolve().parent
R = P.parents[1]
parser = argparse.ArgumentParser()
parser.add_argument('--check', action='store_true')
args = parser.parse_args()
load = lambda p: json.loads((R / p).read_text())
doc = load('content/planning/decision-briefing.json')
w = load('content/planning/work-breakdown.json')
reg = load('content/planning/design-integration-register.json')
tech = load('content/planning/technology-selection.json')
experiments = load('content/planning/technology-validation-plan.json')['experiments']
rr = load('content/specifications/return-recovery-contract.json')['openDecisions'][0]
ledger = load('content/planning/preimplementation-handoff.json')['decisionLedger']
tasks = {t['id'] for t in w['tasks']}
choices = {t['id']: t for t in tech['choices']}
cards = {c['id']: c for c in doc['cards']}
sources = {s['id']: s for s in doc['externalSources']}
assert len(cards) == len(doc['cards']) == 20
assert set(cards) == {d['id'] for d in w['decisions']} | {'RR-DEC-01'}
assert doc['implementation'] == 'deferred_by_user' and not doc['runtimeVerified']
assert doc['confirmedConstraints'] == load('content/planning/full-scope-design-review.json')['confirmed']
assert all(d['status'] == 'open' for d in w['decisions'])
assert all(d['selection'] is None for d in ledger)
assert all(c['selectedVersion'] is None for c in tech['choices'])
assert rr['selection'] is None
for path, digest in doc['sourceHashes'].items():
    assert hashlib.sha256((R/path).read_bytes()).hexdigest() == digest, path
for row in doc['cards']:
    assert row['selection'] is None and row['status'] == 'prepared_not_selected'
    assert not row['runtimeVerified']
    assert row['recommendedOption']['id'] != row['alternativeOption']['id']
    for key in ('unresolvedFields','selectionEvidence','freezeAfterSelection','tradeoff','questionRouting'):
        assert row[key], (row['id'], key)
    assert set(row['taskRefs']) <= tasks
    assert set(row['technologyRefs']) <= choices.keys()
    assert set(row['coordinateWith']) <= cards.keys() and row['id'] not in row['coordinateWith']
    assert set(row['externalSourceRefs']) <= sources.keys()
    assert set(row['experimentRefs']) <= {e['id'] for e in experiments}
    obj=load(row['source'])
    for key in row['pointer'].strip('/').split('/'):
        obj = obj[int(key)] if isinstance(obj,list) else obj[key]
    assert obj['id'] == row['id']
    if row['id'].startswith('D'):
        original = next(d for d in w['decisions'] if d['id'] == row['id'])
        route = next(d for d in reg['decisions'] if d['id'] == row['id'])
        assert row['taskRefs'] == original['affectedTasks']
        assert row['technologyRefs'] == original['technologyRefs']
        for source, field in [('policyInputs','policyInputs'),('engineeringDesign','engineeringDesign'),('implementationVerification','runtimeEvidence'),('policyRouting','questionRouting')]:
            assert row[field] == route[source]
        assert set(row['experimentRefs']) == {e for tid in row['technologyRefs'] for e in choices[tid]['experimentRefs']}
    else:
        assert {row['recommendedOption']['id'],row['alternativeOption']['id']} == set(rr['options'])
        assert row['questionRouting'] == 'existing_question_pending_do_not_reask'
grouped = [r for g in doc['groups'] for r in g['decisionRefs']]
assert len(grouped) == len(set(grouped)) == 20 and set(grouped) == cards.keys()
for target in doc['exampleTargets']:
    assert set(target['decisionRefs']) <= cards.keys()
    assert target['basis'] and target['measurement'] and target['onFailure']
assert 16000 * 2 * 30 * 60 == 57_600_000

lines = ['# 구현 전 선택을 위한 결정 자료','',
 '기존 19개 결정과 신규 여행 EOA 복구 질문 1개를 선택 카드로 정리했다. 권장안은 검토를 위한 제안이며 선택값은 모두 비워 두었다. 전체 15개 기능·12주·3명·앱 연동 범위를 유지한다.','',
 '## 먼저 볼 내용','',
 '- 사용자 경험·비용·복구 권한을 바꾸는 선택과 기술 검증으로 좁힐 수 있는 선택을 구분했다. 기술 항목마다 새 사용자 승인을 요구하는 목록이 아니다.',
 '- D02(연결 규칙), D10(시험 환경)은 기존 분류상 새 사용자 질문 없이 기술 검토를 이어갈 수 있다. 미확인 실배포값이나 하드웨어 능력은 임의로 확정하지 않는다.',
 '- RR-DEC-01은 이미 열린 복구 정책 질문이다. 이 문서에서 재질문하거나 답변을 추정하지 않는다.',
 '- 제공자·SDK 버전·보드 저장 용량·법적 운영 가능 여부는 이 자료로 확인 완료되지 않는다. 운영자 후원은 기존 후속 범위를 유지한다.','',
 '## 검토 묶음','', '| 묶음 | 포함 선택 |','|---|---|']
for g in doc['groups']:
    lines.append(f"| {g['title']} | {', '.join(g['decisionRefs'])} |")
lines += ['', doc['readingOrder'], '', '## 선택이 설계로 반영되는 과정', '',
          '```mermaid','flowchart LR',' A["제품 선호·통제 권한"] --> C["선택 기록 + 적용 범위"]',
          ' B["기술 비교·보드/제공자 근거"] --> C',' C --> D["profile·wire·ABI·저장 배치 고정"]',
          ' D --> E["계약 묶음 채택 검증"]',' E --> F["구현 전환 지시 이후 개발"]',
          ' F --> G["실기·앱·체인 수용 증거"]','```','',
          '다른 독립 설계는 미선택 항목 때문에 멈출 필요가 없다. 선택이나 문서 검증은 실행 검증을 대신하지 않는다.','',
          '## 결정별 비교 카드','']
for row in doc['cards']:
    lines += [f"### {row['id']} · {row['title']}", '',
              '**권장 검토안** — '+row['recommendedOption']['proposal'], '',
              '**대안** — '+row['alternativeOption']['proposal'], '',
              '**차이와 조건** — '+row['tradeoff'], '',
              '**입력할 선택값**', '']
    lines += ['- '+s for s in row['unresolvedFields']]
    lines += ['', '**선택을 뒷받침할 근거**', '']
    lines += ['- '+s for s in row['selectionEvidence']]
    lines += ['', '**결정 후 고정할 설계** — '+' / '.join(row['freezeAfterSelection']), '',
              '**판단 경계** — 사용자·운영 정책: '+(' / '.join(row['policyInputs']) or '새 사용자 질문 없음; 기술 근거로 진행')+
              ' · 기술 설계: '+' / '.join(row['engineeringDesign'])+
              ' · 실제 확인: '+' / '.join(row['runtimeEvidence']), '',
              '**연결** — 작업 '+', '.join(row['taskRefs'])+'; 기술 '+', '.join(row['technologyRefs'])+
              '; 검증 계획 '+(', '.join(row['experimentRefs']) or '기존 복구/반납 수용 사례')+
              '; 함께 검토 '+', '.join(row['coordinateWith'])+'.', '',
              '**상태** — 선택 없음 · 원본 `'+row['source']+'#'+row['pointer']+'` · '+row['questionRouting']
              ]
    for ref in row['externalSourceRefs']:
        s=sources[ref]
        lines += ['', f"근거: [{s['title']}]({s['url']}) — {s['supports']}. 이 근거만으로 {s['doesNotEstablish']}을 확인했다고 보지 않는다."]
    lines += ['']
lines += ['## 시험 목표의 비교 출발점','',
          '아래 숫자는 성능 실측이나 채택된 운영 약속이 아니다. 기존 목표가 비어 있어 선택할 수 있도록 만든 제안값이다. 모두 실제 개발 단계에서 측정한다.','']
for t in doc['exampleTargets']:
    lines += [f"### {t['id']} · {t['metric']}", '',
              '- 제안: '+t['proposal'], '- 조건: '+t['basis'], '- 측정: '+t['measurement'], '- 미달 때: '+t['onFailure'],'']
lines += ['## 선택 기록 양식','',
          '| 필드 | 남길 내용 |','|---|---|',
          '| 선택 ID·옵션 | D번호 또는 RR-DEC-01, 권장/대안/수정안 |',
          '| 적용 범위 | 제품·환경·기능·자산·사용자 범위 |',
          '| 근거 | 선택 이유와 기술 자료/사용자 결정 기록 |',
          '| 고정할 버전 | profile/계약 및 변경 전 기록 |',
          '| 실제 검증 | 미실행/실행 구분, 증거·기종·배포 identity |',
          '| 변경 관계 | 시행 조건과 대체하는 이전 선택 |','',
          '선택이 확정되면 원 decision ledger, 기술 선택, 영향 계약·화면·저장 설계를 같은 변경 묶음으로 갱신한다. 현재는 어떤 원본 선택도 변경하지 않았다.','',
          '## 근거 문서','',
          '[원 결정 목록](decisions.md) · [선택의 성격 구분](design-integration-register.md) · [기술 후보](technology-selection.md) · [검증 계획](technology-validation-plan.md) · [설계 인계서](preimplementation-handoff.md) · [구조화된 비교 카드](decision-briefing.json) · [검증 결과](decision-briefing.validation.json)','',
          '다음 설계 작업은 선택값을 주입할 profile 필드와 교차 제약을 정의하고, 미선택 상태에서 허용/보류할 동작과 화면을 연결하는 것이다.']
rendered='\n'.join(lines)+'\n'
result=dict(status='passed',scope='decision_material_structure_and_source_integrity_only',cards=len(cards),groups=len(doc['groups']),
            sourceHashChecks=len(doc['sourceHashes']),technologyCandidates=len(choices),experimentPlans=len(experiments),
            sourceDecisionsUnchanged=True,selectedOptions=0,proposedMeasurementTargets=len(doc['exampleTargets']),
            productImplementationPerformed=False,runtimeVerified=False)
if args.check:
    assert (P/'decision-briefing.md').read_text() == rendered, 'briefing render drift'
    assert json.loads((P/'decision-briefing.validation.json').read_text()) == result
else:
    (P/'decision-briefing.md').write_text(rendered)
    (P/'decision-briefing.validation.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
print(json.dumps(result,ensure_ascii=False))
