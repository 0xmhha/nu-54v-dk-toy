package commerce_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5"
)

type installedUnpaidDriver struct {
	database                                                                *pgx.Conn
	chain                                                                   *ethclient.Client
	step                                                                    func(context.Context) (bool, error)
	binary, dsn, rpc, profiles, signatures, authority, generation, merchant string
}

func (d installedUnpaidDriver) expire(ctx context.Context, sale string) (bool, error) {
	var order string
	var expires time.Time
	var resolved bool
	err := d.database.QueryRow(ctx, `SELECT '0x'||encode(q.order_id,'hex'),q.expires_at,
		EXISTS(SELECT FROM ledger.sale_resolution WHERE sale_id=q.sale_id)
		FROM ledger.quote_work q JOIN ledger.settlement_submission a USING(sale_id)
		WHERE q.sale_id=$1 AND q.signed_digest IS NOT NULL AND a.received_at<q.expires_at`, sale).
		Scan(&order, &expires, &resolved)
	if err != nil || resolved {
		return resolved, err
	}
	if time.Now().Unix() < expires.Unix() {
		return false, nil
	}
	if !filepath.IsAbs(d.binary) || !filepath.IsAbs(d.profiles) || !filepath.IsAbs(d.signatures) {
		return false, errors.New("signed proof fixture paths unavailable")
	}
	head, err := d.chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return false, err
	}
	if err := d.chain.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", max(expires.Unix()+1, int64(head.Time)+1)); err != nil {
		return false, err
	}
	for range 9 {
		if err := d.chain.Client().CallContext(ctx, nil, "evm_mine"); err != nil {
			return false, err
		}
	}
	proof, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	for {
		command := exec.CommandContext(proof, d.binary, "prove-absence", d.generation, d.merchant, order, strconv.FormatInt(expires.Unix(), 10))
		command.Env = append(os.Environ(), "P07_CHAINVIEW_DATABASE_URL="+d.dsn, "P07_RPC="+d.rpc,
			"P07_CHAINVIEW_PROFILES="+d.profiles, "P07_CHAINVIEW_SIGNATURES="+d.signatures, "P07_CHAINVIEW_APPROVAL_KEY="+d.authority)
		if err := command.Run(); err == nil {
			break
		}
		select {
		case <-proof.Done():
			return false, errors.New("P07 finalized absence could not be established")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if did, err := d.step(ctx); err != nil || !did {
		return false, fmt.Errorf("P05 did not reconcile the original absence: %t %w", did, err)
	}
	err = d.database.QueryRow(ctx, `SELECT s.state='closed' AND EXISTS (
		   SELECT FROM ledger.sale_resolution r JOIN chainview.active_evidence e ON e.id=r.evidence_id AND e.kind='absent' AND e.order_key=$2
		   WHERE r.sale_id=s.id)
		 AND NOT EXISTS(SELECT FROM ledger.payment WHERE sale_id=s.id)
		 AND NOT EXISTS(SELECT FROM ledger.pickup WHERE sale_id=s.id)
		 AND NOT EXISTS(SELECT FROM ledger.relay_attempt WHERE work_id=s.id)
		 FROM ledger.sale s WHERE s.id=$1`, sale, order).Scan(&resolved)
	return resolved, err
}
