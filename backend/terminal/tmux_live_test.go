//go:build !windows

package terminal

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"remvyn/backend/keyagent"
	"remvyn/backend/models"
	"remvyn/backend/sshconn"
)

// TestTmuxPersistence runs against a real server with tmux, e.g.
//
//	REMVYN_TMUX_TEST=demo:demo@127.0.0.1:2299 go test ./backend/terminal -run Tmux
func TestTmuxPersistence(t *testing.T) {
	spec := os.Getenv("REMVYN_TMUX_TEST")
	if spec == "" {
		t.Skip("set REMVYN_TMUX_TEST=user:password@host:port to run against a server with tmux")
	}
	cred, addr, _ := strings.Cut(spec, "@")
	user, pass, _ := strings.Cut(cred, ":")
	hostname, port, _ := strings.Cut(addr, ":")
	h := models.Host{Hostname: hostname, Username: user, AuthMethod: "password"}
	if port != "" {
		h.Port = atoi(port)
	}
	dial := func() *sshconn.Client {
		c, err := sshconn.Dial(context.Background(), sshconn.Target{Host: h, Password: pass}, sshconn.Options{
			HostKeyCallback: func(context.Context, string, ssh.PublicKey) error { return nil },
		})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	key := "livetest" + time.Now().Format("150405")
	rec := newRecorder()
	m := New(rec.emit)

	// First connection: a new session, with the startup command.
	info, err := m.Start("sessionone1", dial(), models.SessionInfo{}, 80, 24, StartOptions{Startup: "echo started-$((40+2))", Persist: &Persist{Key: key}})
	if err != nil {
		t.Fatal(err)
	}
	if !info.Persistent || info.Resumed {
		t.Fatalf("first start: %+v", info)
	}
	rec.waitFor(t, "started-42")
	_ = m.Write("sessionone1", "cd /tmp && for i in $(seq 1 60); do echo line-$i; done; MARK=kept\n")
	rec.waitFor(t, "line-60")

	// Drop the connection: the shell keeps running on the server.
	m.Close("sessionone1")
	rec.waitExit(t)

	rec2 := newRecorder()
	m2 := New(rec2.emit)
	info, err = m2.Start("sessiontwo2", dial(), models.SessionInfo{}, 80, 24, StartOptions{Startup: "echo should-not-run", Persist: &Persist{Key: key, Restore: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !info.Persistent || !info.Resumed {
		t.Fatalf("resume: %+v", info)
	}
	rec2.waitFor(t, "line-5\r\n") // replayed scrollback
	_ = m2.Write("sessiontwo2", "echo mark=$MARK dir=$(pwd)\n")
	rec2.waitFor(t, "mark=kept dir=/tmp")
	rec2.mu.Lock()
	ran := strings.Contains(rec2.out.String(), "should-not-run")
	rec2.mu.Unlock()
	if ran {
		t.Fatal("startup command ran again on resume")
	}

	// Ending the session removes it from the server.
	m2.End("sessiontwo2")
	rec2.waitExit(t)
	c := dial()
	defer c.Close()
	for _, s := range ListServerSessions(c) {
		if s.Key == key {
			t.Fatalf("session still on the server after End: %+v", s)
		}
	}
}

// TestAgentForwardingLive checks that a shell can use a key that only exists
// in Remvyn, before and after reconnecting to its tmux session. The server's
// user must have the key printed by the test in ~/.ssh/authorized_keys; set
// REMVYN_AGENT_TEST_KEY to that key's seed (hex) to reuse it.
func TestAgentForwardingLive(t *testing.T) {
	spec := os.Getenv("REMVYN_TMUX_TEST")
	seedHex := os.Getenv("REMVYN_AGENT_TEST_KEY")
	if spec == "" || seedHex == "" {
		t.Skip("set REMVYN_TMUX_TEST and REMVYN_AGENT_TEST_KEY to run")
	}
	seed, err := hex.DecodeString(seedHex)
	if err != nil || len(seed) != ed25519.SeedSize {
		t.Fatal("REMVYN_AGENT_TEST_KEY must be a 32-byte hex seed")
	}
	signer, _ := ssh.NewSignerFromKey(ed25519.NewKeyFromSeed(seed))
	t.Logf("authorized key: %s", strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))))

	cred, addr, _ := strings.Cut(spec, "@")
	user, pass, _ := strings.Cut(cred, ":")
	hostname, port, _ := strings.Cut(addr, ":")
	h := models.Host{Hostname: hostname, Port: atoi(port), Username: user, AuthMethod: "password"}
	var asked atomic.Int32
	newAgent := func() *keyagent.Agent {
		return keyagent.New(func() ([]keyagent.Key, error) { return []keyagent.Key{{ID: "k", Name: "Test", Signer: signer}}, nil },
			func(context.Context, keyagent.Use) keyagent.Decision { asked.Add(1); return keyagent.Session })
	}
	connect := func(m *Manager, id, key string) {
		c, err := sshconn.Dial(context.Background(), sshconn.Target{Host: h, Password: pass}, sshconn.Options{
			HostKeyCallback: func(context.Context, string, ssh.PublicKey) error { return nil },
		})
		if err != nil {
			t.Fatal(err)
		}
		ag := newAgent()
		if err := ag.Forward(c.Client); err != nil {
			t.Fatal(err)
		}
		info, err := m.Start(id, c, models.SessionInfo{}, 100, 30, StartOptions{Persist: &Persist{Key: key}, ForwardAgent: true, OnEnd: ag.Close})
		if err != nil || !info.AgentForwarding || !info.Persistent {
			t.Fatalf("start: %+v %v", info, err)
		}
	}
	hop := "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -o LogLevel=ERROR " + user + "@127.0.0.1 echo hop-ok-$((1+1))\n"
	key := "agenttest" + time.Now().Format("150405")

	rec := newRecorder()
	m := New(rec.emit)
	connect(m, "agentone111", key)
	_ = m.Write("agentone111", hop)
	rec.waitFor(t, "hop-ok-2")
	m.Close("agentone111")
	rec.waitExit(t)

	// After reconnecting, the same shell reaches the new connection's agent.
	rec2 := newRecorder()
	m2 := New(rec2.emit)
	connect(m2, "agenttwo222", key)
	_ = m2.Write("agenttwo222", strings.Replace(hop, "hop-ok-$((1+1))", "hop-again-$((2+2))", 1))
	rec2.waitFor(t, "hop-again-4")
	if asked.Load() != 2 {
		t.Fatalf("expected one approval per connection, got %d", asked.Load())
	}
	m2.End("agenttwo222")
	rec2.waitExit(t)
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}
