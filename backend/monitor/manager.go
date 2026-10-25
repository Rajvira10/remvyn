package monitor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"remvyn/backend/models"
	"remvyn/backend/sshconn"
)

// Events emitted by the manager.
const (
	EventSample = "monitor:sample" // models.Metrics
	EventStatus = "monitor:status" // models.MonitorStatus
)

// ClientFunc returns an SSH client for a host. shared is true when the
// client belongs to someone else (an open terminal) and must not be closed.
type ClientFunc func(ctx context.Context, hostID string) (client *sshconn.Client, shared bool, err error)

// Options configure a Manager.
type Options struct {
	Emit   func(event string, data any)
	Client ClientFunc
	// OnSample is called for every successful sample (history, alerts).
	OnSample func(models.Metrics)
	// OnError is called when collection fails, and OnRecover when it works
	// again (host reachability alerts).
	OnError   func(hostID string, err error)
	OnRecover func(hostID string)
	// OnStop is called when a host's collector stops.
	OnStop func(hostID string)
	// LiveInterval is the sampling interval while someone is watching.
	LiveInterval time.Duration
	// SlowEvery is how often Docker and services are refreshed while live.
	SlowEvery time.Duration
	// Timeout bounds one sample.
	Timeout time.Duration
}

// Manager runs one collector per monitored host.
type Manager struct {
	opts Options

	mu         sync.Mutex
	cols       map[string]*collector
	bg         map[string]bool
	bgInterval time.Duration
	stopped    bool
}

// New creates a Manager.
func New(o Options) *Manager {
	if o.Emit == nil {
		o.Emit = func(string, any) {}
	}
	if o.LiveInterval <= 0 {
		o.LiveInterval = 2 * time.Second
	}
	if o.SlowEvery <= 0 {
		o.SlowEvery = 10 * time.Second
	}
	if o.Timeout <= 0 {
		o.Timeout = 20 * time.Second
	}
	return &Manager{opts: o, cols: map[string]*collector{}, bg: map[string]bool{}, bgInterval: 30 * time.Second}
}

type collector struct {
	m      *Manager
	hostID string
	wake   chan struct{}
	done   chan struct{}

	// guarded by m.mu
	viewers int
	status  models.MonitorStatus
	latest  *models.Metrics

	// owned by the run goroutine (client also read by Exec under clientMu)
	clientMu sync.Mutex
	client   *sshconn.Client
	shared   bool

	prev      *raw
	lastSlow  time.Time
	forceSlow atomic.Bool // set by ContainerAction to refresh containers soon
	docker    models.DockerInfo
	services  models.ServicesInfo
	failing   bool
}

// ensure returns the host's collector, starting it if needed. Caller holds m.mu.
func (m *Manager) ensure(hostID string) *collector {
	if c, ok := m.cols[hostID]; ok {
		return c
	}
	c := &collector{m: m, hostID: hostID, wake: make(chan struct{}, 1), done: make(chan struct{}),
		docker: models.DockerInfo{Containers: []models.Container{}}, services: models.ServicesInfo{Items: []models.Service{}}}
	c.status = models.MonitorStatus{HostID: hostID, State: "connecting"}
	m.cols[hostID] = c
	go c.run()
	return c
}

func (c *collector) poke() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// Watch registers a live viewer (a monitor panel) for a host.
func (m *Manager) Watch(hostID string) models.MonitorStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return models.MonitorStatus{HostID: hostID, State: "stopped"}
	}
	c := m.ensure(hostID)
	c.viewers++
	c.poke()
	return c.status
}

// Unwatch removes a live viewer.
func (m *Manager) Unwatch(hostID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.cols[hostID]; ok && c.viewers > 0 {
		c.viewers--
		c.poke()
	}
}

// SetBackground sets the hosts sampled continuously while Remvyn runs.
func (m *Manager) SetBackground(hostIDs []string, interval time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	m.bgInterval = interval
	next := map[string]bool{}
	for _, id := range hostIDs {
		next[id] = true
	}
	m.bg = next
	if m.stopped {
		return
	}
	for id := range next {
		m.ensure(id)
	}
	for _, c := range m.cols {
		c.poke()
	}
}

