import {keccak_256} from '@noble/hashes/sha3';
import {sha256} from '@noble/hashes/sha256';
import {p256} from '@noble/curves/p256';
import {secp256k1} from '@noble/curves/secp256k1';
import {bigToBytes,bytesToBig,bytesToHex,concat,equalBytes,hexToBytes} from '@nu54/protocol';

export const evmProfiles = {
  '11155111': {label:'Ethereum Sepolia',native:'ETH',l1Fee:false},
  '84532': {label:'Base Sepolia',native:'ETH',l1Fee:true},
  '8283': {label:'StableNet',native:'StableNet gas asset',l1Fee:false},
} as const;
export type EvmChain = keyof typeof evmProfiles;

const headerSize = 101;
const fixed = headerSize + 64;
const transferSelector = 'a9059cbb';
const revokeSelector = '095ea7b3';
const zeroAddress = '0x' + '0'.repeat(40);
const u64 = (value:Uint8Array) => bytesToBig(value);
type Field = {raw:Uint8Array; bytes:Uint8Array; list:boolean};

function fields(bytes:Uint8Array, list=false):Field[] {
  const result:Field[] = [];
  let next = 0;
  while (next < bytes.length) {
    const start = next;
    const tag = bytes[next++];
    const grouped = tag >= 0xc0;
    const prefix = grouped ? 0xc0 : 0x80;
    let size = tag - prefix;
    if (tag < 0x80) {
      next--;
      size = 1;
    } else if (size >= 56) {
      if (size !== 56 || next === bytes.length || bytes[next] < 56) throw new Error('EVM_RLP');
      size = bytes[next++];
    }
    const end = next + size;
    if (end > bytes.length || (!grouped && tag === 0x81 && bytes[next] < 0x80)) throw new Error('EVM_RLP');
    result.push({raw:bytes.subarray(start,end),bytes:bytes.subarray(next,end),list:grouped});
    next = end;
  }
  if (list && (result.length !== 1 || !result[0].list)) throw new Error('EVM_RLP');
  return result;
}

function integer(field:Field, max:number) {
  if (field.list || field.bytes.length > max || (field.bytes.length && field.bytes[0] === 0)) throw new Error('EVM_RLP_INTEGER');
  return field.bytes.length ? bytesToBig(field.bytes) : 0n;
}

function wireFields(raw:Uint8Array) {
  if (raw[0] !== 2) throw new Error('EVM_TX_KIND');
  return fields(fields(raw.subarray(1),true)[0].bytes);
}

export function normalizeEvmAddress(input:string) {
  if (!/^0x[a-fA-F0-9]{40}$/.test(input) || /^0x0{40}$/.test(input)) throw new Error('EVM_RECIPIENT');
  const body=input.slice(2), lowercase=body.toLowerCase();
  if (body===lowercase || body===body.toUpperCase()) return '0x'+lowercase;
  const textHash=bytesToHex(keccak_256(Uint8Array.from(lowercase,value=>value.charCodeAt(0))),false);
  for (let index=0;index<body.length;index++) {
    const expected=parseInt(textHash[index],16)>=8 ? lowercase[index].toUpperCase() : lowercase[index];
    if (body[index]!==expected) throw new Error('EVM_RECIPIENT_CHECKSUM');
  }
  return '0x'+lowercase;
}

export type DappAllowancePolicy={token:string;spender:string;maximum:string};
export const maxFiniteAllowance=(1n<<256n)-2n;
export type ExternalSpendContext={origin:string;pairingTopic:string;sessionTopic:string;peerKey:string};

