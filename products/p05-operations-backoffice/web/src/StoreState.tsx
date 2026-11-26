import { useEffect, useState } from "react";

/** What the server keeps for the branch: its order state, closed days from today, sold-out products and recent changes. */
export type OrderState = {
  mode: "open" | "busy" | "paused";
  busyMinutes?: number | null;
  resumeAt?: string | null;
  changedBy?: string | null;
  changedAt?: string | null;
  today: string;
  closedDays: { day: string; addedBy: string; addedAt: string }[];
  soldOut: { sku: string; by: string; at: string }[];
  log: { by: string; at: string; change: Record<string, unknown> }[];
  taxFreeShop?: { designation: string; designatedOn: string; refundOperator: string; by: string; at: string } | null;
  me: string;
};
type Mode = OrderState["mode"];
type Change = { mode: Mode; busyMinutes?: number; resumeAt?: string };
export type Product = { sku: string; label: string; rows: number; available: number };

const modeName: Record<Mode, string> = { open: "받는 중", busy: "바쁨", paused: "멈춤" };
const stateUrl = (branch: string) => `/v2/operations/state/${encodeURIComponent(branch)}`;

async function send(path: string, body?: unknown): Promise<OrderState> {
  const reply = await fetch(path, body === undefined ? { credentials: "same-origin" } : {
    method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
  });
  if (!reply.ok) throw new Error(reply.status === 400 ? "INVALID" : reply.status === 404 ? "DENIED" : "UNKNOWN");
  return reply.json();
}

/** "나" for the person at this page, otherwise the operator's reference, shortened. */
export const who = (state: OrderState, actor?: string | null) =>
  !actor ? "" : actor === state.me ? "나" : `운영자 ${actor.slice(-4)}`;

export const describe = (change: Change) =>
  change.mode === "busy" ? `바쁨 · 약 ${change.busyMinutes}분` :
  change.mode === "paused" ? `멈춤${change.resumeAt ? ` · ${change.resumeAt}쯤 다시 받음으로 알림` : ""}` : "받는 중";

/** One line of the record, in words. */
export function logLine(entry: OrderState["log"][number], names: Map<string, string>) {
  const c = entry.change;
  if (typeof c.mode === "string") return describe(c as Change);
  if (typeof c.closedDay === "string") return `휴무일 ${c.closed ? "더함" : "뺌"} · ${dayName(c.closedDay)}`;
  if ("taxFreeShop" in c) return c.taxFreeShop ? `면세판매장 지정 번호 저장 · ${c.taxFreeShop}` : "면세판매장 지정 번호 지움";
  if (Array.isArray(c.products))
    return `${c.soldOut ? "품절로" : "다시 팔기"} · ${(c.products as string[]).map((sku) => names.get(sku) ?? sku).join(", ")}`;
  return "바꿈";
}

