// Package storage persists Flux data in SQLite (pure Go driver, no CGO).
// Passwords, private keys and passphrases are sealed with the vault before
// they are written; they never appear in the database in plaintext.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"flux/backend/ids"
	"flux/backend/models"
	"flux/backend/security"
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("not found")

// Store is the SQLite-backed persistence layer.
type Store struct {
	db    *sql.DB
	vault *security.Vault
}

const schema = `
CREATE TABLE IF NOT EXISTS groups (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS keys (
	id             TEXT PRIMARY KEY,
	name           TEXT NOT NULL,
	type           TEXT NOT NULL,
	bits           INTEGER NOT NULL DEFAULT 0,
	fingerprint    TEXT NOT NULL,
	public_key     TEXT NOT NULL,
	private_sealed BLOB NOT NULL,
	passphrase_sealed BLOB,
	created_at     INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS hosts (
	id              TEXT PRIMARY KEY,
	label           TEXT NOT NULL,
	group_id        TEXT NOT NULL DEFAULT '',
	hostname        TEXT NOT NULL,
	port            INTEGER NOT NULL DEFAULT 22,
	username        TEXT NOT NULL DEFAULT '',
	auth_method     TEXT NOT NULL,
	key_id          TEXT NOT NULL DEFAULT '',
	password_sealed BLOB,
	color           TEXT NOT NULL DEFAULT '',
	startup_command TEXT NOT NULL DEFAULT '',
	created_at      INTEGER NOT NULL,
	updated_at      INTEGER NOT NULL,
	last_connected_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS snippets (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	command     TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS forwards (
	id         TEXT PRIMARY KEY,
	label      TEXT NOT NULL,
	host_id    TEXT NOT NULL,
	type       TEXT NOT NULL,
	bind_host  TEXT NOT NULL,
	bind_port  INTEGER NOT NULL,
	dest_host  TEXT NOT NULL DEFAULT '',
	dest_port  INTEGER NOT NULL DEFAULT 0,
	auto_start INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS known_hosts (
	host        TEXT NOT NULL,
	key_type    TEXT NOT NULL,
	public_key  BLOB NOT NULL,
	fingerprint TEXT NOT NULL,
	added_at    INTEGER NOT NULL,
	PRIMARY KEY (host, key_type)
);
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

// Open opens (creating if needed) the database at path. ":memory:" is
// supported for tests.
func Open(path string, vault *security.Vault) (*Store, error) {
	if vault == nil {
		return nil, errors.New("storage requires a vault")
	}
	dsn := "file:" + url.PathEscape(path) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	if path == ":memory:" {
		dsn = "file::memory:"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(4)
	}
	if _, err := db.Exec(schema + monitorSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialise schema: %w", err)
	}
	return &Store{db: db, vault: vault}, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

func now() int64 { return time.Now().UnixMilli() }

func (s *Store) seal(plain string) ([]byte, error) {
	if plain == "" {
		return nil, nil
	}
	return s.vault.Seal([]byte(plain))
}

func (s *Store) open(sealed []byte) (string, error) {
	if len(sealed) == 0 {
		return "", nil
	}
	b, err := s.vault.Open(sealed)
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return string(b), nil
}

func required(name, v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

// ---------------------------------------------------------------- groups

// ListGroups returns all groups ordered by name.
func (s *Store) ListGroups(ctx context.Context) ([]models.Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, created_at FROM groups ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Group{}
	for rows.Next() {
		var g models.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// SaveGroup creates or renames a group.
func (s *Store) SaveGroup(ctx context.Context, g models.Group) (models.Group, error) {
	g.Name = strings.TrimSpace(g.Name)
	if err := required("group name", g.Name); err != nil {
		return g, err
	}
	if g.ID == "" {
		g.ID, g.CreatedAt = ids.New(), now()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO groups(id, name, created_at) VALUES(?,?,?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name`, g.ID, g.Name, g.CreatedAt)
	return g, err
}

