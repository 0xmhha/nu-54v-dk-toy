import { useEffect, useState } from "react";
import { BranchPicker, useAuthorizedBranches } from "./BranchPicker";

type AuditEvent = {
  kind: string;
  at: string;
  reference?: string;
  actor?: string;
  before: string;
  after: string;
};
type Access = {
  requestKey: string;
  target: string;
  role: string;
  operation: string;
  openOrders: number;
  orderRefs?: string[];
  handoffTo?: string;
  appliedAt?: string;
  acknowledgedAt?: string;
  canApprove: boolean;
};
type Proposal = {
  requestKey: string;
  target: string;
  role: string;
  operation: string;
};
type RevocationReview = {
  reviewKey: string;
  orderRefs: string[];
  handoffTo: string | null;
};
type Handoff = {
  requestKey: string;
  saleId: string;
  orderState: string;
  acceptedAt: string | null;
};
const handoffStates: Record<string, string> = {
  preview: "주문 검토 중",
  issued: "기존 견적 발급",
  checking: "원지급 확인 중",
  paid: "지급 확정",
  closed: "주문 종료",
};
const scopePages: Array<{
  page: string;
  name: string;
  capabilities: string[];
}> = [
  { page: "v2", name: "v2 지급 증거", capabilities: ["reconcile_read"] },
  { page: "v1", name: "v1 적립 인출", capabilities: ["legacy_ledger"] },
  { page: "catalog", name: "메뉴와 가격", capabilities: ["catalog"] },
  {
    page: "finance",
    name: "정산과 이행",
    capabilities: [
      "report_read",
      "refund_request",
      "refund_approve",
      "refund_submit",
    ],
  },
  {
    page: "experience",
    name: "예약과 구독",
    capabilities: ["experience_read"],
  },
  {
    page: "payout",
    name: "지급 주소",
    capabilities: ["payout_propose", "payout_approve", "payout_notice"],
  },
  { page: "rental", name: "대여 기기", capabilities: ["rental_manage"] },
  {
    page: "fota",
    name: "펌웨어 캠페인",
    capabilities: ["firmware_manage", "firmware_station"],
  },
];
const uuid = /^[0-9a-f-]{36}$/;
const actions: Record<string, string> = { register: "등록", revoke: "철회" };
const roles: Record<string, string> = { operator: "운영자", terminal: "단말" };

