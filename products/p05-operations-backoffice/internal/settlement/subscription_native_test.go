package settlement

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/jackc/pgx/v5/pgxpool"
)

type nativeSubscriptionLab struct {
	anchor []byte
	path   string
	server []byte
	leaves map[string][]byte
	role   string
}

func nativeCommerceLabDirectory() string {
	if path := os.Getenv("NU54_TRAVEL_NATIVE_LAB"); path != "" {
		return path
	}
	if path := os.Getenv("NU54_KIOSK_OUTCOME_LAB"); path != "" {
		return path
	}
	return os.Getenv("NU54_SUBSCRIPTION_NATIVE_LAB")
}

func nativeSubscriptionServer(t *testing.T, inherited tls.Certificate) tls.Certificate {
	t.Helper()
	if nativeCommerceLabDirectory() == "" {
		return inherited
	}
	return newNativeLoopbackTLS(t)
}

func newNativeLoopbackTLS(t *testing.T) tls.Certificate {
	t.Helper()
	// The shipping iOS transport pins P-256 SPKI. The generic httptest certificate is RSA.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := &x509.Certificate{
		SerialNumber: big.NewInt(97), Subject: pkix.Name{CommonName: "NU54 test server root"},
		IsCA: true, BasicConstraintsValid: true, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(98), Subject: pkix.Name{CommonName: "127.0.0.1"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		BasicConstraintsValid: true, NotBefore: root.NotBefore, NotAfter: root.NotAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der, rootDER}, PrivateKey: key}
}

func enrollNativeSubscriptionLab(t *testing.T, server tls.Certificate, clientPEM []byte, grants map[string]service.Principal, principal service.Principal) ([]byte, *nativeSubscriptionLab) {
	t.Helper()
	dir := nativeCommerceLabDirectory()
	if dir == "" {
		return clientPEM, nil
	}
	return enrollNativeDevices(t, dir, server, clientPEM, grants, principal)
}

func enrollNativeDevices(t *testing.T, dir string, server tls.Certificate, clientPEM []byte, grants map[string]service.Principal, principal service.Principal) ([]byte, *nativeSubscriptionLab) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "public-keys.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries map[string]string
	if err := json.Unmarshal(data, &entries); err != nil || len(entries) == 0 || len(entries) > 2 {
		t.Fatalf("native subscription public keys: %v", err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(99), Subject: pkix.Name{CommonName: "NU54 test device root"}, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Unix(principal.ExpiresAt, 0)}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	devices := &nativeSubscriptionLab{path: dir, server: server.Certificate[0], anchor: server.Certificate[1], leaves: make(map[string][]byte), role: principal.Role}
	for platform, text := range entries {
		if platform != "android" && platform != "ios" && platform != "ios-second" {
			t.Fatal("unknown private test device")
		}
		spki, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			t.Fatal(err)
		}
		candidate, err := x509.ParsePKIXPublicKey(spki)
		pub, ok := candidate.(*ecdsa.PublicKey)
		if err != nil || !ok || pub.Curve != elliptic.P256() {
			t.Fatal("native test enrollment requires a P-256 device SPKI")
		}
		leaf := &x509.Certificate{SerialNumber: big.NewInt(int64(len(devices.leaves) + 100)), Subject: pkix.Name{CommonName: "NU54 test device " + platform}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter,
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		der, err := x509.CreateCertificate(rand.Reader, leaf, parent, pub, key)
		if err != nil {
			t.Fatal(err)
		}
		fingerprint := sha256.Sum256(der)
		grants[hex.EncodeToString(fingerprint[:])] = principal
		devices.leaves[platform] = der
	}
	return append(clientPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})...), devices
}

