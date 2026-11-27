import React from 'react';
import {AppState} from 'react-native';
import Renderer from 'react-test-renderer';
import {
  bindServiceRequestV2 as mockBind, toBase64 as mockBase64, type CommerceCart, type CommerceOrder, type ServiceRequestV2,
} from '@nu54/protocol';
import {CustomerOrders} from '../src/merchant/CustomerOrders';
import {orderRecords} from '../src/merchant/orderRecords';
import {
  boardSignature, branch as mockBranch, customer as mockCustomer, generation, now, payer, order as mockIssued, policy as mockPolicy, quoteFor as mockQuoteFor,
} from './fixtures/order';

// The phone pays an escrow order over the server route; what it keeps for the receipt is read back from its records.
const mockProfile = {scope: {role: 'customer', branchRef: mockBranch, subjectRef: mockCustomer, tenantRef: mockBranch, expiresAt: now + 3600},
  pin: 'a'.repeat(64), payment: mockPolicy};
const mockJournals = new Map<string, string>();
const mockCarts: CommerceCart[] = [];
const mockLocks: ServiceRequestV2[] = [];
let mockState: unknown;
let mockOrder: CommerceOrder = mockIssued;
const mockApprove = jest.fn<Promise<string>, [unknown, string, Uint8Array, () => void]>();
const mockStatus = jest.fn();

jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  loadProfile: async () => JSON.stringify(mockProfile),
  loadJournal: async (name: string) => mockJournals.get(name) ?? '',
  saveJournal: async (name: string, value: string) => { mockJournals.set(name, value); },
  randomBytes: async (length: number) => mockBase64(new Uint8Array(length).fill(7)),
  exchange: async (_role: string, raw: string) => {
    const {command, cart} = JSON.parse(raw) as {command: ServiceRequestV2; cart?: CommerceCart};
    const {requestId, requestDigest} = mockBind(command, {...mockProfile.scope,
      grants: [{domainRef: command.domainRef, objectRef: command.objectRef, operations: [`C3/${command.operation}`]}]}, Math.floor(Date.now() / 1000));
    const ok = {requestId, requestDigest, status: 'ok', resultRef: command.objectRef};
    if (command.operation === 'order.list') return JSON.stringify({...ok, orders: []});
    if (command.operation === 'invoice.list') return JSON.stringify({...ok, invoices: []});
    if (command.operation === 'catalog.read') return JSON.stringify({...ok, catalogVersion: 1,
      catalog: [{sku: 'cookie', optionKey: 'standard', label: '쿠키', asset: mockPolicy.asset, version: 1, unitPrice: mockIssued.amount, available: 3,
        expiresAt: new Date((Math.floor(Date.now() / 1000) + 120) * 1000).toISOString(),
        metadata: {tax: 'vat10', takeAway: true, taxRefund: true}}],
      shop: {branch: mockBranch, branchLabel: '그린 카페'}, ...(mockState ? {orderState: mockState} : {})});
    if (command.operation === 'order.create') { mockCarts.push(cart!); return JSON.stringify({...ok, resultRef: mockIssued.id}); }
    if (command.operation === 'order.status') return JSON.stringify({...ok, order: mockOrder});
    if (command.operation === 'checkout.claim') return JSON.stringify({...ok, claim: {claimDigest: 'a'.repeat(64), expiresAt: Math.floor(Date.now() / 1000) + 60}});
    if (command.operation === 'quote.issue') return JSON.stringify({...ok, quote: mockQuoteFor(mockCarts[0]?.originRef)});
    if (command.operation === 'order.lock') { mockLocks.push(command); return JSON.stringify(ok); }
    throw new Error('unexpected ' + command.operation);
  },
}}));
jest.mock('../src/specs/NativeShopQr.ts', () => ({__esModule: true, default: {scan: async () => ''}}));
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({__esModule: true, default: {permission: async () => true, clear: async () => {}, orderReady: async () => true}}));
jest.mock('../src/specs/NativeNearby.ts', () => ({__esModule: true, default: {}}));
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({__esModule: true, default: {loadProfile: async () => '', setHomeProtection: async () => {}, evmRpc: async () => { throw new Error('offline'); }}}));
jest.mock('../src/specs/NativeRenterBle.ts', () => ({__esModule: true, default: {
  onScan: (receive: (raw: string) => void) => { receive(JSON.stringify({address: 'board-1', name: 'NU54DK'})); return {remove() {}}; },
  scanOwner: async () => {},
}}));
jest.mock('../src/wallet/profile.ts', () => ({decodeWalletStorage: () => mockWallet}));
let mockWallet: Record<string, unknown> = {};
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
  if (!found) throw new Error(`no button ${label} in ${screen()}`);
  expect([label, found.props.disabled ?? false]).toEqual([label, false]);
  await Renderer.act(async () => { (found.props.run ?? found.props.onPress)(); });
}
/** Orders one cookie online, approves it on the board and returns what the phone kept for that order. */
async function payOneCookie() {
  await Renderer.act(async () => { tree = Renderer.create(<CustomerOrders back={() => {}} initialLocator="" shopEntry permission={async () => true} />); });
  await press('온라인으로 매장 확인');
  await press('이 매장 메뉴 열기');
  await press('쿠키 추가');
  await press('주문 접수 요청');
  await press('이 주문의 결제 내용 검토');
  await press('내 보드 확인');
  await press('보드로 승인');
  expect(mockLocks).toHaveLength(1);
  const Http = jest.requireMock('../src/specs/NativeCommerceHttp.ts') as {default: Parameters<typeof orderRecords>[0]};
  return (await orderRecords(Http.default).list()).find(row => row.sale === mockIssued.id);
}

