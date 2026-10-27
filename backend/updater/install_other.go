//go:build !windows && !darwin && !linux

package updater

import "errors"

func plan() (string, error) {
	return "", errors.New("updates aren't published for this platform")
}

func install(string) error { return errors.New("updates aren't published for this platform") }

func cleanupPlatform() {}
