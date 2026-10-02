// Command indexer runs the P07 minimal receipt indexer (WBS2-P07-01): an ingest loop that
// stores finalized PaymentSettled logs in PostgreSQL and a read-only receipt API ([N19]).
// The kiosk never waits for it: "paid" is decided on the chain ([N08]).
//
// Environment:
//
//	P07_DATABASE_URL  PostgreSQL URL (required)
//	P07_RPC           JSON-RPC URL (default the StableNet testnet)
//	P07_DEPLOYMENT    deployment record with the settlement address and block
//	                  (default products/p06-stablenet-contracts/deployments/8283.json)
//	P07_HTTP_ADDR     listen address (default :8080)
//	P07_POLL          poll period (default 2s)
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chain"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/ingest"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/receipt"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/store"
)

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// settlement reads the contract address and deployment block from a deployment record.
func settlement(path string) (common.Address, uint64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return common.Address{}, 0, err
	}
	var dep struct {
		Contracts struct {
			PaymentSettlement struct {
				Address string `json:"address"`
				Block   uint64 `json:"block"`
			} `json:"PaymentSettlement"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(raw, &dep); err != nil {
		return common.Address{}, 0, err
	}
	ps := dep.Contracts.PaymentSettlement
	if !common.IsHexAddress(ps.Address) || ps.Block == 0 {
		return common.Address{}, 0, fmt.Errorf("%s: no PaymentSettlement address and block", path)
	}
	return common.HexToAddress(ps.Address), ps.Block, nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dbURL := os.Getenv("P07_DATABASE_URL")
	if dbURL == "" {
		return errors.New("P07_DATABASE_URL is required")
	}
	poll, err := time.ParseDuration(env("P07_POLL", "2s"))
	if err != nil {
		return err
	}
	contract, block, err := settlement(env("P07_DEPLOYMENT", "products/p06-stablenet-contracts/deployments/8283.json"))
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer st.Close()
	rpc, err := chain.Dial(ctx, env("P07_RPC", "https://api.test.stablenet.network/"))
	if err != nil {
		return err
	}
	in := &ingest.Ingester{Chain: rpc, Sink: st, Contract: contract, Start: block - 1, Poll: poll}
	go in.Run(ctx)

	srv := &http.Server{Addr: env("P07_HTTP_ADDR", ":8080"), Handler: receipt.Handler(st, in), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("p07 indexer: settlement %s from block %d, listening on %s", receipt.Short(contract.Hex()), block, srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
