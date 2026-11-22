import fixtures from './fixtures/w28-local-evm.json';
import {secp256k1} from '@noble/curves/secp256k1';
import {keccak_256} from '@noble/hashes/sha3';
import {bigToBytes,bytesToHex,concat,hexToBytes} from '@nu54/protocol';
import {decodeWalletStorage} from '../src/wallet/profile';
import {checkEvmNetwork,finalityMatchesPending,finalityMatchesSigning,httpsEvmRpc,queryEvmTransfer,readDirectTransfer,readEvmReceiveBalance,verifySignedTransfer,type EvmRpc} from '../src/wallet/evmTransfer';

const receiver = '12'.repeat(20);
const chain = 11155111n;
const raw = hexToBytes('02e283aa36a703030c82520894' + receiver + '0980c0');
const envelope = (chainId=chain) => bytesToHex(concat(Uint8Array.of(9),bigToBytes(1060n,8),
  bigToBytes(chainId,8),bigToBytes(252000n,8),new Uint8Array(8+20),new Uint8Array(32).fill(0xab),bigToBytes(1n,8),bigToBytes(2n,8),new Uint8Array(64),raw),false);

test('accepts exact EIP-1559 terms and recovered HD sender, refusing another chain', () => {
  const parsed = readDirectTransfer(envelope(),1000n);
  expect(parsed).toMatchObject({chain:'11155111',nonce:3n,recipient:'0x'+receiver,amount:9n,maximum:252000n});
  expect(()=>readDirectTransfer(envelope(8283n),1000n)).toThrow('EVM_TX_FIELDS');
  expect(()=>readDirectTransfer(envelope(),1060n)).toThrow('EVM_PROFILE');

  const key = new Uint8Array(32).fill(7);
  const signature = secp256k1.sign(parsed.digest,key);
  const compact = signature.toCompactRawBytes();
  const tail = concat(Uint8Array.of(signature.recovery || 0x80,0xa0),compact.subarray(0,32),
    Uint8Array.of(0xa0),compact.subarray(32));
  const payload = concat(raw.subarray(2),tail);
  const tx = concat(Uint8Array.of(2,0xf8,payload.length),payload);
  const sender = bytesToHex(keccak_256(secp256k1.getPublicKey(key,false).subarray(1)).subarray(12),true);
  expect(verifySignedTransfer(parsed,tx,sender).raw).toBe(bytesToHex(tx,true));
  expect(()=>verifySignedTransfer(parsed,tx,'0x'+receiver)).toThrow('EVM_SENDER');
});

const hash = '0x'+'11'.repeat(32), genesis='0x'+'22'.repeat(32), block='0x'+'33'.repeat(32);
const network = (height:string,canonical:string):EvmRpc => async (method,params) => {
  if (method==='eth_chainId') return '0x14a34';
  if (method==='eth_getTransactionReceipt') return {transactionHash:hash,blockHash:block,blockNumber:'0x7',status:'0x1'};
  if (params[0]==='0x0') return {hash:genesis};
  if (params[0]==='0x7') return {hash:canonical};
  return {number:height};
};

test('distinguishes canonical finalized Base receipts from unfinalized or reorged ones', async () => {
  await expect(queryEvmTransfer(network('0x7',block),'84532',genesis,hash)).resolves.toBe('finalized');
  await expect(queryEvmTransfer(network('0x6',block),'84532',genesis,hash)).resolves.toBe('included');
  await expect(queryEvmTransfer(network('0x7',genesis),'84532',genesis,hash)).resolves.toBe('pending');
  await expect(queryEvmTransfer(network('0x7',block),'11155111',genesis,hash)).rejects.toThrow('EVM_RPC_CHAIN');
});

test('refuses a same-chain-ID RPC on a different genesis', async () => {
  await expect(queryEvmTransfer(network('0x7',block),'84532','0x'+'ff'.repeat(32),hash)).rejects.toThrow('EVM_RPC_CHAIN');
});

test('the direct transfer envelope requires immutable genesis and profile revisions', () => {
  const parsed = readDirectTransfer(envelope(),1000n);
  expect(parsed).toMatchObject({genesis:'0x'+'ab'.repeat(32),profileRevision:1n,leaseRevision:2n});
});


test('retains an old unbound checkpoint for reconciliation', () => {
  const old={chain:'84532',raw:'0x0201',hash,sender:'0x'+'99'.repeat(20),expires:1100,submitted:true};
  const restored=decodeWalletStorage(JSON.stringify({version:2,exercised:true,mode:'personal',pendingEvm:old}));
  expect(restored.pendingEvm).toEqual(old);
  expect(restored.pendingEvm?.genesis).toBeUndefined();
});

test('rechecks canonical inclusion after the finalized-tip read', async () => {
  const base=network('0x7',block);
  let reads=0;
  const racing:EvmRpc=async(method,params)=>{
    if (method==='eth_getBlockByNumber' && params[0]==='0x7' && ++reads>1) return {hash:genesis};
    return base(method,params);
  };
  await expect(queryEvmTransfer(racing,'84532',genesis,hash)).resolves.toBe('pending');
});

