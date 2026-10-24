// Package vaultsync keeps the vault in sync across devices through a private
// GitHub repository. Every file in the repository is encrypted with a sync
// key that only exists wrapped by the user's master password, so GitHub
// never sees plaintext.
//
// Layout:
//
//	vault.json        the sync key, wrapped with the master password
//	records/<name>    one encrypted file per host, key, snippet, ...
//
// Each device keeps per-record bookkeeping (storage.SyncStateRow): the local
// hash and remote blob it last agreed on. Comparing both sides against that
// base tells local edits, remote edits and deletions apart without
// tombstones; when both sides changed a record, the newer edit wins.
package vaultsync

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"flux/backend/security"
	"flux/backend/storage"
)

var b64Std = base64.StdEncoding

const (
	vaultFile     = "vault.json"
	recordsPrefix = "records/"
	vaultFormat   = "flux-vault"
)

// Errors with UI codes.
var (
	ErrVaultMissing = &APIError{Code: "vault_missing", Message: "the vault file is missing from the repository; disconnect and set up sync again"}
	ErrVaultReset   = &APIError{Code: "vault_reset", Message: "the vault in the repository was replaced from another device; disconnect and join it again"}
)

// vaultDoc is vault.json.
type vaultDoc struct {
	Format    string              `json:"format"`
	Version   int                 `json:"version"`
	Key       security.WrappedKey `json:"key"`
	KeyID     string              `json:"keyId"`
	CreatedAt int64               `json:"createdAt"`
}

// Cipher encrypts records with keys derived from the sync key.
type Cipher struct {
	aead    cipher.AEAD
	nameKey []byte
	keyID   string
}

func derive(key []byte, label string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(label))
	return m.Sum(nil)
}

// NewCipher creates a record cipher from a 32-byte sync key.
func NewCipher(syncKey []byte) (*Cipher, error) {
	if len(syncKey) != 32 {
		return nil, errors.New("sync key must be 32 bytes")
	}
	block, err := aes.NewCipher(derive(syncKey, "flux-sync-records-v1"))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead, nameKey: derive(syncKey, "flux-sync-names-v1"),
		keyID: hex.EncodeToString(derive(syncKey, "flux-sync-key-id-v1")[:12])}, nil
}

// Path returns the opaque repository path for a record.
func (c *Cipher) Path(kind, id string) string {
	m := hmac.New(sha256.New, c.nameKey)
	m.Write([]byte(kind + "\x00" + id))
	return recordsPrefix + hex.EncodeToString(m.Sum(nil)[:16])
}

// payload is the plaintext inside a record file.
type payload struct {
	Kind      string          `json:"kind"`
	ID        string          `json:"id"`
	ChangedAt int64           `json:"changedAt"`
	Data      json.RawMessage `json:"data"`
}

// Seal encrypts a payload, binding it to its path.
func (c *Cipher) seal(path string, p payload) ([]byte, error) {
	plain, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ct := c.aead.Seal(nonce, nonce, plain, []byte(path))
	return []byte(b64Std.EncodeToString(ct)), nil
}

func (c *Cipher) open(path string, content []byte) (payload, error) {
	var p payload
	ct, err := b64Std.DecodeString(strings.TrimSpace(string(content)))
	if err != nil || len(ct) < c.aead.NonceSize() {
		return p, fmt.Errorf("%s is not a Flux record", path)
	}
	n := c.aead.NonceSize()
	plain, err := c.aead.Open(nil, ct[:n], ct[n:], []byte(path))
	if err != nil {
		return p, fmt.Errorf("%s could not be decrypted", path)
	}
	if err := json.Unmarshal(plain, &p); err != nil {
		return p, err
	}
	if c.Path(p.Kind, p.ID) != path {
		return p, fmt.Errorf("%s does not match its contents", path)
	}
	return p, nil
}

