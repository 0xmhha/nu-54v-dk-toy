package main

import (
	"context"
	"crypto/cipher"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/settlement"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/pkg/deployment"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

var targetUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var fulfillmentSettings = []string{
	"P05_FULFILLMENT_TARGETS", "P05_DISPATCH_DATABASE_URL", "P05_DISPATCH_CA", "P05_DISPATCH_CERT", "P05_DISPATCH_KEY",
}

var receiverSettings = []string{"P05_RECEIVER_DATABASE_URL", "P05_RECEIVER_GRANTS"}

var settlementSettings = []string{
	"P05_SETTLEMENT_TARGETS", "P05_SETTLEMENT_DATABASE_URL", "P05_FINANCE_DATABASE_URL", "P05_FINANCE_GRANTS",
	"P05_P07_RECONCILIATION_DATABASE_URL", "P05_SETTLEMENT_RPC", "P05_SETTLEMENT_STORAGE_KEY_FILE",
	"P05_RELAY_ADDRESS", "P05_RELAY_MAX_GAS_PRICE", "P05_RELAY_SIGNER", "P05_RELAY_SIGNER_SHA256",
	"P05_REFUND_SIGNER", "P05_REFUND_SIGNER_SHA256", "P05_QUOTE_BOUNDARY_READERS",
	"P05_SETTLEMENT_DEPLOYMENT_PROFILES", "P05_SETTLEMENT_DEPLOYMENT_APPROVAL_KEY", "P05_SETTLEMENT_DEPLOYMENT_SIGNATURES", merchantGasPublication,
}

// The dispute account is optional. Without it nobody can settle a dispute before its deadline.
var resolverSettings = []string{"P05_ESCROW_RESOLVER", "P05_ESCROW_RESOLVER_SIGNER", "P05_ESCROW_RESOLVER_SIGNER_SHA256"}

var payoutSettings = []string{
	"P05_PAYOUT_DATABASE_URL", "P05_PAYOUT_GOVERNOR", "P05_PAYOUT_SIGNER", "P05_PAYOUT_SIGNER_SHA256",
}

func anySetting(keys []string) bool {
	for _, key := range keys {
		if _, set := os.LookupEnv(key); set {
			return true
		}
	}
	return false
}

func validateSettlementMode(commerceDSN string) error {
	fulfillment := anySetting(fulfillmentSettings)
	if !(anySetting(settlementSettings) || anySetting(payoutSettings) || anySetting(resolverSettings)) && !anySetting(receiverSettings) && !anySetting(printSettings) && !fulfillment {
		return nil
	}
	if commerceDSN == "" {
		return errors.New("settlement configuration requires P05_COMMERCE_DATABASE_URL")
	}
	if os.Getenv("P05_SETTLEMENT_TARGETS") == "" {
		return errors.New("settlement configuration requires P05_SETTLEMENT_TARGETS")
	}
	if anySetting(printSettings) && !anySetting(receiverSettings) {
		return errors.New("printer configuration requires fulfillment receiver enrollment")
	}
	if fulfillment && os.Getenv("P05_FULFILLMENT_TARGETS") == "" {
		return errors.New("fulfillment configuration requires P05_FULFILLMENT_TARGETS")
	}
	return nil
}

func installSettlement(ctx, run context.Context, commerceDSN string, quoteCipher cipher.AEAD, profiles map[string]commerce.Profile, quoteSigner settlement.Signer, mux *http.ServeMux, gate *service.RecoveryGate) (*settlement.Forwarder, func(), error) {
	nop := func() {}
	if os.Getenv("P05_SETTLEMENT_TARGETS") == "" {
		if os.Getenv(merchantGasPublication) != "" {
			return nil, nop, errors.New("merchant gas sponsorship requires enrolled settlement targets")
		}
		return nil, nop, nil
	}
	targets := map[string]settlement.Target{}
	if err := configJSON("P05_SETTLEMENT_TARGETS", &targets); err != nil {
		return nil, nop, err
	}
	if len(targets) == 0 || len(targets) > 128 {
		return nil, nop, errors.New("missing enrolled settlement targets")
	}
	if err := matchContracts(profiles, targets); err != nil {
		return nil, nop, err
	}
	financeGrants := map[string]service.Principal{}
	if err := configJSON("P05_FINANCE_GRANTS", &financeGrants); err != nil {
		return nil, nop, err
	}
	if len(financeGrants) == 0 {
		return nil, nop, errors.New("no finance certificate grants enrolled")
	}
	relayKey, err := keyFile("P05_SETTLEMENT_STORAGE_KEY_FILE")
	if err != nil {
		return nil, nop, err
	}
	quoteKey, err := keyFile("P05_QUOTE_STORAGE_KEY_FILE")
	if err != nil || relayKey == quoteKey {
		return nil, nop, errors.New("relay and quote storage require distinct keys")
	}
	commitment, err := keyFile("P05_QUOTE_COMMITMENT_KEY_FILE")
	if err != nil || relayKey == commitment {
		return nil, nop, errors.New("relay and commitment keys must be distinct")
	}
	relay, err := commerce.NewCipher(relayKey[:])
	if err != nil {
		return nil, nop, err
	}
	receiver := os.Getenv("P05_SETTLEMENT_RPC")
	gasAddress := os.Getenv("P05_RELAY_ADDRESS")
	if receiver == "" || !common.IsHexAddress(gasAddress) || common.HexToAddress(gasAddress) == (common.Address{}) {
		return nil, nop, errors.New("enroll RPC and relay sender for settlement")
	}
	limit, ok := new(big.Int).SetString(os.Getenv("P05_RELAY_MAX_GAS_PRICE"), 10)
	if !ok || limit.Sign() <= 0 {
		return nil, nop, errors.New("enroll a positive relay gas price cap")
	}
	gasSigner, err := verifiedSigner("P05_RELAY_SIGNER")
	if err != nil {
		return nil, nop, err
	}
	refundSigner, err := verifiedSigner("P05_REFUND_SIGNER")
	if err != nil {
		return nil, nop, err
	}
	var closers []func()
	var workers sync.WaitGroup
	ctxWorker, stop := context.WithCancel(run)
	closeAll := func() {
		stop()
		workers.Wait()
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}
	ready := false
	defer func() {
		if !ready {
			closeAll()
		}
	}()
	client, err := ethclient.DialContext(ctx, receiver)
	if err != nil {
		return nil, nop, errors.New("settlement RPC unavailable")
	}
	closers = append(closers, client.Close)
	activeTargets, reviews := checkSettlementDeployments(ctx, client, targets, common.HexToAddress(gasAddress))
	sponsors, err := merchantGasSponsors(activeTargets, reviews,
		func(ctx context.Context, target settlement.Target, review settlement.DeploymentReview) bool {
			return settlement.ReviewedDeploymentCurrent(ctx, client, target, review)
		})
	if err != nil {
		return nil, nop, err
	}
	resolver, resolverSigner, err := loadResolver(ctx, client, common.HexToAddress(gasAddress), activeTargets)
	if err != nil {
		return nil, nop, err
	}
	open := func(dsn, role string) (*settlement.Store, error) {
		if dsn == "" {
			return nil, errors.New("all settlement role DSNs must be supplied")
		}
		db, err := settlement.Open(ctx, dsn, role)
		if err == nil {
			closers = append(closers, db.Close)
		}
		return db, err
	}
	forward, err := open(commerceDSN, "nu54_p05_commerce")
	if err != nil {
		return nil, nop, err
	}
	ledger, err := open(os.Getenv("P05_SETTLEMENT_DATABASE_URL"), "nu54_p05_settlement")
	if err != nil {
		return nil, nop, err
	}
	for branch, target := range activeTargets {
		if err := ledger.CheckOpenContracts(ctx, branch, target, quoteCipher); err != nil {
			return nil, nop, err
		}
	}
	finance, err := open(os.Getenv("P05_FINANCE_DATABASE_URL"), "nu54_p05_finance")
	if err != nil {
		return nil, nop, err
	}
	proof, err := settlement.OpenEvidence(ctx, os.Getenv("P05_P07_RECONCILIATION_DATABASE_URL"))
	if err != nil {
		return nil, nop, err
	}
	closers = append(closers, proof.Close)
	// The chain events identify a merchant, never a database branch. Only
	// unique deployment merchant scopes can yield an independent branch total.
	merchants := map[string]string{}
	issuerMerchants := map[string]string{}
	counts := map[string]int{}
	scopes := map[string]string{}
	for branch, target := range targets {
		profile, ok := profiles[branch]
		if !ok || profile.Asset != target.Asset || profile.Contract != target.Contract {
			continue
		}
		merchant := common.Hash(profile.MerchantID).Hex()
		scope := target.Stream + "/" + merchant
		scopes[branch] = scope
		counts[scope]++
		if _, active := activeTargets[branch]; !active {
			continue
		}
		merchants[branch] = merchant
		issuerMerchants[branch] = merchant
	}
	for branch, scope := range scopes {
		if counts[scope] != 1 {
			delete(merchants, branch)
		}
	}
	explorer, err := settlement.NewPublicExplorer(os.Getenv("P05_PUBLIC_EXPLORER_ORIGIN"), os.Getenv("P05_OPERATOR_ORIGIN"))
	if err != nil {
		log.Print("public explorer origin invalid; finance links withheld")
	}
	h := settlement.FinanceHTTP{Store: finance, Credentials: financeGrants,
		OperatorOrigin: os.Getenv("P05_OPERATOR_ORIGIN"), Targets: targets,
		Merchants: merchants, Evidence: proof, CloseStore: ledger, Funds: settlement.LiveChain{Client: client}, QuoteCipher: quoteCipher,
		Deployments: reviews, PublicExplorer: explorer, GasAccount: common.HexToAddress(gasAddress), Resolver: resolver}
	h.Register(mux)
	if path := os.Getenv("P05_QUOTE_BOUNDARY_READERS"); path != "" {
		readers := service.ClientGrantFile[settlement.QuoteBoundaryReaderGrant](path)
		if _, err := readers.Read(); err != nil {
			return nil, nop, errors.New("invalid P05_QUOTE_BOUNDARY_READERS publication")
		}
		operators := service.ClientGrantFile[service.Principal](os.Getenv("P05_FINANCE_GRANTS"))
		if _, err := operators.Read(); err != nil {
			return nil, nop, errors.New("invalid X06 finance grant publication")
		}
		h.RegisterQuoteBoundary(mux, readers.Lookup, operators.Lookup, issuerMerchants)
	}
	worker := &settlement.Worker{Store: ledger, Evidence: proof, Chain: settlement.LiveChain{Client: client},
		Targets: activeTargets, QuoteCipher: quoteCipher, RelayCipher: relay, GasAccount: common.HexToAddress(gasAddress),
		GasSigner: gasSigner, RefundSigner: refundSigner, MaxGasPrice: limit, Now: time.Now,
		MerchantSigner: quoteSigner, MerchantSigners: map[string]common.Address{}, Resolver: resolver, ResolverSigner: resolverSigner}
	merchantIDs := map[string]common.Hash{}
	for branch := range activeTargets {
		if profile, ok := profiles[branch]; ok {
			worker.MerchantSigners[branch], merchantIDs[branch] = profile.Signer, profile.MerchantID
		}
	}
	worker.MerchantIDs = merchantIDs
	if anySetting(receiverSettings) {
		grants := map[string]settlement.ReceiverGrant{}
		if err := configJSON("P05_RECEIVER_GRANTS", &grants); err != nil {
			return nil, nop, err
		}
		if len(grants) == 0 {
			return nil, nop, errors.New("no fulfillment receiver identities enrolled")
		}
		for fingerprint, grant := range grants {
			decoded, err := hex.DecodeString(fingerprint)
			if err != nil || len(decoded) != 32 || len(grant.Branches) == 0 || grant.ExpiresAt <= time.Now().Unix() {
				return nil, nop, errors.New("invalid fulfillment receiver identity")
			}
			for _, branch := range grant.Branches {
				if !targetUUID.MatchString(branch) {
					return nil, nop, errors.New("invalid fulfillment receiver branch scope")
				}
			}
		}
		receiverDB, err := open(os.Getenv("P05_RECEIVER_DATABASE_URL"), "nu54_p05_receiver")
		if err != nil {
			return nil, nop, err
		}
		settlement.Receiver{Store: receiverDB, Evidence: proof, Grants: grants}.Register(mux)
	}
	var printer *settlement.Printer
	if anySetting(printSettings) {
		printer, err = installPrinter(mux, open, proof)
		if err != nil {
			return nil, nop, err
		}
	}
	var dispatch *settlement.Dispatcher
	if os.Getenv("P05_FULFILLMENT_TARGETS") != "" {
		endpoints := map[string]string{}
		if err := configJSON("P05_FULFILLMENT_TARGETS", &endpoints); err != nil {
			return nil, nop, err
		}
		if len(endpoints) == 0 {
			return nil, nop, errors.New("no fulfillment targets enrolled")
		}
		db, err := open(os.Getenv("P05_DISPATCH_DATABASE_URL"), "nu54_p05_dispatch")
		if err != nil {
			return nil, nop, err
		}
		trust := x509.NewCertPool()
		ca, err := os.ReadFile(os.Getenv("P05_DISPATCH_CA"))
		if err != nil || !trust.AppendCertsFromPEM(ca) {
			return nil, nop, errors.New("fulfillment CA required")
		}
		identity, err := tls.LoadX509KeyPair(os.Getenv("P05_DISPATCH_CERT"), os.Getenv("P05_DISPATCH_KEY"))
		if err != nil {
			return nil, nop, errors.New("fulfillment mTLS identity required")
		}
		transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: trust, Certificates: []tls.Certificate{identity}}}
		closers = append(closers, transport.CloseIdleConnections)
		dispatch = &settlement.Dispatcher{Evidence: proof, Store: db, Targets: endpoints, Client: &http.Client{
			Transport: transport, Timeout: 5 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		}}
	}
	if anySetting(payoutSettings) {
		governor := os.Getenv("P05_PAYOUT_GOVERNOR")
		if !common.IsHexAddress(governor) || common.HexToAddress(governor) == (common.Address{}) ||
			common.HexToAddress(governor) == worker.GasAccount {
			return nil, nop, errors.New("payout governor must be enrolled separately from the gas account")
		}
		signer, err := verifiedSigner("P05_PAYOUT_SIGNER")
		if err != nil {
			return nil, nop, err
		}
		database, err := open(os.Getenv("P05_PAYOUT_DATABASE_URL"), "nu54_p05_payout_worker")
		if err != nil {
			return nil, nop, err
		}
		eligible := map[string]commerce.Profile{}
		for branch := range merchants {
			if !targets[branch].Escrow() { // rotation uses SettlementV2
				eligible[branch] = profiles[branch]
			}
		}
		if len(eligible) == 0 && len(activeTargets) != 0 {
			return nil, nop, errors.New("payout worker requires uniquely scoped settlement merchant")
		}
		rotation := &settlement.PayoutWorker{Store: database, Client: client, Signer: signer,
			Governor: common.HexToAddress(governor), Profiles: eligible, MaxGasPrice: limit}
		workers.Add(1)
		go func() { defer workers.Done(); tickWorker(ctxWorker, gate, rotation.Step) }()
	}
	if len(activeTargets) != 0 {
		workers.Add(1)
		go func() { defer workers.Done(); tickWorker(ctxWorker, gate, worker.Step) }()
	}
	if dispatch != nil {
		workers.Add(1)
		go func() { defer workers.Done(); tickWorker(ctxWorker, gate, dispatch.Step) }()
	}
	if printer != nil {
		workers.Add(1)
		go func() { defer workers.Done(); tickWorker(ctxWorker, gate, printer.Step) }()
	}
	ready = true
	return &settlement.Forwarder{Store: forward, QuoteCipher: quoteCipher, RelayCipher: relay, Targets: activeTargets, GasSponsors: sponsors, MerchantIDs: merchantIDs,
		RentalEscrow: settlement.RentalEscrow{Client: client, Targets: activeTargets, Reviews: reviews}}, closeAll, nil
}

