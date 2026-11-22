import {isNonEvmAssetId, nonEvmAssetKey, validNonEvmAssetPreference, type NonEvmAssetPreference} from './nonEvmAssetCatalog.ts';
import {readSolanaTokenBalance, readXrplIssuedBalance, xrplIssuedValue} from './nonEvmAssets.ts';
import {catalogKey, validAccountPreference, validAssetPreference, type AccountPreference, type AssetPreference} from './walletCatalog.ts';
import {bytesToHex} from '@nu54/protocol';
import {readPublicSignet, signetIndexer} from './signetIndexer.ts';
import {evmProfiles, normalizeEvmAddress, readEvmReceiveBalance, type EvmChain} from './evmTransfer.ts';
import {bitcoinSignetCheckpoint, checkSegwitV0, checkXrplClassic, decodePublicBase58, hash160, segwitAddress, type ChainPublicAccount} from './chainAccounts.ts';
import {readChainReceiveBalance, type ChainRpc} from './chainTransfer.ts';

export const readProfiles = {
  stable: {label: 'StableNet Testnet', anchor: '0x2bdf79b3d3cc49f9e6638ff81f3bb85065c79945a8fe4556cd0ff47bbfc02490', evm: '8283'},
  eth: {label: 'Ethereum Sepolia', anchor: '0x25a5cc106eea7138acab33231d7160d69cb777ee0c2c553fcddf5138993e6dd9', evm: '11155111'},
  base: {label: 'Base Sepolia', anchor: '0x0dcc9e089e30b90ddfc55be9a37dd15bc551aeee999d2e2b51414c54eaf934e4', evm: '84532'},
  btc: {label: 'Bitcoin Signet', anchor: '00000008819873e925422c1ff0f99f7cc9bbb232af63a077a480a3633bee1ef6', chain: 1},
  sol: {label: 'Solana Devnet', anchor: 'EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG', chain: 2},
  xrp: {label: 'XRP Ledger Testnet', anchor: '1', chain: 3,
    ledger: '60691D3CA08E04064EBB014D0ADE8DA2CB2BFE0AE9D820D88502FB3A9A8F7149', height: 22470370},
} as const;

export type ReadProfileId = keyof typeof readProfiles;
export function builtInEvmEnabled(chain: EvmChain, disabled?: ReadProfileId[]) {
  const id: Record<EvmChain, ReadProfileId> = {'8283': 'stable', '11155111': 'eth', '84532': 'base'};
  return !disabled?.includes(id[chain]);
}
export type ReadContact = {id: ReadProfileId; name: string; address: string; tag?: string};
export type ReadSelection = {id: ReadProfileId; account: number; hidden: boolean; automatic?: boolean; asset?: string; tag?: string; index?: number};
export type ReadObservation = {address: string; scope: string; time: number; point: string; amount?: string; unit: string; decimals?: number; inactive?: boolean; unconfigured?: boolean; issued?: string; used?: boolean};
export type ReadProfileState = {selection: ReadSelection; endpoints: Partial<Record<ReadProfileId, string>>;
  contacts: ReadContact[]; observations: ReadObservation[]; disabled?: ReadProfileId[]; custom?: CustomEvmDescriptor[]; accounts?: AccountPreference[]; assets?: AssetPreference[]; nonEvmAssets?: NonEvmAssetPreference[]};

const methods = new Set(['eth_chainId', 'eth_getBlockByNumber', 'eth_getBalance', 'eth_call',
  'getblockchaininfo', 'getblockhash', 'gettxout', 'getaddressinfo', 'getreceivedbyaddress', 'listunspent', 'getGenesisHash', 'getBalance',
  'server_info', 'account_info', 'ledger', 'getAccountInfo', 'getTokenAccountsByOwner', 'account_lines']);
const xrplLegacyMethods = new Set(['server_info', 'account_info', 'ledger', 'account_lines']);
const isId = (value: unknown): value is ReadProfileId => typeof value === 'string' && Object.hasOwn(readProfiles, value);
const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);

