package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"remvyn/backend/ids"
	"remvyn/backend/mcp"
	"remvyn/backend/models"
	"remvyn/backend/sshconn"
)

// Output limits keep results within what an agent can usefully read.
const (
	agentStdoutMax       = 40 << 10
	agentStderrMax       = 16 << 10
	agentDefaultTimeout  = 120
	agentMaxTimeout      = 1800
	agentDefaultReadMax  = 256 << 10
	agentMaxRead         = 4 << 20
	agentMaxDirEntries   = 2000
	agentPreviewMax      = 3000
	agentWriteMaxContent = 8 << 20
)

// agentConn is a pooled connection for agent actions on one host.
type agentConn struct {
	client *sshconn.Client

	mu   sync.Mutex
	sftp *sftp.Client
	used time.Time
}

func (ac *agentConn) touch() {
	ac.mu.Lock()
	ac.used = time.Now()
	ac.mu.Unlock()
}

func (ac *agentConn) idleFor() time.Duration {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return time.Since(ac.used)
}

func (ac *agentConn) files() (*sftp.Client, error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.sftp == nil {
		sc, err := sftp.NewClient(ac.client.Client)
		if err != nil {
			return nil, fmt.Errorf("the server does not support SFTP: %w", err)
		}
		ac.sftp = sc
	}
	return ac.sftp, nil
}

func (ac *agentConn) close() {
	ac.mu.Lock()
	if ac.sftp != nil {
		ac.sftp.Close()
	}
	ac.mu.Unlock()
	ac.client.Close()
}

func (ac *agentConn) alive() bool {
	select {
	case <-ac.client.Done():
		return false
	default:
		return true
	}
}

// agentConn reuses or opens a connection. An unknown or changed host key is
// shown to the user only when they approved this action themselves; on hosts
// that run without approval it is refused, so an agent alone can never make
// Remvyn trust a new key.
func (c *Core) agentConn(ctx context.Context, h models.Host, userApproved bool) (*agentConn, error) {
	a := c.agent
	a.mu.Lock()
	if ac := a.conns[h.ID]; ac != nil && ac.alive() {
		a.mu.Unlock()
		ac.touch()
		return ac, nil
	}
	a.mu.Unlock()
	verify := c.quiet.Callback()
	if userApproved {
		verify = c.verifier.Callback()
	}
	client, _, err := c.dialWith(ctx, h.ID, models.ConnectOptions{}, verify)
	if err != nil {
		return nil, err
	}
	ac := &agentConn{client: client, used: time.Now()}
	a.mu.Lock()
	if prev := a.conns[h.ID]; prev != nil && prev.alive() {
		a.mu.Unlock()
		ac.close()
		prev.touch()
		return prev, nil
	}
	a.conns[h.ID] = ac
	a.mu.Unlock()
	return ac, nil
}

