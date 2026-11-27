import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {registerHooks} from 'node:module';
import ts from 'typescript';

const root = import.meta.url;
const repository = new URL('../../../', root);
registerHooks({
  load(url, context, next) {
    if (!url.startsWith(repository.href) || !url.endsWith('.ts') || url.includes('/node_modules/'))
      return next(url, context);
    return {format: 'module', shortCircuit: true,
      source: ts.transpileModule(readFileSync(new URL(url), 'utf8'), {
        compilerOptions: {module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022}
      }).outputText};
  }
});

const {segwitAddress, base58, xrplXAddress, bitcoinWatchDescriptors, bitcoinXpubCommand}=await import(new URL("../src/wallet/chainAccounts.ts",root));
const {presentReceive,receiveQr}=await import(new URL("../src/wallet/receivePresentation.ts",root));
const {evmProfiles}=await import(new URL("../src/wallet/evmTransfer.ts",root));
const btc=segwitAddress('tb',Uint8Array.of(0,20,...new Uint8Array(20).fill(0x12)));
const evm='0x'+'34'.repeat(20), token='0x'+'56'.repeat(20);
const sol=base58(new Uint8Array(32).fill(0x78)), xrp='rGWrZyQqhTp9Xu7G5Pkayo7bXjH4k4QYpf';
assert.equal(xrplXAddress(xrp,'0',true),'TVE26TYGhfLC7tQDno7G8dGtxSkYQnSy8RHqGHoGJ59spi2');
const cases=[
  [{kind:'btc',network:'Bitcoin Signet',hrp:'tb'},btc,'bitcoin:'+btc+'?label=Bitcoin%20Signet'],
  [{kind:'evm',network:'StableNet Testnet',symbol:evmProfiles['8283'].native,chainId:'8283'},evm,'ethereum:'+evm+'@8283'],
  [{kind:'evm',network:'Ethereum Sepolia',symbol:'ETH',chainId:'11155111'},evm,'ethereum:'+evm+'@11155111'],
  [{kind:'evm',network:'Base Sepolia',symbol:'ETH',chainId:'84532',token},evm,'Base Sepolia\nAddress: '+evm+'\nAsset: ERC-20 '+token],
  [{kind:'sol',network:'Solana Devnet'},sol,'Solana Devnet\nAddress: '+sol+'\nAsset: SOL'],
  [{kind:'xrp',network:'XRP Ledger Testnet',tag:'0',test:true},xrp,'XRP Ledger Testnet\nAddress: '+xrp+'\nDestinationTag: 0\nAsset: XRP\nX-address: TVE26TYGhfLC7tQDno7G8dGtxSkYQnSy8RHqGHoGJ59spi2'],
];
if (process.argv.includes('--watch')) {
  const proofArg=process.argv.indexOf("--proof");
  const knownProfile={chain:1,network:process.argv.includes("--signet")?2:1,account:1};
  const raw=proofArg>=0?readFileSync(process.argv[proofArg+1]):Buffer.concat([Buffer.from(bitcoinXpubCommand(knownProfile)),Buffer.from('1122334455667788','hex'),
    Buffer.alloc(32,55),Buffer.from('0330d54fd0dd420a6e5f8d3624f5f3482cae350f79d5f0753bf5beef9c2d91af3c','hex')]);
  const profile={...knownProfile,account:raw.readUInt32BE(3)};
  console.log(JSON.stringify(bitcoinWatchDescriptors(raw, profile)));
} else console.log(JSON.stringify(cases.map(([p,address,payload])=>{
  const card=presentReceive(p,address);
  assert.equal(card.payload,payload);
  assert.ok(card.detail.includes(address));
  assert.ok(card.share.includes(card.detail));
  assert.ok(card.share.includes(payload));
  return {name:p.network,payload,image:receiveQr(card.payload)};
})));
