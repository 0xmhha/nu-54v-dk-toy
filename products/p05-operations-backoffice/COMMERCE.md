# 중앙 주문과 견적 · 07

`opsd`는 `P05_COMMERCE_DATABASE_URL`이 설정된 배치에서 TLS 1.3과 서로 인증한 클라이언트 인증서를 요구한다. 기본 상태에서는 기존 health probe를 제공한다. v1 대여와 반납 CLI는 기존 진입점에서 동작한다. [C3–C9 계약](../../packages/protocol/v2/SERVICE.md)

## 배치 입력

| 환경 변수 | 역할 |
|---|---|
| `P05_MIGRATION_DATABASE_URL` | 관리 연결. `make migrate`가 P05 001–003과 필드 목적 정책을 전진 적용한다. |
| `P05_COMMERCE_DATABASE_URL` | 별도 런타임 연결. `nu54_p05_commerce`가 허용한 함수만 실행하고 원장 테이블을 직접 읽거나 쓰면 시작이 거절된다. |
| `P05_COMMERCE_CA`, `P05_COMMERCE_CERT`, `P05_COMMERCE_KEY` | 클라이언트 CA와 서버 TLS 자격 파일. |
| `P05_COMMERCE_GRANTS` | 인증서 DER SHA-256을 `tenantRef`·`subjectRef`·`role`·`expiresAt`의 선택된 인증 주체로 매핑한다. 각 요청의 지점·객체·명령 grant는 PostgreSQL에서 재확인한다. |
| `P05_COMMERCE_PROFILES` | 지점 ID별 `chainId`, `contract`, `asset`, `beneficiary`, `signer`, `merchantId`, `keyVersion`, `beneficiaryVersion`의 운영 등록이다. `ledger.issuance_profile.authorized_digest`는 동일 검증 등록의 Go `Profile.Digest()`를 기록한다. |
| `P05_QUOTE_COMMITMENT_KEY_FILE`, `P05_QUOTE_STORAGE_KEY_FILE` | 두 개의 독립 32바이트 암호 키 파일. 각각 quote commitment와 저장 암호문에 사용한다. |
| `P05_QUOTE_SIGNER`, `P05_QUOTE_SIGNER_SHA256` | 승인된 독립 실행기의 절대 경로와 SHA-256. 읽어서 해시를 검증한 실행 바이트를 별도 실행 가능한 임시 경로에서 실행한다. stdin의 32바이트 digest hex에 대해 65바이트 canonical r/s/v hex를 반환한다. 역할 키와 방송 권한은 분리한다. |

운영 등록자가 `ledger.commerce_grant`, `ledger.payer_enrollment`, `ledger.menu_release`, `ledger.price`, `ledger.stock`과 지점별 `ledger.command_rule`을 관리한다. 현재 매장 데이터, 실제 인증서, 서명기, 체인 배포 주소는 이 변경에서 등록하지 않았다. 발행된 견적은 원 발행자의 서명 키 지문에 해당하는 주소와 원본 digest·암호문을 함께 유지한다. 정상 수령 주소·키 전환 뒤 같은 주장과 유효한 지급자 권한을 가진 요청은 P06의 발행 시점 근거와 동일한 바이트를 조회한다. 긴급 신규 발행 중지는 미발행 견적·신규 서명에 적용된다. 기존 근거 조회와 체인의 긴급 서명 철회 판정은 별도다.

## 호출 계약

`POST /v2/commerce`는 JSON의 `command`에 [서비스 v2 요청 봉투](../../packages/protocol/v2/service-contracts.json)를 받는다. 주문에는 `cart`도 필요하다. 응답은 동일 요청의 `requestId`·`requestDigest`·`resultRef`를 결합하고 필요한 경우 `catalog`, `order`, `claim`, `quote`를 추가한다. 타 지점 또는 권한 없는 객체는 404로 숨긴다.

| 명령 | 범위와 동작 |
|---|---|
| `catalog.read` | `domainRef`와 `objectRef`는 동일 지점. 승인된 현재 메뉴 버전을 SKU/옵션 순으로 페이지 조회한다. |
| `order.create` | `objectRef`는 앱/POS가 생성한 원천 주문 UUID. `cart`의 `origin`, `originRef`, `menuVersion`, `items` 순서로 JSON을 직렬화한다. 항목은 SKU 순으로 정렬한 `{sku, option_key, quantity}`이며 같은 SKU는 한 번만 등장한다. `body.intentDigest`는 이 정규 JSON의 SHA-256 lowercase hex, `body.quantity`는 합계의 십진 문자열이다. 서버가 게시 가격을 읽고 재고·주문·요청 영수증을 한 트랜잭션에 기록한다. |
| `order.list` | `objectRef`는 인증된 고객 자신, `domainRef`는 현재 등록 지점이다. 첫 페이지에는 `afterSale`로 고객 UUID를 보내고 이후 마지막 sale UUID를 보낸다. 기존 주문 상세 권한을 재사용해 매장 메뉴·POS의 UUID, 상태, 금액, 자산을 한 번에 최대 50건 반환한다. 비교 클레임으로만 읽는 주문에는 만료 시각을 전달해 늦게 도착한 페이지를 폐기한다. 구독·여행은 각각 기존 원장 조회를 사용한다. |
| `order.status` | `objectRef`는 반환된 불투명 sale UUID. 원 주문 생성자 또는 현재 같은 sale에 승인된 고객이 조회한다. 현재 주문의 인증된 지급자에게만 선택적인 `payment`을 반환한다. |
| `checkout.authorize` | 원 주문 생성자는 `customerRef`의 활성 지급자/세대를 대조하고 요청 영수증과 32바이트 private claim을 받는다. 고객이 만든 메뉴 주문은 자기 주체에만 연다. 단말은 사적으로 인증한 배송 경로로 해당 고객에게 넘긴다. 주문 번호나 공개 QR만으로는 해당 claim 또는 소유 권한이 생기지 않는다. |
| `quote.issue` | 승인된 고객 또는 원 주문의 인증 단말이 동일 `objectRef`, `claimDigest`, `orderRevision`으로 호출한다. 먼저 준비를 잡은 동일 지급자/claim만 견적을 발행한다. 같은 지급자/claim에서 주문 채널이 바뀌어도 동일한 암호화 레코드와 견적 바이트를 돌려준다. 단말이 발행/중계할 수 있는 것은 가맹점 서명 견적이며 지급자 승인은 별도 보드 또는 승인 경로에서 발급된다. |

