package sshconfig

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cfg := `
# comment
Host *
    User fallback
    Port 2222

Host a b
    HostName 1.2.3.4
    IdentityFile ~/.ssh/id_a

Host c
    HostName=c.example.com
    User "carol"
    Port 22

Host *.wild
    User x

Match host foo
    User matched

Include other
`
	entries, warnings := Parse(strings.NewReader(cfg), "/home/u")
	if len(entries) != 3 {
		t.Fatalf("entries: %+v", entries)
	}
	a, b, c := entries[0], entries[1], entries[2]
	if a.Alias != "a" || a.HostName != "1.2.3.4" || a.User != "fallback" || a.Port != 2222 || a.IdentityFile != "/home/u/.ssh/id_a" {
		t.Fatalf("a: %+v", a)
	}
	if b.HostName != "1.2.3.4" || b.IdentityFile != a.IdentityFile {
		t.Fatalf("b should share a's settings: %+v", b)
	}
	if c.HostName != "c.example.com" || c.User != "carol" || c.Port != 22 {
		t.Fatalf("c: %+v", c)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings: %v", warnings)
	}
}
