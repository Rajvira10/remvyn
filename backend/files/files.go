// Package files implements the SFTP browser: remote and local directory
// operations plus recursive uploads/downloads with progress and cancellation.
package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"

	"remvyn/backend/ids"
	"remvyn/backend/models"
	"remvyn/backend/sshconn"
)

// EventTransfer carries models.Transfer updates.
const EventTransfer = "transfer:update"

// EventSFTPClosed carries the session ID when an SFTP connection drops.
const EventSFTPClosed = "sftp:closed"

// Emitter delivers events to the UI.
type Emitter func(event string, data any)

type sftpSession struct {
	info   models.SessionInfo
	client *sshconn.Client
	sftp   *sftp.Client
}

type transfer struct {
	mu     sync.Mutex
	t      models.Transfer
	done   atomic.Int64
	cancel context.CancelFunc
}

// Manager owns SFTP sessions and transfers.
type Manager struct {
	emit Emitter

	mu        sync.Mutex
	sessions  map[string]*sftpSession
	transfers map[string]*transfer
}

// New creates a Manager.
func New(emit Emitter) *Manager {
	return &Manager{emit: emit, sessions: map[string]*sftpSession{}, transfers: map[string]*transfer{}}
}

// Open starts the SFTP subsystem on client and takes ownership of it.
func (m *Manager) Open(id string, client *sshconn.Client, info models.SessionInfo) error {
	sc, err := sftp.NewClient(client.Client,
		sftp.UseConcurrentWrites(true),
		sftp.UseConcurrentReads(true),
		sftp.MaxConcurrentRequestsPerFile(64))
	if err != nil {
		client.Close()
		return fmt.Errorf("the server does not support SFTP: %w", err)
	}
	s := &sftpSession{info: info, client: client, sftp: sc}
	m.mu.Lock()
	if _, dup := m.sessions[id]; dup {
		m.mu.Unlock()
		sc.Close()
		client.Close()
		return errors.New("session id already in use")
	}
	m.sessions[id] = s
	m.mu.Unlock()
	go func() {
		<-client.Done()
		m.mu.Lock()
		_, still := m.sessions[id]
		delete(m.sessions, id)
		m.mu.Unlock()
		if still {
			m.emit(EventSFTPClosed, id)
		}
	}()
	return nil
}

func (m *Manager) get(id string) (*sftpSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, errors.New("SFTP session is not connected")
	}
	return s, nil
}

// Close ends a session and cancels its transfers.
func (m *Manager) Close(id string) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	delete(m.sessions, id)
	var cancels []context.CancelFunc
	for _, t := range m.transfers {
		if t.t.SessionID == id && t.cancel != nil {
			cancels = append(cancels, t.cancel)
		}
	}
	m.mu.Unlock()
	for _, c := range cancels {
		c()
	}
	if ok {
		s.sftp.Close()
		s.client.Close()
	}
}

// CloseAll ends every session.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Close(id)
	}
}

// ---------------------------------------------------------------- remote ops

func entry(dir string, fi fs.FileInfo, join func(...string) string) models.FileEntry {
	return models.FileEntry{
		Name:     fi.Name(),
		Path:     join(dir, fi.Name()),
		Size:     fi.Size(),
		Mode:     fi.Mode().String(),
		ModTime:  fi.ModTime().UnixMilli(),
		IsDir:    fi.IsDir(),
		IsLink:   fi.Mode()&fs.ModeSymlink != 0,
		IsHidden: strings.HasPrefix(fi.Name(), "."),
	}
}

func sortEntries(es []models.FileEntry) {
	sort.Slice(es, func(i, j int) bool {
		if es[i].IsDir != es[j].IsDir {
			return es[i].IsDir
		}
		return strings.ToLower(es[i].Name) < strings.ToLower(es[j].Name)
	})
}

// Home returns the remote working directory.
func (m *Manager) Home(id string) (string, error) {
	s, err := m.get(id)
	if err != nil {
		return "", err
	}
	return s.sftp.Getwd()
}

