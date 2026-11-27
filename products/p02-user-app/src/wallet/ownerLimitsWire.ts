import {bigToBytes,bytesToBig,bytesToHex,concat,hexToBytes} from '@nu54/protocol';

export type OwnerLimits = {board:string; generation:bigint; revision:bigint; chain:bigint; asset:string; destination:string;
  perTransfer:bigint; daily:bigint; spent:bigint; mode?:'finality'; recipients?:string[];
  pending?:{perTransfer:bigint; daily:bigint; notBefore:bigint; destination:string; recipients?:string[];
    kind?:'legacy'|'recipients'|'caps'; request?:string}};
const equalRecipients=(left?:string[],right?:string[])=>left?.join(',')===right?.join(',');
export function ownerLimitChangeApplied(expected:OwnerLimits, applied:OwnerLimits):boolean {
 const pending=expected.pending;
 return !!pending && applied.mode!=='finality' && !applied.pending && applied.board===expected.board &&
    applied.generation===expected.generation && applied.revision===expected.revision+1n &&
    applied.chain===expected.chain && applied.asset===expected.asset && applied.destination===pending.destination &&
    equalRecipients(pending.recipients,applied.recipients) &&
    applied.perTransfer===pending.perTransfer && applied.daily===pending.daily;
}

export const ownerIdleLockOperation=0x73;
export type OwnerIdleLock={idleSeconds:120;disconnect:true};
export function readOwnerIdleLock(reply:Uint8Array):OwnerIdleLock {
 if(reply.length!==5 || reply[0]!==ownerIdleLockOperation || reply[1]!==1 || reply[2]!==3 || reply[3]!==0 || reply[4]!==120)
   throw new Error('OWNER_IDLE_LOCK_UNAVAILABLE');
 return {idleSeconds:120,disconnect:true};
}

const nonzero=(bytes:Uint8Array)=>bytes.some(value=>value!==0);
const u64=(bytes:Uint8Array,offset:number)=>bytesToBig(bytes.slice(offset,offset+8));
const address=(bytes:Uint8Array,offset:number)=>'0x'+bytesToHex(bytes.slice(offset,offset+20),false);
export const ownerSetOps = {read:32, recipients:33, caps:34, activate:35} as const;
const maxU64=0xffffffffffffffffn;

function recipientsAt(bytes:Uint8Array,offset:number):string[] {
 const count=bytes[offset];
 if(count>8 || nonzero(bytes.slice(offset+1+count*20,offset+161)))throw new Error('OWNER_LIMITS_SET');
 const entries=Array.from({length:count},(_,i)=>address(bytes,offset+1+i*20));
 if(entries.some((value,i)=>/^0x0{40}$/.test(value) || (i>0 && value<=entries[i-1])))throw new Error('OWNER_LIMITS_SET');
 return entries;
}

export function normalizePolicyRecipients(entries:string[]):string[] {
 if(entries.length>8 || entries.some(value=>!/^0x[0-9a-fA-F]{40}$/.test(value) || /^0x0{40}$/i.test(value)))
   throw new Error('OWNER_LIMITS_SET');
 const canonical=entries.map(value=>value.toLowerCase()).sort();
 if(new Set(canonical).size!==canonical.length)throw new Error('OWNER_LIMITS_SET');
 return canonical;
}

export function readOwnerLimitSet(wire:Uint8Array,board:string,generation:bigint):OwnerLimits|undefined {
 if(wire[0]!==ownerSetOps.read || wire[1]!==2)throw new Error('OWNER_LIMITS_SET');
 if(wire.length===3 && wire[2]===0)return undefined;
 if(wire.length!==466 || wire[2]!==1 || !/^[a-f0-9]{64}$/.test(board) ||
    bytesToHex(wire.slice(3,35),false)!==board || generation<=0n || u64(wire,35)!==generation)
   throw new Error('OWNER_LIMITS_BINDING');
 const revision=u64(wire,43),chain=u64(wire,51),asset=address(wire,59),perTransfer=u64(wire,79),daily=u64(wire,87),spent=u64(wire,95);
 const recipients=recipientsAt(wire,103), kind=wire[264];
 if(!revision || !chain || !nonzero(wire.slice(59,79)) || !perTransfer || perTransfer>daily || spent>daily ||
     kind>3 || (!kind && nonzero(wire.slice(265))))throw new Error('OWNER_LIMITS_SET');
 let pending:OwnerLimits['pending'];
 if(kind){
   const proposed=recipientsAt(wire,305),request=bytesToHex(wire.slice(289,305),false);
   const notBefore=u64(wire,265),per=u64(wire,273),cap=u64(wire,281);
   if(!notBefore || !per || per>cap || /^0+$/.test(request) || (kind===1 && proposed.length!==1) ||
      (kind===2 && (per!==perTransfer || cap!==daily || equalRecipients(proposed,recipients))) ||
      (kind===3 && (!equalRecipients(proposed,recipients) || (per===perTransfer && cap===daily))))
     throw new Error('OWNER_LIMITS_SET');
   pending={kind:kind===1?'legacy':kind===2?'recipients':'caps',perTransfer:per,daily:cap,notBefore,
     request,recipients:proposed,destination:proposed.length===1?proposed[0]:''};
 }
 return {board,generation,revision,chain,asset,perTransfer,daily,spent,recipients,
   destination:recipients.length===1?recipients[0]:'',pending};
}

