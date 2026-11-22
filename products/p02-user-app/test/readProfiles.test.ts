import {secp256k1} from '@noble/curves/secp256k1';
import {bigToBytes, concat, hexToBytes} from '@nu54/protocol';
import {decodeWalletStorage} from '../src/wallet/profile';
import {defaultReadState, discoverSignetBranches, discoverSignetReceive, formatReadAmount, observationScope, readContact, readCustomEvmBalance, readCustomEvmDescriptor, readProfiles, readSelectedBalance, setCustomEvmDescriptor, updateReadObservation, verifyReadNetwork, walletReadRpc} from '../src/wallet/readProfiles';
import {base58, bitcoinSignetCheckpoint, bitcoinWatchDescriptors, bitcoinXpubCommand, chainPublicCommand, checkSegwitV0, hash160, readChainPublic, segwitAddress} from '../src/wallet/chainAccounts';
import type {ChainRpc} from '../src/wallet/chainTransfer';

test('the six read-only profiles pin independent identity and persist chain/account/asset/hide separately', () => {
  expect(Object.keys(readProfiles)).toEqual(['stable', 'eth', 'base', 'btc', 'sol', 'xrp']);
  const board = 'de'.repeat(32), address = '0x' + '42'.repeat(20);
  const selection = {id: 'btc' as const, account: 2, hidden: true};
  const scope = observationScope(board, 4, selection);
  const walletRead = {...defaultReadState, selection, endpoints: {btc: 'https://node.example/signet'},
    contacts: [{id: 'eth', name: 'Work wallet', address}], observations: [{scope, time: 2000, amount: '123',
      address: 'tb1qexamplepublicaddress', unit: 'sats', point: 'Bitcoin tip 100'}]};
  const saved = decodeWalletStorage(JSON.stringify({version: 2, exercised: true, mode: 'personal',
    verified: {network: '8283', boardFingerprint: board, address, generation: 4}, walletRead})).walletRead!;
  expect(saved.selection).toEqual(selection);
  expect(saved.contacts).toEqual(walletRead.contacts);
  expect(saved.observations.find(row => row.scope === observationScope(board, 5, selection))).toBeUndefined();
  expect(saved.observations.find(row => row.scope === observationScope(board, 4, {...selection, account: 3}))).toBeUndefined();
  expect(saved.observations.find(row => row.scope === observationScope(board, 4, {...selection, id: 'sol'}))).toBeUndefined();
  expect(saved.endpoints.btc).toBe('https://node.example/signet');
});

test('Signet uses a tb address and verifies each confirmed UTXO against the same active block', async () => {
  const publicKey = hexToBytes('0330d54fd0dd420a6e5f8d3624f5f3482cae350f79d5f0753bf5beef9c2d91af3c');
  const address = segwitAddress('tb', Uint8Array.of(0, 20, ...hash160(publicKey)));
  const block = '13'.repeat(32), txid = '14'.repeat(32);
  const account = {chain: 1 as const, network: 1 as const, account: 0, address: 'bcrt-not-used', publicKey};
  let actual = block, chain = 'signet', checkpoint: string = bitcoinSignetCheckpoint.hash;
  const rpc: ChainRpc = jest.fn(async (method: string, params: unknown) => {
    switch (method) {
      case 'getblockchaininfo': return {chain, blocks: 19, bestblockhash: block};
      case 'getblockhash': return JSON.stringify(params) === '[0]' ? readProfiles.btc.anchor :
        JSON.stringify(params) === '[321295]' ? checkpoint : block;
      case 'getaddressinfo': expect(params).toEqual([address]); return {address, iswatchonly: true};
      case 'gettxout': expect(params).toEqual([txid, 0, false]); return {bestblock: actual, value: '0.001',
        confirmations: 8, scriptPubKey: {address}};
      case 'listunspent': expect(params).toEqual([6, 9999999, [address]]); return [{address, txid, vout: 0,
        amount: '0.001', confirmations: 8}];
      default: throw Error(method);
    }
  });
  expect(address).toMatch(/^tb1q/);
  const selection = {id: 'btc' as const, account: 0, hidden: false};
  await expect(readSelectedBalance(rpc, selection, account)).resolves.toMatchObject({address, amount: '100000'});
  actual = '17'.repeat(32);
  await expect(readSelectedBalance(rpc, selection, account)).rejects.toThrow('BTC_BALANCE_EVIDENCE');
  actual = block;
  checkpoint = 'ab'.repeat(32);
  await expect(verifyReadNetwork(rpc, 'btc')).rejects.toThrow('WALLET_READ_CHAIN');
  await expect(readSelectedBalance(rpc, selection, account)).rejects.toThrow('BTC_SIGNET_NETWORK');
  chain = 'regtest';
  await expect(verifyReadNetwork(rpc, 'btc')).rejects.toThrow('WALLET_READ_CHAIN');
});

