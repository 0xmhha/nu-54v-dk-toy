import {bytesToHex,hexToBytes} from '@nu54/protocol';
import {authorizeDappRequest,httpsOrigin,issueDappGrant,refusedAdvancedRequest,
  type AdvancedRequestRefusal,type DappMethod,type DappScope} from './gate.ts';
import {negotiate,parsePairing,type Pairing,type ProposalScope,type VerifiedPeer} from './walletConnectPolicy.ts';
import {open,seal,sessionMaterial,topicOf} from './walletConnectCrypto.ts';
import type {DappAllowancePolicy} from '../evmTransfer.ts';
import type {Relay,RelayMessage} from './walletConnectRelay.ts';

type Request={id:number;method:string;params?:Record<string,unknown>};
export const viewAllowanceConnection=Symbol('viewAllowanceConnection');
export type ReturnedLogin={signature:string;nonce:string};
type Authorized=ReturnType<typeof authorizeDappRequest>;
export type ConnectCheckpoint={topic:string;key:string;scope:DappScope;proposal:ProposalScope;expires:number;lastId:number};
export type SessionOptions={
  rtc:Relay;
  scope:DappScope;
  clock:()=>number;
  random:(length:number)=>Promise<Uint8Array>;
  save:(checkpoint:ConnectCheckpoint|undefined,topic?:string)=>Promise<void>;
  approve:(proposal:ProposalScope,signal:AbortSignal)=>Promise<boolean>;
  trust:(pairing:Pairing,proposal:unknown)=>Promise<VerifiedPeer|undefined>;
  request:(action:Authorized,proposal:ProposalScope,signal:AbortSignal,sessionTopic:string,expiry:number)=>Promise<string|ReturnedLogin|typeof viewAllowanceConnection>;
  allowSpend:boolean;
  allowancePolicy?:DappAllowancePolicy;
  allowanceSubmitted?:()=>void;
  viewAllowanceConnection?:(origin:string)=>void;
  loginReturned?:(nonce:string)=>void;
  pairingRefused?:()=>void;
  expired?:()=>void;
  advancedRefused?:(reason:AdvancedRequestRefusal)=>void;
};

const word=/^[a-f0-9]{64}$/;
const methodCodes:Record<string,number>={wc_sessionRequest:1109,wc_sessionPing:1115,wc_sessionDelete:1113,
  wc_sessionUpdate:1105,wc_sessionExtend:1107,wc_sessionEvent:1111};

export function validateConnectCheckpoint(input:unknown,scope:DappScope,now:number):ConnectCheckpoint {
  if (!input || typeof input!=='object') throw new Error('WC_CHECKPOINT');
  const saved=input as ConnectCheckpoint;
  if (!word.test(saved.topic) || !word.test(saved.key) || topicOf(hexToBytes(saved.key))!==saved.topic ||
      !saved.scope || saved.scope.chain!==scope.chain || saved.scope.genesis!==scope.genesis ||
      saved.scope.account!==scope.account || saved.scope.board!==scope.board || saved.scope.generation!==scope.generation ||
      !Number.isSafeInteger(saved.lastId) || saved.lastId<0 || !Number.isSafeInteger(saved.expires) ||
      saved.expires<=now || saved.expires>now+600 || !saved.proposal || !word.test(saved.proposal.peerKey) || !word.test(saved.proposal.pairingTopic) || saved.proposal.pairingTopic===saved.topic ||
      saved.proposal.chain!=='eip155:'+scope.chain || saved.proposal.account!==saved.proposal.chain+':'+scope.account ||
      httpsOrigin(saved.proposal.origin)!==saved.proposal.origin || typeof saved.proposal.verified!=='boolean' ||
      !Array.isArray(saved.proposal.methods) || !saved.proposal.methods.length ||
      saved.proposal.methods.some(method=>!['eth_accounts','eth_chainId','personal_sign','eth_sendTransaction'].includes(method) ||
        !saved.proposal.verified && ['personal_sign','eth_sendTransaction'].includes(method)) ||
      !Array.isArray(saved.proposal.events) || saved.proposal.events.some(event=>!['accountsChanged','chainChanged'].includes(event)))
    throw new Error('WC_CHECKPOINT');
  return saved;
}

export class WalletConnectSession {
  private pairing?:Pairing & {allowSpend:boolean};
  private readonly decidedPairings=new Map<string,number>();
  private session?:ConnectCheckpoint;
  private settling?:number;
  private pending?:AbortController;
  private timer?:ReturnType<typeof setTimeout>;
  private active=true;
  private storageQueue=Promise.resolve();

  private readonly config:SessionOptions;

  constructor(config:SessionOptions) {
    this.config=config;
    config.rtc.listen(message=>{this.receive(message).catch(()=>{});});
  }

