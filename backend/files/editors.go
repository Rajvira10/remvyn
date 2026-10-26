package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"remvyn/backend/models"
)

// EventEditSync carries an EditSync after a file opened in an external
// editor was uploaded back to the server, or failed to.
const EventEditSync = "edit:sync"

// SystemEditor opens a file with the operating system's default app.
const SystemEditor = "system"

// maxExternalEditSize bounds files downloaded for an external editor.
const maxExternalEditSize = 200 << 20

// EditSync reports an upload of an externally edited file.
type EditSync struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	// Conflict means the server copy changed since it was downloaded, so
	// the upload was held back; ForceEditUpload overwrites it.
	Conflict bool   `json:"conflict"`
	Error    string `json:"error"`
}

type editor struct {
	models.Editor
	argv []string // the file path is appended
}

var (
	editorsMu   sync.Mutex
	editorsList []editor
)

// Editors lists the editors installed on this computer, detecting them on
// first use.
func Editors() []models.Editor {
	editorsMu.Lock()
	defer editorsMu.Unlock()
	if editorsList == nil {
		editorsList = detectEditors()
	}
	out := make([]models.Editor, len(editorsList))
	for i, e := range editorsList {
		out[i] = e.Editor
	}
	return out
}

func findEditor(id string) (editor, bool) {
	Editors()
	editorsMu.Lock()
	defer editorsMu.Unlock()
	for _, e := range editorsList {
		if e.ID == id {
			return e, true
		}
	}
	return editor{}, false
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func detectEditors() []editor {
	var out []editor
	seen := map[string]bool{}
	add := func(id, name string, argv ...string) {
		if seen[id] {
			return
		}
		seen[id] = true
		out = append(out, editor{Editor: models.Editor{ID: id, Name: name}, argv: argv})
	}
	switch runtime.GOOS {
	case "windows":
		local, pf, pf86 := os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")
		for _, c := range []struct {
			id, name string
			paths    []string
		}{
			{"vscode", "Visual Studio Code", []string{local + `\Programs\Microsoft VS Code\Code.exe`, pf + `\Microsoft VS Code\Code.exe`}},
			{"cursor", "Cursor", []string{local + `\Programs\cursor\Cursor.exe`}},
			{"windsurf", "Windsurf", []string{local + `\Programs\Windsurf\Windsurf.exe`}},
			{"zed", "Zed", []string{local + `\Programs\Zed\Zed.exe`}},
			{"sublime", "Sublime Text", []string{pf + `\Sublime Text\sublime_text.exe`, pf + `\Sublime Text 3\sublime_text.exe`}},
			{"notepadpp", "Notepad++", []string{pf + `\Notepad++\notepad++.exe`, pf86 + `\Notepad++\notepad++.exe`}},
		} {
			for _, p := range c.paths {
				if fileExists(p) {
					add(c.id, c.name, p)
					break
				}
			}
		}
		var jb []string
		for _, pattern := range []string{pf + `\JetBrains\*\bin\*64.exe`, local + `\Programs\*\bin\*64.exe`} {
			m, _ := filepath.Glob(pattern)
			jb = append(jb, m...)
		}
		for _, p := range jb {
			if jetbrainsLaunchers[strings.TrimSuffix(strings.ToLower(filepath.Base(p)), "64.exe")] {
				dir := filepath.Base(filepath.Dir(filepath.Dir(p)))
				add("jb-"+strings.ToLower(dir), dir, p)
			}
		}
		add("notepad", "Notepad", filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe"))
	case "darwin":
		home, _ := os.UserHomeDir()
		var apps []string
		for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
			m, _ := filepath.Glob(filepath.Join(dir, "*.app"))
			apps = append(apps, m...)
		}
		apps = append(apps, "/System/Applications/TextEdit.app")
		for _, app := range apps {
			name := strings.TrimSuffix(filepath.Base(app), ".app")
			if !macEditor(name) {
				continue
			}
			if _, err := os.Stat(app); err == nil {
				add("app-"+strings.ToLower(name), name, "open", "-a", app)
			}
		}
	default:
		for _, c := range []struct{ id, name, bin string }{
			{"vscode", "Visual Studio Code", "code"},
			{"cursor", "Cursor", "cursor"},
			{"windsurf", "Windsurf", "windsurf"},
			{"zed", "Zed", "zeditor"},
			{"zed", "Zed", "zed"},
			{"sublime", "Sublime Text", "subl"},
			{"kate", "Kate", "kate"},
			{"gnome-text-editor", "Text Editor", "gnome-text-editor"},
			{"gedit", "gedit", "gedit"},
			{"kwrite", "KWrite", "kwrite"},
			{"mousepad", "Mousepad", "mousepad"},
			{"xed", "Xed", "xed"},
			{"pluma", "Pluma", "pluma"},
			{"geany", "Geany", "geany"},
			{"gvim", "gVim", "gvim"},
		} {
			if p, err := exec.LookPath(c.bin); err == nil {
				add(c.id, c.name, p)
			}
		}
		home, _ := os.UserHomeDir()
		scripts, _ := filepath.Glob(filepath.Join(home, ".local/share/JetBrains/Toolbox/scripts/*"))
		for name := range jetbrainsLaunchers {
			if p, err := exec.LookPath(name); err == nil {
				scripts = append(scripts, p)
			}
		}
		for _, p := range scripts {
			base := strings.TrimSuffix(filepath.Base(p), ".sh")
			if jetbrainsLaunchers[base] {
				add("jb-"+base, jetbrainsNames[base], p)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

var jetbrainsLaunchers = map[string]bool{
	"idea": true, "goland": true, "phpstorm": true, "webstorm": true, "pycharm": true,
	"clion": true, "rider": true, "rubymine": true, "datagrip": true, "rustrover": true,
}

var jetbrainsNames = map[string]string{
	"idea": "IntelliJ IDEA", "goland": "GoLand", "phpstorm": "PhpStorm", "webstorm": "WebStorm", "pycharm": "PyCharm",
	"clion": "CLion", "rider": "Rider", "rubymine": "RubyMine", "datagrip": "DataGrip", "rustrover": "RustRover",
}

func macEditor(app string) bool {
	for _, prefix := range []string{
		"Visual Studio Code", "Cursor", "Windsurf", "Zed", "Sublime Text", "BBEdit", "CotEditor", "Nova", "TextMate", "TextEdit",
		"IntelliJ IDEA", "GoLand", "PhpStorm", "WebStorm", "PyCharm", "CLion", "Rider", "RubyMine", "DataGrip", "RustRover",
	} {
		if strings.HasPrefix(app, prefix) {
			return true
		}
	}
	return false
}

// openWithSystem opens p with the default app for its type.
func openWithSystem(p string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", p)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", p)
	default:
		cmd = exec.Command("xdg-open", p)
	}
	return startDetached(cmd)
}

func startDetached(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func launch(editorID, p string) error {
	if editorID == SystemEditor {
		return openWithSystem(p)
	}
	e, ok := findEditor(editorID)
	if !ok {
		return errors.New("that editor is no longer installed")
	}
	argv := append(append([]string{}, e.argv...), p)
	if err := startDetached(exec.Command(argv[0], argv[1:]...)); err != nil {
		return fmt.Errorf("could not start %s: %w", e.Name, err)
	}
	return nil
}

// OpenLocalWith opens a local file in an installed editor (or SystemEditor).
func OpenLocalWith(editorID, p string) error { return launch(editorID, p) }

// RevealLocal shows a local file or folder in the system file manager.
func RevealLocal(p string) error { return startDetached(revealCmd(p)) }

// ---------------------------------------------------------------- remote edits

// editWatch tracks a remote file downloaded for an external editor.
type editWatch struct {
	sessionID, remote, local, dir string
	cancel                        context.CancelFunc

	mu        sync.Mutex
	remoteMod int64 // server mtime at download or last upload
	localMod  time.Time
	localSize int64
	unsynced  bool // local changes not on the server yet
}

func editKey(sessionID, remote string) string { return sessionID + "\x00" + remote }

// safeLocalName makes a remote file name valid on this computer.
func safeLocalName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		name = "file"
	}
	return name
}

// OpenRemoteWith downloads a remote file to a private temp folder and opens
// it in an installed editor. Each save in the editor is uploaded back.
func (m *Manager) OpenRemoteWith(id, remote, editorID string) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	remote = path.Clean(remote)
	m.mu.Lock()
	w := m.edits[editKey(id, remote)]
	m.mu.Unlock()
	if w != nil {
		// Already open: reopen the same copy so unsynced edits aren't lost.
		return launch(editorID, w.local)
	}
	fi, err := s.sftp.Stat(remote)
	if err != nil {
		return remoteErr(err, remote)
	}
	if fi.IsDir() {
		return fmt.Errorf("%s is a folder", remote)
	}
	if fi.Size() > maxExternalEditSize {
		return fmt.Errorf("%s is too large to open in an editor (the limit is 200 MB)", path.Base(remote))
	}
	dir, err := os.MkdirTemp("", "remvyn-edit-")
	if err != nil {
		return err
	}
	local := filepath.Join(dir, safeLocalName(path.Base(remote)))
	if err := downloadTo(s, remote, local); err != nil {
		os.RemoveAll(dir)
		return err
	}
	lfi, err := os.Stat(local)
	if err != nil {
		os.RemoveAll(dir)
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	w = &editWatch{sessionID: id, remote: remote, local: local, dir: dir, cancel: cancel,
		remoteMod: fi.ModTime().UnixMilli(), localMod: lfi.ModTime(), localSize: lfi.Size()}
	m.mu.Lock()
	m.edits[editKey(id, remote)] = w
	m.mu.Unlock()
	if err := launch(editorID, local); err != nil {
		m.stopEdit(w)
		return err
	}
	go m.watchEdit(ctx, w)
	return nil
}

func downloadTo(s *sftpSession, remote, local string) error {
	src, err := s.sftp.Open(remote)
	if err != nil {
		return remoteErr(err, remote)
	}
	defer src.Close()
	dst, err := os.OpenFile(local, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	return err
}

// watchEdit polls the local copy and uploads it once it stops changing.
// Polling (rather than file events) also catches editors that save by
// writing a new file and renaming it over the old one.
func (m *Manager) watchEdit(ctx context.Context, w *editWatch) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var pendingMod time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		fi, err := os.Stat(w.local)
		if err != nil {
			continue // mid-save rename
		}
		w.mu.Lock()
		changed := !fi.ModTime().Equal(w.localMod) || fi.Size() != w.localSize
		w.mu.Unlock()
		if !changed {
			pendingMod = time.Time{}
			continue
		}
		// Wait one more tick for the save to settle.
		if !fi.ModTime().Equal(pendingMod) {
			pendingMod = fi.ModTime()
			continue
		}
		pendingMod = time.Time{}
		m.uploadEdit(w, false)
	}
}

// uploadEdit sends the local copy to the server and reports the result.
func (m *Manager) uploadEdit(w *editWatch, force bool) {
	ev := EditSync{SessionID: w.sessionID, Path: w.remote, Name: path.Base(w.remote)}
	err := func() error {
		w.mu.Lock()
		defer w.mu.Unlock()
		fi, err := os.Stat(w.local)
		if err != nil {
			return err
		}
		w.localMod, w.localSize, w.unsynced = fi.ModTime(), fi.Size(), true
		s, err := m.get(w.sessionID)
		if err != nil {
			return fmt.Errorf("not connected; your changes are kept in %s", w.local)
		}
		rfi, err := s.sftp.Stat(w.remote)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return remoteErr(err, w.remote)
		}
		if rfi != nil && !force && rfi.ModTime().UnixMilli() != w.remoteMod {
			ev.Conflict = true
			return errors.New("the file was changed on the server since you opened it")
		}
		data, err := os.ReadFile(w.local)
		if err != nil {
			return err
		}
		if err := writeRemote(s.sftp, w.remote, data, rfi); err != nil {
			return remoteErr(err, w.remote)
		}
		if nfi, err := s.sftp.Stat(w.remote); err == nil {
			w.remoteMod = nfi.ModTime().UnixMilli()
		}
		w.unsynced = false
		return nil
	}()
	if err != nil {
		ev.Error = err.Error()
	}
	m.emit(EventEditSync, ev)
}

