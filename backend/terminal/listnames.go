package terminal

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Path completion lists folders on the computer a terminal runs on as the
// user types, so each listing must be quick: on a server it goes through one
// long-lived helper shell per session (one round trip, no new channel), and
// a relative folder is resolved there against the shell's directory.

// MaxNames caps a folder listing.
const MaxNames = 2000

const listTimeout = 5 * time.Second

// ErrNoDir means a folder doesn't exist or can't be opened.
var ErrNoDir = errors.New("that folder doesn't exist or can't be opened")

// listMarker ends each answer from the helper shell.
const listMarker = "\x1e"

// listFunc prints the entries of folder $1, one per line, folders (and links
// to folders) with a trailing /. A relative $1 is taken from the tmux pane $3
// if given, else folder $2 if given, else the home folder. It uses only sh
// builtins, so it works on BusyBox too, and runs in a subshell so the helper
// keeps no state.
var listFunc = `remvyn_ls() (
  d=$1
  case $d in "~"/*) d=$HOME/${d#??} ;; esac
  case $d in
  /*) ;;
  *)
    b=$2
    case $b in "~") b=$HOME ;; "~"/*) b=$HOME/${b#??} ;; esac
    if [ -n "$3" ]; then p=$(tmux -L remvyn display -p -t "$3" '#{pane_current_path}' 2>/dev/null) && [ -n "$p" ] && b=$p; fi
    if [ -n "$b" ]; then cd -- "$b" 2>/dev/null || cd; else cd; fi
    ;;
  esac
  cd -- "$d" 2>/dev/null || { echo 'remvyn:nodir'; exit 0; }
  for f in * .[!.]* ..?*; do
    [ -e "$f" ] || [ -L "$f" ] || continue
    if [ -d "$f" ]; then printf '%s/\n' "$f"; else printf '%s\n' "$f"; fi
  done | head -n ` + strconv.Itoa(MaxNames) + `
)
`

// ListNames lists folder dir for a session's path completion. dir may be
// absolute, ~/…, or relative to the shell's directory; base is the UI's best
// guess at that directory, used when the session can't tell.
func (m *Manager) ListNames(id, dir, base string) ([]string, error) {
	s, err := m.get(id)
	if err != nil {
		return nil, err
	}
	if s.client == nil {
		return listLocal(s, dir, base)
	}
	target := ""
	if s.persist != "" {
		target = "=" + sessionName(s.persist) + ":"
	}
	s.listMu.Lock()
	defer s.listMu.Unlock()
	if s.lister == nil {
		l, err := startLister(s.client)
		if err != nil {
			return nil, err
		}
		s.lister = l
	}
	out, err := s.lister.run("remvyn_ls " + quote(dir) + " " + quote(base) + " " + quote(target) + "; printf '" + listMarker + "\\n'\n")
	if err != nil {
		// Start a fresh helper next time.
		s.lister.close()
		s.lister = nil
		return nil, err
	}
	return ParseNames(out)
}

func listLocal(s *session, dir, base string) ([]string, error) {
	home, _ := os.UserHomeDir()
	if rest, ok := strings.CutPrefix(dir, "~/"); ok {
		dir = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(dir) {
		if s.dir != nil {
			if d := s.dir(); d != "" {
				base = d
			}
		}
		if base == "" {
			base = home
		}
		dir = filepath.Join(base, dir)
	}
	return ListNamesLocal(dir)
}

// lister is a helper shell on the server that answers listing requests.
type lister struct {
	sess  *ssh.Session
	stdin io.WriteCloser
	out   *bufio.Reader
}

func startLister(client interface {
	NewSession() (*ssh.Session, error)
}) (*lister, error) {
	sess, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	if err := sess.Start("sh"); err != nil {
		sess.Close()
		return nil, err
	}
	if _, err := io.WriteString(stdin, listFunc); err != nil {
		sess.Close()
		return nil, err
	}
	return &lister{sess: sess, stdin: stdin, out: newReader(stdout)}, nil
}

// run sends one command and returns its output up to the marker line.
func (l *lister) run(cmd string) (string, error) {
	if _, err := io.WriteString(l.stdin, cmd); err != nil {
		return "", err
	}
	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		var b strings.Builder
		for {
			line, err := l.out.ReadString('\n')
			if err != nil {
				done <- result{err: err}
				return
			}
			if line == listMarker+"\n" {
				done <- result{out: b.String()}
				return
			}
			if b.Len() < 1<<20 {
				b.WriteString(line)
			}
		}
	}()
	select {
	case r := <-done:
		return r.out, r.err
	case <-time.After(listTimeout):
		// Closing unblocks the reader.
		l.close()
		<-done
		return "", errors.New("listing that folder took too long")
	}
}

func (l *lister) close() {
	if l.sess != nil {
		_ = l.sess.Close()
	}
}

func newReader(r io.Reader) *bufio.Reader { return bufio.NewReader(r) }

// ParseNames turns listFunc output into sorted names.
func ParseNames(out string) ([]string, error) {
	if strings.HasPrefix(out, "remvyn:nodir") {
		return nil, ErrNoDir
	}
	names := []string{}
	for _, line := range strings.Split(out, "\n") {
		if line != "" && line != "/" {
			names = append(names, line)
		}
	}
	sortNames(names)
	return names, nil
}

// ListNamesLocal lists a folder on this computer like listFunc.
func ListNamesLocal(dir string) ([]string, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, ErrNoDir
	}
	names := make([]string, 0, min(len(des), MaxNames))
	for _, de := range des {
		if len(names) == MaxNames {
			break
		}
		name := de.Name()
		isDir := de.IsDir()
		if de.Type()&os.ModeSymlink != 0 {
			if fi, err := os.Stat(filepath.Join(dir, name)); err == nil {
				isDir = fi.IsDir()
			}
		}
		if isDir {
			name += "/"
		}
		names = append(names, name)
	}
	sortNames(names)
	return names, nil
}

// sortNames puts folders first, then sorts case-insensitively.
func sortNames(names []string) {
	sort.SliceStable(names, func(i, j int) bool {
		di, dj := strings.HasSuffix(names[i], "/"), strings.HasSuffix(names[j], "/")
		if di != dj {
			return di
		}
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
}

// quote quotes s as one POSIX shell word.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
