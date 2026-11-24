import {accountProofDigest, bigToBytes, bytesToBig, bytesToHex, concat, equalBytes, hexToBytes, recoverSigner} from '@nu54/protocol';

export const OWNER_GATT = {
  service: '41ba05a0-e0dd-4fd7-901c-4c25d5010000',
  input: '41ba05a0-e0dd-4fd7-901c-4c25d5010001',
  output: '41ba05a0-e0dd-4fd7-901c-4c25d5010002',
} as const;

export type OwnerStatus = {
  /** `released`: a factory reset removed the owner; the board takes a new registration. */
  state: 'new' | 'active' | 'pending' | 'revoked' | 'released';
  generation: bigint;
  device: string;
  owner: string;
  root: number;
  profile: number;
  backedUp: boolean;
  /** Status byte 4 bit 1: the board holds no legacy BLE bond, so no previous phone keeps one. */
  legacyBondsClear?: boolean;
};

export type Enrollment = {
  version: 2;
  device: string;
  boardSPKI: string;
  ca: string;
  certificateChain: string[];
};

const fingerprint = /^[a-f0-9]{64}$/;
const certificate = /^[A-Za-z0-9+/]+={0,2}$/;

export type CommissioningPins = Pick<Enrollment, 'device' | 'boardSPKI'>;

export function readCommissioningQr(uri: string): CommissioningPins | undefined {
  const match = /^nu54:\/\/commissioning\/v1\/([a-f0-9]{64})\/([a-f0-9]{64})$/.exec(uri);
  if (!match || match[0] !== uri || /^0+$/.test(match[1]) || /^0+$/.test(match[2])) return undefined;
  return {device: match[1], boardSPKI: match[2]};
}

// The operator supplies public certificates; the independently displayed board
// fingerprint is entered separately before a session can authenticate.
export function importEnrollment(raw: string, independentDevice: string, independentPin: string): Enrollment {
  if (raw.length > 8000) throw new Error('PROFILE_INVALID');
  const item = JSON.parse(raw) as Enrollment;
  if (!item || Object.keys(item).sort().join() !== 'boardSPKI,ca,certificateChain,device,version' ||
      item.version !== 2 || !fingerprint.test(item.device) || !fingerprint.test(item.boardSPKI) ||
      !fingerprint.test(independentDevice) || !fingerprint.test(independentPin) ||
      item.device !== independentDevice || item.boardSPKI !== independentPin ||
      typeof item.ca !== 'string' || !Array.isArray(item.certificateChain) ||
      item.certificateChain.length < 1 || item.certificateChain.length > 4 ||
      [item.ca, ...item.certificateChain].some(value => typeof value !== 'string' || !certificate.test(value))) {
    throw new Error('PROFILE_NOT_AUTHENTICATED');
  }
  return item;
}

export function readStatus(payload: Uint8Array, device: string): OwnerStatus {
  const states = ['new', 'active', 'pending', 'revoked', 'released'] as const;
  if (payload.length !== 77 || payload[0] !== 3 || !states[payload[1]] || payload[4] > 3 ||
      bytesToHex(payload.subarray(13,45), false) !== device) throw new Error('OWNER_BINDING');
  return {
    state: states[payload[1]], root: payload[2], profile: payload[3], backedUp: payload[4] % 2===1, legacyBondsClear: payload[4] >= 2,
    generation: bytesToBig(payload.subarray(5,13)), device, owner: bytesToHex(payload.subarray(45), false),
  };
}

export function ownerBegin(operation: 1 | 2 | 3 | 5, owner: string): Uint8Array {
  if (!fingerprint.test(owner) || owner === '0'.repeat(64)) throw new Error('OWNER_BINDING');
  return concat(Uint8Array.of(1, operation), hexToBytes(owner));
}

