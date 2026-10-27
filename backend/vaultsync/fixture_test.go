package vaultsync

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestWriteMobileFixture writes a vault and an encrypted record for the
// Android app's compatibility test (mobile/android/app/src/test).
// Run with REMVYN_WRITE_FIXTURE=<file>.
func TestWriteMobileFixture(t *testing.T) {
	out := os.Getenv("REMVYN_WRITE_FIXTURE")
	if out == "" {
		t.Skip("set REMVYN_WRITE_FIXTURE to write the fixture")
	}
	doc, key, err := NewVaultDoc("correct horse battery", time.UnixMilli(1700000000000))
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"id": "h1", "label": "prod <db>", "hostname": "10.0.0.5", "port": 2222,
		"username": "root", "authMethod": "password", "password": "pä$$wörd"})
	path := c.Path("host", "h1")
	content, err := c.seal(path, payload{Kind: "host", ID: "h1", ChangedAt: 42, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	// A password vault item, shaped like backend/storage's syncItem.
	item, _ := json.Marshal(map[string]any{"kind": "login", "name": "GitHub", "username": "octo", "url": "github.com",
		"favorite": true, "hasPassword": true, "hasTotp": true, "fieldCount": 1, "createdAt": 1, "updatedAt": 1700000000000,
		"secret": map[string]any{"password": "s3cr€t", "totp": "otpauth://totp/GitHub:octo?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ&issuer=GitHub",
			"notes": "", "fields": []map[string]any{{"name": "Recovery", "value": "abcd-efgh", "hidden": true}}}})
	itemPath := c.Path("vaultitem", "v1")
	itemContent, err := c.seal(itemPath, payload{Kind: "vaultitem", ID: "v1", ChangedAt: 43, Data: item})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.MarshalIndent(map[string]string{"vault": string(doc), "path": path, "content": string(content), "keyId": c.keyID,
		"itemPath": itemPath, "itemContent": string(itemContent)}, "", "  ")
	if err := os.WriteFile(out, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
