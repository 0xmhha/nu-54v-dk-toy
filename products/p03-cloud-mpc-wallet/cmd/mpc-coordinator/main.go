package main

import (
	"cmp"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/marketlab"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type config struct {
	MarketPolicy            *marketlab.Policy
	Listen                  string
	APICertificate          string
	APIKey                  string
	ParticipantCertificate  string
	ParticipantKey          string
	ParticipantCA           string
	SigningSeed             string
	IdentityIssuer          string
	ProfileSubject          string
	ProfilesDirectory       string
	ProfilesAuthority       ed25519.PublicKey
	IdentityKey             ed25519.PublicKey
	ApprovalKey             ed25519.PublicKey
	RecoveryKey             ed25519.PublicKey
	RecoveryRefusalCA       string
	RefusalCA               string
	First                   []ceremony.Peer
	Replacement             []ceremony.Peer
	Future                  [][]ceremony.Peer
	RecoveryDelaySeconds    uint64
	MobileRequests          string
	MobileApproved          string
	RecoveryOperatorIssuer  string
	RecoveryOperatorSubject string
	RecoveryOperatorKey     ed25519.PublicKey
	RecoveryOperatorGrants  string
	OperatorConsole         *operatorConsoleConfig
	Endpoints               map[string]ceremony.Endpoint
}

