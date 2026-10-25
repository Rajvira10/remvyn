package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"remvyn/backend/ids"
	"remvyn/backend/mcp"
	"remvyn/backend/models"
	"remvyn/backend/sshconn"
)

// AI agent access: Claude Code (or any MCP client) reaches Remvyn through
// `remvyn mcp`, which forwards tool calls to the socket served here. Nothing
// is reachable until the user turns access on and opens individual hosts;
// hosts in "ask" mode wait for approval in the UI for every action.

// Agent events.
const (
	EventAgentApproval     = "agent:approval"      // models.AgentApproval
	EventAgentApprovalDone = "agent:approval-done" // approval ID
	EventAgentActivity     = "agent:activity"      // models.AgentLogEntry
	EventAgentStatus       = "agent:status"        // models.AgentStatus
)

const (
	agentApprovalTimeout = 5 * time.Minute
	agentIdleClose       = 5 * time.Minute
	agentLogOutputMax    = 16 << 10
)

type agentState struct {
	socketPath      string
	command         []string
	approvalTimeout time.Duration

	mu        sync.Mutex
	listener  *mcp.Listener
	listenErr string
	approvals map[string]*pendingApproval
	grants    map[string]time.Time // host ID → allowed without asking until
	ops       map[int]context.CancelFunc
	nextOp    int
	conns     map[string]*agentConn
}

type pendingApproval struct {
	a  models.AgentApproval
	ch chan string
}

func newAgentState(socketPath string, command []string, timeout time.Duration) *agentState {
	if len(command) == 0 {
		command = []string{"remvyn", "mcp"}
	}
	if timeout <= 0 {
		timeout = agentApprovalTimeout
	}
	return &agentState{
		socketPath:      socketPath,
		command:         command,
		approvalTimeout: timeout,
		approvals:       map[string]*pendingApproval{},
		grants:          map[string]time.Time{},
		ops:             map[int]context.CancelFunc{},
		conns:           map[string]*agentConn{},
	}
}

