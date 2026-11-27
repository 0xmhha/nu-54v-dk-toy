package settlement

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pilotBatch struct {
	ids            []string
	approved, paid time.Time
	clearedUnits   string
}

type batchPayment struct {
	Sale     string `json:"saleId"`
	Hash     string `json:"transactionHash"`
	Digest   string `json:"quoteDigest"`
	Evidence string `json:"evidenceId"`
	Journal  string `json:"journalId"`
	Event    string `json:"consumerEventId"`
	Receipt  string `json:"receiptRef"`
}

func saveBulkReport(t *testing.T, out, name string, report any) {
	t.Helper()
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, name), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func preparePilotBatch(t *testing.T, ctx context.Context, db *pgxpool.Pool, rpc *ethclient.Client,
	token *testusdc.Testusdc, contract *v2.Settlementv2, client *http.Client, url string,
	source seededOrder, target Target, ids []string, out string, owners []pilotIdentity) *pilotBatch {
	t.Helper()
	key, err := crypto.ToECDSA(common.LeftPadBytes([]byte{100}, 32))
	if err != nil {
		t.Fatal(err)
	}
	operator, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(8283))
	if err != nil {
		t.Fatal(err)
	}
	operator.Context, operator.GasLimit = ctx, 500_000
	settled := func(tx *types.Transaction, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := bind.WaitMined(ctx, rpc, tx)
		if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
			t.Fatalf("bulk escrow fixture failed: %v", err)
		}
	}
	principal := big.NewInt(int64(len(ids) * 10))
	beneficiary, err := token.BalanceOf(nil, source.Quote.Args.Beneficiary)
	if err != nil {
		t.Fatal(err)
	}
	settled(token.Mint(operator, operator.From, principal))
	settled(token.Approve(operator, target.Contract, principal))
	if len(owners) == 0 {
		settled(contract.DepositEscrow(operator, source.Order.Payer, 1, principal))
	} else {
		nonce, err := rpc.PendingNonceAt(ctx, operator.From)
		if err != nil {
			t.Fatal(err)
		}
		var latest *types.Transaction
		for _, owner := range owners {
			operator.Nonce = new(big.Int).SetUint64(nonce)
			if _, err := contract.RegisterPayer(operator, owner.board.Address(), 1, 1, owner.board.Address()); err != nil {
				t.Fatal(err)
			}
			nonce++
			operator.Nonce = new(big.Int).SetUint64(nonce)
			latest, err = contract.DepositEscrow(operator, owner.board.Address(), 1, big.NewInt(10))
			if err != nil {
				t.Fatal(err)
			}
			nonce++
		}
		settled(latest, nil)
	}
	issued := time.Now().UTC().Truncate(time.Second)
	batch := &pilotBatch{ids: ids}
	payloads := make([][]byte, len(ids))
	commands := make([]service.Request, len(ids))
	var official []seededOrder
	if len(owners) > 0 {
		official = genuineBulkQuotes(t, ctx, db, url, source, ids, owners, out)
	}
	for i, sale := range ids {
		item := source
		item.Sale = sale
		board := fixtureSigner(102)
		if len(owners) == 0 {
			item = seedPayerIssue(t, db, item, issued, "10", byte(i+110), 1, 120*time.Second)
		} else {
			item, board = official[i], owners[i].board
		}
		signature := pilotBoardApproval(t, ctx, item, board, "management")
		commands[i] = service.Request{Version: 2, Contract: "C3", Operation: "payment.forward",
			RequestID: fixtureUUID(t, db), Domain: item.Branch, Object: sale, IdempotencyKey: fixtureUUID(t, db),
			ExpiresAt: time.Now().Unix() + 40, Body: map[string]string{"quoteDigest": rawHex(item.Quote.Digest[:]),
				"approvalDigest": rawHex(item.Quote.ApprovalHash[:])}}
		raw, err := json.Marshal(map[string]any{"command": commands[i], "approvalSignature": rawHex(signature)})
		if err != nil {
			t.Fatal(err)
		}
		payloads[i] = raw
	}
	start := make(chan struct{})
	failures := make(chan error, len(ids))
	var submitters sync.WaitGroup
	batch.approved = time.Now().UTC()
	for i, sale := range ids {
		submitters.Go(func() {
			<-start
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/v2/commerce", bytes.NewReader(payloads[i]))
			if err != nil {
				failures <- err
				return
			}
			req.Header.Set("Content-Type", "application/json")
			httpClient := client
			if len(owners) > 0 {
				httpClient = owners[i].client
			}
			response, err := httpClient.Do(req)
			if err != nil {
				failures <- err
				return
			}
			defer response.Body.Close()
			result, err := service.DecodeReply(io.LimitReader(response.Body, 8192))
			if err != nil || response.StatusCode != http.StatusOK || result.RequestID != commands[i].RequestID || result.ResultRef != sale {
				failures <- fmt.Errorf("approval %d failed (HTTP %d %s): %v", i, response.StatusCode, result.Error, err)
			}
		})
	}
	close(start)
	submitters.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var attempts, paid, reverted int
	timeout := time.NewTimer(105 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for paid < len(ids) {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-timeout.C:
			saveBulkReport(t, out, "bulk-incomplete.json", map[string]any{"paid": paid, "relayAttempts": attempts, "reverted": reverted})
			t.Fatalf("bulk approvals missed deadline: paid=%d attempts=%d reverts=%d", paid, attempts, reverted)
		case <-tick.C:
			mine(t, ctx, rpc, 1)
			err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.payment WHERE sale_id::text=ANY($1)),
				(SELECT count(*) FROM ledger.relay_attempt WHERE work_id::text=ANY($1)),
				(SELECT count(*) FROM ledger.relay_attempt WHERE work_id::text=ANY($1) AND state='reverted')`, ids).
				Scan(&paid, &attempts, &reverted)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	batch.paid = time.Now().UTC()
	if attempts != len(ids) || reverted != 0 {
		t.Fatalf("bulk relay attempts=%d reverted=%d", attempts, reverted)
	}
	newBalance, err := token.BalanceOf(nil, source.Quote.Args.Beneficiary)
	if err != nil || newBalance.Sub(newBalance, beneficiary).Cmp(principal) != 0 {
		t.Fatalf("bulk beneficiary increase differs from committed gross: %v %v", newBalance, err)
	}
	batch.clearedUnits = principal.String()
	var outbox, premature int
	err = db.QueryRow(ctx, `SELECT count(*),count(c.event_id) FROM ledger.fulfillment_outbox e
		LEFT JOIN ledger.consumer_receipt c USING(event_id) WHERE e.sale_id::text=ANY($1) AND e.kind='paid'`, ids).
		Scan(&outbox, &premature)
	if err != nil || outbox != len(ids) || premature != 0 {
		t.Fatalf("expected 100 queued events while notification worker offline, got %d/%d: %v", outbox, premature, err)
	}
	return batch
}

func (b *pilotBatch) finish(t *testing.T, ctx context.Context, db *pgxpool.Pool, rpc *ethclient.Client, available time.Time, out string) {
	t.Helper()
	timeout := time.NewTimer(time.Until(available.Add(time.Minute)))
	defer timeout.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	var done int
	for done < len(b.ids) {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-timeout.C:
			saveBulkReport(t, out, "bulk-incomplete.json", map[string]int{"paid": len(b.ids), "durableInboxes": done})
			t.Fatalf("only %d/%d durable deliveries after 60 seconds", done, len(b.ids))
		case <-tick.C:
			if err := db.QueryRow(ctx, `SELECT count(*) FROM ledger.fulfillment_outbox e
				JOIN ledger.consumer_receipt c USING(event_id) JOIN ledger.fulfillment_inbox i USING(event_id)
				WHERE e.sale_id::text=ANY($1) AND e.kind='paid' AND e.state='delivered' AND c.payload_digest=i.payload_digest`,
				b.ids).Scan(&done); err != nil {
				t.Fatal(err)
			}
		}
	}
	observed := time.Now().UTC()
	rows, err := db.Query(ctx, `SELECT p.sale_id::text,p.tx_hash,c.quote_digest,c.id::text,j.id::text,e.event_id::text,k.receipt_ref::text
		FROM ledger.payment p JOIN chainview.settlement_receipt c ON p.evidence_id=c.id
		JOIN ledger.quote_work q ON q.sale_id=p.sale_id AND '0x'||encode(q.signed_digest,'hex')=c.quote_digest
		JOIN ledger.journal j ON j.sale_id=p.sale_id AND j.refund_id IS NULL JOIN ledger.pickup k ON k.sale_id=p.sale_id
		JOIN ledger.fulfillment_outbox e ON e.sale_id=p.sale_id AND e.kind='paid'
		JOIN ledger.relay_attempt r ON r.work_id=p.sale_id AND r.tx_hash=p.tx_hash
		WHERE p.sale_id::text=ANY($1) AND r.broadcast_fence>0 AND e.authorized_fence=e.fence
		  AND c.amount=p.principal AND p.tx_hash=c.tx_hash ORDER BY p.sale_id`, b.ids)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	hashes, events, sales, receipts := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	var payments []batchPayment
	for rows.Next() {
		var item batchPayment
		if err := rows.Scan(&item.Sale, &item.Hash, &item.Digest, &item.Evidence, &item.Journal, &item.Event, &item.Receipt); err != nil {
			t.Fatal(err)
		}
		if hashes[item.Hash] || events[item.Event] || sales[item.Sale] || receipts[item.Receipt] {
			t.Fatal("duplicate payment or kitchen event in cohort")
		}
		hashes[item.Hash], events[item.Event], sales[item.Sale], receipts[item.Receipt] = true, true, true, true
		payments = append(payments, item)
	}
	if err := rows.Err(); err != nil || len(payments) != len(b.ids) {
		t.Fatalf("linked active finalized receipts are incomplete: %d %v", len(payments), err)
	}
	var gas uint64
	nativeGasCost := new(big.Int)
	for _, item := range payments {
		receipt, err := rpc.TransactionReceipt(ctx, common.HexToHash(item.Hash))
		if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
			t.Fatalf("cohort chain receipt missing: %v", err)
		}
		gas += receipt.GasUsed
		nativeGasCost.Add(nativeGasCost, new(big.Int).Mul(receipt.EffectiveGasPrice, new(big.Int).SetUint64(receipt.GasUsed)))
	}
	elapsed := observed.Sub(available)
	if elapsed > time.Minute {
		t.Fatalf("convergence time %s exceeds 60s", elapsed)
	}
	saveBulkReport(t, out, "bulk-settlement.json", map[string]any{
		"mode":                "actual opsd single relay and single dispatcher, loopback Anvil/PostgreSQL",
		"concurrentApprovals": len(b.ids), "distinctFinalizedPayments": len(hashes), "distinctKitchenEvents": len(events),
		"distinctReceipts": len(receipts), "duplicatePaymentOrKitchenEvents": 0,
		"beneficiaryIncreaseBaseUnits": b.clearedUnits, "gasUsed": gas, "gasCostNativeBaseUnits": nativeGasCost.String(),
		"relayAndFinalityMS": b.paid.Sub(b.approved).Milliseconds(), "readyToDurableInboxMS": elapsed.Milliseconds(),
		"approvedAt": b.approved, "allPaidAt": b.paid, "dispatchResumedAt": available.UTC(), "allInboxesAt": observed,
		"payments": payments,
	})
	t.Logf("bulk actual P06 and kitchen: %d/%d, chain+posting %.3fs, resumed queue %.3fs",
		len(hashes), len(events), b.paid.Sub(b.approved).Seconds(), elapsed.Seconds())
}
