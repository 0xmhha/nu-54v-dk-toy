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
