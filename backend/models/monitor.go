package models

// Metrics is one monitoring sample for a host. Rates are bytes per second,
// percentages are 0–100, sizes are bytes.
type Metrics struct {
	HostID    string `json:"hostId"`
	At        int64  `json:"at"`
	OS        string `json:"os"`
	Kernel    string `json:"kernel"`
	Hostname  string `json:"hostname"`
	UptimeSec int64  `json:"uptimeSec"`
	CPUCount  int    `json:"cpuCount"`

	CPUPct float64 `json:"cpuPct"`
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`

	MemTotal     int64   `json:"memTotal"`
	MemUsed      int64   `json:"memUsed"`
	MemAvailable int64   `json:"memAvailable"`
	MemPct       float64 `json:"memPct"`
	SwapTotal    int64   `json:"swapTotal"`
	SwapUsed     int64   `json:"swapUsed"`
	SwapPct      float64 `json:"swapPct"`

	Disks []DiskUsage `json:"disks"`
	// DiskPct is the fullest disk's usage.
	DiskPct float64 `json:"diskPct"`

	Net    []NetIface `json:"net"`
	RxRate float64    `json:"rxRate"`
	TxRate float64    `json:"txRate"`

	Processes []Process    `json:"processes"`
	Docker    DockerInfo   `json:"docker"`
	Services  ServicesInfo `json:"services"`
}

// DiskUsage is one mounted filesystem.
type DiskUsage struct {
	Mount   string  `json:"mount"`
	FS      string  `json:"fs"`
	Total   int64   `json:"total"`
	Used    int64   `json:"used"`
	UsedPct float64 `json:"usedPct"`
}

// NetIface is one network interface's traffic.
type NetIface struct {
	Name    string  `json:"name"`
	RxRate  float64 `json:"rxRate"`
	TxRate  float64 `json:"txRate"`
	RxTotal int64   `json:"rxTotal"`
	TxTotal int64   `json:"txTotal"`
}

// Process is one of the busiest processes.
type Process struct {
	PID     int     `json:"pid"`
	User    string  `json:"user"`
	Command string  `json:"command"`
	CPUPct  float64 `json:"cpuPct"`
	MemPct  float64 `json:"memPct"`
}

// DockerInfo describes containers on the host.
type DockerInfo struct {
	// Available is false when Docker isn't installed or not usable.
	Available  bool        `json:"available"`
	Error      string      `json:"error"`
	Containers []Container `json:"containers"`
	// UpdatedAt is when container data was last collected (it refreshes
	// less often than the other metrics).
	UpdatedAt int64 `json:"updatedAt"`
}

// Container is one Docker container.
type Container struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Image    string  `json:"image"`
	State    string  `json:"state"`
	Status   string  `json:"status"`
	Ports    string  `json:"ports"`
	CPUPct   float64 `json:"cpuPct"`
	MemPct   float64 `json:"memPct"`
	MemUsage string  `json:"memUsage"`
}

// ServicesInfo describes systemd services.
type ServicesInfo struct {
	Available bool      `json:"available"`
	Error     string    `json:"error"`
	Running   int       `json:"running"`
	Failed    int       `json:"failed"`
	Total     int       `json:"total"`
	Items     []Service `json:"items"`
	UpdatedAt int64     `json:"updatedAt"`
}

// Service is one systemd service unit.
type Service struct {
	Name        string `json:"name"`
	Active      string `json:"active"` // active | inactive | failed | activating | ...
	Sub         string `json:"sub"`    // running | exited | dead | failed | ...
	Description string `json:"description"`
}

// MonitorStatus is a host's collection state.
type MonitorStatus struct {
	HostID string `json:"hostId"`
	// State is connecting | live | error | stopped.
	State string `json:"state"`
	Error string `json:"error"`
	// Source is "session" when reusing an open terminal's connection, or
	// "direct" when Flux connected just for monitoring.
	Source      string `json:"source"`
	Background  bool   `json:"background"`
	IntervalSec int    `json:"intervalSec"`
	LastAt      int64  `json:"lastAt"`
}

// MetricPoint is an aggregated historical sample.
type MetricPoint struct {
	At     int64   `json:"at"`
	CPU    float64 `json:"cpu"`
	CPUMax float64 `json:"cpuMax"`
	Mem    float64 `json:"mem"`
	Swap   float64 `json:"swap"`
	Disk   float64 `json:"disk"`
	Load1  float64 `json:"load1"`
	Rx     float64 `json:"rx"`
	Tx     float64 `json:"tx"`
}

// Alert metrics.
const (
	AlertCPU         = "cpu"
	AlertMemory      = "memory"
	AlertSwap        = "swap"
	AlertDisk        = "disk"
	AlertLoad        = "load"
	AlertServices    = "services_failed"
	AlertContainers  = "containers_stopped"
	AlertUnreachable = "host_unreachable"
	AlertUptime      = "uptime_down"
)

// AlertRule fires when a metric stays past a threshold for a duration.
type AlertRule struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Metric string `json:"metric"`
	// HostID limits the rule to one host; empty means every monitored host.
	HostID string `json:"hostId"`
	// CheckID is the uptime check for uptime_down rules (empty = any check).
	CheckID     string  `json:"checkId"`
	Op          string  `json:"op"` // ">" or "<"
	Threshold   float64 `json:"threshold"`
	DurationSec int     `json:"durationSec"`
	Enabled     bool    `json:"enabled"`
	CreatedAt   int64   `json:"createdAt"`
}

// AlertEvent is a fired (and possibly resolved) alert.
type AlertEvent struct {
	ID         int64   `json:"id"`
	RuleID     string  `json:"ruleId"`
	RuleName   string  `json:"ruleName"`
	Metric     string  `json:"metric"`
	Subject    string  `json:"subject"` // host label or check name
	SubjectID  string  `json:"subjectId"`
	Value      float64 `json:"value"`
	Threshold  float64 `json:"threshold"`
	Message    string  `json:"message"`
	StartedAt  int64   `json:"startedAt"`
	ResolvedAt int64   `json:"resolvedAt"`
}

// Uptime check kinds.
const (
	CheckHTTP = "http"
	CheckTCP  = "tcp"
)

// UptimeCheck probes a URL or TCP port from this computer.
type UptimeCheck struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Target      string `json:"target"` // URL for http, host:port for tcp
	IntervalSec int    `json:"intervalSec"`
	TimeoutSec  int    `json:"timeoutSec"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   int64  `json:"createdAt"`
}

// UptimeResult is one probe.
type UptimeResult struct {
	CheckID    string `json:"checkId"`
	At         int64  `json:"at"`
	OK         bool   `json:"ok"`
	LatencyMs  int64  `json:"latencyMs"`
	StatusCode int    `json:"statusCode"`
	Error      string `json:"error"`
}

// UptimeSummary is a check with its current state and availability.
type UptimeSummary struct {
	Check     UptimeCheck    `json:"check"`
	Up        *bool          `json:"up"`
	LastAt    int64          `json:"lastAt"`
	LatencyMs int64          `json:"latencyMs"`
	LastError string         `json:"lastError"`
	Uptime24h float64        `json:"uptime24h"` // -1 when there is no data
	Uptime7d  float64        `json:"uptime7d"`
	Uptime30d float64        `json:"uptime30d"`
	Recent    []UptimeResult `json:"recent"` // newest last
	// DownSince is when the current outage started (0 when up).
	DownSince int64 `json:"downSince"`
}
