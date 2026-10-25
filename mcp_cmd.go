package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"remvyn/backend/core"
	"remvyn/backend/mcp"
)

// runMCP is `remvyn mcp`: an MCP server on stdin/stdout that forwards tool
// calls to the running app. MCP clients launch it, e.g.
//
//	claude mcp add remvyn -- /Applications/Remvyn.app/Contents/MacOS/Remvyn mcp
func runMCP() int {
	dir, err := core.DefaultDataDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "remvyn mcp:", err)
		return 1
	}
	if err := mcp.Serve(context.Background(), os.Stdin, os.Stdout, mcp.Bridge(mcp.SocketPath(dir)), Version); err != nil {
		fmt.Fprintln(os.Stderr, "remvyn mcp:", err)
		return 1
	}
	return 0
}

// agentCommand is the command MCP clients run to reach this app.
func agentCommand() []string {
	exe, err := os.Executable()
	if err != nil {
		return []string{"remvyn", "mcp"}
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return []string{exe, "mcp"}
}