// loadResolver enrolls the account that settles disputes. It must be the contract's arbiter and must
// not be the gas account, whose key signs every other relay transaction.
func loadResolver(ctx context.Context, client *ethclient.Client, gas common.Address, targets map[string]settlement.Target) (common.Address, settlement.Signer, error) {
	if !anySetting(resolverSettings) {
		return common.Address{}, nil, nil
	}
	configured := os.Getenv("P05_ESCROW_RESOLVER")
	resolver := common.HexToAddress(configured)
	if !common.IsHexAddress(configured) || resolver == (common.Address{}) || resolver == gas {
		return common.Address{}, nil, errors.New("the dispute account must be enrolled separately from the gas account")
	}
	signer, err := verifiedSigner("P05_ESCROW_RESOLVER_SIGNER")
	if err != nil {
		return common.Address{}, nil, err
	}
	if err := settlement.CheckArbiter(ctx, client, resolver, targets); err != nil {
		return common.Address{}, nil, err
	}
	return resolver, signer, nil
}

// matchContracts refuses a branch whose issuance profile and relay target name different payment
// contracts: its quotes could never be paid the way its relay pays.
func matchContracts(profiles map[string]commerce.Profile, targets map[string]settlement.Target) error {
	for branch, target := range targets {
		if profile, enrolled := profiles[branch]; enrolled && profile.Escrow != target.Escrow() {
			return errors.New("issuance profile and settlement target use different payment contracts")
		}
	}
	return nil
}

