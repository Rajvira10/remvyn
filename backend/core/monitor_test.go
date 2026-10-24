package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"remvyn/backend/models"
	"remvyn/backend/sshconn"
)

func monitorFixture(t *testing.T) (*fixture, *[]string, *sync.Mutex) {
	t.Helper()
	f := setup(t)
	f.core.Close()
	var mu sync.Mutex
	notes := &[]string{}
	c, err := Open(context.Background(), Options{DataDir: f.dir, Emit: f.ev.emit, PromptTimeout: 5 * time.Second,
		MonitorLiveInterval: 300 * time.Millisecond, MonitorSlowEvery: time.Second,
		Notify: func(title, body string) {
			mu.Lock()
			*notes = append(*notes, title+" | "+body)
			mu.Unlock()
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	f.core = c
	f.trustAll()
	return f, notes, &mu
}

func (f *fixture) samples(hostID string) []models.Metrics {
	f.ev.mu.Lock()
	defer f.ev.mu.Unlock()
	var out []models.Metrics
	for _, m := range f.ev.samples {
		if m.HostID == hostID {
			out = append(out, m)
		}
	}
	return out
}

func (f *fixture) monStatus(hostID string) models.MonitorStatus {
	f.ev.mu.Lock()
	defer f.ev.mu.Unlock()
	return f.ev.status[hostID]
}

func TestLiveMonitoringBesideTerminal(t *testing.T) {
	f, _, _ := monitorFixture(t)
	id := sessionID()
	if _, err := f.core.OpenTerminal(id, f.host.ID, models.ConnectOptions{Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.MonitorWatch(f.host.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "two samples", func() bool { return len(f.samples(f.host.ID)) >= 2 })
	ss := f.samples(f.host.ID)
	m := ss[len(ss)-1]
	if m.OS != "Linux" || m.Hostname != "fake-linux" || m.CPUCount != 2 {
		t.Fatalf("identity: %+v", m)
	}
	if m.CPUPct < 49 || m.CPUPct > 51 || m.MemPct < 74.9 || m.MemPct > 75.1 || m.DiskPct < 89.9 {
		t.Fatalf("cpu %v mem %v disk %v", m.CPUPct, m.MemPct, m.DiskPct)
	}
	if m.RxRate <= 0 || m.TxRate <= 0 || m.RxRate < m.TxRate {
		t.Fatalf("network rates: rx %v tx %v", m.RxRate, m.TxRate)
	}
	if !m.Docker.Available || len(m.Docker.Containers) != 2 || m.Docker.Containers[0].Name != "worker" {
		t.Fatalf("docker: %+v", m.Docker)
	}
	if !m.Services.Available || m.Services.Running != 1 {
		t.Fatalf("services: %+v", m.Services)
	}
	// The terminal's connection is reused: no second login.
	if st := f.monStatus(f.host.ID); st.State != "live" || st.Source != "session" {
		t.Fatalf("status: %+v", st)
	}
	if got := f.core.MonitorLatest(f.host.ID); got == nil || got.Hostname != "fake-linux" {
		t.Fatalf("latest: %+v", got)
	}

	// Docker actions go through the same connection; names are validated.
	if err := f.core.ContainerAction(f.host.ID, "worker", "restart"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.srv.ContainerOps, "docker restart worker") {
		t.Fatalf("container ops: %v", f.srv.ContainerOps)
	}
	for _, bad := range []string{"worker; rm -rf /", "$(id)", "-rf", ""} {
		if err := f.core.ContainerAction(f.host.ID, bad, "restart"); err == nil {
			t.Fatalf("accepted container name %q", bad)
		}
	}
	if err := f.core.ContainerAction(f.host.ID, "worker", "rm"); err == nil {
		t.Fatal("accepted unknown action")
	}

	// Free tier: no history, alerts or uptime checks.
	if _, err := f.core.MetricsHistory(f.host.ID, 60); sshconn.CodeOf(err) != CodeProRequired {
		t.Fatalf("history without Pro: %v", err)
	}
	if _, err := f.core.SaveAlertRule(models.AlertRule{Metric: models.AlertCPU, Threshold: 90, Enabled: true}); sshconn.CodeOf(err) != CodeProRequired {
		t.Fatalf("alerts without Pro: %v", err)
	}
	if _, err := f.core.SaveUptimeCheck(models.UptimeCheck{Kind: models.CheckTCP, Target: "127.0.0.1:1"}); sshconn.CodeOf(err) != CodeProRequired {
		t.Fatalf("uptime without Pro: %v", err)
	}

	// Closing the viewer stops collection.
	f.core.MonitorUnwatch(f.host.ID)
	waitFor(t, "collector stops", func() bool { return f.monStatus(f.host.ID).State == "stopped" })
	n := f.srv.Reports()
	time.Sleep(700 * time.Millisecond)
	if f.srv.Reports() != n {
		t.Fatal("collection continued after the viewer left")
	}
}

func TestMonitoringWithoutTerminal(t *testing.T) {
	f, _, _ := monitorFixture(t)
	// The host key must already be trusted for silent connections.
	if _, err := f.core.MonitorWatch(f.host.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "host key error", func() bool { return f.monStatus(f.host.ID).State == "error" })
	if st := f.monStatus(f.host.ID); !strings.Contains(st.Error, "verify its host key") {
		t.Fatalf("unknown host key: %+v", st)
	}
	f.core.MonitorUnwatch(f.host.ID)
	waitFor(t, "stopped", func() bool { return f.monStatus(f.host.ID).State == "stopped" })

	id := sessionID()
	if _, err := f.core.OpenTerminal(id, f.host.ID, models.ConnectOptions{}); err != nil { // trusts the key
		t.Fatal(err)
	}
	f.core.CloseTerminal(id)
	if _, err := f.core.MonitorWatch(f.host.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "direct sample", func() bool { st := f.monStatus(f.host.ID); return st.State == "live" && st.Source == "direct" })
	f.core.MonitorUnwatch(f.host.ID)

	// Without a saved password, monitoring explains what is needed.
	h, _ := f.core.SaveHost(models.HostInput{Host: models.Host{Label: "nopw", Hostname: "127.0.0.1", Port: f.srv.Port(), Username: "alice", AuthMethod: models.AuthPassword}})
	if _, err := f.core.MonitorWatch(h.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "password error", func() bool { return strings.Contains(f.monStatus(h.ID).Error, "save this host's password") })
}

func TestProHistoryAlertsAndUptime(t *testing.T) {
	f, notes, mu := monitorFixture(t)
	id := sessionID()
	if _, err := f.core.OpenTerminal(id, f.host.ID, models.ConnectOptions{}); err != nil {
		t.Fatal(err)
	}
	f.core.CloseTerminal(id)
	noteCount := func() int { mu.Lock(); defer mu.Unlock(); return len(*notes) }
	lastNotes := func() string { mu.Lock(); defer mu.Unlock(); return strings.Join(*notes, "\n") }

	st := f.core.Settings()
	st.ProEnabled = true
	st.MonitorHostIDs = []string{f.host.ID, "does-not-exist"}
	st.MonitorIntervalSec = 5
	saved, err := f.core.UpdateSettings(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.MonitorHostIDs) != 1 {
		t.Fatalf("unknown host IDs should be dropped: %v", saved.MonitorHostIDs)
	}
	// Background monitoring starts without any viewer.
	waitFor(t, "background sample", func() bool { return len(f.samples(f.host.ID)) >= 1 })
	if s := f.monStatus(f.host.ID); !s.Background || s.Source != "direct" {
		t.Fatalf("background status: %+v", s)
	}

	// History is recorded.
	waitFor(t, "history", func() bool {
		pts, err := f.core.MetricsHistory(f.host.ID, 60)
		return err == nil && len(pts) >= 1 && pts[0].Disk > 89
	})

	// A disk alert fires on the next sample and notifies.
	if _, err := f.core.SaveAlertRule(models.AlertRule{Name: "Disk almost full", Metric: models.AlertDisk, Op: ">", Threshold: 85, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	f.srv.FailedService.Store("backup")
	if _, err := f.core.SaveAlertRule(models.AlertRule{Metric: models.AlertServices, Op: ">", Threshold: 0, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.MonitorWatch(f.host.ID); err != nil { // speed up sampling
		t.Fatal(err)
	}
	waitFor(t, "alerts fired", func() bool {
		s := lastNotes()
		return strings.Contains(s, "Disk almost full") && strings.Contains(s, "failed service(s)") && strings.Contains(s, "backup")
	})
	evs, _ := f.core.AlertEvents()
	open := 0
	for _, e := range evs {
		if e.ResolvedAt == 0 {
			open++
		}
	}
	if open != 2 {
		t.Fatalf("open alerts: %+v", evs)
	}
	f.srv.FailedService.Store("")
	waitFor(t, "service alert resolved", func() bool { return strings.Contains(lastNotes(), "Resolved: Failed services") })

	// Uptime checks probe from this computer and can trigger alerts.
	var healthy atomic.Bool
	healthy.Store(true)
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer web.Close()
	chk, err := f.core.SaveUptimeCheck(models.UptimeCheck{Kind: models.CheckHTTP, Target: web.URL, IntervalSec: 10, TimeoutSec: 5, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "first probe", func() bool {
		sum, _ := f.core.UptimeSummaries()
		return len(sum) == 1 && sum[0].Up != nil && *sum[0].Up && sum[0].Uptime24h == 100
	})
	if _, err := f.core.SaveAlertRule(models.AlertRule{Metric: models.AlertUptime, CheckID: chk.ID, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	healthy.Store(false)
	if r, err := f.core.TestUptimeCheck(chk); err != nil || r.OK || r.StatusCode != 503 {
		t.Fatalf("test probe: %+v %v", r, err)
	}
	chk.IntervalSec = 11 // a changed check restarts and probes immediately
	if _, err := f.core.SaveUptimeCheck(chk); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "uptime alert", func() bool { return strings.Contains(lastNotes(), "is down: HTTP 503") })
	sum, _ := f.core.UptimeSummaries()
	if *sum[0].Up || sum[0].DownSince == 0 || sum[0].Uptime24h != 50 {
		t.Fatalf("summary after outage: %+v", sum[0])
	}
	if _, err := f.core.SaveUptimeCheck(models.UptimeCheck{Kind: models.CheckTCP, Target: "no-port"}); err == nil {
		t.Fatal("invalid TCP target accepted")
	}

	// Turning Pro off stops background work and closes open alerts.
	f.core.MonitorUnwatch(f.host.ID)
	before := noteCount()
	st = f.core.Settings()
	st.ProEnabled = false
	if _, err := f.core.UpdateSettings(st); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "background stopped", func() bool { return f.monStatus(f.host.ID).State == "stopped" })
	if noteCount() == before || !strings.Contains(lastNotes(), "turned off") {
		t.Fatalf("alerts should resolve when Pro is off:\n%s", lastNotes())
	}

	// Deleting a host removes its history and rules.
	if err := f.core.DeleteHost(f.host.ID); err != nil {
		t.Fatal(err)
	}
	rules, _ := f.core.AlertRules()
	for _, r := range rules {
		if r.HostID == f.host.ID {
			t.Fatal("host-scoped rule survived host deletion")
		}
	}
}