// startAgent opens the socket. Failure is reported in AgentStatus rather
// than stopping the app.
func (c *Core) startAgent() {
	a := c.agent
	a.mu.Lock()
	if a.listener != nil {
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()
	l, err := mcp.Listen(a.socketPath, c.AgentCall)
	a.mu.Lock()
	if err != nil {
		a.listenErr = err.Error()
	} else {
		a.listener, a.listenErr = l, ""
	}
	a.mu.Unlock()
	go c.agentReaper()
	c.emitAgentStatus()
}

func (c *Core) closeAgent() {
	a := c.agent
	a.mu.Lock()
	l := a.listener
	a.listener = nil
	a.mu.Unlock()
	if l != nil {
		l.Close()
	}
	c.agentStop()
}

// agentStop cancels running actions, refuses pending approvals, forgets
// temporary grants and closes agent connections (kill switch, lock).
func (c *Core) agentStop() {
	a := c.agent
	a.mu.Lock()
	ops := slices.Collect(maps.Values(a.ops))
	pending := slices.Collect(maps.Values(a.approvals))
	conns := a.conns
	a.conns = map[string]*agentConn{}
	clear(a.grants)
	a.mu.Unlock()
	for _, cancel := range ops {
		cancel()
	}
	for _, p := range pending {
		select {
		case p.ch <- models.AgentDeny:
		default:
		}
	}
	for _, ac := range conns {
		ac.close()
	}
	c.emitAgentStatus()
}

// agentForgetHost drops a host's access, grant and connection.
func (c *Core) agentForgetHost(hostID string) {
	a := c.agent
	a.mu.Lock()
	delete(a.grants, hostID)
	ac := a.conns[hostID]
	delete(a.conns, hostID)
	var pending []*pendingApproval
	for _, p := range a.approvals {
		if p.a.HostID == hostID {
			pending = append(pending, p)
		}
	}
	a.mu.Unlock()
	for _, p := range pending {
		select {
		case p.ch <- models.AgentDeny:
		default:
		}
	}
	if ac != nil {
		ac.close()
	}
}

// AgentStatus describes the bridge for the UI.
func (c *Core) AgentStatus() models.AgentStatus {
	a := c.agent
	a.mu.Lock()
	defer a.mu.Unlock()
	st := models.AgentStatus{
		Enabled:     c.Settings().AgentEnabled,
		Listening:   a.listener != nil,
		ListenError: a.listenErr,
		SocketPath:  a.socketPath,
		Command:     a.command,
		Running:     len(a.ops),
		Grants:      []models.AgentGrant{},
	}
	now := time.Now()
	for id, until := range a.grants {
		if until.After(now) {
			st.Grants = append(st.Grants, models.AgentGrant{HostID: id, Until: until.UnixMilli()})
		}
	}
	return st
}

func (c *Core) emitAgentStatus() { c.emit(EventAgentStatus, c.AgentStatus()) }

// SetAgentEnabled turns agent access on or off. Off stops everything at once.
func (c *Core) SetAgentEnabled(on bool) (models.Settings, error) {
	c.saveMu.Lock()
	st := c.Settings()
	st.AgentEnabled = on
	err := c.saveSettingsLocked(st)
	c.saveMu.Unlock()
	if err != nil {
		return c.Settings(), err
	}
	if !on {
		c.agentStop()
	}
	c.emitAgentStatus()
	return c.Settings(), nil
}

// SetAgentHostMode opens a host to agents (ask or auto) or closes it ("").
func (c *Core) SetAgentHostMode(hostID, mode string) (models.Settings, error) {
	switch mode {
	case "", models.AgentAsk, models.AgentAuto:
	default:
		return c.Settings(), fmt.Errorf("unknown access mode %q", mode)
	}
	if _, err := c.store.GetHost(context.Background(), hostID); err != nil && mode != "" {
		return c.Settings(), errors.New("host not found")
	}
	c.saveMu.Lock()
	st := c.Settings()
	hosts := maps.Clone(st.AgentHosts)
	if hosts == nil {
		hosts = map[string]string{}
	}
	if mode == "" {
		delete(hosts, hostID)
	} else {
		hosts[hostID] = mode
	}
	st.AgentHosts = hosts
	err := c.saveSettingsLocked(st)
	c.saveMu.Unlock()
	if err != nil {
		return c.Settings(), err
	}
	if mode == "" {
		c.agentForgetHost(hostID)
	}
	c.emitAgentStatus()
	return c.Settings(), nil
}

// saveSettingsLocked persists settings; c.saveMu must be held.
func (c *Core) saveSettingsLocked(st models.Settings) error {
	if err := c.store.SaveSettings(context.Background(), st); err != nil {
		return err
	}
	c.mu.Lock()
	c.settings = st
	c.mu.Unlock()
	return nil
}

// RevokeAgentGrants makes every host in "ask" mode ask again.
func (c *Core) RevokeAgentGrants() {
	c.agent.mu.Lock()
	clear(c.agent.grants)
	c.agent.mu.Unlock()
	c.emitAgentStatus()
}

// PendingAgentApprovals lists approvals waiting for the user.
func (c *Core) PendingAgentApprovals() []models.AgentApproval {
	a := c.agent
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []models.AgentApproval{}
	for _, p := range a.approvals {
		out = append(out, p.a)
	}
	slices.SortFunc(out, func(x, y models.AgentApproval) int { return int(x.CreatedAt - y.CreatedAt) })
	return out
}

// ResolveAgentApproval answers an approval: deny, once or session.
func (c *Core) ResolveAgentApproval(id, decision string) bool {
	switch decision {
	case models.AgentDeny, models.AgentOnce, models.AgentSession:
	default:
		return false
	}
	c.agent.mu.Lock()
	p, ok := c.agent.approvals[id]
	c.agent.mu.Unlock()
	if ok {
		select {
		case p.ch <- decision:
		default:
		}
	}
	return ok
}

// AgentLog returns recent agent actions, newest first.
func (c *Core) AgentLog(limit int) ([]models.AgentLogEntry, error) {
	return c.store.ListAgentLog(context.Background(), limit)
}

// ClearAgentLog deletes the activity log.
func (c *Core) ClearAgentLog() error { return c.store.ClearAgentLog(context.Background()) }

// ---------------------------------------------------------------- calls

func clientName(raw string) string {
	switch strings.ToLower(raw) {
	case "claude-code", "claude code":
		return "Claude Code"
	case "":
		return "An AI agent"
	}
	return raw
}

// agentAction is a validated tool call, ready for approval and execution.
type agentAction struct {
	title   string // completes "Claude Code wants to …"
	detail  string // the command or path
	preview string
	run     func(ctx context.Context, ac *agentConn) agentOutcome
}

type agentOutcome struct {
	text  string
	exit  *int
	isErr bool
}

// AgentCall runs one tool call from an agent. It is the socket's handler.
func (c *Core) AgentCall(ctx context.Context, call mcp.Call) mcp.Result {
	st := c.Settings()
	if !st.AgentEnabled {
		return mcp.Fail("AI agent access is turned off in Remvyn. Ask the user to turn it on in Remvyn → AI agents.")
	}
	if c.store.Locked() {
		return mcp.Fail("Remvyn is locked. Ask the user to unlock it, then try again.")
	}
	if call.Tool == mcp.ToolListHosts {
		return c.agentListHosts(st)
	}
	var target struct {
		Host string `json:"host"`
	}
	if err := json.Unmarshal(call.Args, &target); err != nil {
		return mcp.Fail("invalid arguments: " + err.Error())
	}
	h, mode, res := c.agentHost(st, target.Host)
	if res != nil {
		return *res
	}
	act, err := buildAgentAction(call.Tool, call.Args)
	if err != nil {
		return mcp.Fail(err.Error())
	}

	ctx, done := c.agentBegin(ctx)
	defer done()
	client := clientName(call.Client)
	entry := models.AgentLogEntry{Client: client, HostID: h.ID, HostLabel: h.Label, Tool: call.Tool, Summary: act.detail}

	approval := c.agentApprove(ctx, client, h, mode, call.Tool, act)
	switch approval {
	case "cancelled":
		return mcp.Fail("cancelled")
	case models.AgentDeny, "timeout":
		entry.Status = "denied"
		msg := "The user denied this action in Remvyn. Don't retry it; ask the user how they want to proceed."
		if approval == "timeout" {
			msg = "Nobody approved this action in Remvyn within 5 minutes. Ask the user to watch the Remvyn window, then try again."
		}
		entry.Output = msg
		c.agentLog(entry)
		return mcp.Fail(msg)
	}
	entry.Approval = approval

	start := time.Now()
	ac, err := c.agentConn(ctx, h, approval == models.AgentOnce || approval == models.AgentSession)
	var out agentOutcome
	if err != nil {
		out = agentOutcome{text: agentDialError(h, err), isErr: true}
	} else {
		out = act.run(ctx, ac)
	}
	entry.DurationMs = time.Since(start).Milliseconds()
	entry.ExitCode = out.exit
	switch {
	case err != nil || (out.isErr && out.exit == nil):
		entry.Status = "error"
	case out.isErr:
		entry.Status = "failed"
	default:
		entry.Status = "ok"
	}
	entry.Output = trimMiddle(out.text, agentLogOutputMax)
	c.agentLog(entry)
	if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return mcp.Fail("Stopped: the action was cancelled (agent access was turned off, Remvyn was locked, or the request was cancelled).")
	}
	return mcp.Result{Text: out.text, IsError: out.isErr}
}

