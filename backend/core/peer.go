package core

import (
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// describePeer names the program on the other end of conn and the ones that
// started it, e.g. "ssh ← git ← claude", for the approval prompt.
func describePeer(conn net.Conn) string {
	pid := peerPID(conn)
	var names []string
	for i := 0; i < 4 && pid > 1; i++ {
		out, err := exec.Command("ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			break
		}
		ppid, comm, ok := strings.Cut(strings.TrimSpace(string(out)), " ")
		if !ok {
			break
		}
		name := filepath.Base(strings.TrimSpace(comm))
		// Login shells show as -zsh.
		name = strings.TrimPrefix(name, "-")
		if len(names) == 0 || names[len(names)-1] != name {
			names = append(names, name)
		}
		pid, _ = strconv.Atoi(strings.TrimSpace(ppid))
	}
	if len(names) == 0 {
		return "A program"
	}
	return strings.Join(names, " ← ")
}