export function readEndpoint(input: string) {
  if (typeof input !== 'string' || input.length > 512 ||
      !/^https:\/\/[A-Za-z0-9.[\]:_-]+(?:\/[A-Za-z0-9/_.-]*)?$/.test(input)) throw new Error('WALLET_READ_HTTPS');
  const url = new URL(input);
  if (url.protocol !== 'https:' || url.username || url.password || url.hash || url.search)
    throw new Error('WALLET_READ_HTTPS');
  return url.href;
}

export function walletReadRpc(endpoint: string, exchange: (url: string, packet: string) => Promise<string>): ChainRpc {
  const url = readEndpoint(endpoint);
  return async (method, params) => {
    if (!methods.has(method)) throw new Error('WALLET_READ_METHOD');
    const raw = await exchange(url, JSON.stringify({jsonrpc: '2.0', method, params, id: 1}));
    if (raw.length > 256000) throw new Error('WALLET_READ_SIZE');
    const packet = JSON.parse(raw) as unknown;
    if (!isRecord(packet)) throw new Error('WALLET_READ_RESPONSE');
    // rippled's legacy HTTP result envelope has no JSON-RPC ID. This method
    // uses one synchronous HTTPS request per call; a provided ID must match.
    const legacyXrpl = xrplLegacyMethods.has(method) && packet.id === undefined && packet.jsonrpc === undefined;
    if (!legacyXrpl && (packet.id !== 1 || (packet.jsonrpc !== undefined && packet.jsonrpc !== '2.0')))
      throw new Error('WALLET_READ_RESPONSE');
    if (packet.error) {
      if (isRecord(packet.error) && packet.error.code === 4444 && method === 'eth_getBlockByNumber')
        throw new Error('EVM_RPC_GENESIS_UNAVAILABLE');
      throw new Error('WALLET_READ_RESPONSE');
    }
    if (!Object.hasOwn(packet, 'result')) throw new Error('WALLET_READ_RESPONSE');
    return packet.result;
  };
}

export const defaultReadState: ReadProfileState = {
  selection: {id: 'stable', account: 0, hidden: false}, endpoints: {}, contacts: [], observations: [],
};

export function readContact(id: ReadProfileId, name: string, address: string, tag?: string): ReadContact {
  const profile = readProfiles[id];
  const label = name.trim();
  if (label.length < 1 || label.length > 32 || /[\u0000-\u001f\u007f]/.test(label))
    throw new Error('WALLET_READ_CONTACT_NAME');
  let target = address.trim();
  if ('evm' in profile) target = normalizeEvmAddress(target);
  else if (profile.chain === 1) target = checkSegwitV0('tb', target);
  else if (profile.chain === 2) decodePublicBase58(target, 32);
  else checkXrplClassic(target);
  if (tag !== undefined && (id !== 'xrp' || !isXrpTag(tag))) throw new Error('WALLET_READ_CONTACT_TAG');
  return {id, name: label, address: target, ...(tag === undefined ? {} : {tag})};
}

