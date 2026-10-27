//go:build darwin

package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The .dmg is mounted, its Remvyn.app copied next to the running bundle,
// and the two swapped with renames. The old bundle is deleted by the
// relaunch helper once this process has exited.

func plan() (string, error) {
	const suffix = "-macOS-universal.dmg"
	bundle, err := appBundle()
	if err != nil {
		return suffix, err
	}
	if dir := filepath.Dir(bundle); !writable(dir) {
		return suffix, fmt.Errorf("Remvyn can't write to %s; download the new version from the release page", dir)
	}
	return suffix, nil
}

// appBundle is the .app the running binary lives in.
func appBundle() (string, error) {
	exe, err := executable()
	if err != nil {
		return "", err
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe))) // X.app/Contents/MacOS/X
	if !strings.HasSuffix(bundle, ".app") {
		return "", errors.New("Remvyn isn't running from an app bundle; download the new version from the release page")
	}
	return bundle, nil
}

func install(file string) error {
	bundle, err := appBundle()
	if err != nil {
		return err
	}
	mnt, err := os.MkdirTemp(filepath.Dir(file), "mnt-")
	if err != nil {
		return err
	}
	if out, err := exec.Command("hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", mnt, file).CombinedOutput(); err != nil {
		return fmt.Errorf("could not open the disk image: %s", strings.TrimSpace(string(out)))
	}
	defer func() {
		_ = exec.Command("hdiutil", "detach", mnt, "-force").Run()
		_ = os.Remove(mnt)
		_ = os.Remove(file)
	}()
	apps, _ := filepath.Glob(filepath.Join(mnt, "*.app"))
	if len(apps) == 0 {
		return errors.New("no app found in the disk image")
	}

	staged, old := bundle+".new", bundle+".old"
	_ = os.RemoveAll(staged)
	if out, err := exec.Command("ditto", apps[0], staged).CombinedOutput(); err != nil {
		os.RemoveAll(staged)
		return fmt.Errorf("could not copy the app: %s", strings.TrimSpace(string(out)))
	}
	_ = exec.Command("xattr", "-dr", "com.apple.quarantine", staged).Run()
	_ = os.RemoveAll(old)
	if err := os.Rename(bundle, old); err != nil {
		os.RemoveAll(staged)
		return err
	}
	if err := os.Rename(staged, bundle); err != nil {
		_ = os.Rename(old, bundle)
		os.RemoveAll(staged)
		return err
	}
	return relaunch(`rm -rf "$3"; open "$2"`, bundle, old)
}

func cleanupPlatform() {
	if bundle, err := appBundle(); err == nil {
		_ = os.RemoveAll(bundle + ".old")
		_ = os.RemoveAll(bundle + ".new")
	}
}
