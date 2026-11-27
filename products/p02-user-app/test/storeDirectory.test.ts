import type {EntrancePost} from '../src/merchant/customerEntrance';
import {configuredDirectory, hoursRows, recentStoreCount, recentStores, searchStores, StoreDirectory, storeNow,
  type DayHours, type StoreStatus} from '../src/merchant/storeDirectory';

jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: null}));

const cafe = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002', bakery = '0fbc54ef-2827-4ab5-bfcd-3a27611c3003';
const open: StoreStatus = {mode: 'open', weekday: 4, time: '10:31'};
const wire = (status: object = {...open, timezone: 'Asia/Seoul'}) => ({branchRef: cafe, name: '여울 커피', kind: '카페', address: '시험시 시험로 1', status});
const day = {opens: '10:00', closes: '21:00'};
const week: DayHours[] = [day, day, day, day, day, day, null];

function directory(answers: Record<string, {status: number; body: unknown}>) {
  const calls: [string, string, string | undefined][] = [];
  const post: EntrancePost = async (path, body, session) => {
    calls.push([path, body, session]);
    return answers[path] ?? {status: 404, body: null};
  };
  return {directory: new StoreDirectory(post), calls};
}

describe('the store list', () => {
  test('is read without a body or a session, and keeps the server order', async () => {
    const {directory: stores, calls} = directory({'/v2/customer/stores': {status: 200, body: {stores: [
      {branchRef: bakery, name: '모퉁이 베이커리', kind: '베이커리', status: {mode: 'closed', weekday: 4, time: '10:31', timezone: 'Asia/Seoul'}},
      {...wire({mode: 'busy', busyMinutes: 25, weekday: 4, time: '10:31'}), hours: week}]}}});
    expect(await stores.list()).toEqual([
      {branch: bakery, name: '모퉁이 베이커리', kind: '베이커리', status: {mode: 'closed', weekday: 4, time: '10:31'}},
      {branch: cafe, name: '여울 커피', kind: '카페', address: '시험시 시험로 1', hours: week, status: {mode: 'busy', busyMinutes: 25, weekday: 4, time: '10:31'}}]);
    expect(calls).toEqual([['/v2/customer/stores', '', undefined]]);
  });

  test('leaves out a row that does not read as a store, and a store listed twice', async () => {
    const bad = [{...wire(), branchRef: 'x'}, {...wire(), name: ''}, {...wire(), name: ' 여울 '}, {...wire(), name: '여울‮커피'},
      {...wire(), name: '가'.repeat(121)}, {...wire(), kind: 7}, {...wire(), status: {mode: 'open', weekday: 7, time: '10:31'}},
      {...wire(), status: {mode: 'sleeping', weekday: 1, time: '10:31'}}, {...wire(), status: {mode: 'open', weekday: 1, time: '24:00'}}, null, 'store'];
    const {directory: stores} = directory({'/v2/customer/stores': {status: 200, body: {stores: [...bad, wire(), wire()]}}});
    expect((await stores.list()).map(store => store.branch)).toEqual([cafe]);
  });

  test('an answer that is not a list, a limit and no answer are told apart', async () => {
    for (const body of [null, {}, {stores: 'none'}, {stores: Array.from({length: 201}, () => wire())}])
      await expect(directory({'/v2/customer/stores': {status: 200, body}}).directory.list()).rejects.toThrow('CUSTOMER_ENTRANCE_UNAVAILABLE');
    await expect(directory({'/v2/customer/stores': {status: 429, body: {error: 'BUSY'}}}).directory.list()).rejects.toThrow('CUSTOMER_ENTRANCE_BUSY');
    await expect(directory({}).directory.list()).rejects.toThrow('CUSTOMER_ENTRANCE_REFUSED');
    await expect(directory({'/v2/customer/stores': {status: 500, body: null}}).directory.list()).rejects.toThrow('CUSTOMER_ENTRANCE_UNAVAILABLE');
  });

  test('a build without a trust anchor has no directory', () => {
    expect(configuredDirectory()).toBeUndefined();
    expect(configuredDirectory({origin: 'https://store.example', ca: 'MIIB' + 'A'.repeat(64), pin: 'ab'.repeat(32)})).toBeInstanceOf(StoreDirectory);
  });
});

