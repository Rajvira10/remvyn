// Package mcpclients adds Remvyn's MCP server to the AI tools installed on
// this computer (Claude Code, Codex, Gemini CLI, Cursor, Claude Desktop,
// VS Code, Windsurf, opencode) by writing each tool's own config, so the
// user doesn't have to find and edit those files by hand.
package mcpclients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"remvyn/backend/models"
)

// ServerName is the name Remvyn's server is registered under.
const ServerName = "remvyn"

type format int

const (
	fmtCLI      format = iota // managed through the tool's own CLI (Claude Code)
	fmtJSON                   // {"<key>": {"remvyn": {"command", "args"}}}
	fmtVSCode                 // {"servers": {"remvyn": {"type": "stdio", ...}}}
	fmtOpencode               // {"mcp": {"remvyn": {"type": "local", "command": [...]}}}
	fmtTOML                   // Codex: [mcp_servers.remvyn]
)

type client struct {
	id, name string
	format   format
	// path is the config file; dir must exist for the tool to count as installed.
	path func(env) string
	dir  func(env) string
	bin  string // a CLI whose presence also counts as installed
	key  string // fmtJSON: the top-level key
	// manual is what the user would do by hand.
	manual func(cmd []string) (kind, text string)
}

type env struct{ home, appData, configDir string }

func currentEnv() env {
	home, _ := os.UserHomeDir()
	cfg, _ := os.UserConfigDir() // ~/Library/Application Support, %AppData%, ~/.config
	return env{home: home, appData: os.Getenv("APPDATA"), configDir: cfg}
}

var plainArg = regexp.MustCompile(`^[\w@%+=:,./-]+$`)

func shellJoin(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		switch {
		case plainArg.MatchString(a):
			out[i] = a
		case runtime.GOOS == "windows":
			out[i] = `"` + a + `"`
		default:
			out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(out, " ")
}

func jsonSnippet(key string, entry any) string {
	b, _ := json.MarshalIndent(map[string]any{key: map[string]any{ServerName: entry}}, "", "  ")
	return string(b)
}

func stdioEntry(cmd []string) map[string]any {
	return map[string]any{"command": cmd[0], "args": cmd[1:]}
}

func vscodeEntry(cmd []string) map[string]any {
	return map[string]any{"type": "stdio", "command": cmd[0], "args": cmd[1:]}
}

func opencodeEntry(cmd []string) map[string]any {
	return map[string]any{"type": "local", "command": cmd, "enabled": true}
}

func codexHome(e env) string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return h
	}
	return filepath.Join(e.home, ".codex")
}

