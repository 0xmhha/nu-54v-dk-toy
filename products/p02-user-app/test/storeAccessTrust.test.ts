import {keccak_256} from '@noble/hashes/sha3';
import {accountProofDigest, addressOfPrivateKey, bytesToHex, signDigest, utf8Encode} from '@nu54/protocol';
import type {BoardLink, EntrancePost, PickupBoard} from '../src/merchant/customerEntrance';
import {isPinnedPolicy} from '../src/merchant/orderPaymentTrust';
import {customerSubject, StoreAccess, StoreBusy, type AccessParts} from '../src/merchant/storeAccess';
import {readProfiles} from '../src/wallet/readProfiles';
import {journalStore} from './fixtures/journal';

// One store reached without a registration: what its access keeps, what it reads from the chain, and what an answer
// of the store's server means for the session it holds.

const mockKept = new Map<string, string>();
const mockJournal = journalStore(mockKept);
let mockRefuse = false;
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  loadJournal: async (name: string) => { if (mockRefuse) throw Object.assign(new Error('refused'), {code: 'COMMERCE_REFUSED'}); return mockJournal.loadJournal(name); },
  saveJournal: async (name: string, value: string) => { if (mockRefuse) throw Object.assign(new Error('refused'), {code: 'COMMERCE_REFUSED'}); return mockJournal.saveJournal(name, value); },
  randomBytes: async () => 'AAAAAAAAAAAAAAAAAAAAAA==',
}}));
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({__esModule: true, default: null}));

const key = new Uint8Array(32).fill(7), payer = addressOfPrivateKey(key);
const branch = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002', sale = '0fbc54ef-2827-4ab5-bfcd-3a27611c3001', tenant = '0fbc54ef-2827-4ab5-bfcd-3a27611c3005';
const pin = {chain: 8283 as const, contract: '0x' + '3'.repeat(40), asset: '0x' + '2'.repeat(40)};
const merchantId = 'e1' + '0'.repeat(60) + '07';
const record = {signer: '0x' + '5'.repeat(40), beneficiary: '0x' + '6'.repeat(40), keyVersion: 2, beneficiaryVersion: 3};
const word = (value: string | bigint) => typeof value === 'string' ? value.replace(/^0x/, '').padStart(64, '0') : value.toString(16).padStart(64, '0');
const selector = (signature: string) => bytesToHex(keccak_256(utf8Encode(signature)).subarray(0, 4), false);
let clock = 1_764_000_000;

function setup(answer: (path: string, body: string, session?: string) => {status: number; body: unknown} | undefined = () => undefined) {
  const chain: string[] = [], sent: {path: string; session?: string}[] = [], proofs: string[] = [];
  let down = false, seed = 1;
  const rpc = async (method: string, params: readonly unknown[]) => {
    if (down) throw new Error('offline');
    if (method === 'eth_chainId') return '0x205b';
    if (method === 'eth_getBlockByNumber') return {hash: params[0] === '0x0' ? readProfiles.stable.anchor : '0x' + '5c'.repeat(32), number: '0x64'};
    const [{data}] = params as [{data: string}];
    chain.push(data.slice(0, 10));
    if (data === '0x' + selector('asset()')) return '0x' + word(pin.asset);
    return '0x' + [word(record.signer), word(record.beneficiary), word(BigInt(record.keyVersion)), word(BigInt(record.beneficiaryVersion)), word(172800n), word(604800n), word(1n)].join('');
  };
  const board: PickupBoard = {async open() {
    const link: BoardLink = {account: payer, generation: '3', close() {}, prove: async challenge => {
      proofs.push(payer);
      return {account: payer, signature: bytesToHex(signDigest(accountProofDigest(payer, challenge), key), false)};
    }};
    return link;
  }};
  let asked: {purpose: string; saleRef?: string} = {purpose: ''};
  const post: EntrancePost = async (path, body, session) => {
    sent.push({path, ...(session ? {session} : {})});
    const given = answer(path, body, session);
    if (given) return given;
    if (path === '/v2/customer/challenge') { asked = JSON.parse(body); return {status: 200, body: {challenge: 'c1'.repeat(32), expiresAt: clock + 120}}; }
    if (path === '/v2/customer/proof') return {status: 200, body: {purpose: asked.purpose, branchRef: branch, ...(asked.saleRef ? {saleRef: asked.saleRef} : {}),
      tenantRef: tenant, subjectRef: customerSubject(payer), session: (seed++).toString(16).padStart(2, '0').repeat(32), expiresAt: clock + 900}};
    return {status: 503, body: null};
  };
  const parts: AccessParts = {post, origin: 'https://store.example:8444', pin, board, now: () => clock,
    wallet: async () => ({account: payer, generation: '3', rpc})};
  return {parts, chain, sent, proofs, offline: (value: boolean) => { down = value; },
    open: (store: {merchantId?: string} = {merchantId}) => StoreAccess.open({branch, name: '여울 커피', ...store}, parts)};
}

beforeEach(() => { mockKept.clear(); mockRefuse = false; clock = 1_764_000_000; });
afterEach(() => { jest.restoreAllMocks(); });

