import {keccak_256} from '@noble/hashes/sha3';
import {concat,hexToBytes} from '@nu54/protocol';
import {maxFiniteAllowance,normalizeEvmAddress,type DappAllowancePolicy} from '../evmTransfer.ts';

export type DappMethod='eth_accounts'|'eth_chainId'|'personal_sign'|'eth_sendTransaction';
export type DappScope={board:string;account:string;generation:number;chain:number;genesis:string};
export type DappChannel={transport:'browser'|'walletconnect';session:string;origin:string;topLevel:boolean};
export type DappGrant={channel:DappChannel;scope:DappScope;methods:DappMethod[];expires:number;revoked:boolean};
export type LoginChallenge={nonce:string;origin:string;account:string;chain:number;expires:number};
const supported=new Set<DappMethod>(['eth_accounts','eth_chainId','personal_sign','eth_sendTransaction']);
const fingerprint=/^[a-f0-9]{64}$/;
const address=/^0x[a-f0-9]{40}$/;

export function httpsOrigin(input:string) {
  if (input.length>320 || !/^[\x21-\x7e]+$/.test(input)) throw new Error('DAPP_ORIGIN');
  const parsed=new URL(input);
  if (parsed.protocol!=='https:' || !parsed.hostname || parsed.username || parsed.password) throw new Error('DAPP_ORIGIN');
  return parsed.origin;
}

function channelAllowed(channel:DappChannel) {
  return channel.topLevel && fingerprint.test(channel.session) && httpsOrigin(channel.origin)===channel.origin &&
    (channel.transport==='browser'||channel.transport==='walletconnect');
}

export function issueDappGrant(channel:DappChannel,scope:DappScope,methods:DappMethod[],now:number,expires:number):DappGrant {
  if (!channelAllowed(channel) || !fingerprint.test(scope.board) ||
      !address.test(scope.account) || !Number.isSafeInteger(scope.generation) || scope.generation<1 ||
      ![11155111,84532,8283].includes(scope.chain) || !/^0x[a-f0-9]{64}$/.test(scope.genesis) ||
      !Number.isSafeInteger(now) || !Number.isSafeInteger(expires) || expires<=now || expires>now+3600 ||
      !methods.length || new Set(methods).size!==methods.length || methods.some(method=>!supported.has(method))) throw new Error('DAPP_GRANT');
  return {channel:{...channel},scope:{...scope},methods:[...methods],expires,revoked:false};
}

function activeScope(grant:DappGrant,channel:DappChannel,scope:DappScope,now:number) {
  return !grant.revoked && grant.expires>now && channel.origin===grant.channel.origin &&
    channel.session===grant.channel.session && channel.transport===grant.channel.transport &&
    scope.board===grant.scope.board && scope.account===grant.scope.account && scope.generation===grant.scope.generation &&
    scope.genesis===grant.scope.genesis && scope.chain===grant.scope.chain && channelAllowed(channel);
}

export type AdvancedRequestRefusal='nft'|'batch';
export function refusedAdvancedRequest(grant:DappGrant,channel:DappChannel,scope:DappScope,
  request:{method:string;params:unknown},now:number):AdvancedRequestRefusal|undefined {
  if (!activeScope(grant,channel,scope,now) || !Array.isArray(request.params) || request.params.length!==1)
    return undefined;
  const candidate=request.params[0] as {from?:unknown;chainId?:unknown;calls?:unknown;data?:unknown}|null;
  if (!candidate || typeof candidate!=='object' ||
      candidate.from!==undefined && (typeof candidate.from!=='string' || candidate.from.toLowerCase()!==scope.account) ||
      candidate.chainId!==undefined && candidate.chainId!=='0x'+scope.chain.toString(16)) return undefined;
  if (request.method==='wallet_sendCalls')
    return Array.isArray(candidate.calls) && candidate.calls.length>0 ? 'batch' : undefined;
  if (request.method!=='eth_sendTransaction') return undefined;
  const data = candidate.data;
  if (typeof data!=='string' || data.length>4096 || !/^0x(?:[0-9a-fA-F]{2}){4,}$/.test(data)) return undefined;
  const selector=data.slice(2,10).toLowerCase();
  if (selector==='42842e0e' || selector==='b88d4fde') return 'nft';
  if (selector==='ac9650d8') return 'batch';
  return undefined;
}

