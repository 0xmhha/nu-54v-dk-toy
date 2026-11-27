package commerce

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
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
	// Limiter is the strict limit of each address on what needs no session: a challenge and a proof. ReadLimiter,
	// PhotoLimiter and AddressLimiter fall back to it when they are not set.
	Limiter *RateLimiter
	// ReadLimiter is the limit of each address on the public reads: the store list, a store page and a menu page.
	ReadLimiter *RateLimiter
	// PhotoLimiter is the limit of each address on public menu photo parts; nil leaves them under ReadLimiter.
	PhotoLimiter *RateLimiter
	// AddressLimiter is the ceiling of each address over every request that carries a session token. It is counted
	// before the token is looked up, so a refused request never reaches the database, and no number of sessions,
	// live or made up, takes one address past it.
	AddressLimiter *RateLimiter
	// SessionLimiter is the limit of each live session on its own requests, counted after the lookup. Customers
	// behind one address, such as a store's wireless network, do not share it; they share AddressLimiter.
	SessionLimiter *RateLimiter
	// UnknownLimiter is the limit of each address on session tokens the server does not know: a guess or a stale
	// session. It is apart from Limiter, so phones with expired sessions behind one address do not use up the
	// proofs that address needs to get new ones. Nil leaves such tokens to AddressLimiter alone.
	UnknownLimiter *RateLimiter
	// StartLimiter is the limit of each address on the challenges that can end in a session (checkout and order),
	// and so on the sessions and the new proven accounts one address starts in a time window. Pickup challenges are
	// not counted: they end in a QR, and a waiting customer renews it every minute.
	StartLimiter *RateLimiter
	// HoldLimiter is the limit of each address on the carts its sessions ask to hold. One proven account holds one
	// unpaid cart at a time (migration 122); this bounds how many accounts of one address hold stock at once.
	HoldLimiter *RateLimiter
	// Funds reads a proven account's balance before its cart holds stock (customer_funds.go). Without it no cart
	// of a proven account is held.
	Funds PayerFunds
	// Gate, when set, wraps what a request does once it is admitted. opsd passes the recovery gate, whose lease
	// holds a connection of the control database for as long as the handler runs. A request enters it only after
	// its address passed the rate limit, its body was read and it took a place in InFlight, so a refused request,
	// the health route and a client that sends its body slowly hold no such connection.
	Gate func(http.Handler) http.Handler
	// InFlight is how many requests may be inside Gate at once: its capacity is the cap, and nil sets none. A
	// request that finds no place within InFlightWait (one second when zero) is answered 503 with Retry-After.
	InFlight     chan struct{}
	InFlightWait time.Duration
	// Limited counts refused requests by kind; opsd logs it.
	Limited LimitedCount
	// Zero values take the defaults below. The order session cap is provisional (an owner decision).
	ChallengeLifetime, CheckoutLifetime, OrderLifetime, PickupLifetime time.Duration
	// lookup reads a live session by its token. Nil reads the store; tests count the reads with it.
	lookup func(context.Context, []byte) (customerSession, error)
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
	defaultInFlightWait      = time.Second
	// entranceBody is the longest body an entrance route reads (the C3 envelope).
	entranceBody = 8192
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
	allowed, _ := l.Take(key)
	return allowed
}

// Take is Allow with the time after which a refused client has one request again.
func (l *RateLimiter) Take(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	oneRequest := time.Duration(float64(time.Second) / l.perSec)
	b, ok := l.clients[key]
	if !ok {
		if len(l.clients) >= limiterClients {
			for k, old := range l.clients {
				if old.tokens+now.Sub(old.at).Seconds()*l.perSec >= l.burst {
					delete(l.clients, k)
				}
			}
			if len(l.clients) >= limiterClients {
				return false, oneRequest // every known client is still limited: refuse newcomers until some refill
			}
		}
		b = &bucket{tokens: l.burst, at: now}
		l.clients[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.at).Seconds()*l.perSec)
	b.at = now
	if b.tokens < 1 {
		return false, time.Duration((1 - b.tokens) / l.perSec * float64(time.Second))
	}
	b.tokens--
	return true, 0
}

// LimitedCount counts the requests the entrance refused for their rate, by kind.
type LimitedCount struct {
	mu     sync.Mutex
	counts map[string]int64
}

func (c *LimitedCount) add(kind string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts == nil {
		c.counts = map[string]int64{}
	}
	c.counts[kind]++
}

