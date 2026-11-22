package settlement

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	v2 "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementv2"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

func w10BoardDecision(t *testing.T, ctx context.Context, order seededOrder, buyer fixtureSigner, refused bool) []byte {
	t.Helper()
	board := os.Getenv("NU54_P01_INSTALL_TEST")
	if !filepath.IsAbs(board) {
		t.Fatal("W10 requires the C board host executable")
	}
	challenge, err := exec.CommandContext(ctx, board, "--display-challenge").Output()
	if err != nil || len(challenge) != 32 {
		t.Fatalf("board display challenge: %v", err)
	}
	display, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := x509.MarshalPKIXPublicKey(&display.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	message := append(append([]byte("N54D"), challenge...), order.Signed...)
	digest := sha256.Sum256(message)
	r, s, err := ecdsa.Sign(rand.Reader, display, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	proof := make([]byte, 64)
	r.FillBytes(proof[:32])
	s.FillBytes(proof[32:])
	dir := t.TempDir()
	env := make([]string, 0, 4)
	for _, file := range []struct {
		option string
		data   []byte
	}{
		{"KEY", pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicKey})},
		{"PROOF", proof},
		{"CHALLENGE", challenge},
	} {
		name := filepath.Join(dir, file.option)
		if err := os.WriteFile(name, file.data, 0600); err != nil {
			t.Fatal(err)
		}
		env = append(env, "NU54_INSTALL_DISPLAY_"+file.option+"="+name)
	}
	if refused {
		env = append(env, "NU54_INSTALL_TEST_REFUSE=1")
	}
	return pilotBoardDecision(t, ctx, order, buyer, "management", env, refused)
}

func verifyW10SponsoredSettlement(t *testing.T, ctx context.Context, chain *ethclient.Client, db *pgxpool.Pool,
	token *testusdc.Testusdc, contract *v2.Settlementv2, order seededOrder, sponsor common.Address,
	relay *lostResponseChain, step func() error) string {
	t.Helper()
	snapshot := func() [4]*big.Int {
		t.Helper()
		merchant, tokenErr := token.BalanceOf(nil, order.Quote.Args.Beneficiary)
		escrow, escrowErr := contract.EscrowBalance(nil, order.Order.Payer, 1)
		customerGas, customerErr := chain.BalanceAt(ctx, order.Order.Payer, nil)
		sponsorGas, sponsorErr := chain.BalanceAt(ctx, sponsor, nil)
		if err := errors.Join(tokenErr, escrowErr, customerErr, sponsorErr); err != nil {
			t.Fatal(err)
		}
		return [4]*big.Int{merchant, escrow, customerGas, sponsorGas}
	}
	before := snapshot()
	if err := step(); err != nil {
		t.Fatal(err)
	}
	if err := chain.Client().CallContext(ctx, nil, "evm_setAutomine", false); err != nil {
		t.Fatal(err)
	}
	if err := step(); !errors.Is(err, errLostResponse) {
		t.Fatalf("SW1 must retain the original attempt after one lost chain reply: %v", err)
	}
	var txHash string
	if err := db.QueryRow(ctx, "SELECT tx_hash FROM ledger.relay_attempt WHERE work_id=$1", order.Sale).Scan(&txHash); err != nil {
		t.Fatal(err)
	}
	hash := common.HexToHash(txHash)
	head, err := chain.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	inclusion := max(int64(head.Time)+1, order.Order.Issued.Unix()+1)
	if err := chain.Client().CallContext(ctx, nil, "evm_setNextBlockTimestamp", inclusion); err != nil {
		t.Fatal(err)
	}
	mine(t, ctx, chain, 1)
	tx, pending, err := chain.TransactionByHash(ctx, hash)
	if err != nil || pending {
		t.Fatalf("original sponsor transaction unavailable: %v", err)
	}
	sender, err := types.Sender(types.LatestSignerForChainID(big.NewInt(8283)), tx)
	if err != nil || sender != sponsor || tx.To() == nil || *tx.To() != order.Target.Contract {
		t.Fatalf("unexpected sender or destination for original sponsor transaction: %v", err)
	}
	receipt, err := chain.TransactionReceipt(ctx, hash)
	if err != nil || receipt.Status != 1 {
		t.Fatalf("original sponsor transaction failed: %v", err)
	}
	mine(t, ctx, chain, 5)
	waitUntil(t, func() bool {
		var found bool
		err := db.QueryRow(ctx, "SELECT EXISTS (SELECT FROM chainview.finalized_receipt WHERE tx_hash=$1 AND kind='paid')", txHash).Scan(&found)
		return err == nil && found
	})
	if err := step(); err != nil {
		t.Fatal(err)
	}
	after := snapshot()
	gas := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
	if gas.Sign() <= 0 || new(big.Int).Sub(after[0], before[0]).Cmp(order.Quote.Args.Amount) != 0 ||
		new(big.Int).Sub(before[1], after[1]).Cmp(order.Quote.Args.Amount) != 0 || after[2].Cmp(before[2]) != 0 ||
		new(big.Int).Sub(before[3], after[3]).Cmp(gas) != 0 {
		t.Fatal("the original principal and sponsored gas did not settle to the expected accounts")
	}
	var submissions, attempts, payments, journals, events int
	if err := db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.relay_attempt WHERE work_id=$1),
		(SELECT count(*) FROM ledger.payment WHERE sale_id=$1 AND tx_hash=$2),
		(SELECT count(*) FROM ledger.journal WHERE sale_id=$1),
		(SELECT count(*) FROM ledger.fulfillment_outbox WHERE sale_id=$1)`, order.Sale, txHash).
		Scan(&submissions, &attempts, &payments, &journals, &events); err != nil ||
		relay.sends != 1 || submissions != 1 || attempts != 1 || payments != 1 || journals != 1 || events != 1 {
		t.Fatalf("sponsored board payment did not converge once: sends=%d, submissions=%d, attempts=%d, payments=%d, journals=%d, events=%d: %v",
			relay.sends, submissions, attempts, payments, journals, events, err)
	}
	t.Log("W10 SW1: exact C board approval, one C2 admission, one 8283 sponsored broadcast, one P07-confirmed payment")
	return txHash
}
