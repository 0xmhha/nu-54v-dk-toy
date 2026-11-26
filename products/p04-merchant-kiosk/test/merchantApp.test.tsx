import React from 'react';
import { AccessibilityInfo, Dimensions, Linking } from 'react-native';
import Renderer from 'react-test-renderer';
import App from '../src/App';
import {
  bindServiceRequestV2 as mockBind,
  orderEscrowAbiSha256,
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
let mockAbi: string | undefined;
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
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({
  __esModule: true,
  default: {
    loadProfile: async (slot: string) =>
      slot === mockRole
        ? JSON.stringify({
            scope: mockProfile(),
            pin: 'b'.repeat(64),
            pickupKitchen: mockDedicatedPickup,
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
                    abiSha256: mockAbi,
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

function fakeSessionTimers() {
  // Advance the kiosk's timeouts while keeping React's immediate work queue
  // native across fake-clock teardown and subsequent renderer mounts.
  jest.useFakeTimers({ doNotFake: ['setImmediate', 'clearImmediate'] });
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
  mockAbi = undefined;
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

test('a terminal orders from server inventory, displays its locator, and clears it before the next visitor', async () => {
  mockRole = 'terminal';
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<App />);
  });
  expect(view(tree!)).not.toContain('직원 단말 설정');
  await press(tree!, '새 고객 주문');
  await press(tree!, '차 hot 추가');
  await press(tree!, '하드월렛으로 결제');
  expect(view(tree!)).toContain('이 주문을 여는 일회용 QR');
  expect(mockDialed).toEqual([
    'terminal.state',
    'catalog.read',
    'terminal.state',
    'terminal.state',
    'catalog.read',
    'terminal.state',
    'order.create',
    'order.status',
  ]);
  await press(tree!, '화면을 비우고 대기');
  await press(tree!, '닫고 화면 지우기');
  expect(view(tree!)).not.toContain('이 주문을 여는 일회용 QR');
  expect(view(tree!)).not.toContain(mockSale);
  await Renderer.act(async () => tree!.unmount());
});

test('K02 shows both totals and requires explicit price approval', async () => {
  mockRole = 'terminal';
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<App />);
  });
  await press(tree!, '새 고객 주문');
  await press(tree!, '차 hot 추가');
  mockMenuVersion = 2;
  mockMenuPrice = '10';
  await press(tree!, '하드월렛으로 결제');
  expect(view(tree!)).toContain('기존 합계 · 9 기준 단위');
  expect(view(tree!)).toContain('새 합계 · 10 기준 단위');
  expect(view(tree!)).toContain('선택 1');
  expect(mockDialed).not.toContain('order.create');
  await press(tree!, '바뀐 장바구니 확인 후 주문 접수');
  expect(mockDialed.filter(op => op === 'order.create')).toHaveLength(1);
  await Renderer.act(async () => tree!.unmount());
});

test('K06 requires impact review and resumes the same approved transition after an unknown reply', async () => {
  mockRole = 'operator';
  let tree: Renderer.ReactTestRenderer;
  async function open() {
    await Renderer.act(async () => {
      tree = Renderer.create(<App />);
    });
  }
  async function close() {
    await Renderer.act(async () => tree.unmount());
  }
  await open();
  expect(view(tree!)).not.toContain('직원 단말 설정');
  await press(tree!, '직원 권한 확인');
  expect(view(tree!)).toContain('직원 단말 설정');
  expect(view(tree!)).not.toContain('새 고객 주문');
  const target = tree!.root
    .findAllByProps({ accessibilityLabel: '단말 식별값' })
    .find(n => typeof n.props.onChangeText === 'function')!;
  await Renderer.act(async () => target.props.onChangeText(mockTerminal));
  await press(tree!, '현재 단말 모드 조회');
  await press(tree!, '결제 전용 모드로 전환');
  expect(mockDialed).toEqual(['staff.access', 'terminal.state']);
  expect(mockMode).toBe('menu');
  expect(view(tree!)).toContain('전환 영향 검토');
  expect(view(tree!)).toContain(mockSale);
  mockLoseTransitionReply = true;
  await press(tree!, '전환 요청 저장');
  expect(mockTransitionKeys).toHaveLength(1);
  expect(mockMode).toBe('menu');
  await close();

  mockLoseTransitionReply = false;
  await open();
  await press(tree!, '직원 권한 확인');
  await press(tree!, '기존 요청 재조회');
  expect(mockTransitionKeys).toEqual([
    mockTransitionKeys[0],
    mockTransitionKeys[0],
  ]);
  expect(view(tree!)).toContain('신규 주문 · 중지');
  await press(tree!, '지점 인계 확인');
  expect(mockTransition).toMatchObject({
    handoff: true,
    ack: false,
    state: 'waiting',
  });
  expect(mockMode).toBe('menu');
  await close();

  mockRole = 'terminal';
  mockLoseAckReply = true;
  await open();
  await press(tree!, '새 고객 주문');
  expect(mockTransition).toMatchObject({
    handoff: true,
    ack: true,
    state: 'active',
  });
  expect(mockDialed).not.toContain('order.create');
  expect(view(tree!)).not.toContain('POS 인계 코드');
  await close();
  mockLoseAckReply = false;
  await open();
  await press(tree!, '새 고객 주문');
  expect(view(tree!)).toContain('POS 결제 전용');
  await close();

  mockRole = 'operator';
  await open();
  await press(tree!, '직원 권한 확인');
  await press(tree!, '기존 요청 재조회');
  expect(view(tree!)).toContain('결제 전용 v2 · 적용 완료');
  expect(mockJournal.get('terminal-mode-request-v2')).toBe('');
  await close();
});

test('a visitor can keep exploring for five minutes, then the idle screen clears for the next person', async () => {
  fakeSessionTimers();
  mockRole = 'terminal';
  let tree: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async () => {
      tree = Renderer.create(<App />);
    });
    await press(tree!, '새 고객 주문');
    await press(tree!, '차 hot 추가');
    for (let minute = 0; minute < 6; minute++) {
      await Renderer.act(async () => {
        jest.advanceTimersByTime(50_000);
      });
      await press(tree!, '계속 주문하고 시간 연장');
    }
    expect(view(tree!)).toContain('선택 1');
    expect(view(tree!)).not.toContain(mockSale);
    await Renderer.act(async () => {
      jest.advanceTimersByTime(60_001);
    });
    expect(view(tree!)).toContain('새 고객 주문');
    expect(view(tree!)).not.toContain('선택 1');
    await Renderer.act(async () => tree!.unmount());
  } finally {
    jest.useRealTimers();
  }
});

test('extending the public session does not extend an already displayed locator', async () => {
  fakeSessionTimers();
  mockRole = 'terminal';
  let tree: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async () => {
      tree = Renderer.create(<App />);
    });
    await press(tree!, '새 고객 주문');
    await press(tree!, '차 hot 추가');
    await press(tree!, '하드월렛으로 결제');
    expect(view(tree!)).toContain('이 주문을 여는 일회용 QR');
    await Renderer.act(async () => {
      jest.advanceTimersByTime(30_000);
    });
    await press(tree!, '계속 주문하고 시간 연장');
    await Renderer.act(async () => {
      jest.advanceTimersByTime(11_000);
    });
    expect(view(tree!)).toContain('연결 시간이 지났습니다');
    expect(view(tree!)).not.toContain('이 주문을 여는 일회용 QR');
    expect(view(tree!)).not.toContain('이 주문 다시 확인');
    expect(view(tree!)).not.toContain(mockSale);
    await press(tree!, '공용 화면 닫기');
    expect(view(tree!)).toContain('새 고객 주문');
    await Renderer.act(async () => tree!.unmount());
  } finally {
    jest.useRealTimers();
  }
});

