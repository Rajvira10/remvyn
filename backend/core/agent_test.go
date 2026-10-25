package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"remvyn/backend/ids"
	"remvyn/backend/mcp"
	"remvyn/backend/models"
)

func agentCall(f *fixture, tool string, args map[string]any) mcp.Result {
	f.t.Helper()
	raw, _ := json.Marshal(args)
	return f.core.AgentCall(context.Background(), mcp.Call{Tool: tool, Args: raw, Client: "claude-code"})
}

// trustHost connects once from the UI so the host key is trusted, as a user
// would before opening a host to agents.
func (f *fixture) trustHost() {
	f.t.Helper()
	f.trustAll()
	id := sessionID()
	if _, err := f.core.OpenSFTP(id, f.host.ID, models.ConnectOptions{}); err != nil {
		f.t.Fatal(err)
	}
	f.core.Files().Close(id)
}

func (f *fixture) openToAgents(mode string) {
	f.t.Helper()
	if _, err := f.core.SetAgentEnabled(true); err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.core.SetAgentHostMode(f.host.ID, mode); err != nil {
		f.t.Fatal(err)
	}
}

func mustOK(t *testing.T, r mcp.Result, want ...string) {
	t.Helper()
	if r.IsError {
		t.Fatalf("unexpected error result: %s", r.Text)
	}
	for _, w := range want {
		if !strings.Contains(r.Text, w) {
			t.Fatalf("result %q does not contain %q", r.Text, w)
		}
	}
}

func mustFail(t *testing.T, r mcp.Result, want string) {
	t.Helper()
	if !r.IsError || !strings.Contains(r.Text, want) {
		t.Fatalf("want an error containing %q, got %+v", want, r)
	}
}

func TestAgentAccessIsOffByDefault(t *testing.T) {
	f := setup(t)
	mustFail(t, agentCall(f, mcp.ToolListHosts, nil), "turned off")

	// Turned on, but no host is opened yet: nothing is reachable.
	if _, err := f.core.SetAgentEnabled(true); err != nil {
		t.Fatal(err)
	}
	mustOK(t, agentCall(f, mcp.ToolListHosts, nil), "no servers are opened")
	mustFail(t, agentCall(f, mcp.ToolRunCommand, map[string]any{"host": "test", "command": "id"}), "No servers are opened")
	mustFail(t, agentCall(f, mcp.ToolRunCommand, map[string]any{"host": f.host.ID, "command": "id"}), "No servers are opened")
}

