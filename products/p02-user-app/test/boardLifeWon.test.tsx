import React from 'react';
import Renderer from 'react-test-renderer';
import {Platform} from 'react-native';
import {
  bindServiceRequestV2 as mockBind, dollarToken, namedTokenAbiSha256 as mockNamed, signDigest, toBase64 as mockBase64, walletReturnDigestFor, bytesToHex, type ServiceRequestV2, type WalletReturn, type WalletSend,
} from '@nu54/protocol';
import {orderRecords} from '../src/merchant/orderRecords';
import {BoardLife, type BoardLifeProps} from '../src/wallet/BoardLife';
import {newReturn, returnRecord, verifyReturn, type ReturnRecord} from '../src/wallet/boardReturn';
import {sendRecord, type SendRecord} from '../src/wallet/boardSend';
import {defaultReadState} from '../src/wallet/readProfiles';
import {decodeWalletStorage, type WalletStorage} from '../src/wallet/profile';
import {branch as mockBranch, customer as mockCustomer, generation, now, order as mockOrderBase, payer, payerKey, policy as mockPolicy,
  terminal as mockTerminal} from './fixtures/order';

const mockWonPolicy = {...mockPolicy, abiSha256: mockNamed, token: {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2}};
let mockStore: 'won' | 'usd' | 'other' | 'v2' = 'won', mockBroken = false, mockShop = mockBranch;
const mockJournals = new Map<string, string>();
const mockCalls: string[] = [];
const mockSent: (ServiceRequestV2 & {approvalSignature?: string})[] = [];
let mockBalance: string | undefined = '42000000';
let mockHeld: WalletReturn | undefined;
let mockPayer: {payer: string; generation: string; expiresAt: number} | undefined;
let mockReturnError = '';
let mockReturnLost = false;
let mockEnrolled = true;
let mockSeed = 1;
let mockLatest: string | undefined; // what the newest block says, when it differs from the finalized one
const mockTokens: string[] = [];
let mockOrders: {id: string; state: string; escrow?: unknown}[] = [];
let mockServerDown = false;
let mockSends: WalletSend[] = [], mockPerDay = 3, mockSendError = '', mockSendLost = false, mockRelays = true;
let mockOther: Record<string, string> = {}; // balances of assets the board cannot return
jest.mock('../src/wallet/evmTransfer.ts', () => ({...jest.requireActual('../src/wallet/evmTransfer.ts'),
  readEvmReceiveBalance: async (_rpc: unknown, _chain: string, _genesis: string, _account: string, asset: string, _decimals?: boolean, tag?: string) => {
    if (mockBalance === undefined) throw new Error('offline');
    // The account's own coin and the tokens the owner added are read without a tag.
    if (!tag) return {amount: mockOther[asset ?? 'coin'] ?? '0', block: '1'};
    mockTokens.push(asset);
    return {amount: tag === 'latest' ? mockLatest ?? mockBalance : mockBalance, block: '1'};
  }}));
