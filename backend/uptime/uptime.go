// Package uptime probes URLs and TCP ports on a schedule.
package uptime

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"flux/backend/models"
)

// EventResult is emitted with a models.UptimeResult after every probe.
const EventResult = "uptime:result"

// Checker runs one goroutine per enabled check.
type Checker struct {
	// OnResult receives every probe result (storage, alerts, UI).
	OnResult func(models.UptimeCheck, models.UptimeResult)

	mu      sync.Mutex
	running map[string]*job
}

type job struct {
	check  models.UptimeCheck
	cancel context.CancelFunc
	done   chan struct{}
}

// New creates a Checker.
func New(onResult func(models.UptimeCheck, models.UptimeResult)) *Checker {
	return &Checker{OnResult: onResult, running: map[string]*job{}}
}

// Sync makes the running set match checks: new and changed checks start,
// removed or disabled ones stop.
func (u *Checker) Sync(checks []models.UptimeCheck) {
	u.mu.Lock()
	defer u.mu.Unlock()
	want := map[string]models.UptimeCheck{}
	for _, c := range checks {
		if c.Enabled {
			want[c.ID] = c
		}
	}
	for id, j := range u.running {
		if c, ok := want[id]; !ok || c != j.check {
			j.cancel()
			delete(u.running, id)
		}
	}
	for id, c := range want {
		if _, ok := u.running[id]; ok {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		j := &job{check: c, cancel: cancel, done: make(chan struct{})}
		u.running[id] = j
		go u.loop(ctx, j)
	}
}

// StopAll stops every check and waits for in-flight probes.
func (u *Checker) StopAll() {
	u.mu.Lock()
	jobs := make([]*job, 0, len(u.running))
	for id, j := range u.running {
		j.cancel()
		jobs = append(jobs, j)
		delete(u.running, id)
	}
	u.mu.Unlock()
	for _, j := range jobs {
		<-j.done
	}
}

func (u *Checker) loop(ctx context.Context, j *job) {
	defer close(j.done)
	t := time.NewTicker(time.Duration(j.check.IntervalSec) * time.Second)
	defer t.Stop()
	for {
		r := Probe(ctx, j.check)
		if ctx.Err() != nil {
			return
		}
		if u.OnResult != nil {
			u.OnResult(j.check, r)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

var httpClient = &http.Client{
	Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		DisableKeepAlives:   true, // measure a fresh connection every time
		TLSHandshakeTimeout: 10 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	},
}

// Probe runs one check now.
func Probe(ctx context.Context, c models.UptimeCheck) models.UptimeResult {
	r := models.UptimeResult{CheckID: c.ID, At: time.Now().UnixMilli()}
	timeout := time.Duration(c.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	switch c.Kind {
	case models.CheckTCP:
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", c.Target)
		if err != nil {
			r.Error = friendly(err)
			break
		}
		conn.Close()
		r.OK = true
	case models.CheckHTTP:
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Target, nil)
		if err != nil {
			r.Error = err.Error()
			break
		}
		req.Header.Set("User-Agent", "Flux-Uptime/1.0")
		res, err := httpClient.Do(req)
		if err != nil {
			r.Error = friendly(err)
			break
		}
		_, _ = io.CopyN(io.Discard, res.Body, 64<<10)
		res.Body.Close()
		r.StatusCode = res.StatusCode
		r.OK = res.StatusCode < 400
		if !r.OK {
			r.Error = fmt.Sprintf("HTTP %d %s", res.StatusCode, http.StatusText(res.StatusCode))
		}
	default:
		r.Error = "unknown check type"
	}
	r.LatencyMs = time.Since(start).Milliseconds()
	return r
}

func friendly(err error) string {
	s := err.Error()
	switch {
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(s, "Client.Timeout") || strings.Contains(s, "i/o timeout"):
		return "timed out"
	case strings.Contains(s, "connection refused"):
		return "connection refused"
	case strings.Contains(s, "no such host"):
		return "unknown host name"
	case strings.Contains(s, "certificate"):
		if i := strings.Index(s, "tls: "); i >= 0 {
			return "TLS error: " + s[i+5:]
		}
		if i := strings.Index(s, "x509: "); i >= 0 {
			return "certificate error: " + s[i+6:]
		}
	}
	if i := strings.LastIndex(s, ": "); i >= 0 {
		return s[i+2:]
	}
	return s
}