export function isWalletReadState(value: unknown): value is ReadProfileState {
  if (!isRecord(value) || !isRecord(value.selection) || !isRecord(value.endpoints) ||
      !Array.isArray(value.contacts) || value.contacts.length > 30 ||
      !Array.isArray(value.observations) || value.observations.length > 24) return false;
  const selected = value.selection;
  try {
    if (value.accounts !== undefined && (!Array.isArray(value.accounts) || value.accounts.length > 12 ||
        !value.accounts.every(validAccountPreference) ||
        new Set(value.accounts.map(catalogKey)).size !== value.accounts.length)) return false;
    if (value.nonEvmAssets !== undefined && (!Array.isArray(value.nonEvmAssets) || value.nonEvmAssets.length > 12 ||
        !value.nonEvmAssets.every(validNonEvmAssetPreference) ||
        new Set(value.nonEvmAssets.map(item => catalogKey(item) + '/' + nonEvmAssetKey(item))).size !== value.nonEvmAssets.length)) return false;
    if (value.assets !== undefined && (!Array.isArray(value.assets) || value.assets.length > 12 ||
        !value.assets.every(validAssetPreference) ||
        new Set(value.assets.map(item => catalogKey(item) + '/' + item.contract)).size !== value.assets.length)) return false;
    if (value.disabled !== undefined && (!Array.isArray(value.disabled) || value.disabled.length > 6 ||
        !value.disabled.every(isId) || new Set(value.disabled).size !== value.disabled.length)) return false;
    if (value.custom !== undefined && (!Array.isArray(value.custom) || value.custom.length > 8 ||
        value.custom.some(item => !readCustomEvmDescriptor(item)) ||
        new Set(value.custom.map(item => item.chainId)).size !== value.custom.length)) return false;
    if (!isId(selected.id) || !Number.isInteger(selected.account) || Number(selected.account) < 0 ||
        Number(selected.account) > 99 || typeof selected.hidden !== 'boolean' ||
        (selected.automatic !== undefined && typeof selected.automatic !== 'boolean') ||
        (selected.index !== undefined && (selected.id !== 'btc' || !Number.isInteger(selected.index) ||
          Number(selected.index) < 0 || Number(selected.index) > 9999)) ||
        (selected.asset !== undefined && (typeof selected.asset !== 'string' ||
          ('evm' in readProfiles[selected.id] ? !/^0x[a-f0-9]{40}$/.test(selected.asset) :
            !isNonEvmAssetId(selected.id,selected.asset)))) ||
        (selected.tag !== undefined && (selected.id !== 'xrp' || !isXrpTag(selected.tag)))) return false;
    for (const [id, endpoint] of Object.entries(value.endpoints)) {
      if (!isId(id) || typeof endpoint !== 'string') return false;
      readEndpoint(endpoint);
    }
    for (const contact of value.contacts) {
      if (!isRecord(contact) || !isId(contact.id) || typeof contact.name !== 'string' || typeof contact.address !== 'string') return false;
      if (contact.tag !== undefined && typeof contact.tag !== 'string') return false;
      readContact(contact.id, contact.name, contact.address, contact.tag);
    }
    for (const row of value.observations) {
      if (!isRecord(row) || typeof row.scope !== 'string' || row.scope.length > 240 ||
          typeof row.address !== 'string' || !/^[A-Za-z0-9]{16,90}$/.test(row.address) ||
          typeof row.point !== 'string' || row.point.length > 128 || typeof row.time !== 'number' ||
          !Number.isSafeInteger(row.time) || row.time < 1 || typeof row.unit !== 'string' || row.unit.length > 20 ||
          (row.used !== undefined && typeof row.used !== 'boolean') ||
          (row.decimals !== undefined && (!Number.isInteger(row.decimals) || Number(row.decimals) < 0 || Number(row.decimals) > 36)) ||
          (row.inactive !== undefined && typeof row.inactive !== 'boolean') ||
          (row.unconfigured !== undefined && typeof row.unconfigured !== 'boolean') ||
          (row.inactive === true && row.unconfigured === true) ||
          (row.inactive === true || row.unconfigured === true ? row.amount !== undefined || row.issued !== undefined :
            row.issued !== undefined ? row.amount !== undefined || typeof row.issued !== 'string' ||
              xrplIssuedValue(row.issued) !== row.issued :
              typeof row.amount !== 'string' || !/^\d{1,80}$/.test(row.amount))) return false;
    }
    return true;
  } catch {
    return false;
  }
}

export type CustomEvmDescriptor = {engine: 'evm'; capability: 'read-only'; name: string; chainId: string; genesis: string;
  rpc: string; explorer: string; currency: string; decimals: number; feeCap: string; confirmations: number};

