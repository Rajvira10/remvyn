package remotedesk

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// echoServer accepts connections and echoes everything back.
func echoServer(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln
}

func TestWSProxyNeedsToken(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	p, err := NewWSProxy(a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	base := strings.TrimPrefix(p.URL(), "ws://")
	host := base[:strings.Index(base, "/")]
	token := base[strings.Index(base, "/")+1:]
	if len(token) < 64 {
		t.Fatalf("token too short: %q", token)
	}
	for _, path := range []string{"/", "/" + token[:len(token)-1] + "x", "/" + token + "/", "/x" + token} {
		_, resp, err := websocket.DefaultDialer.Dial("ws://"+host+path, nil)
		if err == nil {
			t.Fatalf("connected without the token via %q", path)
		}
		if resp == nil || resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%q: got %v, want 404", path, resp)
		}
	}
	// Two proxies never share a token.
	c, d := net.Pipe()
	defer d.Close()
	p2, err := NewWSProxy(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p2.Close()
	if strings.HasSuffix(p2.URL(), token) {
		t.Fatal("token reused")
	}
}

func TestWSProxyBridgesBothWaysAndAcceptsOnce(t *testing.T) {
	ln := echoServer(t)
	target, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	ended := make(chan error, 1)
	p, err := NewWSProxy(target, func(err error) { ended <- err })
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	ws, _, err := websocket.DefaultDialer.Dial(p.URL(), http.Header{"Origin": {"wails://wails"}})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	msg := bytes.Repeat([]byte{0, 1, 2, 0xff}, 5000) // binary, larger than one read
	if err := ws.WriteMessage(websocket.BinaryMessage, msg); err != nil {
		t.Fatal(err)
	}
	var got []byte
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	for len(got) < len(msg) {
		typ, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if typ != websocket.BinaryMessage {
			t.Fatalf("message type %d, want binary", typ)
		}
		got = append(got, data...)
	}
	if !bytes.Equal(got, msg) {
		t.Fatal("echoed bytes differ")
	}

	// A second viewer is refused: the listener is gone.
	if _, _, err := websocket.DefaultDialer.Dial(p.URL(), nil); err == nil {
		t.Fatal("a second connection was accepted")
	}

	// Close ends the viewer's connection and the target.
	p.Close()
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := ws.ReadMessage(); err == nil {
		t.Fatal("viewer connection still open after Close")
	}
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("proxy not done")
	}
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("onEnd not called")
	}
	if _, err := target.Write([]byte("x")); err == nil {
		t.Fatal("target still open after Close")
	}
}

func TestWSProxyEndsWhenTargetCloses(t *testing.T) {
	a, b := net.Pipe()
	ended := make(chan error, 1)
	p, err := NewWSProxy(a, func(err error) { ended <- err })
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	ws, _, err := websocket.DefaultDialer.Dial(p.URL(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	_, _ = b.Write([]byte("RFB 003.008\n"))
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, data, err := ws.ReadMessage(); err != nil || string(data) != "RFB 003.008\n" {
		t.Fatalf("read %q, %v", data, err)
	}
	b.Close()
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("onEnd not called after the server hung up")
	}
	if _, _, err := ws.ReadMessage(); err == nil {
		t.Fatal("viewer connection still open")
	}
}

func TestWSProxyGivesUpWithoutViewer(t *testing.T) {
	old := AcceptTimeout
	AcceptTimeout = 50 * time.Millisecond
	defer func() { AcceptTimeout = old }()
	a, b := net.Pipe()
	defer b.Close()
	p, err := NewWSProxy(a, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("proxy kept waiting")
	}
}

func TestTunnel(t *testing.T) {
	ln := echoServer(t)
	dials := 0
	tun, err := NewTunnel(func(ctx context.Context) (net.Conn, error) {
		dials++
		var d net.Dialer
		return d.DialContext(ctx, "tcp", ln.Addr().String())
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tun.Addr(), "127.0.0.1:") {
		t.Fatalf("tunnel listens on %s", tun.Addr())
	}
	conns := make([]net.Conn, 2)
	for i := range conns {
		c, err := net.Dial("tcp", tun.Addr())
		if err != nil {
			t.Fatal(err)
		}
		conns[i] = c
		msg := []byte("hello " + string(rune('a'+i)))
		if _, err := c.Write(msg); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, len(msg))
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, err := io.ReadFull(c, buf); err != nil || !bytes.Equal(buf, msg) {
			t.Fatalf("echo %q, %v", buf, err)
		}
	}
	if dials != 2 {
		t.Fatalf("dialled %d times", dials)
	}
	done := make(chan struct{})
	go func() {
		tun.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close hung")
	}
	for _, c := range conns {
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, err := c.Read(make([]byte, 1)); err == nil {
			t.Fatal("connection still open after Close")
		}
	}
	if c, err := net.DialTimeout("tcp", tun.Addr(), time.Second); err == nil {
		c.Close()
		t.Fatal("tunnel still listening after Close")
	}
	tun.Close() // idempotent
}

func TestRDPFileHasNoPassword(t *testing.T) {
	f := RDPFile(RDPTarget{Addr: "127.0.0.1:50123", Username: "ann", Password: "s3cret"})
	if !strings.Contains(f, "full address:s:127.0.0.1:50123\r\n") || !strings.Contains(f, "username:s:ann\r\n") {
		t.Fatalf("rdp file:\n%s", f)
	}
	if strings.Contains(f, "s3cret") {
		t.Fatal("password written to the .rdp file")
	}
}

func TestFreeRDPPasswordOnStdin(t *testing.T) {
	args, stdin := freerdpArgs("xfreerdp3", RDPTarget{Addr: "h:3389", Username: "ann", Password: "s3cret"})
	if strings.Contains(strings.Join(args, " "), "s3cret") {
		t.Fatalf("password on the command line: %v", args)
	}
	if stdin != "\ns3cret\n" {
		t.Fatalf("stdin %q", stdin)
	}
	args, stdin = freerdpArgs("xfreerdp", RDPTarget{Addr: "h:3389", Username: `CORP\ann`, Password: "pw"})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "/u:ann") || !strings.Contains(joined, "/d:CORP") || stdin != "pw\n" {
		t.Fatalf("args %v stdin %q", args, stdin)
	}
	if _, stdin = freerdpArgs("xfreerdp3", RDPTarget{Addr: "h:3389", Username: "ann"}); stdin != "" {
		t.Fatalf("stdin without a password: %q", stdin)
	}
}

func TestSafeFileName(t *testing.T) {
	if got := safeFileName(` Win/Server: "prod" `); got != "Win-Server- -prod-" {
		t.Fatalf("got %q", got)
	}
}
