package keys

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"remvyn/backend/models"
)

func TestGenerateAllTypes(t *testing.T) {
	cases := []struct {
		in       models.KeyGenerateInput
		wantType string
		bits     int
	}{
		{models.KeyGenerateInput{Name: "a", Type: "ed25519"}, "ssh-ed25519", 256},
		{models.KeyGenerateInput{Name: "b", Type: "rsa", Bits: 2048}, "ssh-rsa", 2048},
		{models.KeyGenerateInput{Name: "c", Type: "ecdsa", Bits: 384}, "ecdsa-sha2-nistp384", 384},
	}
	for _, c := range cases {
		m, err := Generate(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.in.Type, err)
		}
		if m.Meta.Type != c.wantType || m.Meta.Bits != c.bits || !strings.HasPrefix(m.Meta.Fingerprint, "SHA256:") {
			t.Fatalf("%s meta: %+v", c.in.Type, m.Meta)
		}
		pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(m.Meta.PublicKey))
		if err != nil || ssh.FingerprintSHA256(pub) != m.Meta.Fingerprint {
			t.Fatalf("public key does not match fingerprint: %v", err)
		}
		if !strings.Contains(m.PrivatePEM, "OPENSSH PRIVATE KEY") {
			t.Fatal("expected OpenSSH private key format")
		}
	}
	if _, err := Generate(models.KeyGenerateInput{Type: "rsa", Bits: 1024}); err == nil {
		t.Fatal("weak RSA accepted")
	}
	if _, err := Generate(models.KeyGenerateInput{Type: "dsa"}); err == nil {
		t.Fatal("DSA accepted")
	}
}

func TestPassphraseHandling(t *testing.T) {
	m, err := Generate(models.KeyGenerateInput{Name: "p", Type: "ed25519", Passphrase: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Signer(m.PrivatePEM, ""); !errors.Is(err, ErrPassphraseRequired) {
		t.Fatalf("missing passphrase: %v", err)
	}
	if _, err := Signer(m.PrivatePEM, "wrong"); !errors.Is(err, ErrPassphraseRequired) {
		t.Fatalf("wrong passphrase: %v", err)
	}
	if _, err := Signer(m.PrivatePEM, "correct horse"); err != nil {
		t.Fatal(err)
	}
	if !m.Meta.HasPassphrase {
		t.Fatal("HasPassphrase not set")
	}
}

func TestAuthorizedKeyLine(t *testing.T) {
	k := models.Key{Name: "my laptop", PublicKey: "ssh-ed25519 AAAA"}
	if got := AuthorizedKeyLine(k); got != "ssh-ed25519 AAAA my-laptop" {
		t.Fatalf("got %q", got)
	}
}