export function readCustomEvmDescriptor(input: unknown): CustomEvmDescriptor {
  if (!isRecord(input) || input.engine !== 'evm' ||
      (input.capability !== undefined && input.capability !== 'read-only')) throw new Error('WALLET_CUSTOM_ENGINE');
  const {name, chainId, genesis, rpc, explorer, currency, decimals, feeCap, confirmations} = input;
  if (typeof name !== 'string' || !/^.{1,32}$/.test(name) ||
      typeof chainId !== 'string' || !/^[1-9][0-9]{0,19}$/.test(chainId) || BigInt(chainId) >= 2n ** 64n ||
      Object.values(readProfiles).some(value => 'evm' in value && value.evm === chainId) ||
      typeof genesis !== 'string' || !/^0x[a-f0-9]{64}$/.test(genesis) || /^0x0{64}$/.test(genesis) ||
      typeof rpc !== 'string' || typeof explorer !== 'string' ||
      typeof currency !== 'string' || !/^[A-Z][A-Z0-9]{0,11}$/.test(currency) ||
      typeof decimals !== 'number' || !Number.isInteger(decimals) || decimals < 0 || decimals > 36 ||
      typeof feeCap !== 'string' || !/^[1-9][0-9]{0,29}$/.test(feeCap) ||
      typeof confirmations !== 'number' || !Number.isInteger(confirmations) || confirmations < 0 || confirmations > 10000)
    throw new Error('WALLET_CUSTOM_SCHEMA');
  return {engine: 'evm', capability: 'read-only', name, chainId, genesis, rpc: readEndpoint(rpc), explorer: readEndpoint(explorer),
    currency, decimals, feeCap, confirmations};
}

export function setCustomEvmDescriptor(existing: CustomEvmDescriptor[], candidate: CustomEvmDescriptor) {
  const previous = existing.find(row => row.chainId === candidate.chainId);
  if (previous && previous.genesis !== candidate.genesis) throw new Error('WALLET_CUSTOM_IDENTITY');
  const others = existing.filter(row => row.chainId !== candidate.chainId);
  if (others.length >= 8) throw new Error('WALLET_CUSTOM_CAPACITY');
  return [...others, candidate];
}

export async function verifyCustomEvmNetwork(network: CustomEvmDescriptor, rpc: ChainRpc) {
  // No address or read-transaction hints are transmitted before the identity check.
  const chain = await rpc('eth_chainId', []);
  const genesis = await rpc('eth_getBlockByNumber', ['0x0', false]);
  if (typeof chain !== 'string' || !/^0x[a-f0-9]+$/.test(chain) || BigInt(chain).toString() !== network.chainId ||
      !isRecord(genesis) || genesis.hash !== network.genesis) throw new Error('WALLET_CUSTOM_IDENTITY');
}

export async function readCustomEvmBalance(network: CustomEvmDescriptor, address: string, rpc: ChainRpc) {
  await verifyCustomEvmNetwork(network, rpc);
  const owner = normalizeEvmAddress(address);
  let point: unknown;
  if (!network.confirmations) {
    point = await rpc('eth_getBlockByNumber', ['finalized', false]);
  } else {
    const latest = await rpc('eth_getBlockByNumber', ['latest', false]);
    if (!isRecord(latest) || typeof latest.number !== 'string' || !/^0x[a-f0-9]+$/.test(latest.number))
      throw new Error('WALLET_CUSTOM_FINALITY');
    const height = BigInt(latest.number) - BigInt(network.confirmations) + 1n;
    if (height < 0n) throw new Error('WALLET_CUSTOM_FINALITY');
    point = await rpc('eth_getBlockByNumber', ['0x' + height.toString(16), false]);
    if (!isRecord(point) || typeof point.number !== 'string' || !/^0x[a-f0-9]+$/.test(point.number) ||
        BigInt(point.number) !== height) throw new Error('WALLET_CUSTOM_FINALITY');
  }
  if (!isRecord(point) || typeof point.hash !== 'string' || !/^0x[a-f0-9]{64}$/.test(point.hash) ||
      typeof point.number !== 'string' || !/^0x[a-f0-9]+$/.test(point.number))
    throw new Error('WALLET_CUSTOM_FINALITY');
  const amount = await rpc('eth_getBalance', [owner, {blockHash: point.hash, requireCanonical: true}]);
  if (typeof amount !== 'string' || !/^0x[a-f0-9]{1,64}$/.test(amount)) throw new Error('WALLET_CUSTOM_AMOUNT');
  return {address: owner, amount: BigInt(amount).toString(), hash: point.hash, height: BigInt(point.number).toString()};
}

