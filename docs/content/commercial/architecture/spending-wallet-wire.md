# 지출 지갑 공통 규격

주문 한 건을 앱·보드·키오스크·운영 서버·인덱서·운영툴이 함께 처리할 때 쓰는 약속이다. 모델은 [spending-wallet.md](spending-wallet.md), 행위자별 단계는 [spending-wallet-use-cases.md](spending-wallet-use-cases.md)에 있다. 이 문서의 값은 코드와 시험 벡터가 기준이며, 바꿀 때는 벡터와 세 언어의 시험을 함께 바꾼다.

## 1. 서명

| 무엇 | 누가 서명 | 만드는 곳 | 벡터 |
|---|---|---|---|
| 주문 잠금(EIP-3009 `ReceiveWithAuthorization`, nonce가 견적과 휴대전화 키를 묶음) | 보드 | `orderLock`(TS), `orderescrow.NewLock`(Go), `nu54_order.c` | `order-lock.vectors` |
| 전달 완료, 매장 취소 | 매장 견적 키 | `orderMerchantDigest`, `orderescrow.ActionDigest` | `order-actions.vectors` |
| 수령, 이의, 이의 취소(`claimableAt`을 포함) | 휴대전화 키 | `signOrderBuyerAction`, `orderescrow.ActionDigest` | `order-actions.vectors` |
| 주소 확인 | 보드 | `accountProofDigest`, `nu54_account_preimage` | `test_order.c`의 기지값 |
| 반납 | 보드 | `nu54_spending.c` | `wallet-return.vectors` |

모두 65바이트, low-s, v는 27 또는 28이다. 계약 바인딩은 `packages/contracts-abi`의 `OrderEscrow`와 `TestUSDC3009`를 쓴다.

## 2. 서버 요청

`packages/protocol/v2/service-contracts.json`이 기준이다. 서명은 봉투의 `approvalSignature`에 접두어 없는 소문자 hex로 넣는다.

| 요청 | 보내는 쪽 | 필드 | 뜻 |
|---|---|---|---|
| `quote.issue`(기존) | 키오스크, 고객 | 그대로 | escrow 매장이면 `verifyingContract`가 OrderEscrow다 |
| `order.lock` | 키오스크(기본), 고객 | `quoteDigest`, `confirmer`, `lockDigest` + 보드 서명 | 서버가 매장 가스로 `lock`을 제출한다. 같은 요청을 다시 보내면 같은 결과를 준다 |
| `order.answer` | 고객만 | `action`, `claimableAt` + 휴대전화 서명 | 전달 완료 뒤에만 받는다 |
| `order.status`(기존) | 키오스크, 고객 | 그대로 | escrow 주문이면 `order.escrow`가 있다 |

근거리(BLE) 메시지 `lock`은 `order.lock`과 같은 값을 나르고, 응답은 `forward`와 같은 `{submitted}`이다.

## 3. 주문 상태

`order.escrow`는 `OrderEscrowStatus`이고 `validOrderEscrowStatus`가 검사한다.

| `escrow.stage` | `order.state` | `payment.stage` | 언제 |
|---|---|---|---|
| `received` | `checking` | `received` | 서명을 저장했다 |
| `submitted` | `checking` | `relaySigned`, `submitted`, `checking` | 거래를 만들었거나 보냈다 |
| `locked` | `paid` | `finalized` | 잠금이 확정되었다. 주문번호(`pickupNo`)가 생기고 주방에 올라간다 |
| `delivered` | `paid` | `finalized` | 매장이 전달 완료를 눌렀다. `claimableAt`이 있다 |
| `disputed` | `paid` | `finalized` | `resolveBy`가 있다 |
| `paid` | `paid` | `finalized` | 매장이 대금을 받았다. `payout`이 있다 |
| `refunded` | `paid` | `finalized` | 돈이 보드 계정으로 돌아갔다. `refund`가 있다 |
| `failed` | `checking` 또는 `closed` | `checking` 또는 `unpaid` | 잠금이 더는 실행될 수 없다. 빠진 돈이 없다 |

