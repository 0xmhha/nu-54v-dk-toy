package settlement

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
)

type cafeNativeLab struct {
	mu     sync.Mutex
	marks  map[string]bool
	dir    string
	nearby *cafeNativeSelection
	ready  json.RawMessage
}

type cafeNativeSelection struct {
	publicSale, publicEvent string
	publicNumber            int64
	payment                 map[string]any
}

func newCafeNativeLab(t *testing.T, dir string, order seededOrder, staff string, store *commerce.Store, selection ...*cafeNativeSelection) *cafeNativeLab {
	t.Helper()
	lab := &cafeNativeLab{dir: dir, marks: make(map[string]bool)}
	if len(selection) > 0 {
		lab.nearby = selection[0]
	}
	serverCert := newNativeLoopbackTLS(t)
	credentials := make(map[string]service.Principal)
	roots := x509.NewCertPool()
	roles := map[string]service.Principal{
		"staff":    {Tenant: order.Tenant, Subject: staff, Role: "operator", ExpiresAt: time.Now().Add(15 * time.Minute).Unix()},
		"customer": {Tenant: order.Tenant, Subject: order.Actor, Role: "customer", ExpiresAt: time.Now().Add(15 * time.Minute).Unix()},
	}
	profiles := make(map[string]*nativeSubscriptionLab)
	for role, principal := range roles {
		caPEM, profile := enrollNativeDevices(t, filepath.Join(dir, role), serverCert, nil, credentials, principal)
		if !roots.AppendCertsFromPEM(caPEM) || len(profile.leaves) != 1 {
			t.Fatal("each native cafe role requires one local public key")
		}
		profiles[role] = profile
	}
	mux := http.NewServeMux()
	commerce.HTTP{Store: store, Credentials: credentials}.Register(mux)
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{serverCert}, ClientCAs: roots}
	server.StartTLS()
	t.Cleanup(server.Close)

	// UI testing rendezvous is loopback only and carries no business records.
	pathAllowed := map[string]bool{"checking": true, "held-seen": true, "buyer-held-seen": true, "accepted": true,
		"ready": true, "ready-seen": true, "collected": true, "collected-seen": true, "may-ready": true, "ready-data": true, "staff-done": true, "buyer-done": true}
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tag := r.URL.Path[1:]
		if !pathAllowed[tag] || (r.Method != "GET" && r.Method != "POST") {
			http.NotFound(w, r)
			return
		}
		lab.mu.Lock()
		defer lab.mu.Unlock()
		if tag == "ready-data" {
			if r.Method != "GET" || len(lab.ready) == 0 {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(lab.ready)
			return
		}
		if r.Method == "POST" {
			lab.marks[tag] = true
		}
		if lab.marks[tag] {
			_, _ = fmt.Fprint(w, "yes")
		}
	}))
	t.Cleanup(control.Close)
	leaf, err := x509.ParseCertificate(serverCert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	for role, profile := range profiles {
		principal := roles[role]
		doc := map[string]any{
			"endpoint": server.URL + "/v2/commerce", "pin": hex.EncodeToString(pin[:]),
			"ca":           base64.StdEncoding.EncodeToString(profile.anchor),
			"certificates": []string{base64.StdEncoding.EncodeToString(profile.leaves["ios"])},
			"scope": map[string]any{"branchRef": order.Branch, "tenantRef": order.Tenant,
				"subjectRef": principal.Subject, "role": principal.Role, "expiresAt": principal.ExpiresAt},
		}
		config := map[string]any{"profile": doc, "control": control.URL, "sale": order.Sale, "variant": "unknown", "staffPermissions": []string{"pickup", "print_receipts"}}
		if lab.nearby != nil {
			config["variant"] = "nearby"
			doc["payment"] = lab.nearby.payment
			config["publicSale"], config["publicEvent"], config["publicNumber"] =
				lab.nearby.publicSale, lab.nearby.publicEvent, lab.nearby.publicNumber
		}
		encoded, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(dir, role+".json")
		t.Cleanup(func() { _ = os.Remove(file) })
		if err := os.WriteFile(file+".tmp", encoded, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(file+".tmp", file); err != nil {
			t.Fatal(err)
		}
	}
	return lab
}

func (l *cafeNativeLab) mark(mark string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.marks[mark] = true
}

func (l *cafeNativeLab) wait(t *testing.T, ctx context.Context, mark string) {
	t.Helper()
	timer := time.NewTimer(180 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		l.mu.Lock()
		ready := l.marks[mark]
		l.mu.Unlock()
		if ready {
			return
		}
		if _, err := os.Stat(filepath.Join(l.dir, "aborted")); err == nil {
			t.Fatal("installed cafe fixture reported failure")
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-timer.C:
			t.Fatalf("installed cafe fixture never reached %s", mark)
		case <-ticker.C:
		}
	}
}
