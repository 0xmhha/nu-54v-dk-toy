import { readFileSync } from 'fs';
import { join } from 'path';
import { checkReadServer, stableNetGenesis } from '../src/chain/readServer';
import { appVersion, licenseText, notice, notices } from '../src/legal/notices';
import { idleMs, screenMs } from '../src/kiosk/orderProgress';

const reply = (results: Record<string, unknown>) => async (_url: string, init: { body: string }) => {
  const { id, method } = JSON.parse(init.body);
  return { ok: true, json: async () => ({ id, result: results[method] }) };
};
const good = { eth_chainId: '0x205b', eth_getBlockByNumber: { number: '0x0', hash: stableNetGenesis }, eth_blockNumber: '0x2a' };

test('a read server must be HTTPS and show chain 8283 with its first block', async () => {
  expect(await checkReadServer('https://rpc.example/', reply(good))).toEqual({ height: 42 });
  for (const url of ['http://rpc.example', 'https://a@rpc.example', 'https://rpc.example/?key=1', ''])
    await expect(checkReadServer(url, reply(good))).rejects.toThrow('READ_SERVER_ADDRESS');
  for (const change of [{ eth_chainId: '0x1' }, { eth_chainId: 8283 }, { eth_getBlockByNumber: null },
    { eth_getBlockByNumber: { number: '0x0', hash: '0x' + '0'.repeat(64) } }, { eth_getBlockByNumber: { number: '0x1', hash: stableNetGenesis } }])
    await expect(checkReadServer('https://rpc.example/', reply({ ...good, ...change }))).rejects.toThrow('READ_SERVER_CHAIN');
  await expect(checkReadServer('https://rpc.example/', reply({ ...good, eth_blockNumber: undefined }))).rejects.toThrow('READ_SERVER_UNREACHABLE');
  await expect(checkReadServer('https://rpc.example/', async () => ({ ok: false, json: async () => ({}) }))).rejects.toThrow('READ_SERVER_UNREACHABLE');
  await expect(checkReadServer('https://rpc.example/', async () => ({ ok: true, json: async () => ({ id: 99, result: '0x205b' }) }))).rejects.toThrow('READ_SERVER_UNREACHABLE');
});

test('the shown version is the Android one, and every notice and licence text exists', () => {
  const gradle = readFileSync(join(__dirname, '../android/app/build.gradle'), 'utf8');
  expect(/versionName "([^"]+)"/.exec(gradle)?.[1]).toBe(appVersion);
  for (const id of ['terms', 'privacy', 'test-assets'] as const)
    for (const language of ['ko', 'en'] as const) expect(notice(id, language)?.sections.length).toBeGreaterThan(0);
  for (const license of notices.licenses) expect(licenseText(license).length).toBeGreaterThan(0);
  expect(notice('terms', 'en', [{ id: 'terms', language: 'ko', title: 'x', edition: '', status: 'draft', effective: null, sections: [] }])?.title).toBe('x');
});

test('the idle time changes only the time without a touch', () => {
  expect(screenMs('open', undefined, 0)).toBe(idleMs);
  expect(screenMs('open', undefined, 0, 120_000)).toBe(120_000);
  expect(screenMs('approving', 10, 0, 30_000)).toBe(40_000);
  expect(screenMs('paid', undefined, 0, 120_000)).toBe(10_000);
});
