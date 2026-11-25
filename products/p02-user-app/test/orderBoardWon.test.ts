import {sha256} from '@noble/hashes/sha256';
import {p256} from '@noble/curves/p256';
import {
  accountProofDigest, addressOfPrivateKey, bigToBytes, bytesToBig, bytesToHex, concat, encodeV2StreamFrame, equalBytes, fromBase64, hexToBytes,
  orderLockFor, quoteV2Layout as q, walletReturnDigestFor, signDigest, toBase64, V2Journal, V2Link, V2StreamDecoder, type V2Request,
} from '@nu54/protocol';
import type {Spec as Ble} from '../src/specs/NativeRenterBle';
import type {Spec as Native} from '../src/specs/NativeOwnerWallet';
import {OwnerSession} from '../src/wallet/ownerSession';
import {generation, now, payer, payerKey, payment} from './fixtures/order';

// A stand-in for the spending firmware's owner session (device_pilot.c order_request, order_button,
// receipt_status and the 0x7f/0x79 reads). Records outlive a connection, as the board's storage does.
const device = 'd1'.repeat(32), pin = 'd2'.repeat(32);
const phone = new Uint8Array(32).fill(3);
const spki = concat(hexToBytes('3059301306072a8648ce3d020106082a8648ce3d030107034200'), p256.getPublicKey(phone, false));
const owner = sha256(spki);
const confirmer = addressOfPrivateKey(new Uint8Array(32).fill(9));
const request = new Uint8Array(16).fill(0xc);

let boardName = 'NU54 Test KRW';
const stableToken = '0x' + '5'.repeat(40), home = '0x' + 'ab'.repeat(20);
type Board = {
  press: 'approve' | 'refuse' | 'ignore' | 'other-phone'; loseReply?: boolean; cap: bigint; daily: bigint; spent: bigint; clock: boolean; blindDaily?: boolean; wait: number; account: string; state: number;
  prompts: number; spending: boolean; home?: string; far?: boolean; sends?: boolean; otherSend?: boolean; orderOpen?: boolean; nonce?: Uint8Array; records: {request: Uint8Array; digest: Uint8Array; expires: bigint; answer?: Uint8Array}[]; seen: V2Request[]; journal: V2Journal;
};
const board = (change: Partial<Board> = {}): Board => ({press: 'approve', cap: 30_000_000n, daily: 100_000_000n, spent: 0n, clock: true, wait: 0, account: payer, state: 1, prompts: 0,
  spending: true, records: [], seen: [], journal: new V2Journal(32), ...change});

