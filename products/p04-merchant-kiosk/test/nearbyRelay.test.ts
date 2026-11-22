import {
  CommerceClient,
  bindServiceRequestV2,
  decodeNearbyRequest,
  utf8Encode,
  type NearbyReady,
  type ServiceRequestV2,
} from '@nu54/protocol';
import { MerchantInbox } from '../src/kiosk/merchantInbox';
import { NearbyRelay } from '../src/kiosk/nearbyRelay';

const ref = (n: number) =>
  `f0551c76-7592-4850-a414-54464d24d${n.toString(16).padStart(3, '0')}`;
const asset = '0x' + '1'.repeat(40);
const branch = ref(1),
  terminal = ref(2),
  buyerA = ref(3),
  buyerB = ref(4),
  existing = ref(5),
  saleA = ref(6),
  saleB = ref(7);
const now = () => Math.floor(Date.now() / 1000);
const profile = {
  tenantRef: ref(8),
  subjectRef: terminal,
  branchRef: branch,
  role: 'terminal' as const,
  expiresAt: now() + 100,
};
const peer = (principal: string, link: string): NearbyReady => ({
  kind: 'ready',
  link,
  principal,
  branch,
  expiresAt: now() + 60,
  epoch: '',
  peer: '',
});

async function setup(
  network = true,
  rejection?: string,
  pos?: { active: boolean; selected: boolean },
) {
  const disk = new Map<string, string>();
  const app = await MerchantInbox.open(
    {
      getSetting: async name => disk.get(name) ?? '',
      putSetting: async (name, value) => {
        disk.set(name, value);
      },
    },
    branch,
    terminal,
  );
  const transport = jest.fn(async (body: string) => {
    if (!network) throw new Error('NETWORK');
    const { command } = JSON.parse(body) as { command: ServiceRequestV2 };
    const scope = {
      ...profile,
      grants: [
        {
          domainRef: branch,
          objectRef: command.objectRef,
          operations: [`C3/${command.operation}`],
        },
      ],
    };
    const { requestId, requestDigest } = bindServiceRequestV2(
      command,
      scope,
      now(),
    );
    if (command.operation === 'order.create' && rejection)
      return JSON.stringify({
        requestId,
        requestDigest,
        status: 'error',
        error: rejection,
      });
    if (command.operation === 'checkout.pos-nearby') {
      const base = {requestId, requestDigest, resultRef: command.objectRef};
      if (pos?.selected && command.body.customerRef === buyerA)
        return JSON.stringify({...base, status: 'ok', claim: {claimDigest: 'a'.repeat(64), expiresAt: now() + 25}});
      if (pos?.selected || command.body.mode === 'resume')
        return JSON.stringify({...base, status: 'error', error: 'NOT_AUTHORIZED'});
      return JSON.stringify({...base, status: 'ok', comparison: {code: 'afe2a7f1', expiresAt: now() + 25}});
    }
    if (command.operation === 'quote.issue' && pos)
      return JSON.stringify({requestId, requestDigest, status: 'error', error: 'BUSY'});
    const resultRef =
      command.operation === 'order.create'
        ? command.objectRef === saleA
          ? saleA
          : saleB
        : command.objectRef;
    const order = {
      id: resultRef,
      revision: 0,
      state: 'preview',
      asset,
      amount: '7',
      expiresAt: new Date((now() + 90) * 1000).toISOString(),
      futureTerminalGrant: 'private-test-marker',
      menuCheckout: {
        branchLabel: 'Fixture branch',
        payerBound: true,
        customerRef: buyerA,
      },
    };
    return JSON.stringify({
      requestId,
      requestDigest,
      status: 'ok',
      resultRef,
      order,
      ...(command.operation === 'catalog.read' ? {
        catalogVersion: 2,
        catalog: command.body.page === '0' ? [{
          version: 2, expiresAt: new Date((now() + 60) * 1000).toISOString(),
          sku: 'drink', optionKey: 'plain', label: 'Coffee', asset, unitPrice: '7', available: 1,
          category: {key: 'coffee', sortOrder: 1, labels: {ko: '커피', en: 'Coffee'}},
        }] : [],
      } : {}),
      shop: {branch, branchLabel: 'Fixture branch'},
    });
  });
  const api = new CommerceClient(profile, transport, async () => ref(10));
  return {
    app,
    reopen: () => MerchantInbox.open({
      getSetting: async name => disk.get(name) ?? '',
      putSetting: async (name, value) => { disk.set(name, value); },
    }, branch, terminal),
    api,
    transport,
    relay: new NearbyRelay(
      api,
      app,
      async () => new Uint8Array(16).fill(23),
      () => (!pos || pos.active ? existing : undefined),
      () => {},
      pos ? () => (pos.active ? existing : undefined) : undefined,
    ),
  };
}
const cart = (originRef: string) => ({
  origin: 'menu' as const,
  originRef,
  menuVersion: 1,
  items: [{ sku: 'drink', option_key: 'plain', quantity: 1 }],
});

