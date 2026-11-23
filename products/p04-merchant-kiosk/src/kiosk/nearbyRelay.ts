import {sha256} from "@noble/hashes/sha256";
import {
  bytesToHex,
  hexToBytes,
  commerceRequestId,
  CommerceReservationError,
  packNearbyPresentation,
  unpackNearbyPresentation,
  fromBase64,
  toBase64,
  orderLock,
  quoteOrderRevision,
  quoteV2Layout,
  verifyOrderLock,
  preparePresentedPayment,
  verifyRentalReviewAuthority,
  verifyRentalPaymentDecision,
  type CommercePaymentProfile,
  type CommerceQuote,
  type RentalReviewAuthority,
  type VerifiedCommercePayment,
  type CommerceClient,
  type CommerceOrder,
  type NearbyReady,
  type NearbyReply,
  type NearbyRequest,
} from '@nu54/protocol';
import type { MerchantInbox } from './merchantInbox.ts';

const current = () => Math.floor(Date.now() / 1000);
type Pair = {
  sale: string;
  customer: string;
  link: string;
  code: string;
  until: number;
  approved: boolean;
};


type RentalReviewRoute = {
  policy: CommercePaymentProfile;
  read: (payment: VerifiedCommercePayment, authority: RentalReviewAuthority, signal: AbortSignal,
    decided: (wire:string)=>void, begin:()=>void) => Promise<string>;
};

type RentalReviewEntry = {
  link: string;
  request: string;
  challenge: string;
  digest: string;
  until: number;
  controller: AbortController;
  result: Promise<string>;
  reviewDigest?: string;
  decision?: string;
  timer: ReturnType<typeof setTimeout>;
};

type RentalReviewSource = {
  customer: string;
  key: string;
  link: string;
  quote: CommerceQuote;
  timer: ReturnType<typeof setTimeout>;
};

/** A quote this terminal handed to one customer; a lock for that order must answer exactly this. */
type IssuedQuote = { customer: string; quote: CommerceQuote; link?: string; sent?: string };
/** What the public screen may show about a phone that is approving or has submitted its order. */
export type RelayProgress =
  | { sale: string; stage: 'quoted' | 'submitting'; until: number }
  | { sale: string; stage: 'disconnected' | 'connected' }
  | { sale: string; stage: 'unsupported' };
// The phone reads its order number over this link until it collects the order, then a little longer.
const statusGrace = 600;
const statusLongest = 12 * 3_600;
const issuedLimit = 64;

/** Routes verified customer sessions independently of the visitor using the public screen. */
export class NearbyRelay {
  private readonly pair = new Map<string, Pair>();
  private readonly touch: (pair?: { code: string; until: number; codes: string[] }) => void;
  private readonly api: CommerceClient;
  private readonly inbox: MerchantInbox;
  private readonly entropy: () => Promise<Uint8Array>;
  private readonly publicOrder: () => string | undefined;
  private readonly publicPOS?: () => string | undefined;
  private readonly rental?: RentalReviewRoute;
  private readonly progress?: (event: RelayProgress) => void;
  private readonly issued = new Map<string, IssuedQuote>();
  private readonly activeLinks = new Set<string>();
  private readonly quotes = new Map<string, RentalReviewSource>();
  private readonly reviews = new Map<string, RentalReviewEntry>();
  private readonly reviewFences = new Map<string, ReturnType<typeof setTimeout>>();

  constructor(
    api: CommerceClient,
    inbox: MerchantInbox,
    entropy: () => Promise<Uint8Array>,
    publicOrder: () => string | undefined,
    touch: (pair?: { code: string; until: number; codes: string[] }) => void,
    publicPOS?: () => string | undefined,
    rental?: RentalReviewRoute,
    progress?: (event: RelayProgress) => void,
  ) {
    this.api = api;
    this.inbox = inbox;
    this.entropy = entropy;
    this.publicOrder = publicOrder;
    this.touch = touch;
    this.publicPOS = publicPOS;
    this.rental = rental;
    this.progress = progress;
  }

  clearPublic() {
    this.pair.clear();
    this.touch();
  }

  activate(link: string) {
    this.activeLinks.add(link);
  }

