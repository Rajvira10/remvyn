package core

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"remvyn/backend/storage"
)

// CommandHistory lists the commands typed in a host's terminals, most recent
// first.
func (c *Core) CommandHistory(hostID string) ([]string, error) {
	return c.store.CommandHistory(context.Background(), hostID)
}

// AddCommand records a command typed in a host's terminal.
func (c *Core) AddCommand(hostID, cmd string) error {
	return c.store.AddCommand(context.Background(), hostID, cmd)
}

// ForgetCommand removes a command from a host's history.
func (c *Core) ForgetCommand(hostID, cmd string) error {
	return c.store.ForgetCommand(context.Background(), hostID, cmd)
}

// ClearCommandHistory deletes the command history of every host.
func (c *Core) ClearCommandHistory() error {
	return c.store.ClearCommandHistory(context.Background())
}

// historyMarker starts each file in the output of remoteHistoryScript.
const historyMarker = "\x1e"

// remoteHistoryScript prints the end of each shell history file it can read.
// It runs under /bin/sh, so $HISTFILE (set by interactive shells) is unknown
// and the default locations are used.
const remoteHistoryScript = `for f in "$HOME/.bash_history" "$HOME/.zsh_history" "${XDG_DATA_HOME:-$HOME/.local/share}/fish/fish_history"; do
  [ -r "$f" ] || continue
  printf '\036%s\n' "$f"
  tail -n 4000 "$f" 2>/dev/null
done
`

// RemoteShellHistory reads the server's own shell history over a terminal
// session's connection, so suggestions work before anything is typed in
// Remvyn. It is returned most recent first and never stored.
func (c *Core) RemoteShellHistory(sessionID string) ([]string, error) {
	client, err := c.terms.Client(sessionID)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("not an SSH session")
	}
	sess, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	var out bytes.Buffer
	sess.Stdin = strings.NewReader(remoteHistoryScript)
	sess.Stdout = &limitWriter{w: &out, n: 4 << 20}
	done := make(chan error, 1)
	go func() { done <- sess.Run("sh -s") }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	select {
	case <-done:
	case <-ctx.Done():
		return nil, errors.New("reading shell history took too long")
	}
	return parseShellHistory(out.String()), nil
}

var (
	zshExtended = regexp.MustCompile(`^: \d+:\d+;`)
	bashStamp   = regexp.MustCompile(`^#\d+$`)
)

// parseShellHistory turns remoteHistoryScript output into distinct commands,
// most recent first. Multi-line entries are skipped.
func parseShellHistory(raw string) []string {
	var files [][]string
	for _, chunk := range strings.Split(raw, historyMarker)[1:] {
		name, body, _ := strings.Cut(chunk, "\n")
		fish := strings.HasSuffix(name, "/fish_history")
		var cmds []string
		cont := false
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSuffix(line, "\r")
			// A trailing backslash continues a multi-line command; skip all
			// of its lines.
			if !fish {
				prev := cont
				cont = strings.HasSuffix(line, `\`)
				if prev || cont {
					continue
				}
			}
			if fish {
				cmd, ok := strings.CutPrefix(line, "- cmd: ")
				if !ok || strings.Contains(cmd, `\n`) {
					continue
				}
				line = strings.ReplaceAll(cmd, `\\`, `\`)
			} else {
				if bashStamp.MatchString(line) {
					continue
				}
				line = zshExtended.ReplaceAllString(line, "")
			}
			line = strings.TrimRight(line, " \t")
			// Invalid UTF-8 includes zsh's metafied non-ASCII text.
			if strings.TrimSpace(line) == "" || len(line) > storage.MaxCommandLen || !utf8.ValidString(line) {
				continue
			}
			cmds = append(cmds, line)
		}
		files = append(files, cmds)
	}
	seen := map[string]bool{}
	out := []string{}
	for _, cmds := range files {
		for i := len(cmds) - 1; i >= 0 && len(out) < storage.MaxHistory; i-- {
			if !seen[cmds[i]] {
				seen[cmds[i]] = true
				out = append(out, cmds[i])
			}
		}
	}
	return out
}

// limitWriter drops output past n bytes.
type limitWriter struct {
	w *bytes.Buffer
	n int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if rest := l.n - l.w.Len(); rest > 0 {
		l.w.Write(p[:min(len(p), rest)])
	}
	return len(p), nil
}
