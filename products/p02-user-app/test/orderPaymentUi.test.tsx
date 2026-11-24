import React from 'react';
import {AppState} from 'react-native';
import Renderer from 'react-test-renderer';
import {
  bindServiceRequestV2 as mockBind, decodeNearbyRequest as mockDecode, encodeOrderLocator, hexToBytes,
  packNearbyPresentation as mockPack, toBase64 as mockBase64, utf8Encode as mockUtf8, type CommerceOrder, type NearbyReply,
  type NearbyRequest, type ServiceRequestV2,
} from '@nu54/protocol';
import {customerDigest} from '../src/merchant/customerJournal';
import {CustomerOrders} from '../src/merchant/CustomerOrders';
import {
  boardSignature, branch as mockBranch, customer as mockCustomer, escrowStatus, generation, now, order, order as mockIssued, payer, policy, quote, quoteFor as mockQuoteFor,
  sale as mockSale, terminal as mockTerminal,
} from './fixtures/order';

// randomBytes below returns 7s, so the seed and with it the phone key of this account are fixed.
const account = {payer, generation: generation.toString()};
let confirmer = '';
beforeAll(async () => {
  const {orderRecords} = jest.requireActual<typeof import('../src/merchant/orderRecords')>('../src/merchant/orderRecords');
  const Http = jest.requireMock('../src/specs/NativeCommerceHttp.ts') as {default: Parameters<typeof orderRecords>[0]};
  confirmer = await orderRecords(Http.default).confirmer(account);
});
const mockProtect = jest.fn(async (_on: boolean) => {});
const mockProfile = {scope: {role: 'customer', branchRef: mockBranch, subjectRef: mockCustomer, tenantRef: mockBranch, expiresAt: now + 3600},
  pin: 'a'.repeat(64), payment: policy, nearby: {}};
const mockJournals = new Map<string, string>();
const mockLocks: (ServiceRequestV2 & {approvalSignature?: string})[] = [];
const mockNearby: NearbyRequest[] = [];
let mockOrder: CommerceOrder = order;
let mockOffline = false;
let mockClaimed = true;
let mockList: object[] = [];
let mockQuoteRefused = false;
let mockQuotes = 0;
let mockLockError = '';
let mockRelay: CommerceOrder | undefined;
let mockLockReply: () => NearbyReply = () => ({status: 'ok', value: {submitted: mockSale}});
let mockWallet: Record<string, unknown>;
let mockWrites: string[] = [];
const mockApprove = jest.fn<Promise<string>, [unknown, string, Uint8Array, () => void]>();
const mockStatus = jest.fn();

jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  loadProfile: async () => JSON.stringify(mockProfile),
  loadJournal: async (name: string) => mockJournals.get(name) ?? '',
  saveJournal: async (name: string, value: string) => { mockWrites.push(value); mockJournals.set(name, value); },
  randomBytes: async (length: number) => mockBase64(new Uint8Array(length).fill(7)),
  exchange: async (_role: string, raw: string) => {
    const {command, approvalSignature} = JSON.parse(raw) as {command: ServiceRequestV2; approvalSignature?: string};
    if (mockOffline) throw new Error('offline');
    const {requestId, requestDigest} = mockBind(command, {...mockProfile.scope,
      grants: [{domainRef: command.domainRef, objectRef: command.objectRef, operations: [`C3/${command.operation}`]}]}, Math.floor(Date.now() / 1000));
    const ok = {requestId, requestDigest, status: 'ok', resultRef: command.objectRef};
    if (command.operation === 'order.list') return JSON.stringify({...ok, orders: mockList});
    if (command.operation === 'invoice.list') return JSON.stringify({...ok, invoices: []});
    if (command.operation === 'order.status') return JSON.stringify({...ok, order: mockOrder});
    if (command.operation === 'checkout.claim') return JSON.stringify(!mockClaimed ? ok : {...ok, claim: {claimDigest: 'a'.repeat(64), expiresAt: Math.floor(Date.now() / 1000) + 60}});
    if (command.operation === 'checkout.request') return JSON.stringify({...ok, comparison: {code: '37ea19cd', expiresAt: Math.floor(Date.now() / 1000) + 60}});
    if (command.operation === 'quote.issue') {
      mockQuotes++;
      return JSON.stringify(mockQuoteRefused ? {...ok, status: 'error', error: 'UNSUPPORTED_CAPABILITY', resultRef: undefined} : {...ok, quote: mockQuoteFor()});
    }
    if (command.operation === 'order.lock') { mockLocks.push({...command, approvalSignature}); return JSON.stringify(mockLockError ? {...ok, status: 'error', error: mockLockError, resultRef: undefined} : ok); }
    throw new Error('unexpected ' + command.operation);
  },
}}));
let mockScan = '';
jest.mock('../src/specs/NativeShopQr.ts', () => ({__esModule: true, default: {scan: async () => mockScan}}));
const mockAsk = jest.fn(async (_ask: boolean) => true);
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({__esModule: true, default: {permission: (ask: boolean) => mockAsk(ask), clear: async () => {}, orderReady: async () => true}}));
jest.mock('../src/specs/NativeNearby.ts', () => ({__esModule: true, default: {}}));
jest.mock('../src/merchant/nearbyClient.ts', () => ({CustomerNearby: class {
  close() {}
  disconnect() {}
  async scan() { return [{handle: 'terminal'}]; }
  async connect() {
    return {peer: {principal: mockTerminal}, request: async (request: NearbyRequest): Promise<NearbyReply> => {
      mockDecode(mockUtf8(JSON.stringify(request)), mockBranch);
      mockNearby.push(request);
      if (request.op === 'discover') return {status: 'ok', value: {branch: mockBranch, terminal: mockTerminal, branchLabel: '그린 카페'}};
      if (request.op === 'catalog') return {status: 'ok', value: {rows: [{sku: 'latte', optionKey: 'iced', label: '라떼', asset: mockIssued.asset, version: 1,
        unitPrice: mockIssued.amount, available: 3, expiresAt: mockIssued.expiresAt}]}};
      if (request.op === 'reserve') return {status: 'ok', value: {...mockIssued, state: 'preview', revision: 0, quoteDigest: undefined, orderKey: undefined}};
      if (request.op === 'checkout') {
        const reserved = mockNearby.find(item => item.op === 'reserve') as Extract<NearbyRequest, {op: 'reserve'}>;
        const issued = mockQuoteFor(reserved.cart.originRef);
        return {status: 'ok', value: mockPack({...mockIssued, quoteDigest: issued.quoteDigest}, issued, mockBase64)};
      }
      if (request.op === 'status') return {status: 'ok', value: mockRelay ?? mockIssued};
      if (request.op === 'lock') return mockLockReply();
      throw new Error('unexpected ' + request.op);
    }};
  }
}}));
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({__esModule: true, default: {loadProfile: async () => '', setHomeProtection: (on: boolean) => mockProtect(on), evmRpc: async () => { throw new Error('offline'); }}}));
jest.mock('../src/specs/NativeRenterBle.ts', () => ({__esModule: true, default: {
  onScan: (receive: (raw: string) => void) => { receive(JSON.stringify({address: 'board-1', name: 'NU54DK'})); return {remove() {}}; },
  scanOwner: async () => {},
}}));
jest.mock('../src/wallet/profile.ts', () => ({decodeWalletStorage: () => mockWallet}));
jest.mock('../src/wallet/ownerSession.ts', () => ({OwnerSession: class {
  connect = async () => {};
  close = () => {};
  spendingStatus = mockStatus;
  approveOrder = mockApprove;
}}));

let tree: Renderer.ReactTestRenderer | undefined;
const screen = () => JSON.stringify(tree?.toJSON());
const button = (label: string) => tree?.root.findAll(node => ((node.props.label === label || node.props.testID === label) && typeof node.props.run === 'function') ||
  (node.props.title === label && typeof node.props.onPress === 'function'))[0];
async function press(label: string) {
  const found = button(label);
  expect([label, !!found, found?.props.disabled ?? false]).toEqual([label, true, false]);
  await Renderer.act(async () => { (found!.props.run ?? found!.props.onPress)(); });
}
async function open(props: Partial<React.ComponentProps<typeof CustomerOrders>> = {}) {
  await Renderer.act(async () => { tree = Renderer.create(<CustomerOrders back={() => {}} initialLocator="" permission={async () => true} {...props} />); });
}
const tick = (ms: number) => Renderer.act(async () => { await jest.advanceTimersByTimeAsync(ms); });
async function connectStore() {
  await press('근처 단말 찾기');
  await press('근거리 단말 후보 1 인증');
  await press('이 매장 메뉴 열기');
}
async function toConfirm() {
  await open({shopEntry: true});
  await connectStore();
  await press('라떼 iced 추가');
  await press('주문 접수 요청');
}
const saved = () => JSON.parse([...mockJournals].find(([name]) => /^customer-payment-/.test(name) && name !== 'customer-payment-v2')?.[1] ?? 'null');
const checkpoint = (change: object = {}) => ({version: 2, branch: mockBranch, customer: mockCustomer, order, quote, scope: {branch: mockBranch, customer: mockCustomer, sale: mockSale},
  request: 'c'.repeat(32), submitKey: mockSale, route: 'nearby', relayKey: mockSale, confirmer, store: '그린 카페', ...change});
