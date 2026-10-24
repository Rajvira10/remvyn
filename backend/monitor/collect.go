// Package monitor collects server metrics over SSH without an agent: it runs
// a short read-only shell script that reads /proc, df, ps, Docker and
// systemd, and turns successive samples into rates.
package monitor

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	"remvyn/backend/models"
)

// fastScript reads the cheap, frequently sampled metrics. Everything is
// read-only. Sections are separated by "@@name" marker lines.
const fastScript = `LC_ALL=C; export LC_ALL
echo @@uname; uname -s; uname -r; hostname
echo @@uptime; cat /proc/uptime 2>/dev/null
echo @@loadavg; cat /proc/loadavg 2>/dev/null
echo @@cpu; grep '^cpu ' /proc/stat 2>/dev/null; grep -c '^processor' /proc/cpuinfo 2>/dev/null
echo @@mem; grep -E '^(MemTotal|MemAvailable|MemFree|Buffers|Cached|SwapTotal|SwapFree):' /proc/meminfo 2>/dev/null
echo @@net; cat /proc/net/dev 2>/dev/null
echo @@df; df -P -k 2>/dev/null
echo @@ps; ps -eo pid=,user=,pcpu=,pmem=,comm= --sort=-pcpu 2>/dev/null | head -n 8
`

// slowScript adds Docker containers and systemd services, which are more
// expensive ("docker stats" takes about a second) and change less often.
const slowScript = fastScript + `echo @@docker
if command -v docker >/dev/null 2>&1; then
  docker ps -a --no-trunc --format '{{json .}}' 2>&1
  echo @@dockerstats
  docker stats --no-stream --no-trunc --format '{{json .}}' 2>/dev/null
else
  echo remvyn:not-installed
fi
echo @@services
if command -v systemctl >/dev/null 2>&1; then
  systemctl list-units --type=service --all --no-legend --plain --no-pager 2>&1
else
  echo remvyn:not-available
fi
`

// raw is one parsed sample before rates are computed.
type raw struct {
	at        int64
	os        string
	kernel    string
	hostname  string
	uptime    float64
	load      [3]float64
	cpuTotal  uint64
	cpuIdle   uint64
	cpuCount  int
	mem       map[string]int64 // kB
	net       map[string][2]int64
	netOrder  []string
	disks     []models.DiskUsage
	processes []models.Process

	hasSlow  bool
	docker   models.DockerInfo
	services models.ServicesInfo
}

// ErrUnsupported is returned for servers that aren't Linux.
var ErrUnsupported = errors.New("monitoring supports Linux servers only")

func sections(out string) map[string][]string {
	m := map[string][]string{}
	cur := ""
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "@@") && !strings.ContainsAny(line, " \t") {
			cur = line[2:]
			m[cur] = []string{}
			continue
		}
		if cur != "" && line != "" {
			m[cur] = append(m[cur], line)
		}
	}
	return m
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "%")), 64)
	return f
}

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

// parse turns script output into a raw sample.
func parse(out string, at int64) (raw, error) {
	s := sections(out)
	r := raw{at: at, mem: map[string]int64{}, net: map[string][2]int64{}}
	if u := s["uname"]; len(u) > 0 {
		r.os = strings.TrimSpace(u[0])
		if len(u) > 1 {
			r.kernel = strings.TrimSpace(u[1])
		}
		if len(u) > 2 {
			r.hostname = strings.TrimSpace(u[2])
		}
	}
	if r.os != "Linux" {
		return r, ErrUnsupported
	}
	if u := s["uptime"]; len(u) > 0 {
		if f := strings.Fields(u[0]); len(f) > 0 {
			r.uptime = atof(f[0])
		}
	}
	if l := s["loadavg"]; len(l) > 0 {
		f := strings.Fields(l[0])
		for i := 0; i < 3 && i < len(f); i++ {
			r.load[i] = atof(f[i])
		}
	}
	for _, line := range s["cpu"] {
		f := strings.Fields(line)
		if len(f) > 4 && f[0] == "cpu" {
			var vals []uint64
			for _, v := range f[1:] {
				n, _ := strconv.ParseUint(v, 10, 64)
				vals = append(vals, n)
			}
			// user nice system idle iowait irq softirq steal (guest time is
			// already included in user/nice).
			for i := 0; i < len(vals) && i < 8; i++ {
				r.cpuTotal += vals[i]
			}
			r.cpuIdle = vals[3]
			if len(vals) > 4 {
				r.cpuIdle += vals[4]
			}
		} else if len(f) == 1 {
			r.cpuCount = int(atoi64(f[0]))
		}
	}
	for _, line := range s["mem"] {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if f := strings.Fields(v); len(f) > 0 {
			r.mem[k] = atoi64(f[0])
		}
	}
	for _, line := range s["net"] {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		f := strings.Fields(rest)
		if len(f) < 9 || name == "" || strings.Contains(name, "|") {
			continue
		}
		r.net[name] = [2]int64{atoi64(f[0]), atoi64(f[8])}
		r.netOrder = append(r.netOrder, name)
	}
	r.disks = parseDF(s["df"])
	r.processes = parsePS(s["ps"])
	if _, ok := s["docker"]; ok {
		r.hasSlow = true
		r.docker = parseDocker(s["docker"], s["dockerstats"])
		r.services = parseServices(s["services"])
	}
	return r, nil
}

