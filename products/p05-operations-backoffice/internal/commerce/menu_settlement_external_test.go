package commerce_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/settlement"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/test/settlement/w10fixture"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/deployment"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v5"
)

type installedDigestKey struct{ *ecdsa.PrivateKey }

func (k installedDigestKey) SignDigest(_ context.Context, digest [32]byte) ([]byte, error) {
	signature, err := crypto.Sign(digest[:], k.PrivateKey)
	if err != nil {
		return nil, err
	}
	signature[64] += 27
	return signature, nil
}

// This optional test is supplied an already deployed, signed local profile
// and a running P07 reader by the settlement fixture.
func TestInstalledW10SponsoredSettlement(t *testing.T) {
	path := os.Getenv("NU54_W10_SETTLEMENT_FIXTURE")
	if path == "" {
		t.Skip("requires the installed SW1 settlement fixture")
	}
	if !filepath.IsAbs(path) {
		t.Fatal("use an absolute local settlement fixture path")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 32768 {
		t.Fatal("invalid private settlement fixture")
	}
	raw, err := os.ReadFile(path)
	var config struct {
		RPC, WorkerDSN, ForwardDSN, EvidenceDSN, FinanceDSN string
		AbsenceDSN, AbsenceProfiles, AbsenceSignatures      string
		DispatchDSN, ReceiverDSN                            string
		QuoteKey, PayerKey, RelayKey                        string
		QuoteCipher, RelayCipher                            string
		Profile                                             commerce.Profile
		Target                                              settlement.Target
		Grant                                               settlement.GasSponsorGrant
		Deployment                                          deployment.Profile
		Authority, Signature                                string
	}
	if err != nil || json.Unmarshal(raw, &config) != nil {
		t.Fatal("invalid private settlement fixture")
	}
	endpoint, err := url.Parse(config.RPC)
	dbConfig, dbErr := pgx.ParseConfig(os.Getenv("P05_COMMERCE_TEST_DATABASE_URL"))
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" ||
		endpoint.Port() == "" || endpoint.User != nil || dbErr != nil ||
		!(strings.HasPrefix(dbConfig.Database, "nu54_commerce_") || strings.HasPrefix(dbConfig.Database, "nu54_settlement_")) ||
		!(strings.HasPrefix(dbConfig.Host, "/private/tmp/") || strings.HasPrefix(dbConfig.Host, "/tmp/")) {
		t.Fatal("installed SW1 requires a local chain and private database")
	}
	roles := []string{config.WorkerDSN, config.ForwardDSN, config.EvidenceDSN, config.FinanceDSN}
	w11 := os.Getenv("NU54_W11_INSTALLED_PICKUP") == "1"
	if w11 {
		roles = append(roles, config.DispatchDSN, config.ReceiverDSN)
	}
	unpaid := os.Getenv("NU54_W11_INSTALLED_UNPAID") == "1"
	if unpaid {
		roles = append(roles, config.AbsenceDSN)
	}
	for _, dsn := range roles {
		scope, err := pgx.ParseConfig(dsn)
		if err != nil || scope.Host != dbConfig.Host || scope.Port != dbConfig.Port || scope.Database != dbConfig.Database {
			t.Fatal("installed SW1 must share the originating quote database")
		}
	}
	key := func(value string) *ecdsa.PrivateKey {
		t.Helper()
		secret, err := crypto.HexToECDSA(value)
		if err != nil {
			t.Fatal("invalid synthetic signing key")
		}
		return secret
	}
	decode := func(value string) []byte {
		t.Helper()
		raw, err := hex.DecodeString(value)
		if err != nil {
			t.Fatal("invalid fixture key encoding")
		}
		return raw
	}
	signer, payer, relay := key(config.QuoteKey), key(config.PayerKey), key(config.RelayKey)
	quoteCipher, err := commerce.NewCipher(decode(config.QuoteCipher))
	if err != nil {
		t.Fatal("invalid archive key")
	}
	relayCipher, err := commerce.NewCipher(decode(config.RelayCipher))
	if err != nil {
		t.Fatal("invalid relay custody key")
	}
	if config.Profile.Signer != crypto.PubkeyToAddress(signer.PublicKey) ||
		config.Grant.Relay != crypto.PubkeyToAddress(relay.PublicKey) {
		t.Fatal("fixture keys do not match the enrolled signer and sponsor")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	chain, err := ethclient.DialContext(ctx, config.RPC)
	if err != nil {
		t.Fatal("isolated chain unavailable")
	}
	defer chain.Close()
	workerStore, err := settlement.Open(ctx, config.WorkerDSN, "nu54_p05_settlement")
	if err != nil {
		t.Fatal(err)
	}
	defer workerStore.Close()
	forwardStore, err := settlement.Open(ctx, config.ForwardDSN, "nu54_p05_commerce")
	if err != nil {
		t.Fatal("installed SW1 commerce forwarding role unavailable")
	}
	defer forwardStore.Close()
	finance, err := settlement.Open(ctx, config.FinanceDSN, "nu54_p05_finance")
	if err != nil {
		t.Fatal("installed SW1 finance store unavailable")
	}
	defer finance.Close()
	evidence, err := settlement.OpenEvidence(ctx, config.EvidenceDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer evidence.Close()
	authority := decode(config.Authority)
	review := settlement.VerifyOperatorDeployment(ctx, chain, config.Target, config.Grant.Relay,
		[]deployment.Profile{config.Deployment}, authority,
		map[string]string{config.Deployment.GenerationID: config.Signature}, true)
	if review.Status != "verified" {
		t.Fatal("installed SW1 fixture deployment is not verified")
	}
	lookup := func(ctx context.Context, branch string, p commerce.Profile, payer common.Address, now time.Time) (commerce.GasSponsorship, bool) {
		if !settlement.ReviewedDeploymentCurrent(ctx, chain, config.Target, review) {
			return commerce.GasSponsorship{}, false
		}
		witness, valid := settlement.VerifyMerchantGasSponsor(config.Grant, branch, p, config.Target, review, payer, authority, now)
		witness.CurrentPublication = func(at time.Time) bool { return at.Unix() < witness.ExpiresAt }
		return witness, valid
	}
	if os.Getenv("NU54_W10_FIXTURE_ONLY") == "1" {
		payerAddr := crypto.PubkeyToAddress(payer.PublicKey)
		witness, verified := lookup(ctx, config.Grant.Branch, config.Profile, payerAddr, time.Now())
		balances, err := w10fixture.ReadBalances(ctx, chain, config.Profile, payerAddr, config.Grant.Relay)
		if !verified || witness.Account != config.Grant.Relay || err != nil || balances.Escrow.Cmp(big.NewInt(20_000_000)) != 0 {
			t.Fatal("installed W10 fixture authority or genuinely funded escrow is unavailable")
		}
		t.Log("installed W10 deployment, authority, funded payer, finance and evidence roles ready")
		return
	}
	targets := map[string]settlement.Target{config.Grant.Branch: config.Target}
	forwarder := settlement.Forwarder{Store: forwardStore, QuoteCipher: quoteCipher, RelayCipher: relayCipher, Targets: targets, GasSponsors: lookup}
	worker := settlement.Worker{Store: workerStore, Evidence: evidence, Chain: settlement.LiveChain{Client: chain},
		Targets: targets, QuoteCipher: quoteCipher, RelayCipher: relayCipher, GasAccount: config.Grant.Relay,
		GasSigner: installedDigestKey{relay}, MaxGasPrice: big.NewInt(100_000_000_000), Now: time.Now}
	admin, err := pgx.ConnectConfig(ctx, dbConfig)
	if err != nil {
		t.Fatal("originating fixture ledger unavailable")
	}
	defer admin.Close(context.Background())
	fixture := commerce.InstalledW10Settlement{
		Branch:  config.Grant.Branch,
		Profile: config.Profile, Signer: installedDigestKey{signer}, QuoteCipher: quoteCipher, Payer: payer,
		RelayAccount: config.Grant.Relay, LookupSponsor: lookup, Forwarder: forwarder,
		RegisterFinance: func(mux *http.ServeMux, credentials map[string]service.Principal, origin string) {
			settlement.FinanceHTTP{Store: finance, Credentials: credentials, OperatorOrigin: origin,
				Targets: targets, Evidence: evidence,
				Merchants: map[string]string{config.Grant.Branch: common.Hash(config.Profile.MerchantID).Hex()},
			}.Register(mux)
		},
		FinalizeOriginal: func(ctx context.Context, sale string) (string, error) {
			driver := w10fixture.Settlement{DB: admin, Chain: chain, Step: worker.Step, Profile: config.Profile,
				Payer: crypto.PubkeyToAddress(payer.PublicKey), Sponsor: config.Grant.Relay}
			return driver.Finalize(t, ctx, sale, big.NewInt(15_000_000))
		},
	}
	if os.Getenv("NU54_W10_RENTER_INSTALLED") == "1" {
		manifest := decode(config.Target.Manifest)
		escrow := settlement.RentalEscrow{Client: chain, Targets: targets,
			Reviews: map[string]settlement.DeploymentReview{config.Grant.Branch: review}}
		fixture.Rental = &commerce.InstalledW10Rental{Manifest: manifest, Escrow: escrow,
			Finalize: func(ctx context.Context, sale string, wait func(string) error) (string, error) {
				driver := w10fixture.Settlement{DB: admin, Chain: chain, Step: worker.Step, Profile: config.Profile,
					Payer: crypto.PubkeyToAddress(payer.PublicKey), Sponsor: config.Grant.Relay, BeforeMine: wait}
				return driver.Finalize(t, ctx, sale, big.NewInt(15_000_000))
			},
		}
		for index := range fixture.Rental.Kits {
			board, err := w10fixture.NewRenterBoard(t.TempDir(), os.Getenv("NU54_W10_RENTER_BOARD_BRIDGE"),
				os.Getenv("NU54_W10_RENTER_BOARD_BIN"), crypto.FromECDSA(payer))
			if err != nil {
				t.Fatal(err)
			}
			fixture.Rental.Kits[index] = commerce.InstalledW10RentalKit{Commissioning: board.Commissioning, Exchange: board.Exchange}
		}
		forwarder.RentalEscrow = escrow
		fixture.Forwarder = forwarder
	}
	if w11 {
		dispatch, err := settlement.Open(ctx, config.DispatchDSN, "nu54_p05_dispatch")
		if err != nil {
			t.Fatal("isolated pickup dispatch role unavailable")
		}
		defer dispatch.Close()
		receiver, err := settlement.Open(ctx, config.ReceiverDSN, "nu54_p05_receiver")
		if err != nil {
			t.Fatal("isolated pickup receiver role unavailable")
		}
		defer receiver.Close()
		fixture.RegisterPickupReceiver = func(mux *http.ServeMux, fingerprint string) {
			settlement.Receiver{Store: receiver, Evidence: evidence, Grants: map[string]settlement.ReceiverGrant{
				fingerprint: {Branches: []string{config.Grant.Branch}, ExpiresAt: time.Now().Add(8 * time.Minute).Unix()},
			}}.Register(mux)
		}
		fixture.DispatchOriginal = func(ctx context.Context, client *http.Client, origin, sale string) error {
			holds, err := admin.Begin(ctx)
			if err != nil {
				return err
			}
			defer holds.Rollback(context.Background())
			if _, err := holds.Exec(ctx, "SELECT event_id FROM ledger.fulfillment_outbox WHERE sale_id<>$1 AND state<>'delivered' FOR UPDATE", sale); err != nil {
				return err
			}
			driver := settlement.Dispatcher{Store: dispatch, Evidence: evidence, Client: client,
				Targets: map[string]string{config.Grant.Branch: origin}}
			if did, err := driver.Step(ctx); err != nil || !did {
				return fmt.Errorf("expected original pickup delivery: %t, %w", did, err)
			}
			return holds.Rollback(ctx)
		}
	}
	if unpaid {
		absence := installedUnpaidDriver{database: admin, chain: chain, step: worker.Step,
			binary: os.Getenv("P05_SETTLEMENT_TEST_INDEXER"), dsn: config.AbsenceDSN, rpc: config.RPC,
			profiles: config.AbsenceProfiles, signatures: config.AbsenceSignatures, authority: config.Authority,
			generation: config.Deployment.GenerationID, merchant: common.Hash(config.Profile.MerchantID).Hex()}
		fixture.ExpireOriginal = absence.expire
	}
	commerce.RunInstalledW10Settlement(t, fixture)
}
