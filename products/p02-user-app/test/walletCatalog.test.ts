import {accountPreference, registerReviewedAsset, reviewEvmAsset, saveAccountPreference, validAccountPreference} from '../src/wallet/walletCatalog';
import {defaultReadState, readProfiles} from '../src/wallet/readProfiles';
import {decodeWalletStorage} from '../src/wallet/profile';
const owner = {boardFingerprint:'ab'.repeat(32), generation:1};
const selection = {id: 'eth' as const, account:0, hidden:false};
const token = '0x' + '13'.repeat(20);
const hash = '0x' + '42'.repeat(32);
const endpoint = 'https://rpc.example/';
const profile = {...defaultReadState, selection, endpoints: {eth:endpoint}};

test('W25 retains independent account names including earlier verified owners', () => {
  const initial = saveAccountPreference([], owner, selection, '여행', true);
  const nextOwner = {...owner, generation:2};
  const records = saveAccountPreference(initial, nextOwner, selection, '새 지갑', false);
  expect(accountPreference(records, owner, selection)?.name).toBe('여행');
  expect(accountPreference(records, nextOwner, selection)?.name).toBe('새 지갑');
  expect(accountPreference(records, nextOwner, {...selection,id:'base'})).toBeUndefined();
  const deleted=saveAccountPreference(records,owner,selection,undefined,true);
  expect(accountPreference(deleted,owner,selection)).toMatchObject({hidden:true});
  expect(accountPreference(deleted,owner,selection)?.name).toBeUndefined();
  expect(deleted.every(validAccountPreference)).toBe(true);
  const restored=decodeWalletStorage(JSON.stringify({version:2,mode:'personal',exercised:true,
    verified:{...owner,address:'0x'+'11'.repeat(20),network:'8283'},walletRead:{...defaultReadState,accounts:deleted}}));
  expect(accountPreference(restored.walletRead?.accounts,owner,selection)).toMatchObject({hidden:true});
  expect(accountPreference(deleted,nextOwner,selection)?.name).toBe('새 지갑');
  expect(accountPreference(saveAccountPreference(deleted,owner,selection,undefined,false),owner,selection)).toBeUndefined();
});

test('metadata review checks chain and finalized height, sends only contract, and can only register in the exact scope', async () => {
  const rpc = jest.fn(async (method, params) => {
    if (method === 'eth_chainId') return '0xaa36a7';
    if (method === 'eth_getBlockByNumber' && params[0] === '0x0') return {hash:readProfiles.eth.anchor};
    if (method === 'eth_getBlockByNumber') return {hash,number:'0x32'};
    if (method === 'eth_call') return '0x' + '0'.repeat(63) + '6';
    throw new Error('unexpected');
  });
  const asset = await reviewEvmAsset(rpc, owner, selection, endpoint, token, 6);
  expect(asset).toMatchObject({endpoint, contract:token, decimals:6, block:hash});
  expect(rpc.mock.calls.at(-1)).toEqual(['eth_call',[{to:token,data:'0x313ce567'},{blockHash:hash,requireCanonical:true}]]);
  expect(() => registerReviewedAsset(profile,{...owner,generation:2},asset)).toThrow('WALLET_ASSET_SCOPE');
  expect(() => registerReviewedAsset({...profile,endpoints:{eth:'https://other.example/'}},owner,asset)).toThrow('WALLET_ASSET_SCOPE');
  expect(registerReviewedAsset(profile,owner,asset).selection.asset).toBe(token);
  await expect(reviewEvmAsset(rpc,owner,selection,endpoint,token,18)).rejects.toThrow('WALLET_ASSET_DECIMALS');
});