test('pruned Base RPC remains ineligible and the manually selected provider must return the pinned genesis', async () => {
  const pinned = '0x0dcc9e089e30b90ddfc55be9a37dd15bc551aeee999d2e2b51414c54eaf934e4';
  const endpoint = 'https://rpc.example.test';
  const provider = (genesis: string | null, pruned = false) => httpsEvmRpc(endpoint, async (_url, packet) => {
    const request = JSON.parse(packet);
    const response = request.method === 'eth_chainId' ? {result: '0x14a34'} :
      pruned ? {error: {code: 4444, message: 'Pruned history'}} : {result: genesis ? {hash: genesis} : null};
    return JSON.stringify({jsonrpc: '2.0', id: 1, ...response});
  });
  await expect(checkEvmNetwork(provider(null, true), '84532', pinned)).rejects.toThrow('EVM_RPC_GENESIS_UNAVAILABLE');
  await expect(checkEvmNetwork(provider('0x' + 'ab'.repeat(32)), '84532', pinned)).rejects.toThrow('EVM_RPC_CHAIN');
  await expect(checkEvmNetwork(provider(pinned), '84532', pinned)).resolves.toBeUndefined();
});

test.each(['8283', '11155111', '84532'] as const)('confirmed receive balance stays on the pinned %s network and finalized block', async chain => {
  const genesis = '0x' + '11'.repeat(32), block = '0x' + '12'.repeat(32);
  const owner = '0x' + '22'.repeat(20), token = '0x' + '33'.repeat(20);
  const rpc = jest.fn(async (method: string, params: readonly unknown[]) => {
    if (method === 'eth_chainId') return '0x' + BigInt(chain).toString(16);
    if (method === 'eth_getBlockByNumber') return {hash: params[0] === '0x0' ? genesis : block, number: '0x8'};
    expect(params.at(-1)).toEqual({blockHash: block, requireCanonical: true});
    if (method === 'eth_getBalance') return '0x2a';
    expect(params[0]).toEqual({to: token, data: '0x70a08231' + '0'.repeat(24) + owner.slice(2)});
    return '0x' + '0'.repeat(62) + '2b';
  });
  await expect(readEvmReceiveBalance(rpc, chain, genesis, owner)).resolves.toEqual({amount: '42', block: '8'});
  await expect(readEvmReceiveBalance(rpc, chain, genesis, owner, token)).resolves.toEqual({amount: '43', block: '8'});
});


test('pending cleanup requires the same board request, generation, sender and signed original',()=>{
 const f=fixtures[0],board='34'.repeat(32),request='56'.repeat(16);
 const pending={chain:'8283' as const,raw:f.raw,hash:f.hash,sender:f.sender,expires:1000,submitted:true,genesis:f.genesis,profileRevision:1,leaseRevision:'1',evidence:{board,generation:7,request,unsigned:'0x'+f.unsigned}};
 const record={version:2 as const,exercised:true,mode:'personal' as const,pendingEvm:pending};
 expect(decodeWalletStorage(JSON.stringify(record)).pendingEvm).toEqual(pending);
 const body=new Uint8Array(237);body.set(hexToBytes('4e353446'));body.set(hexToBytes(board),4);body.set(hexToBytes(f.genesis.slice(2)),36);body.set(hexToBytes(f.sender.slice(2)),68);body.set(hexToBytes(request),108);body.set(keccak_256(hexToBytes(f.unsigned)),124);body.set(bigToBytes(7n,8),188);body.set(bigToBytes(8283n,8),196);body[236]=1;
 expect(finalityMatchesPending(body,hexToBytes(f.unsigned),pending)).toBe(true);
 for(const offset of [4,36,68,108,124,188,196]){const wrong=body.slice();wrong[offset]^=1;expect(finalityMatchesPending(wrong,hexToBytes(f.unsigned),pending)).toBe(false);}
 const tampered={...pending,evidence:{...pending.evidence,unsigned:'0x'+f.unsigned.slice(0,-2)+'01'}};
 expect(()=>decodeWalletStorage(JSON.stringify({...record,pendingEvm:tampered}))).toThrow('WALLET_STORAGE_CORRUPT');
 expect(finalityMatchesPending(body,hexToBytes(f.unsigned),{...pending,evidence:undefined})).toBe(false);
 const signing={kind:'evm' as const,board,generation:7,evm:{request,unsigned:'0x'+f.unsigned,chain:'8283' as const,genesis:f.genesis,signer:f.sender}};
 expect(finalityMatchesSigning(body,hexToBytes(f.unsigned),signing)).toBe(true);
 for(const offset of [4,36,68,108,124,188,196]){const wrong=body.slice();wrong[offset]^=1;expect(finalityMatchesSigning(wrong,hexToBytes(f.unsigned),signing)).toBe(false);}
 expect(finalityMatchesSigning(body,hexToBytes(f.unsigned),{...signing,evm:undefined})).toBe(false);
 expect(finalityMatchesSigning(body,hexToBytes(f.unsigned),{...signing,evm:{...signing.evm,chain:'84532'}})).toBe(false);
 expect(finalityMatchesSigning(body,hexToBytes(f.unsigned),{...signing,kind:'chain'})).toBe(false);
});
