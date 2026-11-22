package commerce

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
)

const presentationDomain = "NU54-private-presentation-v1\x00"

type InvoicePresentation struct {
	Cycle        string `json:"cycle"`
	TermsVersion string `json:"termsVersion"`
	Signature    string `json:"signature"`
}

// GasSponsorLookup resolves the live, enrolled merchant settlement authority for a branch.
type GasSponsorLookup func(context.Context, string, Profile, common.Address, time.Time) (GasSponsorship, bool)

type GasSponsorship struct {
	Account   common.Address
	Grant     [32]byte
	ExpiresAt int64
	// CurrentPublication checks only the bounded local grant snapshot.
	// The deployment RPC is completed by the lookup outside any DB transaction.
	CurrentPublication func(time.Time) bool
}

type RentalEscrowAuthority interface {
	Deployment(context.Context, string) ([]byte, bool)
	Check(context.Context, string, common.Address, uint64, *big.Int) ([]byte, time.Time, bool)
}

type RentalEscrowPresentation struct {
	ExpiresAt int64  `json:"expiresAt"`
	Signature string `json:"signature"`
}

type GasSponsorPresentation struct {
	Account   string `json:"account"`
	ExpiresAt int64  `json:"expiresAt"`
	Grant     string `json:"grant"`
	Signature string `json:"signature"`
}

type Presentation struct {
	RentalEscrow *RentalEscrowPresentation `json:"rentalEscrow,omitempty"`
	RentalReview *RentalReviewPresentation `json:"rentalReview,omitempty"`
	GasSponsor   *GasSponsorPresentation   `json:"gasSponsor,omitempty"`
	Invoice      *InvoicePresentation      `json:"invoice,omitempty"`
	Customer     string                    `json:"customer"`
	Origin       string                    `json:"origin"`
	Claim        string                    `json:"claim"`
	Signature    string                    `json:"signature"`
}

func presentationMessage(branch, sale, customer, origin string, claim, orderKey, digest [32]byte, generation, expiry uint64) ([]byte, error) {
	body := []byte(presentationDomain)
	for _, ref := range []string{branch, sale, customer, origin} {
		value, err := hex.DecodeString(strings.ReplaceAll(ref, "-", ""))
		if err != nil || len(value) != 16 {
			return nil, service.BadRequest
		}
		body = append(body, value...)
	}
	body = append(body, claim[:]...)
	body = append(body, orderKey[:]...)
	body = append(body, digest[:]...)
	body = binary.BigEndian.AppendUint64(body, generation)
	body = binary.BigEndian.AppendUint64(body, expiry)
	return body, nil
}

func gasSponsorMessage(presentation, signed []byte, witness GasSponsorship) []byte {
	scopeHash := sha256.Sum256(presentation)
	wireHash := sha256.Sum256(signed)
	body := append([]byte("NU54-merchant-gas-sponsor-v1\x00"), scopeHash[:]...)
	body = append(body, wireHash[:]...)
	body = append(body, witness.Account[:]...)
	body = append(body, witness.Grant[:]...)
	return binary.BigEndian.AppendUint64(body, uint64(witness.ExpiresAt))
}

func (h HTTP) present(ctx context.Context, bound service.Bound, ready Ready, profile Profile, digest [32]byte, signed []byte, review *RentalReviewChallenge) (*Presentation, error) {
	if len(h.PresentationKey) != ed25519.PrivateKeySize {
		return nil, service.Capability
	}
	var result Presentation
	err := h.Store.db.QueryRow(ctx, `SELECT customer_ref::text,origin_ref::text
    FROM ledger.private_quote_context($1,$2,$3,$4,$5)`,
		bound.Tenant, bound.Request.Domain, bound.Subject, bound.Request.Object, ready.Claim[:]).Scan(&result.Customer, &result.Origin)
	if err != nil {
		return nil, domainError(err)
	}
	// Quote context identifies the actual payer even when the quote is relayed.
	// Use the existing customer-scoped order projection for the immutable invoice.
	customer := bound
	customer.Subject = result.Customer
	order, err := h.Store.Status(ctx, customer)
	if err != nil {
		return nil, err
	}
	body, err := presentationMessage(bound.Request.Domain, bound.Request.Object, result.Customer, result.Origin, ready.Claim,
		ready.OrderID, digest, ready.PayerGeneration, uint64(ready.Expires.Unix()))
	if err != nil {
		return nil, err
	}
	// Existing readers validate this signature over the original scope.
	result.Signature = hex.EncodeToString(ed25519.Sign(h.PresentationKey, body))
	var sponsorshipCurrent func(time.Time) bool
	if h.GasSponsors != nil {
		expected, err := BuildUnsigned(ready.QuoteOrder, profile)
		if err == nil && len(signed) >= len(expected) && bytes.Equal(expected, signed[:len(expected)]) {
			sponsorshipCurrent = h.presentGasSponsor(ctx, bound, profile, ready, digest, body, signed, &result)
		}
	}
	if bound.Role == "customer" && order.CafeReview != nil && ready.PayerRole == 2 && h.RentalEscrow != nil {
		h.presentRentalEscrow(ctx, bound, profile, ready, digest, body, signed, &result, review, sponsorshipCurrent)
	}
	if invoice := order.Subscription; invoice != nil {
		cycle, cycleErr := time.Parse(time.DateOnly, invoice.Cycle)
		if invoice.InvoiceRef != result.Origin || cycleErr != nil || cycle.Format(time.DateOnly) != invoice.Cycle ||
			len(invoice.TermsVersion) == 0 || len(invoice.TermsVersion) > 160 {
			return nil, service.Mismatch
		}
		result.Invoice = &InvoicePresentation{Cycle: invoice.Cycle, TermsVersion: invoice.TermsVersion}
		// The invoice version, ISO date, length-prefixed terms and wire hash extend that scope.
		body = append(body, 1)
		body = append(body, invoice.Cycle...)
		body = binary.BigEndian.AppendUint16(body, uint16(len(invoice.TermsVersion)))
		body = append(body, invoice.TermsVersion...)
		wireHash := sha256.Sum256(signed)
		body = append(body, wireHash[:]...)
		result.Invoice.Signature = hex.EncodeToString(ed25519.Sign(h.PresentationKey, body))
	}
	if time.Now().Unix() >= ready.Expires.Unix() {
		return nil, service.Expired
	}
	result.Claim = hex.EncodeToString(ready.Claim[:])
	return &result, nil
}

