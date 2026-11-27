import {ed25519} from '@noble/curves/ed25519';
import {secp256k1} from '@noble/curves/secp256k1';
import {sha256} from '@noble/hashes/sha256';
import {keccak_256} from '@noble/hashes/sha3';
import {bytesToHex, concat, equalBytes, fromBase64, hexToBytes, toBase64} from '@nu54/protocol';
import {normalizeEvmAddress} from '../evmTransfer.ts';

export type CloudKind = 'dkg' | 'sign' | 'refresh';
export type CloudProvider = {id: string; name: string; issuer: string};
export type CloudNetwork = 'sepolia-native-test-v1' | 'isolated-market-8283-v1';
export type CloudProfile = {api: string; identity: string; approval: string; recovery: string; deviceParticipant?: boolean; rosterAuthority?: string; onboarding?: {network: CloudNetwork; providers: CloudProvider[]}};
export type CloudCredential = {id: string; linked: string};
export type CloudAccount = {id: string; state: string; epoch: number; address?: string; publicKey?: string; recoveryAvailable?: boolean; profile?: string};
export type CloudTransfer = {
  profile: string; chain: string; to: string; valueWei: string; nonce: string; maxFeeWei: string; priorityFeeWei: string;
  market?: {deploymentHash:string; user:string; calldata:string; offer:string};
};
type Peer = {ID: string; Role: 'A' | 'B' | 'C'; Epoch: number};
type Session = {
  ID: string; Wallet: string; Kind: CloudKind; Epoch: number; KeySession: string; Expires: number;
  Digest: number[]; Intent: number[]; Peers: Peer[];
};
export type CloudOperation = { requestId?: string;
  session: Session; state: string; fingerprint: number[]; transfer?: CloudTransfer;
  notBefore: string; acknowledged: number; signature?: string; recoveryId?: number;
  refusal?: RecoveryRefusal; reviewAvailable?: boolean; successorRequest?: string;
};
type RecoveryRefusal = {
  decision: {subject: string; wallet: string; session: string; fingerprint: number[]; epoch: number; vote: string; issuedAt: number};
  signature: string; authority: string; recordedAt: string;
};
export type RecoveryHistoryItem = {requestId: string; recordedAt: string};
export type PendingCloud = {id: string; kind: CloudKind; transfer?: CloudTransfer; reviewOf?: string};

export type PendingClosure = {request: string; wallet: string; epoch: number; submitted?: boolean};
export type ClosureReceipt = PendingClosure & {acceptedAt: string; completedAt?: string; assetMoves: 0};

const utf8 = (value: string) => Uint8Array.from(value, c => c.charCodeAt(0));
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
const opaque = /^[A-Za-z0-9_-]{24,256}$/;
const profileName = 'sepolia-native-test-v1';
const chain = '11155111';

export function readCloudProfile(raw: string): CloudProfile {
  const input = JSON.parse(raw);
  const profile: CloudProfile = {api: input.api, identity: input.identity, approval: input.approval, recovery: input.recovery};
  const values = Object.values(profile);
  for (const value of values) {
    if (typeof value !== 'string' || !/^https:\/\/[a-zA-Z0-9.-]+(?::[0-9]{1,5})?$/.test(value) || new URL(value).origin !== value) {
      throw new Error('P03_PROFILE');
    }
  }
  if (new Set(values).size !== 4) throw new Error('P03_AUTHORITY');
  if (input.deviceParticipant === true) {
    if (typeof input.rosterAuthority!=='string' || fromBase64(input.rosterAuthority).length!==32) throw new Error('P03_ROSTER_AUTHORITY');
    profile.deviceParticipant = true;
    profile.rosterAuthority = input.rosterAuthority;
  }
  if (input.onboarding !== undefined) {
    const {network, providers} = input.onboarding ?? {};
    if ((network !== profileName && network !== cloudMarketProfile) || !Array.isArray(providers) ||
        providers.length < 1 || providers.length > 8) throw new Error('P03_ONBOARDING');
    const registered=providers.map((entry: unknown)=>{
      if (!entry||typeof entry!=='object') throw new Error('P03_PROVIDER');
      const {id,name,issuer}=entry as CloudProvider;
      if (typeof id !== 'string' || !/^[a-z][a-z0-9-]{0,31}$/.test(id) ||
          typeof name !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9 .&_-]{0,47}$/.test(name) ||
          typeof issuer !== 'string' || !issuer.startsWith('https://') || issuer.length > 256) throw new Error('P03_PROVIDER');
      const url=new URL(issuer);
      if (!url.hostname || url.username || url.password || url.search || url.hash) throw new Error('P03_PROVIDER');
      return {id,name,issuer};
    });
    if (new Set(registered.map(provider=>provider.id)).size!==registered.length ||
        new Set(registered.map(provider=>provider.name.toLowerCase())).size!==registered.length) throw new Error('P03_PROVIDER');
    profile.onboarding={network,providers:registered};
  }
  return profile;
}

