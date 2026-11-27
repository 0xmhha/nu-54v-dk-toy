// One wallet signature pays one order into OrderEscrow. The token authorization's nonce commits to
// the merchant quote and to the phone key that may later confirm or dispute the order.
// The board (nu54_order.c), the contract (OrderEscrow.sol) and this file share order-lock.vectors.
import { sha256 } from "@noble/hashes/sha256";
import { keccak_256 } from "@noble/hashes/sha3";
import { bigToBytes, bytesToHex, concat, hexToBytes, utf8Encode } from "./bytes.ts";
import { quoteDigest, quoteV2Layout as q } from "./quote-v2.ts";
import { recoverSigner, signDigest } from "./signature.ts";
import type { VerifiedCommercePayment } from "./commerce-payment.ts";

const text = (value: string) => keccak_256(utf8Encode(value));
const address20 = /^0x[0-9a-f]{40}$/;
const bytes32 = /^[0-9a-f]{64}$/;

export const orderTokenName = "NU54 Test USD";
export const orderTokenVersion = "1";
const receiveType = text(
  "ReceiveWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
);
const lockType = text("OrderLock(bytes32 quoteDigest,address confirmer)");
const domainType = text(q.domainType);

const addressWord = (value: string) => {
  if (!address20.test(value) || /^0x0+$/.test(value)) throw new Error("ORDER_ADDRESS");
  return concat(new Uint8Array(12), hexToBytes(value));
};
const hashWord = (value: string) => {
  if (!bytes32.test(value) || /^0+$/.test(value)) throw new Error("ORDER_ID");
  return hexToBytes(value);
};

export type OrderLock = {
  /** 66 bytes; its keccak-256 is what the board signs. */
  preimage: Uint8Array;
  /** Lower-case hex without prefix: the C3/order.lock `lockDigest` field. */
  digest: string;
  nonce: string;
};

/** The token's signing name: 1 to 64 printable ASCII characters without spaces at either end. */
export const validTokenName = (name: unknown): name is string =>
  typeof name === "string" && /^[\x21-\x7e](?:[\x20-\x7e]{0,62}[\x21-\x7e])?$/.test(name);
const tokenNameHash = (name: string) => {
  if (!validTokenName(name)) throw new Error("ORDER_TOKEN_NAME");
  return text(name);
};

/** orderLockFor with the dollar token. */
export const orderLock = (unsigned: Uint8Array, confirmer: string): OrderLock => orderLockFor(orderTokenName, unsigned, confirmer);

/** `unsigned` is the 672-byte quote; `confirmer` is the phone key address; `tokenName` is the asset's signing name. */
export function orderLockFor(tokenName: string, unsigned: Uint8Array, confirmer: string): OrderLock {
  const nonce = keccak_256(concat(lockType, quoteDigest(unsigned), addressWord(confirmer)));
  const at = (offset: number) => unsigned.subarray(offset, offset + 32);
  const token = keccak_256(concat(domainType, tokenNameHash(tokenName), text(orderTokenVersion), at(q.chainIdOffset), at(q.assetOffset)));
  const authorization = keccak_256(
    concat(
      receiveType,
      at(q.payerOffset),
      at(q.verifyingContractOffset),
      at(q.amountOffset),
      new Uint8Array(32),
      at(q.expiresAtOffset),
      nonce
    )
  );
  const preimage = concat(Uint8Array.of(0x19, 0x01), token, authorization);
  return { preimage, digest: bytesToHex(keccak_256(preimage), false), nonce: bytesToHex(nonce, false) };
}

type LockedPayment = Pick<VerifiedCommercePayment, "bytes" | "payer" | "deadline">;

/** verifyOrderLockFor with the dollar token. */
export const verifyOrderLock = (payment: LockedPayment, confirmer: string, signature: Uint8Array, now: number): string =>
  verifyOrderLockFor(orderTokenName, payment, confirmer, signature, now);