test.each(['unbound', 'issued'] as const)('K08 automatically resets an idle %s public session without writing another order or quote', async stage => {
  fakeSessionTimers();
  mockRole = 'terminal';
  mockMenuCheckout = true;
  mockMenuPrice = '6500000';
  let tree!: Renderer.ReactTestRenderer;
  try {
    await Renderer.act(async () => {
      tree = Renderer.create(<App />);
    });
    await press(tree, '새 고객 주문');
    await press(tree, '차 hot 추가');
    await press(tree, '하드월렛으로 결제');
    expect(view(tree)).toContain('이 주문을 여는 일회용 QR');
    if (stage === 'issued') {
      await press(tree, '휴대전화 확인 번호 조회');
      await press(tree, '확인 번호 2ba8cf04와 일치');
      mockMenuOrderState = 'issued';
      await press(tree, '이 주문 다시 확인');
      expect(view(tree)).toContain('휴대전화에서 승인 중');
    }
    const requests = [...mockDialed];
    await Renderer.act(async () => jest.advanceTimersByTime(41_000));
    if (stage === 'unbound') {
      expect(view(tree)).toContain('연결 시간이 지났습니다');
      expect(view(tree)).toContain('공용 화면 · 자동 초기화 대기');
      expect(view(tree)).not.toContain(mockSale);
      expect(view(tree)).not.toContain('금액 6.50 TestUSDC');
      expect(view(tree)).not.toContain('이 주문을 여는 일회용 QR');
    } else {
      expect(view(tree)).toContain('휴대전화에서 승인 중');
      expect(view(tree)).not.toContain('연결 시간이 지났습니다');
    }
    await Renderer.act(async () => jest.advanceTimersByTime(20_000));
    if (stage === 'issued') {
      // An order being approved is only read again, and it outlives the idle minute until its own deadline.
      expect(view(tree)).toContain('휴대전화에서 승인 중');
      await Renderer.act(async () => { await jest.advanceTimersByTimeAsync(40_000); });
      expect(view(tree)).toContain('승인 시간이 지났습니다');
      expect(view(tree)).toContain('다시 결제하지 마세요.');
      await Renderer.act(async () => { await jest.advanceTimersByTimeAsync(61_000); });
      expect(mockDialed.filter(op => op !== 'order.status')).toEqual(requests.filter(op => op !== 'order.status'));
    } else {
      expect(mockDialed).toEqual([...requests, 'order.cancel-public']);
    }
    expect(view(tree)).toContain('새 고객 주문');
    for (const prior of [mockSale, '2ba8cf04', '휴대전화에서 승인 중', '연결 시간이 지났습니다', '이 주문을 여는 일회용 QR']) {
      expect(view(tree)).not.toContain(prior);
    }
    await press(tree, '새 고객 주문');
    expect(view(tree)).not.toContain(mockSale);
    expect(mockDialed.filter(op => op === 'order.create')).toHaveLength(1);
  } finally {
    await Renderer.act(async () => tree?.unmount());
    jest.useRealTimers();
  }
});

test('POS handoff opens no menu and resumes the same server receipt after restarting', async () => {
  mockRole = 'terminal';
  mockMode = 'pos';
  let tree: Renderer.ReactTestRenderer;
  const start = async () => {
    await Renderer.act(async () => {
      tree = Renderer.create(<App />);
    });
    await press(tree!, '새 고객 주문');
    expect(view(tree!)).not.toContain('차 hot 추가');
    const input = tree!.root
      .findAllByProps({ accessibilityLabel: 'POS 인계 코드' })
      .find(n => typeof n.props.onChangeText === 'function')!;
    await Renderer.act(async () => input.props.onChangeText(mockHandoff));
    await press(tree!, 'POS 원주문 조회');
  };
  await start();
  expect(view(tree!)).toContain('POS 주문 S-0142');
  expect(view(tree!)).toContain('6.50 TestUSDC');
  expect(view(tree!)).toContain('이 주문을 여는 일회용 QR');
  await Renderer.act(async () => tree!.unmount());
  mockPOSPaid = true;
  await start();
  expect(view(tree!)).toContain('결제 완료');
  expect(view(tree!)).toContain('영수증 ' + mockReceipt.slice(-8).toUpperCase());
  expect(view(tree!)).not.toContain(mockReceipt);
  expect(view(tree!)).not.toContain('이 주문을 여는 일회용 QR');
  expect(mockDialed).toEqual([
    'terminal.state',
    'pos.lookup',
    'terminal.state',
    'pos.lookup',
  ]);
  await Renderer.act(async () => tree!.unmount());
});

async function openPOS(tree: Renderer.ReactTestRenderer, handoff: string) {
  const input = tree.root.findAllByProps({accessibilityLabel: 'POS 인계 코드'})
    .find(n => typeof n.props.onChangeText === 'function')!;
  await Renderer.act(async () => input.props.onChangeText(handoff));
  await press(tree, 'POS 원주문 조회');
}

test('K11 keeps a pending POS original readable and isolates a second POS while the committed quote reply is lost', async () => {
  mockRole = 'terminal';
  mockMode = 'pos';
  let release!: () => void;
  mockPOSQuoteGate = new Promise<void>(resolve => { release = resolve; });
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    await press(tree!, '새 고객 주문');
    await openPOS(tree!, mockHandoff);
    await press(tree!, '휴대전화 확인 번호 조회');
    await press(tree!, '확인 번호 2ba8cf04와 일치');
    expect(view(tree!)).toContain('선택한 지급자의 견적을 기록하고 있습니다');
    expect(view(tree!)).toContain('선택한 표식 · 2ba8cf04');
    expect(view(tree!)).not.toContain('직접 승인할 보드 찾기');
    expect(view(tree!)).not.toContain('이 주문을 여는 일회용 QR');
    await press(tree!, 'POS 원주문 다시 조회');
    expect(view(tree!)).toContain('선택한 지급자의 견적을 기록하고 있습니다');
    await press(tree!, '다른 POS 원주문 열기');
    await openPOS(tree!, mockSecondHandoff);
    expect(view(tree!)).toContain('POS 주문 S-0143');
    expect(view(tree!)).toContain('4.20 TestUSDC');
    expect(view(tree!)).not.toContain('2ba8cf04');
    await Renderer.act(async () => release());
    expect(view(tree!)).toContain('POS 주문 S-0143');
    await Renderer.act(async () => tree!.unmount());
    await Renderer.act(async () => { tree = Renderer.create(<App />); });
    await press(tree!, '새 고객 주문');
    await openPOS(tree!, mockHandoff);
    expect(view(tree!)).toContain('휴대전화에서 승인 중');
    expect(view(tree!)).toContain('연결한 휴대전화 · 2ba8cf04');
    expect(mockDialed.filter(row => row === 'quote.issue')).toHaveLength(1);
    expect(mockDialed.filter(row => row === 'checkout.confirm')).toHaveLength(1);
  } finally {
    release();
    await Renderer.act(async () => tree!.unmount());
  }
});

test('K11 shows a same-original POS conflict without exposing an unselected mark or issuing a quote', async () => {
  mockRole = 'terminal';
  mockMode = 'pos';
  mockPOSConflict = true;
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    await press(tree!, '새 고객 주문');
    await openPOS(tree!, mockHandoff);
    await press(tree!, '휴대전화 확인 번호 조회');
    await press(tree!, '확인 번호 2ba8cf04와 일치');
    expect(view(tree!)).toContain('다른 지급자가 선택됐습니다');
    expect(view(tree!)).not.toContain('2ba8cf04');
    expect(view(tree!)).not.toContain('37ea19cd');
    expect(mockDialed).not.toContain('quote.issue');
    await press(tree!, 'POS 원주문 다시 조회');
    expect(view(tree!)).toContain('POS 주문 S-0142');
    expect(view(tree!)).toContain('지급자 확인됨 · 견적 기록 대기');
  } finally {
    await Renderer.act(async () => tree!.unmount());
  }
});

test('K11 explicitly holds a legacy POS order with competing draft claims', async () => {
  mockRole = 'terminal';
  mockMode = 'pos';
  mockPOSLegacyHeld = true;
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    await press(tree!, '새 고객 주문');
    await openPOS(tree!, mockHandoff);
    expect(view(tree!)).toContain('기존 지급자 결합을 확인해야 합니다 · 운영자 확인 대기');
    expect(view(tree!)).not.toContain('이 주문을 여는 일회용 QR');
    expect(view(tree!)).not.toContain('직접 승인할 보드 찾기');
    expect(mockDialed).not.toContain('quote.issue');
  } finally {
    await Renderer.act(async () => tree!.unmount());
  }
});

