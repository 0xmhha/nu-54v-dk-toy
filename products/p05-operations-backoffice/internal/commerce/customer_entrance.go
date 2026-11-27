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
//
// A fourth purpose, "account", belongs to no store: its proof answers how many unfinished orders the account has, so
// a phone that kept no record of them can still learn that its board must not be reset yet.
//
// A challenge is kept nowhere: it carries its purpose, its store or order and its expiry under the server's own tag
// (customer_challenge.go). The request limits (customer_limits.go) are laid out so that what costs a sender nothing
// uses up nothing the customers behind the same address need.
type CustomerEntrance struct {
	// IssueLimiter is the limit of each address on challenge requests, counted as one arrives. Giving a challenge
	// writes nothing, so this bounds only the reads of the store behind it. Nil counts them under Limiter.
	IssueLimiter *RateLimiter
	// AttemptLimiter is the limit of each address on proof requests, counted as one arrives and before its body is
	// read or its signature checked. Nil counts them under Limiter.
	AttemptLimiter *RateLimiter
	// FailLimiter is the strict limit of each address on challenges and proofs that failed in a way no app's request
	// does: a body that does not read, a challenge the server did not give, a signature that is not the account's, a
	// store that is not public, an order the account did not pay. It is counted after the attempt failed; an
	// address that used it up is answered a wait instead of the error. Nil answers every failed attempt as it is.
	FailLimiter *RateLimiter
	// AccountLimiter is the limit of each proven account, at each address, on its proofs: counted only for a proof
	// with the account's own signature over a challenge of this server.
	AccountLimiter *RateLimiter
	// Limiter is the limit of each address on the proofs that start a checkout session or ask about an account,
	// counted after the account's own. A proof for an order (its session, its pickup QR) is not counted here: the
	// database decides whether the account paid that order. IssueLimiter, AttemptLimiter, ReadLimiter, PhotoLimiter
	// and AddressLimiter fall back to it when they are not set.
	Limiter *RateLimiter
	// ReadLimiter is the limit of each address on the public reads: the store list, a store page, a menu page and
	// the health route.
	ReadLimiter *RateLimiter
	// PhotoLimiter is the limit of each address on public menu photo parts; nil leaves them under ReadLimiter.
	PhotoLimiter *RateLimiter
	// AddressLimiter is the ceiling of each address over requests whose session token the server has not lately
	// seen live. It is counted before the token is looked up, so a refused request never reaches the database, and
	// no number of made-up tokens takes one address past it.
	AddressLimiter *RateLimiter
	// LiveLimiter is the ceiling of each address over the requests of checkout sessions lately seen live: given by
	// a proof of this process, or found live by an earlier lookup. It is counted after the session's own allowance,
	// so a session that asks too often uses up its own and not the address's. The session of a paid order is not
	// under it. Nil counts those requests under AddressLimiter.
	LiveLimiter *RateLimiter
	// SessionLimiter is the limit of each live session on its own requests. For a session lately seen live it is
	// counted first, from memory; for any other after the lookup found it.
	SessionLimiter *RateLimiter
	// UnknownLimiter is the limit of each address on session tokens the server does not know: a guess or a stale
	// session. It is apart from Limiter, so phones with expired sessions behind one address do not use up the
	// proofs that address needs to get new ones. Nil leaves such tokens to AddressLimiter alone.
	UnknownLimiter *RateLimiter
	// StartLimiter is the limit of each proven account, at each address, on the sessions it starts: it is counted
	// only for a proof that gave a checkout or an order session. A pickup QR and the count of unfinished orders are
	// not sessions. One account cannot use up another's, and a request without a valid signature never reaches it.
	StartLimiter *RateLimiter
	// HoldAttempts is the limit of each proven account, at each address, on the carts it asks to hold: each one
	// reads the chain. A request the reservation already answered is not counted.
	HoldAttempts *RateLimiter
	// HoldLimiter is the limit of each address on the carts its accounts come to hold: it is counted only for a cart
	// whose account had the balance and whose stock was held. One proven account holds one unpaid cart at a time
	// (migration 122); this bounds how many funded accounts of one address take stock in a time window.
	HoldLimiter *RateLimiter
	// Funds reads a proven account's balance before its cart holds stock (customer_funds.go). Without it no cart
	// of a proven account is held.
	Funds PayerFunds
	// BalanceReads is how many balance reads may wait for the chain at once: its capacity is the cap, and nil sets
	// none. A chain that answers slowly then holds that many of the places in InFlight and no more, so menus and
	// store pages are still read. A cart that finds no place is refused like one whose balance could not be read.
	BalanceReads chan struct{}
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
	// ListPage is how many stores one page of the store list holds; zero, or more than 200, is 200.
	ListPage int
	// clock is the time challenges and the memory below are measured by; nil is the system's.
	clock func() time.Time
	// keys sign the challenges the entrance gives (customer_challenge.go).
	keys challengeKeys
	// live is the session tokens the entrance lately saw live, each with its purpose and the key of its own
	// allowance, so a request of one is counted for its session before the database is read. It is bounded and is
	// forgotten with the process; a session it does not know is only counted more strictly, never refused for that.
	live recentSet
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

// Drain returns the counts since the last call, by kind: "issue" (challenge requests of an address), "attempt"
// (proof requests of an address), "failed" (failed attempts of an address), "account" (one account's proofs),
// "proof" (proofs of an address that start a checkout session or ask about an account), "start" (sessions one
// account started), "read", "photo", "address" (session tokens of an address not lately seen live), "live" (the
// ceiling of an address over its checkout sessions), "session" (one session's own limit), "unknown" (session tokens
// the server does not know), "hold" (carts an account asked to hold, or an address came to hold), "balance" (no
// place among the balance reads) and "busy" (no place among the requests served at once).
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
		if !readBody(w, r) {
			return
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
func clientKey(r *http.Request) string { return ClientAddressKey(r.RemoteAddr) }

// ClientAddressKey is the key a peer address is counted under (clientKey): opsd counts the connections of the
// customer listener under the same one.
func ClientAddressKey(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
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
	limit := func(next http.HandlerFunc) http.Handler {
		return limited("read", func() *RateLimiter { return first(h.Customer.ReadLimiter, h.Customer.Limiter) }, next)
	}
	photos := func() *RateLimiter { return first(h.Customer.PhotoLimiter, h.Customer.ReadLimiter, h.Customer.Limiter) }
	// The listener's own health: under the limit of public reads, so it does not keep a connection open for
	// nothing, and outside the gate and the cap on requests at once: it says nothing about a store or a customer,
	// and it answers while the control database does not.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if active := first(h.Customer.ReadLimiter, h.Customer.Limiter); active != nil {
			if allowed, wait := active.Take(clientKey(r)); !allowed {
				h.Customer.tooMany(w, "read", wait)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, "ok\n")
	})
	// A challenge and a proof are counted for their address as they arrive, read, and then counted for what they
	// are before they are let through (customer_limits.go).
	mux.Handle("POST /v2/customer/challenge", h.Customer.challenges(h.customerChallenge))
	mux.Handle("POST /v2/customer/proof", h.Customer.proofs(h.customerProof))
	// A request with a session token. A token lately seen live is counted for its session first, from memory, and
	// a request its own session refuses is counted nowhere else; a checkout session is then counted under the
	// ceiling of its address, and the session of a paid order is not. Any other token is counted for its address
	// before it is looked up, under the stricter ceiling of tokens the server may not know, and for its session
	// once the lookup found it (liveSession).
	sessions := func(next http.HandlerFunc) http.Handler {
		admitted := h.Customer.admitted(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := h.Customer
			address := clientKey(r)
			if hash, ok := bearerHash(r); ok {
				if note, known := c.live.get(string(hash[:]), c.now()); known {
					purpose, own, _ := strings.Cut(note, ":")
					if c.SessionLimiter != nil {
						if allowed, wait := c.SessionLimiter.Take(own); !allowed {
							c.tooMany(w, "session", wait)
							return
						}
					}
					ceiling, kind := c.LiveLimiter, "live"
					if ceiling == nil {
						ceiling, kind = first(c.AddressLimiter, c.Limiter), "address"
					}
					if purpose != "order" && ceiling != nil {
						if allowed, wait := ceiling.Take(address); !allowed {
							if c.SessionLimiter != nil {
								c.SessionLimiter.Refund(own) // refused for its address, not for itself
							}
							c.tooMany(w, kind, wait)
							return
						}
					}
					admitted.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), countedSession{}, own)))
					return
				}
			}
			if limiter := first(c.AddressLimiter, c.Limiter); limiter != nil {
				if allowed, wait := limiter.Take(address); !allowed {
					c.tooMany(w, "address", wait)
					return
				}
			}
			admitted.ServeHTTP(w, r)
		})
	}
	mux.Handle("POST /v2/customer/commerce", sessions(h.customerCommerce))
	// A checkout session takes back the cart it has not paid. Counted like any other request of a session.
	mux.Handle("POST /v2/customer/checkout/cancel", sessions(h.customerCancel))
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

