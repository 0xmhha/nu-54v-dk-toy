import { useRef, useState } from "react";
import {
  FinanceRequestError,
  financeRequest,
  formatFinanceAmount,
} from "./Finance";
import type { BranchToken } from "./money";

type Adjustment = {
  id: string;
  amount: string;
  reason?: string;
  rejectionCode?: string;
  reviewRevision: number;
  previousFailure?: string;
  state: string;
  canDecide: boolean;
  canSubmit: boolean;
  submitted: boolean;
  transaction?: string;
  pendingTransaction?: string;
  confirmedAt?: string;
  businessDay?: string;
};
type Review = {
  receiptScope: string;
  paid: boolean;
  destination: string;
  canManageFunds: boolean;
  orderBusinessDay?: string;
  canRequest: boolean;
  principal: string;
  available: string;
  reserved: string;
  inFlight: string;
  refunded: string;
  funds: string;
  fundsAvailable?: string;
  refunds: Adjustment[];
  // Present only for a branch that names its token.
  token?: BranchToken;
};
type QueueItem = Pick<Adjustment, "state" | "amount"> & {
  refundId: string;
  saleId: string;
  principal: string;
  next: string;
};
type QueuePage = {
  items: QueueItem[];
  nextCursor: string | null;
  token?: BranchToken;
};

const nextSteps: Record<string, string> = {
  decide: "승인 또는 반려 검토 가능",
  requester_cannot_decide: "이 환불을 신청한 계정은 승인·반려할 수 없습니다.",
  approval_role_required:
    "이 지점의 승인 권한을 가진 별도 담당자에게 전달하세요.",
  independent_treasury_required:
    "신청자와 승인자 외의 재무 담당자가 제출할 수 있습니다.",
  treasury_role_required:
    "이 지점의 재무 제출 권한을 가진 담당자에게 전달하세요.",
  review_funds: "재원 및 제출 검토 가능",
  await_prior: "기존 환불 처리 결과 확인",
  await_chain: "같은 환불의 전송·확정 조회",
  read_result: "환불 처리 결과 조회",
};
type Draft = { amount: string; reason: string; idempotencyKey: string };

const reasons: Record<string, string> = {
  partial_service: "상품 또는 서비스 일부 환불",
  merchant_cancel: "매장 취소",
  duplicate_charge: "중복 지급",
};
const rejectionReasons: Record<string, string> = {
  route_review: "목적지 검증 자료 필요",
  amount_review: "금액 또는 남은 한도 재확인",
  policy_review: "환불 정책 재확인",
};
const states: Record<string, string> = {
  requested: "별도 승인 대기",
  approved: "승인 기록 · 재원과 전송 확인 대기",
  inflight: "기존 거래의 방송·확정 확인 중",
  final: "환불 확정",
  cancelled: "거절 · 예약 원금 해제",
};
const uuid = /^[0-9a-f-]{36}$/;

function toUnits(amount: string) {
  if (!/^(0|[1-9][0-9]{0,34})(\.[0-9]{1,6})?$/.test(amount)) return undefined;
  const [whole, fraction = ""] = amount.split(".");
  const value = BigInt(whole + fraction.padEnd(6, "0"));
  return value > 0n ? value.toString() : undefined;
}

