import {ed25519} from '@noble/curves/ed25519';
import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha256';
import {bigToBytes, bytesToHex, concat, hexToBytes} from '@nu54/protocol';
import {checksumDescriptor} from '../src/wallet/bitcoinDescriptor';
import {base58, bitcoinWatchDescriptors, bitcoinXpubCommand, decodePublicBase58, hash160, readChainPublic, receiveCard, type ChainPublicAccount} from '../src/wallet/chainAccounts';
import {bitcoinReplacementDraft, checkBitcoinReplacement, checkChainNetwork, loopbackChainRpc, prepareChainTransfer, queryChainTransfer, readChainReceiveBalance, readStoredChain,
  submitChainTransfer, verifyChainSigned, type ChainRpc, type StoredChain, type ChainTransfer} from '../src/wallet/chainTransfer';
import {decodeWalletStorage} from '../src/wallet/profile';

const hex = (b: Uint8Array) => bytesToHex(b, false);
const utf8 = (s: string) => Uint8Array.from(s, c => c.charCodeAt(0));

function timeProof(authority: Uint8Array, board: string, generation: bigint, revision: bigint, nonce: Uint8Array, time: number) {
  const statement = concat(utf8('N54C'), hexToBytes(board), bigToBytes(generation, 8), bigToBytes(revision, 8),
    nonce, bigToBytes(BigInt(time), 8));
  return {time, proof: hex(p256.sign(sha256(statement), authority).toCompactRawBytes())};
}

test('official BIP84 public key and XRP destination tag 0 have exact receive encodings', () => {
  const pub = hexToBytes('0330d54fd0dd420a6e5f8d3624f5f3482cae350f79d5f0753bf5beef9c2d91af3c');
  const bitcoin = readChainPublic(concat(Uint8Array.of(10, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0), pub,
    hash160(pub), new Uint8Array(4)), {chain: 1, network: 0, account: 0}, 0);
  expect(bitcoin.address).toBe('bc1qcr8te4kr609gcawutmrza0j4xv80jy8z306fyu');
  const xrpl = readChainPublic(concat(Uint8Array.of(10, 3, 1, 0, 0, 0, 0, 0, 0, 0, 0), pub,
    hash160(pub), new Uint8Array(4)), {chain: 3, network: 1, account: 0}, 0);
  expect(receiveCard(xrpl, '0')).toMatchObject({tag: '0', uri: expect.stringContaining('DestinationTag: 0')});
  expect(receiveCard(xrpl, '')).not.toMatchObject({tag: '0'});
  expect(() => receiveCard(xrpl, '4294967296')).toThrow('XRPL_DESTINATION_TAG');
});