func main() {
	file := flag.String("config", "", "coordinator configuration file")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *file); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, file string) error {
	raw, err := os.ReadFile(file)
	if err != nil {
		return errors.New("cannot read coordinator configuration")
	}
	var c config
	if err := json.Unmarshal(raw, &c); err != nil {
		return errors.New("invalid coordinator configuration")
	}
	if c.IdentityIssuer == "" || len(c.IdentityKey) != ed25519.PublicKeySize {
		return errors.New("P03 requires a dedicated identity authority")
	}
	if c.ProfileSubject != "" && (len(c.ApprovalKey) != ed25519.PublicKeySize || len(c.RecoveryKey) != ed25519.PublicKeySize ||
		c.IdentityKey.Equal(c.ApprovalKey) || c.IdentityKey.Equal(c.RecoveryKey) || c.ApprovalKey.Equal(c.RecoveryKey) ||
		c.RecoveryDelaySeconds < 60 || c.RecoveryDelaySeconds > 600) {
		return errors.New("legacy P03 profile requires independent approval keys and recovery delay")
	}

	subject, err := uuid.Parse(c.ProfileSubject)
	if c.ProfileSubject != "" && (err != nil || subject == uuid.Nil || subject.String() != c.ProfileSubject) || c.ProfileSubject == "" && c.ProfilesDirectory == "" {
		return errors.New("dedicated pairwise pilot subject is required")
	}
	now := time.Now()
	test := ceremony.Session{ID: "f0ef330e-0aab-4473-b52a-8d5f8dc8e78d", Wallet: "862887ba-0fc2-4d27-ad34-7cf8f7bd15ed",
		Kind: ceremony.DKG, Epoch: 1, Expires: now.Add(time.Minute).Unix(), Peers: c.First}
	if _, err := test.Fingerprint(now); c.ProfileSubject != "" && err != nil {
		return errors.New("invalid initial committee")
	}
	profile := wallet.Profile{First: c.First, Replacement: c.Replacement, Future: c.Future, RecoveryDelay: time.Duration(c.RecoveryDelaySeconds) * time.Second}
	seen := make(map[string]bool)
	pins := make(map[string]bool)
	for epoch := uint64(1); c.ProfileSubject != "" && epoch <= uint64(2+len(c.Future)); epoch++ {
		group := profile.Committee(epoch)
		if epoch > 1 {
			if len(group) == 0 && epoch == 2 && len(c.Future) == 0 {
				break
			}
			test.KeySession, test.Kind, test.Epoch = test.ID, ceremony.Refresh, epoch
			test.Peers = nil
			for _, p := range profile.Committee(epoch - 1) {
				if p.Role != ceremony.A {
					test.Peers = append(test.Peers, p)
				}
			}
			test.Peers = append(test.Peers, group...)
			if _, err := test.Fingerprint(now); err != nil {
				return errors.New("invalid replacement committee")
			}
		}
		for _, p := range group {
			e, ok := c.Endpoints[p.ID]
			if !ok || seen[p.ID] || pins[e.CertificateSHA256] {
				return errors.New("missing independent participant endpoint")
			}
			seen[p.ID], pins[e.CertificateSHA256] = true, true
		}
	}
	seed, err := readSeed(c.SigningSeed)
	if err != nil {
		return err
	}
	signing := ed25519.NewKeyFromSeed(seed)
	clear(seed)
	defer clear(signing)
	public := signing.Public().(ed25519.PublicKey)
	if public.Equal(c.IdentityKey) || public.Equal(c.ApprovalKey) || public.Equal(c.RecoveryKey) {
		return errors.New("coordinator requires a separate signing key")
	}
	if c.RecoveryOperatorIssuer != "" || c.RecoveryOperatorSubject != "" || len(c.RecoveryOperatorKey) > 0 {
		op, err := uuid.Parse(c.RecoveryOperatorSubject)
		if c.RecoveryOperatorSubject != "" && (err != nil || op == uuid.Nil || op.String() != c.RecoveryOperatorSubject || c.RecoveryOperatorSubject == c.ProfileSubject) ||
			c.RecoveryOperatorIssuer == "" || c.RecoveryOperatorIssuer == c.IdentityIssuer || len(c.RecoveryOperatorKey) != ed25519.PublicKeySize ||
			c.RecoveryOperatorKey.Equal(c.IdentityKey) || c.RecoveryOperatorKey.Equal(c.ApprovalKey) || c.RecoveryOperatorKey.Equal(c.RecoveryKey) ||
			c.RecoveryOperatorKey.Equal(public) {
			return errors.New("independent recovery operator configuration required")
		}
	}
	control, err := ceremony.NewController(c.ParticipantCertificate, c.ParticipantKey, c.ParticipantCA, c.Endpoints)
	if err != nil {
		return errors.New("participant control unavailable")
	}
	dsn := os.Getenv("P03_DATABASE_URL")
	if dsn == "" {
		return errors.New("P03_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return errors.New("invalid P03 database configuration")
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return errors.New("P03 database unavailable")
	}
	if c.MarketPolicy != nil && !c.MarketPolicy.Valid() {
		return errors.New("invalid dedicated market signing profile")
	}
	store := wallet.Store{Pool: pool, Profile: profile, MarketPolicy: c.MarketPolicy}
	var router wallet.Participants = control
	if c.ProfilesDirectory != "" {
		if len(c.ProfilesAuthority) != ed25519.PublicKeySize || c.ProfilesAuthority.Equal(c.IdentityKey) ||
			c.ProfilesAuthority.Equal(c.RecoveryOperatorKey) || c.ProfilesAuthority.Equal(c.ApprovalKey) ||
			c.ProfilesAuthority.Equal(c.RecoveryKey) || c.ProfilesAuthority.Equal(public) {
			return errors.New("independent profiles authority required")
		}
		store.Bindings = &wallet.Bindings{Directory: c.ProfilesDirectory, Authority: c.ProfilesAuthority, IdentityKey: c.IdentityKey,
			CoordinatorKey: public, OperatorKey: c.RecoveryOperatorKey, LegacySubject: c.ProfileSubject,
			LegacyEndpoints: c.Endpoints, LegacyApproval: c.ApprovalKey, LegacyRecovery: c.RecoveryKey}
		router = &wallet.BoundParticipants{Store: store, Legacy: control, CertPath: c.ParticipantCertificate, KeyPath: c.ParticipantKey, CAPath: c.ParticipantCA}
	}
	worker := wallet.Worker{Store: store, Control: router, SigningKey: signing}
	workerCtx, stopWorker := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- worker.Run(workerCtx) }()
	defer func() { stopWorker(); <-done }()
	api := wallet.API{Store: store, Mobile: wallet.MobileRegistry{Requests: c.MobileRequests, Approved: c.MobileApproved}, ProfileSubject: c.ProfileSubject, RecoveryOperatorSubject: c.RecoveryOperatorSubject, RecoveryOperator: wallet.IdentityVerifier{Issuer: c.RecoveryOperatorIssuer, Key: c.RecoveryOperatorKey}, Identity: wallet.IdentityVerifier{Issuer: c.IdentityIssuer, Key: c.IdentityKey}, ApprovalKey: c.ApprovalKey, RecoveryKey: c.RecoveryKey, OperatorParticipants: router, OperatorGrant: grantSource(c.RecoveryOperatorGrants)}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13}
	if c.RefusalCA != "" && c.RecoveryRefusalCA != "" && c.RefusalCA != c.RecoveryRefusalCA {
		return errors.New("conflicting refusal client authorities")
	}
	if authority := cmp.Or(c.RefusalCA, c.RecoveryRefusalCA); authority != "" {
		pem, err := os.ReadFile(authority)
		roots := x509.NewCertPool()
		if err != nil || !roots.AppendCertsFromPEM(pem) {
			return errors.New("invalid refusal client authority")
		}
		tlsConfig.ClientCAs = roots
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	}
	server := &http.Server{Addr: c.Listen, Handler: api.Handler(), ReadHeaderTimeout: 4 * time.Second, ReadTimeout: 8 * time.Second,
		WriteTimeout: 8 * time.Second, IdleTimeout: 30 * time.Second, TLSConfig: tlsConfig}
	stopped := make(chan error, 2)
	var operatorServer *http.Server
	if c.OperatorConsole != nil {
		options := *c.OperatorConsole
		if options.Listen == c.Listen || options.Origin == c.IdentityIssuer {
			return errors.New("operator listener must be separate from the wallet")
		}
		operatorServer, err = newOperatorServer(options, c.RecoveryOperatorIssuer, api)
		if err != nil {
			return err
		}
		go func() { stopped <- operatorServer.ListenAndServeTLS(options.Certificate, options.Key) }()
		defer operatorServer.Close()
	}
	go func() { stopped <- server.ListenAndServeTLS(c.APICertificate, c.APIKey) }()
	defer server.Close()
	select {
	case err := <-stopped:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if operatorServer != nil {
			return errors.Join(operatorServer.Shutdown(shutdown), server.Shutdown(shutdown))
		}
		return server.Shutdown(shutdown)
	}
}

func readSeed(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != ed25519.SeedSize {
		return nil, errors.New("coordinator signing seed must be a 32-byte private file")
	}
	return os.ReadFile(path)
}
