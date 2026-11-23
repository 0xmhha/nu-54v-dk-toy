import React from 'react';
import Renderer from 'react-test-renderer';
import App from '../src/App';
import { NearbyCommerceLink, V2Journal, bindServiceRequestV2 as mockBind, toBase64, type CommerceOrder, type ServiceRequestV2 } from '@nu54/protocol';
import type { RelayProgress } from '../src/kiosk/nearbyRelay';

const ref = (n: number) => `f0551c76-7592-4850-a414-54464d24d${n.toString(16).padStart(3, '0')}`;
const mockShop = ref(1), mockTerminal = ref(2), mockSale = ref(4), mockReceipt = ref(7), mockCustomer = ref(10);
const mockAsset = '0x' + 'a'.repeat(40);
const mockNow = () => Math.floor(Date.now() / 1000);
const mockCalls: string[] = [];
let mockOrder: Partial<CommerceOrder> = {};
let mockOffline = false;
let mockTouch: (pair?: { code: string; until: number; codes: string[] }) => void;
let mockProgress: (event: RelayProgress) => void;
const mockApproved: string[] = [];
let mockEmit: (raw: string) => void;
const mockPhones = new Map<string, (data: string) => void>();
let mockHandled = 0;
const mockProfile = () => ({
  branchRef: mockShop, subjectRef: mockTerminal, tenantRef: ref(5), role: 'terminal' as const, expiresAt: mockNow() + 3_600,
});

jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
jest.mock('../src/specs/NativeNearby.ts', () => ({
  __esModule: true,
  default: {
    onNearby: (listen: typeof mockEmit) => { mockEmit = listen; return { remove() {} }; },
    write: async (link: string, data: string) => mockPhones.get(link)?.(data),
    configure: async () => {}, start: async () => {}, stop: async () => {}, close: async () => {},
  },
}));
jest.mock('../src/kiosk/nearbyRelay.ts', () => ({
  NearbyRelay: class {
    constructor(...args: unknown[]) {
      mockTouch = args[4] as typeof mockTouch;
      mockProgress = args[7] as typeof mockProgress;
    }
    rememberIssued() {}
    activate() {}
    retire() {}
    async handle() { mockHandled++; return { status: 'ok' }; }
    approveDisplayed(code: string) { mockApproved.push(code); mockTouch(); }
    clearPublic() { mockTouch(); }
    close() {}
  },
}));
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {
    loadProfile: async (slot: string) => slot === 'terminal'
      ? JSON.stringify({ scope: mockProfile(), pin: 'b'.repeat(64), nearby: { test: true } }) : '',
    publicIdentity: async () => 'key',
    exchangeScoped(slot: string, _digest: string, raw: string) { return this.exchange(slot, raw); },
    loadJournal: async () => '',
    saveJournal: async () => {},
    randomBytes: async () => 'AAAAAAAAAAAAAAAAAAAAAA==',
    exchange: async (_slot: string, raw: string) => {
      const { command } = JSON.parse(raw) as { command: ServiceRequestV2 };
      mockCalls.push(command.operation);
      if (mockOffline) throw new Error('offline');
      const { requestId, requestDigest } = mockBind(command, {
        ...mockProfile(),
        grants: [{ domainRef: command.domainRef, objectRef: command.objectRef, operations: [`C3/${command.operation}`] }],
      }, mockNow());
      const base = { requestId, requestDigest, status: 'ok', resultRef: command.objectRef };
      switch (command.operation) {
        case 'terminal.state': return JSON.stringify({ ...base, terminal: { mode: 'menu', revision: 1 } });
        case 'catalog.read': return JSON.stringify({
          ...base, shop: { branch: mockShop, branchLabel: '여울 커피 성수점' },
          catalog: [{
            version: 1, expiresAt: new Date((mockNow() + 120) * 1000).toISOString(), sku: 'tea', optionKey: 'hot',
            label: '차', asset: mockAsset, unitPrice: '9', available: 2,
          }],
        });
        case 'order.create': return JSON.stringify({ ...base, resultRef: mockSale });
        case 'order.status': return JSON.stringify({
          ...base,
          order: {
            id: mockSale, revision: 0, state: 'preview', asset: mockAsset, amount: '9',
            expiresAt: new Date((mockNow() + 300) * 1000).toISOString(), ...mockOrder,
          },
        });
        default: return JSON.stringify(base);
      }
    },
  },
}));