test('read-only exchange refuses mutations and verifies an alternate network without forwarding any address', async () => {
  const exchange = jest.fn(async (_endpoint: string, packet: string) => {
    const {id, method, params} = JSON.parse(packet);
    expect(params).toEqual([]);
    expect(method).toBe('getGenesisHash');
    return JSON.stringify({id, jsonrpc: '2.0', result: readProfiles.sol.anchor});
  });
  const rpc = walletReadRpc('https://node.example/', exchange);
  await expect(rpc('sendTransaction', ['raw'])).rejects.toThrow('WALLET_READ_METHOD');
  await expect(rpc('submit', [{secret: 'placeholder'}])).rejects.toThrow('WALLET_READ_METHOD');
  expect(exchange).not.toHaveBeenCalled();
  await expect(verifyReadNetwork(rpc, 'sol')).resolves.toBeUndefined();
  expect(exchange).toHaveBeenCalledTimes(1);
});

test('contacts verify network/checksum and preserve XRP tag zero', () => {
  const pub = secp256k1.getPublicKey(bigToBytes(11n, 32), true);
  const script = Uint8Array.of(0, 20, ...hash160(pub));
  const signet = segwitAddress('tb', script);
  expect(readContact('btc', 'Desk', signet)).toMatchObject({address: signet});
  expect(() => readContact('btc', 'Desk', segwitAddress('bcrt', script))).toThrow('BTC_ADDRESS_NETWORK');
  expect(() => readContact('btc', 'Desk', signet.slice(0, -1) + (signet.endsWith('a') ? 'h' : 'a')))
    .toThrow('BTC_ADDRESS_CHECKSUM');
  const xrp = 'rwhKUCPbWBrj7pHZuNy57L8gb7hbfcc7gK';
  expect(readContact('xrp', 'Exchange', xrp, '0')).toMatchObject({tag: '0'});
  expect(() => readContact('xrp', 'Exchange', xrp.slice(0, -1) + 'X')).toThrow();
  expect(() => readContact('xrp', 'Exchange', xrp, '4294967296')).toThrow('WALLET_READ_CONTACT_TAG');
  expect(() => readContact('sol', 'Wallet', base58(new Uint8Array(31)))).toThrow('CHAIN_ADDRESS_ENCODING');
  const sol = base58(new Uint8Array(32).fill(77));
  expect(readContact('sol', 'Wallet', sol).address).toBe(sol);
  expect(formatReadAmount('1234567', 6)).toBe('1.234567');
});

test('only a matched ledger checkpoint makes network ID 1 eligible for public XRP Testnet reads', async () => {
  const {ledger, height} = readProfiles.xrp;
  await expect(verifyReadNetwork(async method => method === 'server_info' ?
    {info: {network_id: 1}} : {validated: true, ledger_hash: ledger, ledger_index: height}, 'xrp')).resolves.toBeUndefined();
  await expect(verifyReadNetwork(async method => method === 'server_info' ?
    {info: {network_id: 1}} : {validated: true, ledger_hash: 'FA'.repeat(32), ledger_index: height}, 'xrp'))
    .rejects.toThrow('WALLET_READ_CHAIN');
});

