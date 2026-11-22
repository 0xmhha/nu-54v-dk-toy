import React from 'react';
import {AsyncLocalStorage} from 'node:async_hooks';
import { AppState, Linking, type AppStateStatus } from 'react-native';
import Renderer from 'react-test-renderer';
import {
  bindServiceRequestV2 as mockBind,
  toBase64 as mockBase64,
  encodeOrderLocator,
  makeEntranceQr,
  paymentProfile,
  presentationFingerprint,
  type ServiceRequestV2,
} from '@nu54/protocol';
import App from '../src/App';
import Http from '../src/specs/NativeCommerceHttp.ts';
import Alerts from '../src/specs/NativeSubscriptionAlerts.ts';
import {CustomerOrders} from '../src/merchant/CustomerOrders.tsx';

const ref = (n: number) =>
  `0fbc54ef-2827-4ab5-bfcd-3a27611c3${n.toString(16).padStart(3, '0')}`;
const mockBranch = ref(1),
  subject = ref(2),
  tenant = ref(3),
  mockSale = ref(4);
const mockCustomerA = subject;
const mockShop = {branch: mockBranch, branchLabel: '여울 커피 성수점'};
const mockTerminal = ref(11);
const mockInvoice = ref(8), mockInvoiceSale = ref(9), mockSchedule = ref(10);
let mockShowInvoice = false;
const mockRequests: string[] = [];
type JourneyRequest = {operation: string; branch: string; order: string};
type MerchantJourney = {
  requests: JourneyRequest[];
  journals: Map<string, string>;
  confirmed: boolean;
  beforeClaim?: () => Promise<void>;
};
const mockJourney = new AsyncLocalStorage<MerchantJourney>();
const mockJournals = new Map<string, string>();
let mockConfirmed = false;
let mockPickupStage: "ready" | "collected" | undefined;
let mockActiveSubject = subject;
let mockOrderBarrier: Promise<void> | undefined;
const mockProfileExpiry = Math.floor(Date.now() / 1000) + 300;
let mockTeaPrice = '3500000';
let mockTeaAvailable = 2;
let mockPairStock: number | undefined;
let mockLegacyPair = false;
let mockCreateResult: 'unknown' | 'stock' | 'ok' = 'ok';
const mockNearbyOrderOps: string[] = [];
const mockCreateKeys: string[] = [];
const mockAsset = '0x' + 'a'.repeat(40);
const mockUnix = () => Math.floor(Date.now() / 1000);
const mockScope = () => ({
  role: 'customer',
  branchRef: mockBranch,
  subjectRef: mockActiveSubject,
  tenantRef: tenant,
  expiresAt: mockProfileExpiry,
});

