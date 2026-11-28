import React from 'react';
import Renderer from 'react-test-renderer';
import {EntranceError} from '../src/merchant/customerEntrance';
import {listPages, mapLink, phoneLink, StoreFinder, StorePageView} from '../src/merchant/StoreFinder';
import {recentStores, type DayHours, type StoreDirectory, type StoreListPage, type StorePage, type StoreSummary} from '../src/merchant/storeDirectory';

jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: null}));

const cafe = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002', green = '0fbc54ef-2827-4ab5-bfcd-3a27611c3003', bakery = '0fbc54ef-2827-4ab5-bfcd-3a27611c3004';
const day = {opens: '10:00', closes: '21:00'};
const week: DayHours[] = [day, day, day, day, day, day, day];
const now = {weekday: 4, time: '10:31'};
const taking = {mode: 'open' as const, orderable: true, ...now};
const closedDay = {mode: 'closed' as const, orderable: false, reason: 'closed_day' as const, ...now};
const stores: StoreSummary[] = [
  {branch: green, name: '그린 카페', kind: '카페', address: '시험시 초록로 2', hours: week, status: {mode: 'busy', orderable: true, busyMinutes: 25, ...now}},
  {branch: bakery, name: '모퉁이 베이커리', kind: '베이커리', address: '시험시 모퉁이길 3', status: closedDay},
  {branch: cafe, name: '여울 커피', kind: '카페', address: '시험시 시험로 1', hours: week, status: taking},
];
const page: StorePage = {...stores[2], intro: '직접 볶은 원두로 내립니다', directions: '역 2번 출구에서 걸어서 3분', phone: '02-555-0100',
  amenities: '무선 인터넷, 단체 이용', parking: '건물 뒤 2대', closedNote: '명절 당일', operatorContact: 'help@operator.example'};

/** `list` is the whole list, or the pages of it by the store each goes on from (`''` is the first). */
function directory(pages: Record<string, StorePage | Error> = {[cafe]: page}, list: StoreSummary[] | Error | Record<string, StoreListPage | Error> = stores) {
  const asked: string[] = [];
  const value = {
    list: jest.fn(async (after?: string): Promise<StoreListPage> => {
      const found = list instanceof Error || Array.isArray(list) ? list : list[after ?? ''];
      if (found instanceof Error) throw found;
      return Array.isArray(found) ? {stores: found} : found;
    }),
    page: jest.fn(async (branch: string) => {
      asked.push(branch);
      const found = pages[branch];
      if (!found) throw new EntranceError('REFUSED');
      if (found instanceof Error) throw found;
      return found;
    }),
  };
  return Object.assign(value as unknown as StoreDirectory, {asked, listed: value.list});
}
function phone(initial = '') {
  let kept = initial;
  return recentStores({loadJournal: async () => kept, saveJournal: async (_name, value) => { kept = value; }});
}

