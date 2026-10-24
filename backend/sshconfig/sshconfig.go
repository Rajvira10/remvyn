// Package sshconfig reads concrete host entries from an OpenSSH client config.
package sshconfig

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Entry is one concrete `Host` block.
type Entry struct {
	Alias        string
	HostName     string
	User         string
	Port         int
	IdentityFile string
	ProxyJump    string
}

// Parse reads Host blocks. Wildcard patterns and Match blocks are skipped
// because they are templates, not connectable hosts; `Host *` defaults for
// User/Port/IdentityFile are applied to entries that don't set them.
func Parse(r io.Reader, home string) ([]Entry, []string) {
	var out []Entry
	var warnings []string
	var current []int // indices into out for the active Host block
	defaults := Entry{}
	inDefaults := false
	inMatch := false

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val := splitKV(line)
		if key == "" {
			continue
		}
		switch key {
		case "host":
			inMatch, inDefaults, current = false, false, nil
			for _, alias := range strings.Fields(val) {
				if alias == "*" {
					inDefaults = true
					continue
				}
				if strings.ContainsAny(alias, "*?!") {
					continue
				}
				out = append(out, Entry{Alias: alias})
				current = append(current, len(out)-1)
			}
			continue
		case "match":
			inMatch, inDefaults, current = true, false, nil
			continue
		case "include":
			warnings = append(warnings, "Include directives are not followed")
			continue
		}
		if inMatch {
			continue
		}
		if inDefaults {
			apply(&defaults, key, val, home)
		}
		for _, i := range current {
			apply(&out[i], key, val, home)
		}
	}
	for i := range out {
		e := &out[i]
		if e.User == "" {
			e.User = defaults.User
		}
		if e.Port == 0 {
			e.Port = defaults.Port
		}
		if e.IdentityFile == "" {
			e.IdentityFile = defaults.IdentityFile
		}
		if e.HostName == "" {
			e.HostName = e.Alias
		}
		if e.Port == 0 {
			e.Port = 22
		}
	}
	return out, warnings
}

func splitKV(line string) (string, string) {
	i := strings.IndexAny(line, " \t=")
	if i < 0 {
		return strings.ToLower(line), ""
	}
	key := strings.ToLower(line[:i])
	val := strings.TrimSpace(strings.TrimLeft(line[i:], " \t="))
	return key, strings.Trim(val, `"`)
}

func apply(e *Entry, key, val, home string) {
	switch key {
	case "hostname":
		if e.HostName == "" {
			e.HostName = val
		}
	case "user":
		if e.User == "" {
			e.User = val
		}
	case "port":
		if e.Port == 0 {
			if p, err := strconv.Atoi(val); err == nil {
				e.Port = p
			}
		}
	case "identityfile":
		if e.IdentityFile == "" {
			e.IdentityFile = expand(val, home)
		}
	case "proxyjump":
		e.ProxyJump = val
	}
}

func expand(p, home string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

// DefaultPath returns ~/.ssh/config.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh", "config")
}
