// Package core wires storage, SSH dialing, terminals, SFTP and port
// forwarding together. It has no Wails dependency so it can be tested.
package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"remvyn/backend/alerts"
	"remvyn/backend/files"
	"remvyn/backend/forward"
	"remvyn/backend/keys"
	"remvyn/backend/mcp"
	"remvyn/backend/models"
	"remvyn/backend/monitor"
	"remvyn/backend/security"
	"remvyn/backend/sshconfig"
	"remvyn/backend/sshconn"
	"remvyn/backend/storage"
	"remvyn/backend/terminal"
	"remvyn/backend/uptime"
	"remvyn/backend/vaultsync"
)

// Emitter delivers events to the UI.
type Emitter func(event string, data any)

// Options configure Core.
type Options struct {
	DataDir string
	Emit    Emitter
	// PromptTimeout bounds how long a host-key prompt waits (tests).
	PromptTimeout time.Duration
	// Notify shows a desktop notification (alerts).
	Notify func(title, body string)
	// MonitorLiveInterval overrides the live sampling interval (tests).
	MonitorLiveInterval time.Duration
	// MonitorSlowEvery overrides how often Docker/services refresh (tests).
	MonitorSlowEvery time.Duration
	// Sync configures GitHub sync (endpoints, built-in app, timings).
	Sync vaultsync.Options
	// AgentSocket overrides the AI agent socket path (tests).
	AgentSocket string
	// AgentCommand is how MCP clients launch the bridge, e.g.
	// ["/Applications/Remvyn.app/Contents/MacOS/Remvyn", "mcp"].
	AgentCommand []string
	// AgentApprovalTimeout bounds how long an approval waits (tests).
	AgentApprovalTimeout time.Duration
}

// Core is the application service layer.
type Core struct {
	DataDir string
	DBPath  string

	emit     Emitter
	store    *storage.Store
	broker   *sshconn.PromptBroker
	verifier *sshconn.Verifier
	terms    *terminal.Manager
	files    *files.Manager
	forwards *forward.Manager
	monitor  *monitor.Manager
	uptime   *uptime.Checker
	alerts   *alerts.Engine
	// quiet verifies host keys without prompting (background monitoring).
	quiet     *sshconn.Verifier
	notify    func(title, body string)
	stop      chan struct{}
	closeOnce sync.Once
	sync      *vaultsync.Service
	agent     *agentState

	// keyMu guards the master-password state.
	keyMu     sync.Mutex
	dek       []byte // data key while unlocked
	protected bool   // a master password is set
	started   bool   // Start was called (UI is up)
	failedTry int

	mu       sync.RWMutex
	settings models.Settings
	// saveMu serialises settings writes.
	saveMu sync.Mutex

	connMu  sync.Mutex
	pending map[string]context.CancelFunc
}

// DefaultDataDir returns the per-user data directory, honouring REMVYN_DATA_DIR.
func DefaultDataDir() (string, error) {
	if d := os.Getenv("REMVYN_DATA_DIR"); d != "" {
		return d, nil
	}
	if d := os.Getenv("FLUX_DATA_DIR"); d != "" { // name before the rename
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "Remvyn")
	migrateLegacyDir(filepath.Join(base, "Flux"), dir)
	return dir, nil
}

// The app was called Flux before; its data folder and database are moved
// over once so existing hosts, keys and settings carry across.
const legacyDBName = "flux-ssh.db"

func migrateLegacyDir(old, dir string) {
	if _, err := os.Stat(dir); err == nil {
		return
	}
	if fi, err := os.Stat(old); err == nil && fi.IsDir() {
		_ = os.Rename(old, dir)
	}
}

func migrateLegacyDB(dir, db string) {
	if _, err := os.Stat(db); err == nil {
		return
	}
	old := filepath.Join(dir, legacyDBName)
	if _, err := os.Stat(old); err != nil {
		return
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Rename(old+suffix, db+suffix)
	}
}

