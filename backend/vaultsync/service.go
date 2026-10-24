package vaultsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"flux/backend/storage"
)

// Events emitted to the UI.
const (
	EventStatus  = "sync:status"
	EventChanged = "vault:changed"
)

const metaKey = "sync"

// Config is persisted in the settings table. Secrets are sealed with the
// local data key, so they're unavailable while Flux is locked.
type Config struct {
	ClientID    string `json:"clientId,omitempty"`
	AppSlug     string `json:"appSlug,omitempty"`
	Repo        string `json:"repo,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Login       string `json:"login,omitempty"`
	TokenSealed []byte `json:"token,omitempty"`
	KeySealed   []byte `json:"key,omitempty"`
	LastSyncAt  int64  `json:"lastSyncAt,omitempty"`
}

func (c Config) connected() bool { return c.Repo != "" && len(c.KeySealed) > 0 }

// Status is shown in Settings → Sync.
type Status struct {
	// AppConfigured is true when a GitHub App client ID is known.
	AppConfigured bool   `json:"appConfigured"`
	ClientID      string `json:"clientId"`
	AppSlug       string `json:"appSlug"`
	// BuiltInApp is true when the app came from the build, not Settings.
	BuiltInApp bool   `json:"builtInApp"`
	Connected  bool   `json:"connected"`
	Repo       string `json:"repo"`
	RepoURL    string `json:"repoUrl"`
	Login      string `json:"login"`
	// State is off, idle, syncing, error or locked.
	State      string `json:"state"`
	LastSyncAt int64  `json:"lastSyncAt"`
	Error      string `json:"error"`
	ErrorCode  string `json:"errorCode"`
	InstallURL string `json:"installUrl"`
	NewRepoURL string `json:"newRepoUrl"`
}

// RepoInfo describes a candidate repository.
type RepoInfo struct {
	Repo
	HasVault bool `json:"hasVault"`
	Empty    bool `json:"empty"`
}

// Options configure the service.
type Options struct {
	Store     *storage.Store
	Emit      func(event string, data any)
	Endpoints Endpoints
	// Built-in GitHub App (from the build); Settings can supply one otherwise.
	ClientID string
	AppSlug  string
	// Interval between background passes (default 60s).
	Interval time.Duration
	// Debounce after a local change (default 2s).
	Debounce time.Duration
	// PollUnit scales device-flow polling intervals (tests).
	PollUnit time.Duration
	Now      func() time.Time
}

// Service owns sync configuration, sign-in and the background loop.
type Service struct {
	o      Options
	device string

	mu        sync.Mutex
	cfg       Config
	engine    *Engine
	state     string
	lastErr   error
	paused    bool // after an auth error, until the user signs in again
	pendingDC *DeviceCode
	pendingTk string
	loginStop context.CancelFunc

	wake    chan struct{}
	changed chan struct{}
	stop    chan struct{}
	done    chan struct{}
	runMu   sync.Mutex // one pass at a time
}

// New loads configuration and starts the background loop.
func New(o Options) (*Service, error) {
	if o.Endpoints.API == "" {
		o.Endpoints = DefaultEndpoints
	}
	if o.Interval == 0 {
		o.Interval = time.Minute
	}
	if o.Debounce == 0 {
		o.Debounce = 2 * time.Second
	}
	if o.Emit == nil {
		o.Emit = func(string, any) {}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	s := &Service{o: o, state: "off", wake: make(chan struct{}, 1), changed: make(chan struct{}, 1),
		stop: make(chan struct{}), done: make(chan struct{})}
	s.device, _ = os.Hostname()
	s.device = strings.TrimSuffix(s.device, ".local")
	if _, err := o.Store.GetMeta(context.Background(), metaKey, &s.cfg); err != nil {
		return nil, err
	}
	if s.cfg.connected() {
		s.state = "idle"
	}
	go s.loop()
	return s, nil
}

// Close stops the loop.
func (s *Service) Close() {
	s.mu.Lock()
	if s.loginStop != nil {
		s.loginStop()
	}
	s.mu.Unlock()
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.done
}

func (s *Service) clientID() (string, string, bool) {
	if s.o.ClientID != "" {
		return s.o.ClientID, s.o.AppSlug, true
	}
	return s.cfg.ClientID, s.cfg.AppSlug, false
}

// Status returns the current state.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *Service) statusLocked() Status {
	id, slug, builtIn := s.clientID()
	st := Status{AppConfigured: id != "", ClientID: id, AppSlug: slug, BuiltInApp: builtIn, Connected: s.cfg.connected(),
		Repo: s.cfg.Repo, Login: s.cfg.Login, State: s.state, LastSyncAt: s.cfg.LastSyncAt,
		NewRepoURL: s.o.Endpoints.Web + "/new?" + url.Values{"name": {"flux-vault"}, "visibility": {"private"},
			"description": {"Encrypted Flux vault (synced by the Flux app)"}}.Encode()}
	if slug != "" {
		st.InstallURL = s.o.Endpoints.Web + "/apps/" + url.PathEscape(slug) + "/installations/new"
	}
	if s.cfg.Repo != "" {
		st.RepoURL = s.o.Endpoints.Web + "/" + s.cfg.Repo
	}
	if st.Connected && s.o.Store.Locked() && s.state != "syncing" {
		st.State = "locked"
	}
	if s.lastErr != nil {
		st.Error = s.lastErr.Error()
		st.ErrorCode = ErrorCode(s.lastErr)
	}
	return st
}

func (s *Service) emitStatus() { s.o.Emit(EventStatus, s.Status()) }

func (s *Service) saveLocked() error {
	return s.o.Store.SetMeta(context.Background(), metaKey, s.cfg)
}

// SetApp stores a GitHub App client ID and slug entered in Settings.
func (s *Service) SetApp(clientID, slug string) error {
	clientID, slug = strings.TrimSpace(clientID), strings.Trim(strings.TrimSpace(slug), "/")
	if i := strings.LastIndex(slug, "/apps/"); i >= 0 {
		slug = slug[i+len("/apps/"):]
	}
	if clientID == "" {
		return errors.New("enter the app's client ID")
	}
	if strings.ContainsAny(clientID, " /") || strings.ContainsAny(slug, " /?") {
		return errors.New("that doesn't look like a GitHub App client ID and name")
	}
	s.mu.Lock()
	s.cfg.ClientID, s.cfg.AppSlug = clientID, slug
	err := s.saveLocked()
	s.mu.Unlock()
	s.emitStatus()
	return err
}

func (s *Service) gh(token string) *GitHub {
	id, _, _ := s.clientID()
	return &GitHub{Endpoints: s.o.Endpoints, ClientID: id, Token: token}
}

// ---------------------------------------------------------------- sign-in

// StartLogin begins GitHub device-flow sign-in.
func (s *Service) StartLogin(ctx context.Context) (DeviceCode, error) {
	s.mu.Lock()
	gh := s.gh("")
	s.mu.Unlock()
	dc, err := gh.StartDeviceFlow(ctx)
	if err != nil {
		return dc, err
	}
	s.mu.Lock()
	s.pendingDC = &dc
	s.pendingTk = ""
	s.mu.Unlock()
	return dc, nil
}

// FinishLogin waits for the user to approve the code and returns their
// login. If sync is already connected, the new token replaces the old one.
func (s *Service) FinishLogin(ctx context.Context) (string, error) {
	s.mu.Lock()
	dc := s.pendingDC
	if dc == nil {
		s.mu.Unlock()
		return "", errors.New("start signing in first")
	}
	ctx, cancel := context.WithCancel(ctx)
	if s.loginStop != nil {
		s.loginStop()
	}
	s.loginStop = cancel
	gh := s.gh("")
	s.mu.Unlock()
	defer cancel()

	token, err := gh.PollDeviceToken(ctx, *dc, s.o.PollUnit)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", &APIError{Code: "cancelled", Message: "sign-in cancelled"}
		}
		return "", err
	}
	gh.Token = token
	login, err := gh.User(ctx)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingDC = nil
	s.pendingTk = token
	if s.cfg.connected() {
		sealed, err := s.o.Store.SealSecret([]byte(token))
		if err != nil {
			return "", err
		}
		s.cfg.TokenSealed, s.cfg.Login = sealed, login
		s.paused, s.lastErr = false, nil
		if err := s.saveLocked(); err != nil {
			return "", err
		}
		go s.SyncNow(context.Background())
	} else {
		s.cfg.Login = login
	}
	return login, nil
}

// CancelLogin stops waiting for sign-in.
func (s *Service) CancelLogin() {
	s.mu.Lock()
	if s.loginStop != nil {
		s.loginStop()
		s.loginStop = nil
	}
	s.pendingDC = nil
	s.mu.Unlock()
}

func (s *Service) loginGH() (*GitHub, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingTk == "" {
		return nil, &APIError{Code: CodeAuth, Message: "sign in with GitHub first"}
	}
	return s.gh(s.pendingTk), nil
}

// Repos lists repositories the signed-in user gave the app access to.
func (s *Service) Repos(ctx context.Context) ([]Repo, bool, error) {
	gh, err := s.loginGH()
	if err != nil {
		return nil, false, err
	}
	return gh.AccessibleRepos(ctx)
}

func (s *Service) remote(gh *GitHub, repo Repo) *GitHubRemote {
	branch := repo.DefaultBranch
	if branch == "" {
		branch = "main"
	}
	return &GitHubRemote{GH: gh, Repo: repo.FullName, Branch: branch}
}

// Inspect reports whether a repository already holds a vault.
func (s *Service) Inspect(ctx context.Context, fullName string) (RepoInfo, error) {
	gh, err := s.loginGH()
	if err != nil {
		return RepoInfo{}, err
	}
	repo, err := gh.GetRepo(ctx, fullName)
	if err != nil {
		return RepoInfo{}, err
	}
	info := RepoInfo{Repo: repo}
	r := s.remote(gh, repo)
	ref, err := r.Ref(ctx)
	if err != nil {
		return info, err
	}
	if ref == "" {
		info.Empty = true
		return info, nil
	}
	snap, err := r.Snapshot(ctx, ref)
	if err != nil {
		return info, err
	}
	_, info.HasVault = snap.Files[vaultFile]
	return info, nil
}

// ---------------------------------------------------------------- connect

// CreateVault writes a new vault (sync key wrapped with password) to an
// empty repository and returns the sync key.
func (s *Service) CreateVault(ctx context.Context, fullName, password string) ([]byte, error) {
	gh, err := s.loginGH()
	if err != nil {
		return nil, err
	}
	repo, err := gh.GetRepo(ctx, fullName)
	if err != nil {
		return nil, err
	}
	if !repo.Private {
		return nil, errors.New("the repository must be private")
	}
	doc, key, err := NewVaultDoc(password, s.o.Now())
	if err != nil {
		return nil, err
	}
	r := s.remote(gh, repo)
	for attempt := 0; attempt < 4; attempt++ {
		ref, err := r.Ref(ctx)
		if err != nil {
			return nil, err
		}
		snap, err := r.Snapshot(ctx, ref)
		if err != nil {
			return nil, err
		}
		if _, ok := snap.Files[vaultFile]; ok {
			return nil, errors.New("this repository already has a vault; join it instead")
		}
		_, err = r.Commit(ctx, snap, map[string][]byte{vaultFile: doc}, "Create Flux vault")
		if errors.Is(err, ErrConflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return key, nil
	}
	return nil, ErrConflict
}

// JoinVault unwraps the sync key of an existing vault with password.
func (s *Service) JoinVault(ctx context.Context, fullName, password string) ([]byte, error) {
	gh, err := s.loginGH()
	if err != nil {
		return nil, err
	}
	repo, err := gh.GetRepo(ctx, fullName)
	if err != nil {
		return nil, err
	}
	r := s.remote(gh, repo)
	ref, err := r.Ref(ctx)
	if err != nil {
		return nil, err
	}
	snap, err := r.Snapshot(ctx, ref)
	if err != nil {
		return nil, err
	}
	sha, ok := snap.Files[vaultFile]
	if !ok {
		return nil, errors.New("this repository doesn't contain a Flux vault")
	}
	raw, err := r.Read(ctx, sha)
	if err != nil {
		return nil, err
	}
	return OpenVaultDoc(raw, password)
}

// Activate saves the connection and starts syncing. The store must be
// unlocked (the token and key are sealed with the local data key).
func (s *Service) Activate(ctx context.Context, fullName string, syncKey []byte) error {
	gh, err := s.loginGH()
	if err != nil {
		return err
	}
	repo, err := gh.GetRepo(ctx, fullName)
	if err != nil {
		return err
	}
	tok, err := s.o.Store.SealSecret([]byte(gh.Token))
	if err != nil {
		return err
	}
	key, err := s.o.Store.SealSecret(syncKey)
	if err != nil {
		return err
	}
	if err := s.o.Store.ClearSyncState(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg.Repo, s.cfg.Branch, s.cfg.TokenSealed, s.cfg.KeySealed = repo.FullName, s.remote(gh, repo).Branch, tok, key
	s.cfg.LastSyncAt = 0
	s.engine, s.lastErr, s.paused, s.state = nil, nil, false, "idle"
	s.pendingTk = ""
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	s.emitStatus()
	return s.SyncNow(ctx)
}

// Disconnect stops syncing on this device. The repository is left as is.
func (s *Service) Disconnect() error {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.mu.Lock()
	s.cfg = Config{ClientID: s.cfg.ClientID, AppSlug: s.cfg.AppSlug}
	s.engine, s.lastErr, s.paused, s.state = nil, nil, false, "off"
	err := s.saveLocked()
	s.mu.Unlock()
	if err == nil {
		err = s.o.Store.ClearSyncState(context.Background())
	}
	s.emitStatus()
	return err
}

// Connected reports whether sync is set up.
func (s *Service) Connected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.connected()
}

// ChangePassword re-wraps the sync key in the repository with a new master
// password. Other devices keep syncing; they need the new password only
// when they join again.
func (s *Service) ChangePassword(ctx context.Context, password string) error {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	eng, err := s.ensureEngine()
	if err != nil {
		return err
	}
	s.mu.Lock()
	keySealed := s.cfg.KeySealed
	s.mu.Unlock()
	key, err := s.o.Store.OpenSecret(keySealed)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		snap, err := eng.head(ctx)
		if err != nil {
			return err
		}
		if err := eng.checkVault(ctx, snap); err != nil {
			return err
		}
		raw, err := eng.Remote.Read(ctx, snap.Files[vaultFile])
		if err != nil {
			return err
		}
		var created int64
		if d, err := parseVaultDoc(raw); err == nil {
			created = d.CreatedAt
		}
		doc, err := vaultDocFor(key, password, created)
		if err != nil {
			return err
		}
		next, err := eng.Remote.Commit(ctx, snap, map[string][]byte{vaultFile: doc}, "Change master password")
		if errors.Is(err, ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		eng.cache, eng.vaultSHA = next, next.Files[vaultFile]
		return nil
	}
	return ErrConflict
}

// ---------------------------------------------------------------- loop

// Changed schedules a pass after local edits.
func (s *Service) Changed() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// Wake schedules an immediate pass (window focus, unlock).
func (s *Service) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) loop() {
	defer close(s.done)
	tick := time.NewTicker(s.o.Interval)
	defer tick.Stop()
	var debounce <-chan time.Time
	for {
		select {
		case <-s.stop:
			return
		case <-s.changed:
			s.Scan()
			debounce = time.After(s.o.Debounce)
			continue
		case <-debounce:
			debounce = nil
		case <-s.wake:
		case <-tick.C:
		}
		_ = s.run(context.Background(), false)
	}
}

// SyncNow runs a pass immediately and returns its error.
func (s *Service) SyncNow(ctx context.Context) error { return s.run(ctx, true) }

func (s *Service) ensureEngine() (*Engine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.cfg.connected() {
		return nil, errors.New("sync is not set up")
	}
	if s.engine != nil {
		return s.engine, nil
	}
	tok, err := s.o.Store.OpenSecret(s.cfg.TokenSealed)
	if err != nil {
		return nil, err
	}
	key, err := s.o.Store.OpenSecret(s.cfg.KeySealed)
	if err != nil {
		return nil, err
	}
	c, err := NewCipher(key)
	if err != nil {
		return nil, err
	}
	s.engine = &Engine{Src: s.o.Store, Cipher: c, Device: s.device, Now: s.o.Now,
		Remote: &GitHubRemote{GH: s.gh(string(tok)), Repo: s.cfg.Repo, Branch: s.cfg.Branch}}
	return s.engine, nil
}

func (s *Service) run(ctx context.Context, manual bool) error {
	s.mu.Lock()
	connected, paused := s.cfg.connected(), s.paused
	s.mu.Unlock()
	if !connected {
		return errors.New("sync is not set up")
	}
	if s.o.Store.Locked() {
		s.emitStatus()
		return storage.ErrLocked
	}
	if paused && !manual {
		return nil
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	eng, err := s.ensureEngine()
	if err != nil {
		s.finish(Result{}, err)
		return err
	}
	s.mu.Lock()
	s.state = "syncing"
	s.mu.Unlock()
	s.emitStatus()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	res, err := eng.Sync(ctx)
	s.finish(res, err)
	return err
}

func (s *Service) finish(res Result, err error) {
	s.mu.Lock()
	if err != nil {
		s.state, s.lastErr = "error", err
		switch ErrorCode(err) {
		case CodeAuth, ErrVaultReset.Code, ErrVaultMissing.Code, CodeNoAccess:
			s.paused = true
		}
		if errors.Is(err, storage.ErrLocked) {
			s.state, s.lastErr = "idle", nil
		}
	} else {
		s.state, s.lastErr, s.paused = "idle", nil, false
		s.cfg.LastSyncAt = s.o.Now().UnixMilli()
		_ = s.saveLocked()
	}
	s.mu.Unlock()
	if err == nil && res.Changed() {
		s.o.Emit(EventChanged, res)
	}
	s.emitStatus()
}

// Scan records the time of local edits without contacting GitHub, so edits
// made offline keep their real time when resolving conflicts.
func (s *Service) Scan() {
	if s.o.Store.Locked() || !s.Connected() {
		return
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if eng, err := s.ensureEngine(); err == nil {
		_ = eng.Scan(context.Background())
	}
}

// Lock drops cached secrets; the next pass after unlocking rebuilds them.
func (s *Service) Lock() {
	s.runMu.Lock()
	s.mu.Lock()
	s.engine = nil
	s.mu.Unlock()
	s.runMu.Unlock()
	s.emitStatus()
}

func parseVaultDoc(raw []byte) (vaultDoc, error) {
	var d vaultDoc
	err := json.Unmarshal(raw, &d)
	if err == nil && d.Format != vaultFormat {
		err = fmt.Errorf("not a Flux vault")
	}
	return d, err
}