test('a rejected POS asset has no payment locator or new order action', async () => {
  mockRole = 'terminal';
  mockMode = 'pos';
  mockPOSUnsupported = true;
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<App />);
  });
  await press(tree!, '새 고객 주문');
  const input = tree!.root
    .findAllByProps({ accessibilityLabel: 'POS 인계 코드' })
    .find(n => typeof n.props.onChangeText === 'function')!;
  await Renderer.act(async () => input.props.onChangeText(mockHandoff));
  await press(tree!, 'POS 원주문 조회');
  expect(view(tree!)).toContain('TestUSDC 결제만 가능합니다.');
  expect(view(tree!)).not.toContain('이 주문을 여는 일회용 QR');
  expect(mockDialed).toEqual(['terminal.state', 'pos.lookup']);
  await Renderer.act(async () => tree!.unmount());
});

test('K02 preserves the other purchasable line when one selected SKU sells out', async () => {
  mockRole = 'terminal';
  mockTeaAvailable = 2;
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<App />);
  });
  try {
    await press(tree!, '새 고객 주문');
    await press(tree!, '차 hot 추가');
    await press(tree!, '빵 plain 추가');
    expect(view(tree!)).toContain('가능 수량 2 · 선택 1');
    mockTeaAvailable = 0;
    await press(tree!, '하드월렛으로 결제');
    expect(mockDialed).not.toContain('order.create');
    expect(view(tree!)).toContain('확보 못한 품목 · 차 · hot ×1');
    expect(view(tree!)).toContain('가능 수량 2 · 선택 1');
  } finally {
    await Renderer.act(async () => tree!.unmount());
  }
});

test('K12 ignores an authority reply that arrives after cancellation and clears protected settings on exit', async () => {
  mockRole = 'operator';
  let finish = () => {};
  mockStaffGate = new Promise(resolve => {
    finish = resolve;
  });
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<App />);
  });
  try {
    expect(view(tree!)).not.toContain('현재 단말 모드 조회');
    await press(tree!, '직원 권한 확인');
    expect(view(tree!)).toContain('지점 권한을 확인하고 있습니다.');
    await press(tree!, '인증 취소');
    await Renderer.act(async () => finish());
    expect(view(tree!)).not.toContain('현재 단말 모드 조회');
    await press(tree!, '직원 인증');
    await press(tree!, '직원 권한 확인');
    expect(view(tree!)).toContain('현재 단말 모드 조회');
    await press(tree!, '고객 화면');
    expect(view(tree!)).not.toContain('현재 단말 모드 조회');
    expect(view(tree!)).not.toContain(mockActor);
  } finally {
    await Renderer.act(async () => tree!.unmount());
  }
});

test('K12 clerk receives only the authenticated branch pickup queue', async () => {
  mockRole = 'operator';
  mockStaffPermissions = ['pickup'];
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<App />);
  });
  try {
    await press(tree!, '직원 권한 확인');
    expect(view(tree!)).not.toContain('현재 단말 모드 조회');
    expect(view(tree!)).not.toContain('P05 환불 운영 화면');
    await press(tree!, '지점 픽업 대기열');
    await press(tree!, '주문 목록 새로 고침');
    expect(view(tree!)).toContain('픽업 37');
    expect(mockDialed).toEqual(['staff.access', 'pickup-queue', 'pickup-queue']);
    await press(tree!, '고객 화면');
    expect(view(tree!)).not.toContain('픽업 37');
  } finally {
    await Renderer.act(async () => tree!.unmount());
  }
});

test('K12 denies a mismatched branch or revoked credential and gates the P05 refund console', async () => {
  mockRole = 'finance';
  mockStaffPermissions = ['refund_console'];
  mockStaffBranch = ref(90);
  const openURL = jest.spyOn(Linking, 'openURL').mockResolvedValue(undefined);
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<App />);
  });
  try {
    await press(tree!, '직원 권한 확인');
    expect(view(tree!)).toContain('이 단말을 운영할 권한이 없습니다.');
    expect(view(tree!)).not.toContain('P05 환불 운영 화면');
    mockStaffBranch = mockShop;
    mockStaffDenied = true;
    await press(tree!, '다시 인증');
    await press(tree!, '직원 권한 확인');
    expect(view(tree!)).not.toContain('P05 환불 운영 화면');
    mockStaffDenied = false;
    await press(tree!, '다시 인증');
    await press(tree!, '직원 권한 확인');
    expect(view(tree!)).not.toContain('현재 단말 모드 조회');
    expect(view(tree!)).not.toContain('지점 픽업 대기열');
    await press(tree!, 'P05 환불 운영 화면');
    expect(openURL).toHaveBeenCalledWith('https://ops.example.invalid');
    mockStaffDenied = true;
    await press(tree!, 'P05 환불 운영 화면');
    expect(openURL).toHaveBeenCalledTimes(1);
  } finally {
    openURL.mockRestore();
    await Renderer.act(async () => tree!.unmount());
  }
});

test('K06 shows a 21st original order in both the impact preview and saved handoff', async () => {
  mockRole = 'operator';
  mockOrders = Array.from({ length: 21 }, (_, index) => ref(40 + index));
  const last = mockOrders[20];
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<App />);
  });
  try {
    await press(tree!, '직원 권한 확인');
    const target = tree!.root
      .findAllByProps({ accessibilityLabel: '단말 식별값' })
      .find(n => typeof n.props.onChangeText === 'function')!;
    await Renderer.act(async () => target.props.onChangeText(mockTerminal));
    await press(tree!, '현재 단말 모드 조회');
    await press(tree!, '결제 전용 모드로 전환');
    await press(tree!, '다음 인계 주문');
    expect(view(tree!)).toContain(last);
    expect(mockDialed).not.toContain('terminal.mode');
    await press(tree!, '전환 요청 저장');
    await press(tree!, '다음 인계 주문');
    expect(view(tree!)).toContain(last);
    expect(view(tree!)).toContain('지점 인계 대기열 · 21건');
  } finally {
    await Renderer.act(async () => tree!.unmount());
  }
});

test('K12 dedicated kitchen reauthenticates before resuming the original pickup mutation', async () => {
  mockRole = 'terminal';
  mockDedicatedPickup = true;
  mockStaffPermissions = ['pickup'];
  mockLosePickupReply = true;
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<App />);
  });
  const openQueue = async () => {
    await press(tree!, '직원 픽업 접수');
    expect(view(tree!)).not.toContain('주문 목록 새로 고침');
    await press(tree!, '직원 권한 확인');
    await press(tree!, '지점 픽업 대기열');
    await press(tree!, '주문 목록 새로 고침');
  };
  try {
    expect(view(tree!)).not.toContain('새 고객 주문');
    await openQueue();
    const label = tree!.root.findAll(
      node =>
        typeof node.props.accessibilityLabel === 'string' &&
        node.props.accessibilityLabel.startsWith('픽업 37 ·'),
    )[0].props.accessibilityLabel;
    await press(tree!, label);
    await press(tree!, '조리 시작');
    expect(mockPickupCommands).toHaveLength(1);
    const journal = [...mockJournal.keys()].find(name =>
      name.startsWith('pickup-v2-'),
    )!;
    expect(mockJournal.get(journal)).toContain(mockSale);
    await press(tree!, '고객 화면');
    expect(view(tree!)).not.toContain(mockSale);
    expect(view(tree!)).not.toContain('방금 누른 처리 결과 확인');
    mockLosePickupReply = false;
    await openQueue();
    await press(tree!, '방금 누른 처리 결과 확인');
    expect(mockPickupCommands).toEqual([
      mockPickupCommands[0],
      mockPickupCommands[0],
    ]);
    expect(mockJournal.get(journal)).toBe('null');
    expect(view(tree!)).toContain('준비 중');
    expect(mockDialed.filter(op => op === 'staff.access')).toHaveLength(2);
  } finally {
    await Renderer.act(async () => tree!.unmount());
  }
});

test('K03 shows a branch-only entrance QR separately from the original checkout QR', async () => {
  mockRole = 'terminal';
  mockMenuCheckout = true;
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    await press(tree, '매장 입구 QR 보기');
    expect(view(tree)).toContain('개인 주문을 시작하는 매장 입구 QR');
    expect(view(tree)).not.toContain('이 주문을 여는 일회용 QR');
    expect(view(tree)).not.toContain(mockSale);
    await press(tree, '새 고객 주문');
    await press(tree, '차 hot 추가');
    await press(tree, '하드월렛으로 결제');
    expect(view(tree)).not.toContain('개인 주문을 시작하는 매장 입구 QR');
    expect(view(tree)).toContain('이 주문을 여는 일회용 QR');
  } finally {
    await Renderer.act(async () => tree.unmount());
  }
});