export function validateChallenge(payload: Uint8Array, expected: {
  operation: 1 | 2 | 3 | 5; device: string; owner: string; generation: bigint; id: Uint8Array; expires: bigint; now: bigint;
}): void {
  if (payload.length !== 130 || payload[0] !== 1 || payload[1] !== expected.operation ||
      bytesToHex(payload.subarray(2,34),false) !== expected.device ||
      !equalBytes(payload.subarray(34,42),bigToBytes(expected.generation,8)) ||
      bytesToHex(payload.subarray(42,74),false) !== expected.owner ||
      !payload.subarray(74,106).some(Boolean) || !equalBytes(payload.subarray(106,122),expected.id) ||
      bytesToBig(payload.subarray(122)) !== expected.expires || expected.expires <= expected.now) {
    throw new Error('OWNER_CHALLENGE');
  }
}

export function ownerProof(challenge: Uint8Array, key: Uint8Array, signature: Uint8Array): Uint8Array {
  if (challenge.length !== 130 || key.length !== 91 || signature.length < 64 || signature.length > 80) {
    throw new Error('OWNER_PROOF');
  }
  return concat(Uint8Array.of(2), challenge, Uint8Array.of(key.length), key, Uint8Array.of(signature.length), signature);
}


export function readOwnerAddress(payload: Uint8Array, device: string) {
  if (payload.length !== 97 || !payload.subarray(77).some(Boolean)) throw new Error('OWNER_ADDRESS_BINDING');
  return {status:readStatus(payload.subarray(0,77),device),address:bytesToHex(payload.subarray(77),true)};
}

/** Spending wallet status (board op 0x7f): what the board will sign for, without asking for a button. */
export type SpendingStatus = {
  account: string; returnAddress?: string; perPayment: bigint; daily: bigint; pinWait: number; orderOpen: boolean;
  /** Left of today's total; 0 also while the board has no trusted time. */
  leftToday: bigint;
  /** The firmware signs transfers to an address the owner chose. */
  sends: boolean;
};
export const spendingStatusOperation = 0x7f;
export const accountProofOperation = 0x79;
export const setReturnOperation = 0x7a, returnOperation = 0x7b, resetOperation = 0x7c, resetNonceOperation = 0x7d, operatorResetOperation = 0x7e;
/** op, token(20), to(20), amount(8), validBefore(8), nonce(32). Boards without it never get one: see `sends`. */
export const sendOperation = 0x80;

export function readSpendingStatus(payload: Uint8Array): SpendingStatus {
  // One more byte of capabilities, when the firmware has any: 1 is sending.
  if ((payload.length !== 71 && payload.length !== 72) || (payload.length === 72 && payload[71] !== 1) || payload[0] !== spendingStatusOperation || payload[1] > 1 || payload[62] > 1 ||
      !payload.subarray(2, 22).some(Boolean) || payload.subarray(22, 42).some(Boolean) !== (payload[1] === 1))
    throw new Error('SPENDING_STATUS');
  return {
    account: bytesToHex(payload.subarray(2, 22), true),
    ...(payload[1] ? {returnAddress: bytesToHex(payload.subarray(22, 42), true)} : {}),
    perPayment: bytesToBig(payload.subarray(42, 50)), daily: bytesToBig(payload.subarray(50, 58)),
    pinWait: Number(bytesToBig(payload.subarray(58, 62))), orderOpen: payload[62] === 1,
    leftToday: bytesToBig(payload.subarray(63, 71)), sends: payload[71] === 1,
  };
}

/** The board account signed this session's one-time value. The pinned TLS peer vouches for the board. */
export function readAccountProof(payload: Uint8Array, challenge: Uint8Array): {account: string; signature: string} {
  if (payload.length !== 150 || payload[0] !== accountProofOperation) throw new Error('ACCOUNT_PROOF');
  const account = bytesToHex(payload.subarray(1, 21), true);
  if (recoverSigner(accountProofDigest(account, challenge), payload.subarray(21, 86)) !== account) throw new Error('ACCOUNT_PROOF');
  return {account, signature: bytesToHex(payload.subarray(21, 86), false)};
}