export function ownerSetChange(revision:bigint,entries:string[]):Uint8Array {
 if(revision<=0n || revision>maxU64)throw new Error('OWNER_LIMITS_SET');
 const canonical=normalizePolicyRecipients(entries), set=new Uint8Array(161);
 set[0]=canonical.length;
 canonical.forEach((value,index)=>set.set(hexToBytes(value.slice(2)),1+index*20));
 return concat(Uint8Array.of(ownerSetOps.recipients,2),bigToBytes(revision,8),set);
}

export function ownerSetCaps(revision:bigint,per:bigint,daily:bigint):Uint8Array {
 if(revision<=0n || revision>maxU64 || per<=0n || per>daily || daily>maxU64)throw new Error('OWNER_LIMITS_SET');
 return concat(Uint8Array.of(ownerSetOps.caps,2),bigToBytes(revision,8),bigToBytes(per,8),bigToBytes(daily,8));
}

export function ownerSetActivate(expected:OwnerLimits):Uint8Array {
 const request=expected.pending?.request;
 if(!expected.recipients || !request || !/^[a-f0-9]{32}$/.test(request) || /^0+$/.test(request) ||
    expected.revision<=0n || expected.revision>maxU64)throw new Error('OWNER_LIMITS_SET');
 return concat(Uint8Array.of(ownerSetOps.activate,2),bigToBytes(expected.revision,8),hexToBytes(request));
}
export function readOwnerLimits(reply:Uint8Array,board:string,generation:bigint):OwnerLimits|'fresh'|'finality-fresh'|'finality-pending'|undefined {
 if(reply.length===2 && reply[0]===23 && reply[1]===0)return undefined;
 if(reply.length===2 && reply[0]===23 && reply[1]===2)return 'fresh';
 if(reply.length===2 && reply[0]===23 && reply[1]===3)return 'finality-fresh';
 if(reply.length===2 && reply[0]===23 && reply[1]===4)return 'finality-pending';
 if((reply.length!==123 && reply.length!==124 && reply.length!==167) || reply[0]!==23 || reply[1]!==1 || !/^[a-f0-9]{64}$/.test(board) ||
    bytesToHex(reply.slice(2,34),false)!==board || u64(reply,34)!==generation || generation<=0n)throw new Error('OWNER_LIMITS_BINDING');
 const revision=u64(reply,42),chain=u64(reply,50),asset=reply.slice(58,78),destination=reply.slice(78,98);
 const perTransfer=u64(reply,98),daily=u64(reply,106),spent=u64(reply,114),flag=reply[122];
 if(!revision || !chain || !nonzero(asset) || !nonzero(destination) || !perTransfer || perTransfer>daily ||
    (flag!==0 && flag!==1) || (flag===0 && reply.length!==123 && reply.length!==124) || (flag===1 && reply.length!==167))throw new Error('OWNER_LIMITS_MALFORMED');
 if(reply.length===124 && reply[123]!==2)throw new Error('OWNER_LIMITS_MALFORMED');
 let pending:OwnerLimits['pending'];
 if(flag===1){const per=u64(reply,123),day=u64(reply,131),notBefore=u64(reply,139),target=reply.slice(147,167);
   if(!per || per>day || !notBefore || !nonzero(target))throw new Error('OWNER_LIMITS_MALFORMED');
   pending={perTransfer:per,daily:day,notBefore,destination:address(reply,147)};
 }
 return {board,generation,revision,chain,asset:address(reply,58),destination:address(reply,78),perTransfer,daily,spent,pending,...(reply.length===124?{mode:'finality' as const}:{})};
}
export function ownerLimitChange(per:string,daily:string,destination:string):Uint8Array {
 if(!/^(0|[1-9][0-9]{0,19})$/.test(per) || !/^(0|[1-9][0-9]{0,19})$/.test(daily) || !/^0x[0-9a-fA-F]{40}$/.test(destination))throw new Error('OWNER_LIMITS_INPUT');
 const a=BigInt(per),b=BigInt(daily),target=hexToBytes(destination.slice(2));
 if(a<1n || a>b || b>0xffffffffffffffffn || !nonzero(target))throw new Error('OWNER_LIMITS_INPUT');
 return concat(Uint8Array.of(24),bigToBytes(a,8),bigToBytes(b,8),target);
}

