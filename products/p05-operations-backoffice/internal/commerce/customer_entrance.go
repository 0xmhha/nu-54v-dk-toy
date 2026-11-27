package commerce

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/orderescrow"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
)

// CustomerEntrance takes customer requests that carry no client certificate. A customer proves the board account
// that pays: the server gives a one-time challenge, the board signs it (NU54_ACCOUNT_PROOF_OP), and the proven
// address is checked against what the purpose needs. Terminal and operator routes keep mutual TLS.
//
// Purposes: "checkout" (before payment: the proven address is the customer account, with one unpaid hold per
// account), "order" (the address paid this order: a session for that order only, until it ends) and "pickup" (the
// address paid this ready order: a one-time QR token that staff scan on the order's page).
type CustomerEntrance struct {
	Limiter *RateLimiter
	// Zero values take the defaults below. The order session cap is provisional (an owner decision).
	ChallengeLifetime, CheckoutLifetime, OrderLifetime, PickupLifetime time.Duration
	// RescanWindow is how long after a handover the same staff member's repeated scan gets the first answer again.
	RescanWindow time.Duration
	// OperatorContact is what a store page shows for payment and refund questions; empty shows none.
	OperatorContact string
}

const (
	defaultChallengeLifetime = 2 * time.Minute
	defaultCheckoutLifetime  = 15 * time.Minute
	defaultOrderLifetime     = 30 * 24 * time.Hour
	defaultPickupLifetime    = 60 * time.Second
	defaultRescanWindow      = 2 * time.Minute
)

func lifetime(set, fallback time.Duration) time.Duration {
	if set <= 0 {
		return fallback
	}
	return set
}

// CustomerSubject is the C3 subject of one board account: a version 4 UUID from a domain-separated SHA-256, so an
// address cannot be chosen to land on a store-issued random subject. The ledger also refuses a subject that already
// belongs to a store grant (migration 122).
func CustomerSubject(account common.Address) string {
	sum := sha256.Sum256(append([]byte("nu54-customer-account-v1\x00"), account[:]...))
	sum[6] = sum[6]&0x0f | 0x40
	sum[8] = sum[8]&0x3f | 0x80
	text := hex.EncodeToString(sum[:16])
	return text[:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:]
}

// verifyAccountProof recovers the signer of the board's account proof and compares it with the claimed account.
func verifyAccountProof(account common.Address, challenge [32]byte, signature []byte) bool {
	if challenge == ([32]byte{}) || account == (common.Address{}) || len(signature) != 65 {
		return false
	}
	return orderescrow.SignedBy(orderescrow.AccountProofDigest(account, challenge), signature, account)
}

// RateLimiter is a token bucket per client key. It keeps at most limiterClients keys and forgets full buckets first.
type RateLimiter struct {
	mu      sync.Mutex
	perSec  float64
	burst   float64
	now     func() time.Time
	clients map[string]*bucket
}

type bucket struct {
	tokens float64
	at     time.Time
}

const limiterClients = 10_000

// NewRateLimiter allows perMinute requests a minute and burst at once for each client. now may be nil.
func NewRateLimiter(perMinute, burst int, now func() time.Time) *RateLimiter {
	if now == nil {
		now = time.Now
	}
	return &RateLimiter{perSec: float64(perMinute) / 60, burst: float64(burst), now: now, clients: map[string]*bucket{}}
}

func (l *RateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.clients[key]
	if !ok {
		if len(l.clients) >= limiterClients {
			for k, old := range l.clients {
				if old.tokens+now.Sub(old.at).Seconds()*l.perSec >= l.burst {
					delete(l.clients, k)
				}
			}
			if len(l.clients) >= limiterClients {
				return false // every known client is still limited: refuse newcomers until some refill
			}
		}
		b = &bucket{tokens: l.burst, at: now}
		l.clients[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.at).Seconds()*l.perSec)
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// clientKey is the peer address. Behind a proxy every customer shares the proxy's address (see the report).
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RegisterCustomer adds the customer entrance to a mux served without client certificates.
func (h HTTP) RegisterCustomer(mux *http.ServeMux) {
	if h.Customer == nil {
		return
	}
	limit := func(next http.HandlerFunc) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if h.Customer.Limiter != nil && !h.Customer.Limiter.Allow(clientKey(r)) {
				w.Header().Set("Retry-After", "10")
				writeJSON(w, http.StatusTooManyRequests, service.Reply{Status: "error", Error: service.Busy})
				return
			}
			next(w, r)
		})
	}
	mux.Handle("POST /v2/customer/challenge", limit(h.customerChallenge))
	mux.Handle("POST /v2/customer/proof", limit(h.customerProof))
	mux.Handle("POST /v2/customer/commerce", limit(h.customerCommerce))
	// The store list and a store page are read before any proof: public data only, under the same limit.
	mux.Handle("GET /v2/customer/stores", limit(h.customerStores))
	mux.Handle("GET /v2/customer/stores/{branch}", limit(h.customerStore))
}

