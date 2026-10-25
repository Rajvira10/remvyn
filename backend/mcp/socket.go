package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SocketName is the app's socket file inside the data directory.
const SocketName = "agent.sock"

// SocketPath returns the socket path for a data directory, honouring
// REMVYN_AGENT_SOCKET.
func SocketPath(dataDir string) string {
	if p := os.Getenv("REMVYN_AGENT_SOCKET"); p != "" {
		return p
	}
	return filepath.Join(dataDir, SocketName)
}

// NotRunning is returned to the agent when the app can't be reached.
const NotRunning = "Remvyn isn't running. Ask the user to open the Remvyn app (and unlock it if it is locked), then try again."

// Bridge forwards tool calls to the app listening on socketPath. Each call
// uses its own connection; closing it tells the app the call was cancelled.
func Bridge(socketPath string) Handler {
	return func(ctx context.Context, c Call) Result {
		var d net.Dialer
		conn, err := d.DialContext(ctx, "unix", socketPath)
		if err != nil {
			return Fail(NotRunning)
		}
		defer conn.Close()
		stop := context.AfterFunc(ctx, func() { conn.Close() })
		defer stop()
		b, err := json.Marshal(c)
		if err != nil {
			return Fail(err.Error())
		}
		if _, err := conn.Write(append(b, '\n')); err != nil {
			return Fail("lost the connection to Remvyn: " + err.Error())
		}
		var res Result
		if err := json.NewDecoder(conn).Decode(&res); err != nil {
			if ctx.Err() != nil {
				return Fail("cancelled")
			}
			return Fail("Remvyn closed the connection before answering (was it quit or locked?)")
		}
		return res
	}
}

// Listener serves tool calls from bridges.
type Listener struct {
	path string
	ln   net.Listener
	wg   sync.WaitGroup
}

// ErrAlreadyRunning means another Remvyn instance owns the socket.
var ErrAlreadyRunning = errors.New("another Remvyn window is already serving AI agents")

// Listen creates the socket (readable only by this user) and serves calls
// with h until Close.
func Listen(path string, h Handler) (*Listener, error) {
	if _, err := os.Stat(path); err == nil {
		if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
			c.Close()
			return nil, ErrAlreadyRunning
		}
		_ = os.Remove(path) // left over from a crash
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// Create the socket with no group/other access from the start, so there
	// is no window where another user could connect.
	old := umask(0o177)
	ln, err := net.Listen("unix", path)
	umask(old)
	if err != nil {
		return nil, fmt.Errorf("could not open the agent socket: %w", err)
	}
	_ = os.Chmod(path, 0o600)
	l := &Listener{path: path, ln: ln}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			l.wg.Add(1)
			go func() {
				defer l.wg.Done()
				serveConn(conn, h)
			}()
		}
	}()
	return l, nil
}

// Path is the socket path.
func (l *Listener) Path() string { return l.path }

// Close stops accepting calls and removes the socket. Calls in progress are
// cancelled by their handlers' own shutdown.
func (l *Listener) Close() {
	_ = l.ln.Close()
	_ = os.Remove(l.path)
}

func serveConn(conn net.Conn, h Handler) {
	defer conn.Close()
	br := bufio.NewReaderSize(conn, 64*1024)
	line, err := readLine(br, MaxRequestBytes)
	if err != nil {
		return
	}
	var c Call
	var res Result
	if err := json.Unmarshal(line, &c); err != nil {
		res = Fail("bad request")
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		// The bridge sends nothing more; EOF means the agent gave up.
		go func() {
			_, _ = io.Copy(io.Discard, br)
			cancel()
		}()
		res = h(ctx, c)
		cancel()
	}
	b, _ := json.Marshal(res)
	_, _ = conn.Write(append(b, '\n'))
}

func readLine(r *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	for {
		part, isPrefix, err := r.ReadLine()
		if err != nil {
			return nil, err
		}
		buf = append(buf, part...)
		if len(buf) > max {
			return nil, errors.New("request too large")
		}
		if !isPrefix {
			return buf, nil
		}
	}
}
