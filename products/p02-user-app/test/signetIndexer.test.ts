import {bytesToHex, hexToBytes} from '@nu54/protocol';
import {bitcoinSignetCheckpoint, doubleSha256, segwitAddress} from '../src/wallet/chainAccounts.ts';
import {readProfiles} from '../src/wallet/readProfiles.ts';
import {publicSignetIndexer, readPublicSignet, signetIndexer} from '../src/wallet/signetIndexer.ts';

const program = '0014' + '12'.repeat(20);
const address = segwitAddress('tb', hexToBytes(program));
const raw = '0100000001' + '00'.repeat(32) + 'ffffffff0100ffffffff01640000000000000016' + program + '00000000';
const txHash = doubleSha256(hexToBytes(raw));
const hex = (bytes: Uint8Array) => bytesToHex(bytes, false);
const header = '00'.repeat(36) + hex(txHash) + '00'.repeat(12);
const block = hex(doubleSha256(hexToBytes(header)).reverse());
const txid = hex(txHash.slice().reverse());
const current = bitcoinSignetCheckpoint.height + 10;
const txHeight = current - 5;
const tipHeader = '00'.repeat(80);
const head = hex(doubleSha256(hexToBytes(tipHeader)).reverse());

function oracle() {
  const contract: Record<string, string> = {
    '/block-height/0': readProfiles.btc.anchor,
    ['/block-height/' + bitcoinSignetCheckpoint.height]: bitcoinSignetCheckpoint.hash,
    '/blocks/tip/height': String(current), '/blocks/tip/hash': head, ['/block-height/' + current]: head,
    ['/block/' + head + '/header']: tipHeader,
    ['/block-height/' + txHeight]: block, ['/block/' + block + '/header']: header,
    ['/address/' + address]: JSON.stringify({address, chain_stats: {tx_count: 1}}),
    ['/address/' + address + '/utxo']: JSON.stringify([{txid, vout: 0, value: 100,
      status: {confirmed: true, block_height: txHeight, block_hash: block}}]),
    ['/tx/' + txid + '/merkle-proof']: JSON.stringify({block_height: txHeight, pos: 0, merkle: []}),
    ['/tx/' + txid + '/hex']: raw,
  };
  const exchange = jest.fn(async (url: string, packet: string) => {
    if (packet !== '{"method":"nu54_signetRead"}' || !url.startsWith(publicSignetIndexer)) throw Error('unexpected');
    const value = contract[url.slice(publicSignetIndexer.length)];
    if (value === undefined) throw Error('unexpected route');
    return value;
  });
  return {contract, exchange, run: () => readPublicSignet(signetIndexer(exchange), readProfiles.btc.anchor, address, program)};
}

it('verifies indexed UTXO amount, output, txid, header and inclusion, then deduplicates address disclosure', async () => {
  const run = oracle();
  await expect(run.run()).resolves.toMatchObject({amount: '100', used: true});
  const addressed = run.exchange.mock.calls.filter(([url]) => url.includes('/address/'));
  expect(addressed.map(([url]) => url)).toEqual([
    publicSignetIndexer + '/address/' + address + '/utxo', publicSignetIndexer + '/address/' + address,
  ]);
});
it('shows spent history even when no confirmed UTXO remains', async () => {
  const run = oracle();
  run.contract['/address/' + address + '/utxo'] = '[]';
  await expect(run.run()).resolves.toMatchObject({amount: '0', used: true});
});
it('stops a same-genesis foreign Signet before disclosing the address', async () => {
  const run = oracle();
  run.contract['/block-height/' + bitcoinSignetCheckpoint.height] = '0'.repeat(64);
  await expect(run.run()).rejects.toThrow('WALLET_READ_CHAIN');
  expect(run.exchange.mock.calls.some(([url]) => url.includes('/address/'))).toBe(false);
});
it('rejects false merkle proofs and output values even at a valid chain tip', async () => {
  const bad = oracle();
  bad.contract['/tx/' + txid + '/merkle-proof'] = JSON.stringify({block_height: txHeight, pos: 1, merkle: []});
  await expect(bad.run()).rejects.toThrow('BTC_INDEX_PROOF');
  const badAmount = oracle();
  badAmount.contract['/address/' + address + '/utxo'] = badAmount.contract['/address/' + address + '/utxo']
    .replace('"value":100', '"value":101');
  await expect(badAmount.run()).rejects.toThrow('BTC_INDEX_OUTPUT');
});
it('excludes insufficient confirmations and rejects a mismatched active tip', async () => {
  const shallow = oracle();
  shallow.contract['/address/' + address + '/utxo'] = shallow.contract['/address/' + address + '/utxo']
    .replace('"block_height":' + txHeight, '"block_height":' + (txHeight + 1));
  await expect(shallow.run()).resolves.toMatchObject({amount: '0', used: true});
  expect(shallow.exchange.mock.calls.some(([url]) => url.endsWith('/merkle-proof'))).toBe(false);
  const invalid = oracle();
  invalid.contract['/block/' + head + '/header'] = '01'.repeat(80);
  await expect(invalid.run()).rejects.toThrow('BTC_INDEX_HEADER');
  expect(invalid.exchange.mock.calls.some(([url]) => url.includes('/address/'))).toBe(false);
});