const when = (at: string, zone: string) =>
  new Intl.DateTimeFormat("ko-KR", { timeZone: zone, month: "long", day: "numeric", hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(new Date(at));
/** "2026년 11월 30일 (월)": the day as written on the calendar, whatever the browser's own zone. */
const dayName = (day: string) => {
  const date = new Date(day + "T00:00:00Z");
  const at = (options: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat("ko-KR", { timeZone: "UTC", ...options }).format(date);
  return `${at({ year: "numeric", month: "long", day: "numeric" })} (${at({ weekday: "short" })})`;
};

/** Whether the branch takes orders, its closed days and sold-out products. Changed only here; every change is recorded. */
export function StoreState({ branch, zone, products, onState }: {
  branch: string; zone: string; products: Product[];
  /** Told each time the state is read or changed, so the menu editor knows the tax-free shop designation. */
  onState?: (state: OrderState) => void;
}) {
  const [state, setState] = useState<OrderState>();
  const [mode, setMode] = useState<Mode>("open");
  const [minutes, setMinutes] = useState(15);
  const [resume, setResume] = useState("");
  const [asking, setAsking] = useState<{ title: string; run: () => Promise<OrderState> }>();
  const [day, setDay] = useState("");
  const [picked, setPicked] = useState<string[]>([]);
  const [filter, setFilter] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [shop, setShop] = useState({ designation: "", designatedOn: "", refundOperator: "", confirmed: false });
  const names = new Map(products.map((p) => [p.sku, p.label]));

  const show = (next: OrderState) => {
    setState(next);
    setMode(next.mode);
    if (next.busyMinutes) setMinutes(next.busyMinutes);
    setResume(next.resumeAt ?? "");
    setShop({ designation: next.taxFreeShop?.designation ?? "", designatedOn: next.taxFreeShop?.designatedOn ?? "",
      refundOperator: next.taxFreeShop?.refundOperator ?? "", confirmed: false });
    onState?.(next);
  };
  useEffect(() => {
    send(stateUrl(branch)).then(show, () => setNotice("주문 받기 상태를 읽지 못했습니다."));
  }, [branch]);

  const run = async (work: () => Promise<OrderState>, done: string) => {
    setBusy(true);
    setNotice("");
    try {
      show(await work());
      setNotice(done);
    } catch (error) {
      setNotice((error as Error).message === "INVALID" ? "바꿀 수 없는 값입니다. 다시 확인해 주세요." : "바꾸지 못했습니다. 다시 시도해 주세요.");
    } finally {
      setBusy(false);
      setAsking(undefined);
    }
  };
  if (!state) return <section aria-label="주문 받기"><h2>주문 받기</h2><p role="status">{notice || "읽는 중"}</p></section>;

  const change: Change = mode === "busy" ? { mode, busyMinutes: minutes } : mode === "paused" ? { mode, ...(resume ? { resumeAt: resume } : {}) } : { mode };
  const same = mode === state.mode && (mode !== "busy" || minutes === state.busyMinutes) && (mode !== "paused" || resume === (state.resumeAt ?? ""));
  const soldOut = new Set(state.soldOut.map((s) => s.sku));
  const shown = products.filter((p) => !filter || p.label.includes(filter));
  const ask = (title: string, work: () => Promise<OrderState>) => setAsking({ title, run: work });

  return (
    <>
      <section aria-labelledby="order-state">
        <h2 id="order-state">지금 주문 받기</h2>
        <p>
          지금 상태: <strong>{describe({ mode: state.mode, busyMinutes: state.busyMinutes ?? undefined, resumeAt: state.resumeAt ?? undefined })}</strong>
          {state.changedAt && ` · ${when(state.changedAt, zone)} · 바꾼 사람 ${who(state, state.changedBy)}`}
        </p>
        <fieldset className="modes" disabled={busy}>
          <legend>새 상태</legend>
          <div className="option">
            <label><input type="radio" name="mode" checked={mode === "open"} onChange={() => setMode("open")} /> <strong>받는 중</strong> <small>걸리는 시간을 따로 알리지 않습니다.</small></label>
          </div>
          <div className="option">
            <label><input type="radio" name="mode" checked={mode === "busy"} onChange={() => setMode("busy")} /> <strong>바쁨</strong> <small>주문은 받고, 손님 화면에 걸리는 시간을 보입니다.</small></label>
            <span className="stepper">
              <button type="button" aria-label="5분 줄이기" disabled={mode !== "busy" || minutes <= 5} onClick={() => setMinutes(minutes - 5)}>−</button>
              <output aria-label={`약 ${minutes}분`}>{minutes}분</output>
              <button type="button" aria-label="5분 늘리기" disabled={mode !== "busy" || minutes >= 60} onClick={() => setMinutes(minutes + 5)}>+</button>
              <small>5분부터 60분까지</small>
            </span>
          </div>
          <div className="option">
            <label><input type="radio" name="mode" checked={mode === "paused"} onChange={() => setMode("paused")} /> <strong>멈춤</strong> <small>새 주문을 받지 않습니다.</small></label>
            <label>다시 받을 시각(손님에게 보일 안내) <input type="time" value={resume} disabled={mode !== "paused"} onChange={(e) => setResume(e.target.value)} /></label>
            <small>시각이 지나도 저절로 풀리지 않습니다. 여기서 "받는 중"으로 바꿔야 다시 받습니다.</small>
          </div>
        </fieldset>
        <p className="warning">이미 결제한 주문은 바뀌지 않습니다. 결제하기 전인 장바구니는 결제할 때 서버가 다시 확인합니다.</p>
        <button disabled={busy || same} onClick={() => ask(`${describe(change)}(으)로 바꿀까요?`, () => send(stateUrl(branch) + "/mode", change))}>
          {same ? "지금 상태와 같습니다" : `${describe(change)}(으)로 바꾸기`}
        </button>
      </section>

      {asking && (
        <section role="alertdialog" aria-label="바꾸기 전 확인" className="confirm">
          <h2>{asking.title}</h2>
          <p>키오스크와 휴대전화 주문에 적용됩니다. 손님 화면은 메뉴를 다시 읽을 때 바뀝니다. 이미 결제한 주문은 바뀌지 않습니다. 바꾼 사람과 시각이 기록에 남습니다.</p>
          <button disabled={busy} onClick={() => run(asking.run, "바꿨습니다.")}>바꾸기</button>{" "}
          <button disabled={busy} className="secondary" onClick={() => setAsking(undefined)}>취소</button>
        </section>
      )}

      <section aria-labelledby="closed-days">
        <h2 id="closed-days">휴무일</h2>
        <p>휴무일에는 그날 0시부터 24시까지({zone}) 새 주문을 받지 않습니다.</p>
        <ul className="live" aria-label="정한 휴무일">
          {state.closedDays.map((d) => (
            <li key={d.day}>
              <strong>{dayName(d.day)}</strong> <small>{when(d.addedAt, zone)} · 정한 사람 {who(state, d.addedBy)}</small>{" "}
              <button className="secondary" disabled={busy} aria-label={`${dayName(d.day)} 휴무 빼기`}
                onClick={() => ask(`${dayName(d.day)} 휴무를 뺄까요?`, () => send(stateUrl(branch) + "/closed-day", { day: d.day, closed: false }))}>빼기</button>
            </li>
          ))}
          {!state.closedDays.length && <li>정한 휴무일이 없습니다.</li>}
        </ul>
        <label>날짜 <input type="date" min={state.today} value={day} onChange={(e) => setDay(e.target.value)} /></label>{" "}
        <button disabled={busy || !day || day < state.today}
          onClick={() => ask(`${dayName(day)}을(를) 휴무일로 정할까요?`, () => send(stateUrl(branch) + "/closed-day", { day, closed: true }))}>휴무일 더하기</button>
      </section>

      <section aria-labelledby="sold-out">
        <h2 id="sold-out">품절 · 품목을 골라 한 번에</h2>
        <label>찾기 <input type="search" placeholder="상품 이름" value={filter} onChange={(e) => setFilter(e.target.value)} /></label>
        <ul className="live" aria-label="상품">
          {shown.map((p) => (
            <li key={p.sku}>
              <label>
                <input type="checkbox" checked={picked.includes(p.sku)}
                  onChange={(e) => setPicked(e.target.checked ? [...picked, p.sku] : picked.filter((sku) => sku !== p.sku))} />{" "}
                <strong>{p.label}</strong> <small>{p.rows > 1 ? `조합 ${p.rows}줄 · ` : ""}남은 수량 {p.available}</small>
              </label>{" "}
              <span className={soldOut.has(p.sku) ? "tag warn" : "tag"}>
                {soldOut.has(p.sku) ? (() => { const s = state.soldOut.find((x) => x.sku === p.sku)!; return `품절 · ${when(s.at, zone)} · ${who(state, s.by)}`; })() : "판매 중"}
              </span>
            </li>
          ))}
        </ul>
        <p>고른 {picked.length}개{picked.length ? `: ${picked.map((sku) => names.get(sku) ?? sku).join(", ")}` : ""}</p>
        <button disabled={busy || !picked.length}
          onClick={() => ask(`고른 ${picked.length}개를 품절로 할까요?`, async () => { const next = await send(stateUrl(branch) + "/sold-out", { products: picked, soldOut: true }); setPicked([]); return next; })}>
          고른 {picked.length}개를 품절로</button>{" "}
        <button className="secondary" disabled={busy || !picked.length}
          onClick={() => ask(`고른 ${picked.length}개를 다시 팔까요?`, async () => { const next = await send(stateUrl(branch) + "/sold-out", { products: picked, soldOut: false }); setPicked([]); return next; })}>
          고른 것을 다시 팔기</button>
        <p className="warning">품절은 그 상품의 모든 옵션 조합에 적용됩니다. 남은 수량은 그대로 두고, 다시 팔면 남은 수량대로 팝니다. 게시한 메뉴를 다시 게시하지 않아도 바로 적용됩니다.</p>
      </section>

      <section aria-labelledby="tax-free-shop">
        <h2 id="tax-free-shop">외국인 세금 환급</h2>
        <p>관할 세무서장의 지정을 받은 외국인관광객 면세판매장만 외국인 손님에게 환급 대상 물품을 알릴 수 있습니다.</p>
        {state.taxFreeShop && (
          <p>지금: 지정 번호 <strong>{state.taxFreeShop.designation}</strong> · {when(state.taxFreeShop.at, zone)} · 저장한 사람 {who(state, state.taxFreeShop.by)}</p>
        )}
        <label>지정 번호(지정서에 적힌 번호) <input value={shop.designation} maxLength={40} placeholder="예: 123-45-67890-001"
          onChange={(e) => setShop({ ...shop, designation: e.target.value.trim() })} /></label>
        <label>지정받은 날 <input type="date" max={state.today} value={shop.designatedOn} onChange={(e) => setShop({ ...shop, designatedOn: e.target.value })} /></label>
        <label>환급 창구 운영사(환급 서류를 내주는 곳) <input value={shop.refundOperator} maxLength={80} placeholder="매장이 계약한 운영사 이름"
          onChange={(e) => setShop({ ...shop, refundOperator: e.target.value })} /></label>
        <label><input type="checkbox" checked={shop.confirmed} onChange={(e) => setShop({ ...shop, confirmed: e.target.checked })} /> 지정서를 확인했고, 지정이 취소되면 바로 지우겠습니다.</label>
        <button disabled={busy || !shop.confirmed || !/^[0-9A-Za-z-]{4,40}$/.test(shop.designation) || !shop.designatedOn || !shop.refundOperator.trim()}
          onClick={() => ask(`면세판매장 지정 번호 ${shop.designation}을(를) 저장할까요?`, () => send(stateUrl(branch) + "/tax-free", { ...shop }))}>저장</button>{" "}
        {state.taxFreeShop && <button className="secondary" disabled={busy}
          onClick={() => ask("면세판매장 지정 번호를 지울까요? 모든 상품의 환급 대상 표시가 손님 화면에서 함께 꺼집니다.", () => send(stateUrl(branch) + "/tax-free", { designation: "" }))}>지정 번호 지우기</button>}
        <p className="warning">이 시스템은 국세청에 지정 여부를 묻지 않고, 매장이 적은 번호를 그대로 보입니다. 환급 금액을 계산하거나 돌려주지 않습니다. 결제 금액은 그대로이고, 환급은 매장의 환급 창구에서 합니다.</p>
        <p>손님에게 보이는 조건: 부가세가 붙은, 포장해 가져가는 물품만 대상입니다. 한 번에 15,000원 이상 사면 환급 서류를 받을 수 있고, 출국할 때 물품을 가지고 나가야 합니다.</p>
      </section>

      <section aria-labelledby="state-log">
        <h2 id="state-log">바꾼 기록</h2>
        <ul className="live" aria-label="바꾼 기록">
          {state.log.map((entry, index) => (
            <li key={index}><strong>{logLine(entry, names)}</strong> <small>{when(entry.at, zone)} · {who(state, entry.by)}</small></li>
          ))}
          {!state.log.length && <li>아직 바꾼 것이 없습니다.</li>}
        </ul>
        <p>기록은 지우거나 고칠 수 없습니다. 최근 20건을 보입니다.</p>
      </section>
      <p role="status" aria-live="polite">{notice}</p>
    </>
  );
}