// Open initialises storage and services.
func Open(ctx context.Context, opts Options) (*Core, error) {
	if opts.DataDir == "" {
		d, err := DefaultDataDir()
		if err != nil {
			return nil, err
		}
		opts.DataDir = d
	}
	if err := os.MkdirAll(opts.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	ks, err := security.LoadKeyState(opts.DataDir)
	if err != nil {
		return nil, err
	}
	var vault *security.Vault
	if !ks.Protected {
		if vault, err = security.NewVault(ks.Key); err != nil {
			return nil, err
		}
	}
	emit := opts.Emit
	if emit == nil {
		emit = func(string, any) {}
	}
	c := &Core{
		DataDir:   opts.DataDir,
		DBPath:    filepath.Join(opts.DataDir, "remvyn.db"),
		emit:      emit,
		pending:   map[string]context.CancelFunc{},
		notify:    opts.Notify,
		stop:      make(chan struct{}),
		dek:       ks.Key,
		protected: ks.Protected,
	}
	migrateLegacyDB(c.DataDir, c.DBPath)
	if c.store, err = storage.Open(c.DBPath, vault); err != nil {
		return nil, err
	}
	_ = os.Chmod(c.DBPath, 0o600)
	if c.settings, err = c.store.LoadSettings(ctx); err != nil {
		c.store.Close()
		return nil, err
	}
	c.broker = &sshconn.PromptBroker{Emit: emit, Timeout: opts.PromptTimeout}
	c.verifier = &sshconn.Verifier{Store: c.store, Prompt: c.broker.Ask}
	c.quiet = &sshconn.Verifier{Store: c.store}
	c.terms = terminal.New(terminal.Emitter(emit))
	c.files = files.New(files.Emitter(emit))
	c.forwards = forward.New(forward.Emitter(emit), func(ctx context.Context, r models.ForwardRule) (*sshconn.Client, error) {
		client, _, err := c.dial(ctx, r.HostID, models.ConnectOptions{})
		return client, err
	})
	c.initMonitoring(opts.MonitorLiveInterval, opts.MonitorSlowEvery)
	so := opts.Sync
	so.Store, so.Emit = c.store, emit
	if c.sync, err = vaultsync.New(so); err != nil {
		c.store.Close()
		return nil, err
	}
	c.store.OnChange(c.sync.Changed)
	sock := opts.AgentSocket
	if sock == "" {
		sock = mcp.SocketPath(c.DataDir)
	}
	c.agent = newAgentState(sock, opts.AgentCommand, opts.AgentApprovalTimeout)
	return c, nil
}

// StartAutoForwards starts rules marked auto-start (call after the UI is up,
// since host-key prompts may be needed).
func (c *Core) StartAutoForwards() {
	rules, err := c.store.ListForwards(context.Background())
	if err != nil {
		return
	}
	for _, r := range rules {
		if r.AutoStart {
			go func(r models.ForwardRule) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				_ = c.forwards.Start(ctx, r)
			}(r)
		}
	}
}

// Close shuts everything down. It is safe to call more than once.
func (c *Core) Close() {
	c.closeOnce.Do(c.close)
}

func (c *Core) close() {
	c.connMu.Lock()
	for _, cancel := range c.pending {
		cancel()
	}
	c.connMu.Unlock()
	close(c.stop)
	c.closeAgent()
	c.sync.Close()
	c.monitor.StopAll()
	c.uptime.StopAll()
	c.forwards.StopAll()
	c.terms.CloseAll()
	c.files.CloseAll()
	_ = c.store.Close()
}

// Store exposes storage (tests).
func (c *Core) Store() *storage.Store { return c.store }

// ---------------------------------------------------------------- settings

// Settings returns current settings.
func (c *Core) Settings() models.Settings {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.settings
}

// UpdateSettings validates and saves settings.
func (c *Core) UpdateSettings(s models.Settings) (models.Settings, error) {
	switch s.Theme {
	case "dark", "light", "system":
	default:
		s.Theme = "dark"
	}
	switch s.CursorStyle {
	case "block", "bar", "underline":
	default:
		s.CursorStyle = "block"
	}
	if s.TerminalFontSize < 8 || s.TerminalFontSize > 32 {
		return c.Settings(), errors.New("terminal font size must be between 8 and 32")
	}
	if s.Scrollback < 100 || s.Scrollback > 200000 {
		return c.Settings(), errors.New("scrollback must be between 100 and 200,000 lines")
	}
	if s.KeepaliveSeconds < 0 || s.KeepaliveSeconds > 3600 {
		return c.Settings(), errors.New("keepalive interval must be between 0 and 3600 seconds")
	}
	if s.AutoLockMinutes < 0 || s.AutoLockMinutes > 24*60 {
		return c.Settings(), errors.New("auto-lock must be between 0 and 1440 minutes")
	}
	if s.ConnectTimeout < 3 || s.ConnectTimeout > 300 {
		return c.Settings(), errors.New("connect timeout must be between 3 and 300 seconds")
	}
	if strings.TrimSpace(s.TerminalFontFamily) == "" {
		s.TerminalFontFamily = models.DefaultSettings().TerminalFontFamily
	}
	if s.MonitorIntervalSec == 0 {
		s.MonitorIntervalSec = 30
	}
	if s.MonitorIntervalSec < 5 || s.MonitorIntervalSec > 3600 {
		return c.Settings(), errors.New("monitoring interval must be between 5 seconds and 1 hour")
	}
	if s.MetricsRetentionDays == 0 {
		s.MetricsRetentionDays = 30
	}
	if s.MetricsRetentionDays < 1 || s.MetricsRetentionDays > 365 {
		return c.Settings(), errors.New("history retention must be between 1 and 365 days")
	}
	s.MonitorHostIDs = c.existingHostIDs(s.MonitorHostIDs)
	c.saveMu.Lock()
	// Agent access only changes through SetAgentEnabled/SetAgentHostMode, so
	// a stale settings form can't undo the kill switch.
	cur := c.Settings()
	s.AgentEnabled, s.AgentHosts = cur.AgentEnabled, cur.AgentHosts
	err := c.saveSettingsLocked(s)
	c.saveMu.Unlock()
	if err != nil {
		return c.Settings(), err
	}
	c.applyMonitoring()
	return s, nil
}

