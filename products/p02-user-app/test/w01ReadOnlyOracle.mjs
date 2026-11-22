// Explicit QA run with disposable synthetic accounts. No signing or submission.
import assert from 'node:assert/strict';
import {randomBytes} from 'node:crypto';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {secp256k1} from '@noble/curves/secp256k1';
import {base58, classicXrpl, hash160, segwitAddress} from '../src/wallet/chainAccounts.ts';
import {decodeWalletStorage} from '../src/wallet/profile.ts';
import {defaultReadState, observationScope, readProfiles, readSelectedBalance,
  receiveAddressForProfile, walletReadRpc} from '../src/wallet/readProfiles.ts';
import {publicSignetIndexer, signetIndexer} from '../src/wallet/signetIndexer.ts';

const exec = promisify(execFile);
const urls = {
  stable: 'https://api.test.stablenet.network/',
  eth: 'https://ethereum-sepolia-rpc.publicnode.com/',
  base: 'https://base-sepolia.gateway.tenderly.co/',
  btc: publicSignetIndexer,
  sol: 'https://api.devnet.solana.com/',
  xrp: 'https://s.altnet.rippletest.net:51234/',
};

function syntheticAddress(id) {
  const profile = readProfiles[id];
  if ('evm' in profile) return '0x' + randomBytes(20).toString('hex');
  const publicKey = profile.chain === 2 ? randomBytes(32) : secp256k1.getPublicKey(randomBytes(32), true);
  const address = profile.chain === 1 ? segwitAddress('tb', Uint8Array.of(0, 20, ...hash160(publicKey))) :
    profile.chain === 2 ? base58(publicKey) : classicXrpl(hash160(publicKey));
  return {chain:profile.chain, network:1, account:0, address, publicKey};
}

async function run(id) {
  const selection = {id, account:0, hidden:false};
  const account = syntheticAddress(id);
  const owner = {boardFingerprint:'24'.repeat(32), generation:1, network:'8283',
    address:typeof account === 'string' ? account : '0x' + randomBytes(20).toString('hex')};
  const scope = observationScope(owner.boardFingerprint, owner.generation, selection);
  let online = true, sent = 0;
  const legal = new Set(['eth_chainId','eth_getBlockByNumber','eth_getBalance','getGenesisHash',
    'getBalance','server_info','ledger','account_info','nu54_signetRead']);
  const exchange = async (url, packet) => {
    const {method} = JSON.parse(packet);
    assert.ok(legal.has(method), 'read-only RPC method');
    const get = method === 'nu54_signetRead';
    assert.ok(get ? url.startsWith(publicSignetIndexer + '/') : url === urls[id]);
    if (!online) throw new Error('synthetic transport unavailable');
    sent++;
    const args = ['-fsS','--proto','=https','--max-time','15'];
    if (!get) args.push('-H','Content-Type: application/json','--data-binary',packet);
    try {
      const {stdout} = await exec('curl', [...args,url], {maxBuffer:512000});
      return stdout;
    } catch {
      throw new Error('HTTPS provider not ready');
    }
  };
  const query = () => readSelectedBalance(walletReadRpc(urls[id],exchange),selection,account,
    id === 'btc' ? signetIndexer(exchange) : undefined);
  const first = await query();
  const snapshot = {...first,scope,time:Date.now()};
  let storage = {version:2,exercised:true,mode:'personal',verified:owner,
    walletRead:{...defaultReadState,selection,endpoints:{[id]:urls[id]},observations:[snapshot]}};
  const cached = () => decodeWalletStorage(JSON.stringify(storage)).walletRead.observations[0];
  online = false;
  const prior = sent;
  await assert.rejects(query, /synthetic transport unavailable/);
  assert.equal(sent,prior);
  assert.deepEqual(cached(),snapshot);
  online = true;
  assert.deepEqual(cached(),snapshot);
  const fresh = await query();
  assert.equal(fresh.address,receiveAddressForProfile(selection,account));
  assert.notEqual(observationScope(owner.boardFingerprint, owner.generation + 1, selection),scope);
  assert.ok(fresh.point);
  if (id === 'xrp') assert.equal(fresh.inactive,true);
  else assert.equal(fresh.amount,'0');
  const next = {...fresh,scope,time:Date.now()};
  assert.ok(next.time > snapshot.time);
  storage = {...storage,walletRead:{...storage.walletRead,observations:[next]}};
  assert.deepEqual(cached(),next);
  return {id,point:fresh.point,status:fresh.inactive?'inactive':'zero',cachedWhileOffline:true,finalizedReadAfterReconnect:true};
}

if (process.argv.length !== 3 || process.argv[2] !== '--live')
  throw new Error('Use --live to query public test-network providers with synthetic addresses');
const results = await Promise.allSettled(Object.keys(urls).map(run));
for (const [index, result] of results.entries()) {
  if (result.status === 'fulfilled') console.log(JSON.stringify(result.value));
  else console.error(Object.keys(urls)[index] + ': ' + result.reason.message);
}
if (results.some(result => result.status === 'rejected')) process.exitCode = 1;
