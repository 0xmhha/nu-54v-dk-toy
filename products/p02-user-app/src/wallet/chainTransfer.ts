import {commissionedProfile, readCommissionedChains, verifyCommissionedAnchor} from './commissionedChainProfiles.ts';
import {ed25519} from '@noble/curves/ed25519';
import {p256} from '@noble/curves/p256';
import {hmac} from '@noble/hashes/hmac';
import {sha256} from '@noble/hashes/sha256';
import {secp256k1} from '@noble/curves/secp256k1';
import {sha512} from '@noble/hashes/sha512';
import {bigToBytes, bytesToHex, equalBytes, hexToBytes, toBase64, utf8Encode} from '@nu54/protocol';
import {base58, bitcoinHrp, bitcoinSignetCheckpoint, chainAccountCommand, classicXrpl, doubleSha256, hash160,
  normalizeXrplDestination, segwitAddress, type ChainAccount, type ChainPublicAccount} from './chainAccounts.ts';

const wireHeader = 153, wireOffset = wireHeader + 64;
const toHex = (bytes: Uint8Array) => bytesToHex(bytes, false);
const be = (bytes: Uint8Array) => bytes.reduce((n, byte) => 256n * n + BigInt(byte), 0n);
const u32 = (bytes: Uint8Array, at: number) => Number(be(bytes.subarray(at, at + 4)));
const le = (bytes: Uint8Array) => be(bytes.slice().reverse());
const cat = (...bytes: Uint8Array[]) => Uint8Array.of(...bytes.flatMap(chunk => Array.from(chunk)));

export type ChainTransfer = ChainAccount & {body: Uint8Array; raw: Uint8Array; expires: number};
type XrpAmount = string | {currency: string; issuer: string; value: string};
type SolContext = {blockhash: string; height: number; minimumSlot?: number};
type XrpContext = {firstLedger?: number; lastLedger: number; destination: string; destinationTag?: number; amount: XrpAmount};
export type ChainCheckpoint = ChainAccount & {signed: string; id: string; attempted: boolean;
  expires: number; commissionedGrant?:string; btc?: {fee: number; vsize?:number; replaceable: boolean; previous: string[]; grantHash?: string}; sol?: SolContext; xrp?: XrpContext};

export function readChainTransfer(text: string, now = Math.floor(Date.now() / 1000), op: 11 | 18 = 11): ChainTransfer {
  if (!(op===18 ? /^(?:[a-f0-9]{2}){218,1997}$/ : /^(?:[a-f0-9]{2}){218,1800}$/).test(text)) throw new Error('CHAIN_TRANSFER_WIRE');
  const body = hexToBytes(text);
  const profile = {chain: body[1] as ChainAccount['chain'], network: body[2] as ChainAccount['network'],
    account: u32(body, 3)};
  const expires = Number(be(body.subarray(7, 15)));
  const command = chainAccountCommand(11, profile);
  command[0] = op;
  if (!equalBytes(body.subarray(0, 7), command) ||
      !Number.isSafeInteger(expires) || expires <= now || expires - now > 60)
    throw new Error('CHAIN_TRANSFER_PROFILE');
  return {...profile, body, raw: body.slice(wireOffset), expires};
}

export function verifyChainAuthority(grant: ChainTransfer, key: Uint8Array, board: string, generation: bigint,
  revision: bigint, anchor: string) {
  if (key.length !== 65 || !/^[a-f0-9]{64}$/.test(board) || generation < 1n ||
      !equalBytes(sha256(Uint8Array.from(anchor, char => char.charCodeAt(0))), grant.body.subarray(113, 145)) ||
      be(grant.body.subarray(145, wireHeader)) !== revision) throw new Error('CHAIN_GRANT_SCOPE');
  const signed = cat(Uint8Array.of(0x4e, 0x35, 0x34, 0x4d), hexToBytes(board), bigToBytes(generation, 8),
    grant.body.subarray(0, wireHeader), sha256(grant.raw));
  if (!p256.verify(grant.body.subarray(wireHeader, wireOffset), sha256(signed), key, {lowS: false}))
    throw new Error('CHAIN_GRANT_SIGNATURE');
}

export type ChainDraft = {
  destination: string; amount: string; asset?: string; source?: string; issuer?:string; tag?: number;
  outpoint?: string; replaceable?: boolean; fee?: number; replaces?: string;
};

export async function prepareChainTransfer(rpc: ChainRpc, account: ChainPublicAccount, change: ChainPublicAccount | undefined,
  draft: ChainDraft, anchor: string, board: string, generation: bigint, revision: bigint,
  authority: Uint8Array) {
  if (account.network===2 && (draft.asset || draft.source || draft.issuer || draft.replaces || draft.replaceable))
    throw new Error('CHAIN_COMMISSIONED_SCOPE');
  const canonical = account.chain===3 ?
    {...draft,...normalizeXrplDestination(draft.destination,draft.tag,account.network!==0)} : draft;
  const request={...canonical};
  // The issuer is verified against the returned native bytes and is not a legacy gateway argument.
  delete request.issuer;
  const response = record(await rpc('nu54_prepareWalletTransfer', [{...request, chain: account.chain, network: account.network,
    account: account.account, anchor, board, generation: generation.toString(), leaseRevision: revision.toString(),
    publicKey: toHex(account.publicKey), sender: account.address, fingerprint: account.fingerprint,
    change: change ? toHex(change.publicKey) : undefined}]));
  if (typeof response.grant !== 'string') throw new Error('CHAIN_GRANT_SCHEMA');
  const grant = readChainTransfer(response.grant);
  if (grant.chain !== account.chain || grant.network !== account.network || grant.account !== account.account)
    throw new Error('CHAIN_GRANT_PROFILE');
  verifyChainAuthority(grant, authority, board, generation, revision, anchor);
  return {...grant, review: checkPreparedIntent(grant, canonical, account, change)};
}

class Reader {
  offset = 0;
  constructor(readonly data: Uint8Array) {}
  take(size: number) {
    if (size < 0 || size > this.data.length - this.offset) throw new Error('CHAIN_TRANSACTION_LENGTH');
    const value = this.data.subarray(this.offset, this.offset + size);
    this.offset += size;
    return value;
  }
  byte() { return this.take(1)[0]; }
  expect(value: number) {
    if (this.byte() !== value) throw new Error('CHAIN_TRANSACTION_FIELD');
  }
  compact() {
    const first = this.byte();
    if (first < 253) return first;
    const n = first === 253 ? Number(le(this.take(2))) : 0;
    if (n < 253) throw new Error('BTC_COMPACT_SIZE');
    return n;
  }
  binary() { return this.take(this.compact()); }
  done() {
    if (this.offset !== this.data.length) throw new Error('CHAIN_TRANSACTION_TRAILING');
  }
}