// Statuses returns every collector's state.
func (m *Manager) Statuses() []models.MonitorStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]models.MonitorStatus, 0, len(m.cols))
	for _, c := range m.cols {
		out = append(out, c.status)
	}
	return out
}

// Latest returns the most recent sample for a host, if any.
func (m *Manager) Latest(hostID string) (models.Metrics, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.cols[hostID]; ok && c.latest != nil {
		return *c.latest, true
	}
	return models.Metrics{}, false
}

// StopAll ends every collector.
func (m *Manager) StopAll() {
	m.mu.Lock()
	m.stopped = true
	cols := make([]*collector, 0, len(m.cols))
	for _, c := range m.cols {
		cols = append(cols, c)
	}
	m.mu.Unlock()
	for _, c := range cols {
		c.poke()
		<-c.done
	}
}

func (c *collector) setStatus(f func(s *models.MonitorStatus)) {
	c.m.mu.Lock()
	f(&c.status)
	c.status.Background = c.m.bg[c.hostID]
	st := c.status
	c.m.mu.Unlock()
	c.m.opts.Emit(EventStatus, st)
}

func (c *collector) closeClient() {
	c.clientMu.Lock()
	defer c.clientMu.Unlock()
	if c.client != nil && !c.shared {
		c.client.Close()
	}
	c.client = nil
}

func (c *collector) run() {
	defer close(c.done)
	defer c.closeClient()
	failures := 0
	for {
		m := c.m
		m.mu.Lock()
		viewers, bg, interval, stopped := c.viewers, m.bg[c.hostID], m.bgInterval, m.stopped
		if stopped || (viewers == 0 && !bg) {
			delete(m.cols, c.hostID)
			c.status.State = "stopped"
			st := c.status
			m.mu.Unlock()
			m.opts.Emit(EventStatus, st)
			if m.opts.OnStop != nil {
				m.opts.OnStop(c.hostID)
			}
			return
		}
		m.mu.Unlock()

		live := viewers > 0
		if live {
			interval = m.opts.LiveInterval
		}
		wait := interval
		if err := c.sample(live, interval); err != nil {
			failures++
			if live {
				// Retry sooner while someone is looking, with backoff.
				wait = min(time.Duration(failures)*5*time.Second, 30*time.Second)
			}
			if errors.Is(err, ErrUnsupported) {
				wait = max(wait, 60*time.Second)
			}
			c.setStatus(func(s *models.MonitorStatus) { s.State, s.Error = "error", err.Error() })
			if m.opts.OnError != nil {
				m.opts.OnError(c.hostID, err)
			}
			c.failing = true
		} else {
			failures = 0
			if c.failing && m.opts.OnRecover != nil {
				m.opts.OnRecover(c.hostID)
			}
			c.failing = false
		}
		select {
		case <-time.After(wait):
		case <-c.wake:
		}
	}
}

func clientAlive(cl *sshconn.Client) bool {
	select {
	case <-cl.Done():
		return false
	default:
		return true
	}
}