describe('a store page', () => {
  const full = {...wire(), intro: '직접 볶은 원두로 내립니다', directions: '역 2번 출구에서 걸어서 3분', phone: '02-555-0100', amenities: '무선 인터넷',
    parking: '건물 뒤 2대', closedNote: '매주 일요일', hours: week, paymentToken: 'tKRW', operatorContact: 'help@operator.example'};

  test('carries what the store published and nothing the store left out', async () => {
    const {directory: stores, calls} = directory({[`/v2/customer/stores/${cafe}`]: {status: 200, body: full}});
    expect(await stores.page(cafe)).toEqual({branch: cafe, name: '여울 커피', kind: '카페', address: '시험시 시험로 1', hours: week, status: open,
      intro: '직접 볶은 원두로 내립니다', directions: '역 2번 출구에서 걸어서 3분', phone: '02-555-0100', amenities: '무선 인터넷',
      parking: '건물 뒤 2대', closedNote: '매주 일요일', paymentToken: 'tKRW', operatorContact: 'help@operator.example'});
    expect(calls).toEqual([[`/v2/customer/stores/${cafe}`, '', undefined]]);
    const bare = await directory({[`/v2/customer/stores/${cafe}`]: {status: 200, body: {branchRef: cafe, name: '여울 커피', status: open}}}).directory.page(cafe);
    expect(bare).toEqual({branch: cafe, name: '여울 커피', status: open});
    expect(Object.keys(bare)).toEqual(['branch', 'name', 'status']);
  });

  test('a page about another store, or with a field that does not read, is not shown', async () => {
    const answer = (body: unknown) => directory({[`/v2/customer/stores/${cafe}`]: {status: 200, body}}).directory.page(cafe);
    for (const body of [{...full, branchRef: bakery}, {...full, intro: ''}, {...full, intro: '가'.repeat(201)}, {...full, phone: 'call me'},
      {...full, hours: week.slice(1)}, {...full, hours: [...week.slice(0, 6), {opens: '10:00', closes: '10:00'}]},
      {...full, hours: [...week.slice(0, 6), {opens: '25:00', closes: '10:00'}]}, {...full, address: '시험시\n시험로'}, null])
      await expect(answer(body)).rejects.toThrow('CUSTOMER_ENTRANCE_UNAVAILABLE');
    await expect(directory({}).directory.page(cafe)).rejects.toThrow('CUSTOMER_ENTRANCE_REFUSED');
    const {directory: stores, calls} = directory({});
    await expect(stores.page('../commerce')).rejects.toThrow('CUSTOMER_ENTRANCE_REFUSED');
    expect(calls).toEqual([]);
  });

  test('a reservation button needs the page to say so', async () => {
    const answer = async (reservations: unknown) =>
      (await directory({[`/v2/customer/stores/${cafe}`]: {status: 200, body: {...full, reservations}}}).directory.page(cafe)).reservations;
    expect(await answer(true)).toBe(true);
    for (const value of [undefined, false, 'yes', 1]) expect(await answer(value)).toBeUndefined();
  });
});

