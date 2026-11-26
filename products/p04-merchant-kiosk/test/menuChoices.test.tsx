import React from 'react';
import { AccessibilityInfo, Dimensions } from 'react-native';
import Renderer from 'react-test-renderer';
import App from '../src/App';
import {
  bindServiceRequestV2 as mockBind,
  namedTokenAbiSha256 as mockNamed,
  toBase64 as mockBase64,
  type ServiceRequestV2,
  type CommerceCart,
  type CommerceSkuOptions,
} from '@nu54/protocol';

const mockRef = (n: number) => `f0551c76-7592-4850-a414-54464d24d${n.toString(16).padStart(3, '0')}`;
const mockShop = mockRef(1), mockSale = mockRef(4);
const mockAsset = '0x' + 'a'.repeat(40);
const mockClock = () => Math.floor(Date.now() / 1000);
const mockCarts: CommerceCart[] = [];
const mockJournal = new Map<string, string>();
let mockEntropy = 0;
let mockSoldOut: string[] = [];
let mockVersion = 1;
let mockDropped = '';
// A photo for the latte: its page copy and its list copy, served in parts as opsd does.
const mockPhoto = Uint8Array.from({ length: 30_000 }, (_, i) => (i * 13) & 255);
const mockThumb = mockPhoto.slice(0, 700);
const mockHex = (bytes: Uint8Array) => require('@noble/hashes/sha256').sha256(bytes).reduce((s: string, b: number) => s + b.toString(16).padStart(2, '0'), '');
let mockPhotos = false;
let mockTaxed = false;
let mockState: unknown;
let mockPauseAtPay = false;
const mockPhotoReads: string[] = [];
const mockProfile = () => ({ branchRef: mockShop, subjectRef: mockRef(2), tenantRef: mockRef(5), role: 'terminal', expiresAt: mockClock() + 120 });
const mockGroups: CommerceSkuOptions['groups'] = [
  { key: 'temp', labels: { ko: '온도', en: 'Temperature' }, values: [{ key: 'hot', labels: { ko: '뜨겁게', en: 'Hot' } }, { key: 'ice', labels: { ko: '차갑게', en: 'Iced' } }] },
  { key: 'size', labels: { ko: '크기', en: 'Size' }, values: [{ key: 's', labels: { ko: '보통', en: 'Regular' } }, { key: 'l', labels: { ko: '크게', en: 'Large' } }] },
  { key: 'milk', labels: { ko: '우유' }, values: [{ key: 'cow', labels: { ko: '우유' } }, { key: 'soy', labels: { ko: '두유' } }] },
];
const mockExtra: Record<string, number> = { l: 500, soy: 700 };
function mockCatalog() {
  const common = { version: mockVersion, expiresAt: new Date((mockClock() + 120) * 1000).toISOString(), asset: mockAsset, available: 5, skuAvailable: 5 };
  const keys = mockGroups.reduce<string[][]>((list, { values }) => list.flatMap(row => values.map(({ key }) => [...row, key])), [[]])
    .filter(row => !row.includes(mockDropped));
  const label = (row: string[]) => row.map((key, i) => mockGroups[i].values.find(value => value.key === key)!.labels.ko).join(' · ');
  return {
    catalog: [
      { ...common, sku: 'cookie', optionKey: 'standard', label: '쿠키', unitPrice: '300000' },
      ...keys.map(row => ({
        ...common, sku: 'latte', optionKey: row.join('-'), label: '카페라떼',
        unitPrice: String((5000 + row.reduce((sum, key) => sum + (mockExtra[key] ?? 0), 0)) * 100),
        available: mockSoldOut.some(out => row.join('-').match(out)) ? 0 : 5,
        metadata: { labels: { ko: '카페라떼', en: 'Cafe latte' }, optionLabels: { ko: label(row) }, optionRequired: true, allergens: [row.includes('cow') ? 'milk' : 'soy'],
          ...(mockTaxed ? { tax: 'vat10' as const } : {}) },
      })),
    ],
    catalogVersion: mockVersion,
    ...(mockPhotos ? { catalogPhotos: [{ sku: 'latte', photo: mockHex(mockPhoto), thumb: mockHex(mockThumb) }] } : {}),
    catalogOptions: [{ sku: 'latte', groups: mockGroups.map(group => ({ ...group, values: group.values.filter(({ key }) => key !== mockDropped) })) }],
    shop: { branch: mockShop, branchLabel: '여울 커피' },
  };
}

jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
jest.mock('../src/specs/NativeNearby.ts', () => ({ __esModule: true, default: {
  onNearby: () => ({ remove() {} }), configure: async () => {}, start: async () => {}, stop: async () => {}, close: async () => {} } }));
