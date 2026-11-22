// Test bundle only. P04 NearbyRelay uses the private C2 board host for the renter scenario.
import {NearbyCommerceLink, V2Journal, CommerceClient, commerceRequestId, toBase64, fromBase64, bytesToHex, type CommercePaymentProfile} from '@nu54/protocol';
import {NearbyRelay} from '../../../products/p04-merchant-kiosk/src/kiosk/nearbyRelay.ts';
import {MerchantInbox} from '../../../products/p04-merchant-kiosk/src/kiosk/merchantInbox.ts';
import {AppState} from 'react-native';

// Diagnostics for the installed runner only. The app maps most failures to a generic message, so
// log every error constructed with a code-like message (for example NEARBY_CATALOG) and every
// app-state change. Error stays the same prototype, so instanceof checks are unchanged.
const NativeError = globalThis.Error;
function TracedError(this: unknown, message?: string, options?: ErrorOptions) {
  // Codes (NEARBY_CATALOG) and the protocol's own lower-case link failures ("invalid frame sequence").
  if (typeof message === 'string' && (/^[A-Z][A-Z0-9_]{2,}$/.test(message) ||
      /^(invalid|fragment|request ID|v2 |unexpected|link )/.test(message)))
    console.warn('W10 error', message, String(new NativeError().stack ?? '').split('\n').slice(2, 6).join(' | '));
  return new.target ? Reflect.construct(NativeError, [message, options], new.target) : NativeError(message, options);
}
TracedError.prototype = NativeError.prototype;
Object.setPrototypeOf(TracedError, NativeError);
globalThis.Error = TracedError as unknown as ErrorConstructor;
AppState.addEventListener('change', state => console.warn('W10 app state', state, Date.now()));

type Settings = {
  control: string; token: string; branch: string; tenant: string; terminal: string; customer: string; expiresAt: number; payment: CommercePaymentProfile;
};
let config: Settings | undefined;
let terminal: NearbyCommerceLink | undefined;
let relay: NearbyRelay | undefined;
let counter = 0;
const listeners = new Set<(value: string) => void>();
const journal = new V2Journal(48);
// Stream frame: 2-byte length, epoch (16), sequence (4), fragment index, fragment count.
function frameInfo(clear: string) {
  const bytes = fromBase64(clear);
  if (bytes.length < 24) return 'short ' + bytes.length;
  const sequence = ((bytes[18] << 24) | (bytes[19] << 16) | (bytes[20] << 8) | bytes[21]) >>> 0;
  return `seq ${sequence} part ${bytes[22]}/${bytes[23]} len ${bytes.length} epoch ${bytesToHex(bytes.subarray(2, 6), false)}`;
}
function dispatch(data: object) {
  const frame = data as {kind?: string; link?: string; clear?: string};
  if (frame.kind === 'data' && frame.clear) console.warn('W10 frame to customer', frame.link, frameInfo(frame.clear), 'listeners', listeners.size);
  // One FIFO drained by a single timer: separate zero-delay timers are not guaranteed to run in
  // scheduling order, and reordered fragments fail the V2 link ("invalid fragment order").
  outbox.push(JSON.stringify(data));
  if (outbox.length === 1) setTimeout(drain, 0); // later pushes join the pending drain
}
const outbox: string[] = [];
function drain() {
  while (outbox.length) {
    const event = outbox.shift()!;
    for (const receive of listeners) {
      try {
        receive(event);
      } catch (error) {
        console.warn('W10 listener failed', String(error));
      }
    }
  }
}
function entropy() {
  const data = new Uint8Array(16);
  const value = ++counter;
  data[14] = value >> 8;
  data[15] = value;
  return data;
}
async function control(settings: Settings, route: string, body: object, signal: AbortSignal) {
  const reply = await fetch(settings.control + '/rental/' + route, {
    method: 'POST', headers: {Authorization: 'Bearer ' + settings.token},
    body: JSON.stringify(body), signal,
  });
  if (!reply.ok) throw Error('W10_RENTER_FIXTURE_CONTROL');
  return reply.json();
}

