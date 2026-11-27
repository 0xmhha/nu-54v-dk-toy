package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/crypto"
)

func loadRentalWriterEvidenceReader() (*core.RentalEvidenceReader, error) {
	var config struct{ Origin, Roots, Certificate, KeyFile, SPKI string }
	if err := configJSON("P05_RENTAL_WRITER_P07_CONFIG", &config); err != nil {
		return nil, err
	}
	pin, err := hex.DecodeString(config.SPKI)
	if err != nil || len(pin) != 32 || !filepath.IsAbs(config.Roots) || !filepath.IsAbs(config.Certificate) || !filepath.IsAbs(config.KeyFile) {
		return nil, errors.New("invalid rental writer P07 configuration")
	}
	key, err := os.Lstat(config.KeyFile)
	if err != nil || !key.Mode().IsRegular() || key.Mode().Perm()&0077 != 0 {
		return nil, errors.New("the rental writer needs a private P07 client key")
	}
	identity, err := tls.LoadX509KeyPair(config.Certificate, config.KeyFile)
	if err != nil {
		return nil, errors.New("the rental writer P07 client identity could not be loaded")
	}
	pem, err := os.ReadFile(config.Roots)
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("the rental writer needs a registered P07 trust root")
	}
	return core.NewRentalEvidenceReader(config.Origin, roots, identity, [32]byte(pin))
}

func openRentalWriter(ctx context.Context, dsn string, rental *rentalController, reader *core.RentalEvidenceReader) (*commerce.RentalProofWriter, error) {
	return commerce.OpenRentalProofWriter(ctx, dsn, reader, rental.deployment, rental.manifest, crypto.PubkeyToAddress(rental.operator.PublicKey))
}
