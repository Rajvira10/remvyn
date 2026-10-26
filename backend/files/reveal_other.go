//go:build !windows

package files

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// revealCmd shows p in the file manager (selected, where supported).
func revealCmd(p string) *exec.Cmd {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-R", p)
	}
	return exec.Command("xdg-open", filepath.Dir(p))
}
