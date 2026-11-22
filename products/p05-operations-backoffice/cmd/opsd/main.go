// Command opsd is the P05 back-office API server ([N20]). It is designed in
// docs/content/products/p05/design.md §9. Commerce is separately enabled with mTLS and its database role.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	gate, err := service.OpenRecoveryGate("pgx", os.Getenv("P05_RECOVERY_DATABASE_URL"),
		os.Getenv("P05_RECOVERY_SCOPE"), os.Getenv("P05_RECOVERY_MANIFEST"))
	if err != nil {
		log.Fatal(err)
	}
	defer gate.Close()
	addr := os.Getenv("P05_HTTP_ADDR")
	if addr == "" {
		addr = ":8090"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	database := os.Getenv("P05_COMMERCE_DATABASE_URL")
	if database == "" && os.Getenv("P05_SUBSCRIPTION_WORKER_DATABASE_URL") != "" {
		log.Fatal("subscription worker requires P05_COMMERCE_DATABASE_URL")
	}
	if database == "" && (anySetting(rentalSettings) || anySetting(firmwareSettings) || anySetting(legacyConsoleSettings) ||
		os.Getenv("P05_RENTAL_AUTHORITY_READ_DATABASE_URL") != "" || os.Getenv("P05_RENTAL_PROTECTED_CLOSE_DATABASE_URL") != "" || os.Getenv("P05_RENTAL_RENTER_EVIDENCE") != "" || os.Getenv("P05_RENTAL_V1_ORDER_TERMINALS") != "" || os.Getenv("P05_RENTAL_PROTECTED_WITHDRAWAL") != "" || os.Getenv("P05_RENTAL_WITHDRAWAL_RECONCILER_DATABASE_URL") != "") {
		log.Fatal("fleet operations require the authenticated commerce listener")
	}
	if err := validateSettlementMode(database); err != nil {
		log.Fatal(err)
	}
	if database != "" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := serveCommerce(ctx, addr, database, mux, gate); err != nil {
			log.Fatal(err)
		}
		return
	}
	log.Printf("opsd health probe listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, gate.Handler(mux)))
}
