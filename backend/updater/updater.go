// Package updater checks GitHub Releases for a newer Remvyn and installs it
// in place: download the asset for this platform, verify its SHA-256 against
// the digest GitHub publishes, put it where the running app lives, and start
// a small helper that relaunches Remvyn once this process has exited.
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultRepo is where releases are published.
const DefaultRepo = "Rajvira10/remvyn"

// States reported in Status.State.
const (
	StateIdle        = "idle"
	StateChecking    = "checking"
	StateUpToDate    = "uptodate"
	StateAvailable   = "available"
	StateDownloading = "downloading"
	StateInstalling  = "installing"
	StateRestarting  = "restarting"
	StateError       = "error"
)

// Release is the newest published release.
type Release struct {
	Version     string    `json:"version"`
	Name        string    `json:"name"`
	Notes       string    `json:"notes"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"publishedAt"`
}

// Status is what the UI shows.
type Status struct {
	Current string   `json:"current"`
	State   string   `json:"state"`
	Latest  *Release `json:"latest,omitempty"`
	// Available is true when Latest is newer than Current.
	Available bool      `json:"available"`
	CheckedAt time.Time `json:"checkedAt"`
	// Downloaded and Total are bytes, while downloading.
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
	Error      string `json:"error,omitempty"`
	// Manual explains why this copy can't update itself (for example it
	// was installed by a package manager); the UI links to the release.
	Manual string `json:"manual,omitempty"`
}

// Options configures an Updater.
type Options struct {
	Current string
	Repo    string // owner/name; DefaultRepo when empty
	API     string // GitHub API base; https://api.github.com when empty
	Emit    func(Status)
	Client  *http.Client
}

// Updater is safe for concurrent use.
type Updater struct {
	opts   Options
	client *http.Client

	mu     sync.Mutex
	status Status
	asset  *asset
	busy   bool
}

type asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"` // "sha256:<hex>"
}

// New returns an Updater for the running version.
func New(o Options) *Updater {
	if o.Repo == "" {
		o.Repo = DefaultRepo
	}
	if o.API == "" {
		o.API = "https://api.github.com"
	}
	c := o.Client
	if c == nil {
		c = &http.Client{Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		}}
	}
	return &Updater{opts: o, client: c, status: Status{Current: o.Current, State: StateIdle}}
}

// Status returns the current state.
func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}

func (u *Updater) set(f func(*Status)) Status {
	u.mu.Lock()
	f(&u.status)
	s := u.status
	u.mu.Unlock()
	if u.opts.Emit != nil {
		u.opts.Emit(s)
	}
	return s
}

// Check asks GitHub for the latest release.
func (u *Updater) Check(ctx context.Context) (Status, error) {
	u.mu.Lock()
	if u.busy {
		s := u.status
		u.mu.Unlock()
		return s, nil
	}
	u.busy = true
	u.mu.Unlock()
	defer func() { u.mu.Lock(); u.busy = false; u.mu.Unlock() }()

	u.set(func(s *Status) { s.State = StateChecking; s.Error = "" })
	rel, a, err := u.latest(ctx)
	if err != nil {
		return u.set(func(s *Status) { s.State = StateError; s.Error = err.Error() }), err
	}
	u.mu.Lock()
	u.asset = a
	u.mu.Unlock()
	return u.set(func(s *Status) {
		s.CheckedAt = time.Now()
		s.Latest = rel
		s.Manual = ""
		s.Available = Newer(rel.Version, u.opts.Current)
		if !s.Available {
			s.State = StateUpToDate
			return
		}
		s.State = StateAvailable
		if a == nil {
			s.Manual = "There is no download for this platform in the release."
		} else if _, perr := plan(); perr != nil {
			s.Manual = perr.Error()
		}
	}), nil
}

