package commerce

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/crypto"
)

type InstalledW10RentalKit struct {
	Commissioning RentalReviewCommissioning
	Exchange      func(context.Context, []byte, time.Time, func([]byte) (bool, error)) ([]byte, error)
}

type InstalledW10Rental struct {
	Manifest []byte
	Escrow   RentalEscrowAuthority
	Kits     [2]InstalledW10RentalKit
	Finalize func(context.Context, string, func(string) error) (string, error)
	kit      atomic.Uint32
}

type installedRenterSession struct {
	request      string
	review       chan []byte
	switchIntent chan bool
	final        chan []byte
	switched     atomic.Bool
}

type installedRenterProof struct {
	count           atomic.Int32
	refusalForwards atomic.Int32
	refusalSale     atomic.Pointer[string]
	dropped         atomic.Bool
	arm             atomic.Bool
	sale            atomic.Pointer[string]
}

func (f *installedRenterProof) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !f.arm.Load() || r.URL.Path != "/v2/commerce" {
			next.ServeHTTP(w, r)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32768))
		if err != nil {
			http.Error(w, "invalid fixture C3 request", 400)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var envelope struct{ Command service.Request }
		sale := f.sale.Load()
		if json.Unmarshal(raw, &envelope) != nil || envelope.Command.Operation != "payment.forward" {
			next.ServeHTTP(w, r)
			return
		}
		if refusal := f.refusalSale.Load(); refusal != nil && envelope.Command.Object == *refusal {
			f.refusalForwards.Add(1)
		}
		if sale == nil || envelope.Command.Object != *sale {
			next.ServeHTTP(w, r)
			return
		}
		f.count.Add(1)
		captured := httptest.NewRecorder()
		next.ServeHTTP(captured, r)
		var receipt service.Reply
		if captured.Code == http.StatusOK && json.Unmarshal(captured.Body.Bytes(), &receipt) == nil &&
			receipt.ResultRef == *sale && f.dropped.CompareAndSwap(false, true) {
			http.Error(w, "fixture dropped the committed original reply", http.StatusServiceUnavailable)
			return
		}
		for key, values := range captured.Header() {
			w.Header()[key] = values
		}
		w.WriteHeader(captured.Code)
		_, _ = w.Write(captured.Body.Bytes())
	})
}

