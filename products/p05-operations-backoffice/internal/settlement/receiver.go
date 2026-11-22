package settlement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"time"
)

type ReceiverGrant struct {
	Branches  []string `json:"branches"`
	ExpiresAt int64    `json:"expiresAt"`
}

type Receiver struct {
	Store    *Store
	Evidence deliveryEvidence
	Grants   map[string]ReceiverGrant
}

func (r Receiver) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v2/fulfillment-events", r.serve)
	mux.HandleFunc("GET /v2/fulfillment-events/{eventId}", r.serve)
}

func (r Receiver) serve(w http.ResponseWriter, request *http.Request) {
	client, ok := service.CurrentVerifiedClient(request.TLS, time.Now())
	if !ok {
		http.NotFound(w, request)
		return
	}
	grant, ok := r.Grants[client.FingerprintHex()]
	deadline := minTime(client.ExpiresAt, time.Unix(grant.ExpiresAt, 0))
	if !ok || len(grant.Branches) == 0 || !time.Now().Before(deadline) {
		http.NotFound(w, request)
		return
	}
	epoch := request.Header.Get("X-NU54-Delivery-Epoch")
	fence, err := strconv.ParseInt(request.Header.Get("X-NU54-Delivery-Fence"), 10, 64)
	if len(epoch) != 64 || err != nil || fence < 1 {
		http.NotFound(w, request)
		return
	}
	post := request.Method == http.MethodPost
	event, effect := request.PathValue("eventId"), request.URL.Query().Get("effectKey")
	digest, _ := hex.DecodeString(request.URL.Query().Get("payloadDigest"))
	branch := ""
	if post {
		raw, err := io.ReadAll(http.MaxBytesReader(w, request.Body, 4096))
		if err != nil {
			http.Error(w, "invalid event", http.StatusBadRequest)
			return
		}
		var delivery Delivery
		parser := json.NewDecoder(bytes.NewReader(raw))
		parser.DisallowUnknownFields()
		if parser.Decode(&delivery) != nil || parser.Decode(new(any)) != io.EOF || !financeRef.MatchString(delivery.BranchID) {
			http.Error(w, "invalid event", http.StatusBadRequest)
			return
		}
		branch, event, effect = delivery.BranchID, delivery.EventID, delivery.EffectKey
		hash := sha256.Sum256(raw)
		digest = hash[:]
	}
	if !financeRef.MatchString(event) || !financeRef.MatchString(effect) || len(digest) != sha256.Size || !time.Now().Before(deadline) {
		http.NotFound(w, request)
		return
	}
	w, request, cancel := authority.Bound(request, deadline, func() bool {
		current, exists := r.Grants[client.FingerprintHex()]
		return exists && reflect.DeepEqual(current, grant)
	}, w)
	defer cancel()
	ctx := request.Context()
	var stream, merchant string
	canonical := Delivery{EventID: event, EffectKey: effect}
	err = r.Store.db.QueryRow(ctx, `SELECT stream_id::text,merchant_key,branch_id::text,sale_id::text,
		kind,source_ref::text,revision,pickup_no,receipt_ref::text
		FROM ledger.receiver_source($1,$2,$3,$4,$5,$6::text[]::uuid[])`, event, effect, fence, epoch, digest, grant.Branches).
		Scan(&stream, &merchant, &canonical.BranchID, &canonical.SaleID, &canonical.Kind,
			&canonical.SourceRef, &canonical.Revision, &canonical.PickupNo, &canonical.ReceiptRef)
	if err != nil || (post && canonical.BranchID != branch) {
		http.NotFound(w, request)
		return
	}
	expected, err := json.Marshal(canonical)
	expectedDigest := sha256.Sum256(expected)
	if err != nil || !bytes.Equal(expectedDigest[:], digest) {
		http.NotFound(w, request)
		return
	}
	status := "unknown"
	err = r.Evidence.deliveryEpoch(ctx, stream, merchant, func(ctx context.Context, current string) error {
		if current != epoch {
			return ErrEvidenceHeld
		}
		return r.Store.db.QueryRow(ctx, `SELECT ledger.receive_fulfillment($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			event, effect, fence, epoch, digest, canonical.BranchID, stream, merchant, post).Scan(&status)
	})
	if err != nil {
		status = "unknown"
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"eventId": event, "effectKey": effect,
		"payloadDigest": hex.EncodeToString(digest), "status": status})
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}
