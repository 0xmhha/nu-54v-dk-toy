import React from 'react';
import {AccessibilityInfo, BackHandler} from 'react-native';
import Renderer from 'react-test-renderer';
import {
  bindServiceRequestV2 as mockBind, namedTokenAbiSha256 as mockNamed, toBase64 as mockBase64,
  type CommerceCart, type CommercePrice, type CommerceSkuOptions, type ServiceRequestV2,
} from '@nu54/protocol';
import {CustomerOrders} from '../src/merchant/CustomerOrders.tsx';

const mockRef = (n: number) => `0fbc54ef-2827-4ab5-bfcd-3a27611c3${n.toString(16).padStart(3, '0')}`;
const mockBranch = mockRef(1), mockTerminal = mockRef(11), mockAsset = '0x' + 'a'.repeat(40);
const mockUnix = () => Math.floor(Date.now() / 1000);
const mockScope = {role: 'customer', branchRef: mockBranch, subjectRef: mockRef(2), tenantRef: mockRef(3), expiresAt: mockUnix() + 300};
const mockShop = {branch: mockBranch, branchLabel: '여울 커피'};
const mockCarts: CommerceCart[] = [];
const mockJournals = new Map<string, string>();
const mockThumb = Uint8Array.from({length: 900}, (_, i) => (i * 11) & 255);
const mockThumbName = require('@noble/hashes/sha256').sha256(mockThumb).reduce((s: string, b: number) => s + b.toString(16).padStart(2, '0'), '');
let mockPhotoReads = 0, mockServerDown = false, mockTaxed = false, mockPauseAtPay = false;
let mockState: unknown;
let mockNearby = false, mockStock: number | undefined = 5, mockOffline = false, mockSoldOut = '-', mockEntropy = 0;
const mockGroups: CommerceSkuOptions['groups'] = [
  {key: 'temp', labels: {ko: '온도', en: 'Temperature'}, values: [{key: 'hot', labels: {ko: '뜨겁게', en: 'Hot'}}, {key: 'ice', labels: {ko: '차갑게', en: 'Iced'}}]},
  {key: 'size', labels: {ko: '크기', en: 'Size'}, values: [{key: 's', labels: {ko: '보통', en: 'Regular'}}, {key: 'l', labels: {ko: '크게', en: 'Large'}}]},
];
function mockRows(choices: boolean): CommercePrice[] {
  const common = {version: 1, expiresAt: new Date((mockUnix() + 120) * 1000).toISOString(), asset: mockAsset, available: mockStock ?? 5,
    ...(mockStock === undefined ? {} : {skuAvailable: mockStock})};
  return [
    {...common, sku: 'cookie', optionKey: 'standard', label: '쿠키', unitPrice: '300000'},
    ...mockGroups[0].values.flatMap((a, i) => mockGroups[1].values.map((b, j) => ({
      ...common, sku: 'latte', optionKey: `${a.key}-${b.key}`, label: '카페라떼', unitPrice: String(500000 + j * 50000),
      available: `${a.key}-${b.key}` === mockSoldOut ? 0 : mockStock ?? 5,
      metadata: {labels: {ko: '카페라떼', en: 'Cafe latte'}, optionLabels: {ko: `${a.labels.ko} · ${b.labels.ko}`}, optionRequired: true, allergens: ['milk' as const],
        ...(mockTaxed ? {tax: 'vat10' as const} : {})},
      ...(choices ? {choices: [{group: {key: 'temp', labels: mockGroups[0].labels}, value: {...a, order: i}}, {group: {key: 'size', labels: mockGroups[1].labels}, value: {...b, order: j}}]} : {}),
    }))),
  ];
}
jest.mock('../src/specs/NativeNearby.ts', () => ({__esModule: true, default: {}}));
jest.mock('../src/merchant/nearbyClient.ts', () => ({CustomerNearby: class {
  close() {} disconnect() {}
  async scan() { return [{handle: 'terminal'}]; }
  async connect() {
    return {peer: {principal: mockTerminal}, request: async ({op, offset, cart}: {op: string; offset?: number; cart?: CommerceCart}) => {
      if (op === 'discover') return {status: 'ok', value: {...mockShop, terminal: mockTerminal}};
      if (op === 'reserve') { mockCarts.push(cart!); return {status: 'pending'}; }
      const rows = mockRows(true), at = offset ?? 0, page = rows.slice(at, at + 2);
      return {status: 'ok', value: {rows: page, ...(at + 2 < rows.length ? {next: at + 2} : {})}};
    }};
  }
}}));
jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
jest.mock('../src/specs/NativeRenterBle.ts', () => ({__esModule: true, default: {loadDevices: async () => ''}}));
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({__esModule: true, default: {clear: async () => {}}}));
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  loadProfile: async () => JSON.stringify({scope: mockScope, pin: 'a'.repeat(64), nearby: mockNearby ? {} : undefined,
    payment: {chain: 8283, contract: '0x' + 'b'.repeat(40), beneficiary: '0x' + 'c'.repeat(40), signer: '0x' + 'd'.repeat(40), asset: mockAsset, merchantId: 'e'.repeat(64),
      keyVersion: 1, beneficiaryVersion: 1, maxBaseUnits: '30000000', abiSha256: mockNamed, token: {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2}}}),
  configure: async () => {},
  loadJournal: async (name: string) => mockJournals.get(name) ?? '',
  saveJournal: async (name: string, value: string) => mockJournals.set(name, value),
  randomBytes: async () => mockBase64(new Uint8Array(16).fill(++mockEntropy)),
  exchange: async (_slot: string, raw: string) => {
    const {command, cart} = JSON.parse(raw) as {command: ServiceRequestV2; cart?: CommerceCart};
    const {requestId, requestDigest} = mockBind(command, {...mockScope,
      grants: [{domainRef: mockBranch, objectRef: command.objectRef, operations: [`C3/${command.operation}`]}]} as never, mockUnix());
    const reply = {requestId, requestDigest, status: 'ok', resultRef: command.objectRef};
    if (command.operation === 'order.list') return JSON.stringify({...reply, orders: []});
    if (command.operation === 'invoice.list') return JSON.stringify({...reply, invoices: []});
    if (mockOffline) throw Object.assign(new Error('offline'), {code: 'COMMERCE_NETWORK_OFFLINE'});
    if (command.operation === 'catalog.read')
      return JSON.stringify({...reply, catalog: mockRows(false), catalogVersion: 1, catalogOptions: [{sku: 'latte', groups: mockGroups}], shop: mockShop,
        catalogPhotos: [{sku: 'latte', photo: 'a'.repeat(64), thumb: mockThumbName}], ...(mockState ? {orderState: mockState} : {})});
    if (command.operation === 'catalog.photo') {
      mockPhotoReads++;
      if (mockServerDown) throw Object.assign(new Error('offline'), {code: 'COMMERCE_NETWORK_OFFLINE'});
      expect(command.body.photo).toBe(mockThumbName); // the small copy is enough for the phone
      return JSON.stringify({...reply, photo: {size: mockThumb.length, data: mockBase64(mockThumb)}});
    }
    if (command.operation === 'order.create') {
      if (mockPauseAtPay) {
        mockState = {mode: 'paused', resumeAt: '11:20'};
        return JSON.stringify({requestId, requestDigest, status: 'error', error: 'CATALOG_CHANGED'});
      }
      mockCarts.push(cart!);
      throw new Error('no reply');
    }
    throw new Error('unexpected ' + command.operation);
  },
}}));

