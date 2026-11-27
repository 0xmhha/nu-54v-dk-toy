import { NearbyCommerceLink, V2Journal, fromBase64, hexToBytes, toBase64, utf8Encode, V2Link, encodeV2StreamFrame, type NearbyReady } from '@nu54/protocol';
import { NearbyJournals } from '../src/kiosk/nearbyJournals';

const ref = (n: number) => `f0551c76-7592-4850-a414-54464d24d${n.toString(16).padStart(3, '0')}`;
const branch = ref(1);
const request = { op: 'discover' as const, branch };
const id = (n: number) => {
  const bytes = new Uint8Array(16);
  new DataView(bytes.buffer).setUint32(0, n + 1);
  return bytes;
};
let epochs = 0;
const ready = (phone: number): NearbyReady => ({
  kind: 'ready', link: 'L' + ++epochs, principal: ref(10 + phone), branch, expiresAt: Math.floor(Date.now() / 1000) + 3_600,
  peer: phone.toString(16).padStart(64, '0'), epoch: toBase64(id(epochs)),
});

/** A customer and the terminal over one link, with the customer's frames recorded. */
function connect(journals: NearbyJournals, phone: number) {
  const peer = ready(phone);
  const frames: string[] = [];
  let served = 0;
  let closed = false;
  const terminal: NearbyCommerceLink = new NearbyCommerceLink(peer, 'terminal', {
    write: async (_link, data) => customer.receive(data),
    close: async () => { closed = true; journals.release(peer.link); customer.close(); },
  }, journals.open(peer), async () => { served++; return { status: 'ok' }; });
  const customer: NearbyCommerceLink = new NearbyCommerceLink(peer, 'customer', {
    write: async (_link, data) => { frames.push(data); terminal.receive(data); },
    close: async () => {},
  }, new V2Journal(4));
  return { peer, terminal, customer, frames, served: () => served, closed: () => closed,
    ask: (n: number) => customer.request(request, id(n)) };
}
const settle = () => new Promise<void>(resolve => setImmediate(() => resolve()));

test('many phones are served past the size one shared journal had', async () => {
  const journals = new NearbyJournals();
  let sent = 0;
  for (let visit = 0; visit < 4; visit++) {
    const phones = Array.from({ length: 5 }, (_, phone) => connect(journals, phone + 1));
    for (let n = 0; n < 12; n++)
      for (const phone of phones) {
        expect(await phone.ask(sent++)).toEqual({ status: 'ok' });
      }
    for (const phone of phones) phone.terminal.close();
  }
  expect(sent).toBe(240);
  expect(journals.size).toBe(5);
});

test('one phone that keeps polling longer than 128 reads is still served', async () => {
  const phone = connect(new NearbyJournals(), 1);
  for (let n = 0; n < 200; n++) expect((await phone.ask(n)).status).toBe('ok');
});

test('a frame replayed in the same link, or from an earlier link, closes the link without serving it again', async () => {
  const journals = new NearbyJournals();
  const phone = connect(journals, 1);
  await phone.ask(1);
  phone.terminal.receive(phone.frames[0]);
  await settle();
  expect(phone.closed()).toBe(true);
  expect(phone.served()).toBe(1);
  const again = connect(journals, 1);
  again.terminal.receive(phone.frames[0]);
  await settle();
  expect(again.closed()).toBe(true);
  expect(again.served()).toBe(0);
});

test('a request id reused with another body is refused across a reconnect of the same phone only', async () => {
  const journals = new NearbyJournals();
  const send = async (phone: number, payload: object) => {
    const link = connect(journals, phone);
    const codec = new V2Link('nearby', 'customer', fromBase64(link.peer.epoch), hexToBytes(link.peer.peer), new V2Journal(4));
    const expires = BigInt(Math.floor(Date.now() / 1000) + 20);
    for (const frame of codec.send({ command: 'nearby.submit', id: id(7), expires, payload: utf8Encode(JSON.stringify(payload)) }))
      link.terminal.receive(toBase64(encodeV2StreamFrame(frame)));
    await settle();
    return link;
  };
  expect((await send(1, request)).served()).toBe(1);
  const conflict = await send(1, { op: 'catalog', branch, offset: 0 });
  expect(conflict.served()).toBe(0);
  expect(conflict.closed()).toBe(true);
  expect((await send(2, { op: 'catalog', branch, offset: 0 })).served()).toBe(1);
});

test('a flooding phone fills only its own room, and memory stays bounded while open links keep theirs', async () => {
  const journals = new NearbyJournals();
  const held = connect(journals, 1);
  await held.ask(1);
  const flood = connect(journals, 2);
  let served = 0;
  try {
    for (let n = 0; n < 600; n++) { await flood.ask(n); served++; }
  } catch { /* its own journal is full */ }
  expect(served).toBe(512);
  expect((await held.ask(2)).status).toBe('ok');
  for (let phone = 3; phone < 120; phone++) {
    const next = connect(journals, phone);
    expect((await next.ask(phone)).status).toBe('ok');
    next.terminal.close();
    expect(journals.size).toBeLessThanOrEqual(32);
  }
  expect((await held.ask(3)).status).toBe('ok');
  // The held phone's journal was not dropped for the others: its earlier id still conflicts.
  const journal = journals.open(held.peer);
  expect(() => journal.record('nearby', hexToBytes(held.peer.peer),
    { command: 'nearby.submit', id: id(1), expires: 1n, payload: new Uint8Array() }, Uint8Array.of(1), BigInt(Math.floor(Date.now() / 1000))))
    .toThrow('request ID conflict');
});
