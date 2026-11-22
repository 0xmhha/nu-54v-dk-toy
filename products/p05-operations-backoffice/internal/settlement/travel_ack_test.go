package settlement

import (
	"context"
	"io"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
)

// The enrolled customer connects through this loopback tunnel. P05 terminates
// the original mTLS connection and sees the original client certificate. Once
// the approved deposit is durable, discard one server reply before delivery.
func travelAckTunnel(t *testing.T, ctx context.Context, endpoint string, committed func(context.Context) (bool, error)) (string, *atomic.Bool, *atomic.Bool) {
	t.Helper()
	target, err := url.Parse(endpoint)
	if err != nil || target.Scheme != "https" || !net.ParseIP(target.Hostname()).IsLoopback() {
		t.Fatal("expected a loopback HTTPS settlement fixture")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	lifetime, cancel := context.WithCancel(ctx)
	var armed, dropped atomic.Bool
	var connections sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			connections.Add(1)
			go func() {
				defer connections.Done()
				defer client.Close()
				server, err := (&net.Dialer{}).DialContext(lifetime, "tcp", target.Host)
				if err != nil {
					return
				}
				defer server.Close()
				closePair := func() { client.Close(); server.Close() }
				stop := context.AfterFunc(lifetime, closePair)
				defer stop()
				sent := make(chan struct{})
				go func() {
					_, _ = io.Copy(server, client)
					closePair()
					close(sent)
				}()
				defer func() { closePair(); <-sent }()
				buf := make([]byte, 32*1024)
				for {
					n, readErr := server.Read(buf)
					if n > 0 {
						if armed.Load() && !dropped.Load() {
							ready, err := committed(lifetime)
							if err != nil {
								t.Errorf("could not verify deposit commit: %v", err)
								return
							}
							if ready && dropped.CompareAndSwap(false, true) {
								return
							}
						}
						if _, err := client.Write(buf[:n]); err != nil {
							return
						}
					}
					if readErr != nil {
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() {
		cancel()
		listener.Close()
		<-done
		connections.Wait()
	})
	return "https://" + listener.Addr().String(), &armed, &dropped
}
