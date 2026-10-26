package files

import (
	"os/exec"
	"syscall"
)

// revealCmd selects p in Explorer. Explorer doesn't parse a quoted
// "/select,<path>" argument, so the command line is built by hand.
func revealCmd(p string) *exec.Cmd {
	cmd := exec.Command("explorer")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer /select,"` + p + `"`}
	return cmd
}
