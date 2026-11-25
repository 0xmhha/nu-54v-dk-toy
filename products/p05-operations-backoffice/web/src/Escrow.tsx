import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  BranchPicker,
  useBranchAccess,
  type BranchScope,
} from "./BranchPicker";
import {
  FinanceReplyLost,
  financeRequest,
  financeTime,
  formatAssetAmount,
  FinanceRequestError,
  OrderDetail,
  type Order,
} from "./Finance";
import { rememberTokens, type Branded } from "./money";

// spending-wallet-wire.md section 7. Unix seconds and base-unit amounts.
export type EscrowItem = {
  saleId: string;
  pickupNo?: number;
  amount: string;
  asset: string;
  status:
    | "held"
    | "waiting"
    | "claimable"
    | "claiming"
    | "disputed"
    | "paid"
    | "refunded";
  lockedAt: number;
  deliverBy: number;
  claimableAt?: number;
  resolveBy?: number;
  payout?: string;
  refund?: string;
  transaction?: string;
  deliveryFailed?: true;
  cancelRefused?: true;
  items?: { label: string; option: string; quantity: number }[];
  decision?: DisputeDecision;
};
export type EscrowPage = {
  receiptScope?: string;
  items: EscrowItem[];
  totals: { claimable: string; waiting: string; disputed: string; held: string };
  nextCursor?: string | null;
} & Branded;
export type ClaimBatch = {
  batchId: string;
  state: "queued" | "submitted" | "final" | "failed";
  createdAt: number;
  orders: number;
  requested: string;
  paid?: string;
  transaction?: string;
  skipped: { saleId: string; pickupNo?: number; reason: string }[];
};
export type DisputeDecision = {
  decisionId: string;
  saleId: string;
  outcome: "pay-merchant" | "refund-buyer";
  reason: string;
  state: "proposed" | "approved" | "submitted" | "final" | "rejected" | "failed";
  canApprove: boolean;
  transaction?: string;
  resolveBy: number;
};

export const escrowStages: Record<EscrowItem["status"], string> = {
  held: "전달 전 보관",
  waiting: "확인 기간 중",
  claimable: "청구 가능",
  claiming: "청구 처리 중",
  disputed: "이의 접수",
  paid: "대금 수령",
  refunded: "구매자 환불",
};
export const payouts: Record<string, string> = {
  receipt: "구매자가 수령을 확인해 지급됨",
  claim: "매장 청구로 지급됨",
  arbiter: "이의 판정으로 지급됨",
  "dispute-withdrawn": "구매자가 이의를 취소해 지급됨",
};
export const refunds: Record<string, string> = {
  store: "매장이 이행하지 못해 환불됨",
  undelivered: "전달 기한이 지나 자동 환불됨",
  arbiter: "이의 판정으로 환불됨",
  "dispute-expired": "판정 기한이 지나 자동 환불됨",
};

const seconds = () => Math.floor(Date.now() / 1000);

/** What happens next to the money of this order. */
export function escrowStage(item: EscrowItem, timezone?: string) {
  const at = (value: number) => financeTime(value, timezone);
  switch (item.status) {
    case "held":
      if (item.deliveryFailed)
        return item.deliverBy > seconds()
          ? `전달 표시가 인정되지 않았습니다. 전달 기한 ${at(item.deliverBy)}이 지나면 구매자에게 환불됩니다`
          : "전달 표시가 인정되지 않아 구매자 환불을 기다립니다";
      return item.deliverBy > seconds()
        ? `전달 기한 ${at(item.deliverBy)}`
        : "전달 기한이 지나 자동 환불을 기다립니다";
    case "waiting":
      return item.claimableAt
        ? `${at(item.claimableAt)}부터 청구 가능`
        : "청구 가능 시각 확인 중";
    case "claimable":
      return item.claimableAt
        ? `${at(item.claimableAt)}부터 청구 가능`
        : "지금 청구 가능";
    case "claiming":
      return "확정되면 매장에 지급됩니다";
    case "disputed":
      return item.resolveBy
        ? item.resolveBy > seconds()
          ? `판정 기한 ${at(item.resolveBy)}`
          : "판정 기한이 지나 자동 환불을 기다립니다"
        : "판정 기한 확인 중";
    case "paid":
      return payouts[item.payout ?? ""] ?? "지급 확정";
    case "refunded":
      return refunds[item.refund ?? ""] ?? "환불 확정";
  }
}

