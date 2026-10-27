package terminal

import (
	"bytes"
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"remvyn/backend/models"
	"remvyn/backend/sshconn"
)

// Sessions that survive disconnects run inside tmux on the server. Remvyn
// uses its own tmux server (socket "remvyn", no config file), so it never
// touches the user's tmux sessions or settings, and configures it to be
// invisible: no status bar, no prefix key, and output flowing into the
// terminal's own scrollback instead of tmux's alternate screen.

// tmuxPrefix starts every session name Remvyn creates.
const tmuxPrefix = "remvyn-"

const tmuxBin = "tmux -L remvyn -f /dev/null"

// tmuxSetup configures the private server. It runs before every attach;
// setting options that are already set is harmless.
var tmuxSetup = strings.Join([]string{
	"start-server",
	"set -g status off",
	"set -g prefix None",
	"set -g prefix2 None",
	"set -g escape-time 10",
	"set -g history-limit 10000",
	"set -g default-terminal screen-256color",
	"set -g terminal-overrides 'xterm*:smcup@:rmcup@:Tc'",
	"set-environment -g COLORTERM truecolor",
	"set-environment -g TERM_PROGRAM Remvyn",
}, ` \; `)

// Persist asks Start to run the shell inside a server-side session that
// outlives the connection.
type Persist struct {
	// Key names the session (a client-chosen ID; see ValidID).
	Key string
	// Restore replays the session's scrollback before attaching, for a
	// terminal that starts out empty.
	Restore bool
}

// tmuxState is what the probe learned about the server.
type tmuxState struct {
	ok       bool   // tmux ≥ 3.0 is installed
	note     string // why persistence is unavailable, if !ok
	exists   bool   // the requested session is already running
	history  int    // its scrollback lines
	sessions []models.ServerSession
}

var tmuxVersion = regexp.MustCompile(`^tmux (?:next-)?(\d+)\.(\d+)`)

func sessionName(key string) string { return tmuxPrefix + key }

// runOutput runs cmd on the server and returns its stdout.
func runOutput(ctx context.Context, client *sshconn.Client, cmd string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	var out bytes.Buffer
	sess.Stdout = &out
	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()
	select {
	case err := <-done:
		return out.String(), err
	case <-ctx.Done():
		return out.String(), ctx.Err()
	}
}

// probeTmux checks for tmux and lists Remvyn's sessions in one round trip.
// The command is valid in POSIX shells and fish.
func probeTmux(ctx context.Context, client *sshconn.Client) tmuxState {
	cmd := "tmux -V 2>/dev/null; " + tmuxBin + ` ls -F '#{session_name}|#{session_attached}|#{session_activity}|#{history_size}|#{pane_current_command}|#{pane_current_path}' 2>/dev/null`
	out, _ := runOutput(ctx, client, cmd)
	return parseProbe(out)
}

func parseProbe(out string) tmuxState {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	first := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(first, "tmux ") {
		return tmuxState{note: "tmux_missing"}
	}
	if m := tmuxVersion.FindStringSubmatch(first); m != nil {
		if major, _ := strconv.Atoi(m[1]); major < 3 {
			return tmuxState{note: "tmux_old"}
		}
	}
	// Other version strings ("tmux master", "tmux openbsd-7.5") are recent builds.
	st := tmuxState{ok: true}
	for _, l := range lines[1:] {
		f := strings.SplitN(strings.TrimSpace(l), "|", 6)
		if len(f) < 6 || !strings.HasPrefix(f[0], tmuxPrefix) {
			continue
		}
		activity, _ := strconv.ParseInt(f[2], 10, 64)
		history, _ := strconv.Atoi(f[3])
		st.sessions = append(st.sessions, models.ServerSession{
			Key:        strings.TrimPrefix(f[0], tmuxPrefix),
			Attached:   f[1] != "0",
			LastActive: activity * 1000,
			History:    history,
			Command:    f[4],
			Path:       f[5],
		})
	}
	return st
}

// lookup marks key as existing and returns the other, detached sessions.
func (st *tmuxState) lookup(key string) []models.ServerSession {
	var others []models.ServerSession
	for _, s := range st.sessions {
		switch {
		case s.Key == key:
			st.exists = true
			st.history = s.History
		case !s.Attached && ValidID(s.Key):
			others = append(others, s)
		}
	}
	return others
}

// agentLink is a stable path to the forwarded agent socket for a session.
// Shells in tmux outlive the connection that started them, and each new
// connection gets a new socket, so they use this link, which every attach
// points at the current socket.
func agentLink(key string) string { return "$HOME/.ssh/remvyn-agent-" + key + ".sock" }

// attachCommand creates the session if needed and attaches to it. With
// forward, a new session's shell uses the agent link.
func attachCommand(key string, cols, rows int, forward bool) string {
	cmd := tmuxBin + " " + tmuxSetup + ` \; new-session -A -s ` + sessionName(key) +
		" -x " + strconv.Itoa(cols) + " -y " + strconv.Itoa(rows)
	if !forward {
		return cmd
	}
	link := agentLink(key)
	// Expanded by the login shell (POSIX or fish) before tmux runs it.
	return `mkdir -p "$HOME/.ssh" 2>/dev/null; ln -sfn "$SSH_AUTH_SOCK" "` + link + `" 2>/dev/null; ` +
		cmd + ` "exec env SSH_AUTH_SOCK=` + link + ` $SHELL -l"`
}

// captureHistory returns the session's scrollback (without the visible
// screen, which attaching redraws), with colors, as terminal output.
func captureHistory(ctx context.Context, client *sshconn.Client, key string, lines int) string {
	out, err := runOutput(ctx, client, tmuxBin+" capture-pane -p -e -J -S -"+strconv.Itoa(lines)+" -E -1 -t ="+sessionName(key)+":")
	if err != nil || out == "" {
		return ""
	}
	return strings.ReplaceAll(strings.TrimRight(out, "\n"), "\n", "\r\n") + "\r\n"
}

// KillServerSession ends a Remvyn session on the server. Ending one that is
// already gone is not an error.
func KillServerSession(client *sshconn.Client, key string) error {
	if !ValidID(key) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := runOutput(ctx, client, tmuxBin+" kill-session -t ="+sessionName(key)+` 2>/dev/null; rm -f "`+agentLink(key)+`"; true`)
	return err
}

// ListServerSessions returns Remvyn's sessions on the server; nil when tmux
// isn't available.
func ListServerSessions(client *sshconn.Client) []models.ServerSession {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return probeTmux(ctx, client).sessions
}
