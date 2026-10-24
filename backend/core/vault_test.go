package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"remvyn/backend/models"
	"remvyn/backend/sshconn"
	"remvyn/backend/testutil"
	"remvyn/backend/vaultsync"
)

const repo = "octo/remvyn-vault"

func openDevice(t *testing.T, dir string, gh *testutil.FakeGitHub) *Core {
	t.Helper()
	opts := Options{DataDir: dir}
	if gh != nil {
		opts.Sync = vaultsync.Options{Endpoints: vaultsync.Endpoints{Web: gh.URL, API: gh.URL}, ClientID: "Iv1.test",
			AppSlug: "remvyn-test", Interval: time.Hour, Debounce: time.Hour, PollUnit: time.Millisecond}
	}
	c, err := Open(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func code(err error) string { return sshconn.CodeOf(err) }

func TestMasterPasswordLockAndUnlock(t *testing.T) {
	dir := t.TempDir()
	c := openDevice(t, dir, nil)
	h, err := c.SaveHost(models.HostInput{Host: models.Host{Label: "db", Hostname: "10.0.0.5", AuthMethod: models.AuthPassword},
		Password: "s3cret-pw", SetPassword: true})
	if err != nil {
		t.Fatal(err)
	}
	if st := c.LockStatus(); st.Protected || st.Locked {
		t.Fatalf("new vault should be unprotected: %+v", st)
	}
	if err := c.Lock(); code(err) != CodeNoPassword {
		t.Fatalf("lock without password: %v", err)
	}
	if _, err := c.SetMasterPassword("", "short"); code(err) != CodeWeakPassword {
		t.Fatalf("weak password accepted: %v", err)
	}
	if _, err := c.SetMasterPassword("", "correct horse"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "secret.key")); !os.IsNotExist(err) {
		t.Fatal("plain key file should be gone once a master password is set")
	}

	// Locking hides secrets but keeps non-secret data readable.
	if err := c.Lock(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.store.HostPassword(context.Background(), h.ID); code(err) != CodeLocked {
		t.Fatalf("password readable while locked: %v", err)
	}
	if hosts, _ := c.Hosts(); len(hosts) != 1 {
		t.Fatal("host list should still load while locked")
	}
	if err := c.Unlock("wrong password"); code(err) != CodeWrongPassword {
		t.Fatalf("wrong password: %v", err)
	}
	if err := c.Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	if pw, _ := c.store.HostPassword(context.Background(), h.ID); pw != "s3cret-pw" {
		t.Fatalf("password after unlock = %q", pw)
	}

	// A restart starts locked.
	c.Close()
	c2 := openDevice(t, dir, nil)
	if st := c2.LockStatus(); !st.Protected || !st.Locked {
		t.Fatalf("restart should be locked: %+v", st)
	}
	if err := c2.Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	if pw, _ := c2.store.HostPassword(context.Background(), h.ID); pw != "s3cret-pw" {
		t.Fatalf("password after restart = %q", pw)
	}

	// Change, then remove.
	if _, err := c2.SetMasterPassword("nope nope", "battery staple"); code(err) != CodeWrongPassword {
		t.Fatalf("change with wrong current password: %v", err)
	}
	if _, err := c2.SetMasterPassword("correct horse", "battery staple"); err != nil {
		t.Fatal(err)
	}
	if _, err := c2.RemoveMasterPassword("correct horse"); code(err) != CodeWrongPassword {
		t.Fatalf("remove with old password: %v", err)
	}
	if _, err := c2.RemoveMasterPassword("battery staple"); err != nil {
		t.Fatal(err)
	}
	c2.Close()
	c3 := openDevice(t, dir, nil)
	if st := c3.LockStatus(); st.Protected || st.Locked {
		t.Fatalf("after removal: %+v", st)
	}
	if pw, _ := c3.store.HostPassword(context.Background(), h.ID); pw != "s3cret-pw" {
		t.Fatalf("password after removal = %q", pw)
	}
}

func TestResetForgottenPassword(t *testing.T) {
	dir := t.TempDir()
	c := openDevice(t, dir, nil)
	if _, err := c.SaveSnippet(models.Snippet{Name: "uptime", Command: "uptime"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetMasterPassword("", "correct horse"); err != nil {
		t.Fatal(err)
	}
	_ = c.Lock()
	if _, err := c.ResetVault(); err != nil {
		t.Fatal(err)
	}
	if st := c.LockStatus(); st.Protected || st.Locked {
		t.Fatalf("after reset: %+v", st)
	}
	if sn, _ := c.Snippets(); len(sn) != 0 {
		t.Fatal("reset should erase data")
	}
}

// signIn runs the GitHub device flow against the fake server.
func signIn(t *testing.T, c *Core) {
	t.Helper()
	dc, err := c.SyncStartLogin()
	if err != nil {
		t.Fatal(err)
	}
	if dc.UserCode == "" {
		t.Fatal("no user code")
	}
	if login, err := c.SyncFinishLogin(); err != nil || login != "octo" {
		t.Fatalf("login = %q, %v", login, err)
	}
}

func syncNow(t *testing.T, c *Core) {
	t.Helper()
	if st, err := c.SyncNow(); err != nil {
		t.Fatalf("sync: %v (%+v)", err, st)
	}
}

func hostByLabel(t *testing.T, c *Core, label string) (models.Host, bool) {
	t.Helper()
	hosts, err := c.Hosts()
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hosts {
		if h.Label == label {
			return h, true
		}
	}
	return models.Host{}, false
}

func TestGitHubSyncBetweenDevices(t *testing.T) {
	gh := testutil.NewFakeGitHub()
	defer gh.Close()
	gh.AddRepo(repo, true)
	gh.AddRepo("octo/public-stuff", false)
	ctx := context.Background()

	// Device A has data and no master password yet.
	a := openDevice(t, t.TempDir(), gh)
	key, err := a.GenerateKey(models.KeyGenerateInput{Name: "deploy", Type: "ed25519"})
	if err != nil {
		t.Fatal(err)
	}
	web, err := a.SaveHost(models.HostInput{Host: models.Host{Label: "web", Hostname: "web.example", AuthMethod: models.AuthKey, KeyID: key.ID}})
	if err != nil {
		t.Fatal(err)
	}
	db, err := a.SaveHost(models.HostInput{Host: models.Host{Label: "db", Hostname: "db.example", AuthMethod: models.AuthPassword},
		Password: "db-password", SetPassword: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveForward(models.ForwardRule{HostID: db.ID, Type: models.ForwardLocal, BindPort: 15432, DestHost: "127.0.0.1", DestPort: 5432}); err != nil {
		t.Fatal(err)
	}
	if err := a.store.TrustHostKey(ctx, "web.example:22", "ssh-ed25519", []byte{1, 2, 3}, "SHA256:x"); err != nil {
		t.Fatal(err)
	}

	if _, err := a.SyncConnect(repo, "x"); err == nil {
		t.Fatal("connect before sign-in should fail")
	}
	signIn(t, a)
	repos, err := a.SyncRepos()
	if err != nil || !repos.Installed || len(repos.Repos) != 2 {
		t.Fatalf("repos = %+v, %v", repos, err)
	}
	if info, err := a.SyncInspect(repo); err != nil || info.HasVault || !info.Empty {
		t.Fatalf("inspect empty repo = %+v, %v", info, err)
	}
	if _, err := a.SyncConnect("octo/public-stuff", "correct horse"); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("public repo accepted: %v", err)
	}
	if _, err := a.SyncConnect(repo, "short"); code(err) != CodeWeakPassword {
		t.Fatalf("weak password: %v", err)
	}
	st, err := a.SyncConnect(repo, "correct horse")
	if err != nil || !st.Connected || st.Error != "" {
		t.Fatalf("connect = %+v, %v", st, err)
	}
	if ls := a.LockStatus(); !ls.Protected || !ls.SyncConnected {
		t.Fatalf("connecting should set the master password: %+v", ls)
	}

	// Nothing in the repository is readable.
	files := gh.Files(repo)
	if _, ok := files["vault.json"]; !ok {
		t.Fatal("vault.json missing")
	}
	records := 0
	for p, content := range files {
		for _, secret := range []string{"db-password", "web.example", "PRIVATE KEY", "deploy", "15432"} {
			if strings.Contains(content, secret) || strings.Contains(p, secret) {
				t.Fatalf("%s leaks %q", p, secret)
			}
		}
		if strings.HasPrefix(p, "records/") {
			records++
		}
	}
	if records != 5 { // key, 2 hosts, forward, known host
		t.Fatalf("records = %d, want 5", records)
	}

	// Device B joins with the vault's password.
	b := openDevice(t, t.TempDir(), gh)
	if _, err := b.SaveSnippet(models.Snippet{Name: "disk", Command: "df -h"}); err != nil {
		t.Fatal(err)
	}
	signIn(t, b)
	if info, err := b.SyncInspect(repo); err != nil || !info.HasVault {
		t.Fatalf("inspect = %+v, %v", info, err)
	}
	if _, err := b.SyncConnect(repo, "wrong password"); code(err) != CodeWrongPassword {
		t.Fatalf("wrong vault password: %v", err)
	}
	if _, err := b.SyncConnect(repo, "correct horse"); err != nil {
		t.Fatal(err)
	}
	bdb, ok := hostByLabel(t, b, "db")
	if !ok {
		t.Fatal("db host not synced to B")
	}
	if pw, _ := b.store.HostPassword(ctx, bdb.ID); pw != "db-password" {
		t.Fatalf("synced password = %q", pw)
	}
	bweb, _ := hostByLabel(t, b, "web")
	if pem, _, err := b.store.KeySecret(ctx, bweb.KeyID); err != nil || !strings.Contains(pem, "PRIVATE KEY") {
		t.Fatalf("synced key: %v", err)
	}
	if fw, _ := b.Forwards(); len(fw) != 1 || fw[0].HostID != bdb.ID {
		t.Fatalf("forwards on B = %+v", fw)
	}
	if kh, _ := b.KnownHosts(); len(kh) != 1 {
		t.Fatalf("known hosts on B = %+v", kh)
	}
	// B's own snippet went up and reaches A.
	syncNow(t, a)
	if sn, _ := a.Snippets(); len(sn) != 1 || sn[0].Command != "df -h" {
		t.Fatalf("snippet on A = %+v", sn)
	}

	// Edits flow both ways; deletions propagate.
	web.Label = "web-1"
	if _, err := a.SaveHost(models.HostInput{Host: web}); err != nil {
		t.Fatal(err)
	}
	syncNow(t, a)
	syncNow(t, b)
	if _, ok := hostByLabel(t, b, "web-1"); !ok {
		t.Fatal("rename did not reach B")
	}
	if err := b.DeleteHost(bdb.ID); err != nil { // also deletes its forward
		t.Fatal(err)
	}
	syncNow(t, b)
	syncNow(t, a)
	if _, ok := hostByLabel(t, a, "db"); ok {
		t.Fatal("deletion did not reach A")
	}
	if fw, _ := a.Forwards(); len(fw) != 0 {
		t.Fatal("forward deletion did not reach A")
	}

	// Both edit the same snippet offline: the later edit wins everywhere.
	sn, _ := a.Snippets()
	s := sn[0]
	s.Command = "df -h /"
	if _, err := b.SaveSnippet(s); err != nil {
		t.Fatal(err)
	}
	b.sync.Scan() // B's edit is older
	time.Sleep(5 * time.Millisecond)
	s.Command = "df -h /home"
	if _, err := a.SaveSnippet(s); err != nil {
		t.Fatal(err)
	}
	a.sync.Scan()
	syncNow(t, b) // B pushes first
	syncNow(t, a) // A's newer edit wins over B's
	syncNow(t, b)
	for _, c := range []*Core{a, b} {
		if sn, _ := c.Snippets(); len(sn) != 1 || sn[0].Command != "df -h /home" {
			t.Fatalf("conflict result = %+v", sn)
		}
	}

	// A sync pass with no changes makes no commit.
	before := gh.CommitCount(repo)
	syncNow(t, a)
	syncNow(t, b)
	if gh.CommitCount(repo) != before {
		t.Fatal("idle sync created commits")
	}

	// Concurrent push: B's commit races another device and retries.
	raced := false
	gh.BeforeRefUpdate = func(r string) {
		if !raced {
			raced = true
			gh.WriteFile(r, "README.md", []byte("touched elsewhere"))
		}
	}
	if _, err := b.SaveSnippet(models.Snippet{Name: "mem", Command: "free -m"}); err != nil {
		t.Fatal(err)
	}
	syncNow(t, b)
	gh.BeforeRefUpdate = nil
	if !raced {
		t.Fatal("race hook did not run")
	}
	syncNow(t, a)
	if sn, _ := a.Snippets(); len(sn) != 2 {
		t.Fatalf("snippet after raced push: %+v", sn)
	}

	// Sync pauses while locked.
	if err := b.Lock(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SyncNow(); code(err) != CodeLocked {
		t.Fatalf("sync while locked: %v", err)
	}
	if err := b.Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	syncNow(t, b)

	// Re-joining after a disconnect: the repository's newer copy beats the
	// stale local one.
	if _, err := b.SyncDisconnect(); err != nil {
		t.Fatal(err)
	}
	sn, _ = a.Snippets()
	var mem models.Snippet
	for _, x := range sn {
		if x.Name == "mem" {
			mem = x
		}
	}
	mem.Command = "free -h"
	if _, err := a.SaveSnippet(mem); err != nil {
		t.Fatal(err)
	}
	syncNow(t, a)
	time.Sleep(5 * time.Millisecond)
	stale := mem
	stale.Command = "stale edit while disconnected"
	if _, err := b.SaveSnippet(stale); err != nil {
		t.Fatal(err)
	}
	signIn(t, b)
	if _, err := b.SyncConnect(repo, "correct horse"); err != nil {
		t.Fatal(err)
	}
	syncNow(t, a)
	for _, c := range []*Core{a, b} {
		for _, x := range must(c.Snippets()) {
			if x.Name == "mem" && x.Command != "free -h" {
				t.Fatalf("after re-join, mem = %q", x.Command)
			}
		}
	}

	// Password change: B keeps syncing; a new device needs the new password.
	if _, err := a.SetMasterPassword("correct horse", "battery staple"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SaveSnippet(models.Snippet{Name: "who", Command: "who"}); err != nil {
		t.Fatal(err)
	}
	syncNow(t, b)
	syncNow(t, a)
	if sn, _ := a.Snippets(); len(sn) != 3 {
		t.Fatalf("sync after password change: %+v", sn)
	}
	c := openDevice(t, t.TempDir(), gh)
	signIn(t, c)
	if _, err := c.SyncConnect(repo, "correct horse"); code(err) != CodeWrongPassword {
		t.Fatalf("old password joined: %v", err)
	}
	if _, err := c.SyncConnect(repo, "battery staple"); err != nil {
		t.Fatal(err)
	}
	if sn, _ := c.Snippets(); len(sn) != 3 {
		t.Fatalf("new device snippets: %+v", sn)
	}

	// Removing the password is blocked while syncing; clearing data
	// disconnects first, so the wipe stays local.
	if _, err := a.RemoveMasterPassword("battery staple"); err == nil {
		t.Fatal("password removed while sync is on")
	}
	if _, err := a.ClearAppData(); err != nil {
		t.Fatal(err)
	}
	if a.SyncStatus().Connected {
		t.Fatal("clear should disconnect sync")
	}
	syncNow(t, b)
	if sn, _ := b.Snippets(); len(sn) != 3 {
		t.Fatal("A's wipe reached B")
	}
}

func TestSyncSignInErrors(t *testing.T) {
	gh := testutil.NewFakeGitHub()
	defer gh.Close()
	gh.AddRepo(repo, true)

	gh.DenyLogin = true
	a := openDevice(t, t.TempDir(), gh)
	if _, err := a.SyncStartLogin(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SyncFinishLogin(); code(err) != vaultsync.CodeDenied {
		t.Fatalf("denied login: %v", err)
	}
	gh.DenyLogin = false

	gh.Installed = false
	signIn(t, a)
	if repos, err := a.SyncRepos(); err != nil || repos.Installed || len(repos.Repos) != 0 {
		t.Fatalf("not installed: %+v, %v", repos, err)
	}
	if _, err := a.SyncInspect(repo); code(err) != vaultsync.CodeNoAccess {
		t.Fatalf("inaccessible repo: %v", err)
	}
	st := a.SyncStatus()
	if !strings.HasSuffix(st.InstallURL, "/apps/remvyn-test/installations/new") || !strings.Contains(st.NewRepoURL, "visibility=private") {
		t.Fatalf("setup links: %+v", st)
	}

	// No built-in app: sign-in explains the setup step.
	b := openDevice(t, t.TempDir(), nil)
	if _, err := b.SyncStartLogin(); code(err) != vaultsync.CodeNotSetUp {
		t.Fatalf("no app: %v", err)
	}
	if _, err := b.SyncSetApp("Iv1.abc", "https://github.com/apps/my-remvyn"); err != nil {
		t.Fatal(err)
	}
	if st := b.SyncStatus(); !st.AppConfigured || st.AppSlug != "my-remvyn" || st.BuiltInApp {
		t.Fatalf("custom app: %+v", st)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