// NewVaultDoc wraps a fresh sync key with password.
func NewVaultDoc(password string, now time.Time) ([]byte, []byte, error) {
	key, err := security.NewKey()
	if err != nil {
		return nil, nil, err
	}
	doc, err := vaultDocFor(key, password, now.UnixMilli())
	return doc, key, err
}

func vaultDocFor(key []byte, password string, created int64) ([]byte, error) {
	w, err := security.WrapKey(key, password)
	if err != nil {
		return nil, err
	}
	c, err := NewCipher(key)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(vaultDoc{Format: vaultFormat, Version: 1, Key: w, KeyID: c.keyID, CreatedAt: created}, "", "  ")
}

// OpenVaultDoc unwraps the sync key in vault.json with password.
func OpenVaultDoc(raw []byte, password string) ([]byte, error) {
	var d vaultDoc
	if err := json.Unmarshal(raw, &d); err != nil || d.Format != vaultFormat {
		return nil, errors.New("this repository doesn't contain a Flux vault")
	}
	if d.Version > 1 {
		return nil, errors.New("this vault was created by a newer version of Flux; update Flux to sync")
	}
	return d.Key.Unwrap(password)
}

func vaultKeyID(raw []byte) string {
	var d vaultDoc
	_ = json.Unmarshal(raw, &d)
	return d.KeyID
}

// ---------------------------------------------------------------- engine

// Source is the local side of sync (implemented by *storage.Store).
type Source interface {
	SyncSnapshot(ctx context.Context) ([]storage.SyncRecord, error)
	ApplySyncRecord(ctx context.Context, r storage.SyncRecord) error
	DeleteSyncRecord(ctx context.Context, kind, id string) error
	SyncState(ctx context.Context) (map[string]storage.SyncStateRow, error)
	SaveSyncState(ctx context.Context, put []storage.SyncStateRow, del []string) error
}

// Engine runs sync passes between a Source and a Remote.
type Engine struct {
	Src    Source
	Remote Remote
	Cipher *Cipher
	Device string
	Now    func() time.Time

	cache    Snapshot
	vaultSHA string
}

// Result summarises one pass.
type Result struct {
	Uploaded      int    `json:"uploaded"`
	Downloaded    int    `json:"downloaded"`
	DeletedRemote int    `json:"deletedRemote"`
	DeletedLocal  int    `json:"deletedLocal"`
	Commit        string `json:"commit"`
}

// Changed reports whether this device's data changed.
func (r Result) Changed() bool { return r.Downloaded > 0 || r.DeletedLocal > 0 }

type localRec struct {
	rec  storage.SyncRecord
	hash string
}

func recordHash(r storage.SyncRecord) string {
	h := sha256.New()
	h.Write([]byte(r.Kind + "\x00" + r.ID + "\x00"))
	h.Write(r.Data)
	return hex.EncodeToString(h.Sum(nil))
}

func kindRank(kind string) int {
	for i, k := range storage.SyncKinds {
		if k == kind {
			return i
		}
	}
	return len(storage.SyncKinds)
}

func (e *Engine) now() int64 {
	if e.Now != nil {
		return e.Now().UnixMilli()
	}
	return time.Now().UnixMilli()
}

func (e *Engine) scanLocal(ctx context.Context) (map[string]localRec, map[string]storage.SyncStateRow, error) {
	recs, err := e.Src.SyncSnapshot(ctx)
	if err != nil {
		return nil, nil, err
	}
	state, err := e.Src.SyncState(ctx)
	if err != nil {
		return nil, nil, err
	}
	local := make(map[string]localRec, len(recs))
	var seen []storage.SyncStateRow
	now := e.now()
	for _, r := range recs {
		p := e.Cipher.Path(r.Kind, r.ID)
		l := localRec{rec: r, hash: recordHash(r)}
		local[p] = l
		st, ok := state[p]
		if !ok || st.SeenHash != l.hash {
			st.Path, st.Kind, st.ID = p, r.Kind, r.ID
			st.SeenHash, st.SeenAt = l.hash, now
			state[p] = st
			seen = append(seen, st)
		}
	}
	if len(seen) > 0 {
		if err := e.Src.SaveSyncState(ctx, seen, nil); err != nil {
			return nil, nil, err
		}
	}
	return local, state, nil
}

