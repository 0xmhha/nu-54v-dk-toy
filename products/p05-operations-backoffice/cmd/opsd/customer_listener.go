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
	"sync/atomic"
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
	// request limits count it) unless P05_CUSTOMER_CONNECTIONS_PER_ADDRESS says otherwise. The app opens a
	// connection for each request and closes it after the answer (CommerceHttp.kt, CommerceHttp.h: a new TLS
	// session per request, none kept), so a customer holds one for about as long as one request takes on a mobile
	// network. The request limits let 20 customers of an address go from cart to lock in one moment and 50 follow
	// an order; 64 is those 50 with a request on its way in the same second, with room. One address then holds a
	// sixteenth of the places, not all of them.
	customerConnectionShare = 64
	// customerConnectionLifetime is the longest a connection is kept, whatever is sent on it. The app's one request
	// ends within 8 seconds on iOS and 12 on Android (its own timeouts); a connection older than this is not the
	// app's, and is closed so that requests alone do not keep a place for good.
	customerConnectionLifetime = 20 * time.Second
	// customerRequestsPerConnection is how many requests a connection serves before the server closes it. The app
	// sends one; a probe that reuses its connection opens a new one after four.
	customerRequestsPerConnection = 4
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

// connectionRequests counts the requests served on one connection.
type connectionRequests struct{ served atomic.Int32 }

type connectionRequestsKey struct{}

// customerEntranceServer serves the entrance: TLS 1.3 without a client certificate, and deadlines that cut a
// connection that does not go on. A connection that never starts or finishes its TLS handshake is closed after 5
// seconds (the shortest of the timeouts bounds the handshake), and so is one that finished it and sends no request
// (ReadHeaderTimeout); a request that arrives slowly ends after 8, and a connection that was served and then sends
// nothing more is closed after 8 too. Each of them gives its place back (limitListener).
//
// A connection that keeps sending is bounded as well: it is closed after customerRequestsPerConnection requests,
// and the listener closes it at customerConnectionLifetime whatever it does. HTTP/2 is not offered: one connection
// would carry many requests at once, each of which could send its body slowly, and the caps on connections would
// no longer be caps on what a host holds. The app sends one request per connection over HTTP/1.1 either way.
func customerEntranceServer(addr string, handler http.Handler, certificate tls.Certificate, lifetime context.Context) *http.Server {
	counted := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests, ok := r.Context().Value(connectionRequestsKey{}).(*connectionRequests); ok && requests.served.Add(1) >= customerRequestsPerConnection {
			w.Header().Set("Connection", "close")
		}
		handler.ServeHTTP(w, r)
	})
	return &http.Server{Addr: addr, Handler: counted, BaseContext: func(net.Listener) context.Context { return lifetime },
		ConnContext: func(ctx context.Context, _ net.Conn) context.Context {
			return context.WithValue(ctx, connectionRequestsKey{}, &connectionRequests{})
		},
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 8 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 8 * time.Second,
		MaxHeaderBytes: 8 << 10,
		// A map that is set and empty: the server takes no protocol but HTTP/1.1 from the handshake.
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.NoClientCert, Certificates: []tls.Certificate{certificate},
			NextProtos: []string{"http/1.1"}}}
}

// listenCustomers opens the entrance's listener with its caps on connections: all of them, one address's share,
// and how long one is kept.
func listenCustomers(addr string, share int) (*limitListener, error) {
	accepting, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	listener := newLimitListener(accepting, customerConnections, share)
	listener.lifetime = customerConnectionLifetime
	return listener, nil
}

// limitListener accepts at most cap(slots) connections at once, and holds at most share of them for one address.
// A further one is accepted when one closes. When an address that holds its share opens one more, the connection
// it has held longest is closed and the new one is served: a customer's connection lives for one request, so the
// one held longest is the one most likely to be held for nothing, and a host that holds its address's share with
// idle or slow connections does not keep the customers of that address out. What it would take instead is to open
// connections faster than a request is served: share of them within the time of one request.
type limitListener struct {
	net.Listener
	slots chan struct{}
	done  chan struct{}
	once  sync.Once
	share int
	// lifetime is the longest a connection is kept; zero sets none.
	lifetime time.Duration
	mu       sync.Mutex
	// held is the connections of each address, the one held longest first.
	held map[string][]*limitedConn
}

// newLimitListener caps inner at limit connections, share of them for one address; share below 1 sets no share.
func newLimitListener(inner net.Listener, limit, share int) *limitListener {
	return &limitListener{Listener: inner, slots: make(chan struct{}, limit), done: make(chan struct{}), share: share, held: map[string][]*limitedConn{}}
}

func (l *limitListener) Accept() (net.Conn, error) {
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
	accepted := &limitedConn{Conn: connection}
	accepted.release = func() {
		l.mu.Lock()
		l.forget(key, accepted)
		l.mu.Unlock()
		<-l.slots
	}
	l.mu.Lock()
	var longest *limitedConn
	if l.share > 0 && len(l.held[key]) >= l.share {
		longest = l.held[key][0]
		l.forget(key, longest) // its place among the address's is free now; its place among all is when it closes
	}
	l.held[key] = append(l.held[key], accepted)
	l.mu.Unlock()
	if longest != nil {
		_ = longest.Close()
	}
	if l.lifetime > 0 {
		accepted.expiry = time.AfterFunc(l.lifetime, func() { _ = accepted.Close() })
	}
	return accepted, nil
}

// forget takes one connection out of its address's list. The caller holds mu.
func (l *limitListener) forget(key string, connection *limitedConn) {
	list := l.held[key]
	for i, held := range list {
		if held == connection {
			list = append(list[:i:i], list[i+1:]...)
			break
		}
	}
	if len(list) == 0 {
		delete(l.held, key)
		return
	}
	l.held[key] = list
}

func (l *limitListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return l.Listener.Close()
}

type limitedConn struct {
	net.Conn
	once    sync.Once
	release func()
	expiry  *time.Timer
}

func (c *limitedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() {
		if c.expiry != nil {
			c.expiry.Stop()
		}
		c.release()
	})
	return err
}