func (u *Updater) latest(ctx context.Context) (*Release, *asset, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(u.opts.API, "/")+"/repos/"+u.opts.Repo+"/releases/latest", nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Remvyn/"+u.opts.Current)
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("could not reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var body struct {
		Tag         string    `json:"tag_name"`
		Name        string    `json:"name"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
		Assets      []asset   `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return nil, nil, fmt.Errorf("unexpected reply from GitHub: %w", err)
	}
	rel := &Release{Version: strings.TrimPrefix(body.Tag, "v"), Name: body.Name, Notes: body.Body, URL: body.HTMLURL, PublishedAt: body.PublishedAt}
	suffix, _ := plan()
	return rel, pickAsset(body.Assets, suffix), nil
}

func pickAsset(assets []asset, suffix string) *asset {
	if suffix == "" {
		return nil
	}
	for i := range assets {
		if strings.HasSuffix(assets[i].Name, suffix) {
			return &assets[i]
		}
	}
	return nil
}

// Install downloads and installs the release found by the last Check, then
// starts the helper that relaunches Remvyn. The caller must quit the app
// promptly after it returns nil.
func (u *Updater) Install(ctx context.Context) error {
	u.mu.Lock()
	if u.busy {
		u.mu.Unlock()
		return errors.New("an update is already in progress")
	}
	a, st := u.asset, u.status
	if st.Latest == nil || !Newer(st.Latest.Version, u.opts.Current) {
		u.mu.Unlock()
		return errors.New("no update is available; check for updates first")
	}
	if a == nil || st.Manual != "" {
		u.mu.Unlock()
		return errors.New("this copy of Remvyn can't update itself; download the new version from the release page")
	}
	u.busy = true
	u.mu.Unlock()
	defer func() { u.mu.Lock(); u.busy = false; u.mu.Unlock() }()

	fail := func(err error) error {
		u.set(func(s *Status) { s.State = StateError; s.Error = err.Error() })
		return err
	}
	dir := filepath.Join(os.TempDir(), tempDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fail(err)
	}
	file := filepath.Join(dir, filepath.Base(a.Name))
	u.set(func(s *Status) { s.State = StateDownloading; s.Error = ""; s.Downloaded = 0; s.Total = a.Size })
	if err := u.download(ctx, a, file); err != nil {
		_ = os.Remove(file)
		return fail(err)
	}
	u.set(func(s *Status) { s.State = StateInstalling })
	if err := install(file); err != nil {
		return fail(fmt.Errorf("install failed: %w", err))
	}
	u.set(func(s *Status) { s.State = StateRestarting })
	return nil
}

func (u *Updater) download(ctx context.Context, a *asset, dst string) error {
	want, ok := strings.CutPrefix(a.Digest, "sha256:")
	if !ok || len(want) != 64 {
		return errors.New("the release has no SHA-256 checksum for " + a.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Remvyn/"+u.opts.Current)
	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	total := resp.ContentLength
	if total <= 0 {
		total = a.Size
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	pw := &progress{total: total, report: func(n, t int64) {
		u.set(func(s *Status) { s.Downloaded = n; s.Total = t })
	}}
	_, err = io.Copy(io.MultiWriter(f, h, pw), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	pw.flush()
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, want) {
		return errors.New("the download is corrupt (checksum mismatch); try again")
	}
	return nil
}

// progress reports at most a few times a second.
type progress struct {
	n, total int64
	last     time.Time
	report   func(n, total int64)
}

func (p *progress) Write(b []byte) (int, error) {
	p.n += int64(len(b))
	if time.Since(p.last) > 200*time.Millisecond {
		p.flush()
	}
	return len(b), nil
}

func (p *progress) flush() {
	p.last = time.Now()
	p.report(p.n, p.total)
}

const tempDirName = "remvyn-update"

// Cleanup removes files left by a previous update. Call it at startup.
func Cleanup() {
	_ = os.RemoveAll(filepath.Join(os.TempDir(), tempDirName))
	cleanupPlatform()
}

// Newer reports whether version a is newer than b ("2.10.0" > "2.9.1").
// A leading "v" and any "-pre"/"+build" suffix are ignored.
func Newer(a, b string) bool {
	pa, pb := parse(a), parse(b)
	for i := range 3 {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parse(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out [3]int
	for i, p := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

// writable reports whether files can be created in dir.
func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".remvyn-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

// executable is the running binary with symlinks resolved.
func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return exe, nil
}
