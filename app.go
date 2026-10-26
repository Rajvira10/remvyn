package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync/atomic"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"remvyn/backend/core"
	"remvyn/backend/files"
	"remvyn/backend/models"
	"remvyn/backend/sshconn"
	"remvyn/backend/vaultsync"
)

// Version is the application version.
const Version = "2.1.2"

// The public GitHub App used for "Sign in with GitHub" sync. Override at
// build time:
//
//	wails build -ldflags "-X main.githubClientID=Iv23... -X main.githubAppSlug=my-app"
//
// or at run time with REMVYN_GITHUB_CLIENT_ID / REMVYN_GITHUB_APP_SLUG. Users
// can still enter their own app in Settings → Sync.
var (
	githubClientID = "Iv23liVRmducYJnHWSnT"
	githubAppSlug  = "remvyn-sync-rajvira10"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// App is the Wails-bound facade over core.Core. Every exported method is
// callable from the frontend.
type App struct {
	ctx     context.Context
	core    *core.Core
	initErr string

	notifyReady atomic.Bool
}

// AppInfo describes the running application.
type AppInfo struct {
	Version  string `json:"version"`
	DataDir  string `json:"dataDir"`
	Platform string `json:"platform"`
	HomeDir  string `json:"homeDir"`
	HasAgent bool   `json:"hasAgent"`
	InitErr  string `json:"initError,omitempty"`
}

// BridgeError is the structured error shape returned to the frontend.
type BridgeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// formatError converts backend errors into a JSON {code, message} string.
// The Wails JS runtime wraps errors with new Error(value), which would turn
// an object into "[object Object]", so the structure travels as text and is
// parsed by the frontend (see errorCode/errorMessage in lib/api.ts).
func formatError(err error) any {
	b, jerr := json.Marshal(BridgeError{Code: sshconn.CodeOf(err), Message: err.Error()})
	if jerr != nil {
		return err.Error()
	}
	return string(b)
}

// NewApp creates the application.
func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	c, err := core.Open(ctx, core.Options{
		Emit:   func(event string, data any) { wruntime.EventsEmit(ctx, event, data) },
		Notify: a.notify,
		Sync: vaultsync.Options{
			// REMVYN_GITHUB_WEB/API point sync at GitHub Enterprise (or a test server).
			Endpoints: vaultsync.Endpoints{
				Web: envOr("REMVYN_GITHUB_WEB", vaultsync.DefaultEndpoints.Web),
				API: envOr("REMVYN_GITHUB_API", vaultsync.DefaultEndpoints.API),
			},
			ClientID: envOr("REMVYN_GITHUB_CLIENT_ID", githubClientID),
			AppSlug:  envOr("REMVYN_GITHUB_APP_SLUG", githubAppSlug),
		},
		AgentCommand: agentCommand(),
	})
	if err != nil {
		a.initErr = err.Error()
		wruntime.LogErrorf(ctx, "remvyn: initialisation failed: %v", err)
		return
	}
	a.core = c
}

func (a *App) domReady(context.Context) {
	if a.core != nil {
		a.core.Start()
	}
	// Ask for notification permission once, in the background: on macOS
	// this can wait on the user (or never answer for unsigned builds).
	go func() {
		if wruntime.InitializeNotifications(a.ctx) != nil {
			return
		}
		ok, _ := wruntime.CheckNotificationAuthorization(a.ctx)
		if !ok {
			ok, _ = wruntime.RequestNotificationAuthorization(a.ctx)
		}
		a.notifyReady.Store(ok)
	}()
}

// notify shows a desktop notification without blocking the caller. Native
// notifications need permission (and a signed bundle on macOS); otherwise
// it falls back to osascript.
func (a *App) notify(title, body string) {
	go func() {
		if a.notifyReady.Load() {
			err := wruntime.SendNotification(a.ctx, wruntime.NotificationOptions{ID: fmt.Sprintf("remvyn-%d", time.Now().UnixNano()), Title: title, Body: body})
			if err == nil {
				return
			}
		}
		if runtime.GOOS == "darwin" {
			// Arguments are passed via argv, never interpolated into the script.
			_ = exec.Command("osascript", "-e", "on run argv", "-e", "display notification (item 2 of argv) with title (item 1 of argv)",
				"-e", "end run", title, body).Run()
		}
	}()
}

func (a *App) shutdown(context.Context) {
	if a.core != nil {
		a.core.Close()
	}
}

