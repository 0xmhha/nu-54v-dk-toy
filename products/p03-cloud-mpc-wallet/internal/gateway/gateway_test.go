package gateway

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/sha3"
)

func TestIdentityAndIndependentApprovalDeviceFlows(t *testing.T) {
	public, identity, _ := ed25519.GenerateKey(rand.Reader)
	approvalPublic, approval, _ := ed25519.GenerateKey(rand.Reader)
	secret := token()
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	realm := &Realm{Role: "identity", Origin: "https://identity.test", Subject: uuid.NewString(), Credential: uuid.NewString(),
		Username: "test-owner", PasswordHash: string(hash), Key: identity}
	login := realm.Handler()
	call := func(h http.Handler, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", path, bytes.NewReader(raw))
		r := httptest.NewRecorder()
		h.ServeHTTP(r, req)
		return r
	}
	verifier := token()
	digest := sha256.Sum256([]byte(verifier))
	start := func(h http.Handler, request any) string {
		t.Helper()
		r := call(h, "/p03/device/start", map[string]any{"request": request, "codeChallenge": encoding.EncodeToString(digest[:]), "codeChallengeMethod": "S256"})
		var result struct {
			Code string `json:"deviceCode"`
		}
		if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &result) != nil || result.Code == "" {
			t.Fatalf("challenge start %d", r.Code)
		}
		return result.Code
	}
	poll := func(h http.Handler, code, verifier string, status int) map[string]string {
		t.Helper()
		r := call(h, "/p03/device/poll", map[string]string{"deviceCode": code, "codeVerifier": verifier})
		var output map[string]string
		if r.Code != status || status == 200 && json.Unmarshal(r.Body.Bytes(), &output) != nil {
			t.Fatalf("poll returned %d", r.Code)
		}
		return output
	}
	confirm := func(h http.Handler, code, origin, credential string, status int, decisions ...string) {
		t.Helper()
		decision := "allow"
		if len(decisions) != 0 {
			decision = decisions[0]
		}
		form := url.Values{"user": {"test-owner"}, "password": {credential}, "code": {code}, "decision": {decision}}
		req := httptest.NewRequest("POST", "/p03/device/verify", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", origin)
		r := httptest.NewRecorder()
		h.ServeHTTP(r, req)
		if r.Code != status {
			t.Fatalf("confirm returned %d", r.Code)
		}
	}
	code := start(login, map[string]string{"action": "login", "audience": wallet.Audience})
	page := httptest.NewRecorder()
	login.ServeHTTP(page, httptest.NewRequest("GET", "/p03/device/verify?code="+code, nil))
	if page.Code != http.StatusOK || page.Header().Get("Referrer-Policy") != "same-origin" {
		t.Fatal("verification form must preserve same-origin browser POST without cross-origin referrers")
	}
	html := page.Body.String()
	_, style, hasStyle := strings.Cut(html, "<style>")
	css, _, closesStyle := strings.Cut(style, "</style>")
	sum := sha256.Sum256([]byte(css))
	styleSource := "style-src 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	if !hasStyle || !closesStyle || !strings.Contains(page.Header().Get("Content-Security-Policy"), styleSource) ||
		!strings.Contains(html, `<meta name="viewport" content="width=device-width, initial-scale=1">`) {
		t.Fatal("mobile verification stylesheet must match the authorized content hash")
	}
	confirm(login, code, "null", secret, 403)
	poll(login, code, token(), 403)
	if poll(login, code, verifier, 200)["state"] != "pending" {
		t.Fatal("unconfirmed login returned token")
	}
	confirm(login, code, "https://approval.test", secret, 403)
	confirm(login, code, realm.Origin, token(), 403)
	confirm(login, code, realm.Origin, secret, 200)
	creds := poll(login, code, verifier, 200)
	time.Sleep(1100 * time.Millisecond)
	again := poll(login, code, verifier, 200)
	if again["token"] != creds["token"] || again["enrollmentToken"] != creds["enrollmentToken"] {
		t.Fatal("retry minted a new identity grant")
	}
	realm.mu.Lock()
	realm.flows[code].expires = time.Now().Add(-time.Second)
	realm.mu.Unlock()
	poll(login, code, verifier, 403)

	v := wallet.IdentityVerifier{Issuer: realm.Origin, Key: public}
	if _, err := v.Verify(creds["token"], "wallet", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(creds["enrollmentToken"], "wallet:enroll", time.Now()); err != nil {
		t.Fatal(err)
	}
	consentSecret := token()
	consentHash, _ := bcrypt.GenerateFromPassword([]byte(consentSecret), bcrypt.MinCost)
	consent := &Realm{Role: "approval", Origin: "https://approve.test", Username: "test-owner", PasswordHash: string(consentHash), Key: approval}
	h := consent.Handler()
	session := ceremony.Session{ID: uuid.NewString(), Wallet: uuid.NewString(), Kind: ceremony.DKG, Epoch: 1,
		Expires: time.Now().Add(time.Minute).Unix(),
		Peers:   []ceremony.Peer{{ID: "a1", Role: ceremony.A, Epoch: 1}, {ID: "b1", Role: ceremony.B, Epoch: 1}, {ID: "c1", Role: ceremony.C, Epoch: 1}}}
	fingerprint, err := session.Fingerprint(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	code = start(h, map[string]any{"action": "dkg", "session": session, "fingerprint": fingerprint})
	confirm(h, code, consent.Origin, secret, 403)
	confirm(h, code, consent.Origin, consentSecret, 200)
	proof, err := json.Marshal(poll(h, code, verifier, 200))
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ Proof []byte }
	if json.Unmarshal(proof, &out) != nil || !ed25519.Verify(approvalPublic, ceremony.ApprovalMessage(ceremony.DKG, fingerprint), out.Proof) {
		t.Fatal("independent approval did not bind session")
	}
	owner := uuid.NewString()
	otherOwner := uuid.NewString()
	multi := (&Realm{Role: "approval", Origin: "https://shared-approval.test", Accounts: []Account{
		{Subject: owner, Username: "test-owner", PasswordHash: string(consentHash), Key: approval},
		{Subject: otherOwner, Username: "another-owner", PasswordHash: string(hash), Key: identity},
	}}).Handler()
	target := map[string]any{"action": "dkg", "session": session, "fingerprint": fingerprint, "owner": otherOwner}
	crossCode := start(multi, target)
	confirm(multi, crossCode, "https://shared-approval.test", consentSecret, 403)
	if poll(multi, crossCode, verifier, 200)["state"] != "pending" {
		t.Fatal("wrong browser owner obtained a proof")
	}
	target["owner"] = owner
	ownCode := start(multi, target)
	confirm(multi, ownCode, "https://shared-approval.test", consentSecret, 200)
	if poll(multi, ownCode, verifier, 200)["proof"] == "" {
		t.Fatal("bound browser owner lost approval")
	}

	session.Kind, session.Epoch = ceremony.Refresh, 2
	session.KeySession = uuid.NewString()
	session.Peers = []ceremony.Peer{
		{ID: "b1", Role: ceremony.B, Epoch: 1}, {ID: "c1", Role: ceremony.C, Epoch: 1},
		{ID: "a2", Role: ceremony.A, Epoch: 2}, {ID: "b2", Role: ceremony.B, Epoch: 2}, {ID: "c2", Role: ceremony.C, Epoch: 2},
	}
	fingerprint, err = session.Fingerprint(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var proofs []wallet.SignedRecoveryRefusal
	recovery := &Realm{Role: "recovery", Origin: "https://recovery.test", Accounts: []Account{
		{Subject: owner, Username: "test-owner", PasswordHash: string(consentHash), Key: approval},
	}, RecoveryRefusalSink: func(_ context.Context, proof wallet.SignedRecoveryRefusal) error {
		proofs = append(proofs, proof)
		if len(proofs) == 1 {
			return errors.New("synthetic ambiguous response")
		}
		return nil
	}}
	h = recovery.Handler()
	code = start(h, map[string]any{"action": "refresh", "session": session, "fingerprint": fingerprint, "owner": owner})
	confirm(h, code, recovery.Origin, consentSecret, 503, "deny")
	confirm(h, code, recovery.Origin, consentSecret, 403)
	confirm(h, code, recovery.Origin, consentSecret, 200, "deny")
	if len(proofs) != 2 || proofs[0].Decision != proofs[1].Decision || !bytes.Equal(proofs[0].Signature, proofs[1].Signature) ||
		proofs[0].Decision.Subject != owner || proofs[0].Decision.Fingerprint != fingerprint ||
		!ed25519.Verify(approvalPublic, proofs[0].Decision.Message(), proofs[0].Signature) {
		t.Fatal("a refused operation did not retry with the exact signed decision")
	}

	transfer := wallet.Transfer{Profile: wallet.TestProfile, Chain: wallet.TestChain, To: "0x1234567890123456789012345678901234567890",
		ValueWei: "1000", Nonce: "0", MaxFeeWei: "1000000000", PriorityFeeWei: "1000000"}
	unsigned, _ := hex.DecodeString("02eb83aa36a780830f4240843b9aca008252089412345678901234567890123456789012345678908203e880c0")
	session.Kind, session.Epoch, session.ID, session.KeySession = ceremony.Sign, 1, uuid.NewString(), uuid.NewString()
	session.Peers = []ceremony.Peer{{ID: "a1", Role: ceremony.A, Epoch: 1}, {ID: "b1", Role: ceremony.B, Epoch: 1}}
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(unsigned)
	hasher.Sum(session.Digest[:0])
	rawTransfer, _ := json.Marshal(transfer)
	session.Intent = sha256.Sum256(append([]byte("nu54/p03/transfer/v1\x00"), rawTransfer...))
	fingerprint, err = session.Fingerprint(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var denied []wallet.SignedSignRefusal
	signing := &Realm{Role: "approval", Origin: "https://approve.test", Accounts: []Account{
		{Subject: owner, Username: "test-owner", PasswordHash: string(consentHash), Key: approval},
	}, SignRefusalSink: func(_ context.Context, proof wallet.SignedSignRefusal) error {
		denied = append(denied, proof)
		switch len(denied) {
		case 1:
			return errors.New("synthetic missing response")
		case 3:
			return wallet.ErrConflict
		default:
			return nil
		}
	}}
	h = signing.Handler()
	action := map[string]any{"action": "sign", "session": session, "fingerprint": fingerprint, "owner": owner, "transfer": transfer}
	code = start(h, action)
	display := httptest.NewRecorder()
	h.ServeHTTP(display, httptest.NewRequest("GET", "/p03/device/verify?code="+code, nil))
	for _, detail := range []string{"Participants · A · B", "Expires ", session.Wallet, transfer.To} {
		if !strings.Contains(display.Body.String(), detail) {
			t.Fatalf("independent review omitted %q", detail)
		}
	}
	confirm(h, code, signing.Origin, secret, 403, "deny")
	if len(denied) != 0 {
		t.Fatal("unverified login emitted a sign refusal")
	}
	confirm(h, code, signing.Origin, consentSecret, 503, "deny")
	if poll(h, code, verifier, 200)["state"] != "pending" {
		t.Fatal("uncertain refusal marked as successful")
	}
	confirm(h, code, signing.Origin, consentSecret, 403, "allow")
	confirm(h, code, signing.Origin, consentSecret, 200, "deny")
	confirm(h, code, signing.Origin, consentSecret, 200, "deny")
	if len(denied) != 2 || denied[0].Decision != denied[1].Decision || !bytes.Equal(denied[0].Signature, denied[1].Signature) ||
		denied[0].Decision.Subject != owner || denied[0].Decision.Fingerprint != fingerprint ||
		!ed25519.Verify(approvalPublic, denied[0].Decision.Message(), denied[0].Signature) ||
		ed25519.Verify(approvalPublic, ceremony.ApprovalMessage(ceremony.Sign, fingerprint), denied[0].Signature) {
		t.Fatal("sign refusal proof was unbound, non-idempotent, or usable as an approval")
	}
	if outcome := poll(h, code, verifier, 200); outcome["state"] != "denied" || outcome["proof"] != "" {
		t.Fatal("committed sign refusal did not terminate the browser flow")
	}
	code = start(h, action)
	confirm(h, code, signing.Origin, consentSecret, 409, "deny")
	if outcome := poll(h, code, verifier, 200); outcome["state"] != "resolved" || outcome["proof"] != "" {
		t.Fatal("sign refusal conflict was misreported as a committed refusal")
	}

}
