package keyagent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func signer(t *testing.T, raw any) ssh.Signer {
	t.Helper()
	s, err := ssh.NewSignerFromKey(raw)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// serve runs a over a pipe and returns a client for it, like a server would see.
func serve(t *testing.T, a *Agent) agent.ExtendedAgent {
	t.Helper()
	c1, c2 := net.Pipe()
	go func() { _ = agent.ServeAgent(a, c2) }()
	t.Cleanup(func() { c1.Close(); c2.Close() })
	return agent.NewClient(c1)
}

func TestListSignAndApproval(t *testing.T) {
	_, edPriv, _ := ed25519.GenerateKey(rand.Reader)
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keys := []Key{{ID: "k1", Name: "GitHub", Signer: signer(t, edPriv)}, {ID: "k2", Name: "Old RSA", Signer: signer(t, rsaPriv)}}

	var asked atomic.Int32
	answer := Once
	var lastUse Use
	a := New(func() ([]Key, error) { return keys, nil }, func(_ context.Context, u Use) Decision {
		asked.Add(1)
		lastUse = u
		return answer
	})
	c := serve(t, a)

	list, err := c.List()
	if err != nil || len(list) != 2 || list[0].Comment != "GitHub" {
		t.Fatalf("list: %v %v", list, err)
	}

	data := []byte("session data to sign")
	edPub := keys[0].Signer.PublicKey()
	sig, err := c.Sign(edPub, data)
	if err != nil {
		t.Fatal(err)
	}
	if err := edPub.Verify(data, sig); err != nil {
		t.Fatalf("bad signature: %v", err)
	}
	if asked.Load() != 1 || lastUse.KeyName != "GitHub" || lastUse.Fingerprint != ssh.FingerprintSHA256(edPub) {
		t.Fatalf("approval: %d %+v", asked.Load(), lastUse)
	}

	// "Once" asks again next time; "Session" remembers the key.
	answer = Session
	if _, err := c.Sign(edPub, data); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Sign(edPub, data); err != nil {
		t.Fatal(err)
	}
	if asked.Load() != 2 {
		t.Fatalf("session approval not remembered: asked %d times", asked.Load())
	}

	// A different key still asks, and a denial fails the signature.
	answer = Deny
	rsaPub := keys[1].Signer.PublicKey()
	if _, err := c.Sign(rsaPub, data); err == nil {
		t.Fatal("denied signature succeeded")
	}

	// RSA SHA-2 flags produce the requested algorithm.
	answer = Once
	sig, err = c.SignWithFlags(rsaPub, data, agent.SignatureFlagRsaSha256)
	if err != nil || sig.Format != ssh.KeyAlgoRSASHA256 {
		t.Fatalf("rsa-sha2-256: %v %v", sig, err)
	}
	if err := rsaPub.Verify(data, sig); err != nil {
		t.Fatalf("bad rsa signature: %v", err)
	}

	// The server can't change the agent.
	if err := c.RemoveAll(); err == nil {
		t.Fatal("RemoveAll succeeded")
	}

	// After Close nothing is listed or signed.
	a.Close()
	if _, err := c.List(); err == nil {
		t.Fatal("list after close succeeded")
	}
}
