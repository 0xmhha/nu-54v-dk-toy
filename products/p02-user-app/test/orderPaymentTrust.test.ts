import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {keccak_256} from '@noble/hashes/sha3';
import {addressOfPrivateKey, bigToBytes, bytesToHex, hexToBytes, quoteV2Layout as q, utf8Encode, type CommercePaymentProfile} from '@nu54/protocol';
import type {OrderPaymentPin} from '../src/merchant/customerEntranceDeployment';
import {isPinnedPolicy, orderPaymentPin, PaymentTrustError, pinnedPolicy, readMerchantRecord, verifyPinnedQuote, type MerchantRecord} from '../src/merchant/orderPaymentTrust';
import {reviewedPayment} from '../src/merchant/paymentCheckpoint';
import {branch, generation, now, order, payer, policy, quote, quoteFor, sale} from './fixtures/order';

// The fixture store's values split the way a phone without a registration holds them: the pin of its build, the
// number its server tells, and the record its order contract holds.
const pin: OrderPaymentPin = {chain: 8283, contract: policy.contract, asset: policy.asset};
const merchantId = policy.merchantId;
const record: MerchantRecord = {signer: policy.signer, beneficiary: policy.beneficiary, keyVersion: policy.keyVersion, beneficiaryVersion: policy.beneficiaryVersion};
const genesis = '0x' + '9a'.repeat(32), finalized = '0x' + '5c'.repeat(32);
const word = (value: string | bigint) => typeof value === 'string' ? value.replace(/^0x/, '').padStart(64, '0') : value.toString(16).padStart(64, '0');
const selector = (signature: string) => bytesToHex(keccak_256(utf8Encode(signature)).subarray(0, 4), false);

/** A chain that holds one order contract with one merchant, and writes down what it was asked. */
function chain(change: {chainId?: string; genesis?: string; asset?: string; merchant?: Partial<MerchantRecord> & {active?: bigint}; merchantReply?: unknown; offline?: true} = {}) {
  const calls: {to: string; data: string; at: unknown}[] = [];
  const held = {...record, active: 1n, ...change.merchant};
  const rpc = async (method: string, params: readonly unknown[]) => {
    if (change.offline) throw new Error('offline');
    if (method === 'eth_chainId') return change.chainId ?? '0x205b';
    if (method === 'eth_getBlockByNumber') return params[0] === '0x0' ? {hash: change.genesis ?? genesis} : params[0] === 'finalized' ? {hash: finalized, number: '0x64'} : null;
    if (method !== 'eth_call') throw new Error('unexpected ' + method);
    const [{to, data}, at] = params as [{to: string; data: string}, unknown];
    calls.push({to, data, at});
    if (to !== pin.contract) return '0x';
    if (data === '0x' + selector('asset()')) return '0x' + word(change.asset ?? pin.asset);
    if (data === '0x' + selector('merchants(bytes32)') + merchantId) return 'merchantReply' in change ? change.merchantReply : '0x' + [word(held.signer), word(held.beneficiary),
      word(BigInt(held.keyVersion)), word(BigInt(held.beneficiaryVersion)), word(172800n), word(604800n), word(held.active)].join('');
    // Any other merchant number reads as the contract's empty record.
    return '0x' + word(0n).repeat(7);
  };
  return {rpc, calls};
}
const refused = async (run: Promise<unknown>) => run.then(() => 'accepted', error => error instanceof PaymentTrustError ? error.code : String(error));

describe('the pin of a build', () => {
  test('is used only when every part is what a release may bind', () => {
    expect(orderPaymentPin(undefined)).toBeUndefined();
    expect(orderPaymentPin(pin)).toEqual(pin);
    const won = {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2};
    expect(orderPaymentPin({...pin, token: won, rpc: 'https://rpc.example'})).toEqual({...pin, token: won, rpc: 'https://rpc.example'});
    for (const bad of [{...pin, chain: 1}, {...pin, contract: '0x' + '0'.repeat(40)}, {...pin, contract: pin.contract.toUpperCase()}, {...pin, asset: pin.contract},
      {...pin, asset: 'usd'}, {...pin, token: {...won, decimals: 0}}, {...pin, rpc: 'http://rpc.example'}, {...pin, signer: policy.signer}, {...pin, merchantId}])
      expect(orderPaymentPin(bad as OrderPaymentPin)).toBeUndefined();
  });

  test('the calls are those of the order contract the repository publishes', () => {
    const abi = JSON.parse(readFileSync(resolve(__dirname, '../../../packages/contracts-abi/abi/OrderEscrow.json'), 'utf8')) as
      {name?: string; type: string; inputs: {type: string}[]; outputs?: {name: string; type: string}[]}[];
    const merchants = abi.find(item => item.type === 'function' && item.name === 'merchants')!;
    expect(merchants.inputs.map(input => input.type)).toEqual(['bytes32']);
    expect(merchants.outputs!.map(output => `${output.type} ${output.name}`)).toEqual(['address signer', 'address beneficiary', 'uint64 keyVersion',
      'uint64 beneficiaryVersion', 'uint32 confirmWindow', 'uint32 deliveryWindow', 'bool active']);
    expect(abi.find(item => item.type === 'function' && item.name === 'asset')!.outputs!.map(output => output.type)).toEqual(['address']);
  });
});

