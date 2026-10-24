package main

import (
	"encoding/json"
	"errors"
	"testing"

	"flux/backend/sshconn"
)

// The Wails JS runtime wraps errors with new Error(value), so the formatter
// must produce a string that survives that and still carries the code.
func TestFormatErrorIsJSONString(t *testing.T) {
	out, ok := formatError(sshconn.Coded(sshconn.CodePasswordRequired, "a password is required", nil)).(string)
	if !ok {
		t.Fatal("formatter must return a string")
	}
	var b BridgeError
	if err := json.Unmarshal([]byte(out), &b); err != nil {
		t.Fatalf("not JSON: %q", out)
	}
	if b.Code != sshconn.CodePasswordRequired || b.Message != "a password is required" {
		t.Fatalf("got %+v", b)
	}
	plain := formatError(errors.New("boom")).(string)
	if err := json.Unmarshal([]byte(plain), &b); err != nil || b.Code != "" || b.Message != "boom" {
		t.Fatalf("uncoded error: %q %+v", plain, b)
	}
}
