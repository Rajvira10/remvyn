package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"remvyn/backend/ids"
)

// One computer can keep several vaults, each in its own data folder and
// usually synced to its own repository. The first vault lives in the data
// folder itself (so existing installs carry on unchanged); the others live
// under vaults/<id>. vaults.json in the data folder lists them and says
// which one is open.

// DefaultVaultID is the vault kept in the data folder itself.
const DefaultVaultID = "default"

const (
	vaultsFile    = "vaults.json"
	vaultsSubdir  = "vaults"
	maxVaultName  = 60
	defaultVaultN = "Default"
)

// VaultProfile describes one vault on this computer.
type VaultProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Repo      string `json:"repo"`
	CreatedAt int64  `json:"createdAt"`
	Current   bool   `json:"current"`
}

type vaultsDoc struct {
	Active string         `json:"active"`
	Vaults []VaultProfile `json:"vaults"`
}

// Vaults is the list of vaults kept in a base data folder.
type Vaults struct {
	base string
	mu   sync.Mutex
}

// OpenVaults reads the vault list in base (the default data folder).
func OpenVaults(base string) *Vaults { return &Vaults{base: base} }

// Base is the data folder holding the default vault and the list.
func (v *Vaults) Base() string { return v.base }

func (v *Vaults) load() (vaultsDoc, error) {
	var d vaultsDoc
	raw, err := os.ReadFile(filepath.Join(v.base, vaultsFile))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return d, err
	}
	if err == nil {
		if err := json.Unmarshal(raw, &d); err != nil {
			return d, fmt.Errorf("read %s: %w", vaultsFile, err)
		}
	}
	if d.find(DefaultVaultID) < 0 {
		d.Vaults = append([]VaultProfile{{ID: DefaultVaultID, Name: defaultVaultN}}, d.Vaults...)
	}
	if d.find(d.Active) < 0 {
		d.Active = DefaultVaultID
	}
	return d, nil
}

func (v *Vaults) save(d vaultsDoc) error {
	for i := range d.Vaults {
		d.Vaults[i].Current = false
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(v.base, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(v.base, vaultsFile+".tmp")
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(v.base, vaultsFile))
}

func (d vaultsDoc) find(id string) int {
	for i, p := range d.Vaults {
		if p.ID == id {
			return i
		}
	}
	return -1
}

func (v *Vaults) dir(id string) string {
	if id == DefaultVaultID {
		return v.base
	}
	return filepath.Join(v.base, vaultsSubdir, id)
}

// List returns every vault, the open one marked Current.
func (v *Vaults) List() ([]VaultProfile, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, err := v.load()
	if err != nil {
		return nil, err
	}
	for i := range d.Vaults {
		d.Vaults[i].Current = d.Vaults[i].ID == d.Active
	}
	return d.Vaults, nil
}

// Active returns the open vault and its data folder.
func (v *Vaults) Active() (VaultProfile, string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, err := v.load()
	if err != nil {
		return VaultProfile{}, v.base, err
	}
	p := d.Vaults[d.find(d.Active)]
	p.Current = true
	return p, v.dir(p.ID), nil
}

func cleanVaultName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "", errors.New("give the vault a name")
	}
	if len([]rune(name)) > maxVaultName {
		return "", fmt.Errorf("keep the name under %d characters", maxVaultName)
	}
	return name, nil
}

// Create adds an empty vault (not yet open) and returns it.
func (v *Vaults) Create(name string) (VaultProfile, error) {
	name, err := cleanVaultName(name)
	if err != nil {
		return VaultProfile{}, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	d, err := v.load()
	if err != nil {
		return VaultProfile{}, err
	}
	p := VaultProfile{ID: ids.New()[:12], Name: name, CreatedAt: time.Now().UnixMilli()}
	if err := os.MkdirAll(v.dir(p.ID), 0o700); err != nil {
		return VaultProfile{}, fmt.Errorf("create vault folder: %w", err)
	}
	d.Vaults = append(d.Vaults, p)
	return p, v.save(d)
}

// SetActive marks id as the vault to open, and returns its data folder.
func (v *Vaults) SetActive(id string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, err := v.load()
	if err != nil {
		return "", err
	}
	if d.find(id) < 0 {
		return "", errors.New("that vault no longer exists")
	}
	d.Active = id
	return v.dir(id), v.save(d)
}

// Rename changes a vault's name.
func (v *Vaults) Rename(id, name string) error {
	name, err := cleanVaultName(name)
	if err != nil {
		return err
	}
	return v.update(id, func(p *VaultProfile) { p.Name = name })
}

// SetRepo records which repository a vault syncs with ("" for none), so
// the list can show it while the vault is closed.
func (v *Vaults) SetRepo(id, repo string) error {
	return v.update(id, func(p *VaultProfile) { p.Repo = repo })
}

func (v *Vaults) update(id string, fn func(*VaultProfile)) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, err := v.load()
	if err != nil {
		return err
	}
	i := d.find(id)
	if i < 0 {
		return errors.New("that vault no longer exists")
	}
	before := d.Vaults[i]
	fn(&d.Vaults[i])
	if d.Vaults[i] == before {
		return nil
	}
	return v.save(d)
}

// Delete erases a vault's local data. The open vault and the default one
// can't be deleted. A synced copy on GitHub is left untouched.
func (v *Vaults) Delete(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, err := v.load()
	if err != nil {
		return err
	}
	i := d.find(id)
	switch {
	case i < 0:
		return nil
	case id == DefaultVaultID:
		return errors.New("the default vault can't be deleted")
	case id == d.Active:
		return errors.New("switch to another vault before deleting this one")
	}
	if err := os.RemoveAll(v.dir(id)); err != nil {
		return fmt.Errorf("delete vault folder: %w", err)
	}
	d.Vaults = append(d.Vaults[:i], d.Vaults[i+1:]...)
	return v.save(d)
}
