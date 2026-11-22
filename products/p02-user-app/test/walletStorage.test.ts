import evmFixtures from './fixtures/w28-local-evm.json';
import {utf8Encode,bigToBytes,bytesToHex,concat,hexToBytes} from '@nu54/protocol';
import {defaultReadState, observationScope, readContact} from '../src/wallet/readProfiles';
import {decodeWalletStorage, type WalletStorage} from '../src/wallet/profile';
import {topicOf} from '../src/wallet/dapp/walletConnectCrypto';
import {WalletProfileStore, encodeWalletProfile, persistWalletSignature, profileLimit} from '../src/wallet/walletStorage';

const verified = {boardFingerprint: 'ba'.repeat(32), address: evmFixtures[0].sender, network: '8283' as const, generation: 4};
const base: WalletStorage = {version: 2, mode: 'personal', exercised: true, verified};

const evmFixture=evmFixtures[0];
const evmContext={request:'56'.repeat(16),unsigned:'0x'+evmFixture.unsigned,chain:'8283' as const,genesis:evmFixture.genesis,signer:verified.address};
const evmWire=bytesToHex(concat(Uint8Array.of(9),bigToBytes(1234567n,8),bigToBytes(8283n,8),bigToBytes(300000n,8),new Uint8Array(8+20),hexToBytes(evmFixture.genesis.slice(2)),bigToBytes(1n,8),bigToBytes(1n,8),new Uint8Array(64),hexToBytes(evmFixture.unsigned)),false);
const signedEvm={chain:'8283' as const,sender:verified.address,raw:evmFixture.raw,hash:evmFixture.hash,expires:1234567,submitted:false,genesis:evmFixture.genesis,profileRevision:1,leaseRevision:'1',evidence:{board:verified.boardFingerprint,generation:verified.generation,request:evmContext.request,unsigned:evmContext.unsigned}};

function device(initial: string) {
  let raw = initial;
  let fail = '';
  return {
    loadProfile: async () => raw,
    fail: (when: string) => {fail = when;},
    compareAndSaveProfile: async (expected: string, value: string) => {
      if (raw !== expected) return false;
      if (fail === 'before') throw Error('DISK_FULL');
      raw = value;
      if (fail === 'after') throw Error('REPLY_LOST');
      return true;
    },
  };
}

test('valid Korean contacts survive UTF-8 overflow and only cached observations shrink', () => {
  const contacts = Array.from({length: 30}, (_, i) => readContact('eth', '여행 경비 비상 연락처 ' + i,
    '0x' + (i + 1).toString(16).padStart(40, '0')));
  const account = {id: 'eth' as const, hidden: false, account: 0};
  const observations = Array.from({length: 24}, (_, i) => ({address: verified.address, time: 25 - i,
    scope: observationScope(verified.boardFingerprint, 4, {...account, asset: contacts[i].address}),
    unit: 'ETH', amount: '1000', decimals: 18, point: 'Finalized block ' + ('f'.repeat(64))}));
  const proposed = {...base, walletRead: {...defaultReadState, contacts, observations}};
  expect(utf8Encode(JSON.stringify(proposed)).length).toBeGreaterThan(8192);
  const ready = decodeWalletStorage(encodeWalletProfile(proposed));
  expect(ready.walletRead!.contacts).toEqual(contacts);
  expect(ready.verified).toEqual(verified);
  expect(ready.walletRead!.observations.length).toBeLessThan(24);
  expect(ready.walletRead!.observations[0]).toEqual(observations[0]);
  expect(utf8Encode(encodeWalletProfile(ready)).length).toBeLessThan(profileLimit);
});

