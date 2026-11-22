package settlement

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Upgrade only a seeded scratch database after the real P07 and P05
// reconciliation. The previous client read must happen before the migration.
func upgradeW11UnpaidArchive(t *testing.T, ctx context.Context, db *pgxpool.Pool, dsn, sale string, readOld func()) {
	t.Helper()
	requested := os.Getenv("NU54_W11_UNPAID_UPGRADE")
	if requested == "" {
		return
	}
	prior, err := strconv.Atoi(requested)
	if err != nil || (prior != 94 && prior != 95) {
		t.Fatal("W11 upgrade requires predecessor 94 or 95")
	}
	journal := func() (int, string) {
		t.Helper()
		var latest int
		var preserved string
		if err := db.QueryRow(ctx, `SELECT max(version),
			coalesce(jsonb_agg(jsonb_build_array(version,name,sha256,applied_at) ORDER BY version)
			  FILTER (WHERE version<96),'[]'::jsonb)::text FROM nu54_p05_migrations.revision`).Scan(&latest, &preserved); err != nil {
			t.Fatal(err)
		}
		return latest, preserved
	}
	original := func() string {
		t.Helper()
		var persisted string
		if err := db.QueryRow(ctx, `SELECT jsonb_build_array(s.revision,s.state,s.amount,q.signed_digest,
			e.evidence_id,e.resolved_at,oc.nearby_customer)::text
			FROM ledger.sale s JOIN ledger.quote_work q ON q.sale_id=s.id
			JOIN ledger.sale_resolution e ON e.sale_id=s.id
			JOIN ledger.menu_reservation_outcome oc ON (oc.branch_id,oc.sale_id)=(s.branch_id,s.id)
			WHERE s.id=$1`, sale).Scan(&persisted); err != nil {
			t.Fatal(err)
		}
		return persisted
	}
	if latest, _ := journal(); latest != prior {
		t.Fatalf("unexpected W11 upgrade predecessor: %d", latest)
	}
	readOld()
	_, before := journal()
	oldOrder := original()
	const run = `import os,sys
sys.path.insert(0,'../../../../products/p10-platform/database')
import migrate
revisions=migrate.migrations(migrate.OWNERS['p05'][1])
assert revisions[95].name == '096_closed_nearby_receipt.sql'
migrate.apply('p05',os.environ['P05_MIGRATION_DATABASE_URL'],revisions[:96])
`
	apply := func() {
		cmd := exec.CommandContext(ctx, "python3", "-B", "-c", run)
		cmd.Env = append(os.Environ(), "P05_MIGRATION_DATABASE_URL="+dsn)
		if _, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("production migrator could not apply 096 to scratch predecessor: %v", err)
		}
	}
	revision96 := func() string {
		t.Helper()
		var value string
		if err := db.QueryRow(ctx, `SELECT jsonb_build_array(name,sha256,applied_at)::text
			FROM nu54_p05_migrations.revision WHERE version=96`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	apply()
	applied := revision96()
	apply()
	if revision96() != applied {
		t.Fatal("reapplying 096 changed its recorded timestamp or digest")
	}
	latest, after := journal()
	// 094 also applies 095, while the existing 94 rows must remain byte-identical.
	if prior == 94 {
		if err := db.QueryRow(ctx, `SELECT jsonb_agg(jsonb_build_array(version,name,sha256,applied_at)
			ORDER BY version)::text FROM nu54_p05_migrations.revision WHERE version<=94`).Scan(&after); err != nil {
			t.Fatal(err)
		}
	}
	if latest != 96 || before != after || original() != oldOrder {
		t.Fatal("096 did not preserve the exact predecessor journal and closed original")
	}
}