// Scan records when local edits happened without contacting the remote, so
// offline edits keep their real time for conflict resolution.
func (e *Engine) Scan(ctx context.Context) error {
	_, _, err := e.scanLocal(ctx)
	return err
}

func (e *Engine) head(ctx context.Context) (Snapshot, error) {
	ref, err := e.Remote.Ref(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if ref != "" && ref == e.cache.Commit {
		return e.cache, nil
	}
	s, err := e.Remote.Snapshot(ctx, ref)
	if err != nil {
		return Snapshot{}, err
	}
	e.cache = s
	return s, nil
}

func (e *Engine) checkVault(ctx context.Context, snap Snapshot) error {
	sha, ok := snap.Files[vaultFile]
	if !ok {
		return ErrVaultMissing
	}
	if sha == e.vaultSHA {
		return nil
	}
	raw, err := e.Remote.Read(ctx, sha)
	if err != nil {
		return err
	}
	if vaultKeyID(raw) != e.Cipher.keyID {
		return ErrVaultReset
	}
	e.vaultSHA = sha
	return nil
}

type download struct {
	path string
	sha  string
	p    payload
}

// Sync runs one pass, retrying if another device commits concurrently.
func (e *Engine) Sync(ctx context.Context) (Result, error) {
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		res, err := e.syncOnce(ctx)
		if !errors.Is(err, ErrConflict) {
			return res, err
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case <-time.After(time.Duration(attempt*150) * time.Millisecond):
		}
	}
	return Result{}, lastErr
}