export function isXrpTag(value: unknown): value is string {
  return typeof value === 'string' && /^(?:0|[1-9][0-9]{0,9})$/.test(value) && BigInt(value) <= 0xffffffffn;
}

export function formatReadAmount(raw: string, decimals = 0) {
  if (decimals === 0) return raw;
  const units = raw.padStart(decimals + 1, '0');
  const fraction = units.slice(-decimals).replace(/0+$/, '');
  return units.slice(0, -decimals) + (fraction ? '.' + fraction : '');
}

export function observationScope(board: string, generation: number, selection: ReadSelection) {
  return [board, generation, selection.id, readProfiles[selection.id].anchor,
    selection.id === 'xrp' ? readProfiles.xrp.ledger : selection.id === 'btc' ? bitcoinSignetCheckpoint.hash : '', selection.account, selection.index ?? 0, selection.asset ?? 'native'].join('/');
}

export function updateReadObservation(previous: ReadObservation[], current: ReadObservation) {
  const existing = previous.find(item => item.scope === current.scope);
  if (existing && existing.address === current.address && existing.point === current.point &&
      existing.amount === current.amount && existing.inactive === current.inactive && existing.unit === current.unit &&
      existing.decimals === current.decimals && existing.used === current.used &&
      existing.unconfigured === current.unconfigured && existing.issued === current.issued) return previous;
  return [current, ...previous.filter(item => item.scope !== current.scope)].slice(0, 24);
}

export async function verifyReadNetwork(rpc: ChainRpc, id: ReadProfileId) {
  const profile = readProfiles[id];
  if ('evm' in profile) {
    const chain = await rpc('eth_chainId', []);
    const genesis = await rpc('eth_getBlockByNumber', ['0x0', false]);
    if (genesis === null) throw new Error('EVM_RPC_GENESIS_UNAVAILABLE');
    if (typeof chain !== 'string' || !/^0x[a-f0-9]+$/.test(chain) || BigInt(chain).toString() !== profile.evm ||
        !isRecord(genesis) || genesis.hash !== profile.anchor) throw new Error('WALLET_READ_CHAIN');
  } else if (profile.chain === 1) {
    const info = await rpc('getblockchaininfo', []);
    if (!isRecord(info) || info.chain !== 'signet' || await rpc('getblockhash', [0]) !== profile.anchor ||
        await rpc('getblockhash', [bitcoinSignetCheckpoint.height]) !== bitcoinSignetCheckpoint.hash)
      throw new Error('WALLET_READ_CHAIN');
  } else if (profile.chain === 2) {
    if (await rpc('getGenesisHash', []) !== profile.anchor) throw new Error('WALLET_READ_CHAIN');
  } else {
    const info = await rpc('server_info', [{}]);
    if (!isRecord(info) || !isRecord(info.info) || info.info.network_id !== 1)
      throw new Error('WALLET_READ_CHAIN');
    const proof = await rpc('ledger', [{ledger_hash: profile.ledger, transactions: false}]);
    if (!isRecord(proof) || proof.validated !== true || proof.ledger_hash !== profile.ledger ||
        (proof.ledger_index !== profile.height && proof.ledger_index !== String(profile.height)))
      throw new Error('WALLET_READ_CHAIN');
  }
}

