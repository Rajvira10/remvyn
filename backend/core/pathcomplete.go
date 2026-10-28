package core

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	"remvyn/backend/files"
	"remvyn/backend/models"
)

// ShellDir returns where a terminal session's shell is, for path completion.
func (c *Core) ShellDir(sessionID string) (models.ShellDir, error) {
	return c.terms.WorkingDir(sessionID)
}

// ListNames lists a folder on the computer a terminal session runs on, for
// path completion as you type: names only, folders ending in /.
func (c *Core) ListNames(sessionID, dir string) ([]string, error) {
	client, err := c.terms.Client(sessionID)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return files.ListNamesLocal(dir)
	}
	if !strings.HasPrefix(dir, "/") {
		return nil, files.ErrNoDir
	}
	sess, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	var out bytes.Buffer
	sess.Stdin = strings.NewReader(files.ListNamesScript)
	sess.Stdout = &limitWriter{w: &out, n: 1 << 20}
	done := make(chan error, 1)
	go func() { done <- sess.Run("sh -s -- " + shellQuote(dir)) }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case <-done:
	case <-ctx.Done():
		return nil, errors.New("listing that folder took too long")
	}
	return files.ParseNames(out.String())
}

// shellQuote quotes s as one POSIX shell word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
