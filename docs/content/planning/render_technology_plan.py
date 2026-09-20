"""Render and validate technology choices and implementation-stage experiments."""
import json,sys
from pathlib import Path
P=Path(__file__).resolve().parent
L=lambda n:json.loads((P/n).read_text())
w=L('work-breakdown.json');choices=L('technology-selection.json')['choices'];experiments=L('technology-validation-plan.json')['experiments'];sources=L('technology-sources.json')['sources']
taskids={x['id'] for x in w['tasks']};decisionids={x['id'] for x in w['decisions']};sourceby={x['id']:x for x in sources};expby={x['id']:x for x in experiments}
assert len(choices)==len({x['id'] for x in choices})==19
assert len(experiments)==len(expby)==23
assert {d for x in choices for d in x['decisionRefs']}==decisionids
for c in choices:
 assert set(c['taskRefs'])<=taskids and set(c['sourceRefs'])<=sourceby.keys()
 assert set(c['experimentRefs'])=={e['id'] for e in experiments if e['technologyId']==c['id']}
 assert c['status']==('base_confirmed_version_pending' if c['id']=='TECH-06' else 'proposed_not_selected')
 assert c['selectedVersion'] is None and not c['runtimeVerified']
for e in experiments:
 assert set(e['taskRefs'])<=taskids and e['checks'] and e['successEvidence'] and e['failureAction']
 assert e['status']=='planned_not_run' and not e['runtimeVerified']
 assert e['owner'] is None and e['effortEstimate'] is None
for t in w['tasks']:
 assert set(t['technologyRefs'])=={c['id'] for c in choices if t['id'] in c['taskRefs']}
 assert set(t['technologyValidationRefs'])=={e['id'] for e in experiments if t['id'] in e['taskRefs']}
for d in w['decisions']:
 assert set(d['technologyRefs'])=={c['id'] for c in choices if d['id'] in c['decisionRefs']}
intro='''# 프로토콜·라이브러리·저장 방식 선택안

2026-09-18. 19개 기술 선택 카드와 23개 검증 작업을 기존 D01~D19 및 WBS에 연결했다. 전체 15개 기능·앱 연동·12주 완료 전제와 3명 기준을 유지한다. 검증 작업은 기존 세부 작업의 실행/증거를 구체화한 것이며 별도 인력·공수·주차 배정이 아니다.

**작업용 기본 방향은 RN 앱·키오스크, Go 업무 API, PostgreSQL 원장/보안 상태, 보호 객체 저장소, NU용 Zephyr 기반 펌웨어, MCUboot/SMP FOTA, 기존 indexer·계약·viem 계열 재사용이다.** 사용자가 확정한 Zephyr 기반·RN 키오스크·체인 8283·EOA 우선·고객 가스·스마트 계정의 12주 내 지원은 유지한다. Zephyr의 SDK 배포판(NCS 포함 여부)·정확한 revision과 나머지 라이브러리는 검증 전 제안이다.

[선택 원본](technology-selection.json) · [검증 작업 카드](technology-validation-plan.md) · [관찰한 버전](technology-version-baseline.json) · [출처 기록](technology-sources.json) · [기존 API 통합](../specifications/security-api-integration.md)

## 선택 방법과 먼저 확인할 조합

1. **공통 실행 환경:** TECH-01/03/04/06으로 RN native bridge·Go/DB·NU 빌드가 연결되는지 확인한다. 제품별 adapter 경계를 유지하고 새로운 마이크로서비스 19개를 만들지 않는다.
2. **구현 경로를 결정하는 조건:** TECH-07/08/10/12/14/17의 key backend·FOTA·패스키·MPC·UserOperation·x402를 먼저 검증한다. 사용자가 말한 대로 실기 검증은 실제 개발 단계에 수행하며, 이번에는 실험 입력과 판정을 준비한다.
3. **기능 품질과 운영:** 오디오/가격/장소/AI/복구는 실제 표본과 출처를 포함해 검증한다. 참고 구현이 있다는 사실과 앱에서 기능이 동작한다는 증거를 구분한다.

검증 뒤 선택 기록에 후보 revision, 환경, 통과/실패 이유, 대안, 적용 API/작업, 남은 제한을 넣는다. 통과하지 못하면 adapter를 수정하거나 대안을 검증하며 원래 기능을 자동 제외하지 않는다. 제공자 비용·보관 정책·운영 권한 같은 사용자 결정이 필요한 항목은 선택 이유와 영향을 구체화한 뒤 묶어 결정한다.

## 기존 코드에서 확인한 버전과 재사용 경계

| 대상 | 이번에 읽은 선언 | 처리 방침 |
|---|---|---|
| indexer-go | Go 1.24.0, toolchain go1.24.9, chi v5.2.3, Pebble v1.1.5, go-ethereum v1.16.5 | 같은 commit의 동작을 재현한 뒤 지원 버전·취약점·호환성 검토 결과로 pin |
| poc-platform | pnpm 10.30.3, Node >=22, TypeScript ^5.7.2, viem ^2.46.3 | 선언 범위가 실제 resolved 버전이라는 뜻은 아님. lockfile과 RN 적합성 검증 |
| 웹 앱 | Next ^15.5.12, React ^19.0.0 | 웹 백오피스 재사용 후보. RN에 웹 DOM/storage 코드를 그대로 가져오지 않음 |
| stable-poc-contract | solc 0.8.28, EVM prague, optimizer 200 | 체인 fork 기능 및 실제 bytecode/ABI를 확인. compiler 설정만으로 체인 지원 단정 금지 |

위 숫자는 **기존 소스에서 확인한 선언**이지 최신 버전 추천이나 설치 결과가 아니다. 기존 검토 commit은 [버전 기록](technology-version-baseline.json)에 보존했다. Nordic latest 문서에 보이는 개발 버전도 자동 채택하지 않으며 NU board definition과 기능을 함께 검증한 release/commit을 고정한다.

## 호환성 검토에서 드러난 구체 조건

- react-native-ble-plx는 GATT central 작업의 후보이며, README상 bonding/peripheral 기능을 제공하지 않는다. 필요한 본딩 native adapter와 CS peer 연결을 따로 검증한다. [공식 저장소](https://github.com/dotintent/react-native-ble-plx)
- cb-mpc는 암호 primitive 라이브러리이고 공개 지원 환경은 macOS/Linux 중심이다. peer 인증·저장·복구 운영과 RN 모바일 참여자 구현은 별도다. [공식 설명](https://github.com/coinbase/cb-mpc)
- LFDT의 cggmp21 이름을 가진 저장소는 현재 README에서 CGGMP24를 설명하고 key refresh 미지원을 명시한다. 이름만 보고 기존 복구/회전 요구를 충족한다고 선정하지 않는다. [공식 README](https://github.com/LFDT-Lockness/cggmp21)
- x402 exact EVM은 특정 token authorization 경로를 요구한다. 현재 일반 ERC20Mock으로 이를 충족한다고 볼 수 없으며, facilitator의 실제 gas payer가 고객 가스 원칙과 일치하는지도 검증한다. 표준 exact 지원을 주장하려면 해당 규격 시험을 통과해야 한다. custom mechanism은 지원 상대와 규격 차이를 명시한다. [EVM 구현 문서](https://pkg.go.dev/github.com/coinbase/x402/go/mechanisms/evm)
- MCUboot/SMP 지원 자료는 출발점이다. NU의 실제 partition·앱 크기·설정 보존과 모바일 전송을 같은 조합으로 검증해야 한다. [Nordic FOTA](https://nrfconnectdocs.nordicsemi.com/ncs/latest/nrf/app_dev/device_guides/nrf54l/fota_update.html)

Cloud MPC의 2-of-3를 선택한다면 앱 참여자·서버 참여자·독립 복구 참여자의 신뢰/저장 경계를 먼저 정의한다. 서버 운영자 권한 하나로 두 share를 모두 얻을 수 있는 배치는 피한다. 소셜 로그인과 quorum 참여 권한은 분리하며, 기존 HW 단독 승인 기능에 폰 co-sign을 추가로 강제하지 않는다. 라이브러리의 refresh와 participant replacement/recovery는 같은 기능으로 뭉뚱그리지 않고 각각 시험한다.

## 19개 선택 카드
'''
s=[intro]
for c in choices:
 s += [f"### {c['id']} · {c['title']}",'',f"- 작업용 선택안: {c['recommendation']}",f"- 비교 대안: {c['alternative']}",f"- 채택 조건: {c['selectionGate']}",f"- 연결: {', '.join(c['decisionRefs'])}; {', '.join(c['taskRefs'])}",f"- 검증: {', '.join(c['experimentRefs'])}; 현재 미실행",('- 상태: Zephyr 기반은 사용자 확정. SDK 배포판/버전·보드 target 미선정.' if c['id']=='TECH-06' else '- 상태: 제안. 라이브러리/프로토콜 버전 미선정.')]
 if c['sourceRefs']:s+=['- 근거: '+', '.join(f"[{sourceby[i]['title']}]({sourceby[i]['url']})" for i in c['sourceRefs'])]
 else:s+=['- 근거: 기존 요구·API 설계와 [로컬 소스 선언](technology-version-baseline.json). 새 외부 호환성 검증을 의미하지 않음.']
 s+=['']
