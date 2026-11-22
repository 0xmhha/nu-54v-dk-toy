package chainview_test

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	wire "github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/protocol"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

func (s *combinedGenerationOpsd) v1FixtureOrder(t *testing.T) map[string]string {
	t.Helper()
	binding, _, err := chainview.ParseGenerationProjection(s.input.projection)
	if err != nil {
		t.Fatal(err)
	}
	account := strings.ToLower(binding.Account.Hex())
	merchant := strings.ToLower(crypto.PubkeyToAddress(s.input.operator.PublicKey).Hex())
	orderKey := common.Hash{31: 13}
	typed := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain":  {{Name: "name", Type: "string"}, {Name: "version", Type: "string"}, {Name: "chainId", Type: "uint256"}, {Name: "verifyingContract", Type: "address"}},
			"MerchantOrder": {},
		},
		Domain:      apitypes.TypedDataDomain{Name: "NU54 Payment Settlement", Version: "1", ChainId: math.NewHexOrDecimal256(8283), VerifyingContract: binding.Settlement.Hex()},
		PrimaryType: "MerchantOrder",
		Message:     map[string]any{"orderId": orderKey.Hex(), "token": s.input.profile.Asset.Hex(), "amount": "20", "payout": merchant, "expiry": strconv.FormatUint(w12OrderExpiry(s.input.t0), 10)},
	}
	rawType := strings.TrimSuffix(strings.TrimPrefix(wire.EncodeType["MerchantOrder"], "MerchantOrder("), ")")
	for _, field := range strings.Split(rawType, ",") {
		kind, name, _ := strings.Cut(field, " ")
		typed.Types["MerchantOrder"] = append(typed.Types["MerchantOrder"], apitypes.Type{Name: name, Type: kind})
	}
	digest, _, err := apitypes.TypedDataAndHash(typed)
	if err != nil {
		t.Fatal(err)
	}
	merchantSig, err := crypto.Sign(digest, s.input.operator)
	if err != nil {
		t.Fatal(err)
	}
	merchantSig[64] += 27
	return map[string]string{"account": account, "orderKey": orderKey.Hex(), "token": strings.ToLower(s.input.profile.Asset.Hex()),
		"amount": "20", "expiry": strconv.FormatUint(w12OrderExpiry(s.input.t0), 10), "merchantSignature": "0x" + hex.EncodeToString(merchantSig)}

}

