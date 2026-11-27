package chainview_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func (s *combinedGenerationOpsd) rolloverClose(t *testing.T, projection, receipt []byte) {
	t.Helper()
	successorPath := s.verifyLegacyOpenEligibility(t)
	s.recordWithExpectedPending(t, projection, receipt, 0)
	const key = "e39c66cd-adb5-4534-9100-a2018f7bbf05"
	base, query, _ := strings.Cut(s.reviewPath, "?")
	path := base + "/close?" + query
	wire := func(original, recovery string) []byte {
		command := map[string]string{"requestKey": key, "reviewRef": original}
		if recovery != "" {
			command["recoveryReviewRef"] = recovery
		}
		body, _ := json.Marshal(command)
		return body
	}
	var preview struct {
		ReturnReview   struct{ ReviewRef string }
		CleanupBlocked bool
	}
	if raw := s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK); json.Unmarshal(raw, &preview) != nil ||
		preview.ReturnReview.ReviewRef == "" || preview.CleanupBlocked != (s.input.expected.Generation != "1") {
		t.Fatal("pre-078 protected close review absent or predecessor hold differs")
	}
	var blocked struct{ Cleanup struct{ State string } }
	raw := s.call(t, s.operator, http.MethodPost, base+"/cleanup?"+query, "application/json",
		[]byte(`{"requestKey":"c9e26ca3-1a53-4359-8e72-346e95bac254"}`), http.StatusConflict)
	if json.Unmarshal(raw, &blocked) != nil || blocked.Cleanup.State != "protected-evidence-unavailable" {
		t.Fatal("pre-078 cleanup did not stay held")
	}
	legacy := preview.ReturnReview.ReviewRef
	lost := make(chan error, 1)
	go func() {
		r, err := http.NewRequestWithContext(s.ctx, http.MethodPost, s.origin+path, bytes.NewReader(wire(legacy, "")))
		if err == nil {
			r.Header.Set("Origin", s.origin)
			r.Header.Set("Content-Type", "application/json")
			resp, callErr := s.operator.Do(r)
			err = callErr
			if resp != nil {
				_ = resp.Body.Close()
			}
		}
		lost <- err
	}()
	var original types.Transaction
	select {
	case raw := <-s.closeFault.first:
		if err := original.UnmarshalBinary(raw); err != nil {
			t.Fatal(err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("pre-078 transaction did not reach first network send")
	}
	var originalSealed, savedHash []byte
	if err := s.db.QueryRow(s.ctx, "SELECT tx_hash,sealed_transaction FROM ledger.rental_close_candidate").Scan(&savedHash, &originalSealed); err != nil || len(originalSealed) == 0 || common.Hash(savedHash) != original.Hash() {
		t.Fatal("pre-078 transaction was not durably recoverable", err)
	}
	s.restart(t)
	if err := <-lost; err == nil {
		t.Fatal("original POST response survived daemon shutdown")
	}
	verifySuccessorHold := func(held bool) {
		t.Helper()
		var successor struct{ CleanupBlocked *bool }
		raw := s.call(t, s.operator, http.MethodGet, successorPath, "", nil, http.StatusOK)
		if json.Unmarshal(raw, &successor) != nil || successor.CleanupBlocked == nil || *successor.CleanupBlocked != held {
			t.Fatalf("running opsd successor hold did not match %t", held)
		}
	}
	verifySuccessorHold(true)
	migrateCombinedP05(t, s)
	verifySuccessorHold(false)
	t.Log("same opsd and operator observed successor hold transition true to false after migration")
	s.restart(t)
	s.sql(t, "UPDATE ledger.rental_close_candidate SET attempt_until=clock_timestamp()-interval '1 second'")
	review := ""
	if os.Getenv("P05_O09_ROLLOVER_CLOSE") == "renewed" {
		s.sql(t, "UPDATE ledger.rental_authority_audience SET valid_until=valid_until+interval '1 minute' WHERE branch_id=$1", s.input.expected.Branch)
		s.call(t, s.operator, http.MethodPost, path, "application/json", wire(legacy, ""), http.StatusAccepted)
		if s.closeFault.sends.Load() != 1 {
			t.Fatal("changed authority allowed old approval to send")
		}
		var current struct {
			Operations []struct{ ProtectedRecoveryReviewRef string }
		}
		raw := s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK)
		if json.Unmarshal(raw, &current) != nil || len(current.Operations) != 1 || current.Operations[0].ProtectedRecoveryReviewRef == "" {
			t.Fatal("changed scope lacks actionable original recovery review")
		}
		review = current.Operations[0].ProtectedRecoveryReviewRef
		s.call(t, s.operator, http.MethodPost, path, "application/json", wire(legacy, strings.Repeat("a", 64)), http.StatusConflict)
	}
	t.Log("retrying the unchanged original with the currently approved scope")
	s.call(t, s.operator, http.MethodPost, path, "application/json", wire(legacy, review), http.StatusAccepted)
	if s.closeFault.sends.Load() != 2 {
		t.Fatal("migration recovery did not send the identical authenticated original")
	}
	s.call(t, s.operator, http.MethodPost, path, "application/json", wire(legacy, ""), http.StatusAccepted)
	if s.closeFault.sends.Load() != 2 {
		t.Fatal("known original was sent again")
	}
	var recovered []byte
	if err := s.db.QueryRow(s.ctx, "SELECT sealed_transaction FROM ledger.rental_close_candidate").Scan(&recovered); err != nil || !bytes.Equal(originalSealed, recovered) {
		t.Fatal("migration or new review changed original sealed bytes", err)
	}
}

