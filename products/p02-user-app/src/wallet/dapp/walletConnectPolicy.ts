import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha256';
import {bytesToHex,hexToBytes} from '@nu54/protocol';
import {httpsOrigin,type DappMethod,type DappScope} from './gate.ts';

export type Pairing={topic:string;key:string;expiry:number};
export type VerifiedPeer={origin:string;topic:string;publicKey:string;expires:number};
export type ProposalScope={chain:string;account:string;origin:string;peerKey:string;pairingTopic:string;verified:boolean;methods:DappMethod[];events:string[]};
const bytes32=/^[0-9a-f]{64}$/;
const readable:DappMethod[]=['eth_accounts','eth_chainId'];
const events=['accountsChanged','chainChanged'];

export function parsePairing(uri:string,now:number):Pairing {
  const match=/^wc:([a-f0-9]{64})@2\?([^#\s]{1,1100})$/.exec(uri);
  if (!match) throw new Error('WC_URI');
  const parameters=new Map<string,string>();
  for (const item of match[2].split('&')) {
    const separator=item.indexOf('='), name=item.slice(0,separator);
    if (separator<1 || parameters.has(name) ||
        !['symKey','relay-protocol','methods','expiryTimestamp'].includes(name)) throw new Error('WC_URI_PARAMETERS');
    parameters.set(name,decodeURIComponent(item.slice(separator+1)));
  }
  const key=parameters.get('symKey')??'';
  const declared=parameters.get('expiryTimestamp');
  const expiry=declared===undefined ? now+300 : Number(declared);
  if (!bytes32.test(key) || /^0+$/.test(key) || bytesToHex(sha256(hexToBytes(key)),false)!==match[1] || parameters.get('relay-protocol')!=='irn' ||
      (declared!==undefined && !/^[1-9][0-9]{0,10}$/.test(declared)) || expiry<=now || expiry>now+300)
    throw new Error('WC_URI_SCOPE');
  const methods=parameters.get('methods');
  if (methods!==undefined && !/(?:^|\[|,)wc_sessionPropose(?:\]|,|$)/.test(methods)) throw new Error('WC_URI_METHODS');
  return {topic:match[1],key,expiry};
}

function record(value:unknown):Record<string,unknown> {
  if (!value || typeof value!=='object' || Array.isArray(value)) throw new Error('WC_RECORD');
  return value as Record<string,unknown>;
}
function strings(value:unknown):string[] {
  if (!Array.isArray(value) || value.length>32 || value.some(item=>typeof item!=='string' || item.length>72))
    throw new Error('WC_NAMESPACE');
  return [...new Set(value)];
}
export function verifyPeer(proof:VerifiedPeer, signature:string, authority:Uint8Array, now:number) {
  if (!bytes32.test(proof.topic) || !bytes32.test(proof.publicKey) || !/^[a-f0-9]{128}$/.test(signature) ||
      httpsOrigin(proof.origin)!==proof.origin || !Number.isSafeInteger(proof.expires) || proof.expires<=now ||
      proof.expires>now+300) throw new Error('WC_PEER_PROOF');
  const message='N54-walletconnect-v1\n'+proof.topic+'\n'+proof.publicKey+'\n'+proof.origin+'\n'+proof.expires;
  if (!p256.verify(hexToBytes(signature),sha256(Uint8Array.from(message,ch=>ch.charCodeAt(0))),authority,{lowS:false}))
    throw new Error('WC_PEER_PROOF');
  return Object.freeze({...proof});
}

export function negotiate(input:unknown,pairing:Pairing,scope:DappScope,now:number,
  proof?:VerifiedPeer,allowSpend=false):ProposalScope {
  const proposal=record(input), proposer=record(proposal.proposer), metadata=record(proposer.metadata);
  if (typeof proposer.publicKey!=='string' || !bytes32.test(proposer.publicKey) || /^0+$/.test(proposer.publicKey) ||
      typeof metadata.url!=='string' || pairing.expiry<=now || !Array.isArray(proposal.relays) ||
      !proposal.relays.some(item=>record(item).protocol==='irn' && record(item).data===undefined))
    throw new Error('WC_PROPOSAL_SCOPE');
  const origin=httpsOrigin(metadata.url), chain='eip155:'+scope.chain;
  const verified=!!proof && proof.expires>now && proof.topic===pairing.topic &&
    proof.origin===origin && proof.publicKey===proposer.publicKey;
  const permitted:DappMethod[]=[...readable];
  if (verified) permitted.push('personal_sign');
  if (verified && allowSpend) permitted.push('eth_sendTransaction');
  const methods=new Set<DappMethod>(), approvedEvents=new Set<string>();
  for (const required of [true,false]) {
    const raw=required ? proposal.requiredNamespaces : proposal.optionalNamespaces;
    if (raw===undefined && !required) continue;
    for (const [namespace,configuration] of Object.entries(record(raw))) {
      if (namespace!=='eip155' && namespace!==chain) {
        if (required) throw new Error('WC_NAMESPACE_CHAIN');
        continue;
      }
      const item=record(configuration);
      const chains=item.chains===undefined && namespace===chain ? [chain] : strings(item.chains);
      if (!chains.includes(chain) || required && chains.some(value=>value!==chain)) {
        if (required) throw new Error('WC_NAMESPACE_CHAIN');
        continue;
      }
      for (const method of strings(item.methods)) {
        if (permitted.includes(method as DappMethod)) methods.add(method as DappMethod);
        else if (required) throw new Error('WC_NAMESPACE_METHOD');
      }
      for (const event of strings(item.events)) {
        if (events.includes(event)) approvedEvents.add(event);
        else if (required) throw new Error('WC_NAMESPACE_EVENT');
      }
    }
  }
  if (!methods.size) throw new Error('WC_NO_METHODS');
  return {chain,account:chain+':'+scope.account,origin,peerKey:proposer.publicKey,pairingTopic:pairing.topic,verified,methods:[...methods],events:[...approvedEvents]};
}
