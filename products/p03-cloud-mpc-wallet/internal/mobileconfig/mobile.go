// Package mobileconfig manages the native participant's non-exported TLS identity.
package mobileconfig

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

const Domain = "nu54/p03/device-roster/v1\x00"

type Envelope struct {
	Payload   []byte
	Signature []byte
}
type Roster struct {
	Subject     string
	Expires     int64
	Epoch       uint64
	Revision    uint64
	Certificate string
	CA          string
	Config      ceremony.Config
}

func private(root string, key []byte, create bool) (ed25519.PrivateKey, error) {
	if len(key) != 32 {
		return nil, errors.New("invalid vault key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	seal, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, "device-tls.sealed")
	aad := []byte("nu54/p03/mobile-device-tls/v1")
	data, err := os.ReadFile(path)
	if err == nil {
		clearKey, err := seal.Open(nil, nil, data, aad)
		if err != nil || len(clearKey) != ed25519.SeedSize {
			return nil, errors.New("invalid device credential")
		}
		defer clear(clearKey)
		return ed25519.NewKeyFromSeed(clearKey), nil
	}
	if !errors.Is(err, os.ErrNotExist) || !create {
		return nil, err
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	seed := priv.Seed()
	defer clear(seed)
	if err := atomic(root, filepath.Base(path), seal.Seal(nil, nil, seed, aad)); err != nil {
		clear(priv)
		return nil, err
	}
	return priv, nil
}

func atomic(root, name string, data []byte) error {
	file, err := os.CreateTemp(root, ".p03-pending-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, errorWrite := file.Write(data)
	if err := errors.Join(errorWrite, file.Sync(), file.Close()); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), filepath.Join(root, name)); err != nil {
		return err
	}
	dir, err := os.Open(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func Prepare(root string, key []byte) (string, error) {
	p, err := private(root, key, true)
	if err != nil {
		return "", err
	}
	defer clear(p)
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "NU54 P03 mobile participant"}}, p)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})), nil
}

func Install(root string, key []byte, subject string, raw, authority []byte) error {
	guard, err := os.OpenFile(filepath.Join(root, ".roster.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer guard.Close()
	if err := unix.Flock(int(guard.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	cfg, _, _, err := decode(root, key, subject, raw, authority)
	if err != nil {
		return err
	}
	var incoming Envelope
	var next Roster
	_ = json.Unmarshal(raw, &incoming)
	_ = json.Unmarshal(incoming.Payload, &next)
	previous, err := os.ReadFile(filepath.Join(root, "roster.json"))
	if err == nil {
		var prior Envelope
		var old Roster
		if json.Unmarshal(previous, &prior) != nil || !ed25519.Verify(authority, append([]byte(Domain), prior.Payload...), prior.Signature) ||
			json.Unmarshal(prior.Payload, &old) != nil || old.Subject != subject || old.Config.ID != cfg.ID {
			return errors.New("device seat already bound")
		}
		if next.Epoch < old.Epoch || (next.Epoch == old.Epoch && next.Revision < old.Revision) {
			return errors.New("signed roster rollback")
		}
		if next.Epoch == old.Epoch && next.Revision == old.Revision {
			if bytes.Equal(prior.Payload, incoming.Payload) {
				return nil
			}
			return errors.New("conflicting signed roster revision")
		}

	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return atomic(root, "roster.json", raw)
}

func Open(root string, key []byte, subject string, authority []byte) (ceremony.Config, tls.Certificate, []byte, error) {
	raw, err := os.ReadFile(filepath.Join(root, "roster.json"))
	if err != nil {
		return ceremony.Config{}, tls.Certificate{}, nil, err
	}
	return decode(root, key, subject, raw, authority)
}

func decode(root string, key []byte, subject string, raw, authority []byte) (ceremony.Config, tls.Certificate, []byte, error) {
	var cfg ceremony.Config
	var cert tls.Certificate
	bad := errors.New("signed mobile roster is invalid")
	if len(authority) != ed25519.PublicKeySize || len(raw) > 65536 {
		return cfg, cert, nil, bad
	}
	id, err := uuid.Parse(subject)
	if err != nil || id == uuid.Nil || id.String() != subject {
		return cfg, cert, nil, bad
	}
	var signed Envelope
	if json.Unmarshal(raw, &signed) != nil || !ed25519.Verify(authority, append([]byte(Domain), signed.Payload...), signed.Signature) {
		return cfg, cert, nil, bad
	}
	var r Roster
	if json.Unmarshal(signed.Payload, &r) != nil || r.Subject != subject || r.Epoch == 0 || r.Expires <= time.Now().Unix() ||
		r.Config.ID == "" || r.Config.Certificate != "" || r.Config.CertificateKey != "" || r.Config.ClientCA != "" || r.Config.Vault != "" || r.Config.VaultKey != "" {
		return cfg, cert, nil, bad
	}
	p, err := private(root, key, false)
	if err != nil {
		return cfg, cert, nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(p)
	clear(p)
	if err != nil {
		return cfg, cert, nil, err
	}
	defer clear(der)
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	defer clear(privatePEM)
	cert, err = tls.X509KeyPair([]byte(r.Certificate), privatePEM)
	if err != nil {
		return cfg, tls.Certificate{}, nil, bad
	}
	cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
	if err != nil || cert.Leaf.NotAfter.Unix() != r.Expires {
		return cfg, tls.Certificate{}, nil, bad
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(r.CA)) {
		return cfg, tls.Certificate{}, nil, bad
	}
	if _, err := cert.Leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
		return cfg, tls.Certificate{}, nil, bad
	}
	if _, err := cert.Leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return cfg, tls.Certificate{}, nil, bad
	}
	cfg = r.Config
	cfg.Vault = filepath.Join(root, "vault")
	return cfg, cert, []byte(r.CA), nil
}