test('K08 selects one public menu customer and keeps the original order after issuance', async () => {
  mockRole = 'terminal';
  mockMenuCheckout = true;
  mockMenuPrice = '6500000';
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<App />);
  });
  try {
    await press(tree!, '새 고객 주문');
    await press(tree!, '차 hot 추가');
    await press(tree!, '하드월렛으로 결제');
    expect(view(tree!)).toContain('여울 커피 성수점');
    expect(view(tree!)).toContain('StableNet 테스트넷 8283');
    expect(view(tree!)).toContain('6.50 TestUSDC');
    expect(view(tree!)).not.toContain(mockSale);
    expect(view(tree!)).toContain('이 주문을 여는 일회용 QR');
    expect(view(tree!)).not.toContain('POS 주문');
    await press(tree!, '휴대전화 확인 번호 조회');
    expect(view(tree!)).toContain('확인 번호 37ea19cd와 일치');
    await press(tree!, '확인 번호 2ba8cf04와 일치');
    expect(mockSelected).toHaveLength(1);
    expect(view(tree!)).toContain('연결한 휴대전화 · 2ba8cf04');
    // The phone chosen by its mark may now read and lock this order over its nearby link.
    expect(JSON.parse(mockJournal.get('commerce-inbox-v2')!).claims)
      .toEqual([{sale: mockSale, owner: mockCandidates[1].customerRef, until: expect.any(Number)}]);
    expect(view(tree!)).not.toContain('37ea19cd');
    expect(view(tree!)).not.toContain('이 주문을 여는 일회용 QR');

    mockMenuOrderState = 'issued';
    await press(tree!, '이 주문 다시 확인');
    expect(view(tree!)).not.toContain(mockSale);
    expect(view(tree!)).toContain('연결한 휴대전화 · 2ba8cf04');
    expect(view(tree!)).not.toContain('이 주문을 여는 일회용 QR');
    expect(
      mockDialed.filter(operation => operation === 'order.create'),
    ).toHaveLength(1);
  } finally {
    await Renderer.act(async () => tree!.unmount());
  }
});

test('K08 shows the selected mark during binding, then offers a distinct unknown state', async () => {
  mockRole = 'terminal';
  mockMenuCheckout = true;
  mockLoseComparisonReply = 'before';
  let release!: () => void;
  mockComparisonGate = new Promise<void>(resolve => { release = resolve; });
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    await press(tree!, '새 고객 주문');
    await press(tree!, '차 hot 추가');
    await press(tree!, '하드월렛으로 결제');
    await press(tree!, '휴대전화 확인 번호 조회');
    await press(tree!, '확인 번호 2ba8cf04와 일치');
    expect(mockSelected).toHaveLength(1);
    expect(view(tree!)).toContain('이 폰의 결제 요청을 준비합니다');
    expect(view(tree!)).toContain('선택한 표식 · 2ba8cf04');
    expect(view(tree!)).not.toContain('연결한 휴대전화 · 2ba8cf04');
    expect(view(tree!)).not.toContain('37ea19cd');
    expect(view(tree!)).not.toContain('이 주문을 여는 일회용 QR');
    await Renderer.act(async () => release());
    expect(view(tree!)).toContain('기존 결제 연결을 확인하고 있습니다');
    expect(view(tree!)).toContain('기존 연결 조회');
    expect(view(tree!)).not.toContain('선택한 표식');
    expect(view(tree!)).not.toContain('이 주문을 여는 일회용 QR');
  } finally {
    release();
    await Renderer.act(async () => tree!.unmount());
  }
});

test('K08 safely retries an unreceived comparison after relaunch using the same original order and key', async () => {
  mockRole = 'terminal';
  mockMenuCheckout = true;
  mockLoseComparisonReply = 'before';
  let tree: Renderer.ReactTestRenderer;
  const mount = async () => {
    await Renderer.act(async () => {
      tree = Renderer.create(<App />);
    });
  };
  await mount();
  try {
    await press(tree!, '새 고객 주문');
    await press(tree!, '차 hot 추가');
    await press(tree!, '하드월렛으로 결제');
    await press(tree!, '휴대전화 확인 번호 조회');
    await press(tree!, '확인 번호 2ba8cf04와 일치');
    expect(mockSelected).toHaveLength(1);
    expect(view(tree!)).not.toContain('37ea19cd');
    expect(view(tree!)).not.toContain('이 주문을 여는 일회용 QR');
    await Renderer.act(async () => tree!.unmount());
    mockLoseComparisonReply = undefined;
    await mount();
    expect(view(tree!)).not.toContain('2ba8cf04');
    await press(tree!, '보류 연결 상태 확인');
    expect(mockSelected).toEqual([mockSelected[0], mockSelected[0]]);
    expect(view(tree!)).not.toContain('보류 연결 상태 확인');
    expect(view(tree!)).not.toContain('이 주문을 여는 일회용 QR');
    expect(view(tree!)).not.toContain('2ba8cf04');
    expect(
      mockDialed.filter(operation => operation === 'order.create'),
    ).toHaveLength(1);
  } finally {
    await Renderer.act(async () => tree!.unmount());
  }
});

test('K08 recovers the already bound customer after a lost response without submitting again', async () => {
  mockRole = 'terminal';
  mockMenuCheckout = true;
  mockLoseComparisonReply = 'after';
  let tree: Renderer.ReactTestRenderer;
  await Renderer.act(async () => {
    tree = Renderer.create(<App />);
  });
  try {
    await press(tree!, '새 고객 주문');
    await press(tree!, '차 hot 추가');
    await press(tree!, '하드월렛으로 결제');
    await press(tree!, '휴대전화 확인 번호 조회');
    await press(tree!, '확인 번호 2ba8cf04와 일치');
    expect(view(tree!)).not.toContain('연결한 휴대전화 · 2ba8cf04');
    expect(view(tree!)).not.toContain('이 주문을 여는 일회용 QR');
    await press(tree!, '기존 연결 조회');
    expect(view(tree!)).toContain('연결한 휴대전화 · 2ba8cf04');
    expect(view(tree!)).not.toContain(mockSale);
    expect(view(tree!)).not.toContain('37ea19cd');
    expect(view(tree!)).not.toContain('이 주문을 여는 일회용 QR');
    expect(mockSelected).toHaveLength(1);
    expect(mockJournal.get('commerce-inbox-v2')).not.toContain('comparison');
  } finally {
    await Renderer.act(async () => tree!.unmount());
  }
});

test.each(['relay-paid', 'expired-unpaid'] as const)('K08 shows only the authenticated terminal outcome %s and resets the public session', async outcome => {
  mockRole = 'terminal';
  mockMenuCheckout = true;
  mockMenuPrice = '6500000';
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    await press(tree, '새 고객 주문');
    await press(tree, '차 hot 추가');
    mockMenuVersion = 2;
    mockMenuPrice = mockMenuOrderAmount = '7000000';
    await press(tree, '하드월렛으로 결제');
    expect(view(tree)).toContain('새 합계 · 7.00 TestUSDC');
    expect(mockDialed).not.toContain('order.create');
    await press(tree, '바뀐 장바구니 확인 후 주문 접수');
    await press(tree, '휴대전화 확인 번호 조회');
    await press(tree, '확인 번호 2ba8cf04와 일치');
    mockMenuOrderState = 'issued';
    await press(tree, '이 주문 다시 확인');
    expect(view(tree)).toContain('휴대전화에서 승인 중');
    expect(view(tree)).not.toContain('빠져나간 돈은 없습니다');

    if (outcome === 'expired-unpaid') {
      mockMenuOrderState = 'closed';
      await press(tree, '이 주문 다시 확인');
      expect(view(tree)).toContain('주문 종료');
      expect(view(tree)).not.toContain('빠져나간 돈은 없습니다');
    }
    mockMenuOrderState = outcome === 'relay-paid' ? 'paid' : 'closed';
    mockTerminalOutcome = outcome;
    await press(tree, '이 주문 다시 확인');
    if (outcome === 'relay-paid') {
      expect(view(tree)).toContain('결제 완료');
      expect(view(tree)).toContain('7.00 TestUSDC');
      expect(view(tree)).toContain('주문번호');
      expect(view(tree)).toContain('네트워크 수수료는 매장이 냈습니다');
    } else {
      expect(view(tree)).toContain('결제되지 않았습니다 · 빠져나간 돈은 없습니다');
      expect(view(tree)).not.toContain('주문번호');
    }
    expect(view(tree)).not.toContain('2ba8cf04');
    expect(view(tree)).not.toContain(mockReceipt);
    expect(view(tree)).not.toContain('이 주문을 여는 일회용 QR');
    expect(mockDialed.filter(operation => operation === 'order.create')).toHaveLength(1);
    expect(mockDialed).not.toContain('quote.issue');
    expect(mockDialed).not.toContain('payment.forward');
    await press(tree, outcome === 'relay-paid' ? '처음 화면으로' : '화면을 비우고 대기');
    await Renderer.act(async () => tree.unmount());
    await Renderer.act(async () => { tree = Renderer.create(<App />); });
    await press(tree, '새 고객 주문');
    for (const stale of [mockSale, mockReceipt, '2ba8cf04', '주문번호', '빠져나간 돈은 없습니다', '이 주문을 여는 일회용 QR']) {
      expect(view(tree)).not.toContain(stale);
    }
    expect(mockDialed.filter(operation => operation === 'order.create')).toHaveLength(1);
  } finally {
    await Renderer.act(async () => tree.unmount());
  }
});

