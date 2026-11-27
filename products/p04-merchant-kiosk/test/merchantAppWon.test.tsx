import React from 'react';
import Renderer from 'react-test-renderer';
import App from '../src/App';
import { OfflineMenu } from '../src/kiosk/offlineMenu';
import { posAmount } from '../src/kiosk/pos';
import {
  bindServiceRequestV2 as mockBind,
  namedTokenAbiSha256 as mockNamed,
  toBase64 as mockBase64,
  type ServiceRequestV2,
  type CommerceCart,
} from '@nu54/protocol';

const ref = (n: number) =>
  `f0551c76-7592-4850-a414-54464d24d${n.toString(16).padStart(3, '0')}`;
const mockShop = ref(1),
  mockTerminal = ref(2),
  mockActor = ref(3),
  mockSale = ref(4),
  mockTenant = ref(5);
const mockHandoff = ref(6),
  mockReceipt = ref(7);
let mockPOSPaid = false;
let mockPOSIssued = false;
let mockPOSConflict = false;
let mockPOSQuoteUnsupported = false;
const mockToken = {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2};
let mockPOSLegacyHeld = false;
let mockPOSQuoteGate: Promise<void> | undefined;
const mockSecondHandoff = ref(81), mockSecondSale = ref(82);
let mockPOSUnsupported = false;
const mockClock = () => Math.floor(Date.now() / 1000);
let mockRole: 'terminal' | 'operator' | 'finance' = 'terminal';
let mockStaffPermissions: string[] = ['terminal_settings'];
let mockStaffBranch = mockShop;
let mockStaffDenied = false;
let mockDevice: import('@nu54/protocol').TerminalDevice = { revision: 0, language: 'ko', idleSeconds: 60 };
let mockDeviceOffline = false;
const mockDeviceChanges: [string, string, string, string][] = [];
let mockDedicatedPickup = false;
let mockLosePickupReply = false;
let mockPickupState = 'accepted';
const mockPickupCommands: string[] = [];
let mockStaffGate: Promise<void> | undefined;
let mockOrders = [mockSale];
let mockMode: 'menu' | 'pos' = 'menu';
let mockTerminalExpires=0;
let mockTerminalKey='same-native-key-terminal';
let mockMenuFailure:'offline'|'trust'|undefined;
let mockMenuFailureAt:'catalog.read'|'terminal.state'|undefined;
let mockColdBranch=mockShop;
let mockModeRevision = 1;
let mockTransition: import('@nu54/protocol').TerminalTransition | undefined;
let mockLoseTransitionReply = false;
let mockLoseAckReply = false;
const mockTransitionKeys: string[] = [];
const mockK07Fixture = require('./fixtures/k07-publication.json');
let mockK07Published = false;
let mockCategorizedPages = false;
const mockMenuPages: string[] = [];
const mockCategories = [
  { key: 'warm', sortOrder: 10, labels: {ko: '음료', en: 'Drinks'} },
  { key: 'oven', sortOrder: 20, labels: {ko: '베이커리', en: 'Bakery'} },
];
let mockK07Legacy = false;
let mockK07SKUStock: number | undefined;
const mockSubmittedCarts: CommerceCart[] = [];
let mockLoseCancelReply = false;
const mockPublicCancellations: Array<{session:string; key?:string}> = [];
let mockMenuVersion = 1;
let mockTeaAvailable = 2;
let mockMenuPrice = '9';
let mockEntropy = 12;
let mockMenuCheckout = false;
let mockComparisonGate: Promise<void> | undefined;
let mockLoseComparisonReply: 'before' | 'after' | undefined;
let mockBoundCustomer: string | undefined;
let mockMenuOrderState: 'preview' | 'issued' | 'checking' | 'paid' | 'closed' = 'preview';
let mockMenuOrderAmount = '6500000';
let mockTerminalOutcome: 'relay-paid' | 'expired-unpaid' | undefined;
const mockSelected: string[] = [];
const mockCandidates = [
  { customerRef: ref(10), code: '37ea19cd' },
  { customerRef: ref(11), code: '2ba8cf04' },
];
const mockAsset = '0x' + 'a'.repeat(40);
const mockJournal = new Map<string, string>();
const mockDialed: string[] = [];
const mockProfile = () => ({
  branchRef: mockColdBranch,
  subjectRef: mockRole === 'terminal' ? mockTerminal : mockActor,
  tenantRef: mockTenant,
  role: mockRole,
  expiresAt: mockTerminalExpires,
});

