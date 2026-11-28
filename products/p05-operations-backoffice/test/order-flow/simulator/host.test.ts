// Serves the world of ../app/world.ts to the user app on an emulator, and takes the steps the owner, the
// kiosk and the store do themselves. scripts/simulate_user_app.py starts it; nothing here is a check.
import * as http from 'node:http';
import {generateKeyPairSync} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {writeFileSync, writeSync} from 'node:fs';
import {join} from 'node:path';
import {encodeOrderLocator, makeEntranceQr, readPickupKitchen, type PickupTarget} from '@nu54/protocol';
import {readProfiles} from '../../../../p02-user-app/src/wallet/readProfiles';
import * as world from '../app/world';
import {customerEntrance, describeCall, type EntranceCall} from './entranceHttp';
import {addedServer, answerElsewhere, madeUpToken, network, wrongServer} from './networks';
import {writeOrderBinding} from './orderBinding';

const port = 8790, life = Number(process.env.NU54_ORDER_FLOW_MINUTES) * 60_000;
/**
 * What the phone can reach, and what its camera sees. `entrance`: whether the store server's customer entrance can be
 * reached. `loseReply`: the answer to the next entrance request of this operation (or path) is lost on its way back.
 */
const around = {camera: '', board: true, kiosk: true, entrance: true, loseReply: ''};
/**
 * The requests the phone sent to the customer entrance (the last ones, and how many of each kind got which answer),
 * and the last pickup QR a store's scanner would read.
 */
const sent: EntranceCall[] = [];
const answered: Record<string, number> = {};
let pickupToken = '';

// A run with a customer entrance is a phone that never registered at a store: it holds no store's registration, and
// everything about an order goes through the entrance (entranceHttp.ts).
const registrationFree = !!world.fixture.entrance;
async function customerRequest(anchor: string, path: string, session: string, body: string) {
  const call = describeCall(path, body);
  sent.push(call);
  if (sent.length > 40) sent.shift();
  const lost = () => Object.assign(new Error('The enrolled commerce connection is unavailable'), {code: 'COMMERCE_UNAVAILABLE'});
  const count = (answer: string | number) => {
    const kind = `${call.operation ?? call.path.replace(/^.*\/customer\//, '').replace(/\?.*$/, '')} ${answer}`;
    answered[kind] = (answered[kind] ?? 0) + 1;
  };
  try {
    if (!around.entrance) throw lost();
    const raw = await customerEntrance(anchor, path, session, body);
    const reply = JSON.parse(raw) as {status: number; body: string; retryAfter?: number};
    Object.assign(call, {status: reply.status}, reply.retryAfter ? {retryAfter: reply.retryAfter} : {});
    count(reply.status);
    if (around.loseReply && around.loseReply === (call.operation ?? path)) {
      around.loseReply = '';
      throw lost();
    }
    // The QR the phone then shows holds this value; the store's scanner reads it from the screen.
    if (path === '/v2/customer/proof' && reply.status === 200) pickupToken = (JSON.parse(reply.body) as {pickupToken?: string}).pickupToken ?? pickupToken;
    return raw;
  } catch (error) {
    call.failed = (error as {code?: string}).code ?? 'error';
    if (call.status === undefined) count('lost');
    throw error;
  }
}