// ---------------------------------------------------------------- dialing

// dial resolves a host's credentials and connects. On success it stores a
// password when asked and records the connection time.
func (c *Core) dial(ctx context.Context, hostID string, opts models.ConnectOptions) (*sshconn.Client, models.Host, error) {
	return c.dialWith(ctx, hostID, opts, c.verifier.Callback())
}

func (c *Core) dialWith(ctx context.Context, hostID string, opts models.ConnectOptions, verify sshconn.HostKeyCallback) (*sshconn.Client, models.Host, error) {
	h, err := c.store.GetHost(ctx, hostID)
	if err != nil {
		return nil, h, errors.New("host not found")
	}
	t := sshconn.Target{Host: h, Password: opts.Password, Passphrase: opts.Passphrase}
	switch h.AuthMethod {
	case models.AuthPassword:
		if t.Password == "" {
			if t.Password, err = c.store.HostPassword(ctx, h.ID); err != nil {
				return nil, h, err
			}
		}
	case models.AuthKey:
		pem, pass, err := c.store.KeySecret(ctx, h.KeyID)
		if err != nil {
			return nil, h, errors.New("the key assigned to this host is missing from the keychain")
		}
		t.KeyPEM = pem
		if t.Passphrase == "" {
			t.Passphrase = pass
		}
	}
	st := c.Settings()
	client, err := sshconn.Dial(ctx, t, sshconn.Options{
		Timeout:         time.Duration(st.ConnectTimeout) * time.Second,
		Keepalive:       time.Duration(st.KeepaliveSeconds) * time.Second,
		HostKeyCallback: verify,
	})
	if err != nil {
		return nil, h, err
	}
	if opts.SavePassword && opts.Password != "" && h.AuthMethod == models.AuthPassword {
		_ = c.store.SetHostPassword(ctx, h.ID, opts.Password)
	}
	_ = c.store.TouchHost(ctx, h.ID)
	return client, h, nil
}

// trackDial makes an in-flight connection attempt cancellable by session ID.
func (c *Core) trackDial(id string) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	c.connMu.Lock()
	c.pending[id] = cancel
	c.connMu.Unlock()
	return ctx, func() {
		c.connMu.Lock()
		delete(c.pending, id)
		c.connMu.Unlock()
		cancel()
	}
}

// CancelConnect aborts an in-flight connection attempt.
func (c *Core) CancelConnect(id string) {
	c.connMu.Lock()
	cancel, ok := c.pending[id]
	c.connMu.Unlock()
	if ok {
		cancel()
	}
}

// ResolveHostKey answers a host-key prompt.
func (c *Core) ResolveHostKey(promptID string, accept bool) bool {
	return c.broker.Resolve(promptID, accept)
}

func remote(h models.Host) string {
	return sshconn.Username(h) + "@" + sshconn.Addr(h)
}