type PSBTMap = {key: Uint8Array; value: Uint8Array}[];
function psbtMap(input: Reader): PSBTMap {
  const fields: PSBTMap = [];
  while (true) {
    const key = input.binary();
    if (!key.length) return fields;
    if (fields.length >= 8 || fields.some(field => equalBytes(field.key, key))) throw new Error('BTC_PSBT_MAP');
    fields.push({key, value: input.binary()});
  }
}
function mapField(map: PSBTMap, id: number, keyLength = 1) {
  const field = map.find(entry => entry.key.length === keyLength && entry.key[0] === id);
  if (!field) throw new Error('BTC_PSBT_FIELD');
  return field;
}
function btcTxPrefix(input: Reader, maximumCount = 4) {
  const inputCount = input.compact();
  if (inputCount < 1 || inputCount > maximumCount) throw new Error('BTC_INPUT_COUNT');
  const outpoints: Uint8Array[] = [];
  const sequences: Uint8Array[] = [];
  for (let i = 0; i < inputCount; i++) {
    outpoints.push(input.take(36));
    input.binary();
    sequences.push(input.take(4));
  }
  const outputCount = input.compact();
  if (outputCount < 1 || outputCount > maximumCount) throw new Error('BTC_OUTPUT_COUNT');
  const start = input.offset;
  const prevouts: {amount: Uint8Array; script: Uint8Array}[] = [];
  for (let i = 0; i < outputCount; i++) prevouts.push({amount: input.take(8), script: input.binary()});
  return {inputCount, outputCount, outpoints, sequences, prevouts, outputs: input.data.subarray(start, input.offset)};
}
export function previousBitcoinOutput(parent: Uint8Array, outpoint: Uint8Array, maximumCount = 4) {
  if (outpoint.length !== 36) throw new Error('BTC_OUTPOINT');
  if (!equalBytes(doubleSha256(parent), outpoint.subarray(0,32))) throw new Error('BTC_OUTPOINT');
  const previous = new Reader(parent);
  previous.take(4);
  const {prevouts:available} = btcTxPrefix(previous,maximumCount);
  previous.take(4);
  previous.done();
  const spent = available[Number(le(outpoint.subarray(32)))];
  if (!spent) throw new Error('BTC_OUTPOINT');
  return spent;
}
function readBtcPrevout(fields: PSBTMap, outpoint: Uint8Array) {
  const key = mapField(fields, 6, 34).key.subarray(1);
  const spent = previousBitcoinOutput(mapField(fields, 0).value, outpoint);
  const keyHash = hash160(key);
  if (!equalBytes(spent.script,Uint8Array.of(0,20,...keyHash))) throw new Error('BTC_SPENT_SCRIPT');
  return {spent,key,keyHash};
}

export type ChainReview = {sender:string; destination:string; amount:string; asset:string; fee:string;
  feeUnit:string; maximumFee:string; fields:{label:string; value:string}[]};

function verifyBtc(signed: Uint8Array, psbt: Uint8Array) {
  const source = new Reader(psbt);
  if (toHex(source.take(5)) !== '70736274ff') throw new Error('BTC_PSBT_MAGIC');
  const unsigned = mapField(psbtMap(source), 0).value;
  const raw = new Reader(signed);
  const version = raw.take(4);
  raw.expect(0);
  raw.expect(1);
  const tx = btcTxPrefix(raw);
  const noWitness = signed.subarray(6, raw.offset);
  const prevouts = doubleSha256(cat(...tx.outpoints));
  const sequences = doubleSha256(cat(...tx.sequences));
  const outputs = doubleSha256(tx.outputs);
  const lock = signed.subarray(signed.length - 4);
  let total = 0n;
  for (let i = 0; i < tx.inputCount; i++) {
    const {spent, key, keyHash} = readBtcPrevout(psbtMap(source),tx.outpoints[i]);
    total += le(spent.amount);
    raw.expect(2);
    const signature = raw.binary();
    if (signature.at(-1) !== 1 || !equalBytes(raw.binary(), key)) throw new Error('BTC_WITNESS');
    const scriptCode = Uint8Array.of(25, 0x76, 0xa9, 20, ...keyHash, 0x88, 0xac);
    const digest = doubleSha256(cat(version, prevouts, sequences, tx.outpoints[i], scriptCode, spent.amount,
      tx.sequences[i], outputs, lock, Uint8Array.of(1, 0, 0, 0)));
    if (!secp256k1.verify(signature.subarray(0, -1), digest, key, {format: 'der', lowS: true}))
      throw new Error('BTC_SIGNATURE');
  }
  for (let i = 0; i < tx.outputCount; i++) psbtMap(source);
  source.done();
  raw.take(4);
  raw.done();
  if (!equalBytes(cat(version, noWitness, lock), unsigned)) throw new Error('BTC_UNSIGNED_MISMATCH');
  const fee = total - tx.prevouts.reduce((sum, output) => sum + le(output.amount), 0n);
  if (fee < 0n || fee > 1000000n) throw new Error('BTC_FEE');
  return {id: toHex(doubleSha256(unsigned).reverse()), fee: Number(fee),
    vsize:Math.ceil((unsigned.length * 3 + signed.length) / 4), replaceable: tx.sequences.every(sequence => le(sequence) === 0xfffffffdn) && tx.outputCount === 2};
}

function bitcoinSignedIntent(signed: string) {
  const raw = new Reader(hexToBytes(signed));
  raw.take(4);
  raw.expect(0);
  raw.expect(1);
  return btcTxPrefix(raw);
}

function bitcoinAmount(satoshis: bigint) {
  return (satoshis / 100000000n).toString() + '.' + (satoshis % 100000000n).toString().padStart(8, '0');
}

export function bitcoinReplacementDraft(original: StoredChain, fee: number): ChainDraft {
  if (original.chain !== 1 || !original.attempted || !original.btc?.replaceable || original.btc.previous.length >= 8 ||
      !Number.isInteger(fee) || fee <= original.btc.fee || fee > 100000) throw new Error('BTC_REPLACEMENT_FEE');
  const tx = bitcoinSignedIntent(original.signed);
  if (tx.outputCount !== 2 || le(tx.prevouts[1].amount) - BigInt(fee - original.btc.fee) < 330n)
    throw new Error('BTC_REPLACEMENT_CHANGE');
  return {destination: segwitAddress('bcrt', tx.prevouts[0].script), amount: bitcoinAmount(le(tx.prevouts[0].amount)),
    fee, replaceable: true, replaces: original.id};
}

