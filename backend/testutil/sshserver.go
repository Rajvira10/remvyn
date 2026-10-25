// Package testutil provides an in-process SSH server so terminal, SFTP and
// port-forwarding features can be tested without external infrastructure.
package testutil

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Server is a minimal SSH server.
//
// Shell sessions run a tiny line-based fake shell:
//
//	echo <text>   prints text
//	size          prints "<cols>x<rows>" of the PTY
//	exit [code]   ends the session with the given status
//
// The "subsystem sftp" request is served by pkg/sftp against the real
// filesystem; direct-tcpip and tcpip-forward support port forwarding.
type Server struct {
	Addr     string
	User     string
	Password string
	HostKey  ssh.Signer

	mu             sync.Mutex
	authorizedKeys [][]byte
	ln             net.Listener
	conns          map[net.Conn]struct{}
	// AuthorizedLines receives lines appended via the install-key exec command.
	AuthorizedLines []string

	Sessions atomic.Int64

	// Fake Linux metrics for the monitoring collector ("sh -s").
	reports      atomic.Int64
	ContainerOps []string
	// FailedService, when set, is reported as a failed systemd unit.
	FailedService atomic.Value
}

// Reports is how many monitoring scripts the server has answered.
func (s *Server) Reports() int64 { return s.reports.Load() }

// fakeReport answers the monitoring script like a small Linux server whose
// CPU is 50% busy and whose eth0 receives 1 MB per report.
func (s *Server) fakeReport(script string) string {
	n := s.reports.Add(1)
	var b strings.Builder
	fmt.Fprintf(&b, "@@uname\nLinux\n6.8.0-test\nfake-linux\n@@uptime\n%d.00 1.00\n@@loadavg\n0.75 0.50 0.25 1/100 1\n", 1000+n)
	fmt.Fprintf(&b, "@@cpu\ncpu  %d 0 0 %d 0 0 0 0 0 0\n2\n", 1000+n*100, 1000+n*100)
	b.WriteString("@@mem\nMemTotal: 4000000 kB\nMemAvailable: 1000000 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n")
	fmt.Fprintf(&b, "@@net\nInter-| Receive\n face |bytes\n  eth0: %d 1 0 0 0 0 0 0 %d 1 0 0 0 0 0 0\n", n*1_000_000, n*500_000)
	b.WriteString("@@df\nFilesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vda1 10000000 9000000 1000000 90% /\n")
	b.WriteString("@@ps\n 42 root 33.0 2.0 busyd\n")
	if strings.Contains(script, "@@docker") {
		b.WriteString("@@docker\n")
		b.WriteString(`{"ID":"aaa111","Names":"web","Image":"nginx","State":"running","Status":"Up 1 hour","Ports":""}` + "\n")
		b.WriteString(`{"ID":"bbb222","Names":"worker","Image":"app","State":"exited","Status":"Exited (1)","Ports":""}` + "\n")
		b.WriteString("@@dockerstats\n" + `{"ID":"aaa111","Name":"web","CPUPerc":"3.00%","MemPerc":"1.00%","MemUsage":"40MiB / 4GiB"}` + "\n")
		b.WriteString("@@services\nssh.service loaded active running OpenBSD Secure Shell server\n")
		if f, _ := s.FailedService.Load().(string); f != "" {
			fmt.Fprintf(&b, "%s.service loaded failed failed Broken unit\n", f)
		}
	}
	return b.String()
}

// NewServer starts a server on 127.0.0.1 (random port) with a fresh ed25519 host key.
func NewServer(user, password string) (*Server, error) {
	return NewServerOn("127.0.0.1:0", user, password)
}

// NewServerOn starts a server on addr with a fresh ed25519 host key.
func NewServerOn(addr, user, password string) (*Server, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &Server{Addr: ln.Addr().String(), User: user, Password: password, HostKey: signer, ln: ln,
		conns: map[net.Conn]struct{}{}}
	go s.serve()
	return s, nil
}

// Port returns the listening port.
func (s *Server) Port() int {
	_, p, _ := net.SplitHostPort(s.Addr)
	n, _ := strconv.Atoi(p)
	return n
}

// AuthorizeKey allows public-key login with key.
func (s *Server) AuthorizeKey(key ssh.PublicKey) {
	s.mu.Lock()
	s.authorizedKeys = append(s.authorizedKeys, key.Marshal())
	s.mu.Unlock()
}

// RotateHostKey replaces the host key (to simulate a changed server key).
func (s *Server) RotateHostKey() {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	s.mu.Lock()
	s.HostKey = signer
	s.mu.Unlock()
}

// Close stops the server and drops all connections.
func (s *Server) Close() {
	s.ln.Close()
	s.mu.Lock()
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
}

