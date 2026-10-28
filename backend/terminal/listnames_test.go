//go:build !windows

package terminal

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"remvyn/backend/models"
)

func listTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, d := range []string{"www/html", ".config"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"B.txt", "a file's.txt", ".env", "..odd"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "www"), filepath.Join(dir, "site")); err != nil {
		t.Fatal(err)
	}
	return dir
}

var listWant = []string{".config/", "site/", "www/", "..odd", ".env", "a file's.txt", "B.txt"}

func TestListNamesLocal(t *testing.T) {
	dir := listTree(t)
	got, err := ListNamesLocal(dir)
	if err != nil || !reflect.DeepEqual(got, listWant) {
		t.Fatalf("got %q, %v\nwant %q", got, err, listWant)
	}
	if _, err := ListNamesLocal(filepath.Join(dir, "missing")); err != ErrNoDir {
		t.Fatalf("missing: %v", err)
	}
}

// The helper shell's function, driven the way a session's helper is: one sh
// reading requests on stdin, answering each up to the marker.
func TestListFunc(t *testing.T) {
	dir := listTree(t)
	cmd := exec.Command("sh")
	cmd.Env = append(os.Environ(), "HOME="+dir)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { stdin.Close(); _ = cmd.Wait() }()
	if _, err := io.WriteString(stdin, listFunc); err != nil {
		t.Fatal(err)
	}
	l := &lister{stdin: stdin, out: newReader(stdout)}
	ask := func(d, base string) ([]string, error) {
		out, err := l.run("remvyn_ls " + quote(d) + " " + quote(base) + " ''; printf '" + listMarker + "\\n'\n")
		if err != nil {
			t.Fatal(err)
		}
		return ParseNames(out)
	}
	for _, c := range []struct{ dir, base string }{
		{dir, ""},   // absolute
		{"~/", ""},  // home
		{"./", dir}, // relative to the shell's directory
		{"html/../../", filepath.Join(dir, "www")},
	} {
		got, err := ask(c.dir, c.base)
		if err != nil || !reflect.DeepEqual(got, listWant) {
			t.Fatalf("%q from %q: %q, %v", c.dir, c.base, got, err)
		}
	}
	if got, err := ask("www/", dir); err != nil || !reflect.DeepEqual(got, []string{"html/"}) {
		t.Fatalf("www/: %q, %v", got, err)
	}
	if got, err := ask("www/html/", dir); err != nil || len(got) != 0 {
		t.Fatalf("empty folder: %q, %v", got, err)
	}
	if _, err := ask("missing/", dir); err != ErrNoDir {
		t.Fatalf("missing: %v", err)
	}
	// The helper keeps no directory between requests.
	if got, _ := ask("./", ""); !reflect.DeepEqual(got, listWant) {
		t.Fatalf("relative without a base should use home: %q", got)
	}
}

func TestListNamesLocalSession(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	dir := listTree(t)
	rec := newRecorder()
	m := New(rec.emit)
	const id = "local-list-1"
	if err := m.StartLocal(id, models.SessionInfo{ID: id, Kind: "local"}, 80, 24); err != nil {
		t.Fatal(err)
	}
	defer m.Close(id)
	real, _ := filepath.EvalSymlinks(dir)
	if err := m.Write(id, "cd '"+real+"/www' && echo in-$((1+1))\n"); err != nil {
		t.Fatal(err)
	}
	rec.waitFor(t, "in-2")
	// Relative to where the shell is, not the guess.
	got, err := m.ListNames(id, "../", "/nonexistent")
	if err != nil || !reflect.DeepEqual(got, listWant) {
		t.Fatalf("got %q, %v", got, err)
	}
	if !strings.HasSuffix(listFunc, ")\n") {
		t.Fatal("listFunc should be a subshell function")
	}
}
