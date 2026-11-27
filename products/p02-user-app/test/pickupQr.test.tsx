import React from 'react';
import Renderer from 'react-test-renderer';
import {AppState} from 'react-native';
import {accountProofDigest, addressOfPrivateKey, bytesToHex, parsePickupQr, signDigest} from '@nu54/protocol';
import {configuredEntrance, CustomerEntrance, EntranceError, issuePickupQr, shortAccount,
  type BoardLink, type EntrancePost, type IssuedPickup, type PickupBoard} from '../src/merchant/customerEntrance';
import {automaticQrs, PickupQrScreen} from '../src/merchant/PickupQr';
import {OrderDetail} from '../src/merchant/OpenOrders';
import type {OpenOrder} from '../src/merchant/orderRecords';

jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);

const key = new Uint8Array(32).fill(7), otherKey = new Uint8Array(32).fill(9);
const payer = addressOfPrivateKey(key), stranger = addressOfPrivateKey(otherKey);
const branch = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002', sale = '0fbc54ef-2827-4ab5-bfcd-3a27611c3001';
const challengeHex = 'c1'.repeat(32), token = 'a7'.repeat(32);
const order: OpenOrder = {
  payer, generation: '1', sale, branch, customer: '0fbc54ef-2827-4ab5-bfcd-3a27611c3003', confirmer: '0x' + '2'.repeat(40), store: '여울 커피',
  asset: '0x' + '3'.repeat(40), amount: '1250000', escrow: '0x' + '4'.repeat(40), merchantId: 'a'.repeat(64), orderKey: 'b'.repeat(64),
  sentAt: 1_764_000_000, pickupNo: 17, stage: 'locked', pickup: 'ready',
  items: [{name: '카페라떼', quantity: 1, total: '600000'}, {name: '치즈케이크', quantity: 1, total: '650000'}],
};

/** A board that signs the account proof the way the firmware does (op 0x79), with `secret`'s account. */
function board(secret: Uint8Array | 'absent' | 'silent' = key) {
  const opened: string[] = [], closed: string[] = [];
  const value: PickupBoard = {async open(expected) {
    opened.push(expected);
    if (secret === 'absent') throw new Error('BOARD_NOT_FOUND');
    const account = addressOfPrivateKey(secret === 'silent' ? key : secret);
    const link: BoardLink = {account, close: () => { closed.push(account); }, prove: async challenge => {
      if (secret === 'silent') throw new Error('OWNER_DISCONNECTED');
      return {account, signature: bytesToHex(signDigest(accountProofDigest(account, challenge), secret), false)};
    }};
    return link;
  }};
  return Object.assign(value, {opened, closed});
}

/** The customer entrance as opsd answers it; `proofs` lists the answers to the proof requests in order. */
function entrance(proofs: {status: number; body: unknown}[] = [{status: 200, body: {purpose: 'pickup', branchRef: branch, saleRef: sale, pickupToken: token, expiresAt: 0}}]) {
  const calls: {path: string; body: Record<string, string>}[] = [];
  const post: EntrancePost = async (path, body) => {
    calls.push({path, body: JSON.parse(body)});
    if (path === '/v2/customer/challenge') return {status: 200, body: {challenge: challengeHex, expiresAt: 1_764_000_120}};
    return proofs.shift() ?? {status: 503, body: {status: 'error', error: 'UNKNOWN'}};
  };
  return {client: new CustomerEntrance(post), calls};
}

