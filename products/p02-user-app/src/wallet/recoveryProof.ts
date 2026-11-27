import {p256} from '@noble/curves/p256';
import {secp256k1} from '@noble/curves/secp256k1';
import {sha256} from '@noble/hashes/sha256';
import {addressOfPublicKey, bytesToBig, bytesToHex, concat, equalBytes, hexToBytes, utf8Encode} from '@nu54/protocol';

export type RecoveryContext = {
  challenge: Uint8Array;
  id: Uint8Array;
  target: string;
  owner: string;
  epoch: Uint8Array;
  generation: bigint;
  expires: bigint;
};

export type RecoveryTrust = {
  source: string;
  sourceGeneration: bigint;
  sourceKey: Uint8Array;
  targetKey: Uint8Array;
  accessoryKey: Uint8Array;
};

const receiptDomain = utf8Encode('NU54-recovery-receipt-v1\0');
const backupDomain = utf8Encode('NU54-backup-claim-v1\0');
const p256Spki = hexToBytes('3059301306072a8648ce3d020106082a8648ce3d030107034200');

function assure(value: boolean): asserts value {
  if (!value) throw new Error('RECOVERY_EVIDENCE_REJECTED');
}

function signature(key: Uint8Array, sig: Uint8Array, body: Uint8Array) {
  assure(key.length === 91 && equalBytes(key.subarray(0,26),p256Spki));
  const parsed = p256.Signature.fromDER(sig);
  assure(equalBytes(parsed.toDERRawBytes(),sig));
  assure(p256.verify(parsed.toCompactRawBytes(),sha256(body),key.subarray(26),{lowS:false}));
}

/** Verifies public evidence only. Callers must supply their pinned enrollment and live C1 context. */
export function validateRecovery(envelope: Uint8Array, session: RecoveryContext, trust: RecoveryTrust, now: bigint) {
  assure(envelope.length >= 318 + 3*9 && envelope.length <= 318 + 3*73);
  const body = envelope.subarray(0,318);
  const field = (start: number, end: number) => body.subarray(start,end);
  const hex = (start: number, end: number) => bytesToHex(field(start,end),false);
  const originalGeneration = bytesToBig(field(285,293));
  const generation = bytesToBig(field(293,301));
  const issued = bytesToBig(field(302,310));
  const expires = bytesToBig(field(310,318));
  const source = hex(75,107);
  const target = hex(107,139);
  assure(equalBytes(field(0,25),receiptDomain) && equalBytes(field(25,57),session.challenge) &&
    equalBytes(field(57,73),session.id) && session.challenge.some(Boolean) && session.id.some(Boolean));
  assure(body[73] === 2 && body[74] === 1 && body[139] === 1 &&
    bytesToBig(field(140,144)) === 0n && bytesToBig(field(144,152)) === 8283n && body[301] === 1);
  assure(source === trust.source && target === session.target && source !== target &&
    hex(237,269) === session.owner && equalBytes(field(269,285),session.epoch) && session.epoch.some(Boolean));
  assure(originalGeneration === trust.sourceGeneration && generation === session.generation &&
    originalGeneration > 0n && generation > 0n &&
    originalGeneration <= BigInt(Number.MAX_SAFE_INTEGER) && generation <= BigInt(Number.MAX_SAFE_INTEGER));
  assure(issued > 0n && issued <= now && now < expires && expires <= session.expires && expires-issued <= 300n);
  assure(!equalBytes(trust.sourceKey,trust.targetKey) && !equalBytes(trust.sourceKey,trust.accessoryKey) &&
    !equalBytes(trust.targetKey,trust.accessoryKey));

  const walletKey = field(152,217);
  assure(walletKey[0] === 4);
  secp256k1.ProjectivePoint.fromHex(walletKey).assertValidity();
  const address = addressOfPublicKey(walletKey);
  assure(equalBytes(hexToBytes(address),field(217,237)));
  const backup = concat(backupDomain,field(73,75),field(75,107),field(285,293),
    field(139,152),walletKey,field(217,237));

  let cursor = 318;
  for (const [key,signed] of [[trust.targetKey,body],[trust.accessoryKey,body],[trust.sourceKey,backup]]) {
    const size = envelope[cursor++];
    assure(size >= 8 && size <= 72 && cursor + size <= envelope.length);
    signature(key,envelope.subarray(cursor,cursor+size),signed);
    cursor += size;
  }
  assure(cursor === envelope.length);
  return {source,target,address,network:'8283' as const,sourceGeneration:Number(originalGeneration),generation:Number(generation)};
}

/** Provisioned public verifier keys and their independently delivered SPKI SHA-256 pins. */
export function importRecoveryTrust(raw: string, pins: string[], target: string): RecoveryTrust {
  assure(raw.length <= 2048 && pins.length === 3);
  const trust = JSON.parse(raw) as Record<string, unknown>;
  assure(!!trust && Object.keys(trust).sort().join() === 'accessory,original,source,sourceGeneration,target,targetBoard');
  const fingerprint = /^[a-f0-9]{64}$/;
  const {source, sourceGeneration} = trust;
  assure(typeof source === 'string' && fingerprint.test(source) && source !== target && trust.targetBoard === target);
  assure(typeof sourceGeneration === 'number' && Number.isSafeInteger(sourceGeneration) && sourceGeneration > 0);
  const keys = [trust.original,trust.target,trust.accessory].map((value,index) => {
    assure(typeof value === 'string' && /^[a-f0-9]{182}$/.test(value) && fingerprint.test(pins[index]));
    const key = hexToBytes(value);
    assure(equalBytes(key.subarray(0,26),p256Spki) && bytesToHex(sha256(key),false) === pins[index]);
    p256.ProjectivePoint.fromHex(key.subarray(26)).assertValidity();
    return key;
  });
  assure(new Set(pins).size === 3);
  return {source,sourceGeneration:BigInt(sourceGeneration),sourceKey:keys[0],targetKey:keys[1],accessoryKey:keys[2]};
}
