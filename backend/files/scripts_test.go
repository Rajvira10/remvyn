package files

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// The server-side commands run under a real POSIX shell, with awkward names.
func TestRemoteScripts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	sh := func(script string) {
		t.Helper()
		if out, err := exec.Command("sh", "-c", script).CombinedOutput(); err != nil {
			t.Fatalf("%s\n%v: %s", script, err, out)
		}
	}
	has := func(tool string) bool { _, err := exec.LookPath(tool); return err == nil }
	dir := t.TempDir()
	odd := "it's a -dir"
	must(t, os.MkdirAll(filepath.Join(dir, odd, "sub"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, odd, "sub", "f $x.txt"), []byte("data"), 0o640))
	must(t, os.WriteFile(filepath.Join(dir, "-n"), []byte("dash"), 0o644))

	sh(copyScript(filepath.Join(dir, odd), filepath.Join(dir, odd+" copy")))
	fi, err := os.Stat(filepath.Join(dir, odd+" copy", "sub", "f $x.txt"))
	must(t, err)
	if fi.Mode().Perm() != 0o640 {
		t.Fatalf("copy lost the mode: %v", fi.Mode())
	}

	formats := []string{FormatTarGz}
	if has("zip") && has("unzip") {
		formats = append(formats, FormatZip)
	} else {
		t.Log("zip/unzip not installed; testing .tar.gz only")
	}
	for _, format := range formats {
		archive := "out." + format
		script, err := compressScript(dir, []string{odd, "-n"}, archive, format)
		must(t, err)
		sh(script)
		dest := filepath.Join(dir, "x-"+format)
		must(t, os.Mkdir(dest, 0o755))
		sh(extractScript(filepath.Join(dir, archive), dest, ""))
		got, err := os.ReadFile(filepath.Join(dest, odd, "sub", "f $x.txt"))
		must(t, err)
		if string(got) != "data" {
			t.Fatalf("%s round trip: %q", format, got)
		}
		if _, err := os.Stat(filepath.Join(dest, "-n")); err != nil {
			t.Fatalf("%s: %v", format, err)
		}
	}

	// A zip made elsewhere extracts with unzip, or bsdtar/python3 without it.
	if has("unzip") || has("bsdtar") || has("python3") {
		zipped, err := CompressLocal(dir, []string{odd}, "local.zip", FormatZip)
		must(t, err)
		dest := filepath.Join(dir, "from-zip")
		must(t, os.Mkdir(dest, 0o755))
		sh(extractScript(zipped, dest, ""))
		if got, _ := os.ReadFile(filepath.Join(dest, odd, "sub", "f $x.txt")); string(got) != "data" {
			t.Fatalf("zip extract: %q", got)
		}
	}

	sh("gzip -c " + shq(filepath.Join(dir, "-n")) + " > " + shq(filepath.Join(dir, "note.gz")))
	sh(extractScript(filepath.Join(dir, "note.gz"), dir, "note"))
	if got, _ := os.ReadFile(filepath.Join(dir, "note")); string(got) != "dash" {
		t.Fatalf(".gz: %q", got)
	}
}
