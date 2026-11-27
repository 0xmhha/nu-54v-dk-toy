package commerce

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// The request limits of the customer entrance.
//
// Customers behind one address (a store's wireless network, a carrier's shared address) share every allowance that
// is counted by address, and a sender behind the same address can use up any of them that it reaches for nothing.
// So the allowances are laid out by what a request has shown of itself when it is counted:
//
//	nothing yet       a challenge request, and a proof request before it is read. Each has a wide allowance of
//	                  its address (IssueLimiter, AttemptLimiter) that bounds the work the server does unasked: one
//	                  read of the store for a challenge, one body and one signature for a proof. A challenge writes
//	                  nothing and remembers nothing (customer_challenge.go).
//	a failed attempt  a body that does not read, a challenge this server did not give, a signature that is not the
//	                  account's, a store that is not public, an order the account did not pay. It is counted under
//	                  FailLimiter after it failed, and nothing a customer sends is counted there before it was
//	                  looked at: an address whose failed attempts are used up is told to wait instead of being told
//	                  what was wrong, and that is all.
//	an account        a proof with the account's own signature over a challenge of this server is counted for that
//	                  account at its address (AccountLimiter, and StartLimiter for the sessions it starts). One
//	                  account cannot use up another's.
//	a session         a request with a session token the entrance lately saw live is counted for that session first
//	                  (SessionLimiter), from memory; a request its own session refuses is not counted for the
//	                  address.
//
// What the customers of an address still share is small and is counted only for requests that passed the above:
// Limiter (proofs that start a checkout session or ask about an account; a proof for an order the account paid is
// not under it), LiveLimiter (requests of checkout sessions; the session of a paid order is not under it) and
// HoldLimiter (carts that held stock). A sender with many keys, each signing like a customer, cannot be told from
// as many customers and can still use those three up: one new key every two seconds holds the session starts of its
// address at their ceiling, and five unpaid sessions hold the checkout requests there. The customers who already
// paid are not held back by either.
//
// The memory of live sessions is this process's own: after a restart, or once an entry was pushed out, a session
// is counted once like a token the server does not know (AddressLimiter) until its lookup found it live.

