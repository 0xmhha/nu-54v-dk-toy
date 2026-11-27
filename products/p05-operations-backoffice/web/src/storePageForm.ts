// The store page a store writes for the customer app (traveller-services.md chapter 9 item 7): its kind,
// introduction, address, hours, phone and amenities, whether the page is public and whether it appears in the app's
// store list. opsd checks every field again; the checks here only say what is wrong before the save.
//
// `published` is the store's own switch, off until the store turns it on: while it is off the customer app opens
// neither the store's page nor its menu, with or without the store's shortcut QR. `listed` matters only while the
// page is published.

/** One weekday: closed, or open between two wall-clock times of the store's own zone. */
export type DayHours = { opens: string; closes: string } | null;
/** The page as opsd keeps it. `hours` is seven days from Monday, or null when the store has not said its hours. */
export type StorePageWire = {
  published: boolean; listed: boolean; kind: string; intro: string; address: string; directions: string; phone: string;
  amenities: string; parking: string; closedNote: string; hours: DayHours[] | null;
};
export type StorePageSaved = StorePageWire & { name: string | null; saved: boolean; changedBy?: string | null; changedAt?: string | null };

export const weekdays = ["월", "화", "수", "목", "금", "토", "일"] as const;
export type DayForm = { open: boolean; opens: string; closes: string };
export type StorePageForm = Omit<StorePageWire, "hours"> & { hoursGiven: boolean; days: DayForm[] };

const clock = /^([01][0-9]|2[0-3]):[0-5][0-9]$/;
// One visible line: no control character, and none of the characters that do not show but change what a customer
// reads. That is the format characters (zero-width and direction marks, the soft hyphen, the byte order mark), line
// and paragraph separators, every character a display leaves out (the Hangul fillers, variation selectors, the
// combining grapheme joiner) and every blank other than the ordinary space (the no-break space and the other
// spaces, the blank Braille pattern). opsd holds the same list (ledger.store_text_plain) and decides.
const control = /\p{Cc}/u;
const hidden = /[\p{Cf}\p{Zl}\p{Zp}\p{Default_Ignorable_Code_Point}\u00a0\u1680\u2000-\u200a\u202f\u205f\u3000\u2800]/u;
const texts = { kind: 40, intro: 200, address: 200, directions: 200, amenities: 200, parking: 200, closedNote: 120 } as const;

const trimSpaces = (value: string) => value.replace(/^ +| +$/g, "");

/** The form for a saved page. Days without hours start as 10:00 to 21:00, which is sent only once the day is open. */
export function storePageForm(page: StorePageWire): StorePageForm {
  const { hours, ...rest } = page;
  const days = weekdays.map((_, index) => {
    const day = hours?.[index];
    return day ? { open: true, opens: day.opens, closes: day.closes } : { open: false, opens: "10:00", closes: "21:00" };
  });
  return { ...rest, hoursGiven: !!hours, days };
}

/** Why the form cannot be saved, in the words of the page, or nothing. */
export function storePageProblem(form: StorePageForm): string | undefined {
  // Only the ordinary space is trimmed, as opsd trims it: any other blank is one of the hidden characters below.
  if (!trimSpaces(form.kind)) return "매장 종류를 적어 주세요. 예: 카페";
  for (const [field, limit] of Object.entries(texts) as [keyof typeof texts, number][]) {
    const value = trimSpaces(form[field]);
    if (Array.from(value).length > limit) return `${limit}자 안으로 줄여 주세요.`;
    if (control.test(value)) return "줄바꿈은 넣을 수 없습니다.";
    if (hidden.test(value)) return "보이지 않는 문자가 들어 있습니다. 다른 곳에서 붙여 넣은 글이라면 지우고 직접 적어 주세요.";
  }
  const phone = trimSpaces(form.phone);
  if (phone && !/^\+?[0-9][0-9 -]{5,18}[0-9]$/.test(phone)) return "전화번호는 숫자, 빈칸, 하이픈으로 적어 주세요.";
  if (form.hoursGiven)
    for (const [index, day] of form.days.entries()) {
      if (!day.open) continue;
      if (!clock.test(day.opens) || !clock.test(day.closes)) return `${weekdays[index]}요일의 시간을 확인해 주세요.`;
      if (day.opens === day.closes) return `${weekdays[index]}요일의 여는 시간과 닫는 시간이 같습니다.`;
    }
  return undefined;
}

/** What is sent: trimmed text, and hours only when the store gave them. */
export function storePageWire(form: StorePageForm): StorePageWire {
  const text = trimSpaces;
  return {
    published: form.published, listed: form.listed, kind: text(form.kind), intro: text(form.intro), address: text(form.address), directions: text(form.directions),
    phone: text(form.phone), amenities: text(form.amenities), parking: text(form.parking), closedNote: text(form.closedNote),
    hours: form.hoursGiven ? form.days.map((day) => (day.open ? { opens: day.opens, closes: day.closes } : null)) : null,
  };
}

/** Who can open the store's page in the customer app, as the editor says it. */
export const storePageReach = (page: Pick<StorePageWire, "published" | "listed">) =>
  !page.published ? "비공개 · 손님 앱에서 매장 화면과 메뉴가 열리지 않음"
    : page.listed ? "공개 · 매장 목록에 보임" : "공개 · 매장 목록에 안 보임 · 매장 바로가기 QR로만 열림";

/** "평일 10:00~21:00" style lines: neighbouring weekdays with the same hours are read together. */
export function hoursLines(hours: DayHours[]): { days: string; time: string }[] {
  const lines: { from: number; to: number; time: string }[] = [];
  hours.forEach((day, index) => {
    const time = day ? `${day.opens}~${day.closes}` : "쉼";
    const last = lines[lines.length - 1];
    if (last && last.time === time) last.to = index;
    else lines.push({ from: index, to: index, time });
  });
  return lines.map(({ from, to, time }) => ({ days: from === to ? weekdays[from] : `${weekdays[from]}~${weekdays[to]}`, time }));
}