describe('the customer entrance client', () => {
  test('asks a pickup challenge for the order, and trades the proof for the token of the same order', async () => {
    const {client, calls} = entrance();
    const {challenge} = await client.pickupChallenge(branch, sale);
    expect(bytesToHex(challenge, false)).toBe(challengeHex);
    const proof = {account: payer, signature: 'ab'.repeat(65)};
    expect(await client.pickupToken(challenge, proof, {branch, sale})).toEqual({token, expiresAt: 0});
    expect(calls).toEqual([
      {path: '/v2/customer/challenge', body: {purpose: 'pickup', branchRef: branch, saleRef: sale}},
      {path: '/v2/customer/proof', body: {challenge: challengeHex, account: payer, signature: 'ab'.repeat(65)}},
    ]);
  });

  test('a token for another order or purpose, or a malformed one, is not taken', async () => {
    const proof = {account: payer, signature: 'ab'.repeat(65)};
    const challenge = new Uint8Array(32).fill(1);
    for (const body of [{purpose: 'pickup', branchRef: branch, saleRef: branch, pickupToken: token, expiresAt: 1},
      {purpose: 'order', branchRef: branch, saleRef: sale, session: token, expiresAt: 1},
      {purpose: 'pickup', branchRef: branch, saleRef: sale, pickupToken: token.toUpperCase(), expiresAt: 1}, null])
      await expect(entrance([{status: 200, body}]).client.pickupToken(challenge, proof, {branch, sale})).rejects.toThrow('CUSTOMER_ENTRANCE_UNAVAILABLE');
    const refusals: [number, unknown, string][] = [[400, {error: 'MISMATCHED_EVIDENCE'}, 'MISMATCH'], [409, {error: 'CHECKOUT_CONFLICT'}, 'NOT_READY'],
      [400, {error: 'EXPIRED'}, 'EXPIRED'], [429, {error: 'BUSY'}, 'BUSY'], [404, null, 'REFUSED'], [503, {error: 'UNKNOWN'}, 'UNAVAILABLE']];
    for (const [status, body, code] of refusals)
      await expect(entrance([{status, body}]).client.pickupToken(challenge, proof, {branch, sale})).rejects.toEqual(new EntranceError(code as never));
    await expect(entrance().client.pickupChallenge('x', sale)).rejects.toThrow('CUSTOMER_ENTRANCE_REFUSED');
  });

  test('a build without a complete trust anchor has no entrance', () => {
    const anchor = {origin: 'https://store.example', ca: 'MIIB' + 'A'.repeat(64), pin: 'ab'.repeat(32)};
    expect(configuredEntrance(undefined)).toBeUndefined();
    expect(configuredEntrance(anchor)).toBeInstanceOf(CustomerEntrance);
    for (const broken of [{...anchor, origin: ''}, {...anchor, origin: 'http://store.example'}, {...anchor, ca: ''},
      {...anchor, pin: ''}, {...anchor, pin: 'AB'.repeat(32)}, {...anchor, pin: '0'.repeat(64)}])
      expect(configuredEntrance(broken)).toBeUndefined();
  });

  test('short accounts keep the first and last four characters', () => {
    expect(shortAccount('0x7a3c' + '0'.repeat(32) + '91e2')).toBe('0x7a3c…91e2');
  });
});

describe('one pickup QR', () => {
  test('the paying board proves its account over the server challenge, and the QR carries only the token', async () => {
    const {client, calls} = entrance();
    const near = board();
    const issued = await issuePickupQr(client, near, order);
    expect(issued).toEqual({kind: 'qr', qr: `nu54://pickup/${token}`, expiresAt: 0});
    expect(parsePickupQr((issued as {qr: string}).qr)).toBe(token);
    expect(near.opened).toEqual([payer]);
    expect(near.closed).toEqual([payer]);
    // The signature recovers to the payer over the challenge the server gave.
    expect(calls[1].body.challenge).toBe(challengeHex);
    expect(calls[1].body.account).toBe(payer);
    expect(calls[1].body.signature).toMatch(/^[0-9a-f]{130}$/);
  });

  test('another board is named before the server is asked; a refusal of the server says the same', async () => {
    const {client, calls} = entrance();
    const other = board(otherKey);
    expect(await issuePickupQr(client, other, order)).toEqual({kind: 'other-board', payer, board: stranger});
    expect(calls).toEqual([]);
    expect(other.closed).toEqual([stranger]);
    const refused = entrance([{status: 400, body: {status: 'error', error: 'MISMATCHED_EVIDENCE'}}]);
    expect(await issuePickupQr(refused.client, board(), order)).toEqual({kind: 'other-board', payer, board: payer});
  });

  test('no board, a board that stops answering, an order not ready, a busy server and no entrance', async () => {
    expect(await issuePickupQr(entrance().client, board('absent'), order)).toEqual({kind: 'no-board'});
    const silent = board('silent');
    expect(await issuePickupQr(entrance().client, silent, order)).toEqual({kind: 'no-board'});
    expect(silent.closed).toHaveLength(1);
    expect(await issuePickupQr(entrance([{status: 409, body: {error: 'CHECKOUT_CONFLICT'}}]).client, board(), order)).toEqual({kind: 'not-ready'});
    expect(await issuePickupQr(entrance([{status: 429, body: {error: 'BUSY'}}]).client, board(), order)).toEqual({kind: 'busy'});
    expect(await issuePickupQr(undefined, board(), order)).toEqual({kind: 'unavailable'});
  });

  test('a challenge that ran out is asked once more, not again and again', async () => {
    const expired = {status: 400, body: {status: 'error', error: 'EXPIRED'}};
    const once = entrance([expired, {status: 200, body: {purpose: 'pickup', branchRef: branch, saleRef: sale, pickupToken: token, expiresAt: 0}}]);
    expect((await issuePickupQr(once.client, board(), order)).kind).toBe('qr');
    expect(once.calls.map(call => call.path)).toEqual(['/v2/customer/challenge', '/v2/customer/proof', '/v2/customer/challenge', '/v2/customer/proof']);
    const twice = entrance([expired, expired]);
    expect(await issuePickupQr(twice.client, board(), order)).toEqual({kind: 'unavailable'});
    expect(twice.calls).toHaveLength(4);
  });
});