export function checkBitcoinReplacement(grant: ChainTransfer, original: StoredChain, draft: ChainDraft) {
  const context = grant.body.subarray(23, 113);
  if (grant.chain !== 1 || grant.account !== original.account || draft.replaces !== original.id || !draft.fee ||
      context[0] !== 1 || toHex(context.subarray(1, 33)) !== original.id ||
      Number(be(context.subarray(33, 41))) !== original.btc?.fee ||
      Number(be(grant.body.subarray(15, 23))) !== draft.fee) throw new Error('BTC_REPLACEMENT_SCOPE');
  const raw = new Reader(grant.raw);
  raw.take(5);
  const wire = new Reader(mapField(psbtMap(raw), 0).value);
  wire.take(4);
  const replacement = btcTxPrefix(wire), previous = bitcoinSignedIntent(original.signed);
  if (previous.inputCount !== replacement.inputCount || replacement.outputCount !== 2 ||
      previous.outpoints.some((outpoint, index) => !equalBytes(outpoint, replacement.outpoints[index])) ||
      previous.prevouts.some((output, index) => !equalBytes(output.script, replacement.prevouts[index].script)) ||
      !equalBytes(previous.prevouts[0].amount, replacement.prevouts[0].amount) ||
      le(previous.prevouts[1].amount) - le(replacement.prevouts[1].amount) !== BigInt(draft.fee - original.btc.fee))
    throw new Error('BTC_REPLACEMENT_CHANGES_INTENT');
}

function xrpAmount(raw: Reader): XrpAmount {
  const n = be(raw.take(8));
  if (n >> 62n === 1n) return (n & ((1n << 62n) - 1n)).toString();
  if (n >> 62n !== 3n) throw new Error('XRPL_AMOUNT');
  const decimals = Number((n >> 54n) & 255n) - 97;
  const coefficient = (n & ((1n << 54n) - 1n)).toString();
  const c = raw.take(20);
  const currency = c.slice(0, 12).every(byte => byte === 0) && c.slice(15).every(byte => byte === 0)
    && c.slice(12, 15).every(byte => byte >= 33 && byte <= 126)
    ? String.fromCharCode(...c.slice(12, 15)) : toHex(c).toUpperCase();
  return {currency, issuer: classicXrpl(raw.take(20)), value: coefficient + 'e' + decimals};
}

function parseXrpl(raw: Uint8Array, key: Uint8Array) {
  const tx = new Reader(raw);
  tx.expect(0x12);
  tx.expect(0);
  tx.expect(0);
  if (raw[tx.offset] === 0x22) {
    tx.take(1);
    const flags=be(tx.take(4));
    if (flags !== 0n && flags !== 0x80000000n) throw new Error('XRPL_FLAGS');
  }
  tx.expect(0x24);
  const sequence = Number(be(tx.take(4)));
  let destinationTag: number | undefined;
  if (raw[tx.offset] === 0x2e) {
    tx.take(1);
    destinationTag = Number(be(tx.take(4)));
  }
  tx.expect(0x20);
  tx.expect(27);
  const lastLedger = Number(be(tx.take(4)));
  let ticket:number | undefined;
  if (sequence === 0) {
    tx.expect(0x20);
    tx.expect(41);
    ticket=Number(be(tx.take(4)));
  }
  tx.expect(0x61);
  const amount = xrpAmount(tx);
  tx.expect(0x68);
  const fee=xrpAmount(tx);
  if (typeof fee !== 'string') throw new Error('XRPL_FEE');
  if (typeof amount !== 'string' && raw[tx.offset] === 0x69) {
    tx.take(1);
    if (!matchXrpDelivered(xrpAmount(tx), amount)) throw new Error('XRPL_SENDMAX');
  }
  tx.expect(0x73);
  tx.expect(33);
  if (!equalBytes(key, tx.take(33))) throw new Error('XRPL_SIGNER');
  const split = tx.offset;
  tx.expect(0x81);
  tx.expect(20);
  if (!equalBytes(tx.take(20), hash160(key))) throw new Error('XRPL_ACCOUNT');
  tx.expect(0x83);
  tx.expect(20);
  const destination = classicXrpl(tx.take(20));
  tx.done();
  return {destination, destinationTag, amount, lastLedger, split, sequence, ticket, fee};
}

function verifyXrpl(raw: Uint8Array, signed: Uint8Array, key: Uint8Array): XrpContext {
  const {destination, destinationTag, amount, lastLedger, split} = parseXrpl(raw, key);
  const proof = new Reader(signed);
  if (!equalBytes(proof.take(split), raw.subarray(0, split))) throw new Error('XRPL_SIGNED_RAW');
  proof.expect(0x74);
  const signature = proof.take(proof.byte());
  if (!equalBytes(proof.take(raw.length - split), raw.subarray(split))) throw new Error('XRPL_SIGNED_RAW');
  proof.done();
  const digest = sha512(cat(Uint8Array.of(0x53, 0x54, 0x58, 0), raw)).subarray(0, 32);
  if (!secp256k1.verify(signature, digest, key, {format: 'der', lowS: true})) throw new Error('XRPL_SIGNATURE');
  return {destination, destinationTag, amount, lastLedger};
}

export function verifyChainSigned(grant: ChainTransfer, account: ChainPublicAccount, signed: Uint8Array): ChainCheckpoint {
  if (signed.length === 1 && signed[0] === 0) throw new Error('CHAIN_USER_REFUSED');
  if (![grant.chain === account.chain, grant.network === account.network, grant.account === account.account].every(Boolean))
    throw new Error('CHAIN_SIGNER_BINDING');
  const base = {chain: grant.chain, network: grant.network, account: grant.account,
    expires: grant.expires, signed: toHex(signed), attempted: false,
    ...(grant.network===2 ? {commissionedGrant:toHex(sha256(grant.body))} : {})};
  if (grant.chain === 1) {
    const {id, fee, vsize, replaceable} = verifyBtc(signed, grant.raw);
    return {...base, id, btc: {fee, vsize, replaceable, previous: [], grantHash: toHex(sha256(grant.body))}};
  }
  if (grant.chain === 2) {
    const signature = signed.subarray(1, 65);
    const keyOffset = grant.raw[0] === 0x80 ? 5 : 4;
    if (signed[0] !== 1 || !equalBytes(signed.subarray(65), grant.raw) ||
        !equalBytes(grant.raw.subarray(keyOffset, keyOffset + 32), account.publicKey) ||
        !ed25519.verify(signature, grant.raw, account.publicKey, {zip215: false})) throw new Error('SOL_SIGNATURE');
    const height = Number(be(grant.body.subarray(63, 71)));
    if (!Number.isSafeInteger(height) || height < 1) throw new Error('SOL_HEIGHT');
    return {...base, id: base58(signature), sol: {height, blockhash: base58(grant.body.subarray(23, 55))}};
  }
  const xrp = {...verifyXrpl(grant.raw, signed, account.publicKey), firstLedger: u32(grant.body, 23)};
  return {...base, id: toHex(sha512(cat(Uint8Array.of(0x54, 0x58, 0x4e, 0), signed)).subarray(0, 32)).toUpperCase(), xrp};
}


function baseUnits(text: string, decimals: number) {
  const match = /^([0-9]{1,16})(?:\.([0-9]{1,16}))?$/.exec(text);
  if (!match || decimals < 0 || decimals > 18 || (match[2]?.length ?? 0) > decimals)
    throw new Error('CHAIN_DRAFT_AMOUNT');
  const units = BigInt(match[1] + (match[2] ?? '').padEnd(decimals, '0'));
  if (!units) throw new Error('CHAIN_DRAFT_AMOUNT');
  return units;
}

