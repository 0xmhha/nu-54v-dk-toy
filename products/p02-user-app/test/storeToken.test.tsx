import React from 'react';
import Renderer from 'react-test-renderer';
import {
  addressOfPrivateKey, bytesToHex, dollarToken, hexToBytes, namedTokenAbiSha256, orderLockFor, quoteV2Layout as q, signDigest, walletReturnDigestFor,
} from '@nu54/protocol';
import {lockForSubmit, type PaymentCheckpoint} from '../src/merchant/paymentCheckpoint';
import {newReturn, submitReturn, verifyReturn} from '../src/wallet/boardReturn';
import {newSend, submitSend, verifySend} from '../src/wallet/boardSend';
import {BoardLife, type BoardLifeProps} from '../src/wallet/BoardLife';
import {storeMoney, storeToken} from '../src/wallet/tokens';
import {now, order, payer, payerKey, policy, quote, scope} from './fixtures/order';

const won = {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2};
const wonPolicy = {...policy, abiSha256: namedTokenAbiSha256, token: won};
const home = '0x' + 'ab'.repeat(20), key = '0fbc54ef-2827-4ab5-bfcd-3a27611c39ea';

test('the wallet lists and words the store token', () => {
  expect(storeToken(policy)).toEqual({address: policy.asset, symbol: 'TestUSDC', decimals: 6});
  expect(storeToken(wonPolicy)).toEqual({address: policy.asset, symbol: 'tKRW', decimals: 2});
  expect(storeMoney(1_234_500_000n)).toBe('1,234.50 TestUSDC');
  expect(storeMoney('1250000', won)).toBe('12,500원');
  expect(storeMoney(3_420_000n, won, 'en')).toBe('₩34,200');
});

test('an order lock is sent only when the board signed this store\'s token', () => {
  const confirmer = addressOfPrivateKey(new Uint8Array(32).fill(9));
  const signed = (name: string) => bytesToHex(signDigest(hexToBytes(
    orderLockFor(name, hexToBytes(quote.signedQuote).subarray(0, q.unsignedSize), confirmer).digest), payerKey), false);
  const entry = (authorization: string) => ({quote, order, scope, confirmer, authorization} as unknown as PaymentCheckpoint);
  expect(lockForSubmit(entry(signed(won.name)), wonPolicy, now).signature).toBe(signed(won.name));
  expect(lockForSubmit(entry(signed(dollarToken.name)), policy, now).signature).toBe(signed(dollarToken.name));
  expect(() => lockForSubmit(entry(signed(dollarToken.name)), wonPolicy, now)).toThrow('ORDER_LOCK_MISMATCH');
  expect(() => lockForSubmit(entry(signed(won.name)), policy, now)).toThrow('ORDER_LOCK_MISMATCH');
});

test('returns and transfers are checked against the store\'s token', async () => {
  const base = {payer, generation: '1', device: 'd1'.repeat(32), token: policy.asset, to: home};
  const sign = (name: string, record: {token: string; payer: string; to: string; amount: string; validBefore: number; nonce: string}) =>
    bytesToHex(signDigest(walletReturnDigestFor(name, {token: record.token, from: record.payer, to: record.to, amount: BigInt(record.amount),
      validBefore: BigInt(record.validBefore), nonce: record.nonce}), payerKey), false);
  const giving = newReturn(base, 3_420_000n, '7'.repeat(64), key, now), sending = newSend(base, 3_420_000n, '8'.repeat(64), key, now);
  const back = verifyReturn(giving, sign(won.name, giving), won.name), sent = verifySend(sending, sign(won.name, sending), won.name);
  expect(() => verifyReturn(giving, sign(won.name, giving))).toThrow('RETURN_SIGNATURE');
  expect(() => verifyReturn(giving, sign(dollarToken.name, giving), won.name)).toThrow('RETURN_SIGNATURE');
  expect(() => verifySend(sending, sign(won.name, sending))).toThrow('SEND_SIGNATURE');
  const calls: string[] = [];
  const client = {returnWallet: async () => { calls.push('return'); }, sendWallet: async () => { calls.push('send'); }} as never;
  await expect(submitReturn(client, back, async () => {}, now)).rejects.toThrow('RETURN_SIGNATURE');
  await expect(submitSend(client, sent, async () => {}, now)).rejects.toThrow('SEND_SIGNATURE');
  expect(calls).toEqual([]);
  await submitSend(client, sent, async () => {}, now, won.name);
  expect(calls).toContain('send');
});

describe('receiving', () => {
  const device = '34'.repeat(32), account = '0x' + '12'.repeat(20);
  const props = (token?: typeof won): BoardLifeProps => ({
    screen: 'deposit', native: null, hasEnrollment: true, found: [], busy: false, scan() {}, go() {}, orders() {}, work() {}, say() {}, wait() {}, copy() {},
    withBoard: async () => { throw new Error('unused'); }, commit: async () => {}, seen() {}, token, fixture: {balance: undefined},
    seenBoard: {account, generation: 1, at: Date.now()},
    stored: {version: 2, exercised: true, mode: 'personal', verified: {boardFingerprint: device, address: account, generation: 1, network: '8283'},
      spendingBoard: {device, account, generation: 1, testedAt: 1_800_000_000}} as unknown as BoardLifeProps['stored'],
  });
  const render = async (token?: typeof won) => {
    let tree!: Renderer.ReactTestRenderer;
    await Renderer.act(async () => { tree = Renderer.create(<BoardLife {...props(token)} />); });
    return tree;
  };
  test('a won store asks about the network before it shows the address', async () => {
    const tree = await render(won);
    const text = () => JSON.stringify(tree.toJSON());
    expect(text()).not.toContain(account);
    expect(text()).toContain('StableNet 테스트넷으로만 보내세요');
    const button = tree.root.findAll(node => node.props.accessibilityLabel === '확인했습니다. 주소 보기' && typeof node.props.onPress === 'function')[0];
    await Renderer.act(async () => { button.props.onPress(); });
    expect(text()).toContain(account);
    expect(text()).toContain('tKRW');
    expect(text()).not.toContain('TestUSDC');
    await Renderer.act(async () => { tree.unmount(); });
  });
  test('a dollar store shows it as before', async () => {
    const tree = await render();
    expect(JSON.stringify(tree.toJSON())).toContain(account);
    expect(JSON.stringify(tree.toJSON())).toContain('TestUSDC');
    await Renderer.act(async () => { tree.unmount(); });
  });
});