// customerChallenge gives a one-time value for the board to sign. The request was counted and read before it came
// here (CustomerEntrance.challenges). Nothing is written: the database is only asked whether the store takes
// customers, or whether the order named is one of that store, and the challenge carries the rest itself.
//
//	checkout  the store's page is published; any other store answers like a branch that does not exist
//	order, pickup  the order is one of the store named, and the store's page is published or the store already took
//	    payment for that order (being checked or paid): a customer who paid follows, answers and collects the
//	    order even after the store took its page back
//	account  no store and no order: nothing is asked of the database
func (h HTTP) customerChallenge(w http.ResponseWriter, r *http.Request) {
	in := attemptOf(r).challenge
	if in == nil {
		errorResponse(w, service.BadRequest, nil)
		return
	}
	if h.Store == nil {
		http.NotFound(w, r)
		return
	}
	subject := in.Branch
	if in.Purpose != "account" {
		var sale *string
		if in.Sale != "" {
			sale, subject = &in.Sale, in.Sale
		}
		var allowed bool
		if err := h.Store.db.QueryRow(r.Context(), "SELECT ledger.customer_challenge_allowed($1,$2,$3)", in.Branch, in.Purpose, sale).Scan(&allowed); err != nil {
			errorResponse(w, customerError(err), nil)
			return
		}
		if !allowed {
			// One answer for a branch that does not exist, a store that is not public and an order that is not its.
			h.Customer.failed(w, r, service.BadRequest)
			return
		}
	}
	now := h.Customer.now()
	expires := now.Add(min(lifetime(h.Customer.ChallengeLifetime, defaultChallengeLifetime), maxChallengeLifetime)).Truncate(time.Second)
	challenge, err := h.Customer.keys.mint(now, in.Purpose, subject, expires)
	if err != nil {
		errorResponse(w, service.Unknown, nil)
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

// accountOrdersReply is all the proof of an `account` challenge answers: how many orders of the proven account are
// unfinished, at any store. It names no order, no store and no amount.
type accountOrdersReply struct {
	Purpose    string `json:"purpose"`
	OpenOrders int    `json:"openOrders"`
}

// customerProof accepts the board's signature over one challenge and answers with an order session, a pickup token
// or, for an `account` challenge, the number of unfinished orders of the account. The request was read, its
// challenge and signature checked and the request counted before it came here (CustomerEntrance.askedProof). The
// challenge is used up here, by the database, whatever the proof then turns out to be.
func (h HTTP) customerProof(w http.ResponseWriter, r *http.Request) {
	try := attemptOf(r)
	in := try.proof
	if in == nil {
		errorResponse(w, service.BadRequest, nil)
		return
	}
	challenge, account := in.challenge, in.account
	challengeHash := sha256.Sum256(challenge[:])
	if h.Store == nil {
		try.settle(false)
		http.NotFound(w, r)
		return
	}
	if in.ticket.purpose == "account" {
		var open *int
		err := h.Store.db.QueryRow(r.Context(), "SELECT ledger.prove_account_orders($1,$2,$3)", challengeHash[:], account[:], in.ticket.expires).Scan(&open)
		try.settle(false)
		switch {
		case err != nil:
			errorResponse(w, customerError(err), nil)
		case open == nil:
			errorResponse(w, service.Expired, nil) // used before, or past its time by the database's clock
		default:
			writeJSON(w, http.StatusOK, accountOrdersReply{Purpose: "account", OpenOrders: *open})
		}
		return
	}
	token, err := freshId()
	if err != nil {
		try.settle(false)
		errorResponse(w, service.Unknown, nil)
		return
	}
	tokenHash := sha256.Sum256(token[:])
	subject := CustomerSubject(account)
	seconds := func(set, fallback time.Duration) int { return int(lifetime(set, fallback) / time.Second) }
	var outcome, purpose, branchRef string
	var sale, tenantRef *string
	var expires *time.Time
	err = h.Store.db.QueryRow(r.Context(), "SELECT outcome_,purpose_,branch_::text,sale_::text,tenant_::text,expires_ FROM ledger.prove_customer_account($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)",
		challengeHash[:], in.ticket.purpose, in.ticket.subject, in.ticket.expires, account[:], subject, in.generation, tokenHash[:],
		seconds(h.Customer.CheckoutLifetime, defaultCheckoutLifetime), seconds(h.Customer.OrderLifetime, defaultOrderLifetime),
		seconds(h.Customer.PickupLifetime, defaultPickupLifetime)).Scan(&outcome, &purpose, &branchRef, &sale, &tenantRef, &expires)
	if err != nil {
		try.settle(false)
		errorResponse(w, customerError(err), nil)
		return
	}
	started := outcome == "ok" && purpose != "pickup" && expires != nil
	try.settle(started)
	if status, refusal, refused := proofRefusal(outcome, expires); refused {
		writeJSON(w, status, refusal)
		return
	}
	switch outcome {
	case "mismatch":
		// The database says the account is not the one that paid this order: a failed attempt, counted now.
		h.Customer.failed(w, r, service.Mismatch)
		return
	case "ok":
	default:
		errorResponse(w, service.Unknown, nil)
		return
	}
	if expires == nil {
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
		// The session is known from its first request on.
		session := customerSession{Branch: branchRef, Actor: subject, Purpose: purpose, Tenant: *tenantRef, ExpiresAt: *expires}
		if sale != nil {
			session.Sale = *sale
		}
		h.Customer.sawLive(tokenHash, session)
	}
	writeJSON(w, http.StatusOK, reply)
}

// proofRefusal answers a pickup proof whose order has no QR to give, naming which case it is so the app can say it:
//
//	not_ready  409 CHECKOUT_CONFLICT: the order is still being made; the customer waits
//	collected  409 CHECKOUT_CONFLICT with recordedAt (Unix seconds): the order was already handed over then
//
// Both come only after the proof showed the account that paid the order.
func proofRefusal(outcome string, at *time.Time) (int, pickupScanRefusal, bool) {
	refusal := pickupScanRefusal{Reply: service.Reply{Status: "error", Error: service.CheckoutConflict}, Reason: outcome}
	switch outcome {
	case "not_ready":
		return http.StatusConflict, refusal, true
	case "collected":
		if at != nil {
			refusal.RecordedAt = at.Unix()
		}
		return http.StatusConflict, refusal, true
	}
	return 0, pickupScanRefusal{}, false
}

// customerSession is a live session read back from its token.
type customerSession struct {
	Branch, Actor, Purpose, Sale, Tenant string
	ExpiresAt                            time.Time
}

// What a checkout session may ask before payment, on an order of its own account at its branch: the cart, the
// customer's own claim on it, the quote, and the board's signature for the sponsored lock. C3 has no customer cancel
// (order.cancel-public is the kiosk's); a checkout session takes its unpaid cart back on a route of the entrance
// (customerCancel), and an order whose quote was issued ends with that quote.
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
	session, live := h.liveSession(w, r)
	if !live {
		return
	}
	principal := service.Principal{Tenant: session.Tenant, Subject: session.Actor, Role: "customer", ExpiresAt: session.ExpiresAt.Unix()}
	// Like a certificate route, writes commit only while the session still stands.
	w, r, cancel := authority.Bound(r, session.ExpiresAt, func() bool { return time.Now().Before(session.ExpiresAt) }, w)
	defer cancel()
	h.serveFor(w, r, principal, &session)
}

// customerCancel takes back the unpaid cart of a checkout session, so the customer can change the cart without
// waiting for the hold to run out:
//
//	POST /v2/customer/checkout/cancel   Authorization: Bearer <checkout session>   {"saleRef":"<order>"}
//	200 {"status":"ok","saleRef":"<order>","state":"closed"}   the hold is released; the same again answers the same
//	409 CHECKOUT_CONFLICT, reason "quote_issued"              a quote was issued: the order ends with that quote
//	409 CHECKOUT_CONFLICT, reason "paid"                      a payment arrived or is being checked
//	404                                                       not this account's order here, or not a checkout session
//
// It releases only an order no quote was issued for (ledger.cancel_customer_hold). A paid order is not cancelled
// here: that is a refund, which the store decides.
func (h HTTP) customerCancel(w http.ResponseWriter, r *http.Request) {
	session, live := h.liveSession(w, r)
	if !live {
		return
	}
	var in struct {
		Sale string `json:"saleRef"`
	}
	if session.Purpose != "checkout" || h.Store == nil {
		http.NotFound(w, r)
		return
	}
	if !decodeStrict(w, r, 256, &in) || !operatorRef.MatchString(in.Sale) {
		errorResponse(w, service.BadRequest, nil)
		return
	}
	principal := service.Principal{Tenant: session.Tenant, Subject: session.Actor, Role: "customer", ExpiresAt: session.ExpiresAt.Unix()}
	w, r, cancel := authority.Bound(r, session.ExpiresAt, func() bool { return time.Now().Before(session.ExpiresAt) }, w)
	defer cancel()
	var outcome string
	if err := h.Store.db.QueryRow(authority.Scope(r.Context(), principal, session.Branch), "SELECT ledger.cancel_customer_hold($1,$2,$3,$4)",
		session.Tenant, session.Branch, session.Actor, in.Sale).Scan(&outcome); err != nil {
		errorResponse(w, customerError(err), nil)
		return
	}
	switch outcome {
	case "cancelled":
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "saleRef": in.Sale, "state": "closed"})
	case "quote_issued", "paid":
		writeJSON(w, http.StatusConflict, pickupScanRefusal{Reply: service.Reply{Status: "error", Error: service.CheckoutConflict}, Reason: outcome})
	default:
		errorResponse(w, service.Unknown, nil)
	}
}

