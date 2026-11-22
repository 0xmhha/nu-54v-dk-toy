package ceremony_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/marketlab"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/gateway"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
	"github.com/jackc/pgx/v5/pgxpool"
)

type missingReadiness struct {
	wallet.Participants
	since   time.Time
	key     []byte
	db      *pgxpool.Pool
	checked atomic.Bool
}

func (m *missingReadiness) filter(ctx context.Context, id, session string, r ceremony.Result, err error) (ceremony.Result, error) {
	if err != nil || id != "c3" || r.State != "ready" {
		return r, err
	}
	if m.since.IsZero() {
		var recovery bool
		if err := m.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wallet_mpc.operation WHERE operation_id=$1 AND kind='refresh')`, session).Scan(&recovery); err != nil || !recovery {
			return r, err
		}
		m.since = time.Now()
	}
	if time.Since(m.since) > 38*time.Second && m.checked.Load() {
		return r, nil
	}
	if !m.checked.Load() {
		var partial, blocked bool
		query := `SELECT EXISTS(SELECT 1 FROM wallet_mpc.operation o JOIN wallet_mpc.wallet w USING(wallet_id)
   WHERE o.operation_id=$1 AND o.kind='refresh' AND o.state='running' AND w.state='blocked' AND w.active_epoch=1
   AND (SELECT COUNT(*) FROM wallet_mpc.participant_ack a WHERE a.operation_id=o.operation_id AND a.evidence_kind='ready')=2),
   NOT EXISTS (SELECT 1 FROM wallet_mpc.operation o JOIN wallet_mpc.operation r USING(wallet_id) WHERE r.operation_id=$1 AND o.kind='sign' AND o.state='succeeded')`
		if check := m.db.QueryRow(ctx, query, session).Scan(&partial, &blocked); check != nil {
			return ceremony.Result{}, check
		}
		if partial && blocked {
			if err := m.db.QueryRow(ctx, `SELECT w.public_key FROM wallet_mpc.wallet w
				JOIN wallet_mpc.operation o USING(wallet_id) WHERE o.operation_id=$1`, session).Scan(&m.key); err != nil {
				return ceremony.Result{}, err
			}
			m.checked.Store(true)
		}
	}
	return ceremony.Result{}, errors.New("synthetic readiness response temporarily unavailable")
}
func (m *missingReadiness) Status(ctx context.Context, id, session string) (ceremony.Result, error) {
	r, err := m.Participants.Status(ctx, id, session)
	return m.filter(ctx, id, session, r, err)
}
func (m *missingReadiness) Start(ctx context.Context, id string, a ceremony.Authorization) (ceremony.Result, error) {
	r, err := m.Participants.Start(ctx, id, a)
	return m.filter(ctx, id, a.Session.ID, r, err)
}

type browserLab struct {
	out         string
	caDER       []byte
	root        *x509.Certificate
	key         *ecdsa.PrivateKey
	recovery    bool
	refusal     bool
	signRefusal bool
	partial     bool
	resume      bool
	operator    bool
	market      *marketlab.Policy
}

func newBrowserLab(t *testing.T) *browserLab {
	t.Helper()
	out := os.Getenv("NU54_P03_BROWSER_LAB")
	if out == "" {
		return nil
	}
	path, err := filepath.Abs(out)
	if err != nil || !filepath.IsAbs(out) || path == "/tmp" || !filepath.IsLocal(filepath.Base(path)) {
		t.Fatal("invalid browser fixture directory")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("browser fixture directory must be private")
	}
	if _, err := os.Stat(filepath.Join(path, "browser.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("fresh browser fixture directory required")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "NU54 P03 Browser Lab " + serial.Text(16)},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	var market *marketlab.Policy
	if os.Getenv("NU54_P03_BROWSER_MARKET") == "1" {
		if os.Getenv("NU54_P03_BROWSER_RECOVERY") == "1" {
			t.Fatal("choose one browser profile")
		}
		market = installedMarket(t)
		if market == nil {
			t.Fatal("market browser acceptance requires a local isolated deployment")
		}
	}
	return &browserLab{market: market, out: path, root: root, key: key, caDER: der, recovery: os.Getenv("NU54_P03_BROWSER_RECOVERY") == "1", refusal: os.Getenv("NU54_P03_BROWSER_REFUSAL") == "1", signRefusal: os.Getenv("NU54_P03_BROWSER_SIGN_REFUSAL") == "1", partial: os.Getenv("NU54_P03_BROWSER_PARTIAL") == "1", resume: os.Getenv("NU54_P03_BROWSER_RESUME") == "1", operator: os.Getenv("NU54_P03_OPERATOR_BROWSER") == "1"}
}

func (l *browserLab) certificate(t *testing.T) tls.Certificate {
	return l.certificateFor(t, x509.ExtKeyUsageServerAuth)
}

func (l *browserLab) certificateFor(t *testing.T, usage x509.ExtKeyUsage) tls.Certificate {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, NotBefore: now.Add(-time.Minute), NotAfter: l.root.NotAfter,
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
	der, err := x509.CreateCertificate(rand.Reader, template, l.root, &priv.PublicKey, l.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
}

type receiptLoss struct {
	http.Handler
	recovery atomic.Bool
	closure  atomic.Bool
}

func (l *receiptLoss) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var flag *atomic.Bool
	if r.Method == http.MethodPost {
		switch r.URL.Path {
		case "/v1/close":
			flag = &l.closure
		case "/v1/operations":
			raw, err := io.ReadAll(io.LimitReader(r.Body, 32769))
			if err != nil || len(raw) > 32768 {
				http.Error(w, "fixture body", 400)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
			var input struct{ Kind string }
			if json.Unmarshal(raw, &input) == nil && input.Kind == "refresh" {
				flag = &l.recovery
			}
		}
	}
	if flag == nil || flag.Load() {
		l.Handler.ServeHTTP(w, r)
		return
	}
	rec := httptest.NewRecorder()
	l.Handler.ServeHTTP(rec, r)
	if rec.Code == http.StatusOK && flag.CompareAndSwap(false, true) {
		http.Error(w, "synthetic reply lost after durable response", http.StatusServiceUnavailable)
		return
	}
	for k, values := range rec.Header() {
		for _, value := range values {
			w.Header().Add(k, value)
		}
	}
	w.WriteHeader(rec.Code)
	_, _ = w.Write(rec.Body.Bytes())
}

type closureReadiness struct {
	wallet.Participants
	pool  *pgxpool.Pool
	since time.Time
}

func (h *closureReadiness) Fence(ctx context.Context, peer, walletID string, epoch uint64) error {
	if peer == "c3" {
		var closing bool
		err := h.pool.QueryRow(ctx, `SELECT EXISTS(SELECT FROM wallet_mpc.closure_receipt WHERE wallet_id=$1 AND epoch=$2)`, walletID, epoch).Scan(&closing)
		if err != nil {
			return err
		}
		if closing {
			if h.since.IsZero() {
				h.since = time.Now()
			}
			if time.Since(h.since) < 85*time.Second {
				return errors.New("synthetic delayed closure acknowledgement")
			}
		}
	}
	return h.Participants.Fence(ctx, peer, walletID, epoch)
}

func (l *browserLab) run(t *testing.T, config wallet.API, worker wallet.Worker, pool *pgxpool.Pool,
	subjects [2]string, login, approval, recovery, operator []testGate) {
	t.Helper()
	type apiHandler struct{ http.Handler }
	var handler atomic.Pointer[apiHandler]
	var loss *receiptLoss
	if l.resume {
		loss = &receiptLoss{Handler: config.Handler()}
		handler.Store(&apiHandler{loss})
	} else {
		handler.Store(&apiHandler{config.Handler()})
	}
	api := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.Load().ServeHTTP(w, r)
	}))
	api.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{l.certificate(t)}}
	var sink func(context.Context, wallet.SignedRecoveryRefusal) error
	var signSink func(context.Context, wallet.SignedSignRefusal) error
	if l.refusal || l.signRefusal {
		roots := x509.NewCertPool()
		roots.AddCert(l.root)
		api.TLS.ClientCAs, api.TLS.ClientAuth = roots, tls.VerifyClientCertIfGiven
		deliver := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{l.certificateFor(t, x509.ExtKeyUsageClientAuth)},
		}}}
		defer deliver.CloseIdleConnections()
		sendRefusal := func(ctx context.Context, path string, vote any) error {
			raw, err := json.Marshal(vote)
			if err != nil {
				return err
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, api.URL+path, bytes.NewReader(raw))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			res, err := deliver.Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusNoContent {
				return errors.New("refusal was not durably recorded")
			}
			return nil
		}
		sink = func(ctx context.Context, vote wallet.SignedRecoveryRefusal) error {
			return sendRefusal(ctx, "/v1/recovery/refusals", vote)
		}
		signSink = func(ctx context.Context, vote wallet.SignedSignRefusal) error {
			if err := sendRefusal(ctx, "/v1/sign/refusals", vote); err != nil {
				return err
			}
			handler.Store(&apiHandler{config.Handler()})
			// Retry the same authenticated refusal against the recreated API.
			if err := sendRefusal(ctx, "/v1/sign/refusals", vote); err != nil {
				return err
			}
			t.Log("P03_SIGN_REFUSAL_DELIVERED: committed and retried after API recreation")
			return nil
		}
	}
	api.StartTLS()
	defer api.Close()
	if l.refusal {
		realm := recovery[0].realm
		recovery[0].replace(&gateway.Realm{Role: realm.Role, Origin: realm.Origin, Accounts: realm.Accounts, Key: realm.Key, RecoveryRefusalSink: sink})
	}
	if l.signRefusal {
		realm := approval[0].realm
		approval[0].replace(&gateway.Realm{Role: realm.Role, Origin: realm.Origin, Accounts: realm.Accounts, Key: realm.Key, SignRefusalSink: signSink})
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var held *missingReadiness
	if l.resume {
		worker.Control = &closureReadiness{Participants: worker.Control, pool: pool}
	}
	if l.partial {
		held = &missingReadiness{Participants: worker.Control, db: pool}
		worker.Control = held
	}
	go func() { done <- worker.Run(ctx) }()
	defer func() { cancel(); <-done }()

	oidcMode := os.Getenv("NU54_P03_BROWSER_OIDC") == "1"
	profile := map[string]any{"api": api.URL, "identity": login[0].origin, "approval": approval[0].origin, "recovery": recovery[0].origin}
	var loginProvider string
	if oidcMode {
		registered, stop := l.registerOIDC(t, login[0])
		defer stop()
		profile["onboarding"] = map[string]any{"network": wallet.TestProfile, "providers": []gateway.ProviderIdentity{registered}}
		loginProvider = registered.Name
	}
	base, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	type pair struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	type account struct {
		Login    pair `json:"login"`
		Approval pair `json:"approval"`
	}
	accounts := []account{
		{Login: pair{login[0].username, login[0].password}, Approval: pair{approval[0].username, approval[0].password}},
		{Login: pair{login[1].username, login[1].password}, Approval: pair{approval[1].username, approval[1].password}},
	}
	fixture := map[string]any{"profile": base64.StdEncoding.EncodeToString(base), "accounts": accounts, "signRefusal": l.signRefusal}
	if oidcMode {
		fixture["loginProvider"] = loginProvider
	}
	if l.signRefusal {
		// Read only the original fixture subject's reserved request. The UI
		// runner independently compares the persisted values with its screen.
		oracle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request, walletID, chain string
			var expires time.Time
			err := pool.QueryRow(r.Context(), `SELECT o.request_ref, o.wallet_id, o.intent->>'chain', o.expires_at
				FROM wallet_mpc.operation o JOIN wallet_mpc.wallet w USING(wallet_id)
				WHERE w.subject_ref=$1 AND o.kind='sign' AND o.state='reserved' AND o.intent->>'valueWei'='11'`,
				subjects[0]).Scan(&request, &walletID, &chain, &expires)
			if err != nil {
				http.Error(w, "original fixture unavailable", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"request": request, "wallet": walletID,
				"chain": chain, "expires": expires.UTC().Format("2006-01-02T15:04:05.000Z")})
		}))
		defer oracle.Close()
		fixture["originalReviewURL"] = oracle.URL
	}

	if l.market != nil {
		fixture["deployment"] = l.market.Deployment
		fixture["venue"] = l.market.Venue
		fixture["expireMarketOrder"] = os.Getenv("NU54_P03_BROWSER_EXPIRED") == "1"
	}
	if l.recovery {
		fixture["recovery"] = pair{recovery[0].username, recovery[0].password}
		fixture["alternateLogin"] = pair{login[2].username, login[2].password}
		fixture["originalCredential"] = login[0].credential
		fixture["alternateCredential"] = login[2].credential
		fixture["refusal"] = l.refusal
		fixture["partial"] = l.partial
		fixture["resume"] = l.resume
	}
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}

	for name, raw := range map[string][]byte{"ca.der": l.caDER, "browser.pending": encoded} {
		if err := os.WriteFile(filepath.Join(l.out, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Rename(filepath.Join(l.out, "browser.pending"), filepath.Join(l.out, "browser.json")); err != nil {
		t.Fatal(err)
	}
	fixtureWindow := 8 * time.Minute
	if os.Getenv("NU54_P03_BROWSER_RECOVERY") == "1" {
		fixtureWindow = 15 * time.Minute
	}
	deadline := time.Now().Add(fixtureWindow)
	restarted := false
	for time.Now().Before(deadline) {
		if l.resume && !restarted && loss.closure.Load() {
			handler.Store(&apiHandler{config.Handler()})
			realm := approval[0].realm
			approval[0].replace(&gateway.Realm{Role: realm.Role, Origin: realm.Origin, Accounts: realm.Accounts, Key: realm.Key})
			restarted = true
		}
		if l.refusal && !restarted {
			var durable bool
			if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT FROM wallet_mpc.recovery_refusal r
				JOIN wallet_mpc.operation o USING(operation_id) JOIN wallet_mpc.wallet w USING(wallet_id)
				WHERE w.subject_ref=$1 AND o.state='denied')`, subjects[0]).Scan(&durable); err != nil {
				t.Fatal(err)
			}
			if durable {
				realm := recovery[0].realm
				recovery[0].replace(&gateway.Realm{Role: realm.Role, Origin: realm.Origin, Accounts: realm.Accounts, Key: realm.Key, RecoveryRefusalSink: sink})
				handler.Store(&apiHandler{config.Handler()})
				restarted = true
			}
		}
		if _, err := os.Stat(filepath.Join(l.out, "failed")); err == nil {
			t.Fatal("phone browser automation failed")
		}
		if _, err := os.Stat(filepath.Join(l.out, "done")); err == nil {
			if oidcMode {
				var active, dkg, signed int
				err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE w.state='active'),
					count(*) FILTER (WHERE o.kind='dkg' AND o.state='succeeded'), count(*) FILTER (WHERE o.kind='sign')
					FROM wallet_mpc.wallet w JOIN wallet_mpc.operation o USING(wallet_id)
					WHERE w.subject_ref=$1`, subjects[0]).Scan(&active, &dkg, &signed)
				if err != nil || active != 1 || dkg != 1 || signed != 0 {
					t.Fatalf("registered OIDC wallet result (active %d, completed DKG %d, sign requests %d): %v", active, dkg, signed, err)
				}
				t.Log("verified one completed DKG and zero signing requests for the registered OIDC subject")
				return
			}
			if l.market != nil {
				var active, signed int
				err := pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE EXISTS
					(SELECT 1 FROM wallet_mpc.operation o WHERE o.wallet_id=w.wallet_id AND o.kind='sign' AND o.state='succeeded'))
					FROM wallet_mpc.wallet w WHERE w.subject_ref=$1 AND w.state='active'`, subjects[0]).Scan(&active, &signed)
				want := 1
				if os.Getenv("NU54_P03_BROWSER_EXPIRED") == "1" {
					want = 0
				}
				if err != nil || active != 1 || signed != want {
					t.Fatalf("market browser result (active wallets %d, signed wallets %d, expected %d): %v", active, signed, want, err)
				}
				if want == 1 {
					t.Log("market wallet independently approved an isolated order in iOS Safari")
				} else {
					t.Log("expired browser market quote emitted no wallet signature")
				}
				return
			}
			if l.refusal {
				if !restarted {
					t.Fatal("refusal did not survive realm and API recreation")
				}
				checkBrowserRefusal(t, ctx, pool, subjects[0])
				checkBrowserReview(t, ctx, api, operator[0], login[2], pool, subjects)
				return
			}
			if l.partial {
				if !held.checked.Load() {
					t.Fatal("partial quorum freeze not observed in database")
				}
				checkBrowserPartial(t, ctx, pool, subjects[0], held.key)
				return
			}
			if l.recovery {
				if l.resume {
					if !loss.recovery.Load() || !loss.closure.Load() || !restarted {
						t.Fatal("expected two committed response losses")
					}
					checkBrowserReceiptResume(t, ctx, pool, subjects[0])
				}
				checkBrowserRecovery(t, ctx, pool, subjects[0], login[0].credential, login[2].credential)
				return
			}
			if l.signRefusal {
				checkBrowserSignRefusal(t, ctx, pool, subjects[0])
				return
			}
			var active, distinct, signed int
			err := pool.QueryRow(ctx, `SELECT count(*),count(DISTINCT public_key),
				count(*) FILTER (WHERE EXISTS (SELECT 1 FROM wallet_mpc.operation o
					WHERE o.wallet_id=w.wallet_id AND o.kind='sign' AND o.state='succeeded'))
				FROM wallet_mpc.wallet w WHERE subject_ref=ANY($1::uuid[]) AND state='active'`, subjects[:]).Scan(&active, &distinct, &signed)
			if err != nil || active != 2 || distinct != 2 || signed != 2 {
				t.Fatalf("browser enrollment results (active %d, distinct public keys %d, wallets with verified signatures %d): %v", active, distinct, signed, err)
			}
			t.Log("verified two active browser-enrolled wallets with distinct public keys and signature receipts")
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("phone browser fixture timed out")
}