func decodeStrict(w http.ResponseWriter, r *http.Request, limit int64, into any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	if decoder.Decode(into) != nil {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}

var customerHex32 = regexp.MustCompile(`^[0-9a-f]{64}$`)
var customerAccount = regexp.MustCompile(`^0x[0-9a-f]{40}$`)

func (h HTTP) customerChallenge(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Purpose string `json:"purpose"`
		Branch  string `json:"branchRef"`
		Sale    string `json:"saleRef,omitempty"`
	}
	if !decodeStrict(w, r, 512, &in) || !operatorRef.MatchString(in.Branch) || !slices.Contains([]string{"checkout", "order", "pickup"}, in.Purpose) ||
		(in.Purpose == "checkout") != (in.Sale == "") || (in.Sale != "" && !operatorRef.MatchString(in.Sale)) {
		errorResponse(w, service.BadRequest, nil)
		return
	}
	if h.Store == nil {
		http.NotFound(w, r)
		return
	}
	challenge, err := freshId()
	if err != nil {
		errorResponse(w, service.Unknown, nil)
		return
	}
	hash := sha256.Sum256(challenge[:])
	var sale *string
	if in.Sale != "" {
		sale = &in.Sale
	}
	seconds := int(lifetime(h.Customer.ChallengeLifetime, defaultChallengeLifetime) / time.Second)
	var expires time.Time
	if err = h.Store.db.QueryRow(r.Context(), "SELECT ledger.issue_customer_challenge($1,$2,$3,$4,$5)",
		in.Branch, in.Purpose, sale, hash[:], seconds).Scan(&expires); err != nil {
		errorResponse(w, customerError(err), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"challenge": hex.EncodeToString(challenge[:]), "expiresAt": expires.Unix()})
}

func customerError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		if pg.Code == "23503" {
			return service.Forbidden
		}
	}
	return domainError(err)
}

type customerProofReply struct {
	Purpose     string `json:"purpose"`
	Branch      string `json:"branchRef"`
	Sale        string `json:"saleRef,omitempty"`
	Tenant      string `json:"tenantRef,omitempty"`
	Subject     string `json:"subjectRef,omitempty"`
	Session     string `json:"session,omitempty"`
	PickupToken string `json:"pickupToken,omitempty"`
	ExpiresAt   int64  `json:"expiresAt"`
}

