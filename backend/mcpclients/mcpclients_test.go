package mcpclients

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var cmd = []string{`/Applications/Remvyn.app/Contents/MacOS/Remvyn`, "mcp"}

func TestSetJSONKeepsOtherSettings(t *testing.T) {
	in := []byte(`{"theme":"dark","mcpServers":{"other":{"command":"x"}},"z":1}`)
	out, err := setJSON(in, "mcpServers", stdioEntry(cmd))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Index(s, `"theme"`) > strings.Index(s, `"mcpServers"`) || strings.Index(s, `"mcpServers"`) > strings.Index(s, `"z"`) {
		t.Fatalf("key order changed:\n%s", s)
	}
	var doc struct {
		Theme      string                     `json:"theme"`
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(out, &doc); err != nil || doc.Theme != "dark" || doc.MCPServers["other"] == nil || doc.MCPServers[ServerName] == nil {
		t.Fatalf("bad result %s (%v)", s, err)
	}
	out, err = setJSON(out, "mcpServers", nil)
	if err != nil || strings.Contains(string(out), ServerName) || !strings.Contains(string(out), `"other"`) {
		t.Fatalf("remove failed: %s (%v)", out, err)
	}
	if _, err := setJSON([]byte("// comment\n{}"), "servers", vscodeEntry(cmd)); err == nil {
		t.Fatal("JSONC should be refused, not rewritten")
	}
	if out, err := setJSON(nil, "mcp", opencodeEntry(cmd)); err != nil || !strings.Contains(string(out), `"local"`) {
		t.Fatalf("new file: %s (%v)", out, err)
	}
}

func TestCodexTOML(t *testing.T) {
	in := "model = \"o3\"\n\n[mcp_servers.remvyn]\ncommand = \"/old/Remvyn\"\nargs = [\"mcp\"]\n\n[mcp_servers.remvyn.env]\nA = \"1\"\n\n[mcp_servers.github]\ncommand = \"gh\"\n"
	out := string(setCodex([]byte(in), cmd))
	if strings.Contains(out, "/old/Remvyn") || strings.Contains(out, "A = ") {
		t.Fatalf("old table kept:\n%s", out)
	}
	if !strings.Contains(out, "model = \"o3\"") || !strings.Contains(out, "[mcp_servers.github]\ncommand = \"gh\"") {
		t.Fatalf("other settings lost:\n%s", out)
	}
	if strings.Count(out, "[mcp_servers.remvyn]") != 1 || !strings.Contains(out, `command = "`+cmd[0]+`"`) || !strings.Contains(out, `args = ["mcp"]`) {
		t.Fatalf("missing new table:\n%s", out)
	}
	back := string(removeCodex([]byte(out)))
	if strings.Contains(back, "remvyn") || !strings.Contains(back, "[mcp_servers.github]") {
		t.Fatalf("remove failed:\n%s", back)
	}
	if got := tomlString(`C:\Program Files\Remvyn "x".exe`); got != `"C:\\Program Files\\Remvyn \"x\".exe"` {
		t.Fatalf("escape: %s", got)
	}
}

func TestConnectWritesConfigs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	t.Setenv("CODEX_HOME", "")
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"cursor", "codex", "gemini", "vscode", "opencode"} {
		if err := Connect(t.Context(), id, cmd); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	byID := map[string]bool{}
	for _, c := range List(cmd) {
		if c.Connected && !c.Current {
			t.Fatalf("%s connected but not current", c.ID)
		}
		byID[c.ID] = c.Connected
	}
	for _, id := range []string{"cursor", "codex", "gemini", "vscode", "opencode"} {
		if !byID[id] {
			t.Fatalf("%s not reported connected", id)
		}
	}
	if byID["windsurf"] {
		t.Fatal("windsurf should not be connected")
	}
	if c := List([]string{"/moved/Remvyn", "mcp"}); func() bool {
		for _, x := range c {
			if x.ID == "cursor" {
				return x.Current
			}
		}
		return true
	}() {
		t.Fatal("a moved app should show as out of date")
	}
	for _, id := range []string{"cursor", "codex"} {
		if err := Disconnect(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range List(cmd) {
		if (c.ID == "cursor" || c.ID == "codex") && c.Connected {
			t.Fatalf("%s still connected", c.ID)
		}
	}
}
