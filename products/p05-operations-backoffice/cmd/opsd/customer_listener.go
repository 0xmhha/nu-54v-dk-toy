package main

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
)

// The customer entrance takes requests from anyone, so what one client can hold of the server is capped here.
const (
	// customerInFlight is how many customer requests may hold a recovery lease at once. The lease pool has 32
	// connections (service.OpenRecoveryGate) and the operations listener and the workers take theirs from it too,
	// so customers get half; the commerce pool behind them is smaller still, and a request is done in milliseconds.
	customerInFlight = 16
	// customerBalanceReads is how many of those requests may wait for the chain at once (a balance read before a
	// cart is held, up to 3 seconds each): a quarter, so a chain that answers slowly leaves twelve places to the
	// menus, the store pages and the sessions.
	customerBalanceReads = 4
	// customerConnections is how many customer connections are open at once; more wait in the kernel's queue. An
	// idle one is closed after 8 seconds, so this is about a thousand phones in the same few seconds.
	customerConnections = 1024
	// customerConnectionShare is how many of them one address may hold (an IPv4 address or an IPv6 /64, as the
	// request limits count it) unless P05_CUSTOMER_CONNECTIONS_PER_ADDRESS says otherwise. A phone uses one or
	// two; 32 is a store's wireless network at a busy moment, and one host can then hold a thirty-second of the
	// places, not all of them.
	customerConnectionShare = 32
)

// connectionShare reads P05_CUSTOMER_CONNECTIONS_PER_ADDRESS: 1 to customerConnections, or unset for the default.
func connectionShare() (int, error) {
	text := os.Getenv("P05_CUSTOMER_CONNECTIONS_PER_ADDRESS")
	if text == "" {
		return customerConnectionShare, nil
	}
	share, err := strconv.Atoi(text)
	if err != nil || share < 1 || share > customerConnections || strconv.Itoa(share) != text {
		return 0, errors.New("P05_CUSTOMER_CONNECTIONS_PER_ADDRESS must be a whole number from 1 to " + strconv.Itoa(customerConnections))
	}
	return share, nil
}

// newCustomerEntrance is the entrance as opsd serves it: every request limit (commerce.NewCustomerLimits), the cap
// on requests served at once, the cap on balance reads among them, the recovery gate and the chain reader.
//
// The recovery gate is not around the listener: each route enters it after its rate limit, with its body read and
// inside the cap on requests at once, and the health route never does (CustomerEntrance.Gate). So a refused
// request, a slow client and a health probe hold no connection of the control database, which the operations
// listener shares. funds reads a proven account's balance before its cart holds stock; without one (no settlement
// target is enrolled) carts of proven accounts are refused, and the caller says so in the log.
func newCustomerEntrance(contact string, gate *service.RecoveryGate, funds commerce.PayerFunds) *commerce.CustomerEntrance {
	customer := commerce.NewCustomerLimits(nil)
	customer.OperatorContact = contact
	customer.InFlight = make(chan struct{}, customerInFlight)
	customer.BalanceReads = make(chan struct{}, customerBalanceReads)
	if gate != nil {
		customer.Gate = gate.Handler
	}
	customer.Funds = funds
	return customer
}

// customerEntranceServer serves the entrance: TLS 1.3 without a client certificate, and deadlines that cut a
// connection that does not go on. A connection that never starts or finishes its TLS handshake is closed after 5
// seconds (the shortest of the timeouts bounds the handshake), and so is one that finished it and sends no request
// (ReadHeaderTimeout); a request that arrives slowly ends after 8, and a connection that was served and then sends
// nothing more is closed after 8 too. Each of them gives its place back (limitListener).
func customerEntranceServer(addr string, handler http.Handler, certificate tls.Certificate, lifetime context.Context) *http.Server {
	return &http.Server{Addr: addr, Handler: handler, BaseContext: func(net.Listener) context.Context { return lifetime },
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 8 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 8 * time.Second,
		MaxHeaderBytes: 8 << 10,
		TLSConfig:      &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.NoClientCert, Certificates: []tls.Certificate{certificate}}}
}

// listenCustomers opens the entrance's listener with its caps on connections: all of them, and one address's share.
func listenCustomers(addr string, share int) (*limitListener, error) {
	accepting, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return newLimitListener(accepting, customerConnections, share), nil
}

// limitListener accepts at most cap(slots) connections at once, and at most share of them from one address. A
// further one is accepted when one closes; a connection over its address's share is closed as soon as it is
// accepted, so it holds no place and the others' are not taken by one host.
type limitListener struct {
	net.Listener
	slots chan struct{}
	done  chan struct{}
	once  sync.Once
	share int
	mu    sync.Mutex
	held  map[string]int
}

// newLimitListener caps inner at limit connections, share of them for one address; share below 1 sets no share.
func newLimitListener(inner net.Listener, limit, share int) *limitListener {
	return &limitListener{Listener: inner, slots: make(chan struct{}, limit), done: make(chan struct{}), share: share, held: map[string]int{}}
}

func (l *limitListener) Accept() (net.Conn, error) {
	for {
		select {
		case l.slots <- struct{}{}:
		case <-l.done:
			return nil, net.ErrClosed
		}
		connection, err := l.Listener.Accept()
		if err != nil {
			<-l.slots
			return nil, err
		}
		key := commerce.ClientAddressKey(connection.RemoteAddr().String())
		l.mu.Lock()
		over := l.share > 0 && l.held[key] >= l.share
		if !over {
			l.held[key]++
		}
		l.mu.Unlock()
		if over {
			_ = connection.Close()
			<-l.slots
			continue
		}
		return &limitedConn{Conn: connection, release: func() {
			l.mu.Lock()
			if l.held[key]--; l.held[key] <= 0 {
				delete(l.held, key)
			}
			l.mu.Unlock()
			<-l.slots
		}}, nil
	}
}

func (l *limitListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return l.Listener.Close()
}

type limitedConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *limitedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.release)
	return err
}
