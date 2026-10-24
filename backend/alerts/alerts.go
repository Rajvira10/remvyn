// Package alerts evaluates alert rules against monitoring samples, host
// reachability and uptime results, and records fired and resolved alerts.
package alerts

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"remvyn/backend/models"
)

// EventAlert is emitted with a models.AlertEvent when an alert fires or resolves.
const EventAlert = "alert:event"

// Store persists alert events.
type Store interface {
	InsertAlertEvent(ctx context.Context, e models.AlertEvent) (int64, error)
	ResolveAlertEvent(ctx context.Context, id, at int64) error
}

// Options configure an Engine.
type Options struct {
	Store  Store
	Emit   func(event string, data any)
	Notify func(title, body string)
	// HostLabel names a host for messages.
	HostLabel func(hostID string) string
	Now       func() time.Time
}

// Engine tracks each (rule, subject) pair: how long it has been breaching
// and whether it is currently firing.
type Engine struct {
	o Options

	mu      sync.Mutex
	enabled bool
	rules   []models.AlertRule
	state   map[string]*pairState
	// pending holds notifications and events produced while mu is held.
	// They are delivered after unlocking, so a slow notifier (for example
	// a permission prompt) can never block evaluation.
	pending []func()
}

// locked runs f under the lock, then delivers queued notifications.
func (e *Engine) locked(f func()) {
	e.mu.Lock()
	f()
	p := e.pending
	e.pending = nil
	e.mu.Unlock()
	for _, fn := range p {
		fn()
	}
}

func (e *Engine) deliverLocked(event string, ev models.AlertEvent, title, body string) {
	e.pending = append(e.pending, func() {
		e.o.Emit(event, ev)
		e.o.Notify(title, body)
	})
}

type pairState struct {
	ruleID    string
	subjectID string
	since     time.Time // zero when not breaching
	firing    bool
	event     models.AlertEvent
}

