package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/settlement"
)

type processSigner struct {
	path string
	want [32]byte
}

func (s processSigner) material() ([]byte, error) {
	executable, err := os.ReadFile(s.path)
	if err != nil || sha256.Sum256(executable) != s.want {
		return nil, commerce.ErrSigning
	}
	return executable, nil
}

func (s processSigner) SignDigest(ctx context.Context, digest [32]byte) ([]byte, error) {
	executable, err := s.material()
	if err != nil {
		return nil, err
	}
	return signVerified(ctx, executable, digest)
}

func signVerified(ctx context.Context, executable []byte, digest [32]byte) ([]byte, error) {
	path, err := snapshotProgram(executable)
	if err != nil {
		return nil, commerce.ErrSigning
	}
	defer os.RemoveAll(filepath.Dir(path))
	return signWithProgram(ctx, path, digest)
}

type enrolledQuoteSigner struct {
	path string
}

func enrollProcessSigner(source string, want [32]byte) (enrolledQuoteSigner, error) {
	executable, err := os.ReadFile(source)
	if err != nil || sha256.Sum256(executable) != want {
		return enrolledQuoteSigner{}, commerce.ErrSigning
	}
	path, err := snapshotProgram(executable)
	if err != nil {
		return enrolledQuoteSigner{}, commerce.ErrSigning
	}
	return enrolledQuoteSigner{path: path}, nil
}

func (s enrolledQuoteSigner) Close() {
	_ = os.RemoveAll(filepath.Dir(s.path))
}

func (s enrolledQuoteSigner) SignDigest(ctx context.Context, digest [32]byte) ([]byte, error) {
	return signWithProgram(ctx, s.path, digest)
}

func signWithProgram(ctx context.Context, path string, digest [32]byte) ([]byte, error) {
	output := signerOutput{}
	if err := executePinnedProgram(ctx, path, strings.NewReader(hex.EncodeToString(digest[:])+"\n"), &output, nil); err != nil {
		return nil, commerce.ErrSigning
	}
	signed, err := hex.DecodeString(strings.TrimSpace(output.String()))
	if err != nil || len(signed) != 65 {
		return nil, commerce.ErrSigning
	}
	return signed, nil
}

type signerOutput struct{ bytes.Buffer }

func (b *signerOutput) Write(data []byte) (int, error) {
	if b.Len()+len(data) > 140 {
		return 0, commerce.ErrSigning
	}
	return b.Buffer.Write(data)
}

func snapshotProgram(executable []byte) (string, error) {
	// Keep the exact verified bytes in an owner-only directory. Each child
	// executes this file, regardless of changes to the original enrollment path.
	directory, err := os.MkdirTemp("", "nu54-pinned-program-")
	if err != nil {
		return "", err
	}
	snapshot := filepath.Join(directory, "program")
	if err := os.WriteFile(snapshot, executable, 0o500); err != nil {
		_ = os.RemoveAll(directory)
		return "", err
	}
	return snapshot, nil
}

func runPinnedProgram(ctx context.Context, executable []byte, input io.Reader, output io.Writer, env []string) error {
	snapshot, err := snapshotProgram(executable)
	if err != nil {
		return err
	}
	defer os.RemoveAll(filepath.Dir(snapshot))
	return executePinnedProgram(ctx, snapshot, input, output, env)
}

func executePinnedProgram(ctx context.Context, snapshot string, input io.Reader, output io.Writer, env []string) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, snapshot)
	command.Stdin, command.Stdout, command.Env = input, output, env
	return command.Run()
}

type enrolledProfile struct {
	Chain              uint64 `json:"chainId"`
	Contract           string `json:"contract"`
	Asset              string `json:"asset"`
	Beneficiary        string `json:"beneficiary"`
	Signer             string `json:"signer"`
	Merchant           string `json:"merchantId"`
	KeyVersion         uint64 `json:"keyVersion"`
	BeneficiaryVersion uint64 `json:"beneficiaryVersion"`
	ABI                string `json:"abiSha256,omitempty"` // empty: SettlementV2
}