jest.mock('../src/specs/NativeShopQr.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  loadProfile: async () => mockBroken ? Promise.reject(new Error('keystore')) : mockEnrolled ? JSON.stringify({scope: {role: 'customer', branchRef: mockShop, subjectRef: mockCustomer, tenantRef: mockBranch, expiresAt: 4_000_000_000},
    pin: 'a'.repeat(64), payment: mockStore === 'usd' ? mockPolicy : mockStore === 'v2' ? {...mockPolicy, abiSha256: undefined} : mockStore === 'other' ? {...mockWonPolicy, asset: '0x' + '7'.repeat(40)} : mockWonPolicy}) : '',
  loadJournal: async (name: string) => mockJournals.get(name) ?? '',
  saveJournal: async (name: string, value: string) => { mockJournals.set(name, value); },
  randomBytes: async (length: number) => mockBase64(new Uint8Array(length).fill(mockSeed++ % 250 + 1)),
  exchange: async (_role: string, raw: string) => {
    const {command, approvalSignature} = JSON.parse(raw) as {command: ServiceRequestV2; approvalSignature?: string};
    mockCalls.push(command.operation);
    const {requestId, requestDigest} = mockBind(command, {role: 'customer', branchRef: mockShop, subjectRef: mockCustomer, tenantRef: mockBranch, expiresAt: 4_000_000_000,
      grants: [{domainRef: command.domainRef, objectRef: command.objectRef, operations: [`C3/${command.operation}`]}]} as never, Math.floor(Date.now() / 1000));
    const ok = {requestId, requestDigest, status: 'ok', resultRef: command.objectRef};
    if (mockServerDown) throw new Error('offline');
    if (command.operation === 'order.list') return JSON.stringify({...ok, orders: mockOrders.filter(row => row.id > String(command.body.afterSale)).slice(0, 50).map(({id, state}) => ({id, state, amount: '4500000', asset: mockPolicy.asset}))});
    if (command.operation === 'order.status') {
      const {id, state, escrow} = mockOrders.find(row => row.id === command.objectRef)!;
      return JSON.stringify({...ok, order: {...mockOrderBase, quoteDigest: undefined, orderKey: undefined, id, state, pickupNo: 17, receiptRef: mockTerminal, escrow,
        payment: {stage: 'finalized', chainId: 8283, txHash: '0x' + '1'.repeat(64), includedAt: new Date(1_800_000_000_000).toISOString()}}});
    }
    if (command.operation === 'wallet.return-status') return JSON.stringify({...ok, ...(mockHeld ? {walletReturn: mockHeld} : {})});
    if (command.operation === 'wallet.send-status') return JSON.stringify(mockRelays
      ? {...ok, walletSends: {perDay: mockPerDay, remaining: Math.max(0, mockPerDay - mockSends.length), sends: mockSends}}
      : {...ok, status: 'error', error: 'UNSUPPORTED_CAPABILITY', resultRef: undefined});
    if (command.operation === 'wallet.send') {
      mockSent.push({...command, approvalSignature});
      if (mockSendLost) throw new Error('lost');
      return JSON.stringify(mockSendError ? {...ok, status: 'error', error: mockSendError, resultRef: undefined} : ok);
    }
    if (command.operation === 'payer.enrolled' || command.operation === 'payer.status') return JSON.stringify({...ok, ...(mockPayer ? {payer: mockPayer} : {})});
    if (command.operation === 'wallet.return' || command.operation === 'payer.enroll') {
      mockSent.push({...command, approvalSignature});
      if (mockReturnLost) throw new Error('lost');
      return JSON.stringify(mockReturnError ? {...ok, status: 'error', error: mockReturnError, resultRef: undefined} : ok);
    }
    throw new Error('unexpected ' + command.operation);
  },
}}));

const Http = jest.requireMock<{default: Parameters<typeof returnRecord>[0] & Parameters<typeof orderRecords>[0]}>('../src/specs/NativeCommerceHttp.ts').default;
const device = 'ab'.repeat(32), home = '0x' + 'cd'.repeat(20), account = {payer, generation: generation.toString()};
const base: WalletStorage = {version: 2, exercised: false, mode: 'personal',
  verified: {boardFingerprint: device, address: payer, generation: Number(generation), network: '8283'},
  spendingBoard: {device, account: payer, generation: Number(generation), testedAt: now, returnAddress: home},
  walletRead: {...defaultReadState, endpoints: {stable: 'https://rpc.example'}}};
let boardName = 'NU54 Test KRW';
const signed = (record: ReturnRecord) => bytesToHex(signDigest(walletReturnDigestFor(boardName, {token: record.token, from: record.payer, to: record.to,
  amount: BigInt(record.amount), validBefore: BigInt(record.validBefore), nonce: record.nonce}), payerKey), false);
const fresh = (amount = 42_000_000n) => newReturn({...account, device, token: mockPolicy.asset, to: home}, amount, '7'.repeat(64), 'dededede-dede-4ede-9ede-dededededede', now);

