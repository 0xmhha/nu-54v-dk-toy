import {bitcoinSignetCheckpoint, type WalletChain} from './chainAccounts.ts';

const profiles = {
  1: {profile:'bitcoin-signet',route:'/bitcoin',
    anchor:'00000008819873e925422c1ff0f99f7cc9bbb232af63a077a480a3633bee1ef6:'+bitcoinSignetCheckpoint.hash},
  2: {profile:'solana-devnet',route:'/solana',anchor:'EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG'},
  3: {profile:'xrpl-testnet',route:'/xrpl',anchor:'1:60691D3CA08E04064EBB014D0ADE8DA2CB2BFE0AE9D820D88502FB3A9A8F7149'},
} as const;
export function commissionedProfile(chain:WalletChain) {return profiles[chain].profile;}

export type CommissionedChain = {chain:WalletChain;profile:(typeof profiles)[WalletChain]['profile'];anchor:string;rpc:string};

export function readCommissionedChains(input:unknown):CommissionedChain[] {
  if (!Array.isArray(input) || !input.length || input.length > 3) throw new Error('CHAIN_REGISTRY');
  const rows = input.map(chainProfile => {
    if (!chainProfile || typeof chainProfile !== 'object' ||
        Object.keys(chainProfile).sort().join(',') !== 'anchor,chain,profile,rpc') throw new Error('CHAIN_REGISTRY');
    const row = chainProfile as CommissionedChain;
    if (!Number.isInteger(row.chain) || !Object.hasOwn(profiles,row.chain)) throw new Error('CHAIN_REGISTRY');
    const trusted = profiles[row.chain];
    if (row.profile !== trusted.profile || row.anchor !== trusted.anchor || typeof row.rpc !== 'string' || row.rpc.length > 320)
      throw new Error('CHAIN_REGISTRY_SCOPE');
    const endpoint = new URL(row.rpc);
    if (endpoint.protocol !== 'https:' || endpoint.username || endpoint.password || endpoint.search || endpoint.hash ||
        endpoint.pathname !== trusted.route || endpoint.href !== row.rpc ||
        !/^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/.test(endpoint.hostname))
      throw new Error('CHAIN_REGISTRY_ENDPOINT');
    return row;
  });
  if (new Set(rows.map(row=>row.chain)).size !== rows.length) throw new Error('CHAIN_REGISTRY_DUPLICATE');
  return rows;
}

type ReadRpc = (method:string,params:unknown)=>Promise<unknown>;

function record(input:unknown):Record<string,unknown> {
  if (!input || typeof input!=='object' || Array.isArray(input)) throw new Error('CHAIN_COMMISSIONED_ANCHOR');
  return input as Record<string,unknown>;
}

export async function verifyCommissionedChain(candidate:CommissionedChain,rpc:ReadRpc) {
  const [row]=readCommissionedChains([candidate]);
  await verifyCommissionedAnchor(row.chain,row.anchor,rpc);
}

export async function verifyCommissionedAnchor(chain:WalletChain,anchor:string,rpc:ReadRpc) {
  if (anchor!==profiles[chain]?.anchor) throw new Error('CHAIN_COMMISSIONED_ANCHOR');
  const [genesis,checkpoint]=anchor.split(':');
  let matches:boolean;
  if (chain===1) {
    matches=record(await rpc('getblockchaininfo',[])).chain==='signet' &&
      await rpc('getblockhash',[0])===genesis &&
      await rpc('getblockhash',[bitcoinSignetCheckpoint.height])===checkpoint;
  } else if (chain===2) {
    matches=await rpc('getGenesisHash',[])===genesis;
  } else {
    const info=record(record(await rpc('server_info',[{}])).info);
    const result=record(await rpc('ledger',[{ledger_hash:checkpoint,transactions:false}]));
    matches=info.network_id===Number(genesis) && result.validated===true &&
      result.ledger_hash===checkpoint && String(result.ledger_index)==='22470370';
  }
  if (!matches) throw new Error('CHAIN_COMMISSIONED_ANCHOR');
}