func (s *combinedGenerationOpsd) verifyRenterV1Origin(t *testing.T, boardApproval, freezeReceipt []byte) {
	t.Helper()
	binding, _, err := chainview.ParseGenerationProjection(s.input.projection)
	if err != nil {
		t.Fatal(err)
	}
	payload := s.v1FixtureOrder(t)
	account := payload["account"]
	merchant := strings.ToLower(crypto.PubkeyToAddress(s.input.operator.PublicKey).Hex())
	add := "/v2/private/renter-v1-orders/reserve"
	confirm := "/v2/private/renter-v1-orders/confirm"
	jsonBody := func(value any) []byte { b, _ := json.Marshal(value); return b }
	call := func(client *http.Client, path string, code int) []byte {
		return s.call(t, client, http.MethodPost, path, "application/json", jsonBody(payload), code)
	}
	chain, err := ethclient.DialContext(s.ctx, s.input.rpc)
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Close()
	nonce, err := chain.PendingNonceAt(s.ctx, common.HexToAddress(merchant))
	if err != nil {
		t.Fatal(err)
	}
	call(s.v1Terminal, add, http.StatusNotFound)
	offer := jsonBody(map[string]any{"customer": combinedRenter, "generation": s.input.expected.Generation,
		"account": account, "assignmentUntil": time.Now().UTC().Add(35 * time.Second)})
	raw := s.call(t, s.operator, http.MethodPost, "/v2/operations/renter-handover/"+binding.Branch+"/"+binding.Board, "application/json", offer, http.StatusCreated)
	var handover struct{ Assignment string }
	if json.Unmarshal(raw, &handover) != nil || handover.Assignment == "" {
		t.Fatal("handover offer failed")
	}
	call(s.v1Terminal, add, http.StatusNotFound)
	s.call(t, s.renter, http.MethodPost, "/v2/renter/returns/"+handover.Assignment+"/accept", "application/json", jsonBody(map[string]string{"account": account}), http.StatusOK)
	call(s.renter, add, http.StatusNotFound)
	raw = call(s.v1Terminal, add, http.StatusOK)
	var origin struct{ Origin string }
	if json.Unmarshal(raw, &origin) != nil || origin.Origin == "" {
		t.Fatal("missing immutable origin")
	}
	if same := call(s.v1Terminal, add, http.StatusOK); string(same) != string(raw) {
		t.Fatal("reserve replay changed original")
	}
	mixed := os.Getenv("P05_O09_RENTER_ASSESSMENT") == "mixed"
	path := "/v2/renter/returns/" + handover.Assignment
	if mixed {
		s.recordWithExpectedPending(t, s.input.projection, freezeReceipt, 1)
		if got := call(s.v1Terminal, add, http.StatusOK); string(got) != string(raw) {
			t.Fatal("a pre-freeze reservation could not recover its original id")
		}
		var unresolved struct {
			PendingPayments int
			PendingOrders   []json.RawMessage
		}
		reply := s.call(t, s.renter, http.MethodGet, path, "", nil, http.StatusOK)
		if json.Unmarshal(reply, &unresolved) != nil || unresolved.PendingPayments != 1 || len(unresolved.PendingOrders) != 0 {
			t.Fatalf("an unsigned origin was exposed: %s", reply)
		}
	}
	payload["origin"], payload["nonce"], payload["approvalSignature"] = origin.Origin, "3", "0x"+hex.EncodeToString(boardApproval)
	payload["nonce"] = "4"
	call(s.v1Terminal, confirm, http.StatusBadRequest)
	payload["nonce"] = "3"
	call(s.v1Terminal, confirm, http.StatusOK)
	s.restart(t)
	call(s.v1Terminal, confirm, http.StatusOK)
	var confirmed int
	if err := s.db.QueryRow(s.ctx, `SELECT count(*) FROM ledger.renter_v1_order_origin WHERE assignment_id=$1 AND approval_digest IS NOT NULL`, handover.Assignment).Scan(&confirmed); err != nil || confirmed != 1 {
		t.Fatal("expected one matching committed witness", err)
	}
	if mixed {
		reply := s.call(t, s.renter, http.MethodGet, path, "", nil, http.StatusOK)
		var assessment struct {
			PendingPayments, PendingLimits int
			TokenDecimals                  int
			TokenSymbol                    string
			PendingOrders                  []struct{ Origin, Amount, Asset string }
		}
		if json.Unmarshal(reply, &assessment) != nil || assessment.PendingPayments != 1 || assessment.PendingLimits != 1 ||
			assessment.TokenDecimals != 6 || assessment.TokenSymbol != "tUSDC" || len(assessment.PendingOrders) != 1 || assessment.PendingOrders[0].Origin != origin.Origin ||
			assessment.PendingOrders[0].Asset != payload["token"] || assessment.PendingOrders[0].Amount != "20" {
			t.Fatalf("missing linked original payment: %s", reply)
		}
		s.call(t, s.otherRenter, http.MethodGet, path, "", nil, http.StatusNotFound)
		var validUntil time.Time
		if err := s.db.QueryRow(s.ctx, "SELECT valid_until FROM ledger.rental_authority_audience WHERE branch_id=$1", binding.Branch).Scan(&validUntil); err != nil {
			t.Fatal(err)
		}
		raw, status := s.readWhileTokenMetadataChanges(t, path, func() {
			s.sql(t, "UPDATE ledger.rental_authority_audience SET valid_until=clock_timestamp()-interval '1 second' WHERE branch_id=$1", binding.Branch)
		})
		var expired map[string]any
		if status != http.StatusOK || json.Unmarshal(raw, &expired) != nil || expired["settlement"] != "unknown" ||
			expired["pendingOrders"] != nil || expired["pendingPayments"] != nil || expired["tokenSymbol"] != nil || expired["basisAt"] != nil {
			t.Fatalf("authority lost during token lookup disclosed data: %d %s", status, raw)
		}
		s.sql(t, "UPDATE ledger.rental_authority_audience SET valid_until=$2 WHERE branch_id=$1", binding.Branch, validUntil)
		raw, status = s.readWhileTokenMetadataChanges(t, path, func() {
			s.call(t, s.operator, http.MethodPost, "/v2/operations/renter-handover/"+handover.Assignment+"/revoke", "", nil, http.StatusNoContent)
		})
		if status != http.StatusNotFound || strings.Contains(string(raw), origin.Origin) {
			t.Fatalf("handover revoked during token lookup: %d %s", status, raw)
		}
	} else {
		s.call(t, s.operator, http.MethodPost, "/v2/operations/renter-handover/"+handover.Assignment+"/revoke", "", nil, http.StatusNoContent)
	}

	call(s.v1Terminal, confirm, http.StatusNotFound)
	end, err := chain.PendingNonceAt(s.ctx, common.HexToAddress(merchant))
	if err != nil || end != nonce {
		t.Fatal("evidence route sent a transaction", err)
	}
}
