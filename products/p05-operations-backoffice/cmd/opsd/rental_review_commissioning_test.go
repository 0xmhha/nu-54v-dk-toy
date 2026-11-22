package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/common"
)

func TestRentalReviewCommissioningRequiresCurrentSignedInstallationAndDeployment(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	validUntil := now.Add(time.Hour)
	key := func() *ecdsa.PrivateKey {
		t.Helper()
		value, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	authority, caKey, board := key(), key(), key()
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Hour), NotAfter: validUntil.Add(time.Hour)}
	ca, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	uri, err := url.Parse("urn:nu54:role:board")
	if err != nil {
		t.Fatal(err)
	}
	boardDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{SerialNumber: big.NewInt(2),
		URIs: []*url.URL{uri}, NotBefore: now.Add(-time.Hour), NotAfter: validUntil.Add(time.Hour)},
		caTemplate, &board.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	payer := common.HexToAddress("0x1234")
	profile := commerce.Profile{Chain: 8283, Contract: common.HexToAddress("0x21"), Asset: common.HexToAddress("0x22"),
		Beneficiary: common.HexToAddress("0x23"), Signer: common.HexToAddress("0x24"), MerchantID: [32]byte{25},
		KeyVersion: 3, BeneficiaryVersion: 4}
	const generation = 7
	pin := [32]byte{32}
	grant := make([]byte, 488)
	copy(grant, "N54Q")
	copy(grant[36:], elliptic.Marshal(elliptic.P256(), board.X, board.Y))
	copy(grant[101:], payer[:])
	binary.BigEndian.PutUint64(grant[121:], uint64(now.Add(-time.Minute).Unix()))
	policy := grant[129:]
	for index, value := range []uint64{1, generation, uint64(validUntil.Unix()), 3, 4, 6_500_000} {
		binary.BigEndian.PutUint64(policy[index*8:], value)
	}
	policy[48] = 2
	copy(policy[49:], profile.Contract[:])
	copy(policy[69:], profile.Asset[:])
	copy(policy[89:], profile.Beneficiary[:])
	copy(policy[109:], profile.Signer[:])
	copy(policy[129:], profile.MerchantID[:])
	copy(policy[193:], pin[:])
	binary.BigEndian.PutUint16(grant[484:], uint16(len(ca)))
	binary.BigEndian.PutUint16(grant[486:], uint16(len(boardDER)))
	grant = append(grant, ca...)
	grant = append(grant, boardDER...)
	sign := func(message []byte) []byte {
		t.Helper()
		digest := sha256.Sum256(message)
		r, s, err := ecdsa.Sign(rand.Reader, authority, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		var signature [64]byte
		r.FillBytes(signature[:32])
		s.FillBytes(signature[32:])
		return append(message, signature[:]...)
	}
	grant = sign(grant)
	stableManifest := bytes.Repeat([]byte{35}, 32)
	digest := sha256.Sum256(grant)
	lease := append([]byte("N54V"), digest[:]...)
	lease = append(lease, stableManifest...)
	kit := rentalReviewKit{Grant: grant, Deployment: sign(lease)}
	spki, err := x509.MarshalPKIXPublicKey(&board.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	bound, ok := commissionedRentalReview(kit, &authority.PublicKey, profile, stableManifest, payer, generation, big.NewInt(6_500_000), now)
	if !ok || bound.BoardPin != sha256.Sum256(spki) || bound.TerminalPin != pin || !bound.ExpiresAt.Equal(validUntil) {
		t.Fatal("active installation did not bind the enrolled board and terminal")
	}
	for _, trial := range []struct {
		name       string
		kit        rentalReviewKit
		manifest   []byte
		generation uint64
		amount     int64
		time       time.Time
	}{
		{"cert_grant_without_v2_binding", rentalReviewKit{Grant: grant}, stableManifest, generation, 6_500_000, now},
		{"unrelated_v2_deployment", kit, make([]byte, 32), generation, 6_500_000, now},
		{"old_generation", kit, stableManifest, generation + 1, 6_500_000, now},
		{"enrolled_cap", kit, stableManifest, generation, 6_500_001, now},
		{"expired_installation", kit, stableManifest, generation, 6_500_000, validUntil},
	} {
		t.Run(trial.name, func(t *testing.T) {
			if _, ok := commissionedRentalReview(trial.kit, &authority.PublicKey, profile, trial.manifest,
				payer, trial.generation, big.NewInt(trial.amount), trial.time); ok {
				t.Fatal("commissioning unexpectedly authorized review")
			}
		})
	}

	directory := t.TempDir()
	keyFile, kitFile := filepath.Join(directory, "authority.pem"), filepath.Join(directory, "publication.json")
	authorityDER, err := x509.MarshalPKIXPublicKey(&authority.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: authorityDER}), 0600); err != nil {
		t.Fatal(err)
	}
	publish := func(kits []rentalReviewKit) {
		t.Helper()
		raw, err := json.Marshal(kits)
		if err != nil {
			t.Fatal(err)
		}
		replacement := kitFile + ".new"
		if err := os.WriteFile(replacement, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(replacement, kitFile); err != nil {
			t.Fatal(err)
		}
	}
	publish([]rentalReviewKit{kit})
	t.Setenv("P05_W10_BOARD_COMMISSIONING_FILE", kitFile)
	t.Setenv("P05_W10_INSTALL_AUTHORITY_KEY_FILE", keyFile)
	t.Setenv("P05_PRESENTATION_KEY_FILE", "configured")
	lookup, err := rentalReviewCommissioning()
	if err != nil {
		t.Fatal(err)
	}
	check := func(wanted bool) {
		t.Helper()
		if _, ok := lookup(context.Background(), "registered branch", profile, stableManifest, payer, generation, big.NewInt(6_500_000)); ok != wanted {
			t.Fatalf("active publication = %t, want %t", ok, wanted)
		}
	}
	check(true)
	publish([]rentalReviewKit{kit, kit})
	check(false)
	publish([]rentalReviewKit{})
	check(false)
}
