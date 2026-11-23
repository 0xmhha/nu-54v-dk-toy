import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  EscrowBranch,
  escrowCall,
  escrowStage,
  escrowStages,
  orderName,
  OrderLookup,
  readEveryOrder,
  readOrders,
  refused,
  RefundRefused,
  StepHeading,
  storedPending,
  storePending,
  TransactionReference,
  useOnline,
  usePolling,
  useTask,
  type DisputeDecision,
  type EscrowItem,
} from "./Escrow";
import {
  assetWarning,
  FinanceReplyLost,
  financeTime,
  formatAssetAmount,
  knownAsset,
  requestKey,
  scopedReceipt,
} from "./Finance";

const outcomes: Record<DisputeDecision["outcome"], string> = {
  "pay-merchant": "매장 지급",
  "refund-buyer": "구매자 환불",
};
const reasons: Record<string, string> = {
  not_delivered: "전달되지 않은 주문",
  delivered_proof: "전달 기록으로 전달 확인",
  wrong_item: "주문과 다른 상품 전달",
  goodwill: "구매자 배려",
};
const decisionStates: Record<DisputeDecision["state"], string> = {
  proposed: "두 번째 담당자 승인 대기",
  approved: "승인됨 · 제출 대기",
  submitted: "제출됨 · 확정 확인 중",
  final: "판정 확정",
  rejected: "반려됨",
  failed: "처리하지 못함",
};
const noAccess = "이 지점의 판정 계정이 설정되지 않았거나 권한이 없습니다.";
type Draft = Pick<DisputeDecision, "outcome" | "reason"> & {
  idempotencyKey: string;
};

function remaining(deadline: number, now: number) {
  const left = deadline - now;
  if (left <= 0) return "기한 지남";
  const days = Math.floor(left / 86400);
  const hours = Math.floor((left % 86400) / 3600);
  if (days) return `${days}일 ${hours}시간 남음`;
  const minutes = Math.floor((left % 3600) / 60);
  if (hours) return `${hours}시간 ${minutes}분 남음`;
  return minutes ? `${minutes}분 남음` : "1분 미만 남음";
}
const active = (decision?: DisputeDecision) =>
  !!decision && !["rejected", "failed"].includes(decision.state);
const onChain = (decision?: DisputeDecision) =>
  !!decision && ["approved", "submitted"].includes(decision.state);
const byDispute = (item: EscrowItem) =>
  !!item.decision ||
  ["arbiter", "dispute-withdrawn"].includes(item.payout ?? "") ||
  ["arbiter", "dispute-expired"].includes(item.refund ?? "");

export function EscrowDisputes({ initialBranch }: { initialBranch: string }) {
  return (
    <EscrowBranch initialBranch={initialBranch} title="이의 처리">
      {(branch, can, scope) => (
        <Disputes
          branchId={branch}
          canPropose={can("escrow_dispute_request")}
          approver={can("escrow_dispute_approve")}
          knownAccess={!!scope}
          timezone={scope?.timezone}
        />
      )}
    </EscrowBranch>
  );
}

