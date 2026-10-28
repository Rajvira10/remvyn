package terminal

import (
	"os"
	"os/exec"
	"time"

	"github.com/aymanbagabas/go-pty"
)

// ConPTY keeps its output pipe open until the console is closed, so don't
// wait long for a final read after the shell exits.
const drainTimeout = 250 * time.Millisecond

// LocalShell returns PowerShell 7 if installed, otherwise Windows PowerShell,
// otherwise cmd.
func LocalShell() (string, []string) {
	for _, name := range []string{"pwsh.exe", "powershell.exe"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, []string{"-NoLogo"}
		}
	}
	if c := os.Getenv("COMSPEC"); c != "" {
		return c, nil
	}
	return "cmd.exe", nil
}

func afterStart(pty.Pty) {}

// shellDir is unknown on Windows; the path picker starts in the home folder.
func shellDir(pty.Pty, int) string { return "" }