const checkout = (more = {}) => ({ menuCheckout: { branchLabel: '여울 커피 성수점', payerBound: true, customerRef: mockCustomer, ...more } });
const escrow = (stage: 'received' | 'submitted' | 'locked' | 'failed') => ({
  stage, escrow: '0x' + '1'.repeat(40), merchantId: '7'.repeat(64), orderKey: 'e'.repeat(64),
  payer: '0x' + '2'.repeat(40), confirmer: '0x' + '3'.repeat(40),
  ...(stage === 'submitted' || stage === 'locked' ? { lockTxHash: '0x' + '4'.repeat(64) } : {}),
  ...(stage === 'locked' ? { deliverBy: mockNow() + 600 } : {}),
});
let tree: Renderer.ReactTestRenderer;
const text = () => JSON.stringify(tree.toJSON());
const button = (label: string) => tree.root.findAll(n => n.props.label === label && typeof n.props.run === 'function')[0];
const press = async (label: string) => {
  expect(button(label)).toBeDefined();
  await Renderer.act(async () => button(label).props.run());
};
const pass = (ms: number) => Renderer.act(async () => { await jest.advanceTimersByTimeAsync(ms); });
const reads = () => mockCalls.filter(op => op === 'order.status').length;

beforeEach(async () => {
  jest.useFakeTimers({ doNotFake: ['setImmediate', 'clearImmediate'] });
  mockCalls.length = mockApproved.length = 0;
  mockOrder = {};
  mockOffline = false;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  await press('새 고객 주문');
  await press('차 hot 추가');
  await press('하드월렛으로 결제');
  expect(text()).toContain('이 주문을 여는 일회용 QR');
});
afterEach(async () => {
  await Renderer.act(async () => tree.unmount());
  jest.useRealTimers();
});

async function connectAndQuote(lifetime = 100) {
  await Renderer.act(async () => mockTouch({ code: '37ea19cd', until: mockNow() + 35, codes: ['37ea19cd', '2ba8cf04'] }));
  await press('확인 번호 2ba8cf04 휴대전화에 연결');
  expect(mockApproved).toEqual(['2ba8cf04']);
  expect(text()).toContain('연결한 휴대전화 · 2ba8cf04');
  expect(text()).not.toContain('이 주문을 여는 일회용 QR');
  mockOrder = { revision: 1, state: 'issued', quoteDigest: 'd'.repeat(64), orderKey: 'e'.repeat(64), ...checkout() };
  await Renderer.act(async () => mockProgress({ sale: mockSale, stage: 'quoted', until: mockNow() + lifetime }));
  await pass(2_100);
  expect(text()).toContain('휴대전화에서 승인 중');
}

test('a paid order moves by itself from the phone to the order number and back to the first screen', async () => {
  await connectAndQuote();
  await pass(61_000);
  expect(text()).toContain('휴대전화에서 승인 중');
  await Renderer.act(async () => mockProgress({ sale: mockSale, stage: 'submitting', until: mockNow() + 30 }));
  expect(text()).toContain('결제 확인 중입니다');
  for (const stage of ['received', 'submitted'] as const) {
    mockOrder = { revision: 2, state: 'checking', escrow: escrow(stage), ...checkout() };
    await pass(5_100);
    expect(text()).toContain('결제 확인 중입니다');
    expect(text()).not.toContain('결제 완료');
    expect(text()).not.toContain('주문번호');
  }
  mockOrder = { revision: 3, state: 'paid', pickupNo: 17, receiptRef: mockReceipt, escrow: escrow('locked'), ...checkout() };
  await pass(5_100);
  expect(text()).toContain('결제 완료');
  expect(text()).toContain('주문번호 17');
  expect(text()).toContain('영수증 ' + mockReceipt.slice(-8).toUpperCase());
  for (const internal of [mockSale, mockReceipt, 'd'.repeat(64), 'locked', 'submitted', '2ba8cf04']) expect(text()).not.toContain(internal);
  const settled = reads();
  await pass(9_000);
  expect(reads()).toBe(settled);
  expect(text()).toContain('결제 완료');
  await pass(1_500);
  expect(text()).toContain('새 고객 주문');
  expect(text()).not.toContain('주문번호');
  await pass(60_000);
  expect(reads()).toBe(settled);
  expect(mockCalls.filter(op => op === 'order.create')).toHaveLength(1);
  expect(mockCalls).not.toContain('order.cancel-public');
});

test('an unresolved submission never offers another payment and leaves within its bounded time', async () => {
  await connectAndQuote(40);
  mockOrder = { revision: 2, state: 'checking', escrow: escrow('submitted'), ...checkout() };
  await Renderer.act(async () => mockProgress({ sale: mockSale, stage: 'submitting', until: mockNow() + 38 }));
  await pass(39_000);
  expect(text()).toContain('결제 결과를 아직 확인하지 못했습니다');
  expect(text()).toContain('다시 결제하지 마세요.');
  for (const label of ['새 고객 주문', '하드월렛으로 결제', '직접 승인할 보드 찾기', '처음 화면으로']) expect(button(label)).toBeUndefined();
  expect(text()).not.toContain('결제 완료');
  mockOffline = true;
  const before = reads();
  await pass(55_000);
  expect(reads() - before).toBeLessThanOrEqual(12);
  await pass(10_000);
  expect(text()).toContain('새 고객 주문');
  const after = reads();
  await pass(60_000);
  expect(reads()).toBe(after);
});

