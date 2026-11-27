package ceremony_test

import (
	"bytes"
	"context"
	"encoding/asn1"
	"encoding/json"
	"math/big"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/marketlab"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func installedMarket(t *testing.T) *marketlab.Policy {
	t.Helper()
	path := os.Getenv("P03_TEST_MARKET_PROFILE")
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	type target struct {
		Address  common.Address
		CodeHash common.Hash
	}
	var config struct {
		Environment             string
		Chain                   uint64 `json:"chainId"`
		Genesis, DeploymentHash common.Hash
		Venue, Base, Quote      target
		QuoteSigner             common.Address
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	p := &marketlab.Policy{Genesis: config.Genesis, Deployment: config.DeploymentHash, CodeHash: config.Venue.CodeHash,
		Venue: config.Venue.Address, Base: config.Base.Address, Quote: config.Quote.Address, Signer: config.QuoteSigner}
	if config.Environment != "p08-isolated-8283-v1" || config.Chain != marketlab.ChainID || !p.Valid() {
		t.Fatal("invalid test deployment")
	}
	return p
}

type marketOffer struct {
	Digest common.Hash
	Data   hexutil.Bytes
}

func quoteMarket(t *testing.T, actor string, policy *marketlab.Policy) wallet.Transfer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	node, err := ethclient.DialContext(ctx, "http://127.0.0.1:8285")
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	head, err := node.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var accounts []common.Address
	if err := node.Client().CallContext(ctx, &accounts, "eth_accounts"); err != nil || len(accounts) < 5 {
		t.Fatalf("fixture publishers unavailable: %v", err)
	}
	calldata := append(common.FromHex("0x07640c1f"), common.LeftPadBytes(big.NewInt(1_200_000).Bytes(), 32)...)
	calldata = append(calldata, common.LeftPadBytes(new(big.Int).SetUint64(head.Time).Bytes(), 32)...)
	for _, publisher := range accounts[2:4] {
		if err := node.Client().CallContext(ctx, nil, "eth_sendTransaction", map[string]any{"from": publisher, "to": policy.Venue, "data": hexutil.Encode(calldata)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := node.Client().CallContext(ctx, nil, "anvil_mine", "0x04"); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	input, err := json.Marshal(map[string]string{"user": actor, "action": "fx-base", "a": "100000055", "b": "0", "minA": "1200000", "minB": "0"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Post("http://127.0.0.1:8284/v1/quotes", "application/json", bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var offer marketOffer
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&offer) != nil || offer.Digest == (common.Hash{}) {
		t.Fatalf("isolated market quote refused: HTTP %d", resp.StatusCode)
	}
	return wallet.Transfer{Profile: wallet.MarketTestProfile, Chain: "8283", To: strings.ToLower(policy.Venue.Hex()),
		ValueWei: "0", Nonce: "0", MaxFeeWei: "2800000000", PriorityFeeWei: "0",
		Market: &wallet.MarketTransfer{Deployment: policy.Deployment, User: common.HexToAddress(actor), Calldata: offer.Data, Offer: offer.Data}}
}

func sendMarket(t *testing.T, transfer wallet.Transfer, proof wallet.Operation, p *marketlab.Policy) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(ctx, "http://127.0.0.1:8285")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	user := transfer.Market.User
	if err := client.Client().CallContext(ctx, nil, "anvil_setBalance", user, "0x56bc75e2d63100000"); err != nil {
		t.Fatal(err)
	}
	order, _, err := wallet.ReviewMarketTransfer(p, transfer, time.Now())
	if err != nil || *proof.RecoveryID > 1 {
		t.Fatalf("invalid approved result: %v", err)
	}
	r, s, err := readThresholdDER(proof.Signature)
	if err != nil {
		t.Fatal(err)
	}
	sig := append(common.LeftPadBytes(r.Bytes(), 32), common.LeftPadBytes(s.Bytes(), 32)...)
	sig = append(sig, *proof.RecoveryID)
	to := p.Venue
	unsigned := types.NewTx(&types.LegacyTx{Nonce: 0, To: &to, Gas: order.Gas, GasPrice: big.NewInt(2_800_000_000), Data: transfer.Market.Calldata})
	signing := types.NewEIP155Signer(big.NewInt(marketlab.ChainID))
	if signing.Hash(unsigned) != order.Digest {
		t.Fatal("EIP-155 digest mismatch")
	}
	signed, err := unsigned.WithSignature(signing, sig)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := types.Sender(signing, signed)
	if err != nil || actual != user {
		t.Fatalf("participant signature did not recover the dedicated wallet: %s %v", actual, err)
	}
	wire, err := signed.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"raw": hexutil.Encode(wire)})
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post("http://127.0.0.1:8284/v1/transactions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 202 {
		t.Fatalf("isolated market rejected approved threshold transaction: %d", response.StatusCode)
	}
	if err := client.Client().CallContext(ctx, nil, "anvil_mine", "0x04"); err != nil {
		t.Fatal(err)
	}
	proofResponse, err := http.Get("http://127.0.0.1:8284/v1/executions/" + signed.Hash().Hex() + "/" + order.OfferDigest.Hex())
	if err != nil {
		t.Fatal(err)
	}
	defer proofResponse.Body.Close()
	var outcome struct{ State, First string }
	if proofResponse.StatusCode != 200 || json.NewDecoder(proofResponse.Body).Decode(&outcome) != nil ||
		outcome.State != "confirmed" || outcome.First != "1200000" {
		t.Fatalf("threshold FX was not finalized: status %d %+v", proofResponse.StatusCode, outcome)
	}
	t.Log("P03 independent approval and two real participants settled original market FX transaction via P07")
}

func readThresholdDER(data []byte) (*big.Int, *big.Int, error) {
	var values struct{ R, S *big.Int }
	_, err := asn1.Unmarshal(data, &values)
	return values.R, values.S, err
}