/** Accepts only the paying board's signature over this exact quote, token name and phone key. */
export function verifyOrderLockFor(
  tokenName: string,
  payment: LockedPayment,
  confirmer: string,
  signature: Uint8Array,
  now: number
): string {
  const lock = orderLockFor(tokenName, payment.bytes.subarray(0, q.unsignedSize), confirmer);
  if (
    now >= payment.deadline ||
    signature.length !== 65 ||
    ![27, 28].includes(signature[64]) ||
    recoverSigner(hexToBytes(lock.digest), signature) !== payment.payer
  )
    throw new Error("ORDER_LOCK_MISMATCH");
  return bytesToHex(signature, false);
}

/** The board request body: magic, phone key address, merchant-signed quote. */
export function orderBoardRequest(confirmer: string, signedQuote: Uint8Array): Uint8Array {
  if (signedQuote.length !== q.unsignedSize + q.signatureSize) throw new Error("ORDER_QUOTE");
  return concat(utf8Encode("N54O"), addressWord(confirmer).subarray(12), signedQuote);
}

export type OrderBuyerAction = "receipt" | "dispute" | "withdraw";
export const orderActionTypes = {
  delivered: "OrderDelivered(bytes32 merchantId,bytes32 orderId)",
  cancel: "OrderCancel(bytes32 merchantId,bytes32 orderId)",
  receipt: "OrderReceipt(bytes32 merchantId,bytes32 orderId,uint64 claimableAt)",
  dispute: "OrderDispute(bytes32 merchantId,bytes32 orderId,uint64 claimableAt)",
  withdraw: "OrderDisputeWithdraw(bytes32 merchantId,bytes32 orderId,uint64 claimableAt)",
} as const;

function actionDigest(type: string, escrow: string, merchantId: string, orderKey: string, claimableAt?: bigint) {
  const domain = keccak_256(
    concat(domainType, text(q.domainName), text(q.domainVersion), bigToBytes(BigInt(q.pilotChain), 32), addressWord(escrow))
  );
  const fields = [text(type), hashWord(merchantId), hashWord(orderKey)];
  if (claimableAt !== undefined) fields.push(bigToBytes(claimableAt, 32));
  return keccak_256(concat(Uint8Array.of(0x19, 0x01), domain, keccak_256(concat(...fields))));
}

export function orderMerchantDigest(
  action: "delivered" | "cancel",
  order: { escrow: string; merchantId: string; orderKey: string }
): Uint8Array {
  return actionDigest(orderActionTypes[action], order.escrow, order.merchantId, order.orderKey);
}

/** `claimableAt` names the delivery this answers, so nothing signed earlier can be reused. */
export function orderBuyerDigest(
  action: OrderBuyerAction,
  order: { escrow: string; merchantId: string; orderKey: string; claimableAt: number }
): Uint8Array {
  if (!Number.isSafeInteger(order.claimableAt) || order.claimableAt < 1) throw new Error("ORDER_NOT_DELIVERED");
  return actionDigest(orderActionTypes[action], order.escrow, order.merchantId, order.orderKey, BigInt(order.claimableAt));
}

export function signOrderBuyerAction(
  action: OrderBuyerAction,
  order: { escrow: string; merchantId: string; orderKey: string; claimableAt: number },
  phoneKey: Uint8Array
): string {
  return bytesToHex(signDigest(orderBuyerDigest(action, order), phoneKey), false);
}

/** What C3/order.status reports for an order paid through the escrow. Unix seconds. */
export type OrderEscrowStage =
  | "received" // the server stored the wallet signature
  | "submitted" // the lock transaction was sent
  | "locked" // the lock is final; the store has the order
  | "delivered" // the store marked it handed over; `claimableAt` is set
  | "disputed" // `resolveBy` is set
  | "paid" // the merchant received the money; `payout` is set
  | "refunded" // the payer received the money back; `refund` is set
  | "failed"; // the lock can no longer execute; nothing was taken
export type OrderEscrowStatus = {
  stage: OrderEscrowStage;
  escrow: string;
  merchantId: string;
  orderKey: string;
  payer: string;
  confirmer: string;
  lockTxHash?: string;
  deliverBy?: number;
  claimableAt?: number;
  resolveBy?: number;
  payout?: "receipt" | "claim" | "arbiter" | "dispute-withdrawn";
  refund?: "store" | "undelivered" | "arbiter" | "dispute-expired";
  settledTxHash?: string;
  /** A buyer action the server accepted and has not seen on chain yet. */
  pendingAction?: OrderBuyerAction;
};