test('K03 discovery returns only registered branch and relay identity to an authenticated customer', async () => {
  const {app, relay, transport} = await setup();
  const pub = app.openPublic(ref(11));
  await app.stage(pub, cart(saleA), asset, saleA);
  await app.dispatch(pub, async () => existing);
  const request = {op: 'discover' as const, branch};
  expect(decodeNearbyRequest(utf8Encode(JSON.stringify(request)), branch)).toEqual(request);
  expect(() => decodeNearbyRequest(utf8Encode(JSON.stringify({...request, customerRef: buyerB})), branch)).toThrow('NEARBY_COMMAND');
  expect(await relay.handle(request, peer(buyerB, 'B'))).toEqual({
    status: 'ok', value: {branch, branchLabel: 'Fixture branch', terminal},
  });
  expect(JSON.parse(transport.mock.calls[0][0]).command.body).toEqual({page: '0'});
  expect(app.publicOrder()).toBe(existing);
  expect(await relay.handle({...request, branch: ref(33)}, peer(buyerB, 'B'))).toEqual({status: 'denied'});
  expect(transport).toHaveBeenCalledTimes(1);
  expect(await (await setup(false)).relay.handle(request, peer(buyerB, 'B'))).toEqual({status: 'pending'});
});

test('nearby catalog relays the scoped category and empty-page version', async () => {
  const {relay, transport} = await setup();
  expect(await relay.handle({op: 'catalog', branch, offset: 0}, peer(buyerA, 'A'))).toMatchObject({
    status: 'ok', value: {snapshotVersion: 2, rows: [{category: {key: 'coffee', labels: {ko: '커피', en: 'Coffee'}}}]},
  });
  expect(await relay.handle({op: 'catalog', branch, offset: 50}, peer(buyerA, 'A'))).toMatchObject({
    status: 'ok', value: {snapshotVersion: 2, rows: []},
  });
  expect(await relay.handle({op: 'catalog', branch: ref(33), offset: 0}, peer(buyerA, 'A'))).toEqual({status: 'denied'});
  expect(transport).toHaveBeenCalledTimes(2);
});

test('customer B can reserve on a private link while A occupies the public display', async () => {
  const { app, relay } = await setup();
  const pub = app.openPublic(ref(11));
  await app.stage(pub, cart(saleA), asset, saleA);
  await app.dispatch(pub, async () => existing);
  const a = peer(buyerA, 'A'),
    b = peer(buyerB, 'B');
  const bResult = await relay.handle(
    { op: 'reserve', branch, key: saleB, asset, cart: cart(saleB) },
    b,
  );
  expect(bResult).toMatchObject({ status: 'ok', value: { id: saleB } });
  expect(bResult.value).not.toHaveProperty('menuCheckout');
  expect(bResult.value).not.toHaveProperty('futureTerminalGrant');
  expect(app.publicOrder()).toBe(existing);
  expect(await relay.handle(
    {op: 'reserve', branch, key: ref(29), asset, cart: cart(saleB)}, a,
  )).toEqual({status: 'denied'});
  expect(await relay.handle({ op: 'status', branch, key: saleB }, a)).toEqual({
    status: 'denied',
  });
  expect(
    await relay.handle({ op: 'status', branch, key: saleB, order: saleB }, a),
  ).toEqual({ status: 'denied' });
  expect(
    await relay.handle({ op: 'status', branch, key: saleB }, b),
  ).toMatchObject({ status: 'ok', value: { id: saleB } });
  const pairing = await relay.handle(
    { op: 'checkout', branch, key: ref(12), order: existing },
    a,
  );
  expect(pairing).toMatchObject({
    status: 'pending',
    value: { code: expect.stringMatching(/^[a-f0-9]{8}$/) },
  });
  expect(
    await relay.handle(
      { op: 'checkout', branch, key: ref(12), order: existing },
      b,
    ),
  ).toEqual({ status: 'denied' });
  app.endPublic();
  relay.clearPublic();
  expect(app.publicOrder()).toBeUndefined();
});