jest.mock('../src/kiosk/nearbyRelay.ts', () => ({ NearbyRelay: class { close() {} activate() {} retire() {} clearPublic() {} } }));
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {
    loadProfile: async (slot: string) => slot !== 'terminal' ? '' : JSON.stringify({
      scope: mockProfile(), pin: 'b'.repeat(64), nearby: {},
      payment: { chain: 8283, contract: '0x' + '1'.repeat(40), asset: mockAsset, beneficiary: '0x' + '2'.repeat(40), signer: '0x' + '3'.repeat(40),
        merchantId: 'a'.repeat(64), keyVersion: 1, beneficiaryVersion: 1, maxBaseUnits: '30000000', abiSha256: mockNamed,
        token: { name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2 } },
    }),
    publicIdentity: async () => 'same-native-key',
    exchangeScoped(slot: string, _digest: string, raw: string) { return this.exchange(slot, raw); },
    loadJournal: async (name: string) => mockJournal.get(name) ?? '',
    saveJournal: async (name: string, value: string) => mockJournal.set(name, value),
    randomBytes: async () => mockBase64(new Uint8Array(16).fill(++mockEntropy)),
    exchange: async (_slot: string, raw: string) => {
      const { command, cart } = JSON.parse(raw) as { command: ServiceRequestV2; cart?: CommerceCart };
      const { requestId, requestDigest } = mockBind(command, { ...mockProfile(),
        grants: [{ domainRef: command.domainRef, objectRef: command.objectRef, operations: [`C3/${command.operation}`] }] } as never, mockClock());
      const response = { requestId, requestDigest, status: 'ok', resultRef: command.objectRef };
      if (command.operation === 'terminal.device')
        return JSON.stringify({ ...response, terminalDevice: { revision: 0, language: 'ko', idleSeconds: 60 } });
      if (command.operation === 'catalog.photo') {
        const bytes = command.body.photo === mockHex(mockPhoto) ? mockPhoto : mockThumb;
        mockPhotoReads.push(`${bytes === mockPhoto ? 'photo' : 'thumb'} ${command.body.offset}`);
        const at = Number(command.body.offset);
        return JSON.stringify({ ...response, photo: { size: bytes.length, data: mockBase64(bytes.slice(at, at + 18000)) } });
      }
      if (command.operation === 'catalog.read')
        return JSON.stringify({ ...response, ...mockCatalog(), ...(mockState && command.body.page === '0' ? { orderState: mockState } : {}) });
      if (command.operation === 'terminal.state')
        return JSON.stringify({ ...response, terminal: { mode: 'menu', revision: 1 } });
      if (command.operation === 'order.create') {
        if (mockPauseAtPay) {
          mockState = { mode: 'paused', resumeAt: '11:20' };
          return JSON.stringify({ requestId, requestDigest, status: 'error', error: 'CATALOG_CHANGED' });
        }
        mockCarts.push(cart!);
        return JSON.stringify({ ...response, resultRef: mockSale });
      }
      throw new Error('unexpected ' + command.operation);
    },
  },
}));

const view = (tree: Renderer.ReactTestRenderer) => JSON.stringify(tree.toJSON());
const button = (tree: Renderer.ReactTestRenderer, label: string) => tree.root.findAll(n =>
  (n.props.label === label && typeof n.props.run === 'function') || (n.props.accessibilityLabel === label && typeof n.props.onPress === 'function'))[0];
async function press(tree: Renderer.ReactTestRenderer, label: string) {
  const found = button(tree, label);
  if (!found) throw new Error(`no button: ${label}`);
  expect(found.props.disabled).not.toBe(true);
  await Renderer.act(async () => { (found.props.run ?? found.props.onPress)(); });
}
async function kiosk(run: (tree: Renderer.ReactTestRenderer) => Promise<void>) {
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    if (!button(tree, '새 고객 주문')) throw new Error(view(tree));
    await press(tree, '새 고객 주문');
    await run(tree);
  } finally { await Renderer.act(async () => tree.unmount()); }
}
const window = { width: 1280, height: 800, scale: 1, fontScale: 1 };
beforeEach(() => {
  mockCarts.length = 0;
  mockJournal.clear();
  mockSoldOut = [];
  mockVersion = 1;
  mockDropped = '';
  mockPhotos = false;
  mockTaxed = false;
  mockState = undefined;
  mockPauseAtPay = false;
  mockPhotoReads.length = 0;
  Dimensions.set({ window, screen: window });
});

