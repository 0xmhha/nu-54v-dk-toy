import React from 'react';
import Renderer from 'react-test-renderer';
import {Platform} from 'react-native';
import {
  bindServiceRequestV2 as mockBind, signDigest, toBase64 as mockBase64, walletReturnDigest, bytesToHex, type ServiceRequestV2, type WalletReturn,
} from '@nu54/protocol';
import {orderRecords} from '../src/merchant/orderRecords';
import {BoardBalance, BoardLife, setupSteps, type BoardLifeProps} from '../src/wallet/BoardLife';
import {followReturn, newReturn, pendingReset, readResetAnswer, renewReturn, resetRequestText, settleReturn, returnRecord, returnStale, verifyReturn, type ReturnRecord} from '../src/wallet/boardReturn';
import {defaultReadState} from '../src/wallet/readProfiles';
import {decodeWalletStorage, withoutBoardAccount, type WalletStorage} from '../src/wallet/profile';
import {branch as mockBranch, customer as mockCustomer, escrowStatus, generation, now, order, order as mockOrderBase, orderKey, payer, payerKey, policy as mockPolicy, sale,
  terminal as mockTerminal} from './fixtures/order';

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
jest.mock('../src/wallet/evmTransfer.ts', () => ({...jest.requireActual('../src/wallet/evmTransfer.ts'),
  readEvmReceiveBalance: async (_rpc: unknown, _chain: string, _genesis: string, _account: string, asset: string, _decimals?: boolean, tag = 'finalized') => {
    if (mockBalance === undefined) throw new Error('offline');
    mockTokens.push(asset);
    return {amount: tag === 'latest' ? mockLatest ?? mockBalance : mockBalance, block: '1'};
  }}));
