package commerce

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/networklist"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/quotev2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

type PaymentForwarder interface {
	AuthorizeForward(context.Context, service.Request, service.Principal, []byte) (*service.OrderApproval, error)
	Forward(context.Context, service.Bound, []byte) (string, error)
}

// BranchPauser knows the branches whose deployment or token was refused at start. A PaymentForwarder may
// provide it. Such a branch takes no new order, quote, price or POS order.
type BranchPauser interface {
	Paused(branch string) bool
}

func (h HTTP) paused(branch string) bool {
	pauser, ok := h.Payment.(BranchPauser)
	return ok && pauser.Paused(branch)
}

// OrderLocker pays an order into OrderEscrow. A PaymentForwarder may provide it.
type OrderLocker interface {
	AuthorizeLock(context.Context, service.Request, service.Principal, []byte) (*service.OrderApproval, error)
	Lock(context.Context, service.Bound, []byte) (string, error)
	EscrowStatus(context.Context, service.Bound, Order) (*OrderEscrow, error)
	// Answer stores the buyer's receipt, dispute or its withdrawal.
	Answer(context.Context, service.Bound, []byte) (string, error)
}

// PayerAccounts enrolls the customer's board account and returns its balance. A PaymentForwarder may provide it.
type PayerAccounts interface {
	EnrollPayer(context.Context, service.Bound, []byte) error
	EnrolledPayer(context.Context, service.Bound) (*PayerEnrollment, error)
	ReturnWallet(context.Context, service.Bound, []byte) error
	WalletReturnStatus(context.Context, service.Bound) (*WalletReturn, error)
	SendWallet(context.Context, service.Bound, []byte) error
	WalletSends(context.Context, service.Bound) (*WalletSends, error)
}

// Faucet sends test tokens to the enrolled account. A PaymentForwarder may provide it.
type Faucet interface {
	ClaimFaucet(context.Context, service.Bound) error
	FaucetStatus(context.Context, service.Bound) (*FaucetStatus, error)
}

type HTTP struct {
	QuoteAdmission            chan struct{}
	Store                     *Store
	Credentials               map[string]service.Principal // authorized certificate DER SHA-256 to authenticated authority
	Profiles                  map[string]Profile           // enrolled opaque branch reference to verified deployment profile
	Signer                    Signer
	Cipher                    cipher.AEAD
	Payment                   PaymentForwarder
	PresentationKey           ed25519.PrivateKey
	GasSponsors               GasSponsorLookup
	RentalEscrow              RentalEscrowAuthority
	RentalReview              RentalReviewLookup
	OperatorOrigin            string
	Rental                    RentalControl
	ProtectedClose            *ProtectedRentalClose
	ProtectedWithdrawal       bool
	RenterEvidence            *RenterReturnEvidence
	renterCredentialCollector *renterCredentialCollector
	renterSlotOpenCollector   *renterSlotOpenCollector
	RenterV1Origins           *RenterV1Origins
	Legacy                    LegacyConsole
	Firmware                  map[string]FirmwareRelease
	NetworkList               *NetworkList
}

type request struct {
	POS               json.RawMessage        `json:"pos,omitempty"`
	Travel            json.RawMessage        `json:"travel,omitempty"`
	Command           json.RawMessage        `json:"command"`
	Cart              *Cart                  `json:"cart,omitempty"`
	ApprovalSignature string                 `json:"approvalSignature,omitempty"`
	RentalReview      *RentalReviewChallenge `json:"rentalReview,omitempty"`
	TerminalValue     *string                `json:"terminalValue,omitempty"`
}