func clients() []client {
	jsonClient := func(id, name string, path func(env) string, manualKey string) client {
		return client{
			id: id, name: name, format: fmtJSON, key: "mcpServers",
			path: path, dir: func(e env) string { return filepath.Dir(path(e)) },
			manual: func(cmd []string) (string, string) { return "json", jsonSnippet(manualKey, stdioEntry(cmd)) },
		}
	}
	claudeDesktop := func(e env) string {
		switch runtime.GOOS {
		case "windows":
			return filepath.Join(e.appData, "Claude", "claude_desktop_config.json")
		case "darwin":
			return filepath.Join(e.home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
		default:
			return filepath.Join(e.home, ".config", "Claude", "claude_desktop_config.json")
		}
	}
	vscode := func(e env) string { return filepath.Join(e.configDir, "Code", "User", "mcp.json") }
	opencode := func(e env) string { return filepath.Join(e.home, ".config", "opencode", "opencode.json") }

	gemini := jsonClient("gemini", "Gemini CLI", func(e env) string { return filepath.Join(e.home, ".gemini", "settings.json") }, "mcpServers")
	gemini.bin = "gemini"
	gemini.manual = func(cmd []string) (string, string) {
		return "command", "gemini mcp add --scope user " + ServerName + " " + shellJoin(cmd)
	}

	return []client{
		{
			id: "claude-code", name: "Claude Code", format: fmtCLI, bin: "claude",
			path: func(e env) string { return filepath.Join(e.home, ".claude.json") },
			dir:  func(e env) string { return filepath.Join(e.home, ".claude") },
			manual: func(cmd []string) (string, string) {
				return "command", "claude mcp add --scope user " + ServerName + " -- " + shellJoin(cmd)
			},
		},
		{
			id: "codex", name: "Codex", format: fmtTOML, bin: "codex",
			path: func(e env) string { return filepath.Join(codexHome(e), "config.toml") },
			dir:  codexHome,
			manual: func(cmd []string) (string, string) {
				return "command", "codex mcp add " + ServerName + " -- " + shellJoin(cmd)
			},
		},
		gemini,
		jsonClient("cursor", "Cursor", func(e env) string { return filepath.Join(e.home, ".cursor", "mcp.json") }, "mcpServers"),
		jsonClient("claude-desktop", "Claude Desktop", claudeDesktop, "mcpServers"),
		{
			id: "vscode", name: "GitHub Copilot (VS Code)", format: fmtVSCode, path: vscode,
			dir:    func(e env) string { return filepath.Dir(vscode(e)) },
			manual: func(cmd []string) (string, string) { return "json", jsonSnippet("servers", vscodeEntry(cmd)) },
		},
		jsonClient("windsurf", "Windsurf", func(e env) string {
			return filepath.Join(e.home, ".codeium", "windsurf", "mcp_config.json")
		}, "mcpServers"),
		{
			id: "opencode", name: "opencode", format: fmtOpencode, path: opencode, bin: "opencode",
			dir:    func(e env) string { return filepath.Dir(opencode(e)) },
			manual: func(cmd []string) (string, string) { return "json", jsonSnippet("mcp", opencodeEntry(cmd)) },
		},
	}
}

func find(id string) (client, bool) {
	for _, c := range clients() {
		if c.id == id {
			return c, true
		}
	}
	return client{}, false
}

// List describes every supported tool: installed or not, and whether
// Remvyn is already in its config.
func List(cmd []string) []models.MCPClient {
	e := currentEnv()
	out := []models.MCPClient{}
	for _, c := range clients() {
		kind, text := c.manual(cmd)
		m := models.MCPClient{ID: c.id, Name: c.name, ConfigPath: c.path(e), ManualKind: kind, Manual: text}
		m.Installed = exists(c.dir(e)) || exists(c.path(e)) || (c.bin != "" && lookPath(c.bin) != "")
		m.Connected, m.Current = c.status(e, cmd)
		out = append(out, m)
	}
	return out
}

// Connect adds (or updates) Remvyn in a tool's config.
func Connect(ctx context.Context, id string, cmd []string) error {
	c, ok := find(id)
	if !ok {
		return fmt.Errorf("unknown AI tool %q", id)
	}
	e := currentEnv()
	switch c.format {
	case fmtCLI:
		bin := lookPath(c.bin)
		if bin == "" {
			return fmt.Errorf("couldn't find the %s command. Run the command shown under “Set up by hand” in a terminal instead", c.bin)
		}
		_, _ = runCLI(ctx, bin, "mcp", "remove", "--scope", "user", ServerName)
		args := append([]string{"mcp", "add", "--scope", "user", ServerName, "--"}, cmd...)
		if out, err := runCLI(ctx, bin, args...); err != nil {
			return fmt.Errorf("claude mcp add failed: %s", firstLine(out, err))
		}
		return nil
	case fmtTOML:
		return editFile(c.path(e), func(b []byte) ([]byte, error) { return setCodex(b, cmd), nil })
	default:
		key, entry := c.entry(cmd)
		return editFile(c.path(e), func(b []byte) ([]byte, error) { return setJSON(b, key, entry) })
	}
}

// Disconnect removes Remvyn from a tool's config.
func Disconnect(ctx context.Context, id string) error {
	c, ok := find(id)
	if !ok {
		return fmt.Errorf("unknown AI tool %q", id)
	}
	e := currentEnv()
	switch c.format {
	case fmtCLI:
		bin := lookPath(c.bin)
		if bin == "" {
			return fmt.Errorf("couldn't find the %s command. Run `claude mcp remove --scope user %s` in a terminal", c.bin, ServerName)
		}
		if out, err := runCLI(ctx, bin, "mcp", "remove", "--scope", "user", ServerName); err != nil {
			return fmt.Errorf("claude mcp remove failed: %s", firstLine(out, err))
		}
		return nil
	case fmtTOML:
		if !exists(c.path(e)) {
			return nil
		}
		return editFile(c.path(e), func(b []byte) ([]byte, error) { return removeCodex(b), nil })
	default:
		if !exists(c.path(e)) {
			return nil
		}
		key, _ := c.entry(nil)
		return editFile(c.path(e), func(b []byte) ([]byte, error) { return setJSON(b, key, nil) })
	}
}

func (c client) entry(cmd []string) (string, any) {
	if cmd == nil {
		cmd = []string{""}
	}
	switch c.format {
	case fmtVSCode:
		return "servers", vscodeEntry(cmd)
	case fmtOpencode:
		return "mcp", opencodeEntry(cmd)
	default:
		return c.key, stdioEntry(cmd)
	}
}

// status reports whether Remvyn is configured, and whether it points at
// this copy of Remvyn (an old path after moving the app is not current).
func (c client) status(e env, cmd []string) (connected, current bool) {
	raw, err := os.ReadFile(c.path(e))
	if err != nil {
		return false, false
	}
	switch c.format {
	case fmtTOML:
		sec := codexSection(raw)
		if sec == "" {
			return false, false
		}
		return true, strings.Contains(sec, tomlString(cmd[0]))
	case fmtCLI:
		c.key = "mcpServers"
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(raw, &doc) != nil {
		return false, false
	}
	key, _ := c.entry(cmd)
	var servers map[string]json.RawMessage
	if json.Unmarshal(doc[key], &servers) != nil || servers[ServerName] == nil {
		return false, false
	}
	var got struct {
		Command json.RawMessage `json:"command"`
	}
	_ = json.Unmarshal(servers[ServerName], &got)
	var s string
	var list []string
	if json.Unmarshal(got.Command, &s) == nil {
		return true, s == cmd[0]
	}
	if json.Unmarshal(got.Command, &list) == nil && len(list) > 0 {
		return true, list[0] == cmd[0]
	}
	return true, false
}

// ---------------------------------------------------------------- files

func exists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// editFile rewrites a config file atomically, keeping its permissions.
func editFile(path string, edit func([]byte) ([]byte, error)) error {
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	out, err := edit(raw)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".remvyn-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// setJSON sets (or, with entry nil, removes) <key>.remvyn and leaves every
// other value as it was.
func setJSON(raw []byte, key string, entry any) ([]byte, error) {
	doc := map[string]json.RawMessage{}
	var order []string
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, errors.New("the config file isn't plain JSON (it may contain comments), so Remvyn won't rewrite it. Add the snippet under “Set up by hand” yourself")
		}
		order = keyOrder(raw)
	}
	servers := map[string]json.RawMessage{}
	var serverOrder []string
	if v, ok := doc[key]; ok && string(v) != "null" {
		if err := json.Unmarshal(v, &servers); err != nil {
			return nil, fmt.Errorf("%q in the config file isn't an object", key)
		}
		serverOrder = keyOrder(v)
	}
	if entry == nil {
		if _, ok := servers[ServerName]; !ok {
			return raw, nil
		}
		delete(servers, ServerName)
	} else {
		b, _ := json.Marshal(entry)
		servers[ServerName] = b
	}
	doc[key] = marshalOrdered(servers, serverOrder)
	out := marshalOrdered(doc, order)
	var buf bytes.Buffer
	if err := json.Indent(&buf, out, "", "  "); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// keyOrder returns an object's keys in file order.
func keyOrder(raw []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil
	}
	var keys []string
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return keys
		}
		k, _ := t.(string)
		keys = append(keys, k)
		var skip json.RawMessage
		if dec.Decode(&skip) != nil {
			return keys
		}
	}
	return keys
}

