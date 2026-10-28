package core

import (
	"bytes"
	"errors"
	"strings"
	"time"

	"remvyn/backend/files"
	"remvyn/backend/models"
)

// ShellDir returns where a terminal session's shell is, for the path picker.
func (c *Core) ShellDir(sessionID string) (models.ShellDir, error) {
	return c.terms.WorkingDir(sessionID)
}

// FindFiles indexes the tree under dir on the computer a terminal session
// runs on, for the path picker. It returns what it found when it hits the
// time limit.
func (c *Core) FindFiles(sessionID, dir string) (models.FileIndex, error) {
	client, err := c.terms.Client(sessionID)
	if err != nil {
		return models.FileIndex{}, err
	}
	if client == nil {
		return files.FindLocal(dir)
	}
	if !strings.HasPrefix(dir, "/") {
		return models.FileIndex{}, files.ErrNoDir
	}
	sess, err := client.NewSession()
	if err != nil {
		return models.FileIndex{}, err
	}
	defer sess.Close()
	var out bytes.Buffer
	sess.Stdin = strings.NewReader(files.FindScript())
	sess.Stdout = &limitWriter{w: &out, n: 16 << 20}
	done := make(chan error, 1)
	go func() { done <- sess.Run("sh -s -- " + shellQuote(dir)) }()
	complete := true
	select {
	case <-done:
	case <-time.After(files.FindTimeout):
		complete = false
		_ = sess.Close()
		<-done
	}
	idx, err := files.ParseFind(dir, out.String(), complete)
	if err != nil {
		return idx, err
	}
	if len(idx.Files)+len(idx.Dirs)+len(idx.Pruned) == 0 && !complete {
		return idx, errors.New("listing that folder took too long")
	}
	return idx, nil
}

// shellQuote quotes s as one POSIX shell word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
