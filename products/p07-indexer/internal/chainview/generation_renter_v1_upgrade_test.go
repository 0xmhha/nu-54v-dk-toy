package chainview_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
	"github.com/jackc/pgx/v5"
)

const (
	renterMigration087 = "5c117351175d88eee8147bef60c986b7744095b70f92cf344a87e9b55b67d387"
	renterMigration088 = "0cf57b1ac01c7d4f910b6a06451284c82d5d32516a7137aef3461b237544078a"
)

func startRenterV1UpgradeDB(t *testing.T, ctx context.Context) {
	t.Helper()
	if os.Getenv("P05_O09_RENTER_V1_ORIGIN") != "1" || os.Getenv("P05_O09_RENTER_ASSESSMENT") != "mixed" {
		t.Fatal("upgrade fixture requires the v1 terminal and mixed assessment")
	}
	dsn := os.Getenv("P05_O09_COMBINED_DATABASE_URL")
	config, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(config.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(config.Host, "/tmp/") || strings.HasPrefix(config.Host, "/private/tmp/")) {
		t.Fatal("upgrade fixture requires an owned P05 cluster")
	}
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "nu54_commerce_renter_v1_upgrade"
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		_ = admin.Close(context.Background())
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+name); err != nil {
			t.Error(err)
		}
		_ = admin.Close(cleanup)
	})
	t.Setenv("P05_O09_COMBINED_DATABASE_URL", dsn+" dbname="+name)
	migrateCombinedP05(t, &combinedGenerationOpsd{ctx: ctx}, 87)
}

func (s *combinedGenerationOpsd) verifyRenterV1Upgrade(t *testing.T, boardApproval, freezeReceipt []byte) {
	t.Helper()
	binding, _, err := chainview.ParseGenerationProjection(s.input.projection)
	if err != nil {
		t.Fatal(err)
	}
	original := s.v1FixtureOrder(t)
	// Before 088 the v1 terminal retained its signed original locally. P05
	// legitimately retains the accepted handover and 073 aggregate proof.
	original["approvalSignature"] = "0x" + hex.EncodeToString(boardApproval)
	original["nonce"] = "3"
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	archived := filepath.Join(t.TempDir(), "legacy-terminal-order.json")
	if err := os.WriteFile(archived, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	offer, _ := json.Marshal(map[string]any{"customer": combinedRenter, "generation": s.input.expected.Generation,
		"account": original["account"], "assignmentUntil": time.Now().UTC().Add(50 * time.Second)})
	raw := s.call(t, s.operator, http.MethodPost, "/v2/operations/renter-handover/"+binding.Branch+"/"+binding.Board, "application/json", offer, http.StatusCreated)
	var handover struct{ Assignment string }
	if json.Unmarshal(raw, &handover) != nil || handover.Assignment == "" {
		t.Fatal("could not establish the original handover")
	}
	path := "/v2/renter/returns/" + handover.Assignment
	account, _ := json.Marshal(map[string]string{"account": original["account"]})
	s.call(t, s.renter, http.MethodPost, path+"/accept", "application/json", account, http.StatusOK)
	s.recordWithExpectedPending(t, s.input.projection, freezeReceipt, 1)
	s.call(t, s.v1Terminal, http.MethodPost, "/v2/private/renter-v1-orders/reserve", "application/json", encoded, http.StatusNotFound)
	projection := func() []byte {
		t.Helper()
		return s.call(t, s.renter, http.MethodGet, path, "", nil, http.StatusOK)
	}
	before := projection()
	var beforeCounts struct{ PendingPayments, PendingLimits int }
	if json.Unmarshal(before, &beforeCounts) != nil || beforeCounts.PendingPayments != 1 || beforeCounts.PendingLimits != 1 {
		t.Fatalf("087 did not serve its existing authenticated assessment: %s", before)
	}
	facts := func() string {
		t.Helper()
		var snapshot string
		err := s.db.QueryRow(s.ctx, `SELECT jsonb_build_array(to_jsonb(a),to_jsonb(p),to_jsonb(g))::text
			FROM ledger.renter_return_assignment a JOIN ledger.rental_generation_authorized_proof p USING(branch_id,device_ref,generation)
			JOIN ledger.commerce_grant g ON g.branch_id=a.branch_id AND g.actor_id=$2 WHERE a.assignment_id=$1`,
			handover.Assignment, combinedV1Terminal).Scan(&snapshot)
		if err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	oldFacts := facts()
	migrateCombinedP05From(t, s, 87, renterMigration087, renterMigration088, 88)
	s.enableV1Origins()
	s.restart(t)
	if got := facts(); got != oldFacts {
		t.Fatal("migration rewrote the old handover, proof or terminal grant")
	}
	retained, err := os.ReadFile(archived)
	if err != nil || !bytes.Equal(retained, encoded) {
		t.Fatal("historical terminal original changed")
	}
	after := projection()
	var projected struct {
		PendingPayments, PendingLimits int
		PendingOrders                  []json.RawMessage
		BasisAt                        string
	}
	var oldBasis struct{ BasisAt string }
	if json.Unmarshal(after, &projected) != nil || json.Unmarshal(before, &oldBasis) != nil ||
		projected.PendingPayments != 1 || projected.PendingLimits != 1 || projected.BasisAt != oldBasis.BasisAt || len(projected.PendingOrders) != 0 {
		t.Fatalf("088 changed the historical assessment or synthesized an original link: %s", after)
	}
	var counts int
	if err := s.db.QueryRow(s.ctx, `SELECT (SELECT count(*) FROM ledger.renter_v1_order_origin)+(SELECT count(*) FROM ledger.renter_v1_pending_witness)`).Scan(&counts); err != nil || counts != 0 {
		t.Fatal("088 backfilled an unproved historical linkage", err)
	}
	s.call(t, s.otherRenter, http.MethodGet, path, "", nil, http.StatusNotFound)
	if fence := os.Getenv("P05_O09_RENTER_V1_FREEZE_FENCE"); fence != "" {
		t.Run("RejectHistoricalFirstReservation", func(t *testing.T) {
			if fence == "1" {
				migrateCombinedP05(t, s, 89)
				s.restart(t)
			}
			delete(original, "approvalSignature")
			delete(original, "nonce")
			payload, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			s.call(t, s.v1Terminal, http.MethodPost, "/v2/private/renter-v1-orders/reserve", "application/json", payload, http.StatusNotFound)
		})
	}

}
