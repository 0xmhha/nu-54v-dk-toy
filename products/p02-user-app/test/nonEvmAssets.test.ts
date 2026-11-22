import {toBase64} from '@nu54/protocol';
import {base58} from '../src/wallet/chainAccounts';
import {classicTokenProgram, readSolanaMint, readSolanaTokenBalance, readXrplIssuer, readXrplIssuedBalance, xrplIssuedValue, xrplIssue} from '../src/wallet/nonEvmAssets';
import {registerNonEvmAsset} from "../src/wallet/nonEvmAssetCatalog";
import {defaultReadState, isWalletReadState} from "../src/wallet/readProfiles";


const mint = new Uint8Array(32).fill(2), owner = new Uint8Array(32).fill(3);
const mintId = base58(mint), ownerId = base58(owner);
const genesis = 'EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG';

test('Solana accepts only finalized classic SPL layouts and exact 64-bit token balances', async () => {
  const bytes = new Uint8Array(82);
  bytes[44]=6; bytes[45]=1;
  const mintState = {owner:classicTokenProgram,executable:false,data:[toBase64(bytes),'base64']};
  const coin = new Uint8Array(165);
  coin.set(mint); coin.set(owner,32);
  new DataView(coin.buffer).setBigUint64(64,9007199254740993n,true);
  coin[108]=1;
  const tokenState = {owner:classicTokenProgram,executable:false,data:[toBase64(coin),'base64']};
  let badProgram = false, duplicate = false;
  const rpc = jest.fn(async (method, params) => {
    if (method === 'getGenesisHash') return genesis;
    if (method === 'getAccountInfo') {
      expect(params).toEqual([mintId,{encoding:'base64',commitment:'finalized'}]);
      return {context:{slot:91},value:{...mintState,owner:badProgram ? ownerId : classicTokenProgram}};
    }
    expect(method).toBe('getTokenAccountsByOwner');
    expect(params).toEqual([ownerId,{mint:mintId},{encoding:'base64',commitment:'finalized',minContextSlot:91}]);
    const value={pubkey:base58(new Uint8Array(32).fill(4)),account:tokenState};
    return {context:{slot:92},value:duplicate ? [value,value] : [value]};
  });
  const verified = await readSolanaMint(rpc,genesis,mintId,6);
  expect(verified).toEqual({mint:mintId,decimals:6,slot:91});
  await expect(readSolanaTokenBalance(rpc,genesis,verified,ownerId)).resolves.toMatchObject({amount:'9007199254740993',slot:92});
  duplicate=true;
  await expect(readSolanaTokenBalance(rpc,genesis,verified,ownerId)).rejects.toThrow('SOL_TOKEN_ACCOUNT');
  badProgram=true;
  await expect(readSolanaMint(rpc,genesis,mintId)).rejects.toThrow('SOL_TOKEN_PROGRAM');
});

test('XRPL exact issued values distinguish signed debt, exponent, currency and issuer', () => {
  const classic='rGWrZyQqhTp9Xu7G5Pkayo7bXjH4k4QYpf';
  expect(xrplIssue('USD',classic)).toEqual({currency:'USD',issuer:classic});
  expect(() => xrplIssue('XRP',classic)).toThrow('XRPL_ISSUE');
  expect(xrplIssuedValue('-1.000000000000001e-4')).toBe('-0.0001000000000000001');
  expect(xrplIssuedValue('9007199254740993')).toBe('9007199254740993');
  expect(() => xrplIssuedValue('0.12345678901234567')).toThrow('XRPL_ISSUED_AMOUNT');
});

test('XRPL trust line requires exact peer, currency and validated owner ledger', async () => {
  const issuer='rGWrZyQqhTp9Xu7G5Pkayo7bXjH4k4QYpf';
  const owner='rwhKUCPbWBrj7pHZuNy57L8gb7hbfcc7gK';
  const proof={ledger_hash:'AA'.repeat(32),ledger_index:112,validated:true};
  let unproven=false;
  const rpc=jest.fn(async (method, params) => {
    if (method === 'server_info') return {info:{network_id:1,validated_ledger:{hash:proof.ledger_hash,seq:proof.ledger_index}}};
    if (method === 'ledger') return proof;
    if (method === 'account_info') return {...proof,account_data:{Account:params[0].account,Balance:'1000000'}};
    expect(params).toEqual([{account:owner,peer:issuer,ledger_hash:proof.ledger_hash,limit:50}]);
    return {...proof,account:owner,validated:!unproven,lines:[{account:issuer,currency:'USD',balance:'-2.50e-2'}]};
  });
  await expect(readXrplIssuer(rpc,'1','USD',issuer)).resolves.toMatchObject({issuer,ledgerIndex:112});
  await expect(readXrplIssuedBalance(rpc,'1','USD',issuer,owner)).resolves.toMatchObject({status:'active',value:'-0.025'});
  await expect(readXrplIssuedBalance(rpc,'1','EUR',issuer,owner)).resolves.toMatchObject({status:'unconfigured'});
  unproven=true;
  await expect(readXrplIssuedBalance(rpc,'1','USD',issuer,owner)).rejects.toThrow('XRPL_ISSUE_LEDGER');
});

test('non-EVM asset preferences validate at restart and cannot install reviewed data in a different owner scope', () => {
  const current={boardFingerprint:'bd'.repeat(32),generation:6};
  const endpoint='https://rpc.example/';
  const value={...defaultReadState,selection:{id:'sol' as const,account:3,hidden:false},endpoints:{sol:endpoint}};
  const review={id:'sol' as const,board:current.boardFingerprint,generation:6,account:3,mint:mintId,slot:303,decimals:6,endpoint};
  const next=registerNonEvmAsset(value,current,review);
  expect(next.selection.asset).toBe(mintId);
  expect(isWalletReadState(JSON.parse(JSON.stringify(next)))).toBe(true);
  expect(() => registerNonEvmAsset(value,{...current,generation:7},review)).toThrow('WALLET_ASSET_SCOPE');
  expect(() => registerNonEvmAsset({...value,disabled:['sol']},current,review)).toThrow('WALLET_ASSET_SCOPE');
});