async function resumeWith(change: object, withKey = true) {
  if (withKey) {
    const {orderRecords} = jest.requireActual<typeof import('../src/merchant/orderRecords')>('../src/merchant/orderRecords');
    const Http = jest.requireMock('../src/specs/NativeCommerceHttp.ts') as {default: Parameters<typeof orderRecords>[0]};
    expect(await orderRecords(Http.default).confirmer(account)).toBe(confirmer);
  }
  mockJournals.set('customer-payment-v2', JSON.stringify(checkpoint(change)));
  await open();
}

beforeEach(() => {
  jest.useFakeTimers({now: now * 1000});
  jest.spyOn(AppState, 'addEventListener').mockReturnValue({remove() {}});
  mockJournals.clear(); mockLocks.length = 0; mockNearby.length = 0; mockWrites = [];
  mockOrder = order; mockOffline = false; mockClaimed = true; mockRelay = undefined; mockList = []; mockQuoteRefused = false; mockQuotes = 0; mockLockError = '';
  mockLockReply = () => ({status: 'ok', value: {submitted: mockSale}});
  mockWallet = {mode: 'personal', enrollment: {device: 'pinned'}, verified: {network: '8283', address: payer, generation: generation.toString(), boardFingerprint: 'pinned'}};
  mockStatus.mockReset().mockResolvedValue({account: payer, perPayment: 30_000_000n, daily: 100_000_000n, pinWait: 0, orderOpen: false, leftToday: 95_500_000n});
  mockProtect.mockClear();
  mockAsk.mockClear();
  mockApprove.mockReset().mockImplementation(async (payment, phone, _request, onBoard) => {
    onBoard();
    const hex = Buffer.from((payment as {bytes: Uint8Array}).bytes).toString('hex');
    return boardSignature(phone, undefined, hex);
  });
});
afterEach(async () => {
  await Renderer.act(async () => { tree?.unmount(); });
  tree = undefined;
  jest.useRealTimers();
  jest.restoreAllMocks();
});

test('the store, items and amount are shown, the board is checked, and the owner can cancel while it waits', async () => {
  await toConfirm();
  expect(screen()).toContain('결제 내용을 확인하세요');
  expect(screen()).toContain('그린 카페');
  expect(screen()).toContain('4.50 TestUSDC');
  expect(screen()).toContain('라떼');
  expect(button('보드로 승인')).toBeUndefined();
  await press('내 보드 확인');
  expect(screen()).toContain('30.00 TestUSDC');
  expect(screen()).toContain('오늘 남은 한도');
  expect(screen()).toContain('95.50 TestUSDC');
  expect(mockApprove).not.toHaveBeenCalled();
  const waiting = new Promise<string>(() => {});
  mockApprove.mockImplementationOnce(async (_payment, _phone, _request, onBoard) => { onBoard(); return waiting; });
  await press('보드로 승인');
  expect(screen()).toContain('보드에서 승인해 주세요');
  expect(screen()).toContain('SW1');
  expect(screen()).toContain('PIN');
  expect(screen()).not.toContain('독립 표시');
  await press('승인 취소');
  expect(screen()).toContain('결제를 취소했습니다');
  expect(mockNearby.some(item => item.op === 'lock')).toBe(false);
});

