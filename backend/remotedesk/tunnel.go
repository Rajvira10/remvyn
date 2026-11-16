package remotedesk

import (
	"context"
	"net"
	"sync"
	"time"

	"remvyn/backend/forward"
)

// Dialer opens a connection to the remote desktop, directly or through SSH.
type Dialer func(ctx context.Context) (net.Conn, error)

// Tunnel listens on 127.0.0.1 and forwards every connection to the remote
// desktop, for clients (RDP apps) that need a host:port to connect to.
type Tunnel struct {
	ln   net.Listener
	dial Dialer

	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	closed bool
	wg     sync.WaitGroup
}

// NewTunnel starts listening on a free local port.
func NewTunnel(dial Dialer) (*Tunnel, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	t := &Tunnel{ln: ln, dial: dial, conns: map[net.Conn]struct{}{}}
	t.wg.Add(1)
	go t.accept()
	return t, nil
}

// Addr is the local host:port clients connect to.
func (t *Tunnel) Addr() string { return t.ln.Addr().String() }

func (t *Tunnel) accept() {
	defer t.wg.Done()
	for {
		c, err := t.ln.Accept()
		if err != nil {
			return
		}
		if !t.track(c, true) {
			_ = c.Close()
			return
		}
		t.wg.Add(1)
		go t.handle(c)
	}
}

func (t *Tunnel) track(c net.Conn, add bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if add {
		if t.closed {
			return false
		}
		t.conns[c] = struct{}{}
	} else {
		delete(t.conns, c)
	}
	return true
}

func (t *Tunnel) handle(in net.Conn) {
	defer t.wg.Done()
	defer t.track(in, false)
	defer in.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	out, err := t.dial(ctx)
	cancel()
	if err != nil {
		return
	}
	if !t.track(out, true) {
		_ = out.Close()
		return
	}
	defer t.track(out, false)
	defer out.Close()
	forward.Pipe(in, out)
}

// Close stops listening and drops every connection.
func (t *Tunnel) Close() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	_ = t.ln.Close()
	for c := range t.conns {
		_ = c.Close()
	}
	t.mu.Unlock()
	t.wg.Wait()
}
