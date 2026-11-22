package settlement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type Delivery struct {
	EventID    string  `json:"eventId"`
	BranchID   string  `json:"branchId"`
	SaleID     string  `json:"saleId"`
	Kind       string  `json:"kind"`
	SourceRef  string  `json:"sourceRef"`
	Revision   int64   `json:"revision"`
	EffectKey  string  `json:"effectKey"`
	PickupNo   *int64  `json:"pickupNo,omitempty"`
	ReceiptRef *string `json:"receiptRef,omitempty"`
	fence      int64
	lookup     bool
}

type deliveryEvidence interface {
	deliveryEpoch(context.Context, string, string, func(context.Context, string) error) error
}

type Dispatcher struct {
	Evidence deliveryEvidence
	Store    *Store
	Client   *http.Client
	Targets  map[string]string
}

func (d Dispatcher) Step(ctx context.Context) (bool, error) {
	var event Delivery
	var stream, merchant string
	err := d.Store.db.QueryRow(ctx, `SELECT event_id::text,branch_id::text,sale_id::text,kind,
		source_ref::text,revision,effect_key::text,pickup_no,receipt_ref::text,fence,lookup_only,stream_id::text,merchant_key
		FROM ledger.claim_fulfillment()`).Scan(&event.EventID, &event.BranchID, &event.SaleID, &event.Kind, &event.SourceRef,
		&event.Revision, &event.EffectKey, &event.PickupNo, &event.ReceiptRef, &event.fence, &event.lookup, &stream, &merchant)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return true, err
	}
	digest := sha256.Sum256(payload)
	if d.Evidence == nil {
		return true, ErrEvidenceHeld
	}
	var authorizedEpoch string
	err = d.Evidence.deliveryEpoch(ctx, stream, merchant, func(ctx context.Context, epoch string) error {
		authorizedEpoch = epoch
		_, err := d.Store.db.Exec(ctx, "SELECT ledger.prepare_fulfillment($1,$2,$3,$4,$5,$6)", event.EventID, event.fence,
			event.EffectKey, epoch, digest[:], !event.lookup)
		return err
	})
	if err != nil {
		return true, err
	}
	result := d.send(ctx, event, payload, digest, authorizedEpoch)
	err = d.Evidence.deliveryEpoch(ctx, stream, merchant, func(ctx context.Context, epoch string) error {
		if epoch != authorizedEpoch {
			return ErrEvidenceHeld
		}
		_, err := d.Store.db.Exec(ctx, "SELECT ledger.finish_fenced_fulfillment($1,$2,$3,$4,$5,$6)", event.EventID, event.fence, event.EffectKey, epoch, result, digest[:])
		return err
	})
	return true, err
}

func (d Dispatcher) send(ctx context.Context, event Delivery, payload []byte, digest [32]byte, epoch string) string {
	endpoint, err := url.Parse(d.Targets[event.BranchID])
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil ||
		endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return "unknown"
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/v2/fulfillment-events"
	method := http.MethodPost
	var body io.Reader = bytes.NewReader(payload)
	if event.lookup {
		method = http.MethodGet
		endpoint.Path += "/" + event.EventID
		endpoint.RawQuery = url.Values{"effectKey": {event.EffectKey}, "payloadDigest": {hex.EncodeToString(digest[:])}}.Encode()
		body = nil
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return "unknown"
	}
	request.Header.Set("X-NU54-Delivery-Epoch", epoch)
	request.Header.Set("X-NU54-Delivery-Fence", strconv.FormatInt(event.fence, 10))
	request.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := d.Client.Do(request)
	if err != nil {
		return "unknown"
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return "unknown"
	}
	var ack struct {
		EventID       string `json:"eventId"`
		EffectKey     string `json:"effectKey"`
		PayloadDigest string `json:"payloadDigest"`
		Status        string `json:"status"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&ack) != nil || decoder.Decode(new(any)) != io.EOF ||
		ack.EventID != event.EventID || ack.EffectKey != event.EffectKey || ack.PayloadDigest != hex.EncodeToString(digest[:]) {
		return "unknown"
	}
	if ack.Status == "delivered" || (event.lookup && ack.Status == "absent") {
		return ack.Status
	}
	return "unknown"
}