export function readDirectTransfer(text:string, now:bigint) {
  if (!/^[0-9a-f]{334,1352}$/.test(text) || text.length % 2) throw new Error('EVM_TRANSFER_WIRE');
  const body = hexToBytes(text);
  const revoke=body[0] === 14;
  const external=body[0]===20;
  const approval=body[0]===36;
  if (body[0] !== 9 && !revoke && !external && !approval) throw new Error('EVM_TX_KIND');
  const expires = u64(body.subarray(1,9));
  const chain = u64(body.subarray(9,17)).toString() as EvmChain;
  const feeLimit = u64(body.subarray(17,25));
  const l1Fee = u64(body.subarray(25,33));
  const asset = bytesToHex(body.subarray(33,53),true);
  const genesis = bytesToHex(body.subarray(53,85),true);
  const profileRevision = u64(body.subarray(85,93));
  const leaseRevision = u64(body.subarray(93,101));
  let rawOffset=fixed;
  let externalContext:ExternalSpendContext|undefined;
  let allowanceContext:{origin:string;sessionTopic:string;external?:ExternalSpendContext;spender:string;maximum:bigint;decimals:number}|undefined;
  if (external || approval) {
    const transport=approval ? body[fixed] : 1;
    const contextStart=approval ? fixed+54 : fixed;
    const length=body[contextStart+96];
    rawOffset=contextStart+97+length;
    const id=(offset:number)=>bytesToHex(body.subarray(contextStart+offset,contextStart+offset+32),false);
    const first=id(0), second=id(32), third=id(64);
    const origin=String.fromCharCode(...body.subarray(contextStart+97,rawOffset));
    const present=(hex:string)=>/[1-9a-f]/.test(hex);
    if (length<9 || length>104 || rawOffset>=body.length || !present(first) ||
        !/^https:\/\/[a-z0-9.-]+(?::[0-9]{1,5})?$/.test(origin) || new URL(origin).origin!==origin ||
        (transport===1 ? !present(second) || !present(third) || first===second :
          transport!==0 || present(second) || present(third))) throw new Error('EVM_DAPP_SCOPE');
    const peer=transport===1 ? {origin,pairingTopic:first,sessionTopic:second,peerKey:third} : undefined;
    if (external) {
      if (asset!==zeroAddress) throw new Error('EVM_WC_SCOPE');
      externalContext=peer;
    } else {
      const spender=bytesToHex(body.subarray(fixed+1,fixed+21),true);
      const maximum=bytesToBig(body.subarray(fixed+21,fixed+53));
      const decimals=body[fixed+53];
      if (asset===zeroAddress || spender===zeroAddress || maximum===0n || maximum>maxFiniteAllowance || decimals>24)
        throw new Error('EVM_ALLOWANCE_POLICY');
      allowanceContext={origin,sessionTopic:peer?.sessionTopic ?? first,external:peer,spender,maximum,decimals};
    }
  }
  const raw = body.subarray(rawOffset);
  if (!body.subarray(53,85).some(Boolean) || profileRevision !== 1n || !leaseRevision || !Object.hasOwn(evmProfiles,chain) || expires <= now || expires > now+120n ||
      evmProfiles[chain].l1Fee !== (l1Fee > 0n)) throw new Error('EVM_PROFILE');
  const parts = wireFields(raw);
  if (parts.length !== 9 || parts.slice(0,8).some(part=>part.list) ||
      !parts[8].list || parts[8].bytes.length || integer(parts[0],8).toString() !== chain ||
      parts[5].bytes.length !== 20 || bytesToHex(parts[5].bytes,true) === zeroAddress) throw new Error('EVM_TX_FIELDS');
  const nonce = integer(parts[1],8);
  const priority = integer(parts[2],8);
  const maxFee = integer(parts[3],8);
  const gas = integer(parts[4],8);
  const maximum = gas*maxFee+l1Fee;
  const value = integer(parts[6],32);
  const data = parts[7].bytes;
  if (priority > maxFee || !maxFee || gas < 21000n || maximum > feeLimit) throw new Error('EVM_FEE');
  let amount:bigint, recipient:string;
  if (asset === zeroAddress) {
    if (revoke || gas !== 21000n || data.length || !value) throw new Error('EVM_NATIVE');
    amount = value;
    recipient = bytesToHex(parts[5].bytes,true);
  } else {
    if (gas > 150000n || value || bytesToHex(parts[5].bytes,true) !== asset || data.length !== 68 ||
        bytesToHex(data.subarray(0,4),false) !== (revoke || approval ? revokeSelector : transferSelector) || data.subarray(4,16).some(Boolean)) throw new Error('EVM_ERC20');
    amount = bytesToBig(data.subarray(36));
    recipient = bytesToHex(data.subarray(16,36),true);
    if ((amount === 0n) !== revoke || recipient === zeroAddress) throw new Error('EVM_ERC20');
  }
  if (allowanceContext && (amount>allowanceContext.maximum || recipient!==allowanceContext.spender))
    throw new Error('EVM_ALLOWANCE_POLICY');
  return {body,raw,revoke,externalContext,allowanceContext,rawOffset,expires,chain,genesis,profileRevision,leaseRevision,nonce,asset,recipient,amount,maximum,feeLimit,l1Fee,executionMaximum:gas*maxFee,gas,maxFee,priority,digest:keccak_256(raw)};
}