  close() {
    this.activeLinks.clear();
    for (const sale of this.reviews.keys()) this.removeReview(sale);
    for (const sale of this.quotes.keys()) this.removeQuote(sale);
    for (const timer of this.reviewFences.values()) clearTimeout(timer);
    this.reviewFences.clear();
    this.clearPublic();
  }

  retire(link: string) {
    this.activeLinks.delete(link);
    for (const [sale, row] of this.issued) {
      if (row.link !== link) continue;
      row.link = undefined;
      if (row.quote.expiresAt > current()) this.progress?.({ sale, stage: 'disconnected' });
    }
    for (const [sale, input] of this.quotes)
      if (input.link === link) this.removeQuote(sale);
    for (const [sale, row] of this.reviews)
      if (row.link === link) this.removeReview(sale);
    for (const [key, row] of this.pair)
      if (row.link === link) this.pair.delete(key);
    this.showPrompt();
  }

  approveDisplayed(code: string) {
    const selected = [...this.pair.values()].find(
      row =>
        row.code === code &&
        row.until > current() &&
        row.sale === this.publicOrder(),
    );
    if (!selected) throw new Error('PAIRING_CLOSED');
    selected.approved = true;
    this.showPrompt();
  }

  private showPrompt() {
    const waiting = [...this.pair.values()].filter(
      row =>
        !row.approved &&
        row.until > current() &&
        row.sale === this.publicOrder(),
    );
    const next = waiting[0];
    this.touch(next ? { code: next.code, until: next.until, codes: waiting.map(row => row.code) } : undefined);
  }

  async handle(req: NearbyRequest, peer: NearbyReady): Promise<NearbyReply> {
    if (
      peer.branch !== this.api.profile.branchRef ||
      req.branch !== peer.branch ||
      peer.expiresAt <= current()
    )
      return { status: 'denied' };
    for (const [sale, row] of this.issued) {
      if (row.customer !== peer.principal || row.link !== undefined || row.quote.expiresAt <= current()) continue;
      row.link = peer.link;
      this.progress?.({ sale, stage: 'connected' });
    }
    try {
      if (req.op === 'discover') {
        const shop = await this.api.shopIdentity();
        return {status: 'ok', value: {...shop, terminal: this.api.profile.subjectRef}};
      }
      if (req.op === 'catalog') {
        const page = Math.floor(req.offset / 50);
        const {prices, snapshotVersion} = await this.api.catalogWindow(page);
        const offset = req.offset % 50;
        const rows = prices.slice(offset, offset + 4);
        const next =
          offset + rows.length < prices.length || prices.length === 50
            ? req.offset + rows.length
            : undefined;
        return { status: 'ok', value: { rows, next, snapshotVersion } };
      }
      if (req.op === 'reserve') {
        if (req.deadline !== undefined) {
          const sale = await this.api.reserve({...req.cart,
            nearby: {customerRef: peer.principal, expiresAt: req.deadline},
          }, req.asset, req.key);
          return {status: 'ok', value: await this.customerOrder(sale)};
        }
        await this.inbox.remember(req.cart, req.asset, req.key, peer.principal);
        const sale = await this.inbox.reconcile(
          req.key,
          (cart, asset, key) => this.api.reserve(cart, asset, key),
          peer.principal,
        );
        return { status: 'ok', value: await this.customerOrder(sale) };
      }
      if (req.op === 'status') {
        if (req.authoritative) {
          const sale = await this.api.nearbyReservation(req.order ?? req.key, peer.principal, req.order !== undefined);
          return sale ? {status: 'ok', value: await this.customerOrder(sale)} : {status: 'denied'};
        }
        if (req.order)
          return this.inbox.ownsSale(req.order, peer.principal)
            ? { status: 'ok', value: await this.followedOrder(req.order, peer.principal) }
            : { status: 'denied' };
        if (!this.inbox.hasIntent(req.key, peer.principal)) return { status: 'denied' };
        const sale = await this.inbox.reconcile(
          req.key,
          (cart, asset, key) => this.api.reserve(cart, asset, key),
          peer.principal,
        );
        return { status: 'ok', value: await this.customerOrder(sale) };
      }
      if (req.op === 'rental-review-source') return await this.rentalReviewSource(req, peer);
      if (req.op === 'rental-review') return await this.rentalReview(req, peer);
      if (req.op === 'rental-decision') return await this.rentalDecision(req, peer);
      if (req.op === 'forward') {
        if (req.authoritative
          ? !await this.api.nearbyReservation(req.order, peer.principal, true)
          : !this.inbox.ownsSale(req.order, peer.principal))
          return { status: 'denied' };
        const submitted = await this.api.forward(
          req.order,
          req.quoteDigest,
          req.approvalDigest,
          req.signature,
          req.key,
        );
        return { status: 'ok', value: { submitted } };
      }
      if (req.op === 'lock') return await this.lock(req, peer);
      return await this.checkout(req, peer);
    } catch (error) {
      const name = error instanceof Error ? error.message : '';
      if (name === 'OUT_OF_STOCK') return {
        status: 'sold-out',
        ...(error instanceof CommerceReservationError ? {value: error.unavailable} : {}),
      };
      if (name === 'CATALOG_CHANGED') return { status: 'catalog-changed' };
      if ((req.op === 'rental-review' || req.op === 'rental-review-source') &&
          (/^(PRESENTATION_|RENTAL_REVIEW_)/.test(name) ||
            name === 'PAYMENT_ORDER_STATE' || name === 'PAYMENT_REFUSED')) return {status:'denied'};
      if (req.op === 'checkout' && name === 'UNSUPPORTED_CAPABILITY') {
        if (this.publicPOS?.() === req.order) this.progress?.({ sale: req.order, stage: 'unsupported' });
        return { status: 'denied' };
      }
      if (name === 'EXPIRED' && error instanceof CommerceReservationError) return {status: 'expired'};
      if (
        [
          'REPLAY_CONFLICT',
          'BAD_REQUEST',
          'ORDER_INTENT_MISSING',
          'NOT_AUTHORIZED',
          'FORBIDDEN',
          'CHECKOUT_CONFLICT',
        ].includes(name)
      )
        return { status: 'denied' };
      return { status: 'pending' };
    }
  }