test('cross-instance CAS rebases disjoint changes and rejects stale edits or owner generation', async () => {
  const bridge = device(JSON.stringify(base));
  const left = new WalletProfileStore(bridge), right = new WalletProfileStore(bridge);
  const [a, b] = await Promise.all([left.load(), right.load()]);
  await Promise.all([left.save(a, {...a, language: 'en'}), right.save(b, {...b, exercised: false})]);
  expect(await left.load()).toMatchObject({language: 'en', exercised: false});
  await expect(right.save(b, {...b, language: 'ko'})).rejects.toThrow('WALLET_STORAGE_CONFLICT');
  const latest = await left.load();
  await left.save(latest, {...latest, verified: {...verified, generation: 5}});
  await expect(right.save(latest, {...latest, exercised: true})).rejects.toThrow('WALLET_STORAGE_CONFLICT');
});

test('failure before commit retains legacy v2; a lost completion recovers exact committed result', async () => {
  const bridge = device(JSON.stringify(base, null, 2));
  const running = new WalletProfileStore(bridge);
  const v2 = await running.load();
  bridge.fail('before');
  await expect(running.save(v2, {...v2, language: 'en'})).rejects.toThrow('DISK_FULL');
  expect(await running.load()).toEqual(base);
  bridge.fail('after');
  const committed = await running.save(v2, {...v2, language: 'en'});
  expect(await new WalletProfileStore(bridge).load()).toEqual(committed);
});

const pendingEvm = {chain: '8283' as const, sender: verified.address, raw: '0x02' + 'a1'.repeat(322),
  hash: '0x' + 'b2'.repeat(32), expires: 1234567, submitted: true};
const pendingChain = {chain: 1 as const, network: 1 as const, account: 0, signed: 'ca'.repeat(1232), id: 'bd'.repeat(32),
  attempted: true, expires: 1234567, endpoint: 'https://127.0.0.1:18448', anchor: 'ef'.repeat(32),
  ownerAddress: verified.address, ownerGeneration: verified.generation};
const sessionKey = '69'.repeat(32);
const walletConnect: NonNullable<WalletStorage['walletConnect']> = {
  relay: 'wss://relay.example',
  seed: '56'.repeat(32),
  checkpoint: {
    topic: topicOf(hexToBytes(sessionKey)), key: sessionKey, expires: 1234567, lastId: 0,
    scope: {board: verified.boardFingerprint, account: verified.address, generation: verified.generation,
      chain: 8283, genesis: evmFixture.genesis},
    proposal: {chain: 'eip155:8283', account: 'eip155:8283:' + verified.address, origin: 'https://dapp.example',
      peerKey: '45'.repeat(32), pairingTopic: '67'.repeat(32), verified: false,
      methods: ['eth_accounts'], events: []},
  },
};

test('stale observer saves preserve exact attempts, prior owner generations and WalletConnect checkpoint', async () => {
  const bridge = device(JSON.stringify(base));
  const store = new WalletProfileStore(bridge);
  const a = await store.load();
  await store.save(a, {...a, pendingEvm, pendingChain, walletConnect});
  const contact = readContact('eth', '주거래 주소', verified.address);
  const updated = await store.save(a, {...a, walletRead: {...defaultReadState, contacts:[contact]}});
  expect(updated).toMatchObject({pendingEvm, pendingChain, walletConnect});
  expect(await new WalletProfileStore(bridge).load()).toEqual(updated);
  await expect(store.save(a, {...a, pendingEvm: {...pendingEvm, submitted: false}})).rejects.toThrow('WALLET_STORAGE_CONFLICT');
});

test('a crash after approval but before signed-wire commit keeps the reservation and bars a second approval', async () => {
  const bridge = device(JSON.stringify({...base, pendingChain, walletConnect}));
  const store = new WalletProfileStore(bridge);
  const snapshot = await store.load();
  const approve = jest.fn(async () => {
    bridge.fail('before');
    return {pendingEvm: signedEvm};
  });
  const commit = (next: WalletStorage, before: WalletStorage) => store.save(before, next);
  await expect(persistWalletSignature(snapshot, 'evm', evmWire, 1234567, commit, approve,evmContext)).rejects.toThrow('DISK_FULL');
  const restored = await new WalletProfileStore(bridge).load();
  expect(restored).toMatchObject({signing: {kind: 'evm', generation: 4}, pendingChain, walletConnect});
  bridge.fail('');
  await expect(persistWalletSignature(restored, 'evm', evmWire, 1234567, commit, approve,evmContext))
    .rejects.toThrow('WALLET_SIGNING_UNRESOLVED');
  expect(approve).toHaveBeenCalledTimes(1);
});

