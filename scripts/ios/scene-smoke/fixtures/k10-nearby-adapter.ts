// Test bundle only. P02 and P04 use the real C2 codec over an in-process stream.
import {NearbyCommerceLink, V2Journal, CommerceClient, commerceRequestId, toBase64} from '@nu54/protocol';
import {NearbyRelay} from '../../../../products/p04-merchant-kiosk/src/kiosk/nearbyRelay.ts';
import {MerchantInbox} from '../../../../products/p04-merchant-kiosk/src/kiosk/merchantInbox.ts';

type Settings = {
  control: string; token: string; branch: string; tenant: string; terminal: string; customer: string; expiresAt: number;
};
let config: Settings | undefined;
let terminal: NearbyCommerceLink | undefined;
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
export default {
  async configure(raw: string) {
    const input = JSON.parse(raw) as Settings;
    if (!/^http:\/\/127\.0\.0\.1:[0-9]+$/.test(input.control)) throw Error('LAB_LOOPBACK_REQUIRED');
    config = input;
  },
  async scan() { return JSON.stringify([{handle: 'k10-lab'}]); },
  onNearby(receive: (value: string) => void) {
    listeners.add(receive);
    return {remove() { listeners.delete(receive); }};
  },
  async connect() {
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
      const response = await fetch(fixture.control + '/relay', {
        method: 'POST', headers: {Authorization: 'Bearer ' + fixture.token}, body: command,
      });
      if (!response.ok) throw Error('LAB_RELAY_DISCONNECTED');
      return response.text();
    }, async () => commerceRequestId(entropy()));
    const inbox = await MerchantInbox.open({getSetting: async () => '', putSetting: async () => {}}, fixture.branch, fixture.terminal);
    const relay = new NearbyRelay(api, inbox, async () => entropy(), () => undefined, () => {});
    terminal = new NearbyCommerceLink({...fields, principal: fixture.customer, peer: 'a'.repeat(64)}, 'terminal', {
      async write(_, clear) { dispatch({kind: 'data', link, clear}); },
      async close() {},
    }, journal, (request, peer) => relay.handle(request, peer));
    dispatch({...fields, principal: fixture.terminal, peer: 'b'.repeat(64)});
    return link;
  },
  async write(_: string, clear: string) { terminal?.receive(clear); },
  async close() { terminal?.close(); terminal = undefined; },
};
