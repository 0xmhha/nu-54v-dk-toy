package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p08-market-services/internal/market"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5/pgxpool"
)

func readProfile(path string) (market.Profile, error) {
	file, err := os.Open(path)
	if err != nil {
		return market.Profile{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 16_384))
	decoder.DisallowUnknownFields()
	var profile market.Profile
	if err = decoder.Decode(&profile); err != nil || decoder.Decode(new(any)) != io.EOF {
		return market.Profile{}, errors.New("invalid P08 deployment profile")
	}
	return profile, nil
}

func deploy(ctx context.Context) error {
	key, err := crypto.HexToECDSA(os.Getenv("P08_ADMIN_KEY"))
	if err != nil {
		return errors.New("valid P08_ADMIN_KEY required")
	}
	var roles [4]common.Address
	for i, name := range []string{"P08_QUOTE_SIGNER", "P08_PUBLISHER_A", "P08_PUBLISHER_B", "P08_KEEPER"} {
		value := os.Getenv(name)
		if !common.IsHexAddress(value) {
			return fmt.Errorf("%s must be an address", name)
		}
		roles[i] = common.HexToAddress(value)
	}
	artifact := os.Getenv("P08_ARTIFACT")
	if artifact == "" {
		artifact = "out/Market.sol/Market.json"
	}
	deadline, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	profile, err := market.Deploy(deadline, os.Getenv("P08_RPC"), artifact, key, roles)
	if err != nil {
		return err
	}
	decoder := json.NewEncoder(os.Stdout)
	decoder.SetIndent("", "  ")
	return decoder.Encode(profile)
}

func run(ctx context.Context, mode string) error {
	if mode == "deploy" {
		return deploy(ctx)
	}
	profile, err := readProfile(os.Getenv("P08_PROFILE"))
	if err != nil {
		return fmt.Errorf("read market profile: %w", err)
	}
	dial, cancel := context.WithTimeout(ctx, 20*time.Second)
	chain, err := market.Dial(dial, os.Getenv("P08_RPC"), profile)
	cancel()
	if err != nil {
		return err
	}
	defer chain.RPC.Close()
	switch mode {
	case "mobile-profile":
		return writeMobileProfile(chain.Profile)
	case "serve":
		signing, err := crypto.HexToECDSA(os.Getenv("P08_QUOTE_KEY"))
		if err != nil {
			return errors.New("valid P08_QUOTE_KEY required")
		}
		address := os.Getenv("P08_HTTP_ADDR")
		if address == "" {
			address = "127.0.0.1:8284"
		}
		return serveLoopback(ctx, address, (market.Handler{Chain: chain, Signer: signing}).Routes(), nil)
	case "ops":
		for _, key := range []string{"P08_INSPECT_DATABASE_URL", "P08_OPS_CA", "P08_OPS_CERT", "P08_OPS_KEY", "P08_OPS_GRANTS"} {
			if os.Getenv(key) == "" {
				return fmt.Errorf("%s is required", key)
			}
		}
		ca, err := os.ReadFile(os.Getenv("P08_OPS_CA"))
		trust := x509.NewCertPool()
		if err != nil || !trust.AppendCertsFromPEM(ca) {
			return errors.New("P08 operations CA unavailable")
		}
		grants := service.ClientGrantFile[market.OperatorGrant](os.Getenv("P08_OPS_GRANTS"))
		if _, err := grants.Read(); err != nil {
			return errors.New("invalid P08 operations grants")
		}
		cert, err := tls.LoadX509KeyPair(os.Getenv("P08_OPS_CERT"), os.Getenv("P08_OPS_KEY"))
		if err != nil {
			return errors.New("invalid P08 operations certificate")
		}
		db, err := market.OpenInspector(ctx, os.Getenv("P08_INSPECT_DATABASE_URL"))
		if err != nil {
			return err
		}
		defer db.Close()
		address := os.Getenv("P08_OPS_HTTP_ADDR")
		if address == "" {
			address = "127.0.0.1:8286"
		}
		config := &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust, Certificates: []tls.Certificate{cert}}
		return serveLoopback(ctx, address, market.OperatorMTLS(chain, db, grants.Lookup), config)

	case "keeper", "keeper-once", "inspect":
		databaseVariable := "P08_DATABASE_URL"
		if mode == "inspect" {
			databaseVariable = "P08_INSPECT_DATABASE_URL"
		}
		dsn := os.Getenv(databaseVariable)
		if dsn == "" {
			return fmt.Errorf("%s is required", databaseVariable)
		}
		var pool *pgxpool.Pool
		if mode == "inspect" {
			pool, err = market.OpenInspector(ctx, dsn)
		} else {
			pool, err = pgxpool.New(ctx, dsn)
		}
		if err != nil {
			return errors.New("could not configure P08 database")
		}
		defer pool.Close()
		if mode == "inspect" {
			rows, err := market.WorkHistory(ctx, pool, profile)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(rows)
		}
		key, err := crypto.HexToECDSA(os.Getenv("P08_KEEPER_KEY"))
		if err != nil {
			return errors.New("valid P08_KEEPER_KEY required")
		}
		keeper := market.Keeper{Chain: chain, DB: pool, Key: key}
		if mode == "keeper-once" {
			return keeper.Tick(ctx, nil)
		}
		for {
			tick, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := keeper.Tick(tick, nil)
			cancel()
			if err != nil && ctx.Err() == nil {
				log.Printf("market keeper: %v", err)
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
		}
	default:
		return errors.New("expected deploy, mobile-profile, serve, keeper, keeper-once, inspect, or ops")
	}
}

func serveLoopback(ctx context.Context, address string, handler http.Handler, config *tls.Config) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil || (host != "127.0.0.1" && host != "::1") {
		return errors.New("P08 HTTP requires a loopback listener")
	}
	server := &http.Server{
		Addr: address, Handler: handler, TLSConfig: config,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(stop)
	}()
	if config != nil {
		err = server.ListenAndServeTLS("", "")
	} else {
		err = server.ListenAndServe()
	}
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("expected deploy, mobile-profile, serve, keeper, keeper-once, inspect, or ops")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1]); err != nil {
		log.Fatal(err)
	}
}
