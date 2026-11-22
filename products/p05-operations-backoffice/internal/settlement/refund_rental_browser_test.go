package settlement

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5/pgxpool"
)

func verifyRentalRefundBrowser(t *testing.T, ctx context.Context, db *pgxpool.Pool, order seededOrder, actor string, browser *exec.Cmd) {
	t.Helper()
	account := lower(order.Order.Payer)
	wrong := lower(common.HexToAddress("0xf135"))
	device := fixtureUUID(t, db)
	fixtureExec(t, db, "INSERT INTO ledger.login_subject VALUES($1,$2) ON CONFLICT DO NOTHING", "refund-rental-"+actor, actor)
	fixtureExec(t, db, `INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,enrollment_digest,attested_until)
          VALUES($1,$2,1,$3,$4,'fixture',decode(repeat('09',32),'hex'),clock_timestamp()+interval '1 hour')`, order.Branch, device, account, wrong)
	fixtureExec(t, db, `INSERT INTO ledger.rental_operation(branch_id,device_ref,generation,action,request_key,actor_id,status)
          VALUES($1,$2,1,'close',gen_random_uuid(),$3,'acknowledged')`, order.Branch, device, actor)
	for _, stage := range []string{"held", "verified"} {
		if stage == "verified" {
			fixtureExec(t, db, "UPDATE ledger.rental_enrollment SET withdraw_ref=$3 WHERE branch_id=$1 AND device_ref=$2", order.Branch, device, account)
		}
		process := exec.CommandContext(ctx, browser.Args[0], browser.Args[1:]...)
		process.Env = append(os.Environ(), "NU54_RENTAL_PHASE="+stage)
		if out, err := process.CombinedOutput(); err != nil {
			t.Fatalf("rental refund UI %s: %v %s", stage, err, out)
		}
	}
	t.Log("O05 closed rental destination stayed held until the enrollment matched the original generation at finalized Anvil state")
}
