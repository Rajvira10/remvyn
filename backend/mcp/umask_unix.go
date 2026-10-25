//go:build unix

package mcp

import "syscall"

func umask(mask int) int { return syscall.Umask(mask) }