// agentBegin registers a cancellable action so the kill switch can stop it.
func (c *Core) agentBegin(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	a := c.agent
	a.mu.Lock()
	a.nextOp++
	id := a.nextOp
	a.ops[id] = cancel
	a.mu.Unlock()
	c.emitAgentStatus()
	return ctx, func() {
		a.mu.Lock()
		delete(a.ops, id)
		a.mu.Unlock()
		cancel()
		c.emitAgentStatus()
	}
}

func (c *Core) agentLog(e models.AgentLogEntry) {
	e.At = time.Now().UnixMilli()
	e.ID = ids.New()
	saved, err := c.store.InsertAgentLog(context.Background(), e)
	if err == nil {
		c.emit(EventAgentActivity, saved)
	}
}

func (c *Core) agentHosts(st models.Settings) []models.Host {
	all, _ := c.store.ListHosts(context.Background())
	out := []models.Host{}
	for _, h := range all {
		if st.AgentHosts[h.ID] != "" {
			out = append(out, h)
		}
	}
	return out
}

func (c *Core) agentListHosts(st models.Settings) mcp.Result {
	hosts := c.agentHosts(st)
	if len(hosts) == 0 {
		return mcp.OK("Agent access is on, but no servers are opened to agents yet. Ask the user to choose servers in Remvyn → AI agents.")
	}
	groups := map[string]string{}
	if gs, err := c.store.ListGroups(context.Background()); err == nil {
		for _, g := range gs {
			groups[g.ID] = g.Name
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d server(s) available:\n", len(hosts))
	for _, h := range hosts {
		fmt.Fprintf(&b, "- %s (id %s): %s", h.Label, h.ID, remote(h))
		if g := groups[h.GroupID]; g != "" {
			fmt.Fprintf(&b, ", group %s", g)
		}
		if st.AgentHosts[h.ID] == models.AgentAuto {
			b.WriteString(", runs without approval\n")
		} else {
			b.WriteString(", each action needs the user's approval\n")
		}
	}
	return mcp.OK(strings.TrimRight(b.String(), "\n"))
}

// agentHost finds an opened host by id or label.
func (c *Core) agentHost(st models.Settings, ref string) (models.Host, string, *mcp.Result) {
	ref = strings.TrimSpace(ref)
	hosts := c.agentHosts(st)
	var matches []models.Host
	for _, h := range hosts {
		if h.ID == ref {
			return h, st.AgentHosts[h.ID], nil
		}
		if strings.EqualFold(h.Label, ref) {
			matches = append(matches, h)
		}
	}
	if len(matches) == 1 {
		return matches[0], st.AgentHosts[matches[0].ID], nil
	}
	var res mcp.Result
	switch {
	case ref == "":
		res = mcp.Fail("host is required; call list_hosts to see the available servers")
	case len(matches) > 1:
		res = mcp.Fail(fmt.Sprintf("more than one server is called %q; use its id from list_hosts", ref))
	case len(hosts) == 0:
		res = mcp.Fail("No servers are opened to agents. Ask the user to choose servers in Remvyn → AI agents.")
	default:
		labels := make([]string, len(hosts))
		for i, h := range hosts {
			labels[i] = h.Label
		}
		res = mcp.Fail(fmt.Sprintf("No server called %q is available to agents. Available: %s.", ref, strings.Join(labels, ", ")))
	}
	return models.Host{}, "", &res
}

// agentApprove returns auto, session, once, deny, timeout or cancelled.
func (c *Core) agentApprove(ctx context.Context, client string, h models.Host, mode, tool string, act agentAction) string {
	if mode == models.AgentAuto {
		return "auto"
	}
	a := c.agent
	now := time.Now()
	p := &pendingApproval{ch: make(chan string, 1), a: models.AgentApproval{
		ID: ids.New(), Client: client, HostID: h.ID, HostLabel: h.Label, Remote: remote(h), Tool: tool,
		Title: act.title, Detail: act.detail, Preview: act.preview,
		CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(a.approvalTimeout).UnixMilli(),
	}}
	a.mu.Lock()
	if until, ok := a.grants[h.ID]; ok && until.After(now) {
		a.mu.Unlock()
		return models.AgentSession
	}
	a.approvals[p.a.ID] = p
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.approvals, p.a.ID)
		a.mu.Unlock()
		c.emit(EventAgentApprovalDone, p.a.ID)
	}()
	c.emit(EventAgentApproval, p.a)
	if c.notify != nil {
		c.notify(fmt.Sprintf("%s wants to %s", client, act.title), h.Label+": "+trimMiddle(act.detail, 120))
	}
	timer := time.NewTimer(a.approvalTimeout)
	defer timer.Stop()
	select {
	case d := <-p.ch:
		if d == models.AgentSession {
			a.mu.Lock()
			a.grants[h.ID] = time.Now().Add(models.AgentGrantMinutes * time.Minute)
			a.mu.Unlock()
			c.emitAgentStatus()
		}
		return d
	case <-timer.C:
		return "timeout"
	case <-ctx.Done():
		return "cancelled"
	}
}