/** Only while the money is still kept: a settled order has nothing left to refund. */
export function RefundRefused({ item }: { item: EscrowItem }) {
  return item.cancelRefused && !["paid", "refunded"].includes(item.status) ? (
    <p>
      <strong>환불이 처리되지 않았습니다.</strong> 매장이 요청한 환불을 결제
      자산이 받아들이지 않아 간격을 두고 다시 시도합니다. 구매자에게 연락해
      환불이 늦어진다고 알리고 받을 지갑에 문제가 없는지 확인하세요.
    </p>
  ) : null;
}

export const orderName = (number?: number) =>
  number === undefined ? "주문번호 없는 주문" : `주문 ${number}번`;

export async function escrowCall<T>(operation: string, body: object) {
  const reply = await financeRequest(operation, body);
  try {
    return (await reply.json()) as T;
  } catch {
    throw new FinanceReplyLost(
      "응답을 끝까지 받지 못했습니다. 같은 요청을 다시 확인하세요."
    );
  }
}

export async function readOrders(
  branchId: string,
  status: string,
  cursor?: string
) {
  const page = await escrowCall<EscrowPage>("escrow-orders", {
    branchId,
    status,
    ...(cursor ? { cursor } : {}),
  });
  if (!Array.isArray(page.items) || !page.totals)
    throw new Error("주문 목록을 확인할 수 없습니다. 다시 조회하세요.");
  rememberTokens([{ asset: page.tokenAsset, token: page.token }]);
  return page;
}

/** Follows the cursor to the end. A cursor that repeats would never end, so it is refused. */
export async function readEveryOrder(
  branchId: string,
  status: string,
  progress?: (count: number) => void
) {
  const seen = new Set<string>();
  const items = new Map<string, EscrowItem>();
  let page = await readOrders(branchId, status);
  for (;;) {
    page.items.forEach((item) => items.set(item.saleId, item));
    progress?.(items.size);
    if (!page.nextCursor) return { ...page, items: [...items.values()] };
    if (seen.has(page.nextCursor))
      throw new Error("주문 목록을 끝까지 확인할 수 없습니다. 다시 조회하세요.");
    seen.add(page.nextCursor);
    page = await readOrders(branchId, status, page.nextCursor);
  }
}

// A request is written down before it is sent, so a lost reply is answered by the same request.
const storageError = "보호된 재시도 기록을 사용할 수 없습니다.";
export function storePending(key: string, value: unknown) {
  try {
    if (value === undefined) localStorage.removeItem(key);
    else localStorage.setItem(key, JSON.stringify(value));
  } catch {
    throw new Error(storageError);
  }
}
export function storedPending(key: string): unknown {
  try {
    const raw = localStorage.getItem(key);
    return raw ? JSON.parse(raw) : undefined;
  } catch {
    throw new Error(storageError);
  }
}

export function useTask() {
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const active = useRef(false);
  function run(task: () => Promise<unknown>) {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    setNotice("");
    task()
      .catch((error: Error) => setNotice(error.message))
      .finally(() => {
        active.current = false;
        setBusy(false);
      });
  }
  return { busy, notice, setNotice, run };
}

/** Calls `read` every few seconds while `pending`, slower after a failure, never while hidden. */
export function usePolling(
  pending: boolean,
  read: () => Promise<unknown>,
  every = 4000
) {
  const latest = useRef(read);
  latest.current = read;
  useEffect(() => {
    if (!pending) return;
    let stopped = false;
    let wait = every;
    let timer: ReturnType<typeof setTimeout>;
    const tick = async () => {
      if (!document.hidden && navigator.onLine) {
        try {
          await latest.current();
          wait = every;
        } catch {
          wait = Math.min(wait * 2, 60000);
        }
      }
      if (!stopped) timer = setTimeout(() => void tick(), wait);
    };
    timer = setTimeout(() => void tick(), wait);
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, [pending, every]);
}

export function useOnline() {
  const [online, setOnline] = useState(navigator.onLine);
  useEffect(() => {
    const update = () => setOnline(navigator.onLine);
    window.addEventListener("online", update);
    window.addEventListener("offline", update);
    return () => {
      window.removeEventListener("online", update);
      window.removeEventListener("offline", update);
    };
  }, []);
  return online;
}

/** Moves the keyboard and the screen reader to a step that just opened. */
export function StepHeading({ children }: { children: ReactNode }) {
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => heading.current?.focus(), []);
  return (
    <h3 ref={heading} tabIndex={-1}>
      {children}
    </h3>
  );
}

