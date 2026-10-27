package core

import (
	"context"
	"slices"
	"testing"
	"time"

	"remvyn/backend/keyagent"
	"remvyn/backend/models"
)

func TestForwardKeysSettingsAndApproval(t *testing.T) {
	f := setup(t)
	if _, err := f.core.GenerateKey(models.KeyGenerateInput{Name: "GitHub", Type: "ed25519"}); err != nil {
		t.Fatal(err)
	}
	if f.core.forwardsKeys(f.host.ID) {
		t.Fatal("forwarding should be off by default")
	}
	stale := f.core.Settings()
	if _, err := f.core.SetForwardKeys(f.host.ID, true); err != nil {
		t.Fatal(err)
	}
	// A settings form loaded before the change can't switch it back off.
	stale.ForwardKeysToAI = true
	if _, err := f.core.UpdateSettings(stale); err != nil {
		t.Fatal(err)
	}
	st := f.core.Settings()
	if !slices.Contains(st.ForwardKeyHosts, f.host.ID) || st.ForwardKeysToAI {
		t.Fatalf("forwarding settings changed through UpdateSettings: %+v %v", st.ForwardKeyHosts, st.ForwardKeysToAI)
	}

	// The agent lists the keychain and asks before signing.
	ag := f.core.keyAgent(f.host, "terminal")
	defer ag.Close()
	list, err := ag.List()
	if err != nil || len(list) != 1 || list[0].Comment != "GitHub" {
		t.Fatalf("list: %v %v", list, err)
	}
	got := make(chan keyagent.Decision, 1)
	go func() {
		got <- f.core.askKeyUse(context.Background(), f.host, "terminal", keyagent.Use{KeyName: "GitHub", Fingerprint: "SHA256:x"})
	}()
	var req models.KeyUseRequest
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if p := f.core.PendingKeyUses(); len(p) == 1 {
			req = p[0]
			break
		}
	}
	if req.ID == "" || req.HostLabel != f.host.Label || req.KeyName != "GitHub" || req.AI {
		t.Fatalf("request: %+v", req)
	}
	if f.core.ResolveKeyUse(req.ID, "maybe") {
		t.Fatal("unknown decision accepted")
	}
	if !f.core.ResolveKeyUse(req.ID, models.AgentSession) {
		t.Fatal("resolve failed")
	}
	if d := <-got; d != keyagent.Session {
		t.Fatalf("decision %v", d)
	}

	// Turning it off applies to new connections.
	if _, err := f.core.SetForwardKeys(f.host.ID, false); err != nil {
		t.Fatal(err)
	}
	if f.core.forwardsKeys(f.host.ID) {
		t.Fatal("forwarding still on")
	}
}