export function receiveAddressForProfile(selection: ReadSelection, publicAccount: ChainPublicAccount | string) {
  const profile = readProfiles[selection.id];
  if ('evm' in profile) {
    if (typeof publicAccount !== 'string' || selection.account !== 0) throw new Error('WALLET_READ_BOARD_ACCOUNT');
    return normalizeEvmAddress(publicAccount);
  }
  if (typeof publicAccount === 'string' || publicAccount.chain !== profile.chain ||
      publicAccount.network !== 1 || publicAccount.account !== selection.account)
    throw new Error('WALLET_READ_BOARD_ACCOUNT');
  return profile.chain === 1 ? segwitAddress('tb', Uint8Array.of(0, 20, ...hash160(publicAccount.publicKey))) : publicAccount.address;
}

export async function readSelectedBalance(rpc: ChainRpc, selection: ReadSelection, publicAccount: ChainPublicAccount | string,
  indexer?: ReturnType<typeof signetIndexer>, token?:NonEvmAssetPreference): Promise<Omit<ReadObservation,'scope'|'time'>> {
  const profile = readProfiles[selection.id];
  if ('evm' in profile) {
    if (typeof publicAccount !== 'string' || selection.account !== 0) throw new Error('WALLET_READ_BOARD_ACCOUNT');
    const result = await readEvmReceiveBalance((method, params) => rpc(method, params), profile.evm as EvmChain,
      profile.anchor, publicAccount, selection.asset, true);
    const native = !selection.asset || selection.asset === '0x' + '0'.repeat(40);
    return {address: publicAccount, amount: result.amount, unit: native ? evmProfiles[profile.evm].native : 'ERC-20',
      decimals: result.decimals, point: 'Finalized block ' + result.block};
  }
  if (typeof publicAccount === 'string' || publicAccount.chain !== profile.chain || publicAccount.account !== selection.account)
    throw new Error('WALLET_READ_BOARD_ACCOUNT');
  if (profile.chain === 1) {
    const program = Uint8Array.of(0, 20, ...hash160(publicAccount.publicKey));
    const address = segwitAddress('tb', program);
    if (indexer) {
      const observed = await readPublicSignet(indexer, profile.anchor, address, bytesToHex(program, false));
      return {address, ...observed, unit: 'BTC', decimals: 8};
    }
    const result = await readChainReceiveBalance(rpc, {...publicAccount, address}, profile.anchor, 'signet');
    if (!('amount' in result)) throw new Error('BTC_BALANCE_SCHEMA');
    return {address, amount: result.amount, unit: 'BTC', decimals: 8, point: result.observation};
  }
  if (profile.chain === 3) await verifyReadNetwork(rpc, selection.id);
  if (selection.asset) {
    if (!token || token.id !== selection.id || token.account !== selection.account || token.hidden ||
        nonEvmAssetKey(token) !== selection.asset) throw new Error('WALLET_ASSET_SCOPE');
    const address = receiveAddressForProfile(selection,publicAccount);
    if (token.id === 'sol') {
      const result = await readSolanaTokenBalance(rpc,profile.anchor,token,address);
      return {address,amount:result.amount,decimals:result.decimals,unit:'SPL',point:'Finalized slot ' + result.slot};
    }
    const issued = await readXrplIssuedBalance(rpc,profile.anchor,token.currency,token.issuer,address);
    if (issued.ledgerIndex < token.ledgerIndex) throw new Error('XRPL_ISSUE_LEDGER');
    return {address,unit:token.currency.length > 20 ? 'Issued token' : token.currency,
      point:'XRPL validated ledger ' + issued.ledgerIndex,
      ...(issued.status === 'active' ? {issued:issued.value} : issued.status === 'inactive' ? {inactive:true} : {unconfigured:true})};
  }
  const value = await readChainReceiveBalance(rpc, publicAccount, profile.anchor);
  return {address: publicAccount.address, ...('amount' in value ? {amount: value.amount} : {inactive: true}),
    unit: profile.chain === 2 ? 'SOL' : 'XRP', decimals: profile.chain === 2 ? 9 : 6, point: value.observation};
}

