package remotedesk

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Error codes for RDP launch failures.
const (
	// CodeNoRDPApp means no app on this computer opens .rdp files (macOS) or
	// no supported RDP client is installed (Linux).
	CodeNoRDPApp = "no_rdp_app"
)

// CodedError carries a machine-readable code for the UI.
type CodedError struct {
	Code string
	Msg  string
}

func (e *CodedError) Error() string   { return e.Msg }
func (e *CodedError) ErrCode() string { return e.Code }

// RDPTarget is what the system client is pointed at.
type RDPTarget struct {
	Label    string
	Addr     string // host:port to connect to (the tunnel's local end, or the server)
	Username string
	Password string
}

// RDPLaunch is an RDP client started for one session. Close undoes what the
// launch left behind (temporary .rdp file, stored Windows credential); it
// doesn't quit the client app.
type RDPLaunch struct {
	// Client names the app that was opened.
	Client string

	mu      sync.Mutex
	cleanup []func()
}

// Close removes temporary files and credentials. Safe to call repeatedly.
func (l *RDPLaunch) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	fns := l.cleanup
	l.cleanup = nil
	l.mu.Unlock()
	for _, f := range fns {
		f()
	}
}

func (l *RDPLaunch) onClose(f func()) {
	l.mu.Lock()
	l.cleanup = append(l.cleanup, f)
	l.mu.Unlock()
}

// lookPath and command are swapped in tests.
var (
	lookPath = exec.LookPath
	command  = exec.Command
)

// LaunchRDP opens the system's RDP client for t.
func LaunchRDP(t RDPTarget) (*RDPLaunch, error) {
	switch runtime.GOOS {
	case "darwin":
		return launchMac(t)
	case "windows":
		return launchWindows(t)
	default:
		return launchLinux(t)
	}
}

// RDPFile is the content of a .rdp file for t. The password is never
// written; the RDP app asks for it (or uses its keychain).
func RDPFile(t RDPTarget) string {
	var b strings.Builder
	line := func(k, v string) { fmt.Fprintf(&b, "%s:%s\r\n", k, v) }
	line("full address:s", t.Addr)
	if t.Username != "" {
		line("username:s", t.Username)
	}
	line("prompt for credentials:i", "0")
	line("screen mode id:i", "1")
	line("use multimon:i", "0")
	line("session bpp:i", "32")
	return b.String()
}

func launchMac(t RDPTarget) (*RDPLaunch, error) {
	dir, err := os.MkdirTemp("", "remvyn-rdp-")
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(dir, 0o700)
	l := &RDPLaunch{Client: "Windows App"}
	l.onClose(func() { _ = os.RemoveAll(dir) })
	name := safeFileName(t.Label)
	if name == "" {
		name = "Remote Desktop"
	}
	path := filepath.Join(dir, name+".rdp")
	if err := os.WriteFile(path, []byte(RDPFile(t)), 0o600); err != nil {
		l.Close()
		return nil, err
	}
	out, err := command("open", path).CombinedOutput()
	if err != nil {
		l.Close()
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "No application") || strings.Contains(msg, "-10814") {
			return nil, &CodedError{Code: CodeNoRDPApp, Msg: "No app on this Mac opens Remote Desktop (.rdp) files. Install Windows App from the App Store."}
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("could not open the RDP app: %s", msg)
	}
	return l, nil
}

func launchWindows(t RDPTarget) (*RDPLaunch, error) {
	l := &RDPLaunch{Client: "Remote Desktop Connection"}
	host, _, err := net.SplitHostPort(t.Addr)
	if err != nil {
		host = t.Addr
	}
	if t.Username != "" && t.Password != "" {
		target := "TERMSRV/" + host
		c := command("cmdkey", "/generic:"+target, "/user:"+t.Username, "/pass:"+t.Password)
		hideWindow(c)
		if err := c.Run(); err == nil {
			l.onClose(func() {
				d := command("cmdkey", "/delete:"+target)
				hideWindow(d)
				_ = d.Run()
			})
		}
	}
	c := command("mstsc", "/v:"+t.Addr)
	if err := c.Start(); err != nil {
		l.Close()
		return nil, fmt.Errorf("could not start Remote Desktop Connection (mstsc): %w", err)
	}
	go func() { _ = c.Wait() }()
	return l, nil
}

func launchLinux(t RDPTarget) (*RDPLaunch, error) {
	for _, bin := range []string{"xfreerdp3", "xfreerdp"} {
		path, err := lookPath(bin)
		if err != nil {
			continue
		}
		args, stdin := freerdpArgs(bin, t)
		c := command(path, args...)
		if stdin != "" {
			c.Stdin = strings.NewReader(stdin)
		}
		if err := c.Start(); err != nil {
			return nil, fmt.Errorf("could not start %s: %w", bin, err)
		}
		go func() { _ = c.Wait() }()
		return &RDPLaunch{Client: bin}, nil
	}
	if path, err := lookPath("remmina"); err == nil {
		u := "rdp://"
		if t.Username != "" {
			u += t.Username + "@"
		}
		c := command(path, "-c", u+t.Addr)
		if err := c.Start(); err != nil {
			return nil, fmt.Errorf("could not start Remmina: %w", err)
		}
		go func() { _ = c.Wait() }()
		return &RDPLaunch{Client: "Remmina"}, nil
	}
	return nil, &CodedError{Code: CodeNoRDPApp, Msg: "No RDP client found. Install FreeRDP (xfreerdp3 or xfreerdp) or Remmina."}
}

// freerdpArgs builds FreeRDP's command line. The password is passed on
// stdin (/from-stdin), never as an argument other users could see.
func freerdpArgs(bin string, t RDPTarget) ([]string, string) {
	args := []string{"/v:" + t.Addr}
	if bin == "xfreerdp3" {
		args = append(args, "/cert:tofu")
	} else {
		args = append(args, "/cert-tofu")
	}
	user, domain, hasDomain := t.Username, "", false
	if i := strings.Index(user, `\`); i > 0 {
		domain, user, hasDomain = user[:i], user[i+1:], true
	}
	if user == "" {
		return args, ""
	}
	args = append(args, "/u:"+user)
	if hasDomain {
		args = append(args, "/d:"+domain)
	}
	if t.Password == "" {
		return args, ""
	}
	// FreeRDP asks for what's missing in order: domain (when not given),
	// then password.
	stdin := t.Password + "\n"
	if !hasDomain {
		stdin = "\n" + stdin
	}
	return append(args, "/from-stdin:force"), stdin
}

func safeFileName(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r < 32, strings.ContainsRune(`/\:*?"<>|`, r):
			return '-'
		}
		return r
	}, strings.TrimSpace(s))
	if len(s) > 60 {
		s = s[:60]
	}
	return strings.Trim(s, ". ")
}

// ErrClosed is returned for actions on a session that has ended.
var ErrClosed = errors.New("this remote desktop session has ended")