test('K07 requires an explicit published option and keeps quantity on validation', async () => {
  mockK07Published = true;
  mockMenuCheckout = true;
  let tree!: Renderer.ReactTestRenderer;
  const heading = {};
  const announce = jest.spyOn(AccessibilityInfo, 'announceForAccessibility').mockImplementation(() => {});
  const focus = jest.spyOn(AccessibilityInfo, 'sendAccessibilityEvent').mockImplementation(() => {});
  await Renderer.act(async () => {
    tree = Renderer.create(<App />, {
      createNodeMock: element => (element.props as {testID?: string}).testID === 'menu-option-group-heading' ? heading : null,
    });
  });
  try {
    await press(tree, '새 고객 주문');
    await press(tree, '크루아상 상세');
    expect(view(tree)).toContain('점주 신고 성분 밀, 우유, 달걀');
    expect(view(tree)).toContain('공용 제빵 설비에서 교차접촉 가능 · 견과류');
    expect(view(tree)).toContain('알레르기가 있으면 주문 전에 직원에게 제조 환경을 확인하세요.');
    await press(tree, '수량 늘리기');
    expect(view(tree)).toContain('소계 · 6.00 TestUSDC');
    await press(tree, '장바구니에 담기');
    await press(tree, '아메리카노 옵션 선택');
    await press(tree, '수량 늘리기');
    await press(tree, '장바구니에 담기');
    expect(view(tree)).toContain('온도 (필수)');
    expect(view(tree)).toContain('온도를 선택해 주세요');
    expect(announce).toHaveBeenCalledWith('온도를 선택해 주세요');
    await Renderer.act(async () => { await new Promise<void>(resolve => setTimeout(() => resolve(), 25)); });
    expect(focus.mock.calls.some(([target, event]) => !!target && event === 'focus')).toBe(true);
    expect(view(tree)).toContain('수량 · 2');
    expect(mockDialed.filter(command => command === 'order.create')).toHaveLength(0);
    await press(tree, '메뉴 언어 English');
    expect(view(tree)).toContain('Quantity · 2');
    expect(view(tree)).toContain('Temperature (required)');
    expect(view(tree)).toContain('Choose an option for Temperature');
    await press(tree, 'Select Iced');
    expect(view(tree)).toContain('Subtotal · 7.00 TestUSDC');
    await press(tree, 'Menu language 한국어');
    await press(tree, '장바구니에 담기');
    expect(view(tree)).toContain('아메리카노 · 아이스 ×2');
    expect(view(tree)).toContain('크루아상 · 기본 ×2');
    await press(tree, '하드월렛으로 결제');
    expect(mockSubmittedCarts).toHaveLength(1);
    expect(mockSubmittedCarts[0].items).toEqual([
      {sku: 'baked-09', option_key: 'standard', quantity: 2},
      {sku: 'drink-12', option_key: 'iced', quantity: 2},
    ]);
  } finally {
    announce.mockRestore();
    focus.mockRestore();
    await Renderer.act(async () => tree.unmount());
  }
});

test('K07 keeps the original SKU quantity until an explicit option replacement', async () => {
  mockK07Published = true;
  mockMenuCheckout = true;
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    await press(tree, '새 고객 주문');
    await press(tree, '아메리카노 옵션 선택');
    await press(tree, '아이스 선택');
    await press(tree, '장바구니에 담기');
    await press(tree, '아메리카노 옵션 선택');
    await press(tree, '따뜻하게 선택');
    await press(tree, '수량 늘리기');
    await press(tree, '장바구니에 담기');
    expect(view(tree)).toContain('아메리카노 · 아이스 ×1');
    expect(view(tree)).toContain('아메리카노 · 따뜻하게 ×2');
    await press(tree, '기존 옵션 유지');
    expect(view(tree)).toContain('아메리카노 · 아이스 ×1');
    expect(mockSubmittedCarts).toHaveLength(0);
    await press(tree, '장바구니에 담기');
    await press(tree, '새 옵션으로 교체');
    expect(view(tree)).toContain('아메리카노 · 따뜻하게 ×2');
    expect(view(tree)).not.toContain('아메리카노 · 아이스 ×1');
    expect(mockSubmittedCarts).toHaveLength(0);
  } finally { await Renderer.act(async () => tree.unmount()); }
});

test('K07 adds, edits and removes independent options within one SKU limit', async () => {
  mockK07Published = true;
  mockK07SKUStock = 3;
  mockMenuCheckout = true;
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    await press(tree, '새 고객 주문');
    await press(tree, '아메리카노 옵션 선택');
    await press(tree, '따뜻하게 선택');
    await press(tree, '장바구니에 담기');
    await press(tree, '아메리카노 옵션 선택');
    await press(tree, '아이스 선택');
    await press(tree, '수량 늘리기');
    await press(tree, '장바구니에 담기');
    expect(view(tree)).toContain('아메리카노 · 따뜻하게 ×1');
    expect(view(tree)).toContain('아메리카노 · 아이스 ×2');
    expect(view(tree)).toContain('합계 · 11.00 TestUSDC');
    await press(tree, '아메리카노 옵션 선택');
    await press(tree, '아이스 선택');
    expect(view(tree)).toContain('수량 · 2');
    const increase = tree.root.findAll(node =>
      node.props.label === '수량 늘리기' && typeof node.props.run === 'function')[0];
    expect(increase.props.disabled).toBe(true);
    await press(tree, '수량 줄이기');
    await press(tree, '메뉴로 돌아가기');
    expect(view(tree)).toContain('아메리카노 · 아이스 ×2');
    await press(tree, '아메리카노 옵션 선택');
    await press(tree, '아이스 선택');
    await press(tree, '수량 줄이기');
    await press(tree, '장바구니에 담기');
    expect(view(tree)).toContain('아메리카노 · 아이스 ×1');
    expect(view(tree)).toContain('아메리카노 · 따뜻하게 ×1');
    await press(tree, '아메리카노 옵션 선택');
    await press(tree, '아이스 선택');
    await press(tree, '수량 늘리기');
    await press(tree, '장바구니에 담기');
    await press(tree, '아메리카노 따뜻하게 빼기');
    expect(view(tree)).not.toContain('아메리카노 · 따뜻하게 ×1');
    expect(view(tree)).toContain('아메리카노 · 아이스 ×2');
    await press(tree, '아메리카노 옵션 선택');
    await press(tree, '따뜻하게 선택');
    await press(tree, '장바구니에 담기');
    await press(tree, '하드월렛으로 결제');
    expect(mockSubmittedCarts.map(cart => cart.items)).toEqual([[
      {sku: 'drink-12', option_key: 'hot', quantity: 1},
      {sku: 'drink-12', option_key: 'iced', quantity: 2},
    ]]);
  } finally { await Renderer.act(async () => tree.unmount()); }
});

