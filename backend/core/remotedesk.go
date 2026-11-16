package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"remvyn/backend/models"
	"remvyn/backend/remotedesk"
	"remvyn/backend/sshconn"
	"remvyn/backend/terminal"
)

// EventRemoteEnded is emitted (models.RemoteDesktopEnded) when a remote
// desktop's connection or tunnel ends without the tab asking for it.
const EventRemoteEnded = "remote:ended"

// launchRDPClient opens the system's RDP app (swapped in tests).
var launchRDPClient = remotedesk.LaunchRDP

// remoteSession is an open RDP or VNC tab.
type remoteSession struct {
	info   models.RemoteDesktop
	host   models.Host
	ssh    *sshconn.Client // the SSH host tunnelled through, if any
	proxy  *remotedesk.WSProxy
	tunnel *remotedesk.Tunnel

	mu        sync.Mutex // guards launch and info.Client
	launch    *remotedesk.RDPLaunch
	closed    bool
	closeOnce sync.Once
}

func (s *remoteSession) close() {
	s.closeOnce.Do(func() {
		if s.proxy != nil {
			s.proxy.Close()
		}
		if s.tunnel != nil {
			s.tunnel.Close()
		}
		s.mu.Lock()
		s.closed = true
		s.launch.Close()
		s.mu.Unlock()
		if s.ssh != nil {
			_ = s.ssh.Close()
		}
	})
}

type remoteSessions struct {
	mu sync.Mutex
	m  map[string]*remoteSession
}

func (c *Core) remotes() *remoteSessions {
	c.remoteOnce.Do(func() { c.remote = &remoteSessions{m: map[string]*remoteSession{}} })
	return c.remote
}

func (r *remoteSessions) get(id string) *remoteSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m[id]
}

func (r *remoteSessions) take(id string) *remoteSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.m[id]
	delete(r.m, id)
	return s
}

// closeRemoteDesktops ends every RDP/VNC session (lock, vault switch, quit).
func (c *Core) closeRemoteDesktops(reason string) {
	r := c.remotes()
	r.mu.Lock()
	all := r.m
	r.m = map[string]*remoteSession{}
	r.mu.Unlock()
	for id, s := range all {
		s.close()
		if reason != "" {
			c.emit(EventRemoteEnded, models.RemoteDesktopEnded{ID: id, Error: reason})
		}
	}
}

// remoteDialer connects to a remote desktop host, through its Via SSH host
// when it has one. The returned SSH client (nil when direct) must be closed
// by the caller.
func (c *Core) remoteDialer(ctx context.Context, h models.Host) (remotedesk.Dialer, *sshconn.Client, string, error) {
	addr := sshconn.Addr(h)
	timeout := time.Duration(c.Settings().ConnectTimeout) * time.Second
	if h.ViaHostID == "" {
		return func(ctx context.Context) (net.Conn, error) {
			d := net.Dialer{Timeout: timeout}
			return d.DialContext(ctx, "tcp", addr)
		}, nil, "", nil
	}
	via, err := c.store.GetHost(ctx, h.ViaHostID)
	if err != nil {
		return nil, nil, "", errors.New("the SSH host this connection tunnels through was deleted; edit the host and choose another")
	}
	client, _, err := c.dial(ctx, via.ID, models.ConnectOptions{})
	if err != nil {
		return nil, nil, "", fmt.Errorf("could not connect to %s to tunnel through: %w", via.Label, err)
	}
	return func(ctx context.Context) (net.Conn, error) {
		type res struct {
			c   net.Conn
			err error
		}
		ch := make(chan res, 1)
		go func() {
			conn, err := client.Dial("tcp", addr)
			ch <- res{conn, err}
		}()
		select {
		case r := <-ch:
			if r.err != nil {
				return nil, fmt.Errorf("%s could not reach %s: %w", via.Label, addr, r.err)
			}
			return r.c, nil
		case <-ctx.Done():
			go func() {
				if r := <-ch; r.c != nil {
					_ = r.c.Close()
				}
			}()
			return nil, ctx.Err()
		}
	}, client, via.Label, nil
}