test('Signet gap discovery counts spent historical receipts, returns the next index, and never transmits xpub', async () => {
  const account = {chain: 1 as const, network: 1 as const, account: 3};
  let lastAddress = '', calls = 0;
  const known = new Map<string, number>();
  const derive = async (index: number) => {
    const publicKey = secp256k1.getPublicKey(bigToBytes(BigInt(index + 21), 32), true);
    const cmd = chainPublicCommand(account, 0, index);
    const reply = Uint8Array.of(...cmd, ...publicKey, ...hash160(publicKey), 1, 2, 3, 4);
    const child = readChainPublic(reply, account, 0, index);
    const signet = segwitAddress('tb', Uint8Array.of(0, 20, ...hash160(publicKey)));
    expect(checkSegwitV0('tb', signet)).toBe(signet);
    known.set(signet, index);
    lastAddress = signet;
    return child;
  };
  const rpc: ChainRpc = async (method, params) => {
    if (method === 'getblockchaininfo') return {chain: 'signet', bestblockhash: 'ab'.repeat(32)};
    if (method === 'getblockhash') return JSON.stringify(params) === '[0]' ? readProfiles.btc.anchor : bitcoinSignetCheckpoint.hash;
    if (method === 'getaddressinfo') {
      expect(params).toEqual([lastAddress]);
      return {address: lastAddress, iswatchonly: true};
    }
    expect(method).toBe('getreceivedbyaddress');
    expect(params).toEqual([lastAddress, 1]);
    calls++;
    return known.get(lastAddress) === 3 ? '0.01' : '0.00000000';
  };
  expect(await discoverSignetReceive(rpc, 3, derive)).toEqual({lastUsed: 3, nextIndex: 4, scanned: 24});
  expect(calls).toBe(24);
  const cancelled = new AbortController();
  cancelled.abort();
  await expect(discoverSignetReceive(rpc, 3, derive, 0, cancelled.signal)).rejects.toThrow('BTC_DISCOVERY_CANCELLED');
});

test('W25 compares both branches of one prior account at a single verified Signet head',async()=>{
  const account={chain:1 as const,network:2 as const,account:3};
  const exported=bitcoinWatchDescriptors(concat(bitcoinXpubCommand(account),new Uint8Array(40).fill(7),
    secp256k1.getPublicKey(bigToBytes(55n,32),true)),account);
  const head='ac'.repeat(32);
  const derived=new Map<string,{branch:0|1;index:number}>();
  const derive=jest.fn(async(branch:0|1,index:number)=>{
    const publicKey=exported.publicChild(branch,index);
    const address=segwitAddress('tb',Uint8Array.of(0,20,...hash160(publicKey)));
    derived.set(address,{branch,index});
    return {...account,publicKey,address};
  });
  let heads=0,changeHead=false;
  const provider:ChainRpc=async(method,params)=>{
    if (method==='getblockchaininfo') return {chain:'signet',bestblockhash:changeHead && ++heads===3?'bc'.repeat(32):head};
    if (method==='getblockhash') return JSON.stringify(params)==='[0]' ? readProfiles.btc.anchor : bitcoinSignetCheckpoint.hash;
    const [address,depth]=params as [string,number?];
    const item=derived.get(address);
    expect(item).toBeDefined();
    if (method==='getaddressinfo') return {address,iswatchonly:true};
    expect(method).toBe('getreceivedbyaddress');
    expect(depth).toBe(1);
    return item?.index===(item?.branch===0?3:5)?'0.01':'0';
  };
  const endpoint='https://private-node.example/wallet/receive';
  const exchange=jest.fn(async(url:string,packet:string)=>{
    expect(url).toBe(endpoint);
    expect(packet).not.toContain(exported.xpub);
    const {method,params}=JSON.parse(packet);
    return JSON.stringify({id:1,result:await provider(method,params)});
  });
  const rpc=walletReadRpc(endpoint,exchange);
  await expect(discoverSignetBranches(rpc,account.account,derive)).resolves.toEqual({head,account:3,
    receive:{lastUsed:3,nextIndex:4,scanned:24},change:{lastUsed:5,nextIndex:6,scanned:26}});
  expect(derive).toHaveBeenCalledWith(0,3);
  expect(derive).toHaveBeenCalledWith(1,5);
  changeHead=true;
  await expect(discoverSignetBranches(rpc,3,derive)).rejects.toThrow('BTC_DISCOVERY_HEAD_CHANGED');
  const cancel=new AbortController();
  changeHead=false;
  await expect(discoverSignetBranches(rpc,3,async(branch,index)=>{
    if (branch===1) cancel.abort();
    return derive(branch,index);
  },cancel.signal)).rejects.toThrow('BTC_DISCOVERY_CANCELLED');
});

