import { useEffect, useRef, useState } from "react";
import {
  amountOf,
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
  type ClaimBatch,
  type EscrowItem,
  type EscrowPage,
} from "./Escrow";
import {
  assetWarning,
  financeTime,
  formatAssetAmount,
  knownAsset,
} from "./Finance";

// A claim of more than 100 orders is several requests; each keeps its own key until it is answered.
type Chunk = { saleIds: string[]; idempotencyKey: string; sent: boolean };
type Pending = { total: number; chunks: Chunk[] };
const batchLimit = 100;

const skips: Record<string, string> = {
  "in-another-batch": "다른 청구에 이미 포함된 주문",
  "not-delivered": "아직 전달하지 않은 주문",
  "window-open": "구매자 확인 기간이 끝나지 않은 주문",
  disputed: "이의가 접수된 주문",
  settled: "이미 지급됐거나 환불된 주문",
  "other-beneficiary": "받는 주소가 다른 주문",
  unknown: "체인에서 확인하지 못한 주문",
};
const batchStates: Record<ClaimBatch["state"], string> = {
  queued: "제출 대기",
  submitted: "확정 확인 중",
  final: "처리 완료",
  failed: "처리하지 못함",
};
const open = (batch: ClaimBatch) =>
  batch.state === "queued" || batch.state === "submitted";

function validPending(value: unknown): Pending | undefined {
  if (value === undefined) return undefined;
  const pending = value as Pending;
  if (
    !Number.isSafeInteger(pending?.total) ||
    !Array.isArray(pending.chunks) ||
    !pending.chunks.length ||
    !pending.chunks.every(
      (chunk) =>
        /^[0-9a-f-]{36}$/.test(chunk?.idempotencyKey) &&
        Array.isArray(chunk.saleIds) &&
        chunk.saleIds.length > 0 &&
        chunk.saleIds.length <= batchLimit
    )
  )
    throw new Error("저장된 청구 기록을 확인할 수 없습니다.");
  return pending;
}

export function EscrowClaims({ initialBranch }: { initialBranch: string }) {
  return (
    <EscrowBranch initialBranch={initialBranch} title="대금 정산">
      {(branch, can, scope) => (
        <Claims
          branchId={branch}
          canClaim={can("escrow_claim")}
          timezone={scope?.timezone}
        />
      )}
    </EscrowBranch>
  );
}