// OpenRemoteDesktop starts an RDP or VNC session for a host. For VNC it
// connects and returns the local WebSocket URL for the viewer; for RDP it
// starts the SSH tunnel (if any) and opens the system's RDP client.
func (c *Core) OpenRemoteDesktop(id, hostID string) (models.RemoteDesktop, error) {
	if !terminal.ValidID(id) {
		return models.RemoteDesktop{}, errors.New("invalid session id")
	}
	if c.store.Locked() {
		return models.RemoteDesktop{}, storageLocked()
	}
	ctx, done := c.trackDial(id)
	defer done()
	h, err := c.store.GetHost(ctx, hostID)
	if err != nil {
		return models.RemoteDesktop{}, errors.New("host not found")
	}
	if h.Protocol != models.ProtocolRDP && h.Protocol != models.ProtocolVNC {
		return models.RemoteDesktop{}, fmt.Errorf("%s is not a remote desktop host", h.Label)
	}
	dial, client, viaLabel, err := c.remoteDialer(ctx, h)
	if err != nil {
		return models.RemoteDesktop{}, err
	}
	s := &remoteSession{host: h, ssh: client, info: models.RemoteDesktop{ID: id, HostID: h.ID, Kind: h.Protocol,
		Target: sshconn.Addr(h), Via: viaLabel, Username: h.Username, StartedAt: time.Now().UnixMilli()}}
	ended := func(err error) {
		if c.remotes().take(id) == nil {
			return // closed on purpose
		}
		s.close()
		ev := models.RemoteDesktopEnded{ID: id}
		if err != nil {
			ev.Error = err.Error()
		}
		c.emit(EventRemoteEnded, ev)
	}

	switch h.Protocol {
	case models.ProtocolVNC:
		conn, err := dial(ctx)
		if err != nil {
			s.close()
			return models.RemoteDesktop{}, fmt.Errorf("could not connect to %s: %w", s.info.Target, unwrapDial(err))
		}
		if s.proxy, err = remotedesk.NewWSProxy(conn, ended); err != nil {
			_ = conn.Close()
			s.close()
			return models.RemoteDesktop{}, err
		}
		s.info.URL = s.proxy.URL()
	case models.ProtocolRDP:
		s.info.Addr = s.info.Target
		if client != nil {
			if s.tunnel, err = remotedesk.NewTunnel(dial); err != nil {
				s.close()
				return models.RemoteDesktop{}, err
			}
			s.info.Addr = s.tunnel.Addr()
		}
	}
	if client != nil {
		go func() {
			select {
			case <-client.Done():
				ended(fmt.Errorf("the SSH connection to %s closed", viaLabel))
			case <-c.stop:
			}
		}()
	}
	if err := ctx.Err(); err != nil {
		s.close()
		return models.RemoteDesktop{}, err
	}
	r := c.remotes()
	r.mu.Lock()
	if old := r.m[id]; old != nil {
		old.close()
	}
	r.m[id] = s
	r.mu.Unlock()
	_ = c.store.TouchHost(context.Background(), h.ID)

	if h.Protocol == models.ProtocolRDP {
		// The tunnel stays up even if the client can't be opened, so the user
		// can point another app at it; the tab shows why.
		if err := c.launchRDP(s); err != nil {
			info := s.snapshot()
			info.LaunchError, info.LaunchCode = err.Error(), sshconn.CodeOf(err)
			return info, nil
		}
	}
	return s.snapshot(), nil
}

// unwrapDial trims Go's "dial tcp 1.2.3.4:5900: " prefix.
func unwrapDial(err error) error {
	var op *net.OpError
	if errors.As(err, &op) && op.Err != nil {
		return op.Err
	}
	return err
}

func (c *Core) launchRDP(s *remoteSession) error {
	pw, err := c.store.HostPassword(context.Background(), s.host.ID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return remotedesk.ErrClosed
	}
	s.launch.Close() // a previous launch's file and credential
	l, err := launchRDPClient(remotedesk.RDPTarget{Label: s.host.Label, Addr: s.info.Addr,
		Username: strings.TrimSpace(s.host.Username), Password: pw})
	if err != nil {
		return err
	}
	s.launch = l
	s.info.Client = l.Client
	return nil
}

func (s *remoteSession) snapshot() models.RemoteDesktop {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info
}

// LaunchRDP opens the RDP client again for an open RDP session.
func (c *Core) LaunchRDP(id string) (models.RemoteDesktop, error) {
	s := c.remotes().get(id)
	if s == nil || s.info.Kind != models.ProtocolRDP {
		return models.RemoteDesktop{}, remotedesk.ErrClosed
	}
	if err := c.launchRDP(s); err != nil {
		return models.RemoteDesktop{}, err
	}
	return s.snapshot(), nil
}

// CloseRemoteDesktop ends an RDP or VNC session: the viewer connection,
// the tunnel and the SSH connection behind it.
func (c *Core) CloseRemoteDesktop(id string) {
	c.CancelConnect(id)
	if s := c.remotes().take(id); s != nil {
		s.close()
	}
}