test('normal SOL draft obtains a network and lease-bound grant, verifies the signed wire, and resumes by ID', async () => {
  const payer = Uint8Array.from({length: 32}, (_, i) => i + 1);
  const payee = ed25519.getPublicKey(new Uint8Array(32).fill(9));
  const payerKey = ed25519.getPublicKey(payer);
  const account: ChainPublicAccount = {chain: 2, network: 1, account: 0, publicKey: payerKey, address: base58(payerKey)};
  const cluster = base58(new Uint8Array(32).fill(22));
  const blockhash = new Uint8Array(32).fill(33);
  const board = 'ab'.repeat(32), generation = 6n, revision = 7n;
  const authority = new Uint8Array(32).fill(42);
  const head = concat(Uint8Array.of(11, 2, 1, 0, 0, 0, 0), bigToBytes(BigInt(Math.floor(Date.now() / 1000) + 50), 8),
    bigToBytes(5000n, 8), blockhash, bigToBytes(42n, 8), bigToBytes(180n, 8), bigToBytes(5000n, 8),
    new Uint8Array(34), sha256(utf8(cluster)), bigToBytes(revision, 8));
  const raw = concat(hexToBytes('01000103'), payerKey, payee, new Uint8Array(32), blockhash,
    hexToBytes('01020200010c02000000'), bigToBytes(10_000_000n, 8).reverse());
  const statement = concat(utf8('N54M'), hexToBytes(board), bigToBytes(generation, 8), head, sha256(raw));
  const proof = p256.sign(sha256(statement), authority).toCompactRawBytes();
  const packet = hex(concat(head, proof, raw));
  const destination = base58(payee);
  const grantRpc = jest.fn(async () => ({grant: packet}));
  const draft = {destination, amount: '0.01'};
  const trust = p256.getPublicKey(authority, false);
  const transfer = await prepareChainTransfer(grantRpc, account, undefined, draft, cluster, board, generation, revision, trust);
  const stakeProgram=decodePublicBase58('Stake11111111111111111111111111111111111111',32);
  const delegation=concat(hexToBytes('01000507'),payerKey,payee,new Uint8Array(32).fill(61),
    new Uint8Array(32).fill(62),new Uint8Array(32).fill(63),new Uint8Array(32).fill(64),stakeProgram,
    blockhash,hexToBytes('0106060102030405000402000000'));
  const stakeStatement=concat(utf8('N54M'),hexToBytes(board),bigToBytes(generation,8),head,sha256(delegation));
  const stake=hex(concat(head,p256.sign(sha256(stakeStatement),authority).toCompactRawBytes(),delegation));
  const stakeRpc=jest.fn(async()=>({grant:stake}));
  await expect(prepareChainTransfer(stakeRpc,account,undefined,draft,cluster,board,generation,revision,trust))
    .rejects.toThrow('SOL_UNSUPPORTED_STAKE');
  expect(stakeRpc).toHaveBeenCalledTimes(1);
  expect(grantRpc).toHaveBeenCalledWith('nu54_prepareWalletTransfer', [expect.objectContaining(draft)]);
  expect(transfer.review).toMatchObject({sender:account.address,destination,amount:'0.01',asset:'SOL',fee:'5000',
    feeUnit:'lamports',fields:expect.arrayContaining([
      {label:'Instruction',value:'System Program · Transfer'},{label:'Last valid block height',value:'180'},
    ])});

  await expect(prepareChainTransfer(grantRpc, account, undefined, {...draft, amount: '1'}, cluster,
    board, generation, revision, trust)).rejects.toThrow('SOL_DRAFT_MISMATCH');
  await expect(prepareChainTransfer(grantRpc, account, undefined, draft, cluster,
    board, generation, revision + 1n, trust)).rejects.toThrow('CHAIN_GRANT_SCOPE');
  expect(() => verifyChainSigned(transfer, account, Uint8Array.of(0))).toThrow('CHAIN_USER_REFUSED');
  const signed = concat(Uint8Array.of(1), ed25519.sign(raw, payer), raw);
  const result = verifyChainSigned(transfer, account, signed);
  const ownerAddress = '0x' + '54'.repeat(20);
  const verified = {network: '8283', address: ownerAddress, boardFingerprint: board, generation: Number(generation)};
  const checkpoint = {...result, endpoint: 'https://127.0.0.1:8443/solana', anchor: cluster, ownerAddress, ownerGeneration: 6,
    clock: {key: hex(trust), board, revision: revision.toString()}};
  const issueTime = result.expires - 50;
  const nonce = new Uint8Array(32).fill(88);
  const saveAttempt = jest.fn(async () => {});
  expect(readStoredChain(checkpoint)).toBe(true);
  const stored = decodeWalletStorage(JSON.stringify({version: 2, exercised: true, mode: 'personal',
    verified, pendingChain: {...checkpoint, attempted: true}}));
  const rpc: ChainRpc = jest.fn(async (method: string, params: unknown) => {
    if (method === 'nu54_walletTime') {
      expect(params).toMatchObject([{nonce: hex(nonce), board, generation: '6', leaseRevision: '7'}]);
      return timeProof(authority, board, generation, revision, nonce, issueTime);
    }
    if (method === 'getGenesisHash') return cluster;
    if (method === 'isBlockhashValid') return {value: true};
    if (method === 'getBlockHeight') return 60;
    if (method === 'sendTransaction') return checkpoint.id;
    if (method === 'getSignatureStatuses') {
      expect(params).toMatchObject([[checkpoint.id], {searchTransactionHistory: true}]);
      return {value: [{confirmationStatus: 'finalized', err: null}]};
    }
    throw Error('Unexpected RPC call');
  });
  expect(() => loopbackChainRpc('https://example.com', async () => '')).toThrow('CHAIN_LOOPBACK_ENDPOINT');
  await expect(checkChainNetwork(rpc, checkpoint, base58(new Uint8Array(32).fill(8)))).rejects.toThrow('SOL_GENESIS');
  await checkChainNetwork(rpc, checkpoint, checkpoint.anchor);
  expect(await submitChainTransfer(rpc, checkpoint, {challenge: async () => nonce, monotonic: () => 0, saveAttempt}))
    .toBe('finalized');
  expect(rpc).not.toHaveBeenCalledWith('sendTransaction', expect.anything());
  expect(saveAttempt).toHaveBeenCalledWith({...checkpoint, attempted: true});
  expect(await queryChainTransfer(rpc, stored.pendingChain!)).toBe('finalized');
});