var errNotReady = errors.New("Remvyn failed to initialise; see Settings for details")

func (a *App) ready() error {
	if a.core == nil {
		return errNotReady
	}
	return nil
}

// ---------------------------------------------------------------- app

// GetAppInfo returns version and environment details.
func (a *App) GetAppInfo() AppInfo {
	info := AppInfo{Version: Version, Platform: runtime.GOOS + "/" + runtime.GOARCH, InitErr: a.initErr,
		HomeDir: files.LocalHome(), HasAgent: os.Getenv("SSH_AUTH_SOCK") != ""}
	if a.core != nil {
		info.DataDir = a.core.DataDir
	}
	return info
}

// GetSettings returns settings.
func (a *App) GetSettings() models.Settings {
	if a.core == nil {
		return models.DefaultSettings()
	}
	return a.core.Settings()
}

// UpdateSettings saves settings.
func (a *App) UpdateSettings(s models.Settings) (models.Settings, error) {
	if err := a.ready(); err != nil {
		return s, err
	}
	return a.core.UpdateSettings(s)
}

// ClearAppData wipes all data and closes sessions.
func (a *App) ClearAppData() (models.Settings, error) {
	if err := a.ready(); err != nil {
		return models.DefaultSettings(), err
	}
	return a.core.ClearAppData()
}

// RevealDataDir opens the data folder in the file manager.
func (a *App) RevealDataDir() {
	if a.core != nil {
		openPath(a.core.DataDir)
	}
}

func openPath(p string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", p)
	case "windows":
		cmd = exec.Command("explorer", p)
	default:
		cmd = exec.Command("xdg-open", p)
	}
	_ = cmd.Start()
}

// ---------------------------------------------------------------- hosts & groups

// ListHosts returns all hosts.
func (a *App) ListHosts() ([]models.Host, error) {
	if err := a.ready(); err != nil {
		return []models.Host{}, err
	}
	return a.core.Hosts()
}

// SaveHost creates or updates a host.
func (a *App) SaveHost(in models.HostInput) (models.Host, error) {
	if err := a.ready(); err != nil {
		return in.Host, err
	}
	return a.core.SaveHost(in)
}

// RevealHostPassword returns a host's saved password so it can be shown in the editor.
func (a *App) RevealHostPassword(id string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.core.HostPassword(id)
}

// DeleteHost removes a host.
func (a *App) DeleteHost(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.DeleteHost(id)
}

// ListGroups returns all groups.
func (a *App) ListGroups() ([]models.Group, error) {
	if err := a.ready(); err != nil {
		return []models.Group{}, err
	}
	return a.core.Groups()
}

// SaveGroup creates or renames a group.
func (a *App) SaveGroup(g models.Group) (models.Group, error) {
	if err := a.ready(); err != nil {
		return g, err
	}
	return a.core.SaveGroup(g)
}

// DeleteGroup removes a group; hosts are kept.
func (a *App) DeleteGroup(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.DeleteGroup(id)
}

// ImportSSHConfig imports hosts from ~/.ssh/config.
func (a *App) ImportSSHConfig() (models.SSHConfigImport, error) {
	if err := a.ready(); err != nil {
		return models.SSHConfigImport{Warnings: []string{}}, err
	}
	return a.core.ImportSSHConfig("")
}

// ImportSSHConfigFile lets the user pick an OpenSSH config file to import.
// The result's Path is empty if they cancel.
func (a *App) ImportSSHConfigFile() (models.SSHConfigImport, error) {
	empty := models.SSHConfigImport{Warnings: []string{}}
	if err := a.ready(); err != nil {
		return empty, err
	}
	home, _ := os.UserHomeDir()
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:            "Choose an SSH config file",
		DefaultDirectory: home,
		ShowHiddenFiles:  true,
	})
	if err != nil || path == "" {
		return empty, err
	}
	return a.core.ImportSSHConfig(path)
}

// ---------------------------------------------------------------- connections

// OpenTerminal connects to a host and starts a shell. Subscribe to
// "term:data:<id>" and "term:exit:<id>" before calling.
func (a *App) OpenTerminal(id, hostID string, opts models.ConnectOptions) (models.SessionInfo, error) {
	if err := a.ready(); err != nil {
		return models.SessionInfo{}, err
	}
	return a.core.OpenTerminal(id, hostID, opts)
}

// WriteTerminal sends keyboard input.
func (a *App) WriteTerminal(id, data string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.WriteTerminal(id, data)
}