func (lab *nativeSubscriptionLab) enrollmentDocument(t *testing.T, url string, order seededOrder, invoice any) []byte {
	t.Helper()
	server, err := x509.ParseCertificate(lab.server)
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(server.RawSubjectPublicKeyInfo)
	profiles := make(map[string]any, len(lab.leaves))
	for platform, leaf := range lab.leaves {
		profiles[platform] = map[string]any{"endpoint": url + "/v2/commerce", "pin": hex.EncodeToString(pin[:]),
			"ca": base64.StdEncoding.EncodeToString(lab.anchor), "certificates": []string{base64.StdEncoding.EncodeToString(leaf)},
			"scope": map[string]any{"branchRef": order.Branch, "tenantRef": order.Tenant, "subjectRef": order.Actor,
				"role": lab.role, "expiresAt": time.Now().Add(30 * time.Minute).Unix()},
			"payment": map[string]any{"chain": 8283, "contract": lower(order.Target.Contract), "asset": lower(order.Target.Asset),
				"beneficiary": lower(order.Quote.Args.Beneficiary), "merchantId": rawHex(order.Quote.Args.MerchantId[:]),
				"signer": lower(fixtureSigner(101).Address()), "keyVersion": 1, "beneficiaryVersion": 1,
				"maxBaseUnits": "30000000", "presentationKey": rawHex(pilotPresentationKey())}}
	}
	presentation := sha256.Sum256(pilotPresentationKey())
	data, err := json.Marshal(map[string]any{"profiles": profiles, "invoice": invoice,
		"tlsPin": hex.EncodeToString(pin[:]), "presentationPin": hex.EncodeToString(presentation[:])})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (lab *nativeSubscriptionLab) verifyPreview(t *testing.T, ctx context.Context, db *pgxpool.Pool, url string, order seededOrder, invoice commerce.Invoice) {
	t.Helper()
	data := lab.enrollmentDocument(t, url, order, invoice)
	preview := filepath.Join(lab.path, "preview.json")
	aborted := filepath.Join(lab.path, "ui-aborted")
	if _, err := os.Stat(aborted); err == nil {
		t.Fatal("native subscription UI did not complete")
	}
	staged := filepath.Join(lab.path, "preview.tmp")
	defer os.Remove(staged)
	defer os.Remove(preview)
	if err := os.WriteFile(staged, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(staged, preview); err != nil {
		t.Fatal(err)
	}
	// The host keeps both real services alive for the paired native UI test.
	deadline := time.NewTimer(180 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(250 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-deadline.C:
			t.Fatal("native subscription UI did not finish")
		case <-poll.C:
			if _, err := os.Stat(aborted); err == nil {
				t.Fatal("native subscription UI did not complete")
			}
			_, err := os.Stat(filepath.Join(lab.path, "ui-completed"))
			if err == nil {
				var state string
				var claims, quotes, submissions, payments int
				err := db.QueryRow(ctx, `SELECT state,
					(SELECT count(*) FROM ledger.checkout_claim WHERE sale_id=s.id),
					(SELECT count(*) FROM ledger.quote_work WHERE sale_id=s.id),
					(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=s.id),
					(SELECT count(*) FROM ledger.payment WHERE sale_id=s.id)
					FROM ledger.sale s WHERE id=$1`, invoice.Sale).Scan(&state, &claims, &quotes, &submissions, &payments)
				if err != nil || state != "preview" || claims+quotes+submissions+payments != 0 {
					t.Fatalf("invoice review caused writes: state=%s claims=%d quotes=%d submissions=%d payments=%d: %v",
						state, claims, quotes, submissions, payments, err)
				}
				evidence, err := json.Marshal(map[string]any{"state": state, "claims": claims, "quotes": quotes, "submissions": submissions,
					"payments": payments, "enrolledNativeInstallations": len(lab.leaves)})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(lab.path, "read-only.json"), evidence, 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			if !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
	}
}

func (lab *nativeSubscriptionLab) publish(t *testing.T, name, value string) {
	t.Helper()
	temp := filepath.Join(lab.path, name+".tmp")
	if err := os.WriteFile(temp, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temp, filepath.Join(lab.path, name)); err != nil {
		t.Fatal(err)
	}
}

func (lab *nativeSubscriptionLab) await(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	deadline := time.NewTimer(150 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(filepath.Join(lab.path, "ui-aborted")); err == nil {
			t.Fatalf("native UI stopped waiting for %s", name)
		}
		if _, err := os.Stat(filepath.Join(lab.path, name)); err == nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-deadline.C:
			t.Fatalf("native UI did not reach %s", name)
		case <-ticker.C:
		}
	}
}