const modules: Record<string, object> = {
  CommerceHttp: {...world.commerceHttp, async publicIdentity() { return 'key'; }, async configure() {}, async discardTravelRecording() {},
    ...(registrationFree ? {
      customerRequest,
      async loadProfile() { return ''; },
      async exchangeScoped() { throw Object.assign(new Error('The enrolled commerce connection is unavailable'), {code: 'COMMERCE_UNAVAILABLE'}); },
    } : {})},
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

/** One change on the operations web, by the store's operator: the same routes, origin and certificate. */
async function operate(path: string, body: object) {
  const {status, raw} = await world.post(world.fixture.kitchen, path, JSON.stringify(body));
  return {status, body: raw ? JSON.parse(raw) as Record<string, unknown> : null};
}
async function operated(path: string, body: object) {
  const done = await operate(path, body);
  if (done.status !== 200) throw new Error(`${path} ${done.status} ${JSON.stringify(done.body)}`);
  return done.body;
}
const storeOf = (name?: string) => {
  const branch = !name || name === 'cafe' ? world.fixture.branch : world.fixture.stores?.[name];
  if (!branch) throw new Error('no such store ' + name);
  return branch;
};
const weekdays = {opens: '08:00', closes: '22:00'}, weekend = {opens: '09:00', closes: '23:00'};
/** What each store of the run wrote on its store page. A field left empty is not on the page. */
const pages: Record<string, object> = {
  cafe: {listed: true, kind: '카페', intro: '직접 볶은 원두로 내리는 동네 카페입니다.', address: '서울 종로구 자하문로 12', directions: '경복궁역 2번 출구에서 걸어서 3분',
    phone: '02-555-0142', amenities: '와이파이, 콘센트 좌석', parking: '주차 불가, 건너편 공영주차장 이용', closedNote: '매달 첫째 월요일',
    hours: [weekdays, weekdays, weekdays, weekdays, weekdays, weekend, weekend]},
  hanok: {listed: true, kind: '찻집', intro: '한옥 마루에서 마시는 전통차.', address: '서울 종로구 북촌로 31', directions: '', phone: '02-555-0177', amenities: '', parking: '',
    closedNote: '', hours: [weekdays, weekdays, weekdays, weekdays, weekdays, weekend, null]},
  seochon: {listed: true, kind: '빵집', intro: '아침마다 굽는 식사빵.', address: '서울 종로구 필운대로 5', directions: '', phone: '', amenities: '', parking: '', closedNote: '', hours: null},
  night: {listed: true, kind: '술집', intro: '', address: '서울 마포구 와우산로 21', directions: '', phone: '', amenities: '', parking: '', closedNote: '',
    hours: [{opens: '18:00', closes: '02:00'}, {opens: '18:00', closes: '02:00'}, {opens: '18:00', closes: '02:00'}, {opens: '18:00', closes: '02:00'},
      {opens: '18:00', closes: '03:00'}, {opens: '18:00', closes: '03:00'}, null]},
  busy: {listed: true, kind: '국수', intro: '줄 서서 먹는 잔치국수.', address: '서울 중구 을지로 88', directions: '', phone: '', amenities: '', parking: '', closedNote: '', hours: null},
  garden: {listed: true, kind: '브런치', intro: '정원이 보이는 브런치 카페.', address: '서울 용산구 이태원로 200', directions: '', phone: '', amenities: '', parking: '', closedNote: '', hours: null},
  alley: {listed: false, kind: '분식', intro: '바로가기 QR로만 여는 작은 가게.', address: '', directions: '', phone: '', amenities: '', parking: '', closedNote: '', hours: null},
};
/** The operator publishes every store page and sets the states the store finder shows. */
async function openStores() {
  for (const [name, page] of Object.entries(pages)) await operated(`/v2/operations/store-page/${storeOf(name)}`, {published: true, ...page});
  // The store's own day, as the operations web shows it.
  const {today} = JSON.parse((await world.read(world.fixture.kitchen, `/v2/operations/state/${storeOf('hanok')}`)).raw) as {today: string};
  await operated(`/v2/operations/state/${storeOf('hanok')}/closed-day`, {day: today, closed: true});
  await operated(`/v2/operations/state/${storeOf('seochon')}/mode`, {mode: 'paused', resumeAt: '15:30'});
  await operated(`/v2/operations/state/${storeOf('busy')}/mode`, {mode: 'busy', busyMinutes: 15});
}

let authority = '', finish = () => {};
type Input = {sale: string; code: string; text: string; amount: string; quantity?: number; targets: PickupTarget[]; request: string; change: object;
  store?: string; action?: string; sku?: string; count?: number; kind?: string; perMinute?: number; burst?: number};
const steps: Record<string, (input: Input) => unknown> = {
  around: ({change}) => Object.assign(around, change),
  // The owner at the board: how it answers ('press', 'refuse', 'wrongpin', 'ignore'), or one press now.
  board: ({change}) => { Object.assign(world.board, change); },
  press: () => world.board.command('press'),
  drop: () => world.board.drop(),
  restart: async () => { await world.board.boot(); await world.board.grant(); },
  // The owner resets the board with its own buttons, away from the phone; the operator then permits it again.
  // Answers false when the board refused, as it does while an approved order still holds it.
  factoryReset: async () => {
    const erased = await world.board.factoryReset();
    if (erased) await world.board.grant();
    return erased;
  },
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
  // ---- a run with a customer entrance ----
  // The stores of the run by short name, the entrance as the build pins it, and the store server's business day.
  stores: () => ({cafe: world.fixture.branch, ...world.fixture.stores, entrance: world.fixture.entrance?.origin}),
  // The camera sees a store's shortcut QR, or whatever text is given.
  shortcut: ({store, text}) => { around.camera = text ?? makeEntranceQr(storeOf(store)); return around.camera; },
  // The operator on the operations web: a store page (the whole page, as the web sends it), and a store's state
  // ('mode', 'closed-day', 'sold-out').
  page: ({store, change}) => operated(`/v2/operations/store-page/${storeOf(store)}`, {published: true, ...pages[store ?? 'cafe'], ...change}),
  storeState: ({store, action, change}) => operated(`/v2/operations/state/${storeOf(store)}/${action}`, change),
  // The kitchen's queue of the store that takes orders: the orders the phone placed, newest last.
  async orders() { return readPickupKitchen(await kitchen('pickup-queue')); },
  // The store's scanner reads the pickup QR on the phone's screen: the operator's route of the operations web.
  scan: ({sale, text}) => operate(`/v2/operations/pickup-scan/${world.fixture.branch}/${sale}`, {token: text ?? pickupToken}),
  // What the phone sent to the entrance, oldest first; `change` sets what it can reach.
  entrance: ({change}) => { Object.assign(around, change); return {sent: [...sent], answered: {...answered}, pickupToken: !!pickupToken}; },
  // The server's side of a run: what is left of a product, the end of an unpaid cart's hold, a small request limit.
  stock: ({store, sku, count}) => world.control('stock', {store, sku, count}),
  endHold: () => world.control('end-hold'),
  limit: ({kind, perMinute = 0, burst = 0}) => world.control('entrance-limit', {kind, perMinute, burst}),
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
  if (registrationFree) {
    await openStores();
    // What the app of this run binds, for the bundler configuration of this run alone (metro.config.js).
    if (process.env.NU54_SIMULATOR_ORDER_BINDING)
      writeOrderBinding(process.env.NU54_SIMULATOR_ORDER_BINDING, {entrance: world.fixture.entrance!, payment: world.fixture.payment});
  }
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