- 확정되지 않은 거래로 주문번호를 만들지 않는다.
- 견적이 만료된 뒤 잠금이 없다는 확정 증거가 있을 때만 `failed`와 `closed`로 바꾼다.
- `pendingAction`은 서버가 받은 구매자 요청이 아직 체인에서 확인되지 않았다는 뜻이다.

## 4. 인덱서가 서버에 주는 사실

`chainview.escrow_receipt(p_stream uuid, p_merchant text, p_order text)`는 확정되고 현재 세대에 속한 사건만 `block_height`, `log_index` 순으로 돌려준다. 실행 권한은 `nu54_p07_reconcile`에만 준다. merchant와 order는 `0x`를 붙인 소문자 32바이트다.

| 열 | 형식 | 뜻 |
|---|---|---|
| `id` | uuid | 사건 식별자 |
| `kind` | text | `locked`, `delivered`, `disputed`, `released`, `cancelled`, `claim-skipped` |
| `chain_ref`, `contract_ref`, `abi_hash`, `policy_ref` | text | 배포 신원. 기존 `reconciliation_receipt`와 같은 규칙 |
| `payer_ref`, `beneficiary_ref`, `confirmer_ref` | text, 없으면 null | 소문자 주소 |
| `amount` | numeric, 없으면 null | |
| `quote_digest` | text, 없으면 null | `locked`만 |
| `deliver_by`, `confirm_window`, `claimable_at`, `resolve_by` | bigint, 없으면 null | 유닉스 초(`confirm_window`는 초 길이) |
| `reason` | smallint, 없으면 null | 계약의 `Payout`, `Refund`, `SkipReason` 값 그대로 |
| `tx_hash`, `inclusion_hash` | text | |
| `block_height` | bigint | |
| `log_index` | int | |
| `included_at` | timestamptz | |

견적 만료 뒤 잠금이 없다는 증거는 기존 `chainview.reconciliation_absence`로 준다. 조건은 확정 블록의 시각이 만료 시각 이상이고 그 블록에서 `orders(merchantId, orderId).state`가 0인 것이다. `reconciliation_ready`도 그대로 쓴다.

서버는 만료된 미결제 주문마다 `chainview.request_absence(p_stream uuid, p_merchant text, p_order text, p_expires bigint)`로 증거를 요청한다(권한 `nu54_p07_reconcile`, 같은 요청은 한 번만 남는다). 인덱서 프로세스가 요청을 처리한다. 사람이 `prove-absence`를 실행하지 않아도 주문이 닫힌다.

## 5. 배포 설정

옛 결제 계약(SettlementV2)과 OrderEscrow는 매장 단위로 고른다. 매장의 배포 프로필에 적힌 ABI hash가 OrderEscrow 것이면 새 경로, SettlementV2 것이면 옛 경로다. 한 매장은 한 경로만 쓴다. 옛 경로의 동작은 바꾸지 않는다. 앱과 서버는 같은 이름 `abiSha256`을 쓰고, 값이 없으면 SettlementV2다. 앱이 모르는 값이면 결제를 거절한다(`paysThroughEscrow`).

## 6. 주문 이후

| 일 | 누가 시작 | 새 요청 | 체인 호출 |
|---|---|---|---|
| 준비 완료 | 매장 직원 | 없음(기존 픽업 `ready`) | 없음 |
| 전달 완료 | 매장 직원 | 없음(기존 픽업 `collected`) | 서버가 매장 견적 키로 서명해 `markDelivered` |
| 이행 불가 | 매장 직원 | 없음(기존 픽업 `unfulfillable`) | 서버가 매장 견적 키로 서명해 `cancel` |
| 수령, 이의, 이의 취소 | 사용자 앱 | `order.answer` | 서버가 휴대전화 서명으로 `release`, `dispute`, `withdrawDispute` |
| 미전달 환불, 이의 기한 환불 | 서버(기한이 지나면 자동) | 없음 | `refundUndelivered`, `refundExpiredDispute` |
| 일괄 청구 | 재무 담당 | 아래 | `claimBatch` |
| 이의 판정 | 재무 담당 둘 | 아래 | `resolve`(운영자 계정이 보냄) |

