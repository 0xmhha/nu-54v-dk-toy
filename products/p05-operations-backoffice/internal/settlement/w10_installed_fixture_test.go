package settlement

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

func TestLoopbackInstalledW10Fixture(t *testing.T) {
	runner := os.Getenv("NU54_K10_INSTALLED_RUNNER")
	mode, requested := os.LookupEnv("NU54_W10_FIXTURE_ONLY")
	if !requested && runner == "" {
		t.Skip("requires the dedicated W10 installed or fixture-only harness")
	}
	if requested && mode != "0" && mode != "1" {
		t.Fatal("NU54_W10_FIXTURE_ONLY must be 0 or 1")
	}
	if mode != "1" && !filepath.IsAbs(runner) {
		t.Fatal("installed W10 fixture requires the native test runner")
	}
	for _, prerequisite := range []string{"P05_SETTLEMENT_TEST_RPC", "P05_SETTLEMENT_TEST_INDEXER", "P05_SETTLEMENT_TEST_DATABASE_URL"} {
		if os.Getenv(prerequisite) == "" {
			t.Fatalf("requested W10 fixture requires %s", prerequisite)
		}
	}
	settlementEndpoint(t)
	endpoint := freshSettlementChain(t)
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	db, dsn := scratchDB(t)
	chain, err := ethclient.DialContext(ctx, endpoint)
	if err != nil {
		t.Fatal("isolated W10 chain unavailable")
	}
	defer chain.Close()
	relay, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	quote, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	payer, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	payerAddress := crypto.PubkeyToAddress(payer.PublicKey)
	relayAddress := crypto.PubkeyToAddress(relay.PublicKey)
	beneficiary := fixtureSigner(104).Address()
	target, deployed, token, contract, _ := deployFixtureWithOperator(t, ctx, chain, db, relay, false)
	operator, err := bind.NewKeyedTransactorWithChainID(relay, big.NewInt(8283))
	if err != nil {
		t.Fatal(err)
	}
	operator.Context = ctx
	mineSetup := func(tx *types.Transaction, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := bind.WaitMined(ctx, chain, tx)
		if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
			t.Fatal("isolated SW1 funding or registration failed")
		}
	}
	var merchant, commitment, quoteCipher, relayCipher [32]byte
	for _, value := range []*[32]byte{&merchant, &commitment, &quoteCipher, &relayCipher} {
		if _, err := rand.Read(value[:]); err != nil {
			t.Fatal(err)
		}
	}
	profile := commerce.Profile{Chain: 8283, Contract: target.Contract, Asset: target.Asset, Beneficiary: beneficiary,
		Signer: crypto.PubkeyToAddress(quote.PublicKey), MerchantID: merchant, CommitmentKey: commitment,
		KeyVersion: 1, BeneficiaryVersion: 1}
	mineSetup(contract.RegisterMerchant(operator, merchant, profile.Signer, beneficiary, target.RefundSigner))
	role := uint8(1)
	if os.Getenv("NU54_W10_RENTER_INSTALLED") == "1" {
		role = 2
	}
	mineSetup(contract.RegisterPayer(operator, payerAddress, 1, role, payerAddress))
	funding := big.NewInt(20_000_000)
	if role == 2 {
		// The renter run reviews two 15.00 sales: the QR sale is paid from escrow, and the W09 sale
		// is reviewed afterwards, so the escrow must still cover its full amount.
		funding = big.NewInt(30_000_000)
	}
	mineSetup(token.Mint(operator, operator.From, funding))
	mineSetup(token.Approve(operator, target.Contract, funding))
	mineSetup(contract.DepositEscrow(operator, payerAddress, 1, funding))
	if role == 2 {
		// Renter admission checks the finalized generation, including the deposit.
		mine(t, ctx, chain, 3)
	}
	approval := &fixtureDeploymentApproval{}
	stop, _, _, _ := startFixtureIndexer(t, ctx, db, dsn, endpoint, os.Getenv("P05_SETTLEMENT_TEST_INDEXER"), target, deployed, approval)
	defer stop()
	evidenceLogin, evidenceDSN := testLogin(t, db, dsn, "nu54_p07_reconcile")
	fixtureExec(t, db, "INSERT INTO chainview.receipt_scope VALUES($1,$2,$3,now()+interval '10 minutes')",
		evidenceLogin, target.Stream, "0x"+rawHex(merchant[:]))
	t.Cleanup(func() {
		fixtureExec(t, db, "DELETE FROM chainview.receipt_scope WHERE login_role=$1 AND stream_id=$2", evidenceLogin, target.Stream)
	})
	_, workerDSN := testLogin(t, db, dsn, "nu54_p05_settlement")
	_, forwardDSN := testLogin(t, db, dsn, "nu54_p05_commerce")
	_, financeDSN := testLogin(t, db, dsn, "nu54_p05_finance")
	now := time.Now()
	grant := GasSponsorGrant{Branch: fixtureUUID(t, db), Chain: 8283, Contract: target.Contract, Asset: target.Asset,
		Manifest: target.Manifest, Merchant: merchant, Relay: relayAddress, StartsAt: now.Add(-time.Minute).Unix(),
		ExpiresAt: now.Add(8 * time.Minute).Unix()}
	message, valid := GasSponsorGrantMessage(grant)
	if !valid {
		t.Fatal("invalid installed W10 sponsorship")
	}
	grant.Signature = rawHex(ed25519.Sign(approval.Signer, message))
	signatureFile, err := os.ReadFile(approval.Signatures)
	var signatures map[string]string
	if err != nil || json.Unmarshal(signatureFile, &signatures) != nil || signatures[approval.Profile.GenerationID] == "" {
		t.Fatal("signed local deployment was not published")
	}
	fixture := map[string]any{
		"RPC": endpoint, "WorkerDSN": workerDSN, "ForwardDSN": forwardDSN, "EvidenceDSN": evidenceDSN, "FinanceDSN": financeDSN,
		"QuoteKey": rawHex(crypto.FromECDSA(quote)), "PayerKey": rawHex(crypto.FromECDSA(payer)), "RelayKey": rawHex(crypto.FromECDSA(relay)),
		"QuoteCipher": rawHex(quoteCipher[:]), "RelayCipher": rawHex(relayCipher[:]),
		"Profile": profile, "Target": target, "Grant": grant, "Deployment": approval.Profile,
		"Authority": approval.Key, "Signature": signatures[approval.Profile.GenerationID],
	}
	if os.Getenv("NU54_W11_INSTALLED_UNPAID") == "1" {
		_, fixture["AbsenceDSN"] = testLogin(t, db, dsn, "nu54_p07_ingest")
		fixture["AbsenceProfiles"], fixture["AbsenceSignatures"] = approval.Profiles, approval.Signatures
	}
	if os.Getenv("NU54_W11_INSTALLED_PICKUP") == "1" {
		_, fixture["DispatchDSN"] = testLogin(t, db, dsn, "nu54_p05_dispatch")
		_, fixture["ReceiverDSN"] = testLogin(t, db, dsn, "nu54_p05_receiver")
	}
	payload, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal("could not encode the local integration fixture")
	}
	path := filepath.Join(t.TempDir(), "installed-w10-settlement.json")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal("could not write the owned fixture")
	}
	child := exec.CommandContext(ctx, "go", "test", "-count=1", "-v", "-run", "^TestInstalledW10SponsoredSettlement$", "../commerce")
	child.Env = append(os.Environ(), "P05_COMMERCE_TEST_DATABASE_URL="+dsn,
		"NU54_W10_SETTLEMENT_FIXTURE="+path, "NU54_K10_SIGNED_REVIEW=1", "NU54_W10_INSTALLED_BOARD=1")
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err := child.Run(); err != nil {
		t.Fatalf("installed W10 settlement child failed: %v", err)
	}
}