func (c *collector) acquire() error {
	c.clientMu.Lock()
	ok := c.client != nil && clientAlive(c.client)
	c.clientMu.Unlock()
	if ok {
		return nil
	}
	c.closeClient()
	c.setStatus(func(s *models.MonitorStatus) {
		if s.State != "error" {
			s.State = "connecting"
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cl, shared, err := c.m.opts.Client(ctx, c.hostID)
	if err != nil {
		return err
	}
	c.clientMu.Lock()
	c.client, c.shared = cl, shared
	c.clientMu.Unlock()
	c.prev = nil
	return nil
}

func (c *collector) sample(live bool, interval time.Duration) error {
	if err := c.acquire(); err != nil {
		return err
	}
	c.clientMu.Lock()
	cl, shared := c.client, c.shared
	c.clientMu.Unlock()

	slow := !live || time.Since(c.lastSlow) >= c.m.opts.SlowEvery || c.forceSlow.Swap(false)
	script := fastScript
	if slow {
		script = slowScript
	}
	at := time.Now()
	out, err := runScript(cl, script, c.m.opts.Timeout)
	if err != nil {
		if !clientAlive(cl) {
			c.closeClient()
			return errors.New("the connection to the server was lost")
		}
		return err
	}
	r, err := parse(out, at.UnixMilli())
	if err != nil {
		return err
	}
	r.taken = at
	if r.hasSlow {
		c.lastSlow = at
		c.docker, c.services = r.docker, r.services
		c.docker.UpdatedAt, c.services.UpdatedAt = at.UnixMilli(), at.UnixMilli()
	}
	metrics := compute(c.hostID, r, c.prev)
	metrics.Docker, metrics.Services = c.docker, c.services
	c.prev = &r

	c.m.mu.Lock()
	c.latest = &metrics
	c.m.mu.Unlock()
	source := "direct"
	if shared {
		source = "session"
	}
	c.setStatus(func(s *models.MonitorStatus) {
		s.State, s.Error, s.Source, s.LastAt, s.IntervalSec = "live", "", source, metrics.At, int(interval/time.Second)
	})
	c.m.opts.Emit(EventSample, metrics)
	if c.m.opts.OnSample != nil {
		c.m.opts.OnSample(metrics)
	}
	return nil
}

// runScript runs a script with /bin/sh (not the login shell, which may be
// fish or similar) and returns its stdout.
func runScript(cl *sshconn.Client, script string, timeout time.Duration) (string, error) {
	sess, err := cl.NewSession()
	if err != nil {
		return "", fmt.Errorf("could not open a session: %w", err)
	}
	defer sess.Close()
	var stdout, stderr bytes.Buffer
	sess.Stdin = strings.NewReader(script)
	sess.Stdout, sess.Stderr = &stdout, &stderr
	done := make(chan error, 1)
	go func() { done <- sess.Run("sh -s") }()
	select {
	case err := <-done:
		// A non-zero exit from the last command (e.g. a missing tool) is
		// fine as long as output arrived.
		if err != nil && stdout.Len() == 0 {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return "", fmt.Errorf("collecting metrics failed: %s", msg)
		}
		return stdout.String(), nil
	case <-time.After(timeout):
		sess.Close()
		return "", errors.New("the server took too long to report metrics")
	}
}

var containerRef = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// ContainerAction starts, stops or restarts a Docker container.
func (m *Manager) ContainerAction(ctx context.Context, hostID, container, action string) error {
	switch action {
	case "start", "stop", "restart":
	default:
		return fmt.Errorf("unknown container action %q", action)
	}
	if !containerRef.MatchString(container) {
		return errors.New("invalid container name")
	}
	var cl *sshconn.Client
	m.mu.Lock()
	if c, ok := m.cols[hostID]; ok {
		c.clientMu.Lock()
		if c.client != nil && clientAlive(c.client) {
			cl = c.client
		}
		c.clientMu.Unlock()
	}
	m.mu.Unlock()
	if cl == nil {
		var shared bool
		var err error
		if cl, shared, err = m.opts.Client(ctx, hostID); err != nil {
			return err
		}
		if !shared {
			defer cl.Close()
		}
	}
	// The container reference is validated above, so it is safe to place
	// in the command.
	out, err := runScript(cl, "docker "+action+" "+container+" 2>&1\n", 90*time.Second)
	if err != nil {
		return err
	}
	if strings.Contains(out, "Error") || strings.Contains(out, "permission denied") {
		return errors.New(strings.TrimSpace(out))
	}
	m.mu.Lock()
	if c, ok := m.cols[hostID]; ok {
		c.forceSlow.Store(true)
		c.poke()
	}
	m.mu.Unlock()
	return nil
}