const stakeProgram='Stake11111111111111111111111111111111111111';
function isStakeDelegation(message:Uint8Array,payer:Uint8Array) {
  const reader=new Reader(message);
  try {
    const v0=message[0]===0x80;
    if (v0) reader.byte();
    const signers=reader.byte(), readonlySigned=reader.byte(), readonlyUnsigned=reader.byte(), count=reader.byte();
    if (count<2 || count>32 || signers<1 || signers>count || readonlySigned>=signers || readonlyUnsigned>count-signers)
      return false;
    const accounts=reader.take(count*32);
    if (!equalBytes(accounts.subarray(0,32),payer)) return false;
    reader.take(32);
    const instructions=reader.byte();
    if (instructions<1 || instructions>16) return false;
    let delegate=false;
    for (let index=0;index<instructions;index++) {
      const program=reader.byte(), operands=reader.byte();
      if (program>=count || operands>32 || reader.take(operands).some(value=>value>=count)) return false;
      const size=reader.byte();
      if (size>=128) return false;
      const data=reader.take(size);
      if (program>=count-readonlyUnsigned && toHex(data)==='02000000' &&
          base58(accounts.subarray(program*32,(program+1)*32))===stakeProgram) delegate=true;
    }
    if (v0) reader.expect(0);
    reader.done();
    return delegate;
  } catch {
    return false;
  }
}

function checkPreparedIntent(grant: ChainTransfer, draft: ChainDraft, owner: ChainPublicAccount,
  change: ChainPublicAccount | undefined): ChainReview {
  const raw = grant.raw;
  const common = {sender:owner.address, destination:draft.destination, amount:draft.amount,
    maximumFee:be(grant.body.subarray(15,23)).toString()};
  const field = (label:string, value:string | number) => ({label,value:String(value)});
  const checkFee = (fee:bigint) => {
    if (fee < 0n || fee > BigInt(common.maximumFee)) throw new Error('CHAIN_FEE_LIMIT');
    return fee.toString();
  };
  if (grant.chain === 1) {
    const psbt = new Reader(raw);
    if (toHex(psbt.take(5)) !== '70736274ff') throw new Error('BTC_PSBT_SHAPE');
    const wire = new Reader(mapField(psbtMap(psbt), 0).value);
    wire.take(4);
    const tx = btcTxPrefix(wire);
    wire.take(4);
    wire.done();
    const outpoint = tx.outpoints[0];
    if (tx.prevouts.length > 2 || !change ||
        segwitAddress(bitcoinHrp(grant.network), tx.prevouts[0].script) !== draft.destination ||
        le(tx.prevouts[0].amount) !== baseUnits(draft.amount, 8) ||
        (tx.prevouts[1] && segwitAddress(bitcoinHrp(grant.network), tx.prevouts[1].script) !== change.address) ||
        !tx.sequences.every(sequence => le(sequence) === (draft.replaceable ? 0xfffffffdn : 0xffffffffn)) ||
        (draft.outpoint && (tx.inputCount !== 1 ||
          draft.outpoint !== toHex(outpoint.slice(0, 32).reverse()) + ':' + le(outpoint.slice(32)))))
      throw new Error('BTC_DRAFT_MISMATCH');
    const inputs = tx.outpoints.map(item => {
      const evidence=readBtcPrevout(psbtMap(psbt),item);
      if (!equalBytes(evidence.key,owner.publicKey)) throw new Error('BTC_PREVIEW_OWNER');
      return evidence.spent;
    });
    for (let i=0; i<tx.outputCount; i++) psbtMap(psbt);
    psbt.done();
    const sum = inputs.reduce((total,item) => total + le(item.amount),0n);
    const spent = tx.prevouts.reduce((total,item) => total + le(item.amount),0n);
    const fee = checkFee(sum-spent);
    return {...common,asset:'BTC',fee,feeUnit:'sats',fields:[
      field('Input total (sats)',sum.toString()),field('Input count',tx.inputCount),
      ...(tx.prevouts[1] ? [field('Change address',change.address),field('Change (sats)',le(tx.prevouts[1].amount).toString()),
        field('Change derivation','m/84h/1h/' + grant.account + 'h/1/0')] : []),
      field('Replaceable',draft.replaceable ? 'RBF' : 'No RBF'),
    ]};
  } else if (grant.chain === 2) {
    if (isStakeDelegation(raw,owner.publicKey)) throw new Error('SOL_UNSUPPORTED_STAKE');
    const spl = !!draft.asset;
    const keys = spl ? 5 : 3;
    const ix = 4 + keys * 32 + 32;
    const token = '06ddf6e1d765a193d9cbe146ceeb79ac1cb485ed5f5b37913a8cf5857eff00a9';
    const expectedPrefix = spl ? '01000205' : '01000103';
    const expectedIx = spl ? '010404010302000a0c' : '01020200010c02000000';
    const valueStart = ix + expectedIx.length / 2;
    const decimals = spl ? grant.body[111] : 9;
    if (toHex(raw.subarray(0, 4)) !== expectedPrefix || raw.length !== valueStart + 8 + Number(spl) ||
        toHex(raw.subarray(ix, valueStart)) !== expectedIx || !equalBytes(raw.subarray(4, 36), owner.publicKey) ||
        !equalBytes(raw.subarray(ix-32,ix),grant.body.subarray(23,55)) ||
        base58(raw.subarray(spl ? 68 : 36, spl ? 100 : 68)) !== draft.destination ||
        le(raw.subarray(valueStart, valueStart + 8)) !== baseUnits(draft.amount, decimals) ||
        (spl && (base58(raw.subarray(36, 68)) !== draft.source ||
          base58(raw.subarray(100, 132)) !== draft.asset || toHex(raw.subarray(132, 164)) !== token || raw.at(-1) !== decimals)))
      throw new Error('SOL_DRAFT_MISMATCH');
    const fee=checkFee(be(grant.body.subarray(71,79)));
    return {...common,asset:draft.asset ?? 'SOL',fee,feeUnit:'lamports',fields:[
      field('Instruction',spl ? 'SPL Token · TransferChecked' : 'System Program · Transfer'),
      field('Fee payer',owner.address),field('Blockhash',base58(grant.body.subarray(23,55))),
      field('Last valid block height',be(grant.body.subarray(63,71)).toString()),
      ...(spl ? [field('Source token account',draft.source!),field('SPL decimals',decimals)] : []),
    ]};
  } else {
    const intent = parseXrpl(raw, owner.publicKey);
    const matches = typeof intent.amount === 'string' ?
      !draft.asset && intent.amount === baseUnits(draft.amount, 6).toString() :
      intent.amount.currency === draft.asset && (!draft.issuer || intent.amount.issuer === draft.issuer) &&
      canonicalDecimal(intent.amount.value) === canonicalDecimal(draft.amount);
    if (!matches || intent.destination !== draft.destination || intent.destinationTag !== draft.tag)
      throw new Error('XRPL_DRAFT_MISMATCH');
    return {...common,asset:typeof intent.amount === 'string' ? 'XRP' : intent.amount.currency,
      fee:checkFee(BigInt(intent.fee)), feeUnit:'drops', fields:[
        field('Transaction type','Payment'),
        ...(typeof intent.amount !== 'string' ? [field('Issuer',intent.amount.issuer)] : []),
        ...(intent.destinationTag !== undefined ? [field('DestinationTag',intent.destinationTag)] : []),
        intent.ticket ? field('TicketSequence',intent.ticket) : field('Sequence',intent.sequence),
        field('LastLedgerSequence',intent.lastLedger),
      ]};
  }
}

