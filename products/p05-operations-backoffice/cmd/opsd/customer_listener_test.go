package main

import (
	"errors"
	"net"
	"testing"
	"time"
)

// The customer listener holds a fixed number of connections: the next one is accepted only when one closes, and a
// closed listener stops waiting for a place.
func TestCustomerListenerCapsConnections(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := newLimitListener(inner, 2)
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
