package terminal

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/aymanbagabas/go-pty"

	"remvyn/backend/models"
)

// StartLocal opens a shell on this computer in a PTY (ConPTY on Windows).
// Output and exit events are the same as for SSH sessions.
func (m *Manager) StartLocal(id string, info models.SessionInfo, cols, rows int) error {
	if !ValidID(id) {
		return errors.New("invalid session id")
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	p, err := pty.New()
	if err != nil {
		return fmt.Errorf("open terminal: %w", err)
	}
	if err := p.Resize(cols, rows); err != nil {
		p.Close()
		return fmt.Errorf("open terminal: %w", err)
	}
	name, args := LocalShell()
	cmd := p.Command(name, args...)
	cmd.Env = localEnv()
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	if err := cmd.Start(); err != nil {
		p.Close()
		return fmt.Errorf("start %s: %w", name, err)
	}
	afterStart(p)

	s := &session{
		info: info, stdin: p,
		resize: p.Resize,
		end: func() {
			// Closing the PTY hangs up the shell; kill it in case it ignores that.
			_ = p.Close()
			_ = cmd.Process.Kill()
		},
	}
	if err := m.add(id, s); err != nil {
		s.close()
		_ = cmd.Wait()
		return err
	}

	var pumps sync.WaitGroup
	pumps.Add(1)
	go m.pump(id, p, &pumps)

	go func() {
		err := cmd.Wait()
		waitTimeout(&pumps, drainTimeout)
		exit := models.SessionExit{ID: id}
		var ee *exec.ExitError
		switch {
		case err == nil:
		case errors.As(err, &ee):
			exit.ExitCode = ee.ExitCode()
		default:
			exit.ExitCode = -1
			exit.Error = err.Error()
		}
		m.remove(id)
		m.emit(ExitEvent(id), exit)
	}()
	return nil
}

func localEnv() []string {
	env := append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=Remvyn")
	// Apps started from the Dock or a desktop launcher often have no locale,
	// which makes shells mangle non-ASCII input.
	if os.Getenv("LANG") == "" && os.Getenv("LC_ALL") == "" && os.Getenv("LC_CTYPE") == "" {
		env = append(env, "LANG=en_US.UTF-8")
	}
	return env
}