// DropConnections closes every client connection (simulates network loss).
func (s *Server) DropConnections() {
	s.mu.Lock()
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
}

func (s *Server) config() *ssh.ServerConfig {
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(m ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if m.User() == s.User && string(pw) == s.Password {
				return nil, nil
			}
			return nil, fmt.Errorf("bad password")
		},
		PublicKeyCallback: func(m ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if m.User() != s.User {
				return nil, fmt.Errorf("unknown user")
			}
			for _, k := range s.authorizedKeys {
				if bytes.Equal(k, key.Marshal()) {
					return nil, nil
				}
			}
			return nil, fmt.Errorf("unknown key")
		},
	}
	s.mu.Lock()
	cfg.AddHostKey(s.HostKey)
	s.mu.Unlock()
	return cfg
}

func (s *Server) serve() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns[c] = struct{}{}
		s.mu.Unlock()
		go s.handleConn(c)
	}
}

func (s *Server) handleConn(c net.Conn) {
	defer func() {
		c.Close()
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
	}()
	sc, chans, reqs, err := ssh.NewServerConn(c, s.config())
	if err != nil {
		return
	}
	defer sc.Close()
	go s.globalRequests(sc, reqs)
	for nc := range chans {
		switch nc.ChannelType() {
		case "session":
			ch, creqs, err := nc.Accept()
			if err != nil {
				continue
			}
			go s.session(ch, creqs)
		case "direct-tcpip":
			go s.directTCPIP(nc)
		default:
			nc.Reject(ssh.UnknownChannelType, "unsupported")
		}
	}
}

func (s *Server) globalRequests(sc *ssh.ServerConn, reqs <-chan *ssh.Request) {
	listeners := map[string]net.Listener{}
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()
	for r := range reqs {
		switch r.Type {
		case "tcpip-forward":
			var p struct {
				Addr string
				Port uint32
			}
			if err := ssh.Unmarshal(r.Payload, &p); err != nil {
				r.Reply(false, nil)
				continue
			}
			l, err := net.Listen("tcp", net.JoinHostPort(p.Addr, strconv.Itoa(int(p.Port))))
			if err != nil {
				r.Reply(false, nil)
				continue
			}
			port := uint32(l.Addr().(*net.TCPAddr).Port)
			listeners[net.JoinHostPort(p.Addr, strconv.Itoa(int(p.Port)))] = l
			resp := make([]byte, 4)
			binary.BigEndian.PutUint32(resp, port)
			r.Reply(true, resp)
			go func(addr string, port uint32) {
				for {
					c, err := l.Accept()
					if err != nil {
						return
					}
					oh, op, _ := net.SplitHostPort(c.RemoteAddr().String())
					opn, _ := strconv.Atoi(op)
					payload := ssh.Marshal(struct {
						Addr       string
						Port       uint32
						OriginAddr string
						OriginPort uint32
					}{addr, port, oh, uint32(opn)})
					ch, creqs, err := sc.OpenChannel("forwarded-tcpip", payload)
					if err != nil {
						c.Close()
						continue
					}
					go ssh.DiscardRequests(creqs)
					go pipe(ch, c)
				}
			}(p.Addr, port)
		case "cancel-tcpip-forward":
			var p struct {
				Addr string
				Port uint32
			}
			_ = ssh.Unmarshal(r.Payload, &p)
			key := net.JoinHostPort(p.Addr, strconv.Itoa(int(p.Port)))
			if l, ok := listeners[key]; ok {
				l.Close()
				delete(listeners, key)
			}
			r.Reply(true, nil)
		case "keepalive@openssh.com":
			r.Reply(true, nil)
		default:
			if r.WantReply {
				r.Reply(false, nil)
			}
		}
	}
}

func pipe(ch ssh.Channel, c net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { io.Copy(ch, c); ch.CloseWrite(); wg.Done() }()
	go func() { io.Copy(c, ch); c.(*net.TCPConn).CloseWrite(); wg.Done() }()
	wg.Wait()
	ch.Close()
	c.Close()
}

