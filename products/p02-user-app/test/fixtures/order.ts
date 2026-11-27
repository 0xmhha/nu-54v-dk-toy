import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {ed25519} from '@noble/curves/ed25519';
import {
  addressOfPrivateKey, bigToBytes, bytesToBig, bytesToHex, concat, hexToBytes, orderLock, presentationMessage, preparePresentedPayment,
  orderEscrowAbiSha256, quoteDigest, quoteV2Layout as q, signDigest, type CommerceOrder, type CommercePaymentProfile, type CommerceQuote, type OrderEscrowStatus,
} from '@nu54/protocol';

// One escrow order signed by test keys, in the shape the kiosk hands to the phone.
export const branch = '0fbc54ef-2827-4ab5-bfcd-3a27611c39ea';
export const customer = '0fbc54ef-2827-4ab5-bfcd-3a27611c39eb';
export const sale = 'f0551c76-7592-4850-a414-54464d24dafe';
export const terminal = 'f0551c76-7592-4850-a414-54464d24daff';
export const payerKey = new Uint8Array(32).fill(32);
const merchantKey = new Uint8Array(32).fill(31);
const presentationSeed = new Uint8Array(32).fill(44);
const [, , , vector, , clock] = readFileSync(
  resolve(__dirname, '../../../../packages/protocol/v2/testdata/quote.vectors'), 'utf8').split('\n')[1].split('|');
export const now = Number(clock);
const raw = hexToBytes(vector);
raw.set(hexToBytes(addressOfPrivateKey(payerKey)), q.payerOffset + 12);
raw[q.modeOffset + 31] = q.escrowMode;
raw[q.payerRoleOffset + 31] = q.ownerRole;
raw.set(bigToBytes(4_500_000n, 32), q.amountOffset);
raw.set(bigToBytes(BigInt(now + 110), 32), q.expiresAtOffset);
const word = (offset: number) => raw.subarray(offset, offset + 32);
const address = (offset: number) => bytesToHex(word(offset).subarray(12));
export const payer = address(q.payerOffset);
export const generation = bytesToBig(word(q.payerGenerationOffset));
export const policy = {
  chain: 8283, contract: address(q.verifyingContractOffset), asset: address(q.assetOffset), beneficiary: address(q.beneficiaryOffset),
  merchantId: bytesToHex(word(q.merchantIdOffset), false), signer: addressOfPrivateKey(merchantKey),
  keyVersion: Number(bytesToBig(word(q.merchantKeyVersionOffset))), beneficiaryVersion: Number(bytesToBig(word(q.beneficiaryVersionOffset))),
  maxBaseUnits: '30000000', presentationKey: bytesToHex(ed25519.getPublicKey(presentationSeed), false), abiSha256: orderEscrowAbiSha256,
} as CommercePaymentProfile;
export const orderKey = bytesToHex(word(q.orderIdOffset), false);

export function quoteFor(origin = sale, change?: (words: Uint8Array) => void): CommerceQuote {
  const words = raw.slice();
  change?.(words);
  const digest = quoteDigest(words);
  const quote: CommerceQuote = {
    saleRef: sale, branchRef: branch, orderKey, quoteDigest: bytesToHex(digest, false),
    signedQuote: bytesToHex(concat(words, signDigest(digest, merchantKey)), false),
    expiresAt: Number(bytesToBig(words.subarray(q.expiresAtOffset, q.expiresAtOffset + 32))),
    presentation: {customer, origin, claim: 'a'.repeat(64), signature: ''},
  };
  quote.presentation!.signature = bytesToHex(ed25519.sign(presentationMessage(quote, generation), presentationSeed), false);
  return quote;
}
export const quote = quoteFor();
export const order: CommerceOrder = {
  id: sale, amount: '4500000', asset: policy.asset, state: 'issued', revision: 1,
  expiresAt: new Date(quote.expiresAt * 1000).toISOString(), quoteDigest: quote.quoteDigest, orderKey,
};
export const scope = {branch, customer, sale};
export const payment = preparePresentedPayment(quote, order, policy, now, scope);

/** What an approving board returns for this quote and phone key. */
export function boardSignature(confirmer: string, key = payerKey, signedQuote = quote.signedQuote) {
  const lock = orderLock(hexToBytes(signedQuote).subarray(0, q.unsignedSize), confirmer);
  return bytesToHex(signDigest(hexToBytes(lock.digest), key), false);
}
export function escrowStatus(stage: OrderEscrowStatus['stage'], confirmer: string): OrderEscrowStatus {
  const base = {stage, escrow: policy.contract, merchantId: policy.merchantId, orderKey, payer, confirmer};
  if (stage === 'received' || stage === 'failed') return base;
  if (stage === 'submitted') return {...base, lockTxHash: '0x' + '1'.repeat(64)};
  const locked = {...base, lockTxHash: '0x' + '1'.repeat(64), deliverBy: now + 604800};
  if (stage === 'locked') return locked;
  const delivered = {...locked, claimableAt: now + 172800};
  const settledTxHash = '0x' + '2'.repeat(64);
  if (stage === 'delivered') return delivered;
  if (stage === 'disputed') return {...delivered, resolveBy: now + 1209600};
  if (stage === 'paid') return {...delivered, payout: 'receipt', settledTxHash};
  return {...locked, refund: 'store', settledTxHash};
}