// OpenTerminal connects and starts a shell. The frontend chooses id and must
// subscribe to terminal output events before calling.
func (c *Core) OpenTerminal(id, hostID string, opts models.ConnectOptions) (models.SessionInfo, error) {
	if !terminal.ValidID(id) {
		return models.SessionInfo{}, errors.New("invalid session id")
	}
	ctx, done := c.trackDial(id)
	defer done()
	client, h, err := c.dial(ctx, hostID, opts)
	if err != nil {
		return models.SessionInfo{}, err
	}
	info := models.SessionInfo{ID: id, HostID: h.ID, Kind: "terminal", Title: h.Label, Remote: remote(h),
		StartedAt: time.Now().UnixMilli()}
	if err := c.terms.Start(id, client, info, opts.Cols, opts.Rows, h.StartupCommand); err != nil {
		return models.SessionInfo{}, err
	}
	return info, nil
}

// OpenLocalTerminal starts a shell on this computer. Like OpenTerminal, the
// frontend chooses id and subscribes to output events first.
func (c *Core) OpenLocalTerminal(id string, opts models.ConnectOptions) (models.SessionInfo, error) {
	if c.store.Locked() {
		return models.SessionInfo{}, storageLocked()
	}
	shell, _ := terminal.LocalShell()
	info := models.SessionInfo{ID: id, Kind: "local", Title: "Local", Remote: filepath.Base(shell),
		StartedAt: time.Now().UnixMilli()}
	if err := c.terms.StartLocal(id, info, opts.Cols, opts.Rows); err != nil {
		return models.SessionInfo{}, err
	}
	return info, nil
}

// WriteTerminal sends input.
func (c *Core) WriteTerminal(id, data string) error { return c.terms.Write(id, data) }

// ResizeTerminal updates the PTY size.
func (c *Core) ResizeTerminal(id string, cols, rows int) error { return c.terms.Resize(id, cols, rows) }

// CloseTerminal ends a session.
func (c *Core) CloseTerminal(id string) { c.terms.Close(id) }

// OpenSFTP connects and starts an SFTP session.
func (c *Core) OpenSFTP(id, hostID string, opts models.ConnectOptions) (models.SessionInfo, error) {
	if !terminal.ValidID(id) {
		return models.SessionInfo{}, errors.New("invalid session id")
	}
	ctx, done := c.trackDial(id)
	defer done()
	client, h, err := c.dial(ctx, hostID, opts)
	if err != nil {
		return models.SessionInfo{}, err
	}
	info := models.SessionInfo{ID: id, HostID: h.ID, Kind: "sftp", Title: h.Label, Remote: remote(h),
		StartedAt: time.Now().UnixMilli()}
	if err := c.files.Open(id, client, info); err != nil {
		return models.SessionInfo{}, err
	}
	return info, nil
}

// Files exposes the SFTP manager.
func (c *Core) Files() *files.Manager { return c.files }

// ---------------------------------------------------------------- hosts

// Hosts lists hosts.
func (c *Core) Hosts() ([]models.Host, error) { return c.store.ListHosts(context.Background()) }

// SaveHost creates or updates a host.
func (c *Core) SaveHost(in models.HostInput) (models.Host, error) {
	return c.store.SaveHost(context.Background(), in)
}

// HostPassword returns a host's saved password ("" if none).
func (c *Core) HostPassword(id string) (string, error) {
	return c.store.HostPassword(context.Background(), id)
}

// DeleteHost removes a host and stops its forwards.
func (c *Core) DeleteHost(id string) error {
	ctx := context.Background()
	rules, _ := c.store.ListForwards(ctx)
	for _, r := range rules {
		if r.HostID == id {
			c.forwards.Forget(r.ID)
		}
	}
	c.forgetMonitoredHost(id)
	if c.Settings().AgentHosts[id] != "" {
		_, _ = c.SetAgentHostMode(id, "")
	}
	return c.store.DeleteHost(ctx, id)
}

// Groups lists groups.
func (c *Core) Groups() ([]models.Group, error) { return c.store.ListGroups(context.Background()) }

// SaveGroup creates or renames a group.
func (c *Core) SaveGroup(g models.Group) (models.Group, error) {
	return c.store.SaveGroup(context.Background(), g)
}

// DeleteGroup removes a group (hosts are kept).
func (c *Core) DeleteGroup(id string) error { return c.store.DeleteGroup(context.Background(), id) }

// ---------------------------------------------------------------- keys

// Keys lists keys.
func (c *Core) Keys() ([]models.Key, error) { return c.store.ListKeys(context.Background()) }

// GenerateKey creates a new key pair.
func (c *Core) GenerateKey(in models.KeyGenerateInput) (models.Key, error) {
	if strings.TrimSpace(in.Name) == "" {
		return models.Key{}, errors.New("key name is required")
	}
	m, err := keys.Generate(in)
	if err != nil {
		return models.Key{}, err
	}
	return c.store.InsertKey(context.Background(), m.Meta, m.PrivatePEM, in.Passphrase)
}

