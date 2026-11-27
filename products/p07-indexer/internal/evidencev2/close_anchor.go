package evidencev2

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
	"github.com/ethereum/go-ethereum/common"
)

type closeAnchorReader interface {
	ConfirmStoredGenerationAnchor(context.Context, rentalv1.Anchor) (rentalv1.Anchor, error)
}

func GenerationCloseAnchorHandler(lookup func(string) (GenerationGrant, bool), authority GenerationAuthority,
	profile chainview.Profile, reader closeAnchorReader, clock func() time.Time) http.Handler {
	mux := http.NewServeMux()
	if lookup == nil || authority == nil || reader == nil || clock == nil {
		return mux
	}
	mux.HandleFunc("POST "+rentalv1.CloseAnchorPath, func(w http.ResponseWriter, r *http.Request) {
		grant, deadline, valid := readGenerationGrant(r, lookup, clock())
		if !valid {
			http.NotFound(w, r)
			return
		}
		now := clock()
		var request rentalv1.CloseAnchorRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || !request.Valid(now) {
			write(w, http.StatusBadRequest, map[string]string{"status": "invalid"})
			return
		}
		if request.Stream != grant.Stream || request.Stream != profile.StreamID || request.StreamGeneration != profile.GenerationID ||
			request.Branch != grant.Branch || request.Board != grant.Board {
			http.NotFound(w, r)
			return
		}
		scope := grant
		scope.RequestedGeneration, _ = strconv.ParseUint(request.Generation, 10, 64)
		challenge := common.HexToHash(request.Challenge)
		conflict := func() { write(w, http.StatusConflict, map[string]string{"status": "unavailable"}) }
		ctx, cancel := context.WithDeadline(r.Context(), timeMin(deadline, timeMin(time.Unix(request.ValidUntil, 0), now.Add(15*time.Second))))
		defer cancel()
		registered, err := authority.GenerationReceiptAuthority(ctx, scope, challenge)
		if err != nil {
			conflict()
			return
		}
		commitment, err := chainview.GenerationBindingDigest(registered.Binding)
		if err != nil || commitment.Hex() != request.RegistrationBinding || !registered.AuthorityUntil.After(now) {
			conflict()
			return
		}
		anchor, err := reader.ConfirmStoredGenerationAnchor(ctx, request.Previous)
		if err != nil {
			conflict()
			return
		}
		current, err := authority.GenerationReceiptAuthority(ctx, scope, challenge)
		if err != nil || current != registered {
			conflict()
			return
		}
		confirmed, err := reader.ConfirmStoredGenerationAnchor(ctx, request.Previous)
		if err != nil || confirmed != anchor {
			conflict()
			return
		}
		reloaded, remaining, ok := readGenerationGrant(r, lookup, clock())
		if !ok || reloaded != grant {
			http.NotFound(w, r)
			return
		}
		now = clock()
		expiry := timeMin(remaining, timeMin(registered.AuthorityUntil, timeMin(time.Unix(request.ValidUntil, 0), now.Add(5*time.Second))))
		if ctx.Err() != nil || expiry.Unix() <= now.Unix() {
			conflict()
			return
		}
		write(w, http.StatusOK, rentalv1.CloseAnchorConfirmation{CloseAnchorRequest: request, Finalized: confirmed, ExpiresAt: expiry.Unix()})
	})
	return mux
}