export type ChainRpc = (method: string, params: unknown) => Promise<unknown>;
export type ChainResult = 'pending' | 'included' | 'finalized' | 'failed' | 'expired';
type RpcObject = Record<string, unknown>;
function record(value: unknown): RpcObject {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('CHAIN_RPC_SCHEMA');
  return value as RpcObject;
}

export function loopbackChainRpc(endpoint: string, exchange: (endpoint: string, packet: string) => Promise<string>): ChainRpc {
  if (!/^https:\/\/(?:localhost|127\.0\.0\.1|\[::1\])(?::[0-9]{1,5})?(?:\/[A-Za-z0-9/_-]*)?$/.test(endpoint))
    throw new Error('CHAIN_LOOPBACK_ENDPOINT');
  return chainRpc(endpoint,exchange);
}

export function storedChainRpc(profile:Pick<StoredChain,'chain'|'network'|'anchor'|'endpoint'>, exchange:(endpoint:string,packet:string)=>Promise<string>, token?:string) {
  if (profile.network===1) return loopbackChainRpc(profile.endpoint,exchange);
  if (profile.network!==2 || !token || !/^[a-f0-9]{64}$/.test(token)) throw new Error('CHAIN_COMMISSIONED_SCOPE');
  readCommissionedChains([{chain:profile.chain, profile:commissionedProfile(profile.chain),anchor:profile.anchor,rpc:profile.endpoint}]);
  const credential=toHex(hmac(sha256,hexToBytes(token),utf8Encode('N54P\n'+profile.endpoint)));
  return chainRpc(profile.endpoint,exchange,credential);
}

function chainRpc(endpoint:string,exchange:(endpoint:string,packet:string)=>Promise<string>,token?:string):ChainRpc {
  return async (method, params) => {
    const packet = JSON.stringify({jsonrpc: '2.0', id: 1, method, params,...(token ? {walletToken:token} : {})});
    const raw = record(JSON.parse(await exchange(endpoint, packet)));
    if (raw.id !== 1) throw new Error('CHAIN_RPC_REPLY');
    if (raw.error) {
      const error = record(raw.error);
      if (method === 'getrawtransaction' && error.code === -5 &&
          (raw.result === null || (raw.jsonrpc === '2.0' && raw.result === undefined))) return null;
      throw new Error('CHAIN_RPC_ERROR');
    }
    if (raw.result === undefined) throw new Error('CHAIN_RPC_REPLY');
    return raw.result;
  };
}

export async function checkChainNetwork(rpc: ChainRpc, profile: ChainAccount, anchor: string) {
  if (profile.network === 2) return verifyCommissionedAnchor(profile.chain,anchor,rpc);
  if (profile.network !== 1) throw new Error('CHAIN_LAB_PROFILE_ONLY');
  switch (profile.chain) {
    case 1:
      if (!/^[a-f0-9]{64}$/.test(anchor) || await rpc('getblockhash', [0]) !== anchor ||
          record(await rpc('getblockchaininfo', [])).chain !== 'regtest') throw new Error('BTC_REGTEST_NETWORK');
      break;
    case 2:
      if (!/^[1-9A-HJ-NP-Za-km-z]{32,44}$/.test(anchor) || await rpc('getGenesisHash', []) !== anchor)
        throw new Error('SOL_GENESIS');
      break;
    case 3: {
      if (!/^(0|[1-9][0-9]{0,9})$/.test(anchor) || BigInt(anchor) > 0xffffffffn ||
          record(record(await rpc('server_info', [{}])).info).network_id !== Number(anchor))
        throw new Error('XRPL_NETWORK');
      break;
    }
  }
}

function canonicalDecimal(value: string) {
  const parts = /^([+-]?)([0-9]{1,120})(?:\.([0-9]{1,120}))?(?:[eE]([+-]?[0-9]{1,3}))?$/.exec(value);
  if (!parts) throw new Error('XRPL_DELIVERED_AMOUNT');
  const fraction = parts[3] ?? '';
  const coefficient = BigInt(parts[1] + parts[2] + fraction);
  const digits = coefficient.toString().replace(/0+$/, '');
  return digits + 'e' + (Number(parts[4] ?? '0') - fraction.length + coefficient.toString().length - digits.length);
}

function matchXrpDelivered(actual: unknown, target: XrpAmount) {
  if (typeof target === 'string') return actual === target;
  const amount = record(actual);
  return amount.currency === target.currency && amount.issuer === target.issuer && typeof amount.value === 'string' &&
    canonicalDecimal(amount.value) === canonicalDecimal(target.value);
}

function coversValidatedHistory(ranges: unknown, first: number, last: number) {
  if (typeof ranges !== 'string' || ranges.length > 4000 ||
      !/^[0-9]+(?:-[0-9]+)?(?:,[0-9]+(?:-[0-9]+)?)*$/.test(ranges)) return false;
  return ranges.split(',').some(row => {
    const [start, end = start] = row.split('-').map(Number);
    return Number.isSafeInteger(start) && Number.isSafeInteger(end) && start <= first && end >= last;
  });
}

async function xrplValidatedLedger(rpc: ChainRpc) {
  const info = record(record(await rpc('server_info', [{}])).info);
  const ledger = record(info.validated_ledger);
  const sequence = ledger.seq;
  if (typeof sequence !== 'number' || !Number.isSafeInteger(sequence) || sequence < 1)
    throw new Error('XRPL_VALIDATED_LEDGER_INDEX');
  return {sequence, history: info.complete_ledgers};
}

