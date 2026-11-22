import {
  bytesToHex,
  encodeCommerceCart,
  commerceReservationRejected,
  utf8Encode,
  type CheckoutComparison,
  type CommerceCart,
  type CommerceOrder,
} from '@nu54/protocol';
import {sha256} from '@noble/hashes/sha256';
import type { KeyValue } from './orders.ts';

type RecordV2 = {
  ownerRef: string;
  requestKey: string;
  cart: CommerceCart;
  asset: string;
  saleRef?: string;
  rejected?: 'OUT_OF_STOCK' | 'CATALOG_CHANGED';
  comparison?: CheckoutComparison;
  publicSession?: string;
  cancelPending?: true;
};
type ClaimAccess = { sale: string; owner: string; until: number };
type Denial = {
  ownerRef: string;
  requestKey: string;
  originDigest: string;
  bodyDigest: string;
  rejected: NonNullable<RecordV2['rejected']>;
};
type CancelPublic = (session: string, reserveKey: string) => Promise<void>;
type Reserve = (
  cart: CommerceCart,
  asset: string,
  key: string,
) => Promise<string>;
const inboxKey = 'commerce-inbox-v2';
const nearbyKey = 'commerce-nearby-inbox-v2';
const denialKey = 'commerce-denials-v2';
const ref =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const address = /^0x[0-9a-f]{40}$/;
const limit = 64;
const legacyRejectionLimit = 256;
const maxJournalBytes = 128000;
const digest = (value: string) => bytesToHex(sha256(utf8Encode(value)), false);
const bodyDigest = (entry: RecordV2) => digest(entry.asset + encodeCommerceCart(entry.cart));
const denied = (entry: RecordV2, rejected: Denial['rejected']): Denial => ({
  ownerRef: entry.ownerRef,
  requestKey: entry.requestKey,
  originDigest: digest(entry.cart.originRef),
  bodyDigest: bodyDigest(entry),
  rejected,
});

function record(input: RecordV2): RecordV2 {
  if (
    !input ||
    !ref.test(input.ownerRef) ||
    !ref.test(input.requestKey) ||
    !address.test(input.asset) ||
    (input.publicSession !== undefined && (!ref.test(input.publicSession) ||
      input.cart.origin !== 'menu' || input.cart.originRef !== input.publicSession)) ||
    (input.cancelPending !== undefined && (input.cancelPending !== true || !input.publicSession)) ||
    (input.saleRef !== undefined && !ref.test(input.saleRef)) ||
    (input.rejected !== undefined &&
      (input.saleRef !== undefined ||
        !['OUT_OF_STOCK', 'CATALOG_CHANGED'].includes(input.rejected))) ||
    (input.comparison !== undefined &&
      (!input.saleRef ||
        !input.comparison ||
        !ref.test(input.comparison.customerRef ?? '') ||
        !/^[0-9a-f]{8}$/.test(input.comparison.code) ||
        !Number.isSafeInteger(input.comparison.expiresAt)))
  )
    throw new Error('INBOX_INVALID');
  return {
    ...input,
    cart: JSON.parse(encodeCommerceCart(input.cart)) as CommerceCart,
    ...(input.comparison ? { comparison: { ...input.comparison } } : {}),
  };
}

/** Durable server references and intents. The public screen owns a separate in-memory capability. */
export class MerchantInbox {
  private slot?: { handle: string; key?: string; pending?: boolean };
  private changes = Promise.resolve();
  private inflight = new Map<string, Promise<string>>();
  private cancellations = new Map<string, Promise<void>>();
  private constructor(
    private readonly disk: KeyValue,
    private readonly branch: string,
    private readonly terminal: string,
    private readonly entries: Map<string, RecordV2>,
    private readonly claims: Map<string, ClaimAccess>,
    private readonly denials: Map<string, Denial>,
    private readonly persisted: Map<string, string>,
  ) {}