// pseudo filesystems that aren't interesting as "disks".
var skipFS = map[string]bool{"tmpfs": true, "devtmpfs": true, "overlay": true, "squashfs": true, "udev": true, "none": true,
	"shm": true, "proc": true, "sysfs": true, "cgroup": true, "cgroup2": true, "efivarfs": true}

func parseDF(lines []string) []models.DiskUsage {
	out := []models.DiskUsage{}
	seen := map[string]bool{}
	for i, line := range lines {
		f := strings.Fields(line)
		if i == 0 && len(f) > 0 && f[0] == "Filesystem" {
			continue
		}
		if len(f) < 6 {
			continue
		}
		fs := f[0]
		mount := strings.Join(f[5:], " ")
		// Inside containers the root filesystem is an overlay: keep it, but
		// skip other overlays (Docker's per-container layers).
		rootOverlay := fs == "overlay" && mount == "/"
		if (skipFS[fs] && !rootOverlay) || strings.HasPrefix(mount, "/etc/") || strings.HasPrefix(mount, "/snap/") || strings.HasPrefix(mount, "/boot/efi") ||
			strings.HasPrefix(mount, "/run") || strings.HasPrefix(mount, "/dev") || strings.HasPrefix(mount, "/sys") ||
			strings.HasPrefix(mount, "/proc") || strings.HasPrefix(mount, "/var/lib/docker/") {
			continue
		}
		total, used := atoi64(f[1])*1024, atoi64(f[2])*1024
		if total == 0 || seen[fs] {
			continue
		}
		seen[fs] = true
		avail := atoi64(f[3]) * 1024
		pct := 0.0
		// Match df's own "Use%": used / (used + available).
		if used+avail > 0 {
			pct = float64(used) / float64(used+avail) * 100
		}
		out = append(out, models.DiskUsage{Mount: mount, FS: fs, Total: total, Used: used, UsedPct: pct})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Mount == "/" && out[j].Mount != "/" })
	return out
}

func parsePS(lines []string) []models.Process {
	out := []models.Process{}
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		out = append(out, models.Process{PID: pid, User: f[1], CPUPct: atof(f[2]), MemPct: atof(f[3]), Command: strings.Join(f[4:], " ")})
	}
	return out
}

type dockerPS struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Image  string `json:"Image"`
	State  string `json:"State"`
	Status string `json:"Status"`
	Ports  string `json:"Ports"`
}

type dockerStat struct {
	ID       string `json:"ID"`
	Name     string `json:"Name"`
	CPUPerc  string `json:"CPUPerc"`
	MemPerc  string `json:"MemPerc"`
	MemUsage string `json:"MemUsage"`
}

func parseDocker(ps, stats []string) models.DockerInfo {
	d := models.DockerInfo{Containers: []models.Container{}}
	if len(ps) == 1 && ps[0] == "remvyn:not-installed" {
		d.Error = "Docker is not installed"
		return d
	}
	var errLines []string
	for _, line := range ps {
		if !strings.HasPrefix(line, "{") {
			errLines = append(errLines, line)
			continue
		}
		var c dockerPS
		if json.Unmarshal([]byte(line), &c) != nil {
			continue
		}
		state := strings.ToLower(c.State)
		if state == "" { // very old Docker: derive from status
			state = "exited"
			if strings.HasPrefix(c.Status, "Up") {
				state = "running"
			}
		}
		d.Containers = append(d.Containers, models.Container{ID: c.ID, Name: c.Names, Image: c.Image, State: state, Status: c.Status, Ports: c.Ports})
	}
	if len(d.Containers) == 0 && len(errLines) > 0 {
		msg := strings.Join(errLines, " ")
		switch {
		case strings.Contains(msg, "permission denied"):
			d.Error = "permission denied: add this user to the docker group to see containers"
		case strings.Contains(msg, "Cannot connect to the Docker daemon"), strings.Contains(msg, "Is the docker daemon running"):
			d.Error = "the Docker daemon is not running"
		default:
			d.Error = msg
		}
		return d
	}
	d.Available = true
	byID := map[string]dockerStat{}
	for _, line := range stats {
		var st dockerStat
		if json.Unmarshal([]byte(line), &st) == nil {
			byID[st.ID] = st
			byID[st.Name] = st
		}
	}
	for i := range d.Containers {
		c := &d.Containers[i]
		st, ok := byID[c.ID]
		if !ok {
			st, ok = byID[c.Name]
		}
		if !ok && len(c.ID) > 12 {
			st, ok = byID[c.ID[:12]]
		}
		if ok {
			c.CPUPct, c.MemPct, c.MemUsage = atof(st.CPUPerc), atof(st.MemPerc), st.MemUsage
		}
	}
	sort.SliceStable(d.Containers, func(i, j int) bool {
		ri, rj := d.Containers[i].State == "running", d.Containers[j].State == "running"
		if ri != rj {
			return !ri // stopped containers first: they need attention
		}
		return d.Containers[i].Name < d.Containers[j].Name
	})
	return d
}