  private removeQuote(sale: string) {
    const input = this.quotes.get(sale);
    if (input) clearTimeout(input.timer);
    this.quotes.delete(sale);
  }

  private removeReview(sale: string) {
    const row = this.reviews.get(sale);
    if (row) {
      clearTimeout(row.timer);
      row.controller.abort();
    }
    this.reviews.delete(sale);
  }

  private fenceReview(sale: string, expires: number) {
    const timer = setTimeout(() => this.reviewFences.delete(sale), Math.max(0, expires * 1000 - Date.now()));
    this.reviewFences.set(sale, timer);
  }

  private rememberQuote(quote: CommerceQuote, peer: NearbyReady, key: string) {
    const until = Math.min(quote.expiresAt, peer.expiresAt);
    if (!this.rental || !this.activeLinks.has(peer.link) || until <= current() ||
        (!this.quotes.has(quote.saleRef) && this.quotes.size >= 8)) return false;
    this.removeQuote(quote.saleRef);
    const timer = setTimeout(() => this.removeQuote(quote.saleRef), Math.max(0, until * 1000 - Date.now()));
    this.quotes.set(quote.saleRef, {customer: peer.principal, key, link: peer.link, quote, timer});
    return true;
  }

  private async rentalReviewSource(
    req: Extract<NearbyRequest, {op: 'rental-review-source'}>,
    peer: NearbyReady,
  ): Promise<NearbyReply> {
    if (!this.rental || !this.activeLinks.has(peer.link)) return {status:'denied'};
    const {quote} = unpackNearbyPresentation(req.value, peer.branch, fromBase64);
    if (quote.saleRef !== req.order) return {status:'denied'};
    const order = await this.api.status(req.order);
    const payment = preparePresentedPayment(quote, order, this.rental.policy, current(),
      {branch:peer.branch,sale:req.order,customer:peer.principal});
    if (payment.payerRole !== 'renter' || peer.expiresAt <= current() || !this.activeLinks.has(peer.link))
      return {status:'denied'};
    return this.rememberQuote(quote, peer, req.key) ? {status:'ok'} : {status:'pending'};
  }

