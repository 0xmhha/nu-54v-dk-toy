import React from 'react';
import {AppState} from 'react-native';
import Renderer from 'react-test-renderer';
import {keccak_256} from '@noble/hashes/sha3';
import {
  accountProofDigest as mockProofDigest, bindServiceRequestV2 as mockBind, bytesToHex as mockHex, hexToBytes as mockBytes,
  recoverSigner as mockRecover, signDigest as mockSign, toBase64 as mockBase64, utf8Encode, type CommerceOrder, type ServiceRequestV2,
} from '@nu54/protocol';
import {CustomerOrders} from '../src/merchant/CustomerOrders';
import {forgetEntranceWaits} from '../src/merchant/customerEntrance';
import {orderRecords} from '../src/merchant/orderRecords';
import {customerSubject, forgetOrderRefusals} from '../src/merchant/storeAccess';
import {journalStore} from './fixtures/journal';
import {
  boardSignature, branch as mockBranch, escrowStatus as mockEscrow, generation as mockGeneration, now, now as mockNow, order as mockIssued, payer as mockPayer, payerKey as mockPayerKey,
  policy as mockPolicy, quoteFor as mockQuoteFor, sale as mockSale, terminal as mockTerminal,
} from './fixtures/order';

// An order from a store page after it was paid, and what its pages say on the way: how the phone learns that the
// order ended, what it does when the store's server no longer answers for it, and where each answer is shown.
// The store server, the chain and the board are those of storeOrder.test.tsx, with an order that can end.

const mockTenant = '0fbc54ef-2827-4ab5-bfcd-3a27611c39e0';
const mockSubject = customerSubject(mockPayer);
const mockSelector = (signature: string) => mockHex(keccak_256(utf8Encode(signature)).subarray(0, 4), false);
const mockWord = (value: string | bigint) => typeof value === 'string' ? value.replace(/^0x/, '').padStart(64, '0') : value.toString(16).padStart(64, '0');
const mockAnchor = {origin: 'https://store.example:8444', ca: mockBase64(new Uint8Array(96).fill(3)), pin: 'ab'.repeat(32)};
let mockPin: object | undefined;
jest.mock('../src/merchant/customerEntranceDeployment.ts', () => ({__esModule: true,
  get approvedCustomerEntrance() { return mockAnchor; }, get approvedOrderPayment() { return mockPin; }}));

type Sent = {path: string; operation?: string; body?: Record<string, unknown>; session?: string};
const mockSent: Sent[] = [];
const mockJournals = new Map<string, string>();
// The sealed journal takes only the names the native module lists, as on a phone.
const mockJournal = journalStore(mockJournals);
let mockJournalDown = false;
const mockProfileReads: string[] = [];
const mockSessions = new Map<string, {purpose: string; sale?: string}>();
let mockStage: 'none' | 'preview' | 'issued' | 'locked' | 'ready' | 'delivered' | 'paid' | 'refunded' = 'none';
// The buyer's answer the server holds and has not seen on the chain yet.
let mockPending: 'receipt' | undefined;
// A store server that ends the session of an order with the order, as it did before it kept the session for reading.
let mockOrderSessionsEnd = false;
// A proof for an order the server does not take: the order is not one it follows any more.
let mockOrderProofRefused = false;
let mockConfirmer = '';
let mockOrigin = '';
let mockStock = 3;
let mockMenuPrice = mockIssued.amount;
let mockSessionSeed = 1;
let mockEntranceDown = false;
let mockQuoteChange: ((words: Uint8Array) => void) | undefined;
let mockHeld = false;
// What the server answers to the next carts instead of holding them, and to a cart that is taken back.
let mockHold: {status: number; retryAfter?: number; more: Record<string, unknown>} | undefined;
let mockCancel: 'closed' | 'quote_issued' | 'paid' | 'gone' = 'closed';
let mockClosedUnpaid = false;
const mockCreateKeys: string[] = [];
const mockCancels: {sale: string; session: string}[] = [];
const mockMenuRow = (available: number) => ({sku: 'latte', optionKey: 'iced', label: '라떼', asset: mockIssued.asset, version: 1, unitPrice: mockMenuPrice,
  available, skuAvailable: available, expiresAt: new Date((mockNow + 3600) * 1000).toISOString()});
