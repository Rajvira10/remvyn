package testutil

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// FakeGitHub implements the slice of GitHub that Remvyn sync uses: device-flow
// sign-in, app installations, and the Git Data / contents APIs.
type FakeGitHub struct {
	*httptest.Server

	mu      sync.Mutex
	tokens  map[string]string // token -> login
	devices map[string]int    // device code -> polls so far
	nextDev int
	blobs   map[string][]byte
	trees   map[string]map[string]string // tree sha -> path -> blob sha
	commits map[string]fakeCommit
	repos   map[string]*FakeRepo

	// Login is the user that signs in.
	Login string
	// Installed controls whether the app is installed (repos visible).
	Installed bool
	// DenyLogin makes sign-in fail with access_denied.
	DenyLogin bool
	// BeforeRefUpdate runs (unlocked) before a branch update, to simulate a
	// concurrent push from another device.
	BeforeRefUpdate func(repo string)
	// Requests counts API calls by "METHOD path-prefix".
	Requests map[string]int
}

type fakeCommit struct {
	Tree    string
	Parents []string
}

// FakeRepo is a repository on the fake server.
type FakeRepo struct {
	FullName string
	Private  bool
	Branch   string
	Head     string
}

// NewFakeGitHub starts the server on a random local port.
func NewFakeGitHub() *FakeGitHub { return NewFakeGitHubOn("") }

// NewFakeGitHubOn starts the server on addr ("" for a random port).
func NewFakeGitHubOn(addr string) *FakeGitHub {
	g := &FakeGitHub{tokens: map[string]string{}, devices: map[string]int{}, blobs: map[string][]byte{},
		trees: map[string]map[string]string{}, commits: map[string]fakeCommit{}, repos: map[string]*FakeRepo{},
		Login: "octo", Installed: true, Requests: map[string]int{}}
	g.Server = httptest.NewUnstartedServer(http.HandlerFunc(g.serve))
	if addr != "" {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			panic(err)
		}
		g.Server.Listener.Close()
		g.Server.Listener = l
	}
	g.Server.Start()
	return g
}

// AddRepo creates an empty repository.
func (g *FakeGitHub) AddRepo(fullName string, private bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.repos[fullName] = &FakeRepo{FullName: fullName, Private: private, Branch: "main"}
}

// Files returns path -> content at the repository head.
func (g *FakeGitHub) Files(fullName string) map[string]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := map[string]string{}
	r := g.repos[fullName]
	if r == nil || r.Head == "" {
		return out
	}
	for p, sha := range g.trees[g.commits[r.Head].Tree] {
		out[p] = string(g.blobs[sha])
	}
	return out
}

// CommitCount returns the number of commits on the branch.
func (g *FakeGitHub) CommitCount(fullName string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for sha := g.repos[fullName].Head; sha != ""; {
		n++
		c := g.commits[sha]
		if len(c.Parents) == 0 {
			break
		}
		sha = c.Parents[0]
	}
	return n
}

// WriteFile commits a file directly (simulating another client).
func (g *FakeGitHub) WriteFile(fullName, path string, content []byte) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.repos[fullName]
	files := map[string]string{}
	if r.Head != "" {
		for p, s := range g.trees[g.commits[r.Head].Tree] {
			files[p] = s
		}
	}
	files[path] = g.putBlob(content)
	tree := g.putTree(files)
	r.Head = g.putCommit(tree, r.Head)
}

