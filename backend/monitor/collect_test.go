package monitor

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func sample(cpu, rx, tx string, docker, services bool) string {
	s := `@@uname
Linux
6.8.0-45-generic
web-1
@@uptime
86523.42 340000.10
@@loadavg
0.52 0.40 0.31 2/301 12345
@@cpu
cpu  ` + cpu + `
4
@@mem
MemTotal:        8000000 kB
MemFree:          500000 kB
MemAvailable:    6000000 kB
Buffers:          100000 kB
Cached:          2000000 kB
SwapTotal:       2000000 kB
SwapFree:        1500000 kB
@@net
Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 999999 100 0 0 0 0 0 0 999999 100 0 0 0 0 0 0
  eth0: ` + rx + ` 2000 0 0 0 0 0 0 ` + tx + ` 1500 0 0 0 0 0 0
docker0: 5000000 10 0 0 0 0 0 0 5000000 10 0 0 0 0 0 0
  gre0: 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0
@@df
Filesystem     1024-blocks     Used Available Capacity Mounted on
tmpfs               800000     1000    799000       1% /run
/dev/sda1         50000000 40000000  10000000      80% /
/dev/sdb1        100000000 10000000  90000000      10% /mnt/data volume
overlay           50000000 40000000  10000000      80% /var/lib/docker/overlay2/abc/merged
/dev/sda1         50000000 40000000  10000000      80% /etc/hosts
@@ps
  812 postgres  12.5  3.1 postgres
  901 www-data   4.0  1.2 nginx
`
	if docker {
		s += `@@docker
{"ID":"abc123def4567890","Names":"web","Image":"nginx:1.27","State":"running","Status":"Up 3 hours","Ports":"0.0.0.0:80->80/tcp"}
{"ID":"fff000aaa1112223","Names":"worker","Image":"app:latest","State":"exited","Status":"Exited (1) 5 minutes ago","Ports":""}
@@dockerstats
{"ID":"abc123def4567890","Name":"web","CPUPerc":"1.25%","MemPerc":"0.80%","MemUsage":"64MiB / 7.6GiB"}
`
	}
	if services {
		s += `@@services
nginx.service            loaded    active   running A high performance web server
postgresql.service       loaded    active   exited  PostgreSQL RDBMS
backup.service           loaded    failed   failed  Nightly backup
ghost.service            not-found inactive dead    ghost.service
cron.service             loaded    active   running Regular background program processing daemon
`
	}
	return s
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestParseAndComputeRates(t *testing.T) {
	r1, err := parse(sample("1000 0 1000 8000 0 0 0 0 0 0", "1000000", "500000", false, false), 10_000)
	if err != nil {
		t.Fatal(err)
	}
	m1 := compute("h", r1, nil)
	if m1.CPUPct != 0 || m1.RxRate != 0 {
		t.Fatalf("first sample has no rates: %+v", m1)
	}
	// 2s later: 1000 busy + 1000 idle jiffies → 50% CPU; eth0 +2MB rx, +1MB tx.
	r2, _ := parse(sample("1500 0 1500 9000 0 0 0 0 0 0", "3000000", "1500000", false, false), 12_000)
	m := compute("h", r2, &r1)

	if !near(m.CPUPct, 50) {
		t.Errorf("cpu = %v", m.CPUPct)
	}
	if m.OS != "Linux" || m.Kernel != "6.8.0-45-generic" || m.Hostname != "web-1" || m.UptimeSec != 86523 || m.CPUCount != 4 {
		t.Errorf("identity: %+v", m)
	}
	if !near(m.Load1, 0.52) || !near(m.Load15, 0.31) {
		t.Errorf("load: %v %v", m.Load1, m.Load15)
	}
	if m.MemTotal != 8000000*1024 || m.MemUsed != 2000000*1024 || !near(m.MemPct, 25) {
		t.Errorf("memory: %d %d %v", m.MemTotal, m.MemUsed, m.MemPct)
	}
	if !near(m.SwapPct, 25) {
		t.Errorf("swap: %v", m.SwapPct)
	}
	// Only eth0 counts toward host traffic (lo and docker0 are excluded).
	if !near(m.RxRate, 1_000_000) || !near(m.TxRate, 500_000) {
		t.Errorf("rates: rx %v tx %v", m.RxRate, m.TxRate)
	}
	if len(m.Net) != 2 || m.Net[0].Name != "eth0" {
		t.Errorf("interfaces: %+v", m.Net)
	}
	// tmpfs, /run and docker overlay mounts are skipped; "/" comes first;
	// mount points with spaces survive.
	if len(m.Disks) != 2 || m.Disks[0].Mount != "/" || m.Disks[1].Mount != "/mnt/data volume" {
		t.Fatalf("disks: %+v", m.Disks)
	}
	if !near(m.Disks[0].UsedPct, 80) || !near(m.DiskPct, 80) || m.Disks[0].Total != 50000000*1024 {
		t.Errorf("disk usage: %+v %v", m.Disks[0], m.DiskPct)
	}
	if len(m.Processes) != 2 || m.Processes[0].Command != "postgres" || !near(m.Processes[0].CPUPct, 12.5) {
		t.Errorf("processes: %+v", m.Processes)
	}
}

func TestParseDockerAndServices(t *testing.T) {
	r, err := parse(sample("1 0 1 1 0 0 0 0", "1", "1", true, true), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !r.hasSlow || !r.docker.Available || len(r.docker.Containers) != 2 {
		t.Fatalf("docker: %+v", r.docker)
	}
	// Stopped containers sort first.
	if c := r.docker.Containers[0]; c.Name != "worker" || c.State != "exited" {
		t.Errorf("first container: %+v", c)
	}
	if c := r.docker.Containers[1]; c.Name != "web" || !near(c.CPUPct, 1.25) || c.MemUsage != "64MiB / 7.6GiB" {
		t.Errorf("container stats: %+v", c)
	}
	s := r.services
	if !s.Available || s.Total != 4 || s.Failed != 1 || s.Running != 2 {
		t.Fatalf("services: %+v", s)
	}
	if s.Items[0].Name != "backup" || s.Items[0].Description != "Nightly backup" {
		t.Errorf("failed services first: %+v", s.Items[0])
	}
}

func TestDockerAndSystemdUnavailable(t *testing.T) {
	base := sample("1 0 1 1", "1", "1", false, false)
	r, _ := parse(base+"@@docker\nflux:not-installed\n@@services\nflux:not-available\n", 1)
	if r.docker.Available || r.docker.Error != "Docker is not installed" || r.services.Available {
		t.Fatalf("unavailable: %+v %+v", r.docker, r.services)
	}
	r, _ = parse(base+"@@docker\npermission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock\n@@dockerstats\n@@services\n", 1)
	if !strings.Contains(r.docker.Error, "docker group") {
		t.Fatalf("permission error: %q", r.docker.Error)
	}
}

func TestContainerRootOverlayIsKept(t *testing.T) {
	disks := parseDF([]string{
		"Filesystem 1024-blocks Used Available Capacity Mounted on",
		"overlay 474095688 45810100 404129380 11% /",
		"tmpfs 65536 0 65536 0% /dev",
		"/dev/vda1 474095688 45810100 404129380 11% /etc/hosts",
		"tmpfs 812648 256 812392 1% /run/docker.sock",
	})
	if len(disks) != 1 || disks[0].Mount != "/" || disks[0].FS != "overlay" {
		t.Fatalf("disks: %+v", disks)
	}
}

func TestNonLinuxIsUnsupported(t *testing.T) {
	if _, err := parse("@@uname\nDarwin\n23.0\nmac\n", 1); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("got %v", err)
	}
}

func TestCounterResetGivesNoNegativeRates(t *testing.T) {
	r1, _ := parse(sample("1000 0 1000 8000", "5000000", "5000000", false, false), 1000)
	r2, _ := parse(sample("10 0 10 80", "100", "100", false, false), 3000) // e.g. after a reboot
	m := compute("h", r2, &r1)
	if m.CPUPct != 0 || m.RxRate != 0 || m.TxRate != 0 {
		t.Fatalf("reset: cpu %v rx %v tx %v", m.CPUPct, m.RxRate, m.TxRate)
	}
}
