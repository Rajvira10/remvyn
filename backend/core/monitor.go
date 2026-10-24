package core

import (
	"context"
	"errors"
	"slices"
	"time"

	"remvyn/backend/alerts"
	"remvyn/backend/models"
	"remvyn/backend/monitor"
	"remvyn/backend/sshconn"
	"remvyn/backend/storage"
	"remvyn/backend/uptime"
)

// CodeProRequired marks errors for Remvyn Pro features used without Pro.
const CodeProRequired = "pro_required"

func (c *Core) initMonitoring(liveInterval, slowEvery time.Duration) {
	c.alerts = alerts.New(alerts.Options{
		Store: c.store,
		Emit:  c.emit,
		Notify: func(title, body string) {
			if c.notify != nil && c.Settings().AlertNotifications {
				c.notify(title, body)
			}
		},
		HostLabel: c.hostLabel,
	})
	c.monitor = monitor.New(monitor.Options{
		LiveInterval: liveInterval,
		SlowEvery:    slowEvery,
		Emit:         c.emit,
		Client:       c.monitorClient,
		OnSample: func(m models.Metrics) {
			if c.pro() {
				_ = c.store.RecordMetrics(context.Background(), m)
			}
			c.alerts.OnSample(m)
		},
		OnError: func(hostID string, err error) { c.alerts.OnHostError(hostID, err) },
		OnStop:  func(hostID string) { c.alerts.ForgetHost(hostID) },
	})
	c.uptime = uptime.New(func(chk models.UptimeCheck, r models.UptimeResult) {
		_ = c.store.InsertUptimeResult(context.Background(), r)
		c.emit(uptime.EventResult, r)
		c.alerts.OnUptime(chk, r)
	})
	_ = c.store.ResolveOpenAlerts(context.Background(), time.Now().UnixMilli())
}

// StartMonitoring starts background monitoring, alerts and uptime checks
// (Pro). Call once the UI is up.
func (c *Core) StartMonitoring() {
	c.applyMonitoring()
	go c.pruneLoop()
}

func (c *Core) pro() bool { return c.Settings().ProEnabled }

func (c *Core) requirePro(feature string) error {
	if c.pro() {
		return nil
	}
	return sshconn.Coded(CodeProRequired, feature+" is a Remvyn Pro feature", nil)
}

// applyMonitoring makes the running Pro services match settings.
func (c *Core) applyMonitoring() {
	st := c.Settings()
	ctx := context.Background()
	if !st.ProEnabled {
		c.monitor.SetBackground(nil, 0)
		c.alerts.SetEnabled(false)
		c.uptime.Sync(nil)
		return
	}
	// Background monitoring dials with saved credentials, which are
	// unavailable while Remvyn is locked.
	if c.store.Locked() {
		c.monitor.SetBackground(nil, 0)
	} else {
		c.monitor.SetBackground(st.MonitorHostIDs, time.Duration(st.MonitorIntervalSec)*time.Second)
	}
	if rules, err := c.store.AlertRules(ctx); err == nil {
		c.alerts.SetRules(rules)
	}
	c.alerts.SetEnabled(true)
	if checks, err := c.store.UptimeChecks(ctx); err == nil {
		c.uptime.Sync(checks)
	}
}

func (c *Core) pruneLoop() {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		days := c.Settings().MetricsRetentionDays
		if days <= 0 {
			days = 30
		}
		_ = c.store.PruneMonitoring(context.Background(), time.Now().AddDate(0, 0, -days).UnixMilli())
		select {
		case <-c.stop:
			return
		case <-t.C:
		}
	}
}

func (c *Core) hostLabel(id string) string {
	if h, err := c.store.GetHost(context.Background(), id); err == nil {
		return h.Label
	}
	return "a deleted host"
}

