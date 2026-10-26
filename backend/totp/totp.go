// Package totp generates time-based one-time passwords (RFC 6238) for the
// password vault's two-factor codes.
package totp

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Params describe how codes are generated.
type Params struct {
	Secret    []byte
	Algorithm string // SHA1, SHA256 or SHA512
	Digits    int
	Period    int // seconds
}

// Parse accepts a base32 secret (spaces and dashes allowed) or an
// otpauth://totp/ URI as shown under a setup QR code.
func Parse(s string) (Params, error) {
	s = strings.TrimSpace(s)
	p := Params{Algorithm: "SHA1", Digits: 6, Period: 30}
	secret := s
	if strings.HasPrefix(strings.ToLower(s), "otpauth://") {
		u, err := url.Parse(s)
		if err != nil {
			return p, errors.New("the two-factor link isn't valid")
		}
		if !strings.EqualFold(u.Host, "totp") {
			return p, errors.New("only time-based (TOTP) codes are supported")
		}
		q := u.Query()
		secret = q.Get("secret")
		if a := strings.ToUpper(q.Get("algorithm")); a != "" {
			p.Algorithm = a
		}
		if d := q.Get("digits"); d != "" {
			if p.Digits, err = strconv.Atoi(d); err != nil {
				return p, errors.New("the two-factor link has invalid digits")
			}
		}
		if per := q.Get("period"); per != "" {
			if p.Period, err = strconv.Atoi(per); err != nil {
				return p, errors.New("the two-factor link has an invalid period")
			}
		}
	}
	key, err := decodeSecret(secret)
	if err != nil {
		return p, err
	}
	p.Secret = key
	if p.Digits < 6 || p.Digits > 10 {
		return p, fmt.Errorf("unsupported number of digits: %d", p.Digits)
	}
	if p.Period < 1 || p.Period > 3600 {
		return p, fmt.Errorf("unsupported period: %d seconds", p.Period)
	}
	if p.hash() == nil {
		return p, fmt.Errorf("unsupported algorithm %q", p.Algorithm)
	}
	return p, nil
}

func decodeSecret(s string) ([]byte, error) {
	s = strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "\t", "").Replace(s))
	s = strings.TrimRight(s, "=")
	if s == "" {
		return nil, errors.New("the two-factor secret is empty")
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil || len(key) == 0 {
		return nil, errors.New("the two-factor secret isn't valid base32 (letters A–Z and digits 2–7)")
	}
	return key, nil
}

func (p Params) hash() func() hash.Hash {
	switch p.Algorithm {
	case "SHA1":
		return sha1.New
	case "SHA256":
		return sha256.New
	case "SHA512":
		return sha512.New
	}
	return nil
}

// At returns the code for time t and when it expires.
func (p Params) At(t time.Time) (string, time.Time) {
	counter := uint64(t.Unix()) / uint64(p.Period)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	m := hmac.New(p.hash(), p.Secret)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint64(1)
	for range p.Digits {
		mod *= 10
	}
	code := fmt.Sprintf("%0*d", p.Digits, uint64(bin)%mod)
	expires := time.Unix(int64((counter+1)*uint64(p.Period)), 0)
	return code, expires
}
