package evidencev2

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPostgresActiveEvidence(t *testing.T) {
	dsn := os.Getenv("P07_EVIDENCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires migrated scratch PostgreSQL")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || !(strings.HasPrefix(config.Host, "/tmp/") || strings.HasPrefix(config.Host, "/private/tmp/")) {
		t.Fatal("must use a local scratch Unix socket")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	if _, err := Open(ctx, dsn); err == nil {
		t.Fatal("administrator connection passed reader-role gate")
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	const generation = "987e50a6-a874-487c-8d85-1451edacfe99"
	const event = "03f43389-3f6b-4bc4-a1d7-6cb11af65ccb"
	const stream = "23a6f801-e925-42b9-b4fa-05f5d4f11cee"
	const block = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	const hash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	use := func(query string, values ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, query, values...); err != nil {
			t.Fatal(err)
		}
	}
	use(`INSERT INTO chainview.stream VALUES ($1,'8283','test-contract','test-abi','test-manifest')`, stream)
	use(`INSERT INTO chainview.generation (id,stream_id,base_height,fence,state) VALUES ($1,$2,12,1,'building')`, generation, stream)
	use(`INSERT INTO chainview.block VALUES ($1,$2,12,'prior',now(),true)`, generation, block)
	use(`UPDATE chainview.generation SET tip_height=12, finalized_height=12, tip_hash=$2, finalized_hash=$2, state='ready' WHERE id=$1`, generation, block)
	use(`INSERT INTO chainview.observed_event 
		(generation_id,block_hash,tx_hash,log_index,kind,receipt_success,recipient_ref,signer_nonce,merchant_key,order_key,asset_ref,amount)
		VALUES ($1,$2,$3,0,'paid',true,'recipient','1',$3,$3,'test-asset',10)`, generation, block, hash)
	use(`INSERT INTO chainview.evidence 
		(id,generation_id,merchant_key,order_key,kind,anchor_height,anchor_hash,policy_ref,log_block,tx_hash,log_index,created_at)
		VALUES ($1,$2,$4,$4,'paid',12,$3,'test-policy',$3,$4,0,now())`, event, generation, block, hash)
	use(`INSERT INTO chainview.read_target VALUES ($1,$2,0)`, stream, generation)
	use(`SET LOCAL ROLE nu54_p07_read`)
	row, err := finalized(ctx, tx, generation, event)
	if err != nil || row == nil || row.EventID != event || row.AnchorHash != block || row.TxHash != hash || row.Fact != "paid" {
		t.Fatalf("active finalized fact missing: %+v %v", row, err)
	}
	if row, err := finalized(ctx, tx, stream, event); err != nil || row != nil {
		t.Fatalf("generation mismatch visible: %+v %v", row, err)
	}
	use(`RESET ROLE`)
	use(`UPDATE chainview.generation SET state='frozen' WHERE id=$1`, generation)
	use(`SET LOCAL ROLE nu54_p07_read`)
	if row, err := finalized(ctx, tx, generation, event); err != nil || row != nil {
		t.Fatalf("frozen evidence visible: %+v %v", row, err)
	}
}
