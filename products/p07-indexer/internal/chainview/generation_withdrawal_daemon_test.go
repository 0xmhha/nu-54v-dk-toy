package chainview_test

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/settlementext"
	"github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/testusdc"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type withdrawalPublishFault struct {
	first          chan []byte
	sends          atomic.Int32
	original       atomic.Value
	allowSuccessor atomic.Bool
}

func interceptFirstRentalSend(t *testing.T, origin, methodName string) (*withdrawalPublishFault, string) {
	t.Helper()
	target, err := url.Parse(origin)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	ext, err := settlementext.SettlementextMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	method := ext.Methods[methodName]
	fault := &withdrawalPublishFault{first: make(chan []byte, 1)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var call struct {
			Method string
			Params []string
		}
		if json.Unmarshal(body, &call) == nil && call.Method == "eth_sendRawTransaction" && len(call.Params) == 1 {
			raw, err := hexutil.Decode(call.Params[0])
			var tx types.Transaction
			if err == nil && tx.UnmarshalBinary(raw) == nil && bytes.HasPrefix(tx.Data(), method.ID) {
				if fault.sends.Add(1) == 1 {
					fault.original.Store(raw)
					fault.first <- raw
					<-r.Context().Done()
					return
				}
				if !fault.allowSuccessor.Load() && !bytes.Equal(fault.original.Load().([]byte), raw) {
					t.Error("process restart produced a different signed rental transaction")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
			}
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return fault, server.URL
}

func (s *combinedGenerationOpsd) supervisedWithdrawal(t *testing.T, dsn string, projection, receipt []byte) {
	t.Helper()
	s.recordWithExpectedPending(t, projection, receipt, 0)
	chain, err := ethclient.DialContext(s.ctx, s.input.rpc)
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Close()
	ingest, err := chainview.Open(s.ctx, dsn+" user=rental_o09_test_ingest")
	if err != nil {
		t.Fatal(err)
	}
	defer ingest.Close()
	source, err := chainview.Dial(s.ctx, s.input.rpc, s.input.profile)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	ingester := chainview.Reader{Profile: s.input.profile, DB: ingest, Source: source}
	poll := func() {
		t.Helper()
		if err := ingester.Poll(s.ctx); err != nil {
			t.Fatal(err)
		}
	}
	mine := func() {
		t.Helper()
		if err := chain.Client().CallContext(s.ctx, nil, "anvil_mine", 5); err != nil {
			t.Fatal(err)
		}
	}
	type result struct {
		Observation  struct{ Status string }
		ReturnReview struct{ ReviewRef string }
		Cleanup      struct{ State, RequestKey string }
		Withdrawal   struct {
			State, CloseHash, WithdrawalHash, PayoutHash, AcceptanceHash, ReviewRef, RecoveryReviewRef, Amount, RequestKey string
			WithdrawAfter                                                                                                  uint64
		}
	}
	decode := func(raw []byte) result {
		t.Helper()
		var value result
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	get := func() result {
		t.Helper()
		return decode(s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK))
	}
	base, query, _ := strings.Cut(s.reviewPath, "?")
	closePath, withdrawalPath := base+"/close?"+query, base+"/withdraw?"+query
	cleanupPath := base + "/cleanup?" + query
	const cleanupKey = "032efd71-6d85-48d0-9183-6a7413a90c60"
	cleanupWire := func(key string) []byte {
		body, _ := json.Marshal(map[string]string{"requestKey": key})
		return body
	}
	verifyPendingCleanup := func(phase, hash string) string {
		t.Helper()
		original := cleanupKey
		if key := s.chromeWithdrawal(t, phase, hash); key != "" {
			original = key
		}
		for i := 0; i < 2; i++ {
			out := decode(s.call(t, s.operator, http.MethodPost, cleanupPath, "application/json", cleanupWire(original), http.StatusAccepted))
			if out.Cleanup.State != "board-receipt-required" || out.Cleanup.RequestKey != original {
				t.Fatalf("cleanup accepted without a distinct board-evidence hold: %+v", out.Cleanup)
			}
		}
		s.call(t, s.operator, http.MethodPost, cleanupPath, "application/json", cleanupWire("409a6c3c-8a23-487d-8055-cce2ed9f9427"), http.StatusConflict)
		view := get()
		if view.Cleanup.State != "board-receipt-required" || view.Cleanup.RequestKey != original || view.Observation.Status == "cleaned" {
			t.Fatalf("reported cleanup without a board receipt: %+v", view)
		}
		var pins, legacy int
		err := s.db.QueryRow(s.ctx, `SELECT
			(SELECT count(*) FROM ledger.rental_cleanup_intent WHERE request_key=$1 AND octet_length(disposition)=32),
			(SELECT count(*) FROM ledger.rental_operation WHERE action='cleanup')`, original).Scan(&pins, &legacy)
		if err != nil || pins != 1 || legacy != 0 {
			t.Fatalf("exactly one pending original and zero generic dispatches required: %d/%d %v", pins, legacy, err)
		}
		return original
	}
	wire := func(key, review string, recovery ...string) []byte {
		command := map[string]string{"requestKey": key, "reviewRef": review}
		if len(recovery) > 0 {
			command["recoveryReviewRef"] = recovery[0]
		}
		body, _ := json.Marshal(command)
		return body
	}
	closeKey := "e39c66cd-adb5-4534-9100-a2018f7bbf05"
	withdrawalKey := "a6996169-0dc9-48b3-be7a-e73676d6b776"
	closeRef := get().ReturnReview.ReviewRef
	s.call(t, s.operator, http.MethodPost, closePath, "application/json", wire(closeKey, closeRef), http.StatusAccepted)
	mine()
	s.call(t, s.operator, http.MethodPost, closePath, "application/json", wire(closeKey, closeRef), http.StatusAccepted)
	out := get()
	if out.Withdrawal.State != "withdrawal-indeterminate" || out.Withdrawal.Amount != "" {
		t.Fatal("P07 lag exposed protected withdrawal approval")
	}
	s.chromeWithdrawal(t, "indeterminate", out.Withdrawal.CloseHash)
	s.call(t, s.operator, http.MethodPost, cleanupPath, "application/json", cleanupWire(cleanupKey), http.StatusConflict)
	poll()
	delay := get().Withdrawal
	if os.Getenv("P05_O09_PROTECTED_WITHDRAWAL") == "zero" {
		s.chromeWithdrawal(t, "zero", delay.CloseHash)
		if delay.State != "withdrawal-no-payout" || delay.Amount != "0" || delay.CloseHash == "" ||
			delay.ReviewRef != "" || delay.WithdrawalHash != "" || delay.PayoutHash != "" {
			t.Fatalf("zero receipt did not bind original close: %+v", delay)
		}
		s.call(t, s.operator, http.MethodPost, withdrawalPath, "application/json", wire(withdrawalKey, strings.Repeat("a", 64)), http.StatusConflict)
		var count int
		if err := s.db.QueryRow(s.ctx, "SELECT count(*) FROM ledger.rental_withdrawal_candidate").Scan(&count); err != nil || count != 0 {
			t.Fatal("zero receipt initiated a payout", err)
		}
		original := verifyPendingCleanup("cleanup-zero", delay.CloseHash)
		t.Run("QualifiedStationIssuesOriginal", func(t *testing.T) {
			s.verifyQualifiedCleanupIssuer(t, chain, original, delay.CloseHash, common.Hash{}, common.Hash{}, big.NewInt(0), mine, poll)
		})
		return
	}
	if delay.State != "withdrawal-delay" || delay.Amount != "25" || delay.WithdrawAfter == 0 || delay.ReviewRef != "" {
		t.Fatalf("withdrawal delay lost original receipt: %+v", delay)
	}
	s.chromeWithdrawal(t, "delay", delay.CloseHash)
	s.call(t, s.operator, http.MethodPost, withdrawalPath, "application/json", wire(withdrawalKey, strings.Repeat("a", 64)), http.StatusConflict)
	var accept func()
	if os.Getenv("P05_O09_CHROME_HOLD") == "1" {
		token, err := testusdc.NewTestusdc(s.input.profile.Asset, chain)
		if err != nil {
			t.Fatal(err)
		}
		recipient, err := bind.NewKeyedTransactorWithChainID(s.input.operator, big.NewInt(8283))
		if err != nil {
			t.Fatal(err)
		}
		recipient.Context = s.ctx
		id, err := token.NextHeldId(&bind.CallOpts{Context: s.ctx})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := token.SetRefusalMode(recipient, true); err != nil {
			t.Fatal(err)
		}
		accept = func() {
			t.Helper()
			if _, err := token.AcceptTransfer(recipient, id); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := chain.Client().CallContext(s.ctx, nil, "evm_setNextBlockTimestamp", delay.WithdrawAfter+1); err != nil {
		t.Fatal(err)
	}
	mine()
	poll()
	ready := get().Withdrawal
	if ready.State != "withdrawal-ready" || ready.ReviewRef == "" || ready.CloseHash != delay.CloseHash || ready.Amount != delay.Amount {
		t.Fatalf("matured protected withdrawal differs: %+v", ready)
	}
	if key := s.chromeWithdrawal(t, "ready", ready.CloseHash); key != "" {
		withdrawalKey = key
	}
	// Amount and route are known, but no terminal payout exists yet.
	s.call(t, s.operator, http.MethodPost, cleanupPath, "application/json", cleanupWire(cleanupKey), http.StatusConflict)
	var first result
	if s.withdrawalFault == nil {
		first = decode(s.call(t, s.operator, http.MethodPost, withdrawalPath, "application/json", wire(withdrawalKey, ready.ReviewRef), http.StatusAccepted))
	} else {
		lost := make(chan error, 1)
		go func() {
			req, err := http.NewRequestWithContext(s.ctx, http.MethodPost, s.origin+withdrawalPath, bytes.NewReader(wire(withdrawalKey, ready.ReviewRef)))
			if err == nil {
				req.Header.Set("Origin", s.origin)
				req.Header.Set("Content-Type", "application/json")
				resp, callErr := s.operator.Do(req)
				err = callErr
				if resp != nil {
					_ = resp.Body.Close()
				}
			}
			lost <- err
		}()
		var original types.Transaction
		select {
		case raw := <-s.withdrawalFault.first:
			if err := original.UnmarshalBinary(raw); err != nil {
				t.Fatal(err)
			}
		case <-time.After(7 * time.Second):
			t.Fatal("did not reach admitted first withdrawal send")
		}
		var stored, sealed []byte
		if err := s.db.QueryRow(s.ctx, "SELECT tx_hash,sealed_transaction FROM ledger.rental_withdrawal_candidate").Scan(&stored, &sealed); err != nil ||
			len(sealed) == 0 || common.Hash(stored) != original.Hash() {
			t.Fatal("first network attempt preceded durable original", err)
		}
		if os.Getenv("P05_O09_RETRY_WITHDRAWAL") == "third-party" {
			s.sql(t, "UPDATE ledger.rental_withdrawal_candidate SET reconcile_after=clock_timestamp()+interval '1 hour'")
		}
		s.restart(t)
		if err := <-lost; err == nil {
			t.Fatal("first response survived opsd shutdown")
		}
		if os.Getenv("P05_O09_RETRY_WITHDRAWAL") == "1" {
			s.proveWithdrawalSuccessor(t, chain, mine, poll, withdrawalPath, delay.CloseHash, withdrawalKey, ready.ReviewRef, &original)
			return
		}
		if os.Getenv("P05_O09_RETRY_WITHDRAWAL") == "third-party" {
			s.proveWithdrawalAlreadyPaid(t, chain, mine, poll, projection, withdrawalPath, delay.CloseHash, withdrawalKey, ready.ReviewRef, &original)
			return
		}
		s.sql(t, "UPDATE ledger.rental_withdrawal_candidate SET attempt_until=clock_timestamp()-interval '1 second'")
		s.sql(t, "UPDATE ledger.rental_authority_audience SET valid_until=valid_until+interval '1 minute' WHERE branch_id=$1", s.input.expected.Branch)
		s.call(t, s.operator, http.MethodPost, withdrawalPath, "application/json", wire(withdrawalKey, ready.ReviewRef), http.StatusAccepted)
		if s.withdrawalFault.sends.Load() != 1 {
			t.Fatal("changed live operator scope allowed replay of an unbroadcast withdrawal")
		}
		fresh := get().Withdrawal
		if fresh.State != "withdrawal-request-recorded" || fresh.RecoveryReviewRef == "" || fresh.ReviewRef != ready.ReviewRef {
			t.Fatalf("changed authority lacks same-withdrawal review: %+v", fresh)
		}
		s.call(t, s.operator, http.MethodPost, withdrawalPath, "application/json", wire(withdrawalKey, ready.ReviewRef, strings.Repeat("a", 64)), http.StatusConflict)
		first = decode(s.call(t, s.operator, http.MethodPost, withdrawalPath, "application/json",
			wire(withdrawalKey, ready.ReviewRef, fresh.RecoveryReviewRef), http.StatusAccepted))
		if first.Withdrawal.WithdrawalHash != original.Hash().Hex() || s.withdrawalFault.sends.Load() != 2 {
			t.Fatalf("recovery did not publish the identical sealed original")
		}
	}
	originalRequest := first.Withdrawal
	if originalRequest.State != "withdrawal-request-recorded" || originalRequest.CloseHash != delay.CloseHash || originalRequest.WithdrawalHash == "" {
		t.Fatalf("unexpected durable withdrawal response: %+v", originalRequest)
	}
	s.restart(t)
	repeat := decode(s.call(t, s.operator, http.MethodPost, withdrawalPath, "application/json", wire(withdrawalKey, ready.ReviewRef), http.StatusAccepted)).Withdrawal
	if originalRequest != repeat {
		t.Fatal("same withdrawal request did not recover the original identity")
	}
	s.call(t, s.operator, http.MethodPost, withdrawalPath, "application/json", wire(closeKey, ready.ReviewRef), http.StatusConflict)
	mine()
	poll()
	if accept != nil {
		held := get().Withdrawal
		if held.State != "withdrawal-held" || held.PayoutHash != originalRequest.WithdrawalHash || held.Amount != "25" {
			t.Fatalf("positive payout bypassed token acceptance: %+v", held)
		}
		s.chromeWithdrawal(t, "held", held.CloseHash)
		s.call(t, s.operator, http.MethodPost, cleanupPath, "application/json", cleanupWire(cleanupKey), http.StatusConflict)
		accept()
		mine()
		poll()
	}
	if os.Getenv("P05_O09_WITHDRAWAL_RECONCILIATION") == "1" {
		t.Run("ReconcileWithoutOperatorRequest", func(t *testing.T) {
			// Let the worker wake on its own schedule, without advancing the
			// database clock or issuing another operator request after payout.
			for deadline := time.Now().Add(55 * time.Second); ; {
				var state string
				var encrypted bool
				err := s.db.QueryRow(s.ctx, `SELECT coalesce(r.status,''),w.sealed_transaction IS NOT NULL
					FROM ledger.rental_withdrawal_candidate w LEFT JOIN ledger.rental_withdrawal_resolution r USING(branch_id,device_ref,generation)`).Scan(&state, &encrypted)
				if err != nil {
					t.Fatal(err)
				}
				if state == "executed" && !encrypted {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("scoped non-GET worker did not clear canonical executed material")
				}
				time.Sleep(100 * time.Millisecond)
			}
		})
	}
	payout := get().Withdrawal
	if accept != nil && (payout.AcceptanceHash == "" || payout.AcceptanceHash == payout.PayoutHash) {
		t.Fatal("current result lacks independent token acceptance receipt")
	}
	if payout.State != "withdrawal-received" || payout.PayoutHash != originalRequest.WithdrawalHash || payout.CloseHash != originalRequest.CloseHash ||
		payout.Amount != delay.Amount || payout.ReviewRef != "" {
		t.Fatalf("protected payout not bound to canonical receipt: %+v", payout)
	}
	if os.Getenv("P05_O09_WITHDRAWAL_RECONCILIATION") == "1" {
		raw := s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK)
		var observed struct {
			Withdrawal struct {
				Attempt struct{ State, OriginalHash, EvidenceHash string }
			}
		}
		if err := json.Unmarshal(raw, &observed); err != nil || observed.Withdrawal.Attempt.State != "executed" ||
			observed.Withdrawal.Attempt.OriginalHash != originalRequest.WithdrawalHash || observed.Withdrawal.Attempt.EvidenceHash != payout.PayoutHash {
			t.Fatal("authenticated GET did not read the protected terminal record", err)
		}
	}
	if os.Getenv("P05_O09_CHROME_DIST") != "" {
		s.chromeWithdrawal(t, "revoked", payout.CloseHash)
		s.setIdentity("8744d55c-c471-48d4-bbbc-6e65872c8346", s.actor)
		s.restart(t)
	}
	binding, _, err := chainview.ParseGenerationProjection(projection)
	if err != nil {
		t.Fatal(err)
	}
	ext, err := settlementext.NewSettlementext(binding.Settlement, chain)
	if err != nil {
		t.Fatal(err)
	}
	logs, err := ext.FilterWithdrawalExecuted(&bind.FilterOpts{Start: uint64(s.input.profile.StartHeight), Context: s.ctx}, []common.Address{binding.Account})
	if err != nil {
		t.Fatal(err)
	}
	defer logs.Close()
	if !logs.Next() || logs.Event.Raw.TxHash.Hex() != payout.PayoutHash || logs.Next() {
		t.Fatal("expected exactly one canonical withdrawal")
	}
	original := verifyPendingCleanup("cleanup-received", payout.CloseHash)
	if os.Getenv("P05_O09_PROTECTED_WITHDRAWAL") == "positive" {
		t.Run("QualifiedStationIssuesPaidOriginal", func(t *testing.T) {
			amount, ok := new(big.Int).SetString(payout.Amount, 10)
			if !ok || amount.Sign() <= 0 || common.HexToHash(payout.PayoutHash) == (common.Hash{}) {
				t.Fatal("terminal payout amount or hash missing")
			}
			s.verifyQualifiedCleanupIssuer(t, chain, original, payout.CloseHash, common.HexToHash(payout.PayoutHash),
				common.HexToHash(payout.AcceptanceHash), amount, mine, poll)
		})
	}
	if s.withdrawalFault != nil && s.withdrawalFault.sends.Load() != 2 {
		t.Fatal("observed transaction or successful payout caused another send")
	}
	s.revoke(t)
	if after := get().Withdrawal; after.State != "withdrawal-request-recorded" || after.Amount != "" || after.PayoutHash != "" {
		t.Fatalf("revoked authority released new evidence: %+v", after)
	}
	s.call(t, s.operator, http.MethodPost, withdrawalPath, "application/json", wire(withdrawalKey, ready.ReviewRef), http.StatusAccepted)
	if os.Getenv("P05_O09_CHROME_DIST") != "" {
		s.setIdentity("abcdef12-0000-4000-8000-000000000091", s.actor)
		s.restart(t)
		s.chromeWithdrawal(t, "denied", payout.CloseHash)
	}
}

func (s *combinedGenerationOpsd) verifyQualifiedCleanupIssuer(t *testing.T, chain *ethclient.Client, requestKey, closeHash string, payout, acceptance common.Hash, principal *big.Int, mine, poll func()) {
	t.Helper()
	ref := s.input.expected
	route := "/v2/private/rental-cleanup/" + ref.Branch + "/" + ref.Board + "/" + ref.Generation + "/" + requestKey + "/issue"
	s.call(t, s.collector, http.MethodPost, route, "", nil, http.StatusNotFound)
	fingerprint, err := hex.DecodeString(s.input.collectorFP)
	if err != nil {
		t.Fatal(err)
	}
	s.sql(t, `INSERT INTO ledger.rental_cleanup_station VALUES($1,$2,$3,$4,clock_timestamp()+interval '1 minute')`,
		ref.Branch, ref.Board, ref.Generation, fingerprint)
	s.call(t, s.operator, http.MethodPost, route, "", nil, http.StatusNotFound)
	var originalHeight int64
	var anchor, disposition, freeze []byte
	if err := s.db.QueryRow(s.ctx, `SELECT i.anchor_height,i.anchor_hash,i.disposition,p.freeze_digest
		FROM ledger.rental_cleanup_intent i
		JOIN ledger.rental_close_candidate c USING(branch_id,device_ref,generation)
		JOIN ledger.rental_generation_authorized_proof p ON (p.branch_id,p.device_ref,p.generation,p.request_key)=(c.branch_id,c.device_ref,c.generation,c.proof_key)
		WHERE i.request_key=$1`, requestKey).Scan(&originalHeight, &anchor, &disposition, &freeze); err != nil {
		t.Fatal(err)
	}
	mine()
	poll()
	block, err := chain.HeaderByNumber(s.ctx, big.NewInt(originalHeight))
	if err != nil || block.Hash() != common.Hash(anchor) {
		t.Fatal("original financial anchor was not canonical after advancing finalized head", err)
	}
	first := s.call(t, s.collector, http.MethodPost, route, "", nil, http.StatusOK)
	retry := s.call(t, s.collector, http.MethodPost, route, "", nil, http.StatusOK)
	if !bytes.Equal(first, retry) {
		t.Fatal("station retry changed immutable RCI1 response")
	}
	var wire struct{ Intent string }
	if json.Unmarshal(first, &wire) != nil {
		t.Fatal("invalid station response")
	}
	raw, err := hex.DecodeString(wire.Intent)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := rentalv1.ParseCleanupIntent(raw)
	if err != nil {
		t.Fatal(err)
	}
	binding, _, err := chainview.ParseGenerationProjection(s.input.projection)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := chainview.GenerationBindingDigest(binding)
	if err != nil {
		t.Fatal(err)
	}
	var height [8]byte
	binary.BigEndian.PutUint64(height[:], uint64(originalHeight))
	var amount [32]byte
	principal.FillBytes(amount[:])
	kind := byte(2)
	if principal.Sign() > 0 {
		kind = 1
		payoutReceipt, err := chain.TransactionReceipt(s.ctx, payout)
		if err != nil || payoutReceipt.Status != types.ReceiptStatusSuccessful || payoutReceipt.BlockNumber.Int64() > originalHeight {
			t.Fatal("payout omitted from original finalized evidence", err)
		}
	}
	withdrawal := crypto.Keccak256Hash([]byte("NU54-rental-withdrawal-evidence-v1\x00"), payout[:], acceptance[:], amount[:], height[:])
	expectedDisposition, err := rentalv1.CleanupDispositionDigest(bound, [32]byte(freeze), common.HexToHash(closeHash), common.Hash(anchor), kind, withdrawal)
	if err != nil || expectedDisposition != [32]byte(disposition) {
		t.Fatal("station lost the original finalized disposition commitment")
	}
	key, err := hex.DecodeString(strings.ReplaceAll(requestKey, "-", ""))
	if err != nil {
		t.Fatal(err)
	}
	scope := rentalv1.CleanupScope{ChainID: 8283, Settlement: binding.Settlement, Operator: crypto.PubkeyToAddress(s.input.operator.PublicKey),
		Board: s.input.boardSigner, Binding: bound, Disposition: expectedDisposition,
		Request: crypto.Keccak256Hash([]byte("NU54-rental-cleanup-request-v1\x00"), bound[:], key)}
	if err := scope.VerifyIntent(intent, uint64(time.Now().Unix())); err != nil || intent.ExpiresAt-intent.IssuedAt > 5 {
		t.Fatal("station did not return the independently scoped short-lived issuer signature", err)
	}
	var captured []byte
	if os.Getenv("P05_O09_RCR1_TEST") == "1" {
		now := uint64(time.Now().Unix())
		if now < intent.IssuedAt || now >= intent.ExpiresAt {
			t.Fatal("fixture could not capture a signed receipt during the actual intent lifetime")
		}
		captured = signCleanupReceiptFixture(t, scope, intent.Digest(scope.ChainID, scope.Settlement), now, s.input.cleanupBoardKey)
	}
	if wait := time.Until(time.Unix(int64(intent.ExpiresAt), 0)); wait > 0 {
		select {
		case <-time.After(wait):
		case <-s.ctx.Done():
			t.Fatal(s.ctx.Err())
		}
	}
	s.call(t, s.collector, http.MethodPost, route, "", nil, http.StatusConflict)
	hold := s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK)
	if !bytes.Contains(hold, []byte("board-receipt-required")) {
		t.Fatal("issuance changed the original operation hold")
	}
	var persisted []byte
	var retired, successors int
	var blocked bool
	if err := s.db.QueryRow(s.ctx, `SELECT signed_intent,
		(SELECT count(*) FROM ledger.rental_operation WHERE action='cleanup'),
		(SELECT count(*) FROM ledger.rental_enrollment e WHERE (e.branch_id,e.device_ref)=(i.branch_id,i.device_ref) AND e.generation>i.generation),
		ledger.rental_predecessor_has_unverified_cleanup(i.branch_id,i.device_ref,i.generation+1)
		FROM ledger.rental_cleanup_intent i WHERE i.request_key=$1`, requestKey).Scan(&persisted, &retired, &successors, &blocked); err != nil || !bytes.Equal(persisted, raw) || retired != 0 || successors != 0 || !blocked {
		t.Fatal("station changed the immutable envelope, retired a key, or released successor admission", err)
	}
	if os.Getenv("P05_O09_RCR1_TEST") == "1" {
		t.Run("AuthenticatedExpiredOriginalReceipt", func(t *testing.T) {
			s.verifyHistoricalCleanupReceipt(t, route, requestKey, closeHash, intent, scope, captured, mine, poll, principal.Sign() == 0)
		})
	}

	t.Log("one exact RCI1 after canonical disposition, authorized retry unchanged and expired replay held; no reset or successor")
}

func (s *combinedGenerationOpsd) verifyHistoricalCleanupReceipt(t *testing.T, issueRoute, requestKey, closeHash string,
	intent rentalv1.CleanupIntent, scope rentalv1.CleanupScope, good []byte, mine, poll func(), browser bool) {
	t.Helper()
	route := strings.TrimSuffix(issueRoute, "/issue") + "/receipt"
	sign := func(at uint64, digest [32]byte, signer *ecdsa.PrivateKey) []byte {
		return signCleanupReceiptFixture(t, scope, digest, at, signer)
	}
	digest := intent.Digest(scope.ChainID, scope.Settlement)
	s.call(t, s.operator, http.MethodPost, route, "application/octet-stream", good, http.StatusNotFound)
	s.call(t, s.collector, http.MethodGet, route, "", nil, http.StatusNotFound)
	otherRequest := strings.Replace(route, requestKey, "6cc1bb74-1cb5-4e70-8f4b-31d0837c568f", 1)
	s.call(t, s.collector, http.MethodPost, otherRequest, "application/octet-stream", good, http.StatusNotFound)
	wrongGeneration := strings.Replace(route, "/"+s.input.expected.Generation+"/"+requestKey, "/99/"+requestKey, 1)
	s.call(t, s.collector, http.MethodPost, wrongGeneration, "application/octet-stream", good, http.StatusNotFound)
	s.call(t, s.collector, http.MethodPost, route, "application/octet-stream", intent.Bytes(), http.StatusBadRequest)
	for _, denied := range [][]byte{
		sign(intent.IssuedAt, digest, s.input.operator), sign(intent.IssuedAt-1, digest, s.input.cleanupBoardKey),
		sign(intent.ExpiresAt, digest, s.input.cleanupBoardKey), sign(intent.IssuedAt, [32]byte{1}, s.input.cleanupBoardKey),
	} {
		s.call(t, s.collector, http.MethodPost, route, "application/octet-stream", denied, http.StatusConflict)
	}
	// Model the end of live board collection and a later enrollment source
	// refresh. Only the independently pinned issuer-time evidence key qualifies.
	var sourceChallenge []byte
	var challengeUntil time.Time
	var sourceSigner string
	if err := s.db.QueryRow(s.ctx, "SELECT challenge,challenge_until,board_signer_ref FROM ledger.rental_generation_source WHERE branch_id=$1",
		s.input.expected.Branch).Scan(&sourceChallenge, &challengeUntil, &sourceSigner); err != nil {
		t.Fatal(err)
	}
	replacement, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s.sql(t, `UPDATE ledger.rental_generation_source SET challenge=sha256('later challenge'),challenge_until=clock_timestamp()-interval '1 second',
		board_signer_ref=$2 WHERE branch_id=$1`, s.input.expected.Branch, strings.ToLower(crypto.PubkeyToAddress(replacement.PublicKey).Hex()))
	s.call(t, s.collector, http.MethodPost, route, "application/octet-stream", sign(intent.IssuedAt, digest, replacement), http.StatusConflict)
	var archivedSigner, retained []byte
	if err := s.db.QueryRow(s.ctx, "SELECT intent_board_signer,signed_intent FROM ledger.rental_cleanup_intent WHERE request_key=$1", requestKey).Scan(&archivedSigner, &retained); err != nil || !bytes.Equal(archivedSigner, scope.Board[:]) || !bytes.Equal(retained, intent.Bytes()) {
		t.Fatal("issuance did not atomically archive its original evidence signer and signed envelope", err)
	}
	s.sql(t, "UPDATE ledger.rental_cleanup_intent SET intent_board_signer=NULL WHERE request_key=$1", requestKey)
	s.call(t, s.collector, http.MethodPost, route, "application/octet-stream", good, http.StatusConflict)
	s.sql(t, "UPDATE ledger.rental_cleanup_intent SET intent_board_signer=$2 WHERE request_key=$1", requestKey, archivedSigner)
	mine()
	poll()
	pins := make(chan []byte, 2)
	for range 2 {
		go func() {
			var result []byte
			defer func() { pins <- result }()
			result = s.call(t, s.collector, http.MethodPost, route, "application/octet-stream", good, http.StatusOK)
		}()
	}
	first := <-pins
	for _, replay := range [][]byte{<-pins, s.call(t, s.collector, http.MethodGet, route, "", nil, http.StatusOK)} {
		if !bytes.Equal(first, replay) {
			t.Fatal("historical receipt retry changed signed bytes")
		}
	}
	var response struct{ Receipt string }
	if json.Unmarshal(first, &response) != nil || response.Receipt != hex.EncodeToString(good) {
		t.Fatal("station did not obtain the original authenticated RCR1")
	}
	if intent.ExpiresAt-intent.IssuedAt > 1 {
		primary, err := rentalv1.ParseCleanupReceipt(good)
		if err != nil {
			t.Fatal(err)
		}
		different := intent.IssuedAt
		if primary.RetiredAt == different {
			different++
		}
		s.call(t, s.collector, http.MethodPost, route, "application/octet-stream", sign(different, digest, s.input.cleanupBoardKey), http.StatusConflict)
	}
	hold := s.call(t, s.operator, http.MethodGet, s.reviewPath, "", nil, http.StatusOK)
	if !bytes.Contains(hold, []byte("receipt-authenticated-handoff-required")) ||
		bytes.Contains(hold, []byte(response.Receipt)) || bytes.Contains(hold, []byte(hex.EncodeToString(intent.Bytes()))) {
		t.Fatal("operator view omitted the hold or disclosed station evidence")
	}
	base, query, _ := strings.Cut(s.reviewPath, "?")
	cleanupPath := base + "/cleanup?" + query
	command, _ := json.Marshal(map[string]string{"requestKey": requestKey})
	recovered := s.call(t, s.operator, http.MethodPost, cleanupPath, "application/json", command, http.StatusAccepted)
	if !bytes.Contains(recovered, []byte("receipt-authenticated-handoff-required")) || !bytes.Contains(recovered, []byte(requestKey)) {
		t.Fatal("original operator could not replay the authenticated request state")
	}
	s.call(t, s.operator, http.MethodPost, cleanupPath, "application/json",
		[]byte(`{"requestKey":"6cc1bb74-1cb5-4e70-8f4b-31d0837c568f"}`), http.StatusConflict)
	var blocked bool
	var stored []byte
	if err := s.db.QueryRow(s.ctx, `SELECT signed_receipt,ledger.rental_predecessor_has_unverified_cleanup(i.branch_id,i.device_ref,i.generation+1)
		FROM ledger.rental_cleanup_intent i WHERE i.request_key=$1`, requestKey).Scan(&stored, &blocked); err != nil || !blocked || !bytes.Equal(stored, good) {
		t.Fatal("authenticated receipt lost or unlocked successor", err)
	}
	s.sql(t, "UPDATE ledger.rental_generation_source SET challenge=$2,challenge_until=$3,board_signer_ref=$4 WHERE branch_id=$1",
		s.input.expected.Branch, sourceChallenge, challengeUntil, sourceSigner)
	if browser {
		s.chromeWithdrawal(t, "cleanup-authenticated", closeHash)
	}
	// Delay the disposable SQL response after reading the current scope and
	// committed bytes. A still-live TLS cert must not outlive the station grant.
	signature := "ledger.record_rental_cleanup_receipt(uuid,uuid,uuid,bigint,uuid,bytea,bytea,bytea,bytea,bytea,bigint)"
	s.sql(t, "ALTER FUNCTION "+signature+" RENAME TO quick_fixture_cleanup_receipt")
	s.sql(t, `CREATE FUNCTION `+signature+` RETURNS TABLE(recorded bytea,current_until timestamptz)
	    LANGUAGE plpgsql SECURITY INVOKER SET search_path=pg_catalog,pg_temp AS $body$
	    BEGIN
	      RETURN QUERY SELECT * FROM ledger.quick_fixture_cleanup_receipt($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);
	      PERFORM pg_sleep(1.8);
	    END $body$`)
	s.sql(t, "REVOKE ALL ON FUNCTION "+signature+" FROM PUBLIC")
	s.sql(t, "GRANT EXECUTE ON FUNCTION "+signature+" TO nu54_p05_rental_close")
	s.sql(t, "UPDATE ledger.rental_cleanup_station SET valid_until=clock_timestamp()+interval '1.3 seconds' WHERE branch_id=$1", s.input.expected.Branch)
	began := time.Now()
	s.call(t, s.collector, http.MethodGet, route, "", nil, http.StatusNotFound)
	if time.Since(began) < 1700*time.Millisecond {
		t.Fatal("test did not reach station expiry after loading the receipt")
	}
	s.sql(t, "DROP FUNCTION "+signature)
	s.sql(t, "ALTER FUNCTION "+strings.Replace(signature, "record_rental_cleanup_receipt", "quick_fixture_cleanup_receipt", 1)+" RENAME TO record_rental_cleanup_receipt")
	s.sql(t, "DELETE FROM ledger.rental_cleanup_station WHERE branch_id=$1", s.input.expected.Branch)
	s.call(t, s.collector, http.MethodGet, route, "", nil, http.StatusNotFound)
	s.call(t, s.collector, http.MethodPost, route, "application/octet-stream", good, http.StatusNotFound)
	t.Log("historical RCR1 accepted with expired live challenge and original signed evidence; replay immutable, release and unauthorized disclosure held")
}

func signCleanupReceiptFixture(t *testing.T, scope rentalv1.CleanupScope, digest [32]byte, at uint64, signer *ecdsa.PrivateKey) []byte {
	t.Helper()
	receipt := rentalv1.CleanupReceipt{Intent: digest, RetiredAt: at}
	receiptHash := receipt.Digest(scope.ChainID, scope.Settlement)
	sig, err := crypto.Sign(receiptHash[:], signer)
	if err != nil {
		t.Fatal(err)
	}
	sig[64] += 27
	copy(receipt.BoardSignature[:], sig)
	return receipt.Bytes()
}