// ResizeTerminal updates the terminal size.
func (a *App) ResizeTerminal(id string, cols, rows int) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.ResizeTerminal(id, cols, rows)
}

// CloseTerminal ends a terminal session.
func (a *App) CloseTerminal(id string) {
	if a.core != nil {
		a.core.CloseTerminal(id)
	}
}

// CancelConnect aborts an in-flight connection attempt.
func (a *App) CancelConnect(id string) {
	if a.core != nil {
		a.core.CancelConnect(id)
	}
}

// ResolveHostKey answers a host-key prompt.
func (a *App) ResolveHostKey(promptID string, accept bool) bool {
	if a.core == nil {
		return false
	}
	return a.core.ResolveHostKey(promptID, accept)
}

// ---------------------------------------------------------------- sftp

// OpenSFTP connects and starts an SFTP session.
func (a *App) OpenSFTP(id, hostID string, opts models.ConnectOptions) (models.SessionInfo, error) {
	if err := a.ready(); err != nil {
		return models.SessionInfo{}, err
	}
	return a.core.OpenSFTP(id, hostID, opts)
}

// CloseSFTP ends an SFTP session.
func (a *App) CloseSFTP(id string) {
	if a.core != nil {
		a.core.Files().Close(id)
	}
}

// RemoteHome returns the remote working directory.
func (a *App) RemoteHome(id string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.core.Files().Home(id)
}

// ListRemote lists a remote directory.
func (a *App) ListRemote(id, dir string) (models.DirListing, error) {
	if err := a.ready(); err != nil {
		return models.DirListing{}, err
	}
	return a.core.Files().List(id, dir)
}

// MkdirRemote creates a remote directory.
func (a *App) MkdirRemote(id, dir, name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.Files().Mkdir(id, dir, name)
}

// RenameRemote renames a remote entry.
func (a *App) RenameRemote(id, from, newName string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.Files().Rename(id, from, newName)
}

// RemoveRemote deletes remote entries (recursively).
func (a *App) RemoveRemote(id string, paths []string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.Files().Remove(id, paths)
}

// ListLocal lists a local directory.
func (a *App) ListLocal(dir string) (models.DirListing, error) {
	return files.ListLocal(dir)
}

// MkdirLocal creates a local directory.
func (a *App) MkdirLocal(dir, name string) error { return files.MkdirLocal(dir, name) }

// RenameLocal renames a local entry.
func (a *App) RenameLocal(from, newName string) error { return files.RenameLocal(from, newName) }

// RemoveLocal deletes local entries (recursively).
func (a *App) RemoveLocal(paths []string) error { return files.RemoveLocal(paths) }

// Upload copies local paths into a remote directory.
func (a *App) Upload(id string, localPaths []string, remoteDir string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.core.Files().Upload(id, localPaths, remoteDir)
}

// Download copies remote paths into a local directory.
func (a *App) Download(id string, remotePaths []string, localDir string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.core.Files().Download(id, remotePaths, localDir)
}

// ListTransfers returns transfers.
func (a *App) ListTransfers() []models.Transfer {
	if a.core == nil {
		return []models.Transfer{}
	}
	return a.core.Files().Transfers()
}

// CancelTransfer aborts a transfer.
func (a *App) CancelTransfer(id string) bool {
	if a.core == nil {
		return false
	}
	return a.core.Files().CancelTransfer(id)
}

// ClearTransfers removes finished transfers from the list.
func (a *App) ClearTransfers() {
	if a.core != nil {
		a.core.Files().ClearTransfers()
	}
}

// ---------------------------------------------------------------- keychain

// ListKeys returns keychain entries.
func (a *App) ListKeys() ([]models.Key, error) {
	if err := a.ready(); err != nil {
		return []models.Key{}, err
	}
	return a.core.Keys()
}

// GenerateKey creates a key pair.
func (a *App) GenerateKey(in models.KeyGenerateInput) (models.Key, error) {
	if err := a.ready(); err != nil {
		return models.Key{}, err
	}
	return a.core.GenerateKey(in)
}

// ImportKey imports a pasted private key.
func (a *App) ImportKey(in models.KeyImportInput) (models.Key, error) {
	if err := a.ready(); err != nil {
		return models.Key{}, err
	}
	return a.core.ImportKey(in)
}

