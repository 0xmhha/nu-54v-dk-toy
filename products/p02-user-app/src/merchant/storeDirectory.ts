import Http from '../specs/NativeCommerceHttp.ts';
import {approvedCustomerEntrance, type CustomerEntranceAnchor} from './customerEntranceDeployment.ts';
import {EntranceError, entranceAnchor, nativeEntrance, type EntrancePost} from './customerEntrance.ts';

// The stores a customer can pick in the app (traveller-services.md chapter 9 items 6 and 7): the list of stores
// that chose to be listed, and one store's page. Both are read from the customer entrance before any proof and
// without the customer's location. A page holds only what the store published; a field the store left empty is
// absent, and the screens leave it out rather than fill it in.

export type StoreMode = 'open' | 'busy' | 'paused' | 'closed';
/** Whether the store takes orders now, and the store's own weekday (0 is Monday) and wall clock. */
export type StoreStatus = {mode: StoreMode; busyMinutes?: number; resumeAt?: string; weekday: number; time: string};
export type DayHours = {opens: string; closes: string} | null;
/** A row of the store list. `hours` is seven days from Monday; absent when the store has not said its hours. */
export type StoreSummary = {branch: string; name: string; kind?: string; address?: string; hours?: DayHours[]; status: StoreStatus};
export type StorePage = StoreSummary & {
  intro?: string; directions?: string; phone?: string; amenities?: string; parking?: string; closedNote?: string;
  /** The token a board pays this store in, such as `tKRW`. */
  paymentToken?: string;
  /**
   * The store's number in the order contract. It is all the app takes from a server about paying: the chain, the
   * contract and the token are those of the build, and the signer and beneficiary are read from that contract.
   */
  merchantId?: string;
  operatorContact?: string;
  /** No store takes reservations in this phase; the button appears only when a page says so. */
  reservations?: boolean;
};

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const clock = /^([01][0-9]|2[0-3]):[0-5][0-9]$/;
// Control characters and the marks that reorder text have no place in a name or an address.
const unsafe = /[\u0000-\u001f\u007f-\u009f‪-‮⁦-⁩]/u;

function text(value: unknown, limit: number, required = false): string | undefined {
  if (value === undefined && !required) return undefined;
  if (typeof value !== 'string' || !value || value !== value.trim() || Array.from(value).length > limit || unsafe.test(value))
    throw new EntranceError('UNAVAILABLE');
  return value;
}

function merchantNumber(value: unknown): string | undefined {
  if (value === undefined) return undefined;
  if (typeof value !== 'string' || !/^[0-9a-f]{64}$/.test(value) || /^0+$/.test(value)) throw new EntranceError('UNAVAILABLE');
  return value;
}

function status(value: unknown): StoreStatus {
  const raw = value as Partial<StoreStatus> | null;
  if (!raw || typeof raw !== 'object' || !['open', 'busy', 'paused', 'closed'].includes(raw.mode as string) ||
      !Number.isInteger(raw.weekday) || raw.weekday! < 0 || raw.weekday! > 6 || typeof raw.time !== 'string' || !clock.test(raw.time) ||
      (raw.busyMinutes !== undefined && (!Number.isInteger(raw.busyMinutes) || raw.busyMinutes < 1 || raw.busyMinutes > 240)) ||
      (raw.resumeAt !== undefined && (typeof raw.resumeAt !== 'string' || !clock.test(raw.resumeAt))))
    throw new EntranceError('UNAVAILABLE');
  return {mode: raw.mode!, weekday: raw.weekday!, time: raw.time,
    ...(raw.mode === 'busy' && raw.busyMinutes !== undefined ? {busyMinutes: raw.busyMinutes} : {}),
    ...(raw.mode === 'paused' && raw.resumeAt !== undefined ? {resumeAt: raw.resumeAt} : {})};
}

function hours(value: unknown): DayHours[] | undefined {
  if (value === undefined) return undefined;
  if (!Array.isArray(value) || value.length !== 7) throw new EntranceError('UNAVAILABLE');
  return value.map(day => {
    if (day === null) return null;
    if (!day || typeof day !== 'object' || typeof day.opens !== 'string' || typeof day.closes !== 'string' ||
        !clock.test(day.opens) || !clock.test(day.closes) || day.opens === day.closes) throw new EntranceError('UNAVAILABLE');
    return {opens: day.opens, closes: day.closes};
  });
}

