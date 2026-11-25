// Everything around the product code: the native modules of the phone, the radio between phone and
// kiosk, HTTPS with client certificates, and the fixture's control of chain and server.
import {createHash, generateKeyPairSync, randomBytes, sign} from 'node:crypto';
import {readFileSync} from 'node:fs';
import * as http from 'node:http';
import * as https from 'node:https';
import {CommerceClient, NearbyCommerceLink, paymentToken, type CommercePaymentProfile} from '@nu54/protocol';
import {MerchantInbox} from '../../../../p04-merchant-kiosk/src/kiosk/merchantInbox';
import {NearbyJournals} from '../../../../p04-merchant-kiosk/src/kiosk/nearbyJournals';
import {NearbyRelay, type RelayProgress} from '../../../../p04-merchant-kiosk/src/kiosk/nearbyRelay';
import {Board, epochOf} from './board';

type Identity = {subject: string; cert: string; key: string};
export const fixture = JSON.parse(readFileSync(process.env.NU54_ORDER_FLOW_FIXTURE!, 'utf8')) as {
  shop: 'usd' | 'won' | 'krwc' | 'refused'; /** Base units in one unit of the board. */ unit: string; /** Stores that run at once on this server. */ shops: number;
  endpoint: string; ca: string; expiresAt: number; tenant: string; branch: string; rpc: string; control: string; secret: string; home: string;
  amount: string; directory: string; payment: CommercePaymentProfile; customer: Identity; terminal: Identity; kitchen: Identity; finance1: Identity; finance2: Identity;
};
/** How this store's token signs and is shown. */
export const token = paymentToken(fixture.payment);
export const now = () => Math.floor(Date.now() / 1000);
export const pause = (ms: number) => new Promise<void>(resolve => setTimeout(resolve, ms));
export async function until<T>(what: string, check: () => T | Promise<T>, seconds = 40): Promise<NonNullable<T>> {
  for (const end = Date.now() + seconds * 1000; ;) {
    const value = await check();
    if (value) return value as NonNullable<T>;
    if (Date.now() > end) throw new Error('never happened: ' + what);
    await pause(150);
  }
}

// ---- the fixture ----
export async function control<T = Record<string, unknown>>(op: string, body: object = {}): Promise<T> {
  return JSON.parse(await new Promise<string>((resolve, reject) => {
    const call = http.request(fixture.control, {method: 'POST', headers: {'Content-Type': 'application/json', 'X-Order-Flow': fixture.secret}}, reply => {
      let raw = '';
      reply.setEncoding('utf8').on('data', text => { raw += text; }).on('end', () => reply.statusCode === 200 ? resolve(raw) : reject(new Error(`${op}: ${raw}`)));
    });
    call.on('error', reject);
    call.end(JSON.stringify({op, ...body}));
  })) as T;
}
export const balance = async (address: string) => BigInt((await control<{amount: string}>('balance', {address})).amount);

// ---- the server, with a client certificate ----
export const offline = {phone: false};
export function post(identity: Identity, path: string, body: string): Promise<{status: number; raw: string}> {
  return new Promise((resolve, reject) => {
    const call = https.request(fixture.endpoint + path, {method: 'POST', ca: readFileSync(fixture.ca), cert: readFileSync(identity.cert), key: readFileSync(identity.key),
      headers: {'Content-Type': 'application/json', 'Cache-Control': 'no-store', Origin: 'https://ops.test'}, timeout: 7000}, reply => {
      let raw = '';
      reply.setEncoding('utf8').on('data', text => { raw += text; }).on('end', () => resolve({status: reply.statusCode!, raw}));
    });
    call.on('timeout', () => call.destroy(new Error('timeout')));
    call.on('error', reject);
    call.end(body);
  });
}
export function read(identity: Identity, path: string): Promise<{status: number; raw: string}> {
  return new Promise((resolve, reject) => {
    https.get(fixture.endpoint + path, {ca: readFileSync(fixture.ca), cert: readFileSync(identity.cert), key: readFileSync(identity.key), timeout: 7000}, reply => {
      let raw = '';
      reply.setEncoding('utf8').on('data', text => { raw += text; }).on('end', () => resolve({status: reply.statusCode!, raw}));
    }).on('error', reject);
  });
}
/** What CommerceHttp.kt does with one call: its size limits and which answers it hands over. */
async function exchange(identity: Identity, body: string) {
  const refused = () => Object.assign(new Error('The enrolled commerce connection is unavailable'), {code: 'COMMERCE_REFUSED'});
  if (Buffer.byteLength(body) < 2 || Buffer.byteLength(body) > 8192) throw refused();
  const {status, raw} = await post(identity, '/v2/commerce', body);
  if (![200, 400, 409, 503].includes(status) || Buffer.byteLength(raw) > 32768) throw refused();
  return raw;
}