let ids = 0; // the board's journal outlives a connection, so request ids must not repeat
async function connect(b: Board, spending = false) {
  const epoch = new Uint8Array(16).fill(7 + b.seen.length % 200);
  const link = new V2Link('management', 'board', epoch, owner, b.journal);
  const decoder = new V2StreamDecoder();
  const ble = {connectOwner: async () => 247, disconnect: async () => {}, onDisconnect: () => ({remove() {}}),
    onFragment: () => ({remove() {}}), writeFragment: async () => {}} as unknown as Ble;
  const left = () => b.clock && b.spent < b.daily ? b.daily - b.spent : 0n;
  // nu54_order_find: the same request or the same order alone, before its quote ended, is an error.
  const find = (id: Uint8Array, digest: Uint8Array) => {
    if (!b.clock) return 'drop';
    const clash = b.records.some(row => equalBytes(row.request, id) !== equalBytes(row.digest, digest) && row.expires > BigInt(Math.floor(Date.now() / 1000)));
    return clash ? 'drop' : b.records.find(row => equalBytes(row.request, id) && equalBytes(row.digest, digest));
  };
  const answer = (message: V2Request): Uint8Array | 'drop' | 'wait' => {
    const body = message.payload;
    if (message.command === 'status') {
      const known = find(body.subarray(1, 17), body.subarray(17));
      if (known === 'drop') return 'drop';
      return body.length === 49 && body[0] === 1 ? known?.answer ?? Uint8Array.of(0) : 'drop';
    }
    if (message.command === 'pay.prepare') {
      if (bytesToHex(body.subarray(0, 4), false) !== '4e35344f' || body.length !== 4 + 20 + 737) return 'drop';
      const lock = orderLockFor(boardName, body.subarray(24, 24 + q.unsignedSize), bytesToHex(body.subarray(4, 24)));
      const digest = sha256(lock.preimage);
      const known = find(message.id, digest);
      if (known === 'drop') return 'drop';
      if (known) return known.answer ?? Uint8Array.of(0);
      const amount = bytesToBig(body.subarray(24 + q.amountOffset, 24 + q.amountOffset + 32));
      if (amount > left()) return 'drop'; // device_order_within_daily, before the prompt
      b.prompts++;
      if (b.press === 'ignore') return 'wait';
      const signed = b.press === 'refuse' ? undefined : signDigest(hexToBytes(b.press === 'other-phone'
        ? orderLockFor(boardName, body.subarray(24, 24 + q.unsignedSize), '0x' + '1'.repeat(40)).digest : lock.digest), payerKey);
      if (signed) b.spent += amount; // device_order_charge commits before the signature exists
      b.records.push({request: message.id, digest, expires: bytesToBig(body.subarray(24 + q.expiresAtOffset, 24 + q.expiresAtOffset + 32)), answer: signed});
      return b.loseReply ? 'drop' : signed ?? Uint8Array.of(0);
    }
    if (body[0] === 3) return concat(Uint8Array.of(3, b.state, 0, 0, 0), bigToBytes(generation, 8), hexToBytes(device), owner);
    if (!b.spending) return 'drop';
    // spending_request: 0x7a once per wallet; 0x7b, 0x7c and 0x7e not while an approved order can execute; SW2 answers one 0.
    if ([0x7a, 0x7b, 0x7c, 0x7e].includes(body[0])) {
      if ((body[0] !== 0x7a && b.orderOpen) || (body[0] === 0x7a && b.home) || (body[0] === 0x7b && (!b.home || b.far))) return 'drop';
      if (body[0] === 0x7e) {
        const live = b.nonce;
        b.nonce = undefined;
        if (!live || body.length !== 97 || !equalBytes(body.subarray(1, 33), live) || body[33] !== 0x5a) return 'drop';
      }
      b.prompts++;
      if (b.press !== 'approve') return Uint8Array.of(0);
      if (body[0] === 0x7a) { b.home = bytesToHex(body.subarray(1)); return body; }
      if (body[0] === 0x7b) {
        const transfer = {token: stableToken, from: b.account, to: b.home!, amount: bytesToBig(body.subarray(1, 9)), validBefore: bytesToBig(body.subarray(9, 17)), nonce: bytesToHex(body.subarray(17), false)};
        return concat(Uint8Array.of(0x7b), hexToBytes(b.home!), signDigest(walletReturnDigestFor(boardName, transfer), payerKey));
      }
      b.state = 4;
      return Uint8Array.of(body[0]);
    }
    if (body[0] === 0x7d) { b.nonce = new Uint8Array(32).fill(0x3c); return concat(Uint8Array.of(0x7d), b.nonce); }
    if (body[0] === 0x7f) return concat(Uint8Array.of(0x7f, b.home ? 1 : 0), hexToBytes(b.account), b.home ? hexToBytes(b.home) : new Uint8Array(20),
      bigToBytes(b.cap, 8), bigToBytes(b.daily, 8), bigToBytes(BigInt(b.wait), 4), Uint8Array.of(b.orderOpen ? 1 : 0),
      bigToBytes(b.blindDaily ? b.daily : left(), 8), b.sends ? Uint8Array.of(1) : new Uint8Array(0));
    // The firmware to come: a transfer to the address in the request, within the payment limits.
    if (body[0] === 0x80) {
      if (!b.sends || body.length !== 89) return 'drop';
      b.prompts++;
      if (b.press !== 'approve') return Uint8Array.of(0);
      const transfer = {token: bytesToHex(body.subarray(1, 21)), from: b.account, to: bytesToHex(body.subarray(21, 41)), amount: bytesToBig(body.subarray(41, 49)),
        validBefore: bytesToBig(body.subarray(49, 57)), nonce: bytesToHex(body.subarray(57), false)};
      return concat(Uint8Array.of(0x80), hexToBytes(transfer.to), signDigest(walletReturnDigestFor(boardName, b.otherSend ? {...transfer, amount: 1n} : transfer), payerKey));
    }
    if (body[0] === 0x79) return concat(Uint8Array.of(0x79), hexToBytes(b.account),
      signDigest(accountProofDigest(b.account, body.subarray(1)), payerKey), new Uint8Array(64));
    return 'drop';
  };
  const native = {
    onTls: () => ({remove() {}}), stopTls: async () => {},
    startTls: async () => JSON.stringify({out: '', epoch: toBase64(epoch), peer: pin}),
    randomBytes: async (size: number) => toBase64(new Uint8Array(size).fill(20 + ++ids % 230)),
    publicIdentity: async () => toBase64(spki),
    sendTls: async (value: string) => {
      const out: Uint8Array[] = [];
      let closed = false;
      decoder.feed(fromBase64(value), frame => {
        const got = link.receive(frame, BigInt(Math.floor(Date.now() / 1000)));
        if (!got) return;
        b.seen.push(got.request);
        const reply = answer(got.request);
        if (reply === 'drop') closed = true;
        else if (reply !== 'wait') for (const part of link.send({command: 'reply', id: got.request.id, expires: got.request.expires, payload: reply}))
          out.push(encodeV2StreamFrame(part));
      });
      return JSON.stringify({out: '', clear: out.length ? toBase64(concat(...out)) : '', closed});
    },
  } as unknown as Native;
  const session = new OwnerSession(ble, native, {version: 2, device, boardSPKI: pin, ca: 'MA==', certificateChain: ['MA==']}, spending);
  await session.connect('board');
  return session;
}
const approve = async (b: Board, name?: string) => {
  const session = await connect(b);
  try { return await session.approveOrder(payment, confirmer, request, () => {}, name); } finally { session.close(); }
};