const view = (tree: Renderer.ReactTestRenderer) => JSON.stringify(tree.toJSON());
const button = (tree: Renderer.ReactTestRenderer, label: string) => tree.root.findAll(node =>
  (node.props.label === label && typeof node.props.run === 'function') ||
  ((node.props.title === label || node.props.accessibilityLabel === label) && typeof node.props.onPress === 'function'))[0];
async function tap(tree: Renderer.ReactTestRenderer, label: string) {
  const node = button(tree, label);
  if (!node) throw new Error(`no button ${label} in ${view(tree)}`);
  if (node.props.disabled) throw new Error(`disabled: ${label}`);
  await Renderer.act(async () => (node.props.run ?? node.props.onPress)());
}
async function shopping(run: (tree: Renderer.ReactTestRenderer) => Promise<void>) {
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<CustomerOrders initialLocator="" shopEntry back={() => {}} permission={async () => true} />); });
  try {
    if (mockNearby) {
      await tap(tree, '근처 단말 찾기');
      await tap(tree, '근거리 단말 후보 1 인증');
    } else await tap(tree, '온라인으로 매장 확인');
    await tap(tree, '이 매장 메뉴 열기');
    await run(tree);
  } finally { await Renderer.act(async () => tree.unmount()); }
}
beforeEach(() => {
  mockCarts.length = 0;
  mockJournals.clear();
  mockNearby = mockOffline = false;
  mockStock = 5;
  mockSoldOut = '-';
  mockPhotoReads = 0;
  mockServerDown = false;
  mockTaxed = false;
  mockState = undefined;
  mockPauseAtPay = false;
});

