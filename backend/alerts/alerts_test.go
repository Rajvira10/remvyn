package alerts

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"flux/backend/models"
)

type memStore struct {
	mu     sync.Mutex
	events map[int64]*models.AlertEvent
	next   int64
}

func (s *memStore) InsertAlertEvent(_ context.Context, e models.AlertEvent) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	e.ID = s.next
	s.events[e.ID] = &e
	return e.ID, nil
}

func (s *memStore) ResolveAlertEvent(_ context.Context, id, at int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[id].ResolvedAt = at
	return nil
}

type harness struct {
	e      *Engine
	store  *memStore
	now    time.Time
	notes  []string
	events []models.AlertEvent
}

func newHarness() *harness {
	h := &harness{store: &memStore{events: map[int64]*models.AlertEvent{}}, now: time.Unix(1_700_000_000, 0)}
	h.e = New(Options{
		Store:     h.store,
		Emit:      func(_ string, d any) { h.events = append(h.events, d.(models.AlertEvent)) },
		Notify:    func(t, b string) { h.notes = append(h.notes, t+" | "+b) },
		HostLabel: func(id string) string { return "label-" + id },
		Now:       func() time.Time { return h.now },
	})
	h.e.SetEnabled(true)
	return h
}

func (h *harness) tick(d time.Duration) { h.now = h.now.Add(d) }

func cpu(host string, v float64) models.Metrics { return models.Metrics{HostID: host, CPUPct: v} }

func TestSustainedThresholdFiresAndResolves(t *testing.T) {
	h := newHarness()
	h.e.SetRules([]models.AlertRule{{ID: "r1", Metric: models.AlertCPU, Op: ">", Threshold: 90, DurationSec: 60, Enabled: true}})

	h.e.OnSample(cpu("a", 95))
	h.tick(30 * time.Second)
	h.e.OnSample(cpu("a", 50)) // dip resets the timer
	h.tick(10 * time.Second)
	h.e.OnSample(cpu("a", 95))
	h.tick(50 * time.Second)
	h.e.OnSample(cpu("a", 96))
	if len(h.notes) != 0 {
		t.Fatalf("fired too early: %v", h.notes)
	}
	h.tick(15 * time.Second) // 65s since the breach began
	h.e.OnSample(cpu("a", 97))
	if len(h.notes) != 1 || !strings.Contains(h.notes[0], "CPU is 97% on label-a (above 90% for 1m)") {
		t.Fatalf("fire: %v", h.notes)
	}
	h.e.OnSample(cpu("a", 99)) // still firing: no duplicate
	if len(h.notes) != 1 || len(h.store.events) != 1 {
		t.Fatalf("duplicate fire: %v", h.notes)
	}
	// Another host is tracked separately.
	h.e.OnSample(cpu("b", 99))
	h.tick(2 * time.Minute)
	h.e.OnSample(cpu("a", 10))
	if len(h.notes) != 2 || !strings.HasPrefix(h.notes[1], "✅ Resolved") || h.store.events[1].ResolvedAt == 0 {
		t.Fatalf("resolve: %v", h.notes)
	}
	if ev := h.store.events[1]; ev.StartedAt != time.Unix(1_700_000_000, 0).Add(40*time.Second).UnixMilli() {
		t.Fatalf("event should start when the breach began: %+v", ev)
	}
}

func TestHostScopedRuleAndDisabledEngine(t *testing.T) {
	h := newHarness()
	h.e.SetRules([]models.AlertRule{{ID: "r", Metric: models.AlertDisk, HostID: "a", Op: ">", Threshold: 80, Enabled: true}})
	h.e.OnSample(models.Metrics{HostID: "b", DiskPct: 99})
	if len(h.notes) != 0 {
		t.Fatal("rule for host a fired for host b")
	}
	h.e.OnSample(models.Metrics{HostID: "a", DiskPct: 85})
	if len(h.notes) != 1 {
		t.Fatalf("zero-duration rule should fire at once: %v", h.notes)
	}
	h.e.SetEnabled(false)
	if len(h.notes) != 2 || !strings.Contains(h.notes[1], "turned off") {
		t.Fatalf("disabling should resolve: %v", h.notes)
	}
	h.e.OnSample(models.Metrics{HostID: "a", DiskPct: 99})
	if len(h.notes) != 2 {
		t.Fatal("disabled engine fired")
	}
}