func marshalOrdered(m map[string]json.RawMessage, order []string) []byte {
	var b bytes.Buffer
	b.WriteByte('{')
	seen := map[string]bool{}
	n := 0
	write := func(k string) {
		v, ok := m[k]
		if !ok || seen[k] {
			return
		}
		seen[k] = true
		if n > 0 {
			b.WriteByte(',')
		}
		n++
		kb, _ := json.Marshal(k)
		b.Write(kb)
		b.WriteByte(':')
		b.Write(v)
	}
	for _, k := range order {
		write(k)
	}
	rest := make([]string, 0, len(m))
	for k := range m {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		write(k)
	}
	b.WriteByte('}')
	return b.Bytes()
}

// ---------------------------------------------------------------- Codex TOML

var tomlHeader = regexp.MustCompile(`^\s*\[\[?\s*([^\]]+?)\s*\]\]?\s*(#.*)?$`)

// isOurs reports whether a table name is mcp_servers.remvyn or one of its
// sub-tables (mcp_servers.remvyn.env).
func isOurs(name string) bool {
	name = strings.ReplaceAll(name, " ", "")
	for _, p := range []string{"mcp_servers." + ServerName, `mcp_servers."` + ServerName + `"`} {
		if name == p || strings.HasPrefix(name, p+".") {
			return true
		}
	}
	return false
}