test('K07 legacy quick-add also asks before changing the option of an existing SKU', async () => {
  mockK07Published = true;
  mockK07Legacy = true;
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    await press(tree, '새 고객 주문');
    await press(tree, '아메리카노 아이스 추가');
    await press(tree, '아메리카노 아이스 추가');
    await press(tree, '아메리카노 따뜻하게 추가');
    expect(view(tree)).toContain('아메리카노 · 아이스 ×2');
    expect(view(tree)).toContain('아메리카노 · 따뜻하게 ×1');
    await press(tree, '기존 옵션 유지');
    expect(view(tree)).toContain('아메리카노 · 아이스 ×2');
    expect(view(tree)).not.toContain('아메리카노 · 따뜻하게 ×1');
    expect(mockDialed.filter(command => command === 'order.create')).toHaveLength(0);
  } finally { await Renderer.act(async () => tree.unmount()); }
});

test('K01 filters two pages of public categories by accepted labels and keeps the cart across locale changes', async () => {
  mockCategorizedPages = true;
  mockMenuCheckout = true;
  let tree!: Renderer.ReactTestRenderer;
  const mount = async () => Renderer.act(async () => { tree = Renderer.create(<App />); });
  const close = async () => Renderer.act(async () => { tree.unmount(); });
  await mount();
  try {
    await press(tree, '새 고객 주문');
    expect(mockMenuPages).toEqual(['0', '1']);
    expect(view(tree)).toContain('한정 메뉴');
    await press(tree, '베이커리');
    expect(view(tree)).not.toContain('한정 메뉴');
    await press(tree, '크루아상 상세');
    await press(tree, '장바구니에 담기');
    await press(tree, '메뉴 언어 English');
    expect(tree.root.findAll(node => node.props.accessibilityLabel === 'Bakery' &&
      node.props.accessibilityState?.selected === true).length).toBeGreaterThan(0);
    await press(tree, 'Drinks');
    expect(view(tree)).toContain('Tea');
    expect(view(tree)).toContain('Croissant');
    await press(tree, 'Menu language 한국어');
    await press(tree, '하드월렛으로 결제');
    expect(mockSubmittedCarts[0]?.items).toEqual([{sku:'z-croissant', option_key:'standard', quantity:1}]);
  } finally { await close(); }

  mockMenuFailure = 'offline';
  await mount();
  try {
    await press(tree, '새 고객 주문');
    await press(tree, '베이커리');
    expect(view(tree)).toContain('크루아상');
    expect(view(tree)).not.toContain('한정 메뉴');
    await press(tree, '메뉴 언어 English');
    expect(view(tree)).toContain('Croissant');
    expect(view(tree)).not.toContain('한정 메뉴');
    expect(view(tree)).toContain('New quotes paused');
    await press(tree, 'All items');
    expect(view(tree)).toContain('한정 메뉴');
    expect(mockSubmittedCarts).toHaveLength(1);
  } finally { await close(); }
});

test('K01 persists an authenticated read-only menu and opens one fresh order after reconnection', async () => {
  mockK07Published=true;
  mockMenuCheckout=true;
  let tree!:Renderer.ReactTestRenderer;
  const mount=async()=>{await Renderer.act(async()=>{tree=Renderer.create(<App />);});};
  const close=async()=>{await Renderer.act(async()=>tree.unmount());};
  await mount();
  try {
    await press(tree,'새 고객 주문');
    expect(view(tree)).toContain('여울 커피 성수점');
    await close();
    const saved=mockJournal.get('terminal-public-menu-v1');
    expect(saved).toContain('여울 커피 성수점');
    expect(saved).toContain('shared-baking-equipment');
    expect(saved).toContain('tree-nuts');
    expect(saved).toContain('Temperature');
    expect(saved).not.toMatch(/privateKey|certificateChain|subjectRef|merchantId|cart|order/i);
    mockMenuFailure='offline';
    await mount();
    await press(tree,'새 고객 주문');
    expect(view(tree)).toContain('최근 메뉴를 보고 있습니다');
    expect(view(tree)).toContain('여울 커피 성수점');
    expect(view(tree)).toContain('마지막 조회');
    expect(view(tree)).not.toMatch(/새 고객 주문|장바구니에 담기|하드월렛으로 결제|매장 입구 QR/);
    expect(mockDialed).not.toContain('order.create');
    await press(tree,'메뉴 언어 English');
    expect(view(tree)).toContain('Viewing the last synced menu');
    expect(view(tree)).toContain('Croissant');
    await press(tree,'Check store connection');
    expect(view(tree)).toContain('Viewing the last synced menu');
    mockMenuFailure=undefined;
    mockMenuVersion++;
    await press(tree,'Check store connection');
    expect(view(tree)).toContain('현재 메뉴 동기화 완료');
    expect(mockDialed).not.toContain('order.create');
    await press(tree,'새 고객 주문');
    await press(tree,'크루아상 상세');
    await press(tree,'장바구니에 담기');
    await press(tree,'하드월렛으로 결제');
    expect(mockSubmittedCarts).toHaveLength(1);
    expect(mockSubmittedCarts[0].menuVersion).toBe(2);
  } finally { await close(); }
});

test.each(['trust', 'identity', 'branch', 'expired', 'POS', 'mode-version'] as const)(
  'K01 keeps the cached menu closed on %s failure or change', async reason => {
    let tree!:Renderer.ReactTestRenderer;
    const mount=async()=>{await Renderer.act(async()=>{tree=Renderer.create(<App />);});};
    const close=async()=>{await Renderer.act(async()=>tree.unmount());};
    await mount();
    await press(tree,'새 고객 주문');
    expect(mockJournal.get('terminal-public-menu-v1')).toContain('여울 커피 성수점');
    await close();
    switch(reason) {
      case 'trust': mockMenuFailure='trust'; break;
      case 'identity': mockTerminalKey='changed-key'; mockMenuFailure='offline'; break;
      case 'branch': mockColdBranch=ref(99); mockMenuFailure='offline'; break;
      case 'expired': mockTerminalExpires=mockClock()-1; mockMenuFailure='offline'; break;
      case 'POS': mockMode='pos'; mockModeRevision=2; break;
      case 'mode-version': mockModeRevision=2; mockMenuFailure='offline'; mockMenuFailureAt='catalog.read'; break;
    }
    await mount();
    try {
      // A locally expired profile can block before the public action is rendered.
      if (view(tree).includes('새 고객 주문')) await press(tree,'새 고객 주문');
      expect(view(tree)).not.toContain('최근 메뉴를 보고 있습니다');
      expect(view(tree)).not.toContain('여울 커피 성수점');
      expect(mockJournal.get('terminal-public-menu-v1') || '').toBe('');
      expect(mockDialed).not.toContain('order.create');
    } finally { await close(); }
  },
);

test('K01 opens the public basket from outside the full menu scroll area', async () => {
  mockCategorizedPages = true;
  mockMenuCheckout = true;
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    await press(tree, '새 고객 주문');
    expect(mockMenuPages).toEqual(['0', '1']);
    expect(view(tree)).toContain('한정 메뉴');
    await press(tree, '내 장바구니');
    expect(view(tree)).toContain('담은 메뉴가 없습니다.');
    expect(mockSubmittedCarts).toHaveLength(0);
    await press(tree, '메뉴로 돌아가기');
    await press(tree, '크루아상 상세');
    await press(tree, '장바구니에 담기');
    const entry = tree.root.findAll(node => node.props.accessibilityLabel === '내 장바구니' &&
      node.props.accessibilityRole === 'button')[0];
    expect(entry).toBeDefined();
    for (let parent = entry.parent; parent; parent = parent.parent) {
      expect(parent.type).not.toBe('RCTScrollView');
    }
    expect(view(tree)).toContain('담은 수량 1');
    expect(view(tree)).toContain('3.00 TestUSDC');
    await press(tree, '메뉴 언어 English');
    await press(tree, 'My basket');
    expect(mockSubmittedCarts).toHaveLength(0);
    await press(tree, 'Menu language 한국어');
    expect(view(tree)).not.toContain('한정 메뉴');
    expect(view(tree)).toContain('크루아상 ×1');
    await press(tree, '메뉴로 돌아가기');
    expect(view(tree)).toContain('한정 메뉴');
    await press(tree, '내 장바구니');
    await press(tree, '하드월렛으로 결제');
    expect(mockSubmittedCarts).toHaveLength(1);
    expect(mockSubmittedCarts[0].items).toEqual([{sku:'z-croissant', option_key:'standard', quantity:1}]);
  } finally { await Renderer.act(async () => tree.unmount()); }
}, 20000);