// List reads a remote directory. Symlinks to directories are reported as
// directories so they can be navigated.
func (m *Manager) List(id, dir string) (models.DirListing, error) {
	s, err := m.get(id)
	if err != nil {
		return models.DirListing{}, err
	}
	if dir == "" {
		if dir, err = s.sftp.Getwd(); err != nil {
			return models.DirListing{}, err
		}
	}
	dir = path.Clean(dir)
	infos, err := s.sftp.ReadDir(dir)
	if err != nil {
		return models.DirListing{}, remoteErr(err, dir)
	}
	out := models.DirListing{Path: dir, Parent: path.Dir(dir), Entries: make([]models.FileEntry, 0, len(infos))}
	for _, fi := range infos {
		e := entry(dir, fi, path.Join)
		if e.IsLink {
			if target, err := s.sftp.Stat(e.Path); err == nil {
				e.IsDir = target.IsDir()
				if !e.IsDir {
					e.Size = target.Size()
				}
			}
		}
		out.Entries = append(out.Entries, e)
	}
	sortEntries(out.Entries)
	return out, nil
}

func remoteErr(err error, p string) error {
	var se *sftp.StatusError
	if errors.As(err, &se) {
		switch se.FxCode() {
		case sftp.ErrSSHFxPermissionDenied:
			return fmt.Errorf("permission denied: %s", p)
		case sftp.ErrSSHFxNoSuchFile:
			return fmt.Errorf("no such file or directory: %s", p)
		}
	}
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no such file or directory: %s", p)
	}
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("permission denied: %s", p)
	}
	return err
}

func validName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
		return fmt.Errorf("invalid name %q", name)
	}
	return nil
}

// Mkdir creates a remote directory inside dir.
func (m *Manager) Mkdir(id, dir, name string) error {
	if err := validName(name); err != nil {
		return err
	}
	s, err := m.get(id)
	if err != nil {
		return err
	}
	p := path.Join(dir, name)
	return remoteErr(s.sftp.Mkdir(p), p)
}

// Rename renames a remote entry within its directory.
func (m *Manager) Rename(id, from, newName string) error {
	if err := validName(newName); err != nil {
		return err
	}
	s, err := m.get(id)
	if err != nil {
		return err
	}
	to := path.Join(path.Dir(from), newName)
	if _, err := s.sftp.Lstat(to); err == nil {
		return fmt.Errorf("%s already exists", newName)
	}
	return remoteErr(s.sftp.Rename(from, to), from)
}

// Remove deletes remote paths (directories recursively).
func (m *Manager) Remove(id string, paths []string) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	for _, p := range paths {
		if p == "/" || p == "" {
			return errors.New("refusing to delete the root directory")
		}
		if err := removeRemote(s.sftp, p); err != nil {
			return remoteErr(err, p)
		}
	}
	return nil
}

func removeRemote(c *sftp.Client, p string) error {
	fi, err := c.Lstat(p)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return c.Remove(p)
	}
	children, err := c.ReadDir(p)
	if err != nil {
		return err
	}
	for _, ch := range children {
		if err := removeRemote(c, path.Join(p, ch.Name())); err != nil {
			return err
		}
	}
	return c.RemoveDirectory(p)
}

// ---------------------------------------------------------------- local ops

// LocalHome returns the user's home directory.
func LocalHome() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return string(filepath.Separator)
}

// ListLocal reads a local directory.
func ListLocal(dir string) (models.DirListing, error) {
	if dir == "" {
		dir = LocalHome()
	}
	dir = filepath.Clean(dir)
	des, err := os.ReadDir(dir)
	if err != nil {
		return models.DirListing{}, err
	}
	out := models.DirListing{Path: dir, Parent: filepath.Dir(dir), Entries: make([]models.FileEntry, 0, len(des))}
	for _, de := range des {
		fi, err := de.Info()
		if err != nil {
			continue
		}
		e := entry(dir, fi, filepath.Join)
		if e.IsLink {
			if target, err := os.Stat(e.Path); err == nil {
				e.IsDir = target.IsDir()
				if !e.IsDir {
					e.Size = target.Size()
				}
			}
		}
		out.Entries = append(out.Entries, e)
	}
	sortEntries(out.Entries)
	return out, nil
}

// MkdirLocal creates a local directory inside dir.
func MkdirLocal(dir, name string) error {
	if err := validName(name); err != nil {
		return err
	}
	return os.Mkdir(filepath.Join(dir, name), 0o755)
}