test('XRPL confirmation requires validated delivery and exact issuer, value and DestinationTag', async () => {
  const id = 'AB'.repeat(32);
  const pending = {chain: 3 as const, network: 1 as const, account: 0, id, signed: 'ab', attempted: true,
    expires: 1, xrp: {lastLedger: 28, destination: 'rwhKUCPbWBrj7pHZuNy57L8gb7hbfcc7gK',
      destinationTag: 0, amount: {currency: 'USD', issuer: 'rKV75A16S6FfHxwoSPriWWPuzt7qKRdkE1',
        value: '1234567890123456e-15'}}};
  const response = {hash: id, validated: true, Destination: pending.xrp.destination, DestinationTag: 0,
    meta: {TransactionResult: 'tesSUCCESS', delivered_amount: {...pending.xrp.amount, value: '1.234567890123456'}}};
  expect(await queryChainTransfer(async () => response, pending)).toBe('finalized');
  await expect(queryChainTransfer(async () => ({...response, DestinationTag: 1}), pending))
    .rejects.toThrow('XRPL_DELIVERED_AMOUNT');
  await expect(queryChainTransfer(async () => ({...response, meta: {...response.meta,
    delivered_amount: {...response.meta.delivered_amount, value: '1.234567890123455'}}}), pending))
    .rejects.toThrow('XRPL_DELIVERED_AMOUNT');
});


test('Bitcoin watch-only export binds the approved network and account and rejects refusal', () => {
  const profile = {chain: 1 as const, network: 1 as const, account: 1};
  const publicKey = hexToBytes('0330d54fd0dd420a6e5f8d3624f5f3482cae350f79d5f0753bf5beef9c2d91af3c');
  const reply = concat(bitcoinXpubCommand(profile), hexToBytes('1122334455667788'),
    new Uint8Array(32).fill(55), publicKey);
  const descriptors = bitcoinWatchDescriptors(reply, profile);
  expect(descriptors.xpub).toMatch(/^tpub[1-9A-HJ-NP-Za-km-z]+$/);
  expect(descriptors.receive).toBe(checksumDescriptor('wpkh([11223344/84h/1h/1h]' + descriptors.xpub + '/0/*)'));
  expect(descriptors.change).toBe(checksumDescriptor('wpkh([11223344/84h/1h/1h]' + descriptors.xpub + '/1/*)'));
  expect(() => bitcoinWatchDescriptors(reply, {...profile, account: 0})).toThrow('BTC_XPUB_REPLY');
  expect(() => bitcoinWatchDescriptors(Uint8Array.of(0), profile)).toThrow('CHAIN_USER_REFUSED');
});

