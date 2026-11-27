/// <reference types="vite/client" />
import { RefundConsole } from "./RefundConsole";
import {
  namedToken,
  replyToken,
  tokenMoney,
  type Branded,
  type BranchToken,
} from "./money";
import {
  escrowStage,
  escrowStages,
  RefundRefused,
  type EscrowItem,
} from "./Escrow";
import { useRef, useState, type FormEvent, type ReactNode } from "react";

type Report = {
  receiptScope: string;
  day: string;
  zone: string;
  gross: string;
  refunds: string;
  net: string;
  activeRefunds: number;
  openFulfillment: number;
  closeRevision: number | null;
  asset?: string;
} & Branded;
type EventSummary = {
  eventId: string;
  saleId: string;
  kind: string;
  state: string;
};
export type Order = {
  saleId: string;
  amount: string;
  asset?: string;
  state: string;
  paidAt: string | null;
  transaction: string | null;
  quoteId: string | null;
  observedHeight: number | null;
  confirmedHeight: number | null;
  fundsStatus: "held" | "closed" | "confirmed";
  fulfillmentStatus: "held" | "closed" | "recorded";
  refunds: string;
  events: EventSummary[];
  // Present when the server also reports the escrow state of the order.
  escrow?: EscrowItem;
} & Branded;
type RetryReceipt = EventSummary & { requestId: string };
type NotificationReview = EventSummary & {
  requestId?: string;
  canRetry: boolean;
  businessDay: string;
  bookedAt: string;
  ledgerAmount: string;
  asset?: string;
  receivedAt?: string;
  deliveredAt?: string;
  originalPayment: {
    status: string;
    beneficiaryVersion?: number;
    transaction?: string;
    includedAt?: string;
    attestedAt?: string;
    explorerURL?: string;
  };
} & Branded;
type Cases = {
  events: EventSummary[];
  receipts: RetryReceipt[];
  status: string;
  chainGross?: string;
  chainRefunds?: string;
  height?: number;
};

export const scopedReceipt = (operation: string, scope: string) =>
  `nu54-finance-v1/${operation}/${scope}`;
export function requestKey(operation: string, scope: string) {
  const name = scopedReceipt(operation, scope);
  const key = localStorage.getItem(name) ?? crypto.randomUUID();
  if (!/^[0-9a-f-]{36}$/.test(key))
    throw new Error("저장된 요청 참조를 확인할 수 없습니다.");
  localStorage.setItem(name, key);
  return key;
}

export function formatFinanceAmount(units: string, token?: BranchToken) {
  return tokenMoney(units, token);
}

// The escrow token of a deployment is named when the console is built, as lower-case addresses
// separated by commas. Its six decimals are part of that choice, so no other asset gets its label.
const pilotAssets = (import.meta.env.VITE_PILOT_ASSETS ?? "")
  .toLowerCase()
  .split(",")
  .map((asset: string) => asset.trim())
  .filter(Boolean);
export const knownAsset = (asset?: string) =>
  !!asset &&
  (pilotAssets.includes(asset.toLowerCase()) || !!namedToken(asset));
export const assetWarning =
  "시범 결제 자산으로 확인되지 않아 금액을 기본 단위로 표시합니다. 배포 설정을 확인하세요.";
export function formatAssetAmount(units: string, asset?: string) {
  return knownAsset(asset)
    ? formatFinanceAmount(units, namedToken(asset))
    : `${BigInt(units).toLocaleString("ko-KR")} 기본 단위`;
}

export function financeTime(seconds: number, timeZone?: string) {
  return new Date(seconds * 1000).toLocaleString("ko-KR", {
    timeZone,
    month: "long",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  });
}

export class FinanceRequestError extends Error {
  constructor(message: string, readonly code: string) {
    super(message);
  }
}

/** The request may have been accepted, so only the same request may be sent again. */
export class FinanceReplyLost extends Error {}

const escrowRefusals: Record<number, string> = {
  400: "요청 내용을 확인할 수 없습니다. 목록을 다시 조회하세요.",
  404: "지점 또는 업무 권한을 확인할 수 없습니다.",
  409: "주문 상태가 바뀌었습니다. 목록을 다시 조회하세요.",
};