func buildAgentAction(tool string, raw json.RawMessage) (agentAction, error) {
	dec := func(v any) error {
		d := json.NewDecoder(bytes.NewReader(raw))
		if err := d.Decode(v); err != nil {
			return fmt.Errorf("invalid arguments: %w", err)
		}
		return nil
	}
	switch tool {
	case mcp.ToolRunCommand:
		var p struct {
			Command string `json:"command"`
			Cwd     string `json:"cwd"`
			Timeout int    `json:"timeout_seconds"`
		}
		if err := dec(&p); err != nil {
			return agentAction{}, err
		}
		if strings.TrimSpace(p.Command) == "" {
			return agentAction{}, errors.New("command is required")
		}
		if p.Timeout <= 0 {
			p.Timeout = agentDefaultTimeout
		}
		p.Timeout = min(p.Timeout, agentMaxTimeout)
		cmd := p.Command
		detail := p.Command
		if p.Cwd != "" {
			cmd = "cd " + shellPath(p.Cwd) + " && " + p.Command
			detail = "cd " + p.Cwd + " && " + p.Command
		}
		timeout := time.Duration(p.Timeout) * time.Second
		return agentAction{title: "run a command", detail: detail, run: func(ctx context.Context, ac *agentConn) agentOutcome {
			return runAgentCommand(ctx, ac, cmd, timeout)
		}}, nil

	case mcp.ToolReadFile:
		var p struct {
			Path     string `json:"path"`
			MaxBytes int64  `json:"max_bytes"`
			Encoding string `json:"encoding"`
		}
		if err := dec(&p); err != nil {
			return agentAction{}, err
		}
		if strings.TrimSpace(p.Path) == "" {
			return agentAction{}, errors.New("path is required")
		}
		if p.MaxBytes <= 0 {
			p.MaxBytes = agentDefaultReadMax
		}
		p.MaxBytes = min(p.MaxBytes, agentMaxRead)
		return agentAction{title: "read a file", detail: p.Path, run: func(ctx context.Context, ac *agentConn) agentOutcome {
			return readAgentFile(ac, p.Path, p.MaxBytes, p.Encoding == "base64")
		}}, nil

	case mcp.ToolWriteFile:
		var p struct {
			Path       string `json:"path"`
			Content    string `json:"content"`
			Encoding   string `json:"encoding"`
			Mode       string `json:"mode"`
			CreateDirs bool   `json:"create_dirs"`
		}
		if err := dec(&p); err != nil {
			return agentAction{}, err
		}
		if strings.TrimSpace(p.Path) == "" {
			return agentAction{}, errors.New("path is required")
		}
		data := []byte(p.Content)
		if p.Encoding == "base64" {
			var err error
			if data, err = base64.StdEncoding.DecodeString(p.Content); err != nil {
				return agentAction{}, errors.New("content is not valid base64")
			}
		}
		if len(data) > agentWriteMaxContent {
			return agentAction{}, fmt.Errorf("content is too large (limit %d MB)", agentWriteMaxContent>>20)
		}
		var mode os.FileMode
		if p.Mode != "" {
			m, err := strconv.ParseUint(p.Mode, 8, 32)
			if err != nil || m > 0o7777 {
				return agentAction{}, fmt.Errorf("mode must be octal permissions such as 644, not %q", p.Mode)
			}
			mode = os.FileMode(m)
		}
		preview := fmt.Sprintf("%d bytes", len(data))
		if mode != 0 {
			preview += fmt.Sprintf(", mode %04o", mode)
		}
		if isText(data) {
			preview += "\n" + trimMiddle(string(data), agentPreviewMax)
		} else {
			preview += " (binary)"
		}
		return agentAction{title: "write a file", detail: p.Path, preview: preview, run: func(ctx context.Context, ac *agentConn) agentOutcome {
			return writeAgentFile(ac, p.Path, data, mode, p.CreateDirs)
		}}, nil

	case mcp.ToolListDir:
		var p struct {
			Path string `json:"path"`
		}
		if err := dec(&p); err != nil {
			return agentAction{}, err
		}
		shown := p.Path
		if shown == "" {
			shown = "~"
		}
		return agentAction{title: "list a directory", detail: shown, run: func(ctx context.Context, ac *agentConn) agentOutcome {
			return listAgentDir(ac, p.Path)
		}}, nil
	}
	return agentAction{}, fmt.Errorf("unknown tool %q", tool)
}

// shellPath quotes a path for sh, keeping a leading ~/ expandable.
func shellPath(p string) string {
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
	switch {
	case p == "~":
		return "~"
	case strings.HasPrefix(p, "~/"):
		return "~/" + quote(p[2:])
	}
	return quote(p)
}

// sftpPath maps ~ and ~/x onto SFTP paths, which are relative to home.
func sftpPath(p string) string {
	switch {
	case p == "" || p == "~":
		return "."
	case strings.HasPrefix(p, "~/"):
		return p[2:]
	}
	return p
}

func isText(b []byte) bool { return utf8.Valid(b) && !bytes.ContainsRune(b, 0) }

// capture keeps the first and last part of a stream.
type capture struct {
	max   int
	head  []byte
	tail  []byte
	total int64
}

func (c *capture) Write(p []byte) (int, error) {
	c.total += int64(len(p))
	headMax := c.max * 2 / 3
	tailMax := c.max - headMax
	rest := p
	if n := headMax - len(c.head); n > 0 {
		n = min(n, len(rest))
		c.head = append(c.head, rest[:n]...)
		rest = rest[n:]
	}
	if len(rest) > 0 {
		c.tail = append(c.tail, rest...)
		if len(c.tail) > tailMax {
			c.tail = c.tail[len(c.tail)-tailMax:]
		}
	}
	return len(p), nil
}

