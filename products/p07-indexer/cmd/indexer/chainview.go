package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	weburl "net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
)

func chainviewReaders(ctx context.Context) ([]*chainview.Reader, func(), error) {
	empty := func() {}
	url, endpoint := os.Getenv("P07_CHAINVIEW_DATABASE_URL"), os.Getenv("P07_RPC")
	if url == "" || endpoint == "" {
		return nil, empty, errors.New("P07_CHAINVIEW_DATABASE_URL and P07_RPC are required")
	}
	file, err := os.Open(os.Getenv("P07_CHAINVIEW_PROFILES"))
	if err != nil {
		return nil, empty, fmt.Errorf("open P07_CHAINVIEW_PROFILES: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var profiles []chainview.Profile
	if err := decoder.Decode(&profiles); err != nil {
		return nil, empty, fmt.Errorf("decode P07_CHAINVIEW_PROFILES: %w", err)
	}
	if len(profiles) == 0 || len(profiles) > 32 || decoder.Decode(new(any)) != io.EOF {
		return nil, empty, errors.New("expected 1 to 32 deployment profiles")
	}
	db, err := chainview.Open(ctx, url)
	if err != nil {
		return nil, empty, err
	}
	var readers []*chainview.Reader
	var sources []*chainview.RPCSource
	closeAll := func() {
		for _, source := range sources {
			source.Close()
		}
		db.Close()
	}
	approvalKey := os.Getenv("P07_CHAINVIEW_APPROVAL_KEY")
	key, keyErr := hex.DecodeString(approvalKey)
	signatureFile := os.Getenv("P07_CHAINVIEW_SIGNATURES")
	requireApproval := approvalKey != "" || signatureFile != ""
	signatures := map[string]string{}
	if requireApproval {
		file, err := os.Open(signatureFile)
		if err != nil {
			closeAll()
			return nil, empty, errors.New("could not load deployment signatures")
		}
		d := json.NewDecoder(io.LimitReader(file, 1<<20))
		err = d.Decode(&signatures)
		extra := d.Decode(new(any))
		file.Close()
		if err != nil || extra != io.EOF || keyErr != nil || len(key) != ed25519.PublicKeySize {
			closeAll()
			return nil, empty, errors.New("invalid deployment signature configuration")
		}
	}
	observe := func(profile chainview.Profile, reason string) {
		if err := db.HoldProfile(ctx, profile, reason); err != nil {
			log.Printf("deployment profile hold could not be recorded: %v", err)
		}
	}
	seen := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		if seen[profile.GenerationID] {
			closeAll()
			return nil, empty, errors.New("duplicate generation in profile list")
		}
		seen[profile.GenerationID] = true
		if _, err := chainview.NewDecoder(profile); err != nil {
			reason := "identity_mismatch"
			if errors.Is(err, chainview.ErrUnknownABI) {
				reason = "unknown_abi"
			}
			observe(profile, reason)
			continue
		}
		signature, signatureErr := hex.DecodeString(signatures[profile.GenerationID])
		if requireApproval && (signatureErr != nil || !chainview.VerifyProfileApproval(profile, key, signature)) {
			observe(profile, "unapproved_profile")
			continue
		}
		registered := profile
		handshake, cancel := context.WithTimeout(ctx, 20*time.Second)
		source, err := chainview.Dial(handshake, endpoint, profile)
		if err != nil {
			cancel()
			if errors.Is(err, chainview.ErrDeploymentIdentity) {
				observe(profile, "identity_mismatch")
			} else {
				log.Printf("deployment RPC unavailable: %v", err)
			}
			continue
		}
		sources = append(sources, source)
		profile.AssetDecimals, err = source.VerifyAssetDecimals(handshake)
		cancel()
		if err != nil {
			if errors.Is(err, chainview.ErrDeploymentIdentity) {
				observe(profile, "identity_mismatch")
			} else {
				log.Printf("deployment token identity unavailable: %v", err)
			}
			continue
		}
		if requireApproval {
			err = db.RegisterSigned(ctx, profile, registered, key, signature)
		} else {
			err = db.Register(ctx, profile)
		}
		if err != nil {
			if !errors.Is(err, chainview.ErrProfileApproval) {
				log.Printf("deployment registration unavailable: %v", err)
			}
			continue
		}
		readers = append(readers, &chainview.Reader{Profile: profile, Source: source, DB: db})
	}
	return readers, closeAll, nil
}

func runChainview(gate *service.RecoveryGate) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var readers []*chainview.Reader
	var closeAll func()
	if err := gate.Run(ctx, func(ctx context.Context) error {
		var err error
		readers, closeAll, err = chainviewReaders(ctx)
		return err
	}); err != nil {
		return err
	}
	defer closeAll()
	if len(readers) == 0 {
		<-ctx.Done()
		return nil
	}
	interval, err := time.ParseDuration(env("P07_POLL", "2s"))
	if err != nil || interval <= 0 {
		return errors.New("invalid P07_POLL")
	}
	done := make(chan struct{}, len(readers))
	for _, reader := range readers {
		go func() {
			defer func() { done <- struct{}{} }()
			tick := time.NewTicker(interval)
			defer tick.Stop()
			for {
				poll, cancel := context.WithTimeout(ctx, 25*time.Second)
				err := gate.Run(poll, func(ctx context.Context) error {
					if err := reader.Poll(ctx); err != nil {
						return err
					}
					return reader.PollReplay(ctx)
				})
				cancel()
				if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, chainview.ErrLeaseUnavailable) && !errors.Is(err, service.ErrRecoveryFenced) {
					log.Printf("chainview generation %s: %v", reader.Profile.GenerationID, err)
				}
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
				}
			}
		}()
	}
	for range readers {
		<-done
	}
	return nil
}