let mockUseNearby = false;
let mockNearbyCatalogFault = false;
let mockNearbyRestocked = false;
let mockK01Categories = false;
let mockK01Disconnected = false;
const mockK01Pages: string[] = [];
const mockCatalogRows = () => {
  const line = (
    sku: string,
    optionKey: string,
    label: string,
    unitPrice: string,
    available: number,
  ) => ({
    sku,
    optionKey,
    label,
    unitPrice,
    available,
    ...(sku === "tea" && mockPairStock !== undefined ? {skuAvailable: mockPairStock} : {}),
    asset: mockAsset,
    version: 1,
    expiresAt: new Date((mockUnix() + 60) * 1000).toISOString(),
  });
  return [
    line('tea', 'hot', '차', mockTeaPrice, mockTeaAvailable),
    ...(mockPairStock === undefined && !mockLegacyPair ? [] : [line('tea', 'iced', '차', '3500000', 2)]),
    line('bread', 'plain', '빵', '3000000', 2),
  ];
};
jest.mock('../src/specs/NativeNearby.ts', () => ({
  __esModule: true,
  default: {},
}));
jest.mock('../src/merchant/nearbyClient.ts', () => ({
  CustomerNearby: class {
    close() {}
    disconnect() {}
    async scan() {
      return [{ handle: 'fixture-terminal' }];
    }
    async connect() {
      return {
        peer: {principal: mockTerminal},
        request: async ({
          op,
          key,
          offset,
          cart,
        }: {
          op: string;
          key?: string;
          offset?: number;
          cart?: {items: {sku: string; option_key: string; quantity: number}[]};
        }) => {
          if (op === 'discover') return {status: 'ok', value: {...mockShop, terminal: mockTerminal}};
          if (op === 'catalog') {
            const rows = mockCatalogRows();
            return {
              status: 'ok',
              value:
                offset && !mockNearbyCatalogFault
                  ? { rows: rows.slice(1) }
                  : { rows: rows.slice(0, 1), next: (offset ?? 0) + 1 },
            };
          }
          if (op !== 'reserve' && op !== 'status') throw new Error('unexpected nearby operation');
          mockNearbyOrderOps.push(op);
          mockCreateKeys.push(key!);
          if (mockCreateResult === 'unknown') return { status: 'pending' };
          mockTeaAvailable = mockNearbyRestocked ? 2 : 1;
          return { status: 'sold-out', value: cart?.items.filter(line => line.sku === 'tea') ??
            [{sku: 'tea', option_key: 'hot', quantity: 2}] };
        },
      };
    }
  },
}));
jest.mock(
  'react-native-safe-area-context',
  () => require('react-native-safe-area-context/jest/mock').default,
);
jest.mock('../src/specs/NativeRenterBle.ts', () => ({
  __esModule: true,
  default: { loadDevices: async () => '' },
}));
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({
  __esModule: true,
  default: { clear: async () => {} },
}));
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {
    loadProfile: async () =>
      JSON.stringify({
        scope: mockScope(),
        pin: 'a'.repeat(64),
        nearby: !mockJourney.getStore() && mockUseNearby ? {} : undefined,
      }),
    configure: async () => {},
    loadJournal: async (name: string) => (mockJourney.getStore()?.journals ?? mockJournals).get(name) ?? '',
    saveJournal: async (name: string, value: string) =>
      (mockJourney.getStore()?.journals ?? mockJournals).set(name, value),
    randomBytes: async () => mockBase64(new Uint8Array(16).fill(5)),
    exchange: async (_slot: string, raw: string) => {
      const { command, travel } = JSON.parse(raw) as {
        command: ServiceRequestV2; travel?: {operation: string; branch: string};
      };
      const journey = mockJourney.getStore();
      if (travel) {
        if (journey) journey.requests.push({operation: travel.operation, branch: travel.branch, order: ''});
        else mockRequests.push(`${travel.operation}/${travel.branch}`);
        if (travel.operation === 'my-bookings' && travel.branch === mockBranch)
          return JSON.stringify({bookings: []});
        throw new Error('unexpected travel request');
      }
      if (journey) journey.requests.push({operation: command.operation, branch: command.domainRef, order: command.objectRef});
      else mockRequests.push(command.operation);
      if (command.operation === 'checkout.claim') await journey?.beforeClaim?.();
      const confirmed = journey?.confirmed ?? mockConfirmed;
      const scope = {
        ...mockScope(),
        grants: [
          {
            domainRef: mockBranch,
            objectRef: command.objectRef,
            operations: [`C3/${command.operation}`],
          },
        ],
      };
      const { requestId, requestDigest } = mockBind(command, scope, mockUnix());
      const envelope = {
        requestId,
        requestDigest,
        status: 'ok',
        resultRef: mockSale,
      };
      if (command.operation === 'order.list') {
        const barrier = mockOrderBarrier;
        if (barrier) await barrier;
        return JSON.stringify({...envelope, resultRef: command.objectRef, orders: confirmed && command.objectRef === mockCustomerA ? [
          {id: mockSale, state: 'preview', amount: '5', asset: mockAsset},
        ] : []});
      }
      if (command.operation === 'invoice.list')
        return JSON.stringify({...envelope, resultRef: command.objectRef, invoices: mockShowInvoice ? [{
          id: mockInvoice, saleRef: mockInvoiceSale, subscriptionRef: mockSchedule, cycle: '2026-11-20',
          dueAt: new Date((mockUnix() + 60) * 1000).toISOString(), state: 'preview',
          amount: '14', asset: mockAsset, fulfillment: 'digital',
        }] : []});
      if (command.operation === 'catalog.read' && mockK01Categories) {
        mockK01Pages.push(command.body.page);
        if (mockK01Disconnected) throw Object.assign(new Error('offline'), {code:'COMMERCE_NETWORK_OFFLINE'});
        const [drink, bread] = mockCatalogRows();
        const catalog = command.body.page === '0' ? [
          {...drink, category:{key:'warm',sortOrder:1,labels:{ko:'음료',en:'Drinks'}},
            metadata:{labels:{ko:'차',en:'Tea'},optionLabels:{ko:'따뜻하게',en:'Hot'}}},
          ...Array.from({length:49}, (_, i) => ({...bread, sku:'item-'+i, label:'오늘의 메뉴'})),
        ] : [
          {...bread, category:{key:'oven',sortOrder:2,labels:{ko:'베이커리',en:'Bakery'}}},
          {...bread, sku:'daily', label:'한정 메뉴'},
        ];
        return JSON.stringify({...envelope, resultRef:mockBranch, shop:mockShop, catalog, catalogVersion:1});
      }
      if (command.operation === 'catalog.read')
        return JSON.stringify({
          ...envelope,
          resultRef: mockBranch,
          catalog: mockCatalogRows(),
          shop: mockShop,
        });
      if (command.operation === 'order.create') {
        mockCreateKeys.push(command.idempotencyKey!);
        if (mockCreateResult === 'unknown') throw new Error('timeout');
        if (mockCreateResult === 'stock') {
          mockTeaAvailable = 1;
          return JSON.stringify({
            requestId,
            requestDigest,
            status: 'error',
            error: 'OUT_OF_STOCK',
          });
        }
        return JSON.stringify(envelope);
      }
      if (command.operation === 'checkout.request')
        return JSON.stringify({
          ...envelope,
          comparison: { code: '76e5c1d3', expiresAt: mockUnix() + 30 },
        });
      if (command.operation === 'checkout.claim')
        return JSON.stringify({
          ...envelope,
          claim: confirmed
            ? { claimDigest: 'a'.repeat(64), expiresAt: mockUnix() + 60 }
            : undefined,
        });
      if (command.operation === 'order.status' && mockPickupStage)
        return JSON.stringify({
          ...envelope,
          resultRef: command.objectRef,
          order: {
            id: command.objectRef, revision: 9, state: 'paid', asset: mockAsset, amount: '6500000',
            pickupNo: 17, receiptRef: ref(17),
            expiresAt: new Date((mockUnix() + 45) * 1000).toISOString(),
            payment: { stage: 'finalized', chainId: 8283, txHash: '0x' + 'b'.repeat(64),
              includedAt: new Date(mockUnix() * 1000).toISOString(), pickupStage: mockPickupStage },
          },
        });
      if (command.operation === 'order.status' && confirmed)
        return JSON.stringify({
          ...envelope,
          resultRef: command.objectRef,
          order: {
            id: command.objectRef,
            revision: 0,
            state: 'preview',
            asset: '0x' + 'a'.repeat(40),
            amount: '5',
            expiresAt: new Date((mockUnix() + 45) * 1000).toISOString(),
          },
        });
      throw new Error('unexpected request');
    },
  },
}));

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>(done => {resolve = done;});
  return {promise, resolve};
}

