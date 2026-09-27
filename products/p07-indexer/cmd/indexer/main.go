// Command indexer runs the P07 minimal receipt indexer (WBS2-P07-01).
// This skeleton only serves the receipt API; ingestion of finalized
// PaymentSettled logs and the PostgreSQL store are added by the WBS task.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/receipt"
)

type emptyStore struct{}

func (emptyStore) Get(context.Context, string, string) (receipt.Receipt, error) {
	return receipt.Receipt{}, receipt.ErrNotFound
}

func main() {
	addr := os.Getenv("P07_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("p07 indexer skeleton listening on %s", addr)
	if err := http.ListenAndServe(addr, receipt.Handler(emptyStore{})); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
