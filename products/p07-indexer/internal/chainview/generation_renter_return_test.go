package chainview_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
)

func (s *combinedGenerationOpsd) verifyRenterReturn(t *testing.T) {
	t.Helper()
	binding, _, err := chainview.ParseGenerationProjection(s.input.projection)
	if err != nil {
		t.Fatal(err)
	}
	account := strings.ToLower(binding.Account.Hex())
	offer, _ := json.Marshal(map[string]any{"customer": combinedRenter, "generation": s.input.expected.Generation, "account": account,
		"assignmentUntil": time.Now().UTC().Add(25 * time.Second)})
	path := "/v2/operations/renter-handover/" + binding.Branch + "/" + binding.Board
	s.call(t, s.renter, http.MethodPost, path, "application/json", offer, http.StatusNotFound)
	raw := s.call(t, s.operator, http.MethodPost, path, "application/json", offer, http.StatusCreated)
	var original struct{ Assignment, Account, Generation string }
	if err := json.Unmarshal(raw, &original); err != nil || original.Assignment == "" || original.Account != account {
		t.Fatalf("invalid handover acknowledgment: %v", err)
	}
	if again := s.call(t, s.operator, http.MethodPost, path, "application/json", offer, http.StatusCreated); !bytes.Equal(again, raw) {
		t.Fatal("same handover offer changed the original reference")
	}
	renterPath := "/v2/renter/returns/" + original.Assignment
	s.call(t, s.renter, http.MethodGet, renterPath, "", nil, http.StatusNotFound)
	s.call(t, s.operator, http.MethodGet, renterPath, "", nil, http.StatusNotFound)
	s.call(t, s.renter, http.MethodPost, renterPath+"/accept", "application/json", []byte(`{"account":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`), http.StatusNotFound)
	accept, _ := json.Marshal(map[string]string{"account": account})
	s.call(t, s.otherRenter, http.MethodPost, renterPath+"/accept", "application/json", accept, http.StatusNotFound)
	s.call(t, s.foreignRenter, http.MethodPost, renterPath+"/accept", "application/json", accept, http.StatusNotFound)
	if os.Getenv("P05_O09_RENTER_ATTESTATION_UPGRADE") == "1" {
		t.Run("Upgrade083To084", func(t *testing.T) {
			migrateCombinedP05From(t, s, 83,
				"2b654c37985a41ec5f42cd9e71e10a779172f88159f140121d07fa5dfd8525ac",
				"5407b95431141ed5a8f25fe2fe927f66fa16c2b9cb7390ddbbde494ada09388c")
		})
	}
	var attested time.Time
	if err := s.db.QueryRow(s.ctx, `SELECT attested_at FROM ledger.rental_enrollment
			WHERE (branch_id,device_ref,generation)=($1,$2,$3)`, binding.Branch, binding.Board, binding.Generation).Scan(&attested); err != nil {
		t.Fatal(err)
	}
	setAttestation := func(value any) {
		s.sql(t, "UPDATE ledger.rental_enrollment SET attested_at=$4 WHERE (branch_id,device_ref,generation)=($1,$2,$3)",
			binding.Branch, binding.Board, binding.Generation, value)
	}
	setAttestation(nil)
	s.call(t, s.renter, http.MethodPost, renterPath+"/accept", "application/json", accept, http.StatusNotFound)
	setAttestation(attested)
	s.call(t, s.renter, http.MethodPost, renterPath+"/accept", "application/json", accept, http.StatusOK)
	s.restart(t)
	reply := s.call(t, s.renter, http.MethodGet, renterPath, "", nil, http.StatusOK)
	var current map[string]any
	if json.Unmarshal(reply, &current) != nil || len(current) != 6 || current["assignment"] != original.Assignment ||
		current["account"] != account || current["generation"] != original.Generation || current["slot"] != "unconfirmed" ||
		current["settlement"] != "unknown" {
		t.Fatal("renter read exposed fields without an authoritative source")
	}
	setAttestation(time.Now().Add(10 * time.Second))
	s.call(t, s.renter, http.MethodGet, renterPath, "", nil, http.StatusNotFound)
	setAttestation(attested)
	s.call(t, s.renter, http.MethodGet, renterPath, "", nil, http.StatusOK)
	s.call(t, s.collector, http.MethodGet, renterPath, "", nil, http.StatusNotFound)
	s.call(t, s.otherRenter, http.MethodGet, renterPath, "", nil, http.StatusNotFound)
	s.call(t, s.foreignRenter, http.MethodGet, renterPath, "", nil, http.StatusNotFound)
	var activeUntil time.Time
	if err := s.db.QueryRow(s.ctx, "SELECT valid_until FROM ledger.finance_grant WHERE branch_id=$1 AND actor_id=$2 AND capability='rental_manage'",
		binding.Branch, s.actor).Scan(&activeUntil); err != nil {
		t.Fatal(err)
	}
	s.sql(t, "UPDATE ledger.finance_grant SET valid_until=clock_timestamp()-interval '1 second' WHERE branch_id=$1 AND actor_id=$2 AND capability='rental_manage'", binding.Branch, s.actor)
	s.call(t, s.renter, http.MethodGet, renterPath, "", nil, http.StatusNotFound)
	s.sql(t, "UPDATE ledger.finance_grant SET valid_until=$3 WHERE branch_id=$1 AND actor_id=$2 AND capability='rental_manage'", binding.Branch, s.actor, activeUntil)
	s.call(t, s.renter, http.MethodGet, renterPath, "", nil, http.StatusOK)
	s.call(t, s.operator, http.MethodPost, "/v2/operations/renter-handover/"+original.Assignment+"/revoke", "", nil, http.StatusNoContent)
	s.call(t, s.renter, http.MethodGet, renterPath, "", nil, http.StatusNotFound)

	issued := s.call(t, s.operator, http.MethodPost, path, "application/json", offer, http.StatusCreated)
	if err := json.Unmarshal(issued, &original); err != nil {
		t.Fatal(err)
	}
	renterPath = "/v2/renter/returns/" + original.Assignment
	s.call(t, s.renter, http.MethodPost, renterPath+"/accept", "application/json", accept, http.StatusOK)
	s.sql(t, `INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,
		revision,enrollment_digest,attested_until,attested_at)
		SELECT branch_id,device_ref,generation+1,'0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee',withdraw_ref,revision,
			sha256(enrollment_digest), attested_until, attested_at FROM ledger.rental_enrollment
			WHERE (branch_id,device_ref,generation)=($1,$2,$3)`, binding.Branch, binding.Board, binding.Generation)
	s.call(t, s.renter, http.MethodGet, renterPath, "", nil, http.StatusNotFound)
}