서로 다른 요청 의도에 멱등 키를 재사용하면 충돌한다. 단말과 폰의 동시 호출은 첫 요청의 결과를 기다리거나 같은 intent로 결과를 재조회한다. 서명 전 준비 트랜잭션은 종료된다. 반환 후에는 현재 지급자·장부 revision·profile 및 검증된 서명·lease fence를 다시 확인하고, 상태·암호문·이벤트를 하나의 트랜잭션에 커밋한다. 요청 종료는 결과 불명으로 처리하고 동일 키로 조회한다. 소진 상태나 만료 견적은 반환하지 않는다. 만료된 미발급 주문만 재고를 되돌리고 발행 주문 재고는 후속 C7 대사에 남긴다.

후속 P06/P07/P05 대사는 브랜치의 내부 sale UUID에 독립적으로 발급한 chain order ID와 merchant ID, `quoteDigest`, `payerGeneration`, 자산 및 exact amount를 연결한다. 지급 체인 제출·출고·환불/전표 효과는 08–11 인수 항목이다.

## 고객용 원지급 조회

[051 조회 함수](internal/store/migrations/051_customer_order_receipt.sql)는 기존 `order.status`와 같은 주문·클레임 권한을 사용한다. 현재 클레임으로 주문을 조회한 다른 고객은 미확정 주문의 기존 상태만 확인할 수 있다. 지급 확정 뒤에는 원지급자 또는 원 주문을 생성한 고객만 영수증을 볼 수 있다. 단말의 기존 접근 경계와 응답 형식을 유지한다. 현재 지점의 인증된 원지급자에게는 다음 증거를 원 sale UUID로 읽는다.

| 상태 | 기존 근거 | 고객에게 전달하는 데이터 |
|---|---|---|
| 요청 접수 | `settlement_submission` | 체인과 접수 상태 |
| 릴레이 서명·네트워크 제출·결과 확인 | `settlement_work`의 원지급 작업과 `relay_attempt` | 체인, 서명/제출/조회 단계, 고정된 원 릴레이 해시 |
| 지급 확정 | 같은 sale의 `payment`, 전표, 영수증 및 픽업 | 확정 거래 해시, 체인 포함 시각, 기존 영수증과 픽업 번호. 픽업 준비·수령 상태는 실제 이행 기록으로만 표시 |
| 미지급 확정 | 서명된 견적에 대한 `sale_resolution` | 최종 미지급과 기존 시도 해시. 영수증과 픽업은 없음 |
| 구형 주문 또는 조회권이 다른 주문 | 기존 주문 상태 | 권한 범위에 따른 원래 주문 상세 |

이 조회는 별도 테이블에 복제하지 않는다. 발급 키·승인 서명·암호문·릴레이 송신자와 nonce는 고객 응답에 포함하지 않는다. 본문을 푸시, 운영 로그 또는 제3자 조회로 보내지 않는다. 원거래 보존과 접근 제한은 기존 원장 정책을 따른다. 고객 요청을 접수했거나 릴레이 해시가 보인 상태에서는 상태 조회만으로 추가 지급 요청을 보내지 않는다.

픽업 영수증의 `includedAt`은 정본 블록 시각이고, `finalizedAt`과 `pickupReadyAt`은 P05 전표 기록과 처음 준비된 시각이다. 체인과 P05의 시계 차이가 있어도 같은 주문과 증거로 결속된 최종 전표를 표시한다. 준비 시각은 같은 원장에 기록된 전표 시각 이상이어야 한다. 과거 전이에서 최초 준비 시각을 입증할 수 없으면 생략한다.

## 검증

`P05_COMMERCE_TEST_DATABASE_URL`에는 로컬 임시 Unix socket을 사용하는 이름이 `nu54_commerce_`로 시작하는 새 DB를 지정한다. test login과 synthetic branch · price · payer · claim을 로컬에서만 준비한다. 요청 본문은 승인 자격을 포함하지 않는다.

```sh
P05_MIGRATION_DATABASE_URL="$SCRATCH_DSN" make -C products/p05-operations-backoffice migrate
P05_COMMERCE_TEST_DATABASE_URL="$SCRATCH_DSN" make -C products/p05-operations-backoffice commerce-test
```

실제 PostgreSQL 역할 및 mTLS loopback에서 마지막 재고 한 단위 경합, 같은 POS 주문, 반대 지급자·타 지점·일반 주문 번호 거절, 서명 대기 중 NOWAIT 잠금, 장애 뒤 동일 nonce/fence, 암호문·outbox 단일 커밋, 단말·폰 견적 동일성 및 미발급 재고 복구를 부하와 장애의 작은 합성 시험으로 재현한다.
