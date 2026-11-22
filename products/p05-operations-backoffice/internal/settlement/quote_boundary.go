package settlement

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
	"github.com/ethereum/go-ethereum/common"
)

var operatorFingerprint = regexp.MustCompile(`^[0-9a-f]{64}$`)

type QuoteBoundaryReaderGrant struct {
	ExpiresAt int64 `json:"expiresAt"`
}

func (h FinanceHTTP) RegisterQuoteBoundary(mux *http.ServeMux, readers func(string) (QuoteBoundaryReaderGrant, bool), operators func(string) (service.Principal, bool), merchants map[string]string) {
	mux.HandleFunc("POST /v2/finance/quote-boundary-origin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		now := time.Now()
		client, ok := service.CurrentVerifiedClient(r.TLS, now)
		if !ok {
			http.NotFound(w, r)
			return
		}
		reader, ok := readers(client.FingerprintHex())
		if !ok || reader.ExpiresAt <= now.Unix() {
			http.NotFound(w, r)
			return
		}
		var in service.QuoteBoundaryLookup
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF ||
			!financeRef.MatchString(in.Branch) || !financeRef.MatchString(in.Sale) || !operatorFingerprint.MatchString(in.Operator) {
			http.NotFound(w, r)
			return
		}
		actor, ok := operators(in.Operator)
		target, configured := h.Targets[in.Branch]
		if !ok || actor.Role != "finance" || actor.ExpiresAt <= now.Unix() || !configured || target.Stream != in.Stream ||
			merchants[in.Branch] == "" || merchants[in.Branch] != in.Merchant || h.QuoteCipher == nil {
			http.NotFound(w, r)
			return
		}
		deadline := minTime(minTime(client.ExpiresAt, time.Unix(reader.ExpiresAt, 0)), time.Unix(actor.ExpiresAt, 0))
		w, r, cancel := authority.Bound(r, deadline, func() bool {
			active, ok := readers(client.FingerprintHex())
			principal, enrolled := operators(in.Operator)
			return ok && active == reader && enrolled && reflect.DeepEqual(principal, actor)
		}, w)
		defer cancel()
		ctx := authority.Scope(r.Context(), actor, in.Branch, "reconcile_read")
		var cipher, digest, signer, quoteID, orderID []byte
		var issued, expires time.Time
		err := h.Store.db.QueryRow(ctx, "SELECT * FROM ledger.operator_quote_boundary($1,$2,$3,$4)",
			actor.Tenant, in.Branch, actor.Subject, in.Sale).
			Scan(&cipher, &digest, &signer, &quoteID, &orderID, &issued, &expires)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		signed, err := h.QuoteCipher.Open(nil, nil, cipher, quoteAAD(in.Branch, in.Sale))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		quote, err := unpackQuote(signed, digest, signer, target)
		if err != nil || !bytes.Equal(quote.Args.QuoteId[:], quoteID) || !bytes.Equal(quote.Args.OrderId[:], orderID) ||
			common.Hash(quote.Args.OrderId).Hex() != in.Order || common.Hash(quote.Args.MerchantId).Hex() != in.Merchant ||
			issued.Unix() != int64(quote.Args.IssuedAt) || expires.Unix() != quote.expiry().Unix() ||
			h.Store.db.CheckRead(ctx) != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.QuoteBoundaryOrigin{QuoteBoundaryLookup: in,
			Chain: "8283", Contract: lower(target.Contract), Asset: lower(target.Asset), Manifest: target.Manifest,
			ABI: ABIHash, CodeHash: target.CodeHash.Hex(), Digest: common.Hash(quote.Digest).Hex(),
			IssuedAt: issued.Unix(), ExpiresAt: expires.Unix()})
	})
}