func (c *Core) existingHostIDs(in []string) []string {
	out := []string{}
	for _, id := range in {
		if slices.Contains(out, id) {
			continue
		}
		if _, err := c.store.GetHost(context.Background(), id); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func (c *Core) forgetMonitoredHost(id string) {
	_ = c.store.DeleteHostMetrics(context.Background(), id)
	st := c.Settings()
	if i := slices.Index(st.MonitorHostIDs, id); i >= 0 {
		st.MonitorHostIDs = slices.Delete(slices.Clone(st.MonitorHostIDs), i, i+1)
		_ = c.store.SaveSettings(context.Background(), st)
		c.mu.Lock()
		c.settings = st
		c.mu.Unlock()
		c.applyMonitoring()
	}
	c.alerts.ForgetHost(id)
	if rules, err := c.store.AlertRules(context.Background()); err == nil {
		c.alerts.SetRules(rules)
	}
}

// monitorClient shares an open terminal's connection when there is one, and
// otherwise connects without prompting (saved credentials and already
// trusted host keys only).
func (c *Core) monitorClient(ctx context.Context, hostID string) (*sshconn.Client, bool, error) {
	if cl := c.terms.ClientForHost(hostID); cl != nil {
		return cl, true, nil
	}
	cl, _, err := c.dialWith(ctx, hostID, models.ConnectOptions{}, c.quiet.Callback())
	if err == nil {
		return cl, false, nil
	}
	switch sshconn.CodeOf(err) {
	case sshconn.CodePasswordRequired:
		return nil, false, sshconn.Coded(sshconn.CodePasswordRequired, "save this host's password (or open a terminal to it) to monitor it", err)
	case sshconn.CodePassphraseRequired:
		return nil, false, sshconn.Coded(sshconn.CodePassphraseRequired, "save the key's passphrase (or open a terminal to the host) to monitor it", err)
	case sshconn.CodeHostKeyRejected:
		return nil, false, sshconn.Coded(sshconn.CodeHostKeyRejected, "open a terminal to this host once to verify its host key, then monitoring can connect", err)
	}
	return nil, false, err
}

// MonitorWatch starts live monitoring of a host for a viewer (free tier).
func (c *Core) MonitorWatch(hostID string) (models.MonitorStatus, error) {
	if _, err := c.store.GetHost(context.Background(), hostID); err != nil {
		return models.MonitorStatus{}, errors.New("host not found")
	}
	return c.monitor.Watch(hostID), nil
}

// MonitorUnwatch removes a viewer.
func (c *Core) MonitorUnwatch(hostID string) { c.monitor.Unwatch(hostID) }

// MonitorStatuses lists collectors.
func (c *Core) MonitorStatuses() []models.MonitorStatus { return c.monitor.Statuses() }

// MonitorLatest returns the newest sample for a host, or nil.
func (c *Core) MonitorLatest(hostID string) *models.Metrics {
	if m, ok := c.monitor.Latest(hostID); ok {
		return &m
	}
	return nil
}

// ContainerAction starts, stops or restarts a Docker container.
func (c *Core) ContainerAction(hostID, container, action string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return c.monitor.ContainerAction(ctx, hostID, container, action)
}

// MetricsHistory returns aggregated history for the last rangeMinutes (Pro).
func (c *Core) MetricsHistory(hostID string, rangeMinutes int) ([]models.MetricPoint, error) {
	if err := c.requirePro("Historical metrics"); err != nil {
		return []models.MetricPoint{}, err
	}
	if rangeMinutes <= 0 || rangeMinutes > 366*24*60 {
		rangeMinutes = 60
	}
	to := time.Now().UnixMilli()
	from := to - int64(rangeMinutes)*60000
	return c.store.MetricsHistory(context.Background(), hostID, from, to, 240)
}

// AlertRules lists rules.
func (c *Core) AlertRules() ([]models.AlertRule, error) {
	return c.store.AlertRules(context.Background())
}

// SaveAlertRule creates or updates a rule (Pro).
func (c *Core) SaveAlertRule(r models.AlertRule) (models.AlertRule, error) {
	if err := c.requirePro("Custom alerts"); err != nil {
		return r, err
	}
	if r.Metric == models.AlertUptime && r.CheckID != "" {
		checks, _ := c.store.UptimeChecks(context.Background())
		if !slices.ContainsFunc(checks, func(k models.UptimeCheck) bool { return k.ID == r.CheckID }) {
			return r, errors.New("the selected uptime check no longer exists")
		}
	}
	out, err := c.store.SaveAlertRule(context.Background(), r)
	if err == nil {
		c.applyMonitoring()
	}
	return out, err
}

// DeleteAlertRule removes a rule.
func (c *Core) DeleteAlertRule(id string) error {
	err := c.store.DeleteAlertRule(context.Background(), id)
	if err == nil {
		c.applyMonitoring()
	}
	return err
}

// AlertEvents returns recent alerts.
func (c *Core) AlertEvents() ([]models.AlertEvent, error) {
	return c.store.AlertEvents(context.Background(), 200)
}

// UptimeSummaries returns every check with its state and availability.
func (c *Core) UptimeSummaries() ([]models.UptimeSummary, error) {
	ctx := context.Background()
	checks, err := c.store.UptimeChecks(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := []models.UptimeSummary{}
	for _, chk := range checks {
		s := models.UptimeSummary{Check: chk}
		s.Uptime24h, _ = c.store.UptimeRatio(ctx, chk.ID, now.Add(-24*time.Hour).UnixMilli())
		s.Uptime7d, _ = c.store.UptimeRatio(ctx, chk.ID, now.Add(-7*24*time.Hour).UnixMilli())
		s.Uptime30d, _ = c.store.UptimeRatio(ctx, chk.ID, now.Add(-30*24*time.Hour).UnixMilli())
		s.Recent, _ = c.store.RecentUptime(ctx, chk.ID, 60)
		if n := len(s.Recent); n > 0 {
			last := s.Recent[n-1]
			up := last.OK
			s.Up, s.LastAt, s.LatencyMs, s.LastError = &up, last.At, last.LatencyMs, last.Error
			if !up {
				s.DownSince, _ = c.store.DownSince(ctx, chk.ID)
			}
		}
		out = append(out, s)
	}
	return out, nil
}

// SaveUptimeCheck creates or updates a check (Pro).
func (c *Core) SaveUptimeCheck(chk models.UptimeCheck) (models.UptimeCheck, error) {
	if err := c.requirePro("Uptime monitoring"); err != nil {
		return chk, err
	}
	out, err := c.store.SaveUptimeCheck(context.Background(), chk)
	if err == nil {
		c.applyMonitoring()
	}
	return out, err
}

// DeleteUptimeCheck removes a check.
func (c *Core) DeleteUptimeCheck(id string) error {
	err := c.store.DeleteUptimeCheck(context.Background(), id)
	if err == nil {
		c.applyMonitoring()
	}
	return err
}

// TestUptimeCheck probes a check configuration once without saving it.
func (c *Core) TestUptimeCheck(chk models.UptimeCheck) (models.UptimeResult, error) {
	if err := storage.ValidateUptimeCheck(&chk); err != nil {
		return models.UptimeResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return uptime.Probe(ctx, chk), nil
}