export default {
  async configure(raw: string) {
    const input = JSON.parse(raw) as Settings;
    if (!/^http:\/\/127\.0\.0\.1:[0-9]+$/.test(input.control)) throw Error('LAB_LOOPBACK_REQUIRED');
    if (input.payment?.chain !== 8283) throw Error('W10_RENTER_FIXTURE_POLICY');
    config = input;
  },
  async scan() { console.warn('W10 renter scan'); return JSON.stringify([{handle: 'k10-lab'}]); },
  onNearby(receive: (value: string) => void) {
    listeners.add(receive);
    return {remove() { listeners.delete(receive); }};
  },
  async connect() {
    console.warn('W10 renter connect', config ? 'configured' : 'not configured');
    if (!config) throw Error('LAB_ENROLLMENT');
    const fixture = config;
    const bytes = entropy();
    // Include the current epoch time to keep C2 links distinct across app launches.
    new DataView(bytes.buffer).setFloat64(0, Date.now());
    const epoch = toBase64(bytes);
    const link = 'k10-lab-' + counter;
    const fields = {kind: 'ready' as const, link, epoch, branch: fixture.branch, expiresAt: fixture.expiresAt};
    const api = new CommerceClient({
      role: 'terminal', tenantRef: fixture.tenant, branchRef: fixture.branch, subjectRef: fixture.terminal,
      expiresAt: fixture.expiresAt,
    }, async command => {
      // NearbyRelay reports every failure as "pending"; log the cause for the installed runner.
      const response = await fetch(fixture.control + '/relay', {
        method: 'POST', headers: {Authorization: 'Bearer ' + fixture.token}, body: command,
      }).catch(error => {
        console.warn('W10 renter relay transport', String(error));
        throw error;
      });
      if (!response.ok) {
        console.warn('W10 renter relay status', response.status);
        throw Error('LAB_RELAY_DISCONNECTED');
      }
      return response.text();
    }, async () => commerceRequestId(entropy()));
    const shopIdentity = api.shopIdentity.bind(api);
    api.shopIdentity = async () => {
      try {
        return await shopIdentity();
      } catch (error) {
        console.warn('W10 renter shop identity', error instanceof Error ? error.name + ' ' + error.message : String(error));
        throw error;
      }
    };
    const inbox = await MerchantInbox.open({getSetting: async () => '', putSetting: async () => {}}, fixture.branch, fixture.terminal);
    relay?.close();
    terminal?.close();
    const active = new NearbyRelay(api, inbox, async () => entropy(), () => undefined, () => {}, undefined, {
      policy: fixture.payment,
      async read(payment, authority, signal, decided, begin) {
        begin();
        const review = await control(fixture, 'open', {
          authority, quote: bytesToHex(payment.bytes, false), signer: fixture.payment.signer,
          payment: {deadline: payment.deadline, payer: payment.payer, generation: String(payment.generation),
            digest: payment.digest, approvalDigest: payment.approvalDigest},
        }, signal);
        if (typeof review.receipt !== 'string') throw Error('W10_RENTER_FIXTURE_RECEIPT');
        void control(fixture, 'decision', {request: authority.request}, signal).then(final => {
          if (typeof final.decision === 'string') decided(final.decision);
        }).catch(() => {});
        return review.receipt;
      },
    });
    relay = active;
    active.activate(link);
    terminal = new NearbyCommerceLink({...fields, principal: fixture.customer, peer: 'a'.repeat(64)}, 'terminal', {
      async write(_, clear) { dispatch({kind: 'data', link, clear}); },
      async close() {},
    }, journal, async (request, peer) => {
      const reply = await active.handle(request, peer).catch(error => {
        console.warn('W10 renter nearby threw', request.op, String(error));
        throw error;
      });
      console.warn('W10 renter nearby', request.op, reply.status,
        request.op === 'catalog' ? JSON.stringify(reply.value).slice(0, 900) :
          request.op === 'rental-review' ? JSON.stringify(reply.value).slice(0, 700) + ' request ' +
            JSON.stringify(request).slice(0, 900) : '',
        'device now', Date.now());
      return reply;
    });
    dispatch({...fields, principal: fixture.terminal, peer: 'b'.repeat(64)});
    return link;
  },
  async write(handle: string, clear: string) {
    console.warn('W10 frame to terminal', handle, frameInfo(clear), terminal ? 'open' : 'none');
    terminal?.receive(clear);
  },
  async close() { console.warn('W10 renter close'); terminal?.close(); terminal = undefined; relay?.close(); relay = undefined; },
};
