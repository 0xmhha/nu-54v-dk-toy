import {keccak_256} from '@noble/hashes/sha3';
import {
  bytesToHex, fromBoard, namedTokenAbiSha256, orderEscrowAbiSha256, paymentProfile, paymentToken, prepareCommercePayment, quoteV2Layout, utf8Encode,
  type CommerceOrder, type CommercePaymentProfile, type CommerceQuote, type VerifiedCommercePayment,
} from '@nu54/protocol';
import {checkEvmNetwork, type EvmRpc} from '../wallet/evmTransfer.ts';
import {approvedOrderPayment, type OrderPaymentPin} from './customerEntranceDeployment.ts';

// What a phone without any store registration trusts when it pays (use-case-review.md chapter 9): the chain, the
// order contract and the token of its own build, and the merchant record that contract holds. The contract's
// governor alone writes that record (OrderEscrow.configureMerchant), and the contract pays an order only when the
// record's signer signed its quote and the quote names the record's beneficiary (OrderEscrow._checkQuote). So the
// phone asks the contract who may quote for a store and where that store's money goes, and checks the quote against
// the answer before the board is asked. The server tells only which merchant number a store has.
//
// What remains: a server that names another registered merchant's number. The quote then verifies, and the money is
// locked in the same contract for that merchant; the buyer's dispute returns it. An operator-signed store profile
// would close this and is the next step before real assets (same chapter).

const address = /^0x[0-9a-f]{40}$/;
const merchantNumber = /^[0-9a-f]{64}$/;
const selector = (signature: string) => bytesToHex(keccak_256(utf8Encode(signature)).subarray(0, 4), false);
const merchantsCall = selector('merchants(bytes32)'), assetCall = selector('asset()');

/** The pin as the app uses it, or undefined when a part is not what a release may bind. */
export function orderPaymentPin(pin: OrderPaymentPin | undefined = approvedOrderPayment): OrderPaymentPin | undefined {
  try {
    if (!pin || Object.keys(pin).some(key => !['chain', 'contract', 'asset', 'token', 'rpc'].includes(key)) || pin.contract === pin.asset) return undefined;
    // The same rules as a store's payment profile: the chain, the address forms, the token and the read server.
    const checked = paymentProfile(JSON.stringify({payment: {...pin, abiSha256: pin.token ? namedTokenAbiSha256 : orderEscrowAbiSha256,
      beneficiary: '0x' + '1'.repeat(40), signer: '0x' + '2'.repeat(40), merchantId: '3'.repeat(64), keyVersion: 1, beneficiaryVersion: 1, maxBaseUnits: '1'}}));
    if (!checked) return undefined;
    return {chain: 8283, contract: checked.contract, asset: checked.asset, ...(checked.token ? {token: checked.token} : {}), ...(checked.rpc ? {rpc: checked.rpc} : {})};
  } catch {
    return undefined;
  }
}

/** A store's record in the order contract, as the chain holds it. */
export type MerchantRecord = {signer: string; beneficiary: string; keyVersion: number; beneficiaryVersion: number};

export type PaymentTrustCode =
  /** The chain could not be read, or what it answered is not this contract. Nothing can be paid now. */
  | 'CHAIN_UNREADABLE'
  /** The contract of this build is not for the token of this build. */
  | 'CONTRACT_MISMATCH'
  /** The contract holds no such merchant, or the governor switched it off. */
  | 'MERCHANT_UNKNOWN';
export class PaymentTrustError extends Error {
  constructor(readonly code: PaymentTrustCode) { super(`ORDER_PAYMENT_${code}`); }
}

function words(result: unknown, count: number): string[] {
  if (typeof result !== 'string' || !new RegExp(`^0x[0-9a-f]{${count * 64}}$`).test(result)) throw new PaymentTrustError('CHAIN_UNREADABLE');
  return Array.from({length: count}, (_, index) => result.slice(2 + index * 64, 66 + index * 64));
}
const addressWord = (word: string) => {
  if (!/^0{24}/.test(word)) throw new PaymentTrustError('CHAIN_UNREADABLE');
  return '0x' + word.slice(24);
};
const versionWord = (word: string) => {
  const value = BigInt('0x' + word);
  if (value > BigInt(Number.MAX_SAFE_INTEGER)) throw new PaymentTrustError('CHAIN_UNREADABLE');
  return Number(value);
};

/**
 * Reads `merchants[merchantId]` and the contract's token from the pinned contract, at one finalized block of the
 * chain whose genesis block is `genesis`. The server chooses nothing here but the merchant number.
 */
