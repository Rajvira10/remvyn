package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.4.0", "2.3.9", true},
		{"v2.10.0", "2.9.1", true},
		{"2.4.0", "2.4.0", false},
		{"2.4.0", "v2.4.0", false},
		{"2.3.0", "2.4.0", false},
		{"3", "2.99.99", true},
		{"2.4.1-rc1", "2.4.0", true},
		{"2.4.0+build7", "2.4.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestPickAsset(t *testing.T) {
	assets := []asset{{Name: "Remvyn-2.4.0-android.apk"}, {Name: "Remvyn-2.4.0-windows-x64-setup.exe"}, {Name: "Remvyn-2.4.0-windows-x64.zip"}}
	if a := pickAsset(assets, "-windows-x64.zip"); a == nil || a.Name != "Remvyn-2.4.0-windows-x64.zip" {
		t.Fatalf("zip: got %+v", a)
	}
	if a := pickAsset(assets, "-windows-x64-setup.exe"); a == nil || a.Name != "Remvyn-2.4.0-windows-x64-setup.exe" {
		t.Fatalf("setup: got %+v", a)
	}
	if a := pickAsset(assets, "-macOS-universal.dmg"); a != nil {
		t.Fatalf("dmg: got %+v", a)
	}
	if a := pickAsset(assets, ""); a != nil {
		t.Fatalf("empty suffix: got %+v", a)
	}
}

// fakeGitHub serves a latest release whose only asset matches this
// platform's suffix (when there is one) and carries payload.
func fakeGitHub(t *testing.T, tag string, payload []byte, digest string) *httptest.Server {
	t.Helper()
	suffix, _ := plan()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		assets := []map[string]any{}
		if suffix != "" {
			assets = append(assets, map[string]any{
				"name": "Remvyn-x" + suffix, "browser_download_url": srv.URL + "/dl", "size": len(payload), "digest": digest,
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": tag, "name": "Remvyn " + tag, "body": "notes", "html_url": "https://example/rel", "assets": assets,
		})
	})
	mux.HandleFunc("/dl", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(s[:])
}

func TestCheck(t *testing.T) {
	srv := fakeGitHub(t, "v2.5.0", []byte("x"), sha([]byte("x")))
	var emitted []Status
	u := New(Options{Current: "2.4.0", Repo: "o/r", API: srv.URL, Emit: func(s Status) { emitted = append(emitted, s) }})
	s, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.State != StateAvailable || !s.Available || s.Latest == nil || s.Latest.Version != "2.5.0" || s.Latest.Notes != "notes" {
		t.Fatalf("status = %+v", s)
	}
	if len(emitted) < 2 || emitted[0].State != StateChecking {
		t.Fatalf("emitted = %+v", emitted)
	}

	u = New(Options{Current: "2.5.0", Repo: "o/r", API: srv.URL})
	if s, _ := u.Check(context.Background()); s.State != StateUpToDate || s.Available {
		t.Fatalf("same version: state = %s", s.State)
	}
}

func TestCheckHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	u := New(Options{Current: "2.4.0", Repo: "o/r", API: srv.URL})
	s, err := u.Check(context.Background())
	if err == nil || s.State != StateError || s.Error == "" {
		t.Fatalf("want error state, got %+v, %v", s, err)
	}
}

func TestDownloadVerifiesDigest(t *testing.T) {
	payload := bytes.Repeat([]byte("remvyn"), 1000)
	srv := fakeGitHub(t, "v2.5.0", payload, "")
	u := New(Options{Current: "2.4.0"})
	dst := filepath.Join(t.TempDir(), "f")
	a := &asset{Name: "f", URL: srv.URL + "/dl", Size: int64(len(payload))}

	a.Digest = sha(payload)
	if err := u.download(context.Background(), a, dst); err != nil {
		t.Fatalf("good digest: %v", err)
	}
	if got, _ := os.ReadFile(dst); !bytes.Equal(got, payload) {
		t.Fatal("downloaded bytes differ")
	}
	if s := u.Status(); s.Downloaded != int64(len(payload)) {
		t.Fatalf("progress = %d", s.Downloaded)
	}

	a.Digest = sha([]byte("something else"))
	if err := u.download(context.Background(), a, dst); err == nil {
		t.Fatal("bad digest accepted")
	}
	a.Digest = ""
	if err := u.download(context.Background(), a, dst); err == nil {
		t.Fatal("missing digest accepted")
	}
}

func TestInstallNeedsAvailableUpdate(t *testing.T) {
	u := New(Options{Current: "2.4.0"})
	if err := u.Install(context.Background()); err == nil {
		t.Fatal("install without a check succeeded")
	}
}

func TestExtract(t *testing.T) {
	dir := t.TempDir()
	body := []byte("binary")

	tgz := filepath.Join(dir, "a.tar.gz")
	var tb bytes.Buffer
	gz := gzip.NewWriter(&tb)
	tw := tar.NewWriter(gz)
	for _, name := range []string{"Remvyn-1-linux/README.txt", "Remvyn-1-linux/remvyn"} {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write(body)
	}
	tw.Close()
	gz.Close()
	_ = os.WriteFile(tgz, tb.Bytes(), 0o600)
	out := filepath.Join(dir, "remvyn")
	if err := extractTarGz(tgz, "remvyn", out, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); !bytes.Equal(got, body) {
		t.Fatal("tar contents differ")
	}
	if err := extractTarGz(tgz, "missing", filepath.Join(dir, "m"), 0o755); err == nil {
		t.Fatal("missing tar entry found")
	}

	zp := filepath.Join(dir, "a.zip")
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	w, _ := zw.Create("Remvyn.exe")
	_, _ = w.Write(body)
	zw.Close()
	_ = os.WriteFile(zp, zb.Bytes(), 0o600)
	out = filepath.Join(dir, "new.exe")
	if err := extractZip(zp, "remvyn.exe", out, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); !bytes.Equal(got, body) {
		t.Fatal("zip contents differ")
	}
}
