# 08 Settlement v2 scope and acceptance

[Quote schema](../../packages/protocol/v2/schema.json) · [Ledger](../../docs/content/commercial/architecture/ledger.md) · [Pilot payment behavior](../../docs/content/commercial/merchant-payments.md)

Use a new deployment on chain 8283 with one pinned TestUSDC contract. The v1 contract, addresses, ABI, renter withdrawal and cash-out remain on their existing paths. The new contract accepts one merchant-signed SDK v2 `MerchantQuote` and one independently payer-signed `PaymentApproval(bytes32 quoteDigest)` in the same v2 domain. The payer signs the approval digest after showing the amount, registered merchant and asset. Both signatures use low-s secp256k1 with recovery byte 27 or 28. A quote signature for another contract, chain or version and a merchant signature replayed as an approval are refused.

## Storage and effects

- Merchant registration is scoped by the public `merchantId`. Verify active status, registered signing address and key version, registered beneficiary and beneficiary version, and the immutable approved asset. Authority changes monotonically advance the affected versions. On-chain quote fields contain randomized quote/order IDs and keyed per-quote commitments from P05. The contract hashes those commitments verbatim and never resolves or stores private IDs, claims or order data.
- Payer registration supplies the active role and authority generation for an escrow account or approved owner. Compare the signed role and generation. Escrow mode spends separate v2 prefunded credit. Direct mode requires the owner to submit its own transaction and set the contract allowance to exactly the quote amount for a single exact debit. Direct remains a separate test profile and the SDK's pilot profile rejects it.
- Compare issue and expiry against inclusion time with `issuedAt <= block.timestamp < expiresAt`. Consume the merchant/order, merchant/nonce and payer/nonce atomically with funds movement. P05 reserves signer nonce by deployment and merchant to match the on-chain merchant nonce. The payer scope independently prevents signed approval replay across merchants. A failed transfer leaves both markers, escrow, refund funds and token state unchanged.
- Before and after transfer, check that the source spendable balance falls by the exact amount and the registered beneficiary balance rises by the exact amount. A revert, false token response, fee/rebase delta or held transfer undoes the transaction. Escrow payments directly credit the beneficiary without a merchant payable. All token-changing functions share a reentrancy guard.
- The merchant funds a separate refund reserve. Refund approval uses an independent registered refund signer and a domain-bound type with merchant ID, order ID, refund ID, amount, key version and expiry. Resolve the destination from the original payment. An escrow payment credits its original v2 escrow account, and a direct payment returns to the original payer. The EVM orders cumulative refund and reserve updates within the same transaction. A distinct refund ID may spend only the remaining paid amount. Failed transfers leave the refund ID, reserve and cumulative total unchanged.
- Emit one `Paid` or `Refunded` event after its effects pass; carry public IDs, addresses, exact units and original authority context needed for P07 reconciliation. ABI and event vectors are versioned separately from v1.

## Registered address and key cutover

A current-version quote needs one settlement transaction. At rotation, P05 closes issuance for the merchant, reconciles the current version and snapshots the committed, still-open quote digests. The governed rotation publishes those exact digests in the same transaction that closes the old registration interval and opens the next one. The rotation accepts up to 256 live digests. If more are pending, P05 keeps issuance paused and waits for enough quotes to settle or expire before submitting a new snapshot. The old interval ends at the cutover block's timestamp. A historical version is usable only for a snapshotted digest issued at or before that timestamp, inside its registration window, and included before the signed expiry. A digest attested before cutover can settle when its whole-second issuance timestamp equals the cutover timestamp. A digest absent from the snapshot cannot use the old version even if its timestamp is equal or earlier. After confirmation P05 resumes issuance using the newly registered versions. The digest binds beneficiary, both versions, order and private commitments. A signed backdated quote missing from the snapshot cannot use an old registration. A permanent emergency key revocation immediately refuses every quote under that key, including snapshotted digests. Refunds already owed have their own signer and reserve.

P05 owns the authenticated open-order snapshot and freezes quote publication while preparing it. The settlement contract enforces the sealed digest set and registration interval. Reconcile the confirmed rotation transaction before reflecting the new beneficiary in P05. No historical authorization is inferred from a client-provided issuance timestamp.

## Acceptance

1. Compare Solidity EIP-712 digest to the shared SDK fixture. Verify the two signatures independently, including old domain, wrong asset or beneficiary, invalid key and beneficiary versions, stale renter generation, role/mode, expiry boundaries, same order with a fresh nonce and same nonce with a different order.
2. On a separate funded v2 deployment, escrow pays amount 30 with beneficiary +30 and payer credit −30. Direct mode requires owner caller and exact owner debit; insufficient allowance leaves all state and balances unchanged.
3. Original amount 30 permits refunds of 10 and then 20. An additional 1 fails without consuming the refund ID. Test recovery after a held or false return, duplicate refund ID and reserve insufficiency.
4. Exercise a held payment on the real TestUSDC test contract and false and fee-on-transfer behavior in test tokens. Compare token balances, held-transfer count, receipt markers, order, nonce, and reserve before and after failure. The same valid signatures remain usable after removing the failure condition.
5. Run the existing v1 suites including cash-out and rental withdrawal and compile an ABI consumer using both deployment versions.

## ABI and fixture

Use the separate [v2 ABI](../../packages/contracts-abi/abi/SettlementV2.json), the Go `settlementv2` binding, or the TypeScript `settlementV2Abi` export. Keep the release manifest bound to the correct ABI digest. Entries for the earlier settlement and registry retain their committed hashes. The [loopback Anvil receipt fixtures](../../packages/contracts-abi/abi/vectors/settlement-v2.json) record a failed held payment, an exact payment of 30, refunds of 10 and 20, and a refused additional 1.

With Foundry 1.5.1 on `PATH`, regenerate the receipt samples from the repository root:

```sh
sh packages/contracts-abi/tools/v2fixture/run.sh
```

The harness starts a fresh loopback-only Anvil chain 8283, uses synthetic fixtures and exits. The address fields and receipts in its output describe that local chain.