function Claims({
  branchId,
  canClaim,
  timezone,
}: {
  branchId: string;
  canClaim: boolean;
  timezone?: string;
}) {
  const [filter, setFilter] = useState("claimable");
  const [page, setPage] = useState<EscrowPage>();
  const [loaded, setLoaded] = useState<string>();
  const [receiptScope, setReceiptScope] = useState<string>();
  const [readAt, setReadAt] = useState<number>();
  const [selected, setSelected] = useState<string[]>([]);
  const [confirm, setConfirm] = useState(false);
  const [pending, setPending] = useState<Pending>();
  const [batches, setBatches] = useState<ClaimBatch[]>();
  const [batchCursor, setBatchCursor] = useState<string | null>();
  const [progress, setProgress] = useState("");
  const { busy, notice, setNotice, run } = useTask();
  const online = useOnline();
  const [assets, setAssets] = useState<string[]>([]);
  const numbers = useRef(new Map<string, number | undefined>());
  const reviewButton = useRef<HTMLButtonElement>(null);
  const results = useRef<HTMLHeadingElement>(null);

  const current = useRef(filter);
  current.current = filter;

  function show(next: EscrowPage, status: string) {
    setLoaded(status);
    next.items.forEach((item) => numbers.current.set(item.saleId, item.pickupNo));
    setAssets((old) => [
      ...new Set([...old, ...next.items.map((item) => item.asset)]),
    ]);
    setPage(next);
    setReadAt(Date.now() / 1000);
  }
  const pendingKey = `nu54-escrow-claim/${branchId}/${receiptScope}`;
  function merge(next: ClaimBatch[]) {
    setBatches((old) => {
      const rows = new Map((old ?? []).map((row) => [row.batchId, row]));
      next.forEach((row) => rows.set(row.batchId, row));
      return [...rows.values()].sort((a, b) => b.createdAt - a.createdAt);
    });
  }
  async function readBatches(cursor?: string) {
    const reply = await escrowCall<{
      items: ClaimBatch[];
      nextCursor?: string | null;
    }>("escrow-claims", { branchId, ...(cursor ? { cursor } : {}) });
    if (!Array.isArray(reply.items))
      throw new Error("청구 결과를 확인할 수 없습니다. 다시 조회하세요.");
    merge(reply.items);
    return reply;
  }
  async function load(status = filter) {
    setSelected([]);
    setConfirm(false);
    const orders = await readOrders(branchId, status);
    const scope = /^[0-9a-f]{64}$/.test(orders.receiptScope ?? "")
      ? orders.receiptScope
      : undefined;
    let unreadable: unknown;
    try {
      setPending(
        scope
          ? validPending(storedPending(`nu54-escrow-claim/${branchId}/${scope}`))
          : undefined
      );
    } catch (error) {
      unreadable = error;
    }
    setReceiptScope(scope);
    show(orders, status);
    const history = await readBatches();
    setBatchCursor((old) => (batches ? old : history.nextCursor));
    // Totals and batches name no asset; when this filter is empty, any order of the branch does.
    if (!orders.items.length && !assets.length) {
      const any = await readOrders(branchId, "");
      setAssets([...new Set(any.items.map((item) => item.asset))]);
    }
    if (unreadable) throw unreadable;
  }
  useEffect(() => {
    run(() => load());
  }, []);

  const waiting = (batches ?? []).filter(open);
  usePolling(waiting.length > 0 && !busy, async () => {
    const next = new Map<string, ClaimBatch>();
    let cursor: string | undefined;
    // A large claim can fill more than one page of results.
    for (let read = 0; read < 10; read++) {
      const reply = await readBatches(cursor);
      reply.items.forEach((row) => next.set(row.batchId, row));
      if (!reply.nextCursor || waiting.every((row) => next.has(row.batchId))) break;
      cursor = reply.nextCursor;
    }
    const done = waiting.filter((row) => {
      const current = next.get(row.batchId);
      return current && !open(current);
    });
    if (!done.length) return;
    // Keep what the operator is reviewing; refresh only when nothing is being chosen.
    if (!confirm && !selected.length) {
      const fresh = await readOrders(branchId, filter);
      if (current.current === filter) show(fresh, filter);
    }
    setNotice(`청구 ${done.length}건의 처리가 끝났습니다. 청구 결과를 확인하세요.`);
  });

  // One branch pays in one asset. More than one seen means none of them can label a total.
  const asset = assets.length === 1 ? assets[0] : undefined;
  const money = (units: string) => formatAssetAmount(units, asset);
  const rows = (loaded === filter && page?.items) || [];
  const claimable = rows.filter((item) => item.status === "claimable");
  const chosen = claimable.filter((item) => selected.includes(item.saleId));
  const requests = Math.ceil(chosen.length / batchLimit);

  function selectEvery() {
    run(async () => {
      let every = claimable;
      if (page?.nextCursor || loaded !== "claimable") {
        setFilter("claimable");
        const all = await readEveryOrder(branchId, "claimable", (count) =>
          setProgress(`청구 가능 주문 ${count}건을 불러왔습니다.`)
        );
        show(all, "claimable");
        every = all.items.filter((item) => item.status === "claimable");
      }
      setProgress("");
      setSelected(every.map((item) => item.saleId));
      if (!every.length) setNotice("지금 청구할 수 있는 주문이 없습니다.");
    });
  }

  async function send(start: Pending) {
    let left = start;
    let accepted = 0;
    let conflicts = 0;
    let denied = 0;
    const keep = (next: Pending | undefined) => {
      storePending(pendingKey, next);
      setPending(next);
    };
    try {
      while (left.chunks.length) {
        const [chunk, ...rest] = left.chunks;
        left = { ...left, chunks: [{ ...chunk, sent: true }, ...rest] };
        keep(left);
        setProgress(
          `묶음 ${left.total - rest.length}/${left.total} 청구를 보내고 있습니다.`
        );
        try {
          const batch = await escrowCall<ClaimBatch>("escrow-claim", {
            branchId,
            saleIds: chunk.saleIds,
            idempotencyKey: chunk.idempotencyKey,
          });
          if (!batch.batchId || !batchStates[batch.state] || !Array.isArray(batch.skipped))
            throw new Error("청구 접수 결과를 확인할 수 없습니다. 같은 청구를 다시 확인하세요.");
          merge([batch]);
          accepted++;
        } catch (error) {
          // Only a refused request is known to have claimed nothing; keep every other one.
          if (!refused(error)) throw error;
          if (error.code === "404") {
            // The rest would be refused the same way, and none of it was sent.
            denied = rest.length + 1;
            keep(undefined);
            break;
          }
          conflicts++;
        }
        left = { ...left, chunks: rest };
        keep(rest.length ? left : undefined);
      }
    } finally {
      setProgress("");
    }
    await load("claimable").catch(() => undefined);
    setFilter("claimable");
    setNotice(
      [
        accepted && `묶음 ${accepted}개의 청구를 접수했습니다. 처리 결과는 청구 결과에서 확인하세요.`,
        conflicts &&
          `묶음 ${conflicts}개는 주문이 이미 다른 청구에 들어갔거나 상태가 바뀌어 접수되지 않았습니다. 청구 가능한 주문을 다시 확인하세요.`,
        denied &&
          `묶음 ${denied}개는 청구하지 않았습니다. 이 지점의 대금 청구가 설정되지 않았거나 권한이 없습니다.`,
      ]
        .filter(Boolean)
        .join(" ")
    );
    results.current?.focus();
  }

  function claim() {
    const chunks: Chunk[] = [];
    for (let i = 0; i < chosen.length; i += batchLimit)
      chunks.push({
        saleIds: chosen.slice(i, i + batchLimit).map((item) => item.saleId),
        idempotencyKey: crypto.randomUUID(),
        sent: false,
      });
    setConfirm(false);
    run(() => send({ total: chunks.length, chunks }));
  }

  function dropUnsent() {
    if (!pending) return;
    run(async () => {
      const chunks = pending.chunks.filter((chunk) => chunk.sent);
      const next = chunks.length ? { total: chunks.length, chunks } : undefined;
      storePending(pendingKey, next);
      setPending(next);
      setNotice("아직 보내지 않은 묶음을 취소했습니다.");
    });
  }

  const sentChunk = pending?.chunks.find((chunk) => chunk.sent);
  const unsent = pending?.chunks.filter((chunk) => !chunk.sent).length ?? 0;
  return (
    <>
      <p role="status">{notice}</p>
      <section aria-busy={busy}>
        <h2>보관 대금</h2>
        {page ? (
          <dl className="grid totals">
            {(
              [
                ["청구 가능", page.totals.claimable],
                ["확인 기간 중", page.totals.waiting],
                ["이의", page.totals.disputed],
                ["전달 전 보관", page.totals.held],
              ] as const
            ).map(([label, value]) => (
              <div key={label}>
                <dt>{label}</dt>
                <dd>{money(value)}</dd>
              </div>
            ))}
          </dl>
        ) : (
          <p>{busy ? "보관 대금을 불러오고 있습니다." : "보관 대금을 불러오지 못했습니다."}</p>
        )}
        {page && !knownAsset(asset) && assets.length > 0 && <p>{assetWarning}</p>}
        <button disabled={busy || !online} onClick={() => run(() => load())}>
          다시 조회
        </button>
        {readAt && <span>{financeTime(readAt, timezone)} 기준</span>}
      </section>

      {pending && !(busy && progress) && (
        <section aria-label="확인이 필요한 청구">
          <h2>확인이 필요한 청구</h2>
          <p>
            {sentChunk
              ? `보낸 청구 1건(주문 ${sentChunk.saleIds.length}건)의 응답을 받지 못했습니다. 같은 청구를 다시 확인해도 두 번 청구되지 않습니다.`
              : "보내지 않은 청구가 남아 있습니다."}
            {unsent > 0 && ` 아직 보내지 않은 묶음이 ${unsent}개 있습니다.`}
          </p>
          <button
            disabled={busy || !online}
            onClick={() => run(() => send(pending))}
          >
            {sentChunk ? "같은 청구 다시 확인" : "남은 청구 보내기"}
          </button>
          {unsent > 0 && (
            <button className="quiet" disabled={busy} onClick={dropUnsent}>
              보내지 않은 묶음 취소
            </button>
          )}
        </section>
      )}

      <section aria-busy={busy}>
        <h2>주문별 대금</h2>
        <label>
          상태
          <select
            value={filter}
            disabled={busy}
            onChange={(event) => {
              const status = event.target.value;
              setFilter(status);
              run(() => load(status));
            }}
          >
            <option value="">전체</option>
            {Object.entries(escrowStages).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
        {canClaim && receiptScope ? (
          <div className="claimbar">
            <button
              disabled={busy || !online || !page || !!pending || confirm}
              onClick={selectEvery}
            >
              청구 가능 주문 모두 선택
            </button>
            {chosen.length > 0 && (
              <button
                className="quiet"
                disabled={busy || confirm}
                onClick={() => setSelected([])}
              >
                선택 해제
              </button>
            )}
            <button
              ref={reviewButton}
              disabled={busy || !online || !chosen.length || !!pending || confirm}
              onClick={() => setConfirm(true)}
            >
              일괄 청구
            </button>
            <span role="status">
              {progress ||
                (chosen.length
                  ? `${chosen.length}건 선택 · ${amountOf(chosen)}`
                  : "선택한 주문 없음")}
            </span>
          </div>
        ) : canClaim ? (
          page &&
          !busy && (
            <p>
              이 계정의 청구 기록 범위를 확인하지 못해 청구할 수 없습니다. 다시
              조회하세요.
            </p>
          )
        ) : (
          <p>
            조회만 할 수 있습니다. 청구하려면 지점 관리자에게 대금 청구 권한을
            요청하세요.
          </p>
        )}
        {busy && pending && progress && (
          <progress
            aria-label="청구 묶음 전송"
            value={pending.total - pending.chunks.length}
            max={pending.total}
          />
        )}
        {confirm && (
          <section aria-label="일괄 청구 확인">
            <StepHeading>
              {chosen.length}건 · {amountOf(chosen)} 청구
            </StepHeading>
            <p>
              확정되면 등록된 매장 수취 주소로 지급됩니다. 그 사이 조건이 바뀐
              주문은 건너뛰고 이유를 알려 줍니다.
              {requests > 1 &&
                ` 한 번에 ${batchLimit}건까지 보낼 수 있어 ${requests}개 묶음으로 나눠 보냅니다.`}
            </p>
            <button disabled={busy || !online} onClick={claim}>
              {chosen.length}건 청구하기
            </button>
            <button
              className="quiet"
              onClick={() => {
                setConfirm(false);
                setTimeout(() => reviewButton.current?.focus());
              }}
            >
              취소
            </button>
          </section>
        )}
        {!rows.length && (
          <p>
            {busy
              ? "주문을 불러오고 있습니다."
              : loaded !== filter
                ? "주문을 불러오지 못했습니다. 다시 조회하세요."
                : filter === "claimable"
                  ? "지금 청구할 수 있는 주문이 없습니다. 전달한 주문은 구매자 확인 기간이 끝나면 여기에 나타납니다."
                  : filter
                    ? `'${escrowStages[filter as EscrowItem["status"]]}' 상태인 주문이 없습니다.`
                    : "주문 계약으로 결제한 주문이 아직 없습니다."}
          </p>
        )}
        <ul className="live ledger" aria-label="주문 목록">
          {rows.map((item) => {
            const name = orderName(item.pickupNo);
            const amount = formatAssetAmount(item.amount, item.asset);
            return (
              <li key={item.saleId}>
                {canClaim && receiptScope && item.status === "claimable" ? (
                  <label>
                    <input
                      type="checkbox"
                      aria-label={`${name}, ${amount} 선택`}
                      checked={selected.includes(item.saleId)}
                      disabled={busy || confirm || !!pending}
                      onChange={(event) =>
                        setSelected((old) =>
                          event.target.checked
                            ? [...old, item.saleId]
                            : old.filter((id) => id !== item.saleId)
                        )
                      }
                    />
                    {name}
                  </label>
                ) : (
                  <strong>{name}</strong>
                )}
                <span className="amount">{amount}</span>
                <span>{escrowStages[item.status]}</span>
                <span>
                  {escrowStage(item, timezone)}
                  <br />
                  결제 {financeTime(item.lockedAt, timezone)}
                </span>
                <RefundRefused item={item} />
                <div>
                  <OrderLookup branchId={branchId} item={item} timezone={timezone} />
                </div>
              </li>
            );
          })}
        </ul>
        {loaded === filter && page?.nextCursor && (
          <button
            disabled={busy || !online}
            onClick={() =>
              run(async () => {
                const next = await readOrders(branchId, filter, page.nextCursor!);
                show({ ...next, items: [...page.items, ...next.items] }, filter);
              })
            }
          >
            주문 더 보기
          </button>
        )}
      </section>

      <section aria-label="청구 결과">
        <h2 ref={results} tabIndex={-1}>
          청구 결과
        </h2>
        {waiting.length > 0 && (
          <p>처리가 끝날 때까지 결과를 자동으로 다시 조회합니다.</p>
        )}
        {batches?.length === 0 && (
          <p>아직 청구한 기록이 없습니다. 청구하면 묶음별 결과가 여기에 남습니다.</p>
        )}
        {!batches && <p>{busy ? "청구 결과를 불러오고 있습니다." : "청구 결과를 불러오지 못했습니다."}</p>}
        {batches?.map((batch) => (
          <Batch
            key={batch.batchId}
            batch={batch}
            numbers={numbers.current}
            money={money}
            timezone={timezone}
          />
        ))}
        {batchCursor && (
          <button
            disabled={busy || !online}
            onClick={() =>
              run(async () =>
                setBatchCursor((await readBatches(batchCursor)).nextCursor)
              )
            }
          >
            이전 청구 더 보기
          </button>
        )}
      </section>
    </>
  );
}

function Batch({
  batch,
  numbers,
  money,
  timezone,
}: {
  batch: ClaimBatch;
  numbers: Map<string, number | undefined>;
  money: (units: string) => string;
  timezone?: string;
}) {
  const reasons = [...new Set(batch.skipped.map((row) => row.reason))];
  const skipped = batch.skipped.length;
  return (
    <article className="batch">
      <h3>
        {financeTime(batch.createdAt, timezone)} 청구 · {batch.orders}건
      </h3>
      <p>
        <strong>{batchStates[batch.state]}</strong> · 청구{" "}
        {money(batch.requested)}
        {batch.paid !== undefined && ` · 지급 ${money(batch.paid)}`}
        {batch.state === "final" &&
          (skipped
            ? ` · ${skipped}건 건너뜀`
            : " · 모두 지급")}
      </p>
      {batch.state === "failed" && (
        <p>
          지급되지 않았습니다. 주문을 다시 조회해 청구 가능한 주문을 다시
          선택하세요.
        </p>
      )}
      {reasons.map((reason) => {
        const ids = batch.skipped.filter((row) => row.reason === reason);
        const known = ids
          .map((row) => row.pickupNo ?? numbers.get(row.saleId))
          .filter((number) => number !== undefined);
        return (
          <p key={reason}>
            건너뜀 {ids.length}건 · {skips[reason] ?? skips.unknown}
            {known.length > 0 && ` (${known.map((n) => `${n}번`).join(", ")})`}
          </p>
        );
      })}
      {skipped > 0 && (
        <details>
          <summary>건너뛴 주문 조회 참조</summary>
          <ul>
            {batch.skipped.map((row) => (
              <li key={row.saleId}>
                {orderName(row.pickupNo ?? numbers.get(row.saleId))} ·{" "}
                {row.saleId}
              </li>
            ))}
          </ul>
        </details>
      )}
      <TransactionReference value={batch.transaction} />
    </article>
  );
}
