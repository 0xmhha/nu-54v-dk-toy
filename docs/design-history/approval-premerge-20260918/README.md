# NU-54V DK — 생활형 제품 기획

**현재 단계: 상세 설계. 구현은 사용자 지시에 따라 보류한다.** 일반적인 “다음 진행”은 설계의 연속이며, 제품 코드 작성·개발환경 설치·배포 착수를 뜻하지 않는다. 기존 구현 순서 문서는 향후 실행을 위한 계획이다.

- [현재 설계 통합 현황: 명세 차이 5곳·반영 묶음 8개·전체 결정 19개](content/planning/design-integration-register.md)
- [HW·Cloud 지갑: 선택·승인·MPC 통제·분실/반납 복구 설계](content/specifications/wallet-control-recovery-design.md)
- [지갑 API 계약 후보: 선택·승인 증거·제출·응답 유실 복구](content/specifications/wallet-api-contracts.md)
- [승인·제출 저장 경계: 전송 허가·원천 연결·환불 보정·장애 복구](content/specifications/signing-submission-storage-design.md)
- [키오스크 결제·점주 환불: 승인 문맥·BLE 증거·제한된 결과 조회](content/specifications/source-approval-contracts.md)
- [승인 계약 병합 계획: 개인·키오스크·환불의 버전·권한·반영 범위](content/specifications/approval-contract-merge-plan.md)
- [공통 승인 스키마·권한 자격: 발급·전달·교체·철회·결과 복구](content/specifications/approval-access-contracts.md)
- [승인 HTTP·권한·화면·저장 연결: 기존 API 10개와 권한 경로 3개](content/specifications/approval-integration-contracts.md)
- [승인 기준 반영 순서·호환 수용표: 6단계와 24개 검토 조합](content/specifications/approval-adoption-plan.md)
- [결제·환불·반납의 책임, 상태 전이, 실패 복구](content/specifications/commerce-lifecycle-design.md)
- [결제·환불 계약 변경안: API 5개·이벤트 2개·논리 저장 계약 4개](content/specifications/commerce-contract-changes.md)
- [반납 복구 설계: 초기화 증거·복구 권한·재대여 조건·늦은 자산](content/specifications/return-recovery-design.md)
- [반납 프로토콜 후보: 요청·응답·권한 타입·ACK 이후 정리 증명](content/specifications/return-protocol-contracts.md)
- [반납 화면 설계: 앱·기기·운영자 상태, 버튼, 재연결 안내](content/specifications/return-screen-design.md)

현재 12주 목표는 **NU-54V-DK 기기·지갑·매장·여행·StableNet 테스트넷 서비스**의 15개 완료 항목입니다. 실제 기기 지갑/FOTA/패스키/녹음/찾기/스탬프, 소셜 로그인·MPC Cloud Wallet, 매장·백오피스, 온체인 계약군·DEX·Indexer, 여행 추천을 포함합니다. 모든 항목을 앱 연동까지 완료하며 참여 인원은 3명입니다. 아래 v3 문서가 최신 범위이며, 현재는 역할·공수 배정 전에 상세 작업 목록과 의존성을 정리하는 단계입니다.

