package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	_ "modernc.org/sqlite"

	"flux/backend/keys"
	"flux/backend/models"
	"flux/backend/sshconn"
	"flux/backend/terminal"
	"flux/backend/testutil"
)

// events records everything Core emits.
type events struct {
	mu      sync.Mutex
	data    map[string]*bytes.Buffer
	exits   map[string]models.SessionExit
	prompts chan models.HostKeyPrompt
	fwd     map[string]models.ForwardStatus
	xfer    map[string]models.Transfer
	samples []models.Metrics
	status  map[string]models.MonitorStatus
	alerts  []models.AlertEvent
}

func newEvents() *events {
	return &events{data: map[string]*bytes.Buffer{}, exits: map[string]models.SessionExit{},
		prompts: make(chan models.HostKeyPrompt, 16), fwd: map[string]models.ForwardStatus{}, xfer: map[string]models.Transfer{},
		status: map[string]models.MonitorStatus{}}
}

func (e *events) emit(name string, payload any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case strings.HasPrefix(name, "term:data:"):
		id := strings.TrimPrefix(name, "term:data:")
		b, _ := base64.StdEncoding.DecodeString(payload.(string))
		if e.data[id] == nil {
			e.data[id] = &bytes.Buffer{}
		}
		e.data[id].Write(b)
	case strings.HasPrefix(name, "term:exit:"):
		x := payload.(models.SessionExit)
		e.exits[x.ID] = x
	case name == sshconn.EventHostKeyPrompt:
		e.prompts <- payload.(models.HostKeyPrompt)
	case name == "forward:status":
		st := payload.(models.ForwardStatus)
		e.fwd[st.RuleID] = st
	case name == "transfer:update":
		t := payload.(models.Transfer)
		e.xfer[t.ID] = t
	case name == "monitor:sample":
		e.samples = append(e.samples, payload.(models.Metrics))
	case name == "monitor:status":
		st := payload.(models.MonitorStatus)
		e.status[st.HostID] = st
	case name == "alert:event":
		e.alerts = append(e.alerts, payload.(models.AlertEvent))
	}
}

func (e *events) output(id string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if b := e.data[id]; b != nil {
		return b.String()
	}
	return ""
}