test('concurrent writers only call board approval once and keep the acknowledged signed raw', async () => {
  const bridge = device(JSON.stringify(base));
  const store = new WalletProfileStore(bridge);
  const snapshot = await store.load();
  const approve = jest.fn(async () => ({pendingEvm: signedEvm}));
  const reserve = () => persistWalletSignature(snapshot, 'evm', evmWire, 1234567, (next, before) => store.save(before, next), approve,evmContext);
  const results = await Promise.allSettled([reserve(), reserve()]);
  expect(results.map(result => result.status).sort()).toEqual(['fulfilled', 'rejected']);
  expect(approve).toHaveBeenCalledTimes(1);
  expect(await store.load()).toEqual({...base, pendingEvm: signedEvm});
});

test('authenticated owner change preserves old signed wires and forces their recovered attempts query-only', async () => {
  const notSent = {...pendingEvm, submitted: false};
  const notAttempted = {...pendingChain, attempted: false};
  const store = new WalletProfileStore(device(JSON.stringify({...base, pendingEvm: notSent, pendingChain: notAttempted})));
  const before = await store.load();
  const changed = await store.save(before, {...before, verified: {...verified, generation: 5}});
  expect(changed).toMatchObject({pendingEvm, pendingChain, verified: {...verified, generation: 5}});
  await expect(store.load()).resolves.toEqual(changed);
});

test('hard UTF-8 limit keeps the previous committed record and does not truncate durable fields', async () => {
  const bridge = device(JSON.stringify(base));
  const store = new WalletProfileStore(bridge);
  const at = await store.load();
  const opaque = {device: verified.boardFingerprint, ca: ''} as NonNullable<WalletStorage['enrollment']>;
  const shell = {...at, enrollment: opaque};
  const fill = 'x'.repeat(profileLimit - utf8Encode(JSON.stringify(shell)).length);
  const maximum = {...shell, enrollment: {...opaque, ca: fill}};
  const limit = await store.save(at, maximum);
  expect(utf8Encode(encodeWalletProfile(limit)).length).toBe(profileLimit);
  await expect(store.save(limit, {...limit, enrollment: {...opaque, ca: fill + 'é'}})).rejects.toThrow('WALLET_STORAGE_FULL');
  await expect(store.load()).resolves.toEqual(limit);
});

test('an authenticated refusal releases the reservation; transport failures retain it', async () => {
  const store = new WalletProfileStore(device(JSON.stringify(base)));
  const before = await store.load();
  const commit = (next: WalletStorage, previous: WalletStorage) => store.save(previous, next);
  await expect(persistWalletSignature(before, 'chain', 'authority grant', 1234567, commit, async () => {
    throw Error('CHAIN_USER_REFUSED');
  })).rejects.toThrow('CHAIN_USER_REFUSED');
  await expect(store.load()).resolves.toEqual(base);
  await expect(persistWalletSignature(before, 'chain', 'authority grant', 1234567, commit, async () => {
    throw Error('DISCONNECTED');
  })).rejects.toThrow('DISCONNECTED');
  await expect(store.load()).resolves.toMatchObject({signing: {kind: 'chain'}});
});