func (s *combinedGenerationOpsd) verifyLegacyOpenEligibility(t *testing.T) string {
	t.Helper()
	branch := s.input.expected.Branch
	destination := common.Address{19: 31}
	firstDevice := "194cc0d7-38cf-4b59-820b-cb34b80baa52"
	successorDevice := "a27331a1-edcc-4bb0-ae9b-9ec59c007551"
	const key = "25b30cbb-0e12-4674-856d-7a004a4fa2fb"
	const priorKey = "8fb3c66f-412a-4c9a-bb88-3532d89a087d"
	// This amount intentionally exceeds the synthetic operator balance. The
	// test observes HTTP and durable admission, without moving the close anchor.
	const amount = "999999999999999999999999"
	for _, enrollment := range []struct {
		device     string
		generation int64
		address    common.Address
	}{{firstDevice, 1, common.Address{19: 32}}, {successorDevice, 1, common.Address{19: 33}},
		{successorDevice, 2, common.Address{19: 34}}} {
		s.sql(t, `INSERT INTO ledger.rental_enrollment
			(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until,attested_at)
			VALUES($1,$2,$3,$4,$5,'fixture',sha256(convert_to($4::text,'UTF8')),now()+interval '2 minutes',now()-interval '1 second')`,
			branch, enrollment.device, enrollment.generation, strings.ToLower(enrollment.address.Hex()), strings.ToLower(destination.Hex()))
	}
	check := func(device string, generation int, held bool) (string, string) {
		t.Helper()
		base := "/v2/operations/rental/" + branch + "/" + device
		query := fmt.Sprintf("?generation=%d", generation)
		var view struct {
			ReviewRef      string
			CleanupBlocked bool
			Observation    struct{ Status string }
		}
		raw := s.call(t, s.operator, http.MethodGet, base+query, "", nil, http.StatusOK)
		if err := json.Unmarshal(raw, &view); err != nil || view.Observation.Status != "unopened" ||
			view.ReviewRef == "" || view.CleanupBlocked != held {
			t.Fatalf("legacy opening eligibility differs for generation %d: %+v, %v", generation, view, err)
		}
		return base + "/open" + query, view.ReviewRef
	}
	wire := func(request, review string) []byte {
		body, _ := json.Marshal(map[string]string{"requestKey": request, "reviewRef": review, "amount": amount})
		return body
	}
	first, firstReview := check(firstDevice, 1, false)
	s.call(t, s.operator, http.MethodPost, first, "application/json", wire(key, firstReview), http.StatusAccepted)
	s.call(t, s.operator, http.MethodPost, first, "application/json", wire(key, firstReview), http.StatusAccepted)
	successor, successorReview := check(successorDevice, 2, true)
	s.call(t, s.operator, http.MethodPost, successor, "application/json", wire(priorKey, successorReview), http.StatusConflict)
	count := func(device string, want int) {
		t.Helper()
		var observed int
		err := s.db.QueryRow(s.ctx, "SELECT count(*) FROM ledger.rental_operation WHERE branch_id=$1 AND device_ref=$2 AND action='open'",
			branch, device).Scan(&observed)
		if err != nil || observed != want {
			t.Fatalf("expected %d durable opening requests, got %d: %v", want, observed, err)
		}
	}
	count(firstDevice, 1)
	count(successorDevice, 0)
	s.sql(t, `INSERT INTO ledger.rental_operation
		(branch_id,device_ref,generation,action,request_key,actor_id,status,amount)
		VALUES($1,$2,2,'open',$3,$4,'unknown',$5::text::numeric)`, branch, successorDevice, priorKey, s.actor, amount)
	s.call(t, s.operator, http.MethodPost, successor, "application/json", wire(priorKey, successorReview), http.StatusAccepted)
	count(successorDevice, 1)
	return strings.Replace(successor, "/open?", "?", 1)
}

