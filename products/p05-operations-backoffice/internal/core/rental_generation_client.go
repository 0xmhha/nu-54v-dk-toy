package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

var ErrRentalEvidence = errors.New("registered rental evidence source unavailable")

type RentalEvidenceReader struct {
	client            *http.Client
	endpoint          string
	closeEndpoint     string
	callerFingerprint [32]byte
}

// NewRentalEvidenceReader binds one dedicated P05 client identity to one P07
// HTTPS origin and its approved leaf public key.
func NewRentalEvidenceReader(origin string, roots *x509.CertPool, identity tls.Certificate, pin [32]byte) (*RentalEvidenceReader, error) {
	endpoint, client, err := NewPinnedRentalHTTP(origin, roots, identity, pin)
	if err != nil {
		return nil, err
	}
	endpoint.Path = rentalv1.ReconcilePath
	reconcile := endpoint.String()
	endpoint.Path = rentalv1.CloseAnchorPath
	return &RentalEvidenceReader{client: client, endpoint: reconcile, closeEndpoint: endpoint.String(),
		callerFingerprint: sha256.Sum256(identity.Certificate[0])}, nil
}

// NewPinnedRentalHTTP creates a dedicated mTLS client for one registered
// private rental service. Redirects and default proxy resolution are disabled.
func NewPinnedRentalHTTP(origin string, roots *x509.CertPool, identity tls.Certificate, pin [32]byte) (*url.URL, *http.Client, error) {
	endpoint, err := url.Parse(origin)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil ||
		(endpoint.Path != "" && endpoint.Path != "/") || endpoint.RawPath != "" || endpoint.RawQuery != "" ||
		endpoint.ForceQuery || endpoint.Fragment != "" || roots == nil || len(identity.Certificate) == 0 ||
		identity.PrivateKey == nil || pin == ([32]byte{}) {
		return nil, nil, ErrRentalEvidence
	}
	config := &tls.Config{
		MinVersion: tls.VersionTLS13, RootCAs: roots.Clone(), Certificates: []tls.Certificate{identity},
		VerifyConnection: func(connection tls.ConnectionState) error {
			if len(connection.VerifiedChains) == 0 || len(connection.PeerCertificates) == 0 ||
				sha256.Sum256(connection.PeerCertificates[0].RawSubjectPublicKeyInfo) != pin {
				return ErrRentalEvidence
			}
			return nil
		},
	}
	client := &http.Client{
		Transport:     &http.Transport{TLSClientConfig: config, MaxIdleConnsPerHost: 1},
		Timeout:       17 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return endpoint, client, nil
}

func (r *RentalEvidenceReader) Close() {
	r.client.CloseIdleConnections()
}

// Read compares the answer with independently authenticated P01 commitments
// and the pinned P07 profile. It applies no ledger change.
func (r *RentalEvidenceReader) Read(ctx context.Context, body rentalv1.Request, expected rentalv1.Binding,
	count int, frozenAt, freezeExpiry, now time.Time) (rentalv1.Assessment, error) {
	started := time.Now()
	if r == nil || r.client == nil || len(body.Projection) < 277 || len(body.Projection) > 1285 ||
		len(body.Receipt) != rentalv1.FreezeReceiptSize || !freezeExpiry.After(now) || frozenAt.After(now) {
		return rentalv1.Assessment{}, ErrRentalEvidence
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return rentalv1.Assessment{}, ErrRentalEvidence
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(encoded))
	if err != nil {
		return rentalv1.Assessment{}, ErrRentalEvidence
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cache-Control", "no-store")
	response, err := r.client.Do(request)
	if err != nil {
		return rentalv1.Assessment{}, ErrRentalEvidence
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.TLS == nil || len(response.TLS.VerifiedChains) == 0 {
		return rentalv1.Assessment{}, ErrRentalEvidence
	}
	reply, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(reply) > 4096 {
		return rentalv1.Assessment{}, ErrRentalEvidence
	}
	decoder := json.NewDecoder(bytes.NewReader(reply))
	decoder.DisallowUnknownFields()
	var result rentalv1.Assessment
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF ||
		result.Validate(expected, count, frozenAt, freezeExpiry, now.Add(time.Since(started))) != nil {
		return rentalv1.Assessment{}, ErrRentalEvidence
	}
	if result.PendingCount != nil && (body.IncludePendingPaymentEntries != (result.PendingPaymentEntries != nil) || (body.Previous == nil) != (result.Previous == nil) ||
		(body.Previous != nil && (result.Previous == nil || *body.Previous != *result.Previous))) {
		return rentalv1.Assessment{}, ErrRentalEvidence
	}
	if result.PendingPaymentEntries != nil {
		// Positions bind to the exact projection sent with the verified freeze.
		// An asserted limit change or a pre-sign burn can never become a sale.
		projection := body.Projection[277:]
		for index := 0; index < 8; index++ {
			if *result.PendingPaymentEntries&(1<<index) == 0 {
				continue
			}
			offset := 126 * index
			if len(projection) < offset+126 || projection[offset] != 1 || projection[offset+1] != 1 {
				return rentalv1.Assessment{}, ErrRentalEvidence
			}
		}
	}
	return result, nil
}

// CallerFingerprint identifies the same registered TLS client used at P07.
func (r *RentalEvidenceReader) CallerFingerprint() [32]byte { return r.callerFingerprint }

func (r *RentalEvidenceReader) ConfirmCloseAnchor(ctx context.Context, expected rentalv1.CloseAnchorRequest) (rentalv1.CloseAnchorConfirmation, error) {
	var empty rentalv1.CloseAnchorConfirmation
	if r == nil || r.client == nil || !expected.Valid(time.Now()) {
		return empty, ErrRentalEvidence
	}
	raw, err := json.Marshal(expected)
	if err != nil {
		return empty, ErrRentalEvidence
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.closeEndpoint, bytes.NewReader(raw))
	if err != nil {
		return empty, ErrRentalEvidence
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cache-Control", "no-store")
	response, err := r.client.Do(request)
	if err != nil {
		return empty, ErrRentalEvidence
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return empty, ErrRentalEvidence
	}
	reply, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(reply) > 4096 {
		return empty, ErrRentalEvidence
	}
	now := time.Now()
	peer, ok := service.CurrentVerifiedClient(response.TLS, now)
	if !ok {
		return empty, ErrRentalEvidence
	}
	var answer rentalv1.CloseAnchorConfirmation
	decoder := json.NewDecoder(bytes.NewReader(reply))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&answer) != nil || decoder.Decode(new(any)) != io.EOF || !answer.ValidFor(expected, now) ||
		answer.ExpiresAt > peer.ExpiresAt.Unix() {
		return empty, ErrRentalEvidence
	}
	return answer, nil
}