test('the signature is written down before the lock leaves, then the number comes from the kiosk', async () => {
  let atLock: {authorization?: string; forwardAttempted?: true; confirmer?: string; quote?: {quoteDigest: string}} = {};
  mockLockReply = () => { atLock = saved(); return {status: 'ok', value: {submitted: mockSale}}; };
  await toConfirm();
  await press('내 보드 확인');
  await press('보드로 승인');
  const lock = mockNearby.find(item => item.op === 'lock') as Extract<NearbyRequest, {op: 'lock'}>;
  expect(atLock).toMatchObject({authorization: lock.signature, forwardAttempted: true, confirmer, quote: {quoteDigest: lock.quoteDigest}});
  expect(lock).toMatchObject({order: mockSale, confirmer, signature: expect.stringMatching(/^[0-9a-f]{128}(1b|1c)$/)});
  expect(mockWrites.filter(value => value.includes('"seed":"' + '07'.repeat(32)))).toHaveLength(1);
  expect(screen()).toContain('결제를 보내고 있습니다');
  mockOffline = true;
  mockRelay = {...order, state: 'checking', escrow: escrowStatus('received', confirmer), payment: {stage: 'received', chainId: 8283}};
  await tick(2000);
  expect(screen()).not.toContain('주문번호 17');
  mockRelay = {...order, state: 'paid', pickupNo: 17, escrow: escrowStatus('locked', confirmer)};
  await tick(6000);
  expect(screen()).toContain('주문이 접수되었습니다');
  expect(screen()).toContain('그린 카페 주문번호 17');
  expect(saved()).toBeNull();
  expect(mockApprove).toHaveBeenCalledTimes(1);
  expect(mockAsk).not.toHaveBeenCalled(); // the dialog would end the kiosk connection
  const rows = JSON.parse([...mockJournals.values()].find(value => value.includes('"sale"') && value.startsWith('[{'))!);
  expect(rows).toMatchObject([{sale: mockSale, confirmer, stage: 'locked', pickupNo: 17}]);
  await press('확인');
  expect(screen()).not.toContain('주문이 접수되었습니다');
  expect(mockAsk.mock.calls).toEqual([[true]]); // once, after the first escrow order
});

test('a kiosk cannot claim that nothing was paid; the server can', async () => {
  await toConfirm();
  await press('내 보드 확인');
  await press('보드로 승인');
  mockOffline = true;
  mockRelay = {...order, state: 'closed', escrow: escrowStatus('failed', confirmer), payment: {stage: 'unpaid', chainId: 8283}};
  await tick(8000);
  expect(screen()).not.toContain('결제되지 않았습니다');
  expect(saved().forwardAttempted).toBe(true);
  mockOffline = false;
  mockOrder = {...mockRelay, quoteDigest: undefined, orderKey: undefined};
  await tick(6000);
  expect(screen()).toContain('결제되지 않았습니다');
  expect(screen()).toContain('빠져나간 돈이 없습니다');
  expect(saved()).toBeNull();
});

test('a status for another phone key or payer never settles the order', async () => {
  await toConfirm();
  await press('내 보드 확인');
  await press('보드로 승인');
  mockOffline = true;
  for (const escrow of [escrowStatus('locked', '0x' + '9'.repeat(40)), {...escrowStatus('locked', confirmer), payer: '0x' + '9'.repeat(40)},
    {...escrowStatus('locked', confirmer), escrow: '0x' + '9'.repeat(40)}]) {
    mockRelay = {...order, state: 'paid', pickupNo: 17, escrow};
    await tick(6000);
    expect(screen()).not.toContain('주문번호 17');
  }
  expect(screen()).toContain('다시 결제하지 마세요');
});

test('a lost lock reply stays unknown, repeats the identical lock and never asks the board again', async () => {
  mockLockReply = () => { throw new Error('NEARBY_DISCONNECTED'); };
  await toConfirm();
  await press('내 보드 확인');
  await press('보드로 승인');
  expect(screen()).toContain('결제 결과를 확인하고 있습니다');
  expect(screen()).toContain('다시 결제하지 마세요');
  expect(button('보드로 승인')).toBeUndefined();
  mockLockReply = () => ({status: 'ok', value: {submitted: mockSale}});
  await press('같은 결제 다시 보내기');
  const locks = mockNearby.filter(item => item.op === 'lock');
  expect(locks).toHaveLength(2);
  expect(locks[1]).toEqual(locks[0]);
  expect(mockApprove).toHaveBeenCalledTimes(1);
  expect(saved().forwardAccepted).toBe(true);
});

test('the payment page is kept out of screenshots and says so when it cannot be', async () => {
  mockProtect.mockRejectedValueOnce(new Error('no window'));
  await toConfirm();
  expect(mockProtect).toHaveBeenCalledWith(true);
  expect(screen()).not.toContain('4.50 TestUSDC');
  expect(screen()).toContain('결제 화면 보호를 시작하지 못했습니다');
  await press('다시 시도');
  expect(screen()).toContain('4.50 TestUSDC');
  await Renderer.act(async () => { tree?.unmount(); });
  expect(mockProtect).toHaveBeenLastCalledWith(false);
});