  private async rentalReview(
    req: Extract<NearbyRequest, {op: 'rental-review'}>,
    peer: NearbyReady,
  ): Promise<NearbyReply> {
    const quote = this.quotes.get(req.order);
    const options = this.rental;
    if (!options || !this.activeLinks.has(peer.link) || quote?.customer !== peer.principal ||
        quote.link !== peer.link || quote.key !== req.key || quote.quote.quoteDigest !== req.quoteDigest ||
        quote.quote.expiresAt <= current())
      return {status: 'denied'};
    const order = await this.api.status(req.order);
    const scope = {branch: peer.branch, sale: req.order, customer: peer.principal};
    const proof = verifyRentalReviewAuthority(quote.quote, order, options.policy, current(), scope,
      req.authority as RentalReviewAuthority, req);
    if (!this.activeLinks.has(peer.link) || proof.expiresAt > peer.expiresAt || peer.expiresAt <= current()) return {status: 'denied'};
    const old = this.reviews.get(req.order);
    if (old) {
      if (old.link !== peer.link || old.request !== proof.request || old.challenge !== proof.challenge ||
          old.digest !== req.quoteDigest || current() >= old.until || old.controller.signal.aborted) return {status: 'denied'};
      const receipt = await old.result;
      return current() < proof.expiresAt && this.activeLinks.has(peer.link) && !old.controller.signal.aborted
        ? {status: 'ok', value: {receipt}} : {status: 'expired'};
    }
    if (this.reviewFences.has(req.order)) return {status:'denied'};
    if (this.reviewFences.size >= 8) return {status:'pending'};
    const controller = new AbortController();
    const payment = preparePresentedPayment(quote.quote, order, options.policy, current(), scope);
    const sale = req.order;
    let began=false;
    const begin=()=>{
      if (began || controller.signal.aborted || current()>=proof.expiresAt ||
          !this.activeLinks.has(peer.link) || this.reviews.get(sale)!==record ||
          this.quotes.get(sale)!==quote || this.reviewFences.has(sale) || this.reviewFences.size>=8)
        throw new Error('RENTAL_REVIEW_SCOPE');
      this.fenceReview(sale,quote.quote.expiresAt);
      began=true;
    };
    const timer = setTimeout(() => {
      this.removeReview(sale);
      if (this.quotes.get(sale) === quote) this.removeQuote(sale);
    }, Math.max(0, proof.expiresAt * 1000 - Date.now()));
    const result = Promise.resolve().then(() => {
      if (controller.signal.aborted) throw new Error('RENTAL_REVIEW_SCOPE');
      return options.read(payment, proof, controller.signal, wire => {
        result.then(review => {
          if (controller.signal.aborted || current() >= proof.expiresAt || this.reviews.get(sale) !== record || record.decision)
            return;
          verifyRentalPaymentDecision(hexToBytes(wire),hexToBytes(review),payment,{...proof,now:current()});
          record.decision = wire;
        }).catch(() => {});
      },begin);
    }).catch(error=>{
      if (!began && this.reviews.get(sale)===record) this.removeReview(sale);
      throw error;
    });
    const record:RentalReviewEntry = {link:peer.link,request:proof.request,challenge:proof.challenge,
      digest:req.quoteDigest,until:proof.expiresAt,controller,result,timer};
    this.reviews.set(sale,record);
    const receipt = await result;
    record.reviewDigest = bytesToHex(sha256(hexToBytes(receipt)),false);
    if (controller.signal.aborted || !this.activeLinks.has(peer.link) || current() >= proof.expiresAt) return {status: 'expired'};
    return {status: 'ok', value: {receipt}};
  }

  private async rentalDecision(
    req:Extract<NearbyRequest, {op:'rental-decision'}>,
    peer:NearbyReady,
  ):Promise<NearbyReply> {
    const scope=this.quotes.get(req.order);
    const record=this.reviews.get(req.order);
    const live=()=>this.activeLinks.has(peer.link) && scope!==undefined && scope.link===peer.link &&
      scope.customer===peer.principal && scope.key===req.key && scope.quote.quoteDigest===req.quoteDigest &&
      record!==undefined && this.reviews.get(req.order)===record && this.quotes.get(req.order)===scope &&
      record.link===peer.link && record.request===req.request && record.challenge===req.challenge &&
      record.reviewDigest===req.reviewDigest && !record.controller.signal.aborted &&
      current()<Math.min(record.until,peer.expiresAt);
    if (!live()) return {status:'denied'};
    const order=await this.api.status(req.order);
    if (!live() || order.id!==req.order || order.state!=='issued' || order.quoteDigest!==req.quoteDigest)
      return {status:'denied'};
    return record?.decision ? {status:'ok',value:{decision:record.decision}} : {status:'pending'};
  }