// NewCustomerLimits is an entrance with the request limits opsd serves customers under, each with its own allowance
// so one kind cannot use up another's. An address is an IPv4 address or an IPv6 /64. now may be nil.
//
//	IssueLimiter    challenge requests, per address: 300 at once, then 20 a second. A checkout, an order session and
//	                a pickup QR take one each.
//	AttemptLimiter  proof requests, per address, before one is read: 300 at once, then 20 a second.
//	FailLimiter     failed attempts, per address: 10 at once, then 1 every 5 seconds, counted after the attempt
//	                failed. An app does not fail this way.
//	AccountLimiter  proofs with the account's own signature, per account at its address: 10 at once, then 1 every
//	                5 seconds. An order takes three or four (checkout, order session, pickup QR, and the question
//	                before a board is reset).
//	Limiter         proofs that start a checkout session or ask about an account, per address: 20 at once, then 1
//	                every 2 seconds. It bounds what proofs write for an address: at most 470 checkout sessions (15
//	                minutes each) alive at once and 43,200 new accounts a day.
//	StartLimiter    sessions an account starts, per account at its address: 6 at once, then 1 every 10 seconds. An
//	                order takes two (before and after payment).
//	ReadLimiter     public reads and the health route, per address: 120 at once, then 10 a second. Opening a store is
//	                the list, the page and at most 13 menu pages (600 rows, 50 a page).
//	PhotoLimiter    menu photo parts, per address: 240 at once, then 20 a second; a part is at most 18,000 bytes.
//	AddressLimiter  requests whose session token was not lately seen live, per address, before the token is read:
//	                60 at once, then 1 a second. After a restart every session is counted here once; a made-up token
//	                is counted here every time.
//	SessionLimiter  a session's own requests: 40 at once, then 2 a second, counted before anything of its address.
//	LiveLimiter     requests of checkout sessions lately seen live, per address, once the session's own allowance
//	                let them through: 200 at once, then 10 a second: 20 customers going from cart to lock in one
//	                moment (ten requests each). The session of a paid order is not counted here.
//	UnknownLimiter  session tokens the server did not know, per address, after the lookup: 10 at once, then 1
//	                every 5 seconds.
//	HoldAttempts    carts an account asks to hold, per account at its address: 6 at once, then 1 every 20 seconds.
//	                Each reads the chain.
//	HoldLimiter     carts the accounts of an address come to hold: 30 at once, then 1 every 2 seconds. Counted only
//	                for an account with the balance whose cart held stock.
func NewCustomerLimits(now func() time.Time) *CustomerEntrance {
	return &CustomerEntrance{
		IssueLimiter: NewRateLimiter(1200, 300, now), AttemptLimiter: NewRateLimiter(1200, 300, now),
		FailLimiter: NewRateLimiter(12, 10, now), AccountLimiter: NewRateLimiter(12, 10, now),
		Limiter: NewRateLimiter(30, 20, now), StartLimiter: NewRateLimiter(6, 6, now),
		ReadLimiter: NewRateLimiter(600, 120, now), PhotoLimiter: NewRateLimiter(1200, 240, now),
		AddressLimiter: NewRateLimiter(60, 60, now), LiveLimiter: NewRateLimiter(600, 200, now),
		SessionLimiter: NewRateLimiter(120, 40, now), UnknownLimiter: NewRateLimiter(12, 10, now),
		HoldAttempts: NewRateLimiter(3, 6, now), HoldLimiter: NewRateLimiter(30, 30, now),
		clock: now,
	}
}

// RateLimiter is a token bucket per client key. It keeps a bounded number of keys; a new key always gets a place.
type RateLimiter struct {
	mu      sync.Mutex
	perSec  float64
	burst   float64
	now     func() time.Time
	clients map[string]*bucket
	// Keys is the most keys kept at once; zero keeps limiterClients.
	Keys int
}

type bucket struct {
	tokens float64
	at     time.Time
}

const (
	limiterClients = 10_000
	// limiterSample is how many keys are looked at to make room for a new one.
	limiterSample = 32
)

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
	b, ok := l.clients[key]
	if !ok {
		limit := l.Keys
		if limit <= 0 {
			limit = limiterClients
		}
		if len(l.clients) >= limit {
			l.evict(now)
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

// Refund gives a client back one request it was counted for.
func (l *RateLimiter) Refund(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if b, ok := l.clients[key]; ok {
		b.tokens = min(l.burst, b.tokens+1)
	}
}

// evict makes room for one new key when the table is full. It looks at a few keys (a map is walked in no fixed
// order) and forgets those whose bucket is full again: forgetting them changes nothing. When none is, it forgets
// the one that asked longest ago. A full table therefore never refuses a newcomer, and a client that keeps asking
// while it is held back stays held back; what is given up is the count of a client that went quiet, which would
// have had its allowance back after a while anyway. A sender that fills the table with keys of its own gains one
// burst per key, as it would with a table of any size.
func (l *RateLimiter) evict(now time.Time) {
	var quietest string
	var asked time.Time
	seen, freed := 0, false
	for key, b := range l.clients {
		if b.tokens+now.Sub(b.at).Seconds()*l.perSec >= l.burst {
			delete(l.clients, key)
			freed = true
		} else if quietest == "" || b.at.Before(asked) {
			quietest, asked = key, b.at
		}
		if seen++; seen >= limiterSample {
			break
		}
	}
	if !freed && quietest != "" {
		delete(l.clients, quietest)
	}
}

// recentSet remembers a bounded number of keys until a time each. The zero value is ready.
type recentSet struct {
	mu      sync.Mutex
	entries map[string]recentEntry
	// keys is the most entries kept; zero keeps recentKeys.
	keys int
}

type recentEntry struct {
	until time.Time
	note  string
}

const recentKeys = 16_384

// put remembers key until a time, with a note. now is the time entries are measured against when one has to go.
func (s *recentSet) put(key string, now, until time.Time, note string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = map[string]recentEntry{}
	}
	limit := s.keys
	if limit <= 0 {
		limit = recentKeys
	}
	if _, known := s.entries[key]; !known && len(s.entries) >= limit {
		// Entries past their time go first; when none of a few is, the one that ends soonest goes.
		var soonest string
		var ends time.Time
		seen, freed := 0, false
		for k, entry := range s.entries {
			if !entry.until.After(now) {
				delete(s.entries, k)
				freed = true
			} else if soonest == "" || entry.until.Before(ends) {
				soonest, ends = k, entry.until
			}
			if seen++; seen >= limiterSample {
				break
			}
		}
		if !freed {
			delete(s.entries, soonest)
		}
	}
	s.entries[key] = recentEntry{until: until, note: note}
}

func (s *recentSet) get(key string, now time.Time) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[key]
	if !ok {
		return "", false
	}
	if !entry.until.After(now) {
		delete(s.entries, key)
		return "", false
	}
	return entry.note, true
}

