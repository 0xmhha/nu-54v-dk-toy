# P03 · Cloud MPC wallet

> **DF-20260925-02 기준 (2026-09-25):** 이 제품은 이번 12주 사이클에서 설계만 하고 구현하지 않는다(out of cycle). [설계 동결 DF-20260925-02](../../docs/content/planning/design-freeze-checkpoint-02.md) 참조. 아래 원문은 DF-20260920-01 기준이다.

소셜 계정과 연결되는 2-of-3 Cloud Wallet 제품 경계다.

- **소유 범위:** 분산 키 생성, 계정 연결, 별도 모바일 승인, threshold 서명,
  참여자 격리, share refresh와 복구
- **주요 경계:** P02와 C4, P06과 C5, P10의 인증·감사 계약
- **WBS:** `WBS-P03-01`부터 `WBS-P03-04`

설계 진입점:

- [제품 WBS](../../docs/content/planning/product-wbs-overview.md#p03--cloud-mpc-wallet)
- [소셜 지갑 복구 설계](../../docs/content/specifications/social-wallet-recovery-design.md)
- [지갑 통제와 복구](../../docs/content/specifications/wallet-control-recovery-design.md)
- [보안 연결 설계](../../docs/content/specifications/security-integration-design.md)

구현 시 참여자별 배포 단위와 저장소를 분리하고, 완전한 private key를 한
영역에서 재구성하지 않는 수용 시험을 이 폴더에 둔다.