test('two delayed expiry replies cannot unlock a subsequent grant or erase a signed wire', async () => {
  const original = {kind:'evm' as const, digest:'af'.repeat(32), board:verified.boardFingerprint,
    generation:verified.generation, expires:1234};
  const store = new WalletProfileStore(device(JSON.stringify({...base, signing: original})));
  const snapshot = await store.load();
  const next = await store.save(snapshot, {...snapshot, signing:undefined});
  const other = {...original, digest:'bf'.repeat(32), expires:1235};
  const reserved = await store.save(next, {...next, signing:other});
  await expect(store.save(snapshot, {...snapshot, signing:undefined})).rejects.toThrow('WALLET_STORAGE_CONFLICT');
  expect(await store.load()).toEqual(reserved);
  const approved = await store.save(reserved,{...reserved, signing:undefined, pendingEvm});
  await expect(store.save(snapshot, {...snapshot, signing:undefined})).rejects.toThrow('WALLET_STORAGE_CONFLICT');
  expect(await store.load()).toEqual(approved);
});

test('older profile reader leaves a retained newer schema and its pending approval untouched', async () => {
  const raw = JSON.stringify({...base, version: 3, pendingEvm,
    pendingAuthorization: {status: 'unknown-outcome', generation: verified.generation}});
  const bridge = device(raw);
  const write = jest.spyOn(bridge, 'compareAndSaveProfile');
  const priorApp = new WalletProfileStore(bridge);

  await expect(priorApp.load()).rejects.toThrow('WALLET_STORAGE_CORRUPT');
  await expect(priorApp.save(base, {...base, language: 'en'})).rejects.toThrow('WALLET_STORAGE_CORRUPT');

  expect(write).not.toHaveBeenCalled();
  expect(await bridge.loadProfile()).toBe(raw);
});

test('a stale pre-upgrade reader cannot revive a consumed signing reservation', async () => {
  const reservation = {kind: 'evm' as const, digest: 'd1'.repeat(32), board: verified.boardFingerprint,
    generation: verified.generation, expires: 9876543};
  const bridge = device(JSON.stringify({...base, signing: reservation}));
  const priorApp = new WalletProfileStore(bridge);
  const before = await priorApp.load();
  const update = new WalletProfileStore(bridge);
  await update.save(before, {...before, signing: undefined, pendingEvm});
  const recoveredApp = new WalletProfileStore(bridge);
  await expect(recoveredApp.save(before, {...before, signing: {...reservation, expires: 9876544}}))
    .rejects.toThrow('WALLET_STORAGE_CONFLICT');
  expect(await recoveredApp.load()).toEqual({...base, pendingEvm});
});

test('EVM request identity and exact original are durable before any approval I/O',async()=>{
 const f=evmFixtures[0];
 const store=new WalletProfileStore(device(JSON.stringify(base)));const before=await store.load();
 const context={request:'56'.repeat(16),unsigned:'0x'+f.unsigned,chain:'8283' as const,genesis:f.genesis,signer:verified.address};
 const approve=jest.fn(async()=>{expect((await store.load()).signing).toMatchObject({evm:context});throw Error('REPLY_LOST');});
 await expect(persistWalletSignature(before,'evm',evmWire,1234567,(next,old)=>store.save(old,next),approve,context)).rejects.toThrow('REPLY_LOST');
 const recovered=await store.load();expect(recovered.signing?.evm).toEqual(context);
 await expect(persistWalletSignature(recovered,'evm',evmWire,1234567,(next,old)=>store.save(old,next),approve,context)).rejects.toThrow('WALLET_SIGNING_UNRESOLVED');
 expect(approve).toHaveBeenCalledTimes(1);
});

test('mismatched EVM identity or original fails before the board can be called',async()=>{
 const store=new WalletProfileStore(device(JSON.stringify(base))),before=await store.load();const approve=jest.fn(async()=>({pendingEvm:signedEvm}));
 for(const context of [{...evmContext,request:'00'.repeat(16)},{...evmContext,signer:'0x'+'ab'.repeat(20)},
   {...evmContext,unsigned:evmContext.unsigned.slice(0,-2)+'01'},{...evmContext,genesis:'0x'+'aa'.repeat(32)}]){
  await expect(persistWalletSignature(before,'evm',evmWire,1234567,(next,old)=>store.save(old,next),approve,context)).rejects.toThrow();
 }
 expect(approve).not.toHaveBeenCalled();expect(await store.load()).toEqual(before);
});