// This ledger belongs to the rendering/async chain that issued the request.
// A late completion from a timed-out test cannot write to the next journey.
function runMerchantJourney(run: (journey: MerchantJourney, ui: ReturnType<typeof merchantJourneyUI>) => Promise<void>) {
  const journey: MerchantJourney = {requests: [], journals: new Map(), confirmed: false};
  const ui = merchantJourneyUI();
  cleanupMerchantJourney = ui.close;
  return mockJourney.run(journey, async () => {
    try {
      await run(journey, ui);
    } finally {
      await ui.close();
    }
  });
}

let cleanupMerchantJourney: (() => Promise<void>) | undefined;
let stopRegistration: (() => void) | undefined;
let registrationBody: Promise<void> | undefined;
afterEach(async () => {
  const journey = cleanupMerchantJourney;
  const stop = stopRegistration;
  const body = registrationBody;
  cleanupMerchantJourney = undefined;
  stopRegistration = undefined;
  registrationBody = undefined;
  await journey?.();
  stop?.();
  await body?.catch(() => {});
});

function merchantJourneyUI() {
  let tree: Renderer.ReactTestRenderer | undefined;
  let initial: ReturnType<typeof jest.spyOn> | undefined;
  let cleanup: (() => void) | undefined;
  let closing: Promise<void> | undefined;

  async function unmount() {
    const visible = tree;
    tree = undefined;
    if (visible) await Renderer.act(async () => visible.unmount());
  }

  return {
    async mount(url: string) {
      await unmount();
      if (closing) throw new Error('Merchant journey already closed');
      initial?.mockRestore();
      initial = jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(url);
      await Renderer.act(async () => { tree = Renderer.create(<App />); });
    },
    text() {
      return JSON.stringify(tree?.toJSON());
    },
    async tap(label: string) {
      const match = tree!.root.findAll(node => {
        const props = node.props;
        return (props.label === label && typeof props.run === 'function') ||
          (props.title === label && typeof props.onPress === 'function');
      })[0];
      expect(match).toBeDefined();
      expect(match.props.disabled).not.toBe(true);
      await Renderer.act(async () => {
        (match.props.run ?? match.props.onPress)();
      });
    },
    onClose(release: () => void) {
      cleanup = release;
    },
    close() {
      if (!closing) {
        cleanup?.();
        initial?.mockRestore();
        closing = unmount();
      }
      return closing;
    },
  };
}

async function waitForJourneyText(ui: ReturnType<typeof merchantJourneyUI>, text: string) {
  // Flush React updates until the business result is visible. No elapsed sleep
  // represents success, and the test always unmounts if the condition fails.
  for (let i = 0; i < 20 && !ui.text().includes(text); i++) {
    await Renderer.act(async () => {});
  }
  expect(ui.text()).toContain(text);
}

async function firstRegistration() {
  let stored = '';
  const raw = JSON.stringify({
    scope: mockScope(),
    pin: 'a'.repeat(64),
    payment: {
      chain: 8283, contract: '0x' + 'b'.repeat(40), beneficiary: '0x' + 'c'.repeat(40),
      signer: '0x' + 'd'.repeat(40), asset: mockAsset, merchantId: 'e'.repeat(64),
      keyVersion: 1, beneficiaryVersion: 1, maxBaseUnits: '30000000', presentationKey: 'f'.repeat(64),
    },
  });
  jest.spyOn(Http!, 'loadProfile').mockImplementation(async () => stored);
  const configure = jest.spyOn(Http!, 'configure').mockImplementation(async (slot, proposed) => {
    expect(slot).toBe('customer');
    stored = proposed;
  });
  const reminderJournal = 'subscription-alerts-v2';
  const previousScope = JSON.stringify({branch: ref(20), owner: ref(21), enabled: true, scheduled: [ref(22)]});
  mockJournals.set(reminderJournal, previousScope);
  const cleanup = deferred();
  const clear = jest.spyOn(Alerts!, 'clear')
    .mockRejectedValueOnce(Object.assign(new Error('native error detail'), {code: 'CLEAR_DEADLINE'}))
    .mockResolvedValueOnce(undefined)
    .mockImplementationOnce(() => cleanup.promise);
  const saveJournal = Http!.saveJournal;
  let failReminderWrite = true;
  jest.spyOn(Http!, 'saveJournal').mockImplementation(async (name, value) => {
    if (name === reminderJournal && failReminderWrite) {
      failReminderWrite = false;
      throw new Error('native journal error detail');
    }
    await saveJournal(name, value);
  });
  jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(null);
  let tree: Renderer.ReactTestRenderer | undefined;
  let closed = false;
  function activeTree() {
    if (!tree || closed) throw new Error('Registration renderer is closed');
    return tree;
  }
  async function unmount() {
    const visible = tree;
    tree = undefined;
    if (visible) await Renderer.act(async () => visible.unmount());
  }
  const stop = () => {
    closed = true;
    cleanup.resolve();
  };
  stopRegistration = stop;
  const open = async () => {
    if (closed) throw new Error('Registration renderer is closed');
    await Renderer.act(async () => { tree = Renderer.create(<App />); });
    // Nearby shop orders open from the 둘러보기 tab.
    await Renderer.act(async () => tree!.root.find(node => node.props.accessibilityRole === 'tab' && node.props.accessibilityLabel === '둘러보기').props.onPress());
    await tap('가까운 매장에서 주문');
  };
  const button = (label: string) => activeTree().root.findAll(node =>
    (node.props.label === label && typeof (node.props.run ?? node.props.onPress) === 'function') ||
    (node.props.title === label && typeof node.props.onPress === 'function'))[0];
  const tap = async (label: string) => {
    const node = button(label);
    expect(node).toBeDefined();
    expect(node.props.disabled).toBeFalsy();
    await Renderer.act(async () => (node.props.run ?? node.props.onPress)());
  };
  const field = (id: string) => activeTree().root.findAll(node =>
    node.props.testID === id && typeof node.props.onChangeText === 'function')[0];
  const update = async (id: string, value: string) => {
    await Renderer.act(async () => field(id).props.onChangeText(value));
  };
  const render = () => JSON.stringify(activeTree().toJSON());
  try {
    await open();
    await update('customer-commerce-registration', raw);
    await update('customer-commerce-tls-pin', '0'.repeat(64));
    const proof = presentationFingerprint(paymentProfile(raw)!);
    await update('customer-commerce-presentation-pin', proof);
    await tap('주문 등록 확인');
    expect(render()).toContain('등록서를 확인할 수 없습니다');
    expect(configure).not.toHaveBeenCalled();

    await update('customer-commerce-tls-pin', 'a'.repeat(64));
    await update('customer-commerce-presentation-pin', '0'.repeat(64));
    await tap('주문 등록 확인');
    expect(render()).toContain('등록서를 확인할 수 없습니다');
    expect(configure).not.toHaveBeenCalled();

    await update('customer-commerce-presentation-pin', proof);
    await tap('주문 등록 확인');
    expect(render()).toContain('시스템 알림을 확인하는 시간이 초과됐습니다. 등록을 다시 시도하세요.');
    expect(render()).not.toContain('native error detail');
    expect(configure).not.toHaveBeenCalled();
    expect(mockJournals.get(reminderJournal)).toBe(previousScope);

    await tap('주문 등록 확인');
    expect(render()).toContain('구독 알림 기록을 저장할 수 없습니다. 등록을 다시 시도하세요.');
    expect(render()).not.toContain('native journal error detail');
    expect(configure).not.toHaveBeenCalled();
    expect(mockJournals.get(reminderJournal)).toBe(previousScope);

    await Renderer.act(async () => field('customer-commerce-presentation-pin').props.onSubmitEditing());
    expect(clear).toHaveBeenCalledTimes(3);
    expect(button('주문 등록 확인').props.disabled).toBeTruthy();
    expect(configure).not.toHaveBeenCalled();
    expect(mockJournals.get(reminderJournal)).toBe(previousScope);
    await Renderer.act(async () => cleanup.resolve());
    expect(mockJournals.get(reminderJournal)).toBe('');
    expect(configure).toHaveBeenCalledTimes(1);
    expect(stored).toBe(raw);
    expect(render()).not.toContain('주문 등록 확인');

    await unmount();
    await open();
    await tap('온라인으로 매장 확인');
    await tap('이 매장 메뉴 열기');
    await tap('차 hot 추가');
    expect(render()).toContain('여울 커피 성수점');
    expect(render()).toContain('합계 · 3.50 TestUSDC');
    expect(configure).toHaveBeenCalledTimes(1);
  } finally {
    stop();
    try {
      await unmount();
    } finally {
      mockJournals.delete(reminderJournal);
      jest.restoreAllMocks();
    }
  }
}

