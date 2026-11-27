import React from 'react';
import {AppState} from 'react-native';
import Renderer from 'react-test-renderer';
import {keccak_256} from '@noble/hashes/sha3';
import {
  accountProofDigest as mockProofDigest, bindServiceRequestV2 as mockBind, bytesToBig, bytesToHex as mockHex, hexToBytes as mockBytes, quoteV2Layout as q,
  recoverSigner as mockRecover, signDigest as mockSign, toBase64 as mockBase64, utf8Encode, type CommerceOrder, type ServiceRequestV2,
} from '@nu54/protocol';
import {CustomerOrders} from '../src/merchant/CustomerOrders';
import {customerSubject} from '../src/merchant/storeAccess';
import {readProfiles} from '../src/wallet/readProfiles';
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
const mockLocks: (ServiceRequestV2 & {approvalSignature?: string})[] = [];
const mockAnswers: ServiceRequestV2[] = [];

function mockOrderNow(): CommerceOrder {
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
  loadJournal: async (name: string) => mockJournals.get(name) ?? '',
  saveJournal: async (name: string, value: string) => { mockJournals.set(name, value); },
  randomBytes: async (length: number) => mockBase64(new Uint8Array(length).fill(7)),
  exchange: async () => { throw new Error('a registered connection was used'); },
  // The customer entrance of the store server, as the native module reaches it with this build's anchor.
  customerRequest: async (anchor: string, path: string, session: string, body: string) => {
    if (JSON.parse(anchor).origin !== mockAnchor.origin) throw new Error('another server');
    if (mockEntranceDown) throw new Error('offline');
    const answer = (status: number, value: unknown) => JSON.stringify({status, body: JSON.stringify(value)});
    if (!body) {
      mockSent.push({path});
      if (path === `/v2/customer/stores/${mockBranch}/menu/0`) return answer(200, {shop: {branch: mockBranch, branchLabel: '그린 카페'}, orderState: {mode: 'open'},
        catalogVersion: 1, catalog: [{sku: 'latte', optionKey: 'iced', label: '라떼', asset: mockIssued.asset, version: 1, unitPrice: mockMenuPrice,
          available: mockStock, skuAvailable: mockStock, expiresAt: new Date((mockNow + 3600) * 1000).toISOString()}]});
      return answer(404, null);
    }
    const sent = JSON.parse(body);
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
    const allowed = held?.purpose === 'checkout' ? ['order.create', 'checkout.claim', 'checkout.authorize', 'quote.issue', 'order.lock', 'order.status']
      : held?.purpose === 'order' && held.sale === command.objectRef ? ['order.status', 'order.answer'] : [];
    if (!allowed.includes(command.operation)) return answer(404, null);
    // The server binds its answer to the session's customer and tenant, never to what the phone claims.
    const {requestId, requestDigest} = mockBind(command, {role: 'customer', tenantRef: mockTenant, subjectRef: mockSubject, expiresAt: mockNow + 900,
      grants: [{domainRef: command.domainRef, objectRef: command.objectRef, operations: [`C3/${command.operation}`]}]}, Math.floor(Date.now() / 1000));
    const ok = {requestId, requestDigest, status: 'ok', resultRef: command.objectRef};
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
  mockStage = 'none'; mockConfirmer = ''; mockOrigin = ''; mockStock = 3; mockMenuPrice = mockIssued.amount; mockSessionSeed = 1; mockEntranceDown = false; mockQuoteChange = undefined; mockHeld = false;
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
  expect(sentPaths()).toEqual(['stores/STORE/menu/0', 'challenge', 'proof', 'stores/STORE/menu/0', 'order.create', 'order.status']);
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
