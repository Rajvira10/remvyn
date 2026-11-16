package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"remvyn/backend/models"
)

func TestRemoteDesktopHostRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, err := Open(":memory:", testVault(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	jump, err := s.SaveHost(ctx, models.HostInput{Host: models.Host{Hostname: "jump.example.com", AuthMethod: models.AuthAgent}})
	if err != nil {
		t.Fatal(err)
	}
	if jump.Protocol != "" || jump.Port != 22 {
		t.Fatalf("ssh host: protocol %q port %d", jump.Protocol, jump.Port)
	}
	rdp, err := s.SaveHost(ctx, models.HostInput{
		Host:     models.Host{Protocol: models.ProtocolRDP, Hostname: "win.lan", Username: `CORP\ann`, ViaHostID: jump.ID, AuthMethod: models.AuthKey, KeyID: "k", StartupCommand: "ls"},
		Password: "s3cret", SetPassword: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rdp.Protocol != models.ProtocolRDP || rdp.ViaHostID != jump.ID || rdp.Port != 3389 || !rdp.HasPassword {
		t.Fatalf("rdp host saved as %+v", rdp)
	}
	if rdp.AuthMethod != models.AuthPassword || rdp.KeyID != "" || rdp.StartupCommand != "" {
		t.Fatalf("ssh-only fields kept on an rdp host: %+v", rdp)
	}
	vnc, err := s.SaveHost(ctx, models.HostInput{Host: models.Host{Protocol: models.ProtocolVNC, Hostname: "mac.lan"}})
	if err != nil {
		t.Fatal(err)
	}
	if vnc.Port != 5900 || vnc.ViaHostID != "" {
		t.Fatalf("vnc host saved as %+v", vnc)
	}

	// Tunnelling only goes through SSH hosts, and never through itself.
	if _, err := s.SaveHost(ctx, models.HostInput{Host: models.Host{Protocol: models.ProtocolVNC, Hostname: "x", ViaHostID: rdp.ID}}); err == nil {
		t.Fatal("tunnelling through an RDP host was accepted")
	}
	if _, err := s.SaveHost(ctx, models.HostInput{Host: models.Host{ID: vnc.ID, Protocol: models.ProtocolVNC, Hostname: "x", ViaHostID: vnc.ID}}); err == nil {
		t.Fatal("tunnelling through itself was accepted")
	}
	if _, err := s.SaveHost(ctx, models.HostInput{Host: models.Host{Protocol: "telnet", Hostname: "x", AuthMethod: models.AuthAgent}}); err == nil {
		t.Fatal("unknown protocol was accepted")
	}
	if _, err := s.SaveForward(ctx, models.ForwardRule{HostID: rdp.ID, Type: models.ForwardLocal, BindPort: 8080, DestHost: "x", DestPort: 80}); err == nil {
		t.Fatal("a forward through an RDP host was accepted")
	}

	// Sync carries the new fields to another device, and leaves SSH records
	// as they were before RDP/VNC existed.
	snap, err := s.SyncSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	other, err := Open(":memory:", testVault(t))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	for _, r := range snap {
		if r.Kind != KindHost {
			continue
		}
		var m map[string]any
		_ = json.Unmarshal(r.Data, &m)
		if r.ID == jump.ID {
			if _, ok := m["protocol"]; ok {
				t.Fatalf("ssh host record gained a protocol field: %s", r.Data)
			}
		}
		if err := other.ApplySyncRecord(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := other.GetHost(ctx, rdp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Protocol != models.ProtocolRDP || got.ViaHostID != jump.ID || got.Port != 3389 || got.Username != `CORP\ann` {
		t.Fatalf("synced rdp host: %+v", got)
	}
	if pw, _ := other.HostPassword(ctx, rdp.ID); pw != "s3cret" {
		t.Fatalf("synced password %q", pw)
	}
	if g, _ := other.GetHost(ctx, vnc.ID); g.Protocol != models.ProtocolVNC {
		t.Fatalf("synced vnc host: %+v", g)
	}
}

// A database from before RDP/VNC gets the new columns, and its hosts read
// as SSH.
func TestHostColumnsMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	old := schema[strings.Index(schema, "CREATE TABLE IF NOT EXISTS hosts"):]
	old = old[:strings.Index(old, ");")+2]
	old = strings.Replace(old, ",\n\tprotocol        TEXT NOT NULL DEFAULT '',\n\tvia_host_id     TEXT NOT NULL DEFAULT ''", "", 1)
	if strings.Contains(old, "protocol") {
		t.Fatalf("could not build the old schema:\n%s", old)
	}
	if _, err := db.Exec(old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO hosts(id, label, hostname, auth_method, created_at, updated_at) VALUES ('h1','old','old.lan','agent',1,1)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(path, testVault(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h, err := s.GetHost(context.Background(), "h1")
	if err != nil {
		t.Fatal(err)
	}
	if !h.IsSSH() || h.Port != 22 {
		t.Fatalf("migrated host: %+v", h)
	}
	if _, err := s.SaveHost(context.Background(), models.HostInput{Host: models.Host{Protocol: models.ProtocolVNC, Hostname: "v"}}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	// Opening again is a no-op.
	s2, err := Open(path, testVault(t))
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
}