export function verifyTransferAuthority(intent:ReturnType<typeof readDirectTransfer>, authority:Uint8Array,
    board:string, generation:bigint, sender:string, revision:bigint) {
  if (authority.length !== 65 || authority[0] !== 4 || revision !== intent.leaseRevision ||
      !/^[a-f0-9]{64}$/.test(board) || !/^0x[a-f0-9]{40}$/.test(sender) || generation <= 0n)
    throw new Error('EVM_TRANSFER_AUTHORITY_SCOPE');
  const context = concat(Uint8Array.of(0x4e,0x35,0x34,0x56),hexToBytes(board),bigToBytes(generation,8),
    hexToBytes(sender.slice(2)),intent.body.subarray(0,headerSize),intent.digest,intent.body.subarray(fixed,intent.rawOffset));
  if (!p256.verify(intent.body.subarray(headerSize,fixed),sha256(context),authority,{lowS:false}))
    throw new Error('EVM_TRANSFER_AUTHORITY_SIGNATURE');
}

export function verifySignedTransfer(intent:ReturnType<typeof readDirectTransfer>, result:Uint8Array, sender:string) {
  const signed = wireFields(result);
  const unsigned = wireFields(intent.raw);
  if (signed.length !== 12 || unsigned.some((field, index)=>!equalBytes(field.raw,signed[index].raw))) throw new Error('EVM_SIGNED_MISMATCH');
  const parity = integer(signed[9],1);
  const r = integer(signed[10],32);
  const s = integer(signed[11],32);
  if (parity > 1n || !r || !s || s > secp256k1.CURVE.n/2n) throw new Error('EVM_SIGNATURE');
  const key = new secp256k1.Signature(r,s).addRecoveryBit(Number(parity)).recoverPublicKey(intent.digest).toRawBytes(false);
  const recovered = bytesToHex(keccak_256(key.subarray(1)).subarray(12),true);
  if (recovered !== sender) throw new Error('EVM_SENDER');
  return {raw:bytesToHex(result,true),hash:bytesToHex(keccak_256(result),true)};
}

export type EvmRpc = (method:string,params:readonly unknown[])=>Promise<unknown>;

const hashFormat = /^0x[a-f0-9]{64}$/;
const quantity = (value:unknown) => {
  if (typeof value !== 'string' || !/^0x(?:0|[a-f1-9][a-f0-9]{0,63})$/.test(value)) throw new Error('EVM_RPC_QUANTITY');
  return BigInt(value);
};
const object = (value:unknown) => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('EVM_RPC_SHAPE');
  return value as Record<string,unknown>;
};

export async function checkEvmNetwork(rpc:EvmRpc, chain:EvmChain, genesis:string) {
  if (!hashFormat.test(genesis) || quantity(await rpc('eth_chainId',[])).toString() !== chain)
    throw new Error('EVM_RPC_CHAIN');
  const block = await rpc('eth_getBlockByNumber',['0x0',false]);
  if (block === null) throw new Error('EVM_RPC_GENESIS_UNAVAILABLE');
  if (object(block).hash !== genesis) throw new Error('EVM_RPC_CHAIN');
}

export function storedAllowanceChange(raw:string) {
  const parts=wireFields(hexToBytes(raw.slice(2)));
  const data=parts[7]?.bytes;
  if (data?.length!==68 || bytesToHex(data.subarray(0,4),false)!==revokeSelector) return;
  const amount=bytesToBig(data.subarray(36));
  if (parts[5].bytes.length!==20 || integer(parts[6],32)!==0n || data.subarray(4,16).some(Boolean) ||
      amount>maxFiniteAllowance) throw new Error('EVM_ALLOWANCE_WIRE');
  return {token:normalizeEvmAddress(bytesToHex(parts[5].bytes,true)),
    spender:normalizeEvmAddress(bytesToHex(data.subarray(16,36),true)),amount};
}

