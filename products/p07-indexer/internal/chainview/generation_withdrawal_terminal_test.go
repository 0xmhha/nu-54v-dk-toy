package chainview_test

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementext"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

func (s *combinedGenerationOpsd) proveWithdrawalSuccessor(t *testing.T, chain *ethclient.Client, mine, poll func(),
	path, closeHash, originalKey, originalReview string, original *types.Transaction) {
	t.Helper()
	token, err := testusdc.NewTestusdc(s.input.profile.Asset, chain)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := bind.NewKeyedTransactorWithChainID(s.input.operator, big.NewInt(8283))
	if err != nil {
		t.Fatal(err)
	}
	signer.Context = s.ctx
	signer.Nonce = new(big.Int).SetUint64(original.Nonce())
	different, err := token.SetRefusalMode(signer, false)
	if err != nil || different.Hash() == original.Hash() {
		t.Fatal("fixture must genuinely finalize a different transaction for the original nonce", err)
	}
	mine()
	poll()
	s.sql(t, "UPDATE ledger.rental_withdrawal_candidate SET reconcile_after=clock_timestamp()")
	for deadline := time.Now().Add(8 * time.Second); ; {
		var state string
		var encrypted bool
		err := s.db.QueryRow(s.ctx, `SELECT coalesce(r.status,''),w.sealed_transaction IS NOT NULL
			FROM ledger.rental_withdrawal_candidate w LEFT JOIN ledger.rental_withdrawal_resolution r USING(branch_id,device_ref,generation)`).Scan(&state, &encrypted)
		if err != nil {
			t.Fatal(err)
		}
		if state == "superseded" && !encrypted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not resolve the canonical competing transaction")
		}
		time.Sleep(100 * time.Millisecond)
	}
	type status struct {
		Withdrawal struct {
			State, CloseHash, ReviewRef, WithdrawalHash, PayoutHash string
			Attempt                                                 struct{ State, EvidenceHash string }
		}
	}
	decode := func(raw []byte) status {
		t.Helper()
		var value status
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	ready := decode(s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK)).Withdrawal
	if ready.State != "withdrawal-retry-review" || ready.CloseHash != closeHash || ready.ReviewRef == "" ||
		ready.Attempt.State != "superseded" || ready.Attempt.EvidenceHash != different.Hash().Hex() {
		t.Fatalf("still-owed terminal attempt did not require a new explicit review: %+v", ready)
	}
	wire := func(key, review string) []byte {
		body, _ := json.Marshal(map[string]string{"requestKey": key, "reviewRef": review})
		return body
	}
	s.call(t, s.operator, http.MethodPost, path, "application/json", wire(originalKey, originalReview), http.StatusAccepted)
	if s.withdrawalFault.sends.Load() != 1 {
		t.Fatal("canonical nonce consumption allowed transmission of the first transaction")
	}
	const successorKey = "267d31c2-32b4-4bc0-98fb-31340b545e12"
	s.call(t, s.operator, http.MethodPost, path, "application/json", wire(successorKey, strings.Repeat("b", 64)), http.StatusConflict)
	s.withdrawalFault.allowSuccessor.Store(true)
	successor := decode(s.call(t, s.operator, http.MethodPost, path, "application/json", wire(successorKey, ready.ReviewRef), http.StatusAccepted)).Withdrawal
	if successor.WithdrawalHash == "" || successor.WithdrawalHash == original.Hash().Hex() || s.withdrawalFault.sends.Load() != 2 {
		t.Fatal("new reviewed request did not publish exactly one distinct transaction")
	}
	s.restart(t)
	archived := decode(s.call(t, s.operator, http.MethodPost, path, "application/json", wire(originalKey, originalReview), http.StatusAccepted)).Withdrawal
	if archived.WithdrawalHash != original.Hash().Hex() || archived.Attempt.State != "superseded" {
		t.Fatalf("new review lost old request identity: %+v", archived)
	}
	mine()
	poll()
	payout := decode(s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK)).Withdrawal
	if payout.State != "withdrawal-received" || payout.PayoutHash != successor.WithdrawalHash || payout.CloseHash != closeHash || payout.ReviewRef != "" {
		t.Fatalf("invalid final result after explicitly reviewed successor: %+v", payout)
	}
	s.call(t, s.operator, http.MethodPost, path, "application/json", wire(successorKey, ready.ReviewRef), http.StatusAccepted)
	if s.withdrawalFault.sends.Load() != 2 {
		t.Fatal("known payout was transmitted again")
	}
}