func TestAgentRunsCommandsAndFilesOnAutoHost(t *testing.T) {
	f := setup(t)
	f.trustHost()
	f.openToAgents(models.AgentAuto)

	mustOK(t, agentCall(f, mcp.ToolListHosts, nil), "test (id "+f.host.ID+")", "alice@127.0.0.1", "without approval")

	// Labels match case-insensitively; secrets never appear in results.
	r := agentCall(f, mcp.ToolRunCommand, map[string]any{"host": "TEST", "command": "echo canary-7f3a"})
	mustOK(t, r, "exit code: 0", "exec: echo canary-7f3a")
	if strings.Contains(r.Text, "s3cret-pw") {
		t.Fatal("password leaked into the result")
	}
	r = agentCall(f, mcp.ToolRunCommand, map[string]any{"host": "test", "command": "fail"})
	mustFail(t, r, "exit code: 3")
	if !strings.Contains(r.Text, "--- stderr ---\nboom") {
		t.Fatalf("stderr missing: %q", r.Text)
	}
	r = agentCall(f, mcp.ToolRunCommand, map[string]any{"host": "test", "command": "ls", "cwd": "/var/it's here"})
	mustOK(t, r, `exec: cd '/var/it'\''s here' && ls`)

	// Files: create with parents, read back, list, replace keeping mode.
	dir := t.TempDir()
	file := filepath.Join(dir, "conf", "app.env")
	mustOK(t, agentCall(f, mcp.ToolWriteFile, map[string]any{"host": "test", "path": file, "content": "A=1\n", "create_dirs": true}), "created")
	if b, _ := os.ReadFile(file); string(b) != "A=1\n" {
		t.Fatalf("file content = %q", b)
	}
	mustOK(t, agentCall(f, mcp.ToolReadFile, map[string]any{"host": "test", "path": file}), "A=1")
	if err := os.Chmod(file, 0o600); err != nil {
		t.Fatal(err)
	}
	mustOK(t, agentCall(f, mcp.ToolWriteFile, map[string]any{"host": "test", "path": file, "content": "A=2\n"}), "replaced", "mode 0600")
	if fi, _ := os.Stat(file); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600 kept", fi.Mode().Perm())
	}
	r = agentCall(f, mcp.ToolListDir, map[string]any{"host": "test", "path": filepath.Dir(file)})
	mustOK(t, r, "app.env")
	if strings.Contains(r.Text, ".remvyn-") {
		t.Fatalf("temporary file left behind: %s", r.Text)
	}
	mustFail(t, agentCall(f, mcp.ToolReadFile, map[string]any{"host": "test", "path": filepath.Join(dir, "missing")}), "no such file")

	// Binary files need base64.
	bin := filepath.Join(dir, "blob")
	os.WriteFile(bin, []byte{0, 1, 2, 255}, 0o644)
	mustFail(t, agentCall(f, mcp.ToolReadFile, map[string]any{"host": "test", "path": bin}), "binary")
	mustOK(t, agentCall(f, mcp.ToolReadFile, map[string]any{"host": "test", "path": bin, "encoding": "base64"}), "AAEC/w==")

	// Every action is in the log, with its details sealed in the database.
	log, err := f.core.AgentLog(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 10 {
		t.Fatalf("log has %d entries, want 10", len(log))
	}
	byTool := map[string]models.AgentLogEntry{}
	for _, e := range log {
		if _, ok := byTool[e.Tool+e.Status]; !ok {
			byTool[e.Tool+e.Status] = e
		}
	}
	if e := byTool["run_commandfailed"]; e.ExitCode == nil || *e.ExitCode != 3 || e.Summary != "fail" || e.Approval != "auto" || e.Client != "Claude Code" {
		t.Fatalf("failed command entry = %+v", e)
	}
	if dbContains(t, f.core.DBPath, "canary-7f3a") {
		t.Fatal("agent command stored in plaintext")
	}
}

func TestAgentNeverTrustsNewHostKeys(t *testing.T) {
	f := setup(t)
	f.openToAgents(models.AgentAuto)
	mustFail(t, agentCall(f, mcp.ToolRunCommand, map[string]any{"host": "test", "command": "id"}), "doesn't trust")
	select {
	case p := <-f.ev.prompts:
		t.Fatalf("agent action raised a host-key prompt: %+v", p)
	default:
	}
}

// After the user approves an action themselves, an unknown host key is put to
// them in the normal host-key dialog instead of being refused.
func TestAgentApprovedActionAsksAboutNewHostKey(t *testing.T) {
	f := setup(t)
	f.openToAgents(models.AgentAsk)
	run := func() chan mcp.Result {
		ch := make(chan mcp.Result, 1)
		go func() { ch <- agentCall(f, mcp.ToolRunCommand, map[string]any{"host": "test", "command": "id"}) }()
		return ch
	}
	prompt := func() models.HostKeyPrompt {
		select {
		case p := <-f.ev.prompts:
			return p
		case <-time.After(5 * time.Second):
			t.Fatal("no host-key prompt")
		}
		return models.HostKeyPrompt{}
	}

	res := run()
	f.core.ResolveAgentApproval((<-f.ev.approvals).ID, models.AgentOnce)
	f.core.ResolveHostKey(prompt().ID, false)
	mustFail(t, <-res, "didn't confirm")

	res = run()
	f.core.ResolveAgentApproval((<-f.ev.approvals).ID, models.AgentOnce)
	f.core.ResolveHostKey(prompt().ID, true)
	mustOK(t, <-res, "exec: id")
	if kh, _ := f.core.KnownHosts(); len(kh) != 1 {
		t.Fatalf("known hosts = %+v", kh)
	}
}

