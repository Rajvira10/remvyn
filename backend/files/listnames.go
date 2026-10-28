package files

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// MaxNames caps a folder listing for the terminal's path completion.
const MaxNames = 2000

// ErrNoDir means a folder doesn't exist or can't be opened.
var ErrNoDir = errors.New("that folder doesn't exist or can't be opened")

// ListNamesScript prints the entries of $1 on a server, one per line, with
// a trailing / on folders (including links to folders). It uses only sh
// builtins, so it works on BusyBox too.
var ListNamesScript = `cd -- "$1" 2>/dev/null || { echo 'remvyn:nodir'; exit 0; }
for f in * .[!.]* ..?*; do
  [ -e "$f" ] || [ -L "$f" ] || continue
  if [ -d "$f" ]; then printf '%s/\n' "$f"; else printf '%s\n' "$f"; fi
done | head -n ` + strconv.Itoa(MaxNames) + `
`

// ParseNames turns ListNamesScript output into sorted names.
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

// ListNamesLocal lists a folder on this computer like ListNamesScript.
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
