// Package forward runs SSH port-forwarding rules: local (-L), remote (-R) and
// dynamic SOCKS5 (-D).
package forward

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"remvyn/backend/models"
	"remvyn/backend/sshconn"
)

// EventStatus carries models.ForwardStatus updates.
const EventStatus = "forward:status"

// Emitter delivers events to the UI.
type Emitter func(event string, data any)

// Dialer opens an SSH connection for a rule.
type Dialer func(ctx context.Context, rule models.ForwardRule) (*sshconn.Client, error)

type runner struct {
	rule   models.ForwardRule
	client *sshconn.Client
	ln     net.Listener
	cancel context.CancelFunc

	mu        sync.Mutex
	state     string
	err       string
	startedAt int64
	active    atomic.Int64
	total     atomic.Int64
	in, out   atomic.Int64
	conns     map[net.Conn]struct{}
	stopped   bool
}

// Manager runs forwarding rules.
type Manager struct {
	emit Emitter
	dial Dialer

	mu      sync.Mutex
	runners map[string]*runner
	last    map[string]models.ForwardStatus
}

// New creates a Manager.
func New(emit Emitter, dial Dialer) *Manager {
	return &Manager{emit: emit, dial: dial, runners: map[string]*runner{}, last: map[string]models.ForwardStatus{}}
}

func (r *runner) status() models.ForwardStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return models.ForwardStatus{
		RuleID: r.rule.ID, State: r.state, Error: r.err, StartedAt: r.startedAt,
		ActiveConns: int(r.active.Load()), TotalConns: r.total.Load(), BytesIn: r.in.Load(), BytesOut: r.out.Load(),
	}
}

func (m *Manager) publish(st models.ForwardStatus) {
	m.mu.Lock()
	m.last[st.RuleID] = st
	m.mu.Unlock()
	if m.emit != nil {
		m.emit(EventStatus, st)
	}
}

// Statuses returns the status of every rule that has been started.
func (m *Manager) Statuses() []models.ForwardStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]models.ForwardStatus, 0, len(m.last))
	for id, st := range m.last {
		if r, ok := m.runners[id]; ok {
			st = r.status()
		}
		out = append(out, st)
	}
	return out
}

// Running reports whether a rule is active.
func (m *Manager) Running(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.runners[id]
	return ok
}

// Start connects and begins forwarding. It returns once the listener is up.
func (m *Manager) Start(ctx context.Context, rule models.ForwardRule) error {
	m.mu.Lock()
	if _, ok := m.runners[rule.ID]; ok {
		m.mu.Unlock()
		return errors.New("this forwarding rule is already running")
	}
	rctx, cancel := context.WithCancel(context.Background())
	r := &runner{rule: rule, cancel: cancel, state: models.ForwardStarting, conns: map[net.Conn]struct{}{}}
	m.runners[rule.ID] = r
	m.mu.Unlock()
	m.publish(r.status())

	fail := func(err error) error {
		cancel()
		m.mu.Lock()
		delete(m.runners, rule.ID)
		m.mu.Unlock()
		m.publish(models.ForwardStatus{RuleID: rule.ID, State: models.ForwardError, Error: err.Error()})
		return err
	}

	client, err := m.dial(ctx, rule)
	if err != nil {
		return fail(err)
	}
	r.client = client
	bind := net.JoinHostPort(rule.BindHost, strconv.Itoa(rule.BindPort))
	switch rule.Type {
	case models.ForwardLocal, models.ForwardDynamic:
		r.ln, err = net.Listen("tcp", bind)
		if err != nil {
			client.Close()
			return fail(fmt.Errorf("could not listen on %s: %w", bind, err))
		}
	case models.ForwardRemote:
		r.ln, err = client.Listen("tcp", bind)
		if err != nil {
			client.Close()
			return fail(fmt.Errorf("the server refused to listen on %s (check AllowTcpForwarding/GatewayPorts): %w", bind, err))
		}
	default:
		client.Close()
		return fail(fmt.Errorf("unknown forwarding type %q", rule.Type))
	}

	r.mu.Lock()
	r.state = models.ForwardRunning
	r.startedAt = time.Now().UnixMilli()
	r.mu.Unlock()
	m.publish(r.status())

	go m.acceptLoop(rctx, r)
	go func() {
		// Connection loss stops the rule.
		select {
		case <-client.Done():
			m.finish(r, errors.New("SSH connection lost"))
		case <-rctx.Done():
		}
	}()
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		var last models.ForwardStatus
		for {
			select {
			case <-rctx.Done():
				return
			case <-tick.C:
				if st := r.status(); st != last {
					last = st
					m.publish(st)
				}
			}
		}
	}()
	return nil
}