describe('the merchant record is read from the pinned contract', () => {
  test('at one finalized block of the pinned chain, from the pinned contract only', async () => {
    const {rpc, calls} = chain();
    expect(await readMerchantRecord(rpc, genesis, pin, merchantId)).toEqual(record);
    expect(calls).toEqual([
      {to: pin.contract, data: '0x' + selector('asset()'), at: {blockHash: finalized, requireCanonical: true}},
      {to: pin.contract, data: '0x' + selector('merchants(bytes32)') + merchantId, at: {blockHash: finalized, requireCanonical: true}},
    ]);
  });

  test('another chain, another genesis block or no answer reads nothing', async () => {
    expect(await refused(readMerchantRecord(chain({chainId: '0x1'}).rpc, genesis, pin, merchantId))).toBe('CHAIN_UNREADABLE');
    expect(await refused(readMerchantRecord(chain({genesis: '0x' + '11'.repeat(32)}).rpc, genesis, pin, merchantId))).toBe('CHAIN_UNREADABLE');
    expect(await refused(readMerchantRecord(chain({offline: true}).rpc, genesis, pin, merchantId))).toBe('CHAIN_UNREADABLE');
    for (const merchantReply of ['0x', '0x' + word(1n), null, {signer: record.signer}, '0x' + word(1n).repeat(8), '0x' + 'f'.repeat(64) + word(1n).repeat(6)])
      expect(await refused(readMerchantRecord(chain({merchantReply}).rpc, genesis, pin, merchantId))).toBe('CHAIN_UNREADABLE');
  });

  test('a contract for another token is not this build\'s contract', async () => {
    expect(await refused(readMerchantRecord(chain({asset: '0x' + '7'.repeat(40)}).rpc, genesis, pin, merchantId))).toBe('CONTRACT_MISMATCH');
  });

  test('a merchant the contract does not hold, or holds switched off, cannot be paid', async () => {
    expect(await refused(readMerchantRecord(chain().rpc, genesis, pin, '4'.repeat(64)))).toBe('MERCHANT_UNKNOWN');
    expect(await refused(readMerchantRecord(chain({merchant: {active: 0n}}).rpc, genesis, pin, merchantId))).toBe('MERCHANT_UNKNOWN');
    expect(await refused(readMerchantRecord(chain({merchant: {signer: '0x' + '0'.repeat(40)}}).rpc, genesis, pin, merchantId))).toBe('MERCHANT_UNKNOWN');
    expect(await refused(readMerchantRecord(chain({merchant: {active: 2n}}).rpc, genesis, pin, merchantId))).toBe('CHAIN_UNREADABLE');
    // A merchant number that is not one is refused before the chain is asked.
    const asked = chain();
    for (const bad of ['', '0'.repeat(64), 'A'.repeat(64), merchantId.slice(2), `0x${merchantId}`])
      expect(await refused(readMerchantRecord(asked.rpc, genesis, pin, bad))).toBe('MERCHANT_UNKNOWN');
    expect(asked.calls).toEqual([]);
  });
});

