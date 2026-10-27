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
	// PersistKey names the server-side session to create or resume when
	// persistent sessions are on (terminals only).
	PersistKey string `json:"persistKey"`
	// Restore replays a resumed session's scrollback into an empty terminal.
	Restore bool `json:"restore"`
}

// SessionInfo describes an open terminal or SFTP session.
type SessionInfo struct {
	ID        string `json:"id"`
	HostID    string `json:"hostId"`
	Kind      string `json:"kind"` // terminal | local | sftp
	Title     string `json:"title"`
	Remote    string `json:"remote"` // user@host:port
	StartedAt int64  `json:"startedAt"`
	// Persistent is true when the shell runs in a tmux session on the server
	// that survives disconnects; Resumed when that session already existed.
	Persistent bool `json:"persistent"`
	Resumed    bool `json:"resumed"`
	// PersistNote says why a requested persistent session fell back to a
	// plain shell: "tmux_missing" or "tmux_old".
	PersistNote string `json:"persistNote,omitempty"`
	// AgentForwarding is true when the server accepted agent forwarding.
	AgentForwarding bool `json:"agentForwarding"`
	// Detached lists other Remvyn sessions on the server that no client is
	// attached to, e.g. from before Remvyn was restarted.
	Detached []ServerSession `json:"detached"`
}

// ServerSession is a persistent terminal session running on a server.
type ServerSession struct {
	Key        string `json:"key"`
	Attached   bool   `json:"attached"`
	LastActive int64  `json:"lastActive"` // unix ms
	History    int    `json:"history"`    // scrollback lines
	Command    string `json:"command"`    // what's running in it
	Path       string `json:"path"`       // its working directory
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

// FileContent is a text file opened in the built-in editor. ModTime is the
// version that was read; pass it back on save to detect concurrent edits.
type FileContent struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
}

// Editor is an external editor installed on this computer.
type Editor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
	// PersistentSessions runs SSH shells inside tmux on the server (when
	// it's installed) so they survive disconnects.
	PersistentSessions bool `json:"persistentSessions"`
	// CommandSuggestions shows past commands as ghost text in terminals.
	CommandSuggestions bool `json:"commandSuggestions"`

	// MonitorHostIDs are monitored in the background while Remvyn runs.
	MonitorHostIDs []string `json:"monitorHostIds"`
	// MonitorIntervalSec is the background sampling interval.
	MonitorIntervalSec int `json:"monitorIntervalSec"`
	// MetricsRetentionDays is how long history and uptime results are kept.
	MetricsRetentionDays int `json:"metricsRetentionDays"`
	// AlertNotifications shows desktop notifications for alerts.
	AlertNotifications bool `json:"alertNotifications"`

	// AgentEnabled lets AI agents (Claude Code, over MCP) use hosts opened
	// to them in AgentHosts. Per device: never synced.
	AgentEnabled bool `json:"agentEnabled"`
	// AgentHosts maps host ID → AgentAsk or AgentAuto. Missing = no access.
	AgentHosts map[string]string `json:"agentHosts"`

	// ForwardKeyHosts are hosts whose SSH sessions may use the keychain's
	// keys through agent forwarding, with approval for each use. Per
	// device: never synced.
	ForwardKeyHosts []string `json:"forwardKeyHosts"`
	// ForwardKeysToAI also forwards keys on AI agents' connections to those
	// hosts (still asking each time).
	ForwardKeysToAI bool `json:"forwardKeysToAI"`
}

// Agent access modes for a host.
const (
	AgentAsk  = "ask"  // every action waits for approval in Remvyn
	AgentAuto = "auto" // actions run without asking
)

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
		PersistentSessions: true,
		CommandSuggestions: true,

		MonitorHostIDs:       []string{},
		MonitorIntervalSec:   30,
		MetricsRetentionDays: 30,
		AlertNotifications:   true,
		AgentHosts:           map[string]string{},
		ForwardKeyHosts:      []string{},
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