describe('finding a store', () => {
  let tree: Renderer.ReactTestRenderer;
  const screen = () => JSON.stringify(tree.toJSON());
  const buttons = () => tree.root.findAll(node => node.props.accessibilityRole === 'button' && typeof node.props.onPress === 'function')
    .map(node => node.props.accessibilityLabel as string);
  const press = async (label: string) => {
    const button = tree.root.findAll(node => node.props.accessibilityRole === 'button' && node.props.accessibilityLabel === label)[0];
    await Renderer.act(async () => { button.props.onPress(); });
  };
  const type = async (value: string) => {
    const input = tree.root.findAll(node => node.props.accessibilityLabel === '매장 이름' && typeof node.props.onChangeText === 'function')[0];
    await Renderer.act(async () => { input.props.onChangeText(value); });
  };
  const render = async (props: Partial<React.ComponentProps<typeof StoreFinder>> = {}) => {
    await Renderer.act(async () => { tree = Renderer.create(<StoreFinder directory={directory()} recent={phone()} back={() => {}} {...props} />); });
  };
  afterEach(() => { Renderer.act(() => tree?.unmount()); });

  test('lists every store by name with what keeps it from taking an order, and never asks where the phone is', async () => {
    await render();
    expect(screen()).toContain('매장 찾기');
    expect(buttons().filter(label => stores.some(store => store.name === label))).toEqual(['그린 카페', '모퉁이 베이커리', '여울 커피']);
    expect(screen()).toContain('카페 · 시험시 초록로 2 · 바쁨');
    expect(screen()).toContain('베이커리 · 시험시 모퉁이길 3 · 오늘 쉽니다');
    expect(screen()).toContain('휴무');
    expect(screen()).toContain('카페 · 시험시 시험로 1');
    expect(screen()).not.toContain('카페 · 시험시 시험로 1 ·');
    expect(screen()).toContain('위치는 쓰지 않습니다. 매장을 누르면 매장 화면이 열리고, 가는 길에 미리 주문할 수 있습니다.');
    expect(screen()).toContain('매장 바로가기 QR에는 매장 번호만 들어 있습니다. 내 정보는 매장에 가지 않습니다.');
    expect(screen()).not.toContain('최근 매장');
    for (const word of ['가까운', '거리', '평점', '리뷰']) expect(screen()).not.toContain(word);
  });

  test('searches by name, and says when no store has that name', async () => {
    await render();
    await type('여울');
    expect(buttons().filter(label => stores.some(store => store.name === label))).toEqual(['여울 커피']);
    await type('국밥');
    expect(screen()).toContain('이 이름의 매장을 찾지 못했습니다.');
    await type('');
    expect(buttons()).toContain('그린 카페');
  });

  test('opens the store page, remembers the store, and shows it first next time with what it says now', async () => {
    const recent = phone();
    const stock = directory();
    await render({directory: stock, recent});
    await press('여울 커피');
    expect(stock.asked).toEqual([cafe]);
    expect(screen()).toContain('카페 · 영업 중 · 21:00에 영업 종료');
    expect(await recent.list()).toEqual([{branch: cafe, name: '여울 커피', kind: '카페'}]);
    await press('매장 찾기로');
    expect(screen()).toContain('최근 매장');
    const row = tree.root.findAll(node => node.props.testID === `recent-${cafe}` && node.props.accessibilityRole === 'button')[0];
    expect(row.props.accessibilityHint).toBe('카페 · 영업 중 · 21:00에 영업 종료');
    expect(stock.listed).toHaveBeenCalledTimes(1);
  });

  test('the customer can empty the recent stores, and they stay empty', async () => {
    const recent = phone(JSON.stringify([{branch: cafe, name: '여울 커피', kind: '카페'}]));
    await render({recent});
    expect(screen()).toContain('최근 매장');
    await press('최근 매장 지우기');
    expect(await recent.list()).toEqual([]);
    expect(screen()).not.toContain('최근 매장');
    expect(buttons()).not.toContain('최근 매장 지우기');
    expect(buttons()).toContain('여울 커피'); // the store itself is still in the list of all stores
  });

  test('a store that is gone, a busy server and no answer each say what happened', async () => {
    await render({directory: directory({[green]: new EntranceError('BUSY'), [bakery]: new EntranceError('UNAVAILABLE')})});
    await press('여울 커피');
    expect(screen()).toContain('이 매장을 찾지 못했습니다');
    await press('매장 찾기로');
    await press('그린 카페');
    expect(screen()).toContain('요청이 많습니다. 잠시 뒤 다시 시도하세요.');
    await press('매장 찾기로');
    await press('모퉁이 베이커리');
    expect(screen()).toContain('매장 화면을 불러오지 못했습니다. 연결을 확인하고 다시 시도하세요.');
    expect(buttons()).toContain('다시 불러오기');
  });

  test('a list that cannot be read offers to read it again', async () => {
    const failing = directory({}, new EntranceError('UNAVAILABLE'));
    await render({directory: failing});
    expect(screen()).toContain('매장 목록을 불러오지 못했습니다. 연결을 확인하고 다시 시도하세요.');
    await press('다시 불러오기');
    expect(failing.listed).toHaveBeenCalledTimes(2);
    await render({directory: directory({}, [])});
    expect(screen()).toContain('아직 목록에 올린 매장이 없습니다.');
  });

  describe('a list longer than one page', () => {
    const row = (n: number, name: string): StoreSummary => ({branch: `0fbc54ef-2827-4ab5-bfcd-3a27611c4${String(n).padStart(3, '0')}`, name, status: taking});
    const first = [row(1, '가람 분식'), row(2, '나루 카페')], second = [row(3, '다온 국밥')], third = [row(4, '라온 빵집')];
    const paged = (more: Record<string, StoreListPage | Error> = {}) => directory({}, {'': {stores: first, next: first[1].branch},
      [first[1].branch]: {stores: second, next: second[0].branch}, [second[0].branch]: {stores: third}, ...more});
    const names = () => buttons().filter(label => [...first, ...second, ...third].some(store => store.name === label));

    test('is never shown as the whole list: it says that more follows and reads the next page when asked', async () => {
      const stock = paged();
      await render({directory: stock});
      expect(names()).toEqual(['가람 분식', '나루 카페']);
      expect(screen()).toContain('매장 2곳을 불러왔습니다. 더 있습니다.');
      expect(stock.listed).toHaveBeenCalledTimes(1); // nothing more is read until it is asked for
      await press('매장 더 보기');
      expect(names()).toEqual(['가람 분식', '나루 카페', '다온 국밥']);
      expect(stock.listed.mock.calls.map(call => call[0])).toEqual([undefined, first[1].branch]);
      await press('매장 더 보기');
      expect(names()).toEqual(['가람 분식', '나루 카페', '다온 국밥', '라온 빵집']);
      expect(buttons()).not.toContain('매장 더 보기');
      expect(screen()).not.toContain('더 있습니다');
    });

    test('a name is looked for on every page, and "not found" is said only of the whole list', async () => {
      const stock = paged();
      await render({directory: stock});
      await type('빵집');
      expect(stock.listed.mock.calls.map(call => call[0])).toEqual([undefined, first[1].branch, second[0].branch]);
      expect(names()).toEqual(['라온 빵집']);
      await type('떡집');
      expect(screen()).toContain('이 이름의 매장을 찾지 못했습니다.');
      expect(stock.listed).toHaveBeenCalledTimes(3); // the list is complete: nothing is read again
    });

    test('a page that cannot be read stops the search, says so instead of "not found", and is read again only by hand', async () => {
      const stock = paged({[first[1].branch]: new EntranceError('BUSY', undefined, 3)});
      await render({directory: stock});
      await type('국밥');
      expect(screen()).toContain('요청이 많아 나머지 매장을 불러오지 못했습니다. 잠시 뒤 다시 시도하세요.');
      expect(screen()).not.toContain('이 이름의 매장을 찾지 못했습니다.');
      await type('국');
      await type('국밥');
      expect(stock.listed).toHaveBeenCalledTimes(2); // typing more does not ask the busy server again
      stock.listed.mockImplementationOnce(async () => ({stores: second}));
      await press('매장 더 보기');
      expect(names()).toEqual(['다온 국밥']);
    });

    test('reads at most a fixed number of pages and then says that it searched only a part', async () => {
      const page_ = (n: number): StoreListPage => ({stores: [row(n, `매장 ${n}`)], next: row(n, '').branch});
      const stock = directory({}, Object.fromEntries(Array.from({length: listPages + 5}, (_, n) => [n ? row(n, '').branch : '', page_(n + 1)])));
      await render({directory: stock});
      await type('없는 이름');
      expect(stock.listed).toHaveBeenCalledTimes(listPages);
      expect(screen()).toContain(`매장이 많아 앞쪽 ${listPages}곳만 불러왔습니다. 찾는 매장이 없으면 매장 바로가기 QR로 여세요.`);
      expect(screen()).toContain('앞쪽 매장에서는 이 이름의 매장을 찾지 못했습니다.');
      expect(buttons()).not.toContain('매장 더 보기');
    });
  });

  test('a build without the entrance address says so and asks nothing', async () => {
    await render({directory: undefined});
    expect(screen()).toContain('이 앱에는 매장 목록을 불러올 매장 서버 주소가 아직 없습니다.');
    expect(buttons()).not.toContain('다시 불러오기');
    await render({directory: undefined, initialBranch: cafe});
    expect(screen()).toContain('이 앱에는 매장 정보를 불러올 매장 서버 주소가 아직 없습니다.');
  });

  test('the shortcut QR opens that store\'s page; it carries only the store reference', async () => {
    const stock = directory();
    await render({directory: stock, scan: async () => `nu54://shop/${cafe}`});
    await press('매장 바로가기 QR 찍기');
    expect(stock.asked).toEqual([cafe]);
    expect(screen()).toContain('직접 볶은 원두로 내립니다');
  });

  test('a shortcut read outside the app opens the page first, without reading the list', async () => {
    const stock = directory();
    await render({directory: stock, initialBranch: cafe});
    expect(stock.asked).toEqual([cafe]);
    expect(stock.listed).not.toHaveBeenCalled();
    expect(screen()).toContain('여울 커피');
    await press('매장 찾기로');
    expect(stock.listed).toHaveBeenCalledTimes(1);
  });

  test('another QR, a payment QR without a payment screen and no camera do not open a store', async () => {
    const back = jest.fn();
    for (const scan of [async () => 'https://example.com', async () => `nu54://shop/${cafe}?x=1`, async () => 'nu54://order?v=2',
      async () => { throw new Error('SHOP_CAMERA'); }, undefined]) {
      const stock = directory();
      await render({directory: stock, scan, back});
      await press('매장 바로가기 QR 찍기');
      expect(screen()).toContain('이 QR로는 매장을 열 수 없습니다');
      expect(screen()).toContain('매장 바로가기 QR이 아닙니다');
      expect(screen()).toContain('결제 QR이거나, 다른 앱의 QR이거나, 카메라를 쓸 수 없는 경우입니다.');
      expect(stock.asked).toEqual([]);
    }
    expect(buttons()).toEqual(['다시 찍기', '이름으로 매장 찾기', '홈으로']);
    await press('홈으로');
    expect(back).toHaveBeenCalled();
  });

  test('a kiosk payment QR goes to the payment screen, and a closed scanner changes nothing', async () => {
    const payQr = jest.fn();
    await render({scan: async () => 'nu54://order?v=2&branch=x', payQr});
    await press('매장 바로가기 QR 찍기');
    expect(payQr).toHaveBeenCalledWith('nu54://order?v=2&branch=x');
    await render({scan: async () => ''});
    await press('매장 바로가기 QR 찍기');
    expect(screen()).toContain('모든 매장');
  });

  test('the page hands the menu to the ordering flow with the store that is open', async () => {
    const order = jest.fn();
    await render({order});
    await press('여울 커피');
    await press('메뉴 보고 주문');
    expect(order).toHaveBeenCalledWith({branch: cafe, name: '여울 커피'});
  });

  test('phone and map open another app with the store\'s own data only, and say so', async () => {
    const opened: string[] = [];
    await render({outside: {openURL: async url => { opened.push(url); }}});
    await press('여울 커피');
    const map = tree.root.findAll(node => node.props.accessibilityRole === 'button' && node.props.accessibilityLabel === '지도 앱으로 열기')[0];
    expect(map.props.accessibilityHint).toBe('매장 주소가 지도 앱으로 갑니다. 내 위치는 보내지 않습니다.');
    await press('지도 앱으로 열기');
    await press('전화');
    expect(opened).toEqual([mapLink('시험시 시험로 1'), 'tel:025550100']);
    expect(mapLink('시험시 시험로 1', 'android')).toBe('geo:0,0?q=' + encodeURIComponent('시험시 시험로 1'));
    expect(mapLink('a&b=c', 'ios')).toBe('https://maps.apple.com/?q=a%26b%3Dc');
    expect(phoneLink('+82 2-555-0100')).toBe('tel:+8225550100');
  });

  test('a map app that does not open says so and keeps the page', async () => {
    await render({outside: {openURL: async () => { throw new Error('no app'); }}});
    await press('여울 커피');
    await press('지도 앱으로 열기');
    expect(screen()).toContain('지도 앱을 열지 못했습니다. 주소를 길게 눌러 복사하세요.');
    expect(screen()).toContain('여울 커피');
  });
});