func (s *Server) directTCPIP(nc ssh.NewChannel) {
	var p struct {
		Host       string
		Port       uint32
		OriginAddr string
		OriginPort uint32
	}
	if err := ssh.Unmarshal(nc.ExtraData(), &p); err != nil {
		nc.Reject(ssh.ConnectionFailed, "bad payload")
		return
	}
	c, err := net.Dial("tcp", net.JoinHostPort(p.Host, strconv.Itoa(int(p.Port))))
	if err != nil {
		nc.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	ch, reqs, err := nc.Accept()
	if err != nil {
		c.Close()
		return
	}
	go ssh.DiscardRequests(reqs)
	pipe(ch, c)
}

type ptySize struct {
	mu         sync.Mutex
	cols, rows uint32
}

func (s *Server) session(ch ssh.Channel, reqs <-chan *ssh.Request) {
	s.Sessions.Add(1)
	defer ch.Close()
	size := &ptySize{cols: 80, rows: 24}
	for r := range reqs {
		switch r.Type {
		case "pty-req":
			var p struct {
				Term          string
				Cols, Rows    uint32
				Width, Height uint32
				Modes         string
			}
			if ssh.Unmarshal(r.Payload, &p) == nil {
				size.mu.Lock()
				size.cols, size.rows = p.Cols, p.Rows
				size.mu.Unlock()
			}
			r.Reply(true, nil)
		case "window-change":
			var p struct{ Cols, Rows, Width, Height uint32 }
			if ssh.Unmarshal(r.Payload, &p) == nil {
				size.mu.Lock()
				size.cols, size.rows = p.Cols, p.Rows
				size.mu.Unlock()
			}
		case "env":
			r.Reply(true, nil)
		case "shell":
			r.Reply(true, nil)
			go func() {
				code := s.fakeShell(ch, size)
				ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(code)}))
				ch.Close()
			}()
		case "exec":
			var p struct{ Command string }
			_ = ssh.Unmarshal(r.Payload, &p)
			r.Reply(true, nil)
			go func() {
				code := s.exec(ch, p.Command)
				ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(code)}))
				ch.Close()
			}()
		case "subsystem":
			var p struct{ Name string }
			_ = ssh.Unmarshal(r.Payload, &p)
			if p.Name != "sftp" {
				r.Reply(false, nil)
				continue
			}
			r.Reply(true, nil)
			go func() {
				srv, err := sftp.NewServer(ch)
				if err != nil {
					ch.Close()
					return
				}
				_ = srv.Serve()
				srv.Close()
				ch.Close()
			}()
		default:
			if r.WantReply {
				r.Reply(false, nil)
			}
		}
	}
}

func (s *Server) exec(ch ssh.Channel, cmd string) int {
	if cmd == "sh -s" {
		data, _ := io.ReadAll(io.LimitReader(ch, 1<<20))
		script := string(data)
		if strings.HasPrefix(script, "docker ") {
			s.mu.Lock()
			s.ContainerOps = append(s.ContainerOps, strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(script), "2>&1")))
			s.mu.Unlock()
			io.WriteString(ch, strings.Fields(script)[2]+"\n")
			return 0
		}
		io.WriteString(ch, s.fakeReport(script))
		return 0
	}
	if strings.Contains(cmd, "authorized_keys") {
		data, _ := io.ReadAll(io.LimitReader(ch, 64*1024))
		line := strings.TrimSpace(string(data))
		s.mu.Lock()
		dup := false
		for _, l := range s.AuthorizedLines {
			if l == line {
				dup = true
			}
		}
		if !dup && line != "" {
			s.AuthorizedLines = append(s.AuthorizedLines, line)
		}
		s.mu.Unlock()
		return 0
	}
	switch cmd {
	case "fail": // stderr and a non-zero exit
		fmt.Fprint(ch.Stderr(), "boom\n")
		return 3
	case "hang": // runs until the client closes the session (or 30s)
		for i := 0; i < 300; i++ {
			if _, err := ch.Stderr().Write([]byte(".")); err != nil {
				return 0
			}
			time.Sleep(100 * time.Millisecond)
		}
		return 0
	}
	fmt.Fprintf(ch, "exec: %s\n", cmd)
	return 0
}

func (s *Server) fakeShell(ch ssh.Channel, size *ptySize) int {
	io.WriteString(ch, "Welcome to the Remvyn test server\r\n$ ")
	var line []byte
	buf := make([]byte, 1024)
	for {
		n, err := ch.Read(buf)
		if err != nil {
			return 0
		}
		for _, b := range buf[:n] {
			switch b {
			case '\r', '\n':
				io.WriteString(ch, "\r\n")
				cmd := strings.TrimSpace(string(line))
				line = line[:0]
				switch {
				case cmd == "exit" || strings.HasPrefix(cmd, "exit "):
					code, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(cmd, "exit")))
					return code
				case cmd == "size":
					size.mu.Lock()
					fmt.Fprintf(ch, "%dx%d\r\n", size.cols, size.rows)
					size.mu.Unlock()
				case strings.HasPrefix(cmd, "echo "):
					fmt.Fprintf(ch, "%s\r\n", strings.TrimPrefix(cmd, "echo "))
				case cmd == "":
				default:
					fmt.Fprintf(ch, "unknown command: %s\r\n", cmd)
				}
				io.WriteString(ch, "$ ")
			case 0x7f:
				if len(line) > 0 {
					line = line[:len(line)-1]
					io.WriteString(ch, "\b \b")
				}
			default:
				line = append(line, b)
				ch.Write([]byte{b})
			}
		}
	}
}
