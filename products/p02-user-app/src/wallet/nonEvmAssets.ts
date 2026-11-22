import {equalBytes, fromBase64, toBase64} from '@nu54/protocol';
import {checkXrplClassic, decodePublicBase58} from './chainAccounts.ts';
import {readXrplReceiveBalance, type ChainRpc} from './chainTransfer.ts';

export const classicTokenProgram = 'TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA';

type Mint = {mint:string; decimals:number; slot:number};
const record = (value: unknown): Record<string, unknown> => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('WALLET_ASSET_SCHEMA');
  return value as Record<string, unknown>;
};
const point = (reply: Record<string,unknown>) => {
  const slot = record(reply.context).slot;
  if (typeof slot !== 'number' || !Number.isSafeInteger(slot) || slot < 1) throw new Error('SOL_TOKEN_SLOT');
  return slot;
};

function splBytes(value: unknown, length: number) {
  const info = record(value);
  if (info.owner !== classicTokenProgram || info.executable !== false) throw new Error('SOL_TOKEN_PROGRAM');
  const {data} = info;
  if (!Array.isArray(data) || data.length !== 2 || data[1] !== 'base64' || typeof data[0] !== 'string' || data[0].length > 224)
    throw new Error('SOL_TOKEN_BYTES');
  const bytes = fromBase64(data[0]);
  if (bytes.length !== length || toBase64(bytes) !== data[0]) throw new Error('SOL_TOKEN_BYTES');
  return bytes;
}

export async function readSolanaMint(rpc: ChainRpc, genesis: string, mint: string, expected?: number): Promise<Mint> {
  decodePublicBase58(mint, 32);
  if (await rpc('getGenesisHash', []) !== genesis) throw new Error('WALLET_ASSET_NETWORK');
  const response = record(await rpc('getAccountInfo', [mint, {encoding:'base64', commitment:'finalized'}]));
  const slot = point(response);
  const bytes = splBytes(response.value, 82);
  const data = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const decimals = bytes[44];
  if (bytes[45] !== 1 || data.getUint32(0, true) > 1 || data.getUint32(46, true) > 1 ||
      decimals > 36 || (expected !== undefined && decimals !== expected))
    throw new Error('SOL_TOKEN_MINT');
  return {mint,decimals,slot};
}

export async function readSolanaTokenBalance(rpc: ChainRpc, genesis: string, review: Mint, owner: string) {
  const mint = decodePublicBase58(review.mint, 32), user = decodePublicBase58(owner, 32);
  const current = await readSolanaMint(rpc, genesis, review.mint, review.decimals);
  if (current.slot < review.slot) throw new Error('SOL_TOKEN_SLOT');
  const response = record(await rpc('getTokenAccountsByOwner', [owner, {mint:review.mint},
    {encoding:'base64',commitment:'finalized',minContextSlot:current.slot}]));
  const slot = point(response), rows = response.value;
  if (slot < current.slot || !Array.isArray(rows) || rows.length > 64) throw new Error('SOL_TOKEN_ACCOUNT');
  const seen = new Set<string>();
  let total = 0n;
  for (const raw of rows) {
    const row = record(raw);
    if (typeof row.pubkey !== 'string' || seen.has(row.pubkey)) throw new Error('SOL_TOKEN_ACCOUNT');
    decodePublicBase58(row.pubkey, 32);
    seen.add(row.pubkey);
    const bytes = splBytes(row.account, 165);
    const data = new DataView(bytes.buffer,bytes.byteOffset,bytes.byteLength);
    if (!equalBytes(bytes.subarray(0,32),mint) || !equalBytes(bytes.subarray(32,64),user) ||
        ![1,2].includes(bytes[108]) || data.getUint32(72,true) > 1 ||
        data.getUint32(109,true) > 1 || data.getUint32(129,true) > 1)
      throw new Error('SOL_TOKEN_ACCOUNT');
    total += data.getBigUint64(64,true);
  }
  if (total >= 2n ** 64n) throw new Error('SOL_TOKEN_AMOUNT');
  return {amount:total.toString(),decimals:review.decimals,slot};
}

