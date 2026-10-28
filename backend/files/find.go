package files

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"remvyn/backend/models"
)

// The terminal's path picker searches a whole tree, within these limits.
const (
	FindMaxFiles  = 50000
	FindMaxDirs   = 20000
	FindMaxPruned = 2000
	FindMaxDepth  = 12
	FindTimeout   = 8 * time.Second
)

// findPruned are directories listed but not searched: they are large and
// rarely what a command needs. Opening one in the picker searches it.
var findPruned = []string{".git", ".hg", ".svn", "node_modules", "vendor", "__pycache__", ".venv", "venv",
	".cache", ".next", ".nuxt", ".terraform", ".gradle", ".idea", "target"}

func isPruned(name string) bool {
	for _, p := range findPruned {
		if name == p {
			return true
		}
	}
	return false
}

// FindScript lists the tree under $1 on a server, for ParseFind. It runs
// under sh -s so it works whatever the login shell is, and uses only what
// every find has (BusyBox's included), so it walks the tree once per kind.
// Each line is a kind (D directory, P pruned directory, F other) and a path
// starting with "./".
func FindScript() string {
	names := make([]string, len(findPruned))
	for i, p := range findPruned {
		names[i] = "-name '" + p + "'"
	}
	f := "find . -mindepth 1 -maxdepth " + strconv.Itoa(FindMaxDepth) + ` -type d \( ` + strings.Join(names, " -o ") + ` \) -prune`
	part := func(kind string, test string, max int) string {
		return f + " " + test + " 2>/dev/null | head -n " + strconv.Itoa(max+1) + " | sed 's/^/" + kind + "/'\n"
	}
	return `cd -- "$1" 2>/dev/null || { echo 'remvyn:nodir'; exit 0; }
` + part("D", "-o -type d -print", FindMaxDirs) + part("P", "-print", FindMaxPruned) + part("F", "-o ! -type d -print", FindMaxFiles)
}

// ErrNoDir means the directory to search doesn't exist or can't be entered.
var ErrNoDir = errors.New("that folder doesn't exist or can't be opened")

// ParseFind turns FindScript output into an index of dir. complete is false
// when the output was cut short (by a timeout).
func ParseFind(dir, out string, complete bool) (models.FileIndex, error) {
	if strings.HasPrefix(out, "remvyn:nodir") {
		return models.FileIndex{}, ErrNoDir
	}
	idx := models.FileIndex{Dir: dir, Files: []string{}, Dirs: []string{}, Pruned: []string{}, Truncated: !complete}
	lines := strings.Split(out, "\n")
	// The last line may be partial unless the output ended cleanly.
	if !strings.HasSuffix(out, "\n") && !complete {
		lines = lines[:len(lines)-1]
	}
	add := func(list *[]string, max int, p string) {
		if len(*list) == max {
			idx.Truncated = true
			return
		}
		*list = append(*list, p)
	}
	for _, line := range lines {
		if len(line) < 4 || line[1:3] != "./" {
			continue
		}
		p := line[3:]
		switch line[0] {
		case 'D':
			add(&idx.Dirs, FindMaxDirs, p)
		case 'P':
			add(&idx.Pruned, FindMaxPruned, p)
		case 'F':
			add(&idx.Files, FindMaxFiles, p)
		}
	}
	return idx, nil
}

// FindLocal indexes the tree under dir on this computer, like FindScript.
func FindLocal(dir string) (models.FileIndex, error) {
	if dir == "" {
		dir = LocalHome()
	}
	dir = filepath.Clean(dir)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return models.FileIndex{}, ErrNoDir
	}
	idx := models.FileIndex{Dir: dir, Files: []string{}, Dirs: []string{}, Pruned: []string{}}
	deadline := time.Now().Add(FindTimeout)
	n := 0
	stop := errors.New("stop")
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if p == dir {
			return err
		}
		if err != nil {
			// Unreadable entries are skipped, as find does.
			return nil
		}
		if n++; n%500 == 0 && time.Now().After(deadline) {
			idx.Truncated = true
			return stop
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		full := func(list []string, max int) bool {
			if len(list) < max {
				return false
			}
			idx.Truncated = true
			return true
		}
		if !d.IsDir() {
			if full(idx.Files, FindMaxFiles) {
				return stop
			}
			idx.Files = append(idx.Files, rel)
			return nil
		}
		if isPruned(d.Name()) {
			if !full(idx.Pruned, FindMaxPruned) {
				idx.Pruned = append(idx.Pruned, rel)
			}
			return filepath.SkipDir
		}
		if full(idx.Dirs, FindMaxDirs) {
			return filepath.SkipDir
		}
		idx.Dirs = append(idx.Dirs, rel)
		if strings.Count(rel, "/")+1 >= FindMaxDepth {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil && err != stop {
		return models.FileIndex{}, err
	}
	return idx, nil
}