// DeleteGroup removes a group; its hosts become ungrouped.
func (s *Store) DeleteGroup(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE hosts SET group_id = '' WHERE group_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM groups WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------------------------------------------------------------- hosts

const hostCols = `id, label, group_id, hostname, port, username, auth_method, key_id,
	password_sealed IS NOT NULL AND length(password_sealed) > 0, color, startup_command, created_at, updated_at, last_connected_at`

func scanHost(row interface{ Scan(...any) error }) (models.Host, error) {
	var h models.Host
	err := row.Scan(&h.ID, &h.Label, &h.GroupID, &h.Hostname, &h.Port, &h.Username, &h.AuthMethod, &h.KeyID,
		&h.HasPassword, &h.Color, &h.StartupCommand, &h.CreatedAt, &h.UpdatedAt, &h.LastConnectedAt)
	return h, err
}

// ListHosts returns all hosts ordered by label.
func (s *Store) ListHosts(ctx context.Context) ([]models.Host, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+hostCols+` FROM hosts ORDER BY label COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Host{}
	for rows.Next() {
		h, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// GetHost loads one host.
func (s *Store) GetHost(ctx context.Context, id string) (models.Host, error) {
	h, err := scanHost(s.db.QueryRowContext(ctx, `SELECT `+hostCols+` FROM hosts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return h, ErrNotFound
	}
	return h, err
}

// HostPassword returns the decrypted saved password ("" if none).
func (s *Store) HostPassword(ctx context.Context, id string) (string, error) {
	var sealed []byte
	err := s.db.QueryRowContext(ctx, `SELECT password_sealed FROM hosts WHERE id = ?`, id).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return s.open(sealed)
}

// ValidateHost normalises and checks a host.
func ValidateHost(h *models.Host) error {
	h.Label = strings.TrimSpace(h.Label)
	h.Hostname = strings.TrimSpace(h.Hostname)
	h.Username = strings.TrimSpace(h.Username)
	if err := required("address", h.Hostname); err != nil {
		return err
	}
	if strings.ContainsAny(h.Hostname, " /\t") {
		return errors.New("address must be a hostname or IP address")
	}
	if h.Port == 0 {
		h.Port = 22
	}
	if h.Port < 1 || h.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if h.Label == "" {
		h.Label = h.Hostname
	}
	switch h.AuthMethod {
	case models.AuthPassword, models.AuthAgent:
		h.KeyID = ""
	case models.AuthKey:
		if h.KeyID == "" {
			return errors.New("choose a key from the keychain for key authentication")
		}
	default:
		return fmt.Errorf("unknown authentication method %q", h.AuthMethod)
	}
	return nil
}

// SaveHost creates or updates a host.
func (s *Store) SaveHost(ctx context.Context, in models.HostInput) (models.Host, error) {
	h := in.Host
	if err := ValidateHost(&h); err != nil {
		return h, err
	}
	if h.AuthMethod == models.AuthKey {
		if _, err := s.GetKey(ctx, h.KeyID); err != nil {
			return h, errors.New("the selected key no longer exists")
		}
	}
	ts := now()
	isNew := h.ID == ""
	if isNew {
		h.ID, h.CreatedAt = ids.New(), ts
	}
	h.UpdatedAt = ts
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return h, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO hosts(id, label, group_id, hostname, port, username, auth_method, key_id,
		color, startup_command, created_at, updated_at, last_connected_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET label=excluded.label, group_id=excluded.group_id, hostname=excluded.hostname,
		port=excluded.port, username=excluded.username, auth_method=excluded.auth_method, key_id=excluded.key_id,
		color=excluded.color, startup_command=excluded.startup_command, updated_at=excluded.updated_at`,
		h.ID, h.Label, h.GroupID, h.Hostname, h.Port, h.Username, h.AuthMethod, h.KeyID, h.Color, h.StartupCommand,
		h.CreatedAt, h.UpdatedAt, h.LastConnectedAt)
	if err != nil {
		return h, err
	}
	if in.SetPassword {
		sealed, err := s.seal(in.Password)
		if err != nil {
			return h, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE hosts SET password_sealed = ? WHERE id = ?`, sealed, h.ID); err != nil {
			return h, err
		}
	}
	if err := tx.Commit(); err != nil {
		return h, err
	}
	return s.GetHost(ctx, h.ID)
}