func checkBrowserRecovery(t *testing.T, ctx context.Context, pool *pgxpool.Pool, subject, original, alternate string) {
	t.Helper()
	var closed, refreshed, signedAtNewEpoch, originalRevoked, alternateLinked, oldRetired bool
	var fenced, ready int
	err := pool.QueryRow(ctx, `
		SELECT w.state='closed' AND w.active_epoch=2,
		r.state='succeeded',
		EXISTS (SELECT FROM wallet_mpc.operation s WHERE s.wallet_id=w.wallet_id AND s.kind='sign' AND s.epoch=2 AND s.state='succeeded'),
		EXISTS (SELECT FROM wallet_mpc.auth_link a WHERE a.subject_ref=w.subject_ref AND a.credential_ref=$2 AND a.unlinked_at IS NOT NULL),
		EXISTS (SELECT FROM wallet_mpc.auth_link a WHERE a.subject_ref=w.subject_ref AND a.credential_ref=$3 AND a.unlinked_at IS NULL),
		EXISTS (SELECT FROM wallet_mpc.key_epoch k WHERE k.wallet_id=w.wallet_id AND k.epoch=1 AND k.retired_at IS NOT NULL),
		(SELECT count(*) FROM wallet_mpc.participant_ack a WHERE a.operation_id=r.operation_id AND a.evidence_kind='fenced'),
		(SELECT count(*) FROM wallet_mpc.participant_ack a WHERE a.operation_id=r.operation_id AND a.evidence_kind='ready')
		FROM wallet_mpc.wallet w JOIN wallet_mpc.operation r USING(wallet_id)
		WHERE w.subject_ref=$1 AND r.kind='refresh' AND r.epoch=2`, subject, original, alternate).
		Scan(&closed, &refreshed, &signedAtNewEpoch, &originalRevoked, &alternateLinked, &oldRetired, &fenced, &ready)
	if err != nil || !closed || !refreshed || !signedAtNewEpoch || !originalRevoked || !alternateLinked || !oldRetired || fenced < 2 || ready != 3 {
		t.Fatalf("recovery evidence unavailable (closed %t, refreshed %t, signed %t, revoked %t, linked %t, retired %t, fence %d, ready %d): %v",
			closed, refreshed, signedAtNewEpoch, originalRevoked, alternateLinked, oldRetired, fenced, ready, err)
	}
	t.Log("verified closed wallet, same-key epoch-2 recovery, old-participant fences, new-committee acknowledgements and revoked original credential")
}