// ImportKey stores an existing private key.
func (c *Core) ImportKey(in models.KeyImportInput) (models.Key, error) {
	m, err := keys.Parse(in.PrivateKey, in.Passphrase)
	if errors.Is(err, keys.ErrPassphraseRequired) {
		return models.Key{}, sshconn.Coded(sshconn.CodePassphraseRequired, err.Error(), err)
	}
	if err != nil {
		return models.Key{}, err
	}
	m.Meta.Name = strings.TrimSpace(in.Name)
	if m.Meta.Name == "" {
		m.Meta.Name = "Imported " + strings.ToUpper(strings.TrimPrefix(m.Meta.Type, "ssh-")) + " key"
	}
	existing, _ := c.Keys()
	for _, k := range existing {
		if k.Fingerprint == m.Meta.Fingerprint {
			return models.Key{}, fmt.Errorf("this key is already in the keychain as %q", k.Name)
		}
	}
	return c.store.InsertKey(context.Background(), m.Meta, m.PrivatePEM, in.Passphrase)
}

// RenameKey renames a key.
func (c *Core) RenameKey(id, name string) error {
	return c.store.RenameKey(context.Background(), id, name)
}

// DeleteKey removes an unused key.
func (c *Core) DeleteKey(id string) error { return c.store.DeleteKey(context.Background(), id) }

// ExportPrivateKey returns the private key PEM (for explicit user export).
func (c *Core) ExportPrivateKey(id string) (string, error) {
	pem, _, err := c.store.KeySecret(context.Background(), id)
	return pem, err
}

