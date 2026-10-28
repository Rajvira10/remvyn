package files

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func findTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, d := range []string{"src/components", "node_modules/pkg", ".git/objects", "it's odd", "empty"} {
		must(t, os.MkdirAll(filepath.Join(dir, d), 0o755))
	}
	for _, f := range []string{"README.md", "src/main.go", "src/components/settings.tsx", "node_modules/pkg/index.js", ".env", "it's odd/100% $x.txt"} {
		must(t, os.WriteFile(filepath.Join(dir, f), nil, 0o644))
	}
	return dir
}

func sorted(idx *lists) {
	slices.Sort(idx.Files)
	slices.Sort(idx.Dirs)
	slices.Sort(idx.Pruned)
}

type lists struct{ Files, Dirs, Pruned []string }

func TestFindLocal(t *testing.T) {
	dir := findTree(t)
	idx, err := FindLocal(dir)
	must(t, err)
	got := lists{idx.Files, idx.Dirs, idx.Pruned}
	sorted(&got)
	want := lists{
		Files:  []string{".env", "README.md", "it's odd/100% $x.txt", "src/components/settings.tsx", "src/main.go"},
		Dirs:   []string{"empty", "it's odd", "src", "src/components"},
		Pruned: []string{".git", "node_modules"},
	}
	if !reflect.DeepEqual(got, want) || idx.Truncated {
		t.Fatalf("got %+v (truncated %v)\nwant %+v", got, idx.Truncated, want)
	}
	if _, err := FindLocal(filepath.Join(dir, "missing")); err != ErrNoDir {
		t.Fatalf("missing dir: %v", err)
	}
}

// The server-side listing agrees with the local one.
func TestFindScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	dir := findTree(t)
	run := func(d string) string {
		cmd := exec.Command("sh", "-s", "--", d)
		cmd.Stdin = strings.NewReader(FindScript())
		out, err := cmd.Output()
		must(t, err)
		return string(out)
	}
	idx, err := ParseFind(dir, run(dir), true)
	must(t, err)
	local, _ := FindLocal(dir)
	got, want := lists{idx.Files, idx.Dirs, idx.Pruned}, lists{local.Files, local.Dirs, local.Pruned}
	sorted(&got)
	sorted(&want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("script %+v\nlocal %+v", got, want)
	}
	if _, err := ParseFind(dir, run(filepath.Join(dir, "missing")), true); err != ErrNoDir {
		t.Fatalf("missing dir: %v", err)
	}
}

func TestParseFindCutShort(t *testing.T) {
	idx, err := ParseFind("/x", "Fa\nF./b\nD./c\nF./par", false)
	must(t, err)
	if !idx.Truncated || !reflect.DeepEqual(idx.Files, []string{"b"}) || !reflect.DeepEqual(idx.Dirs, []string{"c"}) {
		t.Fatalf("%+v", idx)
	}
}
