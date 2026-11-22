// Test bundle only. P04 NearbyRelay uses the private C2 board host for the renter scenario.
import {NearbyCommerceLink, V2Journal, CommerceClient, commerceRequestId, toBase64, bytesToHex, type CommercePaymentProfile} from '@nu54/protocol';
import {NearbyRelay} from '../../../products/p04-merchant-kiosk/src/kiosk/nearbyRelay.ts';
import {MerchantInbox} from '../../../products/p04-merchant-kiosk/src/kiosk/merchantInbox.ts';

type Settings = {
  control: string; token: string; branch: string; tenant: string; terminal: string; customer: string; expiresAt: number; payment: CommercePaymentProfile;
};
let config: Settings | undefined;
let terminal: NearbyCommerceLink | undefined;
let relay: NearbyRelay | undefined;
let counter = 0;
const listeners = new Set<(value: string) => void>();
const journal = new V2Journal(48);
function dispatch(data: object) {
  setTimeout(() => {
    const event = JSON.stringify(data);
    for (const receive of listeners) receive(event);
  }, 0);
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
  async write(_: string, clear: string) { terminal?.receive(clear); },
  async close() { console.warn('W10 renter close'); terminal?.close(); terminal = undefined; relay?.close(); relay = undefined; },
};
