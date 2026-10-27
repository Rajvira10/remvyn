// Package terminal runs interactive shell sessions, over SSH or on this
// computer, and streams their output to the UI.
package terminal

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"remvyn/backend/models"
	"remvyn/backend/sshconn"
)

// Emitter delivers events to the UI.
type Emitter func(event string, data any)

// DataEvent returns the event name carrying base64 output for a session.
func DataEvent(id string) string { return "term:data:" + id }

// ExitEvent returns the event name emitted once when a session ends.
func ExitEvent(id string) string { return "term:exit:" + id }

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// ValidID reports whether a client-supplied session ID is acceptable.
func ValidID(id string) bool { return validID.MatchString(id) }

type session struct {
	info   models.SessionInfo
	client *sshconn.Client // nil for local shells
	// persist is the server-side session key when the shell runs in tmux.
	persist string
	stdin   io.Writer
	resize  func(cols, rows int) error
	end     func()

	writeMu   sync.Mutex
	closeOnce sync.Once
}

// Manager owns all terminal sessions.
type Manager struct {
	emit Emitter

	mu       sync.Mutex
	sessions map[string]*session
}

// New creates a Manager.
func New(emit Emitter) *Manager {
	return &Manager{emit: emit, sessions: map[string]*session{}}
}

// ClientForHost returns a live client from an open session to hostID, so
// other features (monitoring) can share the connection. The caller must not
// close it.
func (m *Manager) ClientForHost(hostID string) *sshconn.Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if s.client == nil || s.info.HostID != hostID {
			continue
		}
		select {
		case <-s.client.Done():
		default:
			return s.client
		}
	}
	return nil
}

// StartOptions tune an SSH shell.
type StartOptions struct {
	// Startup is typed into a new shell.
	Startup string
	// Persist runs the shell in a tmux session that survives disconnects.
	Persist *Persist
	// ForwardAgent asks the server for agent forwarding; the caller must
	// already serve an agent on the client.
	ForwardAgent bool
	// OnEnd runs once when the session ends.
	OnEnd func()
}

// Start opens a PTY shell on client. The caller must already be subscribed to
// DataEvent(id) so the login banner is not missed. The manager takes
// ownership of client and closes it when the session ends.
//
// With a persist key, the shell runs in a tmux session on the server that
// survives disconnects, if the server has tmux; the returned info says
// whether it does. The startup command only runs in a new session.
func (m *Manager) Start(id string, client *sshconn.Client, info models.SessionInfo, cols, rows int, o StartOptions) (models.SessionInfo, error) {
	p, startup := o.Persist, o.Startup
	if o.OnEnd != nil {
		o.OnEnd = sync.OnceFunc(o.OnEnd)
		defer func() {
			if _, err := m.get(id); err != nil {
				o.OnEnd() // never started
			}
		}()
	}
	if !ValidID(id) {
		client.Close()
		return info, errors.New("invalid session id")
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	var history string
	command := ""
	if p != nil && ValidID(p.Key) {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		st := probeTmux(ctx, client)
		if st.ok {
			info.Detached = st.lookup(p.Key)
			info.Persistent = true
			info.Resumed = st.exists
			command = attachCommand(p.Key, cols, rows, o.ForwardAgent)
			if st.exists {
				startup = ""
				if p.Restore && st.history > 0 {
					history = captureHistory(ctx, client, p.Key, 5000)
				}
			}
		} else {
			info.PersistNote = st.note
		}
		cancel()
	}
	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		return info, fmt.Errorf("open session: %w", err)
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 115200,
		ssh.TTY_OP_OSPEED: 115200,
	}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		sess.Close()
		client.Close()
		return info, fmt.Errorf("request terminal: %w", err)
	}
	if o.ForwardAgent {
		// Servers may refuse (AllowAgentForwarding no); the shell still works.
		info.AgentForwarding = agent.RequestAgentForwarding(sess) == nil
	}
	// Servers commonly reject environment variables; failure is harmless.
	_ = sess.Setenv("COLORTERM", "truecolor")
	_ = sess.Setenv("TERM_PROGRAM", "Remvyn")

	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return info, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return info, err
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return info, err
	}
	if command != "" {
		err = sess.Start(command)
	} else {
		err = sess.Shell()
	}
	if err != nil {
		sess.Close()
		client.Close()
		return info, fmt.Errorf("start shell: %w", err)
	}

	s := &session{
		info: info, client: client, stdin: stdin,
		resize: func(cols, rows int) error { return sess.WindowChange(rows, cols) },
		end: func() {
			_ = sess.Close()
			_ = client.Close()
			if o.OnEnd != nil {
				o.OnEnd()
			}
		},
	}
	if info.Persistent {
		s.persist = p.Key
	}
	if err := m.add(id, s); err != nil {
		s.close()
		return info, err
	}
	if history != "" {
		// Push the replayed lines into scrollback before tmux redraws the screen.
		m.emit(DataEvent(id), base64.StdEncoding.EncodeToString([]byte(history+strings.Repeat("\r\n", rows))))
	}

	var pumps sync.WaitGroup
	pumps.Add(2)
	go m.pump(id, stdout, &pumps)
	go m.pump(id, stderr, &pumps)

	go func() {
		err := sess.Wait()
		// Let any buffered output reach the UI before announcing the exit.
		waitTimeout(&pumps, 2*time.Second)
		exit := models.SessionExit{ID: id}
		var ee *ssh.ExitError
		var missing *ssh.ExitMissingError
		switch {
		case err == nil:
		case errors.As(err, &ee):
			exit.ExitCode = ee.ExitStatus()
		case errors.As(err, &missing):
			exit.ExitCode = -1
			exit.Error = "connection closed"
		default:
			exit.ExitCode = -1
			exit.Error = err.Error()
		}
		m.remove(id)
		m.emit(ExitEvent(id), exit)
	}()

	if startup != "" {
		_ = m.Write(id, startup+"\n")
	}
	return info, nil
}

