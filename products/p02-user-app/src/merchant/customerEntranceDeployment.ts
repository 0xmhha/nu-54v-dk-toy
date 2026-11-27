import type {PaymentToken} from '@nu54/protocol';

// The customer entrance of the store server (opsd P05_CUSTOMER_ADDR) and the only trust this build has in it. A
// phone orders without registering at any store, so nothing a store issues can anchor this connection:
//   origin  an https origin such as `https://store.example:8444`, answered without a client certificate
//   ca      the certificate authority that issued the entrance's certificate, DER in base64
//   pin     SHA-256 of the entrance certificate's public key (SPKI), 64 lowercase hex characters
// The native request accepts a server only when its chain ends at `ca` and its leaf carries `pin`; the system
// certificate store is never asked. It is undefined until a release binds the approved values; until then the app
// offers no store list, no order through the entrance and no pickup QR, and says so.
export type CustomerEntranceAnchor = {origin: string; ca: string; pin: string};

export const approvedCustomerEntrance: CustomerEntranceAnchor | undefined = undefined;

// Where a board's money may go from this build (use-case-review.md chapter 9, 2026-11-27): the chain, the order
// contract and the token are fixed here and never taken from a server. A store page only tells the store's number in
// that contract (`merchantId`); the app reads that merchant's quote signer and beneficiary from the contract itself
// (orderPaymentTrust.ts) and refuses a quote that names another signer, beneficiary, contract, token or chain.
//   chain     8283; its genesis block is the one this app already holds (wallet/readProfiles.ts)
//   contract  the OrderEscrow address, 0x and 40 lowercase hex characters
//   asset     the token that contract was deployed for, in the same form
//   token     how that token signs and is shown; absent means the dollar test token
//   rpc       where the app reads the chain unless its owner chose another server
// The field names are those of a store's payment profile, so a local deployment feeds both from one record. It is
// undefined until a release binds the approved values; until then the app takes no order through the entrance.
export type OrderPaymentPin = {chain: 8283; contract: string; asset: string; token?: PaymentToken; rpc?: string};

export const approvedOrderPayment: OrderPaymentPin | undefined = undefined;
