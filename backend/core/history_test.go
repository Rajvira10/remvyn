package core

import (
	"reflect"
	"testing"
)

func TestParseShellHistory(t *testing.T) {
	raw := "\x1e/home/u/.bash_history\n" +
		"#1700000000\nls\ncd /var/www\nls\nfor i in 1 2; do \\\n  echo $i; \\\n done\n\n" +
		"\x1e/home/u/.zsh_history\n" +
		": 1700000001:0;git status\r\n: 1700000002:3;docker ps\n\xe9t\xe9\n" +
		"\x1e/home/u/.local/share/fish/fish_history\n" +
		"- cmd: echo a\\\\b\n  when: 1700000003\n- cmd: printf 'x\\ny'\n"
	got := parseShellHistory(raw)
	want := []string{"ls", "cd /var/www", "docker ps", "git status", `echo a\b`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if got := parseShellHistory(""); len(got) != 0 {
		t.Fatalf("empty = %q", got)
	}
}
