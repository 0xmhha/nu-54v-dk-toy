import {sha256} from '@noble/hashes/sha256';
import {bytesToHex, utf8Encode, type CommerceProfile} from '@nu54/protocol';
import Http from '../specs/NativeCommerceHttp.ts';

export type CustomerJournal = 'order' | 'payment' | 'qr';

export const customerDigest = (value: string) => bytesToHex(sha256(utf8Encode(value)), false);

let adoption = Promise.resolve();

export function customerJournals(raw: string, profile: CommerceProfile) {
  const {endpoint, pin} = JSON.parse(raw) as {endpoint?: string; pin?: string};
  const key = customerDigest(JSON.stringify([
    endpoint, pin, profile.tenantRef, profile.branchRef, profile.subjectRef,
  ]));
  const name = (kind: CustomerJournal) => `customer-${kind}-${key}`;

  return {
    read(kind: CustomerJournal): Promise<string> {
      const result = adoption.then(async () => {
        const storage = Http!;
        const scoped = await storage.loadJournal(name(kind));
        if (scoped) return scoped === 'null' ? '' : scoped;
        const legacy = await storage.loadJournal(`customer-${kind}-v2`);
        if (!legacy) return '';
        const item = JSON.parse(legacy) as {branch?: string; customer?: string; subject?: string} | null;
        if (item?.branch !== profile.branchRef ||
          (kind === 'payment' ? item.customer : item.subject) !== profile.subjectRef) return '';
        if (await storage.loadProfile('customer') !== raw) return '';
        const claim = `customer-legacy-${customerDigest(JSON.stringify([kind, legacy]))}`;
        const owner = await storage.loadJournal(claim);
        if (owner && owner !== key) return '';
        if (!owner) await storage.saveJournal(claim, key);
        await storage.saveJournal(name(kind), legacy);
        return legacy;
      });
      adoption = result.then(() => {}, () => {});
      return result;
    },
    write(kind: CustomerJournal, value?: object) {
      return Http!.saveJournal(name(kind), JSON.stringify(value ?? null));
    },
  };
}
