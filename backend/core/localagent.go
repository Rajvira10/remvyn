package core

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"runtime"
	"sync"

	"golang.org/x/crypto/ssh/agent"

	"remvyn/backend/keyagent"
	"remvyn/backend/mcp"
	"remvyn/backend/models"
)

// Local SSH agent: with Settings.LocalKeyAgent on, programs on this
// computer (git, ssh, an AI coding agent's shell) can log in with the
// keychain's keys by pointing SSH_AUTH_SOCK at Remvyn's socket. The keys
// never leave Remvyn, and every use waits for the user's approval, naming
// the program that asked.

const (
	// LocalAgentSocketName is the socket's file name, next to the AI agent
	// socket in the data folder.
	LocalAgentSocketName = "ssh-agent.sock"
	EventLocalAgent      = "localagent:status" // models.LocalAgentStatus
	// localAgentHost stands in for a host in key-use requests.
	localAgentHost = "local"
)

type localAgent struct {
	mu  sync.Mutex
	ln  net.Listener
	err string
	// agents are the open connections' agents, closed when the socket stops.
	agents map[*keyagent.Agent]struct{}
}

func (c *Core) localAgentPath() string {
	return filepath.Join(filepath.Dir(c.agent.socketPath), LocalAgentSocketName)
}

// LocalAgentStatus describes the socket for the UI.
func (c *Core) LocalAgentStatus() models.LocalAgentStatus {
	l := &c.local
	l.mu.Lock()
	defer l.mu.Unlock()
	return models.LocalAgentStatus{
		Enabled:     c.Settings().LocalKeyAgent,
		Listening:   l.ln != nil,
		ListenError: l.err,
		SocketPath:  c.localAgentPath(),
		Supported:   runtime.GOOS != "windows",
	}
}

// SetLocalKeyAgent turns the local SSH agent on or off.
func (c *Core) SetLocalKeyAgent(on bool) (models.Settings, error) {
	if on && runtime.GOOS == "windows" {
		return c.Settings(), errors.New("the local SSH agent isn't available on Windows yet")
	}
	c.saveMu.Lock()
	st := c.Settings()
	st.LocalKeyAgent = on
	err := c.saveSettingsLocked(st)
	c.saveMu.Unlock()
	if err != nil {
		return c.Settings(), err
	}
	c.applyLocalAgent()
	return c.Settings(), nil
}

// applyLocalAgent opens or closes the socket to match the setting.
func (c *Core) applyLocalAgent() {
	if c.Settings().LocalKeyAgent && runtime.GOOS != "windows" {
		c.startLocalAgent()
	} else {
		c.stopLocalAgent()
	}
}

func (c *Core) startLocalAgent() {
	l := &c.local
	l.mu.Lock()
	if l.ln != nil {
		l.mu.Unlock()
		return
	}
	ln, err := mcp.ListenSocket(c.localAgentPath())
	if err != nil {
		if errors.Is(err, mcp.ErrAlreadyRunning) {
			err = errors.New("another Remvyn window is already serving the SSH agent")
		}
		l.err = err.Error()
	} else {
		l.ln, l.err, l.agents = ln, "", map[*keyagent.Agent]struct{}{}
	}
	l.mu.Unlock()
	if ln != nil {
		go c.serveLocalAgent(ln)
	}
	c.emit(EventLocalAgent, c.LocalAgentStatus())
}

func (c *Core) stopLocalAgent() {
	l := &c.local
	l.mu.Lock()
	ln := l.ln
	agents := l.agents
	l.ln, l.agents, l.err = nil, nil, ""
	l.mu.Unlock()
	if ln == nil {
		return
	}
	// Closing a Unix listener also removes its socket file.
	_ = ln.Close()
	for a := range agents {
		a.Close()
	}
	c.emit(EventLocalAgent, c.LocalAgentStatus())
}

func (c *Core) serveLocalAgent(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go c.serveLocalConn(conn)
	}
}

// serveLocalConn answers one program's agent requests. "Allow for this
// connection" lasts until that program disconnects, usually one ssh run.
func (c *Core) serveLocalConn(conn net.Conn) {
	defer conn.Close()
	who := describePeer(conn)
	h := models.Host{ID: localAgentHost, Label: "This computer"}
	a := c.keyAgentWith(func(ctx context.Context, u keyagent.Use) keyagent.Decision {
		return c.askKeyUse(ctx, h, who, u)
	})
	l := &c.local
	l.mu.Lock()
	if l.agents == nil {
		l.mu.Unlock()
		a.Close()
		return
	}
	l.agents[a] = struct{}{}
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		delete(l.agents, a)
		l.mu.Unlock()
		a.Close()
	}()
	_ = agent.ServeAgent(a, conn)
}
