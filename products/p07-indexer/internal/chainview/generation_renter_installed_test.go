package chainview_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
)

func (s *combinedGenerationOpsd) verifyInstalledRenter(t *testing.T, runner string, server tls.Certificate, ca *x509.Certificate, caKey crypto.Signer, boardApproval []byte, record func(*testing.T)) {
	t.Helper()
	binding, _, err := chainview.ParseGenerationProjection(s.input.projection)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(server.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	account := strings.ToLower(binding.Account.Hex())
	expiry := time.Unix(int64(s.input.t0)+155, 0).UTC()
	if installedW12Credential() {
		expiry = time.Unix(int64(s.input.t0)+190, 0).UTC()
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	token := hex.EncodeToString(secret)
	ctx, cancel := context.WithDeadline(s.ctx, expiry)
	defer cancel()
	type request struct {
		path string
		key  *ecdsa.PublicKey
		done chan map[string]string
	}
	requests := make(chan request)
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.NotFound(w, r)
			return
		}
		next := request{path: r.URL.Path, done: make(chan map[string]string, 1)}
		switch next.path {
		case "/enroll":
			encoded, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, 160))
			der, decodeErr := base64.StdEncoding.DecodeString(string(encoded))
			public, parseErr := x509.ParsePKIXPublicKey(der)
			key, ok := public.(*ecdsa.PublicKey)
			if readErr != nil || decodeErr != nil || parseErr != nil || !ok || key.Curve != elliptic.P256() {
				http.Error(w, "invalid enrollment key", http.StatusBadRequest)
				return
			}
			next.key = key
		case "/restart", "/revoke", "/observe", "/credential-observed":
		default:
			http.NotFound(w, r)
			return
		}
		select {
		case requests <- next:
		case <-ctx.Done():
			return
		case <-r.Context().Done():
			return
		}
		select {
		case reply := <-next.done:
			if reply == nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(reply)
		case <-ctx.Done():
			return
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(bridge.Close)
	program := exec.CommandContext(ctx, runner, bridge.URL)
	program.Env = append(os.Environ(), "NU54_W12_TOKEN="+token)
	if s.input.credential != nil {
		program.Env = append(program.Env, "NU54_W12_CREDENTIAL=1")
		if w12SlotOpenFixture() {
			program.Env = append(program.Env, "NU54_W12_SLOT_OPEN=1")
		}
	}
	if record != nil {
		program.Env = append(program.Env, "NU54_W12_LINKED_PAYMENTS=1")
	}
	program.Cancel = func() error { return program.Process.Signal(syscall.SIGTERM) }
	program.WaitDelay = 7 * time.Second
	program.Stdout, program.Stderr = os.Stdout, os.Stderr
	if err := program.Start(); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- program.Wait() }()
	finished := false
	defer func() {
		cancel()
		if !finished {
			<-result
		}
	}()
	var assignment string
	restarted, revoked, observed := false, false, false
	for {
		select {
		case err := <-result:
			finished = true
			if err != nil || assignment == "" || !restarted || !revoked || (record != nil && !observed) ||
				(s.input.credential != nil && (s.input.credential.issued.Load() == 0 || s.input.credential.refused.Load() == 0)) ||
				(w12SlotOpenFixture() && (s.input.credential == nil || s.input.credential.slotIssued.Load() == 0 || s.input.credential.slotRefused.Load() == 0)) {
				t.Fatalf("installed renter lifecycle incomplete: %v", err)
			}
			return
		case <-ctx.Done():
			t.Fatal("installed renter fixture expired")
		case next := <-requests:
			switch next.path {
			case "/enroll":
				if assignment != "" {
					next.done <- nil
					continue
				}
				template := x509.Certificate{SerialNumber: big.NewInt(101), Subject: pkix.Name{CommonName: "W12 installed customer"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: expiry,
					KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
				cert, err := x509.CreateCertificate(rand.Reader, &template, ca, next.key, caKey)
				if err != nil {
					t.Fatal(err)
				}
				fingerprint := sha256.Sum256(cert)
				s.setInstalledRenter(hex.EncodeToString(fingerprint[:]))
				s.restart(t)
				if s.confirmInstalledCredentialRegistration != nil {
					// The staged proof deadline remains within the signed N54G
					// admission window. The UI may already have used this time.
					if until := time.Until(time.Unix(int64(s.input.t0)+31, 0)); until > 0 {
						select {
						case <-time.After(until):
						case <-ctx.Done():
							t.Fatal(ctx.Err())
						}
					}
					s.confirmInstalledCredentialRegistration(t)
				}
				offer, _ := json.Marshal(map[string]any{"customer": combinedRenter, "generation": s.input.expected.Generation, "account": account, "assignmentUntil": expiry})
				path := "/v2/operations/renter-handover/" + binding.Branch + "/" + binding.Board
				raw := s.call(t, s.operator, http.MethodPost, path, "application/json", offer, http.StatusCreated)
				var issued struct{ Assignment, Account, Generation string }
				if json.Unmarshal(raw, &issued) != nil || issued.Assignment == "" || issued.Account != account || issued.Generation != s.input.expected.Generation {
					t.Fatal("operator did not issue a real assignment")
				}
				assignment = issued.Assignment
				pin := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
				registration, _ := json.Marshal(map[string]any{"endpoint": s.origin + "/v2/commerce", "pin": hex.EncodeToString(pin[:]),
					"ca": base64.StdEncoding.EncodeToString(ca.Raw), "certificates": []string{base64.StdEncoding.EncodeToString(cert)},
					"scope": map[string]any{"role": "customer", "tenantRef": "8744d55c-c471-48d4-bbbc-6e65872c8346", "branchRef": binding.Branch, "subjectRef": combinedRenter, "expiresAt": expiry.Unix()}})
				next.done <- map[string]string{"registration": string(registration), "pin": hex.EncodeToString(pin[:]), "assignment": assignment, "account": account, "generation": issued.Generation}
			case "/credential-observed":
				if s.input.credential == nil || s.input.credential.issued.Load() == 0 || assignment == "" || observed {
					next.done <- nil
					continue
				}
				term := s.input.credential.observed.Load()
				if term == nil {
					next.done <- nil
					continue
				}
				dates := map[string]string{"proof": "C", "credentialExpiresAt": term.CredentialExpiresAt,
					"credentialEvidenceUntil": term.CredentialEvidenceUntil}
				if w12SlotOpenFixture() {
					if s.input.credential.slotIssued.Load() == 0 || term.SlotCheckedAt == "" || term.SlotEvidenceUntil == "" {
						next.done <- nil
						continue
					}
					dates["slotCheckedAt"], dates["slotEvidenceUntil"] = term.SlotCheckedAt, term.SlotEvidenceUntil
				}
				next.done <- dates
			case "/observe":
				if record == nil || assignment == "" || observed {
					next.done <- nil
					continue
				}
				if s.input.credential != nil {
					if s.input.credential.issued.Load() == 0 {
						t.Fatal("fixture cannot freeze before serving a live credential")
					}
					s.input.credential.frozen.Store(true)
				}
				origin := s.prepareInstalledRenterOriginal(t, boardApproval, record)
				observed = true
				next.done <- map[string]string{"origin": origin}
			case "/restart":
				if assignment == "" || restarted {
					next.done <- nil
					continue
				}
				var accepted bool
				if err := s.db.QueryRow(s.ctx, "SELECT accepted_at IS NOT NULL FROM ledger.renter_return_assignment WHERE assignment_id=$1",
					assignment).Scan(&accepted); err != nil || !accepted {
					t.Fatal("installed client had not accepted the original assignment", err)
				}
				s.restart(t)
				restarted = true
				next.done <- map[string]string{}
			case "/revoke":
				if !restarted || revoked {
					next.done <- nil
					continue
				}
				s.call(t, s.operator, http.MethodPost, "/v2/operations/renter-handover/"+assignment+"/revoke", "", nil, http.StatusNoContent)
				revoked = true
				next.done <- map[string]string{}
			}
		}
	}
}

func (s *combinedGenerationOpsd) prepareInstalledRenterOriginal(t *testing.T, boardApproval []byte, record func(*testing.T)) string {
	t.Helper()
	order := s.v1FixtureOrder(t)
	payload, _ := json.Marshal(order)
	raw := s.call(t, s.v1Terminal, http.MethodPost, "/v2/private/renter-v1-orders/reserve", "application/json", payload, http.StatusOK)
	var origin struct{ Origin string }
	if json.Unmarshal(raw, &origin) != nil || origin.Origin == "" {
		t.Fatal("installed fixture could not reserve the original")
	}
	order["origin"], order["nonce"], order["approvalSignature"] = origin.Origin, "3", "0x"+hex.EncodeToString(boardApproval)
	payload, _ = json.Marshal(order)
	s.call(t, s.v1Terminal, http.MethodPost, "/v2/private/renter-v1-orders/confirm", "application/json", payload, http.StatusOK)
	record(t)
	return origin.Origin
}
