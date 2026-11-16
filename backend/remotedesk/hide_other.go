//go:build !windows

package remotedesk

import "os/exec"

func hideWindow(*exec.Cmd) {}
