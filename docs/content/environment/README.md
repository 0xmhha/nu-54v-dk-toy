# 구현 전 외부 환경 준비

2026-09-20 · checkpoint `DF-20260920-01`

이 디렉터리는 구현을 시작하지 않고도 준비할 수 있는 환경 계약만 담는다. 실제 비밀값은 저장하지 않으며 `secretRef`만 기록한다. 현재 사용자의 요청은 이 설계·준비 checkpoint까지이며 제품 코드, 계정 생성, 컨트랙트 배포, 서버 기동은 아직 시작하지 않는다.

| 항목 | 상태 | 산출물 | 외부에서 남은 일 |
|---|---|---|---|
| E-01 · 구현 전환 경계 | prepared | [README.md](../environment/README.md) | 사용자의 별도 구현 착수 지시 |
| E-02 · 개발 기준 고정 | prepared_conditional_hardware | [toolchain-pins.json](../environment/toolchain-pins.json) | NU-54V-DK 실제 revision·board definition commit·flash 측정 |
| E-03 · 계정·공급자 | template_ready_external_registration | [provider-accounts.template.json](../environment/provider-accounts.template.json) | Google/Apple/Kakao/OpenAI 계정·redirect·비밀 등록 |
| E-04 · StableNet 환경 | rpc_verified_deployments_pending | [stablenet-8283.manifest.json](../environment/stablenet-8283.manifest.json) | dummy USDC와 상품 계약 배포·주소/codeHash/ABI digest |
| E-05 · Indexer 환경 | revision_pinned_runtime_pending | [indexer.manifest.json](../environment/indexer.manifest.json) | 실 endpoint·DB/object store·decoder registry 배포 |
| E-06 · 운영 신뢰 bootstrap | template_ready_external_registration | [trust-bootstrap.template.json](../environment/trust-bootstrap.template.json) | 실제 인물·workload identity·KMS/HSM key reference의 2인 등록 |
| E-07 · 개발·시험 데이터 | prepared | [test-fixtures.manifest.json](../environment/test-fixtures.manifest.json) | 실 카페 정보는 동의 후 별도 tenant로 입력 |

`prepared`는 저장소 안의 템플릿·고정값·차단 규칙이 준비됐다는 뜻이다. 계정 발급, 실제 키 등록, 배포 주소, 실기 검증이 끝났다는 뜻은 아니다.