test('Bitcoin first submit fails on expired, absent or rolled-back trusted time; interrupted submit stays query-only', async () => {
  const authority = new Uint8Array(32).fill(32), board = 'bb'.repeat(32), nonce = new Uint8Array(32).fill(18);
  const anchor = '88'.repeat(32), id = '16'.repeat(32);
  const pending: StoredChain = {chain: 1, network: 1, account: 0, signed: 'abcd', id, attempted: false,
    expires: 160, endpoint: 'https://127.0.0.1:8443/bitcoin', anchor, ownerAddress: '0x' + '42'.repeat(20),
    ownerGeneration: 3, clock: {key: hex(p256.getPublicKey(authority, false)), board, revision: '4'}};
  let signedTimes = [160], calls = 0;
  const saveAttempt = jest.fn(async (_draft: StoredChain) => {});
  const auth = {challenge: async () => nonce, monotonic: () => 200, saveAttempt};
  const node = jest.fn(async (method: string, _args: unknown) => {
    if (method === 'nu54_walletTime') return timeProof(authority, board, 3n, 4n, nonce, signedTimes.shift()!);
    if (method === 'getblockhash') return anchor;
    if (method === 'getblockchaininfo') return {chain: 'regtest'};
    if (method === 'getrawtransaction') return null;
    if (method !== 'sendrawtransaction') throw Error(method);
    calls++;
    throw Error('LOST_RESPONSE');
  });
  await expect(submitChainTransfer(node, pending, auth)).rejects.toThrow('CHAIN_SUBMISSION_EXPIRED');
  await expect(submitChainTransfer(node, {...pending, clock: undefined}, auth)).rejects.toThrow('CHAIN_TRUSTED_TIME_REQUIRED');
  await expect(submitChainTransfer(async () => {throw Error('OFFLINE');}, pending, auth)).rejects.toThrow('OFFLINE');
  signedTimes = [145, 144];
  await expect(submitChainTransfer(node, pending, auth)).rejects.toThrow('CHAIN_TRUSTED_TIME_REQUIRED');
  expect(calls).toBe(0);
  expect(saveAttempt).not.toHaveBeenCalled();
  signedTimes = [140, 141, 160];
  await expect(submitChainTransfer(node, pending, auth)).rejects.toThrow('CHAIN_SUBMISSION_EXPIRED');
  expect(calls).toBe(0);
  expect(saveAttempt).toHaveBeenCalledTimes(1);
  signedTimes = [140, 141, 142];
  await expect(submitChainTransfer(node, pending, auth)).rejects.toThrow('LOST_RESPONSE');
  expect(calls).toBe(1);
  const saved = JSON.parse(JSON.stringify(saveAttempt.mock.lastCall![0])) as StoredChain;
  await expect(submitChainTransfer(node, saved, auth)).rejects.toThrow('CHAIN_SUBMISSION_ATTEMPTED');
  expect(await queryChainTransfer(node, saved)).toBe('pending');
  expect(node).toHaveBeenLastCalledWith('getrawtransaction', [id, true]);
  expect(calls).toBe(1);
});