export async function finalizedAllowance(rpc:EvmRpc, chain:EvmChain, genesis:string, token:string, owner:string, spender:string, block:'finalized'|{blockHash:string;requireCanonical:true}='finalized') {
  await checkEvmNetwork(rpc,chain,genesis);
  const argument=(address:string)=>normalizeEvmAddress(address).slice(2).padStart(64,'0');
  const value=await rpc('eth_call',[{to:normalizeEvmAddress(token),data:'0xdd62ed3e'+argument(owner)+argument(spender)},block]);
  if (typeof value!=='string' || !hashFormat.test(value)) throw new Error('EVM_ALLOWANCE_RESULT');
  return BigInt(value);
}

export async function queryEvmTransfer(rpc:EvmRpc, chain:EvmChain, genesis:string, hash:string) {
  if (!hashFormat.test(hash)) throw new Error('EVM_TX_HASH');
  await checkEvmNetwork(rpc,chain,genesis);
  const found = await rpc('eth_getTransactionReceipt',[hash]);
  if (found === null) return 'pending';
  const receipt = object(found);
  if (receipt.transactionHash !== hash || typeof receipt.blockHash !== 'string' || !hashFormat.test(receipt.blockHash))
    throw new Error('EVM_RECEIPT_BINDING');
  const height = quantity(receipt.blockNumber);
  const canonical = object(await rpc('eth_getBlockByNumber',[receipt.blockNumber,false]));
  if (canonical.hash !== receipt.blockHash) return 'pending';
  const tip = object(await rpc('eth_getBlockByNumber',['finalized',false]));
  if (quantity(tip.number) < height) return 'included';
  const confirmedBlock = object(await rpc('eth_getBlockByNumber',[receipt.blockNumber,false]));
  if (confirmedBlock.hash !== receipt.blockHash) return 'pending';
  const status = quantity(receipt.status);
  if (status > 1n) throw new Error('EVM_RECEIPT_STATUS');
  return status === 1n ? 'finalized' : 'reverted';
}

export async function queryEvmAllowanceChange(rpc:EvmRpc, chain:EvmChain, genesis:string, hash:string,
    token:string, owner:string, spender:string):Promise<{state:Awaited<ReturnType<typeof queryEvmTransfer>>;value?:bigint}> {
  const state=await queryEvmTransfer(rpc,chain,genesis,hash);
  if (state!=='finalized') return {state};
  const receipt=object(await rpc('eth_getTransactionReceipt',[hash]));
  const block=object(await rpc('eth_getBlockByNumber',['finalized',false]));
  if (typeof block.hash!=='string' || !hashFormat.test(block.hash)) throw new Error('EVM_FINALIZED_BLOCK');
  if (receipt.transactionHash!==hash || quantity(receipt.status)!==1n ||
      quantity(block.number)<quantity(receipt.blockNumber) ||
      object(await rpc('eth_getBlockByNumber',[receipt.blockNumber,false])).hash!==receipt.blockHash) return {state:'pending'};
  const value=await finalizedAllowance(rpc,chain,genesis,token,owner,spender,{blockHash:block.hash,requireCanonical:true});
  const repeated=await queryEvmTransfer(rpc,chain,genesis,hash);
  if (repeated!=='finalized') return {state:repeated};
  const latestReceipt=object(await rpc('eth_getTransactionReceipt',[hash]));
  if (latestReceipt.transactionHash!==hash || latestReceipt.blockHash!==receipt.blockHash ||
      latestReceipt.blockNumber!==receipt.blockNumber || latestReceipt.status!==receipt.status ||
      object(await rpc('eth_getBlockByNumber',[block.number,false])).hash!==block.hash) return {state:'pending'};
  const latestFinalized=object(await rpc('eth_getBlockByNumber',['finalized',false]));
  if (latestFinalized.hash!==block.hash || latestFinalized.number!==block.number) return {state:'pending'};
  return {state,value};
}

