package commerce

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/jackc/pgx/v5"
)

func installedPickupReady(db *pgx.Conn, branch, origin string, staff, other *http.Client,
	dispatch func(context.Context, string) error) func(context.Context, string) (map[string]string, error) {
	return func(ctx context.Context, sale string) (map[string]string, error) {
		post := func(client *http.Client, body any, expect int) ([]byte, error) {
			raw, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+"/v2/commerce", bytes.NewReader(raw))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			reply, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("pickup TLS request failed")
			}
			defer reply.Body.Close()
			result, err := io.ReadAll(io.LimitReader(reply.Body, 32768))
			if err != nil || reply.StatusCode != expect {
				return nil, fmt.Errorf("pickup HTTP %d, expected %d", reply.StatusCode, expect)
			}
			return result, nil
		}
		if err := dispatch(ctx, sale); err != nil {
			return nil, fmt.Errorf("original fulfillment was not acknowledged")
		}
		var key string
		for revision, target := range []string{"preparing", "ready"} {
			if err := db.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&key); err != nil {
				return nil, err
			}
			input := map[string]any{"travel": map[string]any{
				"operation": "pickup-change", "branch": branch, "sale": sale,
				"target": target, "expectedRevision": revision, "requestKey": key,
			}}
			first, err := post(staff, input, http.StatusOK)
			if err != nil {
				return nil, err
			}
			if second, err := post(staff, input, http.StatusOK); err != nil || !bytes.Equal(first, second) {
				return nil, fmt.Errorf("pickup replay changed the transition")
			}
		}
		if _, err := post(other, map[string]any{"command": service.Request{
			Version: 2, Contract: "C3", Operation: "order.status", RequestID: key, Object: sale,
			Domain: branch, ExpiresAt: time.Now().Unix() + 20, Body: map[string]string{},
		}}, http.StatusNotFound); err != nil {
			return nil, err
		}
		var number int64
		var finalized, ready time.Time
		err := db.QueryRow(ctx, `SELECT n.number,j.booked_at,t.ready_at
			FROM ledger.sale s JOIN ledger.payment p ON p.sale_id=s.id JOIN ledger.pickup n ON n.sale_id=s.id
			JOIN ledger.journal j ON j.sale_id=s.id AND j.evidence_id=p.evidence_id AND j.refund_id IS NULL
			JOIN ledger.travel_pickup_transition t ON t.sale_id=s.id AND t.target='ready'
			WHERE s.id=$1 AND 1=(SELECT count(*) FROM ledger.payment WHERE sale_id=s.id)
			AND 1=(SELECT count(*) FROM ledger.consumer_receipt c JOIN ledger.fulfillment_outbox e USING(event_id) WHERE e.sale_id=s.id)
			AND 2=(SELECT count(*) FROM ledger.travel_pickup_transition WHERE sale_id=s.id)`,
			sale).Scan(&number, &finalized, &ready)
		if err != nil || number <= 0 || ready.Before(finalized) {
			return nil, fmt.Errorf("original pickup proof unavailable")
		}
		return map[string]string{"pickupNo": strconv.FormatInt(number, 10),
			"finalizedAt": finalized.UTC().Format(time.RFC3339Nano), "pickupReadyAt": ready.UTC().Format(time.RFC3339Nano)}, nil
	}
}
