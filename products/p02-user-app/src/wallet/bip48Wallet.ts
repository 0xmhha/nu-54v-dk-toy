import {secp256k1} from '@noble/curves/secp256k1';
import {hmac} from '@noble/hashes/hmac';
import {sha512} from '@noble/hashes/sha512';
import {sha256} from '@noble/hashes/sha256';
import {bigToBytes, bytesToHex, concat, equalBytes, fromBase64, hexToBytes, toBase64} from '@nu54/protocol';
import {bitcoinHrp, doubleSha256, segwitAddress} from './chainAccounts.ts';
import {previousBitcoinOutput, readChainTransfer, verifyChainAuthority, type ChainRpc} from './chainTransfer.ts';

const hex = (bytes: Uint8Array) => bytesToHex(bytes, false);
const little = (bytes: Uint8Array) => bytes.reduceRight((n, byte) => 256n * n + BigInt(byte), 0n);
const serializeLE = (n: bigint, size: number) => bigToBytes(n, size).reverse();

export type Bip48Scope = {board: string; generation: string; leaseRevision: string; anchor: string; network?:1|2; account?:number};
export type Bip48Policy = {bytes: Uint8Array; hash: Uint8Array; descriptors: string[]; account: number; owner: number};

function object(value: unknown) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('BIP48_RPC_SHAPE');
  return value as Record<string, unknown>;
}

const alphabet = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';
function decodeBase58(value: string) {
  let n = 0n;
  for (const ch of value) {
    const digit = alphabet.indexOf(ch);
    if (digit < 0) throw new Error('BIP48_BASE58');
    n = n * 58n + BigInt(digit);
  }
  return bigToBytes(n, 82);
}

export type StoredBip48 = {network:1|2;account:number;board:string;generation:number;policy:string;original:string;psbt:string;id:string;count:1|2};

export function readStoredBip48(input:unknown):input is StoredBip48 {
  if (!input || typeof input!=='object') return false;
  const row=input as StoredBip48;
  if (Object.keys(row).sort().join(',')!=='account,board,count,generation,id,network,original,policy,psbt' ||
      ![1,2].includes(row.network) || ![1,2].includes(row.count) ||
      !Number.isSafeInteger(row.account) || row.account<0 || row.account>99 ||
      !Number.isSafeInteger(row.generation) || row.generation<1 ||
      ![row.board,row.policy,row.id].every(x=>typeof x==='string' && /^[a-f0-9]{64}$/.test(x)) ||
      ![row.original,row.psbt].every(x=>typeof x==='string' && x.length<=2664 && /^[A-Za-z0-9+/]+={0,2}$/.test(x))) return false;
  try {return bip48TransactionId(fromBase64(row.original))===row.id &&
    bip48TransactionId(fromBase64(row.psbt))===row.id;} catch {return false;}
}

