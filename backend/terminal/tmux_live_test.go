//go:build !windows

package terminal

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

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
	info, err := m.Start("sessionone1", dial(), models.SessionInfo{}, 80, 24, "echo started-$((40+2))", &Persist{Key: key})
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
	info, err = m2.Start("sessiontwo2", dial(), models.SessionInfo{}, 80, 24, "echo should-not-run", &Persist{Key: key, Restore: true})
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

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}