func (s *recentSet) has(key string, now time.Time) bool {
	_, ok := s.get(key, now)
	return ok
}

func (s *recentSet) drop(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
}

// liveMemory is how long a session token counts as lately seen live after a lookup found it so.
const liveMemory = 30 * time.Minute

// now is the entrance's clock: the one its limits were built with, or the system's.
func (c *CustomerEntrance) now() time.Time {
	if c.clock != nil {
		return c.clock()
	}
	return time.Now()
}

// limitKey is the key a session's own requests are counted under.
func (s customerSession) limitKey() string {
	return s.Actor + "/" + s.Branch + "/" + s.Purpose + "/" + s.Sale
}

// sawLive notes that the session of this token hash is live, so its next request is counted for the session before
// anything else. What is remembered is the session's purpose and the key of its own allowance.
func (c *CustomerEntrance) sawLive(hash [32]byte, session customerSession) {
	now := c.now()
	// The session ends by the database's clock; what is left of it is measured on the entrance's own.
	remembered := liveMemory
	if left := time.Until(session.ExpiresAt); left < remembered {
		remembered = left
	}
	c.live.put(string(hash[:]), now, now.Add(remembered), session.Purpose+":"+session.limitKey())
}

// bearerHash is the SHA-256 of the request's session token, when it carries one of the right form.
func bearerHash(r *http.Request) ([32]byte, bool) {
	value, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	token, err := hex.DecodeString(value)
	if !found || err != nil || len(token) != 32 || hex.EncodeToString(token) != value {
		return [32]byte{}, false
	}
	return sha256.Sum256(token), true
}

// challengeAsk is a challenge request that read.
type challengeAsk struct {
	Purpose string `json:"purpose"`
	Branch  string `json:"branchRef,omitempty"`
	Sale    string `json:"saleRef,omitempty"`
}

// proofAsk is a proof request that read, over a challenge of this server and with the account's own signature.
type proofAsk struct {
	challenge  [32]byte
	account    common.Address
	generation *int64
	// ticket is what the challenge was given for.
	ticket ticket
}

// attempt is one challenge or proof the entrance let through. For a proof it holds what was counted, so the
// handler can settle it once it knows how the request turned out.
type attempt struct {
	c         *CustomerEntrance
	challenge *challengeAsk
	proof     *proofAsk
	// counted is every allowance the proof took, in the order it took them.
	counted []counted
	// start is the account's key a session start was counted under, when one was.
	start string
	// reached: the request came to its handler. One that did not (no place among the requests served at once)
	// gets everything back.
	reached bool
	settled bool
}

