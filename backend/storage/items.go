package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"remvyn/backend/ids"
	"remvyn/backend/models"
	"remvyn/backend/totp"
)

// Password vault items are stored as two sealed blobs: the details shown in
// lists (name, username, URL) and the secrets (password, two-factor secret,
// notes, extra fields). Nothing about an item is readable without the vault
// key, not even its name.
const itemSchema = `
CREATE TABLE IF NOT EXISTS vault_items (
	id            TEXT PRIMARY KEY,
	meta_sealed   BLOB NOT NULL,
	secret_sealed BLOB,
	created_at    INTEGER NOT NULL,
	updated_at    INTEGER NOT NULL
);
`

// itemMeta is the sealed list view of an item.
type itemMeta struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Username string `json:"username"`
	URL      string `json:"url"`
	Favorite bool   `json:"favorite"`
	// Flags describing the secret, so lists don't need to decrypt it.
	HasPassword bool `json:"hasPassword"`
	HasTOTP     bool `json:"hasTotp"`
	HasNotes    bool `json:"hasNotes"`
	FieldCount  int  `json:"fieldCount"`
}

// Limits keep a single item reasonable; notes can hold recovery codes or a
// certificate, not a database dump.
const (
	maxItemText  = 1024
	maxItemNotes = 64 << 10
	maxFields    = 50
)

func (s *Store) sealJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	vault := s.vault.Load()
	if vault == nil {
		return nil, ErrLocked
	}
	return vault.Seal(b)
}

func (s *Store) openJSON(sealed []byte, v any) error {
	if len(sealed) == 0 {
		return nil
	}
	vault := s.vault.Load()
	if vault == nil {
		return ErrLocked
	}
	b, err := vault.Open(sealed)
	if err != nil {
		return fmt.Errorf("decrypt item: %w", err)
	}
	return json.Unmarshal(b, v)
}

func (m itemMeta) item(id string, created, updated int64) models.VaultItem {
	return models.VaultItem{ID: id, Kind: m.Kind, Name: m.Name, Username: m.Username, URL: m.URL, Favorite: m.Favorite,
		HasPassword: m.HasPassword, HasTOTP: m.HasTOTP, HasNotes: m.HasNotes, FieldCount: m.FieldCount,
		CreatedAt: created, UpdatedAt: updated}
}

// ListVaultItems returns every item without secrets, favourites first. It
// fails while Remvyn is locked.
func (s *Store) ListVaultItems(ctx context.Context) ([]models.VaultItem, error) {
	if s.Locked() {
		return nil, ErrLocked
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, meta_sealed, created_at, updated_at FROM vault_items`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.VaultItem{}
	for rows.Next() {
		var id string
		var sealed []byte
		var created, updated int64
		if err := rows.Scan(&id, &sealed, &created, &updated); err != nil {
			return nil, err
		}
		var m itemMeta
		if err := s.openJSON(sealed, &m); err != nil {
			return nil, err
		}
		out = append(out, m.item(id, created, updated))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Favorite != out[j].Favorite {
			return out[i].Favorite
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (s *Store) loadItem(ctx context.Context, id string) (itemMeta, []byte, int64, error) {
	var metaSealed, secretSealed []byte
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT meta_sealed, secret_sealed, created_at FROM vault_items WHERE id = ?`, id).
		Scan(&metaSealed, &secretSealed, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return itemMeta{}, nil, 0, ErrNotFound
	}
	if err != nil {
		return itemMeta{}, nil, 0, err
	}
	var m itemMeta
	if err := s.openJSON(metaSealed, &m); err != nil {
		return m, nil, 0, err
	}
	return m, secretSealed, created, nil
}

// VaultItemSecret decrypts an item's secrets.
func (s *Store) VaultItemSecret(ctx context.Context, id string) (models.VaultSecret, error) {
	sec := models.VaultSecret{Fields: []models.VaultField{}}
	if s.Locked() {
		return sec, ErrLocked
	}
	_, sealed, _, err := s.loadItem(ctx, id)
	if err != nil {
		return sec, err
	}
	if err := s.openJSON(sealed, &sec); err != nil {
		return sec, err
	}
	if sec.Fields == nil {
		sec.Fields = []models.VaultField{}
	}
	return sec, nil
}

// ValidateVaultItem normalises and checks an item and its secret.
func ValidateVaultItem(it *models.VaultItem, sec *models.VaultSecret) error {
	it.Name = strings.TrimSpace(it.Name)
	it.Username = strings.TrimSpace(it.Username)
	it.URL = strings.TrimSpace(it.URL)
	switch it.Kind {
	case models.ItemLogin, models.ItemToken, models.ItemNote:
	case "":
		it.Kind = models.ItemLogin
	default:
		return fmt.Errorf("unknown item type %q", it.Kind)
	}
	if it.Name == "" && it.URL != "" {
		it.Name = siteName(it.URL)
	}
	if err := required("name", it.Name); err != nil {
		return err
	}
	for _, f := range []string{it.Name, it.Username, it.URL} {
		if len(f) > maxItemText {
			return fmt.Errorf("fields must be under %d characters", maxItemText)
		}
	}
	if sec == nil {
		return nil
	}
	if len(sec.Password) > maxItemNotes || len(sec.Notes) > maxItemNotes {
		return errors.New("the password or notes are too long (64 KB at most)")
	}
	sec.TOTP = strings.TrimSpace(sec.TOTP)
	if sec.TOTP != "" {
		if _, err := totp.Parse(sec.TOTP); err != nil {
			return err
		}
	}
	if len(sec.Fields) > maxFields {
		return fmt.Errorf("an item can have at most %d extra fields", maxFields)
	}
	fields := []models.VaultField{}
	for _, f := range sec.Fields {
		f.Name = strings.TrimSpace(f.Name)
		if f.Name == "" && f.Value == "" {
			continue
		}
		if f.Name == "" {
			return errors.New("every extra field needs a name")
		}
		if len(f.Name) > maxItemText || len(f.Value) > maxItemNotes {
			return errors.New("an extra field is too long")
		}
		fields = append(fields, f)
	}
	sec.Fields = fields
	return nil
}