// SetHostPassword stores (or clears) a host password.
func (s *Store) SetHostPassword(ctx context.Context, id, password string) error {
	sealed, err := s.seal(password)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE hosts SET password_sealed = ? WHERE id = ?`, sealed, id)
	return err
}

// TouchHost records a successful connection.
func (s *Store) TouchHost(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE hosts SET last_connected_at = ? WHERE id = ?`, now(), id)
	return err
}

// DeleteHost removes a host and its forwarding rules.
func (s *Store) DeleteHost(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM forwards WHERE host_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM hosts WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------------------------------------------------------------- keys

const keyCols = `id, name, type, bits, fingerprint, public_key, passphrase_sealed IS NOT NULL AND length(passphrase_sealed) > 0, created_at`

func scanKey(row interface{ Scan(...any) error }) (models.Key, error) {
	var k models.Key
	err := row.Scan(&k.ID, &k.Name, &k.Type, &k.Bits, &k.Fingerprint, &k.PublicKey, &k.HasPassphrase, &k.CreatedAt)
	return k, err
}

// ListKeys returns all keys.
func (s *Store) ListKeys(ctx context.Context) ([]models.Key, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+keyCols+` FROM keys ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Key{}
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// GetKey loads key metadata.
func (s *Store) GetKey(ctx context.Context, id string) (models.Key, error) {
	k, err := scanKey(s.db.QueryRowContext(ctx, `SELECT `+keyCols+` FROM keys WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return k, ErrNotFound
	}
	return k, err
}

// KeySecret returns the decrypted private key PEM and stored passphrase.
func (s *Store) KeySecret(ctx context.Context, id string) (privatePEM, passphrase string, err error) {
	var priv, pass []byte
	err = s.db.QueryRowContext(ctx, `SELECT private_sealed, passphrase_sealed FROM keys WHERE id = ?`, id).Scan(&priv, &pass)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	if privatePEM, err = s.open(priv); err != nil {
		return "", "", err
	}
	passphrase, err = s.open(pass)
	return privatePEM, passphrase, err
}