describe('the pickup QR screen', () => {
  let tree: Renderer.ReactTestRenderer;
  let clock = 1_764_000_000_000;
  const now = () => clock;
  const screen = () => JSON.stringify(tree.toJSON());
  const press = async (label: string) => {
    const button = tree.root.findAll(node => node.props.accessibilityRole === 'button' && node.props.accessibilityLabel === label)[0];
    await Renderer.act(async () => { button.props.onPress(); });
  };
  const advance = async (seconds: number) => {
    for (let i = 0; i < seconds; i++) await Renderer.act(async () => { clock += 1000; jest.advanceTimersByTime(1000); });
  };
  beforeEach(() => { jest.useFakeTimers(); clock = 1_764_000_000_000; });
  afterEach(() => { Renderer.act(() => tree?.unmount()); jest.useRealTimers(); });

  const qr = (n: number): IssuedPickup => ({kind: 'qr', qr: `nu54://pickup/${String(n).padStart(2, '0').repeat(32)}`, expiresAt: Math.floor(clock / 1000) + 60});
  const render = async (issue?: () => Promise<IssuedPickup>, back = () => {}) => {
    await Renderer.act(async () => { tree = Renderer.create(<PickupQrScreen order={order} issue={issue} back={back} now={now} />); });
  };

  test('shows the QR with its minute, and asks the board again before it ends', async () => {
    let n = 0;
    const issue = jest.fn(async () => qr(++n));
    await render(issue);
    expect(screen()).toContain('여울 커피 · 17번');
    expect(screen()).toContain('직원에게 이 QR을 보여 주세요. 직원이 찍으면 전달이 기록됩니다.');
    expect(screen()).toContain('1:00');
    expect(screen()).toContain('카페라떼 · 치즈케이크');
    expect(screen()).toContain('이 주문을 결제한 보드');
    expect(screen()).toContain('QR에는 한 번만 쓰는 값만 들어 있습니다.');
    expect(tree.root.findAll(node => node.props.accessibilityLabel === '수령 QR' && node.props.source).length).toBeGreaterThan(0);
    const first = tree.root.findAll(node => node.props.accessibilityLabel === '수령 QR' && node.props.source)[0].props.source.uri;
    await advance(8);
    expect(screen()).toContain('0:52');
    expect(issue).toHaveBeenCalledTimes(1);
    await advance(47);
    expect(issue).toHaveBeenCalledTimes(2); // five seconds before the end, with a new proof
    expect(screen()).toContain('1:00');
    expect(tree.root.findAll(node => node.props.accessibilityLabel === '수령 QR' && node.props.source)[0].props.source.uri).not.toBe(first);
  });

  test('stops renewing after the automatic QRs, once the last one runs out', async () => {
    let n = 0;
    const issue = jest.fn(async () => qr(++n));
    await render(issue);
    for (let i = 0; i < automaticQrs; i++) await advance(56);
    expect(issue).toHaveBeenCalledTimes(automaticQrs);
    await advance(10);
    expect(screen()).toContain('QR을 여러 번 바꿨습니다');
    await press('새 QR 받기');
    expect(issue).toHaveBeenCalledTimes(automaticQrs + 1);
    expect(screen()).toContain('직원에게 이 QR을 보여 주세요.');
  });

  test('without the board, or with another board, says so and asks again on request', async () => {
    const answers: IssuedPickup[] = [{kind: 'no-board'}, {kind: 'other-board', payer: '0x7a3c' + '0'.repeat(32) + '91e2', board: '0x51d0' + '1'.repeat(32) + '0b4f'}, qr(1)];
    const issue = jest.fn(async () => answers.shift()!);
    await render(issue);
    expect(screen()).toContain('결제한 보드가 필요합니다');
    expect(screen()).toContain('보드를 찾지 못했습니다');
    expect(screen()).toContain('수령 QR은 이 주문을 결제한 보드로 확인한 뒤에만 나옵니다. 보드를 켜고 휴대전화 가까이 두세요.');
    expect(screen()).toContain('주문은 그대로 있습니다. 준비된 음료는 매장이 보관합니다.');
    await press('다시 확인하기');
    expect(screen()).toContain('결제한 보드가 아닙니다');
    expect(screen()).toContain('연결된 보드의 계정이 이 주문의 결제 계정과 다릅니다');
    expect(screen()).toContain('0x7a3c…91e2');
    expect(screen()).toContain('0x51d0…0b4f');
    await press('결제한 보드로 다시 확인');
    expect(screen()).toContain('여울 커피 · 17번');
  });

  test('an order not ready, and a build without the entrance address, offer no QR', async () => {
    const back = jest.fn();
    await render(async () => ({kind: 'not-ready'}), back);
    expect(screen()).toContain('아직 수령 QR을 받을 수 없습니다');
    await press('주문으로 돌아가기');
    expect(back).toHaveBeenCalled();
    await render(undefined);
    expect(screen()).toContain('이 앱에는 수령 QR을 받을 매장 서버 주소가 아직 없습니다.');
  });

  test('a QR that ends while the app is in the background is replaced when it comes back', async () => {
    let n = 0;
    const issue = jest.fn(async () => qr(++n));
    const listeners: ((state: string) => void)[] = [];
    const add = jest.spyOn(AppState, 'addEventListener').mockImplementation(((_: string, listener: (state: string) => void) => {
      listeners.push(listener);
      return {remove() {}};
    }) as never);
    const original = Object.getOwnPropertyDescriptor(AppState, 'currentState')!;
    const set = (value: string) => Object.defineProperty(AppState, 'currentState', {value, configurable: true, writable: true});
    set('background');
    await render(issue);
    await advance(70);
    expect(issue).toHaveBeenCalledTimes(1);
    set('active');
    await Renderer.act(async () => { listeners.forEach(listener => listener('active')); });
    expect(issue).toHaveBeenCalledTimes(2);
    Object.defineProperty(AppState, 'currentState', original);
    add.mockRestore();
  });
});