jest.mock('../src/specs/NativeShopQr.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  loadProfile: async () => mockEnrolled ? JSON.stringify({scope: {role: 'customer', branchRef: mockBranch, subjectRef: mockCustomer, tenantRef: mockBranch, expiresAt: 4_000_000_000},
    pin: 'a'.repeat(64), payment: mockPolicy}) : '',
  loadJournal: async (name: string) => mockJournals.get(name) ?? '',
  saveJournal: async (name: string, value: string) => { mockJournals.set(name, value); },
  randomBytes: async (length: number) => mockBase64(new Uint8Array(length).fill(mockSeed++ % 250 + 1)),
  exchange: async (_role: string, raw: string) => {
    const {command, approvalSignature} = JSON.parse(raw) as {command: ServiceRequestV2; approvalSignature?: string};
    mockCalls.push(command.operation);
    const {requestId, requestDigest} = mockBind(command, {role: 'customer', branchRef: mockBranch, subjectRef: mockCustomer, tenantRef: mockBranch, expiresAt: 4_000_000_000,
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
const signed = (record: ReturnRecord) => bytesToHex(signDigest(walletReturnDigest({token: record.token, from: record.payer, to: record.to,
  amount: BigInt(record.amount), validBefore: BigInt(record.validBefore), nonce: record.nonce}), payerKey), false);
const fresh = (amount = 42_000_000n) => newReturn({...account, device, token: mockPolicy.asset, to: home}, amount, '7'.repeat(64), 'dededede-dede-4ede-9ede-dededededede', now);
const held = (record: ReturnRecord, state: WalletReturn['state']): WalletReturn =>
  ({payer: record.payer, to: record.to, amount: record.amount, validBefore: record.validBefore, state, ...(state === 'queued' ? {} : {txHash: '0x' + '1'.repeat(64)})} as WalletReturn);

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
  mockJournals.clear(); mockCalls.length = 0; mockSent.length = 0; went.length = 0;
  mockLatest = undefined; mockTokens.length = 0; mockOrders = []; mockServerDown = false; mockBalance = '42000000'; mockHeld = undefined; mockReturnError = ''; mockReturnLost = false; mockEnrolled = true;
  mockPayer = {...account, expiresAt: now + 9999};
  stored = base;
  session = {
    signReturn: jest.fn(async (_generation: bigint, transfer: ReturnRecord, shown: () => void) => { shown(); return signed({...transfer, amount: String(transfer.amount)}); }),
    resetBoard: jest.fn(async () => {}), spendingStatus: jest.fn(async () => ({account: payer, orderOpen: false})), status: jest.fn(async () => ({state: 'released'})),
    operatorResetRequest: jest.fn(async () => ({device, generation, nonce: '3c'.repeat(32)})), operatorReset: jest.fn(async (_nonce: string, _signature: string, shown: () => void) => { shown(); }),
    secondsLeft: jest.fn(() => 110), setReturnAddress: jest.fn(async (_g, _a, address: string) => address),
  };
});
afterEach(async () => { await Renderer.act(async () => { tree?.unmount(); }); tree = undefined; jest.useRealTimers(); });

describe('what is remembered', () => {
  test('the spending board belongs to the verified account and generation', () => {
    expect(decodeWalletStorage(JSON.stringify(base)).spendingBoard).toEqual(base.spendingBoard);
    for (const change of [{account: home}, {generation: 9}, {device: 'ef'.repeat(32)}, {returnAddress: '0x' + '0'.repeat(40)}, {extra: 1}])
      expect(() => decodeWalletStorage(JSON.stringify({...base, spendingBoard: {...base.spendingBoard, ...change}}))).toThrow('WALLET_STORAGE_CORRUPT');
    const cleared = decodeWalletStorage(JSON.stringify(withoutBoardAccount(base)));
    expect([cleared.verified, cleared.spendingBoard, cleared.walletRead?.endpoints]).toEqual([undefined, undefined, base.walletRead!.endpoints]);
  });
  test('setup resumes at the first step not done, and the store can wait', () => {
    const at = (value: WalletStorage, enrolled = true, payerNow?: typeof mockPayer) => setupSteps(value, enrolled, payerNow).map(step => step.state);
    expect(at({version: 2, exercised: false}, false)).toEqual(['now', 'later', 'later', 'later', 'later']);
    expect(setupSteps({version: 2, exercised: false}, true, undefined, true).map(step => step.state)).toEqual(['done', 'now', 'later', 'later', 'later']);
    expect(at({...base, spendingBoard: undefined})).toEqual(['done', 'now', 'later', 'later', 'later']);
    expect(at({...base, spendingBoard: {...base.spendingBoard!, returnAddress: undefined}})).toEqual(['done', 'done', 'now', 'now', 'later']);
    expect(at(base)).toEqual(['done', 'done', 'done', 'now', 'now']);
    expect(at(base, true, {...account, expiresAt: Math.floor(Date.now() / 1000) + 10})).toEqual(['done', 'done', 'done', 'done', 'now']);
    expect(at(base, true, {...account, generation: '99', expiresAt: 4_000_000_000})[3]).toBe('now');
  });
  test('a return is checked, followed and ended only by the server or its deadline', () => {
    const made = fresh();
    const sent: ReturnRecord = {...verifyReturn(made, signed(made)), stage: 'sent', sentAt: now};
    expect(() => verifyReturn({...made, amount: '1'}, signed(made))).toThrow('RETURN_SIGNATURE');
    expect(followReturn(sent, held(sent, 'submitted'), now)?.stage).toBe('sent');
    expect(followReturn(sent, held(sent, 'final'), now)?.stage).toBe('final');
    expect(followReturn(sent, held(sent, 'failed'), now)).toBeUndefined();
    expect(followReturn(sent, {...held(sent, 'final'), amount: '1'}, now)?.stage).toBe('sent');
    expect(followReturn(sent, undefined, sent.validBefore + 120)?.stage).toBe('sent');
    expect(followReturn(sent, undefined, sent.validBefore + 121)).toBeUndefined();
    expect(made.validBefore).toBe(now + 170);
    expect(renewReturn(made, now + 500)).toEqual({...made, validBefore: now + 670});
    expect(returnStale({...sent, stage: 'signed', sentAt: undefined}, made.validBefore - 19)).toBe(true);
    expect(returnStale(sent, made.validBefore)).toBe(false);
  });
  test('the operator texts are exact', () => {
    const nonce = '3c'.repeat(32), request = resetRequestText({device, generation: 7n, nonce});
    expect(request).toBe(`NU54-RESET1:${device}:7:${nonce}`);
    const answer = `NU54-RESET1-OK:${nonce}:${'5a'.repeat(64)}`;
    expect(answer).toHaveLength(208);
    expect(readResetAnswer(` ${answer}\n`, nonce)).toBe('5a'.repeat(64));
    expect(() => readResetAnswer(answer, '4c'.repeat(32))).toThrow('RESET_ANSWER_OTHER');
    for (const bad of [answer + '0', answer.slice(0, -1), answer.toUpperCase(), request]) expect(() => readResetAnswer(bad, nonce)).toThrow(/^RESET_ANSWER$/);
    expect(() => resetRequestText({device: device.slice(1), generation: 7n, nonce})).toThrow('RESET_REQUEST');
    expect(() => resetRequestText({device, generation: 0n, nonce})).toThrow('RESET_REQUEST');
  });
});

describe('deposit and return address', () => {
  test('home shows what the board account holds, and hides it when asked', async () => {
    const show = (value: WalletStorage) => Renderer.act(async () => {
      const next = <BoardBalance t={ko => ko} stored={value} native={{evmRpc: async () => ''} as never} active orders={0} hide={() => { went.push('hide'); }} deposit={() => {}} network={() => { went.push('network'); }} />;
      if (tree) tree.update(next); else tree = Renderer.create(next);
    });
    await show(stored); await flush();
    expect(text()).toContain('42.00 TestUSDC');
    await show({...stored, walletRead: {...stored.walletRead!, selection: {...stored.walletRead!.selection, hidden: true}}});
    expect(text()).not.toContain('42.00');
    mockBalance = undefined;
    await Renderer.act(async () => { tree!.unmount(); });
    tree = undefined;
    await show(stored); await flush();
    expect(text()).toContain('확인 안 됨');
    button('네트워크 주소 설정').props.run();
    expect(went).toEqual(['network']);
    // Without a server chosen by the owner, the one in the store registration is read.
    mockBalance = '42000000';
    const unset = {...stored, walletRead: defaultReadState};
    for (const [rpc, shown] of [[undefined, '확인 안 됨'], ['https://rpc.store.example', '42.00 TestUSDC']] as const) {
      (mockPolicy as {rpc?: string}).rpc = rpc;
      await Renderer.act(async () => { tree!.unmount(); });
      tree = undefined;
      await show(unset); await flush();
      expect(text()).toContain(shown);
    }
    delete (mockPolicy as {rpc?: string}).rpc;
  });

  test('the address appears only after the signing test of this registration', async () => {
    stored = {...base, spendingBoard: undefined};
    await open('deposit');
    expect(text()).not.toContain(payer);
    expect(text()).toContain('아직 입금하지 마세요');
    stored = base;
    await open('deposit');
    expect(text()).toContain(payer);
    expect(text()).toContain('42');
    expect(text()).not.toMatch(/approve|컨트랙트|계약/);
    mockBalance = undefined;
    await open('deposit');
    expect(text()).toContain('확인 안 됨');
  });
  test('the return address is entered twice, shown once more, then stored', async () => {
    stored = {...base, spendingBoard: {...base.spendingBoard!, returnAddress: undefined}};
    await open('return-address');
    const type = async (label: string, value: string) => Renderer.act(async () => { tree!.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onChangeText === 'function')[0].props.onChangeText(value); });
    await type('원래 지갑 주소', home);
    await type('원래 지갑 주소 다시', '0x' + 'ce'.repeat(20));
    await press('주소 확인');
    expect(message).toContain('서로 다릅니다');
    await type('원래 지갑 주소 다시', home);
    await press('주소 확인');
    await press('보드에 저장');
    expect(session.setReturnAddress).toHaveBeenCalledWith(generation, payer, home, expect.any(Function));
    expect(stored.spendingBoard?.returnAddress).toBe(home);
  });
});

