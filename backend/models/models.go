// Package models defines the types shared between the Go backend and the
// React frontend. Timestamps are Unix milliseconds.
package models

// Auth methods for a host.
const (
	AuthPassword = "password"
	AuthKey      = "key"
	AuthAgent    = "agent"
)

// Host is a saved SSH connection target. Secrets (password) never leave the
// backend; HasPassword reports whether one is stored.
type Host struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	GroupID     string `json:"groupId"`
	Hostname    string `json:"hostname"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	AuthMethod  string `json:"authMethod"`
	KeyID       string `json:"keyId"`
	HasPassword bool   `json:"hasPassword"`
	Color       string `json:"color"`
	// StartupCommand is typed into the shell after connecting.
	StartupCommand  string `json:"startupCommand"`
	CreatedAt       int64  `json:"createdAt"`
	UpdatedAt       int64  `json:"updatedAt"`
	LastConnectedAt int64  `json:"lastConnectedAt"`
}

// HostInput is used to create or update a host. Password is only applied when
// SetPassword is true (empty + SetPassword clears it).
type HostInput struct {
	Host        Host   `json:"host"`
	Password    string `json:"password"`
	SetPassword bool   `json:"setPassword"`
}

// Group organises hosts.
type Group struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
}

// Key is an SSH key pair stored in the keychain. The private key is kept
// encrypted in the backend and never sent to the frontend.
type Key struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Bits          int    `json:"bits"`
	Fingerprint   string `json:"fingerprint"`
	PublicKey     string `json:"publicKey"`
	HasPassphrase bool   `json:"hasPassphrase"`
	CreatedAt     int64  `json:"createdAt"`
}

// KeyGenerateInput describes a new key pair.
type KeyGenerateInput struct {
	Name string `json:"name"`
	Type string `json:"type"` // ed25519 | rsa | ecdsa
	Bits int    `json:"bits"`
	// Passphrase optionally protects the key; it is stored encrypted so the
	// key can be used without prompting.
	Passphrase string `json:"passphrase"`
}

// KeyImportInput imports an existing private key (OpenSSH or PEM).
type KeyImportInput struct {
	Name       string `json:"name"`
	PrivateKey string `json:"privateKey"`
	Passphrase string `json:"passphrase"`
}

// Snippet is a reusable shell command.
type Snippet struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Command     string `json:"command"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// Forward types.
const (
	ForwardLocal   = "local"
	ForwardRemote  = "remote"
	ForwardDynamic = "dynamic"
)

// ForwardRule is a saved port-forwarding rule.
//
//	local:   BindHost:BindPort (this machine)  → DestHost:DestPort (via SSH server)
//	remote:  BindHost:BindPort (on SSH server) → DestHost:DestPort (from this machine)
//	dynamic: SOCKS5 proxy on BindHost:BindPort, connections exit via the SSH server
type ForwardRule struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	HostID    string `json:"hostId"`
	Type      string `json:"type"`
	BindHost  string `json:"bindHost"`
	BindPort  int    `json:"bindPort"`
	DestHost  string `json:"destHost"`
	DestPort  int    `json:"destPort"`
	AutoStart bool   `json:"autoStart"`
	CreatedAt int64  `json:"createdAt"`
}

// Forward runtime states.
const (
	ForwardStopped  = "stopped"
	ForwardStarting = "starting"
	ForwardRunning  = "running"
	ForwardError    = "error"
)

// ForwardStatus is the live state of a forwarding rule.
type ForwardStatus struct {
	RuleID      string `json:"ruleId"`
	State       string `json:"state"`
	Error       string `json:"error,omitempty"`
	ActiveConns int    `json:"activeConns"`
	TotalConns  int64  `json:"totalConns"`
	BytesIn     int64  `json:"bytesIn"`
	BytesOut    int64  `json:"bytesOut"`
	StartedAt   int64  `json:"startedAt"`
}

// KnownHost is a trusted server host key.
type KnownHost struct {
	Host        string `json:"host"` // host:port
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
	AddedAt     int64  `json:"addedAt"`
}

// HostKeyPrompt asks the user to trust an unknown or changed host key.
type HostKeyPrompt struct {
	ID          string `json:"id"`
	Host        string `json:"host"`
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
	// Changed is true when a different key was previously trusted: a possible
	// man-in-the-middle attack.
	Changed        bool   `json:"changed"`
	OldFingerprint string `json:"oldFingerprint,omitempty"`
}

// ConnectOptions carries one-off credentials for a connection attempt.
type ConnectOptions struct {
	Password     string `json:"password"`
	SavePassword bool   `json:"savePassword"`
	Passphrase   string `json:"passphrase"`
	Cols         int    `json:"cols"`
	Rows         int    `json:"rows"`
}