  static async open(disk: KeyValue, branch: string, terminal: string) {
    const keys = [inboxKey, nearbyKey, denialKey];
    const values = await Promise.all(keys.map(key => disk.getSetting(key)));
    const persisted = new Map(keys.map((key, index) => [key, values[index] ?? '']));
    const entries = new Map<string, RecordV2>();
    const claims = new Map<string, ClaimAccess>();
    const denials = new Map<string, Denial>();
    for (const [key, raw] of persisted) {
      if (!raw) continue;
      if (raw.length > maxJournalBytes) throw new Error('INBOX_INVALID');
      const snapshot = JSON.parse(raw) as {
        version: number;
        branch: string;
        terminal: string;
        intents?: RecordV2[];
        claims?: ClaimAccess[];
        denials?: Denial[];
      };
      if (
        !snapshot ||
        snapshot.version !== 2
      )
        throw new Error('INBOX_INVALID');
      if (snapshot.branch !== branch || snapshot.terminal !== terminal)
        throw new Error('INBOX_IDENTITY_CHANGED');
      if (key === denialKey) {
        if (!Array.isArray(snapshot.denials) || snapshot.denials.length > legacyRejectionLimit + limit)
          throw new Error('INBOX_INVALID');
        for (const item of snapshot.denials) {
          if (!item || !ref.test(item.ownerRef) || item.ownerRef === terminal || !ref.test(item.requestKey) ||
            !/^[a-f0-9]{64}$/.test(item.originDigest) || !/^[a-f0-9]{64}$/.test(item.bodyDigest) ||
            !['OUT_OF_STOCK', 'CATALOG_CHANGED'].includes(item.rejected))
            throw new Error('INBOX_INVALID');
          const prior = denials.get(item.requestKey);
          if (prior && (prior.ownerRef !== item.ownerRef || prior.bodyDigest !== item.bodyDigest ||
            prior.originDigest !== item.originDigest || prior.rejected !== item.rejected))
            throw new Error('INBOX_INVALID');
          denials.set(item.requestKey, item);
        }
        continue;
      }
      if (!Array.isArray(snapshot.intents) || snapshot.intents.length > limit)
        throw new Error('INBOX_INVALID');
      if (snapshot.claims !== undefined) {
        if (!Array.isArray(snapshot.claims) || snapshot.claims.length > limit)
          throw new Error('INBOX_INVALID');
        for (const claim of snapshot.claims) {
          if (
            !claim ||
            !ref.test(claim.sale) ||
            !ref.test(claim.owner) ||
            !Number.isSafeInteger(claim.until)
          )
            throw new Error('INBOX_INVALID');
          if (claim.until > Math.floor(Date.now() / 1000))
            claims.set(claim.sale, claim);
        }
      }
      for (const item of snapshot.intents) {
        const entry = record(item);
        const prior = entries.get(entry.requestKey);
        if (prior && (prior.ownerRef !== entry.ownerRef || bodyDigest(prior) !== bodyDigest(entry) ||
          prior.saleRef && entry.saleRef && entry.saleRef !== prior.saleRef))
          throw new Error('INBOX_INVALID');
        if (entry.rejected) {
          denials.set(entry.requestKey, denied(entry, entry.rejected));
          continue;
        }
        entries.set(entry.requestKey, entry);
      }
    }
    for (const [key, denial] of denials) {
      const previous = entries.get(key);
      if (!previous) continue;
      if (previous.ownerRef !== denial.ownerRef || previous.saleRef || bodyDigest(previous) !== denial.bodyDigest)
        throw new Error('INBOX_INVALID');
      entries.delete(key);
    }
    return new MerchantInbox(disk, branch, terminal, entries, claims, denials, persisted);
  }

  openPublic(handle: string) {
    if (this.slot || !ref.test(handle))
      throw new Error('PUBLIC_SESSION_IN_USE');
    this.slot = { handle };
    return handle;
  }

  endPublic() {
    this.slot = undefined;
  }

