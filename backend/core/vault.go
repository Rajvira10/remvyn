package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"flux/backend/models"
	"flux/backend/security"
	"flux/backend/sshconn"
	"flux/backend/vaultsync"
)

// Lock and sync error codes.
const (
	CodeLocked        = "locked"
	CodeWrongPassword = "wrong_password"
	CodeWeakPassword  = "weak_password"
	CodeNoPassword    = "no_master_password"
)

// EventLock is emitted when Flux locks or unlocks.
const EventLock = "lock:status"

// Start runs background work once the UI is up: auto-start forwards,
// monitoring and sync. While locked, forwards and sync wait for Unlock.
func (c *Core) Start() {
	c.keyMu.Lock()
	c.started = true
	unlocked := !c.store.Locked()
	c.keyMu.Unlock()
	c.StartMonitoring()
	if unlocked {
		c.afterUnlock()
	}
}

func (c *Core) afterUnlock() {
	c.StartAutoForwards()
	c.applyMonitoring()
	c.sync.Wake()
}

// LockStatus reports whether a master password is set and whether Flux is
// locked.
func (c *Core) LockStatus() models.LockStatus {
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	return c.lockStatusLocked()
}

func (c *Core) lockStatusLocked() models.LockStatus {
	return models.LockStatus{Protected: c.protected, Locked: c.store.Locked(), MinLength: security.MinPasswordLength,
		SyncConnected: c.sync.Connected()}
}

func (c *Core) emitLock() { c.emit(EventLock, c.LockStatus()) }

func wrongPassword() error {
	return sshconn.Coded(CodeWrongPassword, "that master password is not correct", nil)
}

func checkStrength(pw string) error {
	if len(pw) < security.MinPasswordLength {
		return sshconn.Coded(CodeWeakPassword, fmt.Sprintf("use at least %d characters", security.MinPasswordLength), nil)
	}
	if strings.TrimSpace(pw) == "" {
		return sshconn.Coded(CodeWeakPassword, "the master password can't be only spaces", nil)
	}
	return nil
}

func (c *Core) installKey(key []byte) error {
	v, err := security.NewVault(key)
	if err != nil {
		return err
	}
	c.dek = key
	c.store.SetVault(v)
	return nil
}

// Unlock opens the vault with the master password.
func (c *Core) Unlock(password string) error {
	c.keyMu.Lock()
	if !c.protected || !c.store.Locked() {
		c.keyMu.Unlock()
		return nil
	}
	// Slow down repeated guesses on top of Argon2's cost.
	delay := time.Duration(c.failedTry) * 400 * time.Millisecond
	c.keyMu.Unlock()
	if delay > 0 {
		time.Sleep(min(delay, 4*time.Second))
	}
	key, err := security.UnlockKey(c.DataDir, password)
	c.keyMu.Lock()
	if errors.Is(err, security.ErrWrongPassword) {
		c.failedTry++
		c.keyMu.Unlock()
		return wrongPassword()
	}
	if err != nil {
		c.keyMu.Unlock()
		return err
	}
	c.failedTry = 0
	err = c.installKey(key)
	started := c.started
	c.keyMu.Unlock()
	if err != nil {
		return err
	}
	c.emitLock()
	if started {
		c.afterUnlock()
	}
	return nil
}

// Lock forgets the data key until the master password is entered again.
// Open terminals keep running; new connections, saved secrets, background
// monitoring and sync wait for Unlock.
func (c *Core) Lock() error {
	c.keyMu.Lock()
	if !c.protected {
		c.keyMu.Unlock()
		return sshconn.Coded(CodeNoPassword, "set a master password first (Settings → Security)", nil)
	}
	c.store.SetVault(nil)
	for i := range c.dek {
		c.dek[i] = 0
	}
	c.dek = nil
	c.keyMu.Unlock()
	c.sync.Lock()
	c.applyMonitoring()
	c.emitLock()
	return nil
}

// SetMasterPassword sets or changes the master password. current is
// ignored when none is set yet. With sync on, the repository's copy of the
// sync key is re-wrapped first, so this needs a connection to GitHub.
func (c *Core) SetMasterPassword(current, next string) (models.LockStatus, error) {
	if err := checkStrength(next); err != nil {
		return c.LockStatus(), err
	}
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	if c.store.Locked() {
		return c.lockStatusLocked(), storageLocked()
	}
	if c.protected {
		if _, err := security.UnlockKey(c.DataDir, current); err != nil {
			if errors.Is(err, security.ErrWrongPassword) {
				return c.lockStatusLocked(), wrongPassword()
			}
			return c.lockStatusLocked(), err
		}
	}
	if c.sync.Connected() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := c.sync.ChangePassword(ctx, next); err != nil {
			return c.lockStatusLocked(), fmt.Errorf("couldn't update the synced vault, so the password was not changed: %w", err)
		}
	}
	if err := security.ProtectKey(c.DataDir, c.dek, next); err != nil {
		return c.lockStatusLocked(), err
	}
	c.protected = true
	st := c.lockStatusLocked()
	go c.emitLock()
	return st, nil
}

// RemoveMasterPassword stores the data key without a password again.
func (c *Core) RemoveMasterPassword(current string) (models.LockStatus, error) {
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	if !c.protected {
		return c.lockStatusLocked(), nil
	}
	if c.sync.Connected() {
		return c.lockStatusLocked(), errors.New("sync needs a master password; disconnect sync first")
	}
	if c.store.Locked() {
		return c.lockStatusLocked(), storageLocked()
	}
	if _, err := security.UnlockKey(c.DataDir, current); err != nil {
		if errors.Is(err, security.ErrWrongPassword) {
			return c.lockStatusLocked(), wrongPassword()
		}
		return c.lockStatusLocked(), err
	}
	if err := security.UnprotectKey(c.DataDir, c.dek); err != nil {
		return c.lockStatusLocked(), err
	}
	c.protected = false
	st := c.lockStatusLocked()
	go c.emitLock()
	return st, nil
}