test.each([false, true])('the combination the customer picked is the one that is ordered (nearby=%s)', async nearby => {
  mockNearby = nearby;
  await shopping(async tree => {
    expect(view(tree)).toContain('카페라떼 · 5,000원부터');
    expect(view(tree)).not.toContain('뜨겁게 · 보통');
    await tap(tree, '카페라떼 옵션 고르기');
    expect(tree.root.findAll(node => node.props.accessibilityState?.selected === true)).toHaveLength(0);
    expect(view(tree)).not.toContain('쿠키');
    await tap(tree, '온도 차갑게');
    await tap(tree, '크기 크게 +500원');
    expect(view(tree)).toContain('✓ 크게');
    await tap(tree, '수량 늘리기');
    await tap(tree, '담기 · 11,000원');
    expect(view(tree)).toContain('차갑게 · 크게');
    expect(view(tree)).toContain('합계 · 11,000원');
    expect(view(tree)).not.toContain('뜨겁게 · 보통');
    // Opening it again edits the same line.
    await tap(tree, '카페라떼 옵션 고르기');
    await tap(tree, '온도 차갑게');
    await tap(tree, '크기 크게 +500원');
    await tap(tree, '담기 · 11,000원');
    await tap(tree, '카페라떼 차갑게 · 크게 빼기');
    expect(view(tree)).toContain('합계 · 5,500원');
    await tap(tree, '쿠키 추가');
    await tap(tree, '주문 접수 요청');
    expect(mockCarts).toHaveLength(1);
    expect(mockCarts[0].items).toEqual([{sku: 'cookie', option_key: 'standard', quantity: 1}, {sku: 'latte', option_key: 'ice-l', quantity: 1}]);
  });
});

test('adding too early names the groups, and the phone Back key returns to the menu first', async () => {
  const announce = jest.spyOn(AccessibilityInfo, 'announceForAccessibility').mockImplementation(() => {});
  const handlers: Parameters<typeof BackHandler.addEventListener>[1][] = [];
  const listen = jest.spyOn(BackHandler, 'addEventListener').mockImplementation((_, handler) => {
    handlers.push(handler);
    return {remove: () => { handlers.splice(handlers.indexOf(handler), 1); }};
  });
  try {
    await shopping(async tree => {
      await tap(tree, '카페라떼 옵션 고르기');
      await tap(tree, '골라 주세요 · 5,000원부터');
      expect(announce).toHaveBeenCalledWith('온도·크기를 골라 주세요');
      const alerts = () => tree.root.findAll(node => node.props.accessibilityRole === 'alert' && typeof node.type === 'string').map(node => node.props.children);
      expect(alerts()).toEqual(['온도를 골라 주세요', '크기를 골라 주세요']);
      await tap(tree, '온도 뜨겁게');
      expect(alerts()).toEqual(['크기를 골라 주세요']);
      await Renderer.act(async () => { expect(handlers.at(-1)!({} as never)).toBe(true); });
      expect(view(tree)).toContain('카페라떼 · 5,000원부터');
      expect(mockCarts).toHaveLength(0);
    });
  } finally { announce.mockRestore(); listen.mockRestore(); }
});

test('what is sold out cannot be picked, alone or with the other choices', async () => {
  mockSoldOut = 'ice-l';
  await shopping(async tree => {
    await tap(tree, '카페라떼 옵션 고르기');
    await tap(tree, '온도 차갑게');
    expect(button(tree, '크기 크게 지금 고른 것과는 품절').props.disabled).toBe(true);
    await tap(tree, '온도 차갑게');
    await tap(tree, '크기 크게 +500원');
    expect(button(tree, '온도 차갑게 지금 고른 것과는 품절').props.disabled).toBe(true);
  });
  mockStock = 0;
  mockSoldOut = '-';
  await shopping(async tree => {
    expect(view(tree)).toContain('품절');
    expect(button(tree, '카페라떼 옵션 고르기').props.disabled).toBe(true);
  });
});

test('the quantity stops at the stock of the product', async () => {
  mockStock = 2;
  await shopping(async tree => {
    await tap(tree, '카페라떼 옵션 고르기');
    for (const label of ['온도 뜨겁게', '크기 보통', '수량 늘리기']) await tap(tree, label);
    expect(button(tree, '수량 늘리기').props.disabled).toBe(true);
    await tap(tree, '담기 · 10,000원');
    expect(button(tree, '카페라떼 옵션 고르기').props.disabled).toBe(true);
  });
});

test('an older store that takes one option of a product says why another is refused', async () => {
  mockStock = undefined;
  await shopping(async tree => {
    await tap(tree, '카페라떼 옵션 고르기');
    for (const label of ['온도 뜨겁게', '크기 보통', '담기 · 5,000원', '카페라떼 옵션 고르기', '온도 차갑게', '크기 보통']) await tap(tree, label);
    expect(view(tree)).toContain('품목당 옵션을 하나씩');
    expect(button(tree, '담기 · 5,000원').props.disabled).toBe(true);
  });
});