type result struct {
	service.Reply
	Shop             *ShopIdentity          `json:"shop,omitempty"`
	Catalog          []Price                `json:"catalog,omitempty"`
	CatalogVersion   *int64                 `json:"catalogVersion,omitempty"`
	Orders           []CustomerOrderSummary `json:"orders,omitempty"`
	Order            *Order                 `json:"order,omitempty"`
	Quote            *quoteReply            `json:"quote,omitempty"`
	Terminal         *TerminalSettings      `json:"terminal,omitempty"`
	Staff            *StaffAccess           `json:"staff,omitempty"`
	TerminalOrders   []string               `json:"terminalOrders,omitempty"`
	Comparison       *Comparison            `json:"comparison,omitempty"`
	Comparisons      []Comparison           `json:"comparisons,omitempty"`
	Claim            *Claim                 `json:"claim,omitempty"`
	Offers           []SubscriptionOffer    `json:"offers,omitempty"`
	Subscriptions    []Subscription         `json:"subscriptions,omitempty"`
	Invoices         []Invoice              `json:"invoices,omitempty"`
	ReservationFound bool                   `json:"reservationFound,omitempty"`
	Unavailable      []Item                 `json:"unavailable,omitempty"`
	Payer            *PayerEnrollment       `json:"payer,omitempty"`
	WalletReturn     *WalletReturn          `json:"walletReturn,omitempty"`
	WalletSends      *WalletSends           `json:"walletSends,omitempty"`
	Faucet           *FaucetStatus          `json:"faucet,omitempty"`
	NetworkList      *networklist.Envelope  `json:"networkList,omitempty"`
	TerminalDevice   *TerminalDevice        `json:"terminalDevice,omitempty"`
}

type quoteReply struct {
	SaleRef      string        `json:"saleRef"`
	BranchRef    string        `json:"branchRef"`
	OrderKey     string        `json:"orderKey"`
	Signed       string        `json:"signedQuote"`
	Digest       string        `json:"quoteDigest"`
	ExpiresAt    int64         `json:"expiresAt"`
	Presentation *Presentation `json:"presentation,omitempty"`
}

var sku = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

func checkCart(bound service.Bound, cart *Cart) error {
	if cart == nil || cart.OriginRef != bound.Request.Object || cart.MenuVersion < 1 ||
		len(cart.Items) < 1 || len(cart.Items) > 20 || (cart.Origin != "menu" && cart.Origin != "pos") {
		return service.BadRequest
	}
	if cart.Pickup != nil {
		parsed, err := time.Parse(time.RFC3339Nano, cart.Pickup.StartsAt)
		if bound.Role != "customer" || cart.Origin != "menu" || !travelUUID.MatchString(cart.Pickup.Window) ||
			err != nil || parsed.Nanosecond()%1000 != 0 || parsed.UTC().Format(time.RFC3339Nano) != cart.Pickup.StartsAt {
			return service.BadRequest
		}
	}

	if cart.Nearby != nil && (bound.Role != "terminal" || cart.Origin != "menu" ||
		cart.Pickup != nil || !travelUUID.MatchString(cart.Nearby.Customer) || cart.Nearby.ExpiresAt <= 0) {
		return service.BadRequest
	}

	seen := map[string]bool{}
	var quantity int64
	for _, line := range cart.Items {
		identity := line.SKU + "/" + line.Option
		if !sku.MatchString(line.SKU) || !sku.MatchString(line.Option) || line.Qty < 1 || line.Qty > 1000 || seen[identity] {
			return service.BadRequest
		}
		seen[identity] = true
		quantity += line.Qty
	}
	slices.SortFunc(cart.Items, func(a, b Item) int {
		if a.SKU < b.SKU {
			return -1
		}
		if a.SKU > b.SKU {
			return 1
		}
		return strings.Compare(a.Option, b.Option)
	})
	payload, err := json.Marshal(cart)
	if err != nil {
		return service.BadRequest
	}
	digest := sha256.Sum256(payload)
	if bound.Request.Body["intentDigest"] != hex.EncodeToString(digest[:]) ||
		bound.Request.Body["quantity"] != strconv.FormatInt(quantity, 10) {
		return service.Conflict
	}
	return nil
}

