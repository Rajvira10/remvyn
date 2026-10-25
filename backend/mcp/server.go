// Package mcp lets AI agents such as Claude Code use Remvyn's saved hosts
// through the Model Context Protocol, without ever seeing a key or password.
//
//	agent ──MCP over stdio──▶ `remvyn mcp` (Bridge) ──unix socket──▶ Remvyn app (Listen)
//
// The helper process speaks MCP to the agent and forwards each tool call to
// the running app over a socket only the user can open. The app checks its
// policy (per-host access, approvals, lock state), connects with the vault's
// credentials and returns only the result.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

// ServerName and the protocol versions this server can speak.
const ServerName = "remvyn"

var protocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// Result is the outcome of a tool call.
type Result struct {
	Text    string `json:"text"`
	IsError bool   `json:"isError,omitempty"`
}

// Fail and OK build results.
func Fail(msg string) Result { return Result{Text: msg, IsError: true} }
func OK(text string) Result  { return Result{Text: text} }

// Call is one tool invocation.
type Call struct {
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args"`
	// Client is the agent's name from MCP initialize (e.g. "claude-code").
	Client string `json:"client"`
}

// Handler runs tool calls. ctx is cancelled when the agent gives up.
type Handler func(ctx context.Context, c Call) Result

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve runs an MCP server over newline-delimited JSON-RPC (the stdio
// transport) until r ends.
func Serve(ctx context.Context, r io.Reader, w io.Writer, h Handler, version string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s := &session{w: w, h: h, version: version, inflight: map[string]context.CancelFunc{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), MaxRequestBytes)
	var wg sync.WaitGroup
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var m rpcMessage
		if err := json.Unmarshal(line, &m); err != nil {
			s.reply(nil, nil, &rpcError{Code: -32700, Message: "parse error"})
			continue
		}
		if m.Method == "tools/call" && m.ID != nil {
			callCtx, stop := context.WithCancel(ctx)
			s.track(string(m.ID), stop)
			wg.Add(1)
			go func(m rpcMessage) {
				defer wg.Done()
				defer s.untrack(string(m.ID))
				s.toolsCall(callCtx, m)
			}(m)
			continue
		}
		s.handle(m)
	}
	cancel()
	wg.Wait()
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

type session struct {
	h       Handler
	version string

	wmu sync.Mutex
	w   io.Writer

	mu       sync.Mutex
	client   string
	inflight map[string]context.CancelFunc
}

func (s *session) track(id string, cancel context.CancelFunc) {
	s.mu.Lock()
	s.inflight[id] = cancel
	s.mu.Unlock()
}

func (s *session) untrack(id string) {
	s.mu.Lock()
	if c, ok := s.inflight[id]; ok {
		c()
		delete(s.inflight, id)
	}
	s.mu.Unlock()
}

func (s *session) reply(id json.RawMessage, result any, rerr *rpcError) {
	if id == nil && rerr == nil {
		return
	}
	msg := map[string]any{"jsonrpc": "2.0", "id": id}
	if id == nil {
		msg["id"] = nil
	}
	if rerr != nil {
		msg["error"] = rerr
	} else {
		msg["result"] = result
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	_, _ = s.w.Write(append(b, '\n'))
}

func (s *session) handle(m rpcMessage) {
	switch m.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
			ClientInfo      struct {
				Name string `json:"name"`
			} `json:"clientInfo"`
		}
		_ = json.Unmarshal(m.Params, &p)
		s.mu.Lock()
		s.client = p.ClientInfo.Name
		s.mu.Unlock()
		version := protocolVersions[0]
		for _, v := range protocolVersions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		s.reply(m.ID, map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": ServerName, "title": "Remvyn", "version": s.version},
			"instructions":    Instructions,
		}, nil)
	case "ping":
		s.reply(m.ID, map[string]any{}, nil)
	case "tools/list":
		s.reply(m.ID, map[string]any{"tools": Tools}, nil)
	case "notifications/cancelled":
		var p struct {
			RequestID json.RawMessage `json:"requestId"`
		}
		if json.Unmarshal(m.Params, &p) == nil {
			s.untrack(string(p.RequestID))
		}
	case "resources/list":
		s.reply(m.ID, map[string]any{"resources": []any{}}, nil)
	case "prompts/list":
		s.reply(m.ID, map[string]any{"prompts": []any{}}, nil)
	default:
		if m.ID != nil { // notifications need no answer
			s.reply(m.ID, nil, &rpcError{Code: -32601, Message: "method not found: " + m.Method})
		}
	}
}

func (s *session) toolsCall(ctx context.Context, m rpcMessage) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(m.Params, &p); err != nil {
		s.reply(m.ID, nil, &rpcError{Code: -32602, Message: "invalid params"})
		return
	}
	if !knownTool(p.Name) {
		s.reply(m.ID, nil, &rpcError{Code: -32602, Message: "unknown tool: " + p.Name})
		return
	}
	if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
		p.Arguments = json.RawMessage("{}")
	}
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	res := s.h(ctx, Call{Tool: p.Name, Args: p.Arguments, Client: client})
	if ctx.Err() != nil {
		return // cancelled: the client no longer expects an answer
	}
	s.reply(m.ID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": res.Text}},
		"isError": res.IsError,
	}, nil)
}