export async function discoverSignetReceive(rpc: ChainRpc, account: number,
  derive: (index: number) => Promise<ChainPublicAccount>, start = 0, signal?: AbortSignal) {
  if (!Number.isInteger(start) || start<0 || start>9936) throw new Error('BTC_DISCOVERY_RANGE');
  const result = await discoverSignet(rpc,account,checkAbort=>scanSignetBranch(rpc,account,derive,start,new Set(),checkAbort),signal);
  return result.value;
}

export async function discoverSignetBranches(rpc:ChainRpc,account:number,
  derive:(branch:0|1,index:number)=>Promise<ChainPublicAccount>,signal?:AbortSignal) {
  const {head,value}=await discoverSignet(rpc,account,async checkAbort=>{
    const addresses=new Set<string>();
    const receive=await scanSignetBranch(rpc,account,index=>derive(0,index),0,addresses,checkAbort);
    const change=await scanSignetBranch(rpc,account,index=>derive(1,index),0,addresses,checkAbort);
    return {receive,change};
  },signal);
  return {head,account,...value};
}

async function discoverSignet<T>(rpc:ChainRpc,account:number,
  scan:(checkAbort:()=>void)=>Promise<T>,signal?:AbortSignal) {
  if (!Number.isInteger(account) || account<0 || account>99) throw new Error('BTC_DISCOVERY_RANGE');
  const checkAbort=()=>{
    if (signal?.aborted) throw new Error('BTC_DISCOVERY_CANCELLED');
  };
  checkAbort();
  await verifyReadNetwork(rpc,'btc');
  const startBlock=await rpc('getblockchaininfo',[]);
  if (!isRecord(startBlock) || startBlock.chain!=='signet' || typeof startBlock.bestblockhash!=='string' ||
      !/^[a-f0-9]{64}$/.test(startBlock.bestblockhash)) throw new Error('BTC_DISCOVERY_HEAD');
  const head=startBlock.bestblockhash;
  const value=await scan(checkAbort);
  checkAbort();
  const endBlock=await rpc('getblockchaininfo',[]);
  checkAbort();
  if (!isRecord(endBlock) || endBlock.chain!=='signet' || endBlock.bestblockhash!==head) throw new Error('BTC_DISCOVERY_HEAD_CHANGED');
  return {head,value};
}

async function scanSignetBranch(rpc:ChainRpc,account:number,derive:(index:number)=>Promise<ChainPublicAccount>,
  start:number,addresses:Set<string>,checkAbort:()=>void) {
  let gap = 0, lastUsed = start - 1;
  for (let index = start; index < start + 64; index++) {
    checkAbort();
    const derived = await derive(index);
    checkAbort();
    if (derived.chain !== 1 || derived.account !== account || (derived.network!==1 && derived.network!==2) ||
        derived.publicKey.length!==33 || (derived.publicKey[0]!==2 && derived.publicKey[0]!==3))
      throw new Error('BTC_DISCOVERY_ACCOUNT');
    const address = segwitAddress('tb', Uint8Array.of(0, 20, ...hash160(derived.publicKey)));
    if (addresses.has(address)) throw new Error('BTC_DISCOVERY_DUPLICATE');
    addresses.add(address);
    const watch = await rpc('getaddressinfo', [address]);
    checkAbort();
    if (!isRecord(watch) || watch.address !== address || (watch.iswatchonly !== true && watch.ismine !== true))
      throw new Error('BTC_DISCOVERY_WATCH_RANGE');
    const total = await rpc('getreceivedbyaddress', [address, 1]);
    checkAbort();
    if ((typeof total !== 'string' && typeof total !== 'number') || !/^(?:0|[1-9][0-9]*)(?:\.\d{1,8})?$/.test(String(total)))
      throw new Error('BTC_DISCOVERY_HISTORY');
    if (/[1-9]/.test(String(total))) {
      lastUsed = index;
      gap = 0;
    } else if (++gap === 20) {
      return {nextIndex: lastUsed + 1, lastUsed, scanned: index - start + 1};
    }
  }
  throw new Error('BTC_DISCOVERY_LIMIT');
}
