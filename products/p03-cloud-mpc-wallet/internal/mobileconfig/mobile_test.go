package mobileconfig

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/google/uuid"
)

func TestMobileEnrollmentSealsPrivateKeyAndBindsSignedRoster(t *testing.T) {
	root := t.TempDir()
	var key [32]byte
	rand.Read(key[:])
	csrText, err := Prepare(root, key[:])
	if err != nil {
		t.Fatal(err)
	}
	repeat, err := Prepare(root, key[:])
	if err != nil || repeat != csrText {
		t.Fatalf("restarted enrollment changed public identity: %v", err)
	}
	block, _ := pem.Decode([]byte(csrText))
	if block == nil {
		t.Fatal("missing public CSR")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || csr.CheckSignature() != nil {
		t.Fatal("invalid public CSR")
	}
	authorityPub, authority, _ := ed25519.GenerateKey(rand.Reader)
	caPub, caKey, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now().Truncate(time.Second)
	expiry := now.Add(time.Hour)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Minute), NotAfter: expiry,
		KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true, IsCA: true}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, caPub, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: now.Add(-time.Minute), NotAfter: expiry,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}}
	cert, err := x509.CreateCertificate(rand.Reader, leaf, ca, csr.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	encode := func(der []byte) string {
		return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	}
	subject := uuid.NewString()
	r := Roster{Subject: subject, Epoch: 1, Expires: expiry.Unix(), Certificate: encode(cert), CA: encode(caDER),
		Config: ceremony.Config{ID: "mobile-a1", Listen: "127.0.0.1:44400"}}
	seal := func(r Roster) []byte {
		t.Helper()
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		out, err := json.Marshal(Envelope{Payload: raw, Signature: ed25519.Sign(authority, append([]byte(Domain), raw...))})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	env := seal(r)
	if err := Install(root, key[:], uuid.NewString(), env, authorityPub); err == nil {
		t.Fatal("wrong actor roster installed")
	}
	bogusPub, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := Install(root, key[:], subject, env, bogusPub); err == nil {
		t.Fatal("wrong authority roster installed")
	}
	if err := Install(root, key[:], subject, env, authorityPub); err != nil {
		t.Fatal(err)
	}
	cfg, tls, _, err := Open(root, key[:], subject, authorityPub)
	if err != nil || cfg.ID != r.Config.ID || !bytes.Equal(tls.Leaf.RawSubjectPublicKeyInfo, csr.RawSubjectPublicKeyInfo) {
		t.Fatalf("restart lost device enrollment: %v", err)
	}
	r.Config.Listen = "127.0.0.1:44401"
	if err := Install(root, key[:], subject, seal(r), authorityPub); err == nil {
		t.Fatal("conflicting same revision accepted")
	}
	r.Revision = 1
	if err := Install(root, key[:], subject, seal(r), authorityPub); err != nil {
		t.Fatal(err)
	}
	if err := Install(root, key[:], subject, env, authorityPub); err == nil {
		t.Fatal("signed prior roster replayed")
	}
	if err := Install(root, key[:], subject, seal(r), authorityPub); err != nil {
		t.Fatal("identical retry rejected")
	}
	if err := os.Chmod(root, 0500); err != nil {
		t.Fatal(err)
	}
	r.Revision = 2
	r.Config.Listen = "127.0.0.1:44402"
	writeErr := Install(root, key[:], subject, seal(r), authorityPub)
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 && writeErr == nil {
		t.Fatal("unwritable vault accepted roster update")
	}
	var race sync.WaitGroup
	for revision := uint64(3); revision <= 12; revision++ {
		candidate := r
		candidate.Revision = revision
		candidate.Config.Listen = fmt.Sprintf("127.0.0.1:%d", 44400+revision)
		raw := seal(candidate)
		race.Go(func() { _ = Install(root, key[:], subject, raw, authorityPub) })
	}
	race.Wait()
	cfg, _, _, err = Open(root, key[:], subject, authorityPub)
	if err != nil || cfg.Listen != "127.0.0.1:44412" {
		t.Fatalf("concurrent roster did not preserve highest committed revision: %v", err)
	}
	r.Revision = 13
	r.Config.ID = "mobile-a2"
	if err := Install(root, key[:], subject, seal(r), authorityPub); err == nil {
		t.Fatal("live seat changed in place")
	}
	if _, err := os.Stat(filepath.Join(root, "tls.key")); !os.IsNotExist(err) {
		t.Fatal("plaintext TLS private key created")
	}
	blob, err := os.ReadFile(filepath.Join(root, "device-tls.sealed"))
	if err != nil || len(blob) != 60 {
		t.Fatal("missing sealed private seed")
	}
	var other [32]byte
	rand.Read(other[:])
	if _, _, _, err := Open(root, other[:], subject, authorityPub); err == nil {
		t.Fatal("different device key decrypted TLS identity")
	}
}