// Verify the installed predecessor survives the normal upgrade. Later migrations
// may already exist by the time this historical fixture runs.
func migrateCombinedP05From(t *testing.T, s *combinedGenerationOpsd, from int, before, after string, through ...int) {
	t.Helper()
	var version int
	var digest string
	var applied time.Time
	err := s.db.QueryRow(s.ctx, "SELECT version,sha256,applied_at FROM nu54_p05_migrations.revision ORDER BY version DESC LIMIT 1").
		Scan(&version, &digest, &applied)
	if err != nil || version != from || digest != before {
		t.Fatalf("expected the original migration %03d, got %03d %q: %v", from, version, digest, err)
	}
	migrateCombinedP05(t, s, through...)
	var unchanged bool
	err = s.db.QueryRow(s.ctx, `SELECT EXISTS (
		SELECT FROM nu54_p05_migrations.revision old
		JOIN nu54_p05_migrations.revision next ON next.version=old.version+1
		WHERE old.version=$1 AND old.sha256=$2 AND old.applied_at=$3 AND next.sha256=$4
	)`, from, before, applied, after).Scan(&unchanged)
	if err != nil || !unchanged {
		t.Fatalf("upgrade must preserve migration %03d and append the original %03d: %v", from, from+1, err)
	}
}

func migrateCombinedP05(t *testing.T, s *combinedGenerationOpsd, through ...int) {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-B", "products/p10-platform/database/migrate.py", "p05"}
	if len(through) != 0 {
		args = []string{"-B", "-c", "import os,sys; sys.path.insert(0,'products/p10-platform/database'); " +
			"import migrate; migrate.apply('p05', os.environ['P05_MIGRATION_DATABASE_URL'], " +
			"migrate.migrations(migrate.OWNERS['p05'][1])[:int(sys.argv[1])])", fmt.Sprint(through[0])}
	}
	upgrade := exec.CommandContext(s.ctx, "python3", args...)
	upgrade.Dir = root
	upgrade.Env = append(os.Environ(), "P05_MIGRATION_DATABASE_URL="+os.Getenv("P05_O09_COMBINED_DATABASE_URL"))
	if output, err := upgrade.CombinedOutput(); err != nil {
		t.Fatalf("actual schema migration failed: %v: %.600s", err, output)
	}
}