func TestUnreachableServicesContainersAndUptime(t *testing.T) {
	h := newHarness()
	h.e.SetRules([]models.AlertRule{
		{ID: "down", Metric: models.AlertUnreachable, DurationSec: 120, Enabled: true},
		{ID: "svc", Metric: models.AlertServices, Op: ">", Threshold: 0, Enabled: true},
		{ID: "ctr", Metric: models.AlertContainers, Op: ">", Threshold: 0, Enabled: true},
		{ID: "up", Metric: models.AlertUptime, Enabled: true},
	})
	h.e.OnHostError("a", errors.New("connection refused"))
	h.tick(3 * time.Minute)
	h.e.OnHostError("a", errors.New("connection refused"))
	if len(h.notes) != 1 || !strings.Contains(h.notes[0], "label-a can't be reached for 2m: connection refused") {
		t.Fatalf("unreachable: %v", h.notes)
	}
	m := models.Metrics{HostID: "a",
		Services: models.ServicesInfo{Available: true, Failed: 1, Items: []models.Service{{Name: "backup", Active: "failed"}}},
		Docker:   models.DockerInfo{Available: true, Containers: []models.Container{{Name: "web", State: "running"}, {Name: "worker", State: "exited"}}},
	}
	h.e.OnSample(m) // reachable again + failed service + stopped container
	joined := strings.Join(h.notes, "\n")
	for _, want := range []string{"Resolved: Host unreachable", "1 failed service(s) on label-a: backup", "1 stopped container(s) on label-a: worker"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
	// Unavailable Docker data must not resolve or fire container alerts.
	n := len(h.notes)
	h.e.OnSample(models.Metrics{HostID: "a", Services: m.Services})
	if len(h.notes) != n {
		t.Fatalf("missing docker data changed alerts: %v", h.notes[n:])
	}

	chk := models.UptimeCheck{ID: "c1", Name: "API"}
	h.e.OnUptime(chk, models.UptimeResult{OK: false, Error: "HTTP 503 Service Unavailable"})
	if !strings.Contains(h.notes[len(h.notes)-1], "API is down: HTTP 503") {
		t.Fatalf("uptime: %v", h.notes)
	}
	h.e.OnUptime(chk, models.UptimeResult{OK: true})
	if !strings.Contains(h.notes[len(h.notes)-1], "Resolved: Uptime check down · API") {
		t.Fatalf("uptime resolve: %v", h.notes)
	}
}

func TestChangingARuleResolvesItsAlerts(t *testing.T) {
	h := newHarness()
	r := models.AlertRule{ID: "r", Metric: models.AlertMemory, Op: ">", Threshold: 50, Enabled: true}
	h.e.SetRules([]models.AlertRule{r})
	h.e.OnSample(models.Metrics{HostID: "a", MemPct: 70})
	r.Threshold = 80
	h.e.SetRules([]models.AlertRule{r})
	if len(h.notes) != 2 || !strings.Contains(h.notes[1], "rule was changed") {
		t.Fatalf("change: %v", h.notes)
	}
	h.e.OnSample(models.Metrics{HostID: "a", MemPct: 70}) // below the new threshold
	if len(h.notes) != 2 {
		t.Fatalf("refired: %v", h.notes)
	}
}

// A notifier that never returns (for example, waiting on a macOS permission
// prompt) must not block other evaluations or rule changes.
func TestBlockedNotifierDoesNotHoldTheLock(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	e := New(Options{Notify: func(string, string) { <-block }})
	e.SetEnabled(true)
	e.SetRules([]models.AlertRule{{ID: "r", Metric: models.AlertCPU, Op: ">", Threshold: 50, Enabled: true}})
	go e.OnSample(cpu("a", 90)) // fires, then blocks inside Notify
	time.Sleep(50 * time.Millisecond)
	done := make(chan struct{})
	go func() {
		e.SetRules([]models.AlertRule{
			{ID: "r", Metric: models.AlertCPU, Op: ">", Threshold: 50, Enabled: true}, // unchanged: stays firing
			{ID: "r2", Metric: models.AlertDisk, Op: ">", Threshold: 50, Enabled: true},
		})
		e.OnSample(models.Metrics{HostID: "b", DiskPct: 10})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("engine stayed locked while a notification was pending")
	}
}
