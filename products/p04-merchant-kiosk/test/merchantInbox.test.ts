import { MerchantInbox } from '../src/kiosk/merchantInbox';
import type { CommerceCart } from '@nu54/protocol';

const sale = 'f0551c76-7592-4850-a414-54464d24dafe';
const branch = '0fbc54ef-2827-4ab5-bfcd-3a27611c39ea';
const actor = 'f0551c76-7592-4850-b414-54464d24dafe';
const asset = '0x' + '1'.repeat(40);
const cart: CommerceCart = {
  origin: 'menu',
  originRef: sale,
  menuVersion: 4,
  items: [{ sku: 'water', option_key: 'still', quantity: 1 }],
};

test('record a durable intent before sending and resume by same idempotency key after a lost reply', async () => {
  const disk = new Map<string, string>();
  const vault = {
    getSetting: async (name: string) => disk.get(name) ?? '',
    putSetting: async (name: string, value: string) => {
      disk.set(name, value);
    },
  };
  const first = await MerchantInbox.open(vault, branch, actor);
  const publicA = first.openPublic(sale);
  await first.stage(publicA, cart, asset, sale);
  expect(disk.get('commerce-inbox-v2')).toContain(sale);
  const send = jest.fn(async () => {
    throw new Error('OFFLINE');
  });
  await expect(first.dispatch(publicA, send)).rejects.toThrow('OFFLINE');

  const restarted = await MerchantInbox.open(vault, branch, actor);
  expect(restarted.publicOrder()).toBeUndefined();
  const request = restarted.recovery()[0];
  expect(request.requestKey).toBe(sale);
  await restarted.reconcile(request.requestKey, async (c, a, k) => {
    expect(c).toEqual(cart);
    expect(a).toBe(asset);
    expect(k).toBe(sale);
    return sale;
  });
  expect(restarted.recovery()[0].saleRef).toBe(sale);
  expect(restarted.publicOrder()).toBeUndefined();
});

test('a late response for customer A never updates newly created public state for customer B', async () => {
  const vault = { getSetting: async () => '', putSetting: async () => {} };
  const box = await MerchantInbox.open(vault, branch, actor);
  const publicA = box.openPublic(sale);
  await box.stage(publicA, cart, asset, sale);
  let release: (value: string) => void = () => {};
  const flight = box.dispatch(
    publicA,
    () =>
      new Promise(resolve => {
        release = resolve;
      }),
  );
  box.endPublic();
  const publicB = box.openPublic(branch);
  expect(publicB).not.toEqual(publicA);
  release(sale);
  await flight;
  expect(box.publicOrder()).toBeUndefined();
  expect(box.recovery()[0].saleRef).toBe(sale);
});

test('storage from one terminal identity cannot be read by a newly enrolled identity', async () => {
  const disk = new Map<string, string>();
  const vault = {
    getSetting: async (name: string) => disk.get(name) ?? '',
    putSetting: async (name: string, value: string) => {
      disk.set(name, value);
    },
  };
  const box = await MerchantInbox.open(vault, branch, actor);
  await box.stage(box.openPublic(sale), cart, asset, sale);
  await expect(MerchantInbox.open(vault, branch, sale)).rejects.toThrow(
    'INBOX_IDENTITY_CHANGED',
  );
});

test('a rapid double tap cannot allocate two durable intents in one public slot', async () => {
  let release: () => void = () => {};
  const vault = {
    getSetting: async () => '',
    putSetting: () =>
      new Promise<void>(resolve => {
        release = resolve;
      }),
  };
  const box = await MerchantInbox.open(vault, branch, actor);
  const slot = box.openPublic(sale);
  const wait = box.stage(slot, cart, asset, sale);
  await expect(box.stage(slot, cart, asset, branch)).rejects.toThrow(
    'PUBLIC_SESSION_CLOSED',
  );
  release();
  await wait;
  expect(box.recovery().map(row => row.requestKey)).toEqual([sale]);
});

test('an authenticated customer cannot reuse a different customer’s saved request key', async () => {
  const vault = { getSetting: async () => '', putSetting: async () => {} };
  const box = await MerchantInbox.open(vault, branch, actor);
  await box.remember(cart, asset, sale, branch);
  await expect(box.remember(cart, asset, sale, sale)).rejects.toThrow(
    'REPLAY_CONFLICT',
  );
  expect(() => box.reconcile(sale, async () => sale, sale)).toThrow(
    'ORDER_INTENT_MISSING',
  );
  expect(box.recovery(sale)).toEqual([]);
  expect(box.publicOrder()).toBeUndefined();
});