describe('a new order after one that was not paid', () => {
  const afterRefusal = async () => {
    mockApprove.mockRejectedValueOnce(new Error('PAYMENT_REFUSED'));
    await toConfirm();
    await press('내 보드 확인');
    await press('보드로 승인');
  };
  test('starts at once from the refusal, with the same choice', async () => {
    await afterRefusal();
    await press('새로 주문하기');
    expect(saved()).toBeNull();
    expect(button('주문 접수 요청')).toBeDefined();
    await press('주문 접수 요청');
    const [first, second] = mockNearby.filter(item => item.op === 'reserve') as {key: string}[];
    expect([!!first, !!second]).toEqual([true, true]);
  });
  test('starts after the page was closed and the quote ended, also after a restart', async () => {
    await afterRefusal();
    await press('결제 화면 닫기');
    expect(button('새로 주문하기')).toBeDefined();
    await tick((quote.expiresAt - now) * 1000 + 1000);
    await Renderer.act(async () => { tree?.unmount(); });
    await open({shopEntry: true});
    await connectStore();
    await press('새로 주문하기');
    expect(button('새로 주문하기')).toBeUndefined();
    expect(button('원래 요청 재조회')).toBeUndefined();
  });
  test('waits while a signature may have left', async () => {
    await toConfirm();
    await press('내 보드 확인');
    await press('보드로 승인');
    expect(mockNearby.some(item => item.op === 'lock')).toBe(true);
    expect(button('새로 주문하기')).toBeUndefined();
    await Renderer.act(async () => { tree?.unmount(); });
    // Without the payment journal the order record alone still blocks it.
    [...mockJournals.keys()].filter(name => /^customer-payment-/.test(name)).forEach(name => mockJournals.set(name, ''));
    await open({shopEntry: true});
    await connectStore();
    expect(button('원래 요청 재조회')).toBeDefined();
    expect(button('새로 주문하기')).toBeUndefined();
  });
  test('is refused when the order records cannot be read', async () => {
    await afterRefusal();
    mockJournals.set(`customer-order-${customerDigest('nu54-open-orders-v1')}`, '{');
    await press('새로 주문하기');
    expect(saved()).toMatchObject({reviewStop: 'refused'});
    expect(screen()).toContain('새로 주문할 수 없습니다');
  });
});

test('a lock the server already holds under another answer is only looked up', async () => {
  mockLockError = 'REPLAY_CONFLICT';
  await resumeWith({route: 'direct', relayKey: undefined, boardStarted: true, authorization: boardSignature(confirmer)});
  await press('같은 결제 보내기');
  expect(mockLocks).toHaveLength(1);
  expect(saved()).toMatchObject({forwardAttempted: true});
  expect(screen()).toContain('다시 결제하지 마세요');
});

test('a refused order is not asked again under a new request; the page waits for its quote to end', async () => {
  mockApprove.mockRejectedValue(new Error('PAYMENT_REFUSED'));
  await toConfirm();
  await press('내 보드 확인');
  await press('보드로 승인');
  expect(screen()).toContain('다시 확인할 때까지');
  expect(button('결제 내용 다시 받기')!.props.disabled).toBe(true);
  // Leaving the page and coming back through the same order meets the same answer.
  await press('결제 화면 닫기');
  await press('원래 요청 재조회');
  expect(screen()).toContain('보드가 이미 답한 결제입니다');
  expect(button('내 보드 확인')).toBeUndefined();
  expect(mockApprove).toHaveBeenCalledTimes(1);
  expect(screen()).not.toContain('0:00');
});

test('cancelling while the board waits keeps the one request id for the same quote', async () => {
  mockApprove.mockImplementationOnce(async (_payment, _phone, _request, onBoard) => { onBoard(); return new Promise<string>(() => {}); });
  await toConfirm();
  await press('내 보드 확인');
  await press('보드로 승인');
  await press('승인 취소');
  expect(screen()).not.toContain('오늘 남은 한도가');
  await Renderer.act(async () => { tree?.unmount(); });
  await open({shopEntry: true});
  await press('결제 화면 닫기');
  await connectStore();
  await press('원래 요청 재조회');
  expect(screen()).toContain('보드 승인을 이어서 확인합니다');
  await press('내 보드 확인');
  await press('보드로 승인');
  expect(mockApprove.mock.calls[1][2]).toEqual(mockApprove.mock.calls[0][2]);
});

