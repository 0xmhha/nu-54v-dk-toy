import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha256';
import {concat,fromBase64,hexToBytes} from '@nu54/protocol';
import {readCommissionedChains,type CommissionedChain} from './commissionedChainProfiles.ts';
import {evmProfiles,maxFiniteAllowance,type DappAllowancePolicy,type EvmChain} from './evmTransfer.ts';

export type EvmAsset = {address:string;symbol:string;decimals:number};
export type EvmNetwork = {chain:EvmChain;genesis:string;rpc:string;assets:EvmAsset[];feeLimit:string};
export type EvmRegistry = {device:string;sender:string;generation:number;leaseRevision:string;expires:number;networks:EvmNetwork[];chainNetworks?:CommissionedChain[];dapp?:{origin:string;chain:EvmChain;allowance?:DappAllowancePolicy};walletConnect?:{relay:string;projectId?:string}};
export type EvmRegistryEntry = {endpoint:string;token:string;payload:string;proof:string;anchor?:string};
const fingerprint=/^[a-f0-9]{64}$/;
const address=/^0x[a-f0-9]{40}$/;
const hexHash=/^0x[a-f0-9]{64}$/;
const numberText=/^[1-9][0-9]{0,19}$/;

export function isHttpsEndpoint(text:unknown):text is string {
  if (typeof text !== 'string' || text.length > 320) return false;
  try {
    const url=new URL(text);
    return url.protocol==='https:' && Boolean(url.hostname) && !url.password && !url.username && !url.hash;
  } catch {return false;}
}

export function readRegistryEntry(candidate:unknown) {
  if (!candidate || typeof candidate !== 'object') throw new Error('EVM_REGISTRY');
  const entry=candidate as EvmRegistryEntry;
  if (!isHttpsEndpoint(entry.endpoint) || !fingerprint.test(entry.token) ||
      typeof entry.payload !== 'string' || entry.payload.length>4096 ||
      typeof entry.proof !== 'string' || !/^[a-f0-9]{128}$/.test(entry.proof) ||
      (Object.keys(entry).length!==4 && (Object.keys(entry).length!==5 || typeof entry.anchor!=='string' || !/^04[a-f0-9]{128}$/.test(entry.anchor)))) throw new Error('EVM_REGISTRY');
  const bytes=fromBase64(entry.payload);
  if (!bytes.length || bytes.some(x=>x<0x20||x>0x7e)) throw new Error('EVM_REGISTRY');
  const result=JSON.parse(String.fromCharCode(...bytes)) as EvmRegistry;
  if (!result || !fingerprint.test(result.device) || !address.test(result.sender) ||
      !Number.isSafeInteger(result.generation) || result.generation < 1 ||
      typeof result.leaseRevision!=='string' || !numberText.test(result.leaseRevision) || BigInt(result.leaseRevision)>=2n**64n ||
      !Number.isSafeInteger(result.expires) || !Array.isArray(result.networks) ||
      (result.networks.length<1 && result.chainNetworks===undefined) || result.networks.length>3 || new Set(result.networks.map(x=>x?.chain)).size!==result.networks.length)
    throw new Error('EVM_REGISTRY');
  if (result.chainNetworks!==undefined) readCommissionedChains(result.chainNetworks);
  if (result.dapp && (!isHttpsEndpoint(result.dapp.origin) || new URL(result.dapp.origin).origin!==result.dapp.origin ||
      !result.networks.some(network=>network.chain===result.dapp?.chain))) throw new Error('EVM_REGISTRY_DAPP');
  if (result.walletConnect) {
    const {relay,projectId}=result.walletConnect;
    if (typeof relay!=='string' || !/^wss:\/\/[a-z0-9.-]+(?::[0-9]{1,5})?(?:\/[a-zA-Z0-9/_-]*)?$/.test(relay) ||
        projectId!==undefined && !/^[a-f0-9]{32}$/.test(projectId)) throw new Error('EVM_REGISTRY_RELAY');
  }
  for (const network of result.networks) {
    if (!network || !Object.hasOwn(evmProfiles,network.chain) ||
        !hexHash.test(network.genesis) || network.genesis==='0x'+'0'.repeat(64) || !isHttpsEndpoint(network.rpc) ||
        typeof network.feeLimit!=='string' || !numberText.test(network.feeLimit) || BigInt(network.feeLimit)>=2n**64n ||
        !Array.isArray(network.assets) || network.assets.length<1 || network.assets.length>8 ||
        network.assets.some(asset=>!asset || !address.test(asset.address) || !/^[A-Za-z0-9-]{1,12}$/.test(asset.symbol) ||
          !Number.isInteger(asset.decimals) || asset.decimals<0 || asset.decimals>24) ||
        new Set(network.assets.map(asset=>asset.address)).size!==network.assets.length) throw new Error('EVM_NETWORK_PROFILE');
  }
  if (result.dapp?.allowance!==undefined) {
    const policy=result.dapp.allowance;
    const zero='0x'+'0'.repeat(40);
    if (!policy || Object.keys(policy).sort().join(',')!=='maximum,spender,token' ||
        !address.test(policy.spender) || policy.spender===zero || policy.token===zero ||
        typeof policy.maximum!=='string' || !/^[1-9][0-9]{0,77}$/.test(policy.maximum) ||
        BigInt(policy.maximum)>maxFiniteAllowance ||
        !result.networks.find(network=>network.chain===result.dapp?.chain)?.assets.some(asset=>asset.address===policy.token))
      throw new Error('EVM_REGISTRY_ALLOWANCE');
  }
  return {entry,bytes,registry:result};
}

export function authenticateRegistry(candidate:unknown, anchor:Uint8Array, revision:bigint, scope:{device:string;address:string;generation:number},now:number) {
  const {entry,bytes,registry}=readRegistryEntry(candidate);
  if (registry.device!==scope.device || registry.sender!==scope.address || registry.generation!==scope.generation ||
      registry.leaseRevision!==revision.toString() || registry.expires<=now || registry.expires>now+86400 ||
      !p256.verify(hexToBytes(entry.proof),sha256(concat(Uint8Array.of(0x4e,0x35,0x34,0x47),bytes)),anchor,{lowS:false}))
    throw new Error('EVM_REGISTRY_AUTHORITY');
  return {entry,registry};
}

export function parseHumanAmount(text:string,decimals:number) {
  if (!/^(?:0|[1-9][0-9]{0,77})(?:\.[0-9]{1,24})?$/.test(text)) throw new Error('EVM_AMOUNT');
  const [whole,fraction='']=text.split('.');
  if (fraction.length>decimals) throw new Error('EVM_AMOUNT_PRECISION');
  const result=BigInt(whole+fraction.padEnd(decimals,'0'));
  if (result<=0n || result>=2n**256n) throw new Error('EVM_AMOUNT');
  return result;
}


export function formatEvmTokenAmount(amount:bigint,asset:EvmAsset) {
  const scale=10n**BigInt(asset.decimals);
  const fraction=asset.decimals ? '.'+(amount%scale).toString().padStart(asset.decimals,'0') : '';
  return (amount/scale).toString()+fraction+' '+asset.symbol;
}
