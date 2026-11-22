import {bytesToHex, hexToBytes} from '@nu54/protocol';
import {bitcoinSignetCheckpoint, checkSegwitV0, doubleSha256, segwitAddress} from './chainAccounts.ts';

export const publicSignetIndexer = 'https://mempool.space/signet/api';
const hash = /^[a-f0-9]{64}$/;
const addressPattern = /^tb1q[a-z0-9]{38}$/;
const object = (value: unknown): Record<string, unknown> => {
  if (!value || Array.isArray(value) || typeof value !== 'object') throw new Error('BTC_INDEX_SCHEMA');
  return value as Record<string, unknown>;
};
const count = (value: unknown) => {
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0) throw new Error('BTC_INDEX_SCHEMA');
  return value;
};
const hex = (bytes: Uint8Array) => bytesToHex(bytes, false);
const digest = (bytes: Uint8Array) => hex(doubleSha256(bytes).reverse());

export function signetIndexer(exchange: (url: string, packet: string) => Promise<string>) {
  return async (path: string) => {
    const result = await exchange(publicSignetIndexer + path, '{"method":"nu54_signetRead"}');
    if (result.length > 256000) throw new Error('BTC_INDEX_SCHEMA');
    return result.trim();
  };
}
type Get = ReturnType<typeof signetIndexer>;

export async function verifyPublicSignet(get: Get, genesis: string) {
  if (await get('/block-height/0') !== genesis ||
      await get('/block-height/' + bitcoinSignetCheckpoint.height) !== bitcoinSignetCheckpoint.hash)
    throw new Error('WALLET_READ_CHAIN');
}
function outputFromTransaction(raw: string, txid: string, vout: number, program: string, value: number) {
  if (raw.length < 20 || raw.length > 200000 || raw.length % 2 || !/^[a-f0-9]+$/.test(raw)) throw new Error('BTC_INDEX_TX');
  const bytes = hexToBytes(raw);
  let pos = 0;
  const take = (n: number) => {
    if (n > bytes.length - pos) throw new Error('BTC_INDEX_TX');
    const part = bytes.subarray(pos, pos + n);
    pos += n;
    return part;
  };
  const little = (part: Uint8Array) => part.reduceRight((n, byte) => n * 256n + BigInt(byte), 0n);
  const compact = () => {
    const start = take(1)[0];
    if (start < 253) return start;
    const n = little(take(start === 253 ? 2 : start === 254 ? 4 : 8));
    if (n < (start === 253 ? 253n : start === 254 ? 65536n : 4294967296n) || n > 100000n)
      throw new Error('BTC_INDEX_TX');
    return Number(n);
  };
  const version = take(4);
  const witness = bytes[pos] === 0 && bytes[pos + 1] === 1;
  if (witness) take(2);
  const start = pos;
  const inputs = compact();
  if (!inputs) throw new Error('BTC_INDEX_TX');
  for (let i = 0; i < inputs; i++) {
    take(36); take(compact()); take(4);
  }
  const outputs = compact();
  if (vout >= outputs) throw new Error('BTC_INDEX_TX');
  for (let i = 0; i < outputs; i++) {
    const amount = little(take(8));
    const script = take(compact());
    if (i === vout && (amount !== BigInt(value) || hex(script) !== program)) throw new Error('BTC_INDEX_OUTPUT');
  }
  const bodyEnd = pos;
  if (witness) {
    for (let i = 0; i < inputs; i++) {
      const items = compact();
      for (let j = 0; j < items; j++) take(compact());
    }
  }
  const lock = take(4);
  if (pos !== bytes.length) throw new Error('BTC_INDEX_TX');
  const stripped = new Uint8Array(8 + bodyEnd - start);
  stripped.set(version); stripped.set(bytes.subarray(start, bodyEnd), 4); stripped.set(lock, stripped.length - 4);
  if (digest(stripped) !== txid) throw new Error('BTC_INDEX_TXID');
}