function Disputes({
  branchId,
  canPropose,
  approver,
  knownAccess,
  timezone,
}: {
  branchId: string;
  canPropose: boolean;
  approver: boolean;
  knownAccess: boolean;
  timezone?: string;
}) {
  const [items, setItems] = useState<EscrowItem[]>();
  const [settled, setSettled] = useState<EscrowItem[]>([]);
  const [readAt, setReadAt] = useState<number>();
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000));
  const { busy, notice, setNotice, run } = useTask();
  const online = useOnline();

  useEffect(() => {
    const timer = setInterval(() => setNow(Math.floor(Date.now() / 1000)), 30000);
    return () => clearInterval(timer);
  }, []);

  async function load() {
    // Sorting by deadline is only true over the whole queue, not over one page of it.
    const queue = await readEveryOrder(branchId, "disputed");
    const ended = [
      ...(await readOrders(branchId, "refunded")).items,
      ...(await readOrders(branchId, "paid")).items,
    ].filter(byDispute);
    setItems(
      queue.items.sort((a, b) => (a.resolveBy ?? 0) - (b.resolveBy ?? 0))
    );
    setSettled(ended);
    setNow(Math.floor(Date.now() / 1000));
    setReadAt(Date.now() / 1000);
  }
  useEffect(() => run(load), []);

  // A decision on its way to the chain, or a deadline that passed, ends without another action here.
  const moving = (items ?? []).some((item) => onChain(item.decision));
  const overdue = (items ?? []).some((item) => (item.resolveBy ?? Infinity) <= now);
  usePolling((moving || overdue) && !busy, load, moving ? 4000 : 30000);

  return (
    <>
      <p role="status">{notice}</p>
      <section aria-busy={busy}>
        <h2>판정할 이의</h2>
        <p>
          구매자가 못 받았다고 알린 주문입니다. 한 사람이 판정을 신청하고 다른
          사람이 승인해야 돈이 움직입니다. 기한이 지나면 구매자에게 자동으로
          환불됩니다.
        </p>
        <button disabled={busy || !online} onClick={() => run(load)}>
          다시 조회
        </button>
        {readAt && <span>{financeTime(readAt, timezone)} 기준</span>}
        {!items && (
          <p>{busy ? "이의를 불러오고 있습니다." : "이의를 불러오지 못했습니다. 다시 조회하세요."}</p>
        )}
        {items?.length === 0 && <p>지금 판정할 이의가 없습니다.</p>}
        <ul className="live disputes" aria-label="이의 목록">
          {items?.map((item) => (
            <Dispute
              key={item.saleId}
              branchId={branchId}
              item={item}
              now={now}
              timezone={timezone}
              canPropose={canPropose}
              approver={approver}
              knownAccess={knownAccess}
              online={online}
              reload={load}
              announce={setNotice}
            />
          ))}
        </ul>
      </section>
      {settled.length > 0 && (
        <section>
          <h2>최근 끝난 이의</h2>
          <ul className="live disputes" aria-label="끝난 이의 목록">
            {settled.map((item) => (
              <li key={item.saleId}>
                <h3>
                  {orderName(item.pickupNo)} · {formatAssetAmount(item.amount, item.asset)}
                </h3>
                <p>
                  <strong>{escrowStages[item.status]}</strong> ·{" "}
                  {escrowStage(item, timezone)}
                </p>
                {item.decision && (
                  <p>
                    {outcomes[item.decision.outcome]} 판정 ·{" "}
                    {reasons[item.decision.reason] ?? "사유 확인 중"} ·{" "}
                    {decisionStates[item.decision.state]}
                  </p>
                )}
                <TransactionReference
                  value={item.decision?.transaction ?? item.transaction}
                />
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  );
}

function Dispute({
  branchId,
  item,
  now,
  timezone,
  canPropose,
  approver,
  knownAccess,
  online,
  reload,
  announce,
}: {
  branchId: string;
  item: EscrowItem;
  now: number;
  timezone?: string;
  canPropose: boolean;
  approver: boolean;
  knownAccess: boolean;
  online: boolean;
  reload: () => Promise<void>;
  announce: (notice: string) => void;
}) {
  const pendingKey = `nu54-escrow-resolve/${branchId}/${item.saleId}`;
  const [outcome, setOutcome] = useState<DisputeDecision["outcome"] | "">("");
  const [reason, setReason] = useState("");
  const [draft, setDraft] = useState<Draft>();
  const [sent, setSent] = useState(false);
  const [decide, setDecide] = useState<"approve" | "reject">();
  const { busy, notice, run } = useTask();
  const proposeButton = useRef<HTMLButtonElement>(null);
  const name = orderName(item.pickupNo);
  const decision = item.decision;
  const deadline = item.resolveBy ?? decision?.resolveBy;
  const expired = deadline !== undefined && deadline <= now;

  useEffect(() => {
    if (active(decision)) return;
    run(async () => {
      const stored = storedPending(pendingKey) as Draft | undefined;
      if (!stored) return;
      if (
        !outcomes[stored.outcome] ||
        !reasons[stored.reason] ||
        !/^[0-9a-f-]{36}$/.test(stored.idempotencyKey)
      )
        throw new Error("저장된 판정 신청을 확인할 수 없습니다.");
      setDraft(stored);
      setSent(true);
    });
  }, []);

  // Once a decision exists, the stored request has been answered.
  const answered = active(decision);
  useEffect(() => {
    if (!answered) return;
    try {
      closeDraft();
    } catch {
      // Storage is checked again before the next request is written.
    }
  }, [answered]);

  function review(event: FormEvent) {
    event.preventDefault();
    if (!outcome || !reason) return;
    setDraft({ outcome, reason, idempotencyKey: crypto.randomUUID() });
  }
  function closeDraft() {
    storePending(pendingKey, undefined);
    setDraft(undefined);
    setSent(false);
  }

  function propose(request: Draft) {
    run(async () => {
      storePending(pendingKey, request);
      setSent(true);
      try {
        const reply = await escrowCall<DisputeDecision>("escrow-resolve", {
          branchId,
          saleId: item.saleId,
          ...request,
        });
        if (!reply.decisionId || reply.saleId !== item.saleId)
          throw new FinanceReplyLost(
            "판정 신청 결과를 확인할 수 없습니다. 같은 신청을 다시 확인하세요."
          );
      } catch (error) {
        if (refused(error)) {
          closeDraft();
          announce(
            error.code === "404"
              ? `${name}: 신청하지 않았습니다. ${noAccess}`
              : `${name}: 이의 상태가 바뀌어 신청하지 않았습니다. 현재 상태를 확인하세요.`
          );
          await reload();
          return;
        }
        throw error;
      }
      closeDraft();
      await reload();
      announce(`${name}: 판정을 신청했습니다. 다른 담당자의 승인을 기다립니다.`);
    });
  }

  function answer(kind: "approve" | "reject") {
    if (!decision) return;
    const operation = `escrow-resolve-${kind}`;
    run(async () => {
      const key = scopedReceipt(operation, decision.decisionId);
      try {
        await escrowCall<DisputeDecision>(operation, {
          branchId,
          decisionId: decision.decisionId,
          idempotencyKey: requestKey(operation, decision.decisionId),
        });
      } catch (error) {
        if (refused(error)) {
          localStorage.removeItem(key);
          setDecide(undefined);
          announce(
            error.code === "404"
              ? `${name}: ${kind === "approve" ? "승인" : "반려"}하지 않았습니다. ${noAccess}`
              : `${name}: 판정 상태가 바뀌었습니다. 현재 상태를 확인하세요.`
          );
          await reload();
          return;
        }
        throw error;
      }
      localStorage.removeItem(key);
      setDecide(undefined);
      await reload();
      announce(
        kind === "approve"
          ? `${name}: 판정을 승인했습니다. 확정될 때까지 자동으로 다시 조회합니다.`
          : `${name}: 판정을 반려했습니다.`
      );
    });
  }

  const urgent = !expired && deadline !== undefined && deadline - now < 86400;
  return (
    <li>
      <h3>
        {name} · {formatAssetAmount(item.amount, item.asset)}
      </h3>
      <p>
        <strong>
          {deadline === undefined ? "기한 확인 중" : remaining(deadline, now)}
        </strong>
        {urgent && " · 기한 임박"}
        {deadline !== undefined && ` · 판정 기한 ${financeTime(deadline, timezone)}`}
      </p>
      <RefundRefused item={item} />
      {!knownAsset(item.asset) && <p>{assetWarning}</p>}
      {!!item.items?.length && (
        <ul aria-label={`${name} 주문 내용`}>
          {item.items.map((line, index) => (
            <li key={index}>
              {line.label}
              {line.option && line.option !== "standard" && ` · ${line.option}`} ·{" "}
              {line.quantity}개
            </li>
          ))}
        </ul>
      )}
      <OrderLookup branchId={branchId} item={item} timezone={timezone} />

      {decision && (
        <div>
          <p>
            <strong>{decisionStates[decision.state]}</strong> ·{" "}
            {outcomes[decision.outcome]} ·{" "}
            {reasons[decision.reason] ?? "사유 확인 중"}
          </p>
          {decision.state === "failed" && (
            <p>돈이 움직이지 않았습니다. 기한 안에 다시 신청할 수 있습니다.</p>
          )}
          <TransactionReference value={decision.transaction} />
        </div>
      )}

      {expired && !onChain(decision) && (
        <p>
          판정 기한이 지나 더는 판정할 수 없습니다. 대금은 구매자의 지갑으로
          자동 환불되며, 환불이 확정되면 끝난 이의로 옮겨집니다.
        </p>
      )}

      {!expired && decision?.state === "proposed" && (
        <>
          {decision.canApprove ? (
            !decide && (
              <>
                <button disabled={busy || !online} onClick={() => setDecide("approve")}>
                  승인 검토
                </button>
                <button disabled={busy || !online} onClick={() => setDecide("reject")}>
                  반려 검토
                </button>
              </>
            )
          ) : (
            <p>
              {!knownAccess
                ? "신청한 계정이거나 승인 권한이 없어 이 판정을 승인·반려할 수 없습니다. 다른 승인 담당자에게 전달하세요."
                : approver
                  ? "이 판정을 신청한 계정은 승인도 반려도 할 수 없습니다. 다른 승인 담당자에게 전달하세요."
                  : "승인과 반려는 이 지점의 이의 승인 권한을 가진 다른 담당자만 할 수 있습니다."}
            </p>
          )}
          {decide && decision.canApprove && (
            <section aria-label="판정 결재 확인">
              <StepHeading>
                {decide === "approve" ? "판정 승인" : "판정 반려"} ·{" "}
                {outcomes[decision.outcome]}
              </StepHeading>
              <p>
                {decide === "reject"
                  ? "반려하면 돈이 움직이지 않으며, 기한 안에 다시 신청할 수 있습니다."
                  : decision.outcome === "pay-merchant"
                    ? `${formatAssetAmount(item.amount, item.asset)}을 매장 수취 주소로 지급합니다. 확정되면 되돌릴 수 없습니다.`
                    : `${formatAssetAmount(item.amount, item.asset)}을 구매자의 지갑으로 환불합니다. 확정되면 되돌릴 수 없습니다.`}
              </p>
              <button disabled={busy || !online} onClick={() => answer(decide)}>
                {decide === "approve" ? "현재 계정으로 승인" : "현재 계정으로 반려"}
              </button>
              <button
                className="quiet"
                disabled={busy}
                onClick={() => setDecide(undefined)}
              >
                취소
              </button>
            </section>
          )}
        </>
      )}

      {!expired && !active(decision) && !canPropose && (
        <p>판정을 신청할 권한이 없습니다. 판정 신청 권한을 가진 담당자에게 전달하세요.</p>
      )}
      {!expired && !active(decision) && canPropose && !draft && (
        <form onSubmit={review}>
          <fieldset disabled={busy}>
            <legend>{decision ? "판정 다시 신청" : "판정 신청"}</legend>
            {(Object.keys(outcomes) as DisputeDecision["outcome"][]).map((value) => (
              <label key={value}>
                <input
                  type="radio"
                  name={`outcome-${item.saleId}`}
                  required
                  checked={outcome === value}
                  onChange={() => setOutcome(value)}
                />
                {outcomes[value]}
              </label>
            ))}
            <label>
              판정 사유
              <select
                required
                value={reason}
                onChange={(event) => setReason(event.target.value)}
              >
                <option value="">판정 사유 선택</option>
                {Object.entries(reasons).map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            </label>
            <button ref={proposeButton}>판정 신청 검토</button>
          </fieldset>
        </form>
      )}
      {!expired && !active(decision) && draft && (
        <section aria-label="판정 신청 확인">
          <StepHeading>
            {name} · {outcomes[draft.outcome]} 신청
          </StepHeading>
          <p>
            {reasons[draft.reason]} · {formatAssetAmount(item.amount, item.asset)} · 다른
            담당자가 승인해야 처리됩니다.
          </p>
          {sent && !busy && (
            <p>
              보낸 신청의 응답을 확인하지 못했습니다. 같은 신청을 다시 확인해도
              두 번 신청되지 않습니다.
            </p>
          )}
          <button disabled={busy || !online} onClick={() => propose(draft)}>
            {sent ? "같은 신청 다시 확인" : "판정 신청"}
          </button>
          {!sent && (
            <button
              className="quiet"
              disabled={busy}
              onClick={() => {
                closeDraft();
                setTimeout(() => proposeButton.current?.focus());
              }}
            >
              취소
            </button>
          )}
        </section>
      )}
      <p role="status">{notice}</p>
    </li>
  );
}
