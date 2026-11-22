# 제품별 상용 설계와 요구사항 추적

이 설계는 NU-54V-DK를 직접 조립하는 소유자, 디스플레이를 맡는 모바일 앱, 가맹점 결제와 이를 뒷받침하는 서비스를 대상으로 한다. 표의 PRD는 사용자에게 보장할 결과와 기능별 인수 조건, 기획은 작업 순서와 책임·전환·미결정 사항, 유즈케이스는 시작 조건·정상·예외·종료·시험 증거, IA는 화면 또는 서비스 자원의 탐색·권한·상태를 정의한다.

기준 코드는 `4ca2e59`다. 기존 12주 계획에서 P01·02·04·05·06·07·10은 구현 범위였고 P03·08·09는 설계 전용이었다. 이 문서의 상용 기능은 구현·출시 목표다. 지갑의 6개 네트워크와 가맹점 데모의 StableNet 테스트넷(8283) 지정 TestUSDC 한 종은 서로 다른 상품 범위다. [12주 동결 기준](../../planning/design-freeze-checkpoint-02.md) · [기존 코드와 상용 설계 차이](../README.md) · [제품별 소스](../../../../products/README.md)

| 제품 | PRD | 기획 | Use Case | IA |
|---|---|---|---|---|
| P01 보드 펌웨어 | [요구사항](p01-device-firmware/prd.md) | [제공 순서](p01-device-firmware/plan.md) | [흐름·시험](p01-device-firmware/use-cases.md) | [구조·권한](p01-device-firmware/ia.md) |
| P02 소유자 모바일 앱 | [요구사항](p02-user-app/prd.md) | [제공 순서](p02-user-app/plan.md) | [흐름·시험](p02-user-app/use-cases.md) | [구조·권한](p02-user-app/ia.md) |
| P03 클라우드 MPC 지갑 | [요구사항](p03-cloud-mpc-wallet/prd.md) | [제공 순서](p03-cloud-mpc-wallet/plan.md) | [흐름·시험](p03-cloud-mpc-wallet/use-cases.md) | [구조·권한](p03-cloud-mpc-wallet/ia.md) |
| P04 가맹점 키오스크 | [요구사항](p04-merchant-kiosk/prd.md) | [제공 순서](p04-merchant-kiosk/plan.md) | [흐름·시험](p04-merchant-kiosk/use-cases.md) | [구조·권한](p04-merchant-kiosk/ia.md) |
| P05 가맹점 운영·정산 백오피스 | [요구사항](p05-operations-backoffice/prd.md) | [제공 순서](p05-operations-backoffice/plan.md) | [흐름·시험](p05-operations-backoffice/use-cases.md) | [구조·권한](p05-operations-backoffice/ia.md) |
| P06 StableNet 결제 계약 | [요구사항](p06-stablenet-contracts/prd.md) | [제공 순서](p06-stablenet-contracts/plan.md) | [흐름·시험](p06-stablenet-contracts/use-cases.md) | [구조·권한](p06-stablenet-contracts/ia.md) |
| P07 체인 인덱서·영수증 조회 | [요구사항](p07-indexer/prd.md) | [제공 순서](p07-indexer/plan.md) | [흐름·시험](p07-indexer/use-cases.md) | [구조·권한](p07-indexer/ia.md) |
| P08 테스트넷 시장 서비스 | [요구사항](p08-market-services/prd.md) | [제공 순서](p08-market-services/plan.md) | [흐름·시험](p08-market-services/use-cases.md) | [구조·권한](p08-market-services/ia.md) |
| P09 여행 탐색·여행 기록 | [요구사항](p09-travel-ai/prd.md) | [제공 순서](p09-travel-ai/plan.md) | [흐름·시험](p09-travel-ai/use-cases.md) | [구조·권한](p09-travel-ai/ia.md) |
| P10 공통 플랫폼·통신 프로토콜 | [요구사항](p10-platform/prd.md) | [제공 순서](p10-platform/plan.md) | [흐름·시험](p10-platform/use-cases.md) | [구조·권한](p10-platform/ia.md) |

각 제품의 데이터 쓰기 권한과 스키마는 [상용 구현 데이터 설계](../architecture/README.md)에 연결된다.

## 원 요구사항 12개와 구현 인수 경로

페이지 설계가 필요한 기능은 아래 제품 링크와 함께 [페이지별 UX 설계](../screens/book.html), [제품 기능·유즈케이스 대조표](../screens/coverage.md), [설계 감사와 원문 변경 내역](../screens/audit.md)에서 확인한다.