describe('what a store says about now', () => {
  const at = (time: string, change: Partial<StoreStatus> = {}, hours: DayHours[] | 'unsaid' = week, weekday = 4) =>
    storeNow({status: {mode: 'open', weekday, time, ...change}, hours: hours === 'unsaid' ? undefined : hours});

  test('follows the hours of the store\'s own clock', () => {
    expect(at('10:31')).toEqual({line: '영업 중 · 21:00에 영업 종료', shut: false, dayOff: false});
    expect(at('09:59')).toEqual({line: '영업 전 · 10:00에 영업 시작', shut: true, dayOff: false});
    expect(at('21:00')).toEqual({line: '영업 종료', shut: true, dayOff: false});
    expect(at('12:00', {}, week, 6)).toEqual({line: '오늘 쉽니다', shut: true, dayOff: true});
  });

  test('says busy and paused with the hours', () => {
    expect(at('10:31', {mode: 'busy', busyMinutes: 25}).line).toBe('영업 중 · 바쁨, 약 25분 · 21:00에 영업 종료');
    expect(at('10:31', {mode: 'paused', resumeAt: '11:00'})).toEqual({line: '영업 중 · 주문 잠시 멈춤 · 11:00쯤 다시 받습니다 · 21:00에 영업 종료', shut: true, dayOff: false});
    expect(at('10:31', {mode: 'closed'})).toEqual({line: '오늘 쉽니다', shut: true, dayOff: true});
  });

  test('hours past midnight belong to the day they started on', () => {
    const late: DayHours[] = [null, null, null, null, {opens: '18:00', closes: '02:00'}, null, null];
    expect(at('23:30', {}, late, 4).line).toBe('영업 중 · 02:00에 영업 종료');
    expect(at('01:30', {}, late, 5).line).toBe('영업 중 · 02:00에 영업 종료');
    expect(at('02:00', {}, late, 5)).toEqual({line: '오늘 쉽니다', shut: true, dayOff: true});
    expect(at('17:00', {}, late, 4).line).toBe('영업 전 · 18:00에 영업 시작');
  });

  test('without hours only the order state is said', () => {
    expect(at('03:00', {}, 'unsaid')).toEqual({line: '주문 받는 중', shut: false, dayOff: false});
    expect(at('03:00', {mode: 'busy', busyMinutes: 15}, 'unsaid').line).toBe('바쁨, 약 15분');
    expect(at('03:00', {mode: 'paused'}, 'unsaid')).toEqual({line: '주문 잠시 멈춤', shut: true, dayOff: false});
  });

  test('hours by weekday read as the page lists them', () => {
    expect(hoursRows([day, day, day, day, day, day, day])).toEqual([{days: '매일', time: '10:00~21:00'}]);
    const weekend = {opens: '11:00', closes: '20:00'};
    expect(hoursRows([day, day, day, day, day, weekend, weekend])).toEqual([{days: '평일', time: '10:00~21:00'}, {days: '토·일', time: '11:00~20:00'}]);
    expect(hoursRows([null, day, day, day, day, weekend, null])).toEqual([{days: '월', time: '쉼'}, {days: '화~금', time: '10:00~21:00'},
      {days: '토', time: '11:00~20:00'}, {days: '일', time: '쉼'}]);
  });
});

describe('finding a store by name', () => {
  const stores = [{name: '여울 커피'}, {name: '그린 카페'}, {name: 'Corner Bakery'}];
  test('ignores spaces and letter case, and an empty search keeps every store', () => {
    expect(searchStores(stores, '여울커피')).toEqual([{name: '여울 커피'}]);
    expect(searchStores(stores, ' 카 페 ')).toEqual([{name: '그린 카페'}]);
    expect(searchStores(stores, 'corner')).toEqual([{name: 'Corner Bakery'}]);
    expect(searchStores(stores, '  ')).toEqual(stores);
    expect(searchStores(stores, '빵집')).toEqual([]);
  });
});

describe('recent stores', () => {
  function storage(initial = '') {
    const kept = {value: initial, names: [] as string[]};
    return {kept, loadJournal: async (name: string) => { kept.names.push(name); return kept.value; },
      saveJournal: async (name: string, value: string) => { kept.names.push(name); kept.value = value; }};
  }
  const id = (n: number) => `0fbc54ef-2827-4ab5-bfcd-3a27611c30${String(n).padStart(2, '0')}`;

  test('keeps the newest first, each store once, up to the count, in the stores journal', async () => {
    const phone = storage();
    const recent = recentStores(phone);
    expect(await recent.list()).toEqual([]);
    for (let n = 1; n <= recentStoreCount + 1; n++) await recent.remember({branch: id(n), name: `매장 ${n}`, kind: '카페'});
    await recent.remember({branch: id(recentStoreCount), name: '새 이름'});
    const rows = await recent.list();
    expect(rows).toHaveLength(recentStoreCount);
    expect(rows[0]).toEqual({branch: id(recentStoreCount), name: '새 이름'});
    expect(rows.map(row => row.branch)).not.toContain(id(1));
    expect(new Set(phone.kept.names)).toEqual(new Set(['customer-stores-v1']));
    await recent.forget();
    expect(await recent.list()).toEqual([]);
  });

  test('a journal that does not read, or a phone without storage, gives no recent store and breaks nothing', async () => {
    expect(await recentStores(storage('not json')).list()).toEqual([]);
    expect(await recentStores(storage(JSON.stringify([{branch: 'x', name: '가'}, {branch: id(1), name: ''}, {branch: id(2), name: '여울 커피'}]))).list())
      .toEqual([{branch: id(2), name: '여울 커피'}]);
    const none = recentStores(null);
    expect(await none.list()).toEqual([]);
    await expect(none.remember({branch: id(1), name: '여울 커피'})).resolves.toBeUndefined();
  });
});
