"""Rebuild a design trace audit from current sources; does not run product code.
Run build_graph.py first. Output is confined to this analysis directory.
"""
import collections
import hashlib
import json
from pathlib import Path

O = Path(__file__).resolve().parent
R = O.parents[2]
ARCHIVE = R / 'design-history/approval-premerge-20260918'
load = lambda p: json.loads((R / p).read_text())
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()

def pointer(value, path):
    for part in path.strip('/').split('/') if path else []:
        part = part.replace('~1', '/').replace('~0', '~')
        if '[id=' in part and part.endswith(']'):
            field, identity = part[:-1].split('[id=')
            matched = [row for row in value[field] if row.get('id') == identity]
            assert len(matched) == 1
            value = matched[0]
        else:
            value = value[int(part)] if isinstance(value, list) else value[part]
    return value

w = load('content/planning/work-breakdown.json')
f = load('content/planning/full-scope-design-review.json')
h = load('content/planning/preimplementation-handoff.json')
freeze = load('content/planning/design-freeze-checkpoint.json')
ov = load('content/specifications/preimplementation-contract-overlay.json')
trace = load('content/analysis/document-logic/contract-task-trace.json')
g = load('content/analysis/document-logic/graph.json')
findings = load('content/analysis/document-logic/findings.json')['findings']
tasks = {t['id']: t for t in w['tasks']}
assert len(tasks) == len(w['tasks']) == 104
steps = [s['id'] for t in tasks.values() for s in t['implementationSteps']]
assert len(steps) == len(set(steps)) == 320
homes = collections.defaultdict(list)
for p in f['designPackages']:
    for tid in p['primaryTaskRefs']:
        assert tid in tasks
        homes[tid].append(p['id'])
assert set(homes) == set(tasks) and all(len(v) == 1 for v in homes.values())
assert len(f['requirements']) == 15
requirements = []
for r in f['requirements']:
    derived = sorted(t['id'] for t in tasks.values() if r['requirement'] in t['requirements'])
    assert set(derived) == set(r['taskRefs']) | set(r['verificationTaskRefs'])
    assert not set(r['taskRefs']) & set(r['verificationTaskRefs'])
    assert r['verificationTaskRefs'] and set(r['verificationTaskRefs']) <= tasks.keys()
    requirements.append(dict(requirement=r['requirement'], title=r['title'], taskRefs=derived,
                             packageRefs=sorted({homes[t][0] for t in derived}),
                             verificationTaskRefs=r['verificationTaskRefs'],
                             sourcePointer=f"/requirements/{len(requirements)}"))
visiting, done = set(), set()
def walk(tid):
    assert tid not in visiting, ('dependency cycle', tid)
    if tid in done:
        return
    visiting.add(tid)
    for dep in tasks[tid]['dependsOn']:
        assert dep in tasks
        walk(dep)
    visiting.remove(tid)
    done.add(tid)
for tid in tasks:
    walk(tid)
routes = {r['id']: r for r in ov['routeContracts']}
assert len(trace['routes']) == len(routes) == 26
assert {r['contractId'] for r in trace['routes']} == routes.keys()
contract_rows = []
for r in trace['routes']:
    assert pointer(load(r['source']), r['pointer'])['id'] == r['contractId']
    assert r['taskRefs'] and set(r['taskRefs']) <= tasks.keys()
    contract_rows.append(dict(r, packageRefs=sorted({homes[t][0] for t in r['taskRefs']})))

# Historical baseline fields resolve against the immutable premerge checkpoint.
# Current agreement alone is not used as evidence of a historical baseline.
hashes = []
map_fields = {'sourceHashes', 'sourceFiles', 'referenceSqlHashes', 'baselineFiles',
              'baselineHashes', 'migrationHashes', 'files'}
def record(container, field, target, digest, historical=False):
    assert target.is_file(), (container, field, target)
    rel = str(target.relative_to(R))
    archive = ARCHIVE / rel
    if historical:
        assert archive.is_file() and sha(archive) == digest, (container, field, rel, 'historical pin mismatch')
        status = 'historical_matches_current' if sha(target) == digest else 'historical_differs_from_current'
        evidence = str(archive.relative_to(R))
    else:
        assert sha(target) == digest, (container, field, rel, 'current pin mismatch')
        status, evidence = 'current_matches', rel
    hashes.append(dict(container=container, field=field, source=rel, expectedSha256=digest,
                       status=status, evidence=evidence))