- 가스는 모두 서버 계정이 낸다. `resolve`만 이의 판정 계정이 보내며, 이 계정은 가스 계정과 다르다.
- 픽업 `collected`가 기록되면 전달 완료로 본다. 체인에서 확정되기 전까지 `escrow.stage`는 `locked`이다.
- `order.answer`는 서버가 서명을 확인해 저장한 뒤 `resultRef`를 준다. 확정 전에는 `pendingAction`으로 보인다. 같은 요청을 다시 보내면 같은 결과이고, 다른 요청은 `REPLAY_CONFLICT`이다. 이미 정해진 주문이면 `CHECKOUT_CONFLICT`이다.
- 이의가 이긴 경쟁(수령과 이의가 함께 온 경우)은 체인에서 먼저 확정된 쪽을 따른다.
- 픽업 대기열 응답은 `accepted`, `preparing`, `ready`, `collected` 주문에 `items`를 함께 준다(형식은 `readPickupKitchen`이 이미 허용한다). escrow 주문의 대기열과 상세 행에는 `escrow: {stage}`(`locked`, `delivered`, `disputed`, `paid`, `refunded`)가 있다. 이 값이 있으면 새 환불 안내를, 없으면 옛 환불 안내를 보인다.

## 7. 재무 요청

기존 `POST /v2/finance/{operation}`의 인증, Origin, 오류 형식을 그대로 쓴다. 본문은 `branchId`가 필수이고, 쓰기 요청은 `idempotencyKey`가 필수이다. 시각은 유닉스 초, 금액은 기준 단위 문자열이다.

| operation | 권한 | 본문 | 응답 |
|---|---|---|---|
| `escrow-orders` | `report_read` | `status`, `cursor` | `{items: EscrowItem[], totals: {claimable, waiting, disputed, held}, receiptScope, nextCursor}` |
| `escrow-claim` | `escrow_claim` | `saleIds`(1~100, 중복 없음), `idempotencyKey` | `ClaimBatch` |
| `escrow-claims` | `report_read` | `cursor` | `{items: ClaimBatch[], nextCursor}` |
| `escrow-resolve` | `escrow_dispute_request` | `saleId`, `outcome`, `reason`, `idempotencyKey` | `DisputeDecision` |
| `escrow-resolve-approve` | `escrow_dispute_approve` | `decisionId`, `idempotencyKey` | `DisputeDecision` |
| `escrow-resolve-reject` | `escrow_dispute_approve` | `decisionId`, `idempotencyKey` | `DisputeDecision` |

```ts
type EscrowItem = { saleId: string; pickupNo?: number; amount: string; asset: string;
  status: "held" | "waiting" | "claimable" | "claiming" | "disputed" | "paid" | "refunded";
  lockedAt: number; deliverBy: number; claimableAt?: number; resolveBy?: number;
  payout?: string; refund?: string; transaction?: string;
  deliveryFailed?: true; // the delivered mark can no longer execute; the buyer is refunded after deliverBy
  cancelRefused?: true; // the token refuses the store's refund; the server keeps trying less often
  items?: { label: string; option: string; quantity: number }[]; // disputed orders only
  decision?: DisputeDecision };
type ClaimBatch = { batchId: string; state: "queued" | "submitted" | "final" | "failed"; createdAt: number;
  orders: number; requested: string; paid?: string; transaction?: string;
  receiptScope: string; // same meaning as in the other finance replies: whose receipt the key belongs to
  skipped: { saleId: string; pickupNo?: number; reason: "in-another-batch" | "not-delivered" | "window-open" | "disputed" | "settled" | "other-beneficiary" | "unknown" }[] };
type DisputeDecision = { decisionId: string; saleId: string; outcome: "pay-merchant" | "refund-buyer"; reason: string;
  state: "proposed" | "approved" | "submitted" | "final" | "rejected" | "failed";
  canApprove: boolean; transaction?: string; resolveBy: number };
```

