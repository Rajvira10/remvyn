package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"flux/backend/ids"
	"flux/backend/models"
)

const monitorSchema = `
CREATE TABLE IF NOT EXISTS metrics_minute (
	host_id  TEXT NOT NULL,
	ts       INTEGER NOT NULL,
	cpu      REAL NOT NULL,
	cpu_max  REAL NOT NULL,
	mem      REAL NOT NULL,
	swap     REAL NOT NULL,
	disk     REAL NOT NULL,
	load1    REAL NOT NULL,
	rx       REAL NOT NULL,
	tx       REAL NOT NULL,
	samples  INTEGER NOT NULL,
	PRIMARY KEY (host_id, ts)
);
CREATE TABLE IF NOT EXISTS alert_rules (
	id           TEXT PRIMARY KEY,
	name         TEXT NOT NULL,
	metric       TEXT NOT NULL,
	host_id      TEXT NOT NULL DEFAULT '',
	check_id     TEXT NOT NULL DEFAULT '',
	op           TEXT NOT NULL,
	threshold    REAL NOT NULL,
	duration_sec INTEGER NOT NULL,
	enabled      INTEGER NOT NULL,
	created_at   INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS alert_events (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	rule_id     TEXT NOT NULL,
	rule_name   TEXT NOT NULL,
	metric      TEXT NOT NULL,
	subject     TEXT NOT NULL,
	subject_id  TEXT NOT NULL,
	value       REAL NOT NULL,
	threshold   REAL NOT NULL,
	message     TEXT NOT NULL,
	started_at  INTEGER NOT NULL,
	resolved_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS uptime_checks (
	id           TEXT PRIMARY KEY,
	name         TEXT NOT NULL,
	kind         TEXT NOT NULL,
	target       TEXT NOT NULL,
	interval_sec INTEGER NOT NULL,
	timeout_sec  INTEGER NOT NULL,
	enabled      INTEGER NOT NULL,
	created_at   INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS uptime_results (
	check_id    TEXT NOT NULL,
	ts          INTEGER NOT NULL,
	ok          INTEGER NOT NULL,
	latency_ms  INTEGER NOT NULL,
	status_code INTEGER NOT NULL,
	error       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS uptime_results_check_ts ON uptime_results(check_id, ts);
`

// ---------------------------------------------------------------- metrics

// RecordMetrics folds a sample into its minute bucket (running average).
func (s *Store) RecordMetrics(ctx context.Context, m models.Metrics) error {
	ts := m.At / 60000 * 60000
	_, err := s.db.ExecContext(ctx, `INSERT INTO metrics_minute(host_id, ts, cpu, cpu_max, mem, swap, disk, load1, rx, tx, samples)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(host_id, ts) DO UPDATE SET
			cpu = (cpu*samples + excluded.cpu)/(samples+1),
			cpu_max = max(cpu_max, excluded.cpu),
			mem = (mem*samples + excluded.mem)/(samples+1),
			swap = (swap*samples + excluded.swap)/(samples+1),
			disk = excluded.disk,
			load1 = (load1*samples + excluded.load1)/(samples+1),
			rx = (rx*samples + excluded.rx)/(samples+1),
			tx = (tx*samples + excluded.tx)/(samples+1),
			samples = samples + 1`,
		m.HostID, ts, m.CPUPct, m.CPUPct, m.MemPct, m.SwapPct, m.DiskPct, m.Load1, m.RxRate, m.TxRate)
	return err
}