jest.mock(
  'react-native-safe-area-context',
  () => require('react-native-safe-area-context/jest/mock').default,
);
const mockRelays: unknown[][] = [];
jest.mock('../src/specs/NativeNearby.ts', () => ({__esModule: true, default: {
  onNearby: () => ({remove() {}}), configure: async () => {}, start: async () => {}, stop: async () => {}, close: async () => {}}}));
jest.mock('../src/kiosk/nearbyRelay.ts', () => ({NearbyRelay: class {
  constructor(...args: unknown[]) { mockRelays.push(args); }
  close() {} activate() {} retire() {} clearPublic() {}
}}));
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {
    loadProfile: async (slot: string) =>
      slot === mockRole
        ? JSON.stringify({
            scope: mockProfile(),
            pin: 'b'.repeat(64),
            pickupKitchen: mockDedicatedPickup,
            nearby: {},
            ...(mockMenuCheckout
              ? {
                  payment: {
                    chain: 8283,
                    contract: '0x' + '1'.repeat(40),
                    asset: mockAsset,
                    beneficiary: '0x' + '2'.repeat(40),
                    signer: '0x' + '3'.repeat(40),
                    merchantId: 'a'.repeat(64),
                    keyVersion: 1,
                    beneficiaryVersion: 1,
                    maxBaseUnits: '30000000',
                    abiSha256: mockNamed,
                    token: mockToken,
                  },
                }
              : {}),
          })
        : '',
    publicIdentity: async (slot: string) => slot==='terminal' ? mockTerminalKey : 'same-native-key-' + slot,
    exchangeScoped(slot: string, _digest: string, raw: string) {
      return this.exchange(slot, raw);
    },
    loadJournal: async (name: string) => mockJournal.get(name) ?? '',
    saveJournal: async (name: string, value: string) =>
      mockJournal.set(name, value),
    randomBytes: async () => mockBase64(new Uint8Array(16).fill(++mockEntropy)),
    exchange: async (_slot: string, raw: string) => {
      const wire = JSON.parse(raw) as {
        command: ServiceRequestV2;
        cart?: CommerceCart;
        pos?: { operation: string; handoff: string };
        travel?: { branch: string; operation: string; target?: string };
      };
      if (wire.pos) {
        mockDialed.push('pos.' + wire.pos.operation);
        if (mockPOSUnsupported)
          return JSON.stringify({ error: 'POS_ASSET_UNSUPPORTED' });
        const isOther = wire.pos.handoff === mockSecondHandoff;
        if (wire.pos.handoff !== mockHandoff && !isOther) throw new Error('UNKNOWN_POS');
        const bound = !isOther && (mockPOSPaid || mockPOSIssued || mockPOSLegacyHeld || mockBoundCustomer !== undefined);
        const issued = !isOther && mockPOSIssued;
        return JSON.stringify({
          branch: mockShop,
          branchLabel: '여울 커피 성수점',
          handoff: wire.pos.handoff,
          reference: isOther ? 'S-0143' : 'S-0142',
          chainId: 8283,
          payerBound: bound,
          ...(!isOther && mockPOSLegacyHeld ? {selectionHeld: true} : {}),
          ...(bound && !mockPOSPaid && mockBoundCustomer
            ? {selectedCode: mockCandidates.find(row => row.customerRef === mockBoundCustomer)!.code}
            : {}),
          order: {
            id: isOther ? mockSecondSale : mockSale,
            revision: mockPOSPaid ? 2 : issued ? 1 : 0,
            state: mockPOSPaid ? 'paid' : issued ? 'issued' : 'preview',
            amount: isOther ? '4200000' : '6500000',
            asset: mockAsset,
            expiresAt: new Date((mockClock() + 60) * 1000).toISOString(),
            ...(issued ? {quoteDigest: 'd'.repeat(64), orderKey: 'e'.repeat(64)} : {}),
            ...(mockPOSPaid ? { pickupNo: 17, receiptRef: mockReceipt } : {}),
          },
        });
      }
      if (wire.travel) {
        if (
          (_slot !== 'operator' &&
            !(_slot === 'terminal' && mockDedicatedPickup)) ||
          wire.travel.branch !== mockShop
        )
          throw new Error('staff scope');
        mockDialed.push(wire.travel.operation);
        if (wire.travel.operation === 'pickup-change') {
          mockPickupCommands.push(raw);
          mockPickupState = wire.travel.target!;
          if (mockLosePickupReply) throw new Error('lost pickup response');
          return JSON.stringify({ revision: 1 });
        }
        return JSON.stringify({
          pickups: [
            {
              saleRef: mockSale,
              startsAt: '2026-11-21T12:00:00Z',
              timezone: 'Asia/Seoul',
              pickupNo: 37,
              state: mockPickupState,
              revision: mockPickupState === 'accepted' ? 0 : 1,
            },
          ],
        });
      }
      const { command } = wire;
      // The settings are read beside every step; they are no part of an order.
      if (!command.operation.startsWith('terminal.device')) mockDialed.push(command.operation);
      if (mockMenuFailure && (!mockMenuFailureAt || mockMenuFailureAt===command.operation) &&
          ['terminal.state','catalog.read'].includes(command.operation)) {
        throw Object.assign(new Error('transport failed'), {code: mockMenuFailure==='offline' ?
          'COMMERCE_NETWORK_OFFLINE' : 'COMMERCE_UNAVAILABLE'});
      }
      const { requestId, requestDigest } = mockBind(
        command,
        {
          ...mockProfile(),
          grants: [
            {
              domainRef: command.domainRef,
              objectRef: command.objectRef,
              operations: [`C3/${command.operation}`],
            },
          ],
        },
        mockClock(),
      );
      const response = {
        requestId,
        requestDigest,
        status: 'ok',
        resultRef: command.objectRef,
      };
      if (command.operation.startsWith('terminal.device')) {
        if (mockDeviceOffline) throw new Error('COMMERCE_UNAVAILABLE');
        if (command.operation === 'terminal.device-change') {
          const next = (wire as { terminalValue?: string }).terminalValue!;
          mockDeviceChanges.push([command.objectRef, command.body.name, next, command.body.expectedRevision]);
          mockDevice = {
            ...mockDevice,
            revision: mockDevice.revision + 1,
            ...(command.body.name === 'language' ? { language: next as 'ko' | 'en' }
              : command.body.name === 'idle-seconds' ? { idleSeconds: Number(next) } : { readServer: next || undefined }),
          };
        }
        return JSON.stringify({
          ...response,
          terminalDevice: {
            ...mockDevice,
            ...(mockRole === 'operator' ? {
              changes: mockDeviceChanges.map(([, name, to]) => ({ name, from: name === 'language' ? 'ko' : name === 'idle-seconds' ? '60' : '', to,
                actor: mockActor, at: mockClock() })).reverse(),
              payment: { chainId: 8283, asset: mockAsset, beneficiary: '0x' + '2'.repeat(40), contract: '0x' + '1'.repeat(40), escrow: true,
                gasSponsor: '0x' + '4'.repeat(40), gasAccount: '0x' + '5'.repeat(40), registrationExpiresAt: mockClock() + 86400 },
            } : {}),
          },
        });
      }
      if (command.operation === 'staff.access') {
        await mockStaffGate;
        if (mockStaffDenied) throw new Error('NOT_AUTHORIZED');
        return JSON.stringify({
          ...response,
          staff: {
            branchRef: mockStaffBranch,
            actorRef: mockProfile().subjectRef,
            permissions: mockStaffPermissions,
            ...(mockRole === 'finance'
              ? { console: 'https://ops.example.invalid' }
              : {}),
          },
        });
      }
      if (command.operation === 'terminal.orders')
        return JSON.stringify({
          ...response,
          terminalOrders: mockOrders
            .filter(orderRef => orderRef > command.body.afterSale)
            .slice(0, 20),
        });
      if (command.operation === 'terminal.mode') {
        mockTransitionKeys.push(command.idempotencyKey!);
        mockTransition ??= {
          requestRef: command.idempotencyKey!,
          sourceRevision: mockModeRevision,
          targetRevision: mockModeRevision + 1,
          targetMode: 'pos',
          handoff: false,
          ack: false,
          state: 'waiting',
        };
        if (mockLoseTransitionReply) throw new Error('unknown result');
      }
      if (
        command.operation === 'terminal.handoff' ||
        command.operation === 'terminal.ack'
      ) {
        if (
          !mockTransition ||
          command.body.transitionRef !== mockTransition.requestRef ||
          command.body.targetRevision !== String(mockTransition.targetRevision)
        )
          throw new Error('wrong transition');
        mockTransition = {
          ...mockTransition,
          ...(command.operation === 'terminal.handoff'
            ? { handoff: true }
            : { ack: true }),
        };
        if (mockTransition.handoff && mockTransition.ack) {
          mockTransition.state = 'active';
          mockMode = 'pos';
          mockModeRevision = mockTransition.targetRevision;
        }
        if (command.operation === 'terminal.ack' && mockLoseAckReply)
          throw new Error('unknown ACK result');
      }
      if (
        [
          'terminal.state',
          'terminal.mode',
          'terminal.handoff',
          'terminal.ack',
        ].includes(command.operation)
      )
        return JSON.stringify({
          ...response,
          terminal: {
            mode: mockMode,
            revision: mockModeRevision,
            transition: mockTransition,
            ...(mockRole === 'operator'
              ? {
                  orderCount: mockOrders.length,
                  orderRefs: mockOrders.slice(0, 20),
                }
              : {}),
          },
        });
      if (command.operation === 'catalog.read' && mockCategorizedPages) {
        mockMenuPages.push(command.body.page);
        const common = {
          version: mockMenuVersion, expiresAt: new Date((mockClock() + 120) * 1000).toISOString(),
          optionKey: 'standard', asset: mockAsset, unitPrice: '3000000', available: 5,
        };
        const catalog = command.body.page === '0' ? [
          {...common, sku: 'a-drink', label: '차', category: mockCategories[0],
            metadata: {labels: {ko:'차', en:'Tea'}}},
          ...Array.from({length: 49}, (_, i) => ({...common, sku: 'item-' + i, label: '오늘의 메뉴'})),
        ] : [
          {...common, sku: 'z-croissant', label: '크루아상', category: mockCategories[1],
            metadata: {labels: {ko:'크루아상', en:'Croissant'}}},
          {...common, sku: 'z-special', label: '한정 메뉴'},
        ];
        return JSON.stringify({...response, catalog, catalogVersion: mockMenuVersion,
          shop: {branch: mockColdBranch, branchLabel: '여울 커피 성수점'}});
      }
      if (command.operation === 'catalog.read')
        return JSON.stringify({
          ...response,
          shop: {branch: mockColdBranch, branchLabel: '여울 커피 성수점'},
          catalog: mockK07Published
            ? mockK07Fixture.rows.map((row: typeof mockK07Fixture.rows[number]) => ({
                version: mockMenuVersion, expiresAt: new Date((mockClock() + 120) * 1000).toISOString(),
                sku: row.sku, optionKey: row.option, label: row.label, asset: mockAsset,
                unitPrice: String(Number(row.displayPrice) * 1_000_000), available: mockK07SKUStock ?? mockK07Fixture.stock,
                skuAvailable: mockK07SKUStock,
                metadata: {...row.metadata, ...(row.required && !mockK07Legacy
                  ? {optionRequired: true}
                  : {optionGroupLabels: undefined})},
              }))
            : [
            {
              version: mockMenuVersion,
              expiresAt: new Date((mockClock() + 120) * 1000).toISOString(),
              sku: 'tea',
              optionKey: 'hot',
              label: '차',
              asset: mockAsset,
              unitPrice: mockMenuPrice,
              available: mockTeaAvailable,
            },
            {
              version: mockMenuVersion,
              expiresAt: new Date((mockClock() + 120) * 1000).toISOString(),
              sku: 'bread',
              optionKey: 'plain',
              label: '빵',
              asset: mockAsset,
              unitPrice: '13',
              available: 2,
            },
          ],
        });
      if (command.operation === 'order.create') {
        if (wire.cart) mockSubmittedCarts.push(wire.cart);
        return JSON.stringify({ ...response, resultRef: mockSale });
      }
      if (command.operation === 'order.cancel-public') {
        mockPublicCancellations.push({session: command.objectRef, key: command.idempotencyKey});
        if (mockLoseCancelReply) {
          mockLoseCancelReply = false;
          throw new Error('cancel response lost after submission');
        }
        return JSON.stringify({...response, resultRef: command.objectRef});
      }
      if (command.operation === 'checkout.pending')
        return JSON.stringify({
          ...response,
          comparisons: mockCandidates.map(candidate => ({
            ...candidate,
            expiresAt: mockClock() + 35,
          })),
        });
      if (command.operation === 'checkout.confirm') {
        expect(command.objectRef).toBe(mockSale);
        expect(command.body).toEqual(mockCandidates[1]);
        mockSelected.push(command.idempotencyKey!);
        await mockComparisonGate;
        if (mockMode === 'pos' && mockPOSConflict) {
          mockBoundCustomer = mockCandidates[0].customerRef;
          throw new Error('CHECKOUT_CONFLICT');
        }
        if (mockLoseComparisonReply === 'before')
          throw new Error('confirmation did not reach P05');
        mockBoundCustomer = command.body.customerRef;
        if (mockLoseComparisonReply === 'after')
          throw new Error('lost confirmation response');
        return JSON.stringify({
          ...response,
          claim: { claimDigest: 'd'.repeat(64), expiresAt: mockClock() + 60 },
        });
      }
      if (command.operation === 'quote.issue' && mockMode === 'pos') {
        await mockPOSQuoteGate;
        if (mockPOSQuoteUnsupported)
          return JSON.stringify({requestId, requestDigest, status: 'error', error: 'UNSUPPORTED_CAPABILITY'});
        mockPOSIssued = true;
        throw new Error('lost committed quote response');
      }
      if (command.operation === 'order.status')
        return JSON.stringify({
          ...response,
          order: {
            id: mockSale,
            revision: mockMenuOrderState === 'preview' ? 0 : 1,
            state: mockMenuOrderState,
            asset: mockAsset,
            amount: mockMenuCheckout ? mockMenuOrderAmount : '9',
            ...(mockMenuCheckout
              ? {
                  menuCheckout: {
                    branchLabel: '여울 커피 성수점',
                    payerBound: mockBoundCustomer !== undefined,
                    customerRef: mockBoundCustomer,
                    finalOutcome: mockTerminalOutcome,
                  },
                }
              : {}),
            ...(mockMenuOrderState === 'paid' ? { pickupNo: 18, receiptRef: mockReceipt } : {}),
            ...(mockMenuOrderState === 'issued'
              ? { quoteDigest: 'd'.repeat(64), orderKey: 'e'.repeat(64) }
              : {}),
            expiresAt: new Date((mockClock() + 90) * 1000).toISOString(),
          },
        });
      throw new Error('unexpected request');
    },
  },
}));