  async cancelPublic(handle: string, cancel: CancelPublic) {
    const slot = this.slot;
    if (!slot || slot.handle !== handle || !slot.key) return;
    const key = slot.key;
    await this.mutate(draft => {
      const item = draft.get(key);
      if (item?.ownerRef === this.terminal && item.publicSession === handle)
        draft.set(key, {...item, cancelPending:true});
    });
    await this.flushPublicCancellations(cancel);
  }

  async flushPublicCancellations(cancel: CancelPublic) {
    const pending: Promise<void>[] = [];
    for (const item of this.entries.values()) {
      if (!item.cancelPending || !item.publicSession || item.ownerRef !== this.terminal) continue;
      let running = this.cancellations.get(item.requestKey);
      if (!running) {
        const session = item.publicSession;
        running = (async () => {
          await cancel(session, item.requestKey);
          await this.mutate(draft => {
            const current = draft.get(item.requestKey);
            if (current?.publicSession !== session || !current.cancelPending) return;
            if (!current.saleRef && this.slot?.key !== item.requestKey) {
              draft.delete(item.requestKey);
              return;
            }
            const confirmed = {...current};
            delete confirmed.cancelPending;
            draft.set(item.requestKey, confirmed);
          });
        })().catch(() => {
          // Try the same key and session on the next authenticated connection.
        }).finally(() => {
          this.cancellations.delete(item.requestKey);
        });
        this.cancellations.set(item.requestKey, running);
      }
      pending.push(running);
    }
    await Promise.all(pending);
  }

  publicOrder() {
    const publicKey = this.slot?.key;
    return publicKey ? this.entries.get(publicKey)?.saleRef : undefined;
  }

  async stage(
    handle: string,
    cart: CommerceCart,
    asset: string,
    requestKey: string,
  ) {
    const slot = this.slot;
    if (!slot || slot.handle !== handle || slot.pending || slot.key)
      throw new Error('PUBLIC_SESSION_CLOSED');
    slot.pending = true;
    try {
      const publicSession = cart.origin === 'menu' && cart.originRef === handle ? handle : undefined;
      await this.remember(cart, asset, requestKey, this.terminal, publicSession);
      if (this.slot === slot) slot.key = requestKey;
    } finally {
      slot.pending = false;
    }
  }

  /** Independent authenticated customer channels never mutate the public slot. */
  async remember(
    cart: CommerceCart,
    asset: string,
    requestKey: string,
    ownerRef = this.terminal,
    publicSession?: string,
  ) {
    const entry = record({ ownerRef, requestKey, cart, asset, publicSession });
    await this.mutate((draft, _claims, rejections) => {
      const old = draft.get(requestKey);
      const rejected = rejections.get(requestKey);
      if (rejected) {
        if (rejected.ownerRef !== ownerRef || rejected.bodyDigest !== bodyDigest(entry))
          throw new Error('REPLAY_CONFLICT');
        return;
      }
      if (old) {
        if (
          old.ownerRef !== ownerRef ||
          old.asset !== entry.asset ||
          old.publicSession !== entry.publicSession ||
          encodeCommerceCart(old.cart) !== encodeCommerceCart(entry.cart)
        )
          throw new Error('REPLAY_CONFLICT');
        return;
      }
      const records = [...draft.values()];
      const publicOrder = ownerRef === this.terminal;
      if (records.filter(item => (item.ownerRef === this.terminal) === publicOrder).length >= limit)
        throw new Error('INBOX_FULL');
      if (ownerRef !== this.terminal &&
        rejections.size >= legacyRejectionLimit)
        throw new Error('INBOX_FULL');
      if (ownerRef !== this.terminal &&
        (records.some(item => item.cart.origin === entry.cart.origin && item.cart.originRef === entry.cart.originRef) ||
          [...rejections.values()].some(item => item.originDigest === digest(entry.cart.originRef))))
        throw new Error('REPLAY_CONFLICT');
      draft.set(requestKey, entry);
    });
  }

