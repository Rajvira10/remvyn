package core

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh/agent"

	"remvyn/backend/models"
)

func TestLocalKeyAgent(t *testing.T) {
	f := setup(t)
	// Unix socket paths are limited to about 104 bytes; test temp dirs can be longer.
	short, err := os.MkdirTemp("/tmp", "rva")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(short) })
	f.core.agent.socketPath = filepath.Join(short, "agent.sock")
	sock := f.core.localAgentPath()

	if _, err := f.core.GenerateKey(models.KeyGenerateInput{Name: "GitHub", Type: "ed25519"}); err != nil {
		t.Fatal(err)
	}
	if f.core.LocalAgentStatus().Listening {
		t.Fatal("the local agent should be off by default")
	}
	stale := f.core.Settings()
	if _, err := f.core.SetLocalKeyAgent(true); err != nil {
		t.Fatal(err)
	}
	// A settings form loaded before the change can't switch it back off.
	if _, err := f.core.UpdateSettings(stale); err != nil {
		t.Fatal(err)
	}
	if st := f.core.LocalAgentStatus(); !st.Enabled || !st.Listening || st.SocketPath != sock {
		t.Fatalf("status: %+v", st)
	}
	if fi, err := os.Stat(sock); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("socket: %v %v", fi, err)
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := agent.NewClient(conn)
	keys, err := client.List()
	if err != nil || len(keys) != 1 || keys[0].Comment != "GitHub" {
		t.Fatalf("list: %v %v", keys, err)
	}

	// Each signature waits for the user, and the request names this program.
	sign := func(decision string) error {
		done := make(chan error, 1)
		go func() {
			_, err := client.Sign(keys[0], []byte("data"))
			done <- err
		}()
		var req models.KeyUseRequest
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if p := f.core.PendingKeyUses(); len(p) == 1 {
				req = p[0]
				break
			}
		}
		if req.ID == "" || !req.Local || req.AI || req.KeyName != "GitHub" || req.Source == "" || req.Source == "A program" {
			t.Fatalf("request: %+v", req)
		}
		f.core.ResolveKeyUse(req.ID, decision)
		return <-done
	}
	if err := sign(models.AgentDeny); err == nil {
		t.Fatal("a denied signature succeeded")
	}
	if err := sign(models.AgentOnce); err != nil {
		t.Fatalf("allowed signature failed: %v", err)
	}
	// The private key can't be read or replaced through the socket.
	if err := client.RemoveAll(); err == nil {
		t.Fatal("RemoveAll succeeded")
	}

	if _, err := f.core.SetLocalKeyAgent(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("socket still there: %v", err)
	}
	if _, err := client.List(); err == nil {
		t.Fatal("open connection still served after turning it off")
	}
}
