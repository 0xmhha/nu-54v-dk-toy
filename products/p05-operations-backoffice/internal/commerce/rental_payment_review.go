package commerce

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type RentalReviewChallenge struct {
	Request   string `json:"request"`
	Challenge string `json:"challenge"`
}

var reviewRequest = regexp.MustCompile("^[0-9a-f]{32}$")
var reviewChallenge = regexp.MustCompile("^[0-9a-f]{64}$")

func (r RentalReviewChallenge) valid() bool {
	return reviewRequest.MatchString(r.Request) && strings.Trim(r.Request, "0") != "" &&
		reviewChallenge.MatchString(r.Challenge) && strings.Trim(r.Challenge, "0") != ""
}

type RentalReviewCommissioning struct {
	BoardPin    [32]byte
	TerminalPin [32]byte
	Signer      [65]byte
	ExpiresAt   time.Time
}

// RentalReviewLookup resolves current commissioning for the exact renter
// and generation in a verified v2 deployment.
type RentalReviewLookup func(context.Context, string, Profile, []byte, common.Address, uint64, *big.Int) (RentalReviewCommissioning, bool)

type RentalReviewPresentation struct {
	RentalReviewChallenge
	BoardPin    string `json:"boardPin"`
	TerminalPin string `json:"terminalPin"`
	Signer      string `json:"signer"`
	ExpiresAt   int64  `json:"expiresAt"`
	Signature   string `json:"signature"`
}

func signRentalReview(key ed25519.PrivateKey, scope, signed []byte, now time.Time, escrowUntil int64,
	challenge RentalReviewChallenge, commissioned RentalReviewCommissioning) *RentalReviewPresentation {
	until := min(escrowUntil, commissioned.ExpiresAt.Unix())
	if !challenge.valid() || until <= now.Unix() || commissioned.BoardPin == ([32]byte{}) ||
		commissioned.TerminalPin == ([32]byte{}) || commissioned.BoardPin == commissioned.TerminalPin {
		return nil
	}
	x, y := elliptic.Unmarshal(elliptic.P256(), commissioned.Signer[:])
	if x == nil {
		return nil
	}
	spki, err := x509.MarshalPKIXPublicKey(&ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y})
	if err != nil || sha256.Sum256(spki) != commissioned.BoardPin {
		return nil
	}
	reply := &RentalReviewPresentation{
		RentalReviewChallenge: challenge,
		BoardPin:              hex.EncodeToString(commissioned.BoardPin[:]), TerminalPin: hex.EncodeToString(commissioned.TerminalPin[:]),
		Signer: hex.EncodeToString(commissioned.Signer[:]), ExpiresAt: until,
	}
	scopeHash, quoteHash := sha256.Sum256(scope), sha256.Sum256(signed)
	body := append([]byte("NU54-rental-payment-review-authority-v1\x00"), scopeHash[:]...)
	body = append(body, quoteHash[:]...)
	body = append(body, commissioned.BoardPin[:]...)
	body = append(body, commissioned.TerminalPin[:]...)
	body = append(body, commissioned.Signer[:]...)
	request, _ := hex.DecodeString(challenge.Request)
	nonce, _ := hex.DecodeString(challenge.Challenge)
	body = append(body, request...)
	body = append(body, nonce...)
	body = binary.BigEndian.AppendUint64(body, uint64(until))
	reply.Signature = hex.EncodeToString(ed25519.Sign(key, body))
	return reply
}