func runPublicReceipts(gate *service.RecoveryGate) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	url := os.Getenv("P07_PUBLIC_DATABASE_URL")
	if url == "" {
		return errors.New("P07_PUBLIC_DATABASE_URL is required")
	}
	store, err := chainview.OpenPublic(ctx, url)
	if err != nil {
		return err
	}
	defer store.Close()
	if origin := os.Getenv("P07_LEGACY_OPERATIONS_ORIGIN"); origin != "" {
		ledger, err := weburl.Parse(origin)
		if err != nil || ledger.Scheme != "https" || ledger.Hostname() == "" || ledger.User != nil ||
			(ledger.Path != "" && ledger.Path != "/") || ledger.RawQuery != "" || ledger.Fragment != "" {
			return errors.New("P07_LEGACY_OPERATIONS_ORIGIN must be the trusted HTTPS operator origin")
		}
		ledger.Path = "/"
		ledger.RawQuery = "view=v1"
		store.LegacyLedgerURL = ledger.String()
	}

	server := &http.Server{Addr: env("P07_PUBLIC_HTTP_ADDR", "127.0.0.1:8081"),
		Handler: gate.Handler(store.PublicHandler()), ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runProveAbsence(gate *service.RecoveryGate) error {
	if len(os.Args) != 6 {
		return errors.New("usage: indexer prove-absence <generation> <merchant> <order> <signed-expiry-unix-seconds>")
	}
	expiry, err := strconv.ParseInt(os.Args[5], 10, 64)
	if err != nil || expiry <= 0 {
		return errors.New("invalid signed expiry")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	return gate.Run(ctx, func(ctx context.Context) error {
		return proveAbsence(ctx, expiry)
	})
}

func proveAbsence(ctx context.Context, expiry int64) error {
	readers, closeAll, err := chainviewReaders(ctx)
	if err != nil {
		return err
	}
	defer closeAll()
	for _, reader := range readers {
		if reader.Profile.GenerationID != os.Args[2] {
			continue
		}
		evidence, err := reader.ProveAbsence(ctx, os.Args[3], os.Args[4], time.Unix(expiry, 0))
		if err != nil {
			return err
		}
		fmt.Println(evidence)
		return nil
	}
	return errors.New("generation is outside the configured deployment manifest")
}