const mockLocks: (ServiceRequestV2 & {approvalSignature?: string})[] = [];
const mockAnswers: ServiceRequestV2[] = [];

function mockOrderNow(): CommerceOrder {
  if (mockClosedUnpaid) return {...mockIssued, state: 'closed', revision: 1, quoteDigest: undefined, orderKey: undefined};
  if (mockStage === 'preview') return {...mockIssued, state: 'preview', revision: 0, quoteDigest: undefined, orderKey: undefined};
  if (mockStage === 'issued') return {...mockIssued, quoteDigest: mockQuoteFor(mockOrigin, mockQuoteChange).quoteDigest};
  return {...mockIssued, quoteDigest: undefined, orderKey: undefined, receiptRef: mockTerminal, state: 'paid', pickupNo: 17,
    escrow: {...mockEscrow(mockStage === 'delivered' || mockStage === 'paid' || mockStage === 'refunded' ? mockStage : 'locked', mockConfirmer),
      ...(mockPending && mockStage === 'delivered' ? {pendingAction: mockPending} : {})}, payment: {stage: 'finalized', chainId: 8283, txHash: '0x' + '1'.repeat(64),
      includedAt: new Date(mockNow * 1000).toISOString(),
      ...(mockStage === 'ready' ? {pickupStage: 'ready' as const, finalizedAt: new Date(mockNow * 1000).toISOString(), pickupReadyAt: new Date(mockNow * 1000).toISOString()} : {}),
      ...(mockStage === 'delivered' || mockStage === 'paid' ? {pickupStage: 'collected' as const, finalizedAt: new Date(mockNow * 1000).toISOString(), pickupReadyAt: new Date(mockNow * 1000).toISOString()} : {})}};
}

jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  loadProfile: async (slot: string) => { mockProfileReads.push(slot); return ''; },
  loadJournal: async (name: string) => {
    if (mockJournalDown && name.startsWith('customer-sessions-')) throw Object.assign(new Error('refused'), {code: 'COMMERCE_REFUSED'});
    return mockJournal.loadJournal(name);
  },
  saveJournal: async (name: string, value: string) => {
    if (mockJournalDown && name.startsWith('customer-sessions-')) throw Object.assign(new Error('refused'), {code: 'COMMERCE_REFUSED'});
    return mockJournal.saveJournal(name, value);
  },
  randomBytes: async (length: number) => mockBase64(new Uint8Array(length).fill(7)),
  exchange: async () => { throw new Error('a registered connection was used'); },
  // The customer entrance of the store server, as the native module reaches it with this build's anchor.
  customerRequest: async (anchor: string, path: string, session: string, body: string) => {
    if (JSON.parse(anchor).origin !== mockAnchor.origin) throw new Error('another server');
    if (mockEntranceDown) throw new Error('offline');
    const answer = (status: number, value: unknown, retryAfter?: number) => JSON.stringify({status, body: JSON.stringify(value), ...(retryAfter ? {retryAfter} : {})});
    if (!body) {
      mockSent.push({path});
      // The public menu tells sold out (0) or on sale (1000), never how many are left (opsd public_menu_window).
      if (path === `/v2/customer/stores/${mockBranch}/menu/0`) return answer(200, {shop: {branch: mockBranch, branchLabel: '그린 카페'}, orderState: {mode: 'open'},
        catalogVersion: 1, catalog: [mockMenuRow(mockStock > 0 ? 1000 : 0)]});
      return answer(404, null);
    }
    const sent = JSON.parse(body);
    if (path === '/v2/customer/checkout/cancel') {
      mockSent.push({path, body: sent, session});
      if (mockSessions.get(session)?.purpose !== 'checkout' || mockCancel === 'gone') return answer(404, null);
      mockCancels.push({sale: sent.saleRef, session});
      if (mockCancel !== 'closed') return answer(409, {requestId: '', requestDigest: '', status: 'error', error: 'CHECKOUT_CONFLICT', reason: mockCancel});
      mockStage = 'none'; mockStock++;
      return answer(200, {status: 'ok', saleRef: sent.saleRef, state: 'closed'});
    }
    if (path === '/v2/customer/challenge') {
      mockSent.push({path, body: sent});
      return answer(200, {challenge: 'c1'.repeat(32), expiresAt: mockNow + 120});
    }
    if (path === '/v2/customer/proof') {
      mockSent.push({path, body: sent});
      // The server accepts only the account's own signature over the challenge.
      if (mockRecover(mockProofDigest(sent.account, mockBytes(sent.challenge)), mockBytes(sent.signature)) !== sent.account)
        return answer(400, {status: 'error', error: 'MISMATCHED_EVIDENCE'});
      if (mockHeld) return answer(409, {status: 'error', error: 'CHECKOUT_CONFLICT'});
      const asked = [...mockSent].reverse().find(item => item.path === '/v2/customer/challenge')!.body as {purpose: string; saleRef?: string};
      // A ready order's proof is traded for a one-time pickup value, never for a session.
      if (asked.purpose === 'order' && mockOrderProofRefused) return answer(400, {status: 'error', error: 'MISMATCHED_EVIDENCE'});
      if (asked.purpose === 'pickup') return answer(200, {purpose: 'pickup', branchRef: mockBranch, saleRef: asked.saleRef, pickupToken: 'd4'.repeat(32), expiresAt: mockNow + 60});
      const token = (mockSessionSeed++).toString(16).padStart(2, '0').repeat(32);
      mockSessions.set(token, {purpose: asked.purpose, sale: asked.saleRef});
      return answer(200, {purpose: asked.purpose, branchRef: mockBranch, ...(asked.saleRef ? {saleRef: asked.saleRef} : {}), tenantRef: mockTenant,
        subjectRef: mockSubject, session: token, expiresAt: mockNow + (asked.purpose === 'checkout' ? 900 : 86400)});
    }
    const {command, approvalSignature} = sent as {command: ServiceRequestV2; approvalSignature?: string};
    mockSent.push({path, operation: command.operation, session});
    const held = mockSessions.get(session);
    const allowed = held?.purpose === 'checkout' ? ['catalog.read', 'order.create', 'checkout.claim', 'checkout.authorize', 'quote.issue', 'order.lock', 'order.status']
      : held?.purpose === 'order' && held.sale === command.objectRef ? ['order.status', 'order.answer'] : [];
    if (!allowed.includes(command.operation) || (mockOrderSessionsEnd && held?.purpose === 'order')) return answer(404, null);
    // The server binds its answer to the session's customer and tenant, never to what the phone claims.
    const {requestId, requestDigest} = mockBind(command, {role: 'customer', tenantRef: mockTenant, subjectRef: mockSubject, expiresAt: mockNow + 900,
      grants: [{domainRef: command.domainRef, objectRef: command.objectRef, operations: [`C3/${command.operation}`]}]}, Math.floor(Date.now() / 1000));
    const ok = {requestId, requestDigest, status: 'ok', resultRef: command.objectRef};
    // A session's menu is the public one with the counts of what is left.
    if (command.operation === 'catalog.read') return command.objectRef !== mockBranch ? answer(404, null) : answer(200, {...ok, shop: {branch: mockBranch, branchLabel: '그린 카페'},
      orderState: {mode: 'open'}, catalogVersion: 1, catalog: [mockMenuRow(mockStock)]});
    if (command.operation === 'order.create') mockCreateKeys.push(command.idempotencyKey ?? "");
    if (command.operation === 'order.create' && mockHold)
      return answer(mockHold.status, {requestId, requestDigest, status: 'error', ...mockHold.more}, mockHold.retryAfter);
    if (command.operation === 'order.create') { mockStage = 'preview'; mockStock--; mockOrigin = command.objectRef; return answer(200, {...ok, resultRef: mockSale}); }
    if (command.operation === 'order.status') return answer(200, {...ok, order: mockOrderNow()});
    if (command.operation === 'checkout.claim') return answer(200, ok);
    if (command.operation === 'checkout.authorize') return answer(200, {...ok, claim: {claimDigest: 'a'.repeat(64), expiresAt: Math.floor(Date.now() / 1000) + 60}});
    if (command.operation === 'quote.issue') { mockStage = 'issued'; return answer(200, {...ok, quote: {...mockQuoteFor(mockOrigin, mockQuoteChange), presentation: undefined}}); }
    if (command.operation === 'order.lock') { mockLocks.push({...command, approvalSignature}); mockConfirmer = command.body.confirmer; mockStage = 'locked'; return answer(200, ok); }
    if (command.operation === 'order.answer') { mockAnswers.push(command); mockPending = 'receipt'; return answer(200, ok); }
    return answer(404, null);
  },
}}));

