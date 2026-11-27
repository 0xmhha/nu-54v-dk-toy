import { useEffect, useRef, useState, type FormEvent } from "react";
import { lineCount, lineName, scannedToken, sendPickupScan, type Handed, type Refusal } from "./pickupScan";

/** One of today's orders, as the store finds it by its number. */
export type CheckedOrder = { sale: string; number: number; stage: string; items?: { label: string; option?: string; quantity: number }[] };

const stageName: Record<string, string> = { accepted: "접수", preparing: "만드는 중", ready: "준비 완료", collected: "전달됨", unfulfillable: "준비할 수 없음으로 취소됨" };
const orderUrl = (branch: string, number: string) => `/v2/operations/state/${encodeURIComponent(branch)}/order?number=${encodeURIComponent(number)}`;
const clock = (zone: string, at = new Date()) =>
  new Intl.DateTimeFormat("ko-KR", { timeZone: zone, hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(at);

/** The order check: find today's order by its number, and for a ready one check the customer's pickup QR. */
export function PickupCheck({ branch, zone }: { branch: string; zone: string }) {
  const [number, setNumber] = useState("");
  const [order, setOrder] = useState<CheckedOrder>();
  const [note, setNote] = useState("");
  const [scanning, setScanning] = useState(false);
  const field = useRef<HTMLInputElement>(null);

  async function find(value: string, quiet = false) {
    if (!quiet) { setOrder(undefined); setNote(""); setScanning(false); }
    const reply = await fetch(orderUrl(branch, value), { credentials: "same-origin" }).catch(() => undefined);
    if (reply?.ok) setOrder(await reply.json());
    else if (!quiet) setNote(reply?.status === 404 ? "오늘 이 번호의 주문을 찾지 못했습니다. 수령 확인 권한이 없을 때도 이렇게 보입니다." : "주문을 읽지 못했습니다. 다시 시도해 주세요.");
  }
  const next = () => { setScanning(false); setOrder(undefined); setNumber(""); setNote(""); field.current?.focus(); };

  return (
    <section aria-labelledby="order-check">
      <h2 id="order-check">주문 확인{order ? ` · ${order.number}번` : ""}</h2>
      <p>받으러 온 손님의 주문을 번호로 열고, 준비 완료인 주문은 손님 앱의 수령 QR을 찍어 전달을 기록합니다.</p>
      <form onSubmit={(e) => { e.preventDefault(); void find(number); }}>
        <label>주문번호 <input ref={field} inputMode="numeric" pattern="[0-9]*" maxLength={5} value={number} onChange={(e) => setNumber(e.target.value.replace(/[^0-9]/g, ""))} /></label>{" "}
        <button disabled={!number}>열기</button>
      </form>
      {note && <p role="status">{note}</p>}
      {order && !scanning && (
        <div aria-label={`${order.number}번 주문`}>
          <p><strong>{order.number}번 · {(order.items ?? []).map((i) => `${i.label}${i.option ? ` ${i.option}` : ""} ×${i.quantity}`).join(", ")}</strong></p>
          <p>{stageName[order.stage] ?? order.stage}</p>
          {order.stage === "ready" ? <button onClick={() => setScanning(true)}>수령 QR 확인</button>
            : order.stage === "collected" ? <p>이미 전달이 기록된 주문입니다.</p>
            : order.stage !== "unfulfillable" && <p>아직 준비 전인 주문입니다. 준비 완료를 누른 뒤에 받을 수 있습니다.</p>}
        </div>
      )}
      {order && scanning && <PickupScanner key={order.sale} branch={branch} zone={zone} order={order}
        reread={() => find(String(order.number), true)} next={next} />}
    </section>
  );
}

// The browser's barcode reader where it has one (Chrome on Android, ChromeOS, macOS); not in TypeScript's DOM types yet.
type Detector = { detect(source: HTMLVideoElement): Promise<{ rawValue: string }[]> };
type DetectorClass = { new (options: { formats: string[] }): Detector; getSupportedFormats?(): Promise<string[]> };
const detectorClass = () => (globalThis as { BarcodeDetector?: DetectorClass }).BarcodeDetector;

type Camera = "off" | "starting" | "on" | "unavailable" | "refused";
type Result = { handed: Handed; at: Date } | { refused: Refusal; collected?: boolean };

/** Reads the pickup QR of the order that is open, from the camera or from a field a handheld scanner types into. */
function PickupScanner({ branch, zone, order, reread, next }: {
  branch: string; zone: string; order: CheckedOrder; reread: () => Promise<void>; next: () => void;
}) {
  const [camera, setCamera] = useState<Camera>("off");
  const [typed, setTyped] = useState("");
  const [wrong, setWrong] = useState(false);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<Result>();
  const video = useRef<HTMLVideoElement>(null);
  const field = useRef<HTMLInputElement>(null);
  const stream = useRef<MediaStream>(undefined);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const sending = useRef(false);
  const live = useRef(true);

  // Each start gets a number; stopping moves it on, so a camera that opens after a stop is closed at once.
  const run = useRef(0);
  const stop = () => {
    run.current++;
    clearTimeout(timer.current);
    timer.current = undefined;
    stream.current?.getTracks().forEach((track) => track.stop());
    stream.current = undefined;
    if (video.current) video.current.srcObject = null;
  };

  async function send(token: string) {
    if (sending.current) return;
    sending.current = true;
    stop();
    setCamera("off");
    setBusy(true);
    try {
      const outcome = await sendPickupScan(branch, order.sale, token);
      if (!live.current) return;
      if (outcome.kind === "handed") { setResult({ handed: outcome.handed, at: new Date() }); return; }
      // A lost answer may still have recorded the handover, so the order is read again before anything is said.
      await reread();
      setResult({ refused: outcome.reason });
    } finally {
      sending.current = false;
      if (live.current) setBusy(false);
    }
  }

  async function start() {
    stop();
    const mine = run.current;
    const current = () => live.current && mine === run.current;
    const Barcode = detectorClass();
    if (!Barcode || !navigator.mediaDevices?.getUserMedia) { setCamera("unavailable"); return; }
    setCamera("starting");
    try {
      if (Barcode.getSupportedFormats && !(await Barcode.getSupportedFormats()).includes("qr_code")) { if (current()) setCamera("unavailable"); return; }
      const detector = new Barcode({ formats: ["qr_code"] });
      const media = await navigator.mediaDevices.getUserMedia({ video: { facingMode: "environment" }, audio: false });
      if (!current() || !video.current) { media.getTracks().forEach((track) => track.stop()); return; }
      stream.current = media;
      video.current.srcObject = media;
      await video.current.play();
      if (!current()) return;
      setCamera("on");
      const look = async () => {
        if (!current() || !video.current) return;
        const codes = await detector.detect(video.current).catch(() => []);
        if (!current()) return;
        for (const code of codes) {
          const token = scannedToken(code.rawValue);
          if (token) return void send(token);
        }
        if (codes.length) setWrong(true);
        timer.current = setTimeout(look, 250);
      };
      void look();
    } catch (error) {
      if (!current()) return;
      stop();
      setCamera((error as Error)?.name === "NotAllowedError" ? "refused" : "unavailable");
    }
  }

  useEffect(() => {
    live.current = true;
    const hidden = () => { if (document.hidden) { stop(); setCamera("off"); } };
    document.addEventListener("visibilitychange", hidden);
    window.addEventListener("pagehide", stop);
    return () => {
      live.current = false;
      document.removeEventListener("visibilitychange", hidden);
      window.removeEventListener("pagehide", stop);
      stop();
    };
  }, []);
  // The camera runs while the check is on screen: it stops on a result, when the page is hidden, and when this
  // order is left; "다시 찍기" and "카메라 켜기" start it again.
  const [attempt, setAttempt] = useState(0);
  const scanning = !result;
  useEffect(() => {
    if (!scanning) return;
    void start();
    field.current?.focus();
    return stop;
  }, [scanning, attempt]);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const token = scannedToken(typed);
    setTyped("");
    if (!token) { setWrong(true); return; }
    setWrong(false);
    void send(token);
  };
  const again = () => { setResult(undefined); setWrong(false); setAttempt((n) => n + 1); };

  if (result && "handed" in result) {
    const { handed, at } = result;
    return (
      <div className="pickup-result">
        <h3>방금 찍은 QR</h3>
        <div className="handed" role="status"><strong>{handed.pickupNo}번 · 건네 주세요</strong><br />결제한 보드로 확인한 손님입니다. {clock(zone, at)}에 전달을 기록했습니다.</div>
        <ul className="live" aria-label="건넬 품목">
          {handed.items.map((line, index) => <li key={index}><strong>{lineName(line)}</strong> <small>{lineCount(line)}</small></li>)}
        </ul>
        <dl className="pickup-facts">
          <div><dt>결제 확정</dt><dd>확인됨</dd></div>
          <div><dt>전달 기록</dt><dd>{clock(zone, at)} · 이 기기</dd></div>
          <div><dt>매장이 받는 때</dt><dd>손님이 받았어요를 누르거나 확인 기간이 끝날 때</dd></div>
        </dl>
        <button className="secondary" onClick={next}>다음 손님 찍기</button>
      </div>
    );
  }
  if (result) return <Refused order={order} reason={result.refused} again={again} />;

  return (
    <div className="pickup-scan">
      <h3>{order.number}번의 수령 QR 확인</h3>
      <p>주문 확인에서 {order.number}번을 연 뒤 브라우저 카메라로 찍습니다. 다른 주문의 QR이면 받지 않습니다.</p>
      <div className="camera" aria-label="카메라 화면">
        <video ref={video} muted playsInline hidden={camera !== "on"} />
        {camera === "starting" && <p role="status">카메라를 켜고 있습니다.</p>}
        {camera === "refused" && <p role="status">카메라 사용이 허용되지 않았습니다. 아래 칸에 스캐너로 찍거나 QR 글자를 붙여 넣으세요.</p>}
        {camera === "unavailable" && <p role="status">이 브라우저에서는 카메라로 QR을 읽지 못합니다. 아래 칸에 스캐너로 찍거나 QR 글자를 붙여 넣으세요.</p>}
        {camera === "off" && !busy && <button className="secondary" onClick={() => setAttempt((n) => n + 1)}>카메라 켜기</button>}
      </div>
      <form onSubmit={submit}>
        <label>수령 QR 글자 (스캐너로 찍거나 붙여 넣기)
          <input ref={field} value={typed} disabled={busy} autoComplete="off" spellCheck={false} maxLength={200}
            onChange={(e) => setTyped(e.target.value)} placeholder="nu54://pickup/…" /></label>
        <button disabled={busy || !typed.trim()}>확인</button>
      </form>
      {busy && <p role="status">서버에 확인하고 있습니다.</p>}
      {wrong && !busy && <p role="status">수령 QR이 아닙니다. 손님 앱의 수령 QR을 찍어 주세요.</p>}
      <p className="warning">번호만 말하는 손님에게는 건네지 마세요. 수령 QR은 그 주문을 결제한 보드로 확인한 휴대전화에만 나옵니다.</p>
      <button className="secondary" disabled={busy} onClick={next}>닫기</button>
    </div>
  );
}

const cases: { reason: Refusal | "used" | "other-store"; title: string; detail: (n: number) => string }[] = [
  { reason: "unusable", title: "시간이 지난 QR", detail: () => "QR은 1분 안팎만 씁니다. 캡처한 화면도 여기에 해당합니다." },
  { reason: "used", title: "이미 쓴 QR", detail: () => "이 주문은 이미 전달이 기록되었습니다." },
  { reason: "other-order", title: "다른 주문의 QR", detail: (n) => `지금 연 ${n}번이 아니라 다른 번호의 QR입니다. 그 주문을 열고 다시 찍으세요.` },
  { reason: "other-store", title: "다른 매장의 주문", detail: () => "이 매장에서 결제한 주문이 아닙니다." },
  { reason: "not-ready", title: "아직 준비 전인 주문", detail: () => "준비 완료를 누른 뒤에 받을 수 있습니다." },
];

/** Do not hand over. The server tells apart another order and an order not ready; the rest it answers alike. */
function Refused({ order, reason, again }: { order: CheckedOrder; reason: Refusal; again: () => void }) {
  if (reason === "denied" || reason === "unanswered")
    return (
      <div className="pickup-result">
        <div className="refused" role="alert"><strong>건네지 마세요 · 확인하지 못했습니다</strong><br />
          {reason === "denied" ? "이 지점의 수령 확인 권한이 없거나, 이 서버에서 수령 QR 확인이 꺼져 있습니다."
            : order.stage === "collected" ? "답을 받지 못했지만 이 주문은 전달이 기록되어 있습니다. 손님의 앱에서 상태를 함께 확인하세요."
            : "서버의 답을 받지 못했습니다. 다시 찍어 주세요."}</div>
        <button onClick={again}>다시 찍기</button>
      </div>
    );
  // EXPIRED also means used: when the order now reads as handed over, that is the likely case.
  const known = reason === "unusable" && order.stage === "collected" ? "used" : reason === "unusable" ? undefined : reason;
  return (
    <div className="pickup-result">
      <div className="refused" role="alert"><strong>건네지 마세요 · 쓸 수 없는 QR입니다</strong><br />손님에게 앱에서 수령 QR을 새로 받아 달라고 하세요.</div>
      <h3>이런 경우에 이 화면이 나옵니다</h3>
      <ul className="live" aria-label="쓸 수 없는 QR의 경우">
        {cases.map((item) => {
          const matched = known ? item.reason === known : ["unusable", "used", "other-store"].includes(item.reason);
          return <li key={item.reason} className={matched ? "matched" : undefined}>
            <strong>{item.title}{matched && <span className="tag warn"> · {known ? "이 경우" : "이 중 하나"}</span>}</strong>{" "}
            <small>{item.detail(order.number)}</small></li>;
        })}
      </ul>
      <button onClick={again}>다시 찍기</button>
    </div>
  );
}