// ReadKeyFile asks for a private key file and returns its contents for import.
func (a *App) ReadKeyFile() (string, error) {
	home, _ := os.UserHomeDir()
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:                "Choose a private key",
		DefaultDirectory:     home + string(os.PathSeparator) + ".ssh",
		ShowHiddenFiles:      true,
		CanCreateDirectories: false,
	})
	if err != nil || path == "" {
		return "", err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if fi.Size() > 64*1024 {
		return "", errors.New("that file is too large to be a private key")
	}
	data, err := os.ReadFile(path)
	return string(data), err
}

// RenameKey renames a key.
func (a *App) RenameKey(id, name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.RenameKey(id, name)
}

// DeleteKey removes a key.
func (a *App) DeleteKey(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.DeleteKey(id)
}

// ExportPrivateKey saves the private key to a user-chosen file (0600).
func (a *App) ExportPrivateKey(id string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	pem, err := a.core.ExportPrivateKey(id)
	if err != nil {
		return "", err
	}
	path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{Title: "Export private key", DefaultFilename: "id_remvyn"})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, []byte(pem), 0o600)
}

// InstallPublicKey appends a key to a host's authorized_keys.
func (a *App) InstallPublicKey(hostID, keyID string, opts models.ConnectOptions) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.InstallPublicKey(hostID, keyID, opts)
}

// ---------------------------------------------------------------- snippets

// ListSnippets returns snippets.
func (a *App) ListSnippets() ([]models.Snippet, error) {
	if err := a.ready(); err != nil {
		return []models.Snippet{}, err
	}
	return a.core.Snippets()
}

// SaveSnippet creates or updates a snippet.
func (a *App) SaveSnippet(s models.Snippet) (models.Snippet, error) {
	if err := a.ready(); err != nil {
		return s, err
	}
	return a.core.SaveSnippet(s)
}

// DeleteSnippet removes a snippet.
func (a *App) DeleteSnippet(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.DeleteSnippet(id)
}

// ---------------------------------------------------------------- forwarding

// ListForwards returns forwarding rules.
func (a *App) ListForwards() ([]models.ForwardRule, error) {
	if err := a.ready(); err != nil {
		return []models.ForwardRule{}, err
	}
	return a.core.Forwards()
}

// ForwardStatuses returns live forwarding states.
func (a *App) ForwardStatuses() []models.ForwardStatus {
	if a.core == nil {
		return []models.ForwardStatus{}
	}
	return a.core.ForwardStatuses()
}

// SaveForward creates or updates a rule.
func (a *App) SaveForward(f models.ForwardRule) (models.ForwardRule, error) {
	if err := a.ready(); err != nil {
		return f, err
	}
	return a.core.SaveForward(f)
}

// DeleteForward removes a rule.
func (a *App) DeleteForward(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.DeleteForward(id)
}

// StartForward starts a rule.
func (a *App) StartForward(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.StartForward(id)
}

// StopForward stops a rule.
func (a *App) StopForward(id string) {
	if a.core != nil {
		a.core.StopForward(id)
	}
}

// ---------------------------------------------------------------- known hosts

// ListKnownHosts returns trusted host keys.
func (a *App) ListKnownHosts() ([]models.KnownHost, error) {
	if err := a.ready(); err != nil {
		return []models.KnownHost{}, err
	}
	return a.core.KnownHosts()
}

// ForgetKnownHost removes a trusted host key.
func (a *App) ForgetKnownHost(host string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.ForgetKnownHost(host)
}

// ---------------------------------------------------------------- monitoring

// MonitorWatch starts live monitoring of a host (call MonitorUnwatch when the panel closes).
func (a *App) MonitorWatch(hostID string) (models.MonitorStatus, error) {
	if err := a.ready(); err != nil {
		return models.MonitorStatus{}, err
	}
	return a.core.MonitorWatch(hostID)
}

// MonitorUnwatch stops live monitoring for one viewer.
func (a *App) MonitorUnwatch(hostID string) {
	if a.core != nil {
		a.core.MonitorUnwatch(hostID)
	}
}

// MonitorStatuses lists monitored hosts and their collection state.
func (a *App) MonitorStatuses() []models.MonitorStatus {
	if a.core == nil {
		return []models.MonitorStatus{}
	}
	return a.core.MonitorStatuses()
}

// MonitorLatest returns the newest sample for a host (null if none).
func (a *App) MonitorLatest(hostID string) *models.Metrics {
	if a.core == nil {
		return nil
	}
	return a.core.MonitorLatest(hostID)
}