func (e *Engine) syncOnce(ctx context.Context) (Result, error) {
	var res Result
	local, state, err := e.scanLocal(ctx)
	if err != nil {
		return res, err
	}
	snap, err := e.head(ctx)
	if err != nil {
		return res, err
	}
	if err := e.checkVault(ctx, snap); err != nil {
		return res, err
	}

	paths := map[string]bool{}
	for p := range local {
		paths[p] = true
	}
	for p := range state {
		paths[p] = true
	}
	for p := range snap.Files {
		if strings.HasPrefix(p, recordsPrefix) {
			paths[p] = true
		}
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)

	changes := map[string][]byte{}
	var downloads []download
	var localDeletes []storage.SyncStateRow
	var put []storage.SyncStateRow
	var drop []string
	uploaded := map[string]localRec{}

	fetch := func(p, sha string) (payload, error) {
		raw, err := e.Remote.Read(ctx, sha)
		if err != nil {
			return payload{}, err
		}
		return e.Cipher.open(p, raw)
	}
	upload := func(p string, l localRec, st storage.SyncStateRow) error {
		changedAt := st.SeenAt
		if changedAt == 0 {
			changedAt = e.now()
		}
		content, err := e.Cipher.seal(p, payload{Kind: l.rec.Kind, ID: l.rec.ID, ChangedAt: changedAt, Data: l.rec.Data})
		if err != nil {
			return err
		}
		changes[p] = content
		uploaded[p] = l
		return nil
	}

	for _, p := range sorted {
		l, inL := local[p]
		st, inS := state[p]
		rsha, inR := snap.Files[p]
		known := inS && (st.BaseHash != "" || st.RemoteSHA != "")
		localChanged := inL && (!known || st.BaseHash != l.hash)
		localDeleted := !inL && known && st.BaseHash != ""
		remoteChanged := inR && (!known || st.RemoteSHA != rsha)
		remoteDeleted := !inR && known && st.RemoteSHA != ""

		switch {
		case remoteChanged && localChanged:
			rp, err := fetch(p, rsha)
			if err != nil {
				return res, err
			}
			rh := recordHash(storage.SyncRecord{Kind: rp.Kind, ID: rp.ID, Data: rp.Data})
			switch {
			case rh == l.hash:
				st.BaseHash, st.RemoteSHA = l.hash, rsha
				put = append(put, st)
			case !known || rp.ChangedAt > st.SeenAt:
				// Without a shared base (a device re-joining), the repository's
				// copy wins: local "edit times" are just when we first saw it.
				downloads = append(downloads, download{p, rsha, rp})
			default:
				if err := upload(p, l, st); err != nil {
					return res, err
				}
			}
		case remoteChanged:
			// Local unchanged or deleted: the remote edit wins (an edit beats a
			// deletion so nothing is lost).
			rp, err := fetch(p, rsha)
			if err != nil {
				return res, err
			}
			downloads = append(downloads, download{p, rsha, rp})
		case localChanged:
			if err := upload(p, l, st); err != nil {
				return res, err
			}
		case localDeleted && inR:
			changes[p] = nil
		case remoteDeleted && inL:
			localDeletes = append(localDeletes, st)
		case !inL && !inR:
			drop = append(drop, p)
		}
	}

	if len(changes) > 0 {
		next, err := e.Remote.Commit(ctx, snap, changes, e.message(changes))
		if err != nil {
			return res, err
		}
		e.cache = next
		snap = next
		for p, c := range changes {
			if c == nil {
				res.DeletedRemote++
				drop = append(drop, p)
				continue
			}
			l := uploaded[p]
			st := state[p]
			st.Path, st.Kind, st.ID = p, l.rec.Kind, l.rec.ID
			st.BaseHash, st.RemoteSHA = l.hash, BlobSHA(c)
			put = append(put, st)
			res.Uploaded++
		}
	}

	// Apply remote changes in dependency order, deletions in reverse.
	sort.SliceStable(downloads, func(i, j int) bool { return kindRank(downloads[i].p.Kind) < kindRank(downloads[j].p.Kind) })
	for _, d := range downloads {
		if err := e.Src.ApplySyncRecord(ctx, storage.SyncRecord{Kind: d.p.Kind, ID: d.p.ID, Data: d.p.Data}); err != nil {
			return res, fmt.Errorf("apply %s: %w", d.p.Kind, err)
		}
		res.Downloaded++
	}
	sort.SliceStable(localDeletes, func(i, j int) bool { return kindRank(localDeletes[i].Kind) > kindRank(localDeletes[j].Kind) })
	for _, st := range localDeletes {
		if err := e.Src.DeleteSyncRecord(ctx, st.Kind, st.ID); err != nil {
			return res, err
		}
		drop = append(drop, st.Path)
		res.DeletedLocal++
	}
	if len(downloads) > 0 {
		// Hash what was actually stored, so the next scan sees no local edit.
		recs, err := e.Src.SyncSnapshot(ctx)
		if err != nil {
			return res, err
		}
		stored := map[string]string{}
		for _, r := range recs {
			stored[e.Cipher.Path(r.Kind, r.ID)] = recordHash(r)
		}
		for _, d := range downloads {
			h := stored[d.path]
			put = append(put, storage.SyncStateRow{Path: d.path, Kind: d.p.Kind, ID: d.p.ID, BaseHash: h, RemoteSHA: d.sha,
				SeenHash: h, SeenAt: d.p.ChangedAt})
		}
	}
	if err := e.Src.SaveSyncState(ctx, put, drop); err != nil {
		return res, err
	}
	res.Commit = snap.Commit
	return res, nil
}

func (e *Engine) message(changes map[string][]byte) string {
	up, del := 0, 0
	for _, c := range changes {
		if c == nil {
			del++
		} else {
			up++
		}
	}
	var parts []string
	if up > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", up))
	}
	if del > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", del))
	}
	msg := "Sync: " + strings.Join(parts, ", ")
	if e.Device != "" {
		msg += " from " + e.Device
	}
	return msg
}
