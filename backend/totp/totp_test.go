package totp

import (
	"encoding/base32"
	"testing"
	"time"
)

// RFC 6238 appendix B test vectors (8 digits).
func TestRFC6238(t *testing.T) {
	seeds := map[string]string{
		"SHA1":   "12345678901234567890",
		"SHA256": "12345678901234567890123456789012",
		"SHA512": "1234567890123456789012345678901234567890123456789012345678901234",
	}
	cases := []struct {
		unix int64
		want map[string]string
	}{
		{59, map[string]string{"SHA1": "94287082", "SHA256": "46119246", "SHA512": "90693936"}},
		{1111111109, map[string]string{"SHA1": "07081804", "SHA256": "68084774", "SHA512": "25091201"}},
		{1234567890, map[string]string{"SHA1": "89005924", "SHA256": "91819424", "SHA512": "93441116"}},
		{20000000000, map[string]string{"SHA1": "65353130", "SHA256": "77737706", "SHA512": "47863826"}},
	}
	for alg, seed := range seeds {
		secret := base32.StdEncoding.EncodeToString([]byte(seed))
		uri := "otpauth://totp/Example:alice?secret=" + secret + "&algorithm=" + alg + "&digits=8&period=30"
		p, err := Parse(uri)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cases {
			if got, _ := p.At(time.Unix(c.unix, 0)); got != c.want[alg] {
				t.Errorf("%s at %d: got %s want %s", alg, c.unix, got, c.want[alg])
			}
		}
	}
}

func TestParsePlainSecret(t *testing.T) {
	// Lowercase with spaces, as many sites show it.
	p, err := Parse("gezd gnbv gy3t qojq gezd gnbv gy3t qojq")
	if err != nil {
		t.Fatal(err)
	}
	if p.Digits != 6 || p.Period != 30 || p.Algorithm != "SHA1" {
		t.Fatalf("defaults: %+v", p)
	}
	code, exp := p.At(time.Unix(59, 0))
	if code != "287082" || exp.Unix() != 60 {
		t.Fatalf("code %s expires %d", code, exp.Unix())
	}
	for _, bad := range []string{"", "not base32!", "otpauth://hotp/x?secret=GEZDGNBV", "otpauth://totp/x?secret=GEZDGNBV&digits=3"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
