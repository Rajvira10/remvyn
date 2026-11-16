package core

import (
	"bytes"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"remvyn/backend/models"
	"remvyn/backend/remotedesk"
)

// fakeDesktop is a TCP server that greets like a VNC server and then echoes.
func fakeDesktop(t *testing.T) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = c.Write([]byte("RFB 003.008\n"))
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return "127.0.0.1", ln.Addr().(*net.TCPAddr).Port
}

func (e *events) ended(id string) (models.RemoteDesktopEnded, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, x := range e.remoteEnded {
		if x.ID == id {
			return x, true
		}
	}
	return models.RemoteDesktopEnded{}, false
}

func checkVNC(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, greet, err := ws.ReadMessage()
	if err != nil || string(greet) != "RFB 003.008\n" {
		t.Fatalf("greeting %q, %v", greet, err)
	}
	if err := ws.WriteMessage(websocket.BinaryMessage, []byte("ping")); err != nil {
		t.Fatal(err)
	}
	if _, echo, err := ws.ReadMessage(); err != nil || !bytes.Equal(echo, []byte("ping")) {
		t.Fatalf("echo %q, %v", echo, err)
	}
	return ws
}

func TestVNCDirectAndThroughSSH(t *testing.T) {
	f := setup(t)
	f.trustAll()
	addr, port := fakeDesktop(t)

	direct, err := f.core.SaveHost(models.HostInput{Host: models.Host{Label: "mac", Protocol: models.ProtocolVNC, Hostname: addr, Port: port}})
	if err != nil {
		t.Fatal(err)
	}
	id := sessionID()
	info, err := f.core.OpenRemoteDesktop(id, direct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if info.Kind != "vnc" || info.Via != "" || !strings.HasPrefix(info.URL, "ws://127.0.0.1:") {
		t.Fatalf("info %+v", info)
	}
	ws := checkVNC(t, info.URL)
	f.core.CloseRemoteDesktop(id)
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := ws.ReadMessage(); err == nil {
		t.Fatal("viewer still connected after close")
	}
	ws.Close()

	// Through the SSH test server (direct-tcpip).
	tunnelled, err := f.core.SaveHost(models.HostInput{Host: models.Host{Label: "mac via", Protocol: models.ProtocolVNC, Hostname: addr, Port: port, ViaHostID: f.host.ID}})
	if err != nil {
		t.Fatal(err)
	}
	id2 := sessionID()
	info, err = f.core.OpenRemoteDesktop(id2, tunnelled.ID)
	if err != nil {
		t.Fatal(err)
	}
	if info.Via != "test" {
		t.Fatalf("via %q", info.Via)
	}
	ws = checkVNC(t, info.URL)
	defer ws.Close()

	// Dropping the SSH connection ends the session and tells the UI.
	f.srv.DropConnections()
	waitFor(t, "ended event", func() bool { _, ok := f.ev.ended(id2); return ok })
	if f.core.remotes().get(id2) != nil {
		t.Fatal("session still registered")
	}
}

func TestVNCUnreachable(t *testing.T) {
	f := setup(t)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	h, err := f.core.SaveHost(models.HostInput{Host: models.Host{Protocol: models.ProtocolVNC, Hostname: "127.0.0.1", Port: port}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.OpenRemoteDesktop(sessionID(), h.ID); err == nil || !strings.Contains(err.Error(), "could not connect") {
		t.Fatalf("err %v", err)
	}
}

func TestRDPTunnelAndLaunch(t *testing.T) {
	f := setup(t)
	f.trustAll()
	addr, port := fakeDesktop(t)

	var mu sync.Mutex
	var launched []remotedesk.RDPTarget
	old := launchRDPClient
	launchRDPClient = func(tg remotedesk.RDPTarget) (*remotedesk.RDPLaunch, error) {
		mu.Lock()
		launched = append(launched, tg)
		mu.Unlock()
		return &remotedesk.RDPLaunch{Client: "Fake RDP"}, nil
	}
	defer func() { launchRDPClient = old }()

	h, err := f.core.SaveHost(models.HostInput{Host: models.Host{Label: "win", Protocol: models.ProtocolRDP, Hostname: addr, Port: port,
		Username: "ann", ViaHostID: f.host.ID}, Password: "rdp-pw", SetPassword: true})
	if err != nil {
		t.Fatal(err)
	}
	// SSH-only actions refuse a remote desktop host.
	if _, err := f.core.OpenTerminal(sessionID(), h.ID, models.ConnectOptions{}); err == nil {
		t.Fatal("opened a terminal to an RDP host")
	}

	id := sessionID()
	info, err := f.core.OpenRemoteDesktop(id, h.ID)
	if err != nil {
		t.Fatal(err)
	}
	if info.Client != "Fake RDP" || info.LaunchError != "" || info.Target != net.JoinHostPort(addr, strconv.Itoa(port)) {
		t.Fatalf("info %+v", info)
	}
	if !strings.HasPrefix(info.Addr, "127.0.0.1:") || info.Addr == info.Target {
		t.Fatalf("no tunnel: %+v", info)
	}
	mu.Lock()
	if len(launched) != 1 || launched[0].Addr != info.Addr || launched[0].Username != "ann" || launched[0].Password != "rdp-pw" {
		t.Fatalf("launched %+v", launched)
	}
	mu.Unlock()

	// The tunnel carries traffic to the desktop.
	c, err := net.Dial("tcp", info.Addr)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	greet := make([]byte, 12)
	if _, err := io.ReadFull(c, greet); err != nil || string(greet) != "RFB 003.008\n" {
		t.Fatalf("through tunnel: %q, %v", greet, err)
	}
	c.Close()

	if _, err := f.core.LaunchRDP(id); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(launched) != 2 {
		t.Fatalf("open again launched %d times", len(launched))
	}
	mu.Unlock()

	f.core.CloseRemoteDesktop(id)
	if c, err := net.DialTimeout("tcp", info.Addr, time.Second); err == nil {
		c.Close()
		t.Fatal("tunnel still open after close")
	}
	if _, err := f.core.LaunchRDP(id); err == nil {
		t.Fatal("launched a closed session")
	}
	if _, ok := f.ev.ended(id); ok {
		t.Fatal("closing on purpose emitted an ended event")
	}

	// Lock-style shutdown ends sessions and tells the UI why.
	id2 := sessionID()
	if _, err := f.core.OpenRemoteDesktop(id2, h.ID); err != nil {
		t.Fatal(err)
	}
	f.core.closeRemoteDesktops("Remvyn was locked")
	if x, ok := f.ev.ended(id2); !ok || x.Error != "Remvyn was locked" {
		t.Fatalf("ended %+v %v", x, ok)
	}
}

func TestRemoteDesktopHostsAreNotSSHHosts(t *testing.T) {
	f := setup(t)
	h, err := f.core.SaveHost(models.HostInput{Host: models.Host{Label: "desk", Protocol: models.ProtocolVNC, Hostname: "10.0.0.9"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetAgentHostMode(h.ID, models.AgentAsk); err == nil {
		t.Fatal("opened a VNC host to agents")
	}
	if _, err := f.core.SetForwardKeys(h.ID, true); err == nil {
		t.Fatal("key forwarding allowed for a VNC host")
	}
	if ids := f.core.existingHostIDs([]string{h.ID, f.host.ID}); len(ids) != 1 || ids[0] != f.host.ID {
		t.Fatalf("monitorable hosts %v", ids)
	}
	st := f.core.Settings()
	st.AgentHosts = map[string]string{h.ID: models.AgentAuto, f.host.ID: models.AgentAuto}
	if hs := f.core.agentHosts(st); len(hs) != 1 || hs[0].ID != f.host.ID {
		t.Fatalf("agent hosts %+v", hs)
	}
}
