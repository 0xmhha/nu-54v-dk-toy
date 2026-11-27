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
// is counted by address. So an allowance customers need must not be one a single sender can empty for nothing. The
// rule here: a request pays from the allowance customers use only when the entrance already has a reason to take it
// for an app's request; any other request pays from a strict allowance of failed attempts, and gets that back when
// it turns out well.
//
//	challenge  recognised when it names a store the entrance lately saw public (a checkout challenge it gave, a
//	           store page or a menu it served), or asks about the account alone
//	proof      recognised when its signature is the account's and it is over a challenge this process gave and has
//	           not seen proven
//	session    recognised when its token was given by a proof of this process or found live by an earlier lookup
//
// What a sender must do to use up the customers' allowance of an address is therefore what a customer does: ask
// for a real store, sign a challenge that was given, hold a live session. Such a sender cannot be told from
// customers; the allowances it can reach are sized for a busy store, and the ones that matter to a customer who is
// paying are counted for that customer's own account (StartLimiter, HoldAttempts) or session (SessionLimiter).
//
// The memory behind the recognition is this process's own: after a restart, or once an entry was pushed out, a
// request is counted the strict way until it turned out well once.

// NewCustomerLimits is an entrance with the request limits opsd serves customers under, each with its own allowance
// so one kind cannot use up another's. An address is an IPv4 address or an IPv6 /64; customers behind one address
// share what is counted by address, so those allowances are sized for several customers at once. now may be nil.
//
//	Limiter       challenges and proofs the entrance recognises, per address: 20 at once, then 1 a second. A
//	              checkout, an order session and a pickup QR take two requests each, so a store's network starts
//	              ten of them in one moment and thirty a minute.
//	FailLimiter   failed attempts, per address: 10 at once, then 1 every 5 seconds. An app does not fail this way
//	              (a challenge that ran out before the board answered is the exception, and is one).
//	StartLimiter  sessions an account starts, per account at its address: 6 at once, then 1 every 10 seconds. An
//	              order takes two (before and after payment). Per address the starts are bounded by Limiter: at
//	              most one every two seconds, 460 checkout sessions (15 minutes each) alive at once.
//	ReadLimiter   public reads, per address: 120 at once, then 10 a second. Opening a store is the list, the page
//	              and at most 13 menu pages (600 rows, 50 a page).
//	PhotoLimiter  menu photo parts, per address: 240 at once, then 20 a second; a part is at most 18,000 bytes.
//	AddressLimiter  requests whose session token was not lately seen live, per address, before the token is read:
//	              60 at once, then 1 a second. After a restart every session is counted here once; a made-up token
//	              is counted here every time.
//	LiveLimiter   requests of sessions lately seen live, per address: 200 at once, then 10 a second: 20 customers
//	              going from cart to lock in one moment (ten requests each), or 50 following an order.
//	SessionLimiter  a session's own requests: 40 at once, then 2 a second.
//	UnknownLimiter  session tokens the server did not know, per address, after the lookup: 10 at once, then 1
//	              every 5 seconds.
//	HoldAttempts  carts an account asks to hold, per account at its address: 6 at once, then 1 every 20 seconds.
//	              Each reads the chain.
//	HoldLimiter   carts the accounts of an address come to hold: 30 at once, then 1 every 2 seconds. Counted only
//	              for an account with the balance whose cart held stock.
func NewCustomerLimits(now func() time.Time) *CustomerEntrance {
	return &CustomerEntrance{
		Limiter: NewRateLimiter(60, 20, now), FailLimiter: NewRateLimiter(12, 10, now), StartLimiter: NewRateLimiter(6, 6, now),
		ReadLimiter: NewRateLimiter(600, 120, now), PhotoLimiter: NewRateLimiter(1200, 240, now),
		AddressLimiter: NewRateLimiter(60, 60, now), LiveLimiter: NewRateLimiter(600, 200, now),
		SessionLimiter: NewRateLimiter(120, 40, now), UnknownLimiter: NewRateLimiter(12, 10, now),
		HoldAttempts: NewRateLimiter(3, 6, now), HoldLimiter: NewRateLimiter(30, 30, now),
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

func (s *recentSet) put(key string, until time.Time, note string) {
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
		now := time.Now()
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

const (
	// publishedMemory is how long a store counts as lately seen public. The database is asked about the store on
	// every challenge whatever this says; it decides only which allowance the challenge is counted under.
	publishedMemory = 10 * time.Minute
	// liveMemory is how long a session token counts as lately seen live after a lookup found it so.
	liveMemory = 30 * time.Minute
)

// sawPublished notes that the database just answered for this store as a public one, or for this store and order
// (`branch/sale`) as one a challenge is given for.
func (c *CustomerEntrance) sawPublished(branch string) {
	if c != nil {
		c.published.put(branch, time.Now().Add(publishedMemory), "")
	}
}

// sawLive notes that the session of this token hash is live until expires.
func (c *CustomerEntrance) sawLive(hash [32]byte, expires time.Time) {
	until := time.Now().Add(liveMemory)
	if expires.Before(until) {
		until = expires
	}
	c.live.put(string(hash[:]), until, "")
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

// proofAsk is a proof request that read and whose signature is the account's.
type proofAsk struct {
	challenge  [32]byte
	account    common.Address
	generation *int64
	// purpose is what the challenge was given for, when this process remembers giving it.
	purpose string
}

// attempt is one challenge or proof as the entrance counted it before admitting it. The handler settles it once
// it knows how the request turned out.
type attempt struct {
	c       *CustomerEntrance
	address string
	// split: failed attempts have an allowance of their own (FailLimiter); without one every request was counted
	// once under Limiter and nothing is settled.
	split bool
	// prepaid: the request was not recognised, so a failed attempt was counted for it before it was admitted.
	prepaid bool
	// start is the account's key a session start was counted under, when one was.
	start     string
	challenge *challengeAsk
	proof     *proofAsk
	settled   bool
}

type attemptKey struct{}

// attemptOf is the attempt of an admitted challenge or proof; a request that did not come through attempted has an
// empty one.
func attemptOf(r *http.Request) *attempt {
	if try, ok := r.Context().Value(attemptKey{}).(*attempt); ok {
		return try
	}
	return &attempt{settled: true}
}

// settle closes the count of the attempt. honest: the request was one an app sends, whatever it was answered.
// started: it gave a session.
func (a *attempt) settle(honest, started bool) {
	if a.settled {
		return
	}
	a.settled = true
	if a.start != "" && !started {
		a.c.StartLimiter.Refund(a.start) // only a session that was given counts as a start
	}
	if !a.split {
		return
	}
	switch {
	case honest && a.prepaid:
		a.c.FailLimiter.Refund(a.address) // it was no failed attempt after all
	case !honest && a.prepaid:
		if a.c.Limiter != nil {
			a.c.Limiter.Refund(a.address) // the failed attempt was paid for; the customers' allowance is given back
		}
	case !honest:
		a.c.FailLimiter.Take(a.address) // a recognised request failed: counted now, whatever is left
	}
}

// failedAttempt answers a request that is no challenge or proof an app sends, without admitting it: counted as a
// failed attempt of its address and answered as the error it is, or refused for its rate once those ran out.
func (c *CustomerEntrance) failedAttempt(w http.ResponseWriter, r *http.Request, err error) {
	limiter, kind := c.FailLimiter, "failed"
	if limiter == nil {
		limiter, kind = c.Limiter, "proof"
	}
	if limiter != nil {
		if allowed, wait := limiter.Take(clientKey(r)); !allowed {
			c.tooMany(w, kind, wait)
			return
		}
	}
	errorResponse(w, err, nil)
}

// count takes the allowances of one challenge or proof that read. recognised: the entrance has a reason to take it
// for an app's request. start: the account's key when the request may give a session. It reports false when the
// request was refused for its rate, which it has then answered.
func (c *CustomerEntrance) count(w http.ResponseWriter, r *http.Request, try *attempt, recognised bool, start string) bool {
	try.c, try.address = c, clientKey(r)
	try.split = c.FailLimiter != nil
	if try.split && !recognised {
		if allowed, wait := c.FailLimiter.Take(try.address); !allowed {
			c.tooMany(w, "failed", wait)
			return false
		}
		try.prepaid = true
	}
	back := func() {
		if try.prepaid {
			c.FailLimiter.Refund(try.address)
		}
	}
	if c.Limiter != nil {
		if allowed, wait := c.Limiter.Take(try.address); !allowed {
			back()
			c.tooMany(w, "proof", wait)
			return false
		}
	}
	if start != "" && c.StartLimiter != nil {
		if allowed, wait := c.StartLimiter.Take(start); !allowed {
			back()
			if c.Limiter != nil {
				c.Limiter.Refund(try.address) // refused for the account's own starts, not for the address
			}
			c.tooMany(w, "start", wait)
			return false
		}
		try.start = start
	}
	return true
}

// askedChallenge reads a challenge request and counts it. A request that does not read is a failed attempt.
func (c *CustomerEntrance) askedChallenge(w http.ResponseWriter, r *http.Request) (*attempt, bool) {
	var in challengeAsk
	if !decodeStrict(w, r, 512, &in) || !slices.Contains([]string{"checkout", "order", "pickup", "account"}, in.Purpose) ||
		(in.Purpose == "account") != (in.Branch == "") || (in.Branch != "" && !operatorRef.MatchString(in.Branch)) ||
		(in.Purpose == "checkout" || in.Purpose == "account") != (in.Sale == "") || (in.Sale != "" && !operatorRef.MatchString(in.Sale)) {
		c.failedAttempt(w, r, service.BadRequest)
		return nil, false
	}
	try := &attempt{challenge: &in}
	// An order or pickup challenge is also recognised by the order it names, once the database took the two together.
	recognised := in.Purpose == "account" || c.published.has(in.Branch, time.Now()) ||
		(in.Sale != "" && c.published.has(in.Branch+"/"+in.Sale, time.Now()))
	return try, c.count(w, r, try, recognised, "")
}

// askedProof reads a proof request, checks its signature and counts it. A request that does not read, or whose
// signature does not recover to the account it names, is a failed attempt and leaves the challenge unused: only
// its own board can spend it.
func (c *CustomerEntrance) askedProof(w http.ResponseWriter, r *http.Request) (*attempt, bool) {
	var in struct {
		Challenge  string `json:"challenge"`
		Account    string `json:"account"`
		Signature  string `json:"signature"`
		Generation string `json:"generation,omitempty"`
	}
	if !decodeStrict(w, r, 1024, &in) || !customerHex32.MatchString(in.Challenge) || !customerAccount.MatchString(in.Account) {
		c.failedAttempt(w, r, service.BadRequest)
		return nil, false
	}
	signature, err := hex.DecodeString(in.Signature)
	if err != nil || len(signature) != 65 || hex.EncodeToString(signature) != in.Signature {
		c.failedAttempt(w, r, service.BadRequest)
		return nil, false
	}
	asked := proofAsk{account: common.HexToAddress(in.Account)}
	if in.Generation != "" {
		value, err := strconv.ParseInt(in.Generation, 10, 64)
		if err != nil || value <= 0 || strconv.FormatInt(value, 10) != in.Generation {
			c.failedAttempt(w, r, service.BadRequest)
			return nil, false
		}
		asked.generation = &value
	}
	raw, _ := hex.DecodeString(in.Challenge)
	asked.challenge = [32]byte(raw)
	if !verifyAccountProof(asked.account, asked.challenge, signature) {
		c.failedAttempt(w, r, service.Mismatch)
		return nil, false
	}
	hash := sha256.Sum256(asked.challenge[:])
	purpose, recognised := c.issued.get(string(hash[:]), time.Now())
	asked.purpose = purpose
	// A session start is counted for the account at its address. A challenge this process does not remember may be
	// for a session too; the count is given back when it was not.
	start := ""
	if !recognised || purpose == "checkout" || purpose == "order" {
		start = clientKey(r) + "/" + strings.ToLower(asked.account.Hex())
	}
	try := &attempt{proof: &asked}
	return try, c.count(w, r, try, recognised, start)
}

// attempted serves a challenge or a proof: its body is read and the request is counted (ask) before it is admitted,
// so what a request is decides which allowance it uses, and a refused one takes no place among the requests served
// at once and never enters the gate.
func (c *CustomerEntrance) attempted(ask func(http.ResponseWriter, *http.Request) (*attempt, bool), next http.HandlerFunc) http.Handler {
	admitted := c.admitted(next)
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
		try, allowed := ask(w, r)
		if !allowed {
			return
		}
		// Whatever happens to the request after this, its count is closed: one that never reached its handler
		// (no place among the requests served at once) was an app's request and gave no session.
		defer try.settle(true, false)
		admitted.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), attemptKey{}, try)))
	})
}
