package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
)

// The customer listener holds a fixed number of connections: the next one is accepted only when one closes, and a
// closed listener stops waiting for a place.
func TestCustomerListenerCapsConnections(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := newLimitListener(inner, 2, 0)
	defer listener.Close()
	accepted := make(chan net.Conn, 4)
	failed := make(chan error, 1)
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				failed <- err
				return
			}
			accepted <- connection
		}
	}()
	dial := func() net.Conn {
		t.Helper()
		connection, err := net.Dial("tcp", inner.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		return connection
	}
	next := func(wait time.Duration) net.Conn {
		select {
		case connection := <-accepted:
			return connection
		case <-time.After(wait):
			return nil
		}
	}
	defer dial().Close()
	defer dial().Close()
	first, second := next(2*time.Second), next(2*time.Second)
	if first == nil || second == nil {
		t.Fatal("the listener did not accept up to its cap")
	}
	defer dial().Close() // a third client connects to the kernel's queue and is not served yet
	if third := next(150 * time.Millisecond); third != nil {
		t.Fatal("a third connection was accepted past the cap")
	}
	// Closing a connection twice gives back one place, not two.
	first.Close()
	first.Close()
	third := next(2 * time.Second)
	if third == nil {
		t.Fatal("a closed connection did not free its place")
	}
	defer dial().Close()
	if fourth := next(150 * time.Millisecond); fourth != nil {
		t.Fatal("one closed connection freed two places")
	}
	second.Close()
	if fourth := next(2 * time.Second); fourth == nil {
		t.Fatal("the second closed connection did not free its place")
	}
	// At the cap again, the accept loop is waiting for a place: closing the listener ends it.
	listener.Close()
	select {
	case err := <-failed:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("accept after close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a closed listener kept waiting for a place")
	}
}

// peerListener hands out connections that say they come from the addresses a test names, so a share is counted
// for more than one address without more than one host.
type peerListener struct {
	net.Listener
	peers chan string
}

type peerConn struct {
	net.Conn
	peer net.Addr
}

func (c peerConn) RemoteAddr() net.Addr { return c.peer }

func (l peerListener) Accept() (net.Conn, error) {
	connection, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return peerConn{Conn: connection, peer: &net.TCPAddr{IP: net.ParseIP(<-l.peers), Port: 4000}}, nil
}

// open is how many connections the listener holds now. (The accept loop keeps one more place while it waits for the
// next connection.)
func (l *limitListener) open() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	total := 0
	for _, count := range l.held {
		total += count
	}
	return total
}

