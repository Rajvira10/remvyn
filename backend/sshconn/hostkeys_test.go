package sshconn

import (
	"context"
	"testing"
	"time"

	"flux/backend/models"
)

func TestPromptBrokerResolveAndTimeout(t *testing.T) {
	var emitted []string
	b := &PromptBroker{Emit: func(e string, _ any) { emitted = append(emitted, e) }, Timeout: 100 * time.Millisecond}

	go func() {
		time.Sleep(20 * time.Millisecond)
		if !b.Resolve("p1", true) {
			t.Error("pending prompt not found")
		}
	}()
	ok, err := b.Ask(context.Background(), models.HostKeyPrompt{ID: "p1"})
	if err != nil || !ok {
		t.Fatalf("resolve: %v %v", ok, err)
	}
	ok, err = b.Ask(context.Background(), models.HostKeyPrompt{ID: "p2"})
	if ok || err == nil {
		t.Fatal("expected timeout")
	}
	if b.Resolve("p2", true) {
		t.Fatal("expired prompt should be gone")
	}
	if len(emitted) != 4 || emitted[0] != EventHostKeyPrompt || emitted[1] != EventHostKeyResolved {
		t.Fatalf("events: %v", emitted)
	}
}