test('RBF reuses funded prevouts and the exact recipient while reducing only registered change', () => {
  const id = '19'.repeat(32), earlier = '17'.repeat(32);
  const little = (n: bigint, length: number) => bigToBytes(n, length).reverse();
  const prefix = concat(little(2n, 4), Uint8Array.of(0, 1, 2));
  const coin = (txid: string) => concat(hexToBytes(txid), new Uint8Array(5), hexToBytes('fdffffff'));
  const inputs = concat(coin('11'.repeat(32)), coin('12'.repeat(32)));
  const output = (amount: bigint, pattern: string) => concat(little(amount, 8), hexToBytes('160014' + pattern.repeat(20)));
  const receiver = output(100000n, 'ab'), change = output(2000n, 'cd');
  const original: StoredChain = {chain: 1, network: 1, account: 0, attempted: true, id, expires: 160,
    signed: hex(concat(prefix, inputs, Uint8Array.of(2), receiver, change, new Uint8Array(4))),
    endpoint: 'https://127.0.0.1:8443/bitcoin', anchor: '88'.repeat(32),
    ownerAddress: '0x' + '42'.repeat(20), ownerGeneration: 3,
    btc: {fee: 1000, replaceable: true, previous: [earlier]}};
  const restored = JSON.parse(JSON.stringify(original));
  expect(readStoredChain(restored)).toBe(true);
  expect(restored.btc.previous).toEqual([earlier]);
  const draft = bitcoinReplacementDraft(restored, 2000);
  expect(draft).toMatchObject({fee: 2000, amount: '0.00100000', replaces: id});
  expect(() => bitcoinReplacementDraft(restored, 1000)).toThrow('BTC_REPLACEMENT_FEE');
  expect(() => bitcoinReplacementDraft(restored, 3000)).toThrow('BTC_REPLACEMENT_CHANGE');
  const grant = (recipient: Uint8Array, coinInputs = inputs): ChainTransfer => {
    const unsigned = concat(little(2n, 4), Uint8Array.of(2), coinInputs, Uint8Array.of(2), recipient,
      output(1000n, 'cd'), new Uint8Array(4));
    const body = concat(new Uint8Array(15), bigToBytes(2000n, 8), Uint8Array.of(1), hexToBytes(id),
      bigToBytes(1000n, 8), new Uint8Array(49));
    return {chain: 1, network: 1, account: 0, expires: 160, body, raw:
      concat(hexToBytes('70736274ff0100'), Uint8Array.of(unsigned.length), unsigned, Uint8Array.of(0))};
  };
  expect(() => checkBitcoinReplacement(grant(receiver), original, draft)).not.toThrow();
  expect(() => checkBitcoinReplacement(grant(output(100001n, 'ab')), original, draft))
    .toThrow('BTC_REPLACEMENT_CHANGES_INTENT');
  expect(() => checkBitcoinReplacement(grant(receiver, concat(coin('11'.repeat(32)), coin('13'.repeat(32)))), original, draft))
    .toThrow('BTC_REPLACEMENT_CHANGES_INTENT');
});

test('XRPL only expires a missing transaction after validation and complete overlapping history', async () => {
  const pending = {chain: 3 as const, network: 1 as const, account: 0, id: 'AA'.repeat(32), signed: 'ab', attempted: true,
    expires: 1, xrp: {firstLedger: 10, lastLedger: 28, destination: 'rwhKUCPbWBrj7pHZuNy57L8gb7hbfcc7gK', amount: '1000'}};
  const provider = (seq: number, complete_ledgers: string): ChainRpc => async method => method === 'tx' ?
    {error: 'txnNotFound'} : {info: {validated_ledger: {seq}, complete_ledgers}};
  expect(await queryChainTransfer(provider(29, '5-29'), pending)).toBe('expired');
  expect(await queryChainTransfer(provider(28, '5-28'), pending)).toBe('pending');
  expect(await queryChainTransfer(provider(40, '5-20,22-40'), pending)).toBe('pending');
  expect(await queryChainTransfer(provider(40, '11-40'), pending)).toBe('pending');
});