beforeEach(() => { boardName = 'NU54 Test KRW'; });
beforeEach(() => jest.spyOn(Date, 'now').mockReturnValue(now * 1000));
afterEach(() => jest.restoreAllMocks());


const won = 'NU54 Test KRW', dollar = 'NU54 Test USD';
test('an order signed as won is accepted as won only', async () => {
  expect(await approve(board(), won)).toMatch(/^[0-9a-f]{128}(1b|1c)$/);
  await expect(approve(board())).rejects.toThrow('ORDER_LOCK_MISMATCH');
  boardName = dollar;
  await expect(approve(board(), won)).rejects.toThrow('ORDER_LOCK_MISMATCH');
});
test('a lost answer is read back as won', async () => {
  const b = board({loseReply: true});
  await expect(approve(b, won)).rejects.toThrow('OWNER_DISCONNECTED');
  expect(await approve(b, won)).toBe(bytesToHex(b.records[0].answer!, false));
  expect(b.prompts).toBe(1);
});
test('returns and transfers are accepted as the named token only', async () => {
  const transfer = {token: stableToken, payer, to: home, amount: 3_420_000n, validBefore: now + 170, nonce: '7'.repeat(64)};
  for (const [signs, asked, ok] of [[won, won, true], [dollar, won, false], [won, undefined, false], [dollar, undefined, true]] as const) {
    boardName = signs;
    const back = await connect(board({home}), true), send = await connect(board({sends: true}), true);
    const results = [await back.signReturn(generation, transfer, () => {}, asked).then(() => 'ok', error => error.message),
      await send.signSend(generation, transfer, () => {}, asked).then(() => 'ok', error => error.message)];
    expect(results).toEqual(ok ? ['ok', 'ok'] : ['RETURN_SIGNATURE', 'SEND_SIGNATURE']);
    back.close(); send.close();
  }
});