// SessionInfo describes an open terminal or SFTP session.
type SessionInfo struct {
	ID        string `json:"id"`
	HostID    string `json:"hostId"`
	Kind      string `json:"kind"` // terminal | sftp
	Title     string `json:"title"`
	Remote    string `json:"remote"` // user@host:port
	StartedAt int64  `json:"startedAt"`
}

// SessionExit is emitted when a terminal session ends.
type SessionExit struct {
	ID       string `json:"id"`
	ExitCode int    `json:"exitCode"`
	Error    string `json:"error,omitempty"`
}

// FileEntry is a local or remote directory entry.
type FileEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Mode     string `json:"mode"`
	ModTime  int64  `json:"modTime"`
	IsDir    bool   `json:"isDir"`
	IsLink   bool   `json:"isLink"`
	IsHidden bool   `json:"isHidden"`
}

// DirListing is the content of a directory.
type DirListing struct {
	Path    string      `json:"path"`
	Parent  string      `json:"parent"`
	Entries []FileEntry `json:"entries"`
}

// Transfer directions and states.
const (
	TransferUpload   = "upload"
	TransferDownload = "download"

	TransferQueued    = "queued"
	TransferRunning   = "running"
	TransferDone      = "done"
	TransferFailed    = "failed"
	TransferCancelled = "cancelled"
)

// Transfer is an SFTP upload or download.
type Transfer struct {
	ID         string `json:"id"`
	SessionID  string `json:"sessionId"`
	Direction  string `json:"direction"`
	Source     string `json:"source"`
	Dest       string `json:"dest"`
	Name       string `json:"name"`
	State      string `json:"state"`
	Error      string `json:"error,omitempty"`
	BytesTotal int64  `json:"bytesTotal"`
	BytesDone  int64  `json:"bytesDone"`
	FilesTotal int    `json:"filesTotal"`
	FilesDone  int    `json:"filesDone"`
	StartedAt  int64  `json:"startedAt"`
	FinishedAt int64  `json:"finishedAt"`
}

// Settings are persisted preferences.
type Settings struct {
	Theme              string `json:"theme"` // dark | light | system
	TerminalFontSize   int    `json:"terminalFontSize"`
	TerminalFontFamily string `json:"terminalFontFamily"`
	TerminalTheme      string `json:"terminalTheme"`
	// AutoLockMinutes locks Remvyn after this much inactivity (0 = never);
	// only used with a master password.
	AutoLockMinutes  int    `json:"autoLockMinutes"`
	CursorStyle      string `json:"cursorStyle"` // block | bar | underline
	CursorBlink      bool   `json:"cursorBlink"`
	Scrollback       int    `json:"scrollback"`
	CopyOnSelect     bool   `json:"copyOnSelect"`
	KeepaliveSeconds int    `json:"keepaliveSeconds"`
	ConnectTimeout   int    `json:"connectTimeout"`
	ShowHiddenFiles  bool   `json:"showHiddenFiles"`
	ConfirmOnClose   bool   `json:"confirmOnClose"`

	// ProEnabled unlocks Remvyn Pro features (history, alerts, uptime checks,
	// background monitoring and dashboards). It is a local switch until
	// licensing is added.
	ProEnabled bool `json:"proEnabled"`
	// MonitorHostIDs are monitored in the background while Remvyn runs (Pro).
	MonitorHostIDs []string `json:"monitorHostIds"`
	// MonitorIntervalSec is the background sampling interval.
	MonitorIntervalSec int `json:"monitorIntervalSec"`
	// MetricsRetentionDays is how long history and uptime results are kept.
	MetricsRetentionDays int `json:"metricsRetentionDays"`
	// AlertNotifications shows desktop notifications for alerts.
	AlertNotifications bool `json:"alertNotifications"`
}

// DefaultSettings returns defaults.
func DefaultSettings() Settings {
	return Settings{
		Theme:              "dark",
		TerminalFontSize:   13,
		TerminalFontFamily: "'JetBrains Mono', 'SF Mono', Menlo, Consolas, monospace",
		TerminalTheme:      "flux",
		AutoLockMinutes:    15,
		CursorStyle:        "block",
		CursorBlink:        true,
		Scrollback:         10000,
		CopyOnSelect:       false,
		KeepaliveSeconds:   30,
		ConnectTimeout:     15,
		ShowHiddenFiles:    false,
		ConfirmOnClose:     true,

		MonitorHostIDs:       []string{},
		MonitorIntervalSec:   30,
		MetricsRetentionDays: 30,
		AlertNotifications:   true,
	}
}

// SSHConfigImport summarises an import from ~/.ssh/config.
type SSHConfigImport struct {
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Keys     int      `json:"keys"`
	Warnings []string `json:"warnings"`
	// Path is the file that was read; empty when the user cancelled the picker.
	Path string `json:"path"`
}

// LockStatus describes the master password state.
type LockStatus struct {
	Protected     bool `json:"protected"`
	Locked        bool `json:"locked"`
	MinLength     int  `json:"minLength"`
	SyncConnected bool `json:"syncConnected"`
}
