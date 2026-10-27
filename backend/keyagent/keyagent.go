// Package keyagent lets a server use the keys in Remvyn's keychain over SSH
// agent forwarding, without the private keys ever leaving this computer.
// Every signature is approved by the user, once or for the rest of the
// connection.
package keyagent

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// Key is a keychain key the agent can sign with.
type Key struct {
	ID     string
	Name   string
	Signer ssh.Signer
}

// Use describes a request to sign with a key, for the approval prompt.
type Use struct {
	KeyID       string
	KeyName     string
	Fingerprint string
}

// Decision answers a Use.
type Decision int

const (
	Deny Decision = iota
	Once
	// Session allows this key for the rest of the connection.
	Session
)

// Agent is a read-only SSH agent for one connection.
type Agent struct {
	load    func() ([]Key, error)
	approve func(context.Context, Use) Decision

	mu      sync.Mutex
	allowed map[string]bool // key fingerprint → allowed for the session
	closed  bool
	ctx     context.Context
	cancel  context.CancelFunc
}

var errReadOnly = errors.New("Remvyn's forwarded agent is read-only")

// New returns an agent that lists the keys from load (called on each
// request, so it follows the keychain and the lock) and asks approve before
// each signature.
func New(load func() ([]Key, error), approve func(context.Context, Use) Decision) *Agent {
	ctx, cancel := context.WithCancel(context.Background())
	return &Agent{load: load, approve: approve, allowed: map[string]bool{}, ctx: ctx, cancel: cancel}
}

// Close ends the agent: pending approvals are abandoned and later requests fail.
func (a *Agent) Close() {
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	a.cancel()
}

// Forward serves the agent to client's server. Sessions on client must still
// ask for it with agent.RequestAgentForwarding.
func (a *Agent) Forward(client *ssh.Client) error {
	return agent.ForwardToAgent(client, a)
}

func (a *Agent) keys() ([]Key, error) {
	a.mu.Lock()
	closed := a.closed
	a.mu.Unlock()
	if closed {
		return nil, errors.New("the connection has closed")
	}
	return a.load()
}

// List returns the public keys.
func (a *Agent) List() ([]*agent.Key, error) {
	ks, err := a.keys()
	if err != nil {
		return nil, err
	}
	out := make([]*agent.Key, 0, len(ks))
	for _, k := range ks {
		pub := k.Signer.PublicKey()
		out = append(out, &agent.Key{Format: pub.Type(), Blob: pub.Marshal(), Comment: k.Name})
	}
	return out, nil
}

// Sign signs data with the key, if the user allows it.
func (a *Agent) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	return a.SignWithFlags(key, data, 0)
}

// SignWithFlags supports the RSA SHA-2 signature flags.
func (a *Agent) SignWithFlags(key ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	ks, err := a.keys()
	if err != nil {
		return nil, err
	}
	blob := key.Marshal()
	var k *Key
	for i := range ks {
		if bytes.Equal(ks[i].Signer.PublicKey().Marshal(), blob) {
			k = &ks[i]
			break
		}
	}
	if k == nil {
		return nil, errors.New("key not found")
	}
	fp := ssh.FingerprintSHA256(key)
	a.mu.Lock()
	ok := a.allowed[fp]
	a.mu.Unlock()
	if !ok {
		switch a.approve(a.ctx, Use{KeyID: k.ID, KeyName: k.Name, Fingerprint: fp}) {
		case Session:
			a.mu.Lock()
			a.allowed[fp] = true
			a.mu.Unlock()
		case Once:
		default:
			return nil, errors.New("the user denied the use of this key")
		}
	}
	if alg := algorithmFor(key.Type(), flags); alg != "" {
		if as, ok := k.Signer.(ssh.AlgorithmSigner); ok {
			return as.SignWithAlgorithm(nil, data, alg)
		}
	}
	return k.Signer.Sign(nil, data)
}

func algorithmFor(keyType string, flags agent.SignatureFlags) string {
	if keyType != ssh.KeyAlgoRSA {
		return ""
	}
	switch {
	case flags&agent.SignatureFlagRsaSha512 != 0:
		return ssh.KeyAlgoRSASHA512
	case flags&agent.SignatureFlagRsaSha256 != 0:
		return ssh.KeyAlgoRSASHA256
	}
	return ""
}

// Extension is not supported.
func (a *Agent) Extension(string, []byte) ([]byte, error) { return nil, agent.ErrExtensionUnsupported }

// Add is refused: the server can't add keys to Remvyn.
func (a *Agent) Add(agent.AddedKey) error { return errReadOnly }

// Remove is refused.
func (a *Agent) Remove(ssh.PublicKey) error { return errReadOnly }

// RemoveAll is refused.
func (a *Agent) RemoveAll() error { return errReadOnly }

// Lock is refused.
func (a *Agent) Lock([]byte) error { return errReadOnly }

// Unlock is refused.
func (a *Agent) Unlock([]byte) error { return errReadOnly }

// Signers is not available: private keys stay inside the agent.
func (a *Agent) Signers() ([]ssh.Signer, error) { return nil, errReadOnly }
