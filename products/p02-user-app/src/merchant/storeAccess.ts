import {sha256} from '@noble/hashes/sha256';
import {bindServiceRequestV2, bytesToHex, CommerceClient, hexToBytes, utf8Encode, type CommercePaymentProfile, type CommerceProfile, type ServiceRequestV2} from '@nu54/protocol';
import Http from '../specs/NativeCommerceHttp.ts';
import OwnerWallet from '../specs/NativeOwnerWallet.ts';
import {readEvmReceiveBalance, type EvmRpc} from '../wallet/evmTransfer.ts';
import {serverFor} from '../wallet/networks.ts';
import {decodeWalletStorage} from '../wallet/profile.ts';
import {readProfiles, walletReadRpc} from '../wallet/readProfiles.ts';
import {walletId} from './customerClient.ts';
import {EntranceError, nativeEntrance, type EntrancePost, type PickupBoard} from './customerEntrance.ts';
import {approvedCustomerEntrance, approvedOrderPayment} from './customerEntranceDeployment.ts';
import {customerDigest, type CustomerJournal} from './customerJournal.ts';
import {openCustomerSession, SessionEntrance, type CustomerSession, type SessionProblem, type SessionScope} from './customerSession.ts';
import {confirmPolicy, isPinnedPolicy, orderPaymentPin, PaymentTrustError, readMerchantRecord, unconfirmedPolicy, type PaymentTrustCode} from './orderPaymentTrust.ts';
import {ordersWithoutRegistration} from './orderRoute.ts';
import {orderRecords} from './orderRecords.ts';

// One store, ordered from without registering there (traveller-services.md chapter 9 items 2 and 6). The menu and
// its photos are public reads of the customer entrance, so the store page opens them with no board near. The board
// proves its account when the customer asks for the order: once for the cart, the quote and the payment of this
// store, and once for each paid order to follow it, answer for it and dispute it. What may be paid is checked
// against the order contract of this build (orderPaymentTrust.ts) before that first proof.

export type StoreChoice = {branch: string; name: string; merchantId?: string};

/** The store's server has too many requests now. Nothing is known to have changed; the same request is sent again shortly. */
export class StoreBusy extends Error {
  readonly code = 'CUSTOMER_ENTRANCE_BUSY';
  constructor() { super('BUSY'); }
}

/** The request needs the board's account proof first. `scope` says which one. */
export class SessionNeeded extends Error {
  readonly code = 'CUSTOMER_SESSION_NEEDED';
  constructor(readonly scope: SessionScope) { super('CUSTOMER_SESSION_NEEDED'); }
}

export type AccessProblem =
  | SessionProblem
  /** This phone has no board account yet: the wallet connects and tests a board first. */
  | {kind: 'no-account'}
  /** The store cannot be paid from this build now: it has no number in the order contract, or the chain said no. */
  | {kind: 'payment'; code: PaymentTrustCode | 'NO_MERCHANT'};

/**
 * The customer subject of a board account, as the store server derives it (opsd CustomerSubject): a version 4 UUID
 * from a domain-separated SHA-256. The phone needs it before any proof, to find what it kept for this account.
 */
export function customerSubject(account: string): string {
  if (!/^0x[0-9a-f]{40}$/.test(account)) throw new Error('CUSTOMER_ACCOUNT');
  const sum = sha256(new Uint8Array([...utf8Encode('nu54-customer-account-v1\0'), ...hexToBytes(account)]));
  sum[6] = sum[6] & 0x0f | 0x40;
  sum[8] = sum[8] & 0x3f | 0x80;
  const text = bytesToHex(sum.subarray(0, 16), false);
  return `${text.slice(0, 8)}-${text.slice(8, 12)}-${text.slice(12, 16)}-${text.slice(16, 20)}-${text.slice(20)}`;
}

type Wallet = {account: string; generation: string; rpc: EvmRpc};
export type AccessParts = {
  post: EntrancePost; origin: string; pin: NonNullable<ReturnType<typeof orderPaymentPin>>; board: PickupBoard;
  /** The board account this phone tested, and how it reads the chain; undefined while it has none. */
  wallet: () => Promise<Wallet | undefined>;
  now: () => number;
};