func (h HTTP) Register(mux *http.ServeMux) {
	mux.Handle("POST /v2/commerce", authority.Protect(h.Credentials, http.HandlerFunc(h.serve)))
	operators := http.NewServeMux()
	h.registerOperators(operators)
	h.registerPayout(operators)
	h.registerRental(operators)
	if h.ProtectedClose != nil && h.ProtectedWithdrawal {
		mux.HandleFunc("POST /v2/private/rental-cleanup/{branch}/{device}/{generation}/{key}/issue", h.rentalCleanupIssue)
		mux.HandleFunc("POST /v2/private/rental-cleanup/{branch}/{device}/{generation}/{key}/receipt", h.rentalCleanupReceipt)
		mux.HandleFunc("GET /v2/private/rental-cleanup/{branch}/{device}/{generation}/{key}/receipt", h.rentalCleanupReceipt)
	}
	h.registerRenterStatus(mux, operators)
	if h.RenterV1Origins != nil {
		origin := http.NewServeMux()
		origin.HandleFunc("POST /v2/private/renter-v1-orders/reserve", h.renterV1Origin)
		origin.HandleFunc("POST /v2/private/renter-v1-orders/confirm", h.renterV1Origin)
		mux.Handle("/v2/private/renter-v1-orders/", authority.Protect(h.Credentials, origin))
	}
	h.registerExperience(operators)
	h.registerAccess(operators)
	h.registerLegacy(operators)
	h.registerFirmware(operators)
	mux.Handle("/v2/operations/", authority.Protect(h.Credentials, operators))
}