// ContainerAction starts, stops or restarts a Docker container.
func (a *App) ContainerAction(hostID, container, action string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.ContainerAction(hostID, container, action)
}

// MetricsHistory returns history for the last rangeMinutes.
func (a *App) MetricsHistory(hostID string, rangeMinutes int) ([]models.MetricPoint, error) {
	if err := a.ready(); err != nil {
		return []models.MetricPoint{}, err
	}
	return a.core.MetricsHistory(hostID, rangeMinutes)
}

// ListAlertRules lists alert rules.
func (a *App) ListAlertRules() ([]models.AlertRule, error) {
	if err := a.ready(); err != nil {
		return []models.AlertRule{}, err
	}
	return a.core.AlertRules()
}

// SaveAlertRule creates or updates an alert rule.
func (a *App) SaveAlertRule(r models.AlertRule) (models.AlertRule, error) {
	if err := a.ready(); err != nil {
		return r, err
	}
	return a.core.SaveAlertRule(r)
}

// DeleteAlertRule removes an alert rule.
func (a *App) DeleteAlertRule(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.DeleteAlertRule(id)
}

// ListAlertEvents returns recent alerts.
func (a *App) ListAlertEvents() ([]models.AlertEvent, error) {
	if err := a.ready(); err != nil {
		return []models.AlertEvent{}, err
	}
	return a.core.AlertEvents()
}

// ListUptime returns uptime checks with their status.
func (a *App) ListUptime() ([]models.UptimeSummary, error) {
	if err := a.ready(); err != nil {
		return []models.UptimeSummary{}, err
	}
	return a.core.UptimeSummaries()
}

// SaveUptimeCheck creates or updates an uptime check.
func (a *App) SaveUptimeCheck(c models.UptimeCheck) (models.UptimeCheck, error) {
	if err := a.ready(); err != nil {
		return c, err
	}
	return a.core.SaveUptimeCheck(c)
}

// DeleteUptimeCheck removes an uptime check.
func (a *App) DeleteUptimeCheck(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.DeleteUptimeCheck(id)
}

// TestUptimeCheck probes a check once without saving it.
func (a *App) TestUptimeCheck(c models.UptimeCheck) (models.UptimeResult, error) {
	if err := a.ready(); err != nil {
		return models.UptimeResult{}, err
	}
	return a.core.TestUptimeCheck(c)
}

// SendTestNotification shows a sample alert notification.
func (a *App) SendTestNotification() {
	a.notify("Remvyn alerts are working", "You'll see alerts like this when a rule fires.")
}

// ---------------------------------------------------------------- master password

// GetLockStatus reports whether a master password is set and Remvyn is locked.
func (a *App) GetLockStatus() models.LockStatus {
	if a.core == nil {
		return models.LockStatus{}
	}
	return a.core.LockStatus()
}

// Unlock opens the vault with the master password.
func (a *App) Unlock(password string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.Unlock(password)
}

// Lock locks Remvyn until the master password is entered.
func (a *App) Lock() error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.Lock()
}

// SetMasterPassword sets or changes the master password.
func (a *App) SetMasterPassword(current, next string) (models.LockStatus, error) {
	if err := a.ready(); err != nil {
		return models.LockStatus{}, err
	}
	return a.core.SetMasterPassword(current, next)
}

// RemoveMasterPassword turns the master password off.
func (a *App) RemoveMasterPassword(current string) (models.LockStatus, error) {
	if err := a.ready(); err != nil {
		return models.LockStatus{}, err
	}
	return a.core.RemoveMasterPassword(current)
}

// ResetVault erases all local data after a forgotten master password.
func (a *App) ResetVault() (models.Settings, error) {
	if err := a.ready(); err != nil {
		return models.DefaultSettings(), err
	}
	return a.core.ResetVault()
}

// ---------------------------------------------------------------- sync

// GetSyncStatus returns GitHub sync state.
func (a *App) GetSyncStatus() vaultsync.Status {
	if a.core == nil {
		return vaultsync.Status{State: "off"}
	}
	return a.core.SyncStatus()
}

// SyncSetApp stores a GitHub App client ID and name for sign-in.
func (a *App) SyncSetApp(clientID, slug string) (vaultsync.Status, error) {
	if err := a.ready(); err != nil {
		return vaultsync.Status{}, err
	}
	return a.core.SyncSetApp(clientID, slug)
}