function summary(value: unknown): StoreSummary {
  const raw = value as Record<string, unknown> | null;
  if (!raw || typeof raw !== 'object' || typeof raw.branchRef !== 'string' || !uuid.test(raw.branchRef)) throw new EntranceError('UNAVAILABLE');
  const kind = text(raw.kind, 40), address = text(raw.address, 200), week = hours(raw.hours);
  return {branch: raw.branchRef, name: text(raw.name, 120, true)!, ...(kind ? {kind} : {}), ...(address ? {address} : {}),
    ...(week ? {hours: week} : {}), status: status(raw.status)};
}

function refusal(reply: {status: number}): EntranceError {
  if (reply.status === 429 || reply.status === 503) return new EntranceError('BUSY');
  if (reply.status === 400 || reply.status === 404) return new EntranceError('REFUSED');
  return new EntranceError('UNAVAILABLE');
}

export class StoreDirectory {
  constructor(private readonly post: EntrancePost) {}

  /** The listed stores, in the server's order (by name). A row that does not read as a store is left out. */
  async list(): Promise<StoreSummary[]> {
    const reply = await this.post('/v2/customer/stores', '');
    if (reply.status !== 200) throw refusal(reply);
    const rows = (reply.body as {stores?: unknown} | null)?.stores;
    if (!Array.isArray(rows) || rows.length > 200) throw new EntranceError('UNAVAILABLE');
    const stores: StoreSummary[] = [];
    for (const row of rows) {
      try {
        const store = summary(row);
        if (!stores.some(known => known.branch === store.branch)) stores.push(store);
      } catch { /* one bad row does not hide the others */ }
    }
    return stores;
  }

  /** One store's page. REFUSED: no such store. The answer must be about the store that was asked for. */
  async page(branch: string): Promise<StorePage> {
    if (!uuid.test(branch)) throw new EntranceError('REFUSED');
    const reply = await this.post(`/v2/customer/stores/${branch}`, '');
    if (reply.status !== 200) throw refusal(reply);
    const raw = reply.body as Record<string, unknown> | null;
    const base = summary(raw);
    if (base.branch !== branch) throw new EntranceError('UNAVAILABLE');
    const optional: Partial<StorePage> = {
      intro: text(raw!.intro, 200), directions: text(raw!.directions, 200), phone: text(raw!.phone, 20), amenities: text(raw!.amenities, 200),
      parking: text(raw!.parking, 200), closedNote: text(raw!.closedNote, 120),
      paymentToken: text(raw!.paymentToken, 24), merchantId: merchantNumber(raw!.merchantId), operatorContact: text(raw!.operatorContact, 120),
      reservations: raw!.reservations === true ? true : undefined,
    };
    if (optional.phone && !/^\+?[0-9][0-9 -]{5,18}[0-9]$/.test(optional.phone)) throw new EntranceError('UNAVAILABLE');
    for (const key of Object.keys(optional) as (keyof StorePage)[]) if (optional[key] === undefined) delete optional[key];
    return {...base, ...optional};
  }
}

/** The directory of this build, or undefined while no trust anchor is bound (customerEntranceDeployment.ts). */
export const configuredDirectory = (anchor: CustomerEntranceAnchor | undefined = approvedCustomerEntrance) =>
  entranceAnchor(anchor) ? new StoreDirectory(nativeEntrance(anchor)) : undefined;

const fold = (value: string) => value.normalize('NFC').toLocaleLowerCase('ko-KR').replace(/\s+/g, '');
/** Stores whose name contains what was typed, ignoring spaces and letter case. An empty search keeps them all. */
export const searchStores = <T extends {name: string}>(stores: T[], query: string): T[] => {
  const wanted = fold(query);
  return wanted ? stores.filter(store => fold(store.name).includes(wanted)) : stores;
};

export type StoreNow = {
  /** `영업 중 · 21:00에 영업 종료`, `오늘 쉽니다`, … as the store's own clock says. */
  line: string;
  /** The store does not take an order now: a closed day, outside its hours, or paused. */
  shut: boolean;
  /** Closed for the whole day, which the list marks `휴무`. */
  dayOff: boolean;
};