// One address holds its share of the connections and no more: a host that opens connection after connection is
// closed at the door once it has its share, and the places it did not get are there for everyone else.
func TestCustomerListenerSharesConnectionsByAddress(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	peers := make(chan string, 64)
	listener := newLimitListener(peerListener{Listener: inner, peers: peers}, 6, 2)
	defer listener.Close()
	accepted := make(chan net.Conn, 16)
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			accepted <- connection
		}
	}()
	dial := func(from string) net.Conn {
		t.Helper()
		peers <- from
		connection, err := net.Dial("tcp", inner.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { connection.Close() })
		return connection
	}
	next := func(wait time.Duration) net.Conn {
		select {
		case connection := <-accepted:
			return connection
		case <-time.After(wait):
			return nil
		}
	}
	closedByServer := func(connection net.Conn) bool {
		_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, err := connection.Read(make([]byte, 1))
		return errors.Is(err, io.EOF) || (err != nil && !errors.Is(err, os.ErrDeadlineExceeded))
	}
	// One host opens twenty connections: two are served, the others are closed at once and hold no place.
	const greedy = "203.0.113.7"
	var held, refused []net.Conn
	for i := 0; i < 20; i++ {
		client := dial(greedy)
		if i < 2 {
			served := next(2 * time.Second)
			if served == nil {
				t.Fatalf("connection %d of the address was not accepted inside its share", i+1)
			}
			held = append(held, served)
			continue
		}
		refused = append(refused, client)
	}
	if over := next(200 * time.Millisecond); over != nil {
		t.Fatal("a connection past the share of its address was accepted")
	}
	for i, client := range refused {
		if !closedByServer(client) {
			t.Fatalf("connection %d past the share was left open", i+3)
		}
	}
	if listener.open() != 2 || len(listener.slots) > 3 {
		t.Fatalf("%d connections are held (%d places taken), want the 2 the address holds", listener.open(), len(listener.slots))
	}
	// Other addresses still get in, each up to its own share: an IPv6 /64 is one address, like an IPv4 one.
	for _, from := range []string{"198.51.100.1", "198.51.100.2", "2001:db8:1:2::1", "2001:db8:1:2::2"} {
		dial(from)
		if next(2*time.Second) == nil {
			t.Fatalf("%s was not accepted while one address was at its share", from)
		}
	}
	dial("2001:db8:1:2:ffff::3") // a third address of the same /64
	if over := next(200 * time.Millisecond); over != nil {
		t.Fatal("a /64 got more than one address's share")
	}
	// A closed connection gives its address a place back, once.
	held[0].Close()
	held[0].Close()
	dial(greedy)
	if next(2*time.Second) == nil {
		t.Fatal("the address did not get its place back")
	}
	dial(greedy)
	if over := next(200 * time.Millisecond); over != nil {
		t.Fatal("one closed connection gave the address two places")
	}
}

func entranceCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// A connection that never starts its TLS handshake, and one that finished it and then sends nothing, are both cut
// by the entrance's own deadlines, and each gives its place back. So a host cannot keep its share of places by
// opening connections and going quiet for longer than a few seconds.
func TestCustomerEntranceCutsQuietConnections(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the real deadlines of the entrance")
	}
	lifetime, stop := context.WithCancel(context.Background())
	defer stop()
	certificate := entranceCertificate(t)
	listener, err := listenCustomers("127.0.0.1:0", 4)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok\n") })
	server := customerEntranceServer(listener.Addr().String(), mux, certificate, lifetime)
	server.ErrorLog = nil
	go func() { _ = server.ServeTLS(listener, "", "") }()
	defer server.Close()
	address := listener.Addr().String()

	silent, err := net.Dial("tcp", address) // TCP only: no TLS at all
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	idle, err := tls.Dial("tcp", address, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13}) // handshake, then nothing
	if err != nil {
		t.Fatal(err)
	}
	defer idle.Close()
	for deadline := time.Now().Add(2 * time.Second); listener.open() != 2; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%d connections are held for two quiet ones", listener.open())
		}
	}
	started := time.Now()
	cut := func(name string, connection net.Conn, longest time.Duration) {
		t.Helper()
		_ = connection.SetReadDeadline(started.Add(longest))
		if _, err := connection.Read(make([]byte, 1)); err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("%s was still open after %v: %v", name, time.Since(started).Round(time.Millisecond), err)
		}
	}
	cut("a connection without a handshake", silent, 7*time.Second) // 5 seconds: the handshake deadline
	cut("a connection that sends no request", idle, 7*time.Second) // 5 seconds: the request header deadline
	for deadline := time.Now().Add(2 * time.Second); listener.open() != 0; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%d connections are still held after the quiet ones were cut", listener.open())
		}
	}
	if waited := time.Since(started); waited < 4*time.Second {
		t.Fatalf("the quiet connections were cut after %v: the deadlines are not the entrance's", waited)
	}
}

type wiredFunds struct{}

func (wiredFunds) PayerBalance(context.Context, string, common.Address) (common.Address, *big.Int, error) {
	return common.Address{}, nil, service.Capability
}