test('the words are English on the English menu, and nothing can be picked from the last menu', async () => {
  await shopping(async tree => {
    await tap(tree, '메뉴 언어 English');
    expect(view(tree)).toContain('Cafe latte · From 5,000원');
    await tap(tree, 'Choose options for Cafe latte');
    await tap(tree, 'Temperature Iced');
    await tap(tree, 'Choose first · From 5,000원');
    expect(view(tree)).toContain('Choose Size');
    await tap(tree, 'Back to menu');
    await tap(tree, 'Menu language 한국어');
    await tap(tree, '쿠키 추가');
    mockOffline = true;
    await tap(tree, '주문 접수 요청');
    expect(view(tree)).toContain('최근 메뉴를 보고 있습니다');
    expect(view(tree)).toContain('카페라떼 · 5,000원부터');
    expect(button(tree, '카페라떼 옵션 고르기')).toBeUndefined();
  });
});

test('the choice page shows the small menu photo from the server, and only the name when it cannot be read', async () => {
  const photo = (tree: Renderer.ReactTestRenderer) => tree.root.findAll(node => typeof node.type === 'string' && node.props.source?.uri)
    .map(node => [node.props.accessibilityLabel, node.props.source.uri]);
  const settle = () => Renderer.act(async () => { await new Promise(resolve => setTimeout(resolve, 30)); });
  await shopping(async tree => {
    expect(photo(tree)).toEqual([]); // not on the menu list, as drawn
    await tap(tree, '카페라떼 옵션 고르기');
    await settle();
    expect(photo(tree)).toEqual([['카페라떼 메뉴 사진', 'data:image/jpeg;base64,' + mockBase64(mockThumb)]]);
    expect(mockPhotoReads).toBe(1);
  });
  mockNearby = true;
  mockServerDown = true;
  await shopping(async tree => {
    await tap(tree, '카페라떼 옵션 고르기');
    await settle();
    expect(photo(tree)).toEqual([]);
    await tap(tree, '온도 차갑게');
    expect(view(tree)).toContain('카페라떼');
  });
});

test('the basket shows the tax its prices include once every line says it', async () => {
  mockTaxed = true;
  await shopping(async tree => {
    await tap(tree, '카페라떼 옵션 고르기');
    for (const label of ['온도 차갑게', '크기 크게 +500원', '수량 늘리기', '담기 · 11,000원']) await tap(tree, label);
    expect(view(tree)).toContain('공급가액 · 10,000원'); // 11,000원 holds 1,000원 of tax
    expect(view(tree)).toContain('세금(가격에 포함) · 1,000원');
    await tap(tree, '쿠키 추가');
    expect(view(tree)).not.toContain('공급가액');
  });
});

test('a busy store shows its minutes above the menu', async () => {
  mockState = {mode: 'busy', busyMinutes: 25};
  await shopping(async tree => {
    expect(view(tree)).toContain('지금은 약 25분 걸립니다');
    expect(view(tree)).toContain('주문은 받습니다');
  });
});

test('a paused store says when it said it would take orders again, and the order cannot be sent', async () => {
  mockState = {mode: 'paused', resumeAt: '11:20'};
  await shopping(async tree => {
    expect(view(tree)).toContain('매장이 주문을 잠시 멈췄습니다');
    expect(view(tree)).toContain('11:20쯤 다시 받는다고 알렸습니다.');
    await tap(tree, '쿠키 추가');
    expect(button(tree, '주문 접수 요청').props.disabled).toBe(true);
    mockState = {mode: 'open'};
    await tap(tree, '다시 받는지 확인');
    expect(view(tree)).not.toContain('매장이 주문을 잠시 멈췄습니다');
    expect(button(tree, '주문 접수 요청').props.disabled).toBeFalsy();
  });
});

test('a pause that comes while paying keeps the basket and says nothing was charged', async () => {
  mockPauseAtPay = true;
  await shopping(async tree => {
    await tap(tree, '쿠키 추가');
    await tap(tree, '주문 접수 요청');
    expect(view(tree)).toContain('매장이 주문을 잠시 멈췄습니다');
    expect(view(tree)).toContain('결제하지 않았습니다. 돈은 나가지 않았고, 담은 것은 그대로 두었습니다.');
    expect(mockCarts).toHaveLength(0);
  });
});

test('a test store with an automatic kitchen says so above the menu', async () => {
  mockState = {mode: 'open', autoKitchen: true};
  await shopping(async tree => {
    expect(view(tree)).toContain('시험 매장 · 자동 주방: 주방 단계를 자동으로 기록하고');
  });
});