// InsertKey stores a key with its sealed private material.
func (s *Store) InsertKey(ctx context.Context, k models.Key, privatePEM, passphrase string) (models.Key, error) {
	k.Name = strings.TrimSpace(k.Name)
	if err := required("key name", k.Name); err != nil {
		return k, err
	}
	priv, err := s.seal(privatePEM)
	if err != nil {
		return k, err
	}
	if len(priv) == 0 {
		return k, errors.New("private key is empty")
	}
	pass, err := s.seal(passphrase)
	if err != nil {
		return k, err
	}
	k.ID, k.CreatedAt = ids.New(), now()
	k.HasPassphrase = passphrase != ""
	_, err = s.db.ExecContext(ctx, `INSERT INTO keys(id, name, type, bits, fingerprint, public_key, private_sealed,
		passphrase_sealed, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		k.ID, k.Name, k.Type, k.Bits, k.Fingerprint, k.PublicKey, priv, pass, k.CreatedAt)
	return k, err
}

// RenameKey renames a key.
func (s *Store) RenameKey(ctx context.Context, id, name string) error {
	name = strings.TrimSpace(name)
	if err := required("key name", name); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE keys SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// KeyUsage returns the labels of hosts using a key.
func (s *Store) KeyUsage(ctx context.Context, id string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT label FROM hosts WHERE key_id = ? ORDER BY label`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// DeleteKey removes a key. It fails while hosts still use it.
func (s *Store) DeleteKey(ctx context.Context, id string) error {
	used, err := s.KeyUsage(ctx, id)
	if err != nil {
		return err
	}
	if len(used) > 0 {
		return fmt.Errorf("key is used by %s; change their authentication first", strings.Join(used, ", "))
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM keys WHERE id = ?`, id)
	return err
}

// ---------------------------------------------------------------- snippets

// ListSnippets returns all snippets.
func (s *Store) ListSnippets(ctx context.Context) ([]models.Snippet, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, command, description, created_at, updated_at
		FROM snippets ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Snippet{}
	for rows.Next() {
		var sn models.Snippet
		if err := rows.Scan(&sn.ID, &sn.Name, &sn.Command, &sn.Description, &sn.CreatedAt, &sn.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sn)
	}
	return out, rows.Err()
}

// SaveSnippet creates or updates a snippet.
func (s *Store) SaveSnippet(ctx context.Context, sn models.Snippet) (models.Snippet, error) {
	sn.Name = strings.TrimSpace(sn.Name)
	if err := required("snippet name", sn.Name); err != nil {
		return sn, err
	}
	if err := required("command", sn.Command); err != nil {
		return sn, err
	}
	ts := now()
	if sn.ID == "" {
		sn.ID, sn.CreatedAt = ids.New(), ts
	}
	sn.UpdatedAt = ts
	_, err := s.db.ExecContext(ctx, `INSERT INTO snippets(id, name, command, description, created_at, updated_at)
		VALUES (?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name, command=excluded.command,
		description=excluded.description, updated_at=excluded.updated_at`,
		sn.ID, sn.Name, sn.Command, sn.Description, sn.CreatedAt, sn.UpdatedAt)
	return sn, err
}

// DeleteSnippet removes a snippet.
func (s *Store) DeleteSnippet(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM snippets WHERE id = ?`, id)
	return err
}

// ---------------------------------------------------------------- forwards

// ListForwards returns all forwarding rules.
func (s *Store) ListForwards(ctx context.Context) ([]models.ForwardRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, label, host_id, type, bind_host, bind_port, dest_host, dest_port,
		auto_start, created_at FROM forwards ORDER BY label COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.ForwardRule{}
	for rows.Next() {
		var f models.ForwardRule
		if err := rows.Scan(&f.ID, &f.Label, &f.HostID, &f.Type, &f.BindHost, &f.BindPort, &f.DestHost, &f.DestPort,
			&f.AutoStart, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// GetForward loads one rule.
func (s *Store) GetForward(ctx context.Context, id string) (models.ForwardRule, error) {
	var f models.ForwardRule
	err := s.db.QueryRowContext(ctx, `SELECT id, label, host_id, type, bind_host, bind_port, dest_host, dest_port,
		auto_start, created_at FROM forwards WHERE id = ?`, id).Scan(&f.ID, &f.Label, &f.HostID, &f.Type, &f.BindHost,
		&f.BindPort, &f.DestHost, &f.DestPort, &f.AutoStart, &f.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return f, ErrNotFound
	}
	return f, err
}

// ValidateForward normalises and checks a rule.
func ValidateForward(f *models.ForwardRule) error {
	f.Label = strings.TrimSpace(f.Label)
	f.BindHost = strings.TrimSpace(f.BindHost)
	f.DestHost = strings.TrimSpace(f.DestHost)
	if f.HostID == "" {
		return errors.New("choose the SSH host to forward through")
	}
	if f.BindHost == "" {
		f.BindHost = "127.0.0.1"
	}
	if f.BindPort < 1 || f.BindPort > 65535 {
		return errors.New("listen port must be between 1 and 65535")
	}
	switch f.Type {
	case models.ForwardLocal, models.ForwardRemote:
		if f.DestHost == "" {
			return errors.New("destination host is required")
		}
		if f.DestPort < 1 || f.DestPort > 65535 {
			return errors.New("destination port must be between 1 and 65535")
		}
	case models.ForwardDynamic:
		f.DestHost, f.DestPort = "", 0
	default:
		return fmt.Errorf("unknown forwarding type %q", f.Type)
	}
	if f.Label == "" {
		switch f.Type {
		case models.ForwardDynamic:
			f.Label = fmt.Sprintf("SOCKS :%d", f.BindPort)
		default:
			f.Label = fmt.Sprintf(":%d → %s:%d", f.BindPort, f.DestHost, f.DestPort)
		}
	}
	return nil
}

// SaveForward creates or updates a rule.
func (s *Store) SaveForward(ctx context.Context, f models.ForwardRule) (models.ForwardRule, error) {
	if err := ValidateForward(&f); err != nil {
		return f, err
	}
	if _, err := s.GetHost(ctx, f.HostID); err != nil {
		return f, errors.New("the selected host no longer exists")
	}
	if f.ID == "" {
		f.ID, f.CreatedAt = ids.New(), now()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO forwards(id, label, host_id, type, bind_host, bind_port, dest_host,
		dest_port, auto_start, created_at) VALUES (?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET
		label=excluded.label, host_id=excluded.host_id, type=excluded.type, bind_host=excluded.bind_host,
		bind_port=excluded.bind_port, dest_host=excluded.dest_host, dest_port=excluded.dest_port,
		auto_start=excluded.auto_start`,
		f.ID, f.Label, f.HostID, f.Type, f.BindHost, f.BindPort, f.DestHost, f.DestPort, f.AutoStart, f.CreatedAt)
	return f, err
}

// DeleteForward removes a rule.
func (s *Store) DeleteForward(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM forwards WHERE id = ?`, id)
	return err
}

// ---------------------------------------------------------------- known hosts

// KnownHostKeys returns trusted keys for host:port, keyed by key type.
func (s *Store) KnownHostKeys(ctx context.Context, host string) (map[string][]byte, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key_type, public_key FROM known_hosts WHERE host = ?`, host)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]byte{}
	for rows.Next() {
		var t string
		var k []byte
		if err := rows.Scan(&t, &k); err != nil {
			return nil, err
		}
		out[t] = k
	}
	return out, rows.Err()
}

// TrustHostKey records a trusted key, replacing any previous key for the host.
func (s *Store) TrustHostKey(ctx context.Context, host, keyType string, key []byte, fingerprint string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM known_hosts WHERE host = ?`, host); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO known_hosts(host, key_type, public_key, fingerprint, added_at)
		VALUES (?,?,?,?,?)`, host, keyType, key, fingerprint, now()); err != nil {
		return err
	}
	return tx.Commit()
}

// ListKnownHosts returns all trusted host keys.
func (s *Store) ListKnownHosts(ctx context.Context) ([]models.KnownHost, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT host, key_type, fingerprint, added_at FROM known_hosts ORDER BY host`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.KnownHost{}
	for rows.Next() {
		var k models.KnownHost
		if err := rows.Scan(&k.Host, &k.KeyType, &k.Fingerprint, &k.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// ForgetHostKey removes trust for a host.
func (s *Store) ForgetHostKey(ctx context.Context, host string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM known_hosts WHERE host = ?`, host)
	return err
}

// ---------------------------------------------------------------- settings

// LoadSettings returns stored settings merged over defaults.
func (s *Store) LoadSettings(ctx context.Context) (models.Settings, error) {
	st := models.DefaultSettings()
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'app'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return models.DefaultSettings(), nil
	}
	return st, nil
}

// SaveSettings persists settings.
func (s *Store) SaveSettings(ctx context.Context, st models.Settings) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES('app', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, string(raw))
	return err
}

// ClearAll wipes every table.
func (s *Store) ClearAll(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM forwards; DELETE FROM hosts; DELETE FROM groups; DELETE FROM keys;
		DELETE FROM snippets; DELETE FROM known_hosts; DELETE FROM settings; DELETE FROM metrics_minute; DELETE FROM alert_rules;
		DELETE FROM alert_events; DELETE FROM uptime_checks; DELETE FROM uptime_results;`)
	if err == nil {
		_, _ = s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE); VACUUM;`)
	}
	return err
}