func TestAgentAskModeNeedsApproval(t *testing.T) {
	f := setup(t)
	f.trustHost()
	f.openToAgents(models.AgentAsk)
	run := func() chan mcp.Result {
		ch := make(chan mcp.Result, 1)
		go func() { ch <- agentCall(f, mcp.ToolRunCommand, map[string]any{"host": "test", "command": "whoami"}) }()
		return ch
	}
	approval := func() models.AgentApproval {
		select {
		case a := <-f.ev.approvals:
			return a
		case <-time.After(5 * time.Second):
			t.Fatal("no approval requested")
		}
		return models.AgentApproval{}
	}

	res := run()
	a := approval()
	if a.Detail != "whoami" || a.Client != "Claude Code" || a.HostLabel != "test" || a.Title != "run a command" {
		t.Fatalf("approval = %+v", a)
	}
	if got := f.core.PendingAgentApprovals(); len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("pending = %+v", got)
	}
	f.core.ResolveAgentApproval(a.ID, models.AgentDeny)
	mustFail(t, <-res, "denied")

	// Once: runs, and the next action asks again.
	res = run()
	f.core.ResolveAgentApproval(approval().ID, models.AgentOnce)
	mustOK(t, <-res, "exec: whoami")
	res = run()
	f.core.ResolveAgentApproval(approval().ID, models.AgentSession)
	mustOK(t, <-res, "exec: whoami")

	// Allowed for now: no more prompts until revoked.
	mustOK(t, <-run(), "exec: whoami")
	if st := f.core.AgentStatus(); len(st.Grants) != 1 || st.Grants[0].HostID != f.host.ID {
		t.Fatalf("grants = %+v", st.Grants)
	}
	f.core.RevokeAgentGrants()
	res = run()
	f.core.ResolveAgentApproval(approval().ID, models.AgentDeny)
	mustFail(t, <-res, "denied")

	log, _ := f.core.AgentLog(0)
	var statuses []string
	for _, e := range log {
		statuses = append(statuses, e.Status+"/"+e.Approval)
	}
	want := "denied/,ok/session,ok/session,ok/once,denied/"
	if strings.Join(statuses, ",") != want {
		t.Fatalf("log statuses = %v, want %s", statuses, want)
	}
}

