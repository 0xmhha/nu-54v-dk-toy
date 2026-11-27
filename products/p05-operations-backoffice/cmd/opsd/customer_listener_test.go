package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
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
	for _, held := range l.held {
		total += len(held)
	}
	return total
}

// One address holds its share of the connections and no more. When it opens one more, the connection it has held
// longest is closed and the new one is served, so a host that holds its address's share for nothing does not keep
// the next customer of that address out; however many it opens, it holds its share and the places beyond it are
// there for everyone else.
func TestCustomerListenerSharesConnectionsByAddress(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	peers := make(chan string, 64)
	listener := newLimitListener(peerListener{Listener: inner, peers: peers}, 8, 2)
	defer listener.Close()
	accepted := make(chan net.Conn, 32)
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
	closedByServer := func(connection net.Conn, wait time.Duration) bool {
		_ = connection.SetReadDeadline(time.Now().Add(wait))
		_, err := connection.Read(make([]byte, 1))
		return errors.Is(err, io.EOF) || (err != nil && !errors.Is(err, os.ErrDeadlineExceeded))
	}
	// One host opens twenty connections, one after the other. Each is served; from the third on the one it has
	// held longest is closed for it, so it never holds more than two.
	const greedy = "203.0.113.7"
	var clients, served []net.Conn
	for i := 0; i < 20; i++ {
		clients = append(clients, dial(greedy))
		connection := next(2 * time.Second)
		if connection == nil {
			t.Fatalf("connection %d of the address was not accepted", i+1)
		}
		served = append(served, connection)
		if open := listener.open(); open > 2 {
			t.Fatalf("the address holds %d connections after its connection %d, past its share of 2", open, i+1)
		}
	}
	for i, client := range clients[:18] {
		if !closedByServer(client, 2*time.Second) {
			t.Fatalf("connection %d was left open though the address opened two after it", i+1)
		}
	}
	for i, client := range clients[18:] {
		if closedByServer(client, 100*time.Millisecond) {
			t.Fatalf("connection %d, one of the two newest, was closed", i+19)
		}
	}
	if listener.open() != 2 {
		t.Fatalf("%d connections are held, want the 2 the address holds", listener.open())
	}
	// The connections the server closed gave their places back once the server's side was closed too.
	for _, connection := range served[:18] {
		connection.Close()
		connection.Close()
	}
	if taken := len(listener.slots); taken > 3 {
		t.Fatalf("%d places are taken for 2 connections", taken)
	}
	// Other addresses get in, each up to its own share: an IPv6 /64 is one address, like an IPv4 one.
	others := map[string]net.Conn{}
	for _, from := range []string{"198.51.100.1", "198.51.100.2", "2001:db8:1:2::1", "2001:db8:1:2::2"} {
		others[from] = dial(from)
		if next(2*time.Second) == nil {
			t.Fatalf("%s was not accepted while one address was at its share", from)
		}
	}
	for i, client := range clients[18:] {
		if closedByServer(client, 100*time.Millisecond) {
			t.Fatalf("connection %d of the first address was closed for another address", i+19)
		}
	}
	// A third address of the same /64 takes the place of the first of that /64, and of nobody else.
	dial("2001:db8:1:2:ffff::3")
	if next(2*time.Second) == nil {
		t.Fatal("a third connection of a /64 was not accepted")
	}
	if !closedByServer(others["2001:db8:1:2::1"], 2*time.Second) || closedByServer(others["2001:db8:1:2::2"], 100*time.Millisecond) ||
		closedByServer(others["198.51.100.1"], 100*time.Millisecond) {
		t.Fatal("a /64 at its share did not give up exactly the connection it held longest")
	}
	// A closed connection gives its address a place back, once: the next one of the address closes nobody.
	served[18].Close()
	served[18].Close()
	dial(greedy)
	if next(2*time.Second) == nil {
		t.Fatal("the address did not get its place back")
	}
	if closedByServer(clients[19], 100*time.Millisecond) {
		t.Fatal("a connection was closed for one that had a free place")
	}
}

