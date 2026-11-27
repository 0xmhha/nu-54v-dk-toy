package main

import (
	"net"
	"sync"
)

// The customer entrance takes requests from anyone, so what one client can hold of the server is capped here.
const (
	// customerInFlight is how many customer requests may hold a recovery lease at once. The lease pool has 32
	// connections (service.OpenRecoveryGate) and the operations listener and the workers take theirs from it too,
	// so customers get half; the commerce pool behind them is smaller still, and a request is done in milliseconds.
	customerInFlight = 16
	// customerConnections is how many customer connections are open at once; more wait in the kernel's queue. An
	// idle one is closed after 8 seconds, so this is about a thousand phones in the same few seconds.
	customerConnections = 1024
)

// limitListener accepts at most cap(slots) connections at once. A further one is accepted when one closes.
type limitListener struct {
	net.Listener
	slots chan struct{}
	done  chan struct{}
	once  sync.Once
}

func newLimitListener(inner net.Listener, limit int) *limitListener {
	return &limitListener{Listener: inner, slots: make(chan struct{}, limit), done: make(chan struct{})}
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
	return &limitedConn{Conn: connection, release: func() { <-l.slots }}, nil
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
