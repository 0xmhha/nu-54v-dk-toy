# P09 · Travel and AI

> **DF-20260925-02 기준 (2026-09-25):** 이 제품은 이번 12주 사이클에서 설계만 하고 구현하지 않는다(out of cycle). [설계 동결 DF-20260925-02](../../docs/content/planning/design-freeze-checkpoint-02.md) 참조. 아래 원문은 DF-20260920-01 기준이다.

실제 결제와 사용자 동의를 근거로 여행 기록과 추천을 만드는 제품이다.

- **소유 범위:** 장소 검색, 결제 기반 후기, 위치 발자취, 음성 전사·요약,
  AI 코스, 따라하기 챌린지, 스탬프·혜택, 동의·보관·삭제
- **주요 경계:** P02와 C8, P07과 C7, 외부 provider와 C9
- **WBS:** `WBS-P09-01`부터 `WBS-P09-05`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p09--여행기록추천혜택)
- [녹음·여행·AI 설계](../../docs/content/specifications/recording-travel-ai-design.md)
- [자격과 유료 AI 코스](../../docs/content/specifications/credential-paid-resource-design.md)
- [데이터와 화면 흐름](../../docs/content/specifications/screen-flows.md)

구현 시 위치, 오디오, transcript, 후기의 동의와 삭제 경계를 각각 시험할 수
있게 service와 worker를 분리한다.

## 상용 로컬 구현

장소 공개 카탈로그와 예약 보류 연동은 [장소와 예약 경계](BOOKING-V1.md)에
설치 및 재현 명령을 정리했다. 지역을 직접 입력해 P09 공개 장소를 조회할
수 있으며, 예약이 가능한 장소는 등록된 고객 자격으로 P05 재고를 조회한다.