let stored: WalletStorage, tree: Renderer.ReactTestRenderer | undefined, message = '', session: Record<string, jest.Mock>;
const went: string[] = [];
let props: BoardLifeProps;
const text = () => tree?.toJSON() ? JSON.stringify(tree.toJSON()) : '';
const flush = () => Renderer.act(async () => { for (let i = 0; i < 40; i++) await Promise.resolve(); });
const render = async () => { await Renderer.act(async () => { const next = <BoardLife {...props} stored={stored} />; if (tree) tree.update(next); else tree = Renderer.create(next); }); await flush(); };
async function open(screen: BoardLifeProps['screen']) {
  if (tree) await Renderer.act(async () => { tree!.unmount(); });
  tree = undefined;
  message = '';
  props = {screen, stored, native: {evmRpc: async () => ''} as never, hasEnrollment: true, found: [{address: 'board', name: 'NU54'}], busy: false,
    scan() {}, go: next => { went.push(next); }, orders() { went.push('orders'); }, say: value => { message = value; }, wait() {}, copy() {}, seen() {}, seenBoard: {account: payer, generation: Number(generation), at: Date.now()},
    work: (run, failure) => { run().catch(error => { message = failure(error); }); },
    withBoard: async (_handle, run) => run(session as never),
    commit: async change => { stored = decodeWalletStorage(JSON.stringify(change(stored))); }};
  await render();
}
const button = (label: string) => tree!.root.findAll(node => node.props.label === label && typeof node.props.run === 'function')[0];
const press = async (label: string) => {
  expect([label, !!button(label), button(label)?.props.disabled ?? false]).toEqual([label, true, false]);
  await Renderer.act(async () => { button(label).props.run(); });
  await flush();
  await render();
};
const record = () => returnRecord(Http).load();

beforeEach(() => {
  jest.useFakeTimers({now: now * 1000});
  Object.defineProperty(Platform, 'OS', {value: 'android', configurable: true});
  mockStore = 'won'; mockBroken = false; mockShop = mockBranch; boardName = 'NU54 Test KRW';
  mockJournals.clear(); mockCalls.length = 0; mockSent.length = 0; went.length = 0;
  mockLatest = undefined; mockOther = {}; mockSends = []; mockPerDay = 3; mockSendError = ''; mockSendLost = false; mockRelays = true; mockTokens.length = 0; mockOrders = []; mockServerDown = false; mockBalance = '42000000'; mockHeld = undefined; mockReturnError = ''; mockReturnLost = false; mockEnrolled = true;
  mockPayer = {...account, expiresAt: now + 9999};
  stored = base;
  session = {
    signSend: jest.fn(async (_generation: bigint, transfer: SendRecord, shown: () => void) => { shown(); return signed({...transfer, amount: String(transfer.amount)} as never); }),
    signReturn: jest.fn(async (_generation: bigint, transfer: ReturnRecord, shown: () => void) => { shown(); return signed({...transfer, amount: String(transfer.amount)}); }),
    resetBoard: jest.fn(async () => {}), spendingStatus: jest.fn(async () => ({account: payer, orderOpen: false})), status: jest.fn(async () => ({state: 'released'})),
    operatorResetRequest: jest.fn(async () => ({device, generation, nonce: '3c'.repeat(32)})), operatorReset: jest.fn(async (_nonce: string, _signature: string, shown: () => void) => { shown(); }),
    secondsLeft: jest.fn(() => 110), setReturnAddress: jest.fn(async (_g, _a, address: string) => address),
  };
});
afterEach(async () => { await Renderer.act(async () => { tree?.unmount(); }); tree = undefined; jest.useRealTimers(); });


const friend = '0x' + 'f1'.repeat(20), won = 'NU54 Test KRW';
const fill = async (label: string, value: string) => {
  await Renderer.act(async () => { tree!.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onChangeText === 'function')[0].props.onChangeText(value); });
  await render();
};
const start = async (amount = '50000') => { await open('send'); await fill('받는 주소', friend); await fill('금액', amount); await press('다음'); };