export async function financeRequest(operation: string, body: object) {
  const reply = await fetch(`/v2/finance/${operation}`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }).catch(() => {
    throw new FinanceReplyLost(
      "응답을 받지 못했습니다. 연결을 확인한 뒤 같은 요청을 다시 확인하세요."
    );
  });
  if (operation.startsWith("escrow-") && !reply.ok) {
    const refusal = escrowRefusals[reply.status];
    if (refusal) throw new FinanceRequestError(refusal, String(reply.status));
    throw new FinanceReplyLost(
      "서버가 처리 결과를 알려 주지 못했습니다. 잠시 뒤 같은 요청을 다시 확인하세요."
    );
  }
  if (reply.status === 404)
    throw new Error("지점 또는 업무 권한을 확인할 수 없습니다.");
  if (reply.status === 409 && operation.startsWith("refund-")) {
    const conflict = await reply.json().catch(() => undefined);
    const hints: Record<string, string> = {
      REFUND_LIMIT_CHANGED: "남은 환불 금액을 다시 확인하세요.",
      REFUND_FUNDS_HELD: "환불 재원을 다시 조회한 뒤 같은 지급을 검토하세요.",
      REFUND_DESTINATION_HELD:
        "대여 원결제의 등록 반환 경로를 확인할 때까지 보류합니다.",
    };
    if (typeof conflict?.code === "string" && hints[conflict.code]) {
      throw new FinanceRequestError(hints[conflict.code], conflict.code);
    }
  }
  if (!reply.ok)
    throw new Error("기존 요청의 상태를 확인한 뒤 다시 시도하세요.");
  return reply;
}

