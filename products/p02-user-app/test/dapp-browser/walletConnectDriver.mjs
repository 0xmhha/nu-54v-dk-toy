import {randomBytes} from 'node:crypto';
import {readFileSync,writeFileSync} from 'node:fs';
import {createInterface} from 'node:readline';
import {WalletConnectSession} from '../../src/wallet/dapp/walletConnectSession.ts';
import {IrnRelay} from '../../src/wallet/dapp/walletConnectRelay.ts';

const [endpoint,file]=process.argv.slice(2);
const scope={board:'11'.repeat(32),account:'0x'+'22'.repeat(20),generation:3,chain:11155111,genesis:'0x'+'33'.repeat(32)};
const status=message=>process.stdout.write(message+'\n');
const rtc=new IrnRelay(endpoint);
const client=new WalletConnectSession({rtc,scope,clock:()=>Math.floor(Date.now()/1000),random:async size=>randomBytes(size),
  save:async checkpoint=>{writeFileSync(file,JSON.stringify(checkpoint??null),{mode:0o600});status(checkpoint?'saved':'deleted');},
  approve:async proposal=>{status('proposal '+proposal.origin+' '+proposal.verified);return proposal.origin!=='https://denied.example';},
  trust:async()=>undefined,request:async()=>{throw new Error('Unverified peer reached signing callback');},allowSpend:false});
for await (const line of createInterface({input:process.stdin})) {
  try {
    if (line.startsWith('pair ')) {await client.pair(line.slice(5));status('paired');}
    if (line==='resume') {await client.resume(JSON.parse(readFileSync(file)));status('resumed');}
    if (line==='suspend') {client.suspend();break;}
    if (line==='revoke') await client.disconnect();
  } catch (error) {status('error '+error.message);}
}