func TestAgentApprovalTimesOut(t *testing.T) {
	srvFixture := setup(t)
	f := srvFixture
	// Reopen with a short approval timeout.
	f.core.Close()
	ev := newEvents()
	c, err := Open(context.Background(), Options{DataDir: f.dir, Emit: ev.emit, AgentApprovalTimeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	f.core, f.ev = c, ev
	f.openToAgents(models.AgentAsk)
	mustFail(t, agentCall(f, mcp.ToolRunCommand, map[string]any{"host": "test", "command": "id"}), "within 5 minutes")
	if len(c.PendingAgentApprovals()) != 0 {
		t.Fatal("approval still pending after timeout")
	}
}

func TestAgentKillSwitchAndTimeouts(t *testing.T) {
	f := setup(t)
	f.trustHost()
	f.openToAgents(models.AgentAuto)

	r := agentCall(f, mcp.ToolRunCommand, map[string]any{"host": "test", "command": "hang", "timeout_seconds": 1})
	mustFail(t, r, "timed out after 1s")

	res := make(chan mcp.Result, 1)
	go func() { res <- agentCall(f, mcp.ToolRunCommand, map[string]any{"host": "test", "command": "hang"}) }()
	waitFor(t, "command to start", func() bool { return f.core.AgentStatus().Running == 1 })
	if _, err := f.core.SetAgentEnabled(false); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-res:
		mustFail(t, r, "cancelled")
	case <-time.After(5 * time.Second):
		t.Fatal("turning access off did not stop the running command")
	}
	mustFail(t, agentCall(f, mcp.ToolListHosts, nil), "turned off")
	if f.core.AgentStatus().Running != 0 {
		t.Fatal("action still counted as running")
	}
}

func TestAgentBlockedWhileLocked(t *testing.T) {
	f := setup(t)
	f.trustHost()
	f.openToAgents(models.AgentAuto)
	if _, err := f.core.SetMasterPassword("", "correct horse"); err != nil {
		t.Fatal(err)
	}
	if err := f.core.Lock(); err != nil {
		t.Fatal(err)
	}
	mustFail(t, agentCall(f, mcp.ToolListHosts, nil), "locked")
	if _, err := f.core.AgentLog(0); err == nil {
		t.Fatal("the activity log is readable while locked")
	}
	if err := f.core.Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	mustOK(t, agentCall(f, mcp.ToolRunCommand, map[string]any{"host": "test", "command": "id"}), "exec: id")
}

func TestAgentSettingsAreGuarded(t *testing.T) {
	f := setup(t)
	f.openToAgents(models.AgentAsk)

	// A settings form saved with stale values can't change agent access.
	st := f.core.Settings()
	st.AgentEnabled = false
	st.AgentHosts = map[string]string{f.host.ID: models.AgentAuto}
	if _, err := f.core.UpdateSettings(st); err != nil {
		t.Fatal(err)
	}
	if got := f.core.Settings(); !got.AgentEnabled || got.AgentHosts[f.host.ID] != models.AgentAsk {
		t.Fatalf("agent settings changed through UpdateSettings: %+v", got.AgentHosts)
	}
	if _, err := f.core.SetAgentHostMode(f.host.ID, "root"); err == nil {
		t.Fatal("accepted an unknown mode")
	}
	if _, err := f.core.SetAgentHostMode("nope", models.AgentAuto); err == nil {
		t.Fatal("accepted an unknown host")
	}
	if err := f.core.DeleteHost(f.host.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.core.Settings().AgentHosts[f.host.ID]; ok {
		t.Fatal("deleted host still open to agents")
	}
}

// TestAgentOverMCP drives the real path: MCP client → bridge → socket → app.
func TestAgentOverMCP(t *testing.T) {
	f := setup(t)
	f.trustHost()
	f.openToAgents(models.AgentAuto)
	// Unix socket paths are limited to ~100 bytes, too short for t.TempDir on macOS.
	sock := fmt.Sprintf("/tmp/remvyn-test-%s.sock", ids.New()[:8])
	f.core.agent.socketPath = sock
	f.core.startAgent()
	t.Cleanup(func() { os.Remove(sock) })
	if st := f.core.AgentStatus(); !st.Listening {
		t.Fatalf("not listening: %s", st.ListenError)
	}
	if fi, err := os.Stat(sock); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("socket permissions: %v %v", fi.Mode(), err)
	}

	cr, cw := io.Pipe()
	sr, sw := io.Pipe()
	go func() {
		_ = mcp.Serve(context.Background(), cr, sw, mcp.Bridge(sock), "test")
		sw.Close()
	}()
	out := bufio.NewScanner(sr)
	out.Buffer(make([]byte, 1<<20), 1<<20)
	send := func(msg string) map[string]any {
		t.Helper()
		if _, err := io.WriteString(cw, msg+"\n"); err != nil {
			t.Fatal(err)
		}
		if !out.Scan() {
			t.Fatal("no response")
		}
		var m map[string]any
		if err := json.Unmarshal(out.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	init := send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"claude-code"},"capabilities":{}}}`)
	if res := init["result"].(map[string]any); res["protocolVersion"] != "2025-06-18" {
		t.Fatalf("initialize = %v", init)
	}
	io.WriteString(cw, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n")
	list := send(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if n := len(list["result"].(map[string]any)["tools"].([]any)); n != len(mcp.Tools) {
		t.Fatalf("tools/list returned %d tools", n)
	}
	call := send(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"run_command","arguments":{"host":"test","command":"hostname"}}}`)
	res := call["result"].(map[string]any)
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if res["isError"] != false || !strings.Contains(text, "exec: hostname") {
		t.Fatalf("tools/call = %v", call)
	}
	log, _ := f.core.AgentLog(1)
	if len(log) != 1 || log[0].Client != "Claude Code" {
		t.Fatalf("log = %+v", log)
	}

	// With the app gone, the agent is told to open it.
	f.core.closeAgent()
	call = send(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_hosts","arguments":{}}}`)
	text = call["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if text != mcp.NotRunning {
		t.Fatalf("with no app: %q", text)
	}
	cw.Close()
}