func checkBrowserRefusal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, subject string) {
	t.Helper()
	var frozen bool
	var refusals, signatures, activated int
	err := pool.QueryRow(ctx, `
		SELECT w.state='blocked' AND w.active_epoch=1 AND o.state='denied' AND o.approval_proof IS NULL,
		 (SELECT count(*) FROM wallet_mpc.recovery_refusal r WHERE r.operation_id=o.operation_id),
		 (SELECT count(*) FROM wallet_mpc.operation s WHERE s.wallet_id=w.wallet_id AND s.kind='sign' AND s.state='succeeded'),
		 (SELECT count(*) FROM wallet_mpc.key_epoch k WHERE k.wallet_id=w.wallet_id AND k.epoch=2)
		FROM wallet_mpc.wallet w JOIN wallet_mpc.operation o USING(wallet_id)
		WHERE w.subject_ref=$1 AND o.kind='refresh'`, subject).Scan(&frozen, &refusals, &signatures, &activated)
	if err != nil || !frozen || refusals != 1 || signatures != 0 || activated != 0 {
		t.Fatalf("refusal invariants: frozen=%t refusals=%d signatures=%d activated=%d: %v", frozen, refusals, signatures, activated, err)
	}
	t.Log("P03_DENIED_DB: one durable refusal, original epoch frozen, signatures zero")
}

