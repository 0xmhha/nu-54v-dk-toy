package service

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"time"
)

// VerifiedClient is the connection's authenticated leaf and the deadline of
// its longest currently valid verified path. A cached TLS handshake alone is
// insufficient for authorizing a subsequent request.
type VerifiedClient struct {
	Certificate *x509.Certificate
	Fingerprint [32]byte
	ExpiresAt   time.Time
}

// CurrentVerifiedClient evaluates the paths Go verified for this TLS peer.
// The leaf, intermediates, and trust anchor must all be current. Handshake
// paths with another leaf do not confer authority on this connection.
func CurrentVerifiedClient(state *tls.ConnectionState, now time.Time) (VerifiedClient, bool) {
	if state == nil || state.Version != tls.VersionTLS13 || len(state.PeerCertificates) == 0 {
		return VerifiedClient{}, false
	}
	leaf := state.PeerCertificates[0]
	if leaf == nil || len(leaf.Raw) == 0 {
		return VerifiedClient{}, false
	}

	var latest time.Time
	for _, path := range state.VerifiedChains {
		if len(path) == 0 || path[0] == nil || !bytes.Equal(path[0].Raw, leaf.Raw) {
			continue
		}
		deadline := leaf.NotAfter
		for _, certificate := range path {
			if certificate == nil || now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
				deadline = time.Time{}
				break
			}
			if certificate.NotAfter.Before(deadline) {
				deadline = certificate.NotAfter
			}
		}
		if deadline.After(latest) {
			latest = deadline
		}
	}
	if latest.IsZero() {
		return VerifiedClient{}, false
	}
	return VerifiedClient{Certificate: leaf, Fingerprint: sha256.Sum256(leaf.Raw), ExpiresAt: latest}, true
}

func (c VerifiedClient) FingerprintHex() string {
	return hex.EncodeToString(c.Fingerprint[:])
}

// CurrentClientPrincipal also caps the verified path at the enrolled grant's
// expiry. Callers still authorize each operation against its scope.
func CurrentClientPrincipal(state *tls.ConnectionState, credentials map[string]Principal, now time.Time) (Principal, VerifiedClient, bool) {
	return CurrentClientPrincipalFrom(state, func(key string) (Principal, bool) {
		p, ok := credentials[key]
		return p, ok
	}, now)
}

// CurrentClientPrincipalFrom resolves the enrollment at use time. A dynamic
// source must return false whenever its current authority cannot be read.
func CurrentClientPrincipalFrom(state *tls.ConnectionState, lookup func(string) (Principal, bool), now time.Time) (Principal, VerifiedClient, bool) {
	client, ok := CurrentVerifiedClient(state, now)
	if !ok {
		return Principal{}, VerifiedClient{}, false
	}
	principal, ok := lookup(client.FingerprintHex())
	if !ok {
		return Principal{}, VerifiedClient{}, false
	}
	if expiry := time.Unix(principal.ExpiresAt, 0); expiry.Before(client.ExpiresAt) {
		client.ExpiresAt = expiry
	}
	return principal, client, now.Before(client.ExpiresAt)
}
