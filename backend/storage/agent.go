package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"remvyn/backend/ids"
	"remvyn/backend/models"
)

// The agent activity log. What the agent ran and saw can include secrets, so
// the summary and output are sealed with the vault like passwords.
const agentSchema = `
CREATE TABLE IF NOT EXISTS agent_log (
	id          TEXT PRIMARY KEY,
	at          INTEGER NOT NULL,
	client      TEXT NOT NULL,
	host_id     TEXT NOT NULL,
	host_label  TEXT NOT NULL,
	tool        TEXT NOT NULL,
	status      TEXT NOT NULL,
	approval    TEXT NOT NULL,
	exit_code   INTEGER,
	duration_ms INTEGER NOT NULL,
	sealed      BLOB
);
CREATE INDEX IF NOT EXISTS agent_log_at ON agent_log(at);
`

// AgentLogKeep is how many entries are kept.
const AgentLogKeep = 1000

type agentSecret struct {
	Summary string `json:"s"`
	Output  string `json:"o"`
}

// InsertAgentLog records an agent action and trims old entries. If the vault
// is locked the details are dropped but the entry is still kept.
func (s *Store) InsertAgentLog(ctx context.Context, e models.AgentLogEntry) (models.AgentLogEntry, error) {
	if e.ID == "" {
		e.ID = ids.New()
	}
	raw, _ := json.Marshal(agentSecret{Summary: e.Summary, Output: e.Output})
	sealed, err := s.seal(string(raw))
	if err != nil {
		sealed = nil
	}
	var exit sql.NullInt64
	if e.ExitCode != nil {
		exit = sql.NullInt64{Int64: int64(*e.ExitCode), Valid: true}
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO agent_log(id, at, client, host_id, host_label, tool, status, approval, exit_code, duration_ms, sealed)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.At, e.Client, e.HostID, e.HostLabel, e.Tool, e.Status, e.Approval, exit, e.DurationMs, sealed)
	if err != nil {
		return e, err
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM agent_log WHERE id NOT IN (SELECT id FROM agent_log ORDER BY at DESC LIMIT ?)`, AgentLogKeep)
	return e, nil
}

// ListAgentLog returns the newest entries first. It needs the vault unlocked.
func (s *Store) ListAgentLog(ctx context.Context, limit int) ([]models.AgentLogEntry, error) {
	if s.Locked() {
		return nil, ErrLocked
	}
	if limit <= 0 || limit > AgentLogKeep {
		limit = AgentLogKeep
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, at, client, host_id, host_label, tool, status, approval, exit_code, duration_ms, sealed
		FROM agent_log ORDER BY at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.AgentLogEntry{}
	for rows.Next() {
		var e models.AgentLogEntry
		var exit sql.NullInt64
		var sealed []byte
		if err := rows.Scan(&e.ID, &e.At, &e.Client, &e.HostID, &e.HostLabel, &e.Tool, &e.Status, &e.Approval, &exit, &e.DurationMs, &sealed); err != nil {
			return nil, err
		}
		if exit.Valid {
			code := int(exit.Int64)
			e.ExitCode = &code
		}
		if raw, err := s.open(sealed); err == nil && raw != "" {
			var sec agentSecret
			if json.Unmarshal([]byte(raw), &sec) == nil {
				e.Summary, e.Output = sec.Summary, sec.Output
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ClearAgentLog deletes the activity log.
func (s *Store) ClearAgentLog(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agent_log`)
	return err
}
