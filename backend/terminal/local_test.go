//go:build !windows

package terminal

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"remvyn/backend/models"
)

// recorder collects the output and exit events of one session.
type recorder struct {
	mu   sync.Mutex
	out  strings.Builder
	exit chan models.SessionExit
}

func newRecorder() *recorder { return &recorder{exit: make(chan models.SessionExit, 1)} }

func (r *recorder) emit(event string, data any) {
	switch {
	case strings.HasPrefix(event, "term:data:"):
		b, _ := base64.StdEncoding.DecodeString(data.(string))
		r.mu.Lock()
		r.out.Write(b)
		r.mu.Unlock()
	case strings.HasPrefix(event, "term:exit:"):
		r.exit <- data.(models.SessionExit)
	}
}

func (r *recorder) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		got := r.out.String()
		r.mu.Unlock()
		if strings.Contains(got, want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t.Fatalf("output never contained %q; got:\n%s", want, r.out.String())
}

func (r *recorder) waitExit(t *testing.T) models.SessionExit {
	t.Helper()
	select {
	case e := <-r.exit:
		return e
	case <-time.After(10 * time.Second):
		t.Fatal("no exit event")
		return models.SessionExit{}
	}
}

func TestLocalShell(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	rec := newRecorder()
	m := New(rec.emit)
	const id = "local-test-1"
	if err := m.StartLocal(id, models.SessionInfo{ID: id, Kind: "local"}, 80, 24); err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 1 {
		t.Fatalf("List = %v", m.List())
	}
	// Arithmetic so the echoed input line doesn't satisfy the check.
	if err := m.Write(id, "echo hi-$((40+2))\n"); err != nil {
		t.Fatal(err)
	}
	rec.waitFor(t, "hi-42")

	if err := m.Resize(id, 100, 30); err != nil {
		t.Fatal(err)
	}
	if err := m.Write(id, "stty size\n"); err != nil {
		t.Fatal(err)
	}
	rec.waitFor(t, "30 100")

	if err := m.Write(id, "exit 3\n"); err != nil {
		t.Fatal(err)
	}
	if e := rec.waitExit(t); e.ExitCode != 3 || e.Error != "" {
		t.Fatalf("exit = %+v, want code 3", e)
	}
	if len(m.List()) != 0 {
		t.Fatal("session still listed after exit")
	}
	if err := m.Write(id, "x"); err == nil {
		t.Fatal("writing to an ended session should fail")
	}
}

func TestLocalShellClose(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	rec := newRecorder()
	m := New(rec.emit)
	const id = "local-test-2"
	if err := m.StartLocal(id, models.SessionInfo{ID: id, Kind: "local"}, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := m.StartLocal(id, models.SessionInfo{ID: id, Kind: "local"}, 0, 0); err == nil {
		t.Fatal("a duplicate session id should be refused")
	}
	if m.ClientForHost("") != nil {
		t.Fatal("local sessions have no SSH client to share")
	}
	m.Close(id)
	rec.waitExit(t)
	if err := m.StartLocal("bad id!", models.SessionInfo{}, 0, 0); err == nil {
		t.Fatal("invalid id accepted")
	}
}

// The path picker follows cd, including in a shell started from the shell.
func TestLocalShellDir(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	rec := newRecorder()
	m := New(rec.emit)
	const id = "local-test-3"
	if err := m.StartLocal(id, models.SessionInfo{ID: id, Kind: "local"}, 80, 24); err != nil {
		t.Fatal(err)
	}
	defer m.Close(id)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Write(id, "sh\ncd '"+dir+"' && echo in-$((1+1))\n"); err != nil {
		t.Fatal(err)
	}
	rec.waitFor(t, "in-2")
	got, err := m.WorkingDir(id)
	if err != nil || got.Path != dir || !got.Exact {
		t.Fatalf("WorkingDir = %+v, %v; want %s", got, err, dir)
	}
}
