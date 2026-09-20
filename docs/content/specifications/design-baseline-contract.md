# 구현 진입 기준 계약

2026-09-20 · `DF-20260920-01` · **설계 기준 채택 / runtime 비활성**

이 문서는 [설계 동결 checkpoint](../planning/design-freeze-checkpoint.md)의 8개 묶음을 공동 채택한 기준 overlay다. 기존 API 110개, BLE 34개, 정책 60개, 7개 reference migration의 상세 inventory는 유지한다. 구현 중 schema·migration·generated client를 만들 때 이 overlay의 선택과 불변조건을 우선한다.

## 공통 wire

- HTTP: `v1-json-jcs`
- BLE: `v1-deterministic-cbor`
- profile: `v1`
- 같은 idempotency key와 payload digest는 원 결과, 다른 digest는 conflict
- chain 제출, canonical 관측, 업무 확정은 서로 다른 상태

## 채택 묶음

| ID | 범위 | 채택 상태 |
|---|---|---|
| UA-01 · 인증·MPC·보호 결과 | refresh generation, logout/unlink revocation, mobile approval capability, MPC epoch/quorum, typed protected result reader를 v1 기준으로 채택 | `adopted_design_baseline` |
| UA-02 · 상거래·영수증·단말 | order/recipient snapshot, exact payment allocation, fulfillment, refund, settlement correction, receipt ownership, terminal handover를 채택 | `adopted_design_baseline` |
| UA-03 · 대여·반납·기기 연속성 | enrollment holder, BLE/HTTP owner, reset journal, returned/cleanup/reuse gate와 RR-DEC-01 백업 확인을 채택 | `adopted_design_baseline` |
| UA-04 · 시장·자격·유료 자원 | quote/keeper, DID/status, restricted membership, x402 payment-entitlement-delivery-refund의 typed source를 채택 | `adopted_design_baseline` |
| UA-05 · 녹음·여행·개인정보 | audio owner/segment, transcript/AI provenance, consent/deletion generation, itinerary draft/apply, verified visit/review를 채택 | `adopted_design_baseline` |
| UA-06 · 설정 적용·관리 신뢰 | profile stage/approve/activate/apply, 독립 bootstrap, key rotation, restriction/recovery 및 O03/O04 reader를 채택 | `adopted_design_baseline` |
| UA-07 · 공유 저장·이벤트·화면 | event v1/v2, transaction+outbox, consumer checkpoint, single effect writer, unknown-result UI와 rebuild generation을 채택 | `adopted_design_baseline` |
| UA-08 · 통합 checkpoint | DF-20260920-01를 implementation-entry design baseline으로 채택하고 각 candidate는 preserve/change/add/hold disposition을 가진다 | `adopted_design_baseline` |

## 활성화 gate

각 묶음은 생성 schema, 후속 migration, 실제 profile, 배포 manifest와 runtime 수용 증거가 모두 통과하기 전까지 실행 비활성이다. 기존 migration을 수정하지 않고 expand→dual observation→activation→cleanup 순서로 새 migration을 추가한다.