func (s *combinedGenerationOpsd) proveWithdrawalAlreadyPaid(t *testing.T, chain *ethclient.Client, mine, poll func(),
	projection []byte, path, closeHash, originalKey, originalReview string, original *types.Transaction) {
	t.Helper()
	binding, _, err := chainview.ParseGenerationProjection(projection)
	if err != nil {
		t.Fatal(err)
	}
	ext, err := settlementext.NewSettlementext(binding.Settlement, chain)
	if err != nil {
		t.Fatal(err)
	}
	key, err := crypto.ToECDSA(append(make([]byte, 31), 232))
	if err != nil {
		t.Fatal(err)
	}
	payer, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(8283))
	if err != nil {
		t.Fatal(err)
	}
	payer.Context = s.ctx
	if err := chain.Client().CallContext(s.ctx, nil, "anvil_setBalance", payer.From, "0x56BC75E2D63100000"); err != nil {
		t.Fatal(err)
	}
	var thirdParty *types.Transaction
	advance := func() {
		thirdParty, err = ext.ExecuteWithdrawal(payer, binding.Account)
		if err != nil {
			t.Fatal(err)
		}
		mine()
		if os.Getenv("P05_O09_RECEIPT_ATTRIBUTION_UPGRADE") == "1" {
			poll()
			t.Run("Upgrade081To082", func(t *testing.T) {
				s.verifyWithdrawalAttributionUpgrade(t, chain, original.Hash(), thirdParty.Hash())
			})
		}
		if err := chain.SendTransaction(s.ctx, original); err != nil {
			t.Fatal(err)
		}
		mine()
		poll()
		revert, err := chain.TransactionReceipt(s.ctx, original.Hash())
		if err != nil || revert.Status != types.ReceiptStatusFailed || thirdParty.Hash() == original.Hash() {
			t.Fatal("fixture must produce genuine original revert after a distinct paid obligation", err)
		}
		s.sql(t, "UPDATE ledger.rental_withdrawal_candidate SET reconcile_after=clock_timestamp()")
		for deadline := time.Now().Add(8 * time.Second); ; {
			var outcome string
			var encrypted bool
			err := s.db.QueryRow(s.ctx, `SELECT coalesce(r.status,''),w.sealed_transaction IS NOT NULL
			FROM ledger.rental_withdrawal_candidate w LEFT JOIN ledger.rental_withdrawal_resolution r USING(branch_id,device_ref,generation)`).Scan(&outcome, &encrypted)
			if err != nil {
				t.Fatal(err)
			}
			if outcome == "reverted" && !encrypted {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("worker missed the exact failed original after a permissionless payout")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	if os.Getenv("P05_O09_CHROME_DIST") != "" {
		s.chromeWithdrawalAt(t, "stale-paid", closeHash, advance)
	} else {
		advance()
	}
	var settled struct {
		Withdrawal struct {
			State, CloseHash, ReviewRef, PayoutHash, WithdrawalHash string
			Attempt                                                 struct{ State, EvidenceHash string }
		}
	}
	raw := s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK)
	if err := json.Unmarshal(raw, &settled); err != nil || settled.Withdrawal.State != "withdrawal-received" ||
		settled.Withdrawal.PayoutHash != thirdParty.Hash().Hex() || settled.Withdrawal.WithdrawalHash != original.Hash().Hex() ||
		settled.Withdrawal.CloseHash != closeHash || settled.Withdrawal.ReviewRef != "" ||
		settled.Withdrawal.Attempt.State != "reverted" || settled.Withdrawal.Attempt.EvidenceHash != original.Hash().Hex() {
		t.Fatalf("failed original and paid obligation were conflated: %+v %v", settled, err)
	}
	wire := func(key, review string) []byte {
		body, _ := json.Marshal(map[string]string{"requestKey": key, "reviewRef": review})
		return body
	}
	response := s.call(t, s.operator, http.MethodPost, path, "application/json", wire(originalKey, originalReview), http.StatusAccepted)
	if !strings.Contains(string(response), original.Hash().Hex()) {
		t.Fatal("stale browser did not recover exact old request")
	}
	s.call(t, s.operator, http.MethodPost, path, "application/json", wire("267d31c2-32b4-4bc0-98fb-31340b545e12", originalReview), http.StatusConflict)
	if s.withdrawalFault.sends.Load() != 1 {
		t.Fatal("paid obligation was ever resubmitted through opsd")
	}
	events, err := ext.FilterWithdrawalExecuted(&bind.FilterOpts{Start: uint64(s.input.profile.StartHeight), Context: s.ctx}, []common.Address{binding.Account})
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	if !events.Next() || events.Event.Raw.TxHash != thirdParty.Hash() || events.Next() {
		t.Fatal("expected exactly the one permissionless payout")
	}
}

func (s *combinedGenerationOpsd) verifyWithdrawalAttributionUpgrade(t *testing.T, chain *ethclient.Client, original, payout common.Hash) {
	t.Helper()
	digest, err := hex.DecodeString(s.input.profile.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	final, err := chain.HeaderByNumber(s.ctx, big.NewInt(int64(rpc.FinalizedBlockNumber)))
	if err != nil {
		t.Fatal(err)
	}
	h := final.Hash()
	// The old worker marked the actual independent payout as an executed
	// original. Exercise the real pre-upgrade schema/function with that case.
	var stored bool
	err = s.db.QueryRow(s.ctx, `SELECT ledger.rental_withdrawal_reconcile_finish($1,$2,$3,$4,'executed',$5,$6,$7,$8,$9)`,
		s.input.expected.Branch, s.input.expected.Board, s.input.expected.Generation, original[:], payout[:], final.Number.Int64(), h[:],
		digest, strings.ToLower(s.input.profile.Contract.Hex())).Scan(&stored)
	if err != nil || !stored {
		t.Fatal("could not reproduce former third-party attribution", err)
	}
	migrateCombinedP05From(t, s, 81,
		"001eb79cf92e83d4f234c2a93c983cc2b0eff9513f0a201d591271b989b7f799",
		"db281423c1df2b953ef11a5627ed230087e41dc66518b49c372698936f4731b0")
	var state string
	var old, evidence []byte
	err = s.db.QueryRow(s.ctx, "SELECT status,original_hash,evidence_hash FROM ledger.rental_withdrawal_resolution").Scan(&state, &old, &evidence)
	if err != nil || state != "settled_other" || common.Hash(old) != original || common.Hash(evidence) != payout {
		t.Fatal("082 changed custody or original identity rather than its attribution", err)
	}
	var visible struct {
		Withdrawal struct {
			PayoutHash, ReviewRef string
			Attempt               struct{ State, OriginalHash, EvidenceHash string }
		}
	}
	raw := s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK)
	if err := json.Unmarshal(raw, &visible); err != nil || visible.Withdrawal.PayoutHash != payout.Hex() || visible.Withdrawal.ReviewRef != "" ||
		visible.Withdrawal.Attempt.State != "settled_other" || visible.Withdrawal.Attempt.OriginalHash != original.Hex() ||
		visible.Withdrawal.Attempt.EvidenceHash != payout.Hex() {
		t.Fatalf("unknown original and independent payout conflated: %+v", visible)
	}
}
