package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

type client struct {
	t   *testing.T
	w   io.WriteCloser
	out *bufio.Scanner
}

func start(t *testing.T, h Handler) *client {
	cr, cw := io.Pipe()
	sr, sw := io.Pipe()
	go func() {
		_ = Serve(context.Background(), cr, sw, h, "1.0")
		sw.Close()
	}()
	t.Cleanup(func() { cw.Close() })
	return &client{t: t, w: cw, out: bufio.NewScanner(sr)}
}

func (c *client) send(msg string) { io.WriteString(c.w, msg+"\n") }

func (c *client) read() map[string]any {
	c.t.Helper()
	got := make(chan map[string]any, 1)
	go func() {
		if !c.out.Scan() {
			got <- nil
			return
		}
		var m map[string]any
		json.Unmarshal(c.out.Bytes(), &m)
		got <- m
	}()
	select {
	case m := <-got:
		return m
	case <-time.After(5 * time.Second):
		c.t.Fatal("no response")
	}
	return nil
}

func TestNegotiatesVersionAndRejectsUnknownMethods(t *testing.T) {
	c := start(t, func(context.Context, Call) Result { return OK("") })
	c.send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	res := c.read()["result"].(map[string]any)
	if res["protocolVersion"] != protocolVersions[0] || res["instructions"] != Instructions {
		t.Fatalf("initialize = %v", res)
	}
	c.send(`{"jsonrpc":"2.0","id":2,"method":"bogus"}`)
	if e := c.read()["error"].(map[string]any); e["code"].(float64) != -32601 {
		t.Fatalf("error = %v", e)
	}
	c.send(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"rm_rf","arguments":{}}}`)
	if e := c.read()["error"].(map[string]any); !strings.Contains(e["message"].(string), "unknown tool") {
		t.Fatalf("error = %v", e)
	}
	c.send(`not json`)
	if e := c.read()["error"].(map[string]any); e["code"].(float64) != -32700 {
		t.Fatalf("error = %v", e)
	}
}

func TestCancelledCallStopsHandler(t *testing.T) {
	stopped := make(chan struct{})
	c := start(t, func(ctx context.Context, call Call) Result {
		if call.Tool == ToolListHosts {
			return OK("hosts for " + call.Client)
		}
		<-ctx.Done()
		close(stopped)
		return OK("late")
	})
	c.send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"claude-code"}}}`)
	c.read()
	c.send(`{"jsonrpc":"2.0","id":"slow","method":"tools/call","params":{"name":"run_command","arguments":{"host":"a","command":"sleep 99"}}}`)
	c.send(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"slow"}}`)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("handler was not cancelled")
	}
	// The cancelled call gets no answer; the next call is answered normally.
	c.send(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_hosts"}}`)
	m := c.read()
	if m["id"].(float64) != 2 {
		t.Fatalf("got a response for the cancelled call: %v", m)
	}
	text := m["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"]
	if text != "hosts for claude-code" {
		t.Fatalf("text = %v", text)
	}
}