for p in sorted((R / 'content').rglob('*.json')):
    if O in p.parents:
        continue
    doc = json.loads(p.read_text())
    if not isinstance(doc, dict):
        continue
    rel = str(p.relative_to(R))
    for field in map_fields:
        value = doc.get(field, {})
        if not isinstance(value, dict):
            continue
        for name, digest in value.items():
            if not isinstance(digest, str) or len(digest) != 64:
                continue
            candidates = {q.resolve() for q in (R/name, p.parent/name, p.parent/'database'/name) if q.is_file()}
            assert len(candidates) == 1, (rel, field, name, candidates)
            record(rel, field+'/'+name, candidates.pop(), digest, field in {'baselineFiles', 'baselineHashes'})
root_pins = [
 ('content/specifications/critical-dtos.json','sharedSourceSha256','content/specifications/core.schema.json'),
 ('content/specifications/extended-dtos.json','sourceSha256','content/specifications/critical-dtos.schema.json'),
 ('content/specifications/lifecycle-journey-acceptance.json','sourceWbsHash','content/planning/work-breakdown.json'),
 ('content/specifications/approval-baseline.json','checkpointManifestSha256','design-history/approval-premerge-20260918/checkpoint.json'),
 ('content/specifications/approval-baseline.json','checkpointReportSha256','design-history/approval-premerge-20260918/validation-report.json')]
for container, field, target in root_pins:
    record(container, field, R/target, load(container)[field])
for i, m in enumerate(load('content/specifications/database/schema-catalog.json')['migrations']):
    record('content/specifications/database/schema-catalog.json', f'migrations/{i}/sha256',
           R/'content/specifications/database'/m['file'], m['sha256'])
checkpoint = json.loads((ARCHIVE/'checkpoint.json').read_text())
for name, digest in checkpoint['files'].items():
    assert sha(ARCHIVE/name) == digest, name

# Both pre-fix evidence and current resolutions must remain reproducible.
for row in findings:
    assert row['status'] == 'resolved_in_design_runtime_unverified'
    for e in row['evidence']:
        p = R/e['snapshotFile']
        assert sha(p) == e['sha256'] and e['quote'] in p.read_text()
        if e.get('pointer'):
            pointer(json.loads(p.read_text()), e['pointer'])
    for e in row['resolutionEvidence']:
        assert sha(R/e['file']) == e['sha256']
        pointer(load(e['file']), e['pointer'])
    assert (R/row['validationCommand'].split()[1]).is_file()
nodes = {n['id']: n for n in g['nodes']}
assert len(nodes) == len(g['nodes'])
for n in nodes.values():
    if n['kind'] == 'document':
        assert sha(R/n['source']) == n['sha256'], n['source']
for e in g['edges']:
    assert e['source'] in nodes and e['target'] in nodes
    p = e['provenance']
    assert (R/p['file']).is_file()
    if (p.get('pointer') or '').startswith('/') and p['file'].endswith('.json'):
        pointer(load(p['file']), p['pointer'])
for r in trace['routes']:
    for tid in r['taskRefs']:
        assert any(e['source']=='candidate:'+r['contractId'] and e['target']=='task:'+tid and
                   e['relation']=='reviewed_task_impact' for e in g['edges'])
assert len(w['decisions']) == 19 and all(d['status']=='open' for d in w['decisions'])  # legacy source record
assert all(c['selectedVersion'] is None for c in load('content/planning/technology-selection.json')['choices'])  # legacy comparison record
assert next(d for d in load('content/specifications/return-recovery-contract.json')['openDecisions'] if d['id']=='RR-DEC-01')['selection'] is None  # legacy question record
assert freeze['counts']['selectedDecisions'] == 20
assert {d['id'] for d in freeze['decisions']} == {f'D{i:02}' for i in range(1,20)} | {'RR-DEC-01'}
assert all(d['status']=='selected_for_design_baseline' for d in freeze['decisions'])
assert all(nodes['decision:'+d['id']]['status']=='selected_for_design_baseline' for d in freeze['decisions'])
assert not h['runtimeVerified'] and not h['allImplementationReady']
assert h['ownerAssignment'] is None and h['effortEstimates'] is None
summary = dict(requirements=len(requirements), tasks=len(tasks), steps=len(steps), packages=len(f['designPackages']),
               candidateContracts=len(contract_rows), contractTaskLinks=sum(len(r['taskRefs']) for r in contract_rows),
               dependencyEdges=sum(len(t['dependsOn']) for t in tasks.values()), dependencyCycles=0,
               digestChecks=len(hashes), digestKinds=dict(collections.Counter(x['status'] for x in hashes)),
               frozenArchiveFiles=len(checkpoint['files']), resolvedDesignFindings=len(findings),
               graphNodes=len(nodes), graphEdges=len(g['edges']), legacyOpenDecisionRecords=19,
               currentSelectedDecisions=20, currentOpenDecisions=0)