test('when the terminal is offline the same customer intent remains pending and no public data is made', async () => {
  const { app, relay, transport } = await setup(false);
  const auth = peer(buyerA, 'A');
  const request = {
    op: 'reserve' as const,
    branch,
    key: saleA,
    asset,
    cart: cart(saleA),
  };
  await expect(relay.handle(request, auth)).resolves.toEqual({
    status: 'pending',
  });
  await expect(relay.handle(request, auth)).resolves.toEqual({
    status: 'pending',
  });
  expect(app.recovery(buyerA)).toHaveLength(1);
  expect(app.publicOrder()).toBeUndefined();
  const sent = transport.mock.calls.map(
    ([raw]) => JSON.parse(raw).command.idempotencyKey,
  );
  expect(sent).toEqual([saleA, saleA]);
});

test('twenty concurrent retries for the same authenticated nearby cart reserve one central sale', async () => {
  const { app, relay, transport } = await setup();
  const request = {
    op: 'reserve' as const,
    branch,
    key: saleA,
    asset,
    cart: cart(saleA),
  };
  const authenticated = peer(buyerA, 'session-a');
  const replies = await Promise.all(
    Array.from({ length: 20 }, () => relay.handle(request, authenticated)),
  );
  expect(replies.every(reply => reply.status === 'ok')).toBe(true);
  const reserves = transport.mock.calls.filter(
    ([raw]) => JSON.parse(raw).command.operation === 'order.create',
  );
  expect(reserves).toHaveLength(1);
  expect(app.recovery(buyerA)).toHaveLength(1);
  expect(app.recovery(buyerB)).toHaveLength(0);
});

test.each([
  ['OUT_OF_STOCK', 'sold-out'],
  ['CATALOG_CHANGED', 'catalog-changed'],
  ['EXPIRED', 'pending'],
])(
  'maps %s without changing unrelated public and private orders',
  async (error, status) => {
    const { app, relay } = await setup(true, error);
    const pub = app.openPublic(ref(11));
    await app.stage(pub, cart(saleA), asset, saleA);
    await app.dispatch(pub, async () => existing);
    await expect(
      relay.handle(
        { op: 'reserve', branch, key: saleB, asset, cart: cart(saleB) },
        peer(buyerB, 'B'),
      ),
    ).resolves.toEqual({ status });
    expect(app.hasIntent(saleB, buyerB)).toBe(true);
    expect(app.publicOrder()).toBe(existing);
  },
);

test('K10 a lost sold-out reply stays final across relay restart and the original origin cannot be replayed with another key', async () => {
  const {app, relay, transport, reopen, api} = await setup(true, 'OUT_OF_STOCK');
  const publicSession = app.openPublic(ref(11));
  await app.stage(publicSession, cart(saleA), asset, saleA);
  await app.dispatch(publicSession, async () => existing);
  const request = {op: 'reserve' as const, branch, key: saleB, asset, cart: cart(saleB)};
  expect(await relay.handle(request, peer(buyerB, 'B'))).toEqual({status: 'sold-out'});
  const restarted = new NearbyRelay(api, await reopen(), async () => new Uint8Array(16), () => undefined, () => {});
  expect(await restarted.handle({op: 'status', branch, key: saleB}, peer(buyerB, 'B2'))).toEqual({status: 'sold-out'});
  expect(await restarted.handle(request, peer(buyerB, 'B2'))).toEqual({status: 'sold-out'});
  expect(await restarted.handle({...request, key: ref(29)}, peer(buyerA, 'A'))).toEqual({status: 'denied'});
  expect(await restarted.handle({op: 'status', branch, key: saleB}, peer(buyerA, 'A'))).toEqual({status: 'denied'});
  expect(app.publicOrder()).toBe(existing);
  expect(transport).toHaveBeenCalledTimes(1);
});

test('K10 one hundred private reservations use server receipts without filling the public inbox', async () => {
  const {app, api, relay, reopen, transport} = await setup();
  const active = app.openPublic(existing);
  await app.stage(active, cart(existing), asset, saleA);
  await app.dispatch(active, async () => existing);
  const owners = new Map<string, string>();
  const unavailable = [{sku: 'drink', option_key: 'plain', quantity: 1}];
  transport.mockImplementation(async raw => {
    const {command, cart} = JSON.parse(raw) as {command: ServiceRequestV2; cart?: {nearby: {customerRef: string; expiresAt: number}}};
    const {requestId, requestDigest} = bindServiceRequestV2(command, {...profile, grants: [{
      domainRef: branch, objectRef: command.objectRef, operations: [`C3/${command.operation}`],
    }]}, now());
    const envelope = {requestId, requestDigest};
    if (command.operation === 'order.create') {
      owners.set(command.idempotencyKey!, cart!.nearby.customerRef);
      expect(cart!.nearby.expiresAt).toBeGreaterThan(now());
    } else if (command.operation === 'order.nearby-result' && owners.get(command.objectRef) !== command.body.customerRef) {
      return JSON.stringify({...envelope, status: 'ok', resultRef: command.objectRef});
    }
    return JSON.stringify({...envelope, status: 'error', error: 'OUT_OF_STOCK', reservationFound: true, unavailable});
  });
  const deadline = now() + 295;
  for (let n = 0; n < 100; n++) {
    const buyer = peer(ref(500 + n), `arrival-${n}`);
    const request = {op: 'reserve' as const, branch, key: ref(1000 + n), asset, cart: cart(ref(1200 + n)), deadline};
    expect(await relay.handle(request, buyer)).toEqual({status: 'sold-out', value: unavailable});
    expect(app.recovery(buyer.principal)).toHaveLength(0);
  }
  expect(owners.size).toBe(100);
  expect(app.publicOrder()).toBe(existing);
  const resumed = new NearbyRelay(api, await reopen(), async () => new Uint8Array(16), () => undefined, () => {});
  const lookup = {op: 'status' as const, branch, key: ref(1000), authoritative: true as const};
  expect(await resumed.handle(lookup, peer(ref(500), 'reconnected'))).toEqual({status: 'sold-out', value: unavailable});
  expect(await resumed.handle(lookup, peer(ref(501), 'other'))).toEqual({status: 'denied'});
  expect(owners.size).toBe(100);
});