s += ['## 채택 전 남길 산출물','', '- source/lockfile/toolchain/model digest를 포함한 재현 manifest와 license 검토 기록.','- MCU key handle·MPC signer·BLE/FOTA·API/schema·object/DB의 producer/consumer 호환 행렬.','- 기능별 성공/실패/중단 복구 결과와 민감정보를 제거한 증거.','- 허용 지연·자원 예산·TTL·철회 SLA·보관 수명의 실제 설정값. 미측정 값을 사용자 완료 약속으로 꾸미지 않는다.','', '이번 검사는 계획 ID·원문 결정·작업·출처·검증 카드 연결의 일관성이다. 라이브러리 설치, provider 등록, firmware 빌드/플래시, 컨트랙트 배포나 실결제는 수행하지 않았다.']
views={'technology-selection.md':'\n'.join(s)+'\n'}
s=['# 기술 선택 검증 작업 23개','','[선택 카드](technology-selection.md) · [JSON 원본](technology-validation-plan.json). 실제 개발 단계에서 수행할 검증이다. 모두 planned_not_run이며 기존 104개 패키지/320개 세부 작업의 완료 증거를 구체화한다.','']
for e in experiments:
 s += [f"## {e['id']} · {e['title']}",'',f"- 기술 선택: {e['technologyId']}; 관련 작업: {', '.join(e['taskRefs'])}",'- 실행 환경: 실제 대상 기기/앱 또는 격리된 시험 환경. 정확한 빌드·profile·chain 설정을 기록한다.','- 판정 항목:']+['  - '+x for x in e['checks']]+['- 증거: '+ '; '.join(e['successEvidence']),'- 실패 시: '+e['failureAction'],'- 상태: 미실행; 담당자·공수 미지정.','']
views['technology-validation-plan.md']='\n'.join(s)+'\n'
for n,v in views.items():
 if '--check' in sys.argv:assert (P/n).read_text()==v,n
 else:(P/n).write_text(v)
print(json.dumps(dict(technologyChoices=len(choices),validationCards=len(experiments),coveredDecisions=len(decisionids),sourceRecords=len(sources),runtimeTests='not_run',mode='check' if '--check' in sys.argv else 'render')))