func (e *events) exit(id string) (models.SessionExit, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	x, ok := e.exits[id]
	return x, ok
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// autoTrust accepts every host-key prompt.
func autoTrust(c *Core, ev *events, stop <-chan struct{}) {
	for {
		select {
		case p := <-ev.prompts:
			c.ResolveHostKey(p.ID, true)
		case <-stop:
			return
		}
	}
}

type fixture struct {
	t    *testing.T
	core *Core
	ev   *events
	srv  *testutil.Server
	host models.Host
	dir  string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	srv, err := testutil.NewServer("alice", "s3cret-pw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	ev := newEvents()
	c, err := Open(context.Background(), Options{DataDir: dir, Emit: ev.emit, PromptTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	h, err := c.SaveHost(models.HostInput{Host: models.Host{Label: "test", Hostname: "127.0.0.1", Port: srv.Port(),
		Username: "alice", AuthMethod: models.AuthPassword}, Password: "s3cret-pw", SetPassword: true})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, core: c, ev: ev, srv: srv, host: h, dir: dir}
}

func (f *fixture) trustAll() {
	stop := make(chan struct{})
	f.t.Cleanup(func() { close(stop) })
	go autoTrust(f.core, f.ev, stop)
}

func sessionID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func TestTerminalSessionLifecycle(t *testing.T) {
	f := setup(t)
	id := sessionID()

	// The first connection must ask about the unknown host key.
	resCh := make(chan error, 1)
	go func() {
		_, err := f.core.OpenTerminal(id, f.host.ID, models.ConnectOptions{Cols: 100, Rows: 30})
		resCh <- err
	}()
	var p models.HostKeyPrompt
	select {
	case p = <-f.ev.prompts:
	case <-time.After(5 * time.Second):
		t.Fatal("no host key prompt")
	}
	if p.Changed || p.Host != f.srv.Addr || !strings.HasPrefix(p.Fingerprint, "SHA256:") {
		t.Fatalf("bad prompt: %+v", p)
	}
	f.core.ResolveHostKey(p.ID, true)
	if err := <-resCh; err != nil {
		t.Fatalf("open: %v", err)
	}

	waitFor(t, "banner", func() bool { return strings.Contains(f.ev.output(id), "Welcome to the Flux test server") })
	if err := f.core.WriteTerminal(id, "echo hello flux\r"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "echo output", func() bool { return strings.Contains(f.ev.output(id), "hello flux\r\n$ ") })

	if err := f.core.WriteTerminal(id, "size\r"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "initial size", func() bool { return strings.Contains(f.ev.output(id), "100x30") })
	if err := f.core.ResizeTerminal(id, 132, 40); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	_ = f.core.WriteTerminal(id, "size\r")
	waitFor(t, "resized", func() bool { return strings.Contains(f.ev.output(id), "132x40") })

	_ = f.core.WriteTerminal(id, "exit 3\r")
	waitFor(t, "exit event", func() bool { _, ok := f.ev.exit(id); return ok })
	if x, _ := f.ev.exit(id); x.ExitCode != 3 {
		t.Fatalf("exit code: %+v", x)
	}
	if err := f.core.WriteTerminal(id, "x"); err == nil {
		t.Fatal("writing to an ended session should fail")
	}

	// Second connection: key is now trusted, no prompt.
	id2 := sessionID()
	if _, err := f.core.OpenTerminal(id2, f.host.ID, models.ConnectOptions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-f.ev.prompts:
		t.Fatalf("unexpected prompt for trusted host: %+v", p)
	default:
	}
	known, _ := f.core.KnownHosts()
	if len(known) != 1 || known[0].Host != f.srv.Addr {
		t.Fatalf("known hosts: %+v", known)
	}
	f.core.CloseTerminal(id2)
	waitFor(t, "close exit event", func() bool { _, ok := f.ev.exit(id2); return ok })
}

func TestChangedHostKeyIsFlaggedAndRejectable(t *testing.T) {
	f := setup(t)
	go func() { p := <-f.ev.prompts; f.core.ResolveHostKey(p.ID, true) }()
	id := sessionID()
	if _, err := f.core.OpenTerminal(id, f.host.ID, models.ConnectOptions{}); err != nil {
		t.Fatal(err)
	}
	f.core.CloseTerminal(id)

	f.srv.RotateHostKey()
	go func() {
		p := <-f.ev.prompts
		if !p.Changed || p.OldFingerprint == "" || p.OldFingerprint == p.Fingerprint {
			t.Errorf("expected changed-key prompt, got %+v", p)
		}
		f.core.ResolveHostKey(p.ID, false)
	}()
	_, err := f.core.OpenTerminal(sessionID(), f.host.ID, models.ConnectOptions{})
	if sshconn.CodeOf(err) != sshconn.CodeHostKeyRejected {
		t.Fatalf("expected hostkey_rejected, got %v", err)
	}
}

func TestPasswordAuthFlows(t *testing.T) {
	f := setup(t)
	f.trustAll()
	h, err := f.core.SaveHost(models.HostInput{Host: models.Host{Label: "nopw", Hostname: "127.0.0.1", Port: f.srv.Port(),
		Username: "alice", AuthMethod: models.AuthPassword}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.core.OpenTerminal(sessionID(), h.ID, models.ConnectOptions{})
	if sshconn.CodeOf(err) != sshconn.CodePasswordRequired {
		t.Fatalf("expected password_required, got %v", err)
	}
	_, err = f.core.OpenTerminal(sessionID(), h.ID, models.ConnectOptions{Password: "wrong"})
	if sshconn.CodeOf(err) != sshconn.CodeAuth {
		t.Fatalf("expected auth_failed, got %v", err)
	}
	id := sessionID()
	if _, err := f.core.OpenTerminal(id, h.ID, models.ConnectOptions{Password: "s3cret-pw", SavePassword: true}); err != nil {
		t.Fatal(err)
	}
	f.core.CloseTerminal(id)
	hosts, _ := f.core.Hosts()
	for _, x := range hosts {
		if x.ID == h.ID && (!x.HasPassword || x.LastConnectedAt == 0) {
			t.Fatalf("password not saved / connection not recorded: %+v", x)
		}
	}
	if dbContains(t, f.core.DBPath, "s3cret-pw") {
		t.Fatal("password stored in plaintext")
	}
	// Saved password is now used automatically.
	id = sessionID()
	if _, err := f.core.OpenTerminal(id, h.ID, models.ConnectOptions{}); err != nil {
		t.Fatal(err)
	}
	f.core.CloseTerminal(id)
}

func dbContains(t *testing.T, path, needle string) bool {
	t.Helper()
	// Inspect every text/blob column in the raw database.
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`SELECT COALESCE(password_sealed, x'') FROM hosts`,
		`SELECT private_sealed FROM keys`,
		`SELECT COALESCE(passphrase_sealed, x'') FROM keys`,
	} {
		rows, err := db.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var b []byte
			rows.Scan(&b)
			if bytes.Contains(b, []byte(needle)) {
				rows.Close()
				return true
			}
		}
		rows.Close()
	}
	raw, _ := os.ReadFile(path)
	wal, _ := os.ReadFile(path + "-wal")
	return bytes.Contains(raw, []byte(needle)) || bytes.Contains(wal, []byte(needle))
}

func TestKeyAuthentication(t *testing.T) {
	f := setup(t)
	f.trustAll()
	k, err := f.core.GenerateKey(models.KeyGenerateInput{Name: "laptop", Type: "ed25519", Passphrase: "pp-123"})
	if err != nil {
		t.Fatal(err)
	}
	if !k.HasPassphrase || k.Type != "ssh-ed25519" || !strings.HasPrefix(k.Fingerprint, "SHA256:") {
		t.Fatalf("key meta: %+v", k)
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(k.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	f.srv.AuthorizeKey(pub)
	h, err := f.core.SaveHost(models.HostInput{Host: models.Host{Label: "key", Hostname: "127.0.0.1", Port: f.srv.Port(),
		Username: "alice", AuthMethod: models.AuthKey, KeyID: k.ID}})
	if err != nil {
		t.Fatal(err)
	}
	id := sessionID()
	if _, err := f.core.OpenTerminal(id, h.ID, models.ConnectOptions{}); err != nil {
		t.Fatalf("key auth failed: %v", err)
	}
	f.core.CloseTerminal(id)

	if err := f.core.DeleteKey(k.ID); err == nil || !strings.Contains(err.Error(), "used by") {
		t.Fatalf("deleting an in-use key should fail: %v", err)
	}

	// An unauthorized key is rejected.
	other, _ := f.core.GenerateKey(models.KeyGenerateInput{Name: "other", Type: "rsa", Bits: 2048})
	h.KeyID = other.ID
	h, _ = f.core.SaveHost(models.HostInput{Host: h})
	if _, err := f.core.OpenTerminal(sessionID(), h.ID, models.ConnectOptions{}); sshconn.CodeOf(err) != sshconn.CodeAuth {
		t.Fatalf("expected auth failure with unauthorized key, got %v", err)
	}
	if dbContains(t, f.core.DBPath, "PRIVATE KEY") || dbContains(t, f.core.DBPath, "pp-123") {
		t.Fatal("private key material stored in plaintext")
	}
}

func TestImportEncryptedKeyNeedsPassphrase(t *testing.T) {
	f := setup(t)
	m, err := keys.Generate(models.KeyGenerateInput{Name: "enc", Type: "ecdsa", Passphrase: "letmein"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.core.ImportKey(models.KeyImportInput{PrivateKey: m.PrivatePEM})
	if sshconn.CodeOf(err) != sshconn.CodePassphraseRequired {
		t.Fatalf("expected passphrase_required, got %v", err)
	}
	_, err = f.core.ImportKey(models.KeyImportInput{PrivateKey: m.PrivatePEM, Passphrase: "nope"})
	if sshconn.CodeOf(err) != sshconn.CodePassphraseRequired {
		t.Fatalf("wrong passphrase should ask again, got %v", err)
	}
	k, err := f.core.ImportKey(models.KeyImportInput{PrivateKey: m.PrivatePEM, Passphrase: "letmein"})
	if err != nil || k.Fingerprint != m.Meta.Fingerprint {
		t.Fatalf("import: %+v %v", k, err)
	}
	if _, err := f.core.ImportKey(models.KeyImportInput{PrivateKey: m.PrivatePEM, Passphrase: "letmein"}); err == nil {
		t.Fatal("duplicate import should fail")
	}
	if _, err := f.core.ImportKey(models.KeyImportInput{PrivateKey: "hello"}); err == nil {
		t.Fatal("garbage import should fail")
	}
}

func TestSFTPBrowseAndTransfer(t *testing.T) {
	f := setup(t)
	f.trustAll()
	id := sessionID()
	if _, err := f.core.OpenSFTP(id, f.host.ID, models.ConnectOptions{}); err != nil {
		t.Fatal(err)
	}
	fm := f.core.Files()

	remoteRoot := t.TempDir()
	localSrc := t.TempDir()
	localDst := t.TempDir()
	// A small tree to upload.
	os.MkdirAll(filepath.Join(localSrc, "proj", "sub"), 0o755)
	os.WriteFile(filepath.Join(localSrc, "proj", "a.txt"), []byte("alpha"), 0o644)
	big := make([]byte, 3<<20)
	rand.Read(big)
	os.WriteFile(filepath.Join(localSrc, "proj", "sub", "big.bin"), big, 0o600)

	if err := fm.Mkdir(id, remoteRoot, "uploads"); err != nil {
		t.Fatal(err)
	}
	upDir := filepath.Join(remoteRoot, "uploads")
	tid, err := fm.Upload(id, []string{filepath.Join(localSrc, "proj")}, upDir)
	if err != nil {
		t.Fatal(err)
	}
	waitTransfer(t, f, tid, models.TransferDone)
	got, err := os.ReadFile(filepath.Join(upDir, "proj", "sub", "big.bin"))
	if err != nil || !bytes.Equal(got, big) {
		t.Fatal("uploaded file mismatch")
	}

	listing, err := fm.List(id, filepath.Join(upDir, "proj"))
	if err != nil || len(listing.Entries) != 2 || !listing.Entries[0].IsDir || listing.Entries[1].Name != "a.txt" {
		t.Fatalf("listing (dirs first): %+v %v", listing, err)
	}

	tid, err = fm.Download(id, []string{filepath.Join(upDir, "proj")}, localDst)
	if err != nil {
		t.Fatal(err)
	}
	tr := waitTransfer(t, f, tid, models.TransferDone)
	if tr.FilesTotal != 2 || tr.FilesDone != 2 || tr.BytesDone != int64(len(big)+5) {
		t.Fatalf("transfer counters: %+v", tr)
	}
	got, _ = os.ReadFile(filepath.Join(localDst, "proj", "sub", "big.bin"))
	if !bytes.Equal(got, big) {
		t.Fatal("downloaded file mismatch")
	}
	if _, err := os.Stat(filepath.Join(localDst, "proj", "a.txt.flux-part")); err == nil {
		t.Fatal("temp file left behind")
	}

	if err := fm.Rename(id, filepath.Join(upDir, "proj", "a.txt"), "b.txt"); err != nil {
		t.Fatal(err)
	}
	if err := fm.Rename(id, filepath.Join(upDir, "proj", "b.txt"), "../escape"); err == nil {
		t.Fatal("rename with path separators should be rejected")
	}
	if err := fm.Remove(id, []string{filepath.Join(upDir, "proj")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(upDir, "proj")); !os.IsNotExist(err) {
		t.Fatal("recursive remove failed")
	}
	if _, err := fm.List(id, filepath.Join(upDir, "missing")); err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("expected friendly error, got %v", err)
	}
	fm.Close(id)
	if _, err := fm.List(id, upDir); err == nil {
		t.Fatal("closed session should not list")
	}
}

func waitTransfer(t *testing.T, f *fixture, id, state string) models.Transfer {
	t.Helper()
	var tr models.Transfer
	waitFor(t, "transfer "+state, func() bool {
		f.ev.mu.Lock()
		defer f.ev.mu.Unlock()
		tr = f.ev.xfer[id]
		if tr.State == models.TransferFailed && state != models.TransferFailed {
			t.Fatalf("transfer failed: %s", tr.Error)
		}
		return tr.State == state
	})
	return tr
}

func TestTransferCancel(t *testing.T) {
	f := setup(t)
	f.trustAll()
	id := sessionID()
	if _, err := f.core.OpenSFTP(id, f.host.ID, models.ConnectOptions{}); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "huge.bin")
	fh, _ := os.Create(src)
	fh.Truncate(512 << 20) // sparse 512 MiB
	fh.Close()
	dst := t.TempDir()
	tid, err := f.core.Files().Upload(id, []string{src}, dst)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	f.core.Files().CancelTransfer(tid)
	waitTransfer(t, f, tid, models.TransferCancelled)
}

// echoServer is a TCP service standing in for something behind the SSH host.
func echoServer(t *testing.T) string {
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
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()
	return ln.Addr().String()
}

func freePort(t *testing.T) int {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	p := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return p
}

func roundTrip(t *testing.T, c net.Conn, msg string) {
	t.Helper()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != msg {
		t.Fatalf("echo mismatch: %q %v", buf, err)
	}
}

func TestLocalRemoteAndDynamicForwarding(t *testing.T) {
	f := setup(t)
	f.trustAll()
	echo := echoServer(t)
	eh, ep, _ := net.SplitHostPort(echo)
	epn, _ := strconv.Atoi(ep)

	// Local: 127.0.0.1:L → (via SSH) echo
	lp := freePort(t)
	local, err := f.core.SaveForward(models.ForwardRule{HostID: f.host.ID, Type: models.ForwardLocal, BindPort: lp, DestHost: eh, DestPort: epn})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.core.StartForward(local.ID); err != nil {
		t.Fatal(err)
	}
	c, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(lp)))
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, c, "local-forward")
	c.Close()

	// Remote: server listens on R → (back through SSH) echo
	rp := freePort(t)
	remote, _ := f.core.SaveForward(models.ForwardRule{HostID: f.host.ID, Type: models.ForwardRemote, BindPort: rp, DestHost: eh, DestPort: epn})
	if err := f.core.StartForward(remote.ID); err != nil {
		t.Fatal(err)
	}
	c, err = net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(rp)))
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, c, "remote-forward")
	c.Close()

	// Dynamic: SOCKS5 on D, CONNECT to the echo service.
	dp := freePort(t)
	dyn, _ := f.core.SaveForward(models.ForwardRule{HostID: f.host.ID, Type: models.ForwardDynamic, BindPort: dp})
	if err := f.core.StartForward(dyn.ID); err != nil {
		t.Fatal(err)
	}
	c, err = net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(dp)))
	if err != nil {
		t.Fatal(err)
	}
	c.SetDeadline(time.Now().Add(5 * time.Second))
	c.Write([]byte{5, 1, 0})
	resp := make([]byte, 2)
	io.ReadFull(c, resp)
	if resp[1] != 0 {
		t.Fatalf("socks method: %v", resp)
	}
	req := []byte{5, 1, 0, 3, byte(len(eh))}
	req = append(req, eh...)
	req = binary.BigEndian.AppendUint16(req, uint16(epn))
	c.Write(req)
	rep := make([]byte, 10)
	if _, err := io.ReadFull(c, rep); err != nil || rep[1] != 0 {
		t.Fatalf("socks connect reply: %v %v", rep, err)
	}
	roundTrip(t, c, "socks-forward")
	c.Close()

	waitFor(t, "stats", func() bool {
		for _, st := range f.core.ForwardStatuses() {
			if st.RuleID == local.ID && st.TotalConns == 1 && st.BytesOut >= int64(len("local-forward")) {
				return true
			}
		}
		return false
	})
	if err := f.core.StartForward(local.ID); err == nil {
		t.Fatal("starting a running rule should fail")
	}

	// Port already taken.
	busy, _ := f.core.SaveForward(models.ForwardRule{HostID: f.host.ID, Type: models.ForwardLocal, BindPort: lp, DestHost: eh, DestPort: epn})
	if err := f.core.StartForward(busy.ID); err == nil || !strings.Contains(err.Error(), "could not listen") {
		t.Fatalf("expected listen error, got %v", err)
	}

	// Connection loss stops rules with an error state.
	f.srv.DropConnections()
	waitFor(t, "rule error after drop", func() bool {
		f.ev.mu.Lock()
		defer f.ev.mu.Unlock()
		st := f.ev.fwd[local.ID]
		return st.State == models.ForwardError && strings.Contains(st.Error, "lost")
	})
	// Port is released.
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(lp)))
	if err != nil {
		t.Fatalf("listener not released: %v", err)
	}
	ln.Close()

	f.core.StopForward(remote.ID)
	f.core.StopForward(dyn.ID)
	if err := f.core.DeleteForward(local.ID); err != nil {
		t.Fatal(err)
	}
}