test('cancelling after the board approved says that today is used and does not ask again', async () => {
  await resumeWith({boardStarted: true, authorization: boardSignature(confirmer)});
  await press('결제 취소');
  expect(screen()).toContain('오늘 남은 한도가 4.50 TestUSDC만큼 줄었습니다');
  expect(button('결제 내용 다시 받기')!.props.disabled).toBe(true);
  expect(saved()).toMatchObject({boardSigned: true, reviewStop: 'cancelled'});
  expect(saved().authorization).toBeUndefined();
  expect(mockLocks).toHaveLength(0);
});

test('a refusal on the board stops the payment without sending anything', async () => {
  mockApprove.mockRejectedValue(new Error('PAYMENT_REFUSED'));
  await toConfirm();
  await press('내 보드 확인');
  await press('보드로 승인');
  expect(screen()).toContain('보드에서 거절했습니다');
  expect(saved()).toMatchObject({reviewStop: 'refused'});
  expect(mockNearby.some(item => item.op === 'lock')).toBe(false);
});

test('a dropped board connection keeps the same request and says it can be resumed', async () => {
  mockApprove.mockRejectedValueOnce(new Error('OWNER_DISCONNECTED'));
  await toConfirm();
  await press('내 보드 확인');
  await press('보드로 승인');
  expect(screen()).toContain('보드 연결이 끊겼습니다');
  expect(screen()).toContain('두 번 결제되지 않습니다');
  await press('보드로 승인');
  const [first, second] = mockApprove.mock.calls;
  expect(second[2]).toEqual(first[2]);
  expect(second[1]).toBe(first[1]);
  expect(mockNearby.filter(item => item.op === 'lock')).toHaveLength(1);
});

test('an answer that fails verification is never sent', async () => {
  mockApprove.mockRejectedValue(new Error('ORDER_LOCK_MISMATCH'));
  await toConfirm();
  await press('내 보드 확인');
  await press('보드로 승인');
  expect(screen()).toContain('결제를 보내지 않았습니다');
  expect(saved().authorization).toBeUndefined();
  expect(mockNearby.some(item => item.op === 'lock')).toBe(false);
});

test.each([
  ['over the board limit', {perPayment: 4_000_000n}, '보드 한도를 넘는 금액입니다'],
  ['over what is left today', {leftToday: 4_000_000n}, '오늘 남은 한도를 넘는 금액입니다'],
  ['in a PIN wait', {pinWait: 90}, '약 2분 남았습니다'],
  ['another account', {account: '0x' + '9'.repeat(40)}, '내 보드 계정의 주문이 아닙니다'],
])('the board is not asked when it is %s', async (_name, change, text) => {
  mockStatus.mockResolvedValue({account: payer, perPayment: 30_000_000n, daily: 100_000_000n, pinWait: 0, orderOpen: false, leftToday: 95_500_000n, ...change});
  await toConfirm();
  await press('내 보드 확인');
  expect(screen()).toContain(text);
  expect(button('보드로 승인')).toBeUndefined();
  expect(mockApprove).not.toHaveBeenCalled();
});

test('a board that cannot answer is explained with a way to look again', async () => {
  mockStatus.mockRejectedValueOnce(new Error('OWNER_DISCONNECTED'));
  await toConfirm();
  await press('내 보드 확인');
  expect(screen()).toContain('보드가 아직 결제할 수 없습니다');
  await press('보드 다시 찾기');
  expect(button('보드로 승인')).toBeDefined();
});

test('an order made for another account or without a board is blocked before any request', async () => {
  mockWallet = {...mockWallet, verified: {network: '8283', address: '0x' + '9'.repeat(40), generation: generation.toString(), boardFingerprint: 'pinned'}};
  await toConfirm();
  expect(screen()).toContain('내 보드 계정의 주문이 아닙니다');
  expect(saved()).toBeNull();
  await Renderer.act(async () => { tree?.unmount(); });
  mockNearby.length = 0; mockJournals.clear();
  mockWallet = {mode: 'legacy'};
  const openBoard = jest.fn();
  await open({shopEntry: true, openBoard});
  await connectStore();
  await press('라떼 iced 추가');
  await press('주문 접수 요청');
  expect(screen()).toContain('연결된 보드가 없습니다');
  await press('보드 연결하기');
  expect(openBoard).toHaveBeenCalledTimes(1);
});

test('a quote that runs out before approval offers a new one', async () => {
  await toConfirm();
  await tick((quote.expiresAt - now) * 1000 + 1000);
  expect(screen()).toContain('결제 기한이 지났습니다');
  expect(button('보드로 승인')).toBeUndefined();
  expect(button('결제 내용 다시 받기')).toBeDefined();
});