  dispatch(handle: string, reserve: Reserve) {
    if (!this.slot || this.slot.handle !== handle || !this.slot.key)
      throw new Error('PUBLIC_SESSION_CLOSED');
    const key = this.slot.key;
    const slot = this.slot;
    return this.reconcile(key, reserve).catch(async error => {
      if (commerceReservationRejected(error) && this.slot === slot)
        slot.key = undefined;
      throw error;
    });
  }

  reconcile(key: string, reserve: Reserve, ownerRef = this.terminal) {
    const denial = this.denials.get(key);
    if (denial?.ownerRef === ownerRef) return Promise.reject(new Error(denial.rejected));
    const item = this.entries.get(key);
    if (!item || item.ownerRef !== ownerRef)
      throw new Error('ORDER_INTENT_MISSING');
    if (item.saleRef) return Promise.resolve(item.saleRef);
    const current = this.inflight.get(key);
    if (current) return current;
    const run = reserve(item.cart, item.asset, key)
      .then(async saleRef => {
        if (!ref.test(saleRef)) throw new Error('BAD_RESPONSE');
        await this.mutate(draft => {
          const latest = draft.get(key);
          if (latest) draft.set(key, { ...latest, saleRef });
        });
        return saleRef;
      })
      .catch(async error => {
        if (commerceReservationRejected(error)) {
          await this.mutate((draft, _claims, rejections) => {
            const rejected = draft.get(key);
            if (rejected?.ownerRef !== ownerRef || rejected.saleRef || rejected.cancelPending)
              return;
            if (ownerRef !== this.terminal)
              rejections.set(key, denied(rejected, error.message === 'OUT_OF_STOCK' ? 'OUT_OF_STOCK' : 'CATALOG_CHANGED'));
            draft.delete(key);
          });
        }
        throw error;
      })
      .finally(() => this.inflight.delete(key));
    this.inflight.set(key, run);
    return run;
  }

  recovery(ownerRef = this.terminal) {
    return [...this.entries.values()]
      .filter(row => row.ownerRef === ownerRef)
      .map(record);
  }

  hasIntent(key: string, owner: string) {
    return this.entries.get(key)?.ownerRef === owner || this.denials.get(key)?.ownerRef === owner;
  }

  async stageComparison(
    handle: string,
    sale: string,
    candidate: CheckoutComparison,
  ) {
    const key = this.slot?.key;
    if (!key || this.slot?.handle !== handle)
      throw new Error('PUBLIC_SESSION_CLOSED');
    await this.mutate(draft => {
      const item = draft.get(key);
      if (
        item?.saleRef !== sale ||
        item.ownerRef !== this.terminal ||
        item.comparison
      )
        throw new Error('REPLAY_CONFLICT');
      draft.set(key, record({ ...item, comparison: candidate }));
    });
  }

  async clearComparison(key: string, sale: string) {
    await this.mutate(draft => {
      const item = draft.get(key);
      if (item?.saleRef !== sale || item.ownerRef !== this.terminal)
        throw new Error('RECONCILE_REQUIRED');
      const order = { ...item };
      delete order.comparison;
      draft.set(key, order);
    });
  }

  /** Expire a local pointer only after authenticated server reconciliation. */
  async acknowledge(
    key: string,
    saleRef: string,
    check: (id: string) => Promise<CommerceOrder>,
    ownerRef = this.terminal,
  ) {
    const entry = this.entries.get(key);
    if (
      entry?.saleRef !== saleRef ||
      entry.ownerRef !== ownerRef ||
      this.slot?.key === key
    )
      throw new Error('RECONCILE_REQUIRED');
    const proof = await check(saleRef);
    if (
      proof.id !== saleRef ||
      (proof.state !== 'paid' && proof.state !== 'closed')
    )
      throw new Error('RECONCILE_REQUIRED');
    await this.mutate(draft => {
      const current = draft.get(key);
      if (current?.saleRef === saleRef && current.ownerRef === ownerRef &&
        !current.cancelPending && this.slot?.key !== key)
        draft.delete(key);
    });
  }