const margin = 40; // a request lives 30 seconds and must end before its session does
const sessionsLimit = 32;

/**
 * The sealed journal did not take the order sessions. The board proves again, so nothing is lost but a proof; yet a
 * name the native allow-list does not hold fails the same way on every phone, and did once without anyone seeing
 * it. A development build says so.
 */
function sessionsNotKept(step: 'read' | 'written', error: unknown) {
  if (__DEV__) console.error(`Order sessions were not ${step}: the sealed journal refused the name or the store.`, (error as {code?: string})?.code ?? error);
}

export class StoreAccess {
  readonly client: CommerceClient;
  readonly journals: {read(kind: CustomerJournal): Promise<string>; write(kind: CustomerJournal, value?: object): Promise<void>};
  /** What this access is, for telling one page's state from another's. */
  readonly id: string;
  private readonly profile: CommerceProfile;
  private checkout?: CustomerSession;
  private orders = new Map<string, CustomerSession>();
  private current: CommercePaymentProfile;
  private readonly sessionsName: string;

  private constructor(readonly store: StoreChoice, readonly account: string | undefined, private readonly parts: AccessParts) {
    const {branch} = store;
    // Before the first proof the tenant is not known, and nothing that needs it is sent. The subject is the board
    // account's, or a placeholder while this phone has no board; the server's answer to a proof must name the same.
    this.profile = {role: 'customer', branchRef: branch, tenantRef: branch, subjectRef: account ? customerSubject(account) : branch, expiresAt: 4_102_444_800};
    this.id = customerDigest(JSON.stringify(['entrance', parts.origin, branch, this.profile.subjectRef]));
    this.current = unconfirmedPolicy(parts.pin, store.merchantId);
    this.sessionsName = `customer-sessions-${this.id}`;
    const name = (kind: string) => `customer-${kind}-${this.id}`;
    this.journals = {
      read: async kind => { const raw = await Http!.loadJournal(name(kind)); return raw === 'null' ? '' : raw; },
      write: (kind, value) => Http!.saveJournal(name(kind), JSON.stringify(value ?? null)),
    };
    this.client = new CommerceClient(this.profile, body => this.exchange(body), walletId, parts.now);
  }

  static async open(store: StoreChoice, parts: AccessParts): Promise<StoreAccess> {
    const wallet = await parts.wallet().catch(() => undefined);
    const access = new StoreAccess(store, wallet?.account, parts);
    await access.restore();
    return access;
  }

  get subject() { return this.profile.subjectRef; }

  /**
   * What this store is paid by: the unconfirmed policy of the build until the chain confirmed the store, then the
   * pinned one. A confirmation puts a new object here; one handed out before is never changed.
   */
  get policy() { return this.current; }

  /** Order sessions outlive the page: each follows one paid order until it ends. They are kept in the sealed journal. */
  private async restore() {
    let raw: string;
    try {
      raw = await Http!.loadJournal(this.sessionsName);
    } catch (error) {
      sessionsNotKept('read', error);
      return;
    }
    try {
      const rows = raw ? JSON.parse(raw) as CustomerSession[] : [];
      for (const row of Array.isArray(rows) ? rows.slice(0, sessionsLimit) : []) {
        if (row?.purpose === 'order' && row.branch === this.store.branch && row.subject === this.subject && typeof row.sale === 'string' &&
            /^[0-9a-f]{64}$/.test(row.token) && Number.isSafeInteger(row.expiresAt) && row.expiresAt > this.parts.now()) {
          this.orders.set(row.sale, row);
          this.profile.tenantRef = row.tenant;
        }
      }
    } catch { /* what was kept does not read as sessions; the board proves again */ }
  }
  private keep() {
    const live = [...this.orders.values()].filter(row => row.expiresAt > this.parts.now()).slice(-sessionsLimit);
    return Http!.saveJournal(this.sessionsName, JSON.stringify(live)).catch(error => sessionsNotKept('written', error));
  }

  private usable(session?: CustomerSession) {
    return session && session.expiresAt - this.parts.now() > margin ? session : undefined;
  }

