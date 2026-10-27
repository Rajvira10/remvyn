package storage

import (
	"context"
	"crypto/rand"
	"reflect"
	"testing"

	"remvyn/backend/security"
)

func testVault(t *testing.T) *security.Vault {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	v, err := security.NewVault(key)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCommandHistory(t *testing.T) {
	ctx := context.Background()
	s, err := Open(":memory:", testVault(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, c := range []string{"ls -la", "uptime", "ls -la", "  ", "two\nlines", "df -h  "} {
		if err := s.AddCommand(ctx, "h1", c); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.AddCommand(ctx, "h2", "whoami")
	got, err := s.CommandHistory(ctx, "h1")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"df -h", "ls -la", "uptime"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("history = %q, want %q", got, want)
	}

	if err := s.ForgetCommand(ctx, "h1", "ls -la"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.CommandHistory(ctx, "h1"); !reflect.DeepEqual(got, []string{"df -h", "uptime"}) {
		t.Fatalf("after forget = %q", got)
	}

	// Sealed: the command text is not in the database.
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM command_history WHERE instr(sealed, 'uptime') > 0`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("plaintext found (%d, %v)", n, err)
	}

	// Locked: reads and writes fail rather than storing plaintext.
	s.SetVault(nil)
	if _, err := s.CommandHistory(ctx, "h1"); err != ErrLocked {
		t.Fatalf("locked read: %v", err)
	}
	// Another key (after joining a synced vault) reads as empty.
	s.SetVault(testVault(t))
	if got, err := s.CommandHistory(ctx, "h1"); err != nil || len(got) != 0 {
		t.Fatalf("other key = %q, %v", got, err)
	}

	if err := s.ClearCommandHistory(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.CommandHistory(ctx, "h2"); len(got) != 0 {
		t.Fatalf("after clear = %q", got)
	}
}
