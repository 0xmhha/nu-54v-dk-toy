import { sha256 } from '@noble/hashes/sha256';
import {
  bytesToHex,
  utf8Encode,
  type CommerceProfile,
  type PickupTarget,
} from '@nu54/protocol';
import Http from '../specs/NativeCommerceHttp.ts';

const legacyJournal = 'travel-pickup-kitchen-v1';
const ref =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const targets: PickupTarget[] = [
  'preparing',
  'ready',
  'collected',
  'unfulfillable',
];

export type PickupChange = {
  branch: string;
  actor: string;
  sale: string;
  requestKey: string;
  expectedRevision: number;
  target: PickupTarget;
};

export function pickupHash(value: string) {
  return bytesToHex(sha256(utf8Encode(value)), false);
}

export function pickupJournalName(
  profile: CommerceProfile,
  endpoint: string,
  pin: string,
  publicIdentity: string,
) {
  return (
    'pickup-v2-' +
    pickupHash(
      JSON.stringify([
        profile.tenantRef,
        profile.branchRef,
        profile.subjectRef,
        profile.role,
        endpoint,
        pin,
        publicIdentity,
      ]),
    )
  );
}

function valid(item: Partial<PickupChange>): item is PickupChange {
  return (
    typeof item.sale === 'string' &&
    ref.test(item.sale) &&
    typeof item.requestKey === 'string' &&
    ref.test(item.requestKey) &&
    Number.isSafeInteger(item.expectedRevision) &&
    item.expectedRevision! >= 0 &&
    targets.includes(item.target as PickupTarget)
  );
}

let migration: Promise<unknown> = Promise.resolve();

export async function loadPickupIntent(
  name: string,
  branch: string,
  actor: string,
) {
  if (!Http) throw new Error('Pickup storage unavailable');
  const storage = Http;
  function stored(raw: string): PickupChange | undefined {
    const item = JSON.parse(raw) as PickupChange | null;
    if (item === null) return undefined;
    if (item.branch !== branch || item.actor !== actor || !valid(item))
      throw new Error('Pickup intent unavailable');
    return item;
  }

  const current = await storage.loadJournal(name);
  if (current) return stored(current);
  const next = migration.then(async () => {
    const existing = await storage.loadJournal(name);
    if (existing) return stored(existing);
    const legacy = await storage.loadJournal(legacyJournal);
    if (!legacy) return undefined;
    let item: Partial<PickupChange> | null;
    try {
      item = JSON.parse(legacy);
    } catch {
      return undefined;
    }
    if (!item || item.branch !== branch || item.actor !== actor || !valid(item))
      return undefined;
    // The v1 format recorded actor and branch only. Bind its first authenticated
    // adoption to one registration while preserving the original evidence.
    const claim = 'pickup-legacy-' + pickupHash(legacy);
    const owner = await storage.loadJournal(claim);
    if (owner && owner !== name) return undefined;
    if (!owner) await storage.saveJournal(claim, name);
    await storage.saveJournal(name, legacy);
    return item;
  });
  migration = next.catch(() => {});
  return next;
}