func (m *Manager) add(id string, s *session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, dup := m.sessions[id]; dup {
		return errors.New("session id already in use")
	}
	m.sessions[id] = s
	return nil
}

func waitTimeout(wg *sync.WaitGroup, d time.Duration) {
	ch := make(chan struct{})
	go func() { wg.Wait(); close(ch) }()
	select {
	case <-ch:
	case <-time.After(d):
	}
}

// pump forwards output. Reads are up to 32 KiB, which naturally batches
// bursts (e.g. `cat` of a large file) into few events.
func (m *Manager) pump(id string, r io.Reader, wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			m.emit(DataEvent(id), base64.StdEncoding.EncodeToString(buf[:n]))
		}
		if err != nil {
			return
		}
	}
}

func (m *Manager) get(id string) (*session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, errors.New("session is not running")
	}
	return s, nil
}

func (m *Manager) remove(id string) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if ok {
		s.close()
	}
}

func (s *session) close() {
	s.closeOnce.Do(s.end)
}

// Client returns the connection behind a running session, for side commands
// such as reading shell history. The caller must not close it.
func (m *Manager) Client(id string) (*sshconn.Client, error) {
	s, err := m.get(id)
	if err != nil {
		return nil, err
	}
	return s.client, nil
}

// Write sends keyboard input.
func (m *Manager) Write(id, data string) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = io.WriteString(s.stdin, data)
	return err
}

// Resize updates the PTY size.
func (m *Manager) Resize(id string, cols, rows int) error {
	if cols <= 0 || rows <= 0 || cols > 2000 || rows > 1000 {
		return nil
	}
	s, err := m.get(id)
	if err != nil {
		return err
	}
	return s.resize(cols, rows)
}

// End closes a session and, if it runs in tmux, ends the server-side session
// too, so it doesn't linger. The exit event still fires.
func (m *Manager) End(id string) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok {
		return
	}
	if s.persist != "" {
		_ = KillServerSession(s.client, s.persist)
	}
	s.close()
}

// Close ends a session; a tmux session on the server keeps running, so the
// next connection can resume it. The exit event still fires.
func (m *Manager) Close(id string) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	if ok {
		s.close()
	}
}

// CloseAll ends every session (shutdown).
func (m *Manager) CloseAll() {
	m.mu.Lock()
	all := make([]*session, 0, len(m.sessions))
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.mu.Unlock()
	for _, s := range all {
		s.close()
	}
}

// List returns open sessions.
func (m *Manager) List() []models.SessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]models.SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s.info)
	}
	return out
}
