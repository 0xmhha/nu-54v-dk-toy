import React from 'react';
import {AppState} from 'react-native';
import Renderer from 'react-test-renderer';
import {keccak_256} from '@noble/hashes/sha3';
import {
  accountProofDigest as mockProofDigest, bindServiceRequestV2 as mockBind, bytesToBig, bytesToHex as mockHex, hexToBytes as mockBytes, quoteV2Layout as q,
  recoverSigner as mockRecover, signDigest as mockSign, toBase64 as mockBase64, utf8Encode, type CommerceOrder, type ServiceRequestV2,
} from '@nu54/protocol';
import {CustomerOrders, holdOver, missingUnits, shownBalance} from '../src/merchant/CustomerOrders';
import {forgetEntranceWaits} from '../src/merchant/customerEntrance';
import {customerSubject} from '../src/merchant/storeAccess';
import {readProfiles} from '../src/wallet/readProfiles';
import {journalStore} from './fixtures/journal';
import {
  boardSignature, branch as mockBranch, escrowStatus as mockEscrow, generation as mockGeneration, now, now as mockNow, order as mockIssued, payer as mockPayer, payerKey as mockPayerKey,
  policy as mockPolicy, quoteFor as mockQuoteFor, sale as mockSale, terminal as mockTerminal,
} from './fixtures/order';

// A phone that never registered at a store orders from a store page: the menu is read without a proof, the board
// proves its account when the order is asked for, the quote is checked against the build's order contract and the
// merchant record read from it, the board approves, and the store's server submits the sponsored lock.

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
let mockStage: 'none' | 'preview' | 'issued' | 'locked' | 'ready' | 'delivered' = 'none';
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
const mockKeys = () => mockCreateKeys;
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
    escrow: mockEscrow(mockStage === 'delivered' ? 'delivered' : 'locked', mockConfirmer), payment: {stage: 'finalized', chainId: 8283, txHash: '0x' + '1'.repeat(64),
      includedAt: new Date(mockNow * 1000).toISOString(),
      ...(mockStage === 'ready' ? {pickupStage: 'ready' as const, finalizedAt: new Date(mockNow * 1000).toISOString(), pickupReadyAt: new Date(mockNow * 1000).toISOString()} : {}),
      ...(mockStage === 'delivered' ? {pickupStage: 'collected' as const, finalizedAt: new Date(mockNow * 1000).toISOString(), pickupReadyAt: new Date(mockNow * 1000).toISOString()} : {})}};
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
    if (!allowed.includes(command.operation)) return answer(404, null);
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
    if (command.operation === 'order.answer') { mockAnswers.push(command); return answer(200, ok); }
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
const sentPaths = () => mockSent.map(item => item.operation ?? item.path.replace(mockBranch, 'STORE').replace('/v2/customer/', ''));
const proofs = () => mockSent.filter(item => item.path === '/v2/customer/proof').map(item => item.body!);