function base64url(value: Uint8Array) {
  return toBase64(value).replace(/\+/g, '-').replace(/\//g, '_').replace(/[=]+$/, '');
}

export function requestID(random: Uint8Array): string {
  if (random.length !== 16) throw new Error('P03_RANDOM');
  const bytes = random.slice();
  bytes[6] = 0x40 + bytes[6] % 16;
  bytes[8] = 0x80 + bytes[8] % 64;
  const h = bytesToHex(bytes, false);
  return h.slice(0,8)+'-'+h.slice(8,12)+'-'+h.slice(12,16)+'-'+h.slice(16,20)+'-'+h.slice(20);
}

export function readPending(raw: string): PendingCloud | undefined {
  if (!raw) return undefined;
  const item = JSON.parse(raw) as PendingCloud;
  if (!uuid.test(item.id) || !['dkg','sign','refresh'].includes(item.kind) ||
      Boolean(item.transfer) !== (item.kind === 'sign') ||
      (item.reviewOf !== undefined && (item.kind !== 'refresh' || !uuid.test(item.reviewOf) || item.reviewOf === item.id))) throw new Error('P03_PENDING');
  if (item.transfer) transferData(item.transfer);
  return item;
}

export type JournalEntry = {realm: string; subject: string; wallet?: string; pending: PendingCloud};

export function journalName(realm:string,subject:string,wallet?:string, purpose?:'closure') {
  if (!uuid.test(subject) || wallet!==undefined&&!uuid.test(wallet) || realm.length>512) throw new Error('P03_JOURNAL_SCOPE');
  return 'cloud-p03-'+bytesToHex(sha256(utf8((purpose ? purpose+'\0' : '')+realm+'\0'+subject+'\0'+(wallet??''))),false);
}

export function readCloudJournal(raw:string,realm:string,subject:string,wallet?:string): JournalEntry|undefined {
  if (!raw) return undefined;
  const row=JSON.parse(raw) as JournalEntry;
  if (row.realm!==realm || row.subject!==subject || row.wallet!==wallet) throw new Error('P03_JOURNAL_SCOPE');
  const pending=readPending(JSON.stringify(row.pending));
  if (!pending || (!wallet && pending.kind!=='dkg')) throw new Error('P03_JOURNAL_SCOPE');
  return {...row,pending};
}

export function closureJournalName(realm:string,subject:string,wallet:string) {
  return journalName(realm,subject,wallet,'closure');
}

export function readClosureJournal(raw:string,realm:string,subject:string,wallet:CloudAccount): PendingClosure|undefined {
  if(!raw)return undefined;
  const row=JSON.parse(raw);
  if(row.realm!==realm||row.subject!==subject||!uuid.test(row.request)||row.wallet!==wallet.id||!Number.isSafeInteger(row.epoch)||row.epoch<1||
      row.submitted!==undefined&&typeof row.submitted!=='boolean') {
    throw new Error('P03_CLOSURE_SCOPE');
  }
  return row.epoch===wallet.epoch ? {request:row.request,wallet:row.wallet,epoch:row.epoch,submitted:row.submitted} : undefined;
}

function checkClosure(receipt:ClosureReceipt,account:CloudAccount,request?:string) {
  const accepted=Date.parse(receipt.acceptedAt);
  if(!uuid.test(receipt.request)||request!==undefined&&receipt.request!==request||
      receipt.wallet!==account.id||receipt.epoch!==account.epoch||receipt.assetMoves!==0||!Number.isFinite(accepted)||
      receipt.completedAt!==undefined&&(!Number.isFinite(Date.parse(receipt.completedAt))||Date.parse(receipt.completedAt)<accepted)){
    throw new Error('P03_CLOSURE_RECEIPT');
  }
  return receipt;
}

export function prepareTransfer(to: string, valueWei: string, nonce: string, maxFeeWei: string, priorityFeeWei: string): CloudTransfer {
  const t = {profile: profileName, chain, to: normalizeEvmAddress(to.trim()), valueWei, nonce, maxFeeWei, priorityFeeWei};
  transferData(t);
  return t;
}

const bigintBytes = (number: bigint): Uint8Array => {
  if (!number) return new Uint8Array();
  const hex = number.toString(16);
  return hexToBytes(hex.length % 2 ? '0'+hex : hex);
};

function rlpPrefix(length:number, base:number) {
  if (length < 56) return Uint8Array.of(base+length);
  const size=bigintBytes(BigInt(length));
  return concat(Uint8Array.of(base+55+size.length),size);
}

function rlpBytes(value: Uint8Array): Uint8Array {
  return value.length === 1 && value[0] < 128 ? value : concat(rlpPrefix(value.length,128),value);
}

function rlpList(parts: Uint8Array[]): Uint8Array {
  const payload = concat(...parts);
  return concat(rlpPrefix(payload.length,192),payload);
}

export const cloudMarketProfile = 'isolated-market-8283-v1';
function marketTransferData(t:CloudTransfer) {
  const m=t.market;
  if (!m || t.profile!==cloudMarketProfile || t.chain!=='8283' || t.valueWei!=='0' || t.priorityFeeWei!=='0' ||
      !/^0x[0-9a-f]{64}$/.test(m.deploymentHash) || normalizeEvmAddress(t.to)!==t.to ||
      normalizeEvmAddress(m.user)!==m.user || !/^0x[a-f0-9]{904}$/.test(m.offer) ||
      !/^0x(?:[a-f0-9]{136}|[a-f0-9]{200}|[a-f0-9]{904})$/.test(m.calldata) || !/^(0|[1-9][0-9]{0,19})$/.test(t.nonce) ||
      !/^[1-9][0-9]{0,11}$/.test(t.maxFeeWei) || BigInt(t.nonce)>=2n**64n || BigInt(t.maxFeeWei)>100_000_000_000n) {
    throw new Error('P03_MARKET_TRANSFER');
  }
  const call=hexToBytes(m.calldata.slice(2));
  const gas=call.length===452?600000n:65000n;
  const fields=[... [BigInt(t.nonce),BigInt(t.maxFeeWei),gas].map(v=>rlpBytes(bigintBytes(v))),rlpBytes(hexToBytes(t.to.slice(2))),
    rlpBytes(new Uint8Array()),rlpBytes(call)];
  const unsigned=rlpList([...fields,rlpBytes(bigintBytes(8283n)),rlpBytes(new Uint8Array()),rlpBytes(new Uint8Array())]);
  const json=JSON.stringify({profile:t.profile,chain:t.chain,to:t.to,valueWei:t.valueWei,nonce:t.nonce,maxFeeWei:t.maxFeeWei,priorityFeeWei:t.priorityFeeWei,
    market:{deploymentHash:m.deploymentHash,user:m.user,calldata:m.calldata,offer:m.offer}});
  return {fields,digest:keccak_256(unsigned),intent:sha256(utf8('nu54/p03/market-transfer/v1\0'+json)),type:'legacy' as const};
}

function transferData(t: CloudTransfer) {
  if(t.profile===cloudMarketProfile) return marketTransferData(t);
  if (t.market!==undefined || t.profile !== profileName || t.chain !== chain || normalizeEvmAddress(t.to) !== t.to) throw new Error('P03_TRANSFER');
  function integer(value: string, bits: number) {
    if (!/^(0|[1-9][0-9]{0,37})$/.test(value) || BigInt(value) >= 2n ** BigInt(bits)) throw new Error('P03_TRANSFER');
    return BigInt(value);
  }
  const value = integer(t.valueWei,128), nonce = integer(t.nonce,64), fee = integer(t.maxFeeWei,128), priority = integer(t.priorityFeeWei,128);
  if (!value || !fee || priority > fee) throw new Error('P03_TRANSFER');
  const fields = [
    ...[BigInt(chain), nonce, priority, fee, 21000n].map(n=>rlpBytes(bigintBytes(n))),
    rlpBytes(hexToBytes(t.to.slice(2))),rlpBytes(bigintBytes(value)),rlpBytes(new Uint8Array()),rlpList([]),
  ];
  const intent = JSON.stringify({
    profile:t.profile,chain:t.chain,to:t.to,valueWei:t.valueWei,nonce:t.nonce,maxFeeWei:t.maxFeeWei,priorityFeeWei:t.priorityFeeWei,
  });
  return {fields,digest:keccak_256(concat(Uint8Array.of(2),rlpList(fields))),intent:sha256(utf8('nu54/p03/transfer/v1\0'+intent)),type:'dynamic' as const};
}

function octets(value: number[]): Uint8Array {
  if (!Array.isArray(value) || value.length!==32 || value.some(v=>!Number.isInteger(v)||v<0||v>255)) throw new Error('P03_DIGEST');
  return Uint8Array.from(value);
}

export function checkOperation(op: CloudOperation, pending: PendingCloud, subject?:string): CloudOperation {
  const s = op.session;
  const expected = pending.kind === 'sign' && pending.transfer ? transferData(pending.transfer) : undefined;
  const peers = s.Peers.slice().sort((a,b)=>a.ID < b.ID ? -1 : a.ID > b.ID ? 1 : 0)
    .map(p=>({ID:p.ID,Role:p.Role,Epoch:p.Epoch}));
  if ((op.requestId??s.ID)!==pending.id || !uuid.test(s.ID) || s.Kind!==pending.kind || !uuid.test(s.Wallet) || !Number.isSafeInteger(s.Epoch) || s.Epoch<1 ||
      !Number.isSafeInteger(s.Expires) || !Number.isFinite(Date.parse(op.notBefore)) || peers.length<2 || peers.length>5 ||
      !['reserved','running','succeeded','failed','held','expired','denied','refused'].includes(op.state) ||
      !Number.isSafeInteger(op.acknowledged) || op.acknowledged<0 || op.acknowledged>peers.length) throw new Error('P03_OPERATION');
  const digest = octets(s.Digest), intent = octets(s.Intent);
  if (!equalBytes(digest,expected?.digest ?? new Uint8Array(32)) || !equalBytes(intent,expected?.intent ?? new Uint8Array(32))) throw new Error('P03_INTENT');
  if (op.transfer && !equalBytes(transferData(op.transfer).intent,intent)) throw new Error('P03_TRANSFER_MISMATCH');
  const canonical = JSON.stringify({ID:s.ID,Wallet:s.Wallet,Kind:s.Kind,Epoch:s.Epoch,KeySession:s.KeySession,Expires:s.Expires,Digest:s.Digest,Intent:s.Intent,Peers:peers});
  if (!equalBytes(sha256(utf8('nu54/p03/ceremony/v1\0'+canonical)),octets(op.fingerprint))) throw new Error('P03_FINGERPRINT');
  if(op.state==='refused' && (s.Kind!=='sign' || op.acknowledged!==0 ||
      op.signature!==undefined || op.recoveryId!==undefined)) throw new Error('P03_REFUSAL');
  if(op.state==='denied' || op.refusal || op.reviewAvailable || op.successorRequest) {
    const receipt=op.refusal, d=receipt?.decision;
    if(op.state!=='denied' || s.Kind!=='refresh' || op.signature || !receipt || !d ||
          !uuid.test(d.subject) || subject!==undefined && d.subject!==subject ||
          d.wallet!==s.Wallet || d.session!==s.ID || d.epoch!==s.Epoch ||
          !uuid.test(d.vote) || !Number.isSafeInteger(d.issuedAt) || d.issuedAt<=0 ||
          !Number.isFinite(Date.parse(receipt.recordedAt)) || !equalBytes(octets(d.fingerprint),octets(op.fingerprint)) ||
          (op.successorRequest!==undefined && !uuid.test(op.successorRequest))) throw new Error('P03_REFUSAL');
    const authority=fromBase64(receipt.authority), signature=fromBase64(receipt.signature);
    const canonicalRefusal=JSON.stringify({subject:d.subject,wallet:d.wallet,session:d.session,fingerprint:d.fingerprint,
      epoch:d.epoch,vote:d.vote,issuedAt:d.issuedAt});
    const message=utf8('nu54/p03/recovery-refusal/v1\0wallet:recovery-refusal\0refresh\0'+canonicalRefusal);
    if(authority.length!==32 || signature.length!==64 || !ed25519.verify(signature,message,authority,{zip215:false})) {
      throw new Error('P03_REFUSAL');
    }
  }
  return op;
}

export function signedTransfer(op: CloudOperation, pending: PendingCloud, account: CloudAccount): string {
  checkOperation(op,pending);
  if (!pending.transfer || !op.signature || (op.recoveryId!==0 && op.recoveryId!==1) || !account.publicKey ||
      op.state!=='succeeded' || account.state!=='active' || op.session.Wallet!==account.id || op.session.Epoch!==account.epoch) {
    throw new Error('P03_RESULT');
  }
  const {fields,digest,type} = transferData(pending.transfer);
  const market=pending.transfer.market;
  if(type==='legacy' && (!market || account.address!==market.user ||
      account.profile!==cloudMarketProfile+':'+market.deploymentHash.slice(2))) throw new Error('P03_MARKET_WALLET');
  const signature = secp256k1.Signature.fromDER(fromBase64(op.signature)).addRecoveryBit(op.recoveryId);
  const recovered = signature.recoverPublicKey(digest).toRawBytes(false);
  if (signature.hasHighS() || !equalBytes(recovered,fromBase64(account.publicKey)) ||
      !secp256k1.verify(signature.toCompactRawBytes(),digest,recovered,{prehash:false,lowS:true})) throw new Error('P03_SIGNATURE');
  const parity=BigInt(op.recoveryId)+(type==='legacy'?35n+2n*8283n:0n);
  const signed=rlpList([...fields,...[parity,signature.r,signature.s].map(n=>rlpBytes(bigintBytes(n)))]);
  return bytesToHex(type==='legacy'?signed:concat(Uint8Array.of(2),signed),true);
}

export function verifyMarketSigned(raw:Uint8Array,t:CloudTransfer) {
  const parsed=marketTransferData(t);
  function prefix(at:number,list:boolean):{from:number;end:number} {
    const tag=raw[at],base=list?192:128;
    if (!list && tag<128) return {from:at,end:at+1};
    if (tag<base || (list?false:tag>=192)) throw new Error('MARKET_RLP');
    if (tag<base+56) return {from:at+1,end:at+1+tag-base};
    const size=tag-base-55;
    if (size<1 || size>2 || at+1+size>raw.length || raw[at+1]===0) throw new Error('MARKET_RLP');
    const from=at+1+size;
    const length=raw.subarray(at+1,from).reduce((n,x)=>n*256+x,0);
    if(length<56) throw new Error('MARKET_RLP');
    return {from,end:from+length};
  }
  if (raw.length<60 || raw.length>600) throw new Error('MARKET_RLP');
  const list=prefix(0,true);
  if(list.end!==raw.length) throw new Error('MARKET_RLP');
  let offset=list.from;
  const fields:Uint8Array[]=[];
  const values:Uint8Array[]=[];
  for(let index=0;index<9;index++) {
    if(offset>=raw.length) throw new Error('MARKET_RLP');
    const field=prefix(offset,false);
    if(field.end>raw.length) throw new Error('MARKET_RLP');
    const value=raw.slice(field.from,field.end);
    if(!equalBytes(rlpBytes(value),raw.slice(offset,field.end))) throw new Error('MARKET_RLP');
    fields.push(raw.slice(offset,field.end));
    values.push(value);
    offset=field.end;
  }
  if(offset!==raw.length || parsed.fields.some((field,i)=>!equalBytes(field,fields[i]))) throw new Error('MARKET_SIGNED_TERMS');
  const integer=(value:Uint8Array)=>value.reduce((n,x)=>n*256n+BigInt(x),0n);
  const v=integer(values[6])-35n-2n*8283n;
  if ((v!==0n && v!==1n) || values.slice(6).some(value=>!value.length || value.length>32 || value[0]===0)) throw new Error('MARKET_SIGNATURE');
  const signature=new secp256k1.Signature(integer(values[7]),integer(values[8])).addRecoveryBit(Number(v));
  const sender=bytesToHex(keccak_256(signature.recoverPublicKey(parsed.digest).toRawBytes(false).slice(1)).slice(12),true);
  if (signature.hasHighS() || sender!==t.market?.user) throw new Error('MARKET_SIGNATURE');
  return bytesToHex(raw,true);
}

export class CloudHttpError extends Error {
  constructor(readonly status: number) { super('P03_HTTP'); }
}

export class CloudClient {
  constructor(private readonly profile: CloudProfile, private readonly random: (size: number)=>Promise<Uint8Array>,
    private readonly open: (url: string)=>Promise<unknown>,
    private readonly request: (url:string,method:string,body:string,token:string)=>Promise<string>) {}

  private async json<T>(url: string, method: string, body?: unknown, token?: string): Promise<T> {
    const response = JSON.parse(await this.request(url,method,body===undefined?'':JSON.stringify(body),token??'')) as {status:number;body:string};
    if (!Number.isSafeInteger(response.status) || response.status<200 || response.status>=300) throw new CloudHttpError(response.status);
    if (response.status===204) return undefined as T;
    if (typeof response.body!=='string' || response.body.length>16384) throw new Error('P03_RESPONSE');
    return JSON.parse(response.body) as T;
  }

  private async start(origin: string, payload: unknown, onboarding?: {network: CloudNetwork; provider: CloudProvider}) {
    const verifier = base64url(await this.random(32));
    const result = await this.json<{deviceCode:string; verificationURL:string; expiresIn:number; onboarding?:typeof onboarding}>(origin+'/p03/device/start','POST',{
      request:payload, codeChallenge:base64url(sha256(utf8(verifier))), codeChallengeMethod:'S256',
    });
    const uri = new URL(result.verificationURL);
    if (uri.origin!==origin || uri.protocol!=='https:' || uri.username || uri.password || uri.hash || !opaque.test(result.deviceCode) ||
        !Number.isInteger(result.expiresIn) || result.expiresIn<15 || result.expiresIn>300) throw new Error('P03_DEVICE');
    if (onboarding && (result.onboarding?.network!==onboarding.network ||
        result.onboarding.provider?.id!==onboarding.provider.id ||
        result.onboarding.provider?.name!==onboarding.provider.name ||
        result.onboarding.provider?.issuer!==onboarding.provider.issuer ||
        result.verificationURL!==origin+'/p03/device/verify?code='+result.deviceCode)) throw new Error('P03_ONBOARDING');
    await this.open(uri.toString());
    return {origin,deviceCode:result.deviceCode,verifier,expiresAt:Date.now()+result.expiresIn*1000};
  }

  async login(provider?: CloudProvider) {
    const configured=this.profile.onboarding;
    if (!configured) return this.start(this.profile.identity,{action:'login',audience:'nu54-p03-wallet'});
    const registered=configured.providers.find(p=>p.id===provider?.id);
    if (!registered) throw new Error('P03_PROVIDER');
    const authority=await this.json<{network:CloudNetwork;identity:string}>(this.profile.api+'/v1/onboarding','GET');
    if (authority.network!==configured.network || authority.identity!==this.profile.identity) throw new Error('P03_ONBOARDING');
    const onboarding={network:configured.network,provider:registered};
    return this.start(this.profile.identity,{action:'login',audience:'nu54-p03-wallet',onboarding},onboarding);
  }

  authorize(op: CloudOperation,pending: PendingCloud,owner?: string) {
    checkOperation(op,pending);
    if (op.state!=='reserved' || Date.now()<Date.parse(op.notBefore) || Date.now()>=op.session.Expires*1000) throw new Error('P03_APPROVAL');
    return this.start(op.session.Kind==='refresh' ? this.profile.recovery : this.profile.approval,{
      action:op.session.Kind,owner,session:op.session,fingerprint:op.fingerprint,transfer:pending.transfer,
    });
  }

  closeAuthorization(account: CloudAccount,owner?: string,request?:string) {
    const close = {Wallet:account.id,Epoch:account.epoch,...(request?{Request:request}:{})};
    const fingerprint = Array.from(sha256(utf8(JSON.stringify(close))));
    return this.start(this.profile.approval,{action:'close',owner,close,fingerprint,confirmAccessEnds:true});
  }

  poll(flow: CloudFlow) {
    if (Date.now()>=flow.expiresAt || ![this.profile.identity,this.profile.approval,this.profile.recovery].includes(flow.origin)) throw new Error('P03_FLOW_EXPIRED');
    return this.json<{state:'pending'|'denied'|'ready'|'resolved';token?:string;enrollmentToken?:string;proof?:string}>(flow.origin+'/p03/device/poll','POST',{
      deviceCode:flow.deviceCode,codeVerifier:flow.verifier,
    });
  }

  roster(token:string,csr:string) {
    return this.json<{state:'pending'|'ready';roster?:unknown}>(this.profile.api+'/v1/mobile/roster','POST',{csr},token);
  }

  link(token: string, enrollmentToken: string) {
    return this.json<void>(this.profile.api+'/v1/credentials','POST',{enrollmentToken},token);
  }
  enroll(token: string) { return this.json<void>(this.profile.api+'/v1/enrollment','POST',undefined,token); }
  async subject(token:string) {
    const result=await this.json<{subject:string;expires:number;linked?:boolean}>(this.profile.api+'/v1/session','GET',undefined,token);
    if (typeof result.subject!=='string'||!uuid.test(result.subject)) throw new Error('P03_SUBJECT');
    if (!Number.isSafeInteger(result.expires) || result.expires*1000<=Date.now()) throw new CloudHttpError(401);
    if (result.linked!==undefined && typeof result.linked!=='boolean') throw new Error('P03_SUBJECT');
    return result;
  }
  async account(token: string) {
    const account=await this.json<CloudAccount>(this.profile.api+'/v1/wallet','GET',undefined,token);
    if (!uuid.test(account.id) || !Number.isSafeInteger(account.epoch) || account.epoch<0 ||
        !['creating','active','blocked','failed','closed'].includes(account.state)) throw new Error('P03_ACCOUNT');
    const network=this.profile.onboarding?.network;
    if ((network===profileName && account.profile!==network) ||
        (network===cloudMarketProfile && !/^isolated-market-8283-v1:[0-9a-f]{64}$/.test(account.profile??''))) {
      throw new Error('P03_ACCOUNT_PROFILE');
    }
    if (account.state==='active') {
      const key=fromBase64(account.publicKey??'');
      if (key.length!==65 || key[0]!==4 ||
          account.address!==bytesToHex(keccak_256(key.subarray(1)).subarray(12),true)) throw new Error('P03_ACCOUNT_KEY');
      secp256k1.ProjectivePoint.fromHex(key);
    } else if (account.address) throw new Error('P03_ACCOUNT');
    return account;
  }
  reserve(token: string, pending: PendingCloud, subject?:string) {
    const url=this.profile.api+'/v1/operations';
    if(pending.reviewOf!==undefined) {
      if(!uuid.test(pending.reviewOf)||pending.kind!=='refresh'||pending.reviewOf===pending.id) throw new Error('P03_REVIEW');
      return this.json<CloudOperation>(url+'/'+pending.reviewOf+'/review','POST',{successor:pending.id},token)
        .then(op=>checkOperation(op,pending,subject));
    }
    return this.json<CloudOperation>(url,'POST',pending,token).then(op=>checkOperation(op,pending,subject));
  }
  lookup(token: string,pending: PendingCloud,subject?:string) {
    return this.json<CloudOperation>(this.profile.api+'/v1/operations/'+pending.id,'GET',undefined,token).then(op=>checkOperation(op,pending,subject));
  }
  async recoveryHistory(token:string) {
    const history=await this.json<RecoveryHistoryItem[]>(this.profile.api+'/v1/recovery/history','GET',undefined,token);
    if(!Array.isArray(history)||history.length>20||history.some(item=>!uuid.test(item?.requestId)||!Number.isFinite(Date.parse(item.recordedAt)))) {
      throw new Error('P03_RECOVERY_HISTORY');
    }
    return history;
  }
  approve(token: string,pending: PendingCloud,proof: string) {
    return this.json<CloudOperation>(this.profile.api+'/v1/operations/'+pending.id+'/approval','PUT',{proof},token).then(op=>checkOperation(op,pending));
  }
  credentials(token:string) { return this.json<CloudCredential[]>(this.profile.api+'/v1/credentials','GET',undefined,token); }
  unlink(token: string,id: string) {
    if (!uuid.test(id)) throw new Error('P03_CREDENTIAL');
    return this.json<void>(this.profile.api+'/v1/credentials/'+id,'DELETE',undefined,token);
  }
  async close(token:string,account:CloudAccount,proof:string,request:string) {
    const receipt=await this.json<ClosureReceipt>(this.profile.api+'/v1/close','POST',{
      wallet:account.id,epoch:account.epoch,request,proof,confirmAccessEnds:true,
    },token);
    return checkClosure(receipt,account,request);
  }
  async closure(token:string,account:CloudAccount,request?:string) {
    const receipt=await this.json<ClosureReceipt>(this.profile.api+'/v1/close'+(request?'/'+request:''),'GET',undefined,token);
    return checkClosure(receipt,account,request);
  }
}

export type CloudFlow = Awaited<ReturnType<CloudClient['login']>>;