export async function loadBip48Policy(rpc: ChainRpc, scope: Bip48Scope): Promise<Bip48Policy> {
  const reply = object(await rpc('nu54_bip48Policy', [scope]));
  if (typeof reply.policy !== 'string' || !/^[a-f0-9]{448}$/.test(reply.policy) ||
      !Array.isArray(reply.descriptors) || reply.descriptors.length !== 2) throw new Error('BIP48_REGISTRY');
  const bytes = hexToBytes(reply.policy);
  const account = bytes[6], owner = bytes[15];
  if (hex(bytes.subarray(0, 6)) !== (scope.network===2 ? '110102000000' : '110101000000') ||
      account > 99 || (scope.account!==undefined && scope.account!==account) || owner > 2 || bytes[16] !== 2)
    throw new Error('BIP48_REGISTRY');
  for (const [branch, descriptor] of reply.descriptors.entries()) {
    if (typeof descriptor !== 'string' || descriptor.length > 700) throw new Error('BIP48_DESCRIPTOR');
    const format = /^wsh\(sortedmulti\(2,(.*)\)\)(?:#[a-z0-9]{8})?$/.exec(descriptor);
    const roots = format?.[1].split(',');
    if (roots?.length !== 3) throw new Error('BIP48_DESCRIPTOR');
    for (const [slot, item] of roots.entries()) {
      const pattern = new RegExp('^\\[([a-f0-9]{8})/48h/1h/' + account + 'h/2h\\](tpub[1-9A-HJ-NP-Za-km-z]+)/' + branch + '/\\*$');
      const match = pattern.exec(item);
      if (!match) throw new Error('BIP48_DESCRIPTOR');
      const key = decodeBase58(match[2]);
      if (hex(key.subarray(0, 5)) !== '043587cf04' || hex(key.subarray(9, 13)) !== '80000002' ||
          !equalBytes(doubleSha256(key.subarray(0, 78)).subarray(0, 4), key.subarray(78)) ||
          !equalBytes(concat(hexToBytes(match[1]), key.subarray(13, 78)), bytes.subarray(17 + slot * 69, 86 + slot * 69)))
        throw new Error('BIP48_DESCRIPTOR');
    }
  }
  return {bytes, account, owner, hash: sha256(bytes), descriptors: reply.descriptors as string[]};
}

function psbtMaps(raw: Uint8Array) {
  if (raw.length > 1998 || hex(raw.subarray(0, 5)) !== '70736274ff') throw new Error('BIP48_PSBT_FORMAT');
  let offset = 5;
  const read = (length: number) => {
    const end = offset + length;
    if (end > raw.length) throw new Error('BIP48_PSBT_LENGTH');
    const value = raw.subarray(offset, end);
    offset = end;
    return value;
  };
  const vector = () => {
    const first = read(1)[0];
    if (first < 253) return read(first);
    if (first !== 253) throw new Error('BIP48_PSBT_LENGTH');
    const length = Number(little(read(2)));
    if (length < 253) throw new Error('BIP48_PSBT_LENGTH');
    return read(length);
  };
  const maps: Map<string, Uint8Array>[] = [];
  while (offset < raw.length) {
    if (maps.length >= 7) throw new Error('BIP48_PSBT_COUNT');
    const fields = new Map<string, Uint8Array>();
    for (let type = vector(); type.length; type = vector()) {
      const key = hex(type);
      if (fields.has(key) || fields.size >= 15) throw new Error('BIP48_PSBT_FIELD');
      fields.set(key, vector());
    }
    maps.push(fields);
  }
  if (maps.length < 3 || hex(required(maps[0],'fb',4))!=='02000000') throw new Error('BIP48_PSBT_HEADER');
  const inputCount=required(maps[0],'04',1)[0], outputCount=required(maps[0],'05',1)[0];
  if (inputCount<1 || inputCount>2 || outputCount<1 || outputCount>4 || maps.length!==1+inputCount+outputCount)
    throw new Error('BIP48_PSBT_HEADER');
  return maps;
}

const splitMaps=(maps:ReturnType<typeof psbtMaps>)=>{
  const firstOutput=1+required(maps[0],'04',1)[0];
  return {global:maps[0],inputs:maps.slice(1,firstOutput),outputs:maps.slice(firstOutput)};
};


export function readBip48Import(source:string) {
  const text=source.trim();
  if (text.length>2376 || !/^[A-Za-z0-9+/]+={0,2}$/.test(text)) throw new Error('BIP48_PSBT_FORMAT');
  const raw=fromBase64(text);
  if (raw.length>1780 || toBase64(raw)!==text) throw new Error('BIP48_PSBT_FORMAT');
  psbtMaps(raw);
  return text;
}

function required(fields: Map<string, Uint8Array>, name: string, length?: number) {
  const value = fields.get(name);
  if (!value || (length !== undefined && value.length !== length)) throw new Error('BIP48_PSBT_FIELD');
  return value;
}

function verifiedProgram(fields:Map<string,Uint8Array>, type:string, witness:Uint8Array, program:Uint8Array,
  policy:Bip48Policy, change=false) {
  const paths=[...fields].filter(([key])=>key.startsWith(type));
  if (paths.length!==3 || paths.some(([key,path])=>key.length!==68 || path.length!==28))
    throw new Error('BIP48_PATH');
  const branch=Number(little(paths[0][1].subarray(20,24))), addressIndex=Number(little(paths[0][1].subarray(24)));
  if (branch>1 || addressIndex>9999 || (change && branch!==1)) throw new Error('BIP48_PATH');
  const suffix=concat(...[0x80000030,0x80000001,0x80000000+policy.account,0x80000002,branch,addressIndex]
    .map(value=>serializeLE(BigInt(value),4)));
  const children:string[]=[];
  for (let slot=0;slot<3;slot++) {
    const root=policy.bytes.subarray(17+slot*69,86+slot*69);
    const match=paths.find(([,path])=>equalBytes(path,concat(root.subarray(0,4),suffix)));
    if (!match) throw new Error('BIP48_PATH');
    let publicKey:Uint8Array=root.slice(36),chainCode:Uint8Array=root.slice(4,36);
    for (const step of [branch,addressIndex]) {
      const digest=hmac(sha512,chainCode,concat(publicKey,bigToBytes(BigInt(step),4)));
      const scalar=BigInt('0x'+hex(digest.subarray(0,32)));
      if (scalar===0n || scalar>=secp256k1.CURVE.n) throw new Error('BIP48_PATH');
      const point=secp256k1.ProjectivePoint.fromHex(publicKey).add(secp256k1.ProjectivePoint.BASE.multiply(scalar));
      if (point.equals(secp256k1.ProjectivePoint.ZERO)) throw new Error('BIP48_PATH');
      publicKey=point.toRawBytes(true);
      chainCode=digest.slice(32);
    }
    const key=hex(publicKey);
    if (match[0]!==type+key || children.includes(key)) throw new Error('BIP48_PATH');
    children.push(key);
  }
  const script=concat(Uint8Array.of(0x52),...[...children].sort().map(key=>concat(Uint8Array.of(33),hexToBytes(key))),
    Uint8Array.of(0x53,0xae));
  if (!equalBytes(script,witness) || !equalBytes(concat(Uint8Array.of(0,32),sha256(script)),program))
    throw new Error('BIP48_CHANGE');
  return {branch,addressIndex,children};
}

const inputPoint=(fields:Map<string,Uint8Array>)=>concat(required(fields,'0e',32),required(fields,'0f',4));
const sequence=(fields:Map<string,Uint8Array>)=>required(fields,'10',4);
const outputBytes=(fields:Map<string,Uint8Array>)=>{
  const script=required(fields,'04');
  return concat(required(fields,'03',8),Uint8Array.of(script.length),script);
};

function bip48UnsignedTransaction(maps:ReturnType<typeof psbtMaps>) {
  const {global,inputs,outputs}=splitMaps(maps);
  return concat(required(global,'02',4),Uint8Array.of(inputs.length),
    ...inputs.map(input=>concat(inputPoint(input),Uint8Array.of(0),sequence(input))),Uint8Array.of(outputs.length),
    ...outputs.map(outputBytes),global.get('03') ?? new Uint8Array(4));
}

function bip48TransactionId(raw:Uint8Array) {
  return hex(doubleSha256(bip48UnsignedTransaction(psbtMaps(raw))).reverse());
}

function inputsForReview(maps:ReturnType<typeof psbtMaps>,policy:Bip48Policy) {
  const {global,inputs,outputs}=splitMaps(maps);
  const points=inputs.map(inputPoint);
  if (new Set(points.map(hex)).size!==inputs.length) throw new Error('BIP48_DUPLICATE_INPUT');
  const prefix=concat(required(global,'02',4),doubleSha256(concat(...points)),
    doubleSha256(concat(...inputs.map(sequence))));
  const suffix=concat(doubleSha256(concat(...outputs.map(outputBytes))),global.get('03')??new Uint8Array(4),serializeLE(1n,4));
  return inputs.map((fields,index)=>{
    const coin=fields.get('01');
    const spent=previousBitcoinOutput(required(fields,'00'),points[index],16);
    if (coin && (coin.length!==43 || coin[8]!==34 ||
        !equalBytes(spent.amount,coin.subarray(0,8)) || !equalBytes(spent.script,coin.subarray(9))))
      throw new Error('BIP48_PREVOUT');
    const sighash=fields.get('03');
    if (sighash && hex(sighash)!=='01000000') throw new Error('BIP48_SIGNATURE');
    const witness=required(fields,'05',105);
    const path=verifiedProgram(fields,'06',witness,spent.script,policy);
    const digest=doubleSha256(concat(prefix,points[index],Uint8Array.of(witness.length),witness,spent.amount,
      sequence(fields),suffix));
    return {children:path.children,digest,amount:little(spent.amount),script:hex(spent.script),
      outpoint:hex(points[index].slice(0,32).reverse())+':'+little(points[index].subarray(32))};
  });
}

function verifySignature(partial:Uint8Array,digest:Uint8Array,publicKey:string) {
  if (partial.at(-1)!==1 || !secp256k1.verify(partial.subarray(0,-1),digest,hexToBytes(publicKey),
    {format:'der',lowS:true})) throw new Error('BIP48_SIGNATURE');
}

function verifyOtherSignatures(maps:ReturnType<typeof psbtMaps>,evidence:ReturnType<typeof inputsForReview>,owner:number) {
  const slots=splitMaps(maps).inputs.map((fields,index)=>{
    const partials=[...fields.entries()].filter(([key])=>key.startsWith('02'));
    if (partials.length>1) throw new Error('BIP48_THRESHOLD_ALREADY_PRESENT');
    if (!partials.length) return -1;
    const [key,partial]=partials[0];
    const slot=evidence[index].children.indexOf(key.slice(2));
    if (slot<0 || slot===owner) throw new Error('BIP48_SIGNER');
    verifySignature(partial,evidence[index].digest,key.slice(2));
    return slot;
  });
  if (!slots.every(slot=>slot===slots[0])) throw new Error('BIP48_COUNTERPART_SCOPE');
  return slots[0]>=0 ? 1 : 0;
}

function sats(amount: string) {
  const m = /^(\d{1,9})(?:\.(\d{1,8}))?$/.exec(amount);
  if (!m) throw new Error('BIP48_DRAFT_AMOUNT');
  return BigInt(m[1] + (m[2] ?? '').padEnd(8, '0'));
}

export async function prepareBip48(rpc: ChainRpc, scope: Bip48Scope, policy: Bip48Policy, trust: Uint8Array,
  request: {recipient: string; amount: string; counterpart?: string}) {
  const counterpart=request.counterpart ? readBip48Import(request.counterpart) : undefined;
  const reply = object(await rpc('nu54_bip48Prepare', [counterpart ?
    {...scope, counterpart} : {...scope, recipient: request.recipient, amount: request.amount}]));
  if (typeof reply.grant !== 'string') throw new Error('BIP48_GRANT');
  const grant = readChainTransfer(reply.grant, Math.floor(Date.now() / 1000), 18);
  if (grant.chain !== 1 || grant.network !== (scope.network??1) || grant.account !== policy.account ||
      !equalBytes(grant.body.subarray(23, 55), policy.hash)) throw new Error('BIP48_POLICY_MISMATCH');
  verifyChainAuthority(grant, trust, scope.board, BigInt(scope.generation), BigInt(scope.leaseRevision), scope.anchor);
  if (counterpart && !equalBytes(fromBase64(counterpart),grant.raw)) throw new Error('BIP48_ORIGINAL_MISMATCH');
  const maps=psbtMaps(grant.raw);
  const approvedInputs=inputsForReview(maps,policy);
  verifyOtherSignatures(maps,approvedInputs,policy.owner);
  const {outputs}=splitMaps(maps);
  if (!counterpart) {
    const external=outputs.filter(output=>!output.has('01'));
    if (external.length!==1 || segwitAddress(bitcoinHrp(grant.network),required(external[0],'04'))!==request.recipient ||
        little(required(external[0],'03',8))!==sats(request.amount)) throw new Error('BIP48_DRAFT_MISMATCH');
  }
  const facts=outputs.map(output=>{
    const script=required(output,'04');
    const witness=output.get('01');
    const path=output.size>2 ? verifiedProgram(output,'02',witness??new Uint8Array(),script,policy,true) : undefined;
    return {address:segwitAddress(bitcoinHrp(grant.network),script),script:hex(script),
      satoshis:little(required(output,'03',8)).toString(),
      change:!!path,path:path ? `m/48'/1'/${policy.account}'/2'/1/${path.addressIndex}` : undefined};
  });
  const inputTotal=approvedInputs.reduce((sum,input)=>sum+input.amount,0n);
  const outputTotal=facts.reduce((sum,row)=>sum+BigInt(row.satoshis),0n);
  const fee=inputTotal-outputTotal;
  const cap=BigInt('0x'+hex(grant.body.subarray(15,23)));
  if (fee<0n || cap<=0n || fee>cap || cap>BigInt('0x'+hex(policy.bytes.subarray(7,15))))
    throw new Error('BIP48_FEE');
  const original=bip48UnsignedTransaction(maps);
  // Four witness items: CHECKMULTISIG dummy, two low-S signatures with hash types, and the 105-byte script.
  const maximumWitness=1+1+2*(1+72)+1+105;
  const maximumVsize=Math.ceil((4*original.length+2+maximumWitness*approvedInputs.length)/4);
  const rate=fee*100n/BigInt(maximumVsize);
  const minimumFeeRate=`${rate/100n}.${(rate%100n).toString().padStart(2,'0')}`;
  return {grant, outputs:facts,inputs:approvedInputs.map(({outpoint,amount,script})=>({outpoint,amount:amount.toString(),script})),
    fee:fee.toString(),inputTotal:inputTotal.toString(),outputTotal:outputTotal.toString(),maximumVsize,minimumFeeRate,
    id:hex(doubleSha256(original).reverse()),
    fingerprints:[0,1,2].map(slot=>hex(policy.bytes.subarray(17+slot*69,21+slot*69)))};
}

export function verifyBip48Partial(raw:Uint8Array,signed:Uint8Array,policy:Bip48Policy) {
  if (signed.length===1 && signed[0]===0) throw new Error('CHAIN_USER_REFUSED');
  const original=psbtMaps(raw), result=psbtMaps(signed);
  if (original.length!==result.length) throw new Error('BIP48_SIGNED_MISMATCH');
  const evidence=inputsForReview(original,policy);
  for (const [index,fields] of result.entries()) {
    const before=original[index];
    for (const [key,value] of before) {
      if (!fields.has(key) || !equalBytes(value,fields.get(key)!)) throw new Error('BIP48_SIGNED_MISMATCH');
    }
    const appended=[...fields.keys()].filter(key=>!before.has(key));
    if (index===0 || index>evidence.length) {
      if (appended.length) throw new Error('BIP48_SIGNED_MISMATCH');
      continue;
    }
    if (appended.length!==1 || !/^02(?:02|03)[a-f0-9]{64}$/.test(appended[0])) throw new Error('BIP48_SIGNED_MISMATCH');
    const input=evidence[index-1], publicKey=input.children[policy.owner];
    if (appended[0]!=='02'+publicKey) throw new Error('BIP48_SIGNER');
    verifySignature(required(fields,appended[0]),input.digest,publicKey);
  }
  return {count:1+verifyOtherSignatures(original,evidence,policy.owner)};
}