test('K09 phone guidance keeps the existing public basket and uses the branch entrance QR', async () => {
  mockCategorizedPages = true;
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    await press(tree, '새 고객 주문');
    await press(tree, '차 standard 추가');
    await press(tree, '내 폰으로 주문');
    expect(view(tree)).toContain('개인 주문을 시작하는 매장 입구 QR');
    expect(view(tree)).toContain('여울 커피 성수점');
    expect(view(tree)).not.toContain(mockSale);
    expect(view(tree)).not.toContain('이 주문을 여는 일회용 QR');
    expect(mockSubmittedCarts).toHaveLength(0);
    await press(tree, '내 공용 장바구니로');
    expect(view(tree)).toContain('차 ×1');
  } finally { await Renderer.act(async () => tree.unmount()); }
});

test('K09 confirms an unreserved cancellation and keeps the basket when continuing', async () => {
  mockCategorizedPages = true;
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    await press(tree, '새 고객 주문');
    await press(tree, '차 standard 추가');
    await press(tree, '내 장바구니');
    await press(tree, '주문 그만하기');
    expect(view(tree)).toContain('공용 주문을 닫을까요?');
    await press(tree, '계속 주문하기');
    expect(view(tree)).toContain('차 ×1');
    await press(tree, '주문 그만하기');
    await press(tree, '닫고 화면 지우기');
    expect(mockSubmittedCarts).toHaveLength(0);
    expect(view(tree)).not.toContain('차 ×1');
    await press(tree, '새 고객 주문');
    expect(view(tree)).toContain('담은 수량 0');
  } finally { await Renderer.act(async () => tree.unmount()); }
});


test('K09 never resurfaces a preview after cancellation reply loss and retries its exact session', async () => {
  mockMenuCheckout = true;
  mockLoseCancelReply = true;
  let tree!: Renderer.ReactTestRenderer;
  const mount = async () => Renderer.act(async () => { tree = Renderer.create(<App />); });
  const close = async () => Renderer.act(async () => { tree.unmount(); });
  await mount();
  try {
    await press(tree, '새 고객 주문');
    await press(tree, '차 hot 추가');
    await press(tree, '하드월렛으로 결제');
    const scope = mockSubmittedCarts[0].originRef;
    await press(tree, '화면을 비우고 대기');
    expect(view(tree)).toContain('공용 주문을 닫을까요?');
    expect(mockPublicCancellations).toHaveLength(0);
    await press(tree, '닫고 화면 지우기');
    expect(mockPublicCancellations).toHaveLength(1);
    expect(mockPublicCancellations[0].session).toBe(scope);
    expect(view(tree)).not.toContain(mockSale);
    await close();
    await mount();
    await press(tree, '새 고객 주문');
    expect(mockPublicCancellations).toEqual([mockPublicCancellations[0],mockPublicCancellations[0]]);
    expect(mockSubmittedCarts).toHaveLength(1);
    expect(view(tree)).not.toContain(mockSale);
    expect(view(tree)).toContain('담은 수량 0');
  } finally { await close(); }
});

test('K05 an original checking order cannot open new board approval from an unsigned local checkpoint', async () => {
  mockRole = 'terminal';
  mockMenuCheckout = true;
  mockMenuPrice = '6500000';
  mockJournal.set('terminal-payment-v2', JSON.stringify({
    branch: mockShop,
    terminal: mockTerminal,
    order: {id: mockSale},
    request: 'a'.repeat(32),
  }));
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    expect(view(tree)).toContain('보류 지급 상태 확인');
    mockMenuOrderState = 'checking';
    const reads = mockDialed.length;
    await press(tree, '보류 지급 상태 확인');
    expect(view(tree)).toContain('보류 결과를 확인 중입니다.');
    expect(mockDialed.slice(reads)).toEqual(['order.status', 'order.status']);
    expect(mockJournal.get('terminal-payment-v2')).toContain(mockSale);

    await press(tree, '새 고객 주문');
    await press(tree, '차 hot 추가');
    await press(tree, '하드월렛으로 결제');
    expect(view(tree)).not.toContain(mockSale);
    expect(view(tree)).toContain('결제 확인 중입니다');
    expect(view(tree)).not.toContain('직접 승인할 보드 찾기');
    expect(view(tree)).not.toContain('인증 및 검토');
    expect(mockDialed).not.toContain('payment.forward');
  } finally {
    await Renderer.act(async () => tree.unmount());
  }
});

test('K02 a landscape tablet keeps the same basket and its checkout beside the menu', async () => {
  mockRole = 'terminal';
  mockCategorizedPages = true;
  mockMenuCheckout = true;
  const before = Dimensions.get('window');
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => Dimensions.set({ window: { ...before, width: 1280, height: 800 } }));
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    await press(tree, '새 고객 주문');
    expect(view(tree)).toContain('담은 메뉴가 없습니다.');
    expect(view(tree)).not.toContain('내 장바구니');
    await press(tree, '차 standard 추가');
    expect(view(tree)).toContain('한정 메뉴');
    expect(view(tree)).toContain('차 ×1');
    expect(view(tree)).toContain('합계 · 3.00 TestUSDC');
    await press(tree, '하드월렛으로 결제');
    expect(mockSubmittedCarts).toHaveLength(1);
    expect(view(tree)).toContain('이 주문을 여는 일회용 QR');
  } finally {
    await Renderer.act(async () => tree.unmount());
    await Renderer.act(async () => Dimensions.set({ window: before }));
  }
});

test.each(['refused by the server', 'known from the enrollment'] as const)('K11 tells staff that an escrow store cannot take a POS payment: %s', async how => {
  mockRole = 'terminal';
  mockMode = 'pos';
  mockPOSQuoteUnsupported = true;
  if (how === 'known from the enrollment') {
    mockMenuCheckout = true;
    mockAbi = orderEscrowAbiSha256;
  }
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  try {
    await press(tree, '새 고객 주문');
    await openPOS(tree, mockHandoff);
    if (how === 'refused by the server') {
      await press(tree, '휴대전화 확인 번호 조회');
      await press(tree, '확인 번호 2ba8cf04와 일치');
    }
    expect(view(tree)).toContain('POS 결제를 쓸 수 없는 매장입니다');
    expect(view(tree)).toContain('직원은 POS에서 다른 결제 수단을 안내하거나');
    expect(view(tree)).toContain('빠져나간 돈은 없습니다.');
    for (const hidden of ['이 주문을 여는 일회용 QR', '견적 기록 대기', 'UNSUPPORTED', '결합 결과를 확인하지 못했습니다'])
      expect(view(tree)).not.toContain(hidden);
    await press(tree, 'POS 원주문 다시 조회');
    expect(view(tree)).toContain('POS 결제를 쓸 수 없는 매장입니다');
    await press(tree, '다른 POS 원주문 열기');
    expect(view(tree)).toContain('직원은 POS에서 다른 결제 수단을 안내하거나');
    expect(mockDialed.filter(op => op === 'quote.issue').length).toBe(how === 'refused by the server' ? 1 : 0);
  } finally {
    await Renderer.act(async () => tree.unmount());
  }
});

