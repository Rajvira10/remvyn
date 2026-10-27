//go:build darwin || linux

package updater

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// relaunch starts a detached shell that waits for this process to exit and
// then runs then. Positional parameters are $2, $3, … (args); $1 is our pid.
// Values travel as arguments, never interpolated into the script.
func relaunch(then string, args ...string) error {
	script := `while kill -0 "$1" 2>/dev/null; do sleep 0.2; done; ` + then
	argv := append([]string{"-c", script, "sh", strconv.Itoa(os.Getpid())}, args...)
	cmd := exec.Command("/bin/sh", argv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
