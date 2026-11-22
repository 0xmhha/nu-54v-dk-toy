package wallet

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/marketlab"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
)

func testMarketExpiry(t *testing.T, store Store, owner Claims) {
	t.Helper()
	ctx := context.Background()
	signer, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	policy := marketlab.Policy{Venue: common.Address{1}, Base: common.Address{2}, Quote: common.Address{3}, Signer: crypto.PubkeyToAddress(signer.PublicKey),
		CodeHash: common.Hash{4}, Deployment: common.Hash{5}, Genesis: common.Hash{6}}
	store.MarketPolicy = &policy
	initialID := uuid.NewString()
	initial, err := store.Reserve(ctx, owner, initialID, ceremony.DKG, nil)
	if err != nil {
		t.Fatal(err)
	}
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	proof := ed25519.Sign(key, ceremony.ApprovalMessage(initial.Session.Kind, initial.Fingerprint))
	if _, err := store.Approve(ctx, owner, initialID, proof, public, nil); err != nil {
		t.Fatal(err)
	}
	walletKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	reports := observation{results: make(map[string]ceremony.Result)}
	for _, peer := range initial.Session.Peers {
		reports.results[peer.ID] = ceremony.Result{Session: initial.Fingerprint, Peer: peer.ID, State: "ready", PublicKey: walletKey.PubKey().SerializeUncompressed()}
	}
	if err := store.apply(ctx, initial, reports); err != nil {
		t.Fatal(err)
	}
	current, err := store.Wallet(ctx, owner)
	if err != nil || current.State != "active" {
		t.Fatalf("isolated test wallet unavailable: %v", err)
	}
	user := common.HexToAddress(current.Address)
	deadline := time.Now().Add(3 * time.Second).Unix()
	offer := make([]byte, marketlab.QuoteLength)
	copy(offer, crypto.Keccak256([]byte("execute((address,uint8,uint256,uint256,uint256,uint256,uint256,uint256,uint256),bytes)"))[:4])
	copy(offer[16:36], user[:])
	binary.BigEndian.PutUint64(offer[92:100], 100_000_000)
	binary.BigEndian.PutUint64(offer[156:164], 1_000_000)
	offer[227], offer[259], offer[322], offer[323], offer[355] = 1, 9, 1, 64, 65
	binary.BigEndian.PutUint64(offer[284:292], uint64(deadline))
	// Independent EIP-712 fixture for the isolated chain and venue.
	hash := func(value string) []byte { return crypto.Keccak256([]byte(value)) }
	domain := crypto.Keccak256(hash("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)"),
		hash("NU54 Isolated Market"), hash("1"), common.LeftPadBytes(big.NewInt(marketlab.ChainID).Bytes(), 32),
		common.LeftPadBytes(policy.Venue[:], 32), policy.CodeHash[:])
	body := crypto.Keccak256(hash("Quote(address user,uint8 action,uint256 a,uint256 b,uint256 minA,uint256 minB,uint256 priceVersion,uint256 nonce,uint256 deadline)"), offer[4:292])
	signature, err := crypto.Sign(crypto.Keccak256([]byte{25, 1}, domain, body), signer)
	if err != nil {
		t.Fatal(err)
	}
	signature[64] += 27
	copy(offer[356:421], signature)
	transfer := &Transfer{Profile: MarketTestProfile, Chain: "8283", To: strings.ToLower(policy.Venue.Hex()), ValueWei: "0",
		Nonce: "0", MaxFeeWei: "1000000000", PriorityFeeWei: "0",
		Market: &MarketTransfer{Deployment: policy.Deployment, User: user, Calldata: offer, Offer: offer}}
	retryID := uuid.NewString()
	first, err := store.Reserve(ctx, owner, retryID, ceremony.Sign, transfer)
	if err != nil || first.Session.Expires != deadline {
		t.Fatalf("valid market quote could not reserve its first request: %v", err)
	}
	blocker := holdWalletRow(t, store.Pool, current.ID)
	freshID := uuid.NewString()
	result := make(chan error, 1)
	go func() {
		_, err := store.Reserve(ctx, owner, freshID, ceremony.Sign, transfer)
		result <- err
	}()
	blocker.wait(t)
	waitExpiry(deadline)
	blocker.release(t)
	if err := <-result; !errors.Is(err, ErrConflict) {
		t.Fatalf("late reservation with expired market quote: %v", err)
	}
	if _, err := store.Operation(ctx, owner, freshID); !errors.Is(err, ErrMissing) {
		t.Fatalf("expired market reservation left a row: %v", err)
	}
	same, err := store.Reserve(ctx, owner, retryID, ceremony.Sign, transfer)
	if err != nil || same.Fingerprint != first.Fingerprint || same.Session.Expires != deadline {
		t.Fatalf("correlated retry did not preserve existing request: %v", err)
	}
	modified := *transfer
	modified.Nonce = "1"
	if _, err := store.Reserve(ctx, owner, retryID, ceremony.Sign, &modified); !errors.Is(err, ErrConflict) {
		t.Fatalf("market retry changed the original order: %v", err)
	}
	proof = ed25519.Sign(key, ceremony.ApprovalMessage(first.Session.Kind, first.Fingerprint))
	if _, err := store.Approve(ctx, owner, retryID, proof, public, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("market retry reopened expired approval: %v", err)
	}
}