export async function queryChainTransfer(rpc: ChainRpc, pending: ChainCheckpoint): Promise<ChainResult> {
  switch (pending.chain) {
    case 1: {
      const state = await rpc('getrawtransaction', [pending.id, true]);
      if (state === null) return 'pending';
      const tx = record(state);
      if (tx.txid !== pending.id) throw new Error('BTC_TXID_MISMATCH');
      if (typeof tx.confirmations !== 'number' || tx.confirmations <= 0) return 'pending';
      return tx.confirmations >= 6 ? 'finalized' : 'included';
    }
    case 2: {
      const statuses = record(await rpc('getSignatureStatuses', [[pending.id], {searchTransactionHistory: true}])).value;
      if (!Array.isArray(statuses) || statuses.length !== 1) throw new Error('SOL_STATUS');
      if (statuses[0] === null) return (await solHeight(rpc)) > pending.sol!.height ? 'expired' : 'pending';
      const state = record(statuses[0]);
      if (state.confirmationStatus === 'finalized') return state.err === null ? 'finalized' : 'failed';
      return state.confirmationStatus === 'confirmed' ? 'included' : 'pending';
    }
    case 3: {
      const result = record(await rpc('tx', [{transaction: pending.id, binary: false, api_version: 1}]));
      if (result.error === 'txnNotFound') {
        const xrp = pending.xrp!;
        if (!xrp.firstLedger) return 'pending';
        const {sequence, history} = await xrplValidatedLedger(rpc);
        return sequence > xrp.lastLedger && coversValidatedHistory(history, xrp.firstLedger, xrp.lastLedger) ?
          'expired' : 'pending';
      }
      if (result.hash !== pending.id) throw new Error('XRPL_HASH_MISMATCH');
      if (result.validated !== true) return 'pending';
      const meta = record(result.meta);
      if (meta.TransactionResult !== 'tesSUCCESS') return 'failed';
      const xrp = pending.xrp!;
      if (result.Destination !== xrp.destination || result.DestinationTag !== xrp.destinationTag ||
          !matchXrpDelivered(meta.delivered_amount, xrp.amount)) throw new Error('XRPL_DELIVERED_AMOUNT');
      return 'finalized';
    }
  }
}

async function solHeight(rpc: ChainRpc) {
  const value = await rpc('getBlockHeight', [{commitment: 'confirmed'}]);
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 1) throw new Error('SOL_HEIGHT');
  return value;
}

type SubmissionClock = {key: string; board: string; revision: string};
export type StoredChain = ChainCheckpoint & {endpoint: string; anchor: string; ownerAddress: string;
  ownerGeneration: number; clock?: SubmissionClock; review?: ChainReview};

type SubmitAuthorization = {
  challenge: () => Promise<Uint8Array>;
  monotonic: () => number;
  saveAttempt: (next: StoredChain) => Promise<void>;
};

async function requireSubmissionTime(rpc: ChainRpc, pending: StoredChain, auth: SubmitAuthorization) {
  if (!pending.clock) throw new Error('CHAIN_TRUSTED_TIME_REQUIRED');
  const {key, board, revision} = pending.clock;
  const nonce = await auth.challenge();
  const start = auth.monotonic();
  if (nonce.length !== 32 || !Number.isFinite(start) || start < 0) throw new Error('CHAIN_TRUSTED_TIME_REQUIRED');
  const signed = record(await rpc('nu54_walletTime', [{nonce: toHex(nonce), board, generation: String(pending.ownerGeneration),
    leaseRevision: revision}]));
  const end = auth.monotonic();
  if (typeof signed.proof !== 'string' || !/^[a-f0-9]{128}$/.test(signed.proof) ||
      typeof signed.time !== 'number' || !Number.isSafeInteger(signed.time) || signed.time < 1 ||
      !Number.isFinite(end) || end < start || end - start > 8000)
    throw new Error('CHAIN_TRUSTED_TIME_REQUIRED');
  const statement = cat(Uint8Array.of(0x4e, 0x35, 0x34, 0x43), hexToBytes(board),
    bigToBytes(BigInt(pending.ownerGeneration), 8), bigToBytes(BigInt(revision), 8), nonce, bigToBytes(BigInt(signed.time), 8));
  if (!p256.verify(hexToBytes(signed.proof), sha256(statement), hexToBytes(key), {lowS: false}) ||
      signed.time < pending.expires - 60) throw new Error('CHAIN_TRUSTED_TIME_REQUIRED');
  // Round upward for timestamp precision and time spent in the request.
  if (signed.time + Math.ceil((end - start) / 1000) + 1 >= pending.expires) throw new Error('CHAIN_SUBMISSION_EXPIRED');
  return signed.time;
}

export async function ensureBitcoinPredecessor(rpc: ChainRpc, id: string) {
  if (!/^[a-f0-9]{64}$/.test(id)) throw new Error('BTC_REPLACEMENT_ORIGINAL');
  const observed = record(await rpc('nu54_bitcoinReplacementStatus', [id]));
  if (observed.id !== id) throw new Error('BTC_REPLACEMENT_ORIGINAL');
  if (observed.state === 'confirmed') throw new Error('BTC_ORIGINAL_CONFIRMED');
  if (observed.state !== 'live') throw new Error('BTC_ORIGINAL_NOT_IN_MEMPOOL');
}

export async function submitChainTransfer(rpc: ChainRpc, pending: StoredChain, auth: SubmitAuthorization): Promise<ChainResult> {
  if (pending.attempted) throw new Error('CHAIN_SUBMISSION_ATTEMPTED');
  const earliest = await requireSubmissionTime(rpc, pending, auth);
  await checkChainNetwork(rpc, pending, pending.anchor);
  const existing = await queryChainTransfer(rpc, pending);
  if (existing !== 'pending') {
    if (existing !== 'expired') await auth.saveAttempt({...pending, attempted: true});
    return existing;
  }
  if (pending.chain === 1 && pending.btc?.previous.length) {
    if (!pending.btc.grantHash) throw new Error('BTC_REPLACEMENT_GRANT_REQUIRED');
    await ensureBitcoinPredecessor(rpc, pending.btc.previous.at(-1)!);
    const details = await rpc('testmempoolaccept', [[pending.signed]]);
    if (!Array.isArray(details) || details.length !== 1 || record(details[0]).allowed !== true ||
        record(details[0]).txid !== pending.id) throw new Error('BTC_REPLACEMENT_NOT_ACCEPTED');
  } else if (pending.chain === 2) {
    const validity = record(await rpc('isBlockhashValid', [pending.sol!.blockhash, {commitment: 'confirmed'}]));
    if (validity.value !== true || await solHeight(rpc) > pending.sol!.height) throw new Error('SOL_BLOCKHASH_EXPIRED');
  } else if (pending.chain === 3) {
    const head = record(await rpc('ledger_current', [{}])).ledger_current_index;
    if (typeof head !== 'number' || !Number.isInteger(head) || head > pending.xrp!.lastLedger)
      throw new Error('XRPL_LAST_LEDGER');
  }
  const last = await requireSubmissionTime(rpc, pending, auth);
  if (last < earliest) throw new Error('CHAIN_TRUSTED_TIME_REQUIRED');
  // Persist before the network call. Failure afterward can only resume by ID.
  await auth.saveAttempt({...pending, attempted: true});
  if (await requireSubmissionTime(rpc, pending, auth) < last) throw new Error('CHAIN_TRUSTED_TIME_REQUIRED');
  if (pending.network===2) {
    if (!pending.commissionedGrant || await rpc('nu54_submitCommissionedTransfer', [{
      grant:pending.commissionedGrant,id:pending.id,signed:pending.signed,
    }])!==pending.id) throw new Error('CHAIN_COMMISSIONED_SUBMITTED_ID');
    return 'pending';
  }
  switch (pending.chain) {
    case 1:
      if (pending.btc?.previous.length) {
        const original = pending.btc.previous.at(-1)!;
        await ensureBitcoinPredecessor(rpc, original);
        if (await rpc('nu54_submitBitcoinReplacement', [{original, id: pending.id, signed: pending.signed,
          grant: pending.btc.grantHash, anchor: pending.anchor}]) !== pending.id) throw new Error('BTC_SUBMITTED_ID');
      } else if (await rpc('sendrawtransaction', [pending.signed]) !== pending.id) throw new Error('BTC_SUBMITTED_ID');
      break;
    case 2:
      if (await rpc('sendTransaction', [toBase64(hexToBytes(pending.signed)), {encoding: 'base64', skipPreflight: false, preflightCommitment: 'confirmed'}])
          !== pending.id) throw new Error('SOL_SUBMITTED_ID');
      break;
    case 3: {
      const submitted = record(await rpc('submit', [{tx_blob: pending.signed.toUpperCase(), fail_hard: true}]));
      if (record(submitted.tx_json).hash !== pending.id) throw new Error('XRPL_SUBMITTED_HASH');
      break;
    }
  }
  return 'pending';
}

