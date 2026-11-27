import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha256';
import {bytesToHex,concat,toBase64,utf8Encode} from '@nu54/protocol';
import {authenticateRegistry,readRegistryEntry} from '../src/wallet/evmRegistry';
import {verifyCommissionedChain} from '../src/wallet/commissionedChainProfiles';
import {bitcoinSignetCheckpoint,normalizeXrplDestination,xrplXAddress} from '../src/wallet/chainAccounts';

const genesis='00000008819873e925422c1ff0f99f7cc9bbb232af63a077a480a3633bee1ef6';
const candidate={chain:1,profile:'bitcoin-signet',anchor:genesis+':'+bitcoinSignetCheckpoint.hash,rpc:'https://wallet-gateway.example/bitcoin'};
const registry=(chainNetworks: unknown[])=>readRegistryEntry({endpoint:'https://authority.example/wallet',token:'21'.repeat(32),proof:'42'.repeat(64),
  payload:toBase64(utf8Encode(JSON.stringify({device:'11'.repeat(32),sender:'0x'+'22'.repeat(20),generation:1,leaseRevision:'1',
    expires:200,networks:[],chainNetworks})))});
test('accepts only the enumerated commissioned Signet anchor and canonical HTTPS gateway',()=>{
  expect(registry([candidate]).registry).toHaveProperty('chainNetworks',[candidate]);
  for (const bad of [
    {...candidate,anchor:genesis}, {...candidate,anchor:genesis+':'+'12'.repeat(32)},
    {...candidate,rpc:'https://127.0.0.1/bitcoin'}, {...candidate,rpc:'https://wallet-gateway.example/bitcoin?secret=abc'},
    {...candidate,rpc:'https://wallet-gateway.example/solana'}, {...candidate,extra:'unknown'},
  ]) expect(()=>registry([bad])).toThrow();
  expect(()=>registry([candidate,candidate])).toThrow();
});

test('the same-genesis foreign Signet must fail before token or transaction operations', async()=>{
  const calls:string[]=[];
  const exchange=async (method:string, params:unknown) => {
    calls.push(method);
    if (method==='getblockchaininfo') return {chain:'signet'};
    if (method==='getblockhash') return (params as number[])[0]===0 ? genesis : '34'.repeat(32);
    throw Error('unexpected call');
  };
  const [profile] = registry([candidate]).registry.chainNetworks!;
  await expect(verifyCommissionedChain(profile,exchange)).rejects.toThrow('CHAIN_COMMISSIONED_ANCHOR');
  expect(calls).toEqual(['getblockchaininfo','getblockhash','getblockhash']);
});

test.each([
  {chain:2,profile:'solana-devnet',anchor:'EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG',rpc:'https://wallet-gateway.example/solana'},
  {chain:3,profile:'xrpl-testnet',anchor:'1:60691D3CA08E04064EBB014D0ADE8DA2CB2BFE0AE9D820D88502FB3A9A8F7149',rpc:'https://wallet-gateway.example/xrpl'},
] as const)('requires the exact offline network evidence for $profile', async row=>{
  const [admitted]=registry([row]).registry.chainNetworks!;
  await expect(verifyCommissionedChain(admitted,async method=>{
    if (method==='getGenesisHash') return row.anchor;
    if (method==='server_info') return {info:{network_id:1}};
    return {validated:true,ledger_hash:row.anchor.slice(2),ledger_index:22470370};
  })).resolves.toBeUndefined();
  await expect(verifyCommissionedChain(admitted,async method=>{
    if (method==='getGenesisHash') return 'mainnet-genesis';
    if (method==='server_info') return {info:{network_id:1}};
    return {validated:false,ledger_hash:row.anchor.slice(2),ledger_index:22470370};
  })).rejects.toThrow('CHAIN_COMMISSIONED_ANCHOR');
});

test('commissioned routes require the enrolled authority, owner and lease',()=>{
  const {entry,bytes,registry:document}=registry([candidate]);
  const key=new Uint8Array(32).fill(21), anchor=p256.getPublicKey(key,false);
  const proof=bytesToHex(p256.sign(sha256(concat(utf8Encode('N54G'),bytes)),key).toCompactRawBytes(),false);
  const scope={device:document.device,address:document.sender,generation:document.generation};
  const authentic={...entry,proof};
  expect(authenticateRegistry(authentic,anchor,1n,scope,199).registry.chainNetworks).toEqual([candidate]);
  expect(()=>authenticateRegistry(entry,anchor,1n,scope,199)).toThrow('EVM_REGISTRY_AUTHORITY');
  expect(()=>authenticateRegistry(authentic,anchor,2n,scope,199)).toThrow('EVM_REGISTRY_AUTHORITY');
  expect(()=>authenticateRegistry(authentic,anchor,1n,{...scope,generation:2},199)).toThrow('EVM_REGISTRY_AUTHORITY');
  expect(()=>authenticateRegistry(authentic,anchor,1n,scope,200)).toThrow('EVM_REGISTRY_AUTHORITY');
});

test('W07 XRP test X-address retains its embedded zero through W28 and rejects conflicts',()=>{
  const classic='rwhKUCPbWBrj7pHZuNy57L8gb7hbfcc7gK';
  const testAddress=xrplXAddress(classic,'0',true);
  expect(normalizeXrplDestination(testAddress,undefined,true)).toEqual({destination:classic,tag:0});
  expect(normalizeXrplDestination(testAddress,0,true)).toEqual({destination:classic,tag:0});
  expect(()=>normalizeXrplDestination(testAddress,1,true)).toThrow();
  expect(()=>normalizeXrplDestination(testAddress,undefined,false)).toThrow();
  expect(()=>normalizeXrplDestination(testAddress.slice(0,-1),undefined,true)).toThrow();
});
