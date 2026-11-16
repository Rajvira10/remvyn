// Package remotedesk carries remote desktop connections: a one-shot
// WebSocket proxy for the embedded VNC viewer, and a local TCP tunnel for
// the system's RDP client.
package remotedesk

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// AcceptTimeout is how long a proxy waits for the viewer to connect before
// giving up and closing the target connection.
var AcceptTimeout = time.Minute

// WSProxy bridges one WebSocket connection from the viewer to an already
// dialled TCP connection. It listens on 127.0.0.1 with a random token in the
// path, accepts exactly one connection, and stops listening after that.
type WSProxy struct {
	ln     net.Listener
	srv    *http.Server
	token  string
	target net.Conn
	used   atomic.Bool
	onEnd  func(error)

	mu        sync.Mutex
	ws        *websocket.Conn
	closeOnce sync.Once
	done      chan struct{}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  32 << 10,
	WriteBufferSize: 32 << 10,
	// The webview's origin is wails://… (or http://wails.localhost); the
	// unguessable token in the path is what authorises the connection.
	CheckOrigin: func(*http.Request) bool { return true },
}

// NewWSProxy starts a proxy for target. onEnd (may be nil) runs once when the
// bridge ends, with the error that ended it, if any. The proxy owns target.
func NewWSProxy(target net.Conn, onEnd func(error)) (*WSProxy, error) {
	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &WSProxy{ln: ln, token: hex.EncodeToString(tok), target: target, onEnd: onEnd, done: make(chan struct{})}
	p.srv = &http.Server{Handler: http.HandlerFunc(p.serve), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = p.srv.Serve(ln) }()
	wait := AcceptTimeout
	go func() {
		select {
		case <-p.done:
		case <-time.After(wait):
			if !p.used.Load() {
				p.end(errors.New("the viewer did not connect"))
			}
		}
	}()
	return p, nil
}

// URL is the address the viewer connects to. It contains the secret token.
func (p *WSProxy) URL() string { return "ws://" + p.ln.Addr().String() + "/" + p.token }

// Done is closed when the proxy has ended.
func (p *WSProxy) Done() <-chan struct{} { return p.done }

func (p *WSProxy) serve(w http.ResponseWriter, r *http.Request) {
	want := "/" + p.token
	if subtle.ConstantTimeCompare([]byte(r.URL.Path), []byte(want)) != 1 {
		http.NotFound(w, r)
		return
	}
	if !p.used.CompareAndSwap(false, true) {
		http.Error(w, "already connected", http.StatusConflict)
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		p.end(err)
		return
	}
	// Nobody else may connect.
	_ = p.ln.Close()
	p.mu.Lock()
	select {
	case <-p.done:
		p.mu.Unlock()
		_ = ws.Close()
		return
	default:
	}
	p.ws = ws
	p.mu.Unlock()
	go p.bridge(ws)
}

func (p *WSProxy) bridge(ws *websocket.Conn) {
	errc := make(chan error, 2)
	go func() { // viewer → target
		for {
			_, r, err := ws.NextReader()
			if err != nil {
				errc <- nil
				return
			}
			if _, err := io.Copy(p.target, r); err != nil {
				errc <- err
				return
			}
		}
	}()
	go func() { // target → viewer
		buf := make([]byte, 64<<10)
		for {
			n, err := p.target.Read(buf)
			if n > 0 {
				if werr := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					errc <- nil
					return
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = nil
				}
				errc <- err
				return
			}
		}
	}()
	err := <-errc
	if err != nil && errors.Is(err, net.ErrClosed) {
		err = nil
	}
	p.end(err)
}

func (p *WSProxy) end(err error) {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		close(p.done)
		ws := p.ws
		p.mu.Unlock()
		_ = p.ln.Close()
		_ = p.srv.Close()
		if ws != nil {
			_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
			_ = ws.Close()
		}
		_ = p.target.Close()
		if p.onEnd != nil {
			go p.onEnd(err)
		}
	})
}

// Close stops listening and closes both connections.
func (p *WSProxy) Close() { p.end(nil) }