func (m *Manager) acceptLoop(ctx context.Context, r *runner) {
	for {
		c, err := r.ln.Accept()
		if err != nil {
			if ctx.Err() == nil {
				m.finish(r, fmt.Errorf("listener closed: %w", err))
			}
			return
		}
		go m.handle(ctx, r, c)
	}
}

func (m *Manager) track(r *runner, c net.Conn, add bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if add {
		if r.stopped {
			return false
		}
		r.conns[c] = struct{}{}
	} else {
		delete(r.conns, c)
	}
	return true
}

func (m *Manager) handle(ctx context.Context, r *runner, inbound net.Conn) {
	if !m.track(r, inbound, true) {
		inbound.Close()
		return
	}
	defer m.track(r, inbound, false)
	defer inbound.Close()
	r.total.Add(1)
	r.active.Add(1)
	defer r.active.Add(-1)

	var outbound net.Conn
	var err error
	dest := net.JoinHostPort(r.rule.DestHost, strconv.Itoa(r.rule.DestPort))
	switch r.rule.Type {
	case models.ForwardLocal:
		outbound, err = r.client.Dial("tcp", dest)
	case models.ForwardRemote:
		d := net.Dialer{Timeout: 10 * time.Second}
		outbound, err = d.DialContext(ctx, "tcp", dest)
	case models.ForwardDynamic:
		outbound, err = socksHandshake(inbound, func(addr string) (net.Conn, error) {
			return r.client.Dial("tcp", addr)
		})
	}
	if err != nil || outbound == nil {
		return
	}
	if !m.track(r, outbound, true) {
		outbound.Close()
		return
	}
	defer m.track(r, outbound, false)
	defer outbound.Close()
	pipe(inbound, outbound, &r.out, &r.in)
}

// pipe copies both directions until either side closes.
func pipe(a, b net.Conn, aToB, bToA *atomic.Int64) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn, n *atomic.Int64) {
		c, _ := io.Copy(dst, src)
		n.Add(c)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
		done <- struct{}{}
	}
	go cp(b, a, aToB)
	go cp(a, b, bToA)
	<-done
	// Give the other direction a moment to drain after a half-close.
	select {
	case <-done:
	case <-time.After(30 * time.Second):
	}
}

func (m *Manager) finish(r *runner, cause error) {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	conns := make([]net.Conn, 0, len(r.conns))
	for c := range r.conns {
		conns = append(conns, c)
	}
	if cause != nil {
		r.state, r.err = models.ForwardError, cause.Error()
	} else {
		r.state, r.err = models.ForwardStopped, ""
	}
	r.mu.Unlock()

	r.cancel()
	if r.ln != nil {
		r.ln.Close()
	}
	for _, c := range conns {
		c.Close()
	}
	if r.client != nil {
		r.client.Close()
	}
	m.mu.Lock()
	if m.runners[r.rule.ID] == r {
		delete(m.runners, r.rule.ID)
	}
	m.mu.Unlock()
	m.publish(r.status())
}

// Stop ends a running rule.
func (m *Manager) Stop(id string) {
	m.mu.Lock()
	r, ok := m.runners[id]
	m.mu.Unlock()
	if ok {
		m.finish(r, nil)
	}
}

// StopAll ends every rule (shutdown).
func (m *Manager) StopAll() {
	m.mu.Lock()
	rs := make([]*runner, 0, len(m.runners))
	for _, r := range m.runners {
		rs = append(rs, r)
	}
	m.mu.Unlock()
	for _, r := range rs {
		m.finish(r, nil)
	}
}

// Forget drops the cached status for a deleted rule.
func (m *Manager) Forget(id string) {
	m.Stop(id)
	m.mu.Lock()
	delete(m.last, id)
	m.mu.Unlock()
}