  /** Also used when the kiosk itself fetched the quote for a phone chosen by its mark. */
  rememberIssued(quote: CommerceQuote, customer: string, link?: string) {
    for (const [sale, row] of this.issued)
      if (row.quote.expiresAt + statusGrace <= current()) this.issued.delete(sale);
    const prior = this.issued.get(quote.saleRef);
    if (prior?.quote.quoteDigest === quote.quoteDigest && prior.customer === customer) {
      // A repeated checkout must not forget that this lock may already have been sent.
      prior.quote = quote;
      prior.link = link ?? prior.link;
    } else {
      if (this.issued.size >= issuedLimit) this.issued.delete(this.issued.keys().next().value!);
      this.issued.set(quote.saleRef, { customer, quote, link });
    }
    this.progress?.({ sale: quote.saleRef, stage: 'quoted', until: quote.expiresAt });
  }

  /** Keeps `status` open for the customer who was quoted, so the order number can follow the payment. */
  private async keepStatus(sale: string, customer: string) {
    await this.inbox.keepStatus(sale, customer,
      this.inbox.statusUntil(sale, customer) ?? current() + statusLongest).catch(() => {});
  }

  private async followedOrder(sale: string, customer: string) {
    const order = await this.customerOrder(sale);
    const until = this.inbox.statusUntil(sale, customer);
    const over = order.state === 'closed' || order.escrow?.stage === 'failed' ||
      ['collected', 'unfulfillable'].includes(order.payment?.pickupStage ?? '');
    if (over && until !== undefined && until > current() + statusGrace)
      await this.inbox.keepStatus(sale, customer, current() + statusGrace).catch(() => {});
    return order;
  }

  private async lock(
    req: Extract<NearbyRequest, { op: 'lock' }>,
    peer: NearbyReady,
  ): Promise<NearbyReply> {
    const owned = this.inbox.ownsSale(req.order, peer.principal);
    if (!owned && !await this.api.nearbyReservation(req.order, peer.principal, true))
      return { status: 'denied' };
    const held = this.issued.get(req.order);
    if (held) {
      const { quote } = held;
      if (held.customer !== peer.principal || quote.quoteDigest !== req.quoteDigest)
        return { status: 'denied' };
      const bytes = hexToBytes(quote.signedQuote);
      const unsigned = bytes.subarray(0, quoteV2Layout.unsignedSize);
      const payer = bytesToHex(unsigned.subarray(quoteV2Layout.payerOffset + 12, quoteV2Layout.payerOffset + 32));
      const payment = { bytes, payer, deadline: quote.expiresAt };
      try {
        if (orderLock(unsigned, req.confirmer).digest !== req.lockDigest) return { status: 'denied' };
        // Check the signature apart from the clock: a retry of a lock already sent may arrive late.
        verifyOrderLock(payment, req.confirmer, hexToBytes(req.signature), quote.expiresAt - 1);
      } catch {
        return { status: 'denied' };
      }
      const sent = req.confirmer + req.signature;
      if (held.sent !== undefined && held.sent !== sent) return { status: 'denied' };
      if (held.sent === undefined && current() >= quote.expiresAt) return { status: 'expired' };
      held.sent = sent;
      this.progress?.({ sale: req.order, stage: 'submitting', until: quote.expiresAt });
    }
    if (owned) await this.keepStatus(req.order, peer.principal);
    const submitted = await this.api.lock(
      req.order,
      req.quoteDigest,
      req.confirmer,
      req.lockDigest,
      req.signature,
      req.key,
    );
    return { status: 'ok', value: { submitted } };
  }