test('first registration verifies independent pins and opens the same store after app restart', () => {
  registrationBody = firstRegistration();
  return registrationBody;
}, 20_000);

test('OS QR entry asks for proof before reading any private order, then sees the same matched order', () =>
  runMerchantJourney(async (journey, ui) => {
    const delayedClaim = deferred();
    const claimStarted = deferred();
    ui.onClose(delayedClaim.resolve);
    journey.beforeClaim = () => {
      claimStarted.resolve();
      journey.beforeClaim = undefined;
      return delayedClaim.promise;
    };
    const orderRequests = () => journey.requests.filter(request => request.order === mockSale);
    const operations = () => orderRequests().map(request => request.operation);
    await ui.mount(encodeOrderLocator(
      {branchRef: mockBranch, orderRef: mockSale, expiresAt: mockUnix() + 45},
      mockUnix(),
    ));
    await waitForJourneyText(ui, '이 계정에서 열 수 없는 주문입니다.');
    expect(ui.text()).toContain('내 매장 주문');
    expect(operations()).toEqual(['order.status']);
    expect(ui.text()).not.toContain('기준 단위');
    await ui.tap('매장 QR 대조하기');
    await ui.tap('QR 주문 연결');
    await claimStarted.promise;
    expect(operations()).toEqual(['order.status', 'checkout.claim']);
    expect(ui.text()).not.toContain('76e5c1d3');
    expect(ui.text()).not.toContain('기준 단위');
    await Renderer.act(async () => delayedClaim.resolve());
    await waitForJourneyText(ui, '76e5c1d3');
    expect(ui.text()).not.toContain('기준 단위');
    expect(operations()).toEqual(['order.status', 'checkout.claim', 'checkout.request']);

    journey.confirmed = true;
    await ui.tap('같은 주문 연결 확인');
    await waitForJourneyText(ui, '기준 단위');
    expect(operations()).toEqual([
      'order.status', 'checkout.claim', 'checkout.request', 'checkout.claim', 'order.status',
    ]);
    for (const request of journey.requests) {
      expect(request.branch).toBe(mockBranch);
      if (request.operation === 'order.list' || request.operation === 'invoice.list') {
        expect(request.order).toBe(subject);
      } else {
        expect(request.order).toBe(mockSale);
      }
    }
  }),
);

test('K03 opens the actual entrance link through the normal application and rejects a mismatched branch', () =>
  runMerchantJourney(async (journey, ui) => {
    await ui.mount(makeEntranceQr(ref(31)));
    await waitForJourneyText(ui, '등록 지점의 매장 입구 QR을 확인하세요.');
    expect(journey.requests).toEqual([]);

    await ui.mount(makeEntranceQr(mockBranch));
    await waitForJourneyText(ui, '여울 커피 성수점에서 주문');
    await ui.tap('이 매장 메뉴 열기');
    await waitForJourneyText(ui, '여울 커피 성수점 메뉴');
    expect(journey.requests.length).toBeGreaterThan(0);
    for (const request of journey.requests) {
      expect(request).toEqual({operation: 'catalog.read', branch: mockBranch, order: mockBranch});
    }
  }),
);

