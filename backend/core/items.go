package core

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"remvyn/backend/models"
	"remvyn/backend/totp"
)

// VaultItems lists password vault items without their secrets.
func (c *Core) VaultItems() ([]models.VaultItem, error) {
	return c.store.ListVaultItems(context.Background())
}

// SaveVaultItem creates or updates an item.
func (c *Core) SaveVaultItem(in models.VaultItemInput) (models.VaultItem, error) {
	return c.store.SaveVaultItem(context.Background(), in)
}

// SetVaultItemFavorite stars or unstars an item.
func (c *Core) SetVaultItemFavorite(id string, favorite bool) error {
	return c.store.SetVaultItemFavorite(context.Background(), id, favorite)
}

// DeleteVaultItem removes an item.
func (c *Core) DeleteVaultItem(id string) error {
	return c.store.DeleteVaultItem(context.Background(), id)
}

// RevealVaultItem decrypts an item's secrets for viewing or editing.
func (c *Core) RevealVaultItem(id string) (models.VaultSecret, error) {
	return c.store.VaultItemSecret(context.Background(), id)
}

// VaultItemTOTP returns the item's current two-factor code.
func (c *Core) VaultItemTOTP(id string) (models.TOTPCode, error) {
	sec, err := c.store.VaultItemSecret(context.Background(), id)
	if err != nil {
		return models.TOTPCode{}, err
	}
	if sec.TOTP == "" {
		return models.TOTPCode{}, errors.New("this item has no two-factor secret")
	}
	p, err := totp.Parse(sec.TOTP)
	if err != nil {
		return models.TOTPCode{}, err
	}
	code, exp := p.At(time.Now())
	return models.TOTPCode{Code: code, ExpiresAt: exp.UnixMilli(), Period: p.Period}, nil
}

// VaultItemValue returns one value to copy: "username", "password", "url",
// "totp" (the current code) or "field:<n>".
func (c *Core) VaultItemValue(id, field string) (string, error) {
	switch field {
	case "username", "url":
		items, err := c.store.ListVaultItems(context.Background())
		if err != nil {
			return "", err
		}
		for _, it := range items {
			if it.ID == id {
				if field == "url" {
					return it.URL, nil
				}
				return it.Username, nil
			}
		}
		return "", errors.New("item not found")
	case "totp":
		code, err := c.VaultItemTOTP(id)
		return code.Code, err
	}
	sec, err := c.store.VaultItemSecret(context.Background(), id)
	if err != nil {
		return "", err
	}
	if field == "password" {
		return sec.Password, nil
	}
	if n, ok := strings.CutPrefix(field, "field:"); ok {
		if i, err := strconv.Atoi(n); err == nil && i >= 0 && i < len(sec.Fields) {
			return sec.Fields[i].Value, nil
		}
	}
	return "", errors.New("unknown field")
}