export function verifyStoredEvmWire(raw:string,hash:string,chain:EvmChain) {
  if (!/^0x02(?:[a-f0-9]{2}){1,322}$/.test(raw) || !hashFormat.test(hash)) throw new Error('EVM_PENDING_WIRE');
  const bytes=hexToBytes(raw.slice(2)), parsed=wireFields(bytes);
  if (parsed.length!==12 || integer(parsed[0],8).toString()!==chain || bytesToHex(keccak_256(bytes),true)!==hash)
    throw new Error('EVM_PENDING_BINDING');
  const target=bytesToHex(parsed[5].bytes,true), calldata=parsed[7].bytes;
  if (!calldata.length) return {nonce:integer(parsed[1],8),recipient:normalizeEvmAddress(target),
    amount:integer(parsed[6],32), asset:zeroAddress, executionMaximum:integer(parsed[3],8)*integer(parsed[4],8)};
  const method=bytesToHex(calldata.subarray(0,4),false);
  if (calldata.length!==68 || (method!==transferSelector && method!==revokeSelector) || calldata.subarray(4,16).some(Boolean))
    throw new Error('EVM_PENDING_BINDING');
  return {nonce:integer(parsed[1],8),recipient:normalizeEvmAddress(bytesToHex(calldata.subarray(16,36),true)),
    amount:bytesToBig(calldata.subarray(36)),asset:normalizeEvmAddress(target),
    executionMaximum:integer(parsed[3],8)*integer(parsed[4],8)};
}

/** Validate a bounded canonical unsigned EIP-1559 original retained before signing. */
export function unsignedEvmDigest(unsignedRaw:string,chain:EvmChain) {
  if(!/^0x02(?:[a-f0-9]{2}){1,255}$/.test(unsignedRaw))throw new Error('EVM_PENDING_BINDING');
  const bytes=hexToBytes(unsignedRaw.slice(2)),parts=wireFields(bytes);
  if(parts.length!==9 || integer(parts[0],8).toString()!==chain || parts.slice(0,8).some(part=>part.list) || !parts[8].list || parts[8].bytes.length)throw new Error('EVM_PENDING_BINDING');
  return bytesToHex(keccak_256(bytes),false);
}

/** Authenticate the saved original against the signed transaction and sender. */
export function verifyPendingOriginal(signedRaw:string,hash:string,chain:EvmChain,sender:string,unsignedRaw:string) {
  verifyStoredEvmWire(signedRaw,hash,chain);
  if(!/^0x02(?:[a-f0-9]{2}){1,255}$/.test(unsignedRaw))throw new Error('EVM_PENDING_BINDING');
  const unsignedBytes=hexToBytes(unsignedRaw.slice(2)),unsigned=wireFields(unsignedBytes),signed=wireFields(hexToBytes(signedRaw.slice(2)));
  if(unsigned.length!==9 || signed.length!==12 || integer(unsigned[0],8).toString()!==chain ||
     unsigned.some((field,index)=>!equalBytes(field.raw,signed[index].raw)))throw new Error('EVM_PENDING_BINDING');
  const parity=integer(signed[9],1),r=integer(signed[10],32),s=integer(signed[11],32);
  if(parity>1n || !r || !s || s>secp256k1.CURVE.n/2n)throw new Error('EVM_PENDING_BINDING');
  const publicKey=new secp256k1.Signature(r,s).addRecoveryBit(Number(parity)).recoverPublicKey(keccak_256(unsignedBytes)).toRawBytes(false);
  if(bytesToHex(keccak_256(publicKey.subarray(1)).subarray(12),true)!==sender)throw new Error('EVM_PENDING_BINDING');
  return bytesToHex(keccak_256(unsignedBytes),false);
}

export function httpsEvmRpc(endpoint:string, exchange:(endpoint:string,packet:string)=>Promise<string>):EvmRpc {
  const url = new URL(endpoint);
  if (url.protocol !== 'https:' || url.username || url.password || url.hash) throw new Error('EVM_RPC_ENDPOINT');
  return async (method,params) => {
    const raw = await exchange(url.href,JSON.stringify({jsonrpc:'2.0',id:1,method,params}));
    if (raw.length > 256000) throw new Error('EVM_RPC_SIZE');
    const reply = object(JSON.parse(raw));
    if (reply.id !== 1 || reply.jsonrpc !== '2.0') throw new Error('EVM_RPC_RESPONSE');
    if ('error' in reply) {
      const error = object(reply.error);
      if (method === 'eth_getBlockByNumber' && params[0] === '0x0' && error.code === 4444)
        throw new Error('EVM_RPC_GENESIS_UNAVAILABLE');
      throw new Error('EVM_RPC_RESPONSE');
    }
    if (!('result' in reply)) throw new Error('EVM_RPC_RESPONSE');
    return reply.result;
  };
}

