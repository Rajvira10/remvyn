package main

import (
	"crypto/sha256"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"remvyn/backend/models"
)

// clipboardClearAfter is how long a copied vault secret stays on the
// clipboard before Remvyn clears it.
const clipboardClearAfter = 30 * time.Second

// clipState remembers the last secret copied from the vault (as a hash) so
// it is only cleared if it's still on the clipboard.
type clipState struct {
	mu    sync.Mutex
	sum   [32]byte
	timer *time.Timer
}

// ListVaultItems returns password vault items without secrets.
func (a *App) ListVaultItems() ([]models.VaultItem, error) {
	if err := a.ready(); err != nil {
		return []models.VaultItem{}, err
	}
	return a.core().VaultItems()
}

// SaveVaultItem creates or updates a vault item.
func (a *App) SaveVaultItem(in models.VaultItemInput) (models.VaultItem, error) {
	if err := a.ready(); err != nil {
		return in.Item, err
	}
	return a.core().SaveVaultItem(in)
}

// SetVaultItemFavorite stars or unstars a vault item.
func (a *App) SetVaultItemFavorite(id string, favorite bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core().SetVaultItemFavorite(id, favorite)
}

// DeleteVaultItem removes a vault item.
func (a *App) DeleteVaultItem(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.core().DeleteVaultItem(id)
}

// RevealVaultItem decrypts a vault item's secrets.
func (a *App) RevealVaultItem(id string) (models.VaultSecret, error) {
	if err := a.ready(); err != nil {
		return models.VaultSecret{}, err
	}
	return a.core().RevealVaultItem(id)
}

// VaultItemTOTP returns a vault item's current two-factor code.
func (a *App) VaultItemTOTP(id string) (models.TOTPCode, error) {
	if err := a.ready(); err != nil {
		return models.TOTPCode{}, err
	}
	return a.core().VaultItemTOTP(id)
}

// CopyVaultItemValue copies one value of an item to the clipboard without
// passing it through the UI. Secrets are cleared from the clipboard after
// 30 seconds unless something else was copied since.
func (a *App) CopyVaultItemValue(id, field string) error {
	if err := a.ready(); err != nil {
		return err
	}
	v, err := a.core().VaultItemValue(id, field)
	if err != nil {
		return err
	}
	if err := wruntime.ClipboardSetText(a.ctx, v); err != nil {
		return err
	}
	if field == "username" || field == "url" {
		return nil
	}
	a.clip.mu.Lock()
	defer a.clip.mu.Unlock()
	a.clip.sum = sha256.Sum256([]byte(v))
	if a.clip.timer != nil {
		a.clip.timer.Stop()
	}
	a.clip.timer = time.AfterFunc(clipboardClearAfter, a.clearCopiedSecret)
	return nil
}

// clearCopiedSecret empties the clipboard if it still holds the last copied
// secret.
func (a *App) clearCopiedSecret() {
	a.clip.mu.Lock()
	defer a.clip.mu.Unlock()
	if a.clip.sum == ([32]byte{}) {
		return
	}
	if cur, err := wruntime.ClipboardGetText(a.ctx); err == nil && sha256.Sum256([]byte(cur)) == a.clip.sum {
		_ = wruntime.ClipboardSetText(a.ctx, "")
	}
	a.clip.sum = [32]byte{}
	if a.clip.timer != nil {
		a.clip.timer.Stop()
		a.clip.timer = nil
	}
}