func (f installedMenuFixture) runRenter(t *testing.T, ctx context.Context, store *Store, settlement *InstalledW10Settlement, proof *installedRenterProof) {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	renter := settlement.Rental
	if renter == nil || renter.Finalize == nil || store == nil || proof == nil || f.signedMenu == nil {
		t.Fatal("installed renter fixture requires a complete local deployment")
	}
	execSQL := func(query string, args ...any) {
		t.Helper()
		if _, err := f.admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	buyer, device, manager := uuidForCatalogFixture(t, ctx, f.admin), uuidForCatalogFixture(t, ctx, f.admin), uuidForCatalogFixture(t, ctx, f.admin)
	payer := crypto.PubkeyToAddress(settlement.Payer.PublicKey)
	execSQL("INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until) VALUES($1,$2,'customer',now()+interval '7 minutes')", f.branch, buyer)
	execSQL("INSERT INTO ledger.payer_enrollment VALUES($1,$2,$3,1,2,now()+interval '7 minutes')", f.branch, buyer, payer.Bytes())
	execSQL("INSERT INTO ledger.login_subject VALUES($1,$2)", "installed_w10_"+strings.ReplaceAll(manager, "-", ""), manager)
	execSQL("INSERT INTO ledger.finance_grant VALUES($1,$2,'rental_manage',now()+interval '7 minutes')", f.branch, manager)
	execSQL(`INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,
		enrollment_digest,attested_until,attested_at) VALUES($1,$2,1,$3,$4,'installed-renter',
		sha256(convert_to($3,'UTF8')),now()+interval '7 minutes',now()-interval '1 minute')`,
		f.branch, device, hexAddress(payer), hexAddress(settlement.RelayAccount))
	var handoff string
	if err := f.admin.QueryRow(ctx, `SELECT ledger.offer_renter_handover($1,$2,$3,$4,1,$5,$6,now()+interval '6 minutes',$7)::text`,
		tenant, f.branch, manager, device, buyer, hexAddress(payer), renter.Manifest).Scan(&handoff); err != nil {
		t.Fatal(err)
	}
	execSQL("SELECT ledger.change_renter_return($1,$2,$3,'accept',$4)", tenant, buyer, handoff, hexAddress(payer))
	execSQL("UPDATE ledger.stock SET available=available+4 WHERE branch_id=$1 AND sku='tea'", f.branch)
	f.moveHot(2)
	var version int64
	if err := f.admin.QueryRow(ctx, "SELECT version FROM ledger.menu_release WHERE branch_id=$1 ORDER BY version DESC LIMIT 1", f.branch).Scan(&version); err != nil {
		t.Fatal(err)
	}
	sales := make([]string, 2)
	var kept [2]bool
	makeSale := func(index int) (string, error) {
		if sales[index] != "" {
			return sales[index], nil
		}
		key, origin := uuidForCatalogFixture(t, ctx, f.admin), uuidForCatalogFixture(t, ctx, f.admin)
		request := service.Request{Domain: f.branch, IdempotencyKey: key, Body: map[string]string{"asset": hexAddress(settlement.Profile.Asset)}}
		cart := Cart{Origin: "menu", OriginRef: origin, MenuVersion: version, Items: []Item{{SKU: "tea", Option: "hot", Qty: 1}, {SKU: "tea", Option: "cold", Qty: 1}}}
		sale, err := store.Reserve(ctx, service.Bound{Tenant: tenant, Subject: f.kiosk, Role: "terminal", Request: request}, cart)
		if err != nil {
			return "", err
		}
		claimKey := uuidForCatalogFixture(t, ctx, f.admin)
		claim := service.Request{Domain: f.branch, Object: sale, IdempotencyKey: claimKey, Body: map[string]string{"customerRef": buyer}}
		intent := sha256.Sum256([]byte("installed-renter-checkout/" + sale))
		_, err = store.AuthorizeCheckout(ctx, service.Bound{Tenant: tenant, Subject: f.kiosk, Role: "terminal", Request: claim,
			IntentDigest: hex.EncodeToString(intent[:])})
		if err == nil {
			sales[index] = sale
		}
		return sale, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	token := hex.EncodeToString(secret)
	var gate sync.Mutex
	var session *installedRenterSession
	var controlURL string
	var started atomic.Bool
	mine, pending, finalized := make(chan struct{}, 1), make(chan string, 1), make(chan string, 1)

	effects := func(ctx context.Context, sale string) (submissions, payments, relays int, err error) {
		err = f.admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1),
			(SELECT count(*) FROM ledger.payment WHERE sale_id=$1),`+countOrderRelayAttempts+` FROM ledger.sale s WHERE s.id=$1`, sale).
			Scan(&submissions, &payments, &relays)
		return
	}
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.NotFound(w, r)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
		if err != nil {
			http.Error(w, "invalid fixture input", 400)
			return
		}
		send := func(value any) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(value)
		}
		conflict := func() { http.Error(w, "fixture state unavailable", 409) }
		switch r.URL.Path {
		case "/enroll":
			der, err := base64.StdEncoding.DecodeString(string(raw))
			public, parseErr := x509.ParsePKIXPublicKey(der)
			publicKey, ok := public.(*ecdsa.PublicKey)
			if err != nil || parseErr != nil || !ok || publicKey.Curve != elliptic.P256() {
				conflict()
				return
			}
			expires := time.Now().Add(5 * time.Minute)
			cert, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{SerialNumber: big.NewInt(42),
				NotBefore: time.Now().Add(-time.Minute), NotAfter: expires, KeyUsage: x509.KeyUsageDigitalSignature,
				ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}, f.ca, publicKey, f.issuer)
			serverCert, serverErr := x509.ParseCertificate(f.server.TLS.Certificates[0].Certificate[0])
			if err != nil || serverErr != nil {
				conflict()
				return
			}
			fp, pin := sha256.Sum256(cert), sha256.Sum256(serverCert.RawSubjectPublicKeyInfo)
			f.credentials[hex.EncodeToString(fp[:])] = service.Principal{Tenant: tenant, Subject: buyer, Role: "customer", ExpiresAt: expires.Unix()}
			payment, presentationPin := f.signedMenu.profileDocument()
			profile := map[string]any{"endpoint": f.server.URL + "/v2/commerce", "pin": hex.EncodeToString(pin[:]),
				"ca": base64.StdEncoding.EncodeToString(f.ca.Raw), "certificates": []string{base64.StdEncoding.EncodeToString(cert)},
				"scope": map[string]any{"role": "customer", "tenantRef": tenant, "branchRef": f.branch, "subjectRef": buyer, "expiresAt": expires.Unix()},
				"nearby": map[string]any{"control": controlURL, "token": token, "branch": f.branch, "tenant": tenant, "terminal": f.kiosk, "customer": buyer,
					"expiresAt": expires.Unix(), "payment": payment}, "payment": payment}
			doc, _ := json.Marshal(profile)
			send(map[string]string{"registration": string(doc), "pin": hex.EncodeToString(pin[:]), "presentationPin": presentationPin, "branch": f.branch})
		case "/relay":
			request, err := http.NewRequestWithContext(r.Context(), "POST", f.server.URL+"/v2/commerce", bytes.NewReader(raw))
			if err != nil {
				conflict()
				return
			}
			var envelope struct{ Command service.Request }
			_ = json.Unmarshal(raw, &envelope)
			res, err := f.terminal.Do(request)
			if err != nil {
				t.Logf("installed renter relay %q transport failure: %v", envelope.Command.Operation, err)
				conflict()
				return
			}
			defer res.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(res.Body, 32768))
			if res.StatusCode != http.StatusOK {
				// The kiosk maps any relay failure to "pending", so keep the cause for diagnosis.
				t.Logf("installed renter relay %q status %d: %.300s", envelope.Command.Operation, res.StatusCode, body)
			}
			w.WriteHeader(res.StatusCode)
			_, _ = w.Write(body)
		case "/rental/original":
			index := 0
			if string(raw) == "W09" {
				index = 1
			} else if string(raw) != "QR" {
				conflict()
				return
			}
			gate.Lock()
			defer gate.Unlock()
			if index == 1 && !kept[0] {
				conflict()
				return
			}
			renter.kit.Store(uint32(index))
			session = nil
			sale, err := makeSale(index)
			if err != nil {
				conflict()
				return
			}
			if index == 1 {
				proof.refusalSale.Store(&sale)
			}
			send(map[string]string{"sale": sale})
		case "/rental/open":
			var payload struct{ Authority RentalReviewPresentation }
			if json.Unmarshal(raw, &payload) != nil || !payload.Authority.RentalReviewChallenge.valid() {
				conflict()
				return
			}
			gate.Lock()
			index := renter.kit.Load()
			kit := renter.Kits[index]
			if session != nil || sales[index] == "" || payload.Authority.BoardPin != hex.EncodeToString(kit.Commissioning.BoardPin[:]) {
				gate.Unlock()
				conflict()
				return
			}
			active := &installedRenterSession{request: payload.Authority.Request, review: make(chan []byte, 1),
				switchIntent: make(chan bool, 1), final: make(chan []byte, 1)}
			session = active
			gate.Unlock()
			expires := time.Unix(payload.Authority.ExpiresAt, 0)
			go func() {
				defer close(active.review)
				defer close(active.final)
				actual, err := kit.Exchange(ctx, raw, expires, func(review []byte) (bool, error) {
					active.review <- review
					select {
					case approve := <-active.switchIntent:
						return approve, nil
					case <-time.After(time.Until(expires)):
						return false, errors.New("fixture switch timed out")
					case <-ctx.Done():
						return false, ctx.Err()
					}
				})
				if err == nil {
					active.final <- actual
				}
			}()
			select {
			case review := <-active.review:
				if len(review) == 0 {
					conflict()
					return
				}
				send(map[string]string{"receipt": hex.EncodeToString(review)})
			case <-r.Context().Done():
			}
		case "/rental/sw1", "/rental/sw2":
			gate.Lock()
			defer gate.Unlock()
			index := renter.kit.Load()
			wantApprove := index == 0
			if session == nil || wantApprove != (r.URL.Path == "/rental/sw1") || !session.switched.CompareAndSwap(false, true) {
				conflict()
				return
			}
			active := session
			approve, paid, relayed, err := effects(r.Context(), sales[index])
			if err != nil || approve != 0 || paid != 0 || relayed != 0 {
				conflict()
				return
			}
			active.switchIntent <- wantApprove
			send(map[string]bool{"ok": true})
		case "/rental/decision":
			var req struct{ Request string }
			gate.Lock()
			active := session
			gate.Unlock()
			if json.Unmarshal(raw, &req) != nil || active == nil || req.Request != active.request {
				conflict()
				return
			}
			select {
			case decision := <-active.final:
				if len(decision) == 0 {
					conflict()
					return
				}
				send(map[string]string{"decision": hex.EncodeToString(decision)})
			case <-r.Context().Done():
			}
		case "/rental/arm":
			gate.Lock()
			defer gate.Unlock()
			if sales[0] == "" || string(raw) != sales[0] || session == nil || !session.switched.Load() {
				conflict()
				return
			}
			approve, paid, relayed, err := effects(r.Context(), sales[0])
			if err != nil || approve != 0 || paid != 0 || relayed != 0 || !proof.arm.CompareAndSwap(false, true) {
				conflict()
				return
			}
			original := sales[0]
			proof.sale.Store(&original)
			send(map[string]bool{"armed": true})
		case "/rental/broadcast":
			gate.Lock()
			original := sales[0]
			gate.Unlock()
			if original == "" || string(raw) != original || !proof.dropped.Load() || proof.count.Load() != 1 ||
				!started.CompareAndSwap(false, true) {
				conflict()
				return
			}
			go func() {
				defer close(pending)
				defer close(finalized)
				hash, err := renter.Finalize(ctx, original, func(hash string) error {
					pending <- hash
					select {
					case <-mine:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
				if err == nil && validInstalledTransactionHash(hash) {
					finalized <- hash
				}
			}()
			select {
			case hash := <-pending:
				if !validInstalledTransactionHash(hash) {
					conflict()
					return
				}
				send(map[string]string{"hash": hash})
			case <-r.Context().Done():
			}
		case "/rental/finalize":
			gate.Lock()
			original := sales[0]
			gate.Unlock()
			if string(raw) != original || !started.Load() {
				conflict()
				return
			}
			mine <- struct{}{}
			select {
			case hash := <-finalized:
				if !validInstalledTransactionHash(hash) {
					conflict()
					return
				}
				gate.Lock()
				kept[0] = true
				gate.Unlock()
				send(map[string]string{"hash": hash})
			case <-r.Context().Done():
			}
		case "/rental/refused":
			gate.Lock()
			defer gate.Unlock()
			if sales[1] == "" || string(raw) != sales[1] || session == nil || !session.switched.Load() {
				conflict()
				return
			}
			approve, paid, relayed, err := effects(r.Context(), sales[1])
			if err != nil || approve != 0 || paid != 0 || relayed != 0 {
				conflict()
				return
			}
			kept[1] = true
			send(map[string]bool{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer func() { cancel(); bridge.Close() }()
	controlURL = bridge.URL
	cmd := exec.CommandContext(ctx, f.runner, controlURL)
	cmd.Env = append(os.Environ(), "NU54_K10_TOKEN="+token, "NU54_W10_RENTER_INSTALLED=1")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatal("installed renter runner failed", err)
	}
	gate.Lock()
	defer gate.Unlock()
	if !kept[0] || !kept[1] || proof.count.Load() != 1 || proof.refusalForwards.Load() != 0 {
		t.Fatal("installed renter didn't prove both original decisions")
	}
	for index, sale := range sales {
		approve, paid, relayed, err := effects(ctx, sale)
		desired := 1 - index
		if err != nil || approve != desired || paid != desired || relayed != desired {
			t.Fatal(fmt.Errorf("installed renter effects: desired %d got %d/%d/%d: %w", desired, approve, paid, relayed, err))
		}
	}
	t.Log("installed W10 renter decisions: one SW1 admission, one paid original with original relay hash, zero SW2 effects")
}
