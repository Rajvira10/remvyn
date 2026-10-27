package terminal

import (
	"strings"
	"testing"
)

func TestParseProbe(t *testing.T) {
	if st := parseProbe(""); st.ok || st.note != "tmux_missing" {
		t.Fatalf("no tmux: %+v", st)
	}
	if st := parseProbe("sh: tmux: not found\n"); st.ok || st.note != "tmux_missing" {
		t.Fatalf("not found: %+v", st)
	}
	if st := parseProbe("tmux 2.7\n"); st.ok || st.note != "tmux_old" {
		t.Fatalf("old tmux: %+v", st)
	}
	for _, v := range []string{"tmux 3.0a", "tmux 3.4", "tmux next-3.5", "tmux master", "tmux openbsd-7.5"} {
		if st := parseProbe(v + "\n"); !st.ok {
			t.Fatalf("%s should be usable: %+v", v, st)
		}
	}

	out := "tmux 3.4\n" +
		"remvyn-aaaaaaaaaaaa|1|1700000000|10|bash|/home/demo\n" +
		"remvyn-bbbbbbbbbbbb|0|1700000100|250|htop|/var/log\n" +
		"remvyn-bad id|0|1700000100|0|bash|/\n" +
		"work|0|1700000100|0|vim|/srv\n"
	st := parseProbe(out)
	if !st.ok || len(st.sessions) != 3 {
		t.Fatalf("sessions: %+v", st)
	}
	others := st.lookup("aaaaaaaaaaaa")
	if !st.exists || st.history != 10 {
		t.Fatalf("own session not found: %+v", st)
	}
	if len(others) != 1 || others[0].Key != "bbbbbbbbbbbb" || others[0].Command != "htop" || others[0].Path != "/var/log" ||
		others[0].LastActive != 1700000100000 || others[0].History != 250 {
		t.Fatalf("detached: %+v", others)
	}
}

func TestAttachCommandQuoting(t *testing.T) {
	cmd := attachCommand("abcdefgh12", 120, 40)
	for _, want := range []string{"tmux -L remvyn -f /dev/null start-server", `\; new-session -A -s remvyn-abcdefgh12 -x 120 -y 40`, "set -g prefix None"} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("attach command missing %q:\n%s", want, cmd)
		}
	}
}
