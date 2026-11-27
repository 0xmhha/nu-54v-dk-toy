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
	Chain              uint64         `json:"chainId"`
	Contract           string         `json:"contract"`
	Asset              string         `json:"asset"`
	Beneficiary        string         `json:"beneficiary"`
	Signer             string         `json:"signer"`
	Merchant           string         `json:"merchantId"`
	KeyVersion         uint64         `json:"keyVersion"`
	BeneficiaryVersion uint64         `json:"beneficiaryVersion"`
	ABI                string         `json:"abiSha256,omitempty"` // empty: SettlementV2
	Token              commerce.Token `json:"token,omitzero"`      // empty: the dollar token
}

// issuance checks one enrolled branch and gives its quote profile.
func (p enrolledProfile) issuance(commitment [32]byte) (commerce.Profile, error) {
	for _, literal := range []string{p.Contract, p.Asset, p.Beneficiary, p.Signer} {
		if !common.IsHexAddress(literal) || common.HexToAddress(literal) == (common.Address{}) {
			return commerce.Profile{}, errors.New("invalid enrolled merchant profile address")
		}
	}
	merchant, err := hex.DecodeString(p.Merchant)
	if err != nil || len(merchant) != 32 || bytes.Equal(merchant, make([]byte, 32)) ||
		p.Chain != 8283 || p.KeyVersion == 0 || p.BeneficiaryVersion == 0 ||
		(p.ABI != "" && p.ABI != settlement.ABIHash && p.ABI != settlement.EscrowABIHash) || !p.Token.Valid() ||
		(p.Token != (commerce.Token{}) && p.ABI != settlement.EscrowABIHash) {
		return commerce.Profile{}, errors.New("invalid enrolled merchant profile")
	}
	return commerce.Profile{Chain: p.Chain, Contract: common.HexToAddress(p.Contract), Asset: common.HexToAddress(p.Asset),
		Beneficiary: common.HexToAddress(p.Beneficiary), Signer: common.HexToAddress(p.Signer), MerchantID: [32]byte(merchant),
		KeyVersion: p.KeyVersion, BeneficiaryVersion: p.BeneficiaryVersion, CommitmentKey: commitment,
		Escrow: p.ABI == settlement.EscrowABIHash, Token: p.Token}, nil
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
		profile, err := p.issuance(commitment)
		if err != nil {
			return err
		}
		profiles[branch] = profile
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
					_, _ = store.PruneMenuPhotos(ctx)
					// The automatic kitchen of a test store, only on the test chain (8283).
					_, _ = store.StepAutoKitchen(ctx, func(branch string) bool { return profiles[branch].Chain == 8283 })
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
	// P05_CUSTOMER_ADDR opens the customer entrance: no client certificate, an account proof from the paying board,
	// and a per-client request limit. Without it opsd serves only the mutual TLS routes, as before.
	customerAddr := os.Getenv("P05_CUSTOMER_ADDR")
	var customer *commerce.CustomerEntrance
	if customerAddr != "" {
		// P05_CUSTOMER_OPERATOR_CONTACT is shown on every store page for payment and refund questions.
		// Request limits, each with its own allowance so one kind cannot use up another's. An address is an IPv4
		// address or an IPv6 /64. Customers behind one address (a store's wireless network) share every limit but the
		// session's own, so those are sized for several customers at once.
		//   proofs, per address: 20 at once, then 1 a second. A checkout, an order session and a pickup QR take two
		//     requests each (challenge, proof).
		//   session starts, per address: 30 at once, then 6 a minute. Only checkout and order challenges count; an
		//     order takes two (before and after payment), so a store's network starts 15 orders in one moment and
		//     180 an hour. One address holds at most 30 + 6 x 15 = 120 checkout sessions (15 minutes each) and adds
		//     at most 8,640 proven accounts a day, instead of 30 sessions a minute through the proof limit alone.
		//   public reads, per address: 120 at once, then 10 a second. Opening a store is the list, the page and at most
		//     13 menu pages (600 rows, 50 a page), so eight customers can open the largest menu in the same moment.
		//   menu photo parts, per address: 240 at once, then 20 a second. A menu has at most 200 photos; a small one is
		//     one part of at most 18,000 bytes and a full one is nine, so at most about 360 KB a second for an address.
		//   requests with a session token, per address, before the token is read: 200 at once, then 10 a second.
		//     This is the ceiling of an address whatever number of sessions it holds: 20 customers going from cart
		//     to lock in one moment (ten requests each), or 50 following an order (one status every 5 seconds).
		//     Without it 450 sessions of one address at 2 a second each were 900 requests a second.
		//   a session's requests, per session: 40 at once, then 2 a second. An order from cart to lock is about ten
		//     requests, and the app asks for its status every 5 seconds at most.
		//   carts asked to be held, per address: 30 at once, then 6 a minute, like session starts. Each proven
		//     account holds one unpaid cart at a time and only with the balance to pay it; this bounds how many
		//     funded accounts of one address hold stock at once (30 + 6 a minute of the hold time).
		//   session tokens the server does not know, per address: 10 at once, then 1 every 5 seconds. An app whose
		//     session ended learns it from the first refusal, so ten phones behind one address each get theirs; it
		//     is apart from the proofs they then need.
		customer = &commerce.CustomerEntrance{Limiter: commerce.NewRateLimiter(60, 20, nil), ReadLimiter: commerce.NewRateLimiter(600, 120, nil),
			PhotoLimiter: commerce.NewRateLimiter(1200, 240, nil), SessionLimiter: commerce.NewRateLimiter(120, 40, nil),
			AddressLimiter: commerce.NewRateLimiter(600, 200, nil), UnknownLimiter: commerce.NewRateLimiter(12, 10, nil),
			StartLimiter:    commerce.NewRateLimiter(6, 30, nil),
			HoldLimiter:     commerce.NewRateLimiter(6, 30, nil),
			OperatorContact: os.Getenv("P05_CUSTOMER_OPERATOR_CONTACT")}
	}
	handlers := commerce.HTTP{Customer: customer, NetworkList: networks, QuoteAdmission: make(chan struct{}, 24), Store: store, Credentials: credentials, Profiles: profiles, Payment: payment, GasSponsors: sponsors, RentalEscrow: rentalEscrow, RentalReview: commissioning, PresentationKey: presentationKey, OperatorOrigin: origin, Rental: rentalControl, ProtectedClose: protectedClose, ProtectedWithdrawal: protectedWithdrawal == "1", RenterEvidence: renterEvidence, RenterV1Origins: renterV1Origins, Legacy: legacyControl, Firmware: firmware,
		Signer: signer, Cipher: cipher}
	handlers.Register(mux)
	// The customer entrance is a second listener. When it stops serving for any reason but shutdown, opsd stops
	// too: a process that answers staff but no customer must not look healthy.
	var customerServer *http.Server
	entranceFailed := make(chan error, 1)
	if customer != nil {
		// The recovery gate is not around the listener: each route enters it after its rate limit, with its body
		// read and inside the cap on requests at once, and the health route never does (CustomerEntrance.Gate).
		// So a refused request, a slow client and a health probe hold no connection of the control database,
		// which the operations listener shares.
		if gate != nil {
			customer.Gate = gate.Handler
		}
		customer.InFlight = make(chan struct{}, customerInFlight)
		// The cart of a proven account holds stock only after its balance was read from the chain, through the
		// settlement client of the branch's approved target.
		if forwarder != nil {
			customer.Funds = forwarder
		} else {
			log.Print("customer entrance: no settlement targets are enrolled, so no balance can be read and carts of proven accounts are refused")
		}
		entrance := http.NewServeMux()
		handlers.RegisterCustomer(entrance)
		customerServer = &http.Server{Addr: customerAddr, Handler: entrance, BaseContext: func(net.Listener) context.Context { return lifetime },
			ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 8 * time.Second, WriteTimeout: 10 * time.Second, MaxHeaderBytes: 8 << 10,
			TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.NoClientCert, Certificates: []tls.Certificate{certificate}}}
		accepting, err := net.Listen("tcp", customerAddr)
		if err != nil {
			return fmt.Errorf("customer entrance: %w", err)
		}
		listener := newLimitListener(accepting, customerConnections)
		go func() {
			if err := customerServer.ServeTLS(listener, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("customer entrance stopped: %v", err)
				entranceFailed <- err
			}
		}()
		// Refused requests leave one line every half minute: how many, and of which kind.
		go func() {
			tick := time.NewTicker(30 * time.Second)
			defer tick.Stop()
			for {
				select {
				case <-lifetime.Done():
					return
				case <-tick.C:
					if counts := customer.Limited.Drain(); len(counts) != 0 {
						log.Printf("customer entrance refused in 30s: proof=%d start=%d read=%d photo=%d address=%d session=%d unknown=%d hold=%d busy=%d",
							counts["proof"], counts["start"], counts["read"], counts["photo"], counts["address"], counts["session"], counts["unknown"], counts["hold"], counts["busy"])
					}
				}
			}
		}()
	}
	server := &http.Server{Addr: addr, Handler: gate.Handler(mux), BaseContext: func(net.Listener) context.Context { return lifetime }, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 8 * time.Second,
		WriteTimeout: 10 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert,
			ClientCAs: roots, Certificates: []tls.Certificate{certificate}}}
	shutdown := make(chan error, 1)
	go func() {
		var failure error
		select {
		case <-lifetime.Done():
		case failure = <-entranceFailed:
		}
		// Both listeners drain under the same deadline.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		drained := make(chan struct{})
		go func() {
			defer close(drained)
			if customerServer != nil {
				_ = customerServer.Shutdown(ctx)
			}
		}()
		_ = server.Shutdown(ctx)
		<-drained
		if failure != nil {
			failure = fmt.Errorf("customer entrance stopped: %w", failure)
		}
		shutdown <- failure
	}()
	err = server.ListenAndServeTLS("", "")
	if errors.Is(err, http.ErrServerClosed) {
		return <-shutdown
	}
	return err
}