// KeyUseRequest asks the user to let a server sign with a keychain key over
// agent forwarding.
type KeyUseRequest struct {
	ID          string `json:"id"`
	HostID      string `json:"hostId"`
	HostLabel   string `json:"hostLabel"`
	Remote      string `json:"remote"`
	KeyName     string `json:"keyName"`
	Fingerprint string `json:"fingerprint"`
	// Source is "terminal", or the AI agent's name for its connections.
	Source    string `json:"source"`
	AI        bool   `json:"ai"`
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
}

// AgentApproval asks the user to allow an AI agent's action.
type AgentApproval struct {
	ID        string `json:"id"`
	Client    string `json:"client"` // e.g. "Claude Code"
	HostID    string `json:"hostId"`
	HostLabel string `json:"hostLabel"`
	Remote    string `json:"remote"` // user@host:port
	Tool      string `json:"tool"`
	Title     string `json:"title"`  // "run a command"
	Detail    string `json:"detail"` // the command or path
	// Preview is extra context, e.g. the start of a file being written.
	Preview   string `json:"preview,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
}

// Approval decisions.
const (
	AgentDeny    = "deny"
	AgentOnce    = "once"
	AgentSession = "session" // allow this host for AgentGrantMinutes
)

// AgentGrantMinutes is how long "allow for now" lasts.
const AgentGrantMinutes = 30

// AgentLogEntry records one agent action.
type AgentLogEntry struct {
	ID        string `json:"id"`
	At        int64  `json:"at"`
	Client    string `json:"client"`
	HostID    string `json:"hostId"`
	HostLabel string `json:"hostLabel"`
	Tool      string `json:"tool"`
	Summary   string `json:"summary"` // command or path
	// Status is ok, failed (ran but failed), error, denied or blocked.
	Status string `json:"status"`
	// Approval is auto, once, session or "" (not approved).
	Approval   string `json:"approval"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	DurationMs int64  `json:"durationMs"`
	Output     string `json:"output"` // trimmed result shown to the agent
}

// AgentGrant is a temporary "allow without asking" for a host.
type AgentGrant struct {
	HostID string `json:"hostId"`
	Until  int64  `json:"until"`
}

// AgentStatus describes the agent bridge.
type AgentStatus struct {
	Enabled     bool   `json:"enabled"`
	Listening   bool   `json:"listening"`
	ListenError string `json:"listenError,omitempty"`
	SocketPath  string `json:"socketPath"`
	// Command is what an MCP client runs to reach Remvyn.
	Command []string     `json:"command"`
	Running int          `json:"running"`
	Grants  []AgentGrant `json:"grants"`
}

// Password vault item kinds.
const (
	ItemLogin = "login"
	ItemToken = "token"
	ItemNote  = "note"
)

// VaultItem is a saved login, token or note, without its secrets. Every
// field, including the name, is stored encrypted.
type VaultItem struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Username    string `json:"username"`
	URL         string `json:"url"`
	Favorite    bool   `json:"favorite"`
	HasPassword bool   `json:"hasPassword"`
	HasTOTP     bool   `json:"hasTotp"`
	HasNotes    bool   `json:"hasNotes"`
	FieldCount  int    `json:"fieldCount"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// VaultField is an extra named value on an item, such as a client ID or a
// recovery code. Hidden fields are masked in the UI.
type VaultField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Hidden bool   `json:"hidden"`
}

// VaultSecret holds an item's secret parts. It is only decrypted on request.
type VaultSecret struct {
	// Password is the password for a login, or the token itself.
	Password string `json:"password"`
	// TOTP is a two-factor secret: base32, or an otpauth:// URI.
	TOTP   string       `json:"totp"`
	Notes  string       `json:"notes"`
	Fields []VaultField `json:"fields"`
}

// VaultItemInput creates or updates an item. A nil Secret keeps the saved one.
type VaultItemInput struct {
	Item   VaultItem    `json:"item"`
	Secret *VaultSecret `json:"secret"`
}

// TOTPCode is a current two-factor code.
type TOTPCode struct {
	Code string `json:"code"`
	// ExpiresAt is when the code stops being valid (Unix ms).
	ExpiresAt int64 `json:"expiresAt"`
	Period    int   `json:"period"`
}