test('K03 retains the denied permission fallback after the OS dialog suspends the screen', async () => {
  mockUseNearby = true;
  mockJournals.clear();
  let notify: ((state: AppStateStatus) => void) | undefined;
  jest.spyOn(AppState, 'addEventListener').mockImplementation((_, handler) => {
    notify = handler;
    return {remove: () => {}};
  });
  let owned!: (allowed: boolean) => void;
  const permission = jest.fn(() => new Promise<boolean>(resolve => { owned = resolve; }));
  let tree!: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async () => {
      tree = Renderer.create(<CustomerOrders initialLocator="" shopEntry back={() => {}} permission={permission} />);
    });
    await Renderer.act(async () => {
      tree.root.findAll(node => node.props.title === '근처 단말 찾기' &&
        typeof node.props.onPress === 'function')[0].props.onPress();
    });
    expect(permission).toHaveBeenCalledTimes(1);
    await Renderer.act(async () => { notify?.('background'); });
    await Renderer.act(async () => { notify?.('active'); });
    await Renderer.act(async () => { owned(false); });
    expect(JSON.stringify(tree.toJSON())).toContain('매장 입구의 QR을 스캔하세요');
    expect(JSON.stringify(tree.toJSON())).toContain('카메라로 매장 QR 확인');
  } finally {
    owned?.(false);
    await Renderer.act(async () => { tree?.unmount(); });
    mockUseNearby = false;
    jest.restoreAllMocks();
  }
});

test.each([[false, false], [true, false], [true, true]])(
  'K10 retains the pending cart and reconfirms prices after restart (nearby=%s, restocked=%s)',
  async (nearby, restocked) => {
    mockUseNearby = nearby;
    mockNearbyRestocked = restocked;
    jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(null);
    mockJournals.clear();
    mockRequests.length = 0;
    mockCreateKeys.length = 0;
    mockNearbyOrderOps.length = 0;
    mockTeaPrice = '3500000';
    mockTeaAvailable = 2;
    mockCreateResult = 'unknown';
    let tree: Renderer.ReactTestRenderer;
    const open = async () => {
      await Renderer.act(async () => {
        tree = Renderer.create(<App />);
      });
      await Renderer.act(async () => tree.root.find(node => node.props.accessibilityRole === 'tab' && node.props.accessibilityLabel === '둘러보기').props.onPress());
      await Renderer.act(async () => {
        tree.root.findAll(node => node.props.accessibilityLabel === '가까운 매장에서 주문' &&
          typeof node.props.onPress === 'function')[0].props.onPress();
      });
    };
    const tap = async (label: string) => {
      const node = tree.root.findAll(
        candidate =>
          (candidate.props.label === label && typeof candidate.props.run === 'function') ||
          (candidate.props.title === label && typeof candidate.props.onPress === 'function'),
      )[0];
      expect(node).toBeDefined();
      expect(node.props.disabled).toBeFalsy();
      await Renderer.act(async () => (node.props.run ?? node.props.onPress)());
    };
    const screen = () => JSON.stringify(tree.toJSON());
    const connect = async () => {
      if (nearby) {
        await tap('근처 단말 찾기');
        await tap('근거리 단말 후보 1 인증');
      } else await tap('온라인으로 매장 확인');
      await tap('이 매장 메뉴 열기');
    };
    await open();
    try {
      if (nearby) {
        mockNearbyCatalogFault = true;
        await connect();
        expect(screen()).toContain('새 주문을 시작할 수 없습니다');
        expect(screen()).not.toContain('차 hot 추가');
        mockNearbyCatalogFault = false;
        await tap('다른 매장 찾기');
      }
      await connect();
      await tap('차 hot 추가');
      await tap('차 hot 추가');
      await tap('빵 plain 추가');
      mockTeaPrice = '4000000';
      await tap('주문 접수 요청');
      expect(screen()).toContain('기존 합계 · 10000000 기준 단위');
      expect(screen()).toContain('새 합계 · 11000000 기준 단위');
      expect(mockCreateKeys).toHaveLength(0);
      await tap('바뀐 장바구니 확인 후 주문 접수');
      expect(mockCreateKeys).toHaveLength(1);
      if (nearby) expect(screen()).toContain('발급 여부 미확인');
      const key = [...mockJournals.keys()].find(name => name.startsWith('customer-order-'))!;
      const saved = mockJournals.get(key);
      expect(saved).toBeTruthy();
      await Renderer.act(async () => tree.unmount());
      mockCreateResult = 'stock';
      await open();
      await connect();
      const retry = nearby ? '원래 요청 재조회' : '저장한 주문 다시 확인';
      expect(screen()).toContain(retry);
      await tap(retry);
      expect(mockCreateKeys).toEqual([mockCreateKeys[0], mockCreateKeys[0]]);
      if (nearby) {
        expect(mockNearbyOrderOps).toEqual(['reserve', 'status']);
        expect(mockJournals.get(key)).toEqual(saved);
        expect(screen()).toContain('선택한 상품이나 옵션의 재고가 부족합니다');
        expect(screen()).toContain('확보 못한 품목 · 차 · hot ×2');
        expect(screen()).toContain('선택 유지 · 빵 · plain ×1');
        await tap('수량·옵션 수정');
      }
      expect(mockJournals.get(key)).toEqual('null');
      if (restocked) {
        expect(screen()).not.toContain('확보 못한 품목');
        expect(screen()).toContain('합계 · 11000000 기준 단위');
      } else {
        expect(screen()).toContain('확보 못한 품목 · 차 · hot ×1');
        expect(screen()).toContain('합계 · 7000000 기준 단위');
      }
      expect(screen()).toContain('선택 1 · 가능 2');
    } finally {
      await Renderer.act(async () => tree.unmount());
      mockUseNearby = false;
      mockNearbyCatalogFault = false;
      mockNearbyRestocked = false;
      jest.restoreAllMocks();
    }
  },
);