function view(tree: Renderer.ReactTestRenderer) {
  return JSON.stringify(tree.toJSON());
}
async function press(tree: Renderer.ReactTestRenderer, label: string) {
  const button = tree.root.findAll(
    n =>
      (n.props.label === label && typeof n.props.run === 'function') ||
      ((n.props.title === label || n.props.accessibilityLabel === label) && typeof n.props.onPress === 'function'),
  )[0];
  expect(button).toBeDefined();
  await Renderer.act(async () => {
    (button.props.run ?? button.props.onPress)();
  });
}

beforeEach(() => {
  mockJournal.clear();
  mockDialed.length = 0;
  mockMode = 'menu';
  mockTerminalExpires=mockClock()+120;
  mockTerminalKey='same-native-key-terminal';
  mockMenuFailure=undefined;
  mockMenuFailureAt=undefined;
  mockColdBranch=mockShop;
  mockPOSPaid = false;
  mockPOSIssued = false;
  mockPOSConflict = false;
  mockPOSQuoteUnsupported = false;
  mockPOSLegacyHeld = false;
  mockPOSQuoteGate = undefined;
  mockMenuCheckout = false;
  mockLoseComparisonReply = undefined;
  mockComparisonGate = undefined;
  mockBoundCustomer = undefined;
  mockMenuOrderState = 'preview';
  mockMenuOrderAmount = '6500000';
  mockTerminalOutcome = undefined;
  mockSelected.length = 0;
  mockPOSUnsupported = false;
  mockModeRevision = 1;
  mockLoseAckReply = false;
  mockTransition = undefined;
  mockLoseTransitionReply = false;
  mockTransitionKeys.length = 0;
  mockMenuVersion = 1;
  mockK07Published = false;
  mockCategorizedPages = false;
  mockMenuPages.length = 0;
  mockK07Legacy = false;
  mockK07SKUStock = undefined;
  mockSubmittedCarts.length = 0;
  mockPublicCancellations.length = 0;
  mockLoseCancelReply = false;
  mockMenuPrice = '9';
  mockTeaAvailable = 2;
  mockStaffPermissions = ['terminal_settings'];
  mockDevice = { revision: 0, language: 'ko', idleSeconds: 60 };
  mockDeviceOffline = false;
  mockDeviceChanges.length = 0;
  mockStaffBranch = mockShop;
  mockStaffDenied = false;
  mockDedicatedPickup = false;
  mockLosePickupReply = false;
  mockPickupState = 'accepted';
  mockPickupCommands.length = 0;
  mockStaffGate = undefined;
  mockOrders = [mockSale];
});