// ForceEditUpload uploads an externally edited file even though the server
// copy changed since it was downloaded.
func (m *Manager) ForceEditUpload(id, remote string) error {
	m.mu.Lock()
	w := m.edits[editKey(id, path.Clean(remote))]
	m.mu.Unlock()
	if w == nil {
		return errors.New("that file is no longer being edited")
	}
	m.uploadEdit(w, true)
	return nil
}

// stopEdit stops watching w and deletes its temp copy unless it holds
// changes that never reached the server.
func (m *Manager) stopEdit(w *editWatch) {
	w.cancel()
	m.mu.Lock()
	if m.edits[editKey(w.sessionID, w.remote)] == w {
		delete(m.edits, editKey(w.sessionID, w.remote))
	}
	m.mu.Unlock()
	w.mu.Lock()
	keep := w.unsynced
	w.mu.Unlock()
	if !keep {
		os.RemoveAll(w.dir)
	}
}

// stopEdits stops every external edit of a session.
func (m *Manager) stopEdits(sessionID string) {
	m.mu.Lock()
	var ws []*editWatch
	for _, w := range m.edits {
		if w.sessionID == sessionID {
			ws = append(ws, w)
		}
	}
	m.mu.Unlock()
	for _, w := range ws {
		m.stopEdit(w)
	}
}
