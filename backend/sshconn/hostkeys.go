package sshconn

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"remvyn/backend/ids"
	"remvyn/backend/models"
)

// KnownHostStore persists trusted host keys.
type KnownHostStore interface {
	KnownHostKeys(ctx context.Context, host string) (map[string][]byte, error)
	TrustHostKey(ctx context.Context, host, keyType string, key []byte, fingerprint string) error
}

// Prompter asks the user whether to trust a key. It returns true to trust.
type Prompter func(ctx context.Context, p models.HostKeyPrompt) (bool, error)

// Verifier implements trust-on-first-use with explicit user confirmation and
// a loud warning when a host's key changes.
type Verifier struct {
	Store  KnownHostStore
	Prompt Prompter
}

// Callback returns a HostKeyCallback for Dial.
func (v *Verifier) Callback() HostKeyCallback {
	return func(ctx context.Context, host string, key ssh.PublicKey) error {
		known, err := v.Store.KnownHostKeys(ctx, host)
		if err != nil {
			return err
		}
		fp := ssh.FingerprintSHA256(key)
		raw := key.Marshal()
		prompt := models.HostKeyPrompt{ID: ids.New(), Host: host, KeyType: key.Type(), Fingerprint: fp}
		if prev, ok := known[key.Type()]; ok {
			if bytes.Equal(prev, raw) {
				return nil
			}
			prompt.Changed = true
			if pk, err := ssh.ParsePublicKey(prev); err == nil {
				prompt.OldFingerprint = ssh.FingerprintSHA256(pk)
			}
		} else {
			for _, prev := range known {
				if pk, err := ssh.ParsePublicKey(prev); err == nil {
					prompt.OldFingerprint = ssh.FingerprintSHA256(pk)
				}
			}
		}
		if v.Prompt == nil {
			return Coded(CodeHostKeyRejected, "unknown host key for "+host, nil)
		}
		ok, err := v.Prompt(ctx, prompt)
		if err != nil {
			return Coded(CodeHostKeyRejected, "host key not confirmed: "+err.Error(), err)
		}
		if !ok {
			msg := "host key for " + host + " was not trusted"
			if prompt.Changed {
				msg = "host key for " + host + " has changed and was rejected"
			}
			return Coded(CodeHostKeyRejected, msg, nil)
		}
		return v.Store.TrustHostKey(ctx, host, key.Type(), raw, fp)
	}
}

// PromptBroker turns host-key prompts into UI events and waits for answers.
type PromptBroker struct {
	Emit    func(event string, data any)
	Timeout time.Duration

	mu      sync.Mutex
	pending map[string]chan bool
}

// EventHostKeyPrompt is emitted with a models.HostKeyPrompt payload.
const EventHostKeyPrompt = "hostkey:prompt"

// EventHostKeyResolved is emitted with the prompt ID when it is answered or expires.
const EventHostKeyResolved = "hostkey:resolved"

// Ask emits the prompt and blocks for the answer.
func (b *PromptBroker) Ask(ctx context.Context, p models.HostKeyPrompt) (bool, error) {
	ch := make(chan bool, 1)
	b.mu.Lock()
	if b.pending == nil {
		b.pending = map[string]chan bool{}
	}
	b.pending[p.ID] = ch
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.pending, p.ID)
		b.mu.Unlock()
		if b.Emit != nil {
			b.Emit(EventHostKeyResolved, p.ID)
		}
	}()
	if b.Emit == nil {
		return false, errors.New("no interface available to confirm the host key")
	}
	b.Emit(EventHostKeyPrompt, p)
	timeout := b.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	select {
	case ok := <-ch:
		return ok, nil
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(timeout):
		return false, errors.New("timed out waiting for confirmation")
	}
}

// Resolve answers a pending prompt. It reports whether the prompt existed.
func (b *PromptBroker) Resolve(id string, accept bool) bool {
	b.mu.Lock()
	ch, ok := b.pending[id]
	b.mu.Unlock()
	if ok {
		select {
		case ch <- accept:
		default:
		}
	}
	return ok
}
