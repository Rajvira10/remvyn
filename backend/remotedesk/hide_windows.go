//go:build windows

package remotedesk

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps console tools (cmdkey) from flashing a window.
func hideWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