// ---- the phone's native modules ----
const phoneKey = generateKeyPairSync('ec', {namedCurve: 'prime256v1'});
const phonePublic = phoneKey.publicKey.export({type: 'spki', format: 'der'});
const phoneFingerprint = createHash('sha256').update(phonePublic).digest('hex');
export const boardSPKI = createHash('sha256').update('order-flow board').digest('hex');
export const board = new Board(process.env.NU54_ORDER_FLOW_BOARD!, token);

export const journals = new Map<string, string>();
// The names CommerceHttp.kt seals.
const journalName = /^(customer-(order|payment|qr)-v2|customer-(order|payment|qr|legacy)-[a-f0-9]{64}|subscription-alerts-v2)$/;
export const customerProfile = JSON.stringify({
  scope: {role: 'customer', tenantRef: fixture.tenant, branchRef: fixture.branch, subjectRef: fixture.customer.subject, expiresAt: fixture.expiresAt},
  pin: 'a'.repeat(64), payment: fixture.payment, nearby: {},
});
/** Commands the phone sent to the server itself, not through the kiosk. */
export const phoneCalls: string[] = [];
export const commerceHttp = {
  async loadProfile(slot: string) { return slot === 'customer' ? customerProfile : ''; },
  async loadJournal(name: string) {
    if (!journalName.test(name)) throw new Error('journal ' + name);
    return journals.get(name) ?? '';
  },
  async saveJournal(name: string, value: string) {
    if (!journalName.test(name) || Buffer.byteLength(value) > 128_000) throw new Error('journal ' + name);
    if (value) journals.set(name, value); else journals.delete(name);
  },
  async randomBytes(size: number) { return randomBytes(size).toString('base64'); },
  async exchangeScoped(slot: string, digest: string, body: string) {
    if (slot !== 'customer' || digest !== createHash('sha256').update(customerProfile).digest('hex')) throw new Error('scope');
    if (offline.phone) throw Object.assign(new Error('The enrolled commerce connection is unavailable'), {code: 'COMMERCE_UNAVAILABLE'});
    phoneCalls.push((JSON.parse(body) as {command: {operation: string}}).command.operation);
    return exchange(fixture.customer, body);
  },
};

export const notices: string[] = [];
export const alerts = {permission: async () => true, clear: async () => {}, remove: async () => {}, schedule: async () => true,
  orderReady: async (order: string) => { notices.push(order); return true; }};

const listen = <T extends (...args: never[]) => void>(set: Set<T>, run: T) => { set.add(run); return {remove: () => { set.delete(run); }}; };
const scans = new Set<(raw: string) => void>();
export const renterBle = {
  onScan: (run: (raw: string) => void) => listen(scans, run),
  async scanOwner() { for (const run of scans) run(JSON.stringify({address: 'board-1', name: 'NU54DK', rssi: -40})); },
  async stopScan() {},
  async connectOwner() { return 247; },
  async writeFragment(chunk: string) { board.data(chunk); },
  async disconnect() { board.disconnect(); },
  onFragment: (run: (chunk: string) => void) => listen(board.sounds.fragment, run),
  onDisconnect: (run: () => void) => listen(board.sounds.lost, run),
};

export const wallet = {raw: ''};
const silent = {out: '', clear: '', epoch: '', peer: ''};
export const ownerWallet = {
  async loadProfile() { return wallet.raw; },
  async saveProfile(value: string) { wallet.raw = value; },
  async setHomeProtection() {},
  async demoWallet() { return false; },
  async publicIdentity() { return phonePublic.toString('base64'); },
  async signChallenge(record: string) { return sign('sha256', Buffer.from(record, 'base64'), phoneKey.privateKey).toString('base64'); },
  async randomBytes(size: number) { return randomBytes(size).toString('base64'); },
  // In place of TLS: both ends get the same epoch and each other's key fingerprint.
  async startTls() {
    const epoch = epochOf();
    const open = await board.connect(epoch, phoneFingerprint);
    return JSON.stringify(open ? {...silent, epoch: epoch.toString('base64'), peer: boardSPKI} : {...silent, closed: true});
  },
  async feedTls(chunk: string) { return JSON.stringify({...silent, clear: chunk}); },
  async sendTls(clear: string) { return JSON.stringify({...silent, out: clear}); },
  async stopTls() {},
  onTls: () => ({remove() {}}),
  async evmRpc(_endpoint: string, packet: string) {
    const call = JSON.parse(packet) as {method: string; params: unknown[]};
    // The app pins the genesis of the public test network; this chain answers as that one.
    if (call.method === 'eth_getBlockByNumber' && call.params[0] === '0x0')
      return JSON.stringify({jsonrpc: '2.0', id: 1, result: {hash: anchor.hash, number: '0x0'}});
    return new Promise<string>((resolve, reject) => {
      const sent = http.request(fixture.rpc, {method: 'POST', agent: false, timeout: 5000, headers: {'Content-Type': 'application/json'}}, reply => {
        let raw = '';
        reply.setEncoding('utf8').on('data', text => { raw += text; }).on('end', () => resolve(raw));
      });
      sent.on('timeout', () => sent.destroy(new Error('timeout')));
      sent.on('error', reject);
      sent.end(packet);
    });
  },
};
export const anchor = {hash: ''};