func (c *capture) String() string {
	omitted := c.total - int64(len(c.head)) - int64(len(c.tail))
	if omitted <= 0 {
		return string(c.head) + string(c.tail)
	}
	return string(c.head) + fmt.Sprintf("\n… [%d bytes omitted] …\n", omitted) + string(c.tail)
}

func runAgentCommand(ctx context.Context, ac *agentConn, cmd string, timeout time.Duration) agentOutcome {
	sess, err := ac.client.NewSession()
	if err != nil {
		return agentOutcome{text: "could not start a session: " + err.Error(), isErr: true}
	}
	defer sess.Close()
	stdout := &capture{max: agentStdoutMax}
	stderr := &capture{max: agentStderrMax}
	sess.Stdout, sess.Stderr = stdout, stderr
	if err := sess.Start(cmd); err != nil {
		return agentOutcome{text: "could not run the command: " + err.Error(), isErr: true}
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- sess.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var note string
	select {
	case err = <-waitErr:
	case <-timer.C:
		note = fmt.Sprintf("The command timed out after %s and was stopped; it may still be running on the server.", timeout)
	case <-ctx.Done():
		note = "The command was cancelled; it may still be running on the server."
	}
	if note != "" {
		_ = sess.Signal(ssh.SIGKILL)
		sess.Close()
		select {
		case <-waitErr:
		case <-time.After(2 * time.Second):
		}
	}
	ac.touch()

	var b strings.Builder
	var exit *int
	switch {
	case note != "":
		b.WriteString(note + "\n")
	case err == nil:
		code := 0
		exit = &code
	default:
		var ee *ssh.ExitError
		var missing *ssh.ExitMissingError
		switch {
		case errors.As(err, &ee):
			code := ee.ExitStatus()
			exit = &code
			if ee.Signal() != "" {
				fmt.Fprintf(&b, "killed by signal %s\n", ee.Signal())
			}
		case errors.As(err, &missing):
			b.WriteString("the server did not report an exit status\n")
		default:
			fmt.Fprintf(&b, "the connection failed: %v\n", err)
		}
	}
	if exit != nil {
		fmt.Fprintf(&b, "exit code: %d\n", *exit)
	}
	if out := stdout.String(); out != "" {
		b.WriteString("--- stdout ---\n" + out)
		if !strings.HasSuffix(out, "\n") {
			b.WriteString("\n")
		}
	}
	if out := stderr.String(); out != "" {
		b.WriteString("--- stderr ---\n" + out)
	}
	if stdout.total == 0 && stderr.total == 0 && note == "" {
		b.WriteString("(no output)")
	}
	return agentOutcome{text: strings.TrimRight(b.String(), "\n"), exit: exit, isErr: exit == nil || *exit != 0}
}

func readAgentFile(ac *agentConn, p string, max int64, asBase64 bool) agentOutcome {
	sc, err := ac.files()
	if err != nil {
		return agentOutcome{text: err.Error(), isErr: true}
	}
	defer ac.touch()
	rp := sftpPath(p)
	fi, err := sc.Stat(rp)
	if err != nil {
		return agentOutcome{text: fmt.Sprintf("cannot read %s: %v", p, friendlySFTP(err)), isErr: true}
	}
	if fi.IsDir() {
		return agentOutcome{text: p + " is a directory; use list_dir", isErr: true}
	}
	f, err := sc.Open(rp)
	if err != nil {
		return agentOutcome{text: fmt.Sprintf("cannot read %s: %v", p, friendlySFTP(err)), isErr: true}
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max))
	if err != nil {
		return agentOutcome{text: fmt.Sprintf("error reading %s: %v", p, err), isErr: true}
	}
	var note string
	if fi.Size() > int64(len(data)) {
		note = fmt.Sprintf("\n[showing the first %d of %d bytes; pass a larger max_bytes to read more]", len(data), fi.Size())
	}
	if asBase64 {
		return agentOutcome{text: base64.StdEncoding.EncodeToString(data) + note}
	}
	if !isText(data) {
		return agentOutcome{text: fmt.Sprintf("%s looks like a binary file (%d bytes). Call read_file again with encoding \"base64\" to get its bytes.", p, fi.Size()), isErr: true}
	}
	if len(data) == 0 {
		return agentOutcome{text: "(empty file)"}
	}
	return agentOutcome{text: string(data) + note}
}

