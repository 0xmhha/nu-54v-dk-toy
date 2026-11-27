package commerce

import (
	"context"
	"crypto/rand"
	"math/big"
	"strings"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core/ops"
	"github.com/ethereum/go-ethereum/common"
)

type testRentalOperator struct {
	actions  int
	unopened bool
	status   string
}

func (test *testRentalOperator) Observe(_ context.Context, member RentalMember) (ops.RentalObservation, error) {
	status := "active"
	if test.unopened {
		status = "unopened"
	} else if member.Hold {
		status = "payment-hold"
	} else if test.status != "" {
		status = test.status
	}
	return ops.RentalObservation{Status: status, Device: common.HexToAddress(member.Account),
		Destination: common.HexToAddress(member.Destination)}, nil
}

func (test *testRentalOperator) Submit(_ context.Context, _ RentalMember, action, _ string, _ *big.Int) (ops.RentalObservation, error) {
	test.actions++
	if action == "cleanup" {
		return ops.RentalObservation{Status: "cleaned"}, nil
	}
	hash := common.Hash{31: 77}
	return ops.RentalObservation{Status: "active", Submitted: &hash}, nil
}

func exerciseRentalHTTP(t *testing.T, execute func(string, ...any),
	call func(string, string, string, map[string]any, int) map[string]any, branch, foreign, actor, origin string, executor *testRentalOperator) {
	t.Helper()
	const device = "9d23b608-46eb-4fcd-a9a0-452f308d65c6"
	const key = "9619931c-4777-452d-b989-a0248ff3af0f"
	execute(`INSERT INTO ledger.finance_grant VALUES($1,$2,'rental_manage',now()+interval '1 hour')`, branch, actor)
	execute(`INSERT INTO ledger.rental_enrollment VALUES($1,$2,17,'0x3333333333333333333333333333333333333333',
      '0x4444444444444444444444444444444444444444','nu54_rev1',decode(repeat('5a',32),'hex'),now()+interval '1 hour',NULL)`, branch, device)
	path := "/v2/operations/rental/" + branch + "/" + device
	call("GET", "/v2/operations/rental/"+foreign+"/"+device+"?generation=17", origin, nil, 404)
	call("GET", path+"?generation=18", origin, nil, 404)
	view := call("GET", path+"?generation=17", origin, nil, 200)
	if view["attestedUntil"] == nil || view["attestedAt"] != nil || view["reviewRef"] == nil {
		t.Fatalf("legacy attestation lost its validity or gained an invented verification time: %v", view)
	}
	executor.unopened = true
	execute("UPDATE ledger.rental_enrollment SET attested_at=now()-interval '5 minutes' WHERE branch_id=$1 AND device_ref=$2", branch, device)
	openPath := path + "/open?generation=17"
	openKey := "6b5a76d2-0dcd-41b2-b13d-ef1c18a8251f"
	proposal := map[string]any{"requestKey": openKey, "amount": "25", "reviewRef": view["reviewRef"]}
	call("POST", openPath, origin, proposal, 409)
	if executor.actions != 0 {
		t.Fatal("a changed enrollment was submitted with an older review")
	}
	current := call("GET", path+"?generation=17", origin, nil, 200)
	if current["attestedAt"] == nil || current["attestedUntil"] == nil || current["reviewRef"] == view["reviewRef"] {
		t.Fatalf("changed enrollment evidence not visible: %v", current)
	}
	proposal["reviewRef"] = current["reviewRef"]
	call("POST", openPath, origin, proposal, 202)
	call("POST", openPath, origin, proposal, 202)
	proposal["amount"] = "26"
	call("POST", openPath, origin, proposal, 409)
	executor.unopened = false
	closePath := path + "/close?generation=17"
	pending := func(view map[string]any) map[string]any {
		t.Helper()
		review, ok := view["returnReview"].(map[string]any)
		if !ok || review["generation"] != "17" || review["account"] == nil || review["destination"] == nil {
			t.Fatalf("authorized return review did not identify original account, generation and destination: %v", view)
		}
		return review
	}
	unknown := pending(call("GET", path+"?generation=17", origin, nil, 200))
	if unknown["pendingState"] != "reconciliation-required" {
		t.Fatalf("missing evidence was treated as cleared: %v", unknown)
	}
	call("POST", closePath, origin, map[string]any{"requestKey": key}, 409)
	execute("UPDATE ledger.rental_enrollment SET no_pending_until=now()+interval '1 hour' WHERE branch_id=$1 AND device_ref=$2", branch, device)
	cleared := pending(call("GET", path+"?generation=17", origin, nil, 200))
	if cleared["pendingState"] != "clearance-current" || cleared["reviewRef"] == nil {
		t.Fatalf("current reconciliation clearance or review reference missing: %v", cleared)
	}
	assertGeneration := func(review map[string]any, state string) {
		t.Helper()
		got, ok := review["generationPending"].(map[string]any)
		if !ok || got["state"] != state || got["pendingCount"] != nil {
			t.Fatalf("generation-wide review expected %s without a premature zero: %v", state, review)
		}
	}
	readGeneration := func(state string) {
		t.Helper()
		assertGeneration(pending(call("GET", path+"?generation=17", origin, nil, 200)), state)
	}
	assertGeneration(unknown, "unknown")
	assertGeneration(cleared, "unknown") // station clearance never fills the proof model
	const freeze = "62451a4c-694e-4319-b04c-932dbad43640"
	// Database-owner fixtures exercise the projection. Production has no evidence writer yet.
	execute(`DO $$ BEGIN
		IF has_table_privilege('nu54_p05_commerce','ledger.rental_generation_freeze','INSERT,UPDATE,DELETE,TRUNCATE') OR
		   has_table_privilege('nu54_p05_commerce','ledger.rental_generation_reconciliation','INSERT,UPDATE,DELETE,TRUNCATE') THEN
			RAISE EXCEPTION 'commerce role can originate rental evidence';
		END IF;
	END $$`)
	execute(`INSERT INTO ledger.rental_generation_freeze
		SELECT branch_id,device_ref,generation,$3::uuid,account_ref,withdraw_ref,revision,enrollment_digest,
			8283,'0x2222222222222222222222222222222222222222',sha256('deployment'),sha256('board'),
			sha256('challenge'),sha256('freeze'),sha256('journal'),
			decode(repeat('00',31)||'20','hex'),decode(repeat('00',31)||'22','hex'),2,1,0,1,
			clock_timestamp(),clock_timestamp()+interval '15 minutes'
		FROM ledger.rental_enrollment WHERE branch_id=$1 AND device_ref=$2 AND generation=17`, branch, device, freeze)
	readGeneration("frozen")
	execute(`INSERT INTO ledger.rental_generation_reconciliation
		VALUES($1,2,'68f4dc5d-3359-4139-b09e-5d7fea65f45a',100,sha256('anchor'),sha256('assessment'),
			1,0,false,clock_timestamp(),clock_timestamp()+interval '5 minutes')`, freeze)
	readGeneration("incomplete")
	execute(`UPDATE ledger.rental_generation_reconciliation SET coverage_complete=true WHERE freeze_ref=$1`, freeze)
	readGeneration("incomplete")
	execute(`UPDATE ledger.rental_generation_reconciliation SET checked_entries=2,resolved_entries=2 WHERE freeze_ref=$1`, freeze)
	readGeneration("incomplete") // Internal consistency is not an authenticated reconciliation.
	execute(`DO $$ BEGIN
		IF ledger.rental_nonce_number(decode(repeat('ff',32),'hex')) -
		   ledger.rental_nonce_number(decode(repeat('ff',31)||'fd','hex')) <> 2 OR
		   ledger.rental_nonce_number(decode(repeat('00',30)||'0100','hex')) -
		   ledger.rental_nonce_number(decode(repeat('00',31)||'ff','hex')) <> 1 THEN
			RAISE EXCEPTION 'rental nonce range lost uint256 precision';
		END IF;
		BEGIN
			UPDATE ledger.rental_generation_freeze SET journal_entries=1,burned_entries=0;
			RAISE EXCEPTION 'omitted nonce accepted';
		EXCEPTION WHEN check_violation THEN NULL;
		END;
		BEGIN
			UPDATE ledger.rental_generation_freeze SET payment_entries=9223372036854775807,burned_entries=1;
			RAISE EXCEPTION 'overflowing allocation count accepted';
		EXCEPTION WHEN check_violation THEN NULL;
		END;
		BEGIN
			UPDATE ledger.rental_generation_reconciliation SET checked_entries=3;
			RAISE EXCEPTION 'assessment exceeded attested allocation range';
		EXCEPTION WHEN check_violation THEN NULL;
		END;
	END $$`)
	execute(`UPDATE ledger.rental_generation_reconciliation SET checked_at=now()-interval '1 hour',
		fresh_until=now()-interval '1 second' WHERE freeze_ref=$1`, freeze)
	readGeneration("incomplete")
	execute(`UPDATE ledger.rental_generation_freeze SET frozen_at=now()-interval '1 minute',
		fresh_until=now()-interval '1 second' WHERE freeze_ref=$1`, freeze)
	readGeneration("unknown")
	call("POST", closePath, origin, map[string]any{"requestKey": key}, 409)
	closeRequest := map[string]any{"requestKey": key, "reviewRef": cleared["reviewRef"]}
	execute("UPDATE ledger.rental_enrollment SET no_pending_until=no_pending_until+interval '1 minute' WHERE branch_id=$1 AND device_ref=$2", branch, device)
	call("POST", closePath, origin, closeRequest, 409)
	execute("UPDATE ledger.rental_enrollment SET no_pending_until=now()-interval '1 second' WHERE branch_id=$1 AND device_ref=$2", branch, device)
	if expired := pending(call("GET", path+"?generation=17", origin, nil, 200)); expired["pendingState"] != "reconciliation-required" {
		t.Fatalf("expired evidence was retained as cleared: %v", expired)
	}
	call("POST", closePath, origin, closeRequest, 409)
	execute("UPDATE ledger.rental_enrollment SET no_pending_until=now()+interval '1 hour' WHERE branch_id=$1 AND device_ref=$2", branch, device)
	closeRequest["reviewRef"] = pending(call("GET", path+"?generation=17", origin, nil, 200))["reviewRef"]
	execute("UPDATE ledger.rental_enrollment SET withdraw_ref='0x5555555555555555555555555555555555555555' WHERE branch_id=$1 AND device_ref=$2", branch, device)
	call("POST", closePath, origin, closeRequest, 409)
	closeRequest["reviewRef"] = pending(call("GET", path+"?generation=17", origin, nil, 200))["reviewRef"]
	execute(`INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until,no_pending_until)
	  VALUES($1,$2,18,'0x3333333333333333333333333333333333333334','0x4444444444444444444444444444444444444444',
	  'nu54_rev1',sha256('successor'),now()+interval '1 hour',now()+interval '1 hour')`, branch, device)
	call("POST", closePath, origin, closeRequest, 409)
	execute("DELETE FROM ledger.rental_enrollment WHERE branch_id=$1 AND device_ref=$2 AND generation=18", branch, device)
	if executor.actions != 1 {
		t.Fatalf("a stale or absent return review dispatched an action: %d", executor.actions)
	}
	execute("UPDATE ledger.finance_grant SET valid_until=now()-interval '1 second' WHERE branch_id=$1 AND actor_id=$2 AND capability='rental_manage'", branch, actor)
	call("POST", closePath, origin, closeRequest, 404)
	execute("UPDATE ledger.finance_grant SET valid_until=now()+interval '1 hour' WHERE branch_id=$1 AND actor_id=$2 AND capability='rental_manage'", branch, actor)
	call("POST", closePath, origin, closeRequest, 409)
	closeRequest["reviewRef"] = pending(call("GET", path+"?generation=17", origin, nil, 200))["reviewRef"]
	call("POST", closePath, origin, closeRequest, 202)
	call("POST", closePath, origin, closeRequest, 202)
	// Previously recorded commands may still be replayed by an older client.
	call("POST", closePath, origin, map[string]any{"requestKey": key}, 202)
	call("POST", closePath, origin, map[string]any{"requestKey": device}, 409)
	if executor.actions != 2 {
		t.Fatalf("rental command repeated %d times", executor.actions)
	}
	withdraw := map[string]any{"requestKey": "17185e71-a4c6-4463-88a0-72dde6d01f2b"}
	cleanup := map[string]any{"requestKey": "77292a87-113b-40ad-9238-823ec635e1e8"}
	withdrawPath, cleanupPath := path+"/withdraw?generation=17", path+"/cleanup?generation=17"
	execute("UPDATE ledger.rental_enrollment SET no_pending_until=NULL WHERE branch_id=$1 AND device_ref=$2", branch, device)
	executor.status = "withdrawal-ready"
	call("POST", withdrawPath, origin, withdraw, 409)
	executor.status = "cleanup-ready"
	call("POST", cleanupPath, origin, cleanup, 409)
	if executor.actions != 2 {
		t.Fatalf("missing clearance dispatched a new withdrawal or cleanup: %d", executor.actions)
	}
	execute("UPDATE ledger.rental_enrollment SET no_pending_until=now()+interval '1 hour' WHERE branch_id=$1 AND device_ref=$2", branch, device)
	executor.status = "withdrawal-ready"
	call("POST", withdrawPath, origin, withdraw, 202)
	call("POST", withdrawPath, origin, withdraw, 202)
	executor.status = "cleanup-ready"
	refused := call("POST", cleanupPath, origin, cleanup, 409)["cleanup"].(map[string]any)
	if refused["state"] != "protected-evidence-unavailable" {
		t.Fatalf("legacy chain observation accepted as board proof: %v", refused)
	}
	// A previously staged successor must not open after historical generic cleanup.
	var successorAccount common.Address
	if _, err := rand.Read(successorAccount[:]); err != nil {
		t.Fatal(err)
	}
	execute(`INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until,attested_at)
		VALUES($1,$2,18,$3,'0x4444444444444444444444444444444444444444',
		'nu54_rev1',decode(repeat('7a',32),'hex'),now()+interval '1 hour',now())`, branch, device,
		strings.ToLower(successorAccount.Hex()))
	// Historical ACK remains readable for audit and cannot be replayed as cleanup.
	execute(`INSERT INTO ledger.rental_operation(branch_id,device_ref,generation,action,request_key,actor_id,status,evidence)
		VALUES($1,$2,17,'cleanup',$3,$4,'acknowledged','{"status":"cleaned"}')`, branch, device, cleanup["requestKey"], actor)
	call("POST", cleanupPath, origin, cleanup, 409)
	execute("UPDATE ledger.rental_operation SET status='submitted' WHERE branch_id=$1 AND device_ref=$2 AND action='cleanup'", branch, device)
	executor.unopened = true
	successor := call("GET", path+"?generation=18", origin, nil, 200)
	if successor["cleanupBlocked"] != true {
		t.Fatalf("historical submitted cleanup did not fence successor: %v", successor)
	}
	call("POST", path+"/open?generation=18", origin, map[string]any{"requestKey": "d8f91afc-2310-4b40-9410-f3749b079952", "amount": "25", "reviewRef": successor["reviewRef"]}, 409)
	executor.unopened = false
	execute("UPDATE ledger.rental_enrollment SET no_pending_until=NULL WHERE branch_id=$1 AND device_ref=$2", branch, device)
	call("POST", withdrawPath, origin, withdraw, 202)
	if executor.actions != 3 {
		t.Fatalf("withdrawal repeated or legacy reset dispatched: %d", executor.actions)
	}

}