export function RefundConsole({ branchId }: { branchId: string }) {
  const [saleId, setSale] = useState("");
  const [queue, setQueue] = useState<QueuePage>();
  const [queueCursor, setQueueCursor] = useState<string>();
  const [selectedRefund, setSelectedRefund] = useState<string>();
  const [review, setReview] = useState<Review>();
  const [amount, setAmount] = useState("");
  const [reason, setReason] = useState("partial_service");
  const [draft, setDraft] = useState<Draft>();
  const [pendingRequest, setPendingRequest] = useState(false);
  const [rejection, setRejection] = useState("");
  const [decide, setDecide] = useState<{
    refund: Adjustment;
    operation: string;
  }>();
  const [status, setStatus] = useState("");
  const [busy, setBusy] = useState(false);
  const active = useRef(false);
  const format = (units: string) =>
    formatFinanceAmount(units, (review ?? queue)?.token);

  function run(task: () => Promise<unknown>) {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    setStatus("");
    task()
      .catch((error: Error) => setStatus(error.message))
      .finally(() => {
        setBusy(false);
        active.current = false;
      });
  }

  const target = { branchId, saleId };
  const storageKey = (scope: string) => `nu54-console-refund/${scope}`;
  async function readQueue(cursor?: string) {
    setQueue(undefined);
    const response = await financeRequest("refund-queue", { branchId, cursor });
    setQueue((await response.json()) as QueuePage);
    setQueueCursor(cursor);
  }

  async function reload(orderId = saleId) {
    const next = (await (
      await financeRequest("refund-status", { branchId, saleId: orderId })
    ).json()) as Review;
    if (!/^[0-9a-f]{64}$/.test(next.receiptScope))
      throw new Error("원주문 범위를 확인할 수 없습니다.");
    const pending = localStorage.getItem(storageKey(next.receiptScope));
    setPendingRequest(!!pending);
    if (pending) {
      const parsed = JSON.parse(pending) as Draft;
      if (
        !uuid.test(parsed.idempotencyKey) ||
        !/^[1-9][0-9]*$/.test(parsed.amount) ||
        !reasons[parsed.reason]
      ) {
        throw new Error("저장된 환불 참조를 확인할 수 없습니다.");
      }
      setDraft(parsed);
    }
    setReview(next);
    return next;
  }

  async function submitDraft(current: Review, request: Draft) {
    const key = storageKey(current.receiptScope);
    localStorage.setItem(key, JSON.stringify(request));
    setPendingRequest(true);
    let receipt: { refundId: string };
    try {
      receipt = await (
        await financeRequest("refund-reserve", {
          ...target,
          ...request,
        })
      ).json();
    } catch (error) {
      if (
        !(error instanceof FinanceRequestError) ||
        error.code !== "REFUND_LIMIT_CHANGED"
      )
        throw error;
      localStorage.removeItem(key);
      setDraft(undefined);
      setPendingRequest(false);
      setReview(undefined);
      const latest = await reload();
      setStatus(
        `다른 신청이 먼저 원금을 예약했습니다. 현재 신청 가능액은 ${format(
          latest.available
        )}입니다. 금액을 수정한 뒤 새로 검토하세요.`
      );
      return;
    }
    if (!uuid.test(receipt.refundId))
      throw new Error("승인 대기 상태를 다시 확인하세요.");
    localStorage.removeItem(key);
    setDraft(undefined);
    await reload();
    setStatus(`별도 승인 대기 · ${receipt.refundId}`);
  }

  return (
    <section aria-label="부분 환불">
      <h2>부분 환불과 승인</h2>
      <section aria-label="지점 환불 대기열">
        <h3>지점 환불 대기열</h3>
        <button
          disabled={busy || !uuid.test(branchId)}
          onClick={() => run(() => readQueue())}
        >
          {queue ? "환불 대기열 처음부터 다시 조회" : "환불 대기열 조회"}
        </button>
        {queue && (
          <>
            {queue.items.length === 0 && (
              <p>이 지점에서 조회할 환불이 없습니다.</p>
            )}
            <ul>
              {queue.items.map((item) => (
                <li key={item.refundId}>
                  <p>
                    환불 {item.refundId} · {format(item.amount)}
                  </p>
                  <p>
                    원주문 {item.saleId} · 원금 {format(item.principal)}
                  </p>
                  <p>
                    {states[item.state]} · {nextSteps[item.next]}
                  </p>
                  <button
                    disabled={busy}
                    onClick={() => {
                      setReview(undefined);
                      setDraft(undefined);
                      setDecide(undefined);
                      setSale(item.saleId);
                      setSelectedRefund(item.refundId);
                      run(() => reload(item.saleId));
                    }}
                  >
                    환불 {item.refundId} 원주문 열기
                  </button>
                </li>
              ))}
            </ul>
            {queueCursor && (
              <button
                disabled={busy}
                onClick={() => run(() => readQueue(queueCursor))}
              >
                현재 목록 다시 조회
              </button>
            )}
            {queue.nextCursor && (
              <button
                disabled={busy}
                onClick={() =>
                  run(() => readQueue(queue.nextCursor ?? undefined))
                }
              >
                다음 환불 목록
              </button>
            )}
          </>
        )}
      </section>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          setReview(undefined);
          setDecide(undefined);
          setDraft(undefined);
          run(() => reload());
        }}
      >
        <label>
          환불 대상 원주문
          <input
            required
            maxLength={36}
            value={saleId}
            disabled={busy}
            onChange={(event) => {
              setSale(event.target.value);
              setSelectedRefund(undefined);
              setReview(undefined);
              setDraft(undefined);
              setDecide(undefined);
            }}
          />
        </label>
        <button disabled={busy || !uuid.test(branchId)}>
          원금과 환불 상태 조회
        </button>
      </form>
      <p role="status">{status}</p>
      {review && !review.paid && (
        <p>확정 원지급과 검증된 환불 경로를 확인할 때까지 신청을 보류합니다.</p>
      )}
      {review?.paid && (
        <>
          <dl>
            <dt>원금</dt>
            <dd>{format(review.principal)}</dd>
            <dt>확정 환불</dt>
            <dd>{format(review.refunded)}</dd>
            <dt>예약 및 진행 중</dt>
            <dd>
              {format(
                (BigInt(review.reserved) + BigInt(review.inFlight)).toString()
              )}
            </dd>
            <dt>신청 가능액</dt>
            <dd>{format(review.available)}</dd>
            <dt>원결제 반환 경로</dt>
            <dd>
              {review.destination === "rental-verified"
                ? "원결제 대여 세대의 등록 반환 경로 확인"
                : review.destination === "rental-held"
                ? "검증 대기"
                : "검증된 원결제 계정"}
            </dd>
            {review.canManageFunds && (
              <>
                <dt>체인에서 관측한 환불 재원</dt>
                <dd>
                  {review.fundsAvailable
                    ? format(review.fundsAvailable)
                    : "관측 대기"}
                </dd>
              </>
            )}
          </dl>
          {review.destination === "rental-held" && (
            <p>
              종료된 대여의 원결제 반환 경로를 확인할 때까지 신청과 제출을
              보류합니다.
            </p>
          )}
          {review.canRequest &&
            !draft &&
            review.destination !== "rental-held" && (
              <form
                onSubmit={(event) => {
                  event.preventDefault();
                  const value = toUnits(amount);
                  if (!value || BigInt(value) > BigInt(review.available)) {
                    setStatus(
                      "신청 가능액 안에서 금액과 소수 자릿수를 확인하세요."
                    );
                    return;
                  }
                  setDraft({
                    amount: value,
                    reason,
                    idempotencyKey: crypto.randomUUID(),
                  });
                }}
              >
                <label>
                  환불 금액 (TestUSDC)
                  <input
                    inputMode="decimal"
                    value={amount}
                    disabled={busy}
                    onChange={(event) => setAmount(event.target.value)}
                    required
                  />
                </label>
                <label>
                  환불 사유
                  <select
                    value={reason}
                    disabled={busy}
                    onChange={(event) => setReason(event.target.value)}
                  >
                    {Object.entries(reasons).map(([key, label]) => (
                      <option key={key} value={key}>
                        {label}
                      </option>
                    ))}
                  </select>
                </label>
                <button disabled={busy}>환불 신청 검토</button>
              </form>
            )}
          {draft && review.destination !== "rental-held" && (
            <section aria-label="환불 신청 확인">
              <h3>환불 {format(draft.amount)} 신청</h3>
              <p>{reasons[draft.reason]} · 검증된 원결제 계정</p>
              <button
                disabled={busy}
                onClick={() => run(() => submitDraft(review, draft))}
              >
                같은 환불 요청 기록
              </button>
              {pendingRequest ? (
                <p>보낸 신청의 결과를 확인한 뒤 금액을 수정할 수 있습니다.</p>
              ) : (
                <button disabled={busy} onClick={() => setDraft(undefined)}>
                  검토 닫기
                </button>
              )}
            </section>
          )}
          <h3>원주문 환불 이력</h3>
          {selectedRefund && (
            <p>
              선택한 환불 {selectedRefund} · 원주문 {saleId}{" "}
              <button disabled={busy} onClick={() => run(() => reload())}>
                같은 원주문 다시 조회
              </button>
            </p>
          )}
          {review.refunds.map((refund) => (
            <article key={refund.id}>
              <p>
                {format(refund.amount)} ·{" "}
                {refund.submitted && refund.state === "approved"
                  ? "같은 환불 전송 확인 중"
                  : states[refund.state] ?? "확인 대기"}
              </p>
              <p>
                {refund.id}
                {refund.reason ? ` · ${reasons[refund.reason]}` : ""}
              </p>
              {refund.rejectionCode &&
                rejectionReasons[refund.rejectionCode] && (
                  <p>반려 사유 · {rejectionReasons[refund.rejectionCode]}</p>
                )}
              {refund.previousFailure && (
                <p>
                  이전 전송 실패 확정 {refund.previousFailure}
                  {refund.state === "requested" &&
                    " · 같은 원환불의 새 승인과 제출 필요"}
                </p>
              )}
              {!!review.fundsAvailable &&
                ["requested", "approved"].includes(refund.state) &&
                BigInt(review.fundsAvailable) < BigInt(refund.amount) && (
                  <p>
                    재원이 부족합니다. 방송 여부를 확정하기 전에 같은 거래를
                    확인하세요.
                  </p>
                )}
              {!!refund.transaction && <p>확정 거래 {refund.transaction}</p>}
              {refund.state === "final" && (
                <p>
                  원 매출 영업일 {review.orderBusinessDay} · 환불 영업일{" "}
                  {refund.businessDay}
                </p>
              )}
              {refund.state !== "final" && refund.pendingTransaction && (
                <p>기존 전송 {refund.pendingTransaction} · 확정 대기</p>
              )}
              {refund.canSubmit && review.destination !== "rental-held" && (
                <button
                  disabled={
                    busy ||
                    !review.fundsAvailable ||
                    BigInt(review.fundsAvailable) < BigInt(refund.amount)
                  }
                  onClick={() =>
                    setDecide({ refund, operation: "refund-submit" })
                  }
                >
                  재원과 제출 검토
                </button>
              )}
              {refund.canDecide && (
                <>
                  <button
                    disabled={busy || review.destination === "rental-held"}
                    onClick={() =>
                      setDecide({ refund, operation: "refund-approve" })
                    }
                  >
                    {refund.reviewRevision > 0
                      ? "확정 실패 후 새 승인 검토"
                      : "별도 승인 검토"}
                  </button>
                  <button
                    disabled={busy}
                    onClick={() => {
                      setRejection("");
                      setDecide({ refund, operation: "refund-reject" });
                    }}
                  >
                    거절 검토
                  </button>
                </>
              )}
            </article>
          ))}
          {decide && (
            <section aria-label="환불 결재 확인">
              <h3>
                {decide.operation === "refund-submit"
                  ? "환불 재원과 제출 대상 확인"
                  : decide.operation === "refund-approve"
                  ? "별도 승인"
                  : "환불 요청 거절"}{" "}
                · {format(decide.refund.amount)}
              </h3>
              <p>
                {decide.refund.id} ·{" "}
                {review.destination === "rental-verified"
                  ? "검증된 대여 반환 경로"
                  : "검증된 원결제 계정"}
              </p>
              {decide.operation === "refund-submit" && (
                <p>
                  가용 {format(review.fundsAvailable ?? "0")} · 수수료는 원금과
                  분리
                </p>
              )}
              {decide.operation === "refund-reject" && (
                <label>
                  반려 사유
                  <select
                    value={rejection}
                    disabled={busy}
                    onChange={(event) => setRejection(event.target.value)}
                  >
                    <option value="">반려 사유 선택</option>
                    {Object.entries(rejectionReasons).map(([code, label]) => (
                      <option key={code} value={code}>
                        {label}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              <button
                disabled={
                  busy ||
                  (decide.operation === "refund-reject" &&
                    !rejectionReasons[rejection])
                }
                onClick={() =>
                  run(async () => {
                    try {
                      await financeRequest(decide.operation, {
                        ...target,
                        refundId: decide.refund.id,
                        reviewRevision: decide.refund.reviewRevision,
                        ...(decide.operation === "refund-reject"
                          ? { rejectionCode: rejection }
                          : {}),
                      });
                    } catch (error) {
                      if (!(error instanceof FinanceRequestError)) throw error;
                      setDecide(undefined);
                      setReview(undefined);
                      await reload();
                      setStatus(error.message);
                      return;
                    }
                    setDecide(undefined);
                    await reload();
                    setStatus("같은 환불의 결재 결과를 확인했습니다.");
                  })
                }
              >
                {decide.operation === "refund-submit"
                  ? "이 환불만 제출 요청"
                  : "현재 계정으로 결정 기록"}
              </button>
              <button disabled={busy} onClick={() => setDecide(undefined)}>
                취소
              </button>
            </section>
          )}
        </>
      )}
    </section>
  );
}