// A connection is not kept past its lifetime, whatever is sent on it, and gives its place back then.
func TestCustomerListenerEndsOldConnections(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := newLimitListener(inner, 4, 2)
	listener.lifetime = 300 * time.Millisecond
	defer listener.Close()
	served := make(chan net.Conn, 1)
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() { // an echo that would go on for as long as the client does
				_, _ = io.Copy(connection, connection)
				connection.Close()
			}()
			select {
			case served <- connection:
			default:
			}
		}
	}()
	client, err := net.Dial("tcp", inner.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	<-served
	started := time.Now()
	for {
		_ = client.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err := client.Write([]byte("x")); err != nil {
			break
		}
		if _, err := client.Read(make([]byte, 1)); err != nil {
			break
		}
		if time.Since(started) > 2*time.Second {
			t.Fatal("a connection that keeps sending outlived its lifetime")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if lived := time.Since(started); lived < 250*time.Millisecond {
		t.Fatalf("the connection was closed after %v, before its lifetime", lived)
	}
	for deadline := time.Now().Add(2 * time.Second); listener.open() != 0; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%d connections are still held after the lifetime", listener.open())
		}
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
	if limiters != 14 {
		t.Errorf("the entrance has %d request limits; this test knows 14", limiters)
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
	if cap(listener.slots) != customerConnections || listener.share != 7 || listener.lifetime != customerConnectionLifetime {
		t.Errorf("the listener holds %d connections, %d for an address, each for %v", cap(listener.slots), listener.share, listener.lifetime)
	}
	server := customerEntranceServer("127.0.0.1:0", http.NewServeMux(), tls.Certificate{}, context.Background())
	if server.ReadHeaderTimeout != 5*time.Second || server.ReadTimeout != 8*time.Second || server.WriteTimeout != 10*time.Second ||
		server.IdleTimeout != 8*time.Second || server.MaxHeaderBytes != 8<<10 ||
		server.TLSConfig.MinVersion != tls.VersionTLS13 || server.TLSConfig.ClientAuth != tls.NoClientCert ||
		server.TLSNextProto == nil || len(server.TLSNextProto) != 0 || !reflect.DeepEqual(server.TLSConfig.NextProtos, []string{"http/1.1"}) || server.ConnContext == nil {
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

// healthOver asks the health route over an open connection and reports whether it was answered.
func healthOver(connection net.Conn, reader *bufio.Reader) bool {
	_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.WriteString(connection, "GET /healthz HTTP/1.1\r\nHost: entrance\r\n\r\n"); err != nil {
		return false
	}
	answer, err := http.ReadResponse(reader, nil)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, answer.Body)
	answer.Body.Close()
	return answer.StatusCode == http.StatusOK
}

// One host must not keep the customers of its address out by holding connections. It opens as many as an address
// may hold, keeps each alive with the health route (a request every six seconds, inside the idle deadline of
// eight), and once a second opens again whatever was closed. After that has lasted longer than the idle deadline,
// twenty customers of the same address connect, one every 200 milliseconds, each for one request as the app does.
func TestCustomerEntranceIsNotHeldByOneHost(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the real deadlines of the entrance")
	}
	lifetime, stop := context.WithCancel(context.Background())
	defer stop()
	listener, err := listenCustomers("127.0.0.1:0", customerConnectionShare)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	commerce.HTTP{Customer: newCustomerEntrance("", nil, nil)}.RegisterCustomer(mux)
	server := customerEntranceServer(listener.Addr().String(), mux, entranceCertificate(t), lifetime)
	server.ErrorLog = log.New(io.Discard, "", 0)
	go func() { _ = server.ServeTLS(listener, "", "") }()
	defer server.Close()
	address := listener.Addr().String()
	dial := func() (net.Conn, *bufio.Reader) {
		connection, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", address, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13})
		if err != nil {
			return nil, nil
		}
		return connection, bufio.NewReader(connection)
	}

	type hold struct {
		connection net.Conn
		reader     *bufio.Reader
		asked      time.Time
	}
	holds := make([]hold, customerConnectionShare)
	var reopened atomic.Int32
	quit, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			for i := range holds {
				held := &holds[i]
				if held.connection != nil && time.Since(held.asked) >= 6*time.Second {
					held.asked = time.Now()
					if !healthOver(held.connection, held.reader) {
						held.connection.Close()
						held.connection = nil
					}
				}
				if held.connection != nil { // closed by the server meanwhile?
					_ = held.connection.SetReadDeadline(time.Now().Add(time.Millisecond))
					if _, err := held.reader.Peek(1); err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
						held.connection.Close()
						held.connection = nil
					}
				}
				if held.connection == nil {
					if held.connection, held.reader = dial(); held.connection != nil {
						reopened.Add(1)
						held.asked = time.Now()
						if !healthOver(held.connection, held.reader) {
							held.connection.Close()
							held.connection = nil
						}
					}
				}
			}
			select {
			case <-quit:
				for i := range holds {
					if holds[i].connection != nil {
						holds[i].connection.Close()
					}
				}
				return
			case <-tick.C:
			}
		}
	}()
	time.Sleep(9 * time.Second) // past the idle deadline: the hold is kept by requests, not by the deadline
	if open := listener.open(); open < customerConnectionShare-1 {
		t.Fatalf("the host holds %d connections, not its address's share of %d", open, customerConnectionShare)
	}
	served := 0
	for i := 0; i < 20; i++ {
		if connection, reader := dial(); connection != nil {
			if healthOver(connection, reader) {
				served++
			}
			connection.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	close(quit)
	<-done
	t.Logf("one host held %d connections of its address for 13 seconds and opened %d in all: %d of 20 customers of that address were served",
		customerConnectionShare, reopened.Load(), served)
	if served != 20 {
		t.Fatalf("%d of 20 customers of the address were served while one host held its connections", served)
	}
}

// The entrance speaks HTTP/1.1 only, one request at a time on a connection, and closes a connection after a few
// requests. A client that offers HTTP/2 is answered over HTTP/1.1, so one connection cannot carry many requests at
// once past the caps on connections.
func TestCustomerEntranceServesOneRequestAtATime(t *testing.T) {
	lifetime, stop := context.WithCancel(context.Background())
	defer stop()
	listener, err := listenCustomers("127.0.0.1:0", 4)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	commerce.HTTP{Customer: newCustomerEntrance("", nil, nil)}.RegisterCustomer(mux)
	server := customerEntranceServer(listener.Addr().String(), mux, entranceCertificate(t), lifetime)
	server.ErrorLog = log.New(io.Discard, "", 0)
	go func() { _ = server.ServeTLS(listener, "", "") }()
	defer server.Close()
	address := listener.Addr().String()

	offered, err := tls.Dial("tcp", address, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13, NextProtos: []string{"h2", "http/1.1"}})
	if err != nil {
		t.Fatal(err)
	}
	if protocol := offered.ConnectionState().NegotiatedProtocol; protocol != "http/1.1" {
		t.Fatalf("a client that offers HTTP/2 was given %q", protocol)
	}
	offered.Close()
	only, err := tls.Dial("tcp", address, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13, NextProtos: []string{"h2"}})
	if err == nil {
		only.Close()
		t.Fatal("a client that speaks only HTTP/2 was accepted")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, ForceAttemptHTTP2: true}
	defer transport.CloseIdleConnections()
	answer, err := (&http.Client{Transport: transport}).Get("https://" + address + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	answer.Body.Close()
	if answer.ProtoMajor != 1 || answer.StatusCode != http.StatusOK {
		t.Fatalf("a client that prefers HTTP/2 was answered %s %d", answer.Proto, answer.StatusCode)
	}

	// One connection serves customerRequestsPerConnection requests; the last answer says that it is the last, and
	// the server closes the connection after it.
	connection, err := tls.Dial("tcp", address, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	reader := bufio.NewReader(connection)
	for i := 1; i <= customerRequestsPerConnection; i++ {
		_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err := io.WriteString(connection, "GET /healthz HTTP/1.1\r\nHost: entrance\r\n\r\n"); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		reply, err := http.ReadResponse(reader, nil)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		_, _ = io.Copy(io.Discard, reply.Body)
		reply.Body.Close()
		if last := i == customerRequestsPerConnection; reply.StatusCode != http.StatusOK || reply.Close != last {
			t.Fatalf("request %d of a connection: %d, closing %v", i, reply.StatusCode, reply.Close)
		}
	}
	_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := reader.Peek(1); !errors.Is(err, io.EOF) {
		t.Fatalf("the connection was left open after its last request: %v", err)
	}
}