test.each(['direct', 'nearby', 'legacy'])('keeps independent selection and saved checkout (%s)', async route => {
  const nearby = route === 'nearby';
  mockUseNearby = nearby;
  mockLegacyPair = route === 'legacy';
  mockPairStock = mockLegacyPair ? undefined : 3;
  mockTeaPrice = '4000000';
  mockTeaAvailable = 2;
  mockCreateResult = 'unknown';
  mockCreateKeys.length = 0;
  mockJournals.clear();
  jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(null);
  let tree!: Renderer.ReactTestRenderer;
  const tap = async (label: string) => {
    const node = tree.root.findAll(candidate =>
      (candidate.props.label === label && typeof (candidate.props.run ?? candidate.props.onPress) === 'function') ||
      (candidate.props.title === label && typeof candidate.props.onPress === 'function'))[0];
    expect(node).toBeDefined();
    expect(node.props.disabled).toBeFalsy();
    await Renderer.act(async () => (node.props.run ?? node.props.onPress)());
  };
  const open = async () => {
    await Renderer.act(async () => { tree = Renderer.create(<App />); });
    // Nearby shop orders open from the 둘러보기 tab.
    await Renderer.act(async () => tree.root.find(node => node.props.accessibilityRole === 'tab' && node.props.accessibilityLabel === '둘러보기').props.onPress());
    await tap('가까운 매장에서 주문');
    if (nearby) {
      await tap('근처 단말 찾기');
      await tap('근거리 단말 후보 1 인증');
    } else await tap('온라인으로 매장 확인');
    await tap('이 매장 메뉴 열기');
  };
  try {
    await open();
    await tap('차 hot 추가');
    if (mockLegacyPair) {
      const iced = tree.root.findAll(node => node.props.label === '차 iced 추가' && typeof node.props.run === 'function')[0];
      expect(iced.props.disabled).toBe(true);
      expect(JSON.stringify(tree.toJSON())).toContain('옵션을 바꾸려면 담은 옵션을 먼저 빼세요.');
      await tap('차 hot 빼기');
      await tap('차 iced 추가');
      expect(JSON.stringify(tree.toJSON())).toContain('합계 · 3500000 기준 단위');
      expect(mockCreateKeys).toEqual([]);
      return;
    }
    await tap('차 iced 추가');
    await tap('차 iced 추가');
    const hot = tree.root.findAll(node => node.props.label === '차 hot 추가' && typeof node.props.run === 'function')[0];
    expect(hot.props.disabled).toBe(true);
    expect(JSON.stringify(tree.toJSON())).toContain('합계 · 11000000 기준 단위');
    await tap('차 hot 빼기');
    expect(JSON.stringify(tree.toJSON())).toContain('합계 · 7000000 기준 단위');
    await tap('차 hot 추가');
    await tap('주문 접수 요청');
    const key = [...mockJournals.keys()].find(name => name.startsWith('customer-order-'))!;
    const saved = JSON.parse(mockJournals.get(key)!);
    expect(saved.cart.items).toEqual([
      {sku: 'tea', option_key: 'hot', quantity: 1},
      {sku: 'tea', option_key: 'iced', quantity: 2},
    ]);
    await Renderer.act(async () => tree.unmount());
    await open();
    await tap(nearby ? '원래 요청 재조회' : '저장한 주문 다시 확인');
    expect(mockCreateKeys).toEqual([saved.key, saved.key]);
    expect(JSON.parse(mockJournals.get(key)!).cart).toEqual(saved.cart);
  } finally {
    await Renderer.act(async () => tree?.unmount());
    mockUseNearby = false;
    mockPairStock = undefined;
    mockLegacyPair = false;
    mockCreateResult = 'ok';
    jest.restoreAllMocks();
  }
});

test('내 결제 discovers the current customer order without a link or a saved sale', async () => {
  mockJournals.clear();
  const reminderJournal = 'subscription-alerts-v2';
  const scope = mockScope();
  const savedReminders = JSON.stringify({
    branch: scope.branchRef, owner: scope.subjectRef, enabled: true, scheduled: [ref(22)],
  });
  mockJournals.set(reminderJournal, savedReminders);
  const configure = jest.spyOn(Http!, 'configure');
  const clear = jest.spyOn(Alerts!, 'clear');
  mockRequests.length = 0;
  mockConfirmed = true;
  mockShowInvoice = true;
  jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(null);
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  await Renderer.act(async () => {
    tree.root.findAll(node => node.props.accessibilityRole === 'tab' && node.props.accessibilityLabel === '결제' &&
      typeof node.props.onPress === 'function')[0].props.onPress();
  });
  expect(mockRequests).toEqual(['order.list', 'invoice.list']);
  expect(JSON.stringify(tree.toJSON())).not.toContain('주문 등록 확인');
  const open = tree.root.findAll(node => node.props.label === `매장 주문 ${mockSale}` &&
    typeof node.props.run === 'function')[0];
  expect(open).toBeDefined();
  await Renderer.act(async () => { open.props.run(); });
  expect(mockRequests).toContain('order.status');
  expect(JSON.stringify(tree.toJSON())).toContain('기준 단위');
  const invoice = tree.root.findAll(node => typeof node.props.label === 'string' &&
    node.props.label.startsWith('구독 회차 청구서 2026-11-20') &&
    typeof node.props.run === 'function')[0];
  expect(invoice).toBeDefined();
  await Renderer.act(async () => { invoice.props.run(); });
  expect(mockRequests.filter(operation => operation === 'order.status')).toHaveLength(2);
  expect(JSON.stringify(tree.toJSON())).toContain('구독 회차 청구서');
  expect(configure).not.toHaveBeenCalled();
  expect(clear).not.toHaveBeenCalled();
  expect(mockJournals.get(reminderJournal)).toBe(savedReminders);
  mockJournals.delete(reminderJournal);
  mockShowInvoice = false;
  await Renderer.act(async () => { tree.unmount(); });
  jest.restoreAllMocks();
});

