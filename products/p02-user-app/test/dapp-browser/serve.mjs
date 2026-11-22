import {createServer} from 'node:https';
import {readFileSync} from 'node:fs';
import {randomBytes,randomUUID} from 'node:crypto';
import {browserProvider} from '../../src/wallet/dapp/provider.ts';
import {browserRpc} from '../../src/wallet/dapp/browserRpc.ts';
import {issueDappGrant} from '../../src/wallet/dapp/gate.ts';

const [key,certificate]=process.argv.slice(2);
if (!key || !certificate) throw new Error('Supply temporary local TLS key and certificate paths');
const host='127.0.0.1', port=8448, origin='https://'+host+':'+port;
const session=randomBytes(32).toString('hex'), document=randomUUID();
const clock=()=>Math.floor(Date.now()/1000);
const scope={board:'11'.repeat(32),account:'0x'+'23'.repeat(20),generation:1,chain:11155111,genesis:'0x'+'12'.repeat(32)};
const grant=issueDappGrant({transport:'browser',session,origin,topLevel:true},scope,['eth_accounts','eth_chainId'],clock(),clock()+300);

const page=`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Local wallet DApp</title></head><body><h1>Local wallet DApp</h1>
<p>This page uses the repository EIP-1193 provider and a local browser adapter.</p>
<button id="account">Read approved account</button>
<button id="unsupported">Request unsupported method</button>
<button id="revoke">Disconnect wallet</button>
<p role="status" id="wallet">Awaiting provider</p>
<iframe title="Embedded test frame" src="/frame"></iframe>
<script src="/wallet.js"></script></body></html>`;

const client=`${browserProvider}
const pair = new MessageChannel();
const status = document.getElementById('wallet');
pair.port1.onmessage = async event => {
  const answer = await fetch('/rpc', {method:'POST', headers:{'Content-Type':'application/json'}, body:event.data});
  if (answer.ok) pair.port1.postMessage(await answer.text());
};
window.addEventListener('ethereum#initialized', () => {status.textContent='Provider connected';});
window.postMessage('Nu54WalletProvider', location.origin, [pair.port2]);
async function invoke(method) {
  try { const answer=await window.ethereum.request({method}); status.textContent=Array.isArray(answer) ? answer.join() : answer; }
  catch (error) {status.textContent='Refused '+error.code;}
}
document.getElementById('account').addEventListener('click', () => invoke('eth_accounts'));
document.getElementById('unsupported').addEventListener('click', () => invoke('eth_sign'));
document.getElementById('revoke').addEventListener('click', async () => {
  const answer=await fetch('/revoke', {method:'POST'});
  if(answer.ok) status.textContent='Session disconnected';
});`;

createServer({key:readFileSync(key),cert:readFileSync(certificate)},async (request,response)=>{
  response.setHeader('Cache-Control','no-store');
  response.setHeader('Content-Security-Policy',"default-src 'self'; script-src 'self' 'unsafe-inline'; object-src 'none'");
  if (request.url==='/' && request.method==='GET') {
    response.setHeader('Content-Type','text/html; charset=utf-8');
    response.end(page);
  } else if (request.url==='/wallet.js' && request.method==='GET') {
    response.setHeader('Content-Type','text/javascript');
    response.end(client);
  } else if (request.url==='/frame' && request.method==='GET') {
    response.setHeader('Content-Type','text/html');
    response.end(`<p id="frame">Frame</p><script>${browserProvider}
    document.querySelector('#frame').textContent=window.ethereum ? 'Frame provider exposed' : 'Frame provider blocked';
    </script>`);
  } else if (request.method==='POST' && request.headers.origin===origin && request.url==='/revoke') {
    grant.revoked=true;
    response.end('closed');
  } else if (request.method==='POST' && request.headers.origin===origin && request.url==='/rpc') {
    try {
      let payload='';
      for await(const chunk of request) {
        payload+=chunk;
        if (payload.length>4096) throw new Error('size');
      }
      const reply=browserRpc(grant,scope,JSON.stringify({session,origin,document,request:payload}),clock());
      response.end(reply.reply);
    } catch {
      response.writeHead(400).end();
    }
  } else response.writeHead(404).end();
}).listen(port,host,()=>process.stdout.write(origin+'\n'));
