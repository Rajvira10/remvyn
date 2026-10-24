// Package sshconn dials SSH servers with verified host keys, the configured
// authentication method, connect timeouts and keepalives.
package sshconn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"flux/backend/keys"
	"flux/backend/models"
)

// Error codes understood by the frontend.
const (
	CodeAuth               = "auth_failed"
	CodePasswordRequired   = "password_required"
	CodePassphraseRequired = "passphrase_required"
	CodeHostKeyRejected    = "hostkey_rejected"
	CodeNetwork            = "network"
	CodeTimeout            = "timeout"
	CodeAgent              = "agent_unavailable"
	CodeCancelled          = "cancelled"
)

// CodedError carries a machine-readable code for the UI.
type CodedError struct {
	Code string
	Msg  string
	Err  error
}

func (e *CodedError) Error() string { return e.Msg }
func (e *CodedError) Unwrap() error { return e.Err }

// Coded builds a CodedError.
func Coded(code, msg string, err error) error { return &CodedError{Code: code, Msg: msg, Err: err} }

// CodeOf extracts the code from an error, if any.
func CodeOf(err error) string {
	var ce *CodedError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

// Target is everything needed to authenticate to a host.
type Target struct {
	Host       models.Host
	Password   string
	KeyPEM     string
	Passphrase string
}

// Options tune a connection.
type Options struct {
	Timeout         time.Duration
	Keepalive       time.Duration
	HostKeyCallback HostKeyCallback
}

// HostKeyCallback verifies a server key. It may block (for a user prompt);
// the handshake deadline is suspended while it runs.
type HostKeyCallback func(ctx context.Context, hostport string, key ssh.PublicKey) error

// Client is a connected SSH client with keepalive management.
type Client struct {
	*ssh.Client
	Addr      string
	closeOnce sync.Once
	done      chan struct{}
}

// Done is closed when the connection ends.
func (c *Client) Done() <-chan struct{} { return c.done }

// Close terminates the connection.
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		err = c.Client.Close()
	})
	return err
}

// Addr returns host:port for a host.
func Addr(h models.Host) string {
	port := h.Port
	if port == 0 {
		port = 22
	}
	return net.JoinHostPort(h.Hostname, strconv.Itoa(port))
}