// siteName turns "https://mail.google.com/x" into "mail.google.com".
func siteName(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		u, err = url.Parse("https://" + raw)
	}
	if err != nil || u.Host == "" {
		return raw
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

// SaveVaultItem creates or updates an item. A nil secret keeps the saved one.
func (s *Store) SaveVaultItem(ctx context.Context, in models.VaultItemInput) (models.VaultItem, error) {
	it, sec := in.Item, in.Secret
	if s.Locked() {
		return it, ErrLocked
	}
	if err := ValidateVaultItem(&it, sec); err != nil {
		return it, err
	}
	ts := now()
	var secretSealed []byte
	created := ts
	isNew := it.ID == ""
	if !isNew {
		old, oldSecret, oldCreated, err := s.loadItem(ctx, it.ID)
		if err != nil {
			return it, err
		}
		created = oldCreated
		if sec == nil {
			secretSealed = oldSecret
			it.HasPassword, it.HasTOTP, it.HasNotes, it.FieldCount = old.HasPassword, old.HasTOTP, old.HasNotes, old.FieldCount
		}
	} else {
		it.ID = ids.New()
	}
	if sec != nil {
		it.HasPassword, it.HasTOTP, it.HasNotes, it.FieldCount = sec.Password != "", sec.TOTP != "", sec.Notes != "", len(sec.Fields)
		var err error
		if secretSealed, err = s.sealJSON(sec); err != nil {
			return it, err
		}
	}
	meta := itemMeta{Kind: it.Kind, Name: it.Name, Username: it.Username, URL: it.URL, Favorite: it.Favorite,
		HasPassword: it.HasPassword, HasTOTP: it.HasTOTP, HasNotes: it.HasNotes, FieldCount: it.FieldCount}
	metaSealed, err := s.sealJSON(meta)
	if err != nil {
		return it, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO vault_items(id, meta_sealed, secret_sealed, created_at, updated_at)
		VALUES (?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET meta_sealed=excluded.meta_sealed,
		secret_sealed=excluded.secret_sealed, updated_at=excluded.updated_at`,
		it.ID, metaSealed, secretSealed, created, ts)
	it.CreatedAt, it.UpdatedAt = created, ts
	return it, s.changed(err)
}

// SetVaultItemFavorite stars or unstars an item.
func (s *Store) SetVaultItemFavorite(ctx context.Context, id string, favorite bool) error {
	if s.Locked() {
		return ErrLocked
	}
	m, _, _, err := s.loadItem(ctx, id)
	if err != nil {
		return err
	}
	m.Favorite = favorite
	sealed, err := s.sealJSON(m)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE vault_items SET meta_sealed = ?, updated_at = ? WHERE id = ?`, sealed, now(), id)
	return s.changed(err)
}

// DeleteVaultItem removes an item.
func (s *Store) DeleteVaultItem(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM vault_items WHERE id = ?`, id)
	return s.changed(err)
}

// syncItem is an item with its secrets, as carried (encrypted) by sync.
type syncItem struct {
	itemMeta
	Secret    models.VaultSecret `json:"secret"`
	CreatedAt int64              `json:"createdAt"`
	UpdatedAt int64              `json:"updatedAt"`
}

func (s *Store) syncItems(ctx context.Context) ([]SyncRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, meta_sealed, secret_sealed, created_at, updated_at FROM vault_items ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SyncRecord
	for rows.Next() {
		var id string
		var metaSealed, secretSealed []byte
		var si syncItem
		if err := rows.Scan(&id, &metaSealed, &secretSealed, &si.CreatedAt, &si.UpdatedAt); err != nil {
			return nil, err
		}
		if err := s.openJSON(metaSealed, &si.itemMeta); err != nil {
			return nil, err
		}
		if err := s.openJSON(secretSealed, &si.Secret); err != nil {
			return nil, err
		}
		r, err := record(KindVaultItem, id, si)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) applySyncItem(ctx context.Context, id string, data []byte) error {
	var si syncItem
	if err := json.Unmarshal(data, &si); err != nil {
		return err
	}
	metaSealed, err := s.sealJSON(si.itemMeta)
	if err != nil {
		return err
	}
	secretSealed, err := s.sealJSON(si.Secret)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO vault_items(id, meta_sealed, secret_sealed, created_at, updated_at)
		VALUES (?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET meta_sealed=excluded.meta_sealed,
		secret_sealed=excluded.secret_sealed, created_at=excluded.created_at, updated_at=excluded.updated_at`,
		id, metaSealed, secretSealed, si.CreatedAt, si.UpdatedAt)
	return err
}
