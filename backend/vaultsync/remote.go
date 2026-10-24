package vaultsync

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// ErrConflict means the remote moved while we were committing; pull again.
var ErrConflict = errors.New("the vault changed on another device")

// Snapshot is the state of the remote branch.
type Snapshot struct {
	Commit string            // "" for an empty repository
	Tree   string            // root tree SHA
	Files  map[string]string // path -> git blob SHA
}

// Remote stores vault files. GitHubRemote is the real implementation.
type Remote interface {
	// Ref returns the current head commit ("" when the repository is empty).
	Ref(ctx context.Context) (string, error)
	// Snapshot lists the files at commit.
	Snapshot(ctx context.Context, commit string) (Snapshot, error)
	// Read returns a blob's content.
	Read(ctx context.Context, sha string) ([]byte, error)
	// Commit writes changes (nil content deletes) on top of base and moves the
	// branch, failing with ErrConflict if the branch is no longer at base.
	Commit(ctx context.Context, base Snapshot, changes map[string][]byte, message string) (Snapshot, error)
}

// BlobSHA computes git's object ID for content, matching GitHub's blob SHAs.
func BlobSHA(content []byte) string {
	h := sha1.New()
	h.Write([]byte("blob " + strconv.Itoa(len(content)) + "\x00"))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

// GitHubRemote keeps the vault in a GitHub repository via the Git Data API,
// so no git binary or local clone is needed.
type GitHubRemote struct {
	GH     *GitHub
	Repo   string // owner/name
	Branch string
}

func (r *GitHubRemote) path(p string) string { return "/repos/" + r.Repo + p }

// Ref implements Remote.
func (r *GitHubRemote) Ref(ctx context.Context) (string, error) {
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	status, err := r.GH.do(ctx, http.MethodGet, r.path("/git/ref/heads/"+r.Branch), nil, &ref)
	if status == http.StatusConflict || status == http.StatusNotFound {
		// 409: empty repository. 404: branch not created yet.
		if status == http.StatusNotFound {
			if _, rerr := r.GH.GetRepo(ctx, r.Repo); rerr != nil {
				return "", rerr
			}
		}
		return "", nil
	}
	return ref.Object.SHA, err
}

// Snapshot implements Remote.
func (r *GitHubRemote) Snapshot(ctx context.Context, commit string) (Snapshot, error) {
	if commit == "" {
		return Snapshot{Files: map[string]string{}}, nil
	}
	var c struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if _, err := r.GH.do(ctx, http.MethodGet, r.path("/git/commits/"+commit), nil, &c); err != nil {
		return Snapshot{}, err
	}
	var t struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			SHA  string `json:"sha"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if _, err := r.GH.do(ctx, http.MethodGet, r.path("/git/trees/"+c.Tree.SHA+"?recursive=1"), nil, &t); err != nil {
		return Snapshot{}, err
	}
	if t.Truncated {
		return Snapshot{}, errors.New("the vault repository is too large to list")
	}
	s := Snapshot{Commit: commit, Tree: c.Tree.SHA, Files: map[string]string{}}
	for _, e := range t.Tree {
		if e.Type == "blob" {
			s.Files[e.Path] = e.SHA
		}
	}
	return s, nil
}

// Read implements Remote.
func (r *GitHubRemote) Read(ctx context.Context, sha string) ([]byte, error) {
	var b struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if _, err := r.GH.do(ctx, http.MethodGet, r.path("/git/blobs/"+sha), nil, &b); err != nil {
		return nil, err
	}
	if b.Encoding != "base64" {
		return []byte(b.Content), nil
	}
	return decodeB64Lines(b.Content)
}

const readme = `# Flux vault

This repository holds an end-to-end encrypted Flux vault. Every file is
encrypted with a key derived from your master password; GitHub only sees
ciphertext. Don't edit these files by hand.
`

// Commit implements Remote.
func (r *GitHubRemote) Commit(ctx context.Context, base Snapshot, changes map[string][]byte, message string) (Snapshot, error) {
	if base.Commit == "" {
		// The Git Data API doesn't work on an empty repository; the contents
		// API can create the first commit.
		status, err := r.GH.do(ctx, http.MethodPut, r.path("/contents/README.md"), map[string]any{
			"message": "Create Flux vault",
			"content": b64([]byte(readme)),
			"branch":  r.Branch,
		}, nil)
		if err != nil && status != http.StatusUnprocessableEntity && status != http.StatusConflict {
			return Snapshot{}, err
		}
		return Snapshot{}, ErrConflict // re-read the new head and retry
	}
	entries := make([]map[string]any, 0, len(changes))
	for p, content := range changes {
		e := map[string]any{"path": p, "mode": "100644", "type": "blob"}
		if content == nil {
			if _, ok := base.Files[p]; !ok {
				continue
			}
			e["sha"] = nil
		} else {
			e["content"] = string(content)
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return base, nil
	}
	var tree struct {
		SHA string `json:"sha"`
	}
	if _, err := r.GH.do(ctx, http.MethodPost, r.path("/git/trees"), map[string]any{"base_tree": base.Tree, "tree": entries}, &tree); err != nil {
		return Snapshot{}, err
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if _, err := r.GH.do(ctx, http.MethodPost, r.path("/git/commits"), map[string]any{
		"message": message, "tree": tree.SHA, "parents": []string{base.Commit},
	}, &commit); err != nil {
		return Snapshot{}, err
	}
	status, err := r.GH.do(ctx, http.MethodPatch, r.path("/git/refs/heads/"+r.Branch), map[string]any{"sha": commit.SHA, "force": false}, nil)
	if status == http.StatusUnprocessableEntity || status == http.StatusConflict {
		return Snapshot{}, ErrConflict
	}
	if err != nil {
		return Snapshot{}, err
	}
	next := Snapshot{Commit: commit.SHA, Tree: tree.SHA, Files: make(map[string]string, len(base.Files)+len(changes))}
	for p, sha := range base.Files {
		next.Files[p] = sha
	}
	for p, content := range changes {
		if content == nil {
			delete(next.Files, p)
		} else {
			next.Files[p] = BlobSHA(content)
		}
	}
	return next, nil
}

func decodeB64Lines(s string) ([]byte, error) {
	clean := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\n' && s[i] != '\r' {
			clean = append(clean, s[i])
		}
	}
	out := make([]byte, len(clean))
	n, err := b64Std.Decode(out, clean)
	if err != nil {
		return nil, fmt.Errorf("bad blob encoding: %w", err)
	}
	return out[:n], nil
}
