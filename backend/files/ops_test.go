package files

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"remvyn/backend/sshconn"
)

func TestFreeName(t *testing.T) {
	taken := map[string]bool{"/d/a.txt": true, "/d/a copy.txt": true, "/d/x.tar.gz": true, "/d/.env": true}
	exists := func(p string) bool { return taken[p] }
	join := func(p ...string) string { return filepath.ToSlash(filepath.Join(p...)) }
	for in, want := range map[string]string{"b.txt": "b.txt", "a.txt": "a copy 2.txt", "x.tar.gz": "x copy.tar.gz", ".env": ".env copy"} {
		if got := freeName(exists, join, "/d", in); got != want {
			t.Errorf("freeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestArchiveStem(t *testing.T) {
	for in, want := range map[string]string{"site.zip": "site", "a.tar.gz": "a", "b.TGZ": "b", "log.gz": "log", "notes.txt": "notes.txt"} {
		if got := archiveStem(in); got != want {
			t.Errorf("archiveStem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShellQuote(t *testing.T) {
	if got := shq(`it's $HOME`); got != `'it'\''s $HOME'` {
		t.Fatalf("shq = %s", got)
	}
}

func writeTree(t *testing.T, dir string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Join(dir, "site", "css"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "site", "index.html"), []byte("<h1>hi</h1>"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "site", "css", "a.css"), []byte("body{}"), 0o644))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCompressExtractLocalRoundTrip(t *testing.T) {
	for _, format := range []string{FormatZip, FormatTarGz} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			writeTree(t, dir)
			archive, err := CompressLocal(dir, []string{"site"}, "site."+format, format)
			must(t, err)
			if _, err := CompressLocal(dir, []string{"site"}, "site."+format, format); err == nil {
				t.Fatal("compressing over an existing archive should fail")
			}
			dest, err := ExtractLocal(archive, true)
			must(t, err)
			if filepath.Base(dest) != "site copy" {
				t.Fatalf("extracted into %s, want a fresh \"site copy\" folder", dest)
			}
			got, err := os.ReadFile(filepath.Join(dest, "site", "css", "a.css"))
			must(t, err)
			if string(got) != "body{}" {
				t.Fatalf("extracted content %q", got)
			}
		})
	}
}

func TestExtractLocalRejectsZipSlip(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "evil.zip")
	f, err := os.Create(archive)
	must(t, err)
	zw := zip.NewWriter(f)
	w, err := zw.Create("../outside.txt")
	must(t, err)
	_, _ = w.Write([]byte("x"))
	must(t, zw.Close())
	must(t, f.Close())

	if _, err := ExtractLocal(archive, true); err == nil {
		t.Fatal("extracting ../outside.txt should fail")
	}
	if _, err := os.Stat(filepath.Join(dir, "outside.txt")); err == nil {
		t.Fatal("a file was written outside the destination")
	}
	if _, err := os.Stat(filepath.Join(dir, "evil")); err == nil {
		t.Fatal("the empty destination folder should be removed")
	}
}

func TestCopyMoveLocal(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir)
	must(t, CopyLocal([]string{filepath.Join(dir, "site")}, dir))
	if _, err := os.Stat(filepath.Join(dir, "site copy", "css", "a.css")); err != nil {
		t.Fatalf("copy into the same folder: %v", err)
	}
	if err := CopyLocal([]string{filepath.Join(dir, "site")}, filepath.Join(dir, "site", "css")); err == nil {
		t.Fatal("copying a folder into itself should fail")
	}
	must(t, os.Mkdir(filepath.Join(dir, "out"), 0o755))
	must(t, MoveLocal([]string{filepath.Join(dir, "site copy")}, filepath.Join(dir, "out")))
	if _, err := os.Stat(filepath.Join(dir, "out", "site copy", "index.html")); err != nil {
		t.Fatalf("move: %v", err)
	}
}

func TestLocalTextEditing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "app.env")
	must(t, os.WriteFile(p, []byte("A=1\n"), 0o600))
	fc, err := ReadTextLocal(p)
	must(t, err)
	saved, err := WriteTextLocal(p, "A=2\n", fc.ModTime)
	must(t, err)
	got, _ := os.ReadFile(p)
	if string(got) != "A=2\n" {
		t.Fatalf("saved %q", got)
	}
	if _, err := WriteTextLocal(p, "A=3\n", saved.ModTime-5000); sshconn.CodeOf(err) != CodeFileChanged {
		t.Fatalf("stale save: got %v, want %s", err, CodeFileChanged)
	}

	bin := filepath.Join(dir, "a.bin")
	must(t, os.WriteFile(bin, []byte{0x7f, 'E', 'L', 'F', 0, 0, 1}, 0o644))
	if _, err := ReadTextLocal(bin); sshconn.CodeOf(err) != CodeBinary {
		t.Fatalf("binary file: got %v, want %s", err, CodeBinary)
	}
}
