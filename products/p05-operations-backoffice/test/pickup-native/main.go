// A loopback mTLS wrapper around the production commerce routes for native QA.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
)

func main() {
	port := flag.String("port", "", "ephemeral fixture loopback port")
	cert := flag.String("cert", "", "disposable fixture server certificate")
	key := flag.String("key", "", "disposable fixture server key")
	ca := flag.String("client-ca", "", "disposable fixture client authority")
	grants := flag.String("grants", "", "disposable native QA grants")
	dropHold := flag.String("drop-hold-marker", "", "fixture marker for losing one committed booking response")
	dropPickup := flag.String("drop-pickup-marker", "", "fixture marker for losing one committed pickup response")
	pickupTrace := flag.String("pickup-trace", "", "fixture trace of pickup change requests")
	customerOnly := flag.Bool("customer-only", false, "require just the pickup customer for the changed terms test")
	flag.Parse()
	roots := x509.NewCertPool()
	root, err := os.ReadFile(*ca)
	if err != nil || !roots.AppendCertsFromPEM(root) {
		log.Fatal("missing fixture authority")
	}
	encoded, err := os.ReadFile(*grants)
	var actors map[string]service.Principal
	if err != nil || json.Unmarshal(encoded, &actors) != nil {
		log.Fatal("invalid fixture enrollments")
	}
	if *customerOnly {
		if len(actors) != 1 {
			log.Fatal("exactly one fixture customer required")
		}
		for _, actor := range actors {
			if actor.Role != "customer" {
				log.Fatal("fixture enrollment must be a customer")
			}
		}
	} else if len(actors) != 2 {
		log.Fatal("exactly two fixture enrollments required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	store, err := commerce.Open(ctx, os.Getenv("P05_PICKUP_NATIVE_DATABASE_URL"))
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	mux := http.NewServeMux()
	commerce.HTTP{Store: store, Credentials: actors}.Register(mux)
	server := http.Server{Addr: "127.0.0.1:" + *port, Handler: dropTravelReply(mux, *dropHold, *dropPickup, *pickupTrace), ReadTimeout: 5 * time.Second, WriteTimeout: 8 * time.Second,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}}
	log.Fatal(server.ListenAndServeTLS(*cert, *key))
}

// dropTravelReply is confined to the loopback fixture. A response is
// dropped only after the real handler successfully commits the command.
func dropTravelReply(next http.Handler, holdMarker, pickupMarker, trace string) http.Handler {
	if holdMarker == "" && pickupMarker == "" && trace == "" {
		return next
	}
	var lock sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/commerce" || r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
		if err != nil {
			http.Error(w, "invalid fixture request", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var command struct{ Travel struct{ Operation string } }
		if json.Unmarshal(body, &command) != nil {
			next.ServeHTTP(w, r)
			return
		}
		var marker string
		switch command.Travel.Operation {
		case "hold":
			marker = holdMarker
		case "pickup-change":
			marker = pickupMarker
		default:
			next.ServeHTTP(w, r)
			return
		}
		lock.Lock()
		defer lock.Unlock()
		if command.Travel.Operation == "pickup-change" && trace != "" {
			file, err := os.OpenFile(trace, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
			if err != nil {
				http.Error(w, "fixture trace unavailable", http.StatusInternalServerError)
				return
			}
			_, err = file.Write(append(body, '\n'))
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				http.Error(w, "fixture trace unavailable", http.StatusInternalServerError)
				return
			}
		}
		if _, err := os.Stat(marker); err != nil {
			next.ServeHTTP(w, r)
			return
		}
		captured := httptest.NewRecorder()
		next.ServeHTTP(captured, r)
		if captured.Code == http.StatusOK && os.Remove(marker) == nil {
			panic(http.ErrAbortHandler)
		}
		for name, values := range captured.Header() {
			w.Header()[name] = values
		}
		w.WriteHeader(captured.Code)
		_, _ = captured.Body.WriteTo(w)
	})
}