test('a customer picks one value in each group, sees the amount follow, and the exact combination is ordered', () => kiosk(async tree => {
  expect(view(tree)).toContain('5,000원부터');
  await press(tree, '카페라떼 옵션 선택');
  // Nothing is picked beforehand.
  expect(tree.root.findAll(n => n.props.accessibilityState?.selected === true)).toHaveLength(0);
  expect(view(tree)).toContain('점주 신고 성분 옵션에 따라 우유, 대두');
  await press(tree, '온도 차갑게');
  await press(tree, '크기 크게 +500원');
  expect(button(tree, '우유를 골라 주세요 5,500원부터')).toBeDefined();
  expect(view(tree)).toContain('✓ 크게');
  await press(tree, '우유 두유 +700원');
  expect(view(tree)).toContain('점주 신고 성분 대두 ·');
  await press(tree, '수량 늘리기');
  await press(tree, '담기 12,400원');
  expect(view(tree)).toContain('카페라떼 · 차갑게 · 크게 · 두유 ×2');
  await press(tree, '하드월렛으로 결제');
  expect(mockCarts[0].items).toEqual([{ sku: 'latte', option_key: 'ice-l-soy', quantity: 2 }]);
}));

test('adding too early names every missing group, speaks it and moves to the first one', () => kiosk(async tree => {
  const announce = jest.spyOn(AccessibilityInfo, 'announceForAccessibility').mockImplementation(() => {});
  const focus = jest.spyOn(AccessibilityInfo, 'sendAccessibilityEvent').mockImplementation(() => {});
  try {
    await press(tree, '카페라떼 옵션 선택');
    await press(tree, '온도 뜨겁게');
    await press(tree, '크기·우유를 골라 주세요 5,000원부터');
    expect(announce).toHaveBeenCalledWith('크기·우유를 골라 주세요');
    const alerts = tree.root.findAll(n => n.props.accessibilityRole === 'alert' && typeof n.type === 'string');
    expect(alerts.map(n => n.props.children)).toEqual(['크기를 골라 주세요', '우유를 골라 주세요']);
    await press(tree, '크기 보통');
    expect(view(tree)).not.toContain('크기를 골라 주세요');
    expect(view(tree)).toContain('우유를 골라 주세요');
    await press(tree, '메뉴로 돌아가기');
    expect(mockCarts).toHaveLength(0);
    expect(view(tree)).toContain('담은 메뉴가 없습니다.');
  } finally { announce.mockRestore(); focus.mockRestore(); }
}));

test('a sold-out value cannot be picked, and taking a pick back opens the others again', async () => {
  mockSoldOut = ['-soy$', '^ice-l-'];
  await kiosk(async tree => {
    await press(tree, '카페라떼 옵션 선택');
    expect(button(tree, '우유 두유 품절').props.disabled).toBe(true);
    await press(tree, '온도 차갑게');
    expect(button(tree, '크기 크게 지금 고른 것과는 품절').props.accessibilityState).toEqual({ selected: false, disabled: true });
    await press(tree, '온도 차갑게');
    await press(tree, '크기 크게 +500원');
    expect(button(tree, '온도 차갑게 지금 고른 것과는 품절').props.disabled).toBe(true);
  });
});

test('the words, the missing group and the amount are English in English', () => kiosk(async tree => {
  await press(tree, '메뉴 언어 English');
  expect(view(tree)).toContain('From ₩5,000');
  await press(tree, 'Choose options for Cafe latte');
  await press(tree, 'Temperature Hot');
  // A group without an English name keeps its Korean one.
  await press(tree, 'Choose Size, 우유 From ₩5,000');
  expect(view(tree)).toContain('Choose Size');
  await press(tree, 'Size Large +₩500');
  await press(tree, '우유 우유');
  await press(tree, 'Add ₩5,500');
}));

test('a combination that left the menu is reviewed before it is ordered', () => kiosk(async tree => {
  await press(tree, '카페라떼 옵션 선택');
  for (const label of ['온도 뜨겁게', '크기 보통', '우유 두유 +700원', '담기 5,700원']) await press(tree, label);
  mockDropped = 'ice';
  mockVersion = 2;
  mockGroups[0].values.push({ key: 'warm', labels: { ko: '미지근하게' } });
  mockSoldOut = ['^hot-s-soy$'];
  try {
    await press(tree, '하드월렛으로 결제');
    expect(mockCarts).toHaveLength(0);
    expect(view(tree)).toContain('확보 못한 품목 · 카페라떼 · 뜨겁게 · 보통 · 두유 ×1');
    await press(tree, '카페라떼 옵션 선택');
    expect(button(tree, '온도 차갑게')).toBeUndefined();
    expect(button(tree, '온도 미지근하게')).toBeDefined();
  } finally { mockGroups[0].values.pop(); }
}));