export function Finance({ initialBranch }: { initialBranch: string }) {
  const [branchId, setBranch] = useState(initialBranch);
  const [businessDay, setDay] = useState("");
  const [saleId, setSale] = useState("");
  const [report, setReport] = useState<Report>();
  const [order, setOrder] = useState<Order>();
  const [cases, setCases] = useState<Cases>();
  const [notification, setNotification] = useState<NotificationReview>();
  const [lookupEvent, setLookupEvent] = useState("");
  const [lookupRequest, setLookupRequest] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const running = useRef(false);
  const [reviewClose, setReviewClose] = useState(false);
  const [pendingClose, setPendingClose] = useState(false);
  const scope = { branchId, businessDay };

  function run(task: () => Promise<unknown>) {
    if (running.current) return;
    running.current = true;
    setNotice("");
    setBusy(true);
    task()
      .catch((error: Error) => setNotice(error.message))
      .finally(() => {
        running.current = false;
        setBusy(false);
      });
  }

  function clearScope() {
    setReport(undefined);
    setOrder(undefined);
    setCases(undefined);
    setNotification(undefined);
    setLookupEvent("");
    setLookupRequest("");
    setReviewClose(false);
    setPendingClose(false);
    setNotice("");
  }

  async function readCases() {
    setCases(
      (await (await financeRequest("reconciliation", scope)).json()) as Cases
    );
  }

  const notificationScope = (eventId: string) =>
    report && report.receiptScope + "/" + eventId;
  async function reviewNotification(eventId: string, idempotencyKey?: string) {
    const details = (await (
      await financeRequest("fulfillment-review", {
        ...scope,
        eventId,
        idempotencyKey,
      })
    ).json()) as NotificationReview;
    if (
      details.eventId !== eventId ||
      (idempotencyKey && details.requestId !== idempotencyKey)
    )
      throw new Error("같은 통지의 접수 기록을 확인할 수 없습니다.");
    setNotification(details);
    return details;
  }

  async function openOriginalPayment(item: NotificationReview) {
    const refreshed = await reviewNotification(item.eventId, item.requestId);
    const link = refreshed.originalPayment.explorerURL;
    if (!link || link !== item.originalPayment.explorerURL ||
        refreshed.saleId !== item.saleId || refreshed.originalPayment.status !== "verified") {
      throw new Error("공개 탐색기 근거가 달라졌습니다. 같은 통지를 다시 조회하세요.");
    }
    const publicPage = new URL(link);
    const loopback = publicPage.protocol === "http:" && ["127.0.0.1", "[::1]"].includes(publicPage.hostname);
    if ((publicPage.protocol !== "https:" && !loopback) || publicPage.origin === location.origin ||
        publicPage.username || publicPage.password || publicPage.pathname !== "/" || publicPage.hash ||
        [...publicPage.searchParams.keys()].sort().join(",") !== "logIndex,merchant,stream,txHash" ||
        publicPage.searchParams.get("txHash") !== refreshed.originalPayment.transaction) {
      throw new Error("등록된 공개 탐색기 주소를 확인할 수 없습니다.");
    }
    const anchor = document.createElement("a");
    anchor.href = publicPage.href;
    anchor.referrerPolicy = "no-referrer";
    anchor.rel = "noopener noreferrer";
    anchor.click();
  }

  async function retryNotification(item: NotificationReview) {
    const retryScope = notificationScope(item.eventId);
    if (!retryScope || !item.canRetry || item.requestId) return;
    const idempotencyKey = requestKey("fulfillment-retry", retryScope);
    const reply = (await (
      await financeRequest("fulfillment-retry", {
        branchId,
        eventId: item.eventId,
        idempotencyKey,
      })
    ).json()) as RetryReceipt;
    if (reply.eventId !== item.eventId || reply.requestId !== idempotencyKey)
      throw new Error("같은 통지의 접수 기록을 확인할 수 없습니다.");
    await reviewNotification(item.eventId, idempotencyKey);
    localStorage.removeItem(scopedReceipt("fulfillment-retry", retryScope));
    await readCases();
    setNotice("같은 통지의 처리 상태를 다시 조회하도록 요청했습니다.");
  }

  function showReport(event: FormEvent) {
    event.preventDefault();
    clearScope();
    run(async () => {
      const current = (await (
        await financeRequest("report", scope)
      ).json()) as Report;
      if (!/^[0-9a-f]{64}$/.test(current.receiptScope))
        throw new Error("보고서 범위를 확인할 수 없습니다.");
      setReport(current);
      setPendingClose(
        !!localStorage.getItem(
          scopedReceipt("report-close", current.receiptScope)
        )
      );
    });
  }

  function close() {
    if (!report) return;
    run(async () => {
      const idempotencyKey = requestKey("report-close", report.receiptScope);
      setPendingClose(true);
      setReviewClose(false);
      const receipt = (await (
        await financeRequest("report-close", { ...scope, idempotencyKey })
      ).json()) as { revision: number; idempotencyKey: string };
      if (
        receipt.idempotencyKey !== idempotencyKey ||
        !Number.isSafeInteger(receipt.revision)
      ) {
        throw new Error("같은 마감 영수증을 확인할 수 없습니다.");
      }
      localStorage.removeItem(
        scopedReceipt("report-close", report.receiptScope)
      );
      setPendingClose(false);
      setReport({ ...report, closeRevision: receipt.revision });
      setCases(undefined);
      setNotice(`검증된 영업일 마감을 기록했습니다. 버전 ${receipt.revision}`);
    });
  }

  function download() {
    if (!report) return;
    run(async () => {
      const idempotencyKey = requestKey("report-export", report.receiptScope);
      const reply = await financeRequest("report-export", {
        ...scope,
        idempotencyKey,
      });
      if (!reply.headers.get("content-type")?.startsWith("text/csv")) {
        throw new Error("내보낸 파일 형식을 확인할 수 없습니다.");
      }
      const url = URL.createObjectURL(await reply.blob());
      const link = document.createElement("a");
      link.href = url;
      link.download = `settlement-${businessDay}.csv`;
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      localStorage.removeItem(
        scopedReceipt("report-export", report.receiptScope)
      );
      setNotice("지정한 영업일 범위의 내보내기를 기록했습니다.");
    });
  }

  return (
    <>
      <h1>지점 정산</h1>
      <form className="branch" onSubmit={showReport}>
        <label>
          지점 참조
          <input
            value={branchId}
            required
            maxLength={36}
            disabled={busy}
            onChange={(event) => {
              clearScope();
              setBranch(event.target.value);
            }}
          />
        </label>
        <label>
          영업일
          <input
            type="date"
            value={businessDay}
            required
            disabled={busy}
            onChange={(event) => {
              clearScope();
              setDay(event.target.value);
            }}
          />
        </label>
        <button disabled={busy}>정산 조회</button>
      </form>
      <p role="status">{notice}</p>
      {report && (
        <section>
          <h2>
            {report.day} · {report.zone}
          </h2>
          <dl>
            <dt>원장에 반영된 확정 매출</dt>
            <dd>{formatFinanceAmount(report.gross, replyToken(report, report.asset))}</dd>
            <dt>원장에 반영된 확정 환불</dt>
            <dd>{formatFinanceAmount(report.refunds, replyToken(report, report.asset))}</dd>
            <dt>순매출</dt>
            <dd>{formatFinanceAmount(report.net, replyToken(report, report.asset))}</dd>
            <dt>미완료 이행</dt>
            <dd>{report.openFulfillment}</dd>
            <dt>처리 중인 환불</dt>
            <dd>{report.activeRefunds}</dd>
            <dt>기록된 마감 버전</dt>
            <dd>{report.closeRevision ?? "대기"}</dd>
          </dl>
          <button disabled={busy} onClick={download}>
            이 범위 CSV 내려받기
          </button>
          <button disabled={busy} onClick={() => run(readCases)}>
            조사 대기열 조회
          </button>
        </section>
      )}
      {cases && (
        <section>
          <h2>조사 대기열</h2>
          <p>
            {cases.status === "held"
              ? "체인 대사 확인까지 마감 보류"
              : "대사 확인"}
          </p>
          {cases.chainGross !== undefined &&
            cases.chainRefunds !== undefined && (
              <p>
                체인 확정 입금 {formatFinanceAmount(cases.chainGross, replyToken(report, report?.asset))} · 환불{" "}
                {formatFinanceAmount(cases.chainRefunds, replyToken(report, report?.asset))} · 확정 높이{" "}
                {cases.height}
              </p>
            )}
          {cases.status === "matched" && !pendingClose && !reviewClose && (
            <button disabled={busy} onClick={() => setReviewClose(true)}>
              마감 검토
            </button>
          )}
          {cases.events.map((event) => (
            <article key={event.eventId}>
              <p>
                {event.eventId} · {event.kind} · {event.state}
              </p>
              <button
                disabled={busy}
                onClick={() =>
                  run(() =>
                    reviewNotification(
                      event.eventId,
                      cases.receipts?.find(
                        (receipt) => receipt.eventId === event.eventId
                      )?.requestId
                    )
                  )
                }
              >
                통지와 영향 검토
              </button>
            </article>
          ))}
          {(cases.receipts?.length ?? 0) > 0 && <h3>내 재처리 조회 기록</h3>}
          {cases.receipts?.map((receipt) => (
            <p key={receipt.requestId}>
              조회 ID {receipt.requestId} · 통지 {receipt.eventId} ·{" "}
              {receipt.state}{" "}
              <button
                disabled={busy}
                onClick={() =>
                  run(() =>
                    reviewNotification(receipt.eventId, receipt.requestId)
                  )
                }
              >
                같은 조회 ID 확인
              </button>
            </p>
          ))}
          <form
            onSubmit={(event) => {
              event.preventDefault();
              run(() => reviewNotification(lookupEvent, lookupRequest));
            }}
          >
            <label>
              기존 통지 ID{" "}
              <input
                required
                maxLength={36}
                value={lookupEvent}
                onChange={(event) => setLookupEvent(event.target.value)}
              />
            </label>
            <label>
              기존 조회 ID{" "}
              <input
                required
                maxLength={36}
                value={lookupRequest}
                onChange={(event) => setLookupRequest(event.target.value)}
              />
            </label>
            <button disabled={busy}>기존 접수 기록 찾기</button>
          </form>
        </section>
      )}
      {notification && (
        <section aria-label="통지 재처리 검토">
          <h2>
            {notification.requestId
              ? "기존 재처리 접수 기록"
              : "통지 재처리 검토"}
          </h2>
          <dl>
            <dt>통지 ID</dt>
            <dd>{notification.eventId}</dd>
            <dt>원주문</dt>
            <dd>{notification.saleId}</dd>
            <dt>수신자</dt>
            <dd>선택한 지점의 이행 시스템</dd>
            <dt>통지 유형과 상태</dt>
            <dd>
              {notification.kind === "paid" ? "지급" : "환불"} ·{" "}
              {notification.state}
            </dd>
            <dt>영업일과 장부 반영 시각</dt>
            <dd>
              {notification.businessDay} · {notification.bookedAt}
            </dd>
            <dt>장부 금액</dt>
            <dd>{formatFinanceAmount(notification.ledgerAmount, replyToken(notification, notification.asset))}</dd>
            <dt>원결제 등록 주소 버전</dt>
            <dd>
              {notification.originalPayment.beneficiaryVersion ?? "확인 대기"}
            </dd>
            <dt>원결제 체인 증거</dt>
            <dd>
              {notification.originalPayment.status === "verified"
                ? "확정 증거 일치"
                : "확정 증거 확인 대기"}
            </dd>
            {notification.originalPayment.includedAt && (
              <>
                <dt>체인 포함·증거 확인 시각</dt>
                <dd>
                  {notification.originalPayment.includedAt} ·{" "}
                  {notification.originalPayment.attestedAt}
                </dd>
              </>
            )}
            {notification.requestId && (
              <>
                <dt>조회 ID</dt>
                <dd>{notification.requestId}</dd>
              </>
            )}
            {notification.receivedAt && (
              <>
                <dt>수신 영수증</dt>
                <dd>
                  {notification.eventId} · {notification.receivedAt}
                </dd>
              </>
            )}
          </dl>
          {notification.originalPayment.explorerURL ? (
            <button disabled={busy} onClick={() => run(() => openOriginalPayment(notification))}>
              공개 탐색기에서 원결제 보기
            </button>
          ) : (
            <>
              <p>공개 탐색기에서 원결제를 열 수 있는지 확인이 필요합니다.</p>
              <button disabled={busy} onClick={() => run(() => reviewNotification(notification.eventId, notification.requestId))}>
                같은 통지의 원결제 다시 조회
              </button>
            </>
          )}
          <p>
            이 통지 한 건을 같은 ID로 확인합니다. 기존 주문, 지급과 이행 기록은
            유지됩니다.
          </p>
          {notification.requestId ? (
            <button
              disabled={busy}
              onClick={() =>
                run(() =>
                  reviewNotification(
                    notification.eventId,
                    notification.requestId
                  )
                )
              }
            >
              이 접수 기록 다시 조회
            </button>
          ) : notification.canRetry ? (
            <button
              disabled={busy}
              onClick={() => run(() => retryNotification(notification))}
            >
              같은 통지 재확인 요청
            </button>
          ) : (
            <p>현재 통지 상태와 지점 권한으로 재처리를 요청할 수 없습니다.</p>
          )}
          <button disabled={busy} onClick={() => setNotification(undefined)}>
            {notification.requestId ? "조사 대기열로" : "취소"}
          </button>
        </section>
      )}
      {(reviewClose || pendingClose) && report && (
        <section aria-label="영업일 마감">
          <h2>
            {pendingClose ? "같은 마감 상태 다시 확인" : "마감 요청 확인"}
          </h2>
          <p>
            {report.day} · {report.zone} · 순매출{" "}
            {formatFinanceAmount(report.net, replyToken(report, report.asset))}
          </p>
          <button disabled={busy} onClick={close}>
            {pendingClose ? "같은 마감 조회" : "마감 요청"}
          </button>
          {!pendingClose && (
            <button disabled={busy} onClick={() => setReviewClose(false)}>
              취소
            </button>
          )}
        </section>
      )}
      <RefundConsole branchId={branchId} key={branchId} />
      <section>
        <h2>원주문 조회</h2>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            setOrder(undefined);
            run(async () =>
              setOrder(
                (await (
                  await financeRequest("order", { branchId, saleId })
                ).json()) as Order
              )
            );
          }}
        >
          <label>
            원주문 참조
            <input
              value={saleId}
              required
              maxLength={36}
              disabled={busy}
              onChange={(event) => {
                setOrder(undefined);
                setSale(event.target.value);
              }}
            />
          </label>
          <button disabled={busy || !branchId}>
            {order ? "이 주문 다시 조회" : "기존 주문 확인"}
          </button>
        </form>
        {order && (
          <OrderDetail order={order}>
            {order.fundsStatus === "held" && !order.paidAt && businessDay && (
              <button disabled={busy} onClick={() => run(readCases)}>
                이 지점 조사 대기열 조회
              </button>
            )}
          </OrderDetail>
        )}
      </section>
    </>
  );
}

