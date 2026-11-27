package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/common"
)

type rentalReviewKit struct {
	Grant      []byte `json:"grant"`
	Deployment []byte `json:"deploymentProof"`
}

func readRentalCommissioningFile(path string, max int64) ([]byte, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > max {
		return nil, errors.New("rental commissioning must be a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil || int64(len(data)) > max {
		return nil, errors.New("rental commissioning exceeds its read limit")
	}
	return data, nil
}

func readRentalReviewKits(path string) ([]rentalReviewKit, error) {
	data, err := readRentalCommissioningFile(path, 512<<10)
	if err != nil {
		return nil, err
	}
	read := json.NewDecoder(bytes.NewReader(data))
	read.DisallowUnknownFields()
	var kits []rentalReviewKit
	if read.Decode(&kits) != nil || kits == nil || len(kits) > 128 || read.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid commissioned rental review publication")
	}
	return kits, nil
}

func validInstallationSignature(key *ecdsa.PublicKey, signed []byte) bool {
	if len(signed) <= 64 {
		return false
	}
	message, signature := signed[:len(signed)-64], signed[len(signed)-64:]
	digest := sha256.Sum256(message)
	return ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:]))
}

func commissionedRentalReview(kit rentalReviewKit, authority *ecdsa.PublicKey, profile commerce.Profile,
	manifest []byte, payer common.Address, generation uint64, amount *big.Int, now time.Time) (commerce.RentalReviewCommissioning, bool) {
	var empty commerce.RentalReviewCommissioning
	if len(kit.Grant) < 488+64 || len(kit.Grant) > 1600 || len(kit.Deployment) != 132 ||
		string(kit.Grant[:4]) != "N54Q" || string(kit.Deployment[:4]) != "N54V" ||
		len(manifest) != 32 || !bytes.Equal(kit.Deployment[36:68], manifest) {
		return empty, false
	}
	grantHash := sha256.Sum256(kit.Grant)
	if !bytes.Equal(kit.Deployment[4:36], grantHash[:]) || !validInstallationSignature(authority, kit.Deployment) ||
		!validInstallationSignature(authority, kit.Grant) || !bytes.Equal(kit.Grant[101:121], payer[:]) {
		return empty, false
	}
	policy := kit.Grant[129:484]
	maxAmount := binary.BigEndian.Uint64(policy[40:48])
	if amount == nil || !amount.IsUint64() || amount.Sign() <= 0 || amount.Uint64() > maxAmount || maxAmount > 30_000_000 {
		return empty, false
	}
	putAt := binary.BigEndian.Uint64(kit.Grant[121:129])
	endsAt := binary.BigEndian.Uint64(policy[16:24])
	if profile.Chain != 8283 || generation == 0 || binary.BigEndian.Uint64(policy[:8]) == 0 ||
		binary.BigEndian.Uint64(policy[8:16]) != generation || policy[48] != 2 ||
		putAt == 0 || putAt > uint64(now.Unix()) || endsAt <= uint64(now.Unix()) || endsAt-putAt > 86400 ||
		binary.BigEndian.Uint64(policy[24:32]) != profile.KeyVersion ||
		binary.BigEndian.Uint64(policy[32:40]) != profile.BeneficiaryVersion ||
		!bytes.Equal(policy[49:69], profile.Contract[:]) || !bytes.Equal(policy[69:89], profile.Asset[:]) ||
		!bytes.Equal(policy[89:109], profile.Beneficiary[:]) || !bytes.Equal(policy[109:129], profile.Signer[:]) ||
		!bytes.Equal(policy[129:161], profile.MerchantID[:]) {
		return empty, false
	}
	caLen := int(binary.BigEndian.Uint16(kit.Grant[484:486]))
	certLen := int(binary.BigEndian.Uint16(kit.Grant[486:488]))
	if caLen == 0 || certLen == 0 || 488+caLen+certLen+64 != len(kit.Grant) {
		return empty, false
	}
	ca, caErr := x509.ParseCertificate(kit.Grant[488 : 488+caLen])
	cert, certErr := x509.ParseCertificate(kit.Grant[488+caLen : 488+caLen+certLen])
	if caErr != nil || certErr != nil || cert.CheckSignatureFrom(ca) != nil || len(cert.URIs) != 1 ||
		cert.URIs[0].String() != "urn:nu54:role:board" || now.Before(cert.NotBefore) ||
		now.Before(ca.NotBefore) || uint64(cert.NotAfter.Unix()) < endsAt || uint64(ca.NotAfter.Unix()) < endsAt {
		return empty, false
	}
	board, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok || board.Curve != elliptic.P256() {
		return empty, false
	}
	public := elliptic.Marshal(board.Curve, board.X, board.Y)
	canonical, err := x509.MarshalPKIXPublicKey(board)
	if err != nil || !bytes.Equal(public, kit.Grant[36:101]) || !bytes.Equal(canonical, cert.RawSubjectPublicKeyInfo) {
		return empty, false
	}
	terminal := [32]byte(policy[193:225])
	boardPin := sha256.Sum256(canonical)
	if terminal == ([32]byte{}) || terminal == boardPin {
		return empty, false
	}
	return commerce.RentalReviewCommissioning{
		BoardPin: boardPin, TerminalPin: terminal, Signer: [65]byte(public), ExpiresAt: time.Unix(int64(endsAt), 0),
	}, true
}

func rentalReviewCommissioning() (commerce.RentalReviewLookup, error) {
	path := os.Getenv("P05_W10_BOARD_COMMISSIONING_FILE")
	if path == "" {
		return nil, nil
	}
	keyPath := os.Getenv("P05_W10_INSTALL_AUTHORITY_KEY_FILE")
	if !filepath.IsAbs(path) || !filepath.IsAbs(keyPath) || os.Getenv("P05_PRESENTATION_KEY_FILE") == "" {
		return nil, errors.New("rental review commissioning requires absolute paths and the enrolled P05 presentation authority")
	}
	keyPEM, err := readRentalCommissioningFile(keyPath, 4096)
	if err != nil {
		return nil, err
	}
	block, remainder := pem.Decode(keyPEM)
	if block == nil || block.Type != "PUBLIC KEY" || len(bytes.TrimSpace(remainder)) != 0 {
		return nil, errors.New("invalid W10 installation authority key")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	authority, ok := key.(*ecdsa.PublicKey)
	if err != nil || !ok || authority.Curve != elliptic.P256() {
		return nil, errors.New("W10 commissioning requires the enrolled P-256 installation authority")
	}
	if _, err := readRentalReviewKits(path); err != nil {
		return nil, err
	}
	return func(ctx context.Context, _ string, profile commerce.Profile, manifest []byte, payer common.Address,
		generation uint64, amount *big.Int) (commerce.RentalReviewCommissioning, bool) {
		kits, err := readRentalReviewKits(path)
		if err != nil || ctx.Err() != nil {
			return commerce.RentalReviewCommissioning{}, false
		}
		var found commerce.RentalReviewCommissioning
		for _, item := range kits {
			if current, ok := commissionedRentalReview(item, authority, profile, manifest, payer, generation, amount, time.Now()); ok {
				if found != (commerce.RentalReviewCommissioning{}) {
					return commerce.RentalReviewCommissioning{}, false
				}
				found = current
			}
		}
		return found, found != (commerce.RentalReviewCommissioning{})
	}, nil
}
