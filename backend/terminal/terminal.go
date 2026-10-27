// Package terminal runs interactive shell sessions, over SSH or on this
// computer, and streams their output to the UI.
package terminal

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

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
	stdin  io.Writer
	resize func(cols, rows int) error
	end    func()

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

// Start opens a PTY shell on client. The caller must already be subscribed to
// DataEvent(id) so the login banner is not missed. The manager takes
// ownership of client and closes it when the session ends.
func (m *Manager) Start(id string, client *sshconn.Client, info models.SessionInfo, cols, rows int, startup string) error {
	if !ValidID(id) {
		client.Close()
		return errors.New("invalid session id")
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		return fmt.Errorf("open session: %w", err)
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 115200,
		ssh.TTY_OP_OSPEED: 115200,
	}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		sess.Close()
		client.Close()
		return fmt.Errorf("request terminal: %w", err)
	}
	// Servers commonly reject environment variables; failure is harmless.
	_ = sess.Setenv("COLORTERM", "truecolor")
	_ = sess.Setenv("TERM_PROGRAM", "Remvyn")

	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return err
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return err
	}
	if err := sess.Shell(); err != nil {
		sess.Close()
		client.Close()
		return fmt.Errorf("start shell: %w", err)
	}

	s := &session{
		info: info, client: client, stdin: stdin,
		resize: func(cols, rows int) error { return sess.WindowChange(rows, cols) },
		end: func() {
			_ = sess.Close()
			_ = client.Close()
		},
	}
	if err := m.add(id, s); err != nil {
		s.close()
		return err
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
	return nil
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

// Close ends a session. The exit event still fires.
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