// InstallPublicKey appends a key to ~/.ssh/authorized_keys on a host (like
// ssh-copy-id), using the host's own credentials.
func (c *Core) InstallPublicKey(hostID, keyID string, opts models.ConnectOptions) error {
	k, err := c.store.GetKey(context.Background(), keyID)
	if err != nil {
		return errors.New("key not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, err := c.dial(ctx, hostID, opts)
	if err != nil {
		return err
	}
	defer client.Close()
	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	line := keys.AuthorizedKeyLine(k)
	// The key line is passed on stdin, never interpolated into the command.
	sess.Stdin = strings.NewReader(line + "\n")
	cmd := `umask 077; mkdir -p ~/.ssh && touch ~/.ssh/authorized_keys && ` +
		`read -r key; grep -qxF "$key" ~/.ssh/authorized_keys || printf '%s\n' "$key" >> ~/.ssh/authorized_keys`
	if out, err := sess.CombinedOutput(cmd); err != nil {
		return fmt.Errorf("could not install key: %s", strings.TrimSpace(string(out)+" "+err.Error()))
	}
	return nil
}

// ---------------------------------------------------------------- snippets

// Snippets lists snippets.
func (c *Core) Snippets() ([]models.Snippet, error) {
	return c.store.ListSnippets(context.Background())
}

// SaveSnippet creates or updates a snippet.
func (c *Core) SaveSnippet(s models.Snippet) (models.Snippet, error) {
	return c.store.SaveSnippet(context.Background(), s)
}

// DeleteSnippet removes a snippet.
func (c *Core) DeleteSnippet(id string) error { return c.store.DeleteSnippet(context.Background(), id) }

// ---------------------------------------------------------------- forwards

// Forwards lists rules.
func (c *Core) Forwards() ([]models.ForwardRule, error) {
	return c.store.ListForwards(context.Background())
}

// ForwardStatuses returns live states.
func (c *Core) ForwardStatuses() []models.ForwardStatus { return c.forwards.Statuses() }

// SaveForward creates or updates a rule. A running rule must be restarted
// for changes to apply.
func (c *Core) SaveForward(f models.ForwardRule) (models.ForwardRule, error) {
	return c.store.SaveForward(context.Background(), f)
}

// DeleteForward stops and removes a rule.
func (c *Core) DeleteForward(id string) error {
	c.forwards.Forget(id)
	return c.store.DeleteForward(context.Background(), id)
}

// StartForward starts a rule.
func (c *Core) StartForward(id string) error {
	r, err := c.store.GetForward(context.Background(), id)
	if err != nil {
		return errors.New("forwarding rule not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return c.forwards.Start(ctx, r)
}

// StopForward stops a rule.
func (c *Core) StopForward(id string) { c.forwards.Stop(id) }

// ---------------------------------------------------------------- known hosts

// KnownHosts lists trusted host keys.
func (c *Core) KnownHosts() ([]models.KnownHost, error) {
	return c.store.ListKnownHosts(context.Background())
}

// ForgetKnownHost removes a trusted key.
func (c *Core) ForgetKnownHost(host string) error {
	return c.store.ForgetHostKey(context.Background(), host)
}

// ---------------------------------------------------------------- import

// CodeNoSSHConfig means ~/.ssh/config does not exist.
const CodeNoSSHConfig = "no_ssh_config"

// ImportSSHConfig adds hosts from an OpenSSH config file. Unencrypted
// IdentityFile keys are imported into the keychain; other hosts fall back to
// agent authentication.
func (c *Core) ImportSSHConfig(path string) (models.SSHConfigImport, error) {
	res := models.SSHConfigImport{Warnings: []string{}}
	isDefault := path == ""
	if isDefault {
		path = sshconfig.DefaultPath()
	}
	res.Path = path
	f, err := os.Open(path)
	if isDefault && errors.Is(err, os.ErrNotExist) {
		return res, sshconn.Coded(CodeNoSSHConfig, "there is no SSH config file at ~/.ssh/config", err)
	}
	if err != nil {
		return res, fmt.Errorf("could not read %s: %w", path, err)
	}
	defer f.Close()
	home, _ := os.UserHomeDir()
	entries, warnings := sshconfig.Parse(f, home)
	res.Warnings = append(res.Warnings, warnings...)

	ctx := context.Background()
	existing, _ := c.Hosts()
	have := map[string]bool{}
	for _, h := range existing {
		have[strings.ToLower(h.Label)] = true
	}
	keyIDs := map[string]string{}
	allKeys, _ := c.Keys()
	byFP := map[string]string{}
	for _, k := range allKeys {
		byFP[k.Fingerprint] = k.ID
	}
	jumped := 0
	for _, e := range entries {
		if have[strings.ToLower(e.Alias)] {
			res.Skipped++
			continue
		}
		if e.ProxyJump != "" {
			jumped++
		}
		h := models.Host{Label: e.Alias, Hostname: e.HostName, Port: e.Port, Username: e.User, AuthMethod: models.AuthAgent}
		if e.IdentityFile != "" {
			id, ok := keyIDs[e.IdentityFile]
			if !ok {
				id = c.importIdentity(ctx, e.IdentityFile, byFP, &res)
				keyIDs[e.IdentityFile] = id
			}
			if id != "" {
				h.AuthMethod, h.KeyID = models.AuthKey, id
			}
		}
		if _, err := c.store.SaveHost(ctx, models.HostInput{Host: h}); err != nil {
			res.Skipped++
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", e.Alias, err))
			continue
		}
		have[strings.ToLower(e.Alias)] = true
		res.Imported++
	}
	if jumped > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%d host(s) use ProxyJump, which Remvyn does not support yet", jumped))
	}
	return res, nil
}

func (c *Core) importIdentity(ctx context.Context, path string, byFP map[string]string, res *models.SSHConfigImport) string {
	data, err := os.ReadFile(path)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("could not read key %s", path))
		return ""
	}
	m, err := keys.Parse(string(data), "")
	if errors.Is(err, keys.ErrPassphraseRequired) {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s is passphrase-protected; those hosts use the SSH agent (or import the key in Keychain)", filepath.Base(path)))
		return ""
	}
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", filepath.Base(path), err))
		return ""
	}
	if id, ok := byFP[m.Meta.Fingerprint]; ok {
		return id
	}
	m.Meta.Name = filepath.Base(path)
	k, err := c.store.InsertKey(ctx, m.Meta, m.PrivatePEM, "")
	if err != nil {
		return ""
	}
	byFP[k.Fingerprint] = k.ID
	res.Keys++
	return k.ID
}

// ---------------------------------------------------------------- data

// ClearAppData closes all sessions and wipes stored data on this device.
// Sync is disconnected first so the wipe never reaches other devices.
func (c *Core) ClearAppData() (models.Settings, error) {
	if c.sync.Connected() {
		if err := c.sync.Disconnect(); err != nil {
			return c.Settings(), err
		}
	}
	c.forwards.StopAll()
	c.terms.CloseAll()
	c.files.CloseAll()
	if err := c.store.ClearAll(context.Background()); err != nil {
		return c.Settings(), err
	}
	def := models.DefaultSettings()
	c.mu.Lock()
	c.settings = def
	c.mu.Unlock()
	c.agentStop()
	c.applyMonitoring()
	return def, nil
}