// Username returns the effective login name.
func Username(h models.Host) string {
	if h.Username != "" {
		return h.Username
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "root"
}

func authMethods(t Target) ([]ssh.AuthMethod, func(), error) {
	cleanup := func() {}
	switch t.Host.AuthMethod {
	case models.AuthPassword:
		if t.Password == "" {
			return nil, cleanup, Coded(CodePasswordRequired, "a password is required for this host", nil)
		}
		pw := t.Password
		return []ssh.AuthMethod{
			ssh.Password(pw),
			// Many servers only offer keyboard-interactive; answer password
			// prompts with the saved password.
			ssh.KeyboardInteractive(func(_, _ string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					if !echos[i] {
						answers[i] = pw
					}
				}
				return answers, nil
			}),
		}, cleanup, nil
	case models.AuthKey:
		if t.KeyPEM == "" {
			return nil, cleanup, errors.New("the selected key is missing from the keychain")
		}
		signer, err := keys.Signer(t.KeyPEM, t.Passphrase)
		if errors.Is(err, keys.ErrPassphraseRequired) {
			return nil, cleanup, Coded(CodePassphraseRequired, "the key is encrypted; enter its passphrase", err)
		}
		if err != nil {
			return nil, cleanup, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, cleanup, nil
	case models.AuthAgent:
		if runtime.GOOS == "windows" {
			return nil, cleanup, Coded(CodeAgent, "SSH agent authentication is not supported on Windows yet", nil)
		}
		sock := os.Getenv("SSH_AUTH_SOCK")
		if sock == "" {
			return nil, cleanup, Coded(CodeAgent, "no SSH agent is running (SSH_AUTH_SOCK is not set)", nil)
		}
		conn, err := net.Dial("unix", sock)
		if err != nil {
			return nil, cleanup, Coded(CodeAgent, "could not reach the SSH agent: "+err.Error(), err)
		}
		ag := agent.NewClient(conn)
		return []ssh.AuthMethod{ssh.PublicKeysCallback(ag.Signers)}, func() { conn.Close() }, nil
	}
	return nil, cleanup, fmt.Errorf("unknown authentication method %q", t.Host.AuthMethod)
}

// Dial connects and authenticates.
func Dial(ctx context.Context, t Target, o Options) (*Client, error) {
	if o.Timeout <= 0 {
		o.Timeout = 15 * time.Second
	}
	if o.HostKeyCallback == nil {
		return nil, errors.New("host key verification is required")
	}
	auth, cleanup, err := authMethods(t)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	addr := Addr(t.Host)
	d := net.Dialer{Timeout: o.Timeout, KeepAlive: 30 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		if ctx.Err() != nil {
			return nil, Coded(CodeCancelled, "connection cancelled", err)
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return nil, Coded(CodeTimeout, fmt.Sprintf("timed out connecting to %s", addr), err)
		}
		return nil, Coded(CodeNetwork, fmt.Sprintf("could not connect to %s: %s", addr, friendlyNetErr(err)), err)
	}
	// Abort the handshake if the caller gives up.
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	deadline := func() { _ = conn.SetDeadline(time.Now().Add(o.Timeout)) }
	deadline()

	var hostKeyErr error
	cfg := &ssh.ClientConfig{
		User: Username(t.Host),
		Auth: auth,
		HostKeyCallback: func(hostname string, _ net.Addr, key ssh.PublicKey) error {
			// The user may take a while to decide; don't let the handshake
			// deadline expire under them.
			_ = conn.SetDeadline(time.Time{})
			defer deadline()
			hostKeyErr = o.HostKeyCallback(ctx, addr, key)
			return hostKeyErr
		},
		Timeout:       o.Timeout,
		ClientVersion: "SSH-2.0-Flux_1.0",
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		switch {
		case hostKeyErr != nil:
			return nil, hostKeyErr
		case ctx.Err() != nil:
			return nil, Coded(CodeCancelled, "connection cancelled", err)
		case strings.Contains(err.Error(), "unable to authenticate"):
			return nil, Coded(CodeAuth, fmt.Sprintf("authentication failed for %s@%s", cfg.User, addr), err)
		case isTimeout(err):
			return nil, Coded(CodeTimeout, fmt.Sprintf("timed out during SSH handshake with %s", addr), err)
		}
		return nil, Coded(CodeNetwork, "SSH handshake failed: "+strings.TrimPrefix(err.Error(), "ssh: "), err)
	}
	_ = conn.SetDeadline(time.Time{})

	client := &Client{Client: ssh.NewClient(c, chans, reqs), Addr: addr, done: make(chan struct{})}
	go func() {
		_ = client.Wait()
		close(client.done)
	}()
	if o.Keepalive > 0 {
		go keepalive(client, o.Keepalive)
	}
	return client, nil
}

// keepalive pings the server and closes the client after 3 missed replies.
func keepalive(c *Client, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	missed := 0
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
			res := make(chan error, 1)
			go func() {
				_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
				res <- err
			}()
			select {
			case err := <-res:
				if err != nil {
					c.Close()
					return
				}
				missed = 0
			case <-time.After(every):
				missed++
				if missed >= 3 {
					c.Close()
					return
				}
			case <-c.done:
				return
			}
		}
	}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func friendlyNetErr(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "connection refused"):
		return "connection refused (is an SSH server running on that port?)"
	case strings.Contains(s, "no such host"):
		return "unknown host name"
	case strings.Contains(s, "network is unreachable"):
		return "network is unreachable"
	}
	if i := strings.LastIndex(s, ": "); i >= 0 {
		return s[i+2:]
	}
	return s
}
