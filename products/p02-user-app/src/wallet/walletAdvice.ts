import {useEffect, useRef, useState} from 'react';
import {paymentToken, readNetworkList, type CommercePaymentProfile, type NetworkList} from '@nu54/protocol';
import {orderPaymentPin} from '../merchant/orderPaymentTrust.ts';
import {ordersWithoutRegistration} from '../merchant/orderRoute.ts';
import {NETWORK} from '../setup/network.ts';
import {credential} from './BoardLife.tsx';
import {checkServer, networkRows, type Exchange} from './networks.ts';
import {readEndpoint, readProfiles, type ReadProfileId, type ReadProfileState} from './readProfiles.ts';
import {registeredStore} from './registeredStore.ts';
import {storeOf, storeToken} from './tokens.ts';
import type {WalletAdvice} from './WalletTab.tsx';

/** The operator's names of the networks that have no EVM chain ID. */
const listed: Record<string, ReadProfileId> = {'bitcoin-signet': 'btc', 'solana-devnet': 'sol', 'xrpl-testnet': 'xrp'};

/** What a verified operator list says about the networks this app knows. It cannot add one or change what one is. */
export function fromOperator(list: NetworkList): WalletAdvice {
  const advice: Required<Pick<WalletAdvice, 'operator' | 'pages' | 'listed'>> = {operator: {}, pages: {}, listed: {}};
  const page = (value?: string) => { try { return value && readEndpoint(value); } catch { return undefined; } };
  for (const row of list.networks) {
    const id = row.id ? listed[row.id] : (Object.keys(readProfiles) as ReadProfileId[]).find(key => {
      const profile = readProfiles[key];
      return 'evm' in profile && profile.evm === String(row.chainId);
    });
    const server = page(row.rpc);
    if (!id || !server) continue;
    advice.operator[id] = server;
    const explorer = page(row.explorer), gasCoin = page(row.gasCoinUrl);
    if (explorer || gasCoin) advice.pages[id] = {...(explorer ? {explorer} : {}), ...(gasCoin ? {gasCoin} : {})};
    if (row.tokens?.length) advice.listed[id] = row.tokens.map(({address, symbol, decimals}) => ({address, symbol, decimals}));
  }
  return advice;
}

/**
 * What a build that orders from store pages knows without any registration: the token of the order contract it binds
 * is the one a board of this build pays in, and the contract's read server is where its chain is asked
 * (customerEntranceDeployment.ts). Undefined in a build of the earlier route, where a store's registration says both.
 */
export function buildAdvice(): WalletAdvice | undefined {
  const pin = ordersWithoutRegistration() ? orderPaymentPin() : undefined;
  if (!pin) return undefined;
  return {...(pin.rpc ? {store: pin.rpc} : {}), signed: [storeToken(pin as unknown as CommercePaymentProfile)],
    registered: {asset: pin.asset.toLowerCase(), token: paymentToken(pin)}};
}

/**
 * Servers and tokens others named for this phone: the store registration's StableNet server and the one token
 * the board's firmware signs, and the operator's signed list. `round` changes when the registration may have.
 * A phone without a registration takes them from the order contract of its build, when the build binds one.
 */
export function useWalletAdvice(active: boolean, round: unknown, sequence: number, keepSequence: (value: number) => void): WalletAdvice | undefined {
  const [store, setStore] = useState<WalletAdvice>(), [operator, setOperator] = useState<WalletAdvice>();
  const latest = useRef({sequence, keepSequence});
  latest.current = {sequence, keepSequence};
  const asked = useRef(0);
  useEffect(() => {
    if (!active) return;
    let live = true;
    const same = (next: WalletAdvice) => (old?: WalletAdvice) => JSON.stringify(old) === JSON.stringify(next) ? old : next;
    credential().then(async ({client, policy}) => {
      if (live) setStore(same({...(policy.rpc ? {store: policy.rpc} : {}), signed: [storeToken(policy)], registered: storeOf(policy)}));
      if (Date.now() - asked.current < 600_000) return;
      asked.current = Date.now();
      // A list that is missing, unsigned, expired or older than one seen before leaves the other servers in use.
      const list = readNetworkList(await client.networkList(), {operator: NETWORK.operator}, Math.floor(Date.now() / 1000), latest.current.sequence);
      if (live) setOperator(same(fromOperator(list)));
      if (list.sequence > latest.current.sequence) latest.current.keepSequence(list.sequence);
    }).catch(async () => {
      // A store with another contract still names the token of its amounts.
      const registered = await registeredStore().catch(() => undefined);
      const build = registered ? undefined : buildAdvice();
      if (live) setStore(old => build ? same(build)(old) : old ?? (registered ? {registered} : {}));
    });
    return () => { live = false; };
  }, [active, round]);
  const [merged, setMerged] = useState<WalletAdvice>();
  useEffect(() => { if (store) setMerged({...operator, ...store}); }, [store, operator]);
  return merged;
}

/** Whether the server of the board's network answers, asked while `active`. */
export function useBoardServer(active: boolean, state: ReadProfileState | undefined, advice: WalletAdvice | undefined, exchange: Exchange | undefined) {
  const row = networkRows(state, advice).find(item => item.key === 'stable')!;
  const [health, setHealth] = useState<{server: string; ok: boolean}>();
  const server = row.server;
  useEffect(() => {
    if (!active || !exchange) return;
    let live = true;
    const ask = () => checkServer({key: 'stable'}, server, exchange).then(() => true, () => false).then(ok => { if (live) setHealth({server, ok}); });
    ask();
    const timer = setInterval(ask, 60_000);
    return () => { live = false; clearInterval(timer); };
  }, [active, server, exchange]);
  return health?.server === server ? (health.ok ? 'ok' : 'failed') : undefined;
}