describe('a quote is checked against the pin and the on-chain record', () => {
  const scope = {branch, sale};
  const trusted = pinnedPolicy(pin, merchantId, record);
  const at = (offset: number, value: Uint8Array) => (words: Uint8Array) => { words.set(value, offset); };
  const sameOrder = (issued: ReturnType<typeof quoteFor>) => ({...order, quoteDigest: issued.quoteDigest});

  test('the store\'s quote passes, for the payer and generation it was issued for', () => {
    const payment = verifyPinnedQuote(quote, order, trusted, now, scope);
    expect([payment.payer, payment.generation, payment.payerRole, payment.digest]).toEqual([payer, generation, 'owner', quote.quoteDigest]);
    expect(isPinnedPolicy(trusted)).toBe(true);
    expect(trusted).toMatchObject({chain: 8283, contract: pin.contract, asset: pin.asset, merchantId, ...record, maxBaseUnits: '30000000'});
    expect(trusted.presentationKey).toBeUndefined();
    // The one function the ordering screens call takes the pinned check for a pinned policy.
    expect(reviewedPayment(quote, order, trusted, now, {...scope, customer: branch}).digest).toBe(quote.quoteDigest);
  });

  test('a registration\'s policy is never checked the pinned way, and a pinned policy needs no presentation', () => {
    expect(isPinnedPolicy(policy)).toBe(false);
    expect(() => verifyPinnedQuote(quote, order, policy, now, scope)).toThrow('PAYMENT_PROFILE');
    // The same values copied into another object are not a policy this module made.
    expect(() => verifyPinnedQuote(quote, order, {...trusted}, now, scope)).toThrow('PAYMENT_PROFILE');
    const bare = {...quote, presentation: undefined};
    expect(verifyPinnedQuote(bare, order, trusted, now, scope).digest).toBe(quote.quoteDigest);
    expect(() => reviewedPayment(bare, order, policy, now, {...scope, customer: branch})).toThrow('PRESENTATION_SCOPE');
  });

  test.each<[string, Partial<MerchantRecord>]>([
    ['another signer', {signer: addressOfPrivateKey(new Uint8Array(32).fill(77))}],
    ['another beneficiary', {beneficiary: '0x' + '8'.repeat(40)}],
    ['another key version', {keyVersion: record.keyVersion + 1}],
    ['another beneficiary version', {beneficiaryVersion: record.beneficiaryVersion + 1}],
  ])('the contract holds %s than the quote names: refused', (_name, change) => {
    expect(() => verifyPinnedQuote(quote, order, pinnedPolicy(pin, merchantId, {...record, ...change}), now, scope)).toThrow('PAYMENT_QUOTE_MISMATCH');
  });

  test('a quote for another contract, token, chain or merchant is refused though the store signed it', () => {
    for (const [name, change] of [
      ['contract', at(q.verifyingContractOffset + 12, hexToBytes('0x' + '6'.repeat(40)))],
      ['token', at(q.assetOffset + 12, hexToBytes('0x' + '6'.repeat(40)))],
      ['chain', at(q.chainIdOffset, bigToBytes(1n, 32))],
      ['merchant', at(q.merchantIdOffset, hexToBytes('6'.repeat(64)))],
      ['beneficiary', at(q.beneficiaryOffset + 12, hexToBytes('0x' + '6'.repeat(40)))],
    ] as const) {
      const issued = quoteFor(sale, change);
      expect([name, (() => { try { verifyPinnedQuote(issued, sameOrder(issued), trusted, now, scope); return 'accepted'; } catch (error) { return (error as Error).message; } })()])
        .toEqual([name, 'PAYMENT_QUOTE_MISMATCH']);
    }
    // The build pins another contract or token than the store's server quotes for.
    for (const other of [{...pin, contract: '0x' + '6'.repeat(40)}, {...pin, asset: '0x' + '6'.repeat(40)}])
      expect(() => verifyPinnedQuote(quote, {...order, asset: other.asset}, pinnedPolicy(other, merchantId, record), now, scope)).toThrow('PAYMENT_QUOTE_MISMATCH');
    // The server names another registered merchant's number: its record is not the one this quote was signed under.
    const neighbour = pinnedPolicy(pin, '6'.repeat(64), {...record, signer: addressOfPrivateKey(new Uint8Array(32).fill(78))});
    expect(() => verifyPinnedQuote(quote, order, neighbour, now, scope)).toThrow('PAYMENT_QUOTE_MISMATCH');
  });

  test('a quote that is not an order-contract quote for the board\'s own account, or is for another order, is refused', () => {
    const rented = quoteFor(sale, words => { words[q.payerRoleOffset + 31] = 2; });
    expect(() => verifyPinnedQuote(rented, sameOrder(rented), trusted, now, scope)).toThrow('PAYMENT_QUOTE_MISMATCH');
    const direct = quoteFor(sale, words => { words[q.modeOffset + 31] = q.directMode; });
    expect(() => verifyPinnedQuote(direct, sameOrder(direct), trusted, now, scope)).toThrow('PAYMENT_QUOTE_MISMATCH');
    expect(() => verifyPinnedQuote(quote, order, trusted, now, {branch, sale: branch})).toThrow('PAYMENT_ORDER_STATE');
    expect(() => verifyPinnedQuote(quote, order, trusted, now, {branch: sale, sale})).toThrow('PAYMENT_ORDER_STATE');
    expect(() => verifyPinnedQuote(quote, order, trusted, quote.expiresAt, scope)).toThrow('PAYMENT_QUOTE_MISMATCH');
    expect(() => verifyPinnedQuote(quote, {...order, amount: '4500001'}, trusted, now, scope)).toThrow('PAYMENT_QUOTE_MISMATCH');
  });

  test('a quote over the board\'s own cap for one payment is refused', () => {
    const large = quoteFor(sale, at(q.amountOffset, bigToBytes(30_000_001n, 32)));
    expect(() => verifyPinnedQuote(large, {...sameOrder(large), amount: '30000001'}, trusted, now, scope)).toThrow('PAYMENT_QUOTE_MISMATCH');
    const policyOf = (value: CommercePaymentProfile) => value.maxBaseUnits;
    expect(policyOf(pinnedPolicy({...pin, token: {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2}}, merchantId, record))).toBe('30000000');
  });
});