describe('the store page', () => {
  let tree: Renderer.ReactTestRenderer;
  const screen = () => JSON.stringify(tree.toJSON());
  const labels = () => tree.root.findAll(node => node.props.accessibilityRole === 'button' && typeof node.props.onPress === 'function')
    .map(node => node.props.accessibilityLabel as string);
  const show = async (value: StorePage, props: Partial<React.ComponentProps<typeof StorePageView>> = {}) => {
    await Renderer.act(async () => { tree = Renderer.create(<StorePageView page={value} back={() => {}} call={() => {}} map={() => {}} order={() => {}} {...props} />); });
  };
  afterEach(() => { Renderer.act(() => tree?.unmount()); });

  test('shows what the store wrote: kind and state, introduction, location, hours, facts and the operator', async () => {
    await show(page, {payment: 'tKRW'});
    for (const text of ['여울 커피', '카페 · 영업 중 · 21:00에 영업 종료', '직접 볶은 원두로 내립니다', '시험시 시험로 1', '역 2번 출구에서 걸어서 3분',
      '영업시간', '매일', '10:00~21:00', '쉬는 날', '명절 당일', '매장 정보', '무선 인터넷, 단체 이용', '건물 뒤 2대', '보드 결제 · tKRW',
      '결제·환불 문의', 'help@operator.example', '음식 준비는 매장에, 결제와 환불은 운영자에게 묻습니다.'])
      expect(screen()).toContain(text);
    expect(labels()).toEqual(['메뉴 보고 주문', '전화', '지도 앱으로 열기', '매장 찾기로']);
    for (const word of ['리뷰', '평점', '배송', '주변', '예약']) expect(screen()).not.toContain(word);
  });

  test('a store that wrote nothing has its name, its order state and the way to its menu, and no empty rows', async () => {
    await show({branch: cafe, name: '여울 커피', status: taking});
    expect(screen()).toContain('주문 받는 중');
    expect(labels()).toEqual(['메뉴 보고 주문', '매장 찾기로']);
    for (const title of ['위치', '영업시간', '편의', '주차', '운영자 ']) expect(screen()).not.toContain(title);
    expect(screen()).toContain('보드 결제');
    expect(screen()).not.toContain('보드 결제 ·'); // a build without its order contract names no token
  });

  test('the token on the payment line is the build\'s, whatever a server says of it', async () => {
    await show({...page, paymentToken: 'tUSD'} as StorePage, {payment: 'tKRW'});
    expect(screen()).toContain('보드 결제 · tKRW');
    expect(screen()).not.toContain('tUSD');
  });

  test('a busy store and a store that takes no order now say so', async () => {
    await show({...page, status: {mode: 'busy', orderable: true, busyMinutes: 25, ...now}});
    expect(screen()).toContain('카페 · 영업 중 · 바쁨, 약 25분 · 21:00에 영업 종료');
    expect(labels()).toContain('메뉴 보고 주문');
    await show({...page, status: closedDay});
    expect(screen()).toContain('카페 · 오늘 쉽니다');
    expect(labels()).toContain('메뉴만 보기');
    expect(screen()).toContain('오늘은 쉬는 날이라 주문을 받지 않습니다. 메뉴는 볼 수 있습니다.');
  });

  test.each<[StorePage['status'], string, string]>([
    [{mode: 'paused', orderable: false, reason: 'paused', resumeAt: '11:00', ...now}, '카페 · 영업 중 · 주문 잠시 멈춤 · 11:00쯤 다시 받습니다 · 21:00에 영업 종료', '매장이 주문을 잠시 멈췄습니다. 11:00쯤 다시 받는다고 알렸습니다. 메뉴는 볼 수 있습니다.'],
  ])('a store the server takes no order for says the server\'s reason and offers the menu only to read: %j', async (status, line, why) => {
    await show({...page, status});
    expect(screen()).toContain(line);
    expect(screen()).toContain(why);
    expect(labels()).toContain('메뉴만 보기');
    expect(labels()).not.toContain('메뉴 보고 주문');
    expect(screen()).not.toContain('오늘 쉽니다'); // a closed mode is not a day off
  });

  // The server shows no row of a menu that is not on sale, at this hour or at all: the page says why no order is
  // taken and promises no menu.
  test.each<[StorePage['status'], string, string]>([
    [{mode: 'closed', orderable: false, reason: 'no_menu', ...now}, '카페 · 영업 중 · 판매 중인 메뉴 없음 · 21:00에 영업 종료', '지금 판매 중인 메뉴가 없어 주문을 받지 않습니다.'],
    [{mode: 'closed', orderable: false, reason: 'outside_hours', ...now}, '카페 · 영업 중 · 지금은 주문 시간 아님 · 21:00에 영업 종료', '지금은 메뉴를 판매하는 시간이 아니라 주문을 받지 않습니다.'],
  ])('a store with no menu on sale now says the server\'s reason and offers no menu: %j', async (status, line, why) => {
    await show({...page, status});
    expect(screen()).toContain(line);
    expect(screen()).toContain(why);
    expect(screen()).not.toContain('메뉴는 볼 수 있습니다');
    expect(labels()).not.toContain('메뉴만 보기');
    expect(labels()).not.toContain('메뉴 보고 주문');
  });

  test('a store the server takes an order for is offered the order, whatever the hours it wrote say', async () => {
    await show({...page, status: {...taking, time: '09:10'}});
    expect(screen()).toContain('카페 · 영업 전 · 10:00에 영업 시작 · 주문 받는 중');
    expect(labels()).toContain('메뉴 보고 주문');
    expect(screen()).not.toContain('주문을 받지 않습니다');
  });

  test('a reservation button appears only for a store that takes reservations, when the app can make one', async () => {
    await show({...page, reservations: true});
    expect(labels()).not.toContain('예약');
    await show(page, {reserve: () => {}});
    expect(labels()).not.toContain('예약');
    await show({...page, reservations: true}, {reserve: () => {}});
    expect(labels()).toContain('예약');
  });

  test('without an ordering flow the page shows no order button', async () => {
    await show(page, {order: undefined});
    expect(labels()).toEqual(['전화', '지도 앱으로 열기', '매장 찾기로']);
  });
});