// customerProof accepts the board's signature over one challenge and answers with an order session or a pickup token.
func (h HTTP) customerProof(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Challenge  string `json:"challenge"`
		Account    string `json:"account"`
		Signature  string `json:"signature"`
		Generation string `json:"generation,omitempty"`
	}
	if !decodeStrict(w, r, 1024, &in) || !customerHex32.MatchString(in.Challenge) || !customerAccount.MatchString(in.Account) {
		errorResponse(w, service.BadRequest, nil)
		return
	}
	signature, err := hex.DecodeString(in.Signature)
	if err != nil || len(signature) != 65 || hex.EncodeToString(signature) != in.Signature {
		errorResponse(w, service.BadRequest, nil)
		return
	}
	var generation *int64
	if in.Generation != "" {
		value, err := strconv.ParseInt(in.Generation, 10, 64)
		if err != nil || value <= 0 || strconv.FormatInt(value, 10) != in.Generation {
			errorResponse(w, service.BadRequest, nil)
			return
		}
		generation = &value
	}
	raw, _ := hex.DecodeString(in.Challenge)
	challenge := [32]byte(raw)
	account := common.HexToAddress(in.Account)
	// A signature that does not recover to the account leaves the challenge unused: only its own board can spend it.
	if !verifyAccountProof(account, challenge, signature) {
		errorResponse(w, service.Mismatch, nil)
		return
	}
	if h.Store == nil {
		http.NotFound(w, r)
		return
	}
	token, err := freshId()
	if err != nil {
		errorResponse(w, service.Unknown, nil)
		return
	}
	challengeHash, tokenHash := sha256.Sum256(challenge[:]), sha256.Sum256(token[:])
	subject := CustomerSubject(account)
	seconds := func(set, fallback time.Duration) int { return int(lifetime(set, fallback) / time.Second) }
	var outcome, purpose, branchRef string
	var sale, tenantRef *string
	var expires *time.Time
	err = h.Store.db.QueryRow(r.Context(), "SELECT outcome_,purpose_,branch_::text,sale_::text,tenant_::text,expires_ FROM ledger.prove_customer_account($1,$2,$3,$4,$5,$6,$7,$8)",
		challengeHash[:], account[:], subject, generation, tokenHash[:],
		seconds(h.Customer.CheckoutLifetime, defaultCheckoutLifetime), seconds(h.Customer.OrderLifetime, defaultOrderLifetime),
		seconds(h.Customer.PickupLifetime, defaultPickupLifetime)).Scan(&outcome, &purpose, &branchRef, &sale, &tenantRef, &expires)
	if err != nil {
		errorResponse(w, customerError(err), nil)
		return
	}
	switch outcome {
	case "mismatch":
		errorResponse(w, service.Mismatch, nil)
		return
	case "not_ready":
		errorResponse(w, service.CheckoutConflict, nil)
		return
	case "ok":
	default:
		errorResponse(w, service.Unknown, nil)
		return
	}
	reply := customerProofReply{Purpose: purpose, Branch: branchRef, ExpiresAt: expires.Unix()}
	if sale != nil {
		reply.Sale = *sale
	}
	if purpose == "pickup" {
		reply.PickupToken = hex.EncodeToString(token[:])
	} else {
		reply.Session, reply.Subject, reply.Tenant = hex.EncodeToString(token[:]), subject, *tenantRef
	}
	writeJSON(w, http.StatusOK, reply)
}

// customerSession is a live session read back from its token.
type customerSession struct {
	Branch, Actor, Purpose, Sale, Tenant string
	ExpiresAt                            time.Time
}

// Before payment: the menu, the cart, the customer's own claim on it, the quote, the board's signature for the
// sponsored lock, and the status of what this account ordered here. There is no customer cancel: C3 has none (see
// ledger.cancel_public_menu_order, which is the kiosk's), so an unpaid order ends with its quote.
var checkoutOperations = []string{"catalog.read", "catalog.photo", "order.create", "checkout.claim", "checkout.authorize", "quote.issue",
	"order.lock", "payment.forward", "order.status"}

// allows says whether one C3 request is inside the session. The owner database still checks each one.
func (s customerSession) allows(req service.Request) bool {
	if req.Contract != "C3" || req.Domain != s.Branch {
		return false
	}
	switch s.Purpose {
	case "checkout":
		if req.Operation == "catalog.read" || req.Operation == "catalog.photo" {
			return req.Object == s.Branch
		}
		return slices.Contains(checkoutOperations, req.Operation)
	case "order":
		// Status (with the ready notice), and the receipt confirmation, dispute or its withdrawal.
		return req.Object == s.Sale && (req.Operation == "order.status" || req.Operation == "order.answer")
	}
	return false
}

func (s *Store) customerSession(ctx context.Context, token []byte) (customerSession, error) {
	hash := sha256.Sum256(token)
	var stored []byte
	var session customerSession
	var sale *string
	err := s.db.QueryRow(ctx, "SELECT token_hash,branch_id::text,actor_id::text,purpose,sale_id::text,tenant_ref::text,expires_at FROM ledger.customer_session_live($1)",
		hash[:]).Scan(&stored, &session.Branch, &session.Actor, &session.Purpose, &sale, &session.Tenant, &session.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && subtle.ConstantTimeCompare(stored, hash[:]) != 1) {
		return customerSession{}, service.Forbidden
	}
	if err != nil {
		return customerSession{}, domainError(err)
	}
	if sale != nil {
		session.Sale = *sale
	}
	return session, nil
}

