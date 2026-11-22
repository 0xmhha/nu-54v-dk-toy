import {p256} from '@noble/curves/p256';
import {secp256k1} from '@noble/curves/secp256k1';
import {sha256} from '@noble/hashes/sha256';
import {addressOfPublicKey, bigToBytes, concat, hexToBytes, utf8Encode} from '@nu54/protocol';
import {validateRecovery, type RecoveryContext, type RecoveryTrust} from '../src/wallet/recoveryProof.ts';

const source = 'f0'.repeat(32);
const target = 'd0'.repeat(32);
const owner = 'a1'.repeat(32);
const context: RecoveryContext = {
  challenge: new Uint8Array(32).fill(14), id: new Uint8Array(16).fill(3), target, owner,
  epoch: new Uint8Array(16).fill(23), generation: 4n, expires: 2200n,
};
const key = (n: number) => new Uint8Array(32).fill(n);
const x509 = (n: number) => concat(hexToBytes('3059301306072a8648ce3d020106082a8648ce3d030107034200'), p256.getPublicKey(key(n),false));
const sign = (n: number, input: Uint8Array) => p256.sign(sha256(input),key(n)).toDERRawBytes();
const lengthPrefix = (value: Uint8Array) => concat(Uint8Array.of(value.length),value);
const trust: RecoveryTrust = {
  source, sourceGeneration: 1n,
  sourceKey: x509(1), targetKey: x509(2), accessoryKey: x509(3),
};
const root = Uint8Array.of(2,1);
const derivation = concat(Uint8Array.of(1),bigToBytes(0n,4),bigToBytes(8283n,8));
const publicWallet = secp256k1.getPublicKey(key(11),false);
const address = addressOfPublicKey(publicWallet);
const claim = concat(utf8Encode('NU54-backup-claim-v1\0'), root, hexToBytes(source), bigToBytes(1n,8),
  derivation, publicWallet,hexToBytes(address));
const unsigned = concat(utf8Encode('NU54-recovery-receipt-v1\0'),context.challenge,context.id,root,
  hexToBytes(source),hexToBytes(target),derivation,publicWallet,hexToBytes(address),hexToBytes(owner),
  context.epoch,bigToBytes(1n,8),bigToBytes(4n,8),Uint8Array.of(1),bigToBytes(2000n,8),bigToBytes(2199n,8));

function envelope(body = unsigned, accessory = 3, original = 1) {
  return concat(body,lengthPrefix(sign(2,body)),lengthPrefix(sign(accessory,body)),lengthPrefix(sign(original,claim)));
}

test('all independently pinned roles bind original and recovered accounts to the live session', () => {
  expect(unsigned.length).toBe(318);
  expect(claim.length).toBe(161);
  expect(validateRecovery(envelope(), context, trust, 2120n)).toEqual({
    source, target, address, network:'8283', sourceGeneration:1, generation:4,
  });
});

test('missing or replaced independent signatures and copied board roles fail closed', () => {
  expect(() => validateRecovery(envelope(unsigned,2), context, trust, 2120n)).toThrow();
  expect(() => validateRecovery(envelope(unsigned,3,2), context, trust, 2120n)).toThrow();
  expect(() => validateRecovery(envelope().subarray(0,-1), context, trust, 2120n)).toThrow();
  expect(() => validateRecovery(envelope(), context, {...trust,accessoryKey:trust.targetKey}, 2120n)).toThrow();
});

test('re-signed wrong scope, device, address or challenge and replay after reconnect are refused', () => {
  for (const offset of [25,57,73,75,107,139,143,151,217,237,269,292,300,301]) {
    const changed = unsigned.slice();
    changed[offset] = (changed[offset] + 1) % 256;
    expect(() => validateRecovery(envelope(changed),context,trust,2120n)).toThrow();
  }
  expect(() => validateRecovery(envelope(), {...context,epoch:new Uint8Array(16).fill(22)}, trust, 2120n)).toThrow();
  expect(() => validateRecovery(envelope(), {...context,generation:3n}, trust, 2120n)).toThrow();
  expect(() => validateRecovery(envelope(), context, trust, 2199n)).toThrow();
  expect(() => validateRecovery(envelope(), context, trust, 1999n)).toThrow();
});
