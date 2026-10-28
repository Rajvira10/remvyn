package files

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestListNames(t *testing.T) {
	dir := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(dir, "www", "html"), 0o755))
	must(t, os.MkdirAll(filepath.Join(dir, ".config"), 0o755))
	for _, f := range []string{"B.txt", "a file's.txt", ".env", "..odd"} {
		must(t, os.WriteFile(filepath.Join(dir, f), nil, 0o644))
	}
	must(t, os.Symlink(filepath.Join(dir, "www"), filepath.Join(dir, "site")))
	want := []string{".config/", "site/", "www/", "..odd", ".env", "a file's.txt", "B.txt"}

	got, err := ListNamesLocal(dir)
	must(t, err)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("local %q\nwant %q", got, want)
	}
	if _, err := ListNamesLocal(filepath.Join(dir, "missing")); err != ErrNoDir {
		t.Fatalf("missing: %v", err)
	}

	if runtime.GOOS == "windows" {
		return
	}
	run := func(d string) string {
		cmd := exec.Command("sh", "-s", "--", d)
		cmd.Stdin = strings.NewReader(ListNamesScript)
		out, err := cmd.Output()
		must(t, err)
		return string(out)
	}
	got, err = ParseNames(run(dir))
	must(t, err)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("script %q\nwant %q", got, want)
	}
	got, err = ParseNames(run(filepath.Join(dir, "www", "html")))
	if err != nil || len(got) != 0 {
		t.Fatalf("empty folder: %q %v", got, err)
	}
	if _, err := ParseNames(run(filepath.Join(dir, "missing"))); err != ErrNoDir {
		t.Fatalf("missing: %v", err)
	}
}
