// Package keys generates, imports and parses SSH key pairs.
package keys

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"

	"remvyn/backend/models"
)

// ErrPassphraseRequired is returned when an encrypted key is parsed without
// (or with a wrong) passphrase.
var ErrPassphraseRequired = errors.New("this private key is encrypted; enter its passphrase")

// Material is a parsed key pair ready for storage.
type Material struct {
	Meta       models.Key
	PrivatePEM string
}

// Generate creates a new key pair. The private key is written in OpenSSH
// format, encrypted with passphrase when one is given.
func Generate(in models.KeyGenerateInput) (Material, error) {
	var signer crypto.PrivateKey
	bits := 0
	switch strings.ToLower(in.Type) {
	case "", "ed25519":
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return Material{}, err
		}
		signer, bits = priv, 256
	case "rsa":
		bits = in.Bits
		if bits == 0 {
			bits = 4096
		}
		if bits != 2048 && bits != 3072 && bits != 4096 {
			return Material{}, errors.New("RSA keys must be 2048, 3072 or 4096 bits")
		}
		priv, err := rsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			return Material{}, err
		}
		signer = priv
	case "ecdsa":
		bits = in.Bits
		if bits == 0 {
			bits = 256
		}
		var curve elliptic.Curve
		switch bits {
		case 256:
			curve = elliptic.P256()
		case 384:
			curve = elliptic.P384()
		case 521:
			curve = elliptic.P521()
		default:
			return Material{}, errors.New("ECDSA keys must be 256, 384 or 521 bits")
		}
		priv, err := ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			return Material{}, err
		}
		signer = priv
	default:
		return Material{}, fmt.Errorf("unsupported key type %q", in.Type)
	}

	comment := strings.TrimSpace(in.Name)
	var block *pem.Block
	var err error
	if in.Passphrase != "" {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(signer, comment, []byte(in.Passphrase))
	} else {
		block, err = ssh.MarshalPrivateKey(signer, comment)
	}
	if err != nil {
		return Material{}, err
	}
	privPEM := string(pem.EncodeToMemory(block))
	m, err := Parse(privPEM, in.Passphrase)
	if err != nil {
		return Material{}, err
	}
	m.Meta.Name = comment
	m.Meta.Bits = bits
	return m, nil
}

// Parse validates a private key and derives its public metadata.
func Parse(privatePEM, passphrase string) (Material, error) {
	privatePEM = strings.TrimSpace(privatePEM) + "\n"
	if !strings.Contains(privatePEM, "PRIVATE KEY") {
		return Material{}, errors.New("not a private key (expected an OpenSSH or PEM private key)")
	}
	signer, err := Signer(privatePEM, passphrase)
	if err != nil {
		return Material{}, err
	}
	pub := signer.PublicKey()
	m := Material{PrivatePEM: privatePEM}
	m.Meta.Type = pub.Type()
	m.Meta.Fingerprint = ssh.FingerprintSHA256(pub)
	m.Meta.PublicKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	m.Meta.HasPassphrase = passphrase != ""
	if ck, ok := pub.(ssh.CryptoPublicKey); ok {
		switch k := ck.CryptoPublicKey().(type) {
		case *rsa.PublicKey:
			m.Meta.Bits = k.N.BitLen()
		case *ecdsa.PublicKey:
			m.Meta.Bits = k.Curve.Params().BitSize
		case ed25519.PublicKey:
			m.Meta.Bits = 256
		}
	}
	return m, nil
}

// Signer parses a private key into an ssh.Signer.
func Signer(privatePEM, passphrase string) (ssh.Signer, error) {
	var signer ssh.Signer
	var err error
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(privatePEM), []byte(passphrase))
		if errors.Is(err, x509.IncorrectPasswordError) {
			return nil, ErrPassphraseRequired
		}
	} else {
		signer, err = ssh.ParsePrivateKey([]byte(privatePEM))
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) {
			return nil, ErrPassphraseRequired
		}
	}
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	return signer, nil
}

// AuthorizedKeyLine returns the public key line with a comment.
func AuthorizedKeyLine(k models.Key) string {
	if k.Name == "" {
		return k.PublicKey
	}
	return k.PublicKey + " " + strings.ReplaceAll(k.Name, " ", "-")
}