// The whole kiosk with a won registration.
const opened = async () => {
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  return tree;
};
const openPOS = async (tree: Renderer.ReactTestRenderer) => {
  await press(tree, '새 고객 주문');
  expect(view(tree)).toContain('tKRW');
  const input = tree.root.findAllByProps({ accessibilityLabel: 'POS 인계 코드' }).find(n => typeof n.props.onChangeText === 'function')!;
  await Renderer.act(async () => input.props.onChangeText(mockHandoff));
  await press(tree, 'POS 원주문 조회');
};

test('the menu, the basket and the total are in won, also in English', async () => {
  mockRole = 'terminal';
  mockK07Published = true;
  mockMenuCheckout = true;
  const tree = await opened();
  try {
    await press(tree, '새 고객 주문');
    expect(view(tree)).toContain('30,000원');
    await press(tree, '크루아상 상세');
    await press(tree, '수량 늘리기');
    expect(view(tree)).toContain('소계 · 60,000원');
    await press(tree, '장바구니에 담기');
    expect(view(tree)).toContain('합계 · 60,000원');
    await press(tree, '메뉴 언어 English');
    expect(view(tree)).toContain('₩60,000');
    expect(view(tree)).not.toContain('원"');
    await press(tree, 'Menu language 한국어');
    expect(view(tree)).not.toContain('TestUSDC');
  } finally {
    await Renderer.act(async () => tree.unmount());
  }
});

