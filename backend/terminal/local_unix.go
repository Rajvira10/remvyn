//go:build !windows

package terminal

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aymanbagabas/go-pty"
	"golang.org/x/sys/unix"
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

// shellDir returns the working directory of the program in the foreground of
// the terminal (the innermost shell at a prompt), falling back to the login
// shell.
func shellDir(p pty.Pty, shell int) string {
	pid := shell
	if u, ok := p.(pty.UnixPty); ok {
		if pg, err := unix.IoctlGetInt(int(u.Master().Fd()), unix.TIOCGPGRP); err == nil && pg > 0 {
			pid = pg
		}
	}
	if d := processDir(pid); d != "" || pid == shell {
		return d
	}
	return processDir(shell)
}

func processDir(pid int) string {
	if d, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd"); err == nil {
		return d
	}
	// macOS has no /proc.
	out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if d, ok := strings.CutPrefix(line, "n"); ok && strings.HasPrefix(d, "/") {
			return d
		}
	}
	return ""
}
