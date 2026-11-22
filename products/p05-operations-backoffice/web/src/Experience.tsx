import { useRef, useState, type FormEvent } from "react";
import { formatFinanceAmount as showMoney } from "./Finance";

type Leg = {
  kind: string;
  order: string;
  state: string;
  amount: string;
  expiresAt?: string;
  businessDay?: string;
  refundable: string;
};
type Booking = {
  id: string;
  provider: string;
  visitAt: string;
  timezone: string;
  partySize: number;
  deposit: string;
  balance: string;
  releasedAt?: string;
  legs: Leg[];
};
type Invoice = {
  id: string;
  cycle: string;
  dueAt: string;
  order: string;
  state: string;
  amount: string;
  grantedAt?: string;
  revokedAt?: string;
};
type Subscription = {
  id: string;
  state: string;
  revision: number;
  timezone: string;
  nextCycle?: string;
  cancelledAt?: string;
  canCancel: boolean;
  invoices: Invoice[];
};
type Portfolio = {
  bookings: Booking[];
  subscriptions: Subscription[];
  receiptScope: string;
};
type Cancellation = {
  subscription: string;
  expectedRevision: number;
  requestKey: string;
};

const uuid = /^[0-9a-f-]{36}$/;
const stateLabel: Record<string, string> = {
  preview: "접수 확인 중",
  issued: "견적 발행 · 지급 관측 중",
  checking: "기존 지급 관측 중",
  paid: "기존 주문 지급 확인",
  closed: "확정 미지급 또는 무효화",
  active: "운영 중",
  paused: "일시 중지",
  cancelled: "향후 회차 종료",
};
const time = (value: string, zone: string) =>
  new Intl.DateTimeFormat("ko-KR", {
    dateStyle: "short",
    timeStyle: "short",
    timeZone: zone,
  }).format(new Date(value));