test('a changed customer registration discards the delayed prior list and keeps their saved order journal', async () => {
  mockConfirmed = true;
  mockRequests.length = 0;
  mockJournals.clear();
  const legacy = JSON.stringify({
    version: 2, branch: mockBranch, subject, key: ref(48), asset: mockAsset, route: 'direct',
    cart: {origin: 'menu', originRef: mockBranch, menuVersion: 1,
      items: [{sku: 'tea', option_key: 'hot', quantity: 1}]},
  });
  mockJournals.set('customer-order-v2', legacy);
  let release!: () => void;
  mockOrderBarrier = new Promise<void>(done => { release = done; });
  let notify: ((state: AppStateStatus) => void) | undefined;
  jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(null);
  jest.spyOn(AppState, 'addEventListener').mockImplementation((_, handler) => {
    notify = handler;
    return {remove: () => {}};
  });
  let tree!: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async () => { tree = Renderer.create(<App />); });
    await Renderer.act(async () => {
      tree.root.findAll(node => node.props.accessibilityRole === 'tab' && node.props.accessibilityLabel === '결제' &&
        typeof node.props.onPress === 'function')[0].props.onPress();
    });
    expect(mockRequests).toContain('order.list');
    await Renderer.act(async () => { notify?.('background'); });
    mockActiveSubject = ref(39);
    mockOrderBarrier = undefined;
    await Renderer.act(async () => { notify?.('active'); });
    await Renderer.act(async () => { release(); });
    expect(JSON.stringify(tree.toJSON())).toContain('현재 등록의 매장 주문과 구독 청구서가 없습니다.');
    expect(JSON.stringify(tree.toJSON())).not.toContain(mockSale);
    expect(mockJournals.get('customer-order-v2')).toBe(legacy);
  } finally {
    release();
    mockActiveSubject = subject;
    mockOrderBarrier = undefined;
    await Renderer.act(async () => { tree?.unmount(); });
    jest.restoreAllMocks();
  }
});

test('expired order link does not request previews or disclose private history', async () => {
  mockRequests.length = 0;
  mockConfirmed = true;
  mockJournals.clear();
  const link = encodeOrderLocator({branchRef: mockBranch, orderRef: mockSale, expiresAt: mockUnix() - 1}, mockUnix() - 60);
  jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(link);
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  expect(JSON.stringify(tree.toJSON())).toContain('이 계정에서 열 수 없는 주문입니다.');
  expect(JSON.stringify(tree.toJSON())).not.toContain(mockSale);
  expect(mockRequests).toEqual([]);
  await Renderer.act(async () => { tree.unmount(); });
  jest.restoreAllMocks();
});

test('the App payment tab opens the registered travel-booking route and returns to payments', async () => {
  mockRequests.length = 0;
  mockJournals.clear();
  jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(null);
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  await Renderer.act(async () => {
    tree.root.findAll(node => node.props.accessibilityRole === 'tab' && node.props.accessibilityLabel === '결제' &&
      typeof node.props.onPress === 'function')[0].props.onPress();
  });
  await Renderer.act(async () => {
    tree.root.findAll(node => node.props.label === '여행 예약' &&
      typeof node.props.run === 'function')[0].props.run();
  });
  expect(mockRequests).toContain(`my-bookings/${mockBranch}`);
  expect(JSON.stringify(tree.toJSON())).toContain('내 여행 예약');
  await Renderer.act(async () => {
    tree.root.findAll(node => node.props.accessibilityLabel === '내 결제로' &&
      typeof node.props.onPress === 'function')[0].props.onPress();
  });
  expect(JSON.stringify(tree.toJSON())).toContain('내 결제와 미결 청구');
  await Renderer.act(async () => { tree.unmount(); });
  jest.restoreAllMocks();
});

test('W11 reopens the same paid receipt and shows kitchen ready, then collected', async () => {
  mockConfirmed = true;
  mockPickupStage = 'ready';
  mockRequests.length = 0;
  mockJournals.clear();
  jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(null);
  let tree!: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async () => { tree = Renderer.create(<App />); });
    await Renderer.act(async () => {
      tree.root.findAll(node => node.props.accessibilityRole === 'tab' && node.props.accessibilityLabel === '결제' &&
        typeof node.props.onPress === 'function')[0].props.onPress();
    });
    async function tap(label: string) {
      const node = tree.root.findAll(candidate => candidate.props.label === label &&
        typeof candidate.props.run === 'function')[0];
      expect(node).toBeDefined();
      await Renderer.act(async () => node.props.run());
    }
    await tap(`매장 주문 ${mockSale}`);
    expect(JSON.stringify(tree.toJSON())).toContain('픽업 준비 완료');
    expect(JSON.stringify(tree.toJSON())).toContain('매장 픽업대에서 같은 픽업 번호를 보여주세요.');
    expect(JSON.stringify(tree.toJSON())).toContain(ref(17));
    mockPickupStage = 'collected';
    await tap('같은 주문의 지급 결과 확인');
    expect(JSON.stringify(tree.toJSON())).toContain('픽업 수령 완료');
    expect(JSON.stringify(tree.toJSON())).toContain(ref(17));
    expect(mockRequests.filter(operation => operation === 'order.status')).toHaveLength(2);
    expect(mockRequests).not.toContain('payment.forward');
  } finally {
    mockPickupStage = undefined;
    await Renderer.act(async () => tree?.unmount());
    jest.restoreAllMocks();
  }
});