type counted struct {
	limiter *RateLimiter
	key     string
}

type attemptKey struct{}

// attemptOf is the attempt of a challenge or proof that was let through; a request that did not come through the
// entrance's own routes has an empty one.
func attemptOf(r *http.Request) *attempt {
	if try, ok := r.Context().Value(attemptKey{}).(*attempt); ok {
		try.reached = true
		return try
	}
	return &attempt{settled: true}
}

// take counts the proof under one more allowance. When that one refuses, everything counted before is given back
// and the request is answered: it reports false.
func (a *attempt) take(w http.ResponseWriter, limiter *RateLimiter, key, kind string) bool {
	if limiter == nil {
		return true
	}
	if allowed, wait := limiter.Take(key); !allowed {
		a.refund()
		a.c.tooMany(w, kind, wait)
		return false
	}
	a.counted = append(a.counted, counted{limiter, key})
	return true
}

func (a *attempt) refund() {
	for _, one := range a.counted {
		one.limiter.Refund(one.key)
	}
	a.counted, a.start = nil, ""
}

// settle closes the count of a proof. started: it gave a session; only that counts as a session start.
func (a *attempt) settle(started bool) {
	if a.settled {
		return
	}
	a.settled = true
	if !a.reached {
		a.refund()
		return
	}
	if a.start != "" && !started && a.c.StartLimiter != nil {
		a.c.StartLimiter.Refund(a.start)
	}
}

// failed answers a failed attempt: a request no app sends, or one the database refused for what it named. It is
// counted for its address now that it failed, and answered as the error it is while the address has failed
// attempts left; after that the address is told to wait and nothing else.
func (c *CustomerEntrance) failed(w http.ResponseWriter, r *http.Request, err error) {
	if c.FailLimiter != nil {
		if allowed, wait := c.FailLimiter.Take(clientKey(r)); !allowed {
			c.tooMany(w, "failed", wait)
			return
		}
	}
	errorResponse(w, err, nil)
}

// arrived counts a challenge or proof request for its address before anything of it is read, under the wide
// allowance of its kind. A deployment that set only Limiter counts both there, and then nothing is counted there a
// second time (the second result).
func (c *CustomerEntrance) arrived(w http.ResponseWriter, r *http.Request, wide *RateLimiter, kind string) (bool, bool) {
	fallback := wide == nil
	if fallback {
		wide, kind = c.Limiter, "proof"
	}
	if wide != nil {
		if allowed, wait := wide.Take(clientKey(r)); !allowed {
			c.tooMany(w, kind, wait)
			return false, fallback
		}
	}
	return true, fallback
}

// readBody reads the whole body before anything else is done with the request, so a client that sends slowly
// holds no place among the requests served at once and never enters the gate. One byte past the longest body is
// kept, so a longer one still fails its route's own length check.
func readBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil || r.Body == http.NoBody {
		return true
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, entranceBody+1))
	if err != nil {
		errorResponse(w, service.BadRequest, nil) // the client stopped sending
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return true
}

// challenges serves the challenge route. The request is counted for its address, read and checked for its form
// before it is let through; an `account` challenge names no store, so it reads no database and takes no place
// among the requests served at once.
func (c *CustomerEntrance) challenges(next http.HandlerFunc) http.Handler {
	admitted := c.admitted(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowed, _ := c.arrived(w, r, c.IssueLimiter, "issue"); !allowed || !readBody(w, r) {
			return
		}
		var in challengeAsk
		if !decodeStrict(w, r, 512, &in) || !slices.Contains([]string{"checkout", "order", "pickup", "account"}, in.Purpose) ||
			(in.Purpose == "account") != (in.Branch == "") || (in.Branch != "" && !operatorRef.MatchString(in.Branch)) ||
			(in.Purpose == "checkout" || in.Purpose == "account") != (in.Sale == "") || (in.Sale != "" && !operatorRef.MatchString(in.Sale)) {
			c.failed(w, r, service.BadRequest)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), attemptKey{}, &attempt{c: c, challenge: &in, settled: true}))
		if in.Purpose == "account" {
			next(w, r)
			return
		}
		admitted.ServeHTTP(w, r)
	})
}