func configJSON(key string, value any) error {
	data, err := os.ReadFile(os.Getenv(key))
	if err != nil {
		return fmt.Errorf("%s must point to the deployment settings", key)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(value); err != nil {
		return fmt.Errorf("invalid %s", key)
	}
	return nil
}

// loadNetworkList enrolls the signed network list. Without the settings no customer gets one.
func loadNetworkList() (*commerce.NetworkList, error) {
	path, operator := os.Getenv("P05_NETWORK_LIST_FILE"), os.Getenv("P05_NETWORK_LIST_OPERATOR")
	if path == "" && operator == "" {
		return nil, nil
	}
	list := &commerce.NetworkList{Path: path, Operator: common.HexToAddress(operator)}
	if !filepath.IsAbs(path) || !common.IsHexAddress(operator) || list.Operator == (common.Address{}) {
		return nil, errors.New("set P05_NETWORK_LIST_FILE to an absolute path and P05_NETWORK_LIST_OPERATOR to its signer")
	}
	if _, err := list.Read(time.Now()); err != nil {
		log.Printf("network list withheld until a valid one is published: %v", err)
	}
	return list, nil
}

func keyFile(key string) ([32]byte, error) {
	raw, err := os.ReadFile(os.Getenv(key))
	if err != nil {
		return [32]byte{}, fmt.Errorf("cannot read %s", key)
	}
	decoded, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(decoded) != 32 || bytes.Equal(decoded, make([]byte, 32)) {
		return [32]byte{}, fmt.Errorf("invalid %s", key)
	}
	return [32]byte(decoded), nil
}

func serveCommerce(lifetime context.Context, addr, database string, mux *http.ServeMux, gate *service.RecoveryGate) error {
	credentials := map[string]service.Principal{}
	if err := configJSON("P05_COMMERCE_GRANTS", &credentials); err != nil {
		return err
	}
	if len(credentials) == 0 {
		return errors.New("no commerce certificate grants enrolled")
	}
	raw := map[string]enrolledProfile{}
	if err := configJSON("P05_COMMERCE_PROFILES", &raw); err != nil {
		return err
	}
	commitment, err := keyFile("P05_QUOTE_COMMITMENT_KEY_FILE")
	if err != nil {
		return err
	}
	storage, err := keyFile("P05_QUOTE_STORAGE_KEY_FILE")
	if err != nil {
		return err
	}
	if storage == commitment {
		return errors.New("storage and quote commitment keys must be independent")
	}
	cipher, err := commerce.NewCipher(storage[:])
	if err != nil {
		return err
	}
	profiles := map[string]commerce.Profile{}
	for branch, p := range raw {
		for _, literal := range []string{p.Contract, p.Asset, p.Beneficiary, p.Signer} {
			if !common.IsHexAddress(literal) || common.HexToAddress(literal) == (common.Address{}) {
				return errors.New("invalid enrolled merchant profile address")
			}
		}
		merchant, err := hex.DecodeString(p.Merchant)
		if err != nil || len(merchant) != 32 || bytes.Equal(merchant, make([]byte, 32)) ||
			p.Chain != 8283 || p.KeyVersion == 0 || p.BeneficiaryVersion == 0 ||
			(p.ABI != "" && p.ABI != settlement.ABIHash && p.ABI != settlement.EscrowABIHash) {
			return errors.New("invalid enrolled merchant profile")
		}
		profiles[branch] = commerce.Profile{Chain: p.Chain, Contract: common.HexToAddress(p.Contract), Asset: common.HexToAddress(p.Asset),
			Beneficiary: common.HexToAddress(p.Beneficiary), Signer: common.HexToAddress(p.Signer), MerchantID: [32]byte(merchant),
			KeyVersion: p.KeyVersion, BeneficiaryVersion: p.BeneficiaryVersion, CommitmentKey: commitment,
			Escrow: p.ABI == settlement.EscrowABIHash}
	}
	if len(profiles) == 0 {
		return errors.New("no merchant issuance profile configured")
	}

	signerPath := os.Getenv("P05_QUOTE_SIGNER")
	want, err := hex.DecodeString(os.Getenv("P05_QUOTE_SIGNER_SHA256"))
	if err != nil || len(want) != 32 || !filepath.IsAbs(signerPath) {
		return errors.New("pin P05_QUOTE_SIGNER to an absolute executable path and SHA-256")
	}
	signer, err := enrollProcessSigner(signerPath, [32]byte(want))
	if err != nil {
		return errors.New("cannot enroll the pinned quote signer")
	}
	defer signer.Close()
	roots := x509.NewCertPool()
	ca, err := os.ReadFile(os.Getenv("P05_COMMERCE_CA"))
	if err != nil || !roots.AppendCertsFromPEM(ca) {
		return errors.New("C3 client trust CA required")
	}
	certificate, err := tls.LoadX509KeyPair(os.Getenv("P05_COMMERCE_CERT"), os.Getenv("P05_COMMERCE_KEY"))
	if err != nil {
		return errors.New("C3 TLS certificate and key required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store, err := commerce.Open(ctx, database)
	if err != nil {
		return err
	}
	defer store.Close()
	poll, stop := context.WithCancel(context.Background())
	defer stop()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-poll.Done():
				return
			case <-ticker.C:
				_ = gate.Run(poll, func(ctx context.Context) error {
					_ = store.RetireTerminalReceipts(ctx)
					for range 8 {
						changed, err := store.ReclaimExpired(ctx)
						if err != nil || !changed {
							return err
						}
					}
					return nil
				})
			}
		}
	}()
	if dsn := os.Getenv("P05_SUBSCRIPTION_WORKER_DATABASE_URL"); dsn != "" {
		worker, err := commerce.OpenSubscriptionWorker(ctx, dsn)
		if err != nil {
			return errors.New("subscription worker requires a separate enrolled runtime login")
		}
		finished := make(chan struct{})
		go func() { defer close(finished); tickWorker(poll, gate, worker.Step) }()
		defer func() { stop(); <-finished; worker.Close() }()
	}
	forwarder, closeFinance, err := installSettlement(ctx, poll, database, cipher, profiles, signer, mux, gate)
	if err != nil {
		return err
	}
	defer closeFinance()
	var payment commerce.PaymentForwarder
	var sponsors commerce.GasSponsorLookup
	var rentalEscrow commerce.RentalEscrowAuthority
	if forwarder != nil {
		payment = forwarder
		sponsors = forwarder.GasSponsors
		rentalEscrow = forwarder.RentalEscrow
	}
	commissioning, err := rentalReviewCommissioning()
	if err != nil {
		return err
	}
	if commissioning != nil && rentalEscrow == nil {
		return errors.New("rental review commissioning requires v2 settlement")
	}
	var presentationKey ed25519.PrivateKey
	if os.Getenv("P05_PRESENTATION_KEY_FILE") != "" {
		seed, keyErr := keyFile("P05_PRESENTATION_KEY_FILE")
		if keyErr != nil {
			return keyErr
		}
		if seed == storage || seed == commitment {
			return errors.New("presentation signing key must be independent")
		}
		presentationKey = ed25519.NewKeyFromSeed(seed[:])
	}
	origin := os.Getenv("P05_OPERATOR_ORIGIN")
	if origin != "" {
		parsed, parseErr := url.Parse(origin)
		if parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
			return errors.New("P05_OPERATOR_ORIGIN must be an HTTPS origin")
		}
	}
	if directory := os.Getenv("P05_CONSOLE_DIR"); directory != "" {
		if origin == "" || !filepath.IsAbs(directory) {
			return errors.New("console requires an absolute build directory and operator origin")
		}
		mux.Handle("GET /ops/", http.StripPrefix("/ops/", http.FileServer(http.Dir(directory))))
	}
	rental, err := loadRentalController(ctx)
	if err != nil {
		return err
	}
	var rentalControl commerce.RentalControl
	if rental != nil {
		rentalControl = rental
	}
	if dsn := os.Getenv("P05_RENTAL_AUTHORITY_DATABASE_URL"); dsn != "" {
		if rental == nil {
			return errors.New("protected rental collector requires the pinned rental controller")
		}
		issuer, err := commerce.OpenRentalCollectorIssuer(ctx, dsn, rental.operator, rental.deployment, rental.manifest)
		if err != nil {
			return err
		}
		defer issuer.Close()
		issuer.Register(mux, credentials)
	}
	if dsn := os.Getenv("P05_RENTAL_WRITER_DATABASE_URL"); dsn != "" {
		if rental == nil {
			return errors.New("protected rental writer requires the pinned rental controller")
		}
		reader, err := loadRentalWriterEvidenceReader()
		if err != nil {
			return err
		}
		defer reader.Close()
		writer, err := openRentalWriter(ctx, dsn, rental, reader)
		if err != nil {
			return err
		}
		defer writer.Close()
		if os.Getenv("P05_RENTAL_V1_ORDER_TERMINALS") != "" {
			writer.EnablePendingOrderOrigins()
		}
		writer.Register(mux, credentials)
	}
	if dsn := os.Getenv("P05_RENTAL_AUTHORITY_READ_DATABASE_URL"); dsn != "" {
		if rental == nil {
			return errors.New("protected rental authority reader requires the pinned rental controller")
		}
		reader, err := openRentalAuthorityReader(ctx, dsn, rental)
		if err != nil {
			return err
		}
		defer reader.Close()
		reader.Register(mux)
	}
	var renterV1Origins *commerce.RenterV1Origins
	if os.Getenv("P05_RENTAL_V1_ORDER_TERMINALS") != "" {
		if rental == nil {
			return errors.New("v1 renter origins require the pinned rental deployment")
		}
		config := &commerce.RenterV1Origins{Deployment: rental.deployment, Manifest: rental.manifest, MerchantStatus: rental.chain.MerchantStatusContext, TokenMetadata: rental.chain.RentalTokenMetadata}
		if err := configJSON("P05_RENTAL_V1_ORDER_TERMINALS", &config.Issuers); err != nil {
			return err
		}
		if err := config.Validate(); err != nil {
			return err
		}
		for fingerprint, issuer := range config.Issuers {
			actor, found := credentials[fingerprint]
			if !found || actor.Role != "terminal" || actor.Tenant != issuer.Tenant || actor.Subject != issuer.Terminal {
				return errors.New("v1 renter issuer must match an enrolled terminal certificate")
			}
		}
		renterV1Origins = config
	}
	var renterEvidence *commerce.RenterReturnEvidence
	if enabled := os.Getenv("P05_RENTAL_RENTER_EVIDENCE"); enabled != "" {
		if enabled != "1" || rental == nil {
			return errors.New("renter assessment requires the pinned rental deployment")
		}
		reader, err := loadRentalWriterEvidenceReader()
		if err != nil {
			return err
		}
		defer reader.Close()
		renterEvidence = &commerce.RenterReturnEvidence{Reader: reader, Deployment: rental.deployment, Manifest: rental.manifest}
	}
	var protectedClose *commerce.ProtectedRentalClose
	if dsn := os.Getenv("P05_RENTAL_PROTECTED_CLOSE_DATABASE_URL"); dsn != "" {
		if rental == nil || origin == "" {
			return errors.New("protected rental close requires the pinned rental controller and operator origin")
		}
		reader, err := loadRentalWriterEvidenceReader()
		if err != nil {
			return err
		}
		defer reader.Close()
		protectedClose, err = commerce.OpenProtectedRentalClose(ctx, dsn, rental.chain, reader, cipher, rental.operator, rental.deployment, rental.manifest)
		if err != nil {
			return err
		}
		defer protectedClose.Close()
	}
	protectedWithdrawal := os.Getenv("P05_RENTAL_PROTECTED_WITHDRAWAL")
	if protectedWithdrawal != "" && (protectedWithdrawal != "1" || protectedClose == nil) {
		return errors.New("protected withdrawal requires the authenticated protected close configuration")
	}
	if dsn := os.Getenv("P05_RENTAL_WITHDRAWAL_RECONCILER_DATABASE_URL"); dsn != "" {
		if protectedWithdrawal != "1" || rental == nil {
			return errors.New("withdrawal reconciliation requires the pinned protected withdrawal mode")
		}
		reconciler, err := commerce.OpenProtectedWithdrawalReconciler(ctx, dsn, rental.chain, rental.manifest, rental.deployment.Settlement)
		if err != nil {
			return err
		}
		reconcileCtx, cancelReconcile := context.WithCancel(lifetime)
		reconciled := make(chan struct{})
		defer func() {
			cancelReconcile()
			<-reconciled
			reconciler.Close()
		}()
		go func() {
			defer close(reconciled)
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-reconcileCtx.Done():
					return
				case <-ticker.C:
					_ = reconciler.Reconcile(reconcileCtx)
				}
			}
		}()
	}
	legacy, err := loadLegacyConsole(rental)
	if err != nil {
		return err
	}
	var legacyControl commerce.LegacyConsole
	if legacy != nil {
		legacyControl = legacy
	}
	firmware, err := loadFirmwareReleases()
	if err != nil {
		return err
	}
	networks, err := loadNetworkList()
	if err != nil {
		return err
	}
	commerce.HTTP{NetworkList: networks, QuoteAdmission: make(chan struct{}, 24), Store: store, Credentials: credentials, Profiles: profiles, Payment: payment, GasSponsors: sponsors, RentalEscrow: rentalEscrow, RentalReview: commissioning, PresentationKey: presentationKey, OperatorOrigin: origin, Rental: rentalControl, ProtectedClose: protectedClose, ProtectedWithdrawal: protectedWithdrawal == "1", RenterEvidence: renterEvidence, RenterV1Origins: renterV1Origins, Legacy: legacyControl, Firmware: firmware,
		Signer: signer, Cipher: cipher}.Register(mux)
	server := &http.Server{Addr: addr, Handler: gate.Handler(mux), BaseContext: func(net.Listener) context.Context { return lifetime }, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 8 * time.Second,
		WriteTimeout: 10 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert,
			ClientCAs: roots, Certificates: []tls.Certificate{certificate}}}
	shutdown := make(chan struct{})
	go func() {
		defer close(shutdown)
		<-lifetime.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	err = server.ListenAndServeTLS("", "")
	if errors.Is(err, http.ErrServerClosed) {
		<-shutdown
		return nil
	}
	return err
}
