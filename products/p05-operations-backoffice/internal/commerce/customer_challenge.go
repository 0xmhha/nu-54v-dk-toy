package commerce

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"sync"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// A challenge of the customer entrance is 32 bytes the server reads back without having kept anything. Giving one
// writes no row and remembers nothing, so asking for challenges, which costs a sender nothing, costs the server one
// read of the store and the customers of the sender's address nothing they need.
//
//	byte  0      what it is for: 1 checkout, 2 order, 3 pickup, 4 account
//	bytes 1-4    when it runs out, in Unix seconds
//	bytes 5-20   the store of a checkout; the order of an order session or a pickup QR (the server checked that the
//	             order is the named store's before it gave the challenge); 16 random bytes for an `account` challenge
//	bytes 21-23  3 random bytes, so two challenges of one second for one store differ
//	bytes 24-31  the first 8 bytes of HMAC-SHA-256 over bytes 0-23, under a key only this process holds
//
// The board signs the 32 bytes as they are (AccountProof(address account,bytes32 challenge), unchanged), so its
// signature now covers the purpose, the store or order and the expiry as well.
//
// The key is random, made when the process starts, and replaced every challengeKeyLife; the one before is kept for
// as long again, so a challenge given just before a change still reads. A restart forgets both keys: a challenge
// given before it is answered as one that ran out, and the app asks for another. One process serves the entrance,
// as its request limits already assume; several would have to share the key.
//
// The 8 bytes cannot be guessed in a lifetime of requests (one guess in 2^64 passes, and each guess is one proof
// request, counted like any other). A guess that passed would give its sender a challenge of its own choosing,
// which it has from the server for the asking anyway, with a later expiry at most.
//
// One use: a challenge is used up when its proof is taken, by a row of its hash in ledger.customer_challenge
// (ledger.use_customer_challenge, migration 126). That row is written only for a proof with the account's own
// signature over a challenge of this server.

const (
	challengeCheckout byte = 1 + iota
	challengeOrder
	challengePickup
	challengeAccount
)

var challengePurposes = map[string]byte{"checkout": challengeCheckout, "order": challengeOrder, "pickup": challengePickup, "account": challengeAccount}

// challengeKeyLife is how long one key signs new challenges. It is longer than any challenge lives (at most five
// minutes, ledger.use_customer_challenge), so the key before the current one covers every challenge still valid.
const challengeKeyLife = 10 * time.Minute

// ticket is what a challenge says once its tag was checked.
type ticket struct {
	purpose string
	// subject is the store of a checkout or the order of an order session or a pickup QR, as a UUID; empty for an
	// `account` challenge.
	subject string
	expires time.Time
}

// challengeKeys holds the key that signs new challenges and the one before it. The zero value is ready.
type challengeKeys struct {
	mu                sync.Mutex
	current, previous []byte
	since             time.Time
}

// keys returns the current key and the one before it (nil when there is none), changing them when it is time.
func (k *challengeKeys) keys(now time.Time) ([]byte, []byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if age := now.Sub(k.since); k.current == nil || age >= challengeKeyLife || age < 0 {
		fresh := make([]byte, 32)
		if _, err := rand.Read(fresh); err != nil {
			return nil, nil, err
		}
		// A key older than two lifetimes signs nothing that is still valid.
		if k.current != nil && age >= 0 && age < 2*challengeKeyLife {
			k.previous = k.current
		} else {
			k.previous = nil
		}
		k.current, k.since = fresh, now
	}
	return k.current, k.previous, nil
}

func challengeTag(key []byte, head []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("nu54-customer-challenge-v1\x00"))
	mac.Write(head)
	return mac.Sum(nil)[:8]
}

func uuidText(raw []byte) string {
	text := hex.EncodeToString(raw)
	return text[:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:]
}

// mint makes a challenge of purpose for subject (a UUID, or empty for `account`) that runs out at expires.
func (k *challengeKeys) mint(now time.Time, purpose, subject string, expires time.Time) ([32]byte, error) {
	var out [32]byte
	code, known := challengePurposes[purpose]
	seconds := expires.Unix()
	if !known || seconds <= 0 || seconds > 0xffffffff {
		return out, service.BadRequest
	}
	out[0] = code
	binary.BigEndian.PutUint32(out[1:5], uint32(seconds))
	if code == challengeAccount {
		if _, err := rand.Read(out[5:21]); err != nil {
			return [32]byte{}, err
		}
	} else {
		if len(subject) != 36 {
			return [32]byte{}, service.BadRequest
		}
		raw, err := hex.DecodeString(subject[:8] + subject[9:13] + subject[14:18] + subject[19:23] + subject[24:])
		if err != nil || len(raw) != 16 {
			return [32]byte{}, service.BadRequest
		}
		copy(out[5:21], raw)
	}
	if _, err := rand.Read(out[21:24]); err != nil {
		return [32]byte{}, err
	}
	key, _, err := k.keys(now)
	if err != nil {
		return [32]byte{}, err
	}
	copy(out[24:], challengeTag(key, out[:24]))
	return out, nil
}

// open reads a challenge this process gave. It reports false for anything else: another purpose code, or a tag
// that is of neither key. It does not look at the expiry; the caller does, once it knows whose proof this is.
func (k *challengeKeys) open(now time.Time, challenge [32]byte) (ticket, bool) {
	current, previous, err := k.keys(now)
	if err != nil {
		return ticket{}, false
	}
	matched := subtle.ConstantTimeCompare(challengeTag(current, challenge[:24]), challenge[24:])
	if previous != nil {
		matched |= subtle.ConstantTimeCompare(challengeTag(previous, challenge[:24]), challenge[24:])
	}
	if matched != 1 {
		return ticket{}, false
	}
	out := ticket{expires: time.Unix(int64(binary.BigEndian.Uint32(challenge[1:5])), 0)}
	for name, code := range challengePurposes {
		if code == challenge[0] {
			out.purpose = name
		}
	}
	if out.purpose == "" {
		return ticket{}, false
	}
	if challenge[0] != challengeAccount {
		out.subject = uuidText(challenge[5:21])
	}
	return out, true
}