export async function readMerchantRecord(rpc: EvmRpc, genesis: string, pin: OrderPaymentPin, merchantId: string): Promise<MerchantRecord> {
  if (!merchantNumber.test(merchantId) || /^0+$/.test(merchantId) || !address.test(pin.contract) || !address.test(pin.asset))
    throw new PaymentTrustError('MERCHANT_UNKNOWN');
  let token: string[], merchant: string[];
  try {
    await checkEvmNetwork(rpc, String(pin.chain) as '8283', genesis);
    const block = await rpc('eth_getBlockByNumber', ['finalized', false]) as {hash?: unknown} | null;
    if (typeof block?.hash !== 'string' || !/^0x[0-9a-f]{64}$/.test(block.hash)) throw new PaymentTrustError('CHAIN_UNREADABLE');
    const at = {blockHash: block.hash, requireCanonical: true};
    token = words(await rpc('eth_call', [{to: pin.contract, data: '0x' + assetCall}, at]), 1);
    merchant = words(await rpc('eth_call', [{to: pin.contract, data: '0x' + merchantsCall + merchantId}, at]), 7);
  } catch (error) {
    throw error instanceof PaymentTrustError ? error : new PaymentTrustError('CHAIN_UNREADABLE');
  }
  if (addressWord(token[0]) !== pin.asset) throw new PaymentTrustError('CONTRACT_MISMATCH');
  const [signer, beneficiary] = [addressWord(merchant[0]), addressWord(merchant[1])];
  const [keyVersion, beneficiaryVersion] = [versionWord(merchant[2]), versionWord(merchant[3])];
  const active = BigInt('0x' + merchant[6]);
  if (active > 1n) throw new PaymentTrustError('CHAIN_UNREADABLE');
  if (active !== 1n || /^0x0+$/.test(signer) || /^0x0+$/.test(beneficiary) || keyVersion < 1 || beneficiaryVersion < 1)
    throw new PaymentTrustError('MERCHANT_UNKNOWN');
  return {signer, beneficiary, keyVersion, beneficiaryVersion};
}

const pinned = new WeakSet<CommercePaymentProfile>();
/** Whether this policy came from the build and the chain, not from a store registration. */
export const isPinnedPolicy = (policy: CommercePaymentProfile | undefined): boolean => !!policy && pinned.has(policy);

/**
 * The payment policy of one store for a phone without a registration: the pin of this build and the merchant
 * record read from its contract. The cap of one payment is the board's own (30,000,000 of its units).
 */
export function pinnedPolicy(pin: OrderPaymentPin, merchantId: string, record: MerchantRecord): CommercePaymentProfile {
  const policy = paymentProfile(JSON.stringify({payment: {
    chain: pin.chain, contract: pin.contract, asset: pin.asset, ...(pin.token ? {token: pin.token} : {}), ...(pin.rpc ? {rpc: pin.rpc} : {}),
    abiSha256: pin.token ? namedTokenAbiSha256 : orderEscrowAbiSha256, merchantId, ...record,
    maxBaseUnits: fromBoard(30_000_000n, paymentToken(pin)).toString(),
  }}));
  if (!policy) throw new PaymentTrustError('MERCHANT_UNKNOWN');
  pinned.add(policy);
  return policy;
}

/**
 * What a store's menu is shown with before the chain is read: the contract and token of the build, and nobody to pay.
 * It is not a pinned policy, so no quote verifies against it. `confirmPolicy` makes the pinned policy of the same
 * store from it; this object stays as it is.
 */
export function unconfirmedPolicy(pin: OrderPaymentPin, merchantId: string | undefined): CommercePaymentProfile {
  const nobody = '0x' + '0'.repeat(40);
  return {chain: pin.chain, contract: pin.contract, asset: pin.asset, ...(pin.token ? {token: pin.token} : {}), ...(pin.rpc ? {rpc: pin.rpc} : {}),
    abiSha256: pin.token ? namedTokenAbiSha256 : orderEscrowAbiSha256, merchantId: merchantId ?? '0'.repeat(64), signer: nobody, beneficiary: nobody,
    keyVersion: 0, beneficiaryVersion: 0, maxBaseUnits: fromBoard(30_000_000n, paymentToken(pin)).toString()};
}

/**
 * The pinned policy of an unconfirmed policy's merchant, with the record the contract holds. It is a new object: the
 * unconfirmed one is not changed, so a screen that holds it is drawn again with the new one, not altered under it.
 */
export function confirmPolicy(policy: CommercePaymentProfile, pin: OrderPaymentPin, record: MerchantRecord): CommercePaymentProfile {
  // The placeholder number of a store that named none is nobody's.
  if (/^0+$/.test(policy.merchantId)) throw new PaymentTrustError('MERCHANT_UNKNOWN');
  const checked = pinnedPolicy(pin, policy.merchantId, record);
  if (checked.contract !== policy.contract || checked.asset !== policy.asset) throw new PaymentTrustError('CONTRACT_MISMATCH');
  return checked;
}

/**
 * Checks a quote from the entrance against a pinned policy before any screen or board uses it: the merchant
 * signature of the on-chain signer, and the chain, contract, token, beneficiary, merchant number and versions.
 * It must be an order-contract quote for the board's own account (not a rented payer).
 */
export function verifyPinnedQuote(quote: CommerceQuote, order: CommerceOrder, policy: CommercePaymentProfile, now: number,
  expected: {branch: string; sale: string}): VerifiedCommercePayment {
  if (!isPinnedPolicy(policy)) throw new Error('PAYMENT_PROFILE');
  if (quote.saleRef !== expected.sale || order.id !== expected.sale || quote.branchRef !== expected.branch) throw new Error('PAYMENT_ORDER_STATE');
  const payment = prepareCommercePayment(quote, order, policy, now, expected.branch);
  if (payment.bytes[quoteV2Layout.modeOffset + 31] !== quoteV2Layout.escrowMode || payment.payerRole !== 'owner')
    throw new Error('PAYMENT_QUOTE_MISMATCH');
  return payment;
}
