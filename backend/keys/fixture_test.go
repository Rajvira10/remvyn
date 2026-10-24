package keys

import (
	"encoding/json"
	"os"
	"testing"

	"remvyn/backend/models"
)

// TestWriteMobileKeyFixture writes keys the desktop generates, for the
// Android app's JSch compatibility test. Run with REMVYN_WRITE_FIXTURE=<file>.
func TestWriteMobileKeyFixture(t *testing.T) {
	out := os.Getenv("REMVYN_WRITE_FIXTURE")
	if out == "" {
		t.Skip("set REMVYN_WRITE_FIXTURE to write the fixture")
	}
	var keys []map[string]string
	for _, in := range []models.KeyGenerateInput{
		{Type: "ed25519"}, {Type: "ed25519", Passphrase: "pp-secret"}, {Type: "ecdsa"}, {Type: "rsa", Bits: 2048},
	} {
		m, err := Generate(in)
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, map[string]string{"type": in.Type, "passphrase": in.Passphrase, "pem": m.PrivatePEM, "public": m.Meta.PublicKey})
	}
	raw, _ := json.MarshalIndent(keys, "", "  ")
	if err := os.WriteFile(out, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