// New creates an Engine (disabled until SetEnabled(true)).
func New(o Options) *Engine {
	if o.Emit == nil {
		o.Emit = func(string, any) {}
	}
	if o.Notify == nil {
		o.Notify = func(string, string) {}
	}
	if o.HostLabel == nil {
		o.HostLabel = func(id string) string { return id }
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Engine{o: o, state: map[string]*pairState{}}
}

// SetEnabled turns evaluation on or off. Turning it off resolves firing alerts.
func (e *Engine) SetEnabled(on bool) {
	e.locked(func() {
		e.enabled = on
		if !on {
			e.resolveWhereLocked(func(*pairState) bool { return true }, "alerts were turned off")
		}
	})
}

// SetRules replaces the rule set. Alerts of removed, disabled or changed
// rules are resolved.
func (e *Engine) SetRules(rules []models.AlertRule) {
	e.locked(func() {
		keep := map[string]models.AlertRule{}
		for _, r := range rules {
			if r.Enabled {
				keep[r.ID] = r
			}
		}
		old := map[string]models.AlertRule{}
		for _, r := range e.rules {
			old[r.ID] = r
		}
		e.resolveWhereLocked(func(p *pairState) bool {
			n, ok := keep[p.ruleID]
			return !ok || n != old[p.ruleID]
		}, "the rule was changed")
		e.rules = nil
		for _, r := range rules {
			if r.Enabled {
				e.rules = append(e.rules, r)
			}
		}
	})
}

// ForgetHost resolves alerts for a host that is no longer monitored.
func (e *Engine) ForgetHost(hostID string) {
	e.locked(func() {
		e.resolveWhereLocked(func(p *pairState) bool { return p.subjectID == hostID }, "monitoring stopped")
	})
}

func (e *Engine) resolveWhereLocked(match func(*pairState) bool, why string) {
	now := e.o.Now()
	for k, p := range e.state {
		if !match(p) {
			continue
		}
		if p.firing {
			e.resolveLocked(p, now, why)
		}
		delete(e.state, k)
	}
}

var metricLabels = map[string]string{
	models.AlertCPU: "CPU", models.AlertMemory: "Memory", models.AlertSwap: "Swap", models.AlertDisk: "Disk",
	models.AlertLoad: "Load", models.AlertServices: "Failed services", models.AlertContainers: "Stopped containers",
	models.AlertUnreachable: "Host unreachable", models.AlertUptime: "Uptime check down",
}

// MetricLabel is a human name for an alert metric.
func MetricLabel(m string) string {
	if l, ok := metricLabels[m]; ok {
		return l
	}
	return m
}

func ruleName(r models.AlertRule) string {
	if r.Name != "" {
		return r.Name
	}
	return MetricLabel(r.Metric)
}

func fmtValue(metric string, v float64) string {
	switch metric {
	case models.AlertCPU, models.AlertMemory, models.AlertSwap, models.AlertDisk:
		return fmt.Sprintf("%.0f%%", v)
	case models.AlertLoad:
		return fmt.Sprintf("%.2f", v)
	}
	return fmt.Sprintf("%.0f", v)
}

func fmtDuration(sec int) string {
	switch {
	case sec <= 0:
		return ""
	case sec%3600 == 0:
		return fmt.Sprintf(" for %dh", sec/3600)
	case sec%60 == 0:
		return fmt.Sprintf(" for %dm", sec/60)
	}
	return fmt.Sprintf(" for %ds", sec)
}

// observe updates one (rule, subject) pair.
func (e *Engine) observeLocked(r models.AlertRule, subjectID, subject string, value float64, breach bool, msg string) {
	now := e.o.Now()
	key := r.ID + "|" + subjectID
	p := e.state[key]
	if p == nil {
		p = &pairState{ruleID: r.ID, subjectID: subjectID}
		e.state[key] = p
	}
	if !breach {
		p.since = time.Time{}
		if p.firing {
			e.resolveLocked(p, now, "")
		}
		return
	}
	if p.since.IsZero() {
		p.since = now
	}
	if p.firing || now.Sub(p.since) < time.Duration(r.DurationSec)*time.Second {
		return
	}
	p.firing = true
	ev := models.AlertEvent{RuleID: r.ID, RuleName: ruleName(r), Metric: r.Metric, Subject: subject, SubjectID: subjectID,
		Value: value, Threshold: r.Threshold, Message: msg, StartedAt: p.since.UnixMilli()}
	if e.o.Store != nil {
		if id, err := e.o.Store.InsertAlertEvent(context.Background(), ev); err == nil {
			ev.ID = id
		}
	}
	p.event = ev
	e.deliverLocked(EventAlert, ev, "⚠️ "+ev.RuleName+" · "+subject, msg)
}

func (e *Engine) resolveLocked(p *pairState, now time.Time, why string) {
	p.firing = false
	ev := p.event
	ev.ResolvedAt = now.UnixMilli()
	if e.o.Store != nil && ev.ID != 0 {
		_ = e.o.Store.ResolveAlertEvent(context.Background(), ev.ID, ev.ResolvedAt)
	}
	body := "Back to normal after " + time.Duration(ev.ResolvedAt-ev.StartedAt).Round(time.Second).String()
	if why != "" {
		body = "Closed because " + why
	}
	e.deliverLocked(EventAlert, ev, "✅ Resolved: "+ev.RuleName+" · "+ev.Subject, body)
}

func breaches(r models.AlertRule, v float64) bool {
	if r.Op == "<" {
		return v < r.Threshold
	}
	return v > r.Threshold
}

// OnSample evaluates metric rules for a host sample.
func (e *Engine) OnSample(m models.Metrics) {
	e.locked(func() {
		if !e.enabled {
			return
		}
		label := e.o.HostLabel(m.HostID)
		for _, r := range e.rules {
			if r.HostID != "" && r.HostID != m.HostID {
				continue
			}
			var v float64
			switch r.Metric {
			case models.AlertCPU:
				v = m.CPUPct
			case models.AlertMemory:
				v = m.MemPct
			case models.AlertSwap:
				v = m.SwapPct
			case models.AlertDisk:
				v = m.DiskPct
			case models.AlertLoad:
				v = m.Load1
			case models.AlertServices:
				if !m.Services.Available {
					continue
				}
				v = float64(m.Services.Failed)
			case models.AlertContainers:
				if !m.Docker.Available {
					continue
				}
				for _, c := range m.Docker.Containers {
					if c.State != "running" {
						v++
					}
				}
			case models.AlertUnreachable:
				// A sample means the host is reachable.
				e.observeLocked(r, m.HostID, label, 0, false, "")
				continue
			default:
				continue
			}
			dir := "above"
			if r.Op == "<" {
				dir = "below"
			}
			msg := fmt.Sprintf("%s is %s on %s (%s %s%s)", MetricLabel(r.Metric), fmtValue(r.Metric, v), label, dir,
				fmtValue(r.Metric, r.Threshold), fmtDuration(r.DurationSec))
			if r.Metric == models.AlertServices {
				var failed []string
				for _, s := range m.Services.Items {
					if s.Active == "failed" {
						failed = append(failed, s.Name)
					}
				}
				msg = fmt.Sprintf("%d failed service(s) on %s: %s", int(v), label, strings.Join(failed, ", "))
			}
			if r.Metric == models.AlertContainers {
				var stopped []string
				for _, c := range m.Docker.Containers {
					if c.State != "running" {
						stopped = append(stopped, c.Name)
					}
				}
				msg = fmt.Sprintf("%d stopped container(s) on %s: %s", int(v), label, strings.Join(stopped, ", "))
			}
			e.observeLocked(r, m.HostID, label, v, breaches(r, v), msg)
		}
	})
}

// OnHostError records that a host could not be monitored.
func (e *Engine) OnHostError(hostID string, err error) {
	e.locked(func() {
		if !e.enabled {
			return
		}
		label := e.o.HostLabel(hostID)
		for _, r := range e.rules {
			if r.Metric != models.AlertUnreachable || (r.HostID != "" && r.HostID != hostID) {
				continue
			}
			e.observeLocked(r, hostID, label, 1, true, fmt.Sprintf("%s can't be reached%s: %v", label, fmtDuration(r.DurationSec), err))
		}
	})
}

// OnUptime evaluates uptime rules for a probe result.
func (e *Engine) OnUptime(c models.UptimeCheck, res models.UptimeResult) {
	e.locked(func() {
		if !e.enabled {
			return
		}
		for _, r := range e.rules {
			if r.Metric != models.AlertUptime || (r.CheckID != "" && r.CheckID != c.ID) {
				continue
			}
			msg := fmt.Sprintf("%s is down%s: %s", c.Name, fmtDuration(r.DurationSec), res.Error)
			e.observeLocked(r, c.ID, c.Name, 0, !res.OK, msg)
		}
	})
}