func parseServices(lines []string) models.ServicesInfo {
	s := models.ServicesInfo{Items: []models.Service{}}
	if len(lines) == 1 && lines[0] == "remvyn:not-available" {
		s.Error = "systemd is not available on this server"
		return s
	}
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) < 4 || !strings.HasSuffix(f[0], ".service") {
			continue
		}
		if f[1] != "loaded" {
			continue // not-found / masked units
		}
		svc := models.Service{Name: strings.TrimSuffix(f[0], ".service"), Active: f[2], Sub: f[3]}
		if len(f) > 4 {
			svc.Description = strings.Join(f[4:], " ")
		}
		s.Items = append(s.Items, svc)
		switch {
		case svc.Active == "failed":
			s.Failed++
		case svc.Sub == "running":
			s.Running++
		}
	}
	if len(s.Items) == 0 && len(lines) > 0 {
		s.Error = strings.Join(lines, " ")
		return s
	}
	s.Available = true
	s.Total = len(s.Items)
	rank := func(v models.Service) int {
		switch {
		case v.Active == "failed":
			return 0
		case v.Sub == "running":
			return 1
		}
		return 2
	}
	sort.SliceStable(s.Items, func(i, j int) bool {
		if a, b := rank(s.Items[i]), rank(s.Items[j]); a != b {
			return a < b
		}
		return s.Items[i].Name < s.Items[j].Name
	})
	return s
}

// virtual interfaces excluded from the host's total traffic.
func virtualIface(name string) bool {
	for _, p := range []string{"lo", "veth", "br-", "docker", "virbr", "cni", "flannel", "cali", "vxlan", "tunl", "kube-"} {
		if name == p || strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// compute builds Metrics from the current sample and the previous one (for
// CPU and network rates). prev may be nil for the first sample.
func compute(hostID string, cur raw, prev *raw) models.Metrics {
	m := models.Metrics{HostID: hostID, At: cur.at, OS: cur.os, Kernel: cur.kernel, Hostname: cur.hostname,
		UptimeSec: int64(cur.uptime), CPUCount: cur.cpuCount, Load1: cur.load[0], Load5: cur.load[1], Load15: cur.load[2],
		Disks: cur.disks, Processes: cur.processes, Net: []models.NetIface{}}

	if prev != nil && cur.cpuTotal > prev.cpuTotal {
		dt := float64(cur.cpuTotal - prev.cpuTotal)
		di := float64(cur.cpuIdle) - float64(prev.cpuIdle)
		m.CPUPct = clamp((1 - di/dt) * 100)
	}

	kb := func(k string) int64 { return cur.mem[k] * 1024 }
	m.MemTotal = kb("MemTotal")
	m.MemAvailable = kb("MemAvailable")
	if _, ok := cur.mem["MemAvailable"]; !ok { // kernels before 3.14
		m.MemAvailable = kb("MemFree") + kb("Buffers") + kb("Cached")
	}
	m.MemUsed = m.MemTotal - m.MemAvailable
	if m.MemTotal > 0 {
		m.MemPct = clamp(float64(m.MemUsed) / float64(m.MemTotal) * 100)
	}
	m.SwapTotal = kb("SwapTotal")
	m.SwapUsed = m.SwapTotal - kb("SwapFree")
	if m.SwapTotal > 0 {
		m.SwapPct = clamp(float64(m.SwapUsed) / float64(m.SwapTotal) * 100)
	}

	for _, d := range cur.disks {
		if d.UsedPct > m.DiskPct {
			m.DiskPct = d.UsedPct
		}
	}

	secs := 0.0
	if prev != nil {
		secs = float64(cur.at-prev.at) / 1000
	}
	for _, name := range cur.netOrder {
		v := cur.net[name]
		n := models.NetIface{Name: name, RxTotal: v[0], TxTotal: v[1]}
		if prev != nil && secs > 0 {
			if p, ok := prev.net[name]; ok && v[0] >= p[0] && v[1] >= p[1] {
				n.RxRate = float64(v[0]-p[0]) / secs
				n.TxRate = float64(v[1]-p[1]) / secs
			}
		}
		// Skip loopback and interfaces that have never carried traffic
		// (tunnel placeholders such as gre0 or sit0).
		if name == "lo" || v[0]+v[1] == 0 {
			continue
		}
		m.Net = append(m.Net, n)
		if !virtualIface(name) {
			m.RxRate += n.RxRate
			m.TxRate += n.TxRate
		}
	}
	return m
}

func clamp(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 100:
		return 100
	}
	return v
}