- [최신 12주 완료 범위 v3: 15개 요구사항·수용 기준](content/twelve-week-completion-scope-v3.md)
- [현재 작업: 상세 WBS·산출물·선행 조건·완료 기준](content/work-breakdown-plan.md)
- [기능 실행 명세: 화면·데이터·상태·19개 종단 시나리오](content/planning/functional-execution-spec.md)
- [API·BLE·데이터 연결 명세: 요청·권한·복구·검증 예제](content/specifications/interface-contracts.md)
- [화면별 입력·상태·복구 흐름](content/specifications/screen-flows.md) · [주문·결제·환불·반납 상세 DTO](content/specifications/critical-dtos.md)
- [나머지 99개 API·프로토콜 경계](content/specifications/extended-contracts.md) · [저장·동시성·삭제/복구 작업](content/specifications/storage-operations.md)
- [데이터베이스 참조 DDL: 61개 테이블·7단계 마이그레이션·검증](content/specifications/database/README.md)
- [API 권한·트랜잭션: 107개 접근 매핑·57개 정책·28개 검증 시나리오](content/specifications/access-transactions.md)
- [보안 연결 상세 설계: 인증·임시 권한·신뢰 registry·영수증·보호 결과](content/specifications/security-integration-design.md)
- [보안 API 통합 결과: API-103~107·영수증/혜택·보호 스트림](content/specifications/security-api-integration.md)
- [기술 선택안 19개·검증 작업 23개: 프로토콜·라이브러리·저장 방식](content/planning/technology-selection.md)
- [Zephyr 확정·구현 인터페이스 14개·호환 판정 26개](content/specifications/implementation-interfaces.md)
- [구현 순서·104개 착수 조건·320개 산출물 증거 연결](content/planning/implementation-sequence.md) · [작업별 증거 카드](content/planning/execution-readiness.md)
- [320개 세부 작업 CSV](content/planning/implementation-steps.csv)
- [전체 작업 카드](content/planning/work-cards.md) · [CSV](content/planning/work-breakdown.csv) · [결정 목록](content/planning/decisions.md)
- [보류한 역할·주차 배치 초안](content/three-person-delivery-plan.md)
- [확장 범위의 기존 코드 재사용·미완성 항목](content/expanded-scope-reuse-inventory.md)
- [기존 요구사항·설계 기록: 지갑과 매장 운영 플랫폼](content/payment-platform-scope-v2.md)
- [첫 결제 시연: 통합 작업·완료 기준·12주 배치 초안](content/payment-integration-workplan.md)
- [확정 네트워크: StableNet 8283·네이티브 가스·RPC 확인 기록](content/stablenet-testnet-baseline.md)
- [소프트웨어 설계 초안: 화면·권한·주문·결제·Indexer](content/software-product-blueprint.md)
- [기존 Indexer 코드 검토·연동 방식·더미 토큰 설계](content/indexer-integration-and-test-token.md)
- [stable-poc-contract 재사용: 테스트 토큰·스마트 계정·가스비 후원](content/stable-contract-reuse-plan.md)
- [poc-platform 재사용: 웹·지갑 SDK·Bundler·Paymaster와 추가 개발 범위](content/poc-platform-reuse-plan.md)
- [S25 Ultra 외부 거리 측정 모듈: 부품·USB 연결·시험 계획](content/ranging-module-validation-plan.md)

- [최신 Medium 원고: 코인 결제 지갑](content/medium-week-01-payment-wallet.md)
- [최신 Medium 원고 HTML 미리보기](content/medium-week-01-payment-wallet.html)
- [메이커 작성 기준·발행 체크리스트](content/payment-wallet-maker-checklist.md)

- [내부 검토용 상세 기획: 근접 확인형 코인 결제 지갑·키오스크](content/proximity-wallet-kiosk.md)

- [최신 사용 목적 반영: AI 음성·기록·책상 시계와 게임 도구](content/daily-tools-shortlist.md)

- [사양 중심 독립 검토: 가장 적합한 분야와 제품](content/board-fit-research.md)

- [최신: 인테리어 전자 소품·모션 장갑 검토와 이미지](content/interior-and-motion-glove.md)

- [제품 이미지 갤러리 (HTML)](content/concept-gallery.html)
- [제품 이미지 갤러리 (Markdown)](content/concept-gallery.md)
- [드론 활용 가능성·SCOUT-01 기획](content/drone-feasibility.md)
- [현재: 게임·피규어·생활 속 놀이 제품 제안](content/product-directions-v3.md)
- [이전 후보 비교](content/product-directions-v2.md)

이전 Air Cue 기획은 사용자 피드백에 따라 철회했습니다. 아래 파일은 이전 방향의 기록입니다.

- [Medium 원고](content/medium-week-01.md)
- [Medium 원고 HTML 미리보기](content/medium-week-01.html)
- [콘텐츠 기준·12주 실행계획·발행 체크리스트](content/project-plan.md)
- [이미지 출처 및 생성 프롬프트](content/assets/README.md)

[Medium 초안](https://medium.com/p/cb196bcb3de3/edit)에 본문과 이미지 2장을 저장했습니다. 공개 발행 및 대시보드 제출은 미완료입니다. [작업 상태](content/medium-draft-status.md)