describe('setup', () => {
  const active = {state: 'active', owner: 'f0'.repeat(32), device, generation};
  beforeEach(() => Object.assign(session, {status: jest.fn(async () => active), identity: jest.fn(async () => ({fingerprint: active.owner})),
    spendingAccount: jest.fn(async () => payer), spendingStatus: jest.fn(async () => ({account: payer})),
    maintenanceStatus: jest.fn(async () => active), proveAccount: jest.fn(async () => ({account: payer, signature: '11'.repeat(64) + '1b'}))}));

  test('the signing test records the account once and refuses another one of the same registration', async () => {
    stored = {version: 2, exercised: false, mode: 'personal'};
    await open('setup');
    await press('서명 시험 하기');
    expect(stored.verified).toMatchObject({address: payer, generation: Number(generation)});
    expect(stored.spendingBoard).toMatchObject({account: payer, device});
    stored = {...base, spendingBoard: undefined};
    session.spendingAccount.mockResolvedValue(home);
    await open('setup');
    await press('서명 시험 하기');
    expect(message).toContain('저장된 계정과 다릅니다');
    expect(stored.spendingBoard).toBeUndefined();
  });
  test('a new registration after a reset replaces the former account', async () => {
    stored = {...base, verified: {...base.verified!, generation: Number(generation) - 1, address: home}, spendingBoard: undefined};
    await open('setup');
    await press('서명 시험 하기');
    expect(stored.verified).toMatchObject({address: payer, generation: Number(generation)});
    expect(stored.spendingBoard?.returnAddress).toBeUndefined();
  });
  test('a released board is sent back to registration', async () => {
    stored = {...base, spendingBoard: undefined};
    session.status.mockResolvedValue({...active, state: 'released'});
    await open('setup');
    await press('서명 시험 하기');
    expect(message).toContain('먼저 등록하세요');
  });
  test('the store enrollment carries the board proof, and a lost reply is settled by the server', async () => {
    mockPayer = undefined;
    mockReturnLost = true;
    await open('setup');
    await press('이 매장에 보드 등록');
    expect(mockSent[0]).toMatchObject({operation: 'payer.enroll', body: {payer, generation: String(generation)}, approvalSignature: '11'.repeat(64) + '1b'});
    expect(message).toContain('서버의 답을 받지 못했습니다');
    mockPayer = {...account, expiresAt: now + 9999};
    await press('이 매장에 보드 등록');
    expect(message).toContain('등록했습니다');
    expect(text()).toContain('4. 매장 등록, 완료');
  });
  test('without a store registration the rest of setup still works', async () => {
    mockEnrolled = false;
    await open('setup');
    expect(button('결제 탭에서 주문 등록하기')).toBeDefined();
    expect(button('입금 안내 보기')).toBeDefined();
    await press('이 매장에 보드 등록');
    expect(message).toContain('매장 주문 등록이 필요합니다');
  });
});