  private async customerOrder(sale: string) {
    const order = await this.api.status(sale);
    const { id, revision, state, asset, amount, expiresAt, pickupNo, receiptRef,
      receiptKind, quoteDigest, orderKey, payment, escrow } = order;
    return {
      id, revision, state, asset, amount, expiresAt, pickupNo, receiptRef,
      receiptKind, quoteDigest, orderKey,
      ...(payment ? { payment: {
        stage: payment.stage,
        chainId: payment.chainId,
        relayTxHash: payment.relayTxHash,
        txHash: payment.txHash,
        includedAt: payment.includedAt,
        pickupStage: payment.pickupStage,
      } } : {}),
      ...(escrow ? { escrow } : {}),
    } satisfies CommerceOrder;
  }

  private async checkout(
    req: Extract<NearbyRequest, { op: 'checkout' }>,
    peer: NearbyReady,
  ): Promise<NearbyReply> {
    const stored = req.authoritative
      ? !!await this.api.nearbyReservation(req.order, peer.principal, true)
      : this.inbox.ownsSale(req.order, peer.principal);
    if (req.authoritative && !stored) return {status: 'denied'};
    const activePOS = this.publicPOS?.() === req.order;
    if (!req.authoritative && (activePOS || (this.publicPOS && req.order !== this.publicOrder()))) {
      const result = await this.api.posNearbyCheckout(
        req.order,
        peer.principal,
        req.key,
        activePOS && this.publicOrder() === req.order ? 'register' : 'resume',
      ).catch(error => {
        // A locally stored shopping-cart claim can still use the existing menu
        // path. A protected POS order also refuses authorization on that path.
        if (!activePOS && stored && error instanceof Error &&
            ['NOT_AUTHORIZED', 'FORBIDDEN', 'BAD_REQUEST', 'UNSUPPORTED_CAPABILITY'].includes(error.message))
          return undefined;
        throw error;
      });
      if (result) {
        if ('comparison' in result)
          return {status: 'pending', value: {code: result.comparison.code}};
        // Release a presentation only after P05 commits the selected quote.
        const order = await this.api.status(req.order);
        const quote = await this.api.quote(
          req.order,
          result.claim.claimDigest,
          quoteOrderRevision(order),
          req.key,
        );
        await this.inbox.grantClaim(req.order, peer.principal, result.claim.expiresAt);
        this.rememberIssued(quote, peer.principal, peer.link);
        return {
          status: 'ok',
          value: packNearbyPresentation(await this.customerOrder(req.order), quote, toBase64),
        };
      }
    }
    if (!stored) {
      if (req.order !== this.publicOrder()) return { status: 'denied' };
      for (const [key, row] of this.pair)
        if (row.until <= current()) this.pair.delete(key);
      const requested = this.pair.get(req.key);
      if (
        requested &&
        (requested.customer !== peer.principal ||
          requested.link !== peer.link ||
          requested.sale !== req.order)
      )
        return { status: 'denied' };
      if (!requested) {
        if (this.pair.size >= 8) return { status: 'pending' };
        const random = commerceRequestId(await this.entropy())
          .replace(/-/g, '')
          .slice(-8);
        if (
          this.pair.has(req.key) ||
          [...this.pair.values()].some(row => row.code === random) ||
          req.order !== this.publicOrder()
        ) {
          return { status: 'pending' };
        }
        this.pair.set(req.key, {
          sale: req.order,
          customer: peer.principal,
          link: peer.link,
          code: random,
          until: current() + 35,
          approved: false,
        });
        this.showPrompt();
        return { status: 'pending', value: { code: random } };
      }
      if (!requested.approved)
        return { status: 'pending', value: { code: requested.code } };
    }

    const order = await this.api.status(req.order);
    const claim = await this.api.authorize(req.order, peer.principal, req.key);
    if (!req.authoritative) await this.inbox.grantClaim(req.order, peer.principal, claim.expiresAt);
    // Quote issuance idempotency uses a different operation namespace. The same immutable
    // bytes are the result of every retry and of the customer's authenticated direct path.
    const quote = await this.api.quote(
      req.order,
      claim.claimDigest,
      quoteOrderRevision(order),
      req.key,
    );
    this.rememberIssued(quote, peer.principal, peer.link);
    return {
      status: 'ok',
      value: packNearbyPresentation(
        await this.customerOrder(req.order),
        quote,
        toBase64,
      ),
    };
  }
}