func checkSettlementDeployments(ctx context.Context, client *ethclient.Client, targets map[string]settlement.Target, gas common.Address) (
	map[string]settlement.Target, map[string]settlement.DeploymentReview,
) {
	var approvals []deployment.Profile
	var signatures map[string]string
	keyText := os.Getenv("P05_SETTLEMENT_DEPLOYMENT_APPROVAL_KEY")
	required := keyText != "" || os.Getenv("P05_SETTLEMENT_DEPLOYMENT_PROFILES") != "" ||
		os.Getenv("P05_SETTLEMENT_DEPLOYMENT_SIGNATURES") != ""
	key, _ := hex.DecodeString(keyText)
	if required && (configJSON("P05_SETTLEMENT_DEPLOYMENT_PROFILES", &approvals) != nil ||
		configJSON("P05_SETTLEMENT_DEPLOYMENT_SIGNATURES", &signatures) != nil || len(approvals) > 128) {
		approvals = nil // Fail closed for the deployments that need this publication.
	}
	active, reviews := make(map[string]settlement.Target), make(map[string]settlement.DeploymentReview)
	for branch, target := range targets {
		view := settlement.DeploymentReview{Status: "invalid"}
		manifest, err := hex.DecodeString(target.Manifest)
		if targetUUID.MatchString(branch) && targetUUID.MatchString(target.Stream) && err == nil && len(manifest) == 32 &&
			target.Asset != (common.Address{}) && (target.Escrow() || (target.ABIHash() == settlement.ABIHash &&
			target.RefundSigner != (common.Address{}) && target.RefundVersion > 0)) {
			view = settlement.VerifyOperatorDeployment(ctx, client, target, gas, approvals, key, signatures, required)
			if !settlement.SponsorReady(ctx, client, target, gas) {
				view.Status = "invalid"
			}
		}
		reviews[branch] = view
		if view.Status != "invalid" {
			active[branch] = target
		}
	}
	return active, reviews
}

func verifiedSigner(key string) (processSigner, error) {
	file := os.Getenv(key)
	digest, err := hex.DecodeString(os.Getenv(key + "_SHA256"))
	if err != nil || len(digest) != 32 || !filepath.IsAbs(file) {
		return processSigner{}, errors.New("pin settlement signer executable path and SHA-256")
	}
	signer := processSigner{path: file, want: [32]byte(digest)}
	if _, err := signer.material(); err != nil {
		return processSigner{}, err
	}
	return signer, nil
}

func tickWorker(ctx context.Context, gate *service.RecoveryGate, step func(context.Context) (bool, error)) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			for range 8 {
				call, cancel := context.WithTimeout(ctx, 7*time.Second)
				var moved bool
				err := gate.Run(call, func(ctx context.Context) error {
					var err error
					moved, err = step(ctx)
					return err
				})
				cancel()
				if err != nil && !errors.Is(err, service.ErrRecoveryFenced) {
					log.Print("settlement task remains pending")
				}
				if !moved {
					break
				}
			}
		}
	}
}
