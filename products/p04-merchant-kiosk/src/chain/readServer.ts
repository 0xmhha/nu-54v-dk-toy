import { readServer } from '@nu54/protocol';

/** The first block of StableNet Testnet (chain 8283). A server of another chain cannot show it. */
export const stableNetGenesis = '0x2bdf79b3d3cc49f9e6638ff81f3bb85065c79945a8fe4556cd0ff47bbfc02490';
const timeoutMs = 8_000;

type Fetch = (url: string, init: { method: string; headers: Record<string, string>; body: string; signal: AbortSignal }) =>
  Promise<{ ok: boolean; json(): Promise<unknown> }>;

/**
 * Asks a server which chain it serves, before the kiosk uses it. Throws READ_SERVER_ADDRESS for an address that is not
 * plain HTTPS, READ_SERVER_CHAIN for another chain and READ_SERVER_UNREACHABLE when it does not answer.
 */
export async function checkReadServer(url: string, request: Fetch = fetch as unknown as Fetch): Promise<{ height: number }> {
  if (!readServer(url)) throw new Error('READ_SERVER_ADDRESS');
  const stop = new AbortController();
  const timer = setTimeout(() => stop.abort(), timeoutMs);
  const call = async (id: number, method: string, params: unknown[]) => {
    const response = await request(url, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ jsonrpc: '2.0', id, method, params }),
      signal: stop.signal,
    });
    if (!response.ok) throw new Error('READ_SERVER_UNREACHABLE');
    const reply = (await response.json()) as { id?: unknown; result?: unknown };
    if (!reply || reply.id !== id) throw new Error('READ_SERVER_UNREACHABLE');
    return reply.result;
  };
  let chain: unknown;
  let first: unknown;
  let head: unknown;
  try {
    chain = await call(1, 'eth_chainId', []);
    first = await call(2, 'eth_getBlockByNumber', ['0x0', false]);
    head = await call(3, 'eth_blockNumber', []);
  } catch {
    throw new Error('READ_SERVER_UNREACHABLE');
  } finally {
    clearTimeout(timer);
  }
  const block = first as { number?: unknown; hash?: unknown } | null;
  if (chain !== '0x205b' || block?.number !== '0x0' || block.hash !== stableNetGenesis) throw new Error('READ_SERVER_CHAIN');
  const height = typeof head === 'string' && /^0x(0|[1-9a-f][0-9a-f]{0,12})$/.test(head) ? Number(head) : NaN;
  if (!Number.isSafeInteger(height)) throw new Error('READ_SERVER_UNREACHABLE');
  return { height };
}
