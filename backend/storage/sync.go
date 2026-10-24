package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"remvyn/backend/models"
)

const syncSchema = `
CREATE TABLE IF NOT EXISTS sync_state (
	path       TEXT PRIMARY KEY,
	kind       TEXT NOT NULL,
	rec_id     TEXT NOT NULL,
	base_hash  TEXT NOT NULL DEFAULT '',
	remote_sha TEXT NOT NULL DEFAULT '',
	seen_hash  TEXT NOT NULL DEFAULT '',
	seen_at    INTEGER NOT NULL DEFAULT 0
);
`

// Kinds of synced records, in the order they must be applied (records only
// refer to kinds earlier in the list).
const (
	KindGroup     = "group"
	KindKey       = "key"
	KindHost      = "host"
	KindForward   = "forward"
	KindSnippet   = "snippet"
	KindKnownHost = "knownhost"
)

// SyncKinds lists record kinds in dependency order.
var SyncKinds = []string{KindGroup, KindKey, KindHost, KindForward, KindSnippet, KindKnownHost}

// SyncRecord is one synced item with its secrets in plaintext. It only ever
// leaves the process encrypted.
type SyncRecord struct {
	Kind string          `json:"kind"`
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
}

type syncHost struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	GroupID        string `json:"groupId"`
	Hostname       string `json:"hostname"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	AuthMethod     string `json:"authMethod"`
	KeyID          string `json:"keyId"`
	Color          string `json:"color"`
	StartupCommand string `json:"startupCommand"`
	CreatedAt      int64  `json:"createdAt"`
	UpdatedAt      int64  `json:"updatedAt"`
	Password       string `json:"password"`
}

type syncKey struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Bits        int    `json:"bits"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"publicKey"`
	CreatedAt   int64  `json:"createdAt"`
	PrivateKey  string `json:"privateKey"`
	Passphrase  string `json:"passphrase"`
}

type syncHostKey struct {
	KeyType     string `json:"keyType"`
	PublicKey   []byte `json:"publicKey"`
	Fingerprint string `json:"fingerprint"`
	AddedAt     int64  `json:"addedAt"`
}

type syncKnownHost struct {
	Host string        `json:"host"`
	Keys []syncHostKey `json:"keys"`
}

func record(kind, id string, v any) (SyncRecord, error) {
	b, err := json.Marshal(v)
	return SyncRecord{Kind: kind, ID: id, Data: b}, err
}

// SyncSnapshot returns every synced record, secrets decrypted. It fails
// while the vault is locked.
func (s *Store) SyncSnapshot(ctx context.Context) ([]SyncRecord, error) {
	if s.Locked() {
		return nil, ErrLocked
	}
	out := []SyncRecord{}
	add := func(kind, id string, v any) error {
		r, err := record(kind, id, v)
		if err == nil {
			out = append(out, r)
		}
		return err
	}

	groups, err := s.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if err := add(KindGroup, g.ID, g); err != nil {
			return nil, err
		}
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, name, type, bits, fingerprint, public_key, private_sealed,
		passphrase_sealed, created_at FROM keys`)
	if err != nil {
		return nil, err
	}
	var keys []syncKey
	for rows.Next() {
		var k syncKey
		var priv, pass []byte
		if err := rows.Scan(&k.ID, &k.Name, &k.Type, &k.Bits, &k.Fingerprint, &k.PublicKey, &priv, &pass, &k.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if k.PrivateKey, err = s.open(priv); err != nil {
			rows.Close()
			return nil, err
		}
		if k.Passphrase, err = s.open(pass); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, k)
	}
	rows.Close()
	for _, k := range keys {
		if err := add(KindKey, k.ID, k); err != nil {
			return nil, err
		}
	}

	rows, err = s.db.QueryContext(ctx, `SELECT id, label, group_id, hostname, port, username, auth_method, key_id,
		color, startup_command, created_at, updated_at, password_sealed FROM hosts`)
	if err != nil {
		return nil, err
	}
	var hosts []syncHost
	for rows.Next() {
		var h syncHost
		var pw []byte
		if err := rows.Scan(&h.ID, &h.Label, &h.GroupID, &h.Hostname, &h.Port, &h.Username, &h.AuthMethod, &h.KeyID,
			&h.Color, &h.StartupCommand, &h.CreatedAt, &h.UpdatedAt, &pw); err != nil {
			rows.Close()
			return nil, err
		}
		if h.Password, err = s.open(pw); err != nil {
			rows.Close()
			return nil, err
		}
		hosts = append(hosts, h)
	}
	rows.Close()
	for _, h := range hosts {
		if err := add(KindHost, h.ID, h); err != nil {
			return nil, err
		}
	}

	forwards, err := s.ListForwards(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range forwards {
		if err := add(KindForward, f.ID, f); err != nil {
			return nil, err
		}
	}

	snippets, err := s.ListSnippets(ctx)
	if err != nil {
		return nil, err
	}
	for _, sn := range snippets {
		if err := add(KindSnippet, sn.ID, sn); err != nil {
			return nil, err
		}
	}

	rows, err = s.db.QueryContext(ctx, `SELECT host, key_type, public_key, fingerprint, added_at FROM known_hosts ORDER BY host, key_type`)
	if err != nil {
		return nil, err
	}
	known := map[string]*syncKnownHost{}
	var order []string
	for rows.Next() {
		var host string
		var k syncHostKey
		if err := rows.Scan(&host, &k.KeyType, &k.PublicKey, &k.Fingerprint, &k.AddedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if known[host] == nil {
			known[host] = &syncKnownHost{Host: host}
			order = append(order, host)
		}
		known[host].Keys = append(known[host].Keys, k)
	}
	rows.Close()
	for _, h := range order {
		if err := add(KindKnownHost, h, known[h]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ApplySyncRecord writes a record received from another device, keeping its
// ID. It does not trigger the change hook.
func (s *Store) ApplySyncRecord(ctx context.Context, r SyncRecord) error {
	switch r.Kind {
	case KindGroup:
		var g models.Group
		if err := json.Unmarshal(r.Data, &g); err != nil {
			return err
		}
		_, err := s.db.ExecContext(ctx, `INSERT INTO groups(id, name, created_at) VALUES(?,?,?)
			ON CONFLICT(id) DO UPDATE SET name = excluded.name, created_at = excluded.created_at`, r.ID, g.Name, g.CreatedAt)
		return err
	case KindKey:
		var k syncKey
		if err := json.Unmarshal(r.Data, &k); err != nil {
			return err
		}
		priv, err := s.seal(k.PrivateKey)
		if err != nil {
			return err
		}
		pass, err := s.seal(k.Passphrase)
		if err != nil {
			return err
		}
		_, err = s.db.ExecContext(ctx, `INSERT INTO keys(id, name, type, bits, fingerprint, public_key, private_sealed,
			passphrase_sealed, created_at) VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,
			type=excluded.type, bits=excluded.bits, fingerprint=excluded.fingerprint, public_key=excluded.public_key,
			private_sealed=excluded.private_sealed, passphrase_sealed=excluded.passphrase_sealed, created_at=excluded.created_at`,
			r.ID, k.Name, k.Type, k.Bits, k.Fingerprint, k.PublicKey, priv, pass, k.CreatedAt)
		return err
	case KindHost:
		var h syncHost
		if err := json.Unmarshal(r.Data, &h); err != nil {
			return err
		}
		pw, err := s.seal(h.Password)
		if err != nil {
			return err
		}
		_, err = s.db.ExecContext(ctx, `INSERT INTO hosts(id, label, group_id, hostname, port, username, auth_method, key_id,
			password_sealed, color, startup_command, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET label=excluded.label, group_id=excluded.group_id, hostname=excluded.hostname,
			port=excluded.port, username=excluded.username, auth_method=excluded.auth_method, key_id=excluded.key_id,
			password_sealed=excluded.password_sealed, color=excluded.color, startup_command=excluded.startup_command,
			created_at=excluded.created_at, updated_at=excluded.updated_at`,
			r.ID, h.Label, h.GroupID, h.Hostname, h.Port, h.Username, h.AuthMethod, h.KeyID, pw, h.Color, h.StartupCommand,
			h.CreatedAt, h.UpdatedAt)
		return err
	case KindForward:
		var f models.ForwardRule
		if err := json.Unmarshal(r.Data, &f); err != nil {
			return err
		}
		_, err := s.db.ExecContext(ctx, `INSERT INTO forwards(id, label, host_id, type, bind_host, bind_port, dest_host,
			dest_port, auto_start, created_at) VALUES (?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET
			label=excluded.label, host_id=excluded.host_id, type=excluded.type, bind_host=excluded.bind_host,
			bind_port=excluded.bind_port, dest_host=excluded.dest_host, dest_port=excluded.dest_port,
			auto_start=excluded.auto_start, created_at=excluded.created_at`,
			r.ID, f.Label, f.HostID, f.Type, f.BindHost, f.BindPort, f.DestHost, f.DestPort, f.AutoStart, f.CreatedAt)
		return err
	case KindSnippet:
		var sn models.Snippet
		if err := json.Unmarshal(r.Data, &sn); err != nil {
			return err
		}
		_, err := s.db.ExecContext(ctx, `INSERT INTO snippets(id, name, command, description, created_at, updated_at)
			VALUES (?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name, command=excluded.command,
			description=excluded.description, created_at=excluded.created_at, updated_at=excluded.updated_at`,
			r.ID, sn.Name, sn.Command, sn.Description, sn.CreatedAt, sn.UpdatedAt)
		return err
	case KindKnownHost:
		var kh syncKnownHost
		if err := json.Unmarshal(r.Data, &kh); err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `DELETE FROM known_hosts WHERE host = ?`, r.ID); err != nil {
			return err
		}
		for _, k := range kh.Keys {
			if _, err := tx.ExecContext(ctx, `INSERT INTO known_hosts(host, key_type, public_key, fingerprint, added_at)
				VALUES (?,?,?,?,?)`, r.ID, k.KeyType, k.PublicKey, k.Fingerprint, k.AddedAt); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
	return fmt.Errorf("unknown record kind %q", r.Kind)
}

// DeleteSyncRecord removes a record deleted on another device. It does not
// trigger the change hook.
func (s *Store) DeleteSyncRecord(ctx context.Context, kind, id string) error {
	var q string
	switch kind {
	case KindGroup:
		if _, err := s.db.ExecContext(ctx, `UPDATE hosts SET group_id = '' WHERE group_id = ?`, id); err != nil {
			return err
		}
		q = `DELETE FROM groups WHERE id = ?`
	case KindKey:
		q = `DELETE FROM keys WHERE id = ?`
	case KindHost:
		q = `DELETE FROM hosts WHERE id = ?`
	case KindForward:
		q = `DELETE FROM forwards WHERE id = ?`
	case KindSnippet:
		q = `DELETE FROM snippets WHERE id = ?`
	case KindKnownHost:
		q = `DELETE FROM known_hosts WHERE host = ?`
	default:
		return fmt.Errorf("unknown record kind %q", kind)
	}
	_, err := s.db.ExecContext(ctx, q, id)
	return err
}

// SyncStateRow is the per-record bookkeeping that lets each device tell
// local changes from remote ones without tombstones.
type SyncStateRow struct {
	Path string
	Kind string
	ID   string
	// BaseHash is the record's local hash when it last matched the remote.
	BaseHash string
	// RemoteSHA is the remote blob when it last matched the local record.
	RemoteSHA string
	// SeenHash/SeenAt record the newest local version and when it was first
	// observed, so offline edits keep their real time.
	SeenHash string
	SeenAt   int64
}

// SyncState loads all bookkeeping rows keyed by path.
func (s *Store) SyncState(ctx context.Context) (map[string]SyncStateRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path, kind, rec_id, base_hash, remote_sha, seen_hash, seen_at FROM sync_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]SyncStateRow{}
	for rows.Next() {
		var r SyncStateRow
		if err := rows.Scan(&r.Path, &r.Kind, &r.ID, &r.BaseHash, &r.RemoteSHA, &r.SeenHash, &r.SeenAt); err != nil {
			return nil, err
		}
		out[r.Path] = r
	}
	return out, rows.Err()
}

// SaveSyncState upserts rows and deletes the given paths in one transaction.
func (s *Store) SaveSyncState(ctx context.Context, put []SyncStateRow, del []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range put {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sync_state(path, kind, rec_id, base_hash, remote_sha, seen_hash, seen_at)
			VALUES (?,?,?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET kind=excluded.kind, rec_id=excluded.rec_id,
			base_hash=excluded.base_hash, remote_sha=excluded.remote_sha, seen_hash=excluded.seen_hash, seen_at=excluded.seen_at`,
			r.Path, r.Kind, r.ID, r.BaseHash, r.RemoteSHA, r.SeenHash, r.SeenAt); err != nil {
			return err
		}
	}
	for _, p := range del {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sync_state WHERE path = ?`, p); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClearSyncState forgets all bookkeeping (when sync is turned off).
func (s *Store) ClearSyncState(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sync_state`)
	return err
}

// GetMeta reads a JSON value stored under key in the settings table.
func (s *Store) GetMeta(ctx context.Context, key string, v any) (bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), v)
}

// SetMeta stores v as JSON under key; nil deletes it.
func (s *Store) SetMeta(ctx context.Context, key string, v any) error {
	if v == nil {
		_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
		return err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, string(raw))
	return err
}

// SealSecret and OpenSecret expose the vault for small secrets kept outside the tables
// (sync tokens and keys).
func (s *Store) SealSecret(plain []byte) ([]byte, error) {
	v := s.vault.Load()
	if v == nil {
		return nil, ErrLocked
	}
	return v.Seal(plain)
}

func (s *Store) OpenSecret(sealed []byte) ([]byte, error) {
	v := s.vault.Load()
	if v == nil {
		return nil, ErrLocked
	}
	return v.Open(sealed)
}
