import type {EntrancePost} from '../src/merchant/customerEntrance';
import {configuredDirectory, hoursRows, recentStoreCount, recentStores, searchStores, StoreDirectory, storeNow,
  type DayHours, type StoreStatus} from '../src/merchant/storeDirectory';

jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: null}));

const cafe = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002', bakery = '0fbc54ef-2827-4ab5-bfcd-3a27611c3003';
const open: StoreStatus = {mode: 'open', orderable: true, weekday: 4, time: '10:31'};
const wire = (status: object = {...open, timezone: 'Asia/Seoul'}) => ({branchRef: cafe, name: '여울 커피', kind: '카페', address: '시험시 시험로 1', status});
const day = {opens: '10:00', closes: '21:00'};
const week: DayHours[] = [day, day, day, day, day, day, null];

function directory(answers: Record<string, {status: number; body: unknown; retryAfter?: number}>) {
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
      {branchRef: bakery, name: '모퉁이 베이커리', kind: '베이커리', status: {mode: 'closed', orderable: false, reason: 'closed_day', weekday: 4, time: '10:31', timezone: 'Asia/Seoul'}},
      {...wire({mode: 'busy', orderable: true, busyMinutes: 25, weekday: 4, time: '10:31'}), hours: week}]}}});
    expect(await stores.list()).toEqual({stores: [
      {branch: bakery, name: '모퉁이 베이커리', kind: '베이커리', status: {mode: 'closed', orderable: false, reason: 'closed_day', weekday: 4, time: '10:31'}},
      {branch: cafe, name: '여울 커피', kind: '카페', address: '시험시 시험로 1', hours: week, status: {mode: 'busy', orderable: true, busyMinutes: 25, weekday: 4, time: '10:31'}}]});
    expect(calls).toEqual([['/v2/customer/stores', '', undefined]]);
  });

  test('goes on from the store the server names, a page at a time, until a page names none', async () => {
    const second = {...wire(), branchRef: bakery, name: '모퉁이 베이커리'};
    const {directory: stores, calls} = directory({'/v2/customer/stores': {status: 200, body: {stores: [wire()], next: cafe}},
      [`/v2/customer/stores?after=${cafe}`]: {status: 200, body: {stores: [second]}}});
    const first = await stores.list();
    expect([first.stores.map(store => store.branch), first.next]).toEqual([[cafe], cafe]);
    const rest = await stores.list(first.next);
    expect([rest.stores.map(store => store.branch), rest.next]).toEqual([[bakery], undefined]);
    expect(calls.map(call => call[0])).toEqual(['/v2/customer/stores', `/v2/customer/stores?after=${cafe}`]);
    // Only a store reference is sent as the place to go on from, and a page that would never end is not a list.
    await expect(stores.list('x&limit=9')).rejects.toThrow('CUSTOMER_ENTRANCE_REFUSED');
    expect(calls).toHaveLength(2);
    for (const body of [{stores: [wire()], next: 'x'}, {stores: [wire()], next: 7}, {stores: [], next: cafe}, {stores: [second], next: cafe}])
      await expect(directory({[`/v2/customer/stores?after=${cafe}`]: {status: 200, body}}).directory.list(cafe)).rejects.toThrow('CUSTOMER_ENTRANCE_UNAVAILABLE');
  });

  test('leaves out a row that does not read as a store, and a store listed twice', async () => {
    const bad = [{...wire(), branchRef: 'x'}, {...wire(), name: ''}, {...wire(), name: ' 여울 '}, {...wire(), name: '여울‮커피'},
      {...wire(), name: '가'.repeat(121)}, {...wire(), kind: 7}, {...wire(), status: {...open, weekday: 7}},
      {...wire(), status: {...open, mode: 'sleeping', weekday: 1}}, {...wire(), status: {...open, weekday: 1, time: '24:00'}}, null, 'store'];
    const {directory: stores} = directory({'/v2/customer/stores': {status: 200, body: {stores: [...bad, wire(), wire()]}}});
    expect((await stores.list()).stores.map(store => store.branch)).toEqual([cafe]);
  });

  test('a status reads only when it says whether an order can be placed, and its mode agrees', async () => {
    const read = async (status: object) =>
      (await directory({'/v2/customer/stores': {status: 200, body: {stores: [wire(status)]}}}).directory.list()).stores[0]?.status;
    const at = {weekday: 4, time: '10:31'};
    for (const status of [{mode: 'open', orderable: true, ...at}, {mode: 'busy', orderable: true, busyMinutes: 20, ...at},
      {mode: 'paused', orderable: false, reason: 'paused', resumeAt: '11:00', ...at}, {mode: 'closed', orderable: false, reason: 'closed_day', ...at},
      {mode: 'closed', orderable: false, reason: 'no_menu', ...at}, {mode: 'closed', orderable: false, reason: 'outside_hours', ...at}])
      expect(await read({...status, timezone: 'Asia/Seoul'})).toEqual(status);
    // An older answer without the word, a mode that reads open while no order can be placed, and a reason of another kind.
    for (const status of [{mode: 'open', ...at}, {mode: 'open', orderable: 'yes', ...at}, {mode: 'open', orderable: false, reason: 'no_menu', ...at},
      {mode: 'busy', orderable: false, reason: 'outside_hours', ...at}, {mode: 'closed', orderable: true, ...at}, {mode: 'closed', orderable: false, ...at},
      {mode: 'paused', orderable: false, reason: 'closed_day', ...at}, {mode: 'closed', orderable: false, reason: 'holiday', ...at},
      {mode: 'open', orderable: true, reason: 'paused', ...at}, {mode: 'paused', orderable: true, ...at}])
      expect(await read(status)).toBeUndefined();
  });

  test('an answer that is not a list, a limit and no answer are told apart', async () => {
    for (const body of [null, {}, {stores: 'none'}, {stores: Array.from({length: 201}, () => wire())}])
      await expect(directory({'/v2/customer/stores': {status: 200, body}}).directory.list()).rejects.toThrow('CUSTOMER_ENTRANCE_UNAVAILABLE');
    await expect(directory({'/v2/customer/stores': {status: 429, body: {error: 'BUSY'}}}).directory.list()).rejects.toThrow('CUSTOMER_ENTRANCE_BUSY');
    await expect(directory({'/v2/customer/stores': {status: 503, body: {error: 'BUSY'}, retryAfter: 1}}).directory.list()).rejects.toMatchObject({code: 'BUSY', retryAfter: 1});
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
      parking: '건물 뒤 2대', closedNote: '매주 일요일', operatorContact: 'help@operator.example'});
    // An older server still names a token. It is not read: the token is the build's, never a server's word.
    expect(full.paymentToken).toBe('tKRW');
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

  test('takes the store\'s contract merchant number, and no address, token contract, signer or chain a page names', async () => {
    const answer = (more: object) => directory({[`/v2/customer/stores/${cafe}`]: {status: 200, body: {...full, ...more}}}).directory.page(cafe);
    const merchantId = 'e1' + '0'.repeat(60) + '07';
    expect((await answer({merchantId})).merchantId).toBe(merchantId);
    // What a server might add about where the money goes is not read, whatever it is called.
    const page = await answer({merchantId, contract: '0x' + '3'.repeat(40), escrow: '0x' + '3'.repeat(40), asset: '0x' + '2'.repeat(40), token: {decimals: 2},
      signer: '0x' + '5'.repeat(40), beneficiary: '0x' + '4'.repeat(40), chain: 1, chainId: 1, rpc: 'https://rpc.example', payment: {contract: '0x' + '3'.repeat(40)}});
    expect(Object.keys(page).sort()).toEqual(['address', 'amenities', 'branch', 'closedNote', 'directions', 'hours', 'intro', 'kind', 'merchantId', 'name',
      'operatorContact', 'parking', 'phone', 'status']);
    expect(JSON.stringify(await answer({merchantId, paymentToken: 'tUSD'}))).not.toContain('tUSD');
    expect(JSON.stringify(page)).not.toMatch(/0x[0-9a-f]{40}|rpc\.example/);
    for (const bad of ['', '0'.repeat(64), merchantId.toUpperCase(), '0x' + merchantId, merchantId.slice(1), 7, null])
      await expect(answer({merchantId: bad})).rejects.toThrow('CUSTOMER_ENTRANCE_UNAVAILABLE');
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
    storeNow({status: {mode: 'open', orderable: true, weekday, time, ...change}, hours: hours === 'unsaid' ? undefined : hours});
  // The menu is not on sale at this hour: what the server says of a store outside its hours.
  const outside: Partial<StoreStatus> = {mode: 'closed', orderable: false, reason: 'outside_hours'};
  const paused: Partial<StoreStatus> = {mode: 'paused', orderable: false, reason: 'paused'};

  test('follows the hours of the store\'s own clock', () => {
    expect(at('10:31')).toEqual({line: '영업 중 · 21:00에 영업 종료', shut: false, dayOff: false, note: ''});
    expect(at('09:59', outside)).toEqual({line: '영업 전 · 10:00에 영업 시작', shut: true, dayOff: false, note: '영업 전',
      why: '지금은 메뉴를 판매하는 시간이 아니라 주문을 받지 않습니다.'});
    expect(at('21:00', outside)).toMatchObject({line: '영업 종료', shut: true, dayOff: false, note: '영업 종료'});
    expect(at('12:00', outside, week, 6)).toMatchObject({line: '오늘 쉽니다', shut: true, dayOff: true, note: '오늘 쉽니다'});
  });

  test('says busy and paused with the hours', () => {
    expect(at('10:31', {mode: 'busy', busyMinutes: 25})).toEqual({line: '영업 중 · 바쁨, 약 25분 · 21:00에 영업 종료', shut: false, dayOff: false, note: '바쁨'});
    expect(at('10:31', {...paused, resumeAt: '11:00'})).toEqual({line: '영업 중 · 주문 잠시 멈춤 · 11:00쯤 다시 받습니다 · 21:00에 영업 종료', shut: true, dayOff: false,
      note: '주문 잠시 멈춤', why: '매장이 주문을 잠시 멈췄습니다. 11:00쯤 다시 받는다고 알렸습니다.'});
    expect(at('10:31', {mode: 'closed', orderable: false, reason: 'closed_day'})).toEqual({line: '오늘 쉽니다', shut: true, dayOff: true, note: '오늘 쉽니다',
      why: '오늘은 쉬는 날이라 주문을 받지 않습니다.'});
  });

  test('whether an order can be placed is the server\'s word, whatever the hours the store wrote say', () => {
    // Open by its hours, yet nothing can be ordered: the reason is said, and the store never reads as taking orders.
    expect(at('10:31', {mode: 'closed', orderable: false, reason: 'no_menu'})).toEqual({line: '영업 중 · 판매 중인 메뉴 없음 · 21:00에 영업 종료', shut: true,
      dayOff: false, note: '판매 중인 메뉴 없음', why: '지금 판매 중인 메뉴가 없어 주문을 받지 않습니다.'});
    expect(at('10:31', outside)).toMatchObject({line: '영업 중 · 지금은 주문 시간 아님 · 21:00에 영업 종료', shut: true, note: '지금은 주문 시간 아님'});
    expect(at('03:00', {mode: 'closed', orderable: false, reason: 'no_menu'}, 'unsaid')).toMatchObject({line: '판매 중인 메뉴 없음', shut: true, dayOff: false});
    // A closed mode is not a day off by itself: only a closed day, or a weekday the store is closed on, is.
    expect(at('10:31', {mode: 'closed', orderable: false, reason: 'no_menu'}).dayOff).toBe(false);
    // Closed by its hours, yet the server takes an order: the page says both and does not stop the order.
    expect(at('09:59')).toEqual({line: '영업 전 · 10:00에 영업 시작 · 주문 받는 중', shut: false, dayOff: false, note: ''});
    expect(at('12:00', {}, week, 6)).toEqual({line: '오늘 쉽니다 · 주문 받는 중', shut: false, dayOff: false, note: ''});
    expect(at('21:30', paused)).toMatchObject({line: '영업 종료', shut: true, note: '주문 잠시 멈춤', why: '매장이 주문을 잠시 멈췄습니다.'});
  });

  test('hours past midnight belong to the day they started on', () => {
    const late: DayHours[] = [null, null, null, null, {opens: '18:00', closes: '02:00'}, null, null];
    expect(at('23:30', {}, late, 4).line).toBe('영업 중 · 02:00에 영업 종료');
    expect(at('01:30', {}, late, 5).line).toBe('영업 중 · 02:00에 영업 종료');
    expect(at('02:00', outside, late, 5)).toMatchObject({line: '오늘 쉽니다', shut: true, dayOff: true});
    expect(at('17:00', outside, late, 4).line).toBe('영업 전 · 18:00에 영업 시작');
  });

  test('without hours only the order state is said', () => {
    expect(at('03:00', {}, 'unsaid')).toEqual({line: '주문 받는 중', shut: false, dayOff: false, note: ''});
    expect(at('03:00', {mode: 'busy', busyMinutes: 15}, 'unsaid').line).toBe('바쁨, 약 15분');
    expect(at('03:00', paused, 'unsaid')).toMatchObject({line: '주문 잠시 멈춤', shut: true, dayOff: false});
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