beforeEach(() => {
  jest.useFakeTimers({now: now * 1000});
  jest.spyOn(AppState, 'addEventListener').mockReturnValue({remove() {}});
  mockSent.length = 0; mockJournals.clear(); mockProfileReads.length = 0; mockSessions.clear(); mockLocks.length = 0; mockAnswers.length = 0; mockProofs.length = 0;
  mockJournalDown = false;
  mockStage = 'none'; mockConfirmer = ''; mockOrigin = ''; mockStock = 3; mockMenuPrice = mockIssued.amount; mockSessionSeed = 1; mockEntranceDown = false; mockQuoteChange = undefined; mockHeld = false;
  mockHold = undefined; mockCancel = 'closed'; mockCancels.length = 0; mockClosedUnpaid = false; mockCreateKeys.length = 0;
  forgetEntranceWaits();
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
async function toConfirm() {
  await toOrder();
  await press('이 주문의 결제 내용 검토');
}

test('the menu, prices and the basket need no board, no proof and no registration', async () => {
  mockBoardNear = false;
  await open();
  expect(screen()).toContain('그린 카페 메뉴');
  expect(screen()).toContain('4.50 TestUSDC');
  expect(screen()).not.toContain('주문 등록');
  await press('라떼 iced 추가');
  expect(screen()).toContain('합계');
  expect(screen()).toContain('보드 잔액');
  expect(sentPaths()).toEqual(['stores/STORE/menu/0']);
  expect(mockProofs).toEqual([]);
  expect(mockProfileReads).toEqual([]); // the store registration of the earlier route is never read
  expect(mockStock).toBe(3); // reading held nothing
  expect(button('매장 다시 선택')).toBeUndefined();
  await press('매장 화면으로');
  expect(back).toHaveBeenCalledTimes(1);
});

test('asking for the order reads the merchant record, proves the account once, and only then holds the order', async () => {
  await toOrder();
  // After the proof the menu is read again through the session, which tells what is left, before the order is asked for.
  expect(sentPaths()).toEqual(['stores/STORE/menu/0', 'challenge', 'proof', 'catalog.read', 'order.create', 'order.status']);
  expect(mockSent[1].body).toEqual({purpose: 'checkout', branchRef: mockBranch});
  // The generation sent with the proof is the board's, and the one the quote will be issued and signed for.
  expect(proofs()).toEqual([expect.objectContaining({account: mockPayer, generation: mockGeneration.toString(), challenge: 'c1'.repeat(32)})]);
  expect(mockChain.calls.filter(call => call.to === mockPolicy.contract).map(call => call.data.slice(0, 10)))
    .toEqual(['0x' + mockSelector('asset()'), '0x' + mockSelector('merchants(bytes32)')]);
  expect(mockSent.filter(item => item.operation).every(item => item.session === '01'.repeat(32))).toBe(true);
  expect(mockStock).toBe(2);
  expect(mockProofs).toEqual([mockPayer]);
  expect(mockApprove).not.toHaveBeenCalled();
});

test('the quote is checked against the pinned contract and the on-chain record, the board approves, the server locks', async () => {
  await toConfirm();
  expect(sentPaths().slice(6)).toEqual(['order.status', 'checkout.claim', 'checkout.authorize', 'quote.issue', 'order.status']);
  expect(screen()).toContain('결제 내용을 확인하세요');
  expect(screen()).toContain('그린 카페');
  expect(screen()).toContain('4.50 TestUSDC');
  await press('내 보드 확인');
  expect(screen()).toContain('95.50 TestUSDC');
  await press('보드로 승인');
  const [payment, phone] = mockApprove.mock.calls[0] as unknown as [{payer: string; generation: bigint; bytes: Uint8Array}, string];
  expect([payment.payer, payment.generation]).toEqual([mockPayer, mockGeneration]);
  expect(bytesToBig(payment.bytes.subarray(q.payerGenerationOffset, q.payerGenerationOffset + 32)).toString()).toBe(proofs()[0].generation);
  expect(mockLocks).toHaveLength(1);
  expect(mockLocks[0]).toMatchObject({operation: 'order.lock', objectRef: mockSale, body: {confirmer: phone}, approvalSignature: expect.stringMatching(/^[0-9a-f]{128}(1b|1c)$/)});
  await tick(2500);
  expect(screen()).toContain('주문이 접수되었습니다');
  expect(screen()).toContain('그린 카페 주문번호 17');
  // With the board still near, the paid order gets the session that follows it; no button was pressed for it.
  expect(mockSent.filter(item => item.path === '/v2/customer/challenge').map(item => item.body)).toEqual([
    {purpose: 'checkout', branchRef: mockBranch}, {purpose: 'order', branchRef: mockBranch, saleRef: mockSale}]);
  expect(proofs()[1].generation).toBeUndefined();
  const rows = JSON.parse([...mockJournals.values()].find(value => value.includes('"sale"') && value.startsWith('[{'))!);
  expect(rows).toMatchObject([{sale: mockSale, branch: mockBranch, customer: mockSubject, payer: mockPayer, escrow: mockPolicy.contract, merchantId: mockPolicy.merchantId,
    stage: 'locked', pickupNo: 17, store: '그린 카페'}]);
  expect(mockProfileReads).toEqual([]);
});

test.each<[string, () => void]>([
  ['signed by a key the contract does not hold for this store', () => { mockChain.merchant.signer = '0x' + '9'.repeat(40); }],
  ['paying another beneficiary than the contract holds', () => { mockChain.merchant.beneficiary = '0x' + '9'.repeat(40); }],
  ['for a newer key version than the quote names', () => { mockChain.merchant.keyVersion++; }],
  ['for another contract', () => { mockQuoteChange = words => { words.set(mockBytes('0x' + '6'.repeat(40)), q.verifyingContractOffset + 12); }; }],
  ['for another token', () => { mockQuoteChange = words => { words.set(mockBytes('0x' + '6'.repeat(40)), q.assetOffset + 12); }; }],
  ['for another chain', () => { mockQuoteChange = words => { words[q.chainIdOffset + 31] ^= 1; }; }],
  ['for another merchant number', () => { mockQuoteChange = words => { words.set(mockBytes('6'.repeat(64)), q.merchantIdOffset); }; }],
])('a quote %s never reaches the board', async (_name, arrange) => {
  arrange();
  await toConfirm();
  expect(sentPaths()).toContain('quote.issue');
  expect(screen()).not.toContain('결제 내용을 확인하세요');
  expect(screen()).toContain('주문, 자산, 보드 주소 또는 독립 복구 상태를 확인할 수 없습니다.');
  expect(button('내 보드 확인')).toBeUndefined();
  expect(mockApprove).not.toHaveBeenCalled();
  expect(mockLocks).toEqual([]);
});

test('a store the contract does not hold, or a chain that cannot be read, is said before the board is asked', async () => {
  mockChain.merchant.active = 0n;
  await toOrder();
  expect(screen()).toContain('이 매장의 결제 정보가 이 앱의 주문 계약과 맞지 않아 주문할 수 없습니다.');
  expect(sentPaths()).toEqual(['stores/STORE/menu/0']);
  expect(mockProofs).toEqual([]);
  await Renderer.act(async () => { tree!.unmount(); });
  mockChain.merchant.active = 1n;
  mockChain.down = true;
  await toOrder();
  expect(screen()).toContain('결제 정보를 체인에서 확인하지 못해 주문을 시작하지 않았습니다.');
  expect(mockProofs).toEqual([]);
  expect(mockStock).toBe(3);
  // A page that names no merchant number cannot be ordered from, though its menu reads.
  await Renderer.act(async () => { tree!.unmount(); });
  mockChain.down = false;
  await open({store: {branch: mockBranch, name: '그린 카페'}});
  await press('라떼 iced 추가');
  await press('주문 접수 요청');
  expect(screen()).toContain('이 매장은 아직 앱에서 주문 결제를 받지 않습니다.');
  expect(mockProofs).toEqual([]);
});

test('no board, another board, a board registered again and an account that still holds an order are told apart', async () => {
  mockBoardNear = false;
  await toOrder();
  expect(screen()).toContain('결제할 보드를 찾지 못했습니다.');
  expect(sentPaths()).toEqual(['stores/STORE/menu/0']);
  mockBoardNear = true;
  mockBoardKey = new Uint8Array(32).fill(9);
  await press('주문 접수 요청');
  expect(screen()).toContain('이 휴대전화의 보드');
  expect(sentPaths()).toEqual(['stores/STORE/menu/0']);
  mockBoardKey = mockPayerKey;
  mockBoardGeneration = mockGeneration + 1n;
  await press('주문 접수 요청');
  expect(screen()).toContain('보드 등록이 바뀌었습니다.');
  expect(proofs()).toEqual([]); // a generation the phone has not tested is never sent
  await press('내 보드 열기');
  expect(openBoard).toHaveBeenCalledTimes(1);
  mockBoardGeneration = mockGeneration;
  mockHeld = true;
  await press('주문 접수 요청');
  expect(screen()).toContain('끝나지 않은 주문이 있어');
  expect(mockStock).toBe(3);
  mockHeld = false;
  await press('주문 접수 요청');
  expect(screen()).not.toContain('끝나지 않은 주문이 있어');
  expect(sentPaths().slice(-2)).toEqual(['order.create', 'order.status']);
});

test('a phone without a board account reads the menu and is sent to the wallet when it asks for the order', async () => {
  mockWallet = {mode: 'personal'};
  await toOrder();
  expect(screen()).toContain('주문하려면 먼저 지갑 탭에서 보드를 연결하고 서명 시험을 마치세요.');
  expect(sentPaths()).toEqual(['stores/STORE/menu/0']);
  expect(mockProofs).toEqual([]);
});

test('a basket the board account cannot pay is said at the basket, and the board is not asked', async () => {
  mockChain.balance = 4_499_999n;
  await open();
  await press('라떼 iced 추가');
  expect(screen()).toContain('보드 잔액 · 4.49 TestUSDC');
  expect(screen()).toContain('보드 잔액이 합계보다 적어 주문할 수 없습니다.');
  expect(button('주문 접수 요청')!.props.disabled).toBe(true);
  await press('충전하러 가기');
  expect(openBoard).toHaveBeenCalledTimes(1);
  mockChain.balance = 4_500_000n;
  await press('잔액 다시 확인');
  expect(screen()).not.toContain('보드 잔액이 합계보다 적어');
  expect(button('주문 접수 요청')!.props.disabled).toBe(false);
  expect(mockProofs).toEqual([]);
  expect(sentPaths()).toEqual(['stores/STORE/menu/0']);
});

test('a balance that cannot be read does not stop the order; the board check before approval still does', async () => {
  mockChain.balance = 1n;
  const {default: wallet} = jest.requireMock('../src/specs/NativeOwnerWallet.ts') as {default: {evmRpc: (url: string, packet: string) => Promise<string>}};
  const rpc = wallet.evmRpc;
  let hideBalance = true;
  jest.spyOn(wallet, 'evmRpc').mockImplementation(async (url, packet) => {
    if (hideBalance && packet.includes('0x70a08231')) throw new Error('offline');
    return rpc(url, packet);
  });
  await open();
  await press('라떼 iced 추가');
  expect(screen()).toContain('보드 잔액을 확인하지 못했습니다.');
  await press('주문 접수 요청');
  await press('이 주문의 결제 내용 검토');
  hideBalance = false;
  await press('내 보드 확인');
  expect(button('보드로 승인')).toBeUndefined();
  expect(mockApprove).not.toHaveBeenCalled();
});

test('a paid order is followed, answered and handed over with the session of that order and the paying board', async () => {
  await toConfirm();
  await press('내 보드 확인');
  await press('보드로 승인');
  await tick(2500);
  await press('확인');
  await Renderer.act(async () => { tree!.unmount(); });
  // Later, from the home tab: the page of this order. The checkout session is gone; the order's own is kept sealed.
  mockSent.length = 0; mockProofs.length = 0; mockSessions.delete('01'.repeat(32));
  mockStage = 'ready';
  await open({initialOrder: mockSale});
  expect(sentPaths().filter(path => path === 'order.status').length).toBeGreaterThan(0);
  expect(mockSent.filter(item => item.operation).every(item => item.session === '02'.repeat(32))).toBe(true);
  expect(mockProofs).toEqual([]); // the kept order session needs no new proof
  expect(screen()).toContain('준비되었습니다. 카운터에서 수령 QR을 보여 주세요.');
  expect(mockProfileReads).toEqual([]);
  // The order session does not reach the store's cart: a new order here asks the board again.
  expect(mockSent.some(item => item.operation === 'order.create')).toBe(false);
  // Handing over needs the paying board each time: a new proof for a one-time QR, not the session.
  await press('수령 QR 받기');
  await tick(0);
  expect(tree!.root.findAll(node => node.props.testID === 'pickup-qr').length).toBeGreaterThan(0);
  expect(mockSent.filter(item => item.path === '/v2/customer/challenge').map(item => item.body)).toEqual([{purpose: 'pickup', branchRef: mockBranch, saleRef: mockSale}]);
  expect(mockProofs).toEqual([mockPayer]);
  // After the store records the handover the customer answers, with the order's session and the phone's own key.
  await Renderer.act(async () => { tree!.unmount(); });
  mockStage = 'delivered';
  mockSent.length = 0; mockProofs.length = 0;
  await open({initialOrder: mockSale});
  await tick(0);
  await press('못 받았어요');
  expect(mockAnswers).toHaveLength(1);
  expect(mockAnswers[0]).toMatchObject({operation: 'order.answer', objectRef: mockSale, body: {action: 'dispute'}});
  expect(mockSent.filter(item => item.operation === 'order.answer').map(item => item.session)).toEqual(['02'.repeat(32)]);
  expect(mockProofs).toEqual([]);
});

test('an order session the server ended is asked for again from the board, once, when the order is opened', async () => {
  await toConfirm();
  await press('내 보드 확인');
  await press('보드로 승인');
  await tick(2500);
  await press('확인');
  await Renderer.act(async () => { tree!.unmount(); });
  mockSessions.clear();
  mockSent.length = 0; mockProofs.length = 0;
  await open({initialOrder: mockSale});
  await tick(0);
  expect(mockSent.filter(item => item.path === '/v2/customer/challenge').map(item => item.body)).toEqual([{purpose: 'order', branchRef: mockBranch, saleRef: mockSale}]);
  expect(mockProofs).toEqual([mockPayer]);
  expect(sentPaths().at(-1)).toBe('order.status');
  // Without the board the page still shows what the phone last wrote down, and says what it needs.
  await Renderer.act(async () => { tree!.unmount(); });
  mockSessions.clear(); mockBoardNear = false;
  await open({initialOrder: mockSale});
  await tick(0);
  expect(screen()).toContain('결제할 보드를 찾지 못했습니다.');
});

test('a build without its order contract or entrance takes no order from a store page', async () => {
  mockPin = undefined;
  await open();
  expect(screen()).toContain('이 앱에는 주문을 받을 매장 서버와 주문 계약이 아직 정해지지 않았습니다.');
  expect(mockSent).toEqual([]);
  await press('매장 화면으로');
  expect(back).toHaveBeenCalledTimes(1);
  // A pin that is not what a release may bind is no pin.
  await Renderer.act(async () => { tree!.unmount(); });
  mockPin = {chain: 8283, contract: mockPolicy.contract, asset: mockPolicy.asset, signer: mockPolicy.signer};
  await open();
  expect(screen()).toContain('아직 정해지지 않았습니다.');
});

test('the merchant record a server could not choose: only the pinned contract is asked, with the page\'s merchant number', async () => {
  await toOrder();
  const asked = mockChain.calls.filter(call => call.data.startsWith('0x' + mockSelector('merchants(bytes32)')));
  expect(asked).toEqual([{to: mockPolicy.contract, data: '0x' + mockSelector('merchants(bytes32)') + mockPolicy.merchantId}]);
  expect(readProfiles.stable.evm).toBe('8283');
});

describe('coming back to a payment under review', () => {
  const comeBack = async () => {
    await Renderer.act(async () => { tree!.unmount(); });
    mockSent.length = 0; mockProofs.length = 0; mockChain.calls.length = 0;
    await open();
  };

  test('the store\'s record is read again from the contract with no board near, and the payment goes on by it', async () => {
    await toConfirm();
    mockBoardNear = false;
    await comeBack();
    // What the kept payment was checked against is confirmed again before anything uses it: the chain, not a board.
    expect(mockChain.calls.filter(call => call.to === mockPolicy.contract).map(call => call.data.slice(0, 10)))
      .toEqual(['0x' + mockSelector('asset()'), '0x' + mockSelector('merchants(bytes32)')]);
    expect(mockProofs).toEqual([]);
    expect(sentPaths().filter(path => path === 'challenge' || path === 'proof')).toEqual([]);
    expect(screen()).toContain('결제 내용을 확인하세요');
    expect(screen()).not.toContain('체인에서');
    mockBoardNear = true;
    await press('내 보드 확인');
    expect(screen()).toContain('95.50 TestUSDC');
    expect(screen()).not.toContain('보드를 찾지 못했습니다');
    // The store's session of before is gone with the page: the board proves its account once more, then approves.
    await press('보드로 승인');
    await tick(2500);
    expect(mockProofs).toEqual([mockPayer, mockPayer]); // the checkout session again, then the paid order's own
    expect(mockApprove).toHaveBeenCalledTimes(1);
    expect(mockLocks).toHaveLength(1);
    expect(screen()).toContain('주문이 접수되었습니다');
    expect(screen()).not.toContain('키오스크 연결을 확인하고');
  });

  test('a record that cannot be read again is said as that, never as a board that was not found', async () => {
    await toConfirm();
    mockChain.down = true;
    await comeBack();
    expect(screen()).toContain('결제 내용을 확인하세요');
    expect(screen()).toContain('결제 정보를 체인에서 다시 확인하지 못했습니다. 연결을 확인하고 다시 시도하세요.');
    await press('내 보드 확인');
    expect(screen()).toContain('결제 정보를 체인에서 다시 확인하지 못했습니다.');
    expect(screen()).not.toContain('보드를 찾지 못했습니다');
    expect(screen()).not.toContain('보드 상태를 확인하지 못했습니다');
    expect(button('보드로 승인')).toBeUndefined();
    expect(mockProofs).toEqual([]);
    // Once the chain answers, the same button goes on.
    mockChain.down = false;
    await press('내 보드 확인');
    expect(screen()).not.toContain('체인에서 다시 확인하지 못했습니다');
    expect(screen()).toContain('95.50 TestUSDC');
    expect(button('보드로 승인')).toBeDefined();
  });

  test('a signed payment that was not sent yet is not called a lost key when the record cannot be read', async () => {
    await toConfirm();
    await press('내 보드 확인');
    // The board signs; the store's server stops answering before the signature is sent.
    mockApprove.mockImplementationOnce(async (payment, phone, _request, onBoard) => {
      onBoard();
      mockEntranceDown = true;
      return boardSignature(phone, undefined, Buffer.from((payment as {bytes: Uint8Array}).bytes).toString('hex'));
    });
    await press('보드로 승인');
    expect(mockApprove).toHaveBeenCalledTimes(1);
    expect(mockLocks).toEqual([]);
    mockChain.down = true;
    await comeBack();
    expect(screen()).toContain('승인한 결제를 아직 보내지 못했습니다');
    await press('같은 결제 보내기');
    expect(screen()).not.toContain('주문 확인 키를 찾을 수 없습니다');
    expect(screen()).toContain('결제 정보를 체인에서 다시 확인하지 못했습니다.');
    expect(mockLocks).toEqual([]);
    mockChain.down = false; mockEntranceDown = false;
    await press('같은 결제 보내기');
    await tick(2500);
    expect(mockApprove).toHaveBeenCalledTimes(1); // the board is not asked again
    expect(mockLocks).toHaveLength(1);
    expect(screen()).toContain('주문이 접수되었습니다');
  });
});

test('too many requests at the store\'s server is said as that, and the session is not asked for again', async () => {
  await toOrder();
  const {default: http} = jest.requireMock('../src/specs/NativeCommerceHttp.ts') as {default: {customerRequest: (...args: string[]) => Promise<string>}};
  const real = http.customerRequest;
  let busy = true;
  jest.spyOn(http, 'customerRequest').mockImplementation(async (anchor, path, session, body) =>
    busy && path === '/v2/customer/commerce' ? JSON.stringify({status: 429, body: JSON.stringify({status: 'error', error: 'BUSY'})}) : real(anchor, path, session, body));
  mockSent.length = 0; mockProofs.length = 0;
  await press('이 주문의 결제 내용 검토');
  expect(screen()).toContain('요청이 많습니다. 잠시 뒤 다시 시도하세요.');
  expect(mockProofs).toEqual([]);
  busy = false;
  await press('이 주문의 결제 내용 검토');
  expect(screen()).toContain('결제 내용을 확인하세요');
  expect(screen()).not.toContain('요청이 많습니다');
  expect(mockProofs).toEqual([]); // the checkout session stood all along
});

describe('what is left of a menu row', () => {
  test('the menu read without a proof says on sale or sold out, and never a count', async () => {
    await open();
    await press('라떼 iced 추가');
    expect(screen()).toContain('선택 1');
    // 1000 is the server's word for "on sale" in the public menu; it is not a number of items.
    expect(screen()).not.toContain('1000');
    expect(screen()).not.toContain('가능');
    await Renderer.act(async () => { tree!.unmount(); });
    mockStock = 0;
    await open();
    expect(screen()).toContain('품절');
    expect(button('라떼 iced 추가')!.props.disabled).toBe(true);
    expect(screen()).not.toContain('가능');
  });

  test('the count comes with the session\'s menu after the proof, and a basket larger than it is said before any order', async () => {
    await open();
    for (let n = 0; n < 4; n++) await press('라떼 iced 추가'); // three are left; the public menu does not say so
    expect(screen()).toContain('선택 4');
    await press('주문 접수 요청');
    expect(sentPaths()).toEqual(['stores/STORE/menu/0', 'challenge', 'proof', 'catalog.read']);
    expect(screen()).toContain('확보 못한 품목');
    expect(screen()).toContain('×1');
    expect(screen()).toContain('선택 3 · 가능 3');
    expect(mockStock).toBe(3); // nothing was held for a basket that could not be filled
    await press('바뀐 장바구니 확인 후 주문 접수');
    expect(sentPaths().slice(-3)).toEqual(['catalog.read', 'order.create', 'order.status']);
  });
});

describe('a cart the store\'s server does not hold', () => {
  const short = {status: 409, more: {error: 'CHECKOUT_CONFLICT', reason: 'balance_short', required: '4500000', balance: '4499999'}};

  test('a balance the server found short shows what is missing and the way to top up, and holds nothing', async () => {
    mockHold = short; // the phone read 95.50 a moment ago; the server reads the chain immediately before the hold
    await toOrder();
    expect(sentPaths().slice(-1)).toEqual(['order.create']);
    expect(screen()).toContain('보드 잔액이 모자라 주문을 접수하지 않았습니다. 모자란 금액을 확인하고 지갑에서 충전하세요.');
    expect(screen()).toContain('보드 잔액 · 4.49 TestUSDC');
    expect(screen()).toContain('모자란 금액 · 0.01 TestUSDC');
    expect(screen()).toContain('보드 잔액이 합계보다 적어 주문할 수 없습니다.');
    expect(button('주문 접수 요청')!.props.disabled).toBe(true);
    expect(button('저장한 주문 다시 확인')).toBeUndefined(); // no order exists, so nothing is kept to look for
    expect(mockStock).toBe(3);
    expect(mockApprove).not.toHaveBeenCalled();
    await press('충전하러 가기');
    expect(openBoard).toHaveBeenCalledTimes(1);
    // Topped up: the balance is read again, and the same basket is asked for with the session that stood.
    mockHold = undefined;
    await press('잔액 다시 확인');
    expect(screen()).toContain('보드 잔액 · 95.50 TestUSDC');
    expect(screen()).not.toContain('모자란 금액');
    await press('주문 접수 요청');
    expect(sentPaths().slice(-2)).toEqual(['order.create', 'order.status']);
    expect(mockProofs).toEqual([mockPayer]);
    expect(mockStock).toBe(2);
  });

  test('a basket that is changed is another basket: the server\'s word about the earlier one is dropped', async () => {
    mockHold = short;
    await toOrder();
    expect(screen()).toContain('모자란 금액 · 0.01 TestUSDC');
    await press('라떼 iced 추가');
    // The balance the server read stays the newest one known; what is missing is counted for the basket on screen.
    expect(screen()).toContain('보드 잔액 · 4.49 TestUSDC');
    expect(screen()).toContain('모자란 금액 · 4.51 TestUSDC');
  });

  test('a balance the server could not read is "again shortly": nothing is held, and the phone waits before it asks again', async () => {
    mockHold = {status: 503, retryAfter: 3, more: {error: 'BUSY', reason: 'balance_unavailable'}};
    await toOrder();
    expect(screen()).toContain('매장 서버가 보드 잔액을 확인하지 못해 주문을 접수하지 않았습니다. 잠시 뒤 다시 시도하세요.');
    expect(screen()).not.toContain('모자란 금액');
    expect(button('저장한 주문 다시 확인')).toBeUndefined();
    const asked = () => sentPaths().filter(path => path === 'order.create').length;
    expect(asked()).toBe(1);
    mockHold = undefined;
    await press('주문 접수 요청'); // at once: the server asked for three seconds
    expect(asked()).toBe(1);
    expect(screen()).toContain('매장 서버가 보드 잔액을 확인하지 못해 주문을 접수하지 않았습니다.');
    await tick(3000);
    await press('주문 접수 요청');
    expect(asked()).toBe(2);
    expect(sentPaths().slice(-1)).toEqual(['order.status']);
    expect(mockProofs).toEqual([mockPayer]); // the session stood all along
  });

  test('too many carts from this address is "again later", and is not a session that ended', async () => {
    mockHold = {status: 429, retryAfter: 10, more: {error: 'BUSY', reason: 'hold_limit'}};
    await toOrder();
    expect(screen()).toContain('짧은 시간에 주문 요청이 많아 주문을 접수하지 않았습니다. 조금 뒤 다시 시도하세요.');
    expect(button('저장한 주문 다시 확인')).toBeUndefined();
    expect(mockStock).toBe(3);
    mockHold = undefined;
    await press('주문 접수 요청');
    expect(sentPaths().filter(path => path === 'order.create')).toHaveLength(1);
    // The wait is for carts only: the menu and the rest of the session are not held back by it.
    expect(sentPaths().slice(-1)).toEqual(['catalog.read']);
    await tick(10_000);
    await press('주문 접수 요청');
    expect(sentPaths().slice(-2)).toEqual(['order.create', 'order.status']);
    expect(mockProofs).toEqual([mockPayer]);
  });

  test('a store that cannot be paid by a board says so, and does not say to try again shortly', async () => {
    mockHold = {status: 409, more: {error: 'CHECKOUT_CONFLICT', reason: 'orders_closed'}};
    await toOrder();
    expect(screen()).toContain('이 매장은 지금 보드 결제 주문을 받을 수 없습니다. 다시 시도해도 접수되지 않습니다. 매장이나 운영자에게 문의하세요.');
    expect(screen()).not.toContain('잠시 뒤 다시 시도');
    expect(screen()).not.toContain('모자란 금액');
    expect(button('저장한 주문 다시 확인')).toBeUndefined(); // no request is kept: nothing is held and no order exists
    expect(screen()).toContain('선택 1'); // the basket stays
    expect(mockStock).toBe(3);
    expect(mockProofs).toEqual([mockPayer]);
  });

  test('a cart the store released answers the request that made it, and the basket is ordered again with a new one', async () => {
    // The app closed after it asked for the cart and before it wrote the order down; meanwhile the cart was taken
    // back or its hold ran out. The same request sent again is told so, not handed the closed order.
    await toOrder();
    const first = mockSent.filter(item => item.operation === 'order.create').length;
    const name = [...mockJournals.keys()].find(key => key.startsWith('customer-order-') && mockJournals.get(key)!.includes('"cart"'))!;
    const kept = JSON.parse(mockJournals.get(name)!);
    delete kept.order; delete kept.holdUntil;
    mockJournals.set(name, JSON.stringify(kept));
    await Renderer.act(async () => { tree!.unmount(); });
    mockHold = {status: 409, more: {error: 'CHECKOUT_CONFLICT', reason: 'cart_released'}};
    await open();
    await press('저장한 주문 다시 확인');
    expect(mockSent.filter(item => item.operation === 'order.create').length).toBe(first + 1);
    expect(screen()).toContain('이전 주문은 결제되지 않은 채 매장에서 취소되었거나 결제 시간이 지나 닫혔습니다. 잡아 둔 재고는 없습니다. 장바구니를 확인하고 다시 주문하세요.');
    expect(button('저장한 주문 다시 확인')).toBeUndefined();
    expect(screen()).toContain('합계 · 4.50 TestUSDC'); // the basket is back
    expect(mockJournals.get(name)).toBe('null');
    // The next request is a new one, with a key of its own: the server holds a new cart for it. (The random bytes
    // of this test's phone are fixed, so they are changed here to tell a new key from the kept one.)
    mockHold = undefined;
    const {default: http} = jest.requireMock('../src/specs/NativeCommerceHttp.ts') as {default: {randomBytes: (length: number) => Promise<string>}};
    jest.spyOn(http, 'randomBytes').mockImplementation(async length => mockBase64(new Uint8Array(length).fill(9)));
    await press('주문 접수 요청');
    const keys = mockKeys();
    expect(keys.length).toBeGreaterThanOrEqual(2);
    expect(keys[keys.length - 1]).not.toBe(keys[keys.length - 2]);
  });

  test('an order the store closed unpaid, found again after the app was closed, is not shown as an order to pay', async () => {
    await toOrder();
    await Renderer.act(async () => { tree!.unmount(); });
    mockClosedUnpaid = true; // taken back at the store; the app closed before it wrote that down
    await open();
    await press('저장한 주문 다시 확인');
    expect(screen()).toContain('이전 주문은 결제되지 않은 채 매장에서 취소되었거나 결제 시간이 지나 닫혔습니다.');
    expect(button('저장한 주문 다시 확인')).toBeUndefined();
    expect(button('이 주문의 결제 내용 검토')).toBeUndefined();
    expect(screen()).toContain('합계 · 4.50 TestUSDC'); // the basket is back
    expect(button('주문 접수 요청')!.props.disabled).toBe(false);
  });

  test('another unpaid cart of the same account is said as that', async () => {
    mockHold = {status: 409, more: {error: 'CHECKOUT_CONFLICT'}};
    await toOrder();
    expect(screen()).toContain('이 보드 계정이 잡아 둔 다른 미결제 주문이 있어 새 주문을 접수하지 못했습니다.');
    expect(button('저장한 주문 다시 확인')).toBeUndefined();
    expect(button('주문 접수 요청')!.props.disabled).toBe(false);
  });

  test('a refusal that names no reason the phone knows, or numbers that do not read, is not shown as a balance', async () => {
    for (const more of [{error: 'CHECKOUT_CONFLICT', reason: 'balance_short', required: '4500000', balance: '4500000'},
      {error: 'CHECKOUT_CONFLICT', reason: 'balance_short', required: '4.5', balance: '1'}, {error: 'BUSY', reason: 'balance_short', required: '2', balance: '1'},
      {error: 'CHECKOUT_CONFLICT', reason: 'later'}]) {
      mockHold = {status: 409, more};
      await toOrder();
      expect(screen()).toContain('접수 여부를 확인하지 못했습니다. 같은 주문을 다시 조회하세요.');
      expect(screen()).not.toContain('모자란 금액');
      await Renderer.act(async () => { tree!.unmount(); });
      mockJournals.clear();
    }
  });
});

test('what is missing is counted in the unit the screen shows and is never less than what is missing', () => {
  const {tokenAmount} = jest.requireActual('@nu54/protocol') as typeof import('@nu54/protocol');
  const dollar = {name: 'NU54 Test USD', symbol: 'tUSDC', label: 'TestUSDC', decimals: 6};
  const won = {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2};
  const coin = {name: 'WKRC', symbol: 'WKRC', label: 'KRWC', decimals: 18, boardDecimals: 8};
  const shown = (required: bigint, balance: bigint, token: typeof dollar) => tokenAmount(missingUnits(required, balance, token), token);
  // Dollars: a balance is cut to the cent, so one base unit short reads as one cent short, and topping that up is enough.
  expect(shown(4_500_000n, 4_499_999n, dollar)).toBe('0.01 TestUSDC');
  expect(shown(4_500_000n, 4_490_000n, dollar)).toBe('0.01 TestUSDC');
  expect(shown(4_500_000n, 4_489_999n, dollar)).toBe('0.02 TestUSDC');
  expect(shown(9_000_000n, 0n, dollar)).toBe('9.00 TestUSDC');
  // Won are whole on a screen: a part of a won that is missing reads as one won, never as "0.01원" and never as none.
  expect(shown(300_000n, 299_900n, won)).toBe('1원');
  expect(shown(1_250_000n, 0n, won)).toBe('12,500원');
  expect(shown(300_000n, 299_999n, won)).toBe('1원');
  expect(shown(300_000n, 299_850n, won)).toBe('2원');
  // A token of 18 decimals is counted in whole won too.
  expect(shown(3_000n * 10n ** 18n, 3_000n * 10n ** 18n - 1n, coin)).toBe('1원');
  expect(shown(3_000n * 10n ** 18n, 2_000n * 10n ** 18n, coin)).toBe('1,000원');
  expect(missingUnits(1n, 2n, dollar)).toBe('0'); // nothing is missing
  // The balance beside it is cut off at the same unit, so the two rows and the total agree: 2,999 won held and 1 won
  // missing of 3,000, not 2,999.99 and 0.01.
  const held = (balance: bigint, token: typeof dollar) => tokenAmount(shownBalance(balance, token), token);
  expect(held(299_999n, won)).toBe('2,999원');
  expect(held(300_000n, won)).toBe('3,000원');
  expect(held(99n, won)).toBe('0원');
  expect(held(3_000n * 10n ** 18n - 1n, coin)).toBe('2,999원');
  expect(held(4_499_999n, dollar)).toBe('4.49 TestUSDC');
});

test('a hold is over only when its time is known and passed for certain', () => {
  const sale = '0fbc54ef-2827-4ab5-bfcd-3a27611c3001';
  expect(holdOver(undefined, undefined, 9_999)).toBe(false);
  expect(holdOver({order: sale}, undefined, 9_999)).toBe(false); // the store never named a time
  expect(holdOver({holdUntil: 1_000}, undefined, 9_999)).toBe(false); // no order yet
  expect(holdOver({order: sale, holdUntil: 1_000}, undefined, 1_120)).toBe(false); // the clocks may differ by two minutes
  expect(holdOver({order: sale, holdUntil: 1_000}, undefined, 1_121)).toBe(true);
  // A quote this phone holds for the order keeps the order open until that quote ends too.
  expect(holdOver({order: sale, holdUntil: 1_000}, {order: {id: sale}, quote: {expiresAt: 1_300}}, 1_400)).toBe(false);
  expect(holdOver({order: sale, holdUntil: 1_000}, {order: {id: sale}, quote: {expiresAt: 1_300}}, 1_421)).toBe(true);
  expect(holdOver({order: sale, holdUntil: 1_000}, {order: {id: 'another'}, quote: {expiresAt: 9_000}}, 1_121)).toBe(true);
});

describe('too many requests', () => {
  const {default: http} = jest.requireMock('../src/specs/NativeCommerceHttp.ts') as {default: {customerRequest: (...args: string[]) => Promise<string>}};
  /** The server refuses session requests for their rate: 429, or 503 when it serves too many at once. */
  function refuse(status: 429 | 503, retryAfter: number) {
    const real = http.customerRequest;
    const state = {busy: true, calls: 0};
    jest.spyOn(http, 'customerRequest').mockImplementation(async (anchor, path, session, body) => {
      if (path !== '/v2/customer/commerce') return real(anchor, path, session, body);
      state.calls++;
      return state.busy ? JSON.stringify({status, retryAfter, body: JSON.stringify({requestId: '', requestDigest: '', status: 'error', error: 'BUSY'})})
        : real(anchor, path, session, body);
    });
    return state;
  }

  test.each<[429 | 503, number]>([[429, 5], [503, 1]])('a %i with Retry-After is "again shortly": the session stands and nothing is sent until the time has passed', async (status, wait) => {
    await toOrder();
    const state = refuse(status, wait);
    mockProofs.length = 0;
    await press('이 주문의 결제 내용 검토');
    expect(screen()).toContain('요청이 많습니다. 잠시 뒤 다시 시도하세요.');
    expect(state.calls).toBe(1);
    // Pressing again, and the page's own timers, do not reach the server while it asked to wait.
    await press('이 주문의 결제 내용 검토');
    await tick(wait * 1000 - 1);
    await press('이 주문의 결제 내용 검토');
    expect(state.calls).toBe(1);
    expect(screen()).toContain('요청이 많습니다. 잠시 뒤 다시 시도하세요.');
    state.busy = false;
    await tick(1);
    await press('이 주문의 결제 내용 검토');
    expect(screen()).toContain('결제 내용을 확인하세요');
    expect(mockProofs).toEqual([]); // never taken for a session that ended: the board is not asked again
  });

  test('a busy server at the menu says so, and is not taken for a store with nothing on sale', async () => {
    const real = http.customerRequest;
    let busy = true, reads = 0;
    jest.spyOn(http, 'customerRequest').mockImplementation(async (anchor, path, session, body) => {
      if (!path.includes('/menu/')) return real(anchor, path, session, body);
      reads++;
      return busy ? JSON.stringify({status: 429, retryAfter: 2, body: JSON.stringify({requestId: '', requestDigest: '', status: 'error', error: 'BUSY'})}) : real(anchor, path, session, body);
    });
    await open();
    expect(screen()).toContain('요청이 많습니다. 잠시 뒤 다시 시도하세요.');
    expect(screen()).not.toContain('지금 판매 중인 메뉴가 없습니다');
    expect(reads).toBe(1);
  });
});

describe('taking an unpaid cart back', () => {
  const cancels = () => mockSent.filter(item => item.path === '/v2/customer/checkout/cancel');

  test('changing the order releases its hold at the store, and the basket comes back to be ordered again at once', async () => {
    await toOrder();
    expect(mockStock).toBe(2);
    await press('새로 주문하기');
    expect(cancels()).toEqual([{path: '/v2/customer/checkout/cancel', body: {saleRef: mockSale}, session: '01'.repeat(32)}]);
    expect(mockStock).toBe(3);
    expect(screen()).toContain('새로 주문할 수 있습니다. 이전 주문은 취소되어 잡아 둔 재고가 풀렸습니다.');
    expect(screen()).toContain('선택 1');
    expect(mockProofs).toEqual([mockPayer]); // the session that made the cart takes it back; the board is not asked
    await press('주문 접수 요청');
    expect(sentPaths().slice(-2)).toEqual(['order.create', 'order.status']);
    expect(mockStock).toBe(2);
  });

  test('giving the order up releases its hold and empties the basket', async () => {
    await toOrder();
    await press('주문 그만두기');
    expect(mockCancels).toEqual([{sale: mockSale, session: '01'.repeat(32)}]);
    expect(mockStock).toBe(3);
    expect(screen()).toContain('주문을 그만두었습니다. 이전 주문은 취소되어 잡아 둔 재고가 풀렸습니다.');
    expect(screen()).not.toContain('합계');
    expect(button('저장한 주문 다시 확인')).toBeUndefined();
    expect(button('라떼 iced 추가')).toBeDefined();
  });

  test('an order whose quote was issued is not taken back, and the page says how long its hold stays', async () => {
    await toOrder();
    mockCancel = 'quote_issued';
    await press('새로 주문하기');
    expect(screen()).toContain('새로 주문할 수 있습니다. 이전 주문은 결제 견적이 나와 있어 취소되지 않았습니다. 견적 시간이 끝나면 매장이 닫고, 그때까지는 새 주문이 접수되지 않을 수 있습니다.');
    expect(mockStock).toBe(2);
  });

  test('an order that was paid, or is being checked, is not cancelled here: it stays on the page', async () => {
    await toOrder();
    mockCancel = 'paid';
    await press('새로 주문하기');
    expect(screen()).toContain('이 주문의 결제 요청이 나갔을 수 있어 새로 주문할 수 없습니다. 결과를 먼저 확인하세요.');
    expect(button('저장한 주문 다시 확인')).toBeDefined();
    expect(button('이 주문의 결제 내용 검토')).toBeDefined();
    expect(mockStock).toBe(2);
    await press('주문 그만두기');
    expect(screen()).toContain('이 주문의 결제 요청이 나갔을 수 있어 그만둘 수 없습니다. 결과를 먼저 확인하세요.');
    expect(button('저장한 주문 다시 확인')).toBeDefined();
  });

  test('an order whose hold ran out is left without asking the board or the store, once that is certain', async () => {
    await toOrder();
    mockSessions.clear(); // the checkout session ended
    mockBoardNear = false; // and no board is near to prove the account again
    const proofs = mockProofs.length;
    // Right after the time the store named, the phone's clock may still be ahead of the store's: the cancel is asked.
    await tick(115_000);
    await press('새로 주문하기');
    expect(cancels()).toHaveLength(1);
    expect(screen()).not.toContain('결제 요청이 나갔을 수 있어'); // nothing was sent: the page does not say it might have been
    expect(button('저장한 주문 다시 확인')).toBeDefined();
    // Two minutes past it the hold is over for certain: the order is left here, with no proof and no request.
    await tick(120_000);
    await press('새로 주문하기');
    expect(cancels()).toHaveLength(1);
    expect(mockProofs).toHaveLength(proofs);
    expect(screen()).toContain('새로 주문할 수 있습니다. 이전 주문은 결제 시간이 지나 매장에서 이미 닫혔습니다.');
    expect(button('저장한 주문 다시 확인')).toBeUndefined();
    expect(screen()).toContain('선택 1');
  });

  test('a cancel that did not reach the store is said as that, not as a payment that may have gone out', async () => {
    await toOrder();
    const {default: http} = jest.requireMock('../src/specs/NativeCommerceHttp.ts') as {default: {customerRequest: (...args: string[]) => Promise<string>}};
    const real = http.customerRequest;
    jest.spyOn(http, 'customerRequest').mockImplementation(async (anchor, path, session, body) => path === '/v2/customer/checkout/cancel'
      ? JSON.stringify({status: 500, body: 'oops'}) : real(anchor, path, session, body));
    await press('주문 그만두기');
    expect(screen()).toContain('매장에 주문 취소를 전하지 못했습니다. 주문은 그대로 있습니다. 연결을 확인하고 다시 시도하세요.');
    expect(screen()).not.toContain('결제 요청이 나갔을 수 있어');
    expect(button('저장한 주문 다시 확인')).toBeDefined();
  });

  test('a session that ended is proven again once, and the cart is then taken back', async () => {
    await toOrder();
    mockSessions.clear();
    await press('새로 주문하기');
    expect(mockProofs).toEqual([mockPayer, mockPayer]);
    expect(cancels().map(item => item.session)).toEqual(['01'.repeat(32), '02'.repeat(32)]);
    expect(mockStock).toBe(3);
    expect(screen()).toContain('이전 주문은 취소되어 잡아 둔 재고가 풀렸습니다.');
  });

  test('an order the server does not know as this account\'s is left without a word about a release', async () => {
    await toOrder();
    mockCancel = 'gone';
    await press('새로 주문하기');
    expect(cancels()).toHaveLength(2); // once, and once more with a new proof: not found is also how a session ends
    expect(screen()).toContain('새로 주문할 수 있습니다. 이전 주문은 결제되지 않았고 매장이 닫습니다.');
    expect(screen()).not.toContain('재고가 풀렸습니다');
  });

  test('a busy server leaves the order as it is, to be taken back later', async () => {
    await toOrder();
    const {default: http} = jest.requireMock('../src/specs/NativeCommerceHttp.ts') as {default: {customerRequest: (...args: string[]) => Promise<string>}};
    const real = http.customerRequest;
    jest.spyOn(http, 'customerRequest').mockImplementation(async (anchor, path, session, body) => path === '/v2/customer/checkout/cancel'
      ? JSON.stringify({status: 429, retryAfter: 2, body: JSON.stringify({requestId: '', requestDigest: '', status: 'error', error: 'BUSY'})}) : real(anchor, path, session, body));
    await press('주문 그만두기');
    expect(screen()).toContain('요청이 많습니다. 잠시 뒤 다시 시도하세요.');
    expect(button('저장한 주문 다시 확인')).toBeDefined();
    expect(mockProofs).toEqual([mockPayer]);
  });

  test('a paid order offers no way to cancel it, and no cancel is ever sent for it', async () => {
    await toConfirm();
    await press('내 보드 확인');
    await press('보드로 승인');
    await tick(2500);
    expect(screen()).toContain('주문이 접수되었습니다');
    await press('확인');
    expect(button('새로 주문하기')).toBeUndefined();
    expect(button('주문 그만두기')).toBeUndefined();
    await Renderer.act(async () => { tree!.unmount(); });
    await open({initialOrder: mockSale});
    await tick(0);
    expect(button('새로 주문하기')).toBeUndefined();
    expect(button('주문 그만두기')).toBeUndefined();
    expect(cancels()).toEqual([]);
  });
});