  /** The session one request goes with, or the proof it needs first. */
  private async sessionFor(command: ServiceRequestV2): Promise<{session?: CustomerSession; scope: SessionScope}> {
    const branch = this.store.branch, sale = command.objectRef;
    const paid: SessionScope = {purpose: 'order', branch, sale};
    if (command.operation === 'order.answer') return {session: this.usable(this.orders.get(sale)), scope: paid};
    const checkout = this.usable(this.checkout);
    if (command.operation !== 'order.status') return {session: checkout, scope: {purpose: 'checkout', branch}};
    const session = checkout ?? this.usable(this.orders.get(sale));
    if (session) return {session, scope: paid};
    // An order the store's server already reported as paid is followed by its own session; one still being paid
    // belongs to the store's checkout.
    const recorded = (await orderRecords(Http!).list().catch(() => [])).find(row => row.sale === sale && row.branch === branch);
    return {scope: recorded?.stage && recorded.stage !== 'received' && recorded.stage !== 'submitted' && recorded.stage !== 'failed' ? paid : {purpose: 'checkout', branch}};
  }

  private async exchange(body: string): Promise<string> {
    const {command} = JSON.parse(body) as {command: ServiceRequestV2};
    const branch = this.store.branch;
    try {
      if (command.operation === 'catalog.read' || command.operation === 'catalog.photo') {
        // Public reads: the menu page or the photo part, answered in the words of the same C3 reply.
        const path = command.operation === 'catalog.read' ? `/v2/customer/stores/${branch}/menu/${command.body.page}` as const
          : `/v2/customer/stores/${branch}/photos/${command.body.photo}/${command.body.offset}` as const;
        const reply = await this.parts.post(path, '');
        if (reply.status === 429) throw new StoreBusy();
        // A store that does not show its page to the public has no menu here either.
        if (reply.status === 404 && command.operation === 'catalog.read') throw new Error('STORE_NOT_PUBLIC');
        if (reply.status !== 200 || reply.body === null || typeof reply.body !== 'object') throw new Error('BAD_RESPONSE');
        const {requestId, requestDigest} = bindServiceRequestV2(command,
          {...this.profile, grants: [{domainRef: command.domainRef, objectRef: command.objectRef, operations: [`C3/${command.operation}`]}]}, this.parts.now());
        const {shop, orderState, catalog, catalogVersion, catalogOptions, catalogPhotos, photo} = reply.body as Record<string, unknown>;
        return JSON.stringify({requestId, requestDigest, status: 'ok', resultRef: branch,
          ...(command.operation === 'catalog.read' ? {shop, orderState, catalog, catalogVersion, catalogOptions, catalogPhotos} : {photo})});
      }
      const {session, scope} = await this.sessionFor(command);
      if (!session) throw new SessionNeeded(scope);
      const reply = await this.parts.post('/v2/customer/commerce', body, session.token);
      if (reply.status === 404) {
        // The session ended, or the request is outside it: the board proves again.
        if (session === this.checkout) this.checkout = undefined;
        else if (session.sale) { this.orders.delete(session.sale); void this.keep(); }
        throw new SessionNeeded(scope);
      }
      // Too many requests is no word about the session: it stands, and the same request is sent again shortly.
      if (reply.status === 429) throw new StoreBusy();
      if (reply.body === null || typeof reply.body !== 'object') throw new Error('BAD_RESPONSE');
      return JSON.stringify(reply.body);
    } catch (error) {
      // No answer from the entrance reads like any other lost connection to the store.
      if (error instanceof EntranceError && error.code === 'UNAVAILABLE') throw Object.assign(new Error('COMMERCE_NETWORK_OFFLINE'), {code: 'COMMERCE_NETWORK_OFFLINE'});
      throw error;
    }
  }

  /** Whether the next request of this kind would go out without asking the board. */
  live(scope: SessionScope) {
    return !!this.usable(scope.purpose === 'order' ? this.orders.get(scope.sale) : this.checkout);
  }

  /** Reads the store's merchant record from the order contract of this build and makes the policy the pinned one. */
  private async confirmBy(rpc: EvmRpc): Promise<AccessProblem | undefined> {
    if (isPinnedPolicy(this.current)) return undefined;
    if (!this.store.merchantId) return {kind: 'payment', code: 'NO_MERCHANT'};
    try {
      this.current = confirmPolicy(this.current, this.parts.pin, await readMerchantRecord(rpc, readProfiles.stable.anchor, this.parts.pin, this.store.merchantId));
      return undefined;
    } catch (error) {
      return {kind: 'payment', code: error instanceof PaymentTrustError ? error.code : 'CHAIN_UNREADABLE'};
    }
  }

