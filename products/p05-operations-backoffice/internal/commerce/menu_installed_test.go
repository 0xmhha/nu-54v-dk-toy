package commerce

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
)

func menuFixtureServerCert(t *testing.T, ca *x509.Certificate, issuer any) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(40), NotBefore: ca.NotBefore, NotAfter: ca.NotAfter,
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:    x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, issuer)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der, ca.Raw}, PrivateKey: key}
}

type installedMenuFixture struct {
	admin                 *pgx.Conn
	server                *httptest.Server
	terminal              *http.Client
	credentials           map[string]service.Principal
	ca                    *x509.Certificate
	issuer                any
	branch, kiosk, runner string
	moveHot               func(int)
	signedMenu            *installedSignedMenu
	ownerBoard            *installedMenuBoard
	expireOriginal        func(context.Context, string) (bool, error)
	readyOriginal         func(context.Context, string) (map[string]string, error)
	finalizeOriginal      func(context.Context, string) (string, error)
	finance               *http.Client
	financeOrigin         string
	financeCertificate    tls.Certificate
}

func (f installedMenuFixture) run(t *testing.T, ctx context.Context) {
	t.Helper()
	const buyer = "92858326-61de-4311-bbb6-a2312e9489fb"
	if _, err := f.admin.Exec(ctx, `INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until)
		VALUES($1,$2,'customer',now()+interval '7 minutes')`, f.branch, buyer); err != nil {
		t.Fatal(err)
	}
	if f.signedMenu != nil {
		payer := k10FixturePayer
		if f.ownerBoard != nil {
			payer = crypto.PubkeyToAddress(f.ownerBoard.payer.PublicKey)
		}
		if _, err := f.admin.Exec(ctx, `INSERT INTO ledger.payer_enrollment
			VALUES($1,$2,$3,1,1,now()+interval '7 minutes')`, f.branch, buyer, payer.Bytes()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.admin.Exec(ctx, "UPDATE ledger.stock SET available=available+2 WHERE branch_id=$1 AND sku='tea'", f.branch); err != nil {
		t.Fatal(err)
	}
	f.moveHot(1)
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	token := hex.EncodeToString(secret)
	var armed, dropped, replenished, submitArmed, submitDropped atomic.Bool
	var controlURL string
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.NotFound(w, r)
			return
		}
		jsonReply := func(value any) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(value)
		}
		if f.ownerBoard != nil && strings.HasPrefix(r.URL.Path, "/owner/") {
			f.ownerBoard.serve(w, r, f.signedMenu)
			return
		}
		switch r.URL.Path {
		case "/arm-submit-loss", "/finalize-original", "/ready-original", "/expire-original":
			if f.finalizeOriginal == nil || f.signedMenu == nil || f.ownerBoard == nil {
				http.NotFound(w, r)
				return
			}
			raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 36))
			f.signedMenu.mu.Lock()
			sale, recovered := f.signedMenu.initial.SaleRef, f.signedMenu.retrieved
			f.signedMenu.mu.Unlock()
			if err != nil || string(raw) != sale || !recovered {
				http.Error(w, "expected the recovered original sale", http.StatusConflict)
				return
			}
			if r.URL.Path == "/arm-submit-loss" {
				f.ownerBoard.mu.Lock()
				approvedByBoard := f.ownerBoard.decision == 1
				f.ownerBoard.mu.Unlock()
				var approvals, paid, relayed int
				err := f.admin.QueryRow(r.Context(), `SELECT
					(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1),
					(SELECT count(*) FROM ledger.payment WHERE sale_id=$1),
					`+countOrderRelayAttempts+` FROM ledger.sale s WHERE s.id=$1`, sale).Scan(&approvals, &paid, &relayed)
				if err != nil || !approvedByBoard || approvals != 0 || paid != 0 || relayed != 0 ||
					!submitArmed.CompareAndSwap(false, true) {
					http.Error(w, "expected zero settlement effects before explicit submit", http.StatusConflict)
					return
				}
				jsonReply(map[string]string{})
				return
			}
			if !submitDropped.Load() {
				t.Log("installed W10 settlement phase: original admission reply did not reach the loss point")
				http.Error(w, "expected the original durable submission response loss", http.StatusConflict)
				return
			}
			if r.URL.Path == "/expire-original" {
				if f.expireOriginal == nil {
					http.NotFound(w, r)
					return
				}
				closed, err := f.expireOriginal(r.Context(), sale)
				if err != nil {
					t.Logf("W11 original expiry phase failed: %v", err)
					http.Error(w, "original quote evidence unavailable", http.StatusConflict)
					return
				}
				jsonReply(map[string]string{"closed": strconv.FormatBool(closed)})
				return
			}
			if r.URL.Path == "/ready-original" {
				if f.readyOriginal == nil {
					http.NotFound(w, r)
					return
				}
				ready, err := f.readyOriginal(r.Context(), sale)
				if err != nil {
					t.Logf("W11 pickup setup phase failed: %v", err)
					http.Error(w, "original pickup unavailable", http.StatusConflict)
					return
				}
				jsonReply(ready)
				return
			}
			hash, err := f.finalizeOriginal(r.Context(), sale)
			if err != nil || !validInstalledTransactionHash(hash) {
				t.Log("installed W10 settlement phase: local relay or confirmation failed")
				http.Error(w, "original settlement unavailable", http.StatusConflict)
				return
			}
			var confirmed bool
			err = f.admin.QueryRow(r.Context(), `SELECT EXISTS (
				SELECT FROM ledger.payment p JOIN ledger.sale s ON s.id=p.sale_id
				WHERE s.id=$1 AND s.state='paid' AND p.tx_hash=$2)`, sale, hash).Scan(&confirmed)
			if err != nil || !confirmed {
				t.Log("installed W10 settlement phase: original ledger transaction unavailable")
				http.Error(w, "original transaction did not settle", http.StatusConflict)
				return
			}
			if !f.verifyFinanceOriginal(r.Context(), sale, hash) {
				t.Log("installed W10 settlement phase: finance confirmation not available")
				http.Error(w, "finance order differs from the original settlement", http.StatusConflict)
				return
			}
			jsonReply(map[string]string{"hash": hash})
		case "/enroll":
			bytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 160))
			der, decode := base64.StdEncoding.DecodeString(string(bytes))
			public, parse := x509.ParsePKIXPublicKey(der)
			key, ok := public.(*ecdsa.PublicKey)
			if err != nil || decode != nil || parse != nil || !ok || key.Curve != elliptic.P256() {
				http.Error(w, "invalid enrollment key", http.StatusBadRequest)
				return
			}
			expiry := time.Now().Add(5 * time.Minute)
			leaf := &x509.Certificate{SerialNumber: big.NewInt(41), NotBefore: time.Now().Add(-time.Minute), NotAfter: expiry,
				KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
			clientCert, err := x509.CreateCertificate(rand.Reader, leaf, f.ca, key, f.issuer)
			serverCert, certErr := x509.ParseCertificate(f.server.TLS.Certificates[0].Certificate[0])
			if err != nil || certErr != nil {
				http.Error(w, "fixture certificates", http.StatusInternalServerError)
				return
			}
			fingerprint := sha256.Sum256(clientCert)
			f.credentials[hex.EncodeToString(fingerprint[:])] = service.Principal{Tenant: tenant, Subject: buyer, Role: "customer", ExpiresAt: expiry.Unix()}
			pin := sha256.Sum256(serverCert.RawSubjectPublicKeyInfo)
			pinText := hex.EncodeToString(pin[:])
			nearby := map[string]any{"control": controlURL, "token": token, "branch": f.branch, "tenant": tenant,
				"terminal": f.kiosk, "customer": buyer, "expiresAt": expiry.Unix()}
			profile := map[string]any{"endpoint": f.server.URL + "/v2/commerce", "pin": pinText,
				"ca": base64.StdEncoding.EncodeToString(f.ca.Raw), "certificates": []string{base64.StdEncoding.EncodeToString(clientCert)},
				"scope":  map[string]any{"role": "customer", "tenantRef": tenant, "branchRef": f.branch, "subjectRef": buyer, "expiresAt": expiry.Unix()},
				"nearby": nearby}
			response := map[string]string{"pin": pinText}
			if f.signedMenu != nil {
				profile["payment"], response["presentationPin"] = f.signedMenu.profileDocument()
				response["merchant"] = hex.EncodeToString(f.signedMenu.profile.MerchantID[:])
				response["recipient"] = hexAddress(f.signedMenu.profile.Beneficiary)
				nearby["signedReview"] = true
				if f.ownerBoard != nil {
					nearby["hostBoard"] = true
					response["relay"] = hexAddress(f.signedMenu.gasAccount)
				}
			}
			packed, _ := json.Marshal(profile)
			response["registration"] = string(packed)
			jsonReply(response)
		case "/arm":
			armed.Store(true)
			jsonReply(map[string]string{})
		case "/restock":
			if !dropped.Load() || !replenished.CompareAndSwap(false, true) {
				http.Error(w, "no original denial", http.StatusConflict)
				return
			}
			f.moveHot(1)
			jsonReply(map[string]string{})
		case "/relay":
			raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
			var envelope struct{ Command service.Request }
			if err != nil || json.Unmarshal(raw, &envelope) != nil {
				http.Error(w, "invalid relay request", http.StatusBadRequest)
				return
			}
			op := envelope.Command.Operation
			if f.signedMenu != nil && op == "payment.forward" {
				f.signedMenu.forwards.Add(1)
			}
			lost := (f.signedMenu == nil && op == "order.create" ||
				f.signedMenu != nil && op == "quote.issue") && armed.CompareAndSwap(true, false)
			if lost && f.signedMenu == nil {
				f.moveHot(-1)
			}
			outbound, err := http.NewRequestWithContext(r.Context(), http.MethodPost, f.server.URL+"/v2/commerce", bytes.NewReader(raw))
			if err != nil {
				http.Error(w, "invalid commerce endpoint", http.StatusBadRequest)
				return
			}
			reply, err := f.terminal.Do(outbound)
			if err != nil {
				http.Error(w, "commerce unavailable", http.StatusServiceUnavailable)
				return
			}
			defer reply.Body.Close()
			data, err := io.ReadAll(io.LimitReader(reply.Body, 32768))
			if err != nil {
				http.Error(w, "commerce reply unreadable", http.StatusServiceUnavailable)
				return
			}
			if f.finalizeOriginal != nil && op == "payment.forward" {
				t.Logf("installed W10 settlement phase: P05 terminal submission HTTP %d", reply.StatusCode)
			}
			if f.finalizeOriginal != nil && op == "payment.forward" && reply.StatusCode == http.StatusOK && submitArmed.Load() {
				f.signedMenu.mu.Lock()
				original := f.signedMenu.initial.SaleRef
				f.signedMenu.mu.Unlock()
				var accepted result
				if envelope.Command.Object != original || json.Unmarshal(data, &accepted) != nil || accepted.ResultRef != original {
					http.Error(w, "P05 accepted a different submission", http.StatusConflict)
					return
				}
				if submitDropped.CompareAndSwap(false, true) {
					http.Error(w, "isolated submit response loss", http.StatusServiceUnavailable)
					return
				}
			}
			if f.signedMenu != nil && op == "quote.issue" {
				var issued result
				if reply.StatusCode != http.StatusOK || json.Unmarshal(data, &issued) != nil ||
					!f.signedMenu.capture(issued.Quote, lost, buyer) {
					http.Error(w, "signed quote missing or changed", http.StatusServiceUnavailable)
					return
				}
				if lost {
					dropped.Store(true)
				}
			}
			if lost {
				var denial result
				if f.signedMenu == nil && reply.StatusCode == 409 && json.Unmarshal(data, &denial) == nil && denial.Error == service.OutOfStock &&
					len(denial.Unavailable) == 1 && denial.Unavailable[0] == (Item{"tea", "hot", 1}) {
					dropped.Store(true)
				}
				select {
				case <-time.After(2300 * time.Millisecond):
				case <-r.Context().Done():
					return
				}
				http.Error(w, "isolated response loss", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer bridge.Close()
	controlURL = bridge.URL
	if !strings.HasPrefix(controlURL, "http://127.0.0.1:") {
		t.Fatal("fixture must use loopback")
	}
	child := exec.CommandContext(ctx, f.runner, controlURL)
	child.Env = append(os.Environ(), "NU54_K10_TOKEN="+token)
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	child.Cancel = func() error { return child.Process.Signal(syscall.SIGTERM) }
	child.WaitDelay = 10 * time.Second
	if err := child.Run(); err != nil {
		if f.ownerBoard != nil {
			f.ownerBoard.mu.Lock()
			t.Logf("isolated W10 owner setup stage: %q", f.ownerBoard.stage)
			f.ownerBoard.mu.Unlock()
		}
		t.Fatalf("installed K10 test: %v", err)
	}
	if f.signedMenu != nil {
		if !dropped.Load() {
			t.Fatal("the committed quote response was not lost")
		}
		settled := f.finalizeOriginal != nil
		if settled && !submitDropped.Load() {
			t.Fatal("the original settlement submission reply did not get lost")
		}
		state := "issued"
		if f.expireOriginal != nil {
			state = "closed"
		} else if settled {
			state = "paid"
		}
		f.signedMenu.assertLedger(t, ctx, f.admin, f.branch, state)
		if state == "closed" {
			if !f.verifyFinanceOriginal(ctx, f.signedMenu.initial.SaleRef, "") {
				t.Fatal("finance did not reopen the exact closed original without funds or fulfillment")
			}
			t.Log("installed W11 original unpaid: one admission and zero payment, relay, journal, and fulfillment")
			return
		}
		if settled {
			f.assertFinanceBrowser(t, ctx, f.signedMenu.initial.SaleRef)
			return
		}
		if f.ownerBoard != nil {
			f.ownerBoard.mu.Lock()
			defer f.ownerBoard.mu.Unlock()
			refusal, err := os.ReadFile(filepath.Join(f.ownerBoard.dir, "payment-refusal-1"))
			if err != nil || len(refusal) != 56 || f.ownerBoard.decision != 2 {
				t.Fatal("the C host did not durably record the exact review refusal")
			}
			var approvals int
			err = f.admin.QueryRow(ctx, "SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1", f.signedMenu.initial.SaleRef).Scan(&approvals)
			if err != nil || approvals != 0 {
				t.Fatalf("refused review created approvals=%d: %v", approvals, err)
			}
			t.Log("installed W10 SW2: one C host refusal, approvals=0 payments=0 relay=0")
		}
		return
	}
	var denials, sales, keys, quoteWrites int
	err := f.admin.QueryRow(ctx, `SELECT count(*) FILTER (WHERE rejection='OUT_OF_STOCK'),
		count(*) FILTER (WHERE sale_id IS NOT NULL),count(DISTINCT request_key),count(q.sale_id)
		FROM ledger.menu_reservation_outcome o LEFT JOIN ledger.quote_work q USING(sale_id)
		WHERE o.branch_id=$1 AND o.nearby_customer=$2`, f.branch, buyer).Scan(&denials, &sales, &keys, &quoteWrites)
	if err != nil || !dropped.Load() || !replenished.Load() || denials != 1 || sales != 1 || keys != 2 || quoteWrites != 0 {
		t.Fatalf("installed ledger proof denials=%d sales=%d keys=%d quotes=%d err=%v", denials, sales, keys, quoteWrites, err)
	}
	t.Logf("installed simulated C2 ledger proof: denials=%d fresh sales=%d distinct keys=%d quotes=%d", denials, sales, keys, quoteWrites)
}

func validInstalledTransactionHash(hash string) bool {
	digest, err := hex.DecodeString(strings.TrimPrefix(hash, "0x"))
	return err == nil && len(digest) == 32 && hash == "0x"+hex.EncodeToString(digest)
}

func (f installedMenuFixture) verifyFinanceOriginal(ctx context.Context, sale, hash string) bool {
	if f.finance == nil {
		return false
	}
	raw, _ := json.Marshal(map[string]string{"saleId": sale, "branchId": f.branch})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.server.URL+"/v2/finance/order", bytes.NewReader(raw))
	if err != nil {
		return false
	}
	req.Header.Set("Origin", f.financeOrigin)
	req.Header.Set("Content-Type", "application/json")
	reply, err := f.finance.Do(req)
	if err != nil {
		return false
	}
	defer reply.Body.Close()
	var view struct {
		SaleID, Transaction, FundsStatus, FulfillmentStatus, Amount string
		ObservedHeight, ConfirmedHeight                             *int64
		Events                                                      []json.RawMessage
	}
	err = json.NewDecoder(io.LimitReader(reply.Body, 32768)).Decode(&view)
	if err != nil || reply.StatusCode != http.StatusOK || view.SaleID != sale || view.Transaction != hash || view.Amount != "15000000" {
		return false
	}
	if hash == "" {
		return view.FundsStatus == "closed" && view.FulfillmentStatus == "closed" && len(view.Events) == 0 &&
			view.ObservedHeight == nil && view.ConfirmedHeight == nil
	}
	return view.FundsStatus == "confirmed" && view.FulfillmentStatus == "recorded" && view.Amount == "15000000" &&
		view.ObservedHeight != nil && view.ConfirmedHeight != nil && *view.ConfirmedHeight >= *view.ObservedHeight
}

func (f installedMenuFixture) assertFinanceBrowser(t *testing.T, ctx context.Context, sale string) {
	t.Helper()
	var quote, hash string
	if err := f.admin.QueryRow(ctx, `SELECT '0x'||encode(q.quote_id,'hex'),p.tx_hash
		FROM ledger.quote_work q JOIN ledger.payment p ON p.sale_id=q.sale_id WHERE q.sale_id=$1`, sale).Scan(&quote, &hash); err != nil {
		t.Fatal("finance browser requires the original quote and confirmed transaction")
	}
	key, err := x509.MarshalPKCS8PrivateKey(f.financeCertificate.PrivateKey)
	if err != nil || len(f.financeCertificate.Certificate) != 1 {
		t.Fatal("finance browser requires the enrolled reader certificate")
	}
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "reader.pem"), filepath.Join(dir, "reader-key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.financeCertificate.Certificate[0]}), 0600); err != nil {
		t.Fatal("finance reader certificate unavailable")
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600); err != nil {
		t.Fatal("finance reader key unavailable")
	}
	phase := "confirmed"
	if f.readyOriginal != nil {
		phase = "confirmed-delivered"
	}
	command := exec.CommandContext(ctx, "node", "../../test/finance/order-detail-browser.cjs",
		f.financeOrigin, f.branch, sale, certPath, keyPath, quote, phase, hash)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("installed W10 finance browser: %v: %s", err, out)
	}
	t.Log("installed W10 finance browser: same original order, quote and transaction with P07 confirmation")
}