// Drain returns the counts since the last call, by kind: "proof", "read", "photo", "address" (the ceiling of one
// address over session requests), "session" (one session's own limit), "unknown" (session tokens the server does not
// know), "start" (challenges that can end in a session), "hold" (carts an address asked to hold) and "busy" (no
// place among the requests served at once).
func (c *LimitedCount) Drain() map[string]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	counts := c.counts
	c.counts = nil
	return counts
}

// tooMany answers a rate refusal. Retry-After is the whole seconds until the client has one request again.
func (c *CustomerEntrance) tooMany(w http.ResponseWriter, kind string, wait time.Duration) {
	c.Limited.add(kind)
	w.Header().Set("Retry-After", strconv.Itoa(max(1, int((wait+time.Second-1)/time.Second))))
	writeJSON(w, http.StatusTooManyRequests, service.Reply{Status: "error", Error: service.Busy})
}

// admitted serves next for a request that passed its rate limit: with its body already read, inside the cap on
// requests served at once, and inside the gate. The order keeps the gate's connection away from what a client
// controls: how many requests it sends and how slowly it sends each.
func (c *CustomerEntrance) admitted(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Body != http.NoBody {
			// One byte past the longest body is kept, so a longer one still fails its route's own length check.
			body, err := io.ReadAll(io.LimitReader(r.Body, entranceBody+1))
			if err != nil {
				errorResponse(w, service.BadRequest, nil) // the client stopped sending
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		if c.InFlight != nil {
			wait := time.NewTimer(lifetime(c.InFlightWait, defaultInFlightWait))
			defer wait.Stop()
			select {
			case c.InFlight <- struct{}{}:
				defer func() { <-c.InFlight }()
			case <-wait.C:
				c.Limited.add("busy")
				w.Header().Set("Retry-After", "1")
				writeJSON(w, http.StatusServiceUnavailable, service.Reply{Status: "error", Error: service.Busy})
				return
			case <-r.Context().Done():
				return
			}
		}
		if c.Gate != nil {
			c.Gate(next).ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientKey is the peer a limit counts for, without its port. An IPv4 address is one client. An IPv6 client is its
// /64: a network hands one customer a whole /64, so counting each address apart would give one device as many
// allowances as it has addresses. Behind a proxy every customer shares the proxy's address (see the report).
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	address = address.Unmap().WithZone("")
	if address.Is4() {
		return address.String()
	}
	network, _ := address.Prefix(64)
	return network.String()
}

// RegisterCustomer adds the customer entrance to a mux served without client certificates.
func (h HTTP) RegisterCustomer(mux *http.ServeMux) {
	if h.Customer == nil {
		return
	}
	// The limiters are read when a request arrives, so a deployment may set them after the routes. A request that
	// passes its limit is then admitted (CustomerEntrance.admitted); a refused one costs no more than the refusal.
	limited := func(kind string, limiter func() *RateLimiter, next http.HandlerFunc) http.Handler {
		admitted := h.Customer.admitted(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if active := limiter(); active != nil {
				if allowed, wait := active.Take(clientKey(r)); !allowed {
					h.Customer.tooMany(w, kind, wait)
					return
				}
			}
			admitted.ServeHTTP(w, r)
		})
	}
	first := func(limiters ...*RateLimiter) *RateLimiter {
		for _, limiter := range limiters {
			if limiter != nil {
				return limiter
			}
		}
		return nil
	}
	proof := func(next http.HandlerFunc) http.Handler {
		return limited("proof", func() *RateLimiter { return h.Customer.Limiter }, next)
	}
	limit := func(next http.HandlerFunc) http.Handler {
		return limited("read", func() *RateLimiter { return first(h.Customer.ReadLimiter, h.Customer.Limiter) }, next)
	}
	photos := func() *RateLimiter { return first(h.Customer.PhotoLimiter, h.Customer.ReadLimiter, h.Customer.Limiter) }
	// The listener's own health, outside every limit and outside the gate: it says nothing about a store or a
	// customer, and it answers while the control database does not.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.Handle("POST /v2/customer/challenge", proof(h.customerChallenge))
	mux.Handle("POST /v2/customer/proof", proof(h.customerProof))
	// A request with a session token is counted for its address here, before the token is looked up, and for its
	// session once that is known (customerCommerce).
	mux.Handle("POST /v2/customer/commerce", limited("address",
		func() *RateLimiter { return first(h.Customer.AddressLimiter, h.Customer.Limiter) }, h.customerCommerce))
	// The store list and a store page are read before any proof: public data only, under the same limit.
	mux.Handle("GET /v2/customer/stores", limit(h.customerStores))
	mux.Handle("GET /v2/customer/stores/{branch}", limit(h.customerStore))
	// A store's menu and its photos are public too (menu_public.go). Nothing else is: every other request above
	// needs a challenge, a proof or a session.
	mux.Handle("GET /v2/customer/stores/{branch}/menu/{page}", limit(h.customerMenu))
	mux.Handle("GET /v2/customer/stores/{branch}/photos/{photo}/{offset}", limited("photo", photos, h.customerMenuPhoto))
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
	// A checkout or order challenge is the start of a session: each address has a limit on those of its own.
	if limiter := h.Customer.StartLimiter; limiter != nil && in.Purpose != "pickup" {
		if allowed, wait := limiter.Take(clientKey(r)); !allowed {
			h.Customer.tooMany(w, "start", wait)
			return
		}
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

// What a checkout session may ask before payment, on an order of its own account at its branch: the cart, the
// customer's own claim on it, the quote, and the board's signature for the sponsored lock. There is no customer
// cancel in C3 (ledger.cancel_public_menu_order is the kiosk's); an unpaid order ends with its quote.
var checkoutOperations = []string{"order.create", "checkout.claim", "checkout.authorize", "quote.issue", "order.lock", "payment.forward"}

// checkoutReads is what a checkout session may read.
//
// order.status is allowed on purpose, and it reaches further than "before payment": it reads any order of the same
// account at this branch, a paid one too. The app that just paid has only this session, and the payment result (paid
// or not, then the pickup number) is the status of that order; without this read it could not learn it before it
// proves the account again for an order session. The reach is the account's own orders and nothing else: the
// session names no order, so the owner database decides, and ledger.commerce_order_receipt answers only the
// customer the order belongs to (another account or another branch reads not found;
// customer_entrance_postgres_test.go).
var checkoutReads = []string{"catalog.read", "catalog.photo", "order.status"}

// allows says whether one C3 request is inside the session. The owner database still checks each one.
func (s customerSession) allows(req service.Request) bool {
	if req.Contract != "C3" || req.Domain != s.Branch {
		return false
	}
	switch s.Purpose {
	case "checkout":
		if req.Operation == "catalog.read" || req.Operation == "catalog.photo" {
			return req.Object == s.Branch // the menu and the photos of the session's own branch
		}
		return slices.Contains(checkoutOperations, req.Operation) || slices.Contains(checkoutReads, req.Operation)
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
	// The address was counted before this handler (RegisterCustomer). A token the server does not know is a guess
	// or a stale session: it is counted for the address once more, strictly, and apart from the address's proofs.
	refuse := func() {
		if limiter := h.Customer.UnknownLimiter; limiter != nil {
			if allowed, wait := limiter.Take(clientKey(r)); !allowed {
				h.Customer.tooMany(w, "unknown", wait)
				return
			}
		}
		http.NotFound(w, r)
	}
	lookup := h.Customer.lookup
	if lookup == nil && h.Store != nil {
		lookup = h.Store.customerSession
	}
	if !found || err != nil || len(token) != 32 || hex.EncodeToString(token) != value || lookup == nil {
		refuse()
		return
	}
	session, err := lookup(r.Context(), token)
	if err != nil || !time.Now().Before(session.ExpiresAt) {
		refuse()
		return
	}
	// A live session has its own allowance, whatever address it comes from.
	if limiter := h.Customer.SessionLimiter; limiter != nil {
		if allowed, wait := limiter.Take(session.Actor + "/" + session.Branch + "/" + session.Purpose + "/" + session.Sale); !allowed {
			h.Customer.tooMany(w, "session", wait)
			return
		}
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
//
// Only an operator of the operations web with the handover authority (pickup_manage) scans a pickup QR (owner
// decision). A kiosk or pickup terminal certificate is not found here, whatever it is assigned to; the database
// refuses that role again.
func (h HTTP) pickupScan(w http.ResponseWriter, r *http.Request) {
	branchRef, sale := r.PathValue("branch"), r.PathValue("sale")
	principal, _, exists := service.CurrentClientPrincipal(r.TLS, h.Credentials, time.Now())
	if !exists || principal.Role != "operator" || principal.ExpiresAt <= time.Now().Unix() ||
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
	ctx := authority.Scope(r.Context(), principal, branchRef, "pickup_manage")
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
