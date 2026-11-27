// rental-collect runs a private one-shot P01 journal collection and P07 assessment.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/ble"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core/ops"
)

type enrolledEndpoint struct {
	Origin, Roots, Certificate, KeyFile, SPKI string
}

type enrollment struct {
	Binding, FirstNonce, BoardSigner, Operator string
	Stream, StreamGeneration                   string
	RegisteredPeer                             string
	RandomPeer                                 bool
	Issuer, Reconciler                         enrolledEndpoint
	Previous                                   *rentalv1.Anchor
}

var errEnrollment = errors.New("qualified collector enrollment is required")

func privateInput(path string, max int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !filepath.IsAbs(path) || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errEnrollment
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errEnrollment
	}
	defer f.Close()
	current, err := f.Stat()
	if err != nil || !os.SameFile(info, current) || current.Mode().Perm()&0077 != 0 {
		return nil, errEnrollment
	}
	body, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || int64(len(body)) > max {
		return nil, errEnrollment
	}
	return body, nil
}

func hexField(src string, target []byte) bool {
	if len(src) != 2*len(target) {
		return false
	}
	n, err := hex.Decode(target, []byte(src))
	return err == nil && n == len(target)
}

func (e enrolledEndpoint) tlsInputs() (*x509.CertPool, tls.Certificate, [32]byte, error) {
	var cert tls.Certificate
	var pin [32]byte
	if !filepath.IsAbs(e.Roots) || !filepath.IsAbs(e.Certificate) || !hexField(e.SPKI, pin[:]) {
		return nil, cert, pin, errEnrollment
	}
	key, err := privateInput(e.KeyFile, 32*1024)
	if err != nil {
		return nil, cert, pin, err
	}
	defer clear(key)
	material, err := os.ReadFile(e.Certificate)
	if err != nil {
		return nil, cert, pin, errEnrollment
	}
	cert, err = tls.X509KeyPair(material, key)
	if err != nil {
		return nil, cert, pin, errEnrollment
	}
	material, err = os.ReadFile(e.Roots)
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(material) {
		return nil, cert, pin, errEnrollment
	}
	return roots, cert, pin, nil
}

func uuid(id []byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:16])
}

func run(ctx context.Context, path, recordKey string, connect func(string, bool) (ble.Transport, error), stdout io.Writer) error {
	if recordKey != "" && !ops.ValidRentalCollectionRequestKey(recordKey) {
		return errEnrollment
	}
	wire, err := privateInput(path, 16*1024)
	if err != nil {
		return err
	}
	var input enrollment
	reader := json.NewDecoder(bytes.NewReader(wire))
	reader.DisallowUnknownFields()
	if reader.Decode(&input) != nil || reader.Decode(new(any)) != io.EOF || input.RegisteredPeer == "" ||
		(input.Previous != nil && !input.Previous.Valid()) {
		return errEnrollment
	}
	scope := ops.RentalCollectionScope{Stream: input.Stream, StreamGeneration: input.StreamGeneration}
	if !hexField(input.Binding, scope.Binding[:]) || !hexField(input.FirstNonce, scope.FirstNonce[:]) ||
		!hexField(input.BoardSigner, scope.BoardSigner[:]) || !hexField(input.Operator, scope.Operator[:]) {
		return errEnrollment
	}
	issuerRoots, issuerIdentity, issuerPin, err := input.Issuer.tlsInputs()
	if err != nil {
		return err
	}
	readerRoots, readerIdentity, readerPin, err := input.Reconciler.tlsInputs()
	if err != nil {
		return err
	}
	issuerLeaf, issuerErr := x509.ParseCertificate(issuerIdentity.Certificate[0])
	readerLeaf, readerErr := x509.ParseCertificate(readerIdentity.Certificate[0])
	if issuerErr != nil || readerErr != nil || bytes.Equal(issuerLeaf.RawSubjectPublicKeyInfo, readerLeaf.RawSubjectPublicKeyInfo) {
		return errEnrollment
	}
	issuer, err := ops.NewRentalCollectorClient(input.Issuer.Origin, issuerRoots, issuerIdentity, issuerPin,
		uuid(scope.Binding[8:24]), uuid(scope.Binding[24:40]), binary.BigEndian.Uint64(scope.Binding[:8]))
	if err != nil {
		return err
	}
	defer issuer.Close()
	reconciler, err := core.NewRentalEvidenceReader(input.Reconciler.Origin, readerRoots, readerIdentity, readerPin)
	if err != nil {
		return err
	}
	defer reconciler.Close()
	link, err := connect(input.RegisteredPeer, input.RandomPeer)
	if err != nil {
		return ops.ErrRentalCollection
	}
	collected, collectErr := ops.CollectRentalJournal(ctx, link, issuer, scope, time.Now)
	closeErr := link.Close()
	if collectErr != nil || closeErr != nil {
		return ops.ErrRentalCollection
	}
	if recordKey != "" {
		if err := issuer.Record(ctx, recordKey, collected.Request); err != nil {
			return err
		}
		_, err := io.WriteString(stdout, "{\"recorded\":true}\n")
		return err
	}
	collected.Request.Previous = input.Previous
	result, err := reconciler.Read(ctx, collected.Request, collected.Expected, collected.Count,
		collected.FrozenAt, collected.ExpiresAt, time.Now())
	if err != nil {
		return err
	}
	// A private station receives only the validated decision; nothing is written
	// to the P05 ledger by this collection command.
	return json.NewEncoder(stdout).Encode(struct {
		Status          string `json:"status"`
		PendingPayments *int   `json:"pendingPayments,omitempty"`
		PendingLimits   *int   `json:"pendingLimits,omitempty"`
	}{result.Status, result.PendingPayments, result.PendingLimits})
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := run(ctx, os.Getenv("P05_RENTAL_COLLECTOR_ENROLLMENT"), os.Getenv("P05_RENTAL_COLLECTOR_RECORD_KEY"), func(peer string, random bool) (ble.Transport, error) {
		return ble.ConnectRegistered(peer, random)
	}, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