export async function readEvmReceiveBalance(rpc: EvmRpc, chain: EvmChain, genesis: string, account: string,
  asset = '0x' + '0'.repeat(40), includeDecimals = false) {
  await checkEvmNetwork(rpc, chain, genesis);
  const owner = normalizeEvmAddress(account);
  const native = '0x' + '0'.repeat(40);
  const token = asset === native ? native : normalizeEvmAddress(asset);
  const block = object(await rpc('eth_getBlockByNumber', ['finalized', false]));
  if (typeof block.hash !== 'string' || !hashFormat.test(block.hash)) throw new Error('EVM_FINALIZED_BLOCK');
  const blockScope = {blockHash: block.hash, requireCanonical: true};
  const number = quantity(block.number);
  if (token === native) {
    return {amount: quantity(await rpc('eth_getBalance', [owner, blockScope])).toString(), block: number.toString(),
      ...(includeDecimals ? {decimals: 18} : {})};
  }
  const result = await rpc('eth_call', [{to: token, data: '0x70a08231' + '0'.repeat(24) + owner.slice(2)}, blockScope]);
  if (typeof result !== 'string' || !hashFormat.test(result)) throw new Error('EVM_BALANCE_SCHEMA');
  if (!includeDecimals) return {amount: BigInt(result).toString(), block: number.toString()};
  const metadata = await rpc('eth_call', [{to: token, data: '0x313ce567'}, blockScope]);
  if (typeof metadata !== 'string' || !hashFormat.test(metadata) || BigInt(metadata) > 36n)
    throw new Error('EVM_ASSET_DECIMALS');
  return {amount: BigInt(result).toString(), block: number.toString(), decimals: Number(BigInt(metadata))};
}

/** Phone matching is a cleanup guard; the board remains the evidence authority. */
export function finalityMatchesPending(body:Uint8Array,original:Uint8Array,pending:{chain:EvmChain;raw:string;hash:string;sender:string;genesis?:string;evidence?:{board:string;generation:number;request:string;unsigned:string}}) {
  const e=pending.evidence;if(!e || pending.chain!=='8283' || !pending.genesis ||
    !((body.length===237 && bytesToHex(body.slice(0,4),false)==='4e353446' && [1,2].includes(body[236])) ||
      (body.length===269 && bytesToHex(body.slice(0,4),false)==='4e353452' && [3,4].includes(body[236]))))return false;
  try {
    const digest=verifyPendingOriginal(pending.raw,pending.hash,pending.chain,pending.sender,e.unsigned);
    return bytesToHex(body.slice(4,36),false)===e.board && bytesToHex(body.slice(36,68),true)===pending.genesis &&
      bytesToHex(body.slice(68,88),true)===pending.sender && bytesToHex(body.slice(108,124),false)===e.request &&
      bytesToHex(body.slice(124,156),false)===digest && bytesToBig(body.slice(188,196))===BigInt(e.generation) &&
      bytesToBig(body.slice(196,204))===8283n && bytesToHex(original,true)===e.unsigned;
  } catch{return false;}
}

/** An ambiguous signing can clear only after the matching board accepted evidence. */
export function finalityMatchesSigning(body:Uint8Array,original:Uint8Array,signing:{kind:'evm'|'chain';board:string;generation:number;evm?:{request:string;unsigned:string;chain:EvmChain;genesis:string;signer:string}}) {
  const e=signing.evm;if(signing.kind!=='evm' || !e || e.chain!=='8283' ||
    !((body.length===237 && bytesToHex(body.slice(0,4),false)==='4e353446' && [1,2].includes(body[236])) ||
      (body.length===269 && bytesToHex(body.slice(0,4),false)==='4e353452' && [3,4].includes(body[236]))))return false;
  try{return bytesToHex(body.slice(4,36),false)===signing.board && bytesToHex(body.slice(36,68),true)===e.genesis &&
    bytesToHex(body.slice(68,88),true)===e.signer && bytesToHex(body.slice(108,124),false)===e.request &&
    bytesToHex(body.slice(124,156),false)===unsignedEvmDigest(e.unsigned,e.chain) &&
    bytesToBig(body.slice(188,196))===BigInt(signing.generation) && bytesToBig(body.slice(196,204))===8283n &&
    bytesToHex(original,true)===e.unsigned;
  }catch{return false;}
}