  async pair(uri:string,allowSpend=this.config.allowSpend) {
    if (!this.active || this.pairing || this.session) throw new Error('WC_ACTIVE');
    const now=this.config.clock();
    const pairing={...parsePairing(uri,now),allowSpend};
    for (const [topic,expiry] of this.decidedPairings) {
      if (expiry<=now) this.decidedPairings.delete(topic);
    }
    if (this.decidedPairings.has(pairing.topic) || this.decidedPairings.size>=8) throw new Error('WC_PAIRING_DECIDED');
    this.pairing=pairing;
    this.arm(pairing.expiry);
    try {await this.config.rtc.subscribe(pairing.topic);}
    catch (error) {
      if (this.pairing===pairing) this.releasePairing(pairing);
      throw error;
    }
  }

  async resume(checkpoint:unknown) {
    if (!this.active || this.pairing || this.session) throw new Error('WC_ACTIVE');
    this.session=validateConnectCheckpoint(checkpoint,this.config.scope,this.config.clock());
    this.arm(this.session.expires);
    await this.config.rtc.subscribe(this.session.topic);
  }

  private arm(expires:number) {
    clearTimeout(this.timer);
    this.timer=setTimeout(()=>{this.expire();},Math.max(0,(expires-this.config.clock())*1000));
  }

  private expire() {
    if (!this.active) return;
    const retired=this.forget();
    this.suspend();
    this.config.expired?.();
    retired.catch(()=>{});
  }

  private persist(checkpoint:ConnectCheckpoint|undefined,topic=checkpoint?.topic) {
    const write=this.storageQueue.catch(()=>{}).then(()=>{
      if (checkpoint && (!this.active || this.session?.topic!==checkpoint.topic)) throw new Error('WC_CLOSED');
      return this.config.save(checkpoint,topic);
    });
    this.storageQueue=write;
    return write;
  }

  private async publish(topic:string,key:Uint8Array,tag:number,payload:object,ttl=300,live=()=>true) {
    const nonce=await this.config.random(12);
    if (!live()) return;
    await this.config.rtc.publish(topic,seal(key,nonce,{jsonrpc:'2.0',...payload}),tag,ttl);
  }

  private async receive(event:RelayMessage) {
    if (!this.active) return;
    if (this.pairing?.topic===event.topic && event.tag===1100 && !this.pending && !this.session) {
      await this.propose(this.pairing,event.message);
      return;
    }
    const session=this.session;
    if (!session || session.topic!==event.topic) return;
    if (session.expires<=this.config.clock()) {
      this.expire();
      return;
    }
    const key=hexToBytes(session.key);
    const packet=open(key,event.message) as Request & {result?:unknown;jsonrpc:string};
    if (packet?.jsonrpc!=='2.0' || !Number.isSafeInteger(packet.id) || packet.id<=0) return;
    if (event.tag===1103 && packet.id===this.settling && packet.result===true) {
      this.settling=undefined;
      await this.persist({...session});
      return;
    }
    if (this.settling!==undefined) return;
    const tag=methodCodes[packet.method];
    if (!tag || event.tag!==tag-1) return;
    const sessionLive=()=>this.active && this.config.clock()<session.expires;
    const reply=(result:object,live=sessionLive)=>this.publish(session.topic,key,tag,{id:packet.id,...result},300,live);
    if (packet.method==='wc_sessionDelete') {
      await this.forget();
      try {await reply({result:true});} finally {this.suspend();}
      return;
    }
    if (packet.method==='wc_sessionPing') {await reply({result:true});return;}
    if (packet.method!=='wc_sessionRequest') {
      await reply({error:{code:5101,message:'Session permission changes require a new connection'}});
      return;
    }
    if (this.pending || packet.id<=session.lastId) {
      await reply({error:{code:5000,message:'Request already seen or an approval is pending'}});
      return;
    }
    const pending=new AbortController();
    this.pending=pending;
    let expiry=session.expires;
    const live=()=>sessionLive() && this.session===session && !pending.signal.aborted && this.config.clock()<expiry;
    try {
      const body=packet.params;
      const nested=body?.request as {method:string;params:unknown;expiry?:number}|undefined;
      const now=this.config.clock();
      if (!nested || body?.chainId!==session.proposal.chain ||
          nested.expiry!==undefined && (!Number.isSafeInteger(nested.expiry) || nested.expiry<=now || nested.expiry>now+604800))
        throw new Error('WC_REQUEST_SCOPE');
      expiry=Math.min(session.expires,nested.expiry??session.expires);
      const channel={transport:'walletconnect' as const,origin:session.proposal.origin,session:session.topic,topLevel:true};
      const grant=issueDappGrant(channel,session.scope,session.proposal.methods as DappMethod[],now,session.expires);
      const refused=refusedAdvancedRequest(grant,channel,this.config.scope,nested,now);
      if (refused) {
        this.config.advancedRefused?.(refused);
        await reply({error:{code:5000,message:'The wallet cannot verify this transaction method'}},live);
        return;
      }
      const action=authorizeDappRequest(grant,channel,this.config.scope,nested,now,
        session.proposal.verified ? this.config.allowancePolicy : undefined);
      session.lastId=packet.id;
      await this.persist({...session});
      if (!live()) return;
      const timeout=setTimeout(()=>pending.abort(),(expiry-this.config.clock())*1000);
      try {
        const result= action.kind==='accounts' ? action.accounts : action.kind==='chain' ? action.chain :
          await this.config.request(action,session.proposal,pending.signal,session.topic,expiry);
        if (live()) {
          if (result===viewAllowanceConnection) {
            const message=action.kind==='login' ? 'Login review ended. Send a new request to continue.' :
              'Allowance review ended. Send a new request to continue.';
            await reply({error:{code:5000,message}},live);
            if (live())
              this.config.viewAllowanceConnection?.(session.proposal.origin);
            return;
          }
          if (action.kind==='login' && typeof result==='object' && result!==null && 'signature' in result) {
            await reply({result:result.signature},live);
            if (live()) this.config.loginReturned?.(result.nonce);
          } else {
            await reply({result},live);
          }
          if (action.kind==='allowance' && live()) this.config.allowanceSubmitted?.();
        }
      } finally {clearTimeout(timeout);}
    } catch {
      if (live()) await reply({error:{code:5000,message:'Wallet refused this request'}},live);
    } finally {
      if (this.pending===pending) this.pending=undefined;
    }
  }