test('K10 legacy definitive denials stay recoverable past 64 without blocking the next public session', async () => {
  const {app, relay, api, reopen} = await setup(true, 'OUT_OF_STOCK');
  for (let n = 0; n < 100; n++) {
    const buyer = peer(ref(500 + n), `legacy-${n}`);
    const request = {op: 'reserve' as const, branch, key: ref(1000 + n), asset, cart: cart(ref(1200 + n))};
    expect(await relay.handle(request, buyer)).toEqual({status: 'sold-out'});
  }
  const restarted = await reopen();
  const session = restarted.openPublic(existing);
  await restarted.stage(session, cart(existing), asset, saleA);
  await restarted.dispatch(session, async () => existing);
  const resumed = new NearbyRelay(api, restarted, async () => new Uint8Array(16), () => existing, () => {});
  expect(await resumed.handle({op: 'status', branch, key: ref(1000)}, peer(ref(500), 'reconnected'))).toEqual({status: 'sold-out'});
  expect(app.hasIntent(ref(1000), ref(500))).toBe(true);
  expect(restarted.publicOrder()).toBe(existing);
});

test('K06 shelving new reservations preserves an existing private nearby order across customers', async () => {
  const { app, relay } = await setup(true, 'BUSY');
  await app.remember(cart(saleA), asset, saleA, buyerA);
  await app.reconcile(saleA, async () => saleA, buyerA);
  const older = { op: 'status' as const, branch, key: saleA };
  const newer = {
    op: 'reserve' as const,
    branch,
    key: saleB,
    asset,
    cart: cart(saleB),
  };
  const a = peer(buyerA, 'A');
  const b = peer(buyerB, 'B');
  await expect(relay.handle(older, a)).resolves.toMatchObject({
    status: 'ok',
    value: { id: saleA },
  });
  await expect(relay.handle(newer, b)).resolves.toEqual({ status: 'pending' });
  await expect(relay.handle(older, b)).resolves.toEqual({ status: 'denied' });
  await expect(relay.handle(older, a)).resolves.toMatchObject({
    status: 'ok',
    value: { id: saleA },
  });
});

test('K11 nearby uses authenticated peer identity, accepts no unpublished quote and resumes only the selected original', async () => {
  const pos = {active: true, selected: false};
  const {app, relay, transport} = await setup(true, undefined, pos);
  const request = {op: 'checkout' as const, branch, key: ref(30), order: existing};
  expect(await relay.handle(request, peer(buyerA, 'A'))).toEqual({status: 'pending', value: {code: 'afe2a7f1'}});
  expect(JSON.parse(transport.mock.calls[0][0]).command.body).toEqual({customerRef: buyerA, mode: 'register'});
  pos.active = false;
  pos.selected = true;
  expect(await relay.handle(request, peer(buyerB, 'B'))).toEqual({status: 'denied'});
  expect(await relay.handle(request, peer(buyerA, 'A2'))).toEqual({status: 'pending'});
  const commands = transport.mock.calls.map(([raw]) => JSON.parse(raw).command);
  expect(commands[2].body).toEqual({customerRef: buyerA, mode: 'resume'});
  expect(commands.map(command => command.operation)).toEqual([
    'checkout.pos-nearby', 'checkout.pos-nearby', 'checkout.pos-nearby', 'order.status', 'quote.issue',
  ]);
  expect(app.ownsSale(existing, buyerA)).toBe(false);
  expect(app.ownsSale(existing, buyerB)).toBe(false);
});