describe('the policy of a store', () => {
  test('is confirmed from the chain alone, as a new object, and the one handed out before is not changed', async () => {
    const {open, chain, sent, proofs} = setup();
    const access = await open();
    const shown = access.policy, before = JSON.stringify(shown);
    expect(isPinnedPolicy(shown)).toBe(false);
    expect(chain).toEqual([]); // opening a store asks nothing of the chain
    expect(await access.confirm()).toBeUndefined();
    expect(access.policy).not.toBe(shown);
    expect(isPinnedPolicy(access.policy)).toBe(true);
    expect(access.policy).toMatchObject({...record, merchantId});
    expect(JSON.stringify(shown)).toBe(before);
    expect(isPinnedPolicy(shown)).toBe(false);
    expect(chain).toEqual(['0x' + selector('asset()'), '0x' + selector('merchants(bytes32)')]);
    // No board was asked and nothing went to the store's server.
    expect([sent, proofs]).toEqual([[], []]);
    const confirmed = access.policy;
    expect(await access.confirm()).toBeUndefined();
    expect(access.policy).toBe(confirmed);
    expect(chain).toHaveLength(2);
  });

  test('a chain that cannot be read, or a store without a merchant number, leaves the policy unconfirmed and says which', async () => {
    const world = setup();
    world.offline(true);
    const access = await world.open();
    expect(await access.confirm()).toEqual({kind: 'payment', code: 'CHAIN_UNREADABLE'});
    expect(isPinnedPolicy(access.policy)).toBe(false);
    world.offline(false);
    expect(await access.confirm()).toBeUndefined();
    expect(await (await world.open({})).confirm()).toEqual({kind: 'payment', code: 'NO_MERCHANT'});
  });

  test('the proof for an order confirms it on the way, with the same record', async () => {
    const {open, chain, proofs} = setup();
    const access = await open();
    const shown = access.policy;
    expect(await access.ensure({purpose: 'checkout', branch})).toBeUndefined();
    expect(access.policy).not.toBe(shown);
    expect(isPinnedPolicy(access.policy)).toBe(true);
    expect(isPinnedPolicy(shown)).toBe(false);
    expect([chain.length, proofs]).toEqual([2, [payer]]);
  });
});

describe('order sessions', () => {
  test('are kept under a name the phone\'s journal takes, and a new access of the same store finds them', async () => {
    const first = setup();
    const access = await first.open();
    expect(await access.ensure({purpose: 'order', branch, sale})).toBeUndefined();
    const names = [...mockKept.keys()];
    expect(names).toEqual([expect.stringMatching(/^customer-sessions-[0-9a-f]{64}$/)]);
    expect(JSON.parse(mockKept.get(names[0])!)).toMatchObject([{purpose: 'order', sale, branch, token: '01'.repeat(32)}]);
    const again = setup();
    const later = await again.open();
    expect(later.live({purpose: 'order', branch, sale})).toBe(true);
    expect(await later.ensure({purpose: 'order', branch, sale})).toBeUndefined();
    expect(again.proofs).toEqual([]);
  });

  test('a journal that refuses them is said in a development build, and the order goes on with a new proof', async () => {
    const said = jest.spyOn(console, 'error').mockImplementation(() => {});
    mockRefuse = true;
    const world = setup();
    const access = await world.open();
    expect(said).toHaveBeenCalledTimes(1);
    expect(said.mock.calls[0][0]).toContain('Order sessions were not read');
    expect(said.mock.calls[0][1]).toBe('COMMERCE_REFUSED');
    expect(await access.ensure({purpose: 'order', branch, sale})).toBeUndefined();
    expect(said).toHaveBeenCalledTimes(2);
    expect(said.mock.calls[1][0]).toContain('Order sessions were not written');
    expect(access.live({purpose: 'order', branch, sale})).toBe(true);
  });

  test('too many requests is not the end of a session: the same session is used again, with no new proof', async () => {
    let busy = true;
    const world = setup(path => path === '/v2/customer/commerce' ? (busy ? {status: 429, body: {status: 'error', error: 'BUSY'}} : {status: 404, body: null}) : undefined);
    const access = await world.open();
    expect(await access.ensure({purpose: 'order', branch, sale})).toBeUndefined();
    await expect(access.client.status(sale)).rejects.toBeInstanceOf(StoreBusy);
    expect(access.live({purpose: 'order', branch, sale})).toBe(true);
    await expect(access.client.status(sale)).rejects.toBeInstanceOf(StoreBusy);
    expect(world.sent.filter(item => item.path === '/v2/customer/commerce').map(item => item.session)).toEqual(['01'.repeat(32), '01'.repeat(32)]);
    expect(world.proofs).toEqual([payer]);
    // Only the server's 404 ends it; the board then proves again.
    busy = false;
    await expect(access.client.status(sale)).rejects.toMatchObject({code: 'CUSTOMER_SESSION_NEEDED'});
    expect(access.live({purpose: 'order', branch, sale})).toBe(false);
  });

  test('a store that does not show its page has no menu, and a busy entrance is said as busy', async () => {
    const hidden = await setup(path => path.includes('/menu/') ? {status: 404, body: null} : undefined).open();
    await expect(hidden.client.publishedMenu()).rejects.toThrow('STORE_NOT_PUBLIC');
    const busy = await setup(path => path.includes('/menu/') ? {status: 429, body: null} : undefined).open();
    await expect(busy.client.publishedMenu()).rejects.toBeInstanceOf(StoreBusy);
  });
});