test('inbox never evicts uncertain orders to make room, and only frees reconciled terminal outcomes', async () => {
  const vault = { getSetting: async () => '', putSetting: async () => {} };
  const box = await MerchantInbox.open(vault, branch, actor);
  for (let i = 0; i < 64; i++)
    await box.remember(
      cart,
      asset,
      sale.slice(0, -2) + i.toString(16).padStart(2, '0'),
    );
  await expect(box.remember(cart, asset, branch)).rejects.toThrow('INBOX_FULL');
  expect(box.recovery().length).toBe(64);
  const key = box.recovery()[0].requestKey;
  await box.reconcile(key, async () => sale);
  const proof = {
    id: sale,
    revision: 2,
    asset,
    amount: '1',
    expiresAt: '2026-11-20T00:00:00Z',
  };
  await expect(
    box.acknowledge(key, sale, async () => ({ ...proof, state: 'checking' })),
  ).rejects.toThrow('RECONCILE_REQUIRED');
  await box.acknowledge(key, sale, async () => ({ ...proof, state: 'paid' }));
  await box.remember(cart, asset, branch);
  expect(box.recovery().length).toBe(64);
});

test('a definitive stock rejection releases only the rejected intent for a new review', async () => {
  const box = await MerchantInbox.open(
    { getSetting: async () => '', putSetting: async () => {} },
    branch,
    actor,
  );
  const slot = box.openPublic(sale);
  await box.stage(slot, cart, asset, sale);
  await expect(
    box.dispatch(slot, async () => {
      throw new Error('OUT_OF_STOCK');
    }),
  ).rejects.toThrow('OUT_OF_STOCK');
  expect(box.recovery()).toEqual([]);
  await box.stage(
    slot,
    { ...cart, menuVersion: cart.menuVersion + 1 },
    asset,
    branch,
  );
  expect(box.recovery().map(row => row.requestKey)).toEqual([branch]);
});

test('private unresolved intents cannot occupy the public recovery budget', async () => {
  const box = await MerchantInbox.open({getSetting: async () => '', putSetting: async () => {}}, branch, actor);
  const ref = (base: string, n: number) => base.slice(0, -2) + n.toString(16).padStart(2, '0');
  for (let n = 0; n < 64; n++) {
    await box.remember({...cart, originRef: ref(sale, n)}, asset, ref(branch, n), sale);
  }
  await expect(box.remember({...cart, originRef: ref(sale, 64)}, asset, ref(branch, 64), sale)).rejects.toThrow('INBOX_FULL');
  const handle = box.openPublic(branch);
  await box.stage(handle, {...cart, originRef: branch}, asset, branch);
  await box.dispatch(handle, async () => sale);
  expect(box.publicOrder()).toBe(sale);
  expect(box.recovery(sale)).toHaveLength(64);
});

test('a crash between saving the legacy denial and clearing its raw intent cannot reopen the cart', async () => {
  const disk = new Map<string, string>();
  let interrupt = false;
  const vault = {
    getSetting: async (name: string) => disk.get(name) ?? '',
    putSetting: async (name: string, value: string) => {
      if (interrupt && name === 'commerce-nearby-inbox-v2') throw new Error('INTERRUPTED');
      expect(value.length).toBeLessThanOrEqual(128000);
      disk.set(name, value);
    },
  };
  const old = await MerchantInbox.open(vault, branch, actor);
  await old.remember(cart, asset, sale, sale);
  interrupt = true;
  await expect(old.reconcile(sale, async () => {throw new Error('OUT_OF_STOCK');}, sale)).rejects.toThrow('INTERRUPTED');
  expect(disk.get('commerce-nearby-inbox-v2')).toContain('water');
  expect(disk.get('commerce-denials-v2')).not.toContain('water');
  const restored = await MerchantInbox.open(vault, branch, actor);
  interrupt = false;
  const doNotReserve = jest.fn(async () => sale);
  await expect(restored.reconcile(sale, doNotReserve, sale)).rejects.toThrow('OUT_OF_STOCK');
  expect(doNotReserve).not.toHaveBeenCalled();
  await restored.remember(cart, asset, sale, sale);
  expect(disk.get('commerce-nearby-inbox-v2')).toBe('');
});

test('reclaims only checked paid or closed public intents before successive arrivals', async () => {
  const box = await MerchantInbox.open({getSetting: async () => '', putSetting: async () => {}}, branch, actor);
  const ref = (base: string, n: number) => base.slice(0, -2) + n.toString(16).padStart(2, '0');
  const read = async (id: string) => ({
    id, revision: 2, asset, amount: '1', expiresAt: '2026-11-20T00:00:00Z',
    state: id === ref(sale, 1) ? 'checking' as const : 'closed' as const,
  });
  for (let n = 1; n <= 100; n++) {
    await box.reclaimPublicOrders(read);
    const handle = box.openPublic(ref(branch, n));
    await box.stage(handle, {...cart, originRef: handle}, asset, ref(actor, n));
    if (n !== 2) await box.dispatch(handle, async () => ref(sale, n));
    box.endPublic();
  }
  expect(box.recovery().find(row => row.requestKey === ref(actor, 1))?.saleRef).toBe(ref(sale, 1));
  expect(box.recovery().find(row => row.requestKey === ref(actor, 2))?.saleRef).toBeUndefined();
  expect(box.recovery().some(row => row.requestKey === ref(actor, 3))).toBe(false);
  expect(box.recovery().some(row => row.requestKey === ref(actor, 100))).toBe(true);
});