  async reclaimPublicOrders(check: (id: string) => Promise<CommerceOrder>) {
    const publicIntents = () => [...this.entries.values()].filter(item => item.ownerRef === this.terminal);
    const active = publicIntents();
    if (active.length < limit - 8) return;
    const completed = active.filter(item => item.publicSession && item.saleRef &&
      !item.cancelPending && this.slot?.key !== item.requestKey);
    // Bound simultaneous status reads without leaving newer closed orders behind
    // older unresolved ones.
    for (let start = 0; start < completed.length; start += 8) {
      await Promise.allSettled(completed.slice(start, start + 8).map(item =>
        this.acknowledge(item.requestKey, item.saleRef!, check)));
      if (publicIntents().length < limit - 8) return;
    }
  }

  ownsSale(sale: string, owner: string) {
    if (this.recovery(owner).some(entry => entry.saleRef === sale)) return true;
    const access = this.claims.get(sale);
    return (
      !!access &&
      access.owner === owner &&
      access.until > Math.floor(Date.now() / 1000)
    );
  }

  async grantClaim(sale: string, owner: string, until: number) {
    const now = Math.floor(Date.now() / 1000);
    if (
      !ref.test(sale) ||
      !ref.test(owner) ||
      !Number.isSafeInteger(until) ||
      until <= now
    )
      throw new Error('INVALID_CLAIM');
    await this.mutate((_entries, claims) => {
      for (const [key, item] of claims)
        if (item.until <= now) claims.delete(key);
      const existing = claims.get(sale);
      if (existing && existing.owner !== owner)
        throw new Error('REPLAY_CONFLICT');
      if (!existing && claims.size >= limit) throw new Error('INBOX_FULL');
      claims.set(sale, { sale, owner, until });
    });
  }

  private mutate(
    edit: (
      draft: Map<string, RecordV2>,
      claims: Map<string, ClaimAccess>,
      rejections: Map<string, Denial>,
    ) => void,
  ) {
    const saved = this.changes.then(async () => {
      const draft = new Map(this.entries);
      const claimDraft = new Map(this.claims);
      const denialDraft = new Map(this.denials);
      edit(draft, claimDraft, denialDraft);
      const header = {version: 2, branch: this.branch, terminal: this.terminal};
      const privateIntents = [...draft.values()].filter(item => item.ownerRef !== this.terminal);
      const publicIntents = [...draft.values()].filter(item => item.ownerRef === this.terminal);
      const journals = new Map([
        [denialKey, denialDraft.size ? JSON.stringify({...header, denials: [...denialDraft.values()]}) : ''],
        [nearbyKey, privateIntents.length ? JSON.stringify({...header, intents: privateIntents}) : ''],
        [inboxKey, publicIntents.length || claimDraft.size
          ? JSON.stringify({...header, intents: publicIntents, claims: [...claimDraft.values()]}) : ''],
      ]);
      // A denial must reach disk before its raw intent is removed. If a process
      // stops between the two writes, opening the journal prefers the denial.
      for (const [key, raw] of journals) {
        if (raw === this.persisted.get(key)) continue;
        if (raw.length > maxJournalBytes) throw new Error('INBOX_FULL');
        await this.disk.putSetting(key, raw);
        this.persisted.set(key, raw);
        if (key === denialKey) {
          for (const [request, denial] of denialDraft) {
            this.denials.set(request, denial);
            this.entries.delete(request);
          }
        }
      }
      this.denials.clear();
      for (const [key, entry] of denialDraft) this.denials.set(key, entry);
      this.claims.clear();
      for (const [key, claim] of claimDraft) this.claims.set(key, claim);
      this.entries.clear();
      for (const [key, entry] of draft) this.entries.set(key, entry);
    });
    this.changes = saved.catch(() => {});
    return saved;
  }
}
