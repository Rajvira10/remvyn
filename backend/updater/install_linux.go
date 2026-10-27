//go:build linux

package updater

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The tarball's remvyn binary replaces the running one with a rename, which
// is atomic and safe while it runs. Copies under /usr belong to a package
// manager (the Arch PKGBUILD), which should do the update instead.

func plan() (string, error) {
	const suffix = "-linux-x86_64.tar.gz"
	if runtime.GOARCH != "amd64" {
		return "", errors.New("updates are only published for x86_64 Linux")
	}
	exe, err := executable()
	if err != nil {
		return suffix, err
	}
	if strings.HasPrefix(exe, "/usr/") && !strings.HasPrefix(exe, "/usr/local/") {
		return suffix, errors.New("Remvyn was installed by your package manager; update it there (on Arch, makepkg -si with the new PKGBUILD)")
	}
	if dir := filepath.Dir(exe); !writable(dir) {
		return suffix, fmt.Errorf("Remvyn can't write to %s; download the new version from the release page", dir)
	}
	return suffix, nil
}

func install(file string) error {
	exe, err := executable()
	if err != nil {
		return err
	}
	staged := filepath.Join(filepath.Dir(exe), ".remvyn.new")
	if err := extractTarGz(file, "remvyn", staged, 0o755); err != nil {
		return err
	}
	_ = os.Remove(file)
	if err := os.Rename(staged, exe); err != nil {
		os.Remove(staged)
		return err
	}
	return relaunch(`exec "$2"`, exe)
}

func cleanupPlatform() {
	if exe, err := executable(); err == nil {
		_ = os.Remove(filepath.Join(filepath.Dir(exe), ".remvyn.new"))
	}
}