test('a late result still reaches the screen while it is shown', async () => {
  await connectAndQuote(20);
  await pass(25_000);
  expect(text()).toContain('승인 시간이 지났습니다');
  mockOrder = { revision: 3, state: 'paid', pickupNo: 18, receiptRef: mockReceipt, escrow: escrow('locked'), ...checkout() };
  await pass(6_000);
  expect(text()).toContain('주문번호 18');
});

test('a lock that can no longer execute says that nothing was taken', async () => {
  await connectAndQuote();
  mockOrder = { revision: 2, state: 'checking', escrow: escrow('failed'), ...checkout() };
  await pass(5_100);
  expect(text()).toContain('빠져나간 돈은 없습니다');
  expect(text()).toContain('처음 화면에서 다시 주문할 수 있습니다.');
  await press('화면을 비우고 대기');
  expect(text()).toContain('새 고객 주문');
});

test('leaving while a phone may still approve asks first and says the payment can still go through', async () => {
  await connectAndQuote();
  await press('화면을 비우고 대기');
  expect(text()).toContain('휴대전화에서 이미 승인했다면 결제는 그대로 진행됩니다.');
  mockOrder = { revision: 3, state: 'paid', pickupNo: 19, receiptRef: mockReceipt, escrow: escrow('locked'), ...checkout() };
  await pass(5_100);
  expect(text()).toContain('주문번호 19');
});

test('an accepted phone that never fetches its quote gives the terminal back', async () => {
  await Renderer.act(async () => mockTouch({ code: '37ea19cd', until: mockNow() + 35, codes: ['37ea19cd'] }));
  await pass(30_000);
  await press('확인 번호가 같은 휴대전화에 연결');
  await pass(20_000);
  expect(text()).toContain('연결한 휴대전화 · 37ea19cd');
  await pass(16_000);
  expect(text()).toContain('연결 시간이 지났습니다');
  await pass(60_000);
  expect(text()).toContain('새 고객 주문');
});

test('a phone that disconnects while approving is named without offering another payment', async () => {
  await connectAndQuote();
  await Renderer.act(async () => mockProgress({ sale: mockSale, stage: 'disconnected' }));
  expect(text()).toContain('휴대전화와의 연결이 끊어졌습니다.');
  expect(text()).toContain('다시 결제하지 마세요.');
  for (const label of ['새 고객 주문', '하드월렛으로 결제', '직접 승인할 보드 찾기']) expect(button(label)).toBeUndefined();
  expect(text()).not.toContain('이 주문을 여는 일회용 QR');
  await pass(6_000);
  expect(text()).toContain('휴대전화와의 연결이 끊어졌습니다.');
  await Renderer.act(async () => mockProgress({ sale: mockSale, stage: 'connected' }));
  expect(text()).not.toContain('휴대전화와의 연결이 끊어졌습니다.');
  await Renderer.act(async () => mockProgress({ sale: mockSale, stage: 'disconnected' }));
  mockOrder = { revision: 3, state: 'paid', pickupNo: 21, receiptRef: mockReceipt, escrow: escrow('locked'), ...checkout() };
  await pass(5_100);
  expect(text()).toContain('주문번호 21');
  expect(text()).not.toContain('연결이 끊어졌습니다');
});

test('the terminal keeps answering past 128 nearby requests from several phones', async () => {
  const id = (n: number) => { const bytes = new Uint8Array(16); new DataView(bytes.buffer).setUint32(0, n + 1); return bytes; };
  mockHandled = 0;
  for (let n = 0; n < 160; ) {
    const phone = n / 10 % 4;
    const peer = {
      kind: 'ready' as const, link: 'link-' + n, principal: ref(40 + phone), branch: mockShop, expiresAt: mockNow() + 600,
      peer: (phone + 1).toString(16).padStart(64, '0'), epoch: toBase64(id(n)),
    };
    mockEmit(JSON.stringify(peer));
    const customer = new NearbyCommerceLink(peer, 'customer', {
      write: async (link, clear) => mockEmit(JSON.stringify({ kind: 'data', link, clear })), close: async () => {},
    }, new V2Journal(4));
    mockPhones.set(peer.link, data => customer.receive(data));
    for (let i = 0; i < 10; i++, n++)
      expect(await customer.request({ op: 'discover', branch: mockShop }, id(n))).toEqual({ status: 'ok' });
    mockEmit(JSON.stringify({ kind: 'closed', link: peer.link }));
  }
  expect(mockHandled).toBe(160);
});