test('K01 mobile filters both catalog pages, retains its cart and browses offline without new orders', async () => {
  mockK01Categories = true;
  mockK01Pages.length = 0;
  mockCreateKeys.length = 0;
  mockPairStock = undefined;
  mockLegacyPair = false;
  mockTeaPrice = '3500000';
  mockTeaAvailable = 2;
  mockUseNearby = false;
  mockActiveSubject = mockCustomerA;
  mockJournals.clear();
  const url = jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(null);
  let tree!: Renderer.ReactTestRenderer;
  const screen = () => JSON.stringify(tree.toJSON());
  const action = (label: string) => tree.root.findAll(node =>
    (node.props.label === label || node.props.title === label || node.props.accessibilityLabel === label) &&
    typeof (node.props.run ?? node.props.onPress) === 'function')[0];
  const tap = async (label: string) => {
    const button = action(label);
    expect(button).toBeDefined();
    expect(button.props.disabled).not.toBe(true);
    await Renderer.act(async () => (button.props.run ?? button.props.onPress)());
  };
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    // Nearby shop orders open from the 둘러보기 tab.
    await Renderer.act(async () => tree.root.find(node => node.props.accessibilityRole === 'tab' && node.props.accessibilityLabel === '둘러보기').props.onPress());
    await tap('가까운 매장에서 주문');
    await tap('온라인으로 매장 확인');
    await tap('이 매장 메뉴 열기');
    expect(mockK01Pages).toEqual(['0','0','1']);
    expect(action('한정 메뉴 plain 추가')).toBeDefined();
    await tap('음료');
    await tap('차 따뜻하게 추가');
    expect(action('한정 메뉴 plain 추가')).toBeUndefined();
    await tap('메뉴 언어 English');
    expect(action('Drinks').props.accessibilityState.selected).toBe(true);
    await tap('Bakery');
    expect(action('Tea Hot Add')).toBeUndefined();
    expect(screen()).toContain('3500000');
    await tap('All items');
    expect(action('한정 메뉴 plain Add')).toBeDefined();

    mockK01Disconnected = true;
    await tap('주문 접수 요청');
    expect(screen()).toContain('Viewing the last synced menu');
    expect(screen()).toContain('New quotes paused');
    expect(mockCreateKeys).toEqual([]);
    expect(action('주문 접수 요청')).toBeUndefined();
    await tap('Drinks');
    expect(screen()).toContain('Tea');
    expect(screen()).not.toContain('한정 메뉴');
    await tap('Menu language 한국어');
    expect(action('음료').props.accessibilityState.selected).toBe(true);
    mockK01Disconnected = false;
    await tap('매장 상태 다시 확인');
    expect(screen()).not.toContain('신규 견적 발급 일시 중지');
    expect(screen()).toContain('합계 · 3500000 기준 단위');
    mockCreateResult = 'unknown';
    await tap('주문 접수 요청');
    const key = [...mockJournals.keys()].find(name => name.startsWith('customer-order-'))!;
    expect(JSON.parse(mockJournals.get(key)!).cart.items).toEqual([{sku:'tea',option_key:'hot',quantity:1}]);
    expect(mockCreateKeys).toHaveLength(1);
  } finally {
    mockK01Categories = mockK01Disconnected = false;
    mockCreateResult = 'ok';
    mockJournals.clear();
    url.mockRestore();
    await Renderer.act(async () => tree.unmount());
  }
}, 20000);

// An unregistered phone that briefly leaves the app (for example to copy a registration from
// another app) must come back to the registration form, whatever order the profile reads finish in.
test.each(['settled', 'initial-read-pending', 'reads-reversed'] as const)(
  'registration form survives a short background switch (%s)',
  async order => {
    const reads: ((value: string) => void)[] = [];
    jest.spyOn(Http!, 'loadProfile').mockImplementation(() => new Promise<string>(resolve => { reads.push(resolve); }));
    let notify: ((state: AppStateStatus) => void) | undefined;
    jest.spyOn(AppState, 'addEventListener').mockImplementation((_, handler) => {
      notify = handler;
      return {remove: () => {}};
    });
    let tree!: Renderer.ReactTestRenderer;
    try {
      await Renderer.act(async () => {
        tree = Renderer.create(<CustomerOrders initialLocator="" back={() => {}} permission={async () => true} />);
      });
      if (order === 'settled') await Renderer.act(async () => { reads[0](''); });
      await Renderer.act(async () => { notify?.('background'); });
      await Renderer.act(async () => { notify?.('active'); });
      if (order === 'initial-read-pending') {
        await Renderer.act(async () => { reads[0](''); });
        await Renderer.act(async () => { reads.slice(1).forEach(resolve => resolve('')); });
      } else if (order === 'reads-reversed') {
        await Renderer.act(async () => { reads.slice(1).forEach(resolve => resolve('')); });
        await Renderer.act(async () => { reads[0](''); });
      } else {
        await Renderer.act(async () => { reads.slice(1).forEach(resolve => resolve('')); });
      }
      for (let i = 0; i < 3; i++) await Renderer.act(async () => { reads.slice(1).forEach(resolve => resolve('')); });
      const screen = JSON.stringify(tree.toJSON());
      expect({order, reads: reads.length, form: screen.includes('고객 주문 등록서')}).toMatchObject({form: true});
    } finally {
      await Renderer.act(async () => { tree?.unmount(); });
      jest.restoreAllMocks();
    }
  },
);
