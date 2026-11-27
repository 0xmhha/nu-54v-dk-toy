package evidencev2

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
	"github.com/ethereum/go-ethereum/common"
)

const GenerationRoute = "POST " + rentalv1.ReconcilePath

type GenerationGrant struct {
	Stream    string
	Branch    string
	Board     string
	ExpiresAt time.Time
	// RequestedGeneration and ClientFingerprint are populated only from the
	// parsed projection and the verified TLS peer, never from the grant file.
	RequestedGeneration uint64 `json:"-"`
	ClientFingerprint   string `json:"-"`
}

type GenerationFreezeVerifier interface {
	VerifyGenerationFreeze(context.Context, GenerationGrant, []byte) (chainview.VerifiedGenerationFreeze, error)
}

type generationReleaseConfirmer interface {
	ConfirmGenerationAssessment(context.Context, chainview.GenerationAssessment) bool
}

type GenerationReader interface {
	ReconcileGeneration(context.Context, []byte, chainview.VerifiedGenerationFreeze, *rentalv1.Anchor, func() time.Time) chainview.GenerationAssessment
}

// GenerationHandler is a separate mTLS adapter. Its verifier is the registered
// board-authority boundary and must validate the full freeze and challenge.
// The existing public explorer and C7 service never mount it implicitly.
func GenerationHandler(
	lookup func(string) (GenerationGrant, bool),
	verifier GenerationFreezeVerifier,
	read func(string) (GenerationReader, bool),
	clock func() time.Time,
) http.Handler {
	mux := http.NewServeMux()
	if lookup == nil || verifier == nil || read == nil || clock == nil {
		return mux
	}
	mux.HandleFunc(GenerationRoute, func(w http.ResponseWriter, r *http.Request) {
		grant, deadline, ok := readGenerationGrant(r, lookup, clock())
		if !ok {
			http.NotFound(w, r)
			return
		}
		reader, ok := read(grant.Stream)
		if !ok {
			http.NotFound(w, r)
			return
		}
		var body rentalv1.Request
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || decoder.Decode(new(any)) != io.EOF ||
			len(body.Projection) > chainview.MaxGenerationProjectionBytes || len(body.Receipt) == 0 || len(body.Receipt) > 8192 || (body.Previous != nil && !body.Previous.Valid()) {
			write(w, http.StatusBadRequest, chainview.GenerationAssessment{Status: "unknown"})
			return
		}
		binding, root, err := chainview.ParseGenerationProjection(body.Projection)
		if err != nil {
			write(w, http.StatusBadRequest, chainview.GenerationAssessment{Status: "unknown"})
			return
		}
		if binding.Branch != grant.Branch || binding.Board != grant.Board {
			http.NotFound(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeMin(deadline, clock().Add(15*time.Second)).Sub(clock()))
		defer cancel()
		authorityGrant := grant
		authorityGrant.RequestedGeneration = binding.Generation
		freeze, err := verifier.VerifyGenerationFreeze(ctx, authorityGrant, body.Receipt)
		reply := chainview.GenerationAssessment{Status: "unknown"}
		if err == nil && freeze.Root == root && freeze.Binding == binding && freeze.ValidUntil.After(clock()) {
			scoped := freeze
			scoped.ValidUntil = timeMin(freeze.ValidUntil, deadline)
			reply = reader.ReconcileGeneration(ctx, body.Projection, scoped, body.Previous, clock)
			if reply.PendingCount != nil {
				// The board or challenge may have been revoked during chain reads.
				currentFreeze, confirmErr := verifier.VerifyGenerationFreeze(ctx, authorityGrant, body.Receipt)
				if confirmErr != nil || currentFreeze != freeze {
					reply = chainview.GenerationAssessment{Status: "unknown"}
				} else if confirmer, ok := reader.(generationReleaseConfirmer); ok && !confirmer.ConfirmGenerationAssessment(ctx, reply) {
					reply = chainview.GenerationAssessment{Status: "incomplete"}
				}
			}
		}
		if !body.IncludePendingPaymentEntries {
			reply.PendingPaymentEntries = nil
		}
		current, _, ok := readGenerationGrant(r, lookup, clock())
		if !ok || current != grant {
			http.NotFound(w, r)
			return
		}
		if ctx.Err() != nil || (reply.ValidUntil != 0 && reply.ValidUntil <= clock().Unix()) {
			reply = chainview.GenerationAssessment{Status: "incomplete"}
		}
		write(w, http.StatusOK, reply)
	})
	return mux
}

func readGenerationGrant(r *http.Request, lookup func(string) (GenerationGrant, bool), now time.Time) (GenerationGrant, time.Time, bool) {
	client, ok := service.CurrentVerifiedClient(r.TLS, now)
	if !ok {
		return GenerationGrant{}, time.Time{}, false
	}
	grant, ok := lookup(client.FingerprintHex())
	grant.ClientFingerprint = client.FingerprintHex()
	return grant, timeMin(grant.ExpiresAt, client.ExpiresAt), ok && grant.ExpiresAt.After(now)
}

func timeMin(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// GenerationAuthority reads the current registered board and a scoped challenge.
// Its implementation must enforce revocation and challenge validity at use time.
type GenerationAuthority interface {
	GenerationReceiptAuthority(context.Context, GenerationGrant, common.Hash) (chainview.GenerationReceiptAuthority, error)
}

type RegisteredGenerationVerifier struct {
	Authority GenerationAuthority
	Clock     func() time.Time
}

func (v RegisteredGenerationVerifier) VerifyGenerationFreeze(ctx context.Context, grant GenerationGrant, raw []byte) (chainview.VerifiedGenerationFreeze, error) {
	receipt, err := rentalv1.ParseFreezeReceipt(raw)
	if err != nil || v.Authority == nil || v.Clock == nil {
		return chainview.VerifiedGenerationFreeze{}, chainview.ErrGenerationFreeze
	}
	challenge := common.Hash(receipt.Challenge)
	authority, err := v.Authority.GenerationReceiptAuthority(ctx, grant, challenge)
	if err != nil || authority.Binding.Branch != grant.Branch || authority.Binding.Board != grant.Board {
		return chainview.VerifiedGenerationFreeze{}, chainview.ErrGenerationFreeze
	}
	return chainview.VerifyGenerationReceipt(raw, authority, v.Clock())
}