  private releasePairing(pairing:Pairing) {
    if (this.pairing!==pairing) return;
    this.pairing=undefined;
    clearTimeout(this.timer);
  }

  private async propose(pairing:Pairing & {allowSpend:boolean},message:string) {
    const key=hexToBytes(pairing.key);
    const packet=open(key,message) as Request & {jsonrpc:string};
    if (packet?.jsonrpc!=='2.0' || packet.method!=='wc_sessionPropose' || !Number.isSafeInteger(packet.id) || packet.id<=0) return;
    const pending=new AbortController();
    this.pending=pending;
    this.decidedPairings.set(pairing.topic,this.config.clock()+300);
    let checkpoint:ConnectCheckpoint|undefined;
    const current=()=>this.active && this.pairing===pairing && !pending.signal.aborted && this.config.clock()<pairing.expiry;
    try {
      const proof=await this.config.trust(pairing,packet.params);
      const proposal=negotiate(packet.params,pairing,this.config.scope,this.config.clock(),proof,pairing.allowSpend);
      if (!current() || !await this.config.approve(proposal,pending.signal) || !current() ||
          pairing.expiry<=this.config.clock()) throw new Error('WC_PROPOSAL_REFUSED');
      const material=sessionMaterial(await this.config.random(32),proposal.peerKey);
      const now=this.config.clock();
      const expires=Math.min(now+600,proposal.verified && proof ? proof.expires : now+600);
      if (!current() || pairing.expiry<=now || expires<=now) throw new Error('WC_CLOSED');
      checkpoint={
        topic:material.topic,key:bytesToHex(material.key,false),scope:{...this.config.scope},proposal,expires,lastId:0};
      const id=this.config.clock()*1000;
      this.session=checkpoint;
      this.settling=id;
      await this.config.rtc.subscribe(material.topic);
      if (!current()) throw new Error('WC_CLOSED');
      await this.publish(pairing.topic,key,1101,{id:packet.id,result:{relay:{protocol:'irn'},responderPublicKey:material.publicKey}},300,current);
      const settlementLive=()=>current() && this.config.clock()<expires;
      if (!settlementLive()) {if (this.session===checkpoint) this.expire();return;}
      await this.publish(material.topic,material.key,1102,{id,method:'wc_sessionSettle',params:{
        relay:{protocol:'irn'},controller:{publicKey:material.publicKey,metadata:{name:'NU54',description:'NU54 board wallet',url:'',icons:[]}},
        namespaces:{eip155:{chains:[proposal.chain],accounts:[proposal.account],methods:proposal.methods,events:proposal.events}},expiry:expires}},300,settlementLive);
      if (!settlementLive()) {if (this.session===checkpoint) this.expire();return;}
      this.releasePairing(pairing);
      this.arm(expires);
    } catch {
      if (!current()) return;
      if (checkpoint && this.session===checkpoint) {
        this.session=undefined;
        this.settling=undefined;
      }
      // The DApp can receive the refusal before the relay acknowledges its publish.
      // Detach the old attempt before announcing the final decision.
      this.releasePairing(pairing);
      if (this.pending===pending) this.pending=undefined;
      this.config.pairingRefused?.();
      await this.publish(pairing.topic,key,1120,{id:packet.id,error:{code:5000,message:'Session proposal refused'}});
    } finally {
      if (this.pending===pending) this.pending=undefined;
    }
  }

  private async forget() {
    const topic=this.session?.topic;
    this.pending?.abort();
    this.session=undefined;
    this.pairing=undefined;
    if (topic) await this.persist(undefined,topic);
  }

  suspend() {
    this.pending?.abort();
    this.active=false;
    clearTimeout(this.timer);
    this.config.rtc.close();
  }

  async disconnect() {
    const session=this.session;
    await this.forget();
    try {
      if (session && session.expires>this.config.clock())
        await this.publish(session.topic,hexToBytes(session.key),1112,{id:this.config.clock()*1000,method:'wc_sessionDelete',
          params:{code:6000,message:'Wallet disconnected'}},86400);
    } finally {this.suspend();}
  }
}
