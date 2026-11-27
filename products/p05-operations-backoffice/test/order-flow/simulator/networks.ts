// What the other networks of the app answer in a simulation. StableNet is the local chain; everything
// here is made up, so the networks and token pages can be used without a public server.
import {bitcoinSignetCheckpoint} from '../../../../p02-user-app/src/wallet/chainAccounts';
import {publicSignetIndexer} from '../../../../p02-user-app/src/wallet/signetIndexer';
import {builtInServers} from '../../../../p02-user-app/src/wallet/networks';
import {readProfiles} from '../../../../p02-user-app/src/wallet/readProfiles';

/** Servers that do not answer, by the start of their address. */
export const network = {down: [] as string[]};
/** A token on the local chain that only this simulation knows: 20.00 tUSDT on every account. */
export const madeUpToken = '0x' + '7e57'.repeat(10);
export const addedServer = 'https://rpc.added.example/', wrongServer = 'https://rpc.wrong.example/';

const word = (value: bigint) => '0x' + value.toString(16).padStart(64, '0');
const text = (value: string) => word(32n) + BigInt(value.length).toString(16).padStart(64, '0') + Buffer.from(value).toString('hex').padEnd(64, '0');
const block = {hash: '0x' + 'b1'.repeat(32), number: '0x10'};
type Call = {method: string; params: {to?: string; data?: string}[]};

const evm = (chainId: string, genesis: string, coin: bigint) => ({method, params}: Call): unknown => {
  switch (method) {
    case 'eth_chainId': return '0x' + BigInt(chainId).toString(16);
    case 'eth_getBlockByNumber': return (params[0] as unknown) === '0x0' ? {hash: genesis, number: '0x0'} : block;
    case 'eth_getBalance': return '0x' + coin.toString(16);
    default: return '0x';
  }
};
const other: [string, (call: Call) => unknown][] = [
  [builtInServers.eth, evm('11155111', readProfiles.eth.anchor, 5n * 10n ** 17n)],
  [builtInServers.base, evm('84532', readProfiles.base.anchor, 0n)],
  [addedServer, evm('421614', '0x' + 'a1'.repeat(32), 125n * 10n ** 16n)],
  [wrongServer, evm('1', '0x' + 'a2'.repeat(32), 0n)],
  [builtInServers.sol, ({method}) => method === 'getGenesisHash' ? readProfiles.sol.anchor : undefined],
  [builtInServers.xrp, ({method}) => method === 'server_info' ? {info: {network_id: 1}}
    : method === 'ledger' ? {validated: true, ledger_hash: readProfiles.xrp.ledger, ledger_index: readProfiles.xrp.height} : undefined],
];

/** The answer of another network, or undefined when the local chain answers. */
export function answerElsewhere(endpoint: string, packet: string): string | undefined {
  if (network.down.some(start => endpoint.startsWith(start))) throw new Error('not reachable in this simulation');
  if (endpoint.startsWith(publicSignetIndexer)) {
    const path = endpoint.slice(publicSignetIndexer.length);
    return path === '/block-height/0' ? readProfiles.btc.anchor : path === `/block-height/${bitcoinSignetCheckpoint.height}` ? bitcoinSignetCheckpoint.hash : '0';
  }
  const call = JSON.parse(packet) as Call & {id: number};
  const found = other.find(([server]) => endpoint === server);
  if (!found) {
    const {to, data = ''} = call.params?.[0] ?? {};
    if (call.method !== 'eth_call' || to !== madeUpToken) return undefined;
    return JSON.stringify({jsonrpc: '2.0', id: call.id, result: data.startsWith('0x70a08231') ? word(20_000_000n) : data === '0x313ce567' ? word(6n) : text('tUSDT')});
  }
  const result = found[1](call);
  if (result === undefined) throw new Error(`not simulated: ${call.method}`);
  return JSON.stringify({jsonrpc: '2.0', id: call.id, result});
}