export function xrplIssue(currency: string, issuer: string) {
  if ((!/^[A-Za-z0-9]{3}$/.test(currency) && !/^(?!00)[0-9A-F]{40}$/.test(currency)) || currency.toUpperCase() === 'XRP')
    throw new Error('XRPL_ISSUE');
  return {currency,issuer:checkXrplClassic(issuer)};
}

export function xrplIssuedValue(input: string) {
  const match = /^(-?)(0|[1-9][0-9]{0,95})(?:\.([0-9]{1,96}))?(?:e([+-]?[0-9]{1,3}))?$/.exec(input);
  if (!match) throw new Error('XRPL_ISSUED_AMOUNT');
  const [,sign,whole,fraction='',power='0'] = match;
  const digits = whole + fraction;
  const first = digits.search(/[1-9]/);
  if (first < 0) return '0';
  const significant = digits.slice(first).replace(/0+$/,'');
  const offset = whole.length + Number(power) - first;
  if (significant.length > 16 || offset < -80 || offset > 96)
    throw new Error('XRPL_ISSUED_AMOUNT');
  const normalized = offset <= 0 ? '0.' + '0'.repeat(-offset) + significant :
    offset >= significant.length ? significant.padEnd(offset,'0') :
      significant.slice(0,offset) + '.' + significant.slice(offset);
  return sign + normalized;
}

export async function readXrplIssuer(rpc: ChainRpc, network: string, currency: string, issuer: string) {
  const identity = xrplIssue(currency,issuer);
  const state = await readXrplReceiveBalance(rpc, identity.issuer, network);
  if (state.status !== 'active') throw new Error('XRPL_ISSUER_INACTIVE');
  return {...identity, ledgerHash:state.ledgerHash, ledgerIndex:state.ledgerIndex};
}

export async function readXrplIssuedBalance(rpc: ChainRpc, network: string, currency: string, issuer: string, owner: string) {
  const identity = xrplIssue(currency,issuer);
  checkXrplClassic(owner);
  if (owner === issuer) throw new Error('XRPL_ISSUER_BALANCE');
  const state = await readXrplReceiveBalance(rpc, owner, network);
  const ledger = {ledgerHash:state.ledgerHash,ledgerIndex:state.ledgerIndex};
  if (state.status === 'inactive') return {...ledger,status:'inactive' as const};
  let marker: string | undefined, value: string | undefined;
  const markers = new Set<string>();
  for (let round = 0; round < 8; round++) {
    const result = record(await rpc('account_lines',[{account:owner,peer:issuer,ledger_hash:state.ledgerHash,
      limit:50, ...(marker ? {marker} : {})}]));
    if (result.error || result.account !== owner || result.ledger_hash !== state.ledgerHash || result.ledger_index !== state.ledgerIndex ||
        result.validated !== true || !Array.isArray(result.lines) || result.lines.length > 50)
      throw new Error('XRPL_ISSUE_LEDGER');
    for (const raw of result.lines) {
      const row = record(raw);
      if (row.account !== issuer || typeof row.currency !== 'string') throw new Error('XRPL_ISSUE_LINE');
      if (row.currency !== identity.currency) continue;
      if (value !== undefined || typeof row.balance !== 'string') throw new Error('XRPL_ISSUE_LINE');
      value = xrplIssuedValue(row.balance);
    }
    if (result.marker === undefined)
      return value === undefined ? {...ledger,status:'unconfigured' as const} : {...ledger,status:'active' as const,value};
    if (typeof result.marker !== 'string' || result.marker.length > 160 || !result.marker || markers.has(result.marker))
      throw new Error('XRPL_ISSUE_MARKER');
    marker = result.marker;
    markers.add(marker);
  }
  throw new Error('XRPL_ISSUE_MARKER');
}
