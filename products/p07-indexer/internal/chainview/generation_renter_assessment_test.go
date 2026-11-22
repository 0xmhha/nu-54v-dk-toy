package chainview_test

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
	"github.com/ethereum/go-ethereum/ethclient"
)

func (s *combinedGenerationOpsd) verifyRenterAssessment(t *testing.T, dsn string, projection, receipt []byte, payments, limits int) {
	t.Helper()
	binding, _, err := chainview.ParseGenerationProjection(s.input.projection)
	if err != nil {
		t.Fatal(err)
	}
	account := strings.ToLower(binding.Account.Hex())
	offer, _ := json.Marshal(map[string]any{"customer": combinedRenter, "generation": s.input.expected.Generation, "account": account,
		"assignmentUntil": time.Now().UTC().Add(35 * time.Second)})
	path := "/v2/operations/renter-handover/" + binding.Branch + "/" + binding.Board
	raw := s.call(t, s.operator, http.MethodPost, path, "application/json", offer, http.StatusCreated)
	var original struct{ Assignment string }
	if err := json.Unmarshal(raw, &original); err != nil || original.Assignment == "" {
		t.Fatal("missing handover", err)
	}
	renterPath := "/v2/renter/returns/" + original.Assignment
	accept, _ := json.Marshal(map[string]string{"account": account})
	s.call(t, s.renter, http.MethodPost, renterPath+"/accept", "application/json", accept, http.StatusOK)
	check := func(expected *int) {
		t.Helper()
		var view struct {
			Settlement, Account                          string
			PendingCount, PendingPayments, PendingLimits *int
			BasisAt, BasisUntil                          *time.Time
		}
		raw := s.call(t, s.renter, http.MethodGet, renterPath, "", nil, http.StatusOK)
		if err := json.Unmarshal(raw, &view); err != nil || view.Account != account {
			t.Fatal("invalid renter projection", err)
		}
		if expected == nil {
			if view.Settlement != "unknown" || view.PendingCount != nil || view.PendingPayments != nil || view.PendingLimits != nil || view.BasisAt != nil || view.BasisUntil != nil {
				t.Fatal("unknown emitted inferred count")
			}
		} else if view.Settlement != "verified" || view.PendingCount == nil || *view.PendingCount != *expected || view.PendingPayments == nil || *view.PendingPayments != payments ||
			view.PendingLimits == nil || *view.PendingLimits != limits ||
			view.BasisAt == nil || view.BasisAt.After(time.Now()) || view.BasisUntil == nil || !view.BasisUntil.After(time.Now()) {
			t.Fatalf("validated same-checkpoint count absent: %s", raw)
		}
	}
	check(nil)
	want := payments + limits
	s.recordWithExpectedPending(t, projection, receipt, payments)
	if os.Getenv("P05_O09_RENTER_TYPED_UPGRADE") == "1" {
		t.Run("Upgrade085To086", func(t *testing.T) {
			check(nil)
			migrateCombinedP05From(t, s, 85,
				"517d8b944f977dbbd4f0baf164f50909b2dbc793cb0a83a3833adbf872ae2535",
				"62b2d7baf55bb9e0a71f122a562d20512014be719e95584d4b5b9d85e8bf81c4", 86)
		})
	}
	check(&want)
	s.restart(t)
	check(&want)
	s.call(t, s.otherRenter, http.MethodGet, renterPath, "", nil, http.StatusNotFound)
	var authorityUntil time.Time
	if err := s.db.QueryRow(s.ctx, "SELECT valid_until FROM ledger.rental_authority_audience WHERE branch_id=$1", binding.Branch).Scan(&authorityUntil); err != nil {
		t.Fatal(err)
	}
	s.sql(t, "UPDATE ledger.rental_authority_audience SET valid_until=$2 WHERE branch_id=$1", binding.Branch, time.Now().Add(-time.Second))
	check(nil)
	s.sql(t, "UPDATE ledger.rental_authority_audience SET valid_until=$2 WHERE branch_id=$1", binding.Branch, authorityUntil)
	check(&want)
	chain, err := ethclient.DialContext(s.ctx, s.input.rpc)
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Close()
	if err := chain.Client().CallContext(s.ctx, nil, "anvil_mine", 4); err != nil {
		t.Fatal(err)
	}
	check(nil)
	ingest, err := chainview.Open(s.ctx, dsn+" user=rental_o09_test_ingest")
	if err != nil {
		t.Fatal(err)
	}
	defer ingest.Close()
	source, err := chainview.Dial(s.ctx, s.input.rpc, s.input.profile)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	reader := chainview.Reader{Profile: s.input.profile, DB: ingest, Source: source}
	if err := reader.Poll(s.ctx); err != nil {
		t.Fatal(err)
	}
	check(nil)
	s.revoke(t)
	check(nil)
	s.call(t, s.operator, http.MethodPost, "/v2/operations/renter-handover/"+original.Assignment+"/revoke", "", nil, http.StatusNoContent)
	s.call(t, s.renter, http.MethodGet, renterPath, "", nil, http.StatusNotFound)
}