func TestInstallPublicKey(t *testing.T) {
	f := setup(t)
	f.trustAll()
	k, _ := f.core.GenerateKey(models.KeyGenerateInput{Name: "deploy key", Type: "ed25519"})
	if err := f.core.InstallPublicKey(f.host.ID, k.ID, models.ConnectOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := f.core.InstallPublicKey(f.host.ID, k.ID, models.ConnectOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(f.srv.AuthorizedLines) != 1 || !strings.HasPrefix(f.srv.AuthorizedLines[0], "ssh-ed25519 ") {
		t.Fatalf("authorized lines: %v", f.srv.AuthorizedLines)
	}
}

func TestCancelConnectDuringPrompt(t *testing.T) {
	f := setup(t)
	id := sessionID()
	res := make(chan error, 1)
	go func() { _, err := f.core.OpenTerminal(id, f.host.ID, models.ConnectOptions{}); res <- err }()
	<-f.ev.prompts
	f.core.CancelConnect(id)
	select {
	case err := <-res:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not abort the connection")
	}
}

func TestNetworkErrorsAreFriendly(t *testing.T) {
	f := setup(t)
	h, _ := f.core.SaveHost(models.HostInput{Host: models.Host{Label: "down", Hostname: "127.0.0.1", Port: freePort(t),
		Username: "x", AuthMethod: models.AuthPassword}, Password: "x", SetPassword: true})
	_, err := f.core.OpenTerminal(sessionID(), h.ID, models.ConnectOptions{})
	if sshconn.CodeOf(err) != sshconn.CodeNetwork || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("expected friendly network error, got %v", err)
	}
	if _, err := f.core.OpenTerminal("bad id!", h.ID, models.ConnectOptions{}); err == nil {
		t.Fatal("invalid session id accepted")
	}
	if !terminal.ValidID(sessionID()) {
		t.Fatal("generated ids should be valid")
	}
}

func TestHostsGroupsSnippetsAndPersistence(t *testing.T) {
	f := setup(t)
	g, err := f.core.SaveGroup(models.Group{Name: "Production"})
	if err != nil {
		t.Fatal(err)
	}
	h := f.host
	h.GroupID = g.ID
	if _, err := f.core.SaveHost(models.HostInput{Host: h}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SaveHost(models.HostInput{Host: models.Host{Hostname: "bad host", AuthMethod: models.AuthPassword}}); err == nil {
		t.Fatal("invalid hostname accepted")
	}
	if _, err := f.core.SaveHost(models.HostInput{Host: models.Host{Hostname: "x", AuthMethod: models.AuthKey}}); err == nil {
		t.Fatal("key auth without key accepted")
	}
	sn, err := f.core.SaveSnippet(models.Snippet{Name: "disk", Command: "df -h"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.core.DeleteGroup(g.ID); err != nil {
		t.Fatal(err)
	}
	hosts, _ := f.core.Hosts()
	if hosts[0].GroupID != "" || !hosts[0].HasPassword {
		t.Fatalf("group delete should ungroup host: %+v", hosts[0])
	}
	f.core.Close()

	c2, err := Open(context.Background(), Options{DataDir: f.dir})
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	hosts, _ = c2.Hosts()
	snips, _ := c2.Snippets()
	if len(hosts) != 1 || len(snips) != 1 || snips[0].ID != sn.ID {
		t.Fatalf("not persisted: %v %v", hosts, snips)
	}
	pw, _ := c2.Store().HostPassword(context.Background(), hosts[0].ID)
	if pw != "s3cret-pw" {
		t.Fatal("password not recoverable after restart")
	}
}

func TestImportSSHConfig(t *testing.T) {
	f := setup(t)
	keyM, _ := keys.Generate(models.KeyGenerateInput{Name: "id", Type: "ed25519"})
	keyPath := filepath.Join(f.dir, "id_test")
	os.WriteFile(keyPath, []byte(keyM.PrivatePEM), 0o600)
	encM, _ := keys.Generate(models.KeyGenerateInput{Name: "enc", Type: "ed25519", Passphrase: "x"})
	encPath := filepath.Join(f.dir, "id_enc")
	os.WriteFile(encPath, []byte(encM.PrivatePEM), 0o600)
	cfg := filepath.Join(f.dir, "config")
	os.WriteFile(cfg, []byte(`
Host *
  User defaultuser

Host web web-alias
  HostName 10.0.0.5
  Port 2200
  IdentityFile `+keyPath+`

Host db
  HostName db.internal
  User postgres
  IdentityFile `+encPath+`
  ProxyJump bastion

Host *.corp
  User nobody

Host test
  HostName ignored.example
`), 0o600)
	res, err := f.core.ImportSSHConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// "test" already exists (fixture host label) and is skipped.
	if res.Imported != 3 || res.Skipped != 1 || res.Keys != 1 {
		t.Fatalf("import result: %+v", res)
	}
	hosts, _ := f.core.Hosts()
	by := map[string]models.Host{}
	for _, h := range hosts {
		by[h.Label] = h
	}
	if w := by["web"]; w.Hostname != "10.0.0.5" || w.Port != 2200 || w.Username != "defaultuser" || w.AuthMethod != models.AuthKey {
		t.Fatalf("web: %+v", w)
	}
	if by["web-alias"].KeyID != by["web"].KeyID {
		t.Fatal("shared identity file should map to one key")
	}
	if d := by["db"]; d.Username != "postgres" || d.AuthMethod != models.AuthAgent {
		t.Fatalf("db: %+v", d)
	}
	joined := strings.Join(res.Warnings, " ")
	if !strings.Contains(joined, "passphrase") || !strings.Contains(joined, "ProxyJump") {
		t.Fatalf("warnings: %v", res.Warnings)
	}
}

func TestImportSSHConfigWithoutConfigFile(t *testing.T) {
	f := setup(t)
	t.Setenv("HOME", t.TempDir()) // no ~/.ssh/config
	res, err := f.core.ImportSSHConfig("")
	if sshconn.CodeOf(err) != CodeNoSSHConfig {
		t.Fatalf("missing config should be reported with a code, got %v", err)
	}
	if res.Warnings == nil {
		t.Fatal("warnings must be a non-nil slice")
	}
	// An explicitly chosen file that doesn't exist is a plain error.
	if _, err := f.core.ImportSSHConfig(filepath.Join(t.TempDir(), "nope")); err == nil || sshconn.CodeOf(err) == CodeNoSSHConfig {
		t.Fatalf("explicit missing file: %v", err)
	}
}