/** What the page and the list say about now. Without hours only the order state is said. */
export function storeNow(store: {status: StoreStatus; hours?: DayHours[]}): StoreNow {
  const {mode, weekday, time, busyMinutes, resumeAt} = store.status;
  if (mode === 'closed') return {line: '오늘 쉽니다', shut: true, dayOff: true};
  const orders = mode === 'busy' ? (busyMinutes ? `바쁨, 약 ${busyMinutes}분` : '바쁨')
    : mode === 'paused' ? (resumeAt ? `주문 잠시 멈춤 · ${resumeAt}쯤 다시 받습니다` : '주문 잠시 멈춤') : '';
  if (!store.hours) return {line: orders || '주문 받는 중', shut: mode === 'paused', dayOff: false};
  // Hours that run past midnight belong to the day they started on.
  const earlier = store.hours[(weekday + 6) % 7], today = store.hours[weekday];
  const until = earlier && earlier.closes < earlier.opens && time < earlier.closes ? earlier.closes
    : today && time >= today.opens && (today.closes < today.opens || time < today.closes) ? today.closes : undefined;
  if (until) return {line: ['영업 중', orders, `${until}에 영업 종료`].filter(Boolean).join(' · '), shut: mode === 'paused', dayOff: false};
  if (!today) return {line: '오늘 쉽니다', shut: true, dayOff: true};
  if (time < today.opens) return {line: `영업 전 · ${today.opens}에 영업 시작`, shut: true, dayOff: false};
  return {line: '영업 종료', shut: true, dayOff: false};
}

const weekdayNames = ['월', '화', '수', '목', '금', '토', '일'];
/** Hours by weekday as the page lists them: neighbouring days with the same hours are read together. */
export function hoursRows(days: DayHours[]): {days: string; time: string}[] {
  const rows: {from: number; to: number; time: string}[] = [];
  days.forEach((day, index) => {
    const time = day ? `${day.opens}~${day.closes}` : '쉼';
    const last = rows[rows.length - 1];
    if (last && last.time === time) last.to = index;
    else rows.push({from: index, to: index, time});
  });
  return rows.map(({from, to, time}) => ({
    days: from === 0 && to === 4 ? '평일' : from === 5 && to === 6 ? '토·일' : from === 0 && to === 6 ? '매일'
      : from === to ? weekdayNames[from] : `${weekdayNames[from]}~${weekdayNames[to]}`, time}));
}

/** How many stores the finder remembers. Provisional (an owner decision). */
export const recentStoreCount = 3;
export type RecentStore = {branch: string; name: string; kind?: string};
type RecentStorage = {loadJournal(name: string): Promise<string>; saveJournal(name: string, value: string): Promise<void>};
const recentJournal = 'customer-stores-v1';

/** The stores this phone opened last, newest first. They stay on the phone, in the app's sealed journal. */
export function recentStores(storage: RecentStorage | null | undefined = Http) {
  const read = async (): Promise<RecentStore[]> => {
    try {
      const rows = JSON.parse(await storage!.loadJournal(recentJournal) || '[]') as unknown;
      if (!Array.isArray(rows)) return [];
      const stores: RecentStore[] = [];
      for (const row of rows.slice(0, recentStoreCount)) {
        try {
          if (typeof row?.branch !== 'string' || !uuid.test(row.branch) || stores.some(known => known.branch === row.branch)) continue;
          const kind = text(row.kind, 40);
          stores.push({branch: row.branch, name: text(row.name, 120, true)!, ...(kind ? {kind} : {})});
        } catch { /* a row that no longer reads is dropped */ }
      }
      return stores;
    } catch {
      return [];
    }
  };
  return {
    list: read,
    /** Puts the store first. A phone that cannot keep the list still opens the store. */
    async remember(store: RecentStore): Promise<void> {
      try {
        const next = [{branch: store.branch, name: store.name, ...(store.kind ? {kind: store.kind} : {})},
          ...(await read()).filter(known => known.branch !== store.branch)].slice(0, recentStoreCount);
        await storage!.saveJournal(recentJournal, JSON.stringify(next));
      } catch { /* not kept */ }
    },
    async forget(): Promise<void> {
      try { await storage!.saveJournal(recentJournal, ''); } catch { /* nothing kept */ }
    },
  };
}
