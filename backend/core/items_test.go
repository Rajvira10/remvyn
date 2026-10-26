package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"remvyn/backend/models"
	"remvyn/backend/testutil"
)

const (
	gmailPassword = "hunter2-gmail-canary"
	totpSecret    = "JBSWY3DPEHPK3PXP"
)

func saveGmail(t *testing.T, c *Core) models.VaultItem {
	t.Helper()
	it, err := c.SaveVaultItem(models.VaultItemInput{
		Item: models.VaultItem{Kind: models.ItemLogin, Name: "Gmail canary", Username: "me@example.com", URL: "https://mail.google.com"},
		Secret: &models.VaultSecret{Password: gmailPassword, TOTP: totpSecret, Notes: "recovery codes canary",
			Fields: []models.VaultField{{Name: "App password", Value: "app-pw-canary", Hidden: true}, {Name: "", Value: ""}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return it
}

// Nothing about an item, not even its name, is stored in plaintext.
func TestVaultItemsAreEncryptedAtRest(t *testing.T) {
	dir := t.TempDir()
	c := openDevice(t, dir, nil)
	it := saveGmail(t, c)
	if !it.HasPassword || !it.HasTOTP || !it.HasNotes || it.FieldCount != 1 {
		t.Fatalf("flags: %+v", it)
	}
	c.Close()

	var all []byte
	for _, f := range []string{"remvyn.db", "remvyn.db-wal"} {
		b, _ := os.ReadFile(filepath.Join(dir, f))
		all = append(all, b...)
	}
	if len(all) == 0 {
		t.Fatal("database not found")
	}
	for _, s := range []string{gmailPassword, "Gmail canary", "me@example.com", "mail.google.com", totpSecret, "recovery codes canary", "app-pw-canary", "App password"} {
		if bytes.Contains(all, []byte(s)) {
			t.Fatalf("database contains %q in plaintext", s)
		}
	}
}

func TestVaultItemLifecycle(t *testing.T) {
	c := openDevice(t, t.TempDir(), nil)
	it := saveGmail(t, c)

	items, err := c.VaultItems()
	if err != nil || len(items) != 1 || items[0].Name != "Gmail canary" || items[0].Username != "me@example.com" {
		t.Fatalf("list: %+v, %v", items, err)
	}
	sec, err := c.RevealVaultItem(it.ID)
	if err != nil || sec.Password != gmailPassword || len(sec.Fields) != 1 || sec.Fields[0].Value != "app-pw-canary" {
		t.Fatalf("reveal: %+v, %v", sec, err)
	}
	code, err := c.VaultItemTOTP(it.ID)
	if err != nil || len(code.Code) != 6 || code.Period != 30 || code.ExpiresAt == 0 {
		t.Fatalf("totp: %+v, %v", code, err)
	}
	for field, want := range map[string]string{"password": gmailPassword, "username": "me@example.com", "field:0": "app-pw-canary", "totp": code.Code} {
		if v, err := c.VaultItemValue(it.ID, field); err != nil || v != want {
			t.Fatalf("value %s = %q, %v", field, v, err)
		}
	}
	if _, err := c.VaultItemValue(it.ID, "field:9"); err == nil {
		t.Fatal("out-of-range field accepted")
	}

	// Editing without a secret keeps the saved one.
	it.Username = "new@example.com"
	if _, err := c.SaveVaultItem(models.VaultItemInput{Item: it}); err != nil {
		t.Fatal(err)
	}
	if sec, _ := c.RevealVaultItem(it.ID); sec.Password != gmailPassword {
		t.Fatalf("secret lost on edit: %+v", sec)
	}
	if items, _ := c.VaultItems(); !items[0].HasPassword || items[0].Username != "new@example.com" {
		t.Fatalf("after edit: %+v", items[0])
	}

	// A token named from its URL, starred, sorts first.
	tok, err := c.SaveVaultItem(models.VaultItemInput{Item: models.VaultItem{Kind: models.ItemToken, URL: "https://www.github.com/settings/tokens"},
		Secret: &models.VaultSecret{Password: "ghp_canary"}})
	if err != nil || tok.Name != "github.com" {
		t.Fatalf("token: %+v, %v", tok, err)
	}
	if err := c.SetVaultItemFavorite(tok.ID, true); err != nil {
		t.Fatal(err)
	}
	if items, _ := c.VaultItems(); len(items) != 2 || items[0].ID != tok.ID || !items[0].Favorite {
		t.Fatalf("favourite order: %+v", items)
	}

	for _, bad := range []models.VaultItemInput{
		{Item: models.VaultItem{Kind: models.ItemLogin}},
		{Item: models.VaultItem{Kind: "card", Name: "x"}},
		{Item: models.VaultItem{Name: "x"}, Secret: &models.VaultSecret{TOTP: "not base32!"}},
		{Item: models.VaultItem{Name: "x"}, Secret: &models.VaultSecret{Fields: []models.VaultField{{Value: "nameless"}}}},
	} {
		if _, err := c.SaveVaultItem(bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}

	if err := c.DeleteVaultItem(tok.ID); err != nil {
		t.Fatal(err)
	}
	if items, _ := c.VaultItems(); len(items) != 1 {
		t.Fatalf("after delete: %+v", items)
	}
}

func TestVaultItemsNeedUnlock(t *testing.T) {
	c := openDevice(t, t.TempDir(), nil)
	it := saveGmail(t, c)
	if _, err := c.SetMasterPassword("", "correct horse"); err != nil {
		t.Fatal(err)
	}
	if err := c.Lock(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.VaultItems(); code(err) != "locked" {
		t.Fatalf("list while locked: %v", err)
	}
	if _, err := c.RevealVaultItem(it.ID); code(err) != "locked" {
		t.Fatalf("reveal while locked: %v", err)
	}
	if _, err := c.VaultItemValue(it.ID, "password"); code(err) != "locked" {
		t.Fatalf("copy while locked: %v", err)
	}
	if _, err := c.SaveVaultItem(models.VaultItemInput{Item: models.VaultItem{Name: "x"}}); code(err) != "locked" {
		t.Fatalf("save while locked: %v", err)
	}
	if err := c.Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	if sec, err := c.RevealVaultItem(it.ID); err != nil || sec.Password != gmailPassword {
		t.Fatalf("after unlock: %+v, %v", sec, err)
	}
}

func TestVaultItemsSyncBetweenDevices(t *testing.T) {
	gh := testutil.NewFakeGitHub()
	defer gh.Close()
	gh.AddRepo(repo, true)

	a := openDevice(t, t.TempDir(), gh)
	it := saveGmail(t, a)
	signIn(t, a)
	if _, err := a.SyncConnect(repo, "correct horse"); err != nil {
		t.Fatal(err)
	}
	for p, content := range gh.Files(repo) {
		for _, s := range []string{gmailPassword, "Gmail canary", "me@example.com", totpSecret, "app-pw-canary"} {
			if strings.Contains(content, s) || strings.Contains(p, s) {
				t.Fatalf("%s leaks %q", p, s)
			}
		}
	}

	b := openDevice(t, t.TempDir(), gh)
	signIn(t, b)
	if _, err := b.SyncConnect(repo, "correct horse"); err != nil {
		t.Fatal(err)
	}
	items, err := b.VaultItems()
	if err != nil || len(items) != 1 || items[0].ID != it.ID || items[0].Name != "Gmail canary" || !items[0].HasTOTP {
		t.Fatalf("device B items: %+v, %v", items, err)
	}
	if sec, err := b.RevealVaultItem(it.ID); err != nil || sec.Password != gmailPassword || sec.Fields[0].Value != "app-pw-canary" {
		t.Fatalf("device B secret: %+v, %v", sec, err)
	}

	// An edit on B and a deletion on A's side round-trip.
	if _, err := b.SaveVaultItem(models.VaultItemInput{Item: items[0], Secret: &models.VaultSecret{Password: "changed-on-b"}}); err != nil {
		t.Fatal(err)
	}
	syncNow(t, b)
	syncNow(t, a)
	if sec, _ := a.RevealVaultItem(it.ID); sec.Password != "changed-on-b" || sec.TOTP != "" {
		t.Fatalf("device A after edit: %+v", sec)
	}
	if err := a.DeleteVaultItem(it.ID); err != nil {
		t.Fatal(err)
	}
	syncNow(t, a)
	syncNow(t, b)
	if items, _ := b.VaultItems(); len(items) != 0 {
		t.Fatalf("deletion didn't sync: %+v", items)
	}
}