func writeAgentFile(ac *agentConn, p string, data []byte, mode os.FileMode, mkdirs bool) agentOutcome {
	sc, err := ac.files()
	if err != nil {
		return agentOutcome{text: err.Error(), isErr: true}
	}
	defer ac.touch()
	rp := sftpPath(p)
	if rp == "." || strings.HasSuffix(rp, "/") {
		return agentOutcome{text: "path must name a file", isErr: true}
	}
	dir, base := path.Split(rp)
	if dir == "" {
		dir = "."
	}
	if mkdirs {
		if err := sc.MkdirAll(dir); err != nil {
			return agentOutcome{text: fmt.Sprintf("could not create %s: %v", dir, friendlySFTP(err)), isErr: true}
		}
	}
	existed := false
	if fi, err := sc.Stat(rp); err == nil {
		if fi.IsDir() {
			return agentOutcome{text: p + " is a directory", isErr: true}
		}
		existed = true
		if mode == 0 {
			mode = fi.Mode().Perm()
		}
	}
	if mode == 0 {
		mode = 0o644
	}
	tmp := path.Join(dir, "."+base+".remvyn-"+ids.New()[:8])
	f, err := sc.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return agentOutcome{text: fmt.Sprintf("could not write in %s: %v", dir, friendlySFTP(err)), isErr: true}
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = sc.Chmod(tmp, mode)
	}
	if werr == nil {
		if werr = sc.PosixRename(tmp, rp); werr != nil {
			// Servers without the posix-rename extension can't replace a
			// file in one step.
			_ = sc.Remove(rp)
			werr = sc.Rename(tmp, rp)
		}
	}
	if werr != nil {
		_ = sc.Remove(tmp)
		return agentOutcome{text: fmt.Sprintf("could not write %s: %v", p, friendlySFTP(werr)), isErr: true}
	}
	verb := "created"
	if existed {
		verb = "replaced"
	}
	return agentOutcome{text: fmt.Sprintf("%s %s (%d bytes, mode %04o)", verb, p, len(data), mode)}
}

func listAgentDir(ac *agentConn, p string) agentOutcome {
	sc, err := ac.files()
	if err != nil {
		return agentOutcome{text: err.Error(), isErr: true}
	}
	defer ac.touch()
	rp := sftpPath(p)
	entries, err := sc.ReadDir(rp)
	if err != nil {
		return agentOutcome{text: fmt.Sprintf("cannot list %s: %v", p, friendlySFTP(err)), isErr: true}
	}
	full, _ := sc.RealPath(rp)
	if full == "" {
		full = p
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%d entries)\n", full, len(entries))
	for i, e := range entries {
		if i == agentMaxDirEntries {
			fmt.Fprintf(&b, "… %d more not shown\n", len(entries)-i)
			break
		}
		name := e.Name()
		switch {
		case e.IsDir():
			name += "/"
		case e.Mode()&os.ModeSymlink != 0:
			if target, err := sc.ReadLink(path.Join(rp, e.Name())); err == nil {
				name += " -> " + target
			}
		}
		fmt.Fprintf(&b, "%s %10d  %s  %s\n", e.Mode().String(), e.Size(), e.ModTime().Format("2006-01-02 15:04"), name)
	}
	return agentOutcome{text: strings.TrimRight(b.String(), "\n")}
}

func friendlySFTP(err error) string {
	var se *sftp.StatusError
	if errors.As(err, &se) {
		switch se.FxCode() {
		case sftp.ErrSSHFxNoSuchFile:
			return "no such file or directory"
		case sftp.ErrSSHFxPermissionDenied:
			return "permission denied"
		}
	}
	if errors.Is(err, os.ErrNotExist) {
		return "no such file or directory"
	}
	if errors.Is(err, os.ErrPermission) {
		return "permission denied"
	}
	return err.Error()
}