// The chain: the build's order contract with the store's merchant record, and the board account's balance.
const mockChain: {merchant: {signer: string; beneficiary: string; keyVersion: number; beneficiaryVersion: number; active: bigint}; balance: bigint; down: boolean;
  calls: {to: string; data: string}[]} = {merchant: {signer: '', beneficiary: '', keyVersion: 0, beneficiaryVersion: 0, active: 1n}, balance: 0n, down: false, calls: []};
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({__esModule: true, default: {
  loadProfile: async () => '', setHomeProtection: async () => {},
  evmRpc: async (_url: string, packet: string) => {
    const {method, params} = JSON.parse(packet) as {method: string; params: unknown[]};
    if (mockChain.down) throw new Error('offline');
    const result = (() => {
      if (method === 'eth_chainId') return '0x205b';
      if (method === 'eth_getBlockByNumber') return {hash: params[0] === '0x0' ? jest.requireActual('../src/wallet/readProfiles').readProfiles.stable.anchor : '0x' + '5c'.repeat(32), number: '0x64'};
      const [{to, data}] = params as [{to: string; data: string}];
      mockChain.calls.push({to, data});
      if (to === mockPolicy.contract && data === '0x' + mockSelector('asset()')) return '0x' + mockWord(mockPolicy.asset);
      if (to === mockPolicy.contract && data === '0x' + mockSelector('merchants(bytes32)') + mockPolicy.merchantId)
        return '0x' + [mockWord(mockChain.merchant.signer), mockWord(mockChain.merchant.beneficiary), mockWord(BigInt(mockChain.merchant.keyVersion)),
          mockWord(BigInt(mockChain.merchant.beneficiaryVersion)), mockWord(172800n), mockWord(604800n), mockWord(mockChain.merchant.active)].join('');
      if (to === mockPolicy.asset && data.startsWith('0x70a08231')) return '0x' + mockWord(mockChain.balance);
      return '0x' + mockWord(0n).repeat(7);
    })();
    return JSON.stringify({jsonrpc: '2.0', id: 1, result});
  },
}}));
jest.mock('../src/specs/NativeShopQr.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({__esModule: true, default: {permission: async () => true, clear: async () => {}, orderReady: async () => true}}));
jest.mock('../src/specs/NativeNearby.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/specs/NativeRenterBle.ts', () => ({__esModule: true, default: {
  onScan: (receive: (raw: string) => void) => { receive(JSON.stringify({address: 'board-1', name: 'NU54DK'})); return {remove() {}}; },
  scanOwner: async () => {},
}}));
let mockWallet: Record<string, unknown>;
jest.mock('../src/wallet/profile.ts', () => ({decodeWalletStorage: () => mockWallet}));
let mockBoardNear = true;
let mockBoardKey = mockPayerKey;
let mockBoardGeneration = mockGeneration;
const mockProofs: string[] = [];
const mockApprove = jest.fn<Promise<string>, [unknown, string, Uint8Array, () => void]>();
jest.mock('../src/wallet/ownerSession.ts', () => ({OwnerSession: class {
  connect = async () => { if (!mockBoardNear) throw new Error('BOARD_NOT_FOUND'); };
  close = () => {};
  status = async () => ({generation: mockBoardGeneration});
  spendingStatus = async () => ({account: jest.requireActual('@nu54/protocol').addressOfPrivateKey(mockBoardKey), perPayment: 30_000_000n, daily: 100_000_000n, pinWait: 0, orderOpen: false, leftToday: 95_500_000n});
  proveAccount = async (account: string, challenge: Uint8Array) => {
    mockProofs.push(account);
    return {account, signature: mockHex(mockSign(mockProofDigest(account, challenge), mockBoardKey), false)};
  };
  approveOrder = mockApprove;
}}));

let tree: Renderer.ReactTestRenderer | undefined;
const screen = () => JSON.stringify(tree?.toJSON());
const button = (label: string) => tree?.root.findAll(node => (node.props.label === label && typeof node.props.run === 'function'))[0];
async function press(label: string) {
  const found = button(label);
  expect([label, !!found, found?.props.disabled ?? false]).toEqual([label, true, false]);
  await Renderer.act(async () => { found!.props.run(); });
  await Renderer.act(async () => { await jest.advanceTimersByTimeAsync(0); });
}
const tick = (ms: number) => Renderer.act(async () => { await jest.advanceTimersByTimeAsync(ms); });
const store = {branch: mockBranch, name: '그린 카페', merchantId: mockPolicy.merchantId};
const back = jest.fn(), openBoard = jest.fn();
async function open(props: Partial<React.ComponentProps<typeof CustomerOrders>> = {}) {
  await Renderer.act(async () => { tree = Renderer.create(<CustomerOrders back={back} openBoard={openBoard} initialLocator="" shopEntry store={store} permission={async () => true} {...props} />); });
  await tick(0);
}

beforeEach(() => {
  jest.useFakeTimers({now: now * 1000});
  jest.spyOn(AppState, 'addEventListener').mockReturnValue({remove() {}});
  mockSent.length = 0; mockJournals.clear(); mockProfileReads.length = 0; mockSessions.clear(); mockLocks.length = 0; mockAnswers.length = 0; mockProofs.length = 0;
  mockJournalDown = false;
  mockStage = 'none'; mockConfirmer = ''; mockOrigin = ''; mockStock = 3; mockMenuPrice = mockIssued.amount; mockSessionSeed = 1; mockEntranceDown = false; mockQuoteChange = undefined; mockHeld = false;
  mockHold = undefined; mockCancel = 'closed'; mockCancels.length = 0; mockClosedUnpaid = false; mockCreateKeys.length = 0;
  mockPending = undefined; mockOrderSessionsEnd = false; mockOrderProofRefused = false;
  forgetEntranceWaits(); forgetOrderRefusals();
  mockPin = {chain: 8283, contract: mockPolicy.contract, asset: mockPolicy.asset};
  Object.assign(mockChain, {merchant: {signer: mockPolicy.signer, beneficiary: mockPolicy.beneficiary, keyVersion: mockPolicy.keyVersion,
    beneficiaryVersion: mockPolicy.beneficiaryVersion, active: 1n}, balance: 95_500_000n, down: false});
  mockChain.calls.length = 0;
  mockWallet = {mode: 'personal', enrollment: {device: 'pinned'}, verified: {network: '8283', address: mockPayer, generation: Number(mockGeneration), boardFingerprint: 'pinned'}};
  mockBoardNear = true; mockBoardKey = mockPayerKey; mockBoardGeneration = mockGeneration;
  back.mockClear(); openBoard.mockClear();
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

async function toOrder() {
  await open();
  await press('라떼 iced 추가');
  await press('주문 접수 요청');
}
/** A paid order, its page closed: the phone holds the order's record and the session that follows it. */
async function paid() {
  await toOrder();
  await press('이 주문의 결제 내용 검토');
  await press('내 보드 확인');
  await press('보드로 승인');
  await tick(2500);
  await press('확인');
  await Renderer.act(async () => { tree!.unmount(); });
  mockSent.length = 0; mockProofs.length = 0;
}
const record = async () => (await orderRecords(jest.requireMock('../src/specs/NativeCommerceHttp.ts').default).list()).find(row => row.sale === mockSale)!;
const statusReads = () => mockSent.filter(item => item.operation === 'order.status').length;
const asked = () => mockSent.filter(item => item.path === '/v2/customer/challenge').length;
const unread = '매장 서버가 이 주문의 상태를 지금 알려 주지 않습니다. 아래는 이 휴대전화가 마지막으로 확인한 상태입니다. 몇 분 뒤 다시 확인하세요.';

describe('an order that ended', () => {
  test('the answer "받았어요" is followed to its end: the page says the store was paid, the record ends, and nothing is asked after it', async () => {
    await paid();
    mockStage = 'delivered';
    await open({initialOrder: mockSale});
    await tick(0);
    await press('받았어요');
    await press('네, 받았어요');
    expect(screen()).toContain("'받았어요' 답을 확인하고 있습니다. 확정되면 상태가 바뀝니다.");
    expect((await record()).stage).toBe('delivered');
    // The store's server sees the payout on the chain. The order's session still reads its status, which is now final.
    mockStage = 'paid'; mockPending = undefined;
    await tick(15_000);
    await tick(0);
    expect(screen()).toContain('받았다고 확인해서 매장에 지급되었습니다.');
    expect(screen()).not.toContain("'받았어요' 답을 확인하고 있습니다");
    expect(await record()).toMatchObject({stage: 'paid', payout: 'receipt'});
    expect(button('상태 다시 확인')).toBeUndefined();
    expect(button('받았어요')).toBeUndefined();
    // The phone stops asking: no status read and no proof in the next ten minutes.
    const reads = statusReads();
    mockProofs.length = 0;
    await tick(600_000);
    expect([statusReads() - reads, mockProofs]).toEqual([0, []]);
  });

  test('a refund is learned the same way, and the order no longer counts as open', async () => {
    await paid();
    mockStage = 'refunded';
    await open({initialOrder: mockSale});
    await tick(0);
    expect(screen()).toContain('매장이 주문을 취소해서 결제 금액이 보드 계정으로 돌아왔습니다.');
    expect(await record()).toMatchObject({stage: 'refunded', refund: 'store'});
    const records = orderRecords(jest.requireMock('../src/specs/NativeCommerceHttp.ts').default);
    expect(await records.openFor({payer: mockPayer, generation: mockGeneration.toString()})).toEqual([]);
  });
});

describe('a store server that no longer answers for an order', () => {
  test('the board proves once more, and then rests: the page says what is known and asks no board for a minute', async () => {
    await paid();
    mockStage = 'delivered'; mockOrderSessionsEnd = true;
    await open({initialOrder: mockSale});
    await tick(0);
    // The kept session was refused, the board proved again, and the new session was refused as well.
    expect([asked(), mockProofs, statusReads()]).toEqual([1, [mockPayer], 2]);
    for (let i = 0; i < 6; i++) {
      await press('상태 다시 확인');
      await tick(5000);
    }
    expect([asked(), mockProofs, statusReads()]).toEqual([1, [mockPayer], 2]);
    expect(screen()).toContain(unread);
    // What the phone wrote down last stays on the page.
    expect(screen()).toContain('매장이 주문을 받았습니다.');
    expect((await record()).stage).toBe('locked');
    // After the wait one more proof is asked for, and the next wait is twice as long.
    await tick(31_000);
    await press('상태 다시 확인');
    expect([asked(), statusReads()]).toEqual([2, 3]);
    await tick(61_000);
    await press('상태 다시 확인');
    expect(asked()).toBe(2);
    await tick(60_000);
    await press('상태 다시 확인');
    expect(asked()).toBe(3);
  });

  test('once the server answers again the wait is over and the order is read', async () => {
    await paid();
    mockStage = 'delivered'; mockOrderSessionsEnd = true;
    await open({initialOrder: mockSale});
    await tick(0);
    expect(asked()).toBe(1);
    mockOrderSessionsEnd = false; mockStage = 'paid';
    await tick(61_000);
    await press('상태 다시 확인');
    expect(asked()).toBe(2);
    expect(screen()).toContain('받았다고 확인해서 매장에 지급되었습니다.');
    expect(screen()).not.toContain(unread);
  });

  test('a proof the server does not take for the order is not asked for again at once either', async () => {
    await paid();
    mockSessions.clear(); mockOrderProofRefused = true;
    await open({initialOrder: mockSale});
    await tick(0);
    expect(asked()).toBe(1);
    expect(screen()).toContain('매장 서버가 이 보드 계정의 확인을 받아들이지 않았습니다.');
    await press('상태 다시 확인');
    await press('상태 다시 확인');
    expect(asked()).toBe(1);
    expect(screen()).toContain(unread);
  });
});