const branchRef = /^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/;

/** Branch choice and access states shared by both escrow pages. */
export function EscrowBranch({
  initialBranch,
  title,
  children,
}: {
  initialBranch: string;
  title: string;
  children: (
    branch: string,
    can: (capability: string) => boolean,
    scope?: BranchScope
  ) => ReactNode;
}) {
  const [branch, setBranch] = useState(initialBranch);
  const { branches, access } = useBranchAccess("finance");
  const online = useOnline();
  const scope = branches.find((row) => row.id === branch);
  // A branch entered by reference is not in the list; the server decides for it.
  const can = (capability: string) =>
    !scope || scope.capabilities.includes(capability);
  return (
    <>
      <h1>{title}</h1>
      <BranchPicker
        branches={branches}
        value={branch}
        disabled={false}
        change={setBranch}
      />
      <div role="status">
        {!online && (
          <p>
            인터넷 연결이 끊겼습니다. 마지막으로 조회한 내용을 보여 주며, 연결될
            때까지 요청을 보낼 수 없습니다.
          </p>
        )}
        {access === "loading" && <p>담당 지점을 불러오고 있습니다.</p>}
        {access === "failed" && (
          <p>담당 지점을 불러오지 못했습니다. 페이지를 새로 고치세요.</p>
        )}
        {access === "ready" && branches.length === 0 && !branchRef.test(branch) && (
          <p>
            이 계정이 재무 업무를 맡은 지점이 없습니다. 지점 관리자에게 업무
            권한을 요청하세요.
          </p>
        )}
        {scope && !can("report_read") && (
          <p>
            이 지점의 정산을 조회할 권한이 없습니다. 지점 관리자에게 조회 권한을
            요청하세요.
          </p>
        )}
      </div>
      {access !== "loading" && branchRef.test(branch) && can("report_read") ? (
        <div key={branch}>{children(branch, can, scope)}</div>
      ) : (
        access === "ready" &&
        branches.length > 0 &&
        !scope && <p>담당 지점을 선택하면 주문을 불러옵니다.</p>
      )}
    </>
  );
}

/** Opens the existing order lookup in place. */
export function OrderLookup({
  branchId,
  item,
  timezone,
}: {
  branchId: string;
  item: EscrowItem;
  timezone?: string;
}) {
  const [order, setOrder] = useState<Order>();
  const { busy, notice, run } = useTask();
  return (
    <>
      <button
        className="quiet"
        disabled={busy}
        aria-expanded={!!order}
        aria-label={`${orderName(item.pickupNo)} 상세 ${order ? "닫기" : "보기"}`}
        onClick={() => {
          if (order) return setOrder(undefined);
          run(async () =>
            setOrder(
              await escrowCall<Order>("order", {
                branchId,
                saleId: item.saleId,
              })
            )
          );
        }}
      >
        {busy ? "주문 불러오는 중" : order ? "상세 닫기" : "주문 상세"}
      </button>
      <span role="status">{notice}</span>
      {order && <OrderDetail order={order} escrow={item} timezone={timezone} />}
    </>
  );
}

export function TransactionReference({ value }: { value?: string }) {
  return value ? (
    <details>
      <summary>거래 확인 정보</summary>
      <p>{value}</p>
    </details>
  ) : null;
}

/** Base units of different assets have no sum. */
export const amountOf = (items: EscrowItem[]) =>
  new Set(items.map((item) => item.asset)).size > 1
    ? "자산이 섞여 합계 없음"
    : formatAssetAmount(
        items.reduce((sum, item) => sum + BigInt(item.amount), 0n).toString(),
        items[0]?.asset
      );

/** Refused before anything was written, so the request is over and must not be offered again. */
export const refused = (error: unknown): error is FinanceRequestError =>
  error instanceof FinanceRequestError && ["400", "404", "409"].includes(error.code);
