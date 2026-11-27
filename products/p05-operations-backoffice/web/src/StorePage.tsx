import { useEffect, useState } from "react";
import { hoursLines, storePageForm, storePageProblem, storePageWire, weekdays, type StorePageForm, type StorePageSaved } from "./storePageForm";

const pageUrl = (branch: string) => `/v2/operations/store-page/${encodeURIComponent(branch)}`;

async function send(path: string, body?: unknown): Promise<StorePageSaved> {
  const reply = await fetch(path, body === undefined ? { credentials: "same-origin" } : {
    method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
  });
  if (!reply.ok) throw new Error(reply.status === 400 ? "INVALID" : reply.status === 404 ? "DENIED" : "UNKNOWN");
  return reply.json();
}

const when = (at: string, zone: string) =>
  new Intl.DateTimeFormat("ko-KR", { timeZone: zone, month: "long", day: "numeric", hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(new Date(at));

/** The store page customers see in the app: the store writes it here, and nothing else is shown about the store. */
export function StorePage({ branch, zone }: { branch: string; zone: string }) {
  const [saved, setSaved] = useState<StorePageSaved>();
  const [form, setForm] = useState<StorePageForm>();
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  const show = (page: StorePageSaved) => { setSaved(page); setForm(storePageForm(page)); };
  useEffect(() => {
    setSaved(undefined);
    setForm(undefined);
    setNotice("");
    send(pageUrl(branch)).then(show, () => setNotice("매장 화면을 읽지 못했습니다."));
  }, [branch]);

  if (!saved || !form)
    return <section aria-labelledby="store-page"><h2 id="store-page">매장 화면</h2><p role="status">{notice || "읽는 중입니다."}</p></section>;

  const set = (change: Partial<StorePageForm>) => { setForm({ ...form, ...change }); setNotice(""); };
  const setDay = (index: number, change: Partial<StorePageForm["days"][number]>) =>
    set({ days: form.days.map((day, at) => (at === index ? { ...day, ...change } : day)) });
  const problem = storePageProblem(form);
  const save = async () => {
    setBusy(true);
    setNotice("");
    try {
      show(await send(pageUrl(branch), storePageWire(form)));
      setNotice("매장 화면을 저장했습니다.");
    } catch (error) {
      setNotice((error as Error).message === "INVALID" ? "저장할 수 없는 값입니다. 다시 확인해 주세요." : "저장하지 못했습니다. 다시 시도해 주세요.");
    } finally {
      setBusy(false);
    }
  };
  const text = (label: string, field: "kind" | "intro" | "address" | "directions" | "phone" | "amenities" | "parking" | "closedNote", limit: number, hint: string) => (
    <label>{label} <input value={form[field]} maxLength={limit} placeholder={hint} onChange={(e) => set({ [field]: e.target.value })} /></label>
  );

  return (
    <section aria-labelledby="store-page">
      <h2 id="store-page">매장 화면</h2>
      <p>손님 앱의 매장 목록과 매장 화면에 보이는 내용입니다. 여기에 적은 것만 보이고, 비워 둔 칸은 손님 화면에서 빠집니다. 리뷰와 평점은 없습니다.</p>
      {saved.name
        ? <p>매장 이름: <strong>{saved.name}</strong> (지점 이름에서 바꿉니다)</p>
        : <p className="warning">지점 이름이 아직 없습니다. 지점 이름을 먼저 저장해야 매장 화면을 낼 수 있습니다.</p>}
      {saved.saved && saved.changedAt
        ? <p>지금: {saved.listed ? "매장 목록에 보임" : "매장 목록에 안 보임 · 매장 바로가기 QR로만 열림"} · {when(saved.changedAt, zone)}에 저장</p>
        : <p>아직 저장한 매장 화면이 없습니다. 손님에게는 매장 이름과 주문 받기 상태만 보입니다.</p>}
      <label><input type="checkbox" checked={form.listed} onChange={(e) => set({ listed: e.target.checked })} /> 손님 앱의 매장 목록에 보이기</label>
      {text("매장 종류", "kind", 40, "예: 카페")}
      {text("소개", "intro", 200, "매장을 한두 줄로 소개해 주세요")}
      {text("주소", "address", 200, "손님이 복사하거나 지도 앱으로 여는 주소")}
      {text("찾아가는 길", "directions", 200, "예: 역 출구에서 걷는 거리")}
      {text("전화", "phone", 20, "예: 02-555-0100")}
      {text("편의", "amenities", 200, "예: 무선 인터넷, 단체 이용")}
      {text("주차", "parking", 200, "주차 안내")}
      {text("쉬는 날", "closedNote", 120, "예: 매주 일요일")}
      <fieldset>
        <legend>요일별 영업시간 ({zone})</legend>
        <label><input type="checkbox" checked={form.hoursGiven} onChange={(e) => set({ hoursGiven: e.target.checked })} /> 영업시간을 손님에게 보이기</label>
        {form.hoursGiven && weekdays.map((name, index) => {
          const day = form.days[index];
          return (
            <div key={name} className="store-hours">
              <label><input type="checkbox" checked={day.open} onChange={(e) => setDay(index, { open: e.target.checked })} /> {name}요일 영업</label>{" "}
              {day.open && <>
                <label>{name}요일 여는 시간 <input type="time" value={day.opens} onChange={(e) => setDay(index, { opens: e.target.value })} /></label>{" "}
                <label>{name}요일 닫는 시간 <input type="time" value={day.closes} onChange={(e) => setDay(index, { closes: e.target.value })} /></label>
              </>}
            </div>
          );
        })}
        {form.hoursGiven && !problem && (
          <ul className="live" aria-label="손님에게 보이는 영업시간">
            {hoursLines(storePageWire(form).hours!).map((line) => <li key={line.days}><strong>{line.days}</strong> <small>{line.time}</small></li>)}
          </ul>
        )}
        <p>닫는 시간이 여는 시간보다 이르면 다음 날 새벽까지로 봅니다. 하루만 쉬는 날은 위의 휴무일에서 더합니다.</p>
      </fieldset>
      {problem && <p className="warning">{problem}</p>}
      <button disabled={busy || !!problem || !saved.name} onClick={() => void save()}>매장 화면 저장</button>
      <p>사진은 아직 올릴 수 없습니다. 주문 받기 상태(받는 중·바쁨·멈춤·휴무일)는 위의 주문 받기에서 바꾸며 매장 화면에 함께 보입니다.</p>
      <p role="status" aria-live="polite">{notice}</p>
    </section>
  );
}