describe('return and reset', () => {
  test('runs to the end: the transfer, its confirmation, then the reset', async () => {
    await open('return');
    await press('반납 시작');
    expect((await record())?.stage).toBe('signing');
    await press('보드로 승인'); // the signature is written down, then sent at once
    expect(mockSent).toHaveLength(1);
    expect(mockSent[0]).toMatchObject({operation: 'wallet.return', body: {payer, to: home, amount: '42000000'}});
    expect(text()).toContain('반납을 확인하고 있습니다');
    expect(button('보드 초기화')).toBeUndefined();
    const sent = (await record())!;
    expect(sent.signature).toBe(mockSent[0].approvalSignature);
    mockHeld = held(sent, 'final');
    await open('return'); // after a restart
    await press('지금 확인');
    expect(text()).toContain('반납이 확정되었습니다');
    mockBalance = '0';
    await press('보드 초기화');
    expect(session.resetBoard).toHaveBeenCalledTimes(1);
    expect(stored.verified).toBeUndefined();
    expect(await record()).toBeUndefined();
    expect(went).toEqual(['setup']);
  });
  test('an open order, an unreadable record or an unknown transfer stops it', async () => {
    await orderRecords(Http).keep({...account, sale, branch: mockBranch, customer: mockCustomer, confirmer: home, store: 'x', asset: mockPolicy.asset, amount: order.amount,
      escrow: mockPolicy.contract, merchantId: mockPolicy.merchantId, orderKey, sentAt: now, stage: 'locked'});
    await open('return');
    expect(text()).toContain('끝나지 않은 주문이 있습니다');
    expect(button('반납 시작')).toBeUndefined();
    [...mockJournals.keys()].forEach(name => mockJournals.set(name, '{'));
    await open('return').catch(() => {});
    expect(button('반납 시작')).toBeUndefined();
  });
  test('money that arrived later is returned too, before any reset', async () => {
    await returnRecord(Http).save({...verifyReturn(fresh(), signed(fresh())), stage: 'final', sentAt: now});
    mockBalance = '5';
    await open('return');
    await press('보드 초기화');
    expect(session.resetBoard).not.toHaveBeenCalled();
    expect(message).toContain('새로 들어온 돈');
    mockBalance = undefined;
    await returnRecord(Http).save({...verifyReturn(fresh(), signed(fresh())), stage: 'final', sentAt: now});
    await open('return');
    await press('보드 초기화');
    expect(session.resetBoard).not.toHaveBeenCalled();
  });
  test('an empty wallet skips the transfer', async () => {
    mockBalance = '0';
    mockPayer = undefined;
    await open('return');
    await press('잔액 없이 초기화로 진행');
    expect((await record())?.stage).toBe('final');
    await press('보드 초기화');
    expect(mockSent).toHaveLength(0);
    expect(session.signReturn).not.toHaveBeenCalled();
    expect(session.resetBoard).toHaveBeenCalledTimes(1);
  });
  test('an account the store does not know, or an unknown balance, starts nothing', async () => {
    mockPayer = undefined;
    await open('return');
    await press('반납 시작');
    expect(await record()).toBeUndefined();
    expect(message).toContain('등록되어 있지 않습니다');
    mockBalance = undefined;
    await open('return');
    expect(button('반납 시작').props.disabled).toBe(true);
  });
  test('a refused return may be cancelled, a lost one only followed', async () => {
    const ready = verifyReturn(fresh(), signed(fresh()));
    await returnRecord(Http).save(ready);
    mockReturnError = 'CHECKOUT_CONFLICT';
    await open('return');
    await press('반납 보내기');
    expect(await record()).toMatchObject({stage: 'signed'});
    expect((await record())?.sentAt).toBeUndefined();
    expect(message).toContain('끝나지 않은 주문');
    mockReturnError = '';
    mockReturnLost = true;
    await open('return');
    await press('반납 보내기');
    expect((await record())?.stage).toBe('sent');
    expect(button('반납 취소')).toBeUndefined();
    mockReturnLost = false;
    await press('같은 반납 다시 보내기');
    expect(mockSent[2].idempotencyKey).toBe(mockSent[1].idempotencyKey);
    mockHeld = held((await record())!, 'failed');
    await press('지금 확인');
    expect(await record()).toBeUndefined();
    expect(message).toContain('그대로 있습니다');
  });
  test('a sent return is followed without its page, and never over a newer state', async () => {
    const sent: ReturnRecord = {...verifyReturn(fresh(), signed(fresh())), stage: 'sent', sentAt: now};
    await returnRecord(Http).save(sent);
    const client = (state: WalletReturn['state'], meanwhile?: () => Promise<void>) => ({walletReturn: async () => { await meanwhile?.(); return held(sent, state); }}) as never;
    expect((await settleReturn(Http, client('submitted'), () => now))?.stage).toBe('sent');
    await settleReturn(Http, client('final', () => returnRecord(Http).save({...sent, stage: 'resetting'})), () => now);
    expect((await record())?.stage).toBe('resetting');
    await returnRecord(Http).save(sent);
    expect((await settleReturn(Http, client('final'), () => now))?.stage).toBe('final');
  });
  test('a lost reset reply withholds the address until the board is asked again', async () => {
    await returnRecord(Http).save({...fresh(0n)});
    mockBalance = '0';
    session.resetBoard.mockRejectedValue(new Error('OWNER_DISCONNECTED'));
    await open('return');
    await press('보드 초기화');
    expect((await record())?.stage).toBe('resetting');
    expect(stored.spendingBoard?.withheld).toBe(true);
    expect(stored.verified).toBeDefined();
    await press('보드 상태 확인');
    expect(stored.verified).toBeUndefined();
    expect(await record()).toBeUndefined();
  });
  test('a refused reset changes nothing', async () => {
    await returnRecord(Http).save({...fresh(0n)});
    mockBalance = '0';
    session.resetBoard.mockRejectedValue(new Error('BOARD_REFUSED'));
    await open('return');
    await press('보드 초기화');
    expect((await record())?.stage).toBe('final');
    expect(stored.spendingBoard).toEqual(base.spendingBoard);
  });
  const keepOrder = () => orderRecords(Http).keep({...account, sale, branch: mockBranch, customer: mockCustomer, confirmer: home, store: 'x', asset: mockPolicy.asset, amount: order.amount,
    escrow: mockPolicy.contract, merchantId: mockPolicy.merchantId, orderKey, sentAt: now, stage: 'locked'});
  const brokenOrders = () => { for (const [name, value] of mockJournals) if (value.includes('"sale"')) mockJournals.set(name, '{'); };
  test('the key is erased only when the newest and the finalized block both show nothing left of that token', async () => {
    const other = '0x' + '77'.repeat(20);
    await returnRecord(Http).save({...fresh(0n), token: other});
    mockBalance = '0';
    mockLatest = '5';
    await open('return');
    await press('보드 초기화');
    expect(session.resetBoard).not.toHaveBeenCalled();
    expect(message).toContain('확정되지 않은');
    mockLatest = '0';
    mockTokens.length = 0;
    await press('보드 초기화');
    expect(session.resetBoard).toHaveBeenCalledTimes(1);
    expect(new Set(mockTokens)).toEqual(new Set([other]));
  });
  test('money still moving starts no return', async () => {
    mockLatest = '1';
    await open('return');
    await press('반납 시작');
    expect(await record()).toBeUndefined();
    expect(message).toContain('확정되지 않은');
  });
  for (const screen of ['reset', 'operator-reset'] as const) {
    const start = screen === 'reset' ? '앱에서 초기화 계속' : '초기화 요청 만들기';
    test(`${screen}: an open order on the phone hides the way on`, async () => {
      await keepOrder();
      await open(screen);
      expect(button(start)).toBeUndefined();
      await press('진행 중인 주문 보기');
      expect(went).toContain('orders');
    });
    test(`${screen}: an open order only the server knows hides the way on`, async () => {
      mockOrders = [...Array.from({length: 60}, (_, index) => ({id: `dededede-dede-4ede-9ede-${String(index).padStart(12, '0')}`, state: 'closed'})),
        {id: sale, state: 'paid', escrow: {...escrowStatus('locked', home)}}]; // on the second page
      await open(screen);
      for (let i = 0; i < 10; i++) await flush();
      expect(button(start)).toBeUndefined();
      expect(text()).toContain('끝나지 않은 주문이 있습니다');
      mockOrders = [{id: sale, state: 'paid', escrow: {...escrowStatus('paid', home)}}];
      await open(screen);
      expect(button(start)).toBeDefined();
    });
    test.each(['signed', 'sent'] as const)(`${screen}: a return in state %s hides the way on`, async stage => {
      await returnRecord(Http).save({...verifyReturn(fresh(), signed(fresh())), stage, ...(stage === 'sent' ? {sentAt: now} : {})});
      await open(screen);
      expect(button(start)).toBeUndefined();
    });
    test(`${screen}: a return only the server knows hides the way on`, async () => {
      mockHeld = held(fresh(), 'submitted');
      await open(screen);
      expect(button(start)).toBeUndefined();
    });
    test(`${screen}: unreadable records or a silent server need their own confirmation`, async () => {
      for (const broken of ['orders', 'server']) {
        mockJournals.clear();
        mockServerDown = false;
        if (broken === 'orders') { await keepOrder(); brokenOrders(); } else mockServerDown = true;
        await open(screen);
        expect(button(start)).toBeUndefined();
        await press('환불을 잃을 수 있음을 알고 계속');
        expect(button(start)).toBeDefined();
      }
    });
  }
  test('a board call is stopped by an order that appeared after the page was opened', async () => {
    await open('reset');
    await press('앱에서 초기화 계속');
    await keepOrder();
    await press('보드 초기화');
    expect(session.resetBoard).not.toHaveBeenCalled();
  });
  test('the factory reset warns and asks twice', async () => {
    await open('reset');
    expect(text()).toContain('잃습니다');
    expect(button('보드 초기화')).toBeUndefined();
    await press('앱에서 초기화 계속');
    await press('보드 초기화');
    expect(session.resetBoard).toHaveBeenCalledTimes(1);
    expect(stored.verified).toBeUndefined();
  });
  test('the operator reset shows the request, takes only its answer and only for its board', async () => {
    await open('operator-reset');
    await press('초기화 요청 만들기');
    expect(text()).toContain(`NU54-RESET1:${device}:${generation}:${'3c'.repeat(32)}`);
    const answer = async (value: string) => {
      await Renderer.act(async () => { tree!.root.findAll(node => node.props.accessibilityLabel === '운영자의 답' && typeof node.props.onChangeText === 'function')[0].props.onChangeText(value); });
      await press('이 답으로 보드 초기화');
    };
    session.status.mockResolvedValue({state: 'active', device, generation});
    await open('operator-reset'); // the request outlives the page and the connection
    await answer(`NU54-RESET1-OK:${'4c'.repeat(32)}:${'5a'.repeat(64)}`);
    expect(message).toContain('이전 요청의 답');
    expect(session.operatorReset).not.toHaveBeenCalled();
    session.status.mockResolvedValue({state: 'active', device: 'ef'.repeat(32), generation});
    await answer(`NU54-RESET1-OK:${'3c'.repeat(32)}:${'5a'.repeat(64)}`);
    expect(session.operatorReset).not.toHaveBeenCalled();
    session.status.mockResolvedValue({state: 'active', device, generation});
    await press('이 답으로 보드 초기화');
    expect(session.operatorReset).toHaveBeenCalledWith('3c'.repeat(32), '5a'.repeat(64), expect.any(Function));
    expect(stored.verified).toBeUndefined();
    expect(await pendingReset(Http).load()).toBeUndefined();
  });
  test('a request the board never took is kept', async () => {
    session.status.mockResolvedValue({state: 'active', device, generation});
    await open('operator-reset');
    await press('초기화 요청 만들기');
    await Renderer.act(async () => { tree!.root.findAll(node => node.props.accessibilityLabel === '운영자의 답' && typeof node.props.onChangeText === 'function')[0].props.onChangeText(`NU54-RESET1-OK:${'3c'.repeat(32)}:${'5a'.repeat(64)}`); });
    session.spendingStatus.mockResolvedValue({account: payer, orderOpen: true});
    await press('이 답으로 보드 초기화');
    expect(session.operatorReset).not.toHaveBeenCalled();
    expect(message).toContain('최대 4분');
    expect(await pendingReset(Http).load()).toBeDefined();
    session.spendingStatus.mockResolvedValue({account: payer, orderOpen: false});
    session.operatorReset.mockRejectedValue(new Error('BOARD_SESSION_SHORT'));
    await press('이 답으로 보드 초기화');
    expect(await pendingReset(Http).load()).toBeDefined();
    expect(stored.spendingBoard).toEqual(base.spendingBoard);
    // A phone that is not the owner cannot tell the wait for a payment from a wrong answer.
    session.spendingStatus.mockRejectedValue(new Error('PAYMENT_BOARD_BINDING'));
    session.operatorReset.mockImplementation(async (_nonce: string, _signature: string, shown: () => void) => { shown(); throw new Error('OWNER_DISCONNECTED'); });
    await press('이 답으로 보드 초기화');
    expect(await pendingReset(Http).load()).toBeDefined();
    session.spendingStatus.mockResolvedValue({account: payer, orderOpen: false});
    session.operatorReset.mockImplementation(async (_nonce: string, _signature: string, shown: () => void) => { shown(); throw new Error('BOARD_REFUSED'); });
    await press('이 답으로 보드 초기화');
    expect(await pendingReset(Http).load()).toBeUndefined();
  });
  test('a used, replaced or ended request is not offered again', async () => {
    session.status.mockResolvedValue({state: 'active', device, generation});
    session.operatorReset.mockImplementation(async (_nonce: string, _signature: string, shown: () => void) => { shown(); throw new Error('OWNER_DISCONNECTED'); });
    await open('operator-reset');
    await press('초기화 요청 만들기');
    await Renderer.act(async () => { tree!.root.findAll(node => node.props.accessibilityLabel === '운영자의 답' && typeof node.props.onChangeText === 'function')[0].props.onChangeText(`NU54-RESET1-OK:${'3c'.repeat(32)}:${'5a'.repeat(64)}`); });
    await press('이 답으로 보드 초기화');
    expect(message).toContain('보드가 답하지 않았습니다');
    expect(stored.verified).toBeDefined();
    expect(stored.spendingBoard?.withheld).toBe(true);
    expect(await pendingReset(Http).load()).toBeUndefined();
    await press('초기화 요청 만들기');
    await Renderer.act(async () => { await jest.advanceTimersByTimeAsync(841_000); });
    expect(text()).toContain('시간이 끝났습니다');
    expect(button('이 답으로 보드 초기화')).toBeUndefined();
  });
});
