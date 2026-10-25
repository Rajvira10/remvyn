//go:build !unix

package mcp

func umask(int) int { return 0 }
