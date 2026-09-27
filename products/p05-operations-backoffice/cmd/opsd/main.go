// Command opsd is the P05 back-office API server ([N20]). It is designed in
// docs/content/products/p05/design.md §9 and built next cycle on internal/core.
package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	addr := os.Getenv("P05_HTTP_ADDR")
	if addr == "" {
		addr = ":8090"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	log.Printf("opsd skeleton listening on %s (API resources arrive next cycle)", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