export function Experience({ initialBranch }: { initialBranch: string }) {
  const [branch, setBranch] = useState(initialBranch);
  const [reference, setReference] = useState("");
  const [portfolio, setPortfolio] = useState<Portfolio>();
  const [selection, setSelection] = useState("");
  const [cancel, setCancel] = useState<Cancellation>();
  const [cancelPending, setCancelPending] = useState(false);
  const [status, setStatus] = useState("");
  const [busy, setBusy] = useState(false);
  const running = useRef(false);

  async function request(ref?: string, body?: Cancellation) {
    const base = `/v2/operations/experience/${encodeURIComponent(branch)}`;
    const path = body
      ? `${base}/cancel`
      : ref
      ? `${base}?reference=${encodeURIComponent(ref)}`
      : base;
    const response = await fetch(path, {
      method: body ? "POST" : "GET",
      credentials: "same-origin",
      headers: body && { "Content-Type": "application/json" },
      body: body && JSON.stringify(body),
    });
    if (response.status === 404)
      throw new Error("이 지점의 예약과 구독을 조회할 권한이 없습니다.");
    if (response.status === 409)
      throw new Error("구독 상태가 바뀌었습니다. 같은 참조를 다시 확인하세요.");
    if (!response.ok)
      throw new Error(
        "처리가 확인되지 않았습니다. 같은 참조를 다시 조회하세요."
      );
    const snapshot = (await response.json()) as Portfolio;
    if (!/^[0-9a-f]{64}$/.test(snapshot.receiptScope))
      throw new Error("운영 요청의 범위를 확인할 수 없습니다.");
    return snapshot;
  }
  async function run(action: () => Promise<void>) {
    if (running.current) return;
    running.current = true;
    setBusy(true);
    setStatus("");
    try {
      await action();
    } catch (error) {
      setStatus((error as Error).message);
    } finally {
      running.current = false;
      setBusy(false);
    }
  }
  function read(event: FormEvent) {
    event.preventDefault();
    setPortfolio(undefined);
    setSelection("");
    setCancel(undefined);
    setCancelPending(false);
    if (!uuid.test(branch) || (reference && !uuid.test(reference))) return;
    void run(async () => setPortfolio(await request(reference)));
  }
  function refresh(id: string) {
    void run(async () => {
      setPortfolio(await request(id));
      setSelection(id);
    });
  }

  function cancellationKey(scope: string, id: string) {
    return `nu54-experience-cancel/${scope}/${id}`;
  }

  function selectSubscription(snapshot: Portfolio, id: string) {
    setSelection(id);
    setCancel(undefined);
    setCancelPending(false);
    const stored = localStorage.getItem(
      cancellationKey(snapshot.receiptScope, id)
    );
    if (
      !stored ||
      !snapshot.subscriptions.some((item) => item.id === id && item.canCancel)
    )
      return;
    try {
      const command = JSON.parse(stored) as Cancellation;
      if (
        command.subscription !== id ||
        !uuid.test(command.requestKey) ||
        !Number.isSafeInteger(command.expectedRevision) ||
        command.expectedRevision < 0
      )
        throw new Error("invalid receipt");
      setCancel(command);
      setCancelPending(true);
    } catch {
      setStatus("저장된 해지 요청의 참조를 확인할 수 없습니다.");
    }
  }

  async function confirmCancellation(
    snapshot: Portfolio,
    command: Cancellation
  ) {
    const key = cancellationKey(snapshot.receiptScope, command.subscription);
    localStorage.setItem(key, JSON.stringify(command));
    setCancelPending(true);
    const next = await request(undefined, command);
    if (
      next.receiptScope !== snapshot.receiptScope ||
      !next.subscriptions.some(
        (item) => item.id === command.subscription && item.state === "cancelled"
      )
    )
      throw new Error("같은 해지 요청의 확정 상태를 확인할 수 없습니다.");
    localStorage.removeItem(key);
    setPortfolio(next);
    setCancel(undefined);
    setCancelPending(false);
    setStatus("같은 해지 요청이 기록되었습니다.");
  }

  return (
    <>
      <h1>예약과 구독 회차</h1>
      <form className="branch" onSubmit={read}>
        <label>
          지점 참조
          <input
            required
            value={branch}
            maxLength={36}
            disabled={busy}
            onChange={(event) => {
              setBranch(event.target.value);
              setPortfolio(undefined);
              setSelection("");
              setCancel(undefined);
            }}
          />
        </label>
        <label>
          예약 또는 구독 참조
          <input
            value={reference}
            maxLength={36}
            disabled={busy}
            onChange={(event) => setReference(event.target.value)}
          />
        </label>
        <button disabled={busy}>예약과 청구 조회</button>
      </form>
      <p role="status">{status}</p>
      {portfolio && (
        <section>
          <h2>지점 예약과 구독 목록</h2>
          {!portfolio.bookings.length && !portfolio.subscriptions.length && (
            <p>조회 범위에 예약이나 구독이 없습니다.</p>
          )}
          {portfolio.bookings.map((item) => (
            <article key={item.id}>
              <button disabled={busy} onClick={() => setSelection(item.id)}>
                예약 {item.id}
              </button>
              {selection === item.id && (
                <>
                  <p>
                    {item.provider} · 이용 {time(item.visitAt, item.timezone)} ·{" "}
                    {item.partySize}명 · {item.timezone}
                  </p>
                  <p>
                    예약금 {showMoney(item.deposit)} · 잔금{" "}
                    {showMoney(item.balance)}
                  </p>
                  <p>
                    {item.releasedAt
                      ? "예약 재고 반환 확인"
                      : "기존 지급 증거에 따라 예약 재고를 보류합니다."}
                  </p>
                  {item.legs.map((leg) => (
                    <p key={leg.order}>
                      {leg.kind === "deposit" ? "예약금" : "잔금"} ·{" "}
                      {showMoney(leg.amount)} · {stateLabel[leg.state]} ·{" "}
                      {leg.order}
                      {leg.expiresAt &&
                        ` · 지급 기한 ${time(leg.expiresAt, item.timezone)}`}
                      {leg.businessDay && ` · 영업일 ${leg.businessDay}`} · 환불
                      가능액 {showMoney(leg.refundable)}
                    </p>
                  ))}
                  <button disabled={busy} onClick={() => refresh(item.id)}>
                    같은 주문 다시 조회
                  </button>
                </>
              )}
            </article>
          ))}
          {portfolio.subscriptions.map((item) => (
            <article key={item.id}>
              <button
                disabled={busy}
                onClick={() => selectSubscription(portfolio, item.id)}
              >
                구독 {item.id}
              </button>
              {selection === item.id && (
                <>
                  <p>
                    {stateLabel[item.state]}
                    {item.nextCycle && ` · 다음 회차 ${item.nextCycle}`} ·{" "}
                    {item.timezone}
                  </p>
                  {item.invoices.map((invoice) => (
                    <p key={invoice.id}>
                      {invoice.cycle} · {showMoney(invoice.amount)} ·{" "}
                      {stateLabel[invoice.state]} · {invoice.order} · 지급 기한{" "}
                      {time(invoice.dueAt, item.timezone)}
                      {invoice.grantedAt &&
                        (invoice.revokedAt
                          ? " · 이용권 회수"
                          : " · 이용권 기록")}
                    </p>
                  ))}
                  <button disabled={busy} onClick={() => refresh(item.id)}>
                    발급 회차 다시 조회
                  </button>
                  {item.canCancel && item.state !== "cancelled" && !cancel && (
                    <button
                      disabled={busy}
                      onClick={() =>
                        setCancel({
                          subscription: item.id,
                          expectedRevision: item.revision,
                          requestKey: crypto.randomUUID(),
                        })
                      }
                    >
                      해지 범위 검토
                    </button>
                  )}
                  {cancel?.subscription === item.id && (
                    <section aria-label="향후 구독 회차 해지 검토">
                      <h3>향후 회차만 종료</h3>
                      {cancelPending ? (
                        <p>보낸 해지 요청의 결과를 확인하세요.</p>
                      ) : (
                        <p>
                          {item.nextCycle} 이후에는 새 청구를 만들지 않습니다.
                          이미 발급된 주문은 계속 조회할 수 있습니다.
                        </p>
                      )}
                      <button
                        disabled={busy}
                        onClick={() =>
                          void run(() => confirmCancellation(portfolio, cancel))
                        }
                      >
                        같은 해지 요청 확인
                      </button>
                      {!cancelPending && (
                        <button
                          disabled={busy}
                          onClick={() => setCancel(undefined)}
                        >
                          검토 취소
                        </button>
                      )}
                    </section>
                  )}
                </>
              )}
            </article>
          ))}
        </section>
      )}
    </>
  );
}