// RenameLocal renames a local entry within its directory.
func RenameLocal(from, newName string) error {
	if err := validName(newName); err != nil {
		return err
	}
	to := filepath.Join(filepath.Dir(from), newName)
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("%s already exists", newName)
	}
	return os.Rename(from, to)
}

// RemoveLocal deletes local paths (directories recursively).
func RemoveLocal(paths []string) error {
	home := LocalHome()
	for _, p := range paths {
		clean := filepath.Clean(p)
		if clean == string(filepath.Separator) || clean == home || clean == "." {
			return fmt.Errorf("refusing to delete %s", clean)
		}
		if err := os.RemoveAll(clean); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- transfers

// Transfers returns all known transfers, newest first.
func (m *Manager) Transfers() []models.Transfer {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]models.Transfer, 0, len(m.transfers))
	for _, t := range m.transfers {
		out = append(out, t.snapshot())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt > out[j].StartedAt })
	return out
}

// CancelTransfer aborts a running transfer.
func (m *Manager) CancelTransfer(id string) bool {
	m.mu.Lock()
	t, ok := m.transfers[id]
	m.mu.Unlock()
	if ok && t.cancel != nil {
		t.cancel()
	}
	return ok
}

// ClearTransfers forgets finished transfers.
func (m *Manager) ClearTransfers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, t := range m.transfers {
		st := t.snapshot().State
		if st != models.TransferRunning && st != models.TransferQueued {
			delete(m.transfers, id)
		}
	}
}

func (t *transfer) snapshot() models.Transfer {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.t
	s.BytesDone = t.done.Load()
	return s
}

func (t *transfer) update(f func(*models.Transfer)) {
	t.mu.Lock()
	f(&t.t)
	t.mu.Unlock()
}

// item is one file to copy.
type item struct {
	src, dst string
	size     int64
	dir      bool
	mode     fs.FileMode
}

