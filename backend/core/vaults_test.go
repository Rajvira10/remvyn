package core

import (
	"os"
	"path/filepath"
	"testing"

	"remvyn/backend/models"
	"remvyn/backend/testutil"
)

func TestVaultList(t *testing.T) {
	base := t.TempDir()
	v := OpenVaults(base)

	// Without a list there is just the default vault, in the data folder.
	p, dir, err := v.Active()
	if err != nil || p.ID != DefaultVaultID || dir != base {
		t.Fatalf("active = %+v %q %v", p, dir, err)
	}
	if _, err := v.Create("  "); err == nil {
		t.Fatal("blank name accepted")
	}
	work, err := v.Create("  Work   servers ")
	if err != nil {
		t.Fatal(err)
	}
	if work.Name != "Work servers" {
		t.Fatalf("name = %q", work.Name)
	}
	workDir, err := v.SetActive(work.ID)
	if err != nil {
		t.Fatal(err)
	}
	if workDir != filepath.Join(base, "vaults", work.ID) {
		t.Fatalf("work dir = %q", workDir)
	}
	if fi, err := os.Stat(workDir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("work dir: %v %v", fi, err)
	}
	if err := v.SetRepo(work.ID, "octo/work"); err != nil {
		t.Fatal(err)
	}
	if err := v.Rename(DefaultVaultID, "Personal"); err != nil {
		t.Fatal(err)
	}

	// The choice survives a restart.
	v = OpenVaults(base)
	list, err := v.List()
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v %v", list, err)
	}
	if list[0].Name != "Personal" || list[0].Current || list[1].Repo != "octo/work" || !list[1].Current {
		t.Fatalf("list = %+v", list)
	}

	if err := v.Delete(work.ID); err == nil {
		t.Fatal("deleted the open vault")
	}
	if err := v.Delete(DefaultVaultID); err == nil {
		t.Fatal("deleted the default vault")
	}
	if _, err := v.SetActive(DefaultVaultID); err != nil {
		t.Fatal(err)
	}
	if err := v.Delete(work.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatal("vault folder should be gone")
	}
	if list, _ := v.List(); len(list) != 1 {
		t.Fatalf("list after delete = %+v", list)
	}
	if _, err := v.SetActive(work.ID); err == nil {
		t.Fatal("opened a deleted vault")
	}
}

// One GitHub sign-in serves several vaults, each in its own repository.
func TestVaultsShareGitHubAccount(t *testing.T) {
	gh := testutil.NewFakeGitHub()
	defer gh.Close()
	gh.AddRepo(repo, true)
	gh.AddRepo("octo/work-vault", true)

	personal := openDevice(t, t.TempDir(), gh)
	if _, err := personal.SaveHost(models.HostInput{Host: models.Host{Label: "home", Hostname: "home.example", AuthMethod: models.AuthPassword}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := personal.SyncAccount(); ok {
		t.Fatal("account before signing in")
	}
	signIn(t, personal)
	if _, err := personal.SyncConnect(repo, "correct horse"); err != nil {
		t.Fatal(err)
	}

	// A connected vault hands over its sealed token.
	acct, ok := personal.SyncAccount()
	if !ok || acct.Login != "octo" {
		t.Fatalf("account = %+v %v", acct, ok)
	}
	work := openDevice(t, t.TempDir(), gh)
	if work.SyncStatus().SignedIn {
		t.Fatal("new vault signed in already")
	}
	if err := work.SyncUseAccount(acct); err != nil {
		t.Fatal(err)
	}
	if st := work.SyncStatus(); !st.SignedIn || st.Login != "octo" || st.Connected {
		t.Fatalf("status = %+v", st)
	}
	if _, err := work.SyncConnect("octo/work-vault", "battery staple"); err != nil {
		t.Fatal(err)
	}
	if _, err := work.SaveHost(models.HostInput{Host: models.Host{Label: "build", Hostname: "build.example", AuthMethod: models.AuthPassword}}); err != nil {
		t.Fatal(err)
	}
	syncNow(t, work)
	syncNow(t, personal)

	// Each repository holds only its own vault.
	if _, ok := hostByLabel(t, personal, "build"); ok {
		t.Fatal("work host leaked into the personal vault")
	}
	other := openDevice(t, t.TempDir(), gh)
	signIn(t, other)
	if _, err := other.SyncConnect("octo/work-vault", "battery staple"); err != nil {
		t.Fatal(err)
	}
	if _, ok := hostByLabel(t, other, "build"); !ok {
		t.Fatal("work vault missing its host")
	}
	if _, ok := hostByLabel(t, other, "home"); ok {
		t.Fatal("personal host leaked into the work vault")
	}

	// Handing an account to a connected vault changes nothing.
	if err := personal.SyncUseAccount(acct); err != nil || personal.SyncStatus().Repo != repo {
		t.Fatalf("connected vault changed: %v %+v", err, personal.SyncStatus())
	}
}