// ResetVault erases all local data and the master password (for a
// forgotten password). Data synced to GitHub is left untouched.
func (c *Core) ResetVault() (models.Settings, error) {
	if c.sync.Connected() {
		if err := c.sync.Disconnect(); err != nil {
			return c.Settings(), err
		}
	}
	c.forwards.StopAll()
	c.terms.CloseAll()
	c.files.CloseAll()
	c.keyMu.Lock()
	if err := c.store.ClearAll(context.Background()); err != nil {
		c.keyMu.Unlock()
		return c.Settings(), err
	}
	if err := security.ResetKeys(c.DataDir); err != nil {
		c.keyMu.Unlock()
		return c.Settings(), err
	}
	ks, err := security.LoadKeyState(c.DataDir)
	if err == nil {
		err = c.installKey(ks.Key)
	}
	c.protected, c.failedTry = false, 0
	started := c.started
	c.keyMu.Unlock()
	if err != nil {
		return c.Settings(), err
	}
	def := models.DefaultSettings()
	c.mu.Lock()
	c.settings = def
	c.mu.Unlock()
	c.applyMonitoring()
	c.emitLock()
	if started {
		c.afterUnlock()
	}
	return def, nil
}

func storageLocked() error {
	return sshconn.Coded(CodeLocked, "Flux is locked; unlock it first", nil)
}

// ---------------------------------------------------------------- sync

// SyncStatus returns sync state for Settings.
func (c *Core) SyncStatus() vaultsync.Status { return c.sync.Status() }

// SyncSetApp stores the GitHub App used for sign-in.
func (c *Core) SyncSetApp(clientID, slug string) (vaultsync.Status, error) {
	err := c.sync.SetApp(clientID, slug)
	return c.sync.Status(), err
}

// SyncStartLogin begins "Sign in with GitHub".
func (c *Core) SyncStartLogin() (vaultsync.DeviceCode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return c.sync.StartLogin(ctx)
}

// SyncFinishLogin waits until the user approves sign-in on github.com.
func (c *Core) SyncFinishLogin() (string, error) {
	return c.sync.FinishLogin(context.Background())
}

// SyncCancelLogin stops waiting for sign-in.
func (c *Core) SyncCancelLogin() { c.sync.CancelLogin() }

// SyncRepos is the repository picker's data.
type SyncRepos struct {
	Repos []vaultsync.Repo `json:"repos"`
	// Installed is false when the Flux GitHub app isn't installed anywhere.
	Installed bool `json:"installed"`
}

// SyncRepos lists repositories the Flux app can access.
func (c *Core) SyncRepos() (SyncRepos, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repos, installed, err := c.sync.Repos(ctx)
	return SyncRepos{Repos: repos, Installed: installed}, err
}

// SyncInspect reports whether a repository already holds a vault.
func (c *Core) SyncInspect(repo string) (vaultsync.RepoInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return c.sync.Inspect(ctx, repo)
}

// SyncConnect turns on sync with repo. For a new vault, password must be
// the master password (or becomes it when none is set). To join an
// existing vault, password is the one used on the other devices and
// becomes this device's master password too.
func (c *Core) SyncConnect(repo, password string) (vaultsync.Status, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	if c.store.Locked() {
		return c.sync.Status(), storageLocked()
	}
	info, err := c.sync.Inspect(ctx, repo)
	if err != nil {
		return c.sync.Status(), err
	}
	var key []byte
	if info.HasVault {
		key, err = c.sync.JoinVault(ctx, repo, password)
		if errors.Is(err, security.ErrWrongPassword) {
			return c.sync.Status(), sshconn.Coded(CodeWrongPassword, "that isn't the master password for this vault", nil)
		}
		if err != nil {
			return c.sync.Status(), err
		}
	} else {
		if c.protected {
			if _, err := security.UnlockKey(c.DataDir, password); err != nil {
				if errors.Is(err, security.ErrWrongPassword) {
					return c.sync.Status(), wrongPassword()
				}
				return c.sync.Status(), err
			}
		} else if err := checkStrength(password); err != nil {
			return c.sync.Status(), err
		}
		if key, err = c.sync.CreateVault(ctx, repo, password); err != nil {
			return c.sync.Status(), err
		}
	}
	// One master password everywhere: this device adopts the vault's.
	if err := security.ProtectKey(c.DataDir, c.dek, password); err != nil {
		return c.sync.Status(), err
	}
	c.protected = true
	go c.emitLock()
	if err := c.sync.Activate(ctx, repo, key); err != nil {
		// Connected, but the first pass failed; the status shows why.
		return c.sync.Status(), nil
	}
	return c.sync.Status(), nil
}

// SyncDisconnect stops syncing on this device.
func (c *Core) SyncDisconnect() (vaultsync.Status, error) {
	err := c.sync.Disconnect()
	go c.emitLock()
	return c.sync.Status(), err
}

// SyncNow runs a pass immediately.
func (c *Core) SyncNow() (vaultsync.Status, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err := c.sync.SyncNow(ctx)
	return c.sync.Status(), err
}

// SyncWake asks for a pass soon (window focus).
func (c *Core) SyncWake() { c.sync.Wake() }
