import {randomBytes} from 'node:crypto';
import {readFileSync,writeFileSync} from 'node:fs';
import {createInterface} from 'node:readline';
import {hexToBytes} from '@nu54/protocol';
import {WalletConnectSession} from '../../src/wallet/dapp/walletConnectSession.ts';
import {IrnRelay} from '../../src/wallet/dapp/walletConnectRelay.ts';
import {verifyPeer} from '../../src/wallet/dapp/walletConnectPolicy.ts';
import {checkEvmNetwork,readDirectTransfer,verifySignedTransfer,verifyTransferAuthority} from '../../src/wallet/evmTransfer.ts';

const [configPath,checkpointPath]=process.argv.slice(2);
const {socket,http,scope,authority,token,recipient}=JSON.parse(readFileSync(configPath));
const clock=()=>Math.floor(Date.now()/1000);
const status=message=>process.stdout.write(message+'\n');
async function post(path,value) {
  const reply=await fetch(http+path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(value)});
  if (!reply.ok) throw new Error('QA boundary refused operation');
  return reply.json();
}
const rpc=(method,params)=>post('/rpc',{method,params});
const issue=(method,argument)=>post('/wallet',{jsonrpc:'2.0',id:1,method,params:[{token,...argument}]});
const rtc=new IrnRelay(socket);
const client=new WalletConnectSession({rtc,scope,clock,random:async length=>randomBytes(length),allowSpend:true,
  save:async checkpoint=>writeFileSync(checkpointPath,JSON.stringify(checkpoint??null),{mode:0o600}),
  approve:async proposal=>proposal.verified && proposal.methods.includes('eth_sendTransaction'),
  trust:async(pairing,proposal)=>{
    const proof=await issue('nu54_walletConnectPeer',{chain:String(scope.chain),origin:'https://independent.example',
      pairingTopic:pairing.topic,peerKey:proposal.proposer.publicKey});
    return verifyPeer(proof.proof,proof.signature,hexToBytes(authority),clock());
  },
  request:async(action,proposal,signal,sessionTopic)=>{
    if (action.kind!=='spend' || action.recipient!==recipient || signal.aborted) throw new Error('QA user declined');
    const context={origin:proposal.origin,pairingTopic:proposal.pairingTopic,sessionTopic,peerKey:proposal.peerKey};
    const wire=await issue('nu54_walletExternalSpend',{...context,chain:String(scope.chain),asset:'0x'+'00'.repeat(20),
      recipient:action.recipient,amount:action.value.toString()});
    const draft=readDirectTransfer(wire,BigInt(clock()));
    verifyTransferAuthority(draft,hexToBytes(authority),scope.board,BigInt(scope.generation),scope.account,1n);
    if (draft.chain!==String(scope.chain) || draft.genesis!==scope.genesis || draft.recipient!==action.recipient ||
        draft.amount!==action.value || Object.keys(context).some(field=>context[field]!==draft.externalContext?.[field]))
      throw new Error('QA spend differed from consent');
    await checkEvmNetwork(rpc,draft.chain,draft.genesis);
    if (signal.aborted) throw new Error('QA user revoked');
    const signed=verifySignedTransfer(draft,hexToBytes(await post('/board',{wire})),scope.account);
    if (signal.aborted || await rpc('eth_sendRawTransaction',[signed.raw])!==signed.hash) throw new Error('QA transaction changed');
    return signed.hash;
  }});

for await (const line of createInterface({input:process.stdin})) {
  try {
    if (line.startsWith('pair ')) {await client.pair(line.slice(5));status('paired');}
    if (line==='resume') {await client.resume(JSON.parse(readFileSync(checkpointPath)));status('resumed');}
    if (line==='suspend') {client.suspend();break;}
  } catch (error) {status('error '+error.message);}
}
