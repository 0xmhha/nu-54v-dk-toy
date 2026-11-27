// mpc-roster is an offline mobile enrollment authority. It receives only public CSRs.
package main

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"io"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/mobileconfig"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
	"github.com/google/uuid"
)

type input struct {
	CSR, CA, CAKey, RosterSeed, Output, Host string
	Roster                                   mobileconfig.Roster
}

func main() {
	path := flag.String("config", "", "approval configuration")
	profile := flag.String("profile-spec", "", "public account profile to authorize")
	profileSeed := flag.String("profile-seed", "", "independent profile authority seed")
	profileOut := flag.String("profile-out", "", "published profile directory")
	flag.Parse()
	if *profile != "" {
		if *path != "" {
			log.Fatal("select one approval operation")
		}
		if err := publishProfile(*profile, *profileSeed, *profileOut); err != nil {
			log.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(*path)
	if err != nil {
		log.Fatal("cannot read approval configuration")
	}
	var cfg input
	if json.Unmarshal(raw, &cfg) != nil {
		log.Fatal("invalid approval configuration")
	}
	if err := publish(cfg); err != nil {
		log.Fatal(err)
	}
}

func publish(in input) error {
	bad := errors.New("invalid offline enrollment")
	actor, err := uuid.Parse(in.Roster.Subject)
	if err != nil || actor == uuid.Nil || actor.String() != in.Roster.Subject || in.Roster.Epoch == 0 ||
		in.Roster.Config.ID == "" || in.Host == "" || in.Output == "" {
		return bad
	}
	csrBlock := readPEM(in.CSR, false)
	caBlock := readPEM(in.CA, false)
	keyBlock := readPEM(in.CAKey, true)
	if csrBlock == nil || caBlock == nil || keyBlock == nil {
		return bad
	}
	defer clear(keyBlock.Bytes)
	csr, err := x509.ParseCertificateRequest(csrBlock.Bytes)
	if err != nil || csr.CheckSignature() != nil || csr.PublicKeyAlgorithm != x509.Ed25519 {
		return bad
	}
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil || !ca.IsCA {
		return bad
	}
	caValue, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return bad
	}
	caKey, ok := caValue.(crypto.Signer)
	if !ok {
		return bad
	}
	seed, err := privateFile(in.RosterSeed)
	if err != nil || len(seed) != ed25519.SeedSize {
		return bad
	}
	rosterKey := ed25519.NewKeyFromSeed(seed)
	defer clear(rosterKey)
	clear(seed)
	if key, ok := caKey.Public().(ed25519.PublicKey); ok && key.Equal(rosterKey.Public()) {
		return bad
	}
	fingerprint := sha256.Sum256(csr.RawSubjectPublicKeyInfo)
	name := in.Roster.Subject + "-" + hex.EncodeToString(fingerprint[:])
	if filepath.Base(in.CSR) != name+".csr" {
		return bad
	}
	now := time.Now()
	expires := time.Unix(in.Roster.Expires, 0)
	if !expires.After(now.Add(time.Minute)) || expires.After(now.Add(24*time.Hour)) || expires.After(ca.NotAfter) {
		return bad
	}
	sn, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	cert := &x509.Certificate{SerialNumber: sn, Subject: pkix.Name{CommonName: in.Roster.Config.ID},
		NotBefore: now.Add(-time.Minute), NotAfter: expires, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}}
	if ip := net.ParseIP(in.Host); ip != nil {
		cert.IPAddresses = []net.IP{ip}
	} else {
		cert.DNSNames = []string{in.Host}
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, ca, csr.PublicKey, caKey)
	if err != nil {
		return err
	}
	in.Roster.Certificate = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	in.Roster.CA = string(pem.EncodeToMemory(caBlock))
	payload, err := json.Marshal(in.Roster)
	if err != nil {
		return err
	}
	signed := mobileconfig.Envelope{Payload: payload, Signature: ed25519.Sign(rosterKey, append([]byte(mobileconfig.Domain), payload...))}
	out, err := json.Marshal(signed)
	if err != nil || len(out) > 6144 {
		return bad
	}
	candidate, err := os.CreateTemp(in.Output, ".p03-roster-")
	if err != nil {
		return err
	}
	defer os.Remove(candidate.Name())
	_, err = candidate.Write(out)
	if err := errors.Join(err, candidate.Sync(), candidate.Close()); err != nil {
		return err
	}
	return os.Rename(candidate.Name(), filepath.Join(in.Output, name+".json"))
}

func readPEM(path string, secret bool) *pem.Block {
	var b []byte
	var err error
	if secret {
		b, err = privateFile(path)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil
	}
	if secret {
		defer clear(b)
	}
	block, _ := pem.Decode(b)
	return block
}

func privateFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 {
		return nil, errors.New("private enrollment file unavailable")
	}
	return io.ReadAll(io.LimitReader(f, 16385))
}

func publishProfile(spec, seedPath, output string) error {
	invalid := errors.New("invalid dedicated account profile approval")
	raw, err := os.ReadFile(spec)
	if err != nil || len(raw) > 45000 || output == "" {
		return invalid
	}
	var profile wallet.AccountProfile
	if json.Unmarshal(raw, &profile) != nil || profile.Issuer == "" {
		return invalid
	}
	subject, err := uuid.Parse(profile.Subject)
	if err != nil || subject == uuid.Nil || subject.String() != profile.Subject {
		return invalid
	}
	seed, err := privateFile(seedPath)
	if err != nil || len(seed) != ed25519.SeedSize {
		return invalid
	}
	key := ed25519.NewKeyFromSeed(seed)
	clear(seed)
	defer clear(key)
	payload, err := json.Marshal(profile)
	if err != nil {
		return invalid
	}
	delivery := struct{ Payload, Signature []byte }{payload, ed25519.Sign(key, append([]byte(wallet.ProfileDomain), payload...))}
	data, err := json.Marshal(delivery)
	if err != nil || len(data) > 65536 {
		return invalid
	}
	file, err := os.CreateTemp(output, ".account-profile-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(data)
	if err := errors.Join(err, file.Sync(), file.Close()); err != nil {
		return err
	}
	target := filepath.Join(output, profile.Subject+".json")
	if err := os.Link(file.Name(), target); err != nil {
		if errors.Is(err, os.ErrExist) {
			current, readErr := os.ReadFile(target)
			if readErr == nil && bytes.Equal(data, current) {
				return nil
			}
			return errors.New("approved account profile already exists")
		}
		return err
	}
	dir, err := os.Open(output)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
