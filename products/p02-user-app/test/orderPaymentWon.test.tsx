import React from 'react';
import {AppState} from 'react-native';
import Renderer from 'react-test-renderer';
import {
  bindServiceRequestV2 as mockBind, bytesToHex, decodeNearbyRequest as mockDecode, dollarToken, hexToBytes, namedTokenAbiSha256, orderLockFor, quoteV2Layout, signDigest,
  packNearbyPresentation as mockPack, toBase64 as mockBase64, utf8Encode as mockUtf8, type CommerceOrder, type NearbyReply,
  type NearbyRequest, type ServiceRequestV2,
} from '@nu54/protocol';
import {CustomerOrders} from '../src/merchant/CustomerOrders';
import {
  branch as mockBranch, customer as mockCustomer, generation, now, order, order as mockIssued, payer, payerKey, policy, quoteFor as mockQuoteFor,
  sale as mockSale, terminal as mockTerminal,
} from './fixtures/order';

const mockProtect = jest.fn(async (_on: boolean) => {});
const mockProfile = {scope: {role: 'customer', branchRef: mockBranch, subjectRef: mockCustomer, tenantRef: mockBranch, expiresAt: now + 3600},
  pin: 'a'.repeat(64), payment: {...policy, abiSha256: namedTokenAbiSha256, token: {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2}}, nearby: {}};
const mockJournals = new Map<string, string>();
const mockLocks: (ServiceRequestV2 & {approvalSignature?: string})[] = [];
const mockNearby: NearbyRequest[] = [];
let mockOrder: CommerceOrder = order;
let mockOffline = false;
let mockClaimed = true;
let mockList: object[] = [];
let mockQuoteRefused = false;
let mockLockError = '';
let mockRelay: CommerceOrder | undefined;
let mockLockReply: () => NearbyReply = () => ({status: 'ok', value: {submitted: mockSale}});
let mockWallet: Record<string, unknown>;
let mockWrites: string[] = [];
const mockApprove = jest.fn<Promise<string>, [unknown, string, Uint8Array, () => void, string?]>();
let mockDecimals: number | undefined = 2;
jest.mock('../src/wallet/evmTransfer.ts', () => ({...jest.requireActual('../src/wallet/evmTransfer.ts'),
  readEvmReceiveBalance: async (...args: unknown[]) => {
    if (mockDecimals === undefined) throw new Error('offline');
    if (mockDecimals > 36) throw new Error('EVM_ASSET_DECIMALS');
    return {amount: '6000000', block: '1', ...(args[5] ? {decimals: mockDecimals} : {})};
  }}));
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

const wonName = 'NU54 Test KRW';
const signed = (name: string, bytes: Uint8Array, phone: string) =>
  bytesToHex(signDigest(hexToBytes(orderLockFor(name, bytes.subarray(0, quoteV2Layout.unsignedSize), phone).digest), payerKey), false);
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
beforeEach(() => {
  jest.useFakeTimers({now: now * 1000});
  jest.spyOn(AppState, 'addEventListener').mockReturnValue({remove() {}});
  mockJournals.clear(); mockLocks.length = 0; mockNearby.length = 0; mockWrites = [];
  mockDecimals = 2;
  mockOrder = order; mockOffline = false; mockClaimed = true; mockRelay = undefined; mockList = []; mockQuoteRefused = false; mockLockError = '';
  mockLockReply = () => ({status: 'ok', value: {submitted: mockSale}});
  mockWallet = {mode: 'personal', enrollment: {device: 'pinned'}, verified: {network: '8283', address: payer, generation: generation.toString(), boardFingerprint: 'pinned'}};
  mockStatus.mockReset().mockResolvedValue({account: payer, perPayment: 30_000_000n, daily: 100_000_000n, pinWait: 0, orderOpen: false, leftToday: 95_500_000n});
  mockProtect.mockClear();
  mockAsk.mockClear();
  mockApprove.mockReset().mockImplementation(async (payment, phone, _request, onBoard) => {
    onBoard();
    return signed(wonName, (payment as {bytes: Uint8Array}).bytes, phone);
  });
});
afterEach(async () => {
  await Renderer.act(async () => { tree?.unmount(); });
  tree = undefined;
  jest.useRealTimers();
  jest.restoreAllMocks();
});

test('a won order is shown in won and the board is asked for the won token', async () => {
  await toConfirm();
  expect(screen()).toContain('45,000원');
  expect(screen()).not.toContain('TestUSDC');
  await press('내 보드 확인');
  expect(screen()).toContain('300,000원');
  expect(screen()).toContain('60,000원');
  await press('보드로 승인');
  expect(mockApprove.mock.calls[0][4]).toBe(wonName);
  expect(mockNearby.some(item => item.op === 'lock')).toBe(true);
});

test('a board that signed the dollar token pays nothing', async () => {
  mockApprove.mockImplementation(async (payment, phone, _request, onBoard) => {
    onBoard();
    return signed(dollarToken.name, (payment as {bytes: Uint8Array}).bytes, phone);
  });
  await toConfirm();
  await press('내 보드 확인');
  await press('보드로 승인');
  expect(mockNearby.some(item => item.op === 'lock')).toBe(false);
});

test.each([18, 40])('%d decimals on chain stop the payment before the board', async decimals => {
  mockDecimals = decimals;
  await toConfirm();
  await press('내 보드 확인');
  expect(screen()).toContain('이 매장의 토큰이 등록된 내용과 다릅니다');
  expect(button('보드로 승인')).toBeUndefined();
  expect(mockApprove).not.toHaveBeenCalled();
});

test('a chain that cannot be read does not stop it; the server checks too', async () => {
  mockDecimals = undefined;
  await toConfirm();
  await press('내 보드 확인');
  expect(screen()).not.toContain('이 매장의 토큰이 등록된 내용과 다릅니다');
  expect(button('보드로 승인')).toBeDefined();
});

test('the reader itself names an unreadable number of decimals', async () => {
  const {readEvmReceiveBalance} = jest.requireActual<typeof import('../src/wallet/evmTransfer')>('../src/wallet/evmTransfer');
  const word = (value: bigint) => '0x' + value.toString(16).padStart(64, '0'), genesis = '0x' + 'a1'.repeat(32);
  const rpc = (decimals: string) => async (method: string, params: unknown[]) => method === 'eth_chainId' ? '0x205b'
    : method === 'eth_getBlockByNumber' ? {hash: params[0] === '0x0' ? genesis : '0x' + 'b1'.repeat(32), number: '0x10'}
    : (params[0] as {data: string}).data === '0x313ce567' ? decimals : word(5n);
  await expect(readEvmReceiveBalance(rpc(word(2n)) as never, '8283', genesis, payer, policy.asset, true)).resolves.toMatchObject({decimals: 2});
  for (const bad of [word(40n), '0x', 'x']) await expect(readEvmReceiveBalance(rpc(bad) as never, '8283', genesis, payer, policy.asset, true)).rejects.toThrow('EVM_ASSET_DECIMALS');
});