// MetricsHistory returns history between from and to (ms), aggregated into
// at most maxPoints buckets of whole minutes.
func (s *Store) MetricsHistory(ctx context.Context, hostID string, from, to int64, maxPoints int) ([]models.MetricPoint, error) {
	if maxPoints <= 0 {
		maxPoints = 300
	}
	bucket := (to - from) / int64(maxPoints)
	bucket = max(60000, bucket/60000*60000)
	rows, err := s.db.QueryContext(ctx, `SELECT (ts / ?) * ? AS b, avg(cpu), max(cpu_max), avg(mem), avg(swap), max(disk), avg(load1), avg(rx), avg(tx)
		FROM metrics_minute WHERE host_id = ? AND ts >= ? AND ts <= ? GROUP BY b ORDER BY b`, bucket, bucket, hostID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MetricPoint{}
	for rows.Next() {
		var p models.MetricPoint
		if err := rows.Scan(&p.At, &p.CPU, &p.CPUMax, &p.Mem, &p.Swap, &p.Disk, &p.Load1, &p.Rx, &p.Tx); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PruneMonitoring deletes metrics, uptime results and resolved alert events
// older than before (ms).
func (s *Store) PruneMonitoring(ctx context.Context, before int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM metrics_minute WHERE ts < ?; DELETE FROM uptime_results WHERE ts < ?;
		DELETE FROM alert_events WHERE resolved_at != 0 AND resolved_at < ?;`, before, before, before)
	return err
}

// DeleteHostMetrics removes a host's history.
func (s *Store) DeleteHostMetrics(ctx context.Context, hostID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM metrics_minute WHERE host_id = ?; DELETE FROM alert_rules WHERE host_id = ?;`, hostID, hostID)
	return err
}

// ---------------------------------------------------------------- alert rules

var alertMetrics = map[string]bool{models.AlertCPU: true, models.AlertMemory: true, models.AlertSwap: true, models.AlertDisk: true,
	models.AlertLoad: true, models.AlertServices: true, models.AlertContainers: true, models.AlertUnreachable: true, models.AlertUptime: true}

// ValidateAlertRule normalises and checks a rule.
func ValidateAlertRule(r *models.AlertRule) error {
	r.Name = strings.TrimSpace(r.Name)
	if !alertMetrics[r.Metric] {
		return fmt.Errorf("unknown alert metric %q", r.Metric)
	}
	if r.Op != ">" && r.Op != "<" {
		r.Op = ">"
	}
	switch r.Metric {
	case models.AlertCPU, models.AlertMemory, models.AlertSwap, models.AlertDisk:
		if r.Threshold < 0 || r.Threshold > 100 {
			return errors.New("threshold must be a percentage between 0 and 100")
		}
	case models.AlertLoad, models.AlertServices, models.AlertContainers:
		if r.Threshold < 0 {
			return errors.New("threshold can't be negative")
		}
	case models.AlertUnreachable, models.AlertUptime:
		r.Op, r.Threshold = ">", 0
	}
	if r.Metric != models.AlertUptime {
		r.CheckID = ""
	} else {
		r.HostID = ""
	}
	if r.DurationSec < 0 || r.DurationSec > 86400 {
		return errors.New("duration must be between 0 seconds and 24 hours")
	}
	return nil
}

func scanRule(row interface{ Scan(...any) error }) (models.AlertRule, error) {
	var r models.AlertRule
	err := row.Scan(&r.ID, &r.Name, &r.Metric, &r.HostID, &r.CheckID, &r.Op, &r.Threshold, &r.DurationSec, &r.Enabled, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

const ruleCols = `id, name, metric, host_id, check_id, op, threshold, duration_sec, enabled, created_at`

// AlertRules lists rules.
func (s *Store) AlertRules(ctx context.Context) ([]models.AlertRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+ruleCols+` FROM alert_rules ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.AlertRule{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SaveAlertRule creates or updates a rule.
func (s *Store) SaveAlertRule(ctx context.Context, r models.AlertRule) (models.AlertRule, error) {
	if err := ValidateAlertRule(&r); err != nil {
		return r, err
	}
	if r.HostID != "" {
		if _, err := s.GetHost(ctx, r.HostID); err != nil {
			return r, errors.New("the selected host no longer exists")
		}
	}
	if r.ID == "" {
		r.ID, r.CreatedAt = ids.New(), now()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO alert_rules(`+ruleCols+`) VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, metric=excluded.metric, host_id=excluded.host_id, check_id=excluded.check_id,
		op=excluded.op, threshold=excluded.threshold, duration_sec=excluded.duration_sec, enabled=excluded.enabled`,
		r.ID, r.Name, r.Metric, r.HostID, r.CheckID, r.Op, r.Threshold, r.DurationSec, r.Enabled, r.CreatedAt)
	return r, err
}

// DeleteAlertRule removes a rule.
func (s *Store) DeleteAlertRule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE id = ?`, id)
	return err
}

// ---------------------------------------------------------------- alert events

// InsertAlertEvent records a fired alert and returns its ID.
func (s *Store) InsertAlertEvent(ctx context.Context, e models.AlertEvent) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO alert_events(rule_id, rule_name, metric, subject, subject_id, value, threshold, message, started_at)
		VALUES(?,?,?,?,?,?,?,?,?)`, e.RuleID, e.RuleName, e.Metric, e.Subject, e.SubjectID, e.Value, e.Threshold, e.Message, e.StartedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ResolveAlertEvent marks an event resolved.
func (s *Store) ResolveAlertEvent(ctx context.Context, id, at int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE alert_events SET resolved_at = ? WHERE id = ? AND resolved_at = 0`, at, id)
	return err
}

// ResolveOpenAlerts closes events left open by a previous run (their state
// was lost when Flux quit).
func (s *Store) ResolveOpenAlerts(ctx context.Context, at int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE alert_events SET resolved_at = ?, message = message || ' (closed when Flux restarted)' WHERE resolved_at = 0`, at)
	return err
}

// AlertEvents returns recent events, open ones first.
func (s *Store) AlertEvents(ctx context.Context, limit int) ([]models.AlertEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, rule_id, rule_name, metric, subject, subject_id, value, threshold, message, started_at, resolved_at
		FROM alert_events ORDER BY resolved_at != 0, started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.AlertEvent{}
	for rows.Next() {
		var e models.AlertEvent
		if err := rows.Scan(&e.ID, &e.RuleID, &e.RuleName, &e.Metric, &e.Subject, &e.SubjectID, &e.Value, &e.Threshold, &e.Message, &e.StartedAt, &e.ResolvedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- uptime

// ValidateUptimeCheck normalises and checks an uptime check.
func ValidateUptimeCheck(c *models.UptimeCheck) error {
	c.Name = strings.TrimSpace(c.Name)
	c.Target = strings.TrimSpace(c.Target)
	switch c.Kind {
	case models.CheckHTTP:
		if !strings.Contains(c.Target, "://") {
			c.Target = "https://" + c.Target
		}
		u, err := url.Parse(c.Target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("enter a URL such as https://example.com/health")
		}
	case models.CheckTCP:
		host, port, err := net.SplitHostPort(c.Target)
		if err != nil || host == "" || port == "" {
			return errors.New("enter a host and port such as db.example.com:5432")
		}
	default:
		return fmt.Errorf("unknown check type %q", c.Kind)
	}
	if c.Name == "" {
		c.Name = c.Target
	}
	if c.IntervalSec == 0 {
		c.IntervalSec = 60
	}
	if c.IntervalSec < 10 || c.IntervalSec > 3600 {
		return errors.New("interval must be between 10 seconds and 1 hour")
	}
	if c.TimeoutSec == 0 {
		c.TimeoutSec = 10
	}
	if c.TimeoutSec < 1 || c.TimeoutSec > 60 || c.TimeoutSec > c.IntervalSec {
		return errors.New("timeout must be between 1 and 60 seconds, and shorter than the interval")
	}
	return nil
}

const checkCols = `id, name, kind, target, interval_sec, timeout_sec, enabled, created_at`

func scanCheck(row interface{ Scan(...any) error }) (models.UptimeCheck, error) {
	var c models.UptimeCheck
	err := row.Scan(&c.ID, &c.Name, &c.Kind, &c.Target, &c.IntervalSec, &c.TimeoutSec, &c.Enabled, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// UptimeChecks lists checks.
func (s *Store) UptimeChecks(ctx context.Context) ([]models.UptimeCheck, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+checkCols+` FROM uptime_checks ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.UptimeCheck{}
	for rows.Next() {
		c, err := scanCheck(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SaveUptimeCheck creates or updates a check.
func (s *Store) SaveUptimeCheck(ctx context.Context, c models.UptimeCheck) (models.UptimeCheck, error) {
	if err := ValidateUptimeCheck(&c); err != nil {
		return c, err
	}
	if c.ID == "" {
		c.ID, c.CreatedAt = ids.New(), now()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO uptime_checks(`+checkCols+`) VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, kind=excluded.kind, target=excluded.target,
		interval_sec=excluded.interval_sec, timeout_sec=excluded.timeout_sec, enabled=excluded.enabled`,
		c.ID, c.Name, c.Kind, c.Target, c.IntervalSec, c.TimeoutSec, c.Enabled, c.CreatedAt)
	return c, err
}

// DeleteUptimeCheck removes a check, its results and its alert rules.
func (s *Store) DeleteUptimeCheck(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM uptime_checks WHERE id = ?; DELETE FROM uptime_results WHERE check_id = ?;
		DELETE FROM alert_rules WHERE check_id = ?;`, id, id, id)
	return err
}

// InsertUptimeResult records a probe.
func (s *Store) InsertUptimeResult(ctx context.Context, r models.UptimeResult) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO uptime_results(check_id, ts, ok, latency_ms, status_code, error) VALUES(?,?,?,?,?,?)`,
		r.CheckID, r.At, r.OK, r.LatencyMs, r.StatusCode, r.Error)
	return err
}

// UptimeRatio returns the percentage of successful probes since a time, or
// -1 when there are none.
func (s *Store) UptimeRatio(ctx context.Context, checkID string, since int64) (float64, error) {
	var total, ok int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(ok), 0) FROM uptime_results WHERE check_id = ? AND ts >= ?`, checkID, since).Scan(&total, &ok)
	if err != nil || total == 0 {
		return -1, err
	}
	return float64(ok) / float64(total) * 100, nil
}

// RecentUptime returns the newest n results, oldest first.
func (s *Store) RecentUptime(ctx context.Context, checkID string, n int) ([]models.UptimeResult, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT check_id, ts, ok, latency_ms, status_code, error FROM
		(SELECT * FROM uptime_results WHERE check_id = ? ORDER BY ts DESC LIMIT ?) ORDER BY ts`, checkID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.UptimeResult{}
	for rows.Next() {
		var r models.UptimeResult
		if err := rows.Scan(&r.CheckID, &r.At, &r.OK, &r.LatencyMs, &r.StatusCode, &r.Error); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DownSince returns when the current run of failures started (0 if the
// latest probe succeeded).
func (s *Store) DownSince(ctx context.Context, checkID string) (int64, error) {
	var lastOK sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT max(ts) FROM uptime_results WHERE check_id = ? AND ok = 1`, checkID).Scan(&lastOK); err != nil {
		return 0, err
	}
	var first sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT min(ts) FROM uptime_results WHERE check_id = ? AND ok = 0 AND ts > ?`, checkID, lastOK.Int64).Scan(&first)
	return first.Int64, err
}