const stages = ["received", "submitted", "locked", "delivered", "disputed", "paid", "refunded", "failed"];
const payouts = ["receipt", "claim", "arbiter", "dispute-withdrawn"];
const refunds = ["store", "undelivered", "arbiter", "dispute-expired"];
const txHash = /^0x[0-9a-f]{64}$/;
const statusFields = [
  "stage", "escrow", "merchantId", "orderKey", "payer", "confirmer", "lockTxHash", "deliverBy", "claimableAt",
  "resolveBy", "payout", "refund", "settledTxHash", "pendingAction",
];

export function validOrderEscrowStatus(value: unknown): value is OrderEscrowStatus {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const s = value as OrderEscrowStatus;
  const seconds = (v: unknown) => v === undefined || (Number.isSafeInteger(v) && (v as number) > 0);
  const locked = ["locked", "delivered", "disputed", "paid", "refunded"].includes(s.stage);
  const settled = s.stage === "paid" || s.stage === "refunded";
  return (
    Object.keys(s).every((key) => statusFields.includes(key)) &&
    stages.includes(s.stage) &&
    [s.escrow, s.payer, s.confirmer].every((a) => typeof a === "string" && address20.test(a) && !/^0x0+$/.test(a)) &&
    [s.merchantId, s.orderKey].every((h) => typeof h === "string" && bytes32.test(h) && !/^0+$/.test(h)) &&
    (s.lockTxHash === undefined ? !locked && s.stage !== "submitted" : txHash.test(s.lockTxHash) && s.stage !== "received") &&
    [s.deliverBy, s.claimableAt, s.resolveBy].every(seconds) &&
    (locked ? s.deliverBy !== undefined : s.deliverBy === undefined && s.claimableAt === undefined && s.resolveBy === undefined) &&
    (s.stage !== "locked" || s.claimableAt === undefined) &&
    (!["delivered", "disputed", "paid"].includes(s.stage) || s.claimableAt !== undefined) &&
    (s.resolveBy === undefined ? s.stage !== "disputed" : ["disputed", "paid", "refunded"].includes(s.stage) && s.claimableAt !== undefined) &&
    (s.stage === "paid" ? payouts.includes(s.payout ?? "") : s.payout === undefined) &&
    (s.stage === "refunded" ? refunds.includes(s.refund ?? "") : s.refund === undefined) &&
    (settled ? txHash.test(s.settledTxHash ?? "") : s.settledTxHash === undefined) &&
    (s.pendingAction === undefined ||
      (s.stage === "delivered" && ["receipt", "dispute"].includes(s.pendingAction)) ||
      (s.stage === "disputed" && s.pendingAction === "withdraw"))
  );
}

/** What the phone may offer. The contract enforces the same limits. */
export function orderBuyerActions(status: OrderEscrowStatus, now: number): OrderBuyerAction[] {
  if (status.pendingAction) return [];
  if (status.stage === "delivered") return now < status.claimableAt! ? ["receipt", "dispute"] : ["receipt"];
  if (status.stage === "disputed") return ["withdraw"];
  return [];
}

/** Account address proof (board op 0x79): the app checks it before it shows a deposit address. */
export function accountProofDigest(account: string, challenge: Uint8Array): Uint8Array {
  if (challenge.length !== 32 || !challenge.some(Boolean)) throw new Error("ACCOUNT_CHALLENGE");
  const domain = keccak_256(
    concat(
      text("EIP712Domain(string name,string version,uint256 chainId)"),
      text("NU54 Wallet Account"),
      text("1"),
      bigToBytes(BigInt(q.pilotChain), 32)
    )
  );
  return keccak_256(
    concat(
      Uint8Array.of(0x19, 0x01),
      domain,
      keccak_256(concat(text("AccountProof(address account,bytes32 challenge)"), addressWord(account), challenge))
    )
  );
}