test('on a narrow screen the choices and the add button stay together', async () => {
  const narrow = { width: 600, height: 960, scale: 1, fontScale: 1 };
  Dimensions.set({ window: narrow, screen: narrow });
  await kiosk(async tree => {
    await press(tree, '카페라떼 옵션 선택');
    expect(button(tree, '내 장바구니')).toBeUndefined();
    for (const label of ['온도 뜨겁게', '크기 보통', '우유 우유', '담기 5,000원']) await press(tree, label);
    expect(button(tree, '내 장바구니')).toBeDefined();
  });
});

test('the menu shows the small photo, the product page the large one, each read once', async () => {
  mockPhotos = true;
  await kiosk(async tree => {
    const shown = () => tree.root.findAll(n => typeof n.type === 'string' && !!n.props.source?.uri)
      .map(n => [n.props.accessibilityLabel, n.props.source.uri]);
    await Renderer.act(async () => { await new Promise(resolve => setTimeout(resolve, 30)); });
    expect(shown()).toEqual([['카페라떼 메뉴 사진', 'data:image/jpeg;base64,' + mockBase64(mockThumb)]]);
    await press(tree, '카페라떼 옵션 선택');
    await Renderer.act(async () => { await new Promise(resolve => setTimeout(resolve, 30)); });
    expect(shown()).toEqual([['카페라떼 메뉴 사진', 'data:image/jpeg;base64,' + mockBase64(mockPhoto)]]);
    await press(tree, '메뉴로 돌아가기');
    await press(tree, '카페라떼 옵션 선택');
    expect(mockPhotoReads).toEqual(['thumb 0', 'photo 0', 'photo 18000']);
    // The cookie has no photo and shows its name alone.
    await press(tree, '메뉴로 돌아가기');
    expect(view(tree)).toContain('쿠키');
    expect(shown()).toHaveLength(1);
  });
});

test('the basket says how much tax its prices include, line by line, and only when every line says it', async () => {
  mockTaxed = true;
  await kiosk(async tree => {
    await press(tree, '카페라떼 옵션 선택');
    for (const label of ['온도 차갑게', '크기 크게 +500원', '우유 두유 +700원', '수량 늘리기', '담기 12,400원']) await press(tree, label);
    // 12,400원 is 11,273원 and 1,127원 of tax: one line, cut to the whole won.
    expect(view(tree)).toContain('세금 1,127원 포함');
    expect(view(tree)).toContain('공급가액 11,273원 · 세금 1,127원 포함');
    // The cookie says nothing of its tax, so the basket shows no total of it.
    await press(tree, '쿠키 standard 추가');
    expect(view(tree)).not.toContain('공급가액');
    expect(view(tree)).toContain('세금 1,127원 포함');
  });
});

test('a busy store shows its minutes above the menu, and orders still go', async () => {
  mockState = { mode: 'busy', busyMinutes: 25 };
  await kiosk(async tree => {
    expect(view(tree)).toContain('지금은 약 25분 걸립니다');
    expect(view(tree)).toContain('5,000원부터');
    await press(tree, '쿠키 standard 추가');
    await press(tree, '하드월렛으로 결제');
      expect(mockCarts).toHaveLength(1);
  });
});

test('a paused store or a closed day shows why in place of the menu', async () => {
  mockState = { mode: 'paused', resumeAt: '11:20' };
  await kiosk(async tree => {
    expect(view(tree)).toContain('지금은 주문을 잠시 멈췄습니다');
    expect(view(tree)).toContain('11:20쯤 다시 받는다고');
    expect(button(tree, '카페라떼 옵션 선택')).toBeUndefined();
  });
  mockState = { mode: 'closed' };
  await kiosk(async tree => {
    expect(view(tree)).toContain('오늘은 쉬는 날입니다');
    expect(button(tree, '카페라떼 옵션 선택')).toBeUndefined();
  });
});

test('a pause that comes while paying clears the basket and says nothing was charged', async () => {
  mockPauseAtPay = true;
  await kiosk(async tree => {
    await press(tree, '쿠키 standard 추가');
    await press(tree, '하드월렛으로 결제');
    expect(mockCarts).toHaveLength(0);
    expect(view(tree)).toContain('지금은 주문을 잠시 멈췄습니다');
    expect(view(tree)).toContain('담아 둔 것을 지웠습니다. 돈은 나가지 않았습니다.');
    expect(button(tree, '하드월렛으로 결제').props.disabled).toBe(true);
  });
});