test('a return is shown in won and signed as won', async () => {
  await open('return');
  expect(text()).toContain('420,000원');
  await press('반납 시작');
  expect(text()).toContain('420,000원');
  expect(text()).not.toContain('TestUSDC');
  await press('보드로 승인');
  expect(session.signReturn.mock.calls[0][3]).toBe(won);
  expect(mockSent[0]).toMatchObject({operation: 'wallet.return', body: {amount: '42000000'}});
});
test('a board that signed dollars returns nothing', async () => {
  boardName = dollarToken.name;
  await open('return');
  await press('반납 시작');
  await press('보드로 승인');
  expect(mockSent).toHaveLength(0);
  expect((await record())?.stage).toBe('signing');
});
test('a signed return that was not sent is sent again as won, from the record', async () => {
  const asked = fresh();
  await returnRecord(Http).save(verifyReturn(asked, signed(asked), won));
  await open('return');
  expect(text()).toContain('420,000원');
  await press('반납 보내기');
  expect(mockSent).toHaveLength(1);
});
test('the same return is repeated as won', async () => {
  const asked = fresh();
  await returnRecord(Http).save({...verifyReturn(asked, signed(asked), won), stage: 'sent', sentAt: now});
  await open('return');
  await press('같은 반납 다시 보내기');
  expect(mockSent).toHaveLength(1);
});
test('a return of another store is neither signed nor shown as this store\'s money', async () => {
  const asked = fresh();
  await returnRecord(Http).save(asked);
  mockStore = 'other';
  await open('return');
  expect(text()).toContain('확인 안 됨');
  expect(text()).not.toContain('420,000원');
  await press('보드로 승인');
  expect(message).toContain('다른 매장에 등록되어');
  expect(session.signReturn).not.toHaveBeenCalled();
});
test('a transfer takes won with two decimals and is signed as won', async () => {
  await start('1.001');
  expect(message).toContain('소수점 아래 두 자리');
  await start();
  expect(text()).toContain('50,000원');
  expect(text()).toContain('370,000원');
  await press('이 내용으로 보내기');
  const kept = (await sendRecord(Http).load())!;
  expect(kept).toMatchObject({amount: '5000000', token: mockPolicy.asset, stage: 'signing'});
  expect(text()).toContain('50,000원');
  await press('보드로 승인');
  expect(session.signSend.mock.calls[0][3]).toBe(won);
  expect(mockSent.at(-1)).toMatchObject({operation: 'wallet.send', body: {amount: '5000000'}});
  expect(text()).not.toContain('TestUSDC');
});
test('a board that signed dollars sends nothing', async () => {
  boardName = dollarToken.name;
  await start();
  await press('이 내용으로 보내기');
  await press('보드로 승인');
  expect(mockSent).toHaveLength(0);
});
test('a signed transfer is sent and repeated as won', async () => {
  await start();
  await press('이 내용으로 보내기');
  mockSendLost = true;
  await press('보드로 승인');
  expect(mockSent).toHaveLength(1);
  mockSendLost = false;
  await open('send');
  await press('같은 거래 다시 보내기');
  expect(mockSent).toHaveLength(2);
  expect(mockSent[1].approvalSignature).toBe(mockSent[0].approvalSignature);
  await sendRecord(Http).save({...(await sendRecord(Http).load())!, stage: 'signed', sentAt: undefined});
  await open('send');
  await press('보내기');
  expect(mockSent).toHaveLength(3);
});
test('nothing is signed when the registration changed under the page', async () => {
  await start();
  mockStore = 'usd'; // the same token address now registered as dollars
  await press('이 내용으로 보내기');
  expect(message).toContain('화면의 토큰과 매장 등록의 토큰이 다르거나');
  expect(await sendRecord(Http).load()).toBeUndefined();
  mockStore = 'won';
  await press('이 내용으로 보내기');
  mockStore = 'usd';
  await press('보드로 승인');
  expect(session.signSend).not.toHaveBeenCalled();
  mockStore = 'won';
  await sendRecord(Http).save(undefined);
  await open('return');
  mockStore = 'usd';
  await press('반납 시작');
  expect(message).toContain('화면의 토큰과');
  expect(await record()).toBeUndefined();
  mockStore = 'won';
  await press('반납 시작');
  mockStore = 'usd';
  await press('보드로 승인');
  expect(session.signReturn).not.toHaveBeenCalled();
});
test('a reset warns of the money in won', async () => {
  await open('reset');
  expect(text()).toContain('420,000원');
});
const elsewhere = '0fbc54ef-2827-4ab5-bfcd-3a27611c39ff';
test('a return sent through another store is never called failed and never sent again', async () => {
  const asked = fresh();
  await returnRecord(Http).save(verifyReturn(asked, signed(asked), won));
  await open('return');
  await press('반납 보내기');
  expect((await record())).toMatchObject({stage: 'sent', branch: mockBranch});
  mockShop = elsewhere;
  jest.setSystemTime((now + 100_000) * 1000);
  await open('return');
  expect(text()).toContain('결과 확인 안 됨');
  expect(text()).toContain('돈이 이미 나갔을 수 있습니다');
  expect(button('같은 반납 다시 보내기')).toBeUndefined();
  expect(text()).not.toContain('빠져나간 돈은 없습니다');
  const {settleReturn, submitReturn} = jest.requireActual<typeof import('../src/wallet/boardReturn')>('../src/wallet/boardReturn');
  const client = {profile: {branchRef: elsewhere}, walletReturn: async () => undefined, returnWallet: jest.fn()};
  expect((await settleReturn(Http, client as never, () => now + 100_000))?.stage).toBe('sent');
  await expect(submitReturn(client as never, (await record())!, async () => {}, now, won)).rejects.toThrow('SENT_AT_OTHER_STORE');
  expect(client.returnWallet).not.toHaveBeenCalled();
  expect((await settleReturn(Http, {...client, profile: {branchRef: mockBranch}} as never, () => now + 100_000))).toBeUndefined();
});
test('a transfer sent through another store is never called failed and never sent again', async () => {
  await start();
  await press('이 내용으로 보내기');
  await press('보드로 승인');
  expect(await sendRecord(Http).load()).toMatchObject({stage: 'sent', branch: mockBranch});
  mockShop = elsewhere;
  jest.setSystemTime((now + 100_000) * 1000);
  await open('send');
  expect(text()).toContain('결과 확인 안 됨');
  expect(button('같은 거래 다시 보내기')).toBeUndefined();
  const {settleSend, submitSend} = jest.requireActual<typeof import('../src/wallet/boardSend')>('../src/wallet/boardSend');
  const client = {profile: {branchRef: elsewhere}, walletSends: async () => ({perDay: 3, remaining: 3, sends: []}), sendWallet: jest.fn()};
  expect((await settleSend(Http, client as never, () => now + 100_000))?.stage).toBe('sent');
  await expect(submitSend(client as never, (await sendRecord(Http).load())!, async () => {}, now, won)).rejects.toThrow('SENT_AT_OTHER_STORE');
  expect(client.sendWallet).not.toHaveBeenCalled();
  expect((await settleSend(Http, {...client, profile: {branchRef: mockBranch}} as never, () => now + 100_000))?.stage).toBe('failed');
});
test('a transfer of another asset is not shown as this store\'s money', async () => {
  await start();
  await press('이 내용으로 보내기');
  mockStore = 'other';
  await open('send');
  expect(text()).toContain('확인 안 됨');
  expect(text()).not.toContain('50,000원');
  await press('보드로 승인');
  expect(message).toContain('다른 매장에 등록되어');
  expect(session.signSend).not.toHaveBeenCalled();
});
test('a registration that could not be read signs nothing and can be read again', async () => {
  const asked = fresh();
  await returnRecord(Http).save(asked);
  mockBroken = true;
  await open('return');
  expect(text()).toContain('확인 안 됨');
  mockBroken = false; // readable when the button is pressed, but the page still does not know the token
  await press('보드로 승인');
  expect(message).toContain('화면의 토큰과');
  expect(session.signReturn).not.toHaveBeenCalled();
  mockBroken = true;
  await open('send');
  mockBroken = false;
  await open('send');
  expect(text()).toContain('420,000원');
});
test('the store must be the same asset, not only the same token', async () => {
  const {sameStore, storeOf} = jest.requireActual<typeof import('../src/wallet/tokens')>('../src/wallet/tokens');
  const here = storeOf(mockWonPolicy);
  expect([sameStore(here, storeOf(mockWonPolicy, elsewhere)), sameStore(here, storeOf({...mockWonPolicy, asset: '0x' + '7'.repeat(40)})), sameStore(undefined, here)]).toEqual([true, false, false]);
});
test('a dollar store with the older contract still shows dollars', async () => {
  mockStore = 'v2';
  await open('deposit');
  expect(text()).toContain('TestUSDC');
  expect(text()).toContain(payer);
});
test('without a registration no amount is guessed and nothing can be sent', async () => {
  mockEnrolled = false;
  await open('send');
  expect(text()).toContain('매장 주문 등록이 필요합니다');
  expect(button('이 내용으로 보내기')).toBeUndefined();
  await open('deposit');
  expect(text()).toContain('StableNet 테스트넷으로만');
  await press('확인했습니다. 주소 보기');
  expect(text()).toContain('확인 안 됨');
  expect(text()).not.toContain('TestUSDC');
});