- 기존 `order`는 escrow 주문이면 `escrow: EscrowItem`을 준다. `receiptScope`는 읽은 계정과 매장이 같으면 모든 응답에서 같다.
- `status`는 `held`(전달 전), `waiting`(확인 기간 중), `claimable`, `claiming`, `disputed`, `paid`, `refunded`, 또는 빈 값(전체)이다.
- 청구 묶음 하나는 100건까지다. 더 많으면 화면이 나눠 보낸다. 건너뛴 주문은 묶음을 멈추지 않는다. 다른 묶음에 이미 들어간 주문은 `in-another-batch`로 건너뛰고, 남는 주문이 하나도 없을 때만 409를 준다.
- 판정은 신청한 사람이 승인할 수 없다. `reason`은 `not_delivered`, `delivered_proof`, `wrong_item`, `goodwill` 중 하나다.
- 이의 기한(`resolveBy`)이 지나면 판정을 받지 않고 자동 환불한다.
- 잠긴 돈은 매출이 아니다. `released`가 확정된 날 매출로, `cancelled`가 확정된 날 예치 반환으로 기록한다.

## 8. 보드 설정, 반납, 초기화

| 요청 | 보내는 쪽 | 필드 | 뜻 |
|---|---|---|---|
| `payer.enroll` | 고객 | `payer`, `generation` + 보드 주소 확인 서명(0x79) | 이 고객이 이 매장에서 쓸 보드 계정을 등록하거나 바꾼다 |
| `payer.status` | 고객 | 없음 | 응답에 `payer: PayerEnrollment`(없으면 생략) |
| `wallet.return` | 고객 | `payer`, `to`, `amount`, `validBefore`, `nonce` + 보드 반납 서명(0x7b) | 서버가 가스를 내고 `transferWithAuthorization`을 보낸다 |
| `wallet.return-status` | 고객 | 없음 | 응답에 `walletReturn: WalletReturn`(없으면 생략). 가장 최근 것 하나 |

- 대상(`objectRef`)은 고객 자신이다. 서명은 `approvalSignature`에 넣는다.
- `payer.enroll`의 보드 challenge는 `payerEnrollChallenge`이고 요청마다 다르다. 서버는 `AccountProofDigest`로 주소를 복원해 `payer`와 같을 때만 등록한다. 결제 중이거나 끝나지 않은 주문이 있으면 다른 계정으로 바꾸지 못한다(`CHECKOUT_CONFLICT`).
- `wallet.return`은 등록된 `payer`만 받는다. `validBefore`는 지금부터 600초 이내다. 그 계정에 끝나지 않은 주문(받음, 제출, 잠김, 전달, 이의)이 있으면 `CHECKOUT_CONFLICT`이다. 하루 횟수와 동시 진행을 한 건으로 제한한다.
- `final`은 거래가 최종 블록에 성공으로 들어갔을 때만이다. 만료 뒤에도 실행되지 않았음이 확실하면 `failed`다.
- 오류: 등록되지 않은 계정은 `MISMATCHED_EVIDENCE`, 진행 중인 반납이 있으면 `REPLAY_CONFLICT`, 하루 세 번을 넘으면 `BUSY`, 기한이 600초를 넘으면 `BAD_REQUEST`, 가스 대납이 없으면 `UNSUPPORTED_CAPABILITY`. 네 요청은 escrow 매장에서만 된다.
- 앱은 `final`을 확인한 뒤에만 보드 초기화(0x7c)를 보낸다.
- 운영자 초기화는 서버를 거치지 않는다. 앱이 보드 식별값, 세대, 일회용 값을 QR과 글자로 보여 주고, 운영자가 저장소 밖의 발급 키로 `enroll_board.py`에서 서명해 돌려준다.
- 보드 허가(부팅마다 필요)는 지금처럼 운영자가 `enroll_board.py`로 넣는다. 앱은 허가가 없는 상태를 알려 준다.
