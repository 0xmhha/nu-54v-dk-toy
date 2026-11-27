// The customer entrance of the store server (opsd P05_CUSTOMER_ADDR): an https origin such as
// `https://store.example:8444`, answered without a client certificate. It is empty until a release binds the
// approved address; while it is empty the app offers no pickup QR and says so.
export const approvedCustomerEntranceOrigin: string = '';