// agentDialError explains a connection failure in terms the agent can act on.
func agentDialError(h models.Host, err error) string {
	switch sshconn.CodeOf(err) {
	case sshconn.CodePasswordRequired:
		return fmt.Sprintf("No password is saved for %s in Remvyn. Ask the user to save it in the host's settings.", h.Label)
	case sshconn.CodePassphraseRequired:
		return fmt.Sprintf("The SSH key for %s needs a passphrase that isn't saved in Remvyn. Ask the user to save it in Keychain.", h.Label)
	case sshconn.CodeHostKeyRejected:
		return fmt.Sprintf("Remvyn doesn't trust %s's host key yet, or the key has changed, and the user didn't confirm it. Ask the user to connect to it once from Remvyn and check the key.", h.Label)
	case sshconn.CodeAuth:
		return fmt.Sprintf("The server rejected the saved credentials for %s: %v", h.Label, err)
	case CodeLocked:
		return "Remvyn is locked. Ask the user to unlock it, then try again."
	}
	return fmt.Sprintf("Could not connect to %s: %v", h.Label, err)
}

// agentReaper closes agent connections that have been idle for a while.
func (c *Core) agentReaper() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-t.C:
			a := c.agent
			var idle []*agentConn
			a.mu.Lock()
			for id, ac := range a.conns {
				if ac.idleFor() > agentIdleClose {
					idle = append(idle, ac)
					delete(a.conns, id)
				}
			}
			a.mu.Unlock()
			for _, ac := range idle {
				ac.close()
			}
		}
	}
}

// trimMiddle keeps the start and end of s within max bytes.
func trimMiddle(s string, max int) string {
	if len(s) <= max {
		return s
	}
	head := max * 2 / 3
	tail := max - head
	return s[:head] + fmt.Sprintf("\n… [%d bytes omitted] …\n", len(s)-head-tail) + s[len(s)-tail:]
}