// SyncStartLogin begins "Sign in with GitHub" and returns the code to enter.
func (a *App) SyncStartLogin() (vaultsync.DeviceCode, error) {
	if err := a.ready(); err != nil {
		return vaultsync.DeviceCode{}, err
	}
	return a.core.SyncStartLogin()
}

// SyncFinishLogin waits for the user to approve sign-in on GitHub.
func (a *App) SyncFinishLogin() (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.core.SyncFinishLogin()
}

// SyncCancelLogin stops waiting for sign-in.
func (a *App) SyncCancelLogin() {
	if a.core != nil {
		a.core.SyncCancelLogin()
	}
}

// SyncRepos lists repositories the Remvyn GitHub app can access.
func (a *App) SyncRepos() (core.SyncRepos, error) {
	if err := a.ready(); err != nil {
		return core.SyncRepos{}, err
	}
	return a.core.SyncRepos()
}

// SyncInspect reports whether a repository already holds a vault.
func (a *App) SyncInspect(repo string) (vaultsync.RepoInfo, error) {
	if err := a.ready(); err != nil {
		return vaultsync.RepoInfo{}, err
	}
	return a.core.SyncInspect(repo)
}

// SyncConnect creates or joins the vault in repo and starts syncing.
func (a *App) SyncConnect(repo, password string) (vaultsync.Status, error) {
	if err := a.ready(); err != nil {
		return vaultsync.Status{}, err
	}
	return a.core.SyncConnect(repo, password)
}

// SyncDisconnect stops syncing on this device.
func (a *App) SyncDisconnect() (vaultsync.Status, error) {
	if err := a.ready(); err != nil {
		return vaultsync.Status{}, err
	}
	return a.core.SyncDisconnect()
}

// SyncNow runs a sync pass immediately.
func (a *App) SyncNow() (vaultsync.Status, error) {
	if err := a.ready(); err != nil {
		return vaultsync.Status{}, err
	}
	return a.core.SyncNow()
}

// SyncWake asks for a sync pass soon (window focus).
func (a *App) SyncWake() {
	if a.core != nil {
		a.core.SyncWake()
	}
}

func wruntimeMinimise(a *App) { wruntime.WindowMinimise(a.ctx) }

// showWindow brings Remvyn to the front, including after it was hidden.
func (a *App) showWindow() {
	if a.ctx == nil {
		return
	}
	wruntime.Show(a.ctx)
	wruntime.WindowUnminimise(a.ctx)
	wruntime.WindowShow(a.ctx)
}

// ---------------------------------------------------------------- AI agents

// GetAgentStatus describes AI agent access (MCP bridge).
func (a *App) GetAgentStatus() models.AgentStatus {
	if a.core == nil {
		return models.AgentStatus{Command: agentCommand(), Grants: []models.AgentGrant{}}
	}
	return a.core.AgentStatus()
}

// SetAgentEnabled turns AI agent access on or off.
func (a *App) SetAgentEnabled(on bool) (models.Settings, error) {
	if err := a.ready(); err != nil {
		return models.Settings{}, err
	}
	return a.core.SetAgentEnabled(on)
}

// SetAgentHostMode opens a host to agents ("ask" or "auto") or closes it ("").
func (a *App) SetAgentHostMode(hostID, mode string) (models.Settings, error) {
	if err := a.ready(); err != nil {
		return models.Settings{}, err
	}
	return a.core.SetAgentHostMode(hostID, mode)
}

// RevokeAgentGrants makes hosts in "ask" mode ask again.
func (a *App) RevokeAgentGrants() {
	if a.core != nil {
		a.core.RevokeAgentGrants()
	}
}

// PendingAgentApprovals lists agent actions waiting for approval.
func (a *App) PendingAgentApprovals() []models.AgentApproval {
	if a.core == nil {
		return []models.AgentApproval{}
	}
	return a.core.PendingAgentApprovals()
}

// ResolveAgentApproval answers an approval: deny, once or session.
func (a *App) ResolveAgentApproval(id, decision string) bool {
	if a.core == nil {
		return false
	}
	return a.core.ResolveAgentApproval(id, decision)
}

// ListAgentLog returns recent agent actions, newest first.
func (a *App) ListAgentLog(limit int) ([]models.AgentLogEntry, error) {
	if err := a.ready(); err != nil {
		return []models.AgentLogEntry{}, err
	}
	return a.core.AgentLog(limit)
}

// ClearAgentLog deletes the agent activity log.
func (a *App) ClearAgentLog() error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core.ClearAgentLog()
}
