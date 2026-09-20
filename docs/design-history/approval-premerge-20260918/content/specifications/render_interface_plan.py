"""Validate logical interface/version inventory and render planning views only."""
import json,sys,copy
from pathlib import Path
from jsonschema import Draft202012Validator
P=Path(__file__).resolve().parent
L=lambda n:json.loads((P/n).read_text())
w=json.loads((P.parent/'planning/work-breakdown.json').read_text());interfaces=L('implementation-interfaces.json')['interfaces'];matrix=L('compatibility-matrix.json')['rows'];schema=L('compatibility-manifest.schema.json');cases=L('compatibility-manifest.examples.json')['examples']
taskids={t['id'] for t in w['tasks']};apiids={a['id'] for a in L('api-catalog.json')['operations']};bleids={c['name'] for c in L('ble-catalog.json')['commands']};techids={t['id'] for t in json.loads((P.parent/'planning/technology-selection.json').read_text())['choices']}
iids={i['id'] for i in interfaces};mids={m['id'] for m in matrix}
assert len(interfaces)==len(iids)==14 and len(matrix)==len(mids)==26
assert w['confirmedFirmwareBase']['firmwareBase']=='Zephyr'
for i in interfaces:
 assert set(i['taskRefs'])<=taskids and set(i['apiRefs'])<=apiids and set(i['bleCommands'])<=bleids and set(i['technologyRefs'])<=techids
 assert i['status']=='logical_contract_not_implemented' and not i['abiFrozen']
 assert i['methods'] and all(m['inputShape'] and m['outputShape'] for m in i['methods'])
for r in matrix:
 assert r['interfaceId'] in iids and r['status']=='unverified' and not r['testEvidenceRefs']
 assert len(r['requiredPins'])==len(set(r['requiredPins'])) and {'producerBuild','consumerBuild'}<=set(r['requiredPins'])
for t in w['tasks']:
 assert set(t['implementationInterfaceRefs'])=={i['id'] for i in interfaces if t['id'] in i['taskRefs']}
 assert set(t['compatibilityRefs'])=={m['id'] for m in matrix if m['interfaceId'] in t['implementationInterfaceRefs']}
Draft202012Validator.check_schema(schema);v=Draft202012Validator(schema)
def shape_valid(value):
 return v.is_valid(value) and len({e['matrixId'] for e in value['entries']})==len(value['entries']) and {e['matrixId'] for e in value['entries']}==mids
assert shape_valid(L('compatibility-manifest.template.json'))
for c in cases:assert shape_valid(c['value'])==(c['expected']=='accept'),c['name']
text=(P/'implementation-interfaces.md').read_text().split('<!-- GENERATED_INTERFACES -->')[0]+'<!-- GENERATED_INTERFACES -->\n\n'
for i in interfaces:
 text+=f"### {i['id']} · {i['title']}\n\n{i['producer']} → {i['consumer']}\n\n| 논리 메서드 | 입력 shape | 결과 shape |\n|---|---|---|\n"
 for m in i['methods']:text+=f"| {m['name']} | {m['inputShape']} | {m['outputShape']} |\n"
 text+=f"\n- 경계 조건: {i['invariant']}\n- 연결 작업: {', '.join(i['taskRefs'])}\n- API: {', '.join(i['apiRefs']) or '내부/표준 transport 경로'}\n- BLE: {', '.join(i['bleCommands']) or '직접 명령 없음'}\n\n"
mt='# 버전 호환 행렬\n\n[해석 규칙](implementation-interfaces.md) · [원본](compatibility-matrix.json) · [manifest 템플릿](compatibility-manifest.template.json). 26개 모두 미검증이다. 버전 번호를 임의 채우지 않았으며 실행 가능한 릴리스 허용 목록이 아니다.\n\n'
for r in matrix:
 mt+=f"## {r['id']} · {r.get('caseKind',r['interfaceId'])}\n\n- 경계: {r['interfaceId']} — {r['producer']} → {r['consumer']}\n- 고정할 값: {', '.join(r['requiredPins'])}\n- 판정: {r['compatibilityRule']}\n- 불일치: {r['mismatchAction']}\n- 증거: {'; '.join(r['evidenceRequired'])}\n- 상태: unverified\n\n"
for n,s in {'implementation-interfaces.md':text,'compatibility-matrix.md':mt}.items():
 if '--check' in sys.argv:assert (P/n).read_text()==s,n
 else:(P/n).write_text(s)
print(json.dumps(dict(firmwareBase='Zephyr',interfaces=len(interfaces),compatibilityRows=len(matrix),syntheticShapeFixtures=len(cases),runtimeInteropTests='not_run',mode='check' if '--check' in sys.argv else 'render')))