제품 문서의 R/UC 번호는 해당 제품 안에서만 유효하다. 아래 번호는 사용자의 최초 요구사항 번호다. 끝 열의 증거는 출시 전에 수집한다.

| 번호 | 달성해야 할 결과 | 제품별 시나리오 | 통합 인수에서 확인할 결과 |
|---|---|---|---|
| 1 | 시큐어 코딩·취약 의존성 패치 | [P10](p10-platform/use-cases.md) · [P01](p01-device-firmware/use-cases.md) · [언어별 검토와 패치](../security-and-release.md) | 직접·전이·벤더 의존성에서 영향 버전을 식별하고 수정본·재스캔·회귀 결과를 배포 버전에 연결 |
| 2 | 공개 보드 기반 개인 하드월렛 | [P01](p01-device-firmware/use-cases.md) · [P02](p02-user-app/use-cases.md) · [P10](p10-platform/use-cases.md) | 빈 보드 → 이미지 검증 → 소유자 설정 → 백업 확인과 독립 복원 → 첫 주소 대조 |
| 3 | 공개 설치·커스텀, 여섯 네트워크 및 지갑 필수 기능 | [P01](p01-device-firmware/use-cases.md) · [P02](p02-user-app/use-cases.md) · [P10](p10-platform/use-cases.md) · [체인별 기능표](../wallet-and-mobile.md#체인별-자산-지원-표) | 독립 빌드와 지원 조합별 주소·전송·수수료·거절·복원 증거. 지원하지 않는 호출은 닫힘 |
| 4 | 모바일 BLE 설정과 독립 공통 프로젝트 | [P01](p01-device-firmware/use-cases.md) · [P02](p02-user-app/use-cases.md) · [P10](p10-platform/use-cases.md) | P01/P02가 같은 버전 SDK를 가져와 등록 전용 신원으로 본딩하고 로컬 PIN을 설정 |
| 5 | 앱 설정·키오스크 결제 양쪽의 프로토콜 | [P10](p10-platform/use-cases.md) · [P04](p04-merchant-kiosk/use-cases.md) · [P01](p01-device-firmware/use-cases.md) | 관리·지급·근거리 세션의 권한 분리, 잘못된 역할 거절, 혼합 버전·중복/분할 시험 |
| 6 | 모바일 디스플레이, 직관적 UX·튜토리얼 | [P02](p02-user-app/use-cases.md) · [P01](p01-device-firmware/use-cases.md) | 초보 사용자의 첫 거래, 읽기 보조·큰 글씨·거절, 고위험 거래의 독립 표시 확인 |
| 7 | 코인·자산·테스트넷·체인 추가 설정 | [P02](p02-user-app/use-cases.md) · [P01](p01-device-firmware/use-cases.md) | 기존 엔진에 네트워크 등록 후 첫 거래 또는 불일치 거절. 지원 외 엔진은 펌웨어 지원 전 활성화 금지 |
| 8 | 알림과 매회 실물 승인에 의한 구독 결제 | [P02](p02-user-app/use-cases.md) · [P05](p05-operations-backoffice/use-cases.md) · [P01](p01-device-firmware/use-cases.md) | 회차 알림 중복, 앱 종료, 기한 만료, 해지 후 청구를 시험해 회차당 지급 최대 1건 |
| 9 | 지갑·DApp·여행 지도와 예약/디파짓/선주문 | [P02](p02-user-app/use-cases.md) · [P09](p09-travel-ai/use-cases.md) · [P05](p05-operations-backoffice/use-cases.md) | 웹 연결·로그인·지출 승인 분리. 지도 권한을 거절해도 식당/명소 검색과 예약금·잔금 처리 가능. 좌석 예약 없이 픽업 1건, 지도 장애 뒤 동일 주문 조회 |
| 10 | 암호화폐 전용 키오스크와 병렬 근거리 결제 | [P04](p04-merchant-kiosk/use-cases.md) · [P05](p05-operations-backoffice/use-cases.md) · [P06](p06-stablenet-contracts/use-cases.md) · [P07](p07-indexer/use-cases.md) | QR·BLE로 100건 동시 주문, 공용 화면을 5분 점유해도 다른 손님 주문 완료. 무한 대기·정상 DB 교착·중복 지급 0건 |
| 11 | 두 단말 모드, 메뉴·가격·원자적 직접 정산 | [P04](p04-merchant-kiosk/use-cases.md) · [P05](p05-operations-backoffice/use-cases.md) · [P06](p06-stablenet-contracts/use-cases.md) · [P07](p07-indexer/use-cases.md) | 정산 성공 시 등록 주소 가용 잔액이 정확히 증가. 거짓 성공 이체·동시 환불·마감 차이를 검증 |
| 12 | 구현 가능한 상용화 설계·시퀀스·유즈케이스 | 위 10개 제품의 PRD·기획·UC·IA · [전체 설계](../README.md) | 제품별 출발점·담당·실패 분기·완료 증거 및 제품 사이의 인계가 추적 가능 |

## 제품 간 인계와 판정 권한

| 인계 | 기록의 소유자 | 실패 시 다음 행동 |
|---|---|---|
| 기기 등록 → 지갑 생성 → 서명 | P01이 기기/지갑 키·물리 승인을 보유. P02가 내용을 표시하고 P10이 전송 규약 배포 | 본드 교체와 시드 복원을 구분한다. 고위험 요청에 신뢰 표시가 없으면 차단 |
| 탐색 → 예약·주문 → 지급할 견적 | P09가 장소를, P05가 예약·재고·가격·회차·발급 원장을 소유. P02/P04는 같은 주문 ID 사용 | 발급 전 취소 가능. 발급 뒤 이탈하면 기존 시도와 확정 체인 증거를 대사할 때까지 예약 유지 |
| 승인 → 지급 → 확정 → 이행 | P01이 서명, P06이 원자적 지급, P07이 확정 근거 제공, P05가 기록·이행 통지, P04가 픽업 표시 | 전송 응답 유실 시 조회. 확정 뒤 P05가 주문·회계·outbox를 함께 커밋. P04는 이행 eventId 기준으로 1회 접수 |
| v1 → v2, 환불·정산 | P05가 업무 주문별 계약 버전·환불 원장을 소유, P06이 버전별 재원·계약 제약 검증, P07이 양쪽 관측 | 기존 주문을 다른 계약으로 재발행하지 않는다. 환불은 별도 재원과 권한으로 처리 |
| 선택형 클라우드 지갑·시장 학습 | P03은 별도 지갑·참여자, P08은 테스트 시장·가격·keeper 운영 | 두 제품의 장애에도 보드의 매장 지급 경로는 가동. 클라우드 참여자가 보드의 소유권·승인을 대행할 수 없음 |
| 후기·추천·여행 기록 | P09는 자격과 동의만 필요한 범위에서 확인. P07은 체인 증거, P05는 예약·업무 원장 소유 | 여행 동의 철회 시 신규 처리를 차단하고 선택형 기록을 삭제. 지급 영수증과 각기 다른 결과를 표시 |

## 구현 착수와 출시 증거

구현 작업에는 제품 ID, 해당 PRD 기능, 유즈케이스, IA 진입점, 상대 제품, 실패 시 원장을 적는다. 예를 들어 ‘P04 결제 전용 QR’의 종료 조건은 P04 UC2의 지급자 선택, P05의 견적 저장, P06의 단일 지급, P07의 확정 영수증을 동일 주문에서 확인하는 것이다. 화면 시연만으로 정산을 인수할 수 없다.

독립 표시·복구 장치와 보드 보안, 체인별 허용 기능·확정성, 가맹점 재원·환불·주소 정책을 확정한 조합만 실제 자금 대상으로 검토한다. P03의 독립 복구 주체와 P08의 모의 시장 정책은 각 제품의 별도 출시 결정이다. [출시 게이트](../security-and-release.md) · [결정 책임과 통합 완료 기준](../README.md#범위-결정을-맡을-사람)

## 문서 검토 기록

세 분야로 나눠 제품 구현 진입점과 기존 상용 설계를 독립 대조했다. 지갑·프로토콜 검토에서는 폰 교체 시 소유자 권한 세대를 폐기하는 순서와 앱 경유 가맹점 견적 검증을 명확히 했다. 결제 검토에서는 데모와 개인 직접 지급의 재원, 디파짓·잔금의 개별 주문, 확정 후 조리 통지의 책임자를 명시했다. 여행·시장 검토에서는 무예약 픽업, 테스트 결제와 실구매 후기 자격 구분, AI 추천을 실제 일정에 적용하는 동의, 시장 계약 결과를 확정하는 시점을 보완했다.

각 제품의 PRD 기능은 같은 제품의 UC에서 참조한다. 검토 대상은 문서의 책임 경계·흐름·인수 조건이다. 보드 복원, 6개 네트워크의 서명, 동시 주문 부하, 등록 주소 실입금과 가맹점 일마감은 구현 후 별도 시험 증거가 필요하다.
