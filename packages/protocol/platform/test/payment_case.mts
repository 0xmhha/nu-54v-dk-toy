import {readFileSync, writeFileSync} from 'node:fs';
import {join} from 'node:path';
import {secp256k1} from '@noble/curves/secp256k1';
import {keccak_256} from '@noble/hashes/sha3';
import {quoteDigest, quoteV2Layout} from '../../ts/src/quote-v2.ts';
import {addressOfPrivateKey, recoverSigner, signDigest} from '../../ts/src/signature.ts';

const folder = process.argv[2];
if (process.argv[3]) {
  const {digest,payer} = JSON.parse(readFileSync(join(folder,'expected.json'),'utf8')) as {digest:string; payer:string};
  const raw = readFileSync(process.argv[3]);
  if (recoverSigner(Buffer.from(digest,'hex'),raw.subarray(33,98)) !== payer) throw new Error('unexpected payment signer');
  process.exit(0);
}

const vectorFile = new URL('../../v2/testdata/quote.vectors',import.meta.url);
const vector = readFileSync(vectorFile,'utf8').split('\n').find((line)=>line.startsWith('phone-private|'))!.split('|');
const expected = Buffer.from(vector[2],'hex');
const payerKey = secp256k1.utils.randomPrivateKey(), merchantKey = secp256k1.utils.randomPrivateKey();
const payer = addressOfPrivateKey(payerKey);
expected.set(Buffer.from(payer.slice(2),'hex'),quoteV2Layout.payerOffset+12);
expected.writeBigUInt64BE(1n,quoteV2Layout.payerGenerationOffset+24);
const now = BigInt(Math.floor(Date.now()/1000));
expected.writeBigUInt64BE(now-1n,quoteV2Layout.issuedAtOffset+24);
expected.writeBigUInt64BE(now+180n,quoteV2Layout.expiresAtOffset+24);
const quote = Buffer.from(quoteDigest(expected));
const h = (text:string) => Buffer.from(keccak_256(Buffer.from(text)));
const separator = keccak_256(Buffer.concat([h(quoteV2Layout.domainType),h(quoteV2Layout.domainName),
  h(quoteV2Layout.domainVersion),expected.subarray(0,64)]));
const approval = keccak_256(Buffer.concat([Buffer.of(0x19,1),separator,
  keccak_256(Buffer.concat([h('PaymentApproval(bytes32 quoteDigest)'),quote]))]));
const time = Buffer.alloc(8);
time.writeBigUInt64BE(now);
writeFileSync(join(folder,'quote.bin'),Buffer.concat([expected,signDigest(quote,merchantKey)]));
writeFileSync(join(folder,'case.bin'),Buffer.concat([expected,Buffer.from(addressOfPrivateKey(merchantKey).slice(2),'hex'),time,payerKey]),{mode:0o600});
writeFileSync(join(folder,'expected.json'),JSON.stringify({digest:Buffer.from(approval).toString('hex'),payer,expiry:Number(now+120n)}));
