package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Commands typed in each host's terminal, for inline suggestions. A host's
// whole list is one sealed blob, so nothing about it is readable without the
// vault key. History stays on this device; it is not synced.
const historySchema = `
CREATE TABLE IF NOT EXISTS command_history (
	host_id    TEXT PRIMARY KEY,
	sealed     BLOB NOT NULL,
	updated_at INTEGER NOT NULL
);
`

const (
	// MaxHistory is how many distinct commands are kept per host.
	MaxHistory = 2000
	// MaxCommandLen skips pasted scripts and other huge lines.
	MaxCommandLen = 1000
)

// loadHistory reads a host's commands, most recent first. A list sealed with
// another key (after joining a synced vault) reads as empty and is replaced
// on the next write.
func (s *Store) loadHistory(ctx context.Context, hostID string) ([]string, error) {
	var sealed []byte
	err := s.db.QueryRowContext(ctx, `SELECT sealed FROM command_history WHERE host_id = ?`, hostID).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var cmds []string
	if err := s.openJSON(sealed, &cmds); err != nil {
		if errors.Is(err, ErrLocked) {
			return nil, err
		}
		return []string{}, nil
	}
	return cmds, nil
}

// CommandHistory returns a host's commands, most recent first.
func (s *Store) CommandHistory(ctx context.Context, hostID string) ([]string, error) {
	return s.loadHistory(ctx, hostID)
}

// AddCommand records a command as the most recent for a host, moving it to
// the front if it was already there.
func (s *Store) AddCommand(ctx context.Context, hostID, cmd string) error {
	cmd = strings.TrimRight(cmd, " \t")
	if hostID == "" || strings.TrimSpace(cmd) == "" || len(cmd) > MaxCommandLen || strings.ContainsAny(cmd, "\r\n") {
		return nil
	}
	return s.editHistory(ctx, hostID, func(cmds []string) []string {
		out := make([]string, 0, len(cmds)+1)
		out = append(out, cmd)
		for _, c := range cmds {
			if c != cmd && len(out) < MaxHistory {
				out = append(out, c)
			}
		}
		return out
	})
}

// ForgetCommand removes one command from a host's history.
func (s *Store) ForgetCommand(ctx context.Context, hostID, cmd string) error {
	return s.editHistory(ctx, hostID, func(cmds []string) []string {
		out := cmds[:0]
		for _, c := range cmds {
			if c != cmd {
				out = append(out, c)
			}
		}
		return out
	})
}

func (s *Store) editHistory(ctx context.Context, hostID string, edit func([]string) []string) error {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	cmds, err := s.loadHistory(ctx, hostID)
	if err != nil {
		return err
	}
	cmds = edit(cmds)
	if len(cmds) == 0 {
		_, err := s.db.ExecContext(ctx, `DELETE FROM command_history WHERE host_id = ?`, hostID)
		return err
	}
	sealed, err := s.sealJSON(cmds)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO command_history(host_id, sealed, updated_at) VALUES(?,?,?)
		ON CONFLICT(host_id) DO UPDATE SET sealed = excluded.sealed, updated_at = excluded.updated_at`, hostID, sealed, now())
	return err
}

// ClearCommandHistory deletes the history of every host.
func (s *Store) ClearCommandHistory(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM command_history`)
	return err
}