test('custom EVM descriptors retain identity and read capability without touching the signed wallet engines', async () => {
  const genesis = '0x' + 'ab'.repeat(32), block = '0x' + 'cd'.repeat(32);
  const input = {engine: 'evm', name: 'Local archive', chainId: '31337', genesis, rpc: 'https://rpc.example/',
    explorer: 'https://blocks.example/', currency: 'TEST', decimals: 18, feeCap: '10000', confirmations: 6};
  const network = readCustomEvmDescriptor(input);
  expect(network.capability).toBe('read-only');
  const alternate = readCustomEvmDescriptor({...input, rpc: 'https://next.example/'});
  expect(setCustomEvmDescriptor([network], alternate)).toEqual([alternate]);
  expect(() => setCustomEvmDescriptor([network], {...alternate, genesis: '0x' + '12'.repeat(32)}))
    .toThrow('WALLET_CUSTOM_IDENTITY');
  expect(() => readCustomEvmDescriptor({...input, engine: 'unknown'})).toThrow('WALLET_CUSTOM_ENGINE');
  expect(() => readCustomEvmDescriptor({...input, capability: 'sign'})).toThrow('WALLET_CUSTOM_ENGINE');
  expect(() => readCustomEvmDescriptor({...input, chainId: '8283'})).toThrow('WALLET_CUSTOM_SCHEMA');
  const rpc = jest.fn(async (method: string, params: unknown) => {
    if (method === 'eth_chainId') return '0x7a69';
    if (method === 'eth_getBlockByNumber') {
      if (JSON.stringify(params) === '["0x0",false]') return {hash: genesis};
      if (JSON.stringify(params) === '["latest",false]') return {number: '0x20'};
      expect(params).toEqual(['0x1b', false]);
      return {hash: block, number: '0x1b'};
    }
    expect(method).toBe('eth_getBalance');
    expect(params).toEqual(['0x' + '14'.repeat(20), {blockHash: block, requireCanonical: true}]);
    return '0x42';
  });
  expect(await readCustomEvmBalance(alternate, '0x' + '14'.repeat(20), rpc)).toMatchObject({height: '27', amount: '66'});
  expect(rpc.mock.calls.some(([method]) => /send|sign|submit/i.test(method))).toBe(false);
});

test('polling does not count an unchanged, already observed balance as a new deposit', () => {
  const row = {scope: 'owner/network/address/asset', address: 'tb1qexample', amount: '300', unit: 'sats',
    point: 'Bitcoin tip 9999', time: 1234};
  const previous = [row];
  expect(updateReadObservation(previous, {...row, time: 1235})).toBe(previous);
  expect(updateReadObservation(previous, {...row, amount: '310'})).toEqual([{...row, amount: '310'}]);
});