/** What the board signs (0x79) to enroll its account with a store. Fresh for every request. */
export function payerEnrollChallenge(scope: {
  tenantRef: string; branchRef: string; subjectRef: string; payer: string; generation: bigint; idempotencyKey: string;
}): Uint8Array {
  if (!address20.test(scope.payer) || /^0x0+$/.test(scope.payer) || scope.generation < 1n) throw new Error("ORDER_ADDRESS");
  return sha256(utf8Encode(["NU54-payer-enroll-v1", scope.tenantRef, scope.branchRef, scope.subjectRef, scope.payer,
    scope.generation.toString(), scope.idempotencyKey].join("\n")));
}

export type TokenTransfer = { token: string; from: string; to: string; amount: bigint; validBefore: bigint; nonce: string };

/** walletReturnDigestFor with the dollar token. */
export const walletReturnDigest = (transfer: TokenTransfer): Uint8Array => walletReturnDigestFor(orderTokenName, transfer);

/** EIP-3009 TransferWithAuthorization the board signs to send its balance home. */
export function walletReturnDigestFor(tokenName: string, transfer: TokenTransfer): Uint8Array {
  const domain = keccak_256(concat(domainType, tokenNameHash(tokenName), text(orderTokenVersion),
    bigToBytes(BigInt(q.pilotChain), 32), addressWord(transfer.token)));
  return keccak_256(concat(Uint8Array.of(0x19, 0x01), domain, keccak_256(concat(
    text("TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"),
    addressWord(transfer.from), addressWord(transfer.to), bigToBytes(transfer.amount, 32), new Uint8Array(32),
    bigToBytes(transfer.validBefore, 32), hashWord(transfer.nonce)))));
}

export type PayerEnrollment = { payer: string; generation: string; expiresAt: number };
export type WalletReturn = {
  payer: string; to: string; amount: string; validBefore: number;
  state: "queued" | "submitted" | "final" | "failed"; txHash?: string;
};
/** A transfer to any address, newest first. `perDay` is how many the store relays in any 24 hours. */
export type WalletSend = WalletReturn & { token: string; nonce: string; createdAt: number };
export type WalletSends = { perDay: number; remaining: number; sends: WalletSend[] };
export function validWalletSends(value: unknown): value is WalletSends {
  const v = value as WalletSends;
  return !!v && typeof v === "object" && Object.keys(v).sort().join() === "perDay,remaining,sends" &&
    Number.isInteger(v.perDay) && v.perDay > 0 && Number.isInteger(v.remaining) && v.remaining >= 0 && v.remaining <= v.perDay &&
    Array.isArray(v.sends) && v.sends.length <= 20 && v.sends.every((send) => {
      if (!send || typeof send !== "object") return false;
      const { token, nonce, createdAt, ...transfer } = send;
      return typeof token === "string" && address20.test(token) && typeof nonce === "string" && /^[0-9a-f]{64}$/.test(nonce) &&
        Number.isSafeInteger(createdAt) && createdAt > 0 && validWalletReturn(transfer);
    });
}
export function validPayerEnrollment(value: unknown): value is PayerEnrollment {
  const v = value as PayerEnrollment;
  return !!v && typeof v === "object" && Object.keys(v).sort().join() === "expiresAt,generation,payer" &&
    address20.test(v.payer) && !/^0x0+$/.test(v.payer) && /^[1-9][0-9]{0,18}$/.test(v.generation) &&
    Number.isSafeInteger(v.expiresAt) && v.expiresAt > 0;
}
export function validWalletReturn(value: unknown): value is WalletReturn {
  const v = value as WalletReturn;
  return !!v && typeof v === "object" &&
    Object.keys(v).every((key) => ["payer", "to", "amount", "validBefore", "state", "txHash"].includes(key)) &&
    [v.payer, v.to].every((a) => typeof a === "string" && address20.test(a)) && /^[1-9][0-9]{0,77}$/.test(v.amount) &&
    Number.isSafeInteger(v.validBefore) && v.validBefore > 0 && ["queued", "submitted", "final", "failed"].includes(v.state) &&
    (v.txHash === undefined ? v.state === "queued" || v.state === "failed" : txHash.test(v.txHash) && v.state !== "queued");
}