func checkBrowserReview(t *testing.T, ctx context.Context, api *httptest.Server, operator, owner testGate, pool *pgxpool.Pool, subjects [2]string) {
	t.Helper()
	var walletID, request string
	if err := pool.QueryRow(ctx, `SELECT w.wallet_id::text, o.request_ref::text
		FROM wallet_mpc.wallet w JOIN wallet_mpc.operation o USING(wallet_id)
		WHERE w.subject_ref=$1 AND o.kind='refresh' AND o.state='denied'`, subjects[0]).Scan(&walletID, &request); err != nil {
		t.Fatal(err)
	}
	ownerToken := owner.confirm(t, map[string]string{"action": "login", "audience": wallet.Audience})["token"]
	operatorToken := operator.confirm(t, map[string]string{"action": "operator", "audience": wallet.Audience})["token"]
	post := func(path, token string, payload any, want int) {
		t.Helper()
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, api.URL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := api.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != want {
			t.Fatalf("scoped review result %d, wanted %d for %s", res.StatusCode, want, path[:12])
		}
	}
	permit := map[string]string{"wallet": walletID, "request": request}
	review := map[string]string{"successor": "a1a3f07b-ea42-406d-aad8-92d66d146328"}
	post("/v1/recovery/reviews", ownerToken, permit, http.StatusUnauthorized)
	post("/v1/operations/"+request+"/review", ownerToken, review, http.StatusForbidden)
	post("/v1/recovery/reviews", operatorToken, permit, http.StatusNoContent)
	post("/v1/operations/"+request+"/review", ownerToken, review, http.StatusOK)
	var linked, frozen bool
	err := pool.QueryRow(ctx, `SELECT prior.state='denied' AND next.state='reserved' AND prior.fingerprint<>next.fingerprint,
		w.state='blocked' AND w.active_epoch=1
		FROM wallet_mpc.recovery_review r JOIN wallet_mpc.operation prior ON prior.operation_id=r.operation_id
		JOIN wallet_mpc.operation next ON next.operation_id=r.successor_operation
		JOIN wallet_mpc.wallet w ON w.wallet_id=prior.wallet_id
		WHERE w.subject_ref=$1 AND next.request_ref=$2`, subjects[0], review["successor"]).Scan(&linked, &frozen)
	if err != nil || !linked || !frozen {
		t.Fatalf("authorized review remains frozen: linked=%t frozen=%t: %v", linked, frozen, err)
	}
	t.Log("P03_REVIEW_DB: separate operator permitted one fresh request; still frozen")
}