export function authorizeDappRequest(grant:DappGrant,channel:DappChannel,scope:DappScope,request:{method:string;params:unknown},now:number,policy?:DappAllowancePolicy) {
  if (!activeScope(grant,channel,scope,now) ||
      !Array.isArray(request.params) || !grant.methods.includes(request.method as DappMethod)) throw new Error('DAPP_REQUEST_SCOPE');
  const params=request.params as unknown[];
  if (request.method==='eth_accounts' && !params.length) return {kind:'accounts',accounts:[scope.account]} as const;
  if (request.method==='eth_chainId' && !params.length) return {kind:'chain',chain:'0x'+scope.chain.toString(16)} as const;
  if (request.method==='personal_sign' && params.length===2 && typeof params[0]==='string' &&
      params[0].length<=1200 && typeof params[1]==='string' && normalizeEvmAddress(params[1])===scope.account)
    return {kind:'login',message:params[0].startsWith('0x') && /^0x(?:[a-fA-F0-9]{2})+$/.test(params[0])
      ? String.fromCharCode(...hexToBytes(params[0].slice(2))) : params[0]} as const;
  if (request.method==='eth_sendTransaction' && params.length===1) {
    const transaction=params[0] as Record<string,unknown>|null;
    if (transaction && Object.keys(transaction).sort().join(',')==='data,from,to,value' && policy &&
        typeof transaction.from==='string' && normalizeEvmAddress(transaction.from)===scope.account &&
        typeof transaction.to==='string' && normalizeEvmAddress(transaction.to)===policy.token && transaction.value==='0x0' &&
        typeof transaction.data==='string' && /^0x095ea7b30{24}[0-9a-f]{104}$/.test(transaction.data)) {
      const spender='0x'+transaction.data.slice(34,74);
      const amount=BigInt('0x'+transaction.data.slice(74));
      if (spender===policy.spender && amount>0n && amount<=BigInt(policy.maximum) && amount<=maxFiniteAllowance)
        return {kind:'allowance',token:policy.token,spender,amount} as const;
    }
    if (!transaction || Object.keys(transaction).sort().join(',')!=='from,to,value' ||
        typeof transaction.from!=='string' || typeof transaction.to!=='string' ||
        normalizeEvmAddress(transaction.from)!==scope.account || typeof transaction.value!=='string' ||
        !/^0x[a-f1-9][a-f0-9]{0,63}$/.test(transaction.value)) throw new Error('DAPP_UNSUPPORTED_SPEND');
    return {kind:'spend',recipient:normalizeEvmAddress(transaction.to),value:BigInt(transaction.value)} as const;
  }
  throw new Error('DAPP_METHOD_PARAMS');
}

function ascii(input:string) {return Uint8Array.from(input,ch=>ch.charCodeAt(0));}

function parseTime(value:string) {
  if (!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.000)?Z$/.test(value)) throw new Error('SIWE_TIME');
  const milliseconds=Date.parse(value);
  if (!Number.isFinite(milliseconds) || new Date(milliseconds).toISOString()!==(value.endsWith('.000Z') ? value : value.slice(0,-1)+'.000Z'))
    throw new Error('SIWE_TIME');
  return milliseconds/1000;
}

export function parseSiwe(message:string,trustedOrigin:string,scope:DappScope,challenge:LoginChallenge,now:number) {
  if (message.length>600 || !/^[\x20-\x7e\n]+$/.test(message) || httpsOrigin(trustedOrigin)!==trustedOrigin ||
      challenge.origin!==trustedOrigin || challenge.chain!==scope.chain || challenge.account!==scope.account ||
      challenge.expires<=now) throw new Error('SIWE_SCOPE');
  const fields=message.split('\n');
  const host=new URL(trustedOrigin).host;
  if (fields.length!==10 || fields[0]!==host+' wants you to sign in with your Ethereum account:' ||
      normalizeEvmAddress(fields[1])!==scope.account || fields[2] || fields[3] || !fields[4].startsWith('URI: ') ||
      httpsOrigin(fields[4].slice(5))!==trustedOrigin || fields[5]!=='Version: 1' ||
      fields[6]!=='Chain ID: '+scope.chain || !/^[A-Za-z0-9]{8,32}$/.test(challenge.nonce) ||
      fields[7]!=='Nonce: '+challenge.nonce || !fields[8].startsWith('Issued At: ') || !fields[9].startsWith('Expiration Time: '))
    throw new Error('SIWE_FIELDS');
  const issued=parseTime(fields[8].slice(11)), expiry=parseTime(fields[9].slice(17));
  if (issued>now || issued<now-120 || expiry<=now || expiry>issued+120 || expiry!==challenge.expires) throw new Error('SIWE_TIME');
  return {nonce:challenge.nonce,digest:keccak_256(concat(ascii('\x19Ethereum Signed Message:\n'+message.length),ascii(message)))};
}