export async function startBoard(authorityPoint: string) {
  if (!board.device) await board.wait('booted');
  const bare = (value: string) => value.replace(/^0x/, '');
  const {contract, asset, beneficiary, signer, merchantId} = fixture.payment;
  await board.command(['policy', bare(contract), bare(asset), bare(beneficiary), bare(signer), merchantId, phoneFingerprint].join(' '));
  await board.command('authority ' + authorityPoint);
  await board.grant();
}

// ---- the kiosk: its server connection, its inbox and its nearby relay ----
const disk = new Map<string, string>();
export const kiosk = {
  api: new CommerceClient({role: 'terminal', tenantRef: fixture.tenant, branchRef: fixture.branch, subjectRef: fixture.terminal.subject, expiresAt: fixture.expiresAt},
    body => exchange(fixture.terminal, body), async () => id()),
  relay: undefined as unknown as NearbyRelay,
  /** The order on the public screen while its QR code is shown. */
  shown: undefined as string | undefined,
  codes: [] as string[],
  progress: [] as RelayProgress[],
  locks: 0,
  journals: new NearbyJournals(),
};
export const id = () => {
  const raw = randomBytes(16);
  raw[6] = (raw[6] & 15) | 0x40;
  raw[8] = (raw[8] & 63) | 0x80;
  return raw.toString('hex').replace(/^(.{8})(.{4})(.{4})(.{4})(.{12})$/, '$1-$2-$3-$4-$5');
};
export async function startKiosk() {
  const inbox = await MerchantInbox.open({getSetting: async name => disk.get(name) ?? '', putSetting: async (name, value) => { disk.set(name, value); }},
    fixture.branch, fixture.terminal.subject);
  kiosk.relay = new NearbyRelay(kiosk.api, inbox, async () => randomBytes(16), () => kiosk.shown, pair => { kiosk.codes = pair?.codes ?? []; },
    undefined, undefined, event => kiosk.progress.push(event), token.name);
}

const notice = new Set<(raw: string) => void>();
const links = new Map<string, NearbyCommerceLink>();
export const radio = {
  /** The next answer to this request is lost and the link ends. */
  loseReplyTo: undefined as string | undefined,
  seen: [] as string[],
  /** The key of the phone that connects, as the kiosk's TLS reports it. */
  identity: 'a'.repeat(64),
};
const tell = (value: object) => { for (const run of [...notice]) run(JSON.stringify(value)); };
export function dropNearby() { for (const link of [...links.keys()]) { closeLink(link); tell({kind: 'closed', link}); } }
function closeLink(link: string) {
  const open = links.get(link);
  links.delete(link);
  open?.close();
  kiosk.journals.release(link);
  kiosk.relay.retire(link);
}
export const nearby = {
  async publicIdentity() { return ''; },
  async configure() {},
  async scan() { return JSON.stringify([{handle: 'kiosk'}]); },
  onNearby: (run: (raw: string) => void) => listen(notice, run),
  async connect() {
    const link = 'L-' + id(), epoch = randomBytes(16).toString('base64');
    const peer = {kind: 'ready' as const, link, epoch, branch: fixture.branch, expiresAt: fixture.expiresAt};
    const ready = {...peer, peer: radio.identity, principal: fixture.customer.subject};
    links.set(link, new NearbyCommerceLink(ready, 'terminal', {
      async write(_link, clear) { tell({kind: 'data', link, clear}); },
      async close() { links.delete(link); },
    }, kiosk.journals.open(ready), async (request, auth) => {
      radio.seen.push(request.op);
      const reply = await kiosk.relay.handle(request, auth);
      if (request.op === 'lock') kiosk.locks++;
      if (radio.loseReplyTo === request.op) {
        radio.loseReplyTo = undefined;
        dropNearby();
      }
      return reply;
    }));
    kiosk.relay.activate(link);
    setTimeout(() => tell({...peer, peer: 'b'.repeat(64), principal: fixture.terminal.subject}), 0);
    return link;
  },
  async write(link: string, clear: string) {
    const open = links.get(link);
    if (!open) throw new Error('NEARBY_LINK');
    setTimeout(() => open.receive(clear), 0);
  },
  async close(link: string) { if (links.has(link)) closeLink(link); },
};

export function stopWorld() {
  for (const link of [...links.keys()]) closeLink(link);
  kiosk.relay?.close();
  board.stop();
}
