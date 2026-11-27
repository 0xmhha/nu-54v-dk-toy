// Serves the world of ../app/world.ts to the user app on an emulator, and takes the steps the owner, the
// kiosk and the store do themselves. scripts/simulate_user_app.py starts it; nothing here is a check.
import * as http from 'node:http';
import {generateKeyPairSync} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {writeFileSync, writeSync} from 'node:fs';
import {join} from 'node:path';
import {encodeOrderLocator, readPickupKitchen, type PickupTarget} from '@nu54/protocol';
import {readProfiles} from '../../../../p02-user-app/src/wallet/readProfiles';
import * as world from '../app/world';
import {addedServer, answerElsewhere, madeUpToken, network, wrongServer} from './networks';

const port = 8790, life = Number(process.env.NU54_ORDER_FLOW_MINUTES) * 60_000;
/** What the phone can reach, and what its camera sees. */
const around = {camera: '', board: true, kiosk: true};

const modules: Record<string, object> = {
  CommerceHttp: {...world.commerceHttp, async publicIdentity() { return 'key'; }, async configure() {}, async discardTravelRecording() {}},
  Nearby: {...world.nearby, async scan() { return around.kiosk ? world.nearby.scan() : '[]'; }},
  OwnerWallet: {
    ...world.ownerWallet,
    async evmRpc(endpoint: string, packet: string) { return answerElsewhere(endpoint, packet) ?? world.ownerWallet.evmRpc(endpoint, packet); },
    async compareAndSaveProfile(expected: string, value: string) {
      if (world.wallet.raw !== expected) return false;
      world.wallet.raw = value;
      return true;
    },
    async getNetworkLink() { return 'available'; },
    async installationClock() { return JSON.stringify({boot: 1, elapsed: 1000}); },
    async speak() {},
  },
  RenterBle: {
    ...world.renterBle,
    async loadDevices() { return ''; },
    async saveDevices() {},
    async scanOwner() { if (around.board) await world.renterBle.scanOwner(); },
    async connectOwner() {
      if (!around.board) throw new Error('out of range');
      return world.renterBle.connectOwner();
    },
  },
  ShopQr: {async scan() { return around.camera; }},
};

const events: {name: string; value: string}[] = [];
let waiting: (() => void)[] = [];
const emit = (name: string, value: string) => { events.push({name, value}); for (const go of waiting.splice(0)) go(); };

async function kitchen(operation: string, more: object = {}) {
  const {status, raw} = await world.post(world.fixture.kitchen, '/v2/commerce', JSON.stringify({travel: {operation, branch: world.fixture.branch,
    ...(operation === 'pickup-queue' ? {includeCafe: true} : {}), ...more}}));
  if (status !== 200) throw new Error(`${operation} ${status} ${raw}`);
  return raw;
}

let authority = '', finish = () => {};
type Input = {sale: string; code: string; text: string; amount: string; quantity?: number; targets: PickupTarget[]; request: string; change: object};
const steps: Record<string, (input: Input) => unknown> = {
  around: ({change}) => Object.assign(around, change),
  // The owner at the board: how it answers ('press', 'refuse', 'wrongpin', 'ignore'), or one press now.
  board: ({change}) => { Object.assign(world.board, change); },
  press: () => world.board.command('press'),
  drop: () => world.board.drop(),
  restart: async () => { await world.board.boot(); await world.board.grant(); },
  grant: () => world.board.grant(),
  // The network.
  offline: ({change}) => { Object.assign(world.offline, change); },
  // Servers of other networks that stop answering, and what the networks and token pages can be tried with.
  servers: ({change}) => { Object.assign(network, change); return {addedServer, wrongServer, madeUpToken}; },
  lose: ({request}) => { world.radio.loseReplyTo = request; },
  walkAway: () => { world.dropNearby(); },
  // The operator.
  enrollment: () => ({version: 2, device: world.board.device, boardSPKI: world.boardSPKI, ca: 'AAAA', certificateChain: ['AAAA']}),
  operatorReset: ({text}) => execFileSync('python3', ['-B', join(__dirname, '../../../../p01-device-firmware/scripts/enroll_board.py'), 'operator-reset', text,
    '--authority', authority, '--device', world.board.device, '--yes'], {encoding: 'utf8'}).trim(),
  deposit: ({amount}) => world.control('deposit', {address: world.board.account, amount}),
  // The kiosk: an order on its screen, whose code the phone's camera then sees, and the confirmation of a number.
  async order({quantity = 1}) {
    const {prices} = await world.kiosk.api.catalogWindow(0);
    const cart = {origin: 'menu' as const, originRef: world.id(), menuVersion: prices[0].version, items: [{sku: prices[0].sku, option_key: prices[0].optionKey, quantity}]};
    const sale = world.kiosk.shown = await world.kiosk.api.reserve(cart, prices[0].asset, world.id());
    around.camera = encodeOrderLocator({branchRef: world.fixture.branch, orderRef: sale, expiresAt: world.now() + 55}, world.now());
    return {sale, locator: around.camera};
  },
  async confirm({sale, code}) {
    if (world.kiosk.codes.includes(code)) { world.kiosk.relay.approveDisplayed(code); return; }
    const found = (await world.kiosk.api.pendingComparisons(sale)).find(item => item.code === code);
    if (!found) throw new Error('no such number on the kiosk');
    await world.kiosk.api.confirmComparison(sale, found, world.id());
  },
  // The kitchen.
  async cook({sale, targets}) {
    for (const target of targets) {
      const row = await world.until('the order in the kitchen', async () => {
        const found = readPickupKitchen(await kitchen('pickup-queue')).find(item => item.saleRef === sale);
        return found && found.state !== 'checking' ? found : undefined;
      });
      await kitchen('pickup-change', {sale, requestKey: world.id(), expectedRevision: row.revision, target});
    }
  },
  state: async () => ({device: world.board.device, account: world.board.account, home: world.fixture.home, balance: String(await world.balance(world.board.account)),
    prompts: world.board.prompts, lamps: world.board.lamps, locks: world.kiosk.locks, camera: around.camera}),
  stop: () => { finish(); },
};

