package core

import "remvyn/backend/models"

// ShellDir returns where a terminal session's shell is.
func (c *Core) ShellDir(sessionID string) (models.ShellDir, error) {
	return c.terms.WorkingDir(sessionID)
}

// ListNames lists a folder on the computer a terminal session runs on, for
// path completion as you type: names only, folders ending in /. dir may be
// relative to the shell's directory; base is the UI's guess at it.
func (c *Core) ListNames(sessionID, dir, base string) ([]string, error) {
	return c.terms.ListNames(sessionID, dir, base)
}