describe('kiosk settings', () => {
  const genesis = '0x2bdf79b3d3cc49f9e6638ff81f3bb85065c79945a8fe4556cd0ff47bbfc02490';
  const asked: string[] = [];
  let served: { chain: string; hash: string } | undefined;
  const network = global.fetch;
  beforeEach(() => {
    asked.length = 0;
    served = { chain: '0x205b', hash: genesis };
    global.fetch = (async (url: string, init: { body: string }) => {
      asked.push(url);
      if (!served) throw new Error('no route');
      const { id, method } = JSON.parse(init.body);
      const result = method === 'eth_chainId' ? served.chain : method === 'eth_blockNumber' ? '0x10' : { number: '0x0', hash: served.hash };
      return { ok: true, json: async () => ({ jsonrpc: '2.0', id, result }) };
    }) as unknown as typeof fetch;
  });
  afterEach(() => {
    global.fetch = network;
  });
  async function open() {
    let tree: Renderer.ReactTestRenderer;
    await Renderer.act(async () => {
      tree = Renderer.create(<App />);
    });
    return tree!;
  }
  async function type(tree: Renderer.ReactTestRenderer, label: string, value: string) {
    const input = tree.root.findAll(n => n.props.accessibilityLabel === label && typeof n.props.onChangeText === 'function')[0];
    await Renderer.act(async () => input.props.onChangeText(value));
  }
  async function staffSettings() {
    mockRole = 'operator';
    const tree = await open();
    await press(tree, '직원 권한 확인');
    await press(tree, '설정');
    return tree;
  }

  test('anyone reads the store, the notices and a licence without a code in sight', async () => {
    mockRole = 'terminal';
    mockMenuCheckout = true;
    mockAbi = orderEscrowAbiSha256;
    const tree = await open();
    try {
      await press(tree, '설정');
      await Renderer.act(async () => undefined);
      expect(view(tree)).toContain('여울 커피 성수점');
      expect(view(tree)).toContain('주문 보호 계약이 보관합니다');
      expect(view(tree)).toContain('"1.0"');
      expect(view(tree)).not.toMatch(/\b[KW][0-9]{2}\b/);
      await press(tree, '이용약관');
      expect(view(tree)).toContain('검토 중인 초안입니다.');
      await press(tree, '뒤로');
      for (const name of ['개인정보 처리방침', '시험용 자산 안내']) {
        await press(tree, name);
        await press(tree, '뒤로');
      }
      await press(tree, '오픈소스 라이선스');
      // A long list renders only what is near the screen.
      const rows = tree.root.findAll(n => /^\S+ [0-9]+\.[0-9]\S* /.test(n.props.accessibilityLabel ?? '') && typeof n.props.onPress === 'function');
      expect(rows.length).toBeGreaterThan(5);
      expect(new Set(rows.map(n => n.props.accessibilityLabel)).size).toBeLessThan(60);
      await Renderer.act(async () => rows[0].props.onPress());
      expect(view(tree)).toMatch(/Permission is hereby granted|Licensed under|license/i);
      await press(tree, '뒤로');
      expect(tree.root.findAll(n => n.props.testID === 'license-list').length).toBeGreaterThan(0);
      await press(tree, '뒤로');
      await press(tree, 'English');
      expect(view(tree)).toContain('Terms of use');
      await press(tree, 'Close settings');
      expect(view(tree)).toContain('새 고객 주문');
    } finally {
      await Renderer.act(async () => tree.unmount());
    }
  });

  test('the customer terminal shows no staff setting and no payment value', async () => {
    mockRole = 'terminal';
    const tree = await open();
    try {
      await press(tree, '설정');
      await press(tree, '기기');
      expect(view(tree)).toContain('직원 등록 기기에서');
      expect(view(tree)).not.toContain('설정할 단말 식별값');
      await press(tree, '결제');
      expect(view(tree)).toContain('직원 등록 기기에서');
      expect(view(tree)).not.toContain('매장 수령 주소');
    } finally {
      await Renderer.act(async () => tree.unmount());
    }
  });

  test('settings close after a minute without a touch and never cover an order', async () => {
    mockRole = 'terminal';
    const tree = await open();
    fakeSessionTimers();
    try {
      await press(tree, '설정');
      const body = () => tree.root.findAll(n => n.props.testID === 'kiosk-settings' && typeof n.props.onStartShouldSetResponderCapture === 'function')[0];
      await Renderer.act(async () => jest.advanceTimersByTime(59_000));
      await Renderer.act(async () => body().props.onStartShouldSetResponderCapture());
      await Renderer.act(async () => jest.advanceTimersByTime(59_000));
      expect(body()).toBeDefined();
      await Renderer.act(async () => jest.advanceTimersByTime(1_000));
      expect(body()).toBeUndefined();
      await press(tree, '새 고객 주문');
      expect(tree.root.findAll(n => n.props.label === '설정')).toHaveLength(0);
      mockJournal.clear();
    } finally {
      jest.useRealTimers();
      await Renderer.act(async () => tree.unmount());
    }
  });

  test('an unresolved payment keeps settings unreachable', async () => {
    mockRole = 'terminal';
    mockJournal.set('terminal-payment-v2', JSON.stringify({ branch: mockShop, terminal: mockTerminal, request: 'a'.repeat(32) }));
    const tree = await open();
    try {
      expect(view(tree)).toContain('보류 지급 상태 확인');
      expect(tree.root.findAll(n => n.props.label === '설정')).toHaveLength(0);
    } finally {
      await Renderer.act(async () => tree.unmount());
    }
  });

  test('staff change the language and idle time through the server, which records them', async () => {
    const tree = await staffSettings();
    try {
      await press(tree, '기기');
      await type(tree, '설정할 단말 식별값', mockTerminal);
      await press(tree, '단말 설정 조회');
      await press(tree, 'English');
      await press(tree, '120초');
      expect(mockDeviceChanges).toEqual([[mockTerminal, 'language', 'en', '0'], [mockTerminal, 'idle-seconds', '120', '1']]);
      expect(view(tree)).toContain('운영 서버에 기록했습니다');
      expect(view(tree)).toContain('첫 화면 언어 · 한국어 → English');
      mockDeviceOffline = true;
      await press(tree, '30초');
      expect(mockDeviceChanges).toHaveLength(2);
      expect(view(tree)).toContain('확인하지 못했습니다');
    } finally {
      await Renderer.act(async () => tree.unmount());
    }
  });

  test('a read server is saved only after it shows the payment chain', async () => {
    const tree = await staffSettings();
    try {
      await press(tree, '기기');
      await type(tree, '설정할 단말 식별값', mockTerminal);
      await press(tree, '단말 설정 조회');
      const attempt = async (address: string) => {
        await type(tree, '읽기 서버 주소', address);
        await press(tree, '확인하고 저장');
      };
      await attempt('http://rpc.example');
      expect(asked).toEqual([]);
      expect(view(tree)).toContain('https로 시작하고');
      served = { chain: '0x1', hash: genesis };
      await attempt('https://rpc.example/');
      expect(view(tree)).toContain('결제 네트워크가 아닙니다');
      served = { chain: '0x205b', hash: '0x' + '1'.repeat(64) };
      await attempt('https://rpc.example/');
      expect(view(tree)).toContain('결제 네트워크가 아닙니다');
      served = undefined;
      await attempt('https://rpc.example/');
      expect(view(tree)).toContain('응답하지 않습니다');
      expect(mockDeviceChanges).toEqual([]);
      served = { chain: '0x205b', hash: genesis };
      await attempt('https://rpc.example/');
      expect(mockDeviceChanges).toEqual([[mockTerminal, 'read-server', 'https://rpc.example/', '0']]);
      await press(tree, '운영 서버 주소로 되돌리기');
      expect(mockDeviceChanges[1]).toEqual([mockTerminal, 'read-server', '', '1']);
    } finally {
      await Renderer.act(async () => tree.unmount());
    }
  });

  test('payment values are shown and cannot be edited', async () => {
    const tree = await staffSettings();
    try {
      await press(tree, '결제');
      await type(tree, '설정할 단말 식별값', mockTerminal);
      await press(tree, '단말 설정 조회');
      for (const value of [mockAsset, '0x' + '2'.repeat(40), '0x' + '1'.repeat(40), '0x' + '4'.repeat(40), '0x' + '5'.repeat(40), 'StableNet 테스트넷 (8283)'])
        expect(view(tree)).toContain(value);
      expect(tree.root.findAll(n => typeof n.props.onChangeText === 'function' && n.props.accessibilityLabel !== '설정할 단말 식별값')).toHaveLength(0);
    } finally {
      await Renderer.act(async () => tree.unmount());
    }
  });

  test('an operator without the permission cannot open the staff sections', async () => {
    mockStaffPermissions = ['pickup'];
    const tree = await staffSettings();
    try {
      await press(tree, '기기');
      expect(view(tree)).toContain('권한이 있는 직원만');
      await press(tree, '결제');
      expect(view(tree)).toContain('권한이 있는 직원만');
    } finally {
      await Renderer.act(async () => tree.unmount());
    }
  });

  test('the terminal starts each order with the recorded language and idle time', async () => {
    mockRole = 'terminal';
    mockDevice = { revision: 2, language: 'en', idleSeconds: 120 };
    const tree = await open();
    try {
      await press(tree, '새 고객 주문');
      expect(view(tree)).toContain('Menu language 한국어');
      expect(view(tree)).toContain('120초');
    } finally {
      await Renderer.act(async () => tree.unmount());
    }
  });
});
