import {chacha20poly1305} from '@noble/ciphers/chacha';
import {ed25519,x25519} from '@noble/curves/ed25519';
import {hkdf} from '@noble/hashes/hkdf';
import {sha256} from '@noble/hashes/sha256';
import {bytesToHex,concat,fromBase64,hexToBytes,toBase64,utf8Decode,utf8Encode} from '@nu54/protocol';

const alphabet='123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';
const base64url=(value:Uint8Array)=>toBase64(value).replace(/=/g,'').replace(/\+/g,'-').replace(/\//g,'_');
export const topicOf=(key:Uint8Array)=>bytesToHex(sha256(key),false);

export function sessionMaterial(secret:Uint8Array,peer:string) {
  const key=hkdf(sha256,x25519.getSharedSecret(secret,hexToBytes(peer)),undefined,undefined,32);
  return {publicKey:bytesToHex(x25519.getPublicKey(secret),false),topic:topicOf(key),key};
}

export function seal(key:Uint8Array,nonce:Uint8Array,payload:unknown) {
  const bytes=utf8Encode(JSON.stringify(payload));
  if (bytes.length>8192 || nonce.length!==12) throw new Error('WC_ENVELOPE_SIZE');
  return toBase64(concat(Uint8Array.of(0),nonce,chacha20poly1305(key,nonce).encrypt(bytes)));
}

export function open(key:Uint8Array,encoded:string):unknown {
  if (encoded.length>11000 || !/^[A-Za-z0-9+/]+={0,2}$/.test(encoded)) throw new Error('WC_ENVELOPE');
  const bytes=fromBase64(encoded);
  if (bytes.length<30 || bytes[0]!==0 || toBase64(bytes)!==encoded) throw new Error('WC_ENVELOPE_TYPE');
  return JSON.parse(utf8Decode(chacha20poly1305(key,bytes.subarray(1,13)).decrypt(bytes.subarray(13))));
}

export function relayAuthorization(seed:Uint8Array,subject:Uint8Array,audience:string,now:number) {
  const publicKey=ed25519.getPublicKey(seed);
  let encodedKey='', number=BigInt('0x'+bytesToHex(concat(Uint8Array.of(0xed,1),publicKey),false));
  while (number>0n) {
    encodedKey=alphabet[Number(number%58n)]+encodedKey;
    number/=58n;
  }
  const payload={iss:'did:key:z'+encodedKey,sub:bytesToHex(subject,false),aud:audience,iat:now,exp:now+86400};
  const statement=base64url(utf8Encode(JSON.stringify({alg:'EdDSA',typ:'JWT'})))+'.'+base64url(utf8Encode(JSON.stringify(payload)));
  return statement+'.'+base64url(ed25519.sign(utf8Encode(statement),seed));
}