test('a POS order is in won before and after it is paid', async () => {
  mockRole = 'terminal';
  mockMode = 'pos';
  mockMenuCheckout = true;
  for (const paid of [false, true]) {
    mockPOSPaid = paid;
    const tree = await opened();
    await openPOS(tree);
    expect(view(tree)).toContain('65,000원');
    expect(view(tree)).not.toContain('TestUSDC');
    await Renderer.act(async () => tree.unmount());
  }
});

test('the relay checks locks against the won token', async () => {
  mockRole = 'terminal';
  mockMenuCheckout = true;
  mockRelays.length = 0;
  const tree = await opened();
  expect(mockRelays.length).toBeGreaterThan(0);
  expect(mockRelays.at(-1)?.[8]).toBe('NU54 Test KRW');
  await Renderer.act(async () => tree.unmount());
});

test('posAmount follows the token and never rounds', () => {
  expect([posAmount('6509999'), posAmount('6599999'), posAmount('999999')]).toEqual(['6.50 TestUSDC', '6.59 TestUSDC', '0.99 TestUSDC']);
  expect(posAmount('6500000', mockToken)).toBe('65,000원');
});

test('an offline menu follows the language', async () => {
  const cached = { shop: { branchLabel: '여울 커피' }, rows: [{ sku: 'latte', optionKey: 'hot', label: '라떼', asset: mockAsset, unitPrice: '650000', version: 1, available: 1, expiresAt: new Date().toISOString() }] };
  const policy = { asset: mockAsset, token: mockToken } as never;
  for (const [locale, shown] of [['ko', '6,500원'], ['en', '₩6,500']] as const) {
    let tree!: Renderer.ReactTestRenderer;
    await Renderer.act(async () => { tree = Renderer.create(<OfflineMenu cached={cached as never} busy={false} retry={() => {}} policy={policy} />); });
    if (locale === 'en') {
      const change = tree.root.findAll(n => typeof n.props.onPress === 'function' && /English/.test(String(n.props.accessibilityLabel)))[0];
      await Renderer.act(async () => change.props.onPress());
    }
    expect(view(tree)).toContain(shown);
    await Renderer.act(async () => tree.unmount());
  }
});

test('replacing an option compares both prices in won', async () => {
  mockRole = 'terminal';
  mockK07Published = true;
  mockMenuCheckout = true;
  const tree = await opened();
  try {
    await press(tree, '새 고객 주문');
    await press(tree, '메뉴 언어 English');
    for (const option of ['Select Iced', 'Select Hot']) {
      const open = tree.root.findAll(n => /Americano/.test(String(n.props.accessibilityLabel)) && typeof n.props.onPress === 'function')[0];
      await Renderer.act(async () => open.props.onPress());
      await press(tree, option);
      const add = tree.root.findAll(n => /basket/i.test(String(n.props.accessibilityLabel)) && /Add/.test(String(n.props.accessibilityLabel)) && typeof n.props.onPress === 'function')[0];
      await Renderer.act(async () => add.props.onPress());
    }
    expect(view(tree)).toContain('Replace the option for this item?');
    expect(view(tree)).toMatch(/×1 · ₩[0-9,]+/);
    expect(view(tree)).not.toMatch(/×1 · [0-9,]+원/);
  } finally {
    await Renderer.act(async () => tree.unmount());
  }
});