test('Bitcoin, Solana and XRP Ledger return separately finality-scoped confirmed balances', async () => {
  const account = {chain: 1 as const, account: 0, network: 1 as const, address: 'bound-address',
    publicKey: new Uint8Array(33)};
  const anchor = '11'.repeat(32);
  const bitcoin: ChainRpc = async (method, params) => {
    if (method === 'getblockhash') return anchor;
    if (method === 'getblockchaininfo') return {chain: 'regtest', blocks: 101};
    if (method === 'getaddressinfo') return {address: account.address, iswatchonly: true};
    expect(params).toEqual([6, 9999999, [account.address]]);
    return [{amount: '0.00000546', confirmations: 7, address: account.address, txid: 'ab'.repeat(32), vout: 1}];
  };
  await expect(readChainReceiveBalance(bitcoin, account, anchor)).resolves.toMatchObject({amount: '546', unit: 'sats'});
  const cluster = base58(new Uint8Array(32).fill(4));
  await expect(readChainReceiveBalance(async (method, params) => {
    if (method === 'getGenesisHash') return cluster;
    expect(params).toEqual([account.address, {commitment: 'finalized'}]);
    return {value: 4500000, context: {slot: 288}};
  }, {...account, chain: 2}, cluster)).resolves.toMatchObject({amount: '4500000', unit: 'lamports'});
  const ledger_hash = 'C1'.repeat(32);
  await expect(readChainReceiveBalance(async (method, params) => {
    if (method === 'server_info') return {info: {network_id: 1, validated_ledger: {hash: ledger_hash, seq: 17}}};
    if (method === 'ledger') return {validated: true, ledger_hash, ledger_index: '17'};
    expect(params).toEqual([{account: account.address, ledger_hash}]);
    return {validated: true, ledger_hash, ledger_index: 17, account_data: {Account: account.address, Balance: '5000'}};
  }, {...account, chain: 3}, '1')).resolves.toMatchObject({status: 'active', amount: '5000', unit: 'drops'});
});

test('Bitcoin RBF submits only for the exact live original and preserves the attempted hash after a late race', async () => {
  const authority = new Uint8Array(32).fill(32), board = 'bb'.repeat(32), nonce = new Uint8Array(32).fill(18);
  const original = '11'.repeat(32), anchor = '88'.repeat(32), id = '16'.repeat(32), grant = '47'.repeat(32);
  const checkpoint: StoredChain = {chain: 1, network: 1, account: 6, signed: 'abcd', id, attempted: false,
    expires: 160, endpoint: 'https://127.0.0.1:8443/bitcoin', anchor, ownerAddress: '0x' + '42'.repeat(20),
    ownerGeneration: 3, clock: {key: hex(p256.getPublicKey(authority, false)), board, revision: '4'},
    btc: {fee: 2000, replaceable: true, previous: [original], grantHash: grant}};
  let states: (string | null)[], received = 0;
  const saveAttempt = jest.fn(async (_draft: StoredChain) => {});
  const auth = {challenge: async () => nonce, monotonic: () => 200, saveAttempt};
  const rpc = jest.fn(async (method: string, params: unknown) => {
    if (method === 'nu54_walletTime') return timeProof(authority, board, 3n, 4n, nonce, 140);
    if (method === 'getblockhash') return anchor;
    if (method === 'getblockchaininfo') return {chain: 'regtest'};
    if (method === 'getrawtransaction') return null;
    if (method === 'nu54_bitcoinReplacementStatus') {
      expect(params).toEqual([original]);
      const state = states.shift();
      return state === null ? null : {id: state === 'foreign' ? id : original, state};
    }
    if (method === 'testmempoolaccept') return [{allowed: true, txid: id}];
    expect(method).toBe('nu54_submitBitcoinReplacement');
    expect(params).toEqual([{original, id, signed: checkpoint.signed, grant, anchor}]);
    received++;
    return id;
  });
  for (const state of ['absent', 'conflicted', 'confirmed', 'foreign', null]) {
    states = [state];
    await expect(submitChainTransfer(rpc, checkpoint, auth)).rejects.toThrow();
  }
  expect(received).toBe(0);
  expect(saveAttempt).not.toHaveBeenCalled();
  states = ['live', 'absent'];
  await expect(submitChainTransfer(rpc, checkpoint, auth)).rejects.toThrow('BTC_ORIGINAL_NOT_IN_MEMPOOL');
  expect(received).toBe(0);
  const restored = JSON.parse(JSON.stringify(saveAttempt.mock.lastCall![0])) as StoredChain;
  expect(restored.account).toBe(6);
  expect(restored.btc?.previous).toEqual([original]);
  await expect(submitChainTransfer(rpc, restored, auth)).rejects.toThrow('CHAIN_SUBMISSION_ATTEMPTED');
  expect(await queryChainTransfer(rpc, restored)).toBe('pending');
  expect(rpc).toHaveBeenLastCalledWith('getrawtransaction', [id, true]);
  states = ['live', 'live'];
  await expect(submitChainTransfer(rpc, checkpoint, auth)).resolves.toBe('pending');
  expect(received).toBe(1);
});

