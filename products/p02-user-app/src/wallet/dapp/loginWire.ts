import {p256} from '@noble/curves/p256';
import {secp256k1} from '@noble/curves/secp256k1';
import {sha256} from '@noble/hashes/sha256';
import {keccak_256} from '@noble/hashes/sha3';
import {bigToBytes,bytesToBig,bytesToHex,concat,hexToBytes} from '@nu54/protocol';
import {parseSiwe,type DappScope} from './gate.ts';

const domain=Uint8Array.of(0x4e,0x35,0x34,0x53);
export type IssuedLogin={wire:string;nonce:string;expires:number};

export function readIssuedLogin(response:unknown, message:string, origin:string, scope:DappScope, lease:bigint, authority:Uint8Array, now:number) {
  if (!response || typeof response!=='object') throw new Error('DAPP_LOGIN_AUTHORITY');
  const issuance=response as IssuedLogin;
  if (typeof issuance.wire!=='string' || !/^(?:[a-f0-9]{2}){124,820}$/.test(issuance.wire) ||
      !Number.isSafeInteger(issuance.expires) || typeof issuance.nonce!=='string') throw new Error('DAPP_LOGIN_AUTHORITY');
  const body=hexToBytes(issuance.wire), signedEnd=body.length-64;
  const hostname=new URL(origin).host, offset=58+body[57];
  if (body[0]!==15 || body[57]>96 || bytesToBig(body.subarray(1,9))!==BigInt(issuance.expires) ||
      bytesToBig(body.subarray(9,17))!==BigInt(scope.chain) || bytesToHex(body.subarray(17,49),true)!==scope.genesis ||
      bytesToBig(body.subarray(49,57))!==lease || String.fromCharCode(...body.subarray(58,offset))!==hostname ||
      offset+2>signedEnd || bytesToBig(body.subarray(offset,offset+2))!==BigInt(signedEnd-offset-2) ||
      String.fromCharCode(...body.subarray(offset+2,signedEnd))!==message) throw new Error('DAPP_LOGIN_BINDING');
  const digest=parseSiwe(message,origin,scope,{origin,nonce:issuance.nonce,expires:issuance.expires,account:scope.account,chain:scope.chain},now).digest;
  const signed=concat(domain,hexToBytes(scope.board),bigToBytes(BigInt(scope.generation),8),hexToBytes(scope.account.slice(2)),body.subarray(0,signedEnd));
  if (!p256.verify(body.subarray(signedEnd),sha256(signed),authority,{lowS:false})) throw new Error('DAPP_LOGIN_AUTHORITY');
  return {body,digest,nonce:issuance.nonce,expires:issuance.expires};
}

export function checkLoginSignature(signature:Uint8Array,digest:Uint8Array,account:string) {
  if (signature.length!==65 || (signature[64]!==27 && signature[64]!==28)) throw new Error('DAPP_SIGNATURE');
  const signatureObject=secp256k1.Signature.fromCompact(signature.subarray(0,64));
  if (signatureObject.hasHighS()) throw new Error('DAPP_SIGNATURE');
  const key=signatureObject.addRecoveryBit(signature[64]-27).recoverPublicKey(digest).toRawBytes(false);
  if (bytesToHex(keccak_256(key.subarray(1)).subarray(12),true)!==account) throw new Error('DAPP_SIGNATURE_ACCOUNT');
  return bytesToHex(signature,true);
}