func gitSHA(kind string, data []byte) string {
	h := sha1.New()
	h.Write([]byte(kind + " " + strconv.Itoa(len(data)) + "\x00"))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func (g *FakeGitHub) putBlob(data []byte) string {
	sha := gitSHA("blob", data)
	g.blobs[sha] = append([]byte(nil), data...)
	return sha
}

func (g *FakeGitHub) putTree(files map[string]string) string {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, p := range paths {
		b.WriteString(p + " " + files[p] + "\n")
	}
	sha := gitSHA("tree", []byte(b.String()))
	g.trees[sha] = files
	return sha
}

func (g *FakeGitHub) putCommit(tree, parent string) string {
	c := fakeCommit{Tree: tree}
	if parent != "" {
		c.Parents = []string{parent}
	}
	sha := gitSHA("commit", []byte(fmt.Sprintf("%s %v %d", tree, c.Parents, len(g.commits))))
	g.commits[sha] = c
	return sha
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func msg(w http.ResponseWriter, status int, m string) {
	writeJSON(w, status, map[string]string{"message": m})
}

func (g *FakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/login/device/code" && r.Method == http.MethodPost:
		_ = r.ParseForm()
		if r.Form.Get("client_id") == "" {
			writeJSON(w, 200, map[string]string{"error": "incorrect_client_credentials"})
			return
		}
		g.mu.Lock()
		g.nextDev++
		code := fmt.Sprintf("dev-%d", g.nextDev)
		g.devices[code] = 0
		g.mu.Unlock()
		writeJSON(w, 200, map[string]any{"device_code": code, "user_code": "ABCD-1234",
			"verification_uri": g.URL + "/login/device", "expires_in": 900, "interval": 1})
		return
	case path == "/login/oauth/access_token" && r.Method == http.MethodPost:
		_ = r.ParseForm()
		g.mu.Lock()
		defer g.mu.Unlock()
		code := r.Form.Get("device_code")
		polls, ok := g.devices[code]
		if !ok {
			writeJSON(w, 200, map[string]string{"error": "incorrect_device_code"})
			return
		}
		g.devices[code] = polls + 1
		switch {
		case g.DenyLogin:
			writeJSON(w, 200, map[string]string{"error": "access_denied"})
		case polls == 0:
			writeJSON(w, 200, map[string]string{"error": "authorization_pending"})
		default:
			tok := fmt.Sprintf("ghu_%s_%d", g.Login, len(g.tokens)+1)
			g.tokens[tok] = g.Login
			delete(g.devices, code)
			writeJSON(w, 200, map[string]string{"access_token": tok, "token_type": "bearer", "scope": ""})
		}
		return
	}

	g.mu.Lock()
	login, ok := g.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	key := r.Method + " " + path
	if i := strings.Index(path, "/git/"); i >= 0 {
		key = r.Method + " " + path[i:]
		if j := strings.Index(key[len(r.Method)+6:], "/"); j >= 0 {
			key = key[:len(r.Method)+6+j]
		}
	}
	g.Requests[key]++
	g.mu.Unlock()
	if !ok {
		msg(w, 401, "Bad credentials")
		return
	}

	switch {
	case path == "/user":
		writeJSON(w, 200, map[string]string{"login": login})
		return
	case path == "/user/installations":
		g.mu.Lock()
		inst := []map[string]any{}
		if g.Installed {
			inst = append(inst, map[string]any{"id": 1})
		}
		g.mu.Unlock()
		writeJSON(w, 200, map[string]any{"installations": inst})
		return
	case path == "/user/installations/1/repositories":
		g.mu.Lock()
		repos := []map[string]any{}
		if r.URL.Query().Get("page") == "1" || r.URL.Query().Get("page") == "" {
			for _, rp := range g.repos {
				repos = append(repos, g.repoJSON(rp))
			}
		}
		g.mu.Unlock()
		writeJSON(w, 200, map[string]any{"repositories": repos})
		return
	}

	if !strings.HasPrefix(path, "/repos/") {
		msg(w, 404, "Not Found")
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(path, "/repos/"), "/", 3)
	if len(parts) < 2 {
		msg(w, 404, "Not Found")
		return
	}
	name := parts[0] + "/" + parts[1]
	rest := ""
	if len(parts) == 3 {
		rest = parts[2]
	}
	g.mu.Lock()
	repo := g.repos[name]
	if repo == nil || !g.Installed {
		g.mu.Unlock()
		msg(w, 404, "Not Found")
		return
	}
	g.mu.Unlock()
	g.serveRepo(w, r, repo, rest)
}

func (g *FakeGitHub) repoJSON(r *FakeRepo) map[string]any {
	return map[string]any{"full_name": r.FullName, "private": r.Private, "default_branch": r.Branch,
		"html_url": g.URL + "/" + r.FullName}
}

func (g *FakeGitHub) serveRepo(w http.ResponseWriter, r *http.Request, repo *FakeRepo, rest string) {
	var body map[string]any
	if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPatch || r.Method == http.MethodPut) {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	g.mu.Lock()
	unlock := true
	defer func() {
		if unlock {
			g.mu.Unlock()
		}
	}()
	switch {
	case rest == "" && r.Method == http.MethodGet:
		writeJSON(w, 200, g.repoJSON(repo))

	case rest == "git/ref/heads/"+repo.Branch && r.Method == http.MethodGet:
		if repo.Head == "" {
			msg(w, 409, "Git Repository is empty.")
			return
		}
		writeJSON(w, 200, map[string]any{"object": map[string]string{"sha": repo.Head, "type": "commit"}})

	case strings.HasPrefix(rest, "git/commits/") && r.Method == http.MethodGet:
		c, ok := g.commits[strings.TrimPrefix(rest, "git/commits/")]
		if !ok {
			msg(w, 404, "Not Found")
			return
		}
		writeJSON(w, 200, map[string]any{"tree": map[string]string{"sha": c.Tree}})

	case strings.HasPrefix(rest, "git/trees/") && r.Method == http.MethodGet:
		files, ok := g.trees[strings.TrimPrefix(rest, "git/trees/")]
		if !ok {
			msg(w, 404, "Not Found")
			return
		}
		entries := []map[string]string{}
		dirs := map[string]bool{}
		for p, sha := range files {
			if i := strings.LastIndex(p, "/"); i > 0 && !dirs[p[:i]] {
				dirs[p[:i]] = true
				entries = append(entries, map[string]string{"path": p[:i], "type": "tree", "sha": "dir"})
			}
			entries = append(entries, map[string]string{"path": p, "type": "blob", "sha": sha})
		}
		writeJSON(w, 200, map[string]any{"sha": strings.TrimPrefix(rest, "git/trees/"), "tree": entries, "truncated": false})

	case strings.HasPrefix(rest, "git/blobs/") && r.Method == http.MethodGet:
		b, ok := g.blobs[strings.TrimPrefix(rest, "git/blobs/")]
		if !ok {
			msg(w, 404, "Not Found")
			return
		}
		enc := base64.StdEncoding.EncodeToString(b)
		// GitHub wraps base64 content at 60 columns.
		var wrapped strings.Builder
		for len(enc) > 60 {
			wrapped.WriteString(enc[:60] + "\n")
			enc = enc[60:]
		}
		wrapped.WriteString(enc)
		writeJSON(w, 200, map[string]string{"content": wrapped.String(), "encoding": "base64"})

	case rest == "git/trees" && r.Method == http.MethodPost:
		files := map[string]string{}
		if base, _ := body["base_tree"].(string); base != "" {
			bt, ok := g.trees[base]
			if !ok {
				msg(w, 422, "base_tree not found")
				return
			}
			for p, s := range bt {
				files[p] = s
			}
		}
		entries, _ := body["tree"].([]any)
		for _, raw := range entries {
			e, _ := raw.(map[string]any)
			p, _ := e["path"].(string)
			if content, ok := e["content"].(string); ok {
				files[p] = g.putBlob([]byte(content))
				continue
			}
			if v, present := e["sha"]; present && v == nil {
				if _, ok := files[p]; !ok {
					msg(w, 422, "tree.path "+p+" does not exist")
					return
				}
				delete(files, p)
				continue
			}
			if sha, ok := e["sha"].(string); ok {
				files[p] = sha
			}
		}
		writeJSON(w, 201, map[string]string{"sha": g.putTree(files)})

	case rest == "git/commits" && r.Method == http.MethodPost:
		tree, _ := body["tree"].(string)
		if _, ok := g.trees[tree]; !ok {
			msg(w, 422, "tree not found")
			return
		}
		parent := ""
		if ps, _ := body["parents"].([]any); len(ps) > 0 {
			parent, _ = ps[0].(string)
		}
		writeJSON(w, 201, map[string]string{"sha": g.putCommit(tree, parent)})

	case rest == "git/refs/heads/"+repo.Branch && r.Method == http.MethodPatch:
		if hook := g.BeforeRefUpdate; hook != nil {
			g.mu.Unlock()
			hook(repo.FullName)
			g.mu.Lock()
		}
		sha, _ := body["sha"].(string)
		c, ok := g.commits[sha]
		if !ok {
			msg(w, 422, "Object does not exist")
			return
		}
		if force, _ := body["force"].(bool); !force && (len(c.Parents) == 0 || c.Parents[0] != repo.Head) {
			msg(w, 422, "Update is not a fast forward")
			return
		}
		repo.Head = sha
		writeJSON(w, 200, map[string]any{"object": map[string]string{"sha": sha}})

	case strings.HasPrefix(rest, "contents/") && r.Method == http.MethodPut:
		p := strings.TrimPrefix(rest, "contents/")
		content, _ := body["content"].(string)
		data, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			msg(w, 422, "content is not valid Base64")
			return
		}
		files := map[string]string{}
		if repo.Head != "" {
			for k, v := range g.trees[g.commits[repo.Head].Tree] {
				files[k] = v
			}
			if _, exists := files[p]; exists && body["sha"] == nil {
				msg(w, 422, "\"sha\" wasn't supplied.")
				return
			}
		}
		files[p] = g.putBlob(data)
		repo.Head = g.putCommit(g.putTree(files), repo.Head)
		writeJSON(w, 201, map[string]any{"commit": map[string]string{"sha": repo.Head}})

	default:
		msg(w, 404, "Not Found")
	}
}