  /**
   * Confirms the policy from the chain alone: no board is asked and nothing goes to the store's server. A page that
   * comes back to a payment it kept needs the same policy that payment was checked against, before any board is near.
   */
  async confirm(): Promise<AccessProblem | undefined> {
    if (isPinnedPolicy(this.current)) return undefined;
    const wallet = await this.parts.wallet().catch(() => undefined);
    if (!wallet || !this.account || wallet.account !== this.account) return {kind: 'no-account'};
    return this.confirmBy(wallet.rpc);
  }

  /**
   * Makes sure the session of `scope` stands, asking the board for one account proof when it does not. Before a
   * checkout proof the store's merchant record is read from the order contract of this build; a store that cannot
   * be paid is said before the board is asked.
   */
  async ensure(scope: SessionScope): Promise<AccessProblem | undefined> {
    if (scope.branch !== this.store.branch) return {kind: 'unavailable'};
    if (this.live(scope)) return undefined;
    const wallet = await this.parts.wallet().catch(() => undefined);
    if (!wallet || !this.account || wallet.account !== this.account) return {kind: 'no-account'};
    if (scope.purpose === 'checkout') {
      const unpaid = await this.confirmBy(wallet.rpc);
      if (unpaid) return unpaid;
    }
    const opened = await openCustomerSession(new SessionEntrance(this.parts.post), this.parts.board, this.account, scope, wallet.generation);
    if (opened.kind !== 'session') return opened;
    const {session} = opened;
    // The server must speak of the customer this phone derived from the same account.
    if (session.subject !== this.subject) return {kind: 'unavailable'};
    this.profile.tenantRef = session.tenant;
    if (session.purpose === 'order' && session.sale) { this.orders.set(session.sale, session); await this.keep(); }
    else this.checkout = session;
    return undefined;
  }

  /** The board account's balance of the store's token at the newest block, or undefined when it cannot be read. */
  async balance(): Promise<bigint | undefined> {
    try {
      const wallet = await this.parts.wallet();
      if (!wallet || wallet.account !== this.account) return undefined;
      return BigInt((await readEvmReceiveBalance(wallet.rpc, '8283', readProfiles.stable.anchor, wallet.account, this.parts.pin.asset, false, 'latest')).amount);
    } catch {
      return undefined;
    }
  }
}

/** The board account this phone tested, and the chain server its owner chose or the build names. */
async function testedWallet(rpc?: string): Promise<Wallet | undefined> {
  const native = OwnerWallet;
  if (!native) return undefined;
  const local = decodeWalletStorage(await native.loadProfile());
  if (local.mode !== 'personal' || !local.enrollment || !local.verified) return undefined;
  const server = serverFor('stable', local.walletRead, {store: rpc}).server;
  return {account: local.verified.address, generation: String(local.verified.generation), rpc: walletReadRpc(server, (url, packet) => native.evmRpc(url, packet))};
}

/** For a reader that only follows orders with the sessions it already holds: it never asks a board. */
const noBoard: PickupBoard = {open: async () => { throw new Error('BOARD_NOT_FOUND'); }};

/**
 * The access of this build to one store, or undefined while the build has no entrance anchor or no order contract
 * bound (customerEntranceDeployment.ts). Opening it asks nothing of a board or a server. `board` is how the account
 * proof is asked for; without one the access only uses the sessions it kept.
 */
export async function openStoreAccess(store: StoreChoice, board: PickupBoard = noBoard, anchor = approvedCustomerEntrance,
  payment = approvedOrderPayment): Promise<StoreAccess | undefined> {
  const pin = orderPaymentPin(payment);
  if (!anchor || !ordersWithoutRegistration(anchor, payment) || !pin || !Http) return undefined;
  return StoreAccess.open(store, {post: nativeEntrance(anchor), origin: anchor.origin, pin, board, wallet: () => testedWallet(pin.rpc),
    now: () => Math.floor(Date.now() / 1000)});
}