// askedProof reads a proof request and counts it. Nothing a customer shares is counted before the request showed
// that it is a proof: the challenge is one this server gave, and the signature over it is the account's. Then the
// proof is counted for that account at its address, and only a proof that starts a checkout session or asks about
// an account is counted for the address as well. shared is false when the address was already counted under
// Limiter as the request arrived.
func (c *CustomerEntrance) askedProof(w http.ResponseWriter, r *http.Request, shared bool) (*attempt, bool) {
	var in struct {
		Challenge  string `json:"challenge"`
		Account    string `json:"account"`
		Signature  string `json:"signature"`
		Generation string `json:"generation,omitempty"`
	}
	if !decodeStrict(w, r, 1024, &in) || !customerHex32.MatchString(in.Challenge) || !customerAccount.MatchString(in.Account) {
		c.failed(w, r, service.BadRequest)
		return nil, false
	}
	signature, err := hex.DecodeString(in.Signature)
	if err != nil || len(signature) != 65 || hex.EncodeToString(signature) != in.Signature {
		c.failed(w, r, service.BadRequest)
		return nil, false
	}
	asked := proofAsk{account: common.HexToAddress(in.Account)}
	if in.Generation != "" {
		value, err := strconv.ParseInt(in.Generation, 10, 64)
		if err != nil || value <= 0 || strconv.FormatInt(value, 10) != in.Generation {
			c.failed(w, r, service.BadRequest)
			return nil, false
		}
		asked.generation = &value
	}
	raw, _ := hex.DecodeString(in.Challenge)
	asked.challenge = [32]byte(raw)
	now := c.now()
	// The tag is one hash; the signature is a curve operation. The cheaper check comes first.
	given, genuine := c.keys.open(now, asked.challenge)
	if !genuine {
		c.failed(w, r, service.Expired) // not a challenge of this server: answered like one that ran out
		return nil, false
	}
	if !verifyAccountProof(asked.account, asked.challenge, signature) {
		c.failed(w, r, service.Mismatch) // the challenge is left unused: only the account's own board can spend it
		return nil, false
	}
	asked.ticket = given
	try := &attempt{c: c, proof: &asked}
	account := clientKey(r) + "/" + strings.ToLower(asked.account.Hex())
	if !try.take(w, c.AccountLimiter, account, "account") {
		return nil, false
	}
	if !now.Before(given.expires) {
		// The account's own challenge ran out before its board answered: the app asks for another.
		errorResponse(w, service.Expired, nil)
		return nil, false
	}
	if shared && (given.purpose == "checkout" || given.purpose == "account") && !try.take(w, c.Limiter, clientKey(r), "proof") {
		return nil, false
	}
	if given.purpose == "checkout" || given.purpose == "order" {
		if !try.take(w, c.StartLimiter, account, "start") {
			return nil, false
		}
		if c.StartLimiter != nil {
			try.start = account
		}
	}
	return try, true
}

// proofs serves the proof route: the request is counted for its address, read, and counted for what it is
// (askedProof) before it is let through.
func (c *CustomerEntrance) proofs(next http.HandlerFunc) http.Handler {
	admitted := c.admitted(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, fallback := c.arrived(w, r, c.AttemptLimiter, "attempt")
		if !allowed || !readBody(w, r) {
			return
		}
		try, allowed := c.askedProof(w, r, !fallback)
		if !allowed {
			return
		}
		// Whatever happens to the request after this, its count is closed: one that never reached its handler
		// gave no session.
		defer try.settle(false)
		admitted.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), attemptKey{}, try)))
	})
}
