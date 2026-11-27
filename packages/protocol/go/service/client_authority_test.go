package service

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"
)

func TestCurrentVerifiedClientPath(t *testing.T) {
	now := time.Unix(1600, 0)
	cert := func(name string, start, end int64) *x509.Certificate {
		return &x509.Certificate{Raw: []byte(name), NotBefore: time.Unix(start, 0), NotAfter: time.Unix(end, 0)}
	}
	leaf := cert("leaf", 1500, 2000)
	oldCA := cert("old intermediate", 1500, 1600)
	newCA := cert("new intermediate", 1550, 1900)
	root := cert("root", 1500, 3000)
	state := &tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{leaf},
		VerifiedChains: [][]*x509.Certificate{{leaf, oldCA, root}, {leaf, newCA, root}}}

	client, ok := CurrentVerifiedClient(state, now)
	if !ok || client.Certificate != leaf || client.ExpiresAt != time.Unix(1900, 0) {
		t.Fatalf("only current alternate path should authorize: %+v %t", client, ok)
	}
	state.VerifiedChains = append(state.VerifiedChains, []*x509.Certificate{leaf, cert("short", 1550, 1700), root})
	client, ok = CurrentVerifiedClient(state, now)
	if !ok || client.ExpiresAt != time.Unix(1900, 0) {
		t.Fatal("select the latest deadline without depending on path order")
	}
	if _, ok := CurrentVerifiedClient(state, time.Unix(1900, 0)); ok {
		t.Fatal("last intermediate expired")
	}
	state.VerifiedChains = [][]*x509.Certificate{{cert("other leaf", 1500, 2000), newCA, root}}
	if _, ok := CurrentVerifiedClient(state, now); ok {
		t.Fatal("unrelated leaf authorized")
	}
	state.VerifiedChains = [][]*x509.Certificate{{leaf, cert("future", 1601, 2000), root}}
	if _, ok := CurrentVerifiedClient(state, now); ok {
		t.Fatal("future intermediate authorized")
	}
}