describe('after a restart', () => {
  test('an approval in flight continues with the same request', async () => {
    await resumeWith({boardStarted: true});
    expect(screen()).toContain('보드 승인을 이어서 확인합니다');
    await press('내 보드 확인');
    await press('보드로 승인');
    expect(mockApprove.mock.calls[0][2]).toEqual(hexToBytes('c'.repeat(32)));
    expect(mockLocks).toHaveLength(1);
    expect(mockLocks[0]).toMatchObject({operation: 'order.lock', body: {confirmer, quoteDigest: quote.quoteDigest}, approvalSignature: boardSignature(confirmer)});
  });
  test('a signature that was not sent is sent without the board', async () => {
    await resumeWith({boardStarted: true, authorization: boardSignature(confirmer)});
    expect(screen()).toContain('승인한 결제를 아직 보내지 못했습니다');
    await press('같은 결제 보내기');
    expect(mockLocks).toHaveLength(1);
    expect(mockApprove).not.toHaveBeenCalled();
    expect(saved()).toMatchObject({forwardAttempted: true, forwardAccepted: true});
  });
  test('a sent signature is only looked up', async () => {
    await resumeWith({boardStarted: true, authorization: boardSignature(confirmer), forwardAttempted: true});
    expect(screen()).toContain('다시 결제하지 마세요');
    mockOrder = {...order, quoteDigest: undefined, orderKey: undefined, receiptRef: mockTerminal, state: 'paid', pickupNo: 4, escrow: escrowStatus('locked', confirmer),
      payment: {stage: 'finalized', chainId: 8283, txHash: '0x' + '1'.repeat(64), includedAt: new Date(now * 1000).toISOString()}};
    await press('결과 다시 확인');
    expect(screen()).toContain('그린 카페 주문번호 4');
    expect(mockLocks).toHaveLength(0);
    expect(mockApprove).not.toHaveBeenCalled();
  });
  test('an unsent signature is dropped once its quote has ended', async () => {
    jest.setSystemTime((quote.expiresAt + 1) * 1000);
    await resumeWith({boardStarted: true, authorization: boardSignature(confirmer)});
    expect(screen()).toContain('결제 기한이 지났습니다');
    await press('결제 화면 닫기');
    expect(saved()).toBeNull();
    expect(mockLocks).toHaveLength(0);
  });
  test('a checkpoint whose phone key was changed is never submitted', async () => {
    await resumeWith({boardStarted: true, authorization: boardSignature(confirmer), confirmer: '0x' + '8'.repeat(40)});
    await press('같은 결제 보내기');
    expect(mockLocks).toHaveLength(0);
    expect(screen()).toContain('주문 확인 키를 찾을 수 없습니다');
  });
  test('a lost phone key stops the order and is not replaced', async () => {
    await resumeWith({boardStarted: true}, false);
    await press('내 보드 확인');
    await press('보드로 승인');
    expect(mockApprove).not.toHaveBeenCalled();
    expect(screen()).toContain('주문 확인 키를 찾을 수 없습니다');
    expect([...mockJournals.values()].some(value => value.includes('"seed"'))).toBe(false);
  });
  test('a corrupt checkpoint locks payments instead of guessing', async () => {
    await resumeWith({authorization: 'not a signature'});
    expect(screen()).toContain('보호된 주문 등록을 열 수 없습니다');
  });
  test('closing the page keeps the payment and lists it as in progress', async () => {
    await resumeWith({boardStarted: true, authorization: boardSignature(confirmer), forwardAttempted: true});
    await press('결제 화면 닫기');
    expect(screen()).toContain('진행 중');
    expect(saved().forwardAttempted).toBe(true);
    await press('진행 중인 결제 이어서 보기');
    expect(screen()).toContain('다시 결제하지 마세요');
  });
});