function validSavedReview(input: unknown) {
  const summary=record(input);
  const text=(value:unknown,max:number) => typeof value==='string' && value.length>0 && value.length<=max;
  return text(summary.sender,96) && text(summary.destination,96) && text(summary.amount,34) && text(summary.asset,64) &&
    text(summary.fee,24) && text(summary.maximumFee,24) && ['sats','lamports','drops'].includes(String(summary.feeUnit)) &&
    Array.isArray(summary.fields) && summary.fields.length<=12 && summary.fields.every(raw=>{
      const field=record(raw);
      return text(field.label,64) && text(field.value,96) && Object.keys(field).length===2;
    });
}

export function readStoredChain(input: unknown): input is StoredChain {
  try {
    const value = record(input) as StoredChain;
    chainAccountCommand(10, value);
    if ((value.review !== undefined && !validSavedReview(value.review)) || (value.network !== 1 && value.network !== 2) || typeof value.attempted !== 'boolean' ||
        !Number.isSafeInteger(value.expires) || value.expires <= 0 ||
        !Number.isSafeInteger(value.ownerGeneration) || value.ownerGeneration <= 0 ||
        typeof value.ownerAddress !== 'string' || !/^0x[a-f0-9]{40}$/.test(value.ownerAddress) ||
        typeof value.signed !== 'string' || !/^(?:[a-f0-9]{2}){1,1232}$/.test(value.signed) ||
        typeof value.endpoint !== 'string' || typeof value.anchor !== 'string' || value.anchor.length > (value.network===2 ? 129 : 64) ||
        typeof value.id !== 'string' ||
        Object.keys(value).some(key => !['attempted', 'expires', 'account', 'chain', 'network', 'signed',
          'id', 'btc', 'sol', 'xrp', 'endpoint', 'anchor', 'ownerAddress', 'ownerGeneration', 'clock', 'review', 'commissionedGrant'].includes(key))) return false;
    if (value.clock && (!/^04[a-f0-9]{128}$/.test(value.clock.key) || !/^[a-f0-9]{64}$/.test(value.clock.board) ||
        !/^[1-9][0-9]{0,19}$/.test(value.clock.revision) || BigInt(value.clock.revision) >= (1n << 64n))) return false;
    if (value.network===2) readCommissionedChains([{chain:value.chain,profile:commissionedProfile(value.chain),anchor:value.anchor,rpc:value.endpoint}]);
    else loopbackChainRpc(value.endpoint,async () => '');
    if (value.network===2 && (!value.clock || !/^[a-f0-9]{64}$/.test(value.commissionedGrant ?? '') || value.btc?.replaceable || value.btc?.previous.length)) return false;
    if (value.chain === 1) return /^[a-f0-9]{64}$/.test(value.id) && !value.sol && !value.xrp &&
      (!value.btc || (Number.isInteger(value.btc.fee) && value.btc.fee >= 0 && value.btc.fee <= 1000000 &&
        typeof value.btc.replaceable === 'boolean' &&
        (value.btc.vsize === undefined || (Number.isSafeInteger(value.btc.vsize) && value.btc.vsize > 0 && value.btc.vsize <= 1232)) &&
        (value.btc.grantHash === undefined || /^[a-f0-9]{64}$/.test(value.btc.grantHash)) && Array.isArray(value.btc.previous) && value.btc.previous.length <= 8 &&
        value.btc.previous.every(id => /^[a-f0-9]{64}$/.test(id) && id !== value.id) &&
        new Set(value.btc.previous).size === value.btc.previous.length));
    if (value.btc) return false;
    if (value.chain === 2) return /^[1-9A-HJ-NP-Za-km-z]{64,88}$/.test(value.id) && !value.xrp && !!value.sol &&
      /^[1-9A-HJ-NP-Za-km-z]{32,44}$/.test(value.sol.blockhash) &&
      Number.isSafeInteger(value.sol.height) && value.sol.height >= 1;
    const xrp = value.xrp;
    return /^[A-F0-9]{64}$/.test(value.id) && !value.sol && !!xrp &&
      Number.isSafeInteger(xrp.lastLedger) && xrp.lastLedger > 0 &&
      (xrp.firstLedger === undefined || (Number.isSafeInteger(xrp.firstLedger) &&
        xrp.firstLedger > 0 && xrp.firstLedger < xrp.lastLedger)) &&
      /^r[1-9A-HJ-NP-Za-km-z]{24,34}$/.test(xrp.destination) &&
      (xrp.destinationTag === undefined || (Number.isInteger(xrp.destinationTag) &&
        xrp.destinationTag >= 0 && xrp.destinationTag <= 0xffffffff)) &&
      (typeof xrp.amount === 'string' ? /^[0-9]{1,18}$/.test(xrp.amount) :
        !!xrp.amount && typeof xrp.amount.currency === 'string' && typeof xrp.amount.issuer === 'string' &&
        canonicalDecimal(xrp.amount.value).length > 0);
  } catch {
    return false;
  }
}