// countedSession marks a request whose session was counted from memory before it was admitted: its value is the
// key the session's own allowance was counted under.
type countedSession struct{}

// liveSession reads the live session of the request's token and counts the request for it, unless that was done
// from memory before the request was admitted. When there is none, or the session is over its limit, it has
// answered and reports false.
func (h HTTP) liveSession(w http.ResponseWriter, r *http.Request) (customerSession, bool) {
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
		return customerSession{}, false
	}
	hash := sha256.Sum256(token)
	session, err := lookup(r.Context(), token)
	if err != nil || !time.Now().Before(session.ExpiresAt) {
		h.Customer.live.drop(string(hash[:]))
		refuse()
		return customerSession{}, false
	}
	h.Customer.sawLive(hash, session)
	// A live session has its own allowance, whatever address it comes from. One that was counted from memory under
	// the same key is not counted twice.
	if counted, _ := r.Context().Value(countedSession{}).(string); counted != session.limitKey() {
		if limiter := h.Customer.SessionLimiter; limiter != nil {
			if allowed, wait := limiter.Take(session.limitKey()); !allowed {
				h.Customer.tooMany(w, "session", wait)
				return customerSession{}, false
			}
		}
	}
	return session, true
}

// pickupScanRefusal is the service's error reply with the reason a pickup QR was refused, so the operations web can
// say which. RecordedAt (Unix seconds) is the handover a used QR already recorded. A refused pickup proof has the
// same form (proofRefusal).
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