// customerCommerce runs one C3 envelope as the session's customer through the same handler as the certificate route.
func (h HTTP) customerCommerce(w http.ResponseWriter, r *http.Request) {
	value, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	token, err := hex.DecodeString(value)
	if !found || err != nil || len(token) != 32 || hex.EncodeToString(token) != value || h.Store == nil {
		http.NotFound(w, r)
		return
	}
	session, err := h.Store.customerSession(r.Context(), token)
	if err != nil || !time.Now().Before(session.ExpiresAt) {
		http.NotFound(w, r)
		return
	}
	principal := service.Principal{Tenant: session.Tenant, Subject: session.Actor, Role: "customer", ExpiresAt: session.ExpiresAt.Unix()}
	// Like a certificate route, writes commit only while the session still stands.
	w, r, cancel := authority.Bound(r, session.ExpiresAt, func() bool { return time.Now().Before(session.ExpiresAt) }, w)
	defer cancel()
	h.serveFor(w, r, principal, &session)
}

// pickupScanRefusal is the service's error reply with the reason a pickup QR was refused, so the operations web can
// say which. RecordedAt (Unix seconds) is the handover a used QR already recorded.
type pickupScanRefusal struct {
	service.Reply
	Reason     string `json:"reason"`
	RecordedAt int64  `json:"recordedAt,omitempty"`
}

// pickupRefusal turns a refused outcome of ledger.scan_pickup_token into its HTTP status and reply.
func pickupRefusal(outcome string, recordedAt int64) (int, pickupScanRefusal, bool) {
	refusal := pickupScanRefusal{Reply: service.Reply{Status: "error"}, Reason: outcome}
	switch outcome {
	case "expired": // past its time, never issued, or removed since: the customer shows a new QR
		refusal.Error = service.Expired
		return http.StatusBadRequest, refusal, true
	case "already_used":
		refusal.Error, refusal.RecordedAt = service.Conflict, recordedAt
		return http.StatusConflict, refusal, true
	case "other_store", "other_order":
		refusal.Error = service.Mismatch
		return http.StatusBadRequest, refusal, true
	case "not_ready":
		refusal.Error = service.CheckoutConflict
		return http.StatusConflict, refusal, true
	}
	return 0, pickupScanRefusal{}, false
}

// pickupScan records the handover of the order the staff opened, when its QR is scanned on that order's page of the
// operations web. It does not release funds: the delivered mark starts the buyer's receipt window as before.
func (h HTTP) pickupScan(w http.ResponseWriter, r *http.Request) {
	branchRef, sale := r.PathValue("branch"), r.PathValue("sale")
	principal, _, exists := service.CurrentClientPrincipal(r.TLS, h.Credentials, time.Now())
	if !exists || (principal.Role != "operator" && principal.Role != "terminal") || principal.ExpiresAt <= time.Now().Unix() ||
		!operatorRef.MatchString(principal.Tenant) || !operatorRef.MatchString(principal.Subject) ||
		!operatorRef.MatchString(branchRef) || !operatorRef.MatchString(sale) || h.Store == nil {
		http.NotFound(w, r)
		return
	}
	// A browser page: the same origin and JSON checks as the other operations changes.
	if h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin || strings.TrimSpace(r.Header.Get("Content-Type")) != "application/json" {
		http.NotFound(w, r)
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if !decodeStrict(w, r, 256, &in) || !customerHex32.MatchString(in.Token) {
		errorResponse(w, service.BadRequest, nil)
		return
	}
	token, _ := hex.DecodeString(in.Token)
	hash := sha256.Sum256(token)
	capability := "pickup_manage"
	if principal.Role == "terminal" {
		capability = "pickup_assignment"
	}
	ctx := authority.Scope(r.Context(), principal, branchRef, capability)
	var raw []byte
	err := h.Store.db.QueryRow(ctx, "SELECT ledger.scan_pickup_token($1,$2,$3,$4,$5,$6,$7)",
		principal.Tenant, branchRef, principal.Subject, principal.Role, sale, hash[:],
		int(lifetime(h.Customer.RescanWindow, defaultRescanWindow)/time.Second)).Scan(&raw)
	if err != nil {
		errorResponse(w, customerError(err), nil)
		return
	}
	var result struct {
		Outcome    string          `json:"outcome"`
		RecordedAt int64           `json:"recordedAt"`
		Reply      json.RawMessage `json:"reply"`
	}
	if json.Unmarshal(raw, &result) != nil {
		errorResponse(w, service.Unknown, nil)
		return
	}
	if status, refusal, refused := pickupRefusal(result.Outcome, result.RecordedAt); refused {
		writeJSON(w, status, refusal)
		return
	}
	if result.Outcome != "ok" || len(result.Reply) == 0 {
		errorResponse(w, service.Unknown, nil)
		return
	}
	writeJSON(w, http.StatusOK, result.Reply)
}