func (h HTTP) presentGasSponsor(ctx context.Context, bound service.Bound, profile Profile, ready Ready,
	quote [32]byte, scope, signed []byte, present *Presentation) func(time.Time) bool {
	now := time.Now()
	witness, ok := h.GasSponsors(ctx, bound.Request.Domain, profile, ready.Payer, now)
	witness.ExpiresAt = min(witness.ExpiresAt, ready.Expires.Unix(), now.Add(30*time.Second).Unix())
	if !ok || witness.Account == (common.Address{}) || witness.Account == ready.Payer ||
		witness.Grant == ([32]byte{}) || time.Now().Unix() >= witness.ExpiresAt {
		return nil
	}
	var pinned bool
	if err := h.Store.db.QueryRow(ctx, "SELECT ledger.pin_merchant_gas_sponsor($1,$2,$3,$4,$5,$6,$7)",
		bound.Tenant, bound.Request.Domain, bound.Subject, bound.Request.Object, ready.Claim[:], quote[:], witness.Grant[:],
	).Scan(&pinned); err != nil || !pinned || time.Now().Unix() >= witness.ExpiresAt {
		return nil
	}
	present.GasSponsor = &GasSponsorPresentation{
		Account: strings.ToLower(witness.Account.Hex()), ExpiresAt: witness.ExpiresAt,
		Grant:     hex.EncodeToString(witness.Grant[:]),
		Signature: hex.EncodeToString(ed25519.Sign(h.PresentationKey, gasSponsorMessage(scope, signed, witness))),
	}
	return witness.CurrentPublication
}

func (h HTTP) presentRentalEscrow(ctx context.Context, bound service.Bound, profile Profile, ready Ready,
	quote [32]byte, scope, signed []byte, present *Presentation, review *RentalReviewChallenge, sponsorshipCurrent func(time.Time) bool) {
	expected, err := BuildUnsigned(ready.QuoteOrder, profile)
	amount, ok := new(big.Int).SetString(ready.Amount, 10)
	if err != nil || !ok || len(signed) < len(expected) || !bytes.Equal(expected, signed[:len(expected)]) {
		return
	}
	var valid bool
	var ends *time.Time
	err = h.Store.db.QueryRow(ctx, `SELECT * FROM ledger.renter_payment_scope($1,$2,$3,$4,$5,NULL,false)`,
		bound.Tenant, bound.Request.Domain, bound.Subject, bound.Request.Object, quote[:]).Scan(&valid, &ends)
	if err != nil || !valid {
		return
	}
	manifest, chainUntil, ok := h.RentalEscrow.Check(ctx, bound.Request.Domain, ready.Payer, ready.PayerGeneration, amount)
	if !ok {
		return
	}
	var commissioned RentalReviewCommissioning
	var reviewAvailable bool
	if review != nil && h.RentalReview != nil && present.GasSponsor != nil && sponsorshipCurrent != nil {
		commissioned, reviewAvailable = h.RentalReview(ctx, bound.Request.Domain, profile, manifest, ready.Payer, ready.PayerGeneration, amount)
	}
	err = h.Store.db.QueryRow(ctx, `SELECT * FROM ledger.renter_payment_scope($1,$2,$3,$4,$5,$6,false)`,
		bound.Tenant, bound.Request.Domain, bound.Subject, bound.Request.Object, quote[:], manifest).Scan(&valid, &ends)
	if err != nil || !valid || ends == nil {
		return
	}
	present.RentalEscrow = signRentalEscrowProof(h.PresentationKey, scope, signed, time.Now(), *ends, chainUntil, ready.Expires)
	if reviewAvailable && present.RentalEscrow != nil && sponsorshipCurrent(time.Now()) {
		present.RentalReview = signRentalReview(h.PresentationKey, scope, signed, time.Now(),
			min(present.RentalEscrow.ExpiresAt, present.GasSponsor.ExpiresAt), *review, commissioned)
	}
}

func signRentalEscrowProof(key ed25519.PrivateKey, scope, signed []byte, now, authorityUntil, chainUntil, quoteUntil time.Time) *RentalEscrowPresentation {
	until := min(authorityUntil.Unix(), chainUntil.Unix(), quoteUntil.Unix())
	if until <= now.Unix() {
		return nil
	}
	digest := sha256.Sum256(scope)
	wire := sha256.Sum256(signed)
	message := append([]byte("NU54-rental-payment-escrow-v1\x00"), digest[:]...)
	message = append(message, wire[:]...)
	message = binary.BigEndian.AppendUint64(message, uint64(until))
	return &RentalEscrowPresentation{
		ExpiresAt: until, Signature: hex.EncodeToString(ed25519.Sign(key, message)),
	}
}