// Upload copies local paths into a remote directory.
func (m *Manager) Upload(sessionID string, localPaths []string, remoteDir string) (string, error) {
	s, err := m.get(sessionID)
	if err != nil {
		return "", err
	}
	var items []item
	for _, lp := range localPaths {
		lp = filepath.Clean(lp)
		base := filepath.Base(lp)
		err := filepath.WalkDir(lp, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(lp, p)
			dst := path.Join(remoteDir, base, filepath.ToSlash(rel))
			if rel == "." {
				dst = path.Join(remoteDir, base)
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			if d.Type()&fs.ModeSymlink != 0 {
				// Follow file symlinks; skip directory symlinks to avoid cycles.
				target, err := os.Stat(p)
				if err != nil || target.IsDir() {
					return nil
				}
				fi = target
			}
			items = append(items, item{src: p, dst: dst, size: fi.Size(), dir: fi.IsDir(), mode: fi.Mode().Perm()})
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	name := describe(localPaths, filepath.Base)
	return m.start(sessionID, models.TransferUpload, name, strings.Join(localPaths, ", "), remoteDir, items,
		func(ctx context.Context, it item, counter *atomic.Int64) error {
			if it.dir {
				err := s.sftp.MkdirAll(it.dst)
				return remoteErr(err, it.dst)
			}
			src, err := os.Open(it.src)
			if err != nil {
				return err
			}
			defer src.Close()
			dst, err := s.sftp.OpenFile(it.dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
			if err != nil {
				return remoteErr(err, it.dst)
			}
			// ReadFrom uses pipelined concurrent writes, which is much faster
			// than a sequential copy on high-latency links.
			_, err = dst.ReadFrom(&progressReader{r: src, n: counter, ctx: ctx})
			if cerr := dst.Close(); err == nil {
				err = cerr
			}
			if err == nil {
				_ = s.sftp.Chmod(it.dst, it.mode)
			}
			return err
		})
}

// Download copies remote paths into a local directory.
func (m *Manager) Download(sessionID string, remotePaths []string, localDir string) (string, error) {
	s, err := m.get(sessionID)
	if err != nil {
		return "", err
	}
	var items []item
	for _, rp := range remotePaths {
		rp = path.Clean(rp)
		base := path.Base(rp)
		walker := s.sftp.Walk(rp)
		for walker.Step() {
			if err := walker.Err(); err != nil {
				return "", remoteErr(err, walker.Path())
			}
			fi := walker.Stat()
			rel := strings.TrimPrefix(strings.TrimPrefix(walker.Path(), rp), "/")
			dst := filepath.Join(localDir, base, filepath.FromSlash(rel))
			if fi.Mode()&fs.ModeSymlink != 0 {
				target, err := s.sftp.Stat(walker.Path())
				if err != nil || target.IsDir() {
					continue
				}
				fi = target
			}
			items = append(items, item{src: walker.Path(), dst: dst, size: fi.Size(), dir: fi.IsDir(), mode: fi.Mode().Perm()})
		}
	}
	name := describe(remotePaths, path.Base)
	return m.start(sessionID, models.TransferDownload, name, strings.Join(remotePaths, ", "), localDir, items,
		func(ctx context.Context, it item, counter *atomic.Int64) error {
			if it.dir {
				return os.MkdirAll(it.dst, 0o755)
			}
			src, err := s.sftp.Open(it.src)
			if err != nil {
				return remoteErr(err, it.src)
			}
			defer src.Close()
			if err := os.MkdirAll(filepath.Dir(it.dst), 0o755); err != nil {
				return err
			}
			mode := it.mode
			if mode == 0 {
				mode = 0o644
			}
			// Write to a temp file and rename so a cancelled download never
			// leaves a truncated file under the real name.
			tmp := it.dst + ".remvyn-part"
			dst, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			_, err = io.Copy(&progressWriter{w: dst, n: counter, ctx: ctx}, src)
			if cerr := dst.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				os.Remove(tmp)
				return err
			}
			return os.Rename(tmp, it.dst)
		})
}

func describe(paths []string, base func(string) string) string {
	if len(paths) == 1 {
		return base(paths[0])
	}
	return fmt.Sprintf("%d items", len(paths))
}

func (m *Manager) start(sessionID, dir, name, source, dest string, items []item,
	copyFn func(context.Context, item, *atomic.Int64) error) (string, error) {
	if len(items) == 0 {
		return "", errors.New("nothing to transfer")
	}
	var total int64
	files := 0
	for _, it := range items {
		if !it.dir {
			total += it.size
			files++
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	t := &transfer{cancel: cancel, t: models.Transfer{
		ID: ids.New(), SessionID: sessionID, Direction: dir, Source: source, Dest: dest, Name: name,
		State: models.TransferRunning, BytesTotal: total, FilesTotal: files, StartedAt: time.Now().UnixMilli(),
	}}
	m.mu.Lock()
	m.transfers[t.t.ID] = t
	m.mu.Unlock()
	m.emit(EventTransfer, t.snapshot())

	go func() {
		defer cancel()
		stop := make(chan struct{})
		go func() {
			tick := time.NewTicker(200 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-stop:
					return
				case <-tick.C:
					m.emit(EventTransfer, t.snapshot())
				}
			}
		}()
		var err error
		for _, it := range items {
			if ctx.Err() != nil {
				err = ctx.Err()
				break
			}
			if err = copyFn(ctx, it, &t.done); err != nil {
				break
			}
			if !it.dir {
				t.update(func(x *models.Transfer) { x.FilesDone++ })
			}
		}
		close(stop)
		t.update(func(x *models.Transfer) {
			x.FinishedAt = time.Now().UnixMilli()
			switch {
			case err == nil:
				x.State = models.TransferDone
			case errors.Is(err, context.Canceled):
				x.State = models.TransferCancelled
			default:
				x.State = models.TransferFailed
				x.Error = err.Error()
			}
		})
		m.emit(EventTransfer, t.snapshot())
	}()
	return t.t.ID, nil
}

type progressWriter struct {
	w   io.Writer
	n   *atomic.Int64
	ctx context.Context
}

func (p *progressWriter) Write(b []byte) (int, error) {
	if err := p.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := p.w.Write(b)
	p.n.Add(int64(n))
	return n, err
}

type progressReader struct {
	r   io.Reader
	n   *atomic.Int64
	ctx context.Context
}

func (p *progressReader) Read(b []byte) (int, error) {
	if err := p.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := p.r.Read(b)
	p.n.Add(int64(n))
	return n, err
}