describe('the ready order', () => {
  const detail = async (pickupQr?: () => void, shown: OpenOrder = order) => {
    let tree!: Renderer.ReactTestRenderer;
    await Renderer.act(async () => {
      tree = Renderer.create(<OrderDetail order={shown} amount="12,500원" fresh now={1_764_000_100}
        actions={{answer: () => {}, refresh: () => {}, back: () => {}, ...(pickupQr ? {pickupQr} : {})}} />);
    });
    return tree;
  };
  const button = (tree: Renderer.ReactTestRenderer) =>
    tree.root.findAll(node => node.props.accessibilityRole === 'button' && node.props.accessibilityLabel === '수령 QR 받기');

  test('offers the pickup QR when the app has an entrance, and says to show it at the counter', async () => {
    const open = jest.fn();
    const tree = await detail(open);
    const text = JSON.stringify(tree.toJSON());
    expect(text).toContain('준비되었습니다. 카운터에서 수령 QR을 보여 주세요.');
    expect(text).toContain('결제한 보드로 확인한 뒤 QR이 나옵니다. 12,500원은 주문 계약에 보관 중이고, 직원이 QR을 찍어 전달을 기록하면 받았는지 여기에서 묻습니다.');
    await Renderer.act(async () => { button(tree)[0].props.onPress(); });
    expect(open).toHaveBeenCalled();
  });

  test('keeps the order number way without an entrance, and offers no QR before the order is ready', async () => {
    const plain = await detail();
    expect(button(plain)).toHaveLength(0);
    expect(JSON.stringify(plain.toJSON())).toContain('매장에서 주문번호를 보여 주고 받으세요.');
    expect(button(await detail(() => {}, {...order, pickup: 'preparing'}))).toHaveLength(0);
    expect(button(await detail(() => {}, {...order, pickup: 'collected'}))).toHaveLength(0);
  });
});
