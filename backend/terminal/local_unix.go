//go:build !windows

package terminal

import (
	"os"
	"os/exec"
	"time"

	"github.com/aymanbagabas/go-pty"
)

// The PTY reports EOF once the shell exits, so output drains quickly.
const drainTimeout = 2 * time.Second

// LocalShell returns the user's shell, started as a login shell so it reads
// the same profile as a terminal app would.
func LocalShell() (string, []string) {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
		if p, err := exec.LookPath("bash"); err == nil {
			sh = p
		}
	}
	return sh, []string{"-l"}
}

// afterStart drops our copy of the terminal side, so reads see EOF when the
// shell exits instead of blocking forever.
func afterStart(p pty.Pty) {
	if u, ok := p.(pty.UnixPty); ok {
		_ = u.Slave().Close()
	}
}