func checkBrowserPartial(t *testing.T, ctx context.Context, pool *pgxpool.Pool, subject string, originalKey []byte) {
	t.Helper()
	var activated, retired bool
	var activeKey []byte
	var ready, fenced, signatures int
	err := pool.QueryRow(ctx, `SELECT w.state='active' AND w.active_epoch=2 AND o.state='succeeded',
		w.public_key, old.retired_at IS NOT NULL,
		(SELECT count(*) FROM wallet_mpc.participant_ack a WHERE a.operation_id=o.operation_id AND a.evidence_kind='ready'),
		(SELECT count(*) FROM wallet_mpc.participant_ack a WHERE a.operation_id=o.operation_id AND a.evidence_kind='fenced'),
		(SELECT count(*) FROM wallet_mpc.operation s WHERE s.wallet_id=w.wallet_id AND s.kind='sign' AND s.state='succeeded')
		FROM wallet_mpc.wallet w JOIN wallet_mpc.operation o USING(wallet_id)
		JOIN wallet_mpc.key_epoch old ON old.wallet_id=w.wallet_id AND old.epoch=1
		JOIN wallet_mpc.key_epoch new ON new.wallet_id=w.wallet_id AND new.epoch=2
		WHERE w.subject_ref=$1 AND o.kind='refresh'`, subject).Scan(&activated, &activeKey, &retired, &ready, &fenced, &signatures)
	sameKey := len(originalKey) == 65 && bytes.Equal(originalKey, activeKey)
	if err != nil || !activated || !sameKey || !retired || ready != 3 || fenced != 2 || signatures != 0 {
		t.Fatalf("partial recovery completion: active=%t key=%t retired=%t ready=%d fenced=%d signatures=%d: %v", activated, sameKey, retired, ready, fenced, signatures, err)
	}
	t.Log("P03_PARTIAL_DB: frozen at partial ACK; same key completed with all ACKs; signatures zero")
}