export async function readChainReceiveBalance(rpc: ChainRpc, account: ChainPublicAccount, anchor: string,
  bitcoinNetwork: 'regtest' | 'signet' = 'regtest') {
  if (account.chain === 1 && bitcoinNetwork === 'signet') {
    if (!/^[a-f0-9]{64}$/.test(anchor) || await rpc('getblockhash', [0]) !== anchor ||
        record(await rpc('getblockchaininfo', [])).chain !== bitcoinNetwork ||
        await rpc('getblockhash', [bitcoinSignetCheckpoint.height]) !== bitcoinSignetCheckpoint.hash)
      throw new Error('BTC_SIGNET_NETWORK');
  } else if (account.chain !== 3) await checkChainNetwork(rpc, account, anchor);
  if (account.chain === 1) {
    const details = record(await rpc('getaddressinfo', [account.address]));
    if (details.address !== account.address || (details.iswatchonly !== true && details.ismine !== true))
      throw new Error('BTC_WATCH_ADDRESS');
    const confirmations = 6;
    const rows = await rpc('listunspent', [confirmations, 9999999, [account.address]]);
    if (!Array.isArray(rows) || rows.length > (bitcoinNetwork === 'signet' ? 64 : 2000)) throw new Error('BTC_BALANCE_SCHEMA');
    const chain = record(await rpc('getblockchaininfo', []));
    const head = chain.blocks;
    if (typeof head !== 'number' || !Number.isSafeInteger(head) || head < 0) throw new Error('BTC_BALANCE_HEAD');
    const seen = new Set<string>();
    let amount = 0n;
    for (const row of rows) {
      const coin = record(row);
      if (coin.address !== account.address || typeof coin.txid !== 'string' || !/^[a-f0-9]{64}$/.test(coin.txid) ||
          typeof coin.vout !== 'number' || !Number.isInteger(coin.vout) || coin.vout < 0 ||
          typeof coin.confirmations !== 'number' || coin.confirmations < confirmations)
        throw new Error('BTC_BALANCE_COIN');
      const coinID = coin.txid + ':' + coin.vout;
      if (seen.has(coinID)) throw new Error('BTC_BALANCE_DUPLICATE');
      seen.add(coinID);
      const units = baseUnits(String(coin.amount), 8);
      if (bitcoinNetwork === 'signet') {
        const utxo = record(await rpc('gettxout', [coin.txid, coin.vout, false]));
        const script = record(utxo.scriptPubKey);
        if (typeof chain.bestblockhash !== 'string' || !/^[a-f0-9]{64}$/.test(chain.bestblockhash) ||
            utxo.bestblock !== chain.bestblockhash || typeof utxo.confirmations !== 'number' ||
            utxo.confirmations < confirmations || script.address !== account.address ||
            baseUnits(String(utxo.value), 8) !== units) throw new Error('BTC_BALANCE_EVIDENCE');
      }
      amount += units;
    }
    if (bitcoinNetwork === 'signet' &&
        (typeof chain.bestblockhash !== 'string' || !/^[a-f0-9]{64}$/.test(chain.bestblockhash) ||
          await rpc('getblockhash', [head]) !== chain.bestblockhash)) throw new Error('BTC_BALANCE_EVIDENCE');
    return {amount: amount.toString(), unit: 'sats', observation: 'Bitcoin tip ' + head};
  }
  if (account.chain === 2) {
    const reply = record(await rpc('getBalance', [account.address, {commitment: 'finalized'}]));
    const slot = record(reply.context).slot;
    if (typeof slot !== 'number' || !Number.isSafeInteger(slot) || slot < 0 ||
        typeof reply.value !== 'number' || !Number.isSafeInteger(reply.value) || reply.value < 0)
      throw new Error('SOL_BALANCE_SCHEMA');
    return {amount: String(reply.value), unit: 'lamports', observation: 'Solana finalized slot ' + slot};
  }
  return readXrplReceiveBalance(rpc, account.address, anchor);
}

export async function readXrplReceiveBalance(rpc: ChainRpc, address: string, anchor: string) {
  const info = record(record(await rpc('server_info', [{}])).info);
  if (!/^(0|[1-9][0-9]{0,9})$/.test(anchor) || BigInt(anchor) > 0xffffffffn ||
      info.network_id !== Number(anchor)) throw new Error('XRPL_NETWORK');
  const point = record(info.validated_ledger);
  const hash = point.hash, height = point.seq;
  if (typeof hash !== 'string' || !/^[0-9A-Fa-f]{64}$/.test(hash) ||
      typeof height !== 'number' || !Number.isSafeInteger(height) || height < 1)
    throw new Error('XRPL_BALANCE_LEDGER');
  const response = record(await rpc('account_info', [{account: address, ledger_hash: hash}]));
  if (response.error && response.error !== 'actNotFound') throw new Error('XRPL_BALANCE_RPC');
  const ledger = record(await rpc('ledger', [{ledger_hash: hash, transactions: false}]));
  if (response.validated !== true || response.ledger_hash !== hash || response.ledger_index !== height ||
      ledger.validated !== true || ledger.ledger_hash !== hash ||
      (ledger.ledger_index !== height && ledger.ledger_index !== String(height)))
    throw new Error('XRPL_BALANCE_LEDGER');
  const context = {unit: 'drops', observation: 'XRPL validated ledger ' + height, ledgerHash: hash, ledgerIndex: height};
  if (response.error === 'actNotFound') {
    if (response.account_data !== undefined || (response.account !== undefined && response.account !== address))
      throw new Error('XRPL_BALANCE_SCHEMA');
    return {...context, status: 'inactive' as const};
  }
  const entry = record(response.account_data);
  if (entry.Account !== address || typeof entry.Balance !== 'string' || !/^\d{1,18}$/.test(entry.Balance))
    throw new Error('XRPL_BALANCE_SCHEMA');
  return {...context, status: 'active' as const, amount: entry.Balance};
}

export async function verifyChainGrantWindow(rpc: ChainRpc, grant: ChainTransfer) {
  if (grant.chain === 2) {
    const blockhash = base58(grant.body.subarray(23, 55));
    const lastHeight = Number(be(grant.body.subarray(63, 71)));
    const candidate = record(await rpc('isBlockhashValid', [blockhash, {commitment: 'confirmed'}]));
    if (!Number.isSafeInteger(lastHeight) || candidate.value !== true || (await solHeight(rpc)) >= lastHeight)
      throw new Error('SOL_BLOCKHASH_EXPIRED');
  } else if (grant.chain === 3) {
    const tx = new Reader(grant.raw);
    tx.expect(0x12);
    tx.expect(0);
    tx.expect(0);
    if (tx.data[tx.offset] === 0x22) tx.take(5);
    tx.expect(0x24);
    tx.take(4);
    if (tx.data[tx.offset] === 0x2e) tx.take(5);
    tx.expect(0x20);
    tx.expect(27);
    const lastLedger = Number(be(tx.take(4)));
    const head = record(await rpc('ledger_current', [{}])).ledger_current_index;
    const {sequence} = await xrplValidatedLedger(rpc);
    if (!Number.isSafeInteger(head) || Number(head) > lastLedger || sequence >= lastLedger)
      throw new Error('XRPL_LAST_LEDGER');
  }
}