async function checkedHeader(get: Get, block: string) {
  const header = await get('/block/' + block + '/header');
  if (!/^[a-f0-9]{160}$/.test(header) || digest(hexToBytes(header)) !== block) throw new Error('BTC_INDEX_HEADER');
  return header;
}
async function tip(get: Get) {
  const height = await get('/blocks/tip/height');
  if (!/^[1-9][0-9]{0,8}$/.test(height) || Number(height) < bitcoinSignetCheckpoint.height)
    throw new Error('BTC_INDEX_TIP');
  const head = await get('/block-height/' + height);
  if (!hash.test(head) || await get('/blocks/tip/hash') !== head) throw new Error('BTC_INDEX_TIP');
  return {height: Number(height), head};
}

export async function readPublicSignet(get: Get, genesis: string, address: string, program: string) {
  if (!addressPattern.test(address)) throw new Error('BTC_INDEX_ADDRESS');
  checkSegwitV0('tb', address);
  if (!/^0014[a-f0-9]{40}$/.test(program) || segwitAddress('tb', hexToBytes(program)) !== address)
    throw new Error('BTC_INDEX_ADDRESS');
  await verifyPublicSignet(get, genesis);
  const start = await tip(get);
  await checkedHeader(get, start.head);
  const entries: unknown = JSON.parse(await get('/address/' + address + '/utxo'));
  const addressInfo = object(JSON.parse(await get('/address/' + address)));
  const history = count(object(addressInfo.chain_stats).tx_count);
  if (addressInfo.address !== address || !Array.isArray(entries) || entries.length > 64)
    throw new Error('BTC_INDEX_SCHEMA');
  let total = 0n;
  const seen = new Set<string>();
  for (const entry of entries) {
    const row = object(entry), state = object(row.status);
    if (state.confirmed === false) continue;
    if (state.confirmed !== true || typeof row.txid !== 'string' || !hash.test(row.txid) ||
        typeof state.block_hash !== 'string' || !hash.test(state.block_hash)) throw new Error('BTC_INDEX_SCHEMA');
    const height = count(state.block_height);
    if (height === 0 || height > start.height) throw new Error('BTC_INDEX_SCHEMA');
    if (start.height - height < 5) continue;
    const txid = row.txid, vout = count(row.vout), value = count(row.value), block = state.block_hash;
    if (value > 2100000000000000 || seen.has(txid + ':' + vout)) throw new Error('BTC_INDEX_SCHEMA');
    seen.add(txid + ':' + vout);
    if (await get('/block-height/' + height) !== block) throw new Error('BTC_INDEX_BLOCK');
    const header = await checkedHeader(get, block);
    const proof = object(JSON.parse(await get('/tx/' + txid + '/merkle-proof')));
    if (proof.block_height !== height || !Array.isArray(proof.merkle) || proof.merkle.length > 32)
      throw new Error('BTC_INDEX_PROOF');
    let index = count(proof.pos), current = hexToBytes(txid).reverse();
    for (const sibling of proof.merkle) {
      if (typeof sibling !== 'string' || !hash.test(sibling)) throw new Error('BTC_INDEX_PROOF');
      const other = hexToBytes(sibling).reverse();
      current = doubleSha256(index % 2 ? Uint8Array.of(...other, ...current) : Uint8Array.of(...current, ...other));
      index = Math.floor(index / 2);
    }
    if (index || hex(current) !== header.slice(72, 136)) throw new Error('BTC_INDEX_PROOF');
    outputFromTransaction(await get('/tx/' + txid + '/hex'), txid, vout, program, value);
    total += BigInt(value);
  }
  if ((seen.size && !history) || total > 2100000000000000n || (await tip(get)).head !== start.head)
    throw new Error('BTC_INDEX_STALE');
  return {amount: total.toString(), point: 'Signet indexed block ' + start.height + ' · ' + start.head,
    used: history > 0};
}
