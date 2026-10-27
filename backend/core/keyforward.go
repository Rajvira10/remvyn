package core

import (
	"context"
	"errors"
	"slices"
	"time"

	"remvyn/backend/ids"
	"remvyn/backend/keyagent"
	"remvyn/backend/keys"
	"remvyn/backend/models"
)

// Agent forwarding: servers in Settings.ForwardKeyHosts can use the
// keychain's keys (for git, or SSH to other machines) while connected. The
// keys never leave Remvyn, and every use waits for the user's approval.

const (
	EventKeyUse     = "keyuse:request" // models.KeyUseRequest
	EventKeyUseDone = "keyuse:done"    // request ID
)

// keyUseTimeout is how long a request waits before it is denied.
const keyUseTimeout = 60 * time.Second

type pendingKeyUse struct {
	r  models.KeyUseRequest
	ch chan keyagent.Decision
}

func (c *Core) forwardsKeys(hostID string) bool {
	return slices.Contains(c.Settings().ForwardKeyHosts, hostID)
}

// keyAgent returns an agent for one connection to h. source names who is
// using the connection: "terminal", or an AI agent's name.
func (c *Core) keyAgent(h models.Host, source string) *keyagent.Agent {
	load := func() ([]keyagent.Key, error) {
		if c.store.Locked() {
			return nil, storageLocked()
		}
		ctx := context.Background()
		list, err := c.store.ListKeys(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]keyagent.Key, 0, len(list))
		for _, k := range list {
			pem, pass, err := c.store.KeySecret(ctx, k.ID)
			if err != nil {
				continue
			}
			// Keys whose passphrase isn't saved can't be used unattended.
			s, err := keys.Signer(pem, pass)
			if err != nil {
				continue
			}
			out = append(out, keyagent.Key{ID: k.ID, Name: k.Name, Signer: s})
		}
		return out, nil
	}
	approve := func(ctx context.Context, u keyagent.Use) keyagent.Decision {
		return c.askKeyUse(ctx, h, source, u)
	}
	return keyagent.New(load, approve)
}

// askKeyUse shows the approval prompt and waits for the answer.
func (c *Core) askKeyUse(ctx context.Context, h models.Host, source string, u keyagent.Use) keyagent.Decision {
	if c.store.Locked() {
		return keyagent.Deny
	}
	now := time.Now()
	ai := source != "terminal"
	p := &pendingKeyUse{ch: make(chan keyagent.Decision, 1), r: models.KeyUseRequest{
		ID: ids.New(), HostID: h.ID, HostLabel: h.Label, Remote: remote(h),
		KeyName: u.KeyName, Fingerprint: u.Fingerprint, Source: source, AI: ai,
		CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(keyUseTimeout).UnixMilli(),
	}}
	c.keyUseMu.Lock()
	c.keyUses[p.r.ID] = p
	c.keyUseMu.Unlock()
	defer func() {
		c.keyUseMu.Lock()
		delete(c.keyUses, p.r.ID)
		c.keyUseMu.Unlock()
		c.emit(EventKeyUseDone, p.r.ID)
	}()
	c.emit(EventKeyUse, p.r)
	if c.notify != nil {
		who := h.Label
		if ai {
			who = source + " on " + h.Label
		}
		c.notify(who+" wants to use your key "+u.KeyName, "Allow or deny it in Remvyn.")
	}
	timer := time.NewTimer(keyUseTimeout)
	defer timer.Stop()
	select {
	case d := <-p.ch:
		return d
	case <-timer.C:
	case <-ctx.Done():
	}
	return keyagent.Deny
}

// PendingKeyUses lists key-use requests waiting for the user.
func (c *Core) PendingKeyUses() []models.KeyUseRequest {
	c.keyUseMu.Lock()
	defer c.keyUseMu.Unlock()
	out := []models.KeyUseRequest{}
	for _, p := range c.keyUses {
		out = append(out, p.r)
	}
	slices.SortFunc(out, func(x, y models.KeyUseRequest) int { return int(x.CreatedAt - y.CreatedAt) })
	return out
}

// ResolveKeyUse answers a request: deny, once or session (the rest of the
// connection).
func (c *Core) ResolveKeyUse(id, decision string) bool {
	var d keyagent.Decision
	switch decision {
	case models.AgentDeny:
		d = keyagent.Deny
	case models.AgentOnce:
		d = keyagent.Once
	case models.AgentSession:
		d = keyagent.Session
	default:
		return false
	}
	c.keyUseMu.Lock()
	p, ok := c.keyUses[id]
	c.keyUseMu.Unlock()
	if ok {
		select {
		case p.ch <- d:
		default:
		}
	}
	return ok
}

// SetForwardKeys turns key forwarding on or off for a host. It applies to
// new connections.
func (c *Core) SetForwardKeys(hostID string, on bool) (models.Settings, error) {
	if _, err := c.store.GetHost(context.Background(), hostID); err != nil && on {
		return c.Settings(), errors.New("host not found")
	}
	c.saveMu.Lock()
	st := c.Settings()
	hosts := slices.DeleteFunc(slices.Clone(st.ForwardKeyHosts), func(id string) bool { return id == hostID })
	if on {
		hosts = append(hosts, hostID)
	}
	st.ForwardKeyHosts = hosts
	err := c.saveSettingsLocked(st)
	c.saveMu.Unlock()
	if err != nil {
		return c.Settings(), err
	}
	c.agentDropConn(hostID)
	return c.Settings(), nil
}

// SetForwardKeysToAI lets AI agents' connections forward keys too.
func (c *Core) SetForwardKeysToAI(on bool) (models.Settings, error) {
	c.saveMu.Lock()
	st := c.Settings()
	st.ForwardKeysToAI = on
	err := c.saveSettingsLocked(st)
	c.saveMu.Unlock()
	if err != nil {
		return c.Settings(), err
	}
	c.agentStopConns()
	return c.Settings(), nil
}

// agentDropConn closes an AI agent's pooled connection to a host, so the
// next action reconnects with the current forwarding setting.
func (c *Core) agentDropConn(hostID string) {
	a := c.agent
	a.mu.Lock()
	ac := a.conns[hostID]
	delete(a.conns, hostID)
	a.mu.Unlock()
	if ac != nil {
		ac.close()
	}
}

// agentStopConns closes every pooled AI agent connection.
func (c *Core) agentStopConns() {
	a := c.agent
	a.mu.Lock()
	conns := a.conns
	a.conns = map[string]*agentConn{}
	a.mu.Unlock()
	for _, ac := range conns {
		ac.close()
	}
}