// splitCodex returns the file without Remvyn's tables, and those tables.
func splitCodex(raw []byte) (rest, ours string) {
	var keep, mine []string
	in := false
	for _, line := range strings.SplitAfter(string(raw), "\n") {
		if m := tomlHeader.FindStringSubmatch(line); m != nil {
			in = isOurs(m[1])
		}
		if in {
			mine = append(mine, line)
		} else {
			keep = append(keep, line)
		}
	}
	return strings.Join(keep, ""), strings.Join(mine, "")
}

func codexSection(raw []byte) string { _, s := splitCodex(raw); return s }

func tomlString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func setCodex(raw []byte, cmd []string) []byte {
	rest, _ := splitCodex(raw)
	rest = strings.TrimRight(rest, "\n")
	args := make([]string, len(cmd)-1)
	for i, a := range cmd[1:] {
		args[i] = tomlString(a)
	}
	var b strings.Builder
	if rest != "" {
		b.WriteString(rest)
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "[mcp_servers.%s]\ncommand = %s\nargs = [%s]\n", ServerName, tomlString(cmd[0]), strings.Join(args, ", "))
	return []byte(b.String())
}

func removeCodex(raw []byte) []byte {
	rest, ours := splitCodex(raw)
	if ours == "" {
		return raw
	}
	return []byte(strings.TrimRight(rest, "\n") + "\n")
}

// ---------------------------------------------------------------- CLIs

var (
	shellPathOnce sync.Once
	shellPath     string
)

// searchPath is PATH as the user's terminal sees it. Apps started from the
// Dock or Finder get a bare PATH that misses Homebrew, npm and ~/.local/bin.
func searchPath() string {
	shellPathOnce.Do(func() {
		shellPath = os.Getenv("PATH")
		if runtime.GOOS == "windows" {
			return
		}
		home, _ := os.UserHomeDir()
		extra := []string{
			filepath.Join(home, ".local", "bin"), filepath.Join(home, ".claude", "local"),
			"/opt/homebrew/bin", "/usr/local/bin", filepath.Join(home, ".npm-global", "bin"),
			filepath.Join(home, ".bun", "bin"), filepath.Join(home, ".volta", "bin"),
		}
		sh := os.Getenv("SHELL")
		if sh == "" {
			sh = "/bin/zsh"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, sh, "-ilc", `printf '\n__REMVYN_PATH__%s__REMVYN_PATH__\n' "$PATH"`).Output()
		if err == nil {
			if m := regexp.MustCompile(`__REMVYN_PATH__(.*?)__REMVYN_PATH__`).FindSubmatch(out); m != nil {
				shellPath = string(m[1])
			}
		}
		shellPath = strings.Join(append([]string{shellPath}, extra...), string(os.PathListSeparator))
	})
	return shellPath
}

func lookPath(name string) string {
	if runtime.GOOS == "windows" {
		p, _ := exec.LookPath(name)
		return p
	}
	for _, dir := range filepath.SplitList(searchPath()) {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

func runCLI(ctx context.Context, bin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(bin)+string(os.PathListSeparator)+searchPath())
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func firstLine(out string, err error) string {
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return err.Error()
}