beforeEach(() => {
  jest.useFakeTimers({now: now * 1000});
  jest.spyOn(AppState, 'addEventListener').mockReturnValue({remove() {}});
  mockJournals.clear(); mockCarts.length = 0; mockLocks.length = 0;
  mockState = undefined;
  mockOrder = mockIssued;
  mockWallet = {mode: 'personal', enrollment: {device: 'pinned'}, verified: {network: '8283', address: payer, generation: generation.toString(), boardFingerprint: 'pinned'}};
  mockStatus.mockReset().mockResolvedValue({account: payer, perPayment: 30_000_000n, daily: 100_000_000n, pinWait: 0, orderOpen: false, leftToday: 95_500_000n});
  mockApprove.mockReset().mockImplementation(async (payment, phone, _request, onBoard) => {
    onBoard();
    return boardSignature(phone, undefined, Buffer.from((payment as {bytes: Uint8Array}).bytes).toString('hex'));
  });
});
afterEach(async () => {
  await Renderer.act(async () => { tree?.unmount(); });
  tree = undefined;
  jest.useRealTimers();
  jest.restoreAllMocks();
});

test('a designated tax-free shop: the kept receipt line is marked for refund and the shop number is kept', async () => {
  mockState = {mode: 'open', taxFreeShop: '123-45-67890-001'};
  const kept = await payOneCookie();
  expect(mockCarts[0].items).toEqual([{sku: 'cookie', option_key: 'standard', quantity: 1}]);
  expect(kept).toMatchObject({taxFreeShop: '123-45-67890-001',
    items: [{name: '쿠키', quantity: 1, total: mockIssued.amount, taxKind: 'vat10', refund: true}]});
});

test('the same goods at a shop without a designation keep no refund', async () => {
  mockState = {mode: 'open'};
  const kept = await payOneCookie();
  expect(kept?.items).toEqual([expect.objectContaining({name: '쿠키', taxKind: 'vat10'})]);
  expect(kept?.items?.[0]).not.toHaveProperty('refund');
  expect(kept).not.toHaveProperty('taxFreeShop');
});
