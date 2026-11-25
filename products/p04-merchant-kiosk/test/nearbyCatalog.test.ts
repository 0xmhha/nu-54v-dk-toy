import {
  appendCommerceCatalogPage, assertCommerceCatalogPage, assertCommerceChoices, commerceAllergens, utf8Encode, v2PayloadLimit,
  type CommerceClient, type CommercePrice, type NearbyReady, type NearbyReply,
} from '@nu54/protocol';
import { NearbyRelay } from '../src/kiosk/nearbyRelay.ts';

const ref = (number: number) => `0fbc54ef-2827-4ab5-bfcd-${String(number).padStart(12, '0')}`;
const branch = ref(1);
const now = () => Math.floor(Date.now() / 1000);

function rows(count: number): CommercePrice[] {
  return Array.from({ length: count }, (_, index) => {
    const size = index < count / 2 ? 'large' : 'small';
    const shot = `v${String(index % (count / 2)).padStart(2, '0')}`;
    return {
      version: 2, expiresAt: new Date((now() + 60) * 1000).toISOString(), sku: 'latte', optionKey: `${size}-${shot}`, label: '바닐라 라떼',
      asset: '0x' + '4'.repeat(40), unitPrice: '450000', available: 3, skuAvailable: 3,
      metadata: {
        optionRequired: true, labels: { ko: '바닐라 라떼', en: 'Vanilla latte' }, optionLabels: { ko: '큰 컵 · 샷 추가', en: 'Large · Extra shot' },
        optionGroupLabels: { ko: '크기 · 샷', en: 'Size · Shots' }, allergens: [...commerceAllergens.slice(0, 12)],
        crossContact: 'possible', crossContactAllergens: [...commerceAllergens.slice(0, 12)], crossContactSource: 'shared-equipment',
      },
      category: { key: 'coffee', sortOrder: 1, labels: { ko: '커피', en: 'Coffee' } },
      choices: [
        { group: { key: 'size', labels: { ko: '크기', en: 'Size' } }, value: { key: size, labels: { ko: size, en: size }, order: size === 'large' ? 1 : 0 } },
        { group: { key: 'shot', labels: { ko: '샷', en: 'Shots' } }, value: { key: shot, labels: { ko: shot, en: shot }, order: index % (count / 2) } },
      ],
    };
  });
}

function relayOf(all: CommercePrice[]) {
  const pages: number[] = [];
  const api = {
    profile: { branchRef: branch, subjectRef: ref(2) },
    catalogWindow: async (page: number) => { pages.push(page); return { prices: all.slice(page * 50, page * 50 + 50), snapshotVersion: 2 }; },
  } as unknown as CommerceClient;
  const relay = new NearbyRelay(api, {} as never, async () => new Uint8Array(16).fill(1), () => undefined, () => {});
  const peer = { kind: 'ready', link: 'l'.repeat(10), epoch: 'AAAA', peer: 'a'.repeat(64), principal: ref(3), branch, expiresAt: now() + 60 } as NearbyReady;
  return { pages, read: (offset: number) => relay.handle({ op: 'catalog', branch, offset }, peer) };
}

test('a menu with full declarations fits the nearby reply and arrives whole', async () => {
  for (const count of [20, 24]) {
    const all = rows(count);
    const { read } = relayOf(all);
    const received: CommercePrice[] = [];
    const sizes: number[] = [];
    for (let offset: number | undefined = 0; offset !== undefined;) {
      const reply: NearbyReply = await read(offset);
      expect(reply.status).toBe('ok');
      expect(utf8Encode(JSON.stringify(reply)).length).toBeLessThanOrEqual(v2PayloadLimit);
      const page = reply.value as { rows: CommercePrice[]; next?: number };
      assertCommerceCatalogPage(page.rows, now(), 4);
      appendCommerceCatalogPage(received, page.rows);
      sizes.push(page.rows.length);
      offset = page.next;
    }
    assertCommerceChoices(received);
    expect(received).toEqual(all);
    expect(Math.max(...sizes)).toBeLessThan(4); // four of these rows are above one reply
  }
});

test('short rows still travel four at a time, across pages and to an empty end', async () => {
  const all = Array.from({ length: 100 }, (_, index): CommercePrice => ({ version: 2, expiresAt: new Date((now() + 60) * 1000).toISOString(),
    sku: `s${index}`, optionKey: 'plain', label: 'Tea', asset: '0x' + '4'.repeat(40), unitPrice: '1', available: 1 }));
  const { read } = relayOf(all);
  const value = async (offset: number) => (await read(offset)).value as { rows: CommercePrice[]; next?: number };
  expect((await value(0)).rows.length).toBe(4);
  expect(await value(48)).toMatchObject({ rows: [{ sku: 's48' }, { sku: 's49' }], next: 50 });
  expect(await value(98)).toMatchObject({ next: 100 });
  expect(await value(100)).toEqual({ rows: [], next: undefined, snapshotVersion: 2 });
  expect((await value(103)).rows).toEqual([]);
});