/** Payment detail of one order; `escrow` replaces the held wording for an escrow order. */
export function OrderDetail({
  order,
  escrow: listed,
  timezone,
  children,
}: {
  order: Order;
  escrow?: EscrowItem;
  timezone?: string;
  children?: ReactNode;
}) {
  const reported = listed ?? order.escrow;
  const escrow = reported && escrowStages[reported.status] ? reported : undefined;
  // A paid order whose funds are still reported as held is kept by the order escrow.
  const kept = !!reported || (order.fundsStatus === "held" && !!order.paidAt);
  const asset = escrow?.asset ?? order.asset;
  const references = (
    <>
      {order.transaction && (
        <>
          <dt>거래 해시</dt>
          <dd>{order.transaction}</dd>
        </>
      )}
      {order.observedHeight != null && (
        <>
          <dt>관측 포함 높이</dt>
          <dd>{order.observedHeight}</dd>
        </>
      )}
      {order.confirmedHeight != null && (
        <>
          <dt>확정 근거 높이</dt>
          <dd>{order.confirmedHeight}</dd>
        </>
      )}
    </>
  );
  return (
    <section aria-label="원주문 지급 상세" aria-live="polite">
      <h3>
        {kept
          ? "주문 결제 상세"
          : order.fundsStatus === "held"
            ? "기존 주문을 조회하고 있습니다"
            : "기존 주문 결제 상세"}
      </h3>
      <dl>
        {!kept && (
          <>
            <dt>원주문</dt>
            <dd>{order.saleId}</dd>
          </>
        )}
        <dt>주문 금액</dt>
        <dd>
          {kept
            ? order.token
              ? formatFinanceAmount(order.amount, replyToken(order, order.asset))
              : formatAssetAmount(order.amount, asset)
            : formatFinanceAmount(order.amount, replyToken(order, order.asset))}
          {kept && !order.token && !knownAsset(asset) && (
            <>
              <br />
              {assetWarning}
            </>
          )}
        </dd>
        {!kept && (
          <>
            <dt>확정 환불</dt>
            <dd>{formatFinanceAmount(order.refunds, replyToken(order, order.asset))}</dd>
          </>
        )}
        {!kept && order.quoteId && (
          <>
            <dt>발급 견적</dt>
            <dd>{order.quoteId}</dd>
          </>
        )}
        {kept ? (
          <>
            <dt>결제 확정</dt>
            <dd>
              {order.paidAt
                ? financeTime(Date.parse(order.paidAt) / 1000, timezone)
                : "확인 중"}
            </dd>
            <dt>대금 상태</dt>
            <dd>
              {escrow ? (
                <>
                  {escrowStages[escrow.status]}
                  <br />
                  {escrowStage(escrow, timezone)}
                  <RefundRefused item={escrow} />
                </>
              ) : (
                "구매자가 결제한 대금을 주문 계약이 보관하거나 정산했습니다. 현재 상태는 대금 정산에서 확인하세요."
              )}
            </dd>
          </>
        ) : (
          <>
            <dt>지급</dt>
            <dd>
              {order.paidAt
                ? `지급 확정 ${order.paidAt}`
                : order.fundsStatus === "closed"
                  ? "미지급 종료"
                  : "확정 대기"}
            </dd>
            {references}
          </>
        )}
        {!kept && order.fulfillmentStatus === "held" && (
          <>
            <dt>이행</dt>
            <dd>보류</dd>
            <dt>새 지급·영수증</dt>
            <dd>보류</dd>
          </>
        )}
      </dl>
      {kept ? (
        <details>
          <summary>조회 참조</summary>
          <dl>
            <dt>원주문</dt>
            <dd>{order.saleId}</dd>
            {references}
          </dl>
        </details>
      ) : (
        order.events.map((event) => (
          <p key={event.eventId}>
            이행 {event.kind} · {event.state} · {event.eventId}
          </p>
        ))
      )}
      {children}
    </section>
  );
}