test('XRPL treats actNotFound as inactive only at the exact independently validated ledger', async () => {
  const account = {chain: 3 as const, account: 1, network: 1 as const,
    address: 'rwhKUCPbWBrj7pHZuNy57L8gb7hbfcc7gK', publicKey: new Uint8Array(33)};
  const hash = 'AB'.repeat(32), other = 'CD'.repeat(32), height = 22470955;
  let network = 1, accountHash: string | undefined = hash, accountValidated = true, checkedHash = hash;
  const rpc = jest.fn(async (method: string, params: unknown) => {
    if (method === 'server_info') return {info: {network_id: network, validated_ledger: {hash, seq: height}}};
    if (method === 'ledger') {
      expect(params).toEqual([{ledger_hash: hash, transactions: false}]);
      return {ledger_hash: checkedHash, ledger_index: String(height), validated: true};
    }
    expect(method).toBe('account_info');
    expect(params).toEqual([{account: account.address, ledger_hash: hash}]);
    return {error: 'actNotFound', ledger_hash: accountHash, ledger_index: height, validated: accountValidated,
      request: {account: account.address, ledger_hash: hash}};
  });
  await expect(readChainReceiveBalance(rpc, account, '1')).resolves.toMatchObject({status: 'inactive', ledgerHash: hash});
  accountHash = undefined;  // An echoed hash is not evidence of the ledger used.
  await expect(readChainReceiveBalance(rpc, account, '1')).rejects.toThrow('XRPL_BALANCE_LEDGER');
  accountHash = hash;
  checkedHash = other;
  await expect(readChainReceiveBalance(rpc, account, '1')).rejects.toThrow('XRPL_BALANCE_LEDGER');
  checkedHash = hash;
  accountValidated = false;
  await expect(readChainReceiveBalance(rpc, account, '1')).rejects.toThrow('XRPL_BALANCE_LEDGER');
  network = 0;
  await expect(readChainReceiveBalance(rpc, account, '1')).rejects.toThrow('XRPL_NETWORK');
});

test('BIP380 known descriptor checksum', () => expect(checksumDescriptor('raw(deadbeef)')).toBe('raw(deadbeef)#89f8spxm'));

test('Bitcoin Core JSON-RPC 2.0 absent transaction has an error without a result',async()=>{
  const exchange=jest.fn(async()=>JSON.stringify({jsonrpc:'2.0',id:1,error:{code:-5,message:'No such mempool or blockchain transaction'}}));
  const rpc=loopbackChainRpc('https://127.0.0.1:8443/bitcoin',exchange);
  await expect(rpc('getrawtransaction',['78'.repeat(32),true])).resolves.toBeNull();
  await expect(rpc('getblockhash',[0])).rejects.toThrow('CHAIN_RPC_ERROR');
  exchange.mockResolvedValueOnce(JSON.stringify({id:1,error:{code:-5},result:null}));
  await expect(rpc('getrawtransaction',['78'.repeat(32),true])).resolves.toBeNull();
  exchange.mockResolvedValueOnce(JSON.stringify({jsonrpc:'2.0',id:1}));
  await expect(rpc('getrawtransaction',['78'.repeat(32),true])).rejects.toThrow('CHAIN_RPC_REPLY');
  exchange.mockResolvedValueOnce(JSON.stringify({jsonrpc:'2.0',id:2,error:{code:-5}}));
  await expect(rpc('getrawtransaction',['78'.repeat(32),true])).rejects.toThrow('CHAIN_RPC_REPLY');
  exchange.mockResolvedValueOnce(JSON.stringify({jsonrpc:'2.0',id:1,error:{code:-5},result:{txid:'78'.repeat(32)}}));
  await expect(rpc('getrawtransaction',['78'.repeat(32),true])).rejects.toThrow('CHAIN_RPC_ERROR');
});
