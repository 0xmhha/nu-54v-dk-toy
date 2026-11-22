import {sha256} from '@noble/hashes/sha256';
import {utf8Encode, type CommerceProfile} from '@nu54/protocol';

const reference = /^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/;
const amount = /^(?:0|[1-9][0-9]{0,77})$/;
const placeID = /^[a-z0-9][a-z0-9_-]{0,63}$/;
const asset = /^0x[a-f0-9]{40}$/;

export type Hold = {
  branch: string; slot: string; requestKey: string; partySize: number;
  termsVersion: number; asset: string; deposit: string; balance: string;
};
export type HoldCheckpoint = {place: string; startsAt: string; intent: Hold};
export type HoldRejection = 'sold_out' | 'terms_changed';

export function holdJournal(scope: CommerceProfile) {
  const digest = sha256(utf8Encode(JSON.stringify([scope.tenantRef, scope.subjectRef, scope.branchRef])));
  return 'travel-hold-' + Array.from(digest, byte => byte.toString(16).padStart(2, '0')).join('');
}

export function readHold(raw: string, branch: string): HoldCheckpoint | undefined {
  if (!raw) return undefined;
  if (raw.length > 1024) throw new Error('Invalid booking recovery data');
  const value = JSON.parse(raw) as HoldCheckpoint | null;
  const intent = value?.intent;
  if (!intent || typeof value?.place !== 'string' || !placeID.test(value.place) ||
      typeof value.startsAt !== 'string' || !Number.isFinite(Date.parse(value.startsAt)) || intent.branch !== branch ||
      typeof intent.slot !== 'string' || !reference.test(intent.slot) ||
      typeof intent.requestKey !== 'string' || !reference.test(intent.requestKey) ||
      !Number.isSafeInteger(intent.partySize) || intent.partySize < 1 || intent.partySize > 20 ||
      !Number.isSafeInteger(intent.termsVersion) || intent.termsVersion < 1 ||
      typeof intent.asset !== 'string' || !asset.test(intent.asset) ||
      typeof intent.deposit !== 'string' || !amount.test(intent.deposit) ||
      typeof intent.balance !== 'string' || !amount.test(intent.balance)) throw new Error('Invalid booking recovery data');
  // Rebuild the wire object from the originally reviewed fields.
  const {slot, requestKey, partySize, termsVersion, asset: unit, deposit, balance} = intent;
  return {place: value.place, startsAt: value.startsAt, intent: {branch, slot, requestKey, partySize, termsVersion, asset: unit, deposit, balance}};
}

export function matchesHold(booking: HoldBooking | undefined, intent: Hold) {
  return !!booking && typeof booking.id === 'string' && reference.test(booking.id) &&
    booking.slot === intent.slot && booking.partySize === intent.partySize &&
    booking.termsVersion === intent.termsVersion && booking.asset === intent.asset &&
    booking.deposit === intent.deposit && booking.balance === intent.balance;
}

type HoldBooking = Pick<Hold, 'slot' | 'partySize' | 'termsVersion' | 'asset' | 'deposit' | 'balance'> & {id: string};