export function ownerLimitInitialize(chain:string,asset:string,destination:string,per:string,daily:string):Uint8Array {
 if(!/^[1-9][0-9]{0,19}$/.test(chain) || !/^0x[0-9a-fA-F]{40}$/.test(asset))throw new Error('OWNER_LIMITS_INPUT');
 const id=BigInt(chain),token=hexToBytes(asset.slice(2));
 if(id>0xffffffffffffffffn || !nonzero(token))throw new Error('OWNER_LIMITS_INPUT');
 const change=ownerLimitChange(per,daily,destination);
 return concat(Uint8Array.of(26),bigToBytes(id,8),token,change.slice(17,37),change.slice(1,17));
}

/** Carries evidence to the board; only its enrolled independent key grants authority. */
export function ownerFinalityReconcile(body:Uint8Array,signature:Uint8Array,raw:Uint8Array,board:string,generation:bigint):Uint8Array {
 if(body.length!==237 || signature.length!==64 || raw.length<1 || raw.length>256 ||
    bytesToHex(body.slice(0,4),false)!=='4e353446' || !/^[a-f0-9]{64}$/.test(board) ||
    bytesToHex(body.slice(4,36),false)!==board || u64(body,188)!==generation || generation<=0n ||
    u64(body,196)!==8283n || ![1,2].includes(body[236]))throw new Error('OWNER_FINALITY_BINDING');
 return concat(Uint8Array.of(27),body,signature,bigToBytes(BigInt(raw.length),2),raw);
}

export function ownerFinalityReplacement(body:Uint8Array,signature:Uint8Array,raw:Uint8Array,replacement:Uint8Array,board:string,generation:bigint):Uint8Array {
 if(body.length!==269 || signature.length!==64 || raw.length<1 || raw.length>256 || replacement.length<1 || replacement.length>256 ||
    bytesToHex(body.slice(0,4),false)!=='4e353452' || !/^[a-f0-9]{64}$/.test(board) ||
    bytesToHex(body.slice(4,36),false)!==board || u64(body,188)!==generation || generation<=0n ||
    u64(body,196)!==8283n || ![3,4].includes(body[236]))throw new Error('OWNER_FINALITY_BINDING');
 return concat(Uint8Array.of(31),body,signature,bigToBytes(BigInt(raw.length),2),bigToBytes(BigInt(replacement.length),2),raw,replacement);
}

/** Issuer-signed delegation is reviewed independently on the accessory and board. */
export function ownerFinalityEnrollment(grant:Uint8Array,signature:Uint8Array,board:string,generation:bigint,signer:string):Uint8Array {
 if(grant.length!==233 || signature.length!==64 || bytesToHex(grant.slice(0,4),false)!=='4e353444' ||
    !/^[a-f0-9]{64}$/.test(board) || bytesToHex(grant.slice(4,36),false)!==board || u64(grant,36)!==generation ||
    generation<=0n || u64(grant,44)!==8283n || !/^0x[0-9a-fA-F]{40}$/.test(signer) ||
    bytesToHex(grant.slice(52,72),false)!==signer.slice(2).toLowerCase())throw new Error('OWNER_FINALITY_BINDING');
 return concat(Uint8Array.of(28),grant,signature);
}

export function readOwnerFinalityFile(text:string):{kind:'enrollment';grant:Uint8Array;signature:Uint8Array}|{kind:'evidence';body:Uint8Array;signature:Uint8Array;raw:Uint8Array}|{kind:'replacement';body:Uint8Array;signature:Uint8Array;raw:Uint8Array;replacement:Uint8Array} {
 if(text.length>8192)throw new Error('OWNER_FINALITY_FILE');
 let data:unknown;try{data=JSON.parse(text);}catch{throw new Error('OWNER_FINALITY_FILE');}
 if(!data || typeof data!=='object' || Array.isArray(data))throw new Error('OWNER_FINALITY_FILE');
 const value=data as Record<string,unknown>,keys=Object.keys(value).sort().join(',');
 const read=(field:string,length?:number)=>{const v=value[field];if(typeof v!=='string' || !/^(?:[0-9a-fA-F]{2})+$/.test(v) || (length!==undefined && v.length!==length*2))throw new Error('OWNER_FINALITY_FILE');return hexToBytes(v);};
 if(value.type==='nu54-finality-enrollment-v1' && keys==='grant,signature,type')return {kind:'enrollment',grant:read('grant',233),signature:read('signature',64)};
 if(value.type==='nu54-finality-replacement-v1' && keys==='body,raw,replacement,signature,type'){
   const raw=read('raw'),replacement=read('replacement');if(raw.length>256 || replacement.length>256)throw new Error('OWNER_FINALITY_FILE');
   return {kind:'replacement',body:read('body',269),signature:read('signature',64),raw,replacement};
 }
 if(value.type==='nu54-finality-evidence-v1' && keys==='body,raw,signature,type'){
   const raw=read('raw');if(raw.length>256)throw new Error('OWNER_FINALITY_FILE');return {kind:'evidence',body:read('body',237),signature:read('signature',64),raw};
 }
 throw new Error('OWNER_FINALITY_FILE');
}