export function OperatorHome({
  choose,
}: {
  choose: (page: string, branch: string) => void;
}) {
  const operator = useAuthorizedBranches("operator");
  const finance = useAuthorizedBranches("finance");
  const [branch, setBranch] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [nameSaved, setNameSaved] = useState<Record<string, string>>({});
  const current = [...operator, ...finance].find((site) => site.id === branch);
  const capabilities = [...operator, ...finance]
    .filter((site) => site.id === branch)
    .flatMap((site) => site.capabilities);
  const [access, setAccess] = useState<Access[]>([]);
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [target, setTarget] = useState("");
  const [role, setRole] = useState("terminal");
  const [action, setAction] = useState("register");
  const [draft, setDraft] = useState<Proposal>();
  const [handoffTo, setHandoffTo] = useState("");
  const [review, setReview] = useState<RevocationReview>();
  const [handoffs, setHandoffs] = useState<Handoff[]>([]);
  const [decision, setDecision] = useState<Access>();
  const [start, setStart] = useState("");
  const [end, setEnd] = useState("");
  const [status, setStatus] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setAccess([]);
    setEvents([]);
    setDraft(undefined);
    setHandoffTo("");
    setReview(undefined);
    setHandoffs([]);
    setDecision(undefined);
    setStatus("");
    setTarget("");
    setDisplayName("");
  }, [branch]);
  async function request(path: string, body?: object) {
    const response = await fetch(path, {
      method: body ? "POST" : "GET",
      credentials: "same-origin",
      headers: body && { "Content-Type": "application/json" },
      body: body && JSON.stringify(body),
    });
    if (response.status === 404)
      throw new Error("지점 또는 현재 업무 권한을 확인할 수 없습니다.");
    if (!response.ok)
      throw new Error("처리 상태가 바뀌었습니다. 같은 요청을 다시 확인하세요.");
    return response.json();
  }
  async function run(task: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setStatus("");
    try {
      await task();
    } catch (error) {
      setEvents([]);
      setAccess([]);
      setHandoffs([]);
      setStatus((error as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const root = `/v2/operations/access/${encodeURIComponent(branch)}`;
  async function previewRevocation(proposal: Proposal, recipient?: string) {
    setReview((await request(root + "/revoke-review", {
      target: proposal.target,
      role: proposal.role,
      ...(recipient ? { handoffTo: recipient } : {}),
    })) as RevocationReview);
  }
  function closeDraft() {
    setDraft(undefined);
    setReview(undefined);
    setHandoffTo("");
  }
  return (
    <>
      <h1>지점 운영 업무</h1>
      <BranchPicker
        branches={Array.from(
          new Map(
            [...operator, ...finance].map((site) => [site.id, site])
          ).values()
        )}
        value={branch}
        disabled={busy}
        change={setBranch}
      />
      <p role="status">{status}</p>
      {current && (
        <section>
          <h2>{nameSaved[branch] ?? current.label}</h2>
          <p>{current.timezone}</p>
          {scopePages
            .filter((item) =>
              item.capabilities.some((permission) =>
                capabilities.includes(permission)
              )
            )
            .map((item) => (
              <button key={item.page} onClick={() => choose(item.page, branch)}>
                {item.name}
              </button>
            ))}
          {capabilities.includes("catalog") && (
            <form
              onSubmit={(event) => {
                event.preventDefault();
                void run(async () => {
                  await request(root + "/name", { label: displayName.trim() });
                  setNameSaved((record) => ({
                    ...record,
                    [branch]: displayName.trim(),
                  }));
                  setStatus("지점 표시 이름을 저장했습니다.");
                });
              }}
            >
              <label>
                지점 표시 이름
                <input
                  required
                  maxLength={120}
                  disabled={busy}
                  value={displayName}
                  onChange={(event) => setDisplayName(event.target.value)}
                />
              </label>
              <button disabled={busy}>지점 이름 저장</button>
            </form>
          )}
        </section>
      )}
      {capabilities.includes("access_manage") && (
        <section>
          <h2>직원과 단말 자격</h2>
          <button
            disabled={busy}
            onClick={() =>
              void run(async () => setAccess((await request(root)) as Access[]))
            }
          >
            같은 요청 상태 조회
          </button>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (!uuid.test(target)) return;
              const proposal = { requestKey: crypto.randomUUID(), target, role, operation: action };
              if (action === "revoke") {
                void run(async () => {
                  await previewRevocation(proposal);
                  setDraft(proposal);
                });
              } else {
                setDraft(proposal);
              }
            }}
          >
            <label>
              검증된 대상 참조
              <input
                required
                maxLength={36}
                value={target}
                disabled={busy || !!draft}
                onChange={(event) => setTarget(event.target.value)}
              />
            </label>
            <label>
              역할
              <select
                value={role}
                aria-label="역할"
                disabled={busy || !!draft}
                onChange={(event) => setRole(event.target.value)}
              >
                <option value="terminal">단말</option>
                <option value="operator">운영자</option>
              </select>
            </label>
            <label>
              변경
              <select
                value={action}
                aria-label="변경"
                disabled={busy || !!draft}
                onChange={(event) => setAction(event.target.value)}
              >
                <option value="register">등록</option>
                <option value="revoke">철회</option>
              </select>
            </label>
            <button disabled={busy || !!draft}>자격 변경 검토</button>
          </form>
          {draft && (
            <section aria-label={draft.operation === "revoke" ? "자격 회수 검토" : "자격 변경 검토"}>
              <h3>
                {roles[draft.role]} {actions[draft.operation]}
              </h3>
              <p>{draft.target} · 별도 승인 후 반영</p>
              {draft.operation === "revoke" && review && (
                <>
                  <p>별도 승인 뒤 이 자격의 신규 업무가 차단됩니다.</p>
                  {review.orderRefs.length === 0 ? (
                    <p>인계할 미결 원주문이 없습니다.</p>
                  ) : (
                    <>
                      <h4>인계할 미결 원주문</h4>
                      <ul>{review.orderRefs.map((ref) => <li key={ref}>{ref}</li>)}</ul>
                      <label>
                        인계 담당자 참조
                        <input
                          maxLength={36}
                          value={handoffTo}
                          disabled={busy}
                          onChange={(event) => {
                            setHandoffTo(event.target.value);
                            setReview({ ...review, handoffTo: null });
                          }}
                        />
                      </label>
                      <button
                        disabled={busy || !uuid.test(handoffTo)}
                        onClick={() => void run(() => previewRevocation(draft, handoffTo))}
                      >
                        인계 담당 확인
                      </button>
                      {review.handoffTo && <p>확인한 지점 담당자 {review.handoffTo}</p>}
                    </>
                  )}
                </>
              )}
              <button
                disabled={busy || (draft.operation === "revoke" &&
                  (!review || (review.orderRefs.length > 0 && !review.handoffTo)))}
                onClick={() =>
                  void run(async () => {
                    const body = review ? {
                      ...draft, reviewKey: review.reviewKey,
                      ...(review.handoffTo ? { handoffTo: review.handoffTo } : {}),
                    } : draft;
                    setAccess((await request(root + "/propose", body)) as Access[]);
                    closeDraft();
                  })
                }
              >
                검토한 요청 기록
              </button>
              <button disabled={busy} onClick={closeDraft}>
                취소
              </button>
            </section>
          )}
          {access.map((item) => (
            <article key={item.requestKey}>
              <p>
                {roles[item.role]} {actions[item.operation]} · {item.target}
              </p>
              <p>
                {item.requestKey} ·{" "}
                {item.appliedAt ? "권한 변경 확인" : "별도 승인 대기"}
              </p>
              {item.role === "terminal" && item.operation === "register" && (
                <p>
                  {item.acknowledgedAt
                    ? "등록 단말 적용 확인"
                    : "등록 단말 적용 응답 대기"}
                </p>
              )}
              {!!item.openOrders && (
                <p>미결 원주문 {item.openOrders}건
                  {item.handoffTo && ` · 인계 담당 ${item.handoffTo}`}
                </p>
              )}
              {item.canApprove && (
                <button disabled={busy} onClick={() => setDecision(item)}>
                  독립 승인 검토
                </button>
              )}
            </article>
          ))}
          {decision && (
            <section aria-label="자격 독립 승인">
              <p>
                {roles[decision.role]} {actions[decision.operation]} ·{" "}
                {decision.target}
              </p>
              {decision.handoffTo && (
                <>
                  <p>인계 담당 {decision.handoffTo}</p>
                  <ul>{decision.orderRefs?.map((ref) => <li key={ref}>{ref}</li>)}</ul>
                </>
              )}
              <button
                disabled={busy}
                onClick={() =>
                  void run(async () => {
                    setAccess(
                      (await request(root + "/approve", {
                        requestKey: decision.requestKey,
                      })) as Access[]
                    );
                    setDecision(undefined);
                  })
                }
              >
                현재 계정으로 승인
              </button>
              <button disabled={busy} onClick={() => setDecision(undefined)}>
                취소
              </button>
            </section>
          )}
        </section>
      )}
      {capabilities.includes("pickup_manage") && (
        <section aria-label="내가 인계받은 원주문">
          <h2>내가 인계받은 원주문</h2>
          <button disabled={busy} onClick={() => void run(async () =>
            setHandoffs((await request(root + "/handoffs")) as Handoff[])
          )}>
            인계 주문 조회
          </button>
          {handoffs.map((item) => (
            <article key={item.saleId}>
              <p>원주문 {item.saleId}</p>
              <p>회수 요청 {item.requestKey} · {handoffStates[item.orderState] ?? item.orderState}</p>
              {item.acceptedAt ? <p>원주문 인계 접수됨</p> : (
                <button disabled={busy} onClick={() => void run(async () => {
                  setHandoffs((await request(root + "/handoff-ack", {
                    requestKey: item.requestKey,
                    saleId: item.saleId,
                  })) as Handoff[]);
                })}>
                  이 원주문 인계 접수
                </button>
              )}
            </article>
          ))}
        </section>
      )}
      {current && capabilities.includes("audit_read") && (
        <section>
          <h2>지점 변경 이력</h2>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              void run(async () => {
                const query = new URLSearchParams({ from: start, until: end });
                const view = (await request(
                  `/v2/operations/audit/${encodeURIComponent(branch)}?${query}`
                )) as { events: AuditEvent[] };
                setEvents(view.events);
              });
            }}
          >
            <label>
              시작일
              <input
                required
                type="date"
                value={start}
                disabled={busy}
                onChange={(event) => setStart(event.target.value)}
              />
            </label>
            <label>
              종료일
              <input
                required
                type="date"
                value={end}
                disabled={busy}
                onChange={(event) => setEnd(event.target.value)}
              />
            </label>
            <button disabled={busy}>이 범위 이력 조회</button>
          </form>
          {events.map((event, index) => (
            <p key={event.kind + "/" + event.reference + "/" + index}>
              {new Date(event.at).toLocaleString("ko-KR", {
                timeZone: current.timezone,
              })}{" "}
              · {event.kind} · {event.before} → {event.after}
              {event.actor && ` · 담당 ${event.actor}`}
              {event.reference && ` · 참조 ${event.reference}`}
            </p>
          ))}
        </section>
      )}
    </>
  );
}