func checkBrowserReceiptResume(t *testing.T, ctx context.Context, pool *pgxpool.Pool, subject string) {
	t.Helper()
	var refreshes, refEvents, closEvents, closures int
	var completed bool
	err := pool.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM wallet_mpc.operation WHERE wallet_id=w.wallet_id AND kind='refresh'),
	 (SELECT count(*) FROM wallet_mpc.audit_outbox WHERE wallet_id=w.wallet_id AND event_kind='recovery_started'),
	 (SELECT count(*) FROM wallet_mpc.audit_outbox WHERE wallet_id=w.wallet_id AND event_kind='closed'),
	 (SELECT count(*) FROM wallet_mpc.closure_receipt WHERE wallet_id=w.wallet_id),
	 EXISTS(SELECT FROM wallet_mpc.closure_receipt r JOIN wallet_mpc.key_epoch e USING(wallet_id,epoch)
        WHERE r.wallet_id=w.wallet_id AND e.retired_at>=r.accepted_at)
	 FROM wallet_mpc.wallet w WHERE w.subject_ref=$1`, subject).Scan(&refreshes, &refEvents, &closEvents, &closures, &completed)
	if err != nil || refreshes != 1 || refEvents != 1 || closEvents != 1 || closures != 1 || !completed {
		t.Fatalf("receipt cardinality/retirement: refresh %d audit %d close audit %d receipt %d retired %t: %v", refreshes, refEvents, closEvents, closures, completed, err)
	}
	t.Log("P03_RESUME_DB: one refresh, one closure, both audit events single")
}

func checkBrowserSignRefusal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, subject string) {
	t.Helper()
	var refused, succeeded, total int
	err := pool.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE o.state='refused' AND o.intent->>'valueWei'='11' AND o.approval_proof IS NULL
		  AND NOT EXISTS (SELECT FROM wallet_mpc.signature_result r WHERE r.operation_id=o.operation_id)
		  AND NOT EXISTS (SELECT FROM wallet_mpc.participant_ack a WHERE a.operation_id=o.operation_id)
		  AND (SELECT count(*) FROM wallet_mpc.audit_outbox a WHERE a.operation_id=o.operation_id AND a.event_kind='sign_refused')=1),
		count(*) FILTER (WHERE o.state='succeeded' AND o.intent->>'valueWei'='12'
		  AND EXISTS (SELECT FROM wallet_mpc.signature_result r WHERE r.operation_id=o.operation_id)),
		count(*)
		FROM wallet_mpc.operation o JOIN wallet_mpc.wallet w USING(wallet_id)
		WHERE w.subject_ref=$1 AND o.kind='sign'`, subject).Scan(&refused, &succeeded, &total)
	if err != nil || refused != 1 || succeeded != 1 || total != 2 {
		t.Fatalf("expected one durable refusal without round evidence and one independently signed request: refused %d signed %d total %d: %v", refused, succeeded, total, err)
	}
	t.Log("P03_SIGN_REFUSAL_DB: one refused request without proof, signature or participant acknowledgment; one fresh signed request")
}