result = dict(status='passed', scope='design_traceability_and_evidence_integrity_only', runtimeVerified=False,
              implementationReady=False, xTheoryApplied=False, summary=summary, requirements=requirements,
              contractTaskTrace=contract_rows, digestEvidence=hashes, remainingGates=h['phaseGates'],
              residualRegister=h['designResidualRegister'],
              limits=['Mapping is change impact, not exhaustive behavioral proof.',
                      'Finite design examples do not establish runtime security or concurrency correctness.',
                      'Historical baseline mismatches are checked against the frozen archive, never rebased.'])
(O/'traceability-audit.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
lines = ['# 전체 설계 추적 관계·무결성 점검','',
 '15개 요구사항 → 104개 작업 → 320개 세부 작업 → 8개 설계 묶음의 연결을 확인했다. 추가 계약 26개에는 변경 영향 작업을 연결했다. 문서 검사 통과이며 제품 구현·실행 완료를 의미하지 않는다.','',
 '## 보완한 내용','',
 '- 계약별 작업 연결을 별도 검토표와 그래프에 반영했다. 여러 묶음에 걸치는 계약은 모든 관련 작업의 주 설계 묶음을 표시한다.',
 '- 초기 보완 기록의 계약 개수는 당시 20개, 현재 26개임을 인계서에 명시했다.',
 '- 현재 입력 참조와 과거 기준 참조를 구분했다. 과거 해시를 현재 값으로 덮어쓰지 않았다.','',
 '## 검증 결과','',
 f"- WBS 의존 관계 {summary['dependencyEdges']}개: 순환 0개. 작업마다 주 설계 묶음 1개.",
 f"- 계약·작업 연결 {summary['contractTaskLinks']}개: 없는 계약·작업 참조 0개.",
 f"- 참조 해시 {len(hashes)}개: 현재 일치 {summary['digestKinds'].get('current_matches',0)}개, 과거 기준 {sum(x['status'].startswith('historical') for x in hashes)}개 모두 보존본과 일치.",
 f"- 그중 현재와 다른 과거 참조 {summary['digestKinds'].get('historical_differs_from_current',0)}개는 변경 전 기준의 정상 기록이다. 보존 파일 {len(checkpoint['files'])}개도 검증했다.",
 f"- 수정 사항 {len(findings)}개의 수정 전 근거·현재 해결 위치·파일 해시와 검증 명령 연결을 확인했다.",
 f"- 그래프 {len(nodes)}개 노드·{len(g['edges'])}개 관계: 끝점·문서 해시·JSON 위치를 확인했다.",
 '- 이 검사는 기존 322개 설계 검사·예제 및 20개 정합성 검사와 별도의 참조 검증이다.','',
 '## 요구사항별 추적','', '| 번호 | 요구사항 | 작업 수 | 관련 설계 묶음 |','|---|---|---:|---|']
for r in requirements:
    lines.append(f"| {r['requirement']} | {r['title']} | {len(r['taskRefs'])} | {', '.join(r['packageRefs'])} |")
lines += ['', '작업 수는 검증 작업을 포함하며 요구사항 간 중복을 포함한다. 관련 설계 묶음은 작업에서 유도한 영향 범위이며 묶음 자체의 선언 범위와 다를 수 있다.', '',
          '## 추가 계약별 작업 연결','', '| 계약 | 작업 | 설계 묶음 |','|---|---|---|']
for r in contract_rows:
    lines.append(f"| {r['contractId']} | {', '.join(r['taskRefs'])} | {', '.join(r['packageRefs'])} |")
lines += ['', '## 현재 동결과 남은 실행 조건','',
 '정책 19개와 RR-DEC-01의 과거 open 기록은 보존한다. 현재 authority인 DF-20260920-01에는 20개 모두 선택되어 있으며 그래프의 decision 상태도 선택 상태로 overlay했다. 아래 항목은 구현·실증 전이라 계속 남는다.','',
 '| 번호 | 남은 실행 조건 |','|---|---|']
for i,row in enumerate(freeze['residualConditions'],1):
    lines.append(f"| {i} | {row} |")
lines += ['', '담당자·작업량은 배정하지 않았다. 제품 코드·generated schema·후속 migration·실제 profile·배포 주소·실기와 서비스 검증은 아직 수행하지 않았다. x-theory의 정의가 미확인되어 현재 그래프는 중립적 관계 분석이다.','',
 '## 근거와 재현','', '[기존 검사 재실행 결과](traceability-regression-results.json) · [전체 검증 기록](traceability-audit.json) · [계약-작업 검토표](contract-task-trace.json) · [그래프](graph.json) · [논리 보완 결과](review.md)','',
 '```sh','python3 content/analysis/document-logic/build_graph.py','python3 content/analysis/document-logic/validate_traceability.py','python3 content/analysis/document-logic/render_review.py','```']
(O/'traceability-audit.md').write_text('\n'.join(lines)+'\n')
print(json.dumps(dict(status='passed', **summary), ensure_ascii=False))