test('serve the app until it is stopped', async () => {
  world.anchor.hash = readProfiles.stable.anchor;
  const key = generateKeyPairSync('ec', {namedCurve: 'prime256v1'});
  authority = join(world.fixture.directory, 'authority.pem');
  writeFileSync(authority, key.privateKey.export({type: 'pkcs8', format: 'pem'}), {mode: 0o600});
  await world.startBoard(key.publicKey.export({type: 'spki', format: 'der'}).subarray(-65).toString('hex'));
  await world.startKiosk();
  await world.control('quote-lifetime', {seconds: 100});
  world.renterBle.onScan(value => emit('RenterBle.onScan', value));
  world.renterBle.onFragment(value => emit('RenterBle.onFragment', value));
  world.renterBle.onDisconnect(() => emit('RenterBle.onDisconnect', ''));
  world.nearby.onNearby(value => emit('Nearby.onNearby', value));
  const server = http.createServer((request, reply) => {
    const send = (value: unknown) => { reply.setHeader('Content-Type', 'application/json'); reply.end(JSON.stringify(value)); };
    // Pages of a browser on this machine have an origin; the app and the runner have none.
    if (request.headers.origin) { reply.statusCode = 403; reply.end(); return; }
    const url = new URL(request.url!, 'http://host');
    if (url.pathname === '/events') {
      const since = Number(url.searchParams.get('since'));
      if (since < 0) return send({next: events.length, events: []});
      if (since < events.length) return send({next: events.length, events: events.slice(since)});
      const go = () => { clearTimeout(timer); send({next: events.length, events: events.slice(since)}); };
      const timer = setTimeout(() => { waiting = waiting.filter(item => item !== go); go(); }, 10_000);
      waiting.push(go);
      return;
    }
    let raw = '';
    request.setEncoding('utf8').on('data', text => { raw += text; }).on('end', async () => {
      try {
        const body = JSON.parse(raw || '{}');
        if (url.pathname === '/step') {
          if (!Object.hasOwn(steps, body.step)) throw new Error('unknown step ' + body.step);
          return send({value: await steps[body.step](body) ?? null});
        }
        const found = Object.hasOwn(modules, body.module) ? modules[body.module] as Record<string, unknown> : {};
        const run = Object.hasOwn(found, body.method) ? found[body.method] : undefined;
        if (typeof run !== 'function') throw Object.assign(new Error(`not simulated: ${body.module}.${body.method}`), {code: 'NOT_SIMULATED'});
        send({value: await run(...body.args) ?? null});
      } catch (error) {
        send({error: (error as Error).message || 'error', code: (error as {code?: string}).code});
      }
    });
  });
  await new Promise<void>((resolve, reject) => { server.once('error', reject).listen(port, '127.0.0.1', resolve); });
  writeSync(1, 'SIMULATOR-READY\n');
  await new Promise<void>(resolve => { finish = resolve; setTimeout(resolve, life - 120_000); });
  world.stopWorld();
  server.closeAllConnections();
  server.close();
}, 60 * 60_000);