// Everything the entrance is held back by is set where opsd builds it: each request limit, the cap on requests
// served at once, the cap on balance reads, the gate, the chain reader, the caps on connections and the deadlines.
// A line dropped from any of them fails here, and so does a serveCommerce that no longer builds the entrance this way.
func TestCustomerEntranceIsWired(t *testing.T) {
	gate := &service.RecoveryGate{}
	entrance := newCustomerEntrance(" help@operator.example ", gate, wiredFunds{})
	fields := reflect.ValueOf(entrance).Elem()
	limiters := 0
	for i := 0; i < fields.NumField(); i++ {
		if fields.Type().Field(i).Type != reflect.TypeOf((*commerce.RateLimiter)(nil)) {
			continue
		}
		limiters++
		if fields.Field(i).IsNil() {
			t.Errorf("the entrance has no %s", fields.Type().Field(i).Name)
		}
	}
	if limiters != 11 {
		t.Errorf("the entrance has %d request limits; this test knows 11", limiters)
	}
	if cap(entrance.InFlight) != customerInFlight || cap(entrance.BalanceReads) != customerBalanceReads || customerBalanceReads >= customerInFlight {
		t.Errorf("requests at once: %d (want %d), balance reads among them: %d (want %d)", cap(entrance.InFlight), customerInFlight, cap(entrance.BalanceReads), customerBalanceReads)
	}
	if entrance.Gate == nil || entrance.Funds == nil || entrance.OperatorContact != " help@operator.example " {
		t.Errorf("gate set: %v, chain reader set: %v, contact %q", entrance.Gate != nil, entrance.Funds != nil, entrance.OperatorContact)
	}
	if bare := newCustomerEntrance("", nil, nil); bare.Gate != nil || bare.Funds != nil || bare.Limiter == nil {
		t.Errorf("an entrance without a gate and a chain reader: gate %v, funds %v", bare.Gate != nil, bare.Funds)
	}

	listener, err := listenCustomers("127.0.0.1:0", 7)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if cap(listener.slots) != customerConnections || listener.share != 7 {
		t.Errorf("the listener holds %d connections, %d for an address", cap(listener.slots), listener.share)
	}
	server := customerEntranceServer("127.0.0.1:0", http.NewServeMux(), tls.Certificate{}, context.Background())
	if server.ReadHeaderTimeout != 5*time.Second || server.ReadTimeout != 8*time.Second || server.WriteTimeout != 10*time.Second ||
		server.IdleTimeout != 8*time.Second || server.MaxHeaderBytes != 8<<10 ||
		server.TLSConfig.MinVersion != tls.VersionTLS13 || server.TLSConfig.ClientAuth != tls.NoClientCert {
		t.Errorf("the entrance's server: %+v", server)
	}

	t.Setenv("P05_CUSTOMER_CONNECTIONS_PER_ADDRESS", "")
	if share, err := connectionShare(); err != nil || share != customerConnectionShare {
		t.Errorf("the default share of an address: %d, %v", share, err)
	}
	t.Setenv("P05_CUSTOMER_CONNECTIONS_PER_ADDRESS", "8")
	if share, err := connectionShare(); err != nil || share != 8 {
		t.Errorf("a share of 8: %d, %v", share, err)
	}
	for _, bad := range []string{"0", "-1", "1025", "08", "eight", "8 "} {
		t.Setenv("P05_CUSTOMER_CONNECTIONS_PER_ADDRESS", bad)
		if share, err := connectionShare(); err == nil {
			t.Errorf("a share of %q was taken as %d", bad, share)
		}
	}

	source, err := os.ReadFile("commerce.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []string{"customer = newCustomerEntrance(os.Getenv(\"P05_CUSTOMER_OPERATOR_CONTACT\"), gate, funds)", "funds = forwarder",
		"share, err := connectionShare()", "handlers.RegisterCustomer(entrance)", "customerServer = customerEntranceServer(customerAddr, entrance, certificate, lifetime)",
		"listener, err := listenCustomers(customerAddr, share)", "customerServer.ServeTLS(listener, \"\", \"\")", "customer.Limited.Drain()"} {
		if !strings.Contains(string(source), call) {
			t.Errorf("serveCommerce no longer has %q", call)
		}
	}
}