describe('the code on the kiosk', () => {
  const locator = () => encodeOrderLocator({branchRef: mockBranch, orderRef: mockSale, expiresAt: now + 50}, now);
  test('the camera tells an order from a store entrance', async () => {
    const openPayments = jest.fn(), openShopQr = jest.fn();
    await open({openPayments, openShopQr, tabbed: true, openSettings() {}});
    mockScan = locator();
    await press('키오스크 QR 스캔');
    expect(openPayments).toHaveBeenCalledWith(mockScan);
    mockScan = 'nu54://shop/' + mockBranch;
    await press('키오스크 QR 스캔');
    expect(openShopQr).toHaveBeenCalledWith(mockScan);
  });

  test('the review opens by itself once the store confirms the number, and the link is forgotten when it is done', async () => {
    mockOrder = {...order, cafeReview: {branchLabel: '그린 카페', payerBound: true}};
    mockClaimed = false;
    await open({initialCounterQr: locator(), openPayments() {}, tabbed: true, openSettings() {}});
    await press('QR 주문 연결');
    expect(screen()).toContain('37ea19cd');
    await tick(3100);
    expect(screen()).not.toContain('결제 내용을 확인하세요');
    mockClaimed = true;
    await tick(3100);
    expect(screen()).toContain('결제 내용을 확인하세요');
    const kept = () => [...mockJournals].filter(([name, value]) => /^customer-qr-/.test(name) && value && value !== 'null');
    expect(kept()).toHaveLength(1);
    await tick(200_000);
    await press('결제 화면 닫기');
    await tick(0);
    expect(kept()).toHaveLength(0);
  });

  test('a link whose order is already over is dropped without opening it', async () => {
    mockOrder = {...order, state: 'closed', quoteDigest: undefined, orderKey: undefined};
    mockJournals.set('customer-qr-v2', JSON.stringify({order: mockSale, key: mockSale, code: '37ea19cd', branch: mockBranch, subject: mockCustomer}));
    await open({openPayments() {}, tabbed: true, openSettings() {}});
    await tick(3100);
    expect(button('연결 화면 닫기')).toBeUndefined();
    expect(button('키오스크 QR 스캔')).toBeDefined();
  });
});

describe('what an escrow store takes', () => {
  async function openDirect(view: CommerceOrder, props: Partial<React.ComponentProps<typeof CustomerOrders>> = {}) {
    mockOrder = view;
    await open(props);
    await press('매장 주문 ' + mockSale);
  }
  test('an owner-paid cafe order over HTTPS is locked into the escrow', async () => {
    mockList = [{id: mockSale, amount: order.amount, asset: order.asset, state: 'issued'}];
    await openDirect({...order, cafeReview: {branchLabel: '그린 카페', payerBound: true}});
    await press('이 주문의 결제 내용 검토');
    expect(screen()).toContain('결제 내용을 확인하세요');
    expect(screen()).toContain('그린 카페');
    await press('내 보드 확인');
    await press('보드로 승인');
    expect(mockLocks).toHaveLength(1);
    expect(mockLocks[0].body.confirmer).toBe(confirmer);
  });
  test('a subscription or rented-payer order is stopped before the board', async () => {
    mockList = [{id: mockSale, amount: order.amount, asset: order.asset, state: 'issued'}];
    mockQuoteRefused = true;
    await openDirect({...order, cafeReview: {branchLabel: '그린 카페', payerBound: true}});
    await press('이 주문의 결제 내용 검토');
    expect(screen()).toContain('이 매장에서는 아직');
    expect(screen()).not.toContain('결제 기한');
    expect(button('내 보드 확인')).toBeUndefined();
    await press('결제 취소');
    expect(screen()).not.toContain('이 매장에서는 아직');
  });
  test('a travel booking payment is stopped before a quote is asked for', async () => {
    const booking = {id: mockTerminal, slot: mockTerminal, partySize: 1, state: 'pending_payment' as const, expiresAt: order.expiresAt, asset: order.asset,
      deposit: order.amount, balance: '20000000', termsVersion: 1, priceVersion: 1, chainId: 8283, cancellation: '', noShow: '', cancellationDeadline: order.expiresAt};
    await open({travelCheckout: {branch: mockBranch, sale: mockSale, bookingPayment: {scope: {branch: mockBranch, customer: mockCustomer, tenant: mockBranch}, kind: 'deposit', booking}}});
    await press('이 원주문의 서명 견적 요청');
    expect(screen()).toContain('이 매장에서는 아직');
    expect(mockQuotes).toBe(0);
  });
  test('the order list shows the store, the amount and the time, never the order id', async () => {
    mockList = [{id: mockSale, amount: order.amount, asset: order.asset, state: 'issued'}];
    await open();
    const row = button('매장 주문 ' + mockSale);
    expect(row!.props.label).toBe('매장 주문 · 4.50 TestUSDC');
    const shown = tree!.root.findAll(node => typeof node.props.children === 'string').map(node => node.props.children).join('|');
    expect(shown).not.toContain(mockSale);
  });
});