func (h HTTP) serve(w http.ResponseWriter, r *http.Request) {
	principal, _, exists := service.CurrentClientPrincipal(r.TLS, h.Credentials, time.Now())
	if !exists {
		http.NotFound(w, r)
		return
	}
	var outer request
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&outer); err != nil {
		errorResponse(w, service.BadRequest, nil)
		return
	}
	if _, err := decoder.Token(); err != io.EOF {
		errorResponse(w, service.BadRequest, nil)
		return
	}
	if outer.POS != nil {
		if outer.Command != nil || outer.Cart != nil || outer.Travel != nil || outer.ApprovalSignature != "" || outer.RentalReview != nil || outer.TerminalValue != nil {
			errorResponse(w, service.BadRequest, nil)
			return
		}
		h.pos(w, r, principal, outer.POS)
		return
	}
	if outer.Travel != nil {
		if outer.Command != nil || outer.Cart != nil || outer.ApprovalSignature != "" || outer.RentalReview != nil || outer.TerminalValue != nil {
			travelFailure(w, http.StatusNotFound, "booking_unavailable")
			return
		}
		h.travel(w, r, principal, outer.Travel)
		return
	}
	req, err := service.DecodeRequest(bytes.NewReader(outer.Command))
	if err != nil || req.Contract != "C3" || (outer.Cart != nil && req.Operation != "order.create") || (outer.ApprovalSignature != "" &&
		!slices.Contains([]string{"payment.forward", "order.lock", "order.answer", "payer.enroll", "wallet.return", "wallet.send"}, req.Operation)) {
		errorResponse(w, service.BadRequest, nil)
		return
	}
	if (outer.TerminalValue != nil) != (req.Operation == "terminal.device-change") {
		errorResponse(w, service.BadRequest, nil)
		return
	}
	if outer.RentalReview != nil && (req.Operation != "quote.issue" || principal.Role != "customer" || !outer.RentalReview.valid()) {
		errorResponse(w, service.BadRequest, nil)
		return
	}
	if outer.Cart != nil && outer.Cart.Origin == "pos" || principal.Role == "pos" {
		http.NotFound(w, r)
		return
	}
	// Validate C3 fields and the authenticated principal before consulting the
	// owner database. This exact-scope candidate has no authority until Scope
	// confirms it against the branch/catalog/order/claim at the owner.
	principal.Grants = []service.Grant{{Domain: req.Domain, Object: req.Object, Operations: []string{"C3/" + req.Operation}}}
	var approvalSignature []byte
	locker, _ := h.Payment.(OrderLocker)
	accounts, _ := h.Payment.(PayerAccounts)
	faucet, _ := h.Payment.(Faucet)
	if strings.HasPrefix(req.Operation, "faucet.") && faucet == nil {
		errorResponse(w, service.Capability, nil)
		return
	}
	if strings.HasPrefix(req.Operation, "payer.") || strings.HasPrefix(req.Operation, "wallet.") {
		if accounts == nil {
			errorResponse(w, service.Capability, nil)
			return
		}
	}
	if slices.Contains([]string{"order.answer", "payer.enroll", "wallet.return", "wallet.send"}, req.Operation) {
		if req.Operation == "order.answer" && locker == nil {
			errorResponse(w, service.Capability, nil)
			return
		}
		approvalSignature, err = hex.DecodeString(outer.ApprovalSignature)
		if err != nil || len(approvalSignature) != 65 || hex.EncodeToString(approvalSignature) != outer.ApprovalSignature {
			errorResponse(w, service.BadRequest, nil)
			return
		}
	}
	if req.Operation == "payment.forward" || req.Operation == "order.lock" {
		authorize := func(ctx context.Context) (*service.OrderApproval, error) {
			return h.Payment.AuthorizeForward(ctx, req, principal, approvalSignature)
		}
		if req.Operation == "order.lock" {
			authorize = func(ctx context.Context) (*service.OrderApproval, error) {
				return locker.AuthorizeLock(ctx, req, principal, approvalSignature)
			}
		}
		if h.Payment == nil || (req.Operation == "order.lock" && locker == nil) {
			errorResponse(w, service.Capability, nil)
			return
		}
		approvalSignature, err = hex.DecodeString(outer.ApprovalSignature)
		if err != nil || len(approvalSignature) != 65 || hex.EncodeToString(approvalSignature) != outer.ApprovalSignature {
			errorResponse(w, service.BadRequest, nil)
			return
		}
		// Order authority comes from the issuer's archived bytes and current
		// branch scope, never from copying the two request digest strings.
		verify, cancel := context.WithTimeout(authority.Scope(r.Context(), principal, req.Domain), 4*time.Second)
		principal.Order, err = authorize(verify)
		cancel()
		if err != nil {
			errorResponse(w, err, nil)
			return
		}
	}
	bound, err := service.Authorize(req, principal, time.Now())
	if err != nil {
		errorResponse(w, err, nil)
		return
	}
	if req.Operation == "order.create" && h.paused(req.Domain) {
		errorResponse(w, service.Capability, &bound)
		return
	}
	ctx, cancel := context.WithDeadline(authority.Scope(r.Context(), principal, req.Domain), time.Unix(req.ExpiresAt, 0))
	defer cancel()
	if slices.Contains([]string{"catalog.read", "order.create", "checkout.authorize", "quote.issue"}, req.Operation) {
		allowed, err := h.Store.Scope(ctx, req, principal)
		if err != nil || !allowed {
			if err == nil {
				err = service.Forbidden
			}
			errorResponse(w, err, &bound)
			return
		}
	}
	out := result{Reply: service.Outcome(bound, "ok")}
	out.ResultRef = req.Object
	switch req.Operation {
	case "staff.access":
		var staff StaffAccess
		// The SQL function locks and checks the permissions for each individual
		// role. The authenticated request boundary still rechecks the certificate.
		unscoped, finish := context.WithDeadline(r.Context(), time.Unix(req.ExpiresAt, 0))
		if principal.Role == "terminal" {
			unscoped = authority.Scope(unscoped, principal, req.Domain, "pickup_assignment")
		}
		staff, err = h.Store.StaffAccess(unscoped, bound)
		if err == nil && principal.Role == "terminal" {
			err = h.Store.db.CheckRead(unscoped)
		}
		finish()
		if err == nil && slices.Contains(staff.Permissions, "refund_console") {
			staff.Console = h.OperatorOrigin
		}
		out.Staff = &staff
	case "terminal.orders":
		var revision int64
		revision, err = strconv.ParseInt(req.Body["sourceRevision"], 10, 64)
		if err != nil {
			err = service.BadRequest
			break
		}
		out.TerminalOrders, err = h.Store.TerminalOrdersPage(ctx, bound, revision)
	case "terminal.state":
		var terminal TerminalSettings
		terminal, err = h.Store.TerminalState(ctx, bound)
		if err == nil && principal.Role == "terminal" {
			_, err = h.Store.db.Exec(ctx, "SELECT ledger.record_operator_access_ack($1,$2,$3)",
				bound.Tenant, bound.Request.Domain, bound.Subject)
		}
		out.Terminal = &terminal
	case "terminal.mode":
		var revision int64
		revision, err = strconv.ParseInt(req.Body["expectedRevision"], 10, 64)
		if err != nil {
			err = service.BadRequest
			break
		}
		var terminal TerminalSettings
		terminal, err = h.Store.ChangeTerminalMode(ctx, bound, revision)
		out.Terminal = &terminal
	case "terminal.handoff", "terminal.ack":
		var revision int64
		revision, err = strconv.ParseInt(req.Body["targetRevision"], 10, 64)
		if err != nil || req.IdempotencyKey != req.Body["transitionRef"] {
			err = service.BadRequest
			break
		}
		var terminal TerminalSettings
		terminal, err = h.Store.ConfirmTerminalTransition(ctx, bound, revision)
		out.Terminal = &terminal
	case "terminal.device":
		out.TerminalDevice, err = h.terminalDevice(ctx, bound)
	case "terminal.device-change":
		out.TerminalDevice, err = h.changeTerminalDevice(ctx, bound, *outer.TerminalValue)
	case "network.list":
		out.NetworkList, err = h.networkList(ctx, bound)
	case "catalog.read":
		var page int64
		page, err = strconv.ParseInt(req.Body["page"], 10, 32)
		if err != nil {
			err = service.BadRequest
			break
		}
		out.Catalog, out.CatalogVersion, err = h.Store.catalogWindow(ctx, bound, int32(page))
		if err == nil && page == 0 {
			out.Shop, err = h.Store.ShopIdentity(ctx, bound)
		}
	case "order.create":
		if err = checkCart(bound, outer.Cart); err == nil {
			out.ResultRef, err = h.Store.Reserve(ctx, bound, *outer.Cart)
		}
	case "order.nearby-result":
		byOrder := req.Body["byOrder"]
		if byOrder != "0" && byOrder != "1" {
			err = service.BadRequest
			break
		}
		var sale string
		sale, err = h.Store.NearbyReservation(ctx, bound, byOrder == "1")
		if sale != "" {
			out.ResultRef, out.ReservationFound = sale, true
		}
	case "order.cancel-public":
		err = h.Store.CancelPublicOrder(ctx, bound)
	case "order.list":
		if req.Object != bound.Subject {
			err = service.Forbidden
			break
		}
		out.Orders, err = h.Store.Orders(ctx, bound, req.Body["afterSale"])
		out.ResultRef = bound.Subject
	case "order.status":
		var order Order
		order, err = h.status(ctx, bound, locker)
		out.Order = &order
	case "checkout.request":
		var item Comparison
		item, err = h.Store.RequestComparison(ctx, bound)
		out.Comparison = &item
	case "checkout.pos-nearby":
		out.Comparison, out.Claim, err = h.Store.POSNearbyCheckout(ctx, bound)
	case "checkout.pending":
		out.Comparisons, err = h.Store.PendingComparisons(ctx, bound)
	case "checkout.confirm":
		var item Claim
		item, err = h.Store.ConfirmComparison(ctx, bound)
		out.Claim = &item
	case "checkout.claim":
		out.Claim, err = h.Store.ReadClaim(ctx, bound)
	case "checkout.authorize":
		var claim Claim
		claim, err = h.Store.AuthorizeCheckout(ctx, bound)
		out.Claim = &claim
	case "quote.issue":
		out.Quote, err = h.issueWithReview(ctx, bound, outer.RentalReview)
	case "subscription.offers", "subscription.list", "invoice.list", "invoice.skip", "subscription.accept", "subscription.pause", "subscription.resume",
		"subscription.cancel", "subscription.reconsent":
		err = h.Store.subscription(ctx, bound, &out)
	case "payment.forward":
		out.ResultRef, err = h.Payment.Forward(ctx, bound, approvalSignature)
	case "order.lock":
		out.ResultRef, err = locker.Lock(ctx, bound, approvalSignature)
	case "payer.enroll":
		err = accounts.EnrollPayer(ctx, bound, approvalSignature)
	case "payer.status":
		out.Payer, err = accounts.EnrolledPayer(ctx, bound)
	case "wallet.return":
		err = accounts.ReturnWallet(ctx, bound, approvalSignature)
	case "wallet.return-status":
		out.WalletReturn, err = accounts.WalletReturnStatus(ctx, bound)
	case "wallet.send":
		err = accounts.SendWallet(ctx, bound, approvalSignature)
	case "wallet.send-status":
		out.WalletSends, err = accounts.WalletSends(ctx, bound)
	case "faucet.claim":
		err = faucet.ClaimFaucet(ctx, bound)
	case "faucet.status":
		out.Faucet, err = faucet.FaucetStatus(ctx, bound)
	case "order.answer":
		out.ResultRef, err = locker.Answer(ctx, bound, approvalSignature)
	default:
		err = service.Capability
	}
	if err == nil && time.Now().Unix() >= req.ExpiresAt {
		err = service.Unknown
	}
	if err == nil && slices.Contains([]string{"catalog.read", "order.list", "order.status", "order.nearby-result", "checkout.pending", "checkout.pos-nearby", "subscription.offers", "subscription.list", "invoice.list",
		"payer.status", "wallet.return-status", "wallet.send-status", "faucet.status", "network.list", "terminal.device"}, req.Operation) {
		err = h.Store.db.CheckRead(ctx)
	}
	if err == nil && req.Operation == "order.list" {
		err = currentOrderPage(out.Orders, time.Now())
	}
	if err == nil && req.Operation == "order.status" && out.Order.accessEndsAt != nil &&
		!time.Now().Before(*out.Order.accessEndsAt) {
		err = service.Forbidden
	}
	if err != nil {
		if req.Operation == "checkout.pos-nearby" && errors.Is(err, service.Forbidden) {
			// All failed nearby scopes share one response. The terminal can stop
			// polling without learning whether the POS order or customer exists.
			reply := service.Outcome(bound, "error")
			reply.Error = service.Forbidden
			writeJSON(w, http.StatusBadRequest, reply)
			return
		}
		errorResponse(w, err, &bound)
		return
	}
	// Legacy mobile clients cap the envelope at 32 KiB. Preserve their
	// original list at the boundary. Full lifecycle is available by sale.
	if req.Operation == "invoice.list" {
		const legacyReadLimit = 32 * 1024
		if encoded, _ := json.Marshal(out); len(encoded)+1 > legacyReadLimit {
			for index := range out.Invoices {
				out.Invoices[index].Stage = ""
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// status reads the order and, when it is paid through OrderEscrow, its escrow part. Both come
// from the same order revision.
func (h HTTP) status(ctx context.Context, bound service.Bound, locker OrderLocker) (Order, error) {
	for attempt := 0; ; attempt++ {
		order, err := h.Store.Status(ctx, bound)
		if err != nil || locker == nil || (bound.Role != "customer" && bound.Role != "terminal") {
			return order, err
		}
		order.Escrow, err = locker.EscrowStatus(ctx, bound, order)
		if errors.Is(err, service.Busy) && attempt < 2 {
			continue
		}
		return order, err
	}
}

func currentOrderPage(rows []CustomerOrderSummary, now time.Time) error {
	for _, row := range rows {
		if row.AccessEndsAt != nil && !now.Before(*row.AccessEndsAt) {
			return service.Forbidden
		}
	}
	return nil
}

func (h HTTP) issue(ctx context.Context, bound service.Bound) (*quoteReply, error) {
	return h.issueWithReview(ctx, bound, nil)
}

func (h HTTP) issueWithReview(ctx context.Context, bound service.Bound, review *RentalReviewChallenge) (*quoteReply, error) {
	if h.paused(bound.Request.Domain) {
		return nil, service.Capability
	}
	if h.QuoteAdmission != nil {
		// Reserve issuer capacity before acquiring a quote's five-second signing lease.
		wait, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		select {
		case h.QuoteAdmission <- struct{}{}:
			defer func() { <-h.QuoteAdmission }()
			if wait.Err() != nil {
				return nil, service.Busy
			}
		case <-wait.Done():
			return nil, service.Busy
		}
	}

	var profile Profile
	if _, enrolled := h.Profiles[bound.Request.Domain]; enrolled {
		current, err := h.currentProfile(ctx, bound.Request.Domain)
		if err != nil {
			return nil, err
		}
		profile = current
	}
	rawClaim, _ := hex.DecodeString(bound.Request.Body["claimDigest"])
	if len(rawClaim) != 32 {
		return nil, service.BadRequest
	}
	revision, err := strconv.ParseInt(bound.Request.Body["orderRevision"], 10, 64)
	if err != nil {
		return nil, service.BadRequest
	}
	if profile.Escrow {
		if err = h.Store.EscrowQuoteAllowed(ctx, bound, [32]byte(rawClaim)); err != nil {
			return nil, err
		}
	}
	ready, err := h.Store.BeginQuote(ctx, bound, [32]byte(rawClaim), revision, profile)
	if err != nil {
		return nil, err
	}
	aad := quoteContext(ready.Branch, ready.Sale)
	var signed []byte
	var digest [32]byte
	if len(ready.Ciphertext) != 0 {
		signed, err = h.Cipher.Open(nil, nil, ready.Ciphertext, aad)
		if err != nil {
			return nil, service.Unknown
		}
		if len(signed) <= crypto.SignatureLength {
			return nil, service.Mismatch
		}
		// The ciphertext, issue digest and original signer were committed once.
		// Current issuer configuration gates new signatures only.
		raw := signed[:len(signed)-crypto.SignatureLength]
		if quotev2.Verify(signed, quotev2.Policy{Expected: raw, MerchantSigner: ready.IssuedSigner, Now: uint64(time.Now().Unix())}) != 0 {
			return nil, service.Mismatch
		}
		digest, err = quotev2.Digest(raw)
		if err != nil || !bytes.Equal(digest[:], ready.Digest) {
			return nil, service.Mismatch
		}
	} else {
		// The external signer receives a digest after prepare_quote returned its
		// row and committed. No database transaction or row lock spans the call.
		if err = authority.Check(ctx); err != nil {
			return nil, err
		}
		signed, digest, err = SignQuote(ctx, ready.QuoteOrder, profile, h.Signer, time.Now())
		if err != nil {
			return nil, service.Unknown
		}
		ciphertext := h.Cipher.Seal(nil, nil, signed, aad)
		if err = h.Store.FinishQuote(ctx, bound, ready, profile, ciphertext, digest); err != nil {
			return nil, err
		}
	}
	var presented *Presentation
	if len(h.PresentationKey) != 0 {
		presented, err = h.present(ctx, bound, ready, profile, digest, signed, review)
		if err != nil {
			return nil, err
		}
	}
	return &quoteReply{Presentation: presented, SaleRef: bound.Request.Object, BranchRef: bound.Request.Domain, OrderKey: hex.EncodeToString(ready.OrderID[:]), Signed: hex.EncodeToString(signed), Digest: hex.EncodeToString(digest[:]), ExpiresAt: ready.Expires.Unix()}, nil
}

func errorResponse(w http.ResponseWriter, err error, bound *service.Bound) {
	var rejection *reservationRejection
	if bound != nil && errors.As(err, &rejection) {
		reply := service.Outcome(*bound, "error")
		reply.Error = rejection.code
		writeJSON(w, http.StatusConflict, result{Reply: reply, ReservationFound: true, Unavailable: rejection.lines})
		return
	}

	code := service.CodeOf(err)
	if errors.Is(err, ErrStock) {
		code = service.OutOfStock
	}
	status := http.StatusBadRequest
	switch code {
	case service.Forbidden:
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	case service.Conflict, service.CheckoutConflict, service.OutOfStock, service.CatalogChanged:
		status = http.StatusConflict
	case service.Busy, service.Unknown:
		status = http.StatusServiceUnavailable
	case "":
		status, code = http.StatusServiceUnavailable, service.Unknown
	}
	reply := service.Reply{Status: "error", Error: code}
	if bound != nil {
		reply = service.Outcome(*bound, "error")
		reply.Error = code
	}
	writeJSON(w, status, reply)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
