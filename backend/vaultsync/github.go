package vaultsync

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Endpoints are the GitHub base URLs (overridable for tests and GHES).
type Endpoints struct {
	Web string // https://github.com
	API string // https://api.github.com
}

// DefaultEndpoints are github.com's.
var DefaultEndpoints = Endpoints{Web: "https://github.com", API: "https://api.github.com"}

// Error codes surfaced to the UI.
const (
	CodeAuth        = "github_auth"         // token missing, expired or revoked
	CodeNoAccess    = "github_no_access"    // the app can't see the repository
	CodeDenied      = "github_denied"       // the user declined sign-in
	CodeExpired     = "github_code_expired" // the device code timed out
	CodeNotSetUp    = "github_not_set_up"   // no GitHub App client ID configured
	CodeDeviceFlow  = "github_device_flow"  // device flow disabled on the app
	CodeRateLimited = "github_rate_limited"
)

// APIError is a GitHub error with a UI code.
type APIError struct {
	Code    string
	Status  int
	Message string
}

func (e *APIError) Error() string { return e.Message }

// ErrCode lets sshconn.CodeOf report the code to the UI.
func (e *APIError) ErrCode() string { return e.Code }

// ErrorCode returns the UI code of err, if any.
func ErrorCode(err error) string {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

// GitHub talks to the GitHub REST API and OAuth endpoints.
type GitHub struct {
	Endpoints Endpoints
	ClientID  string
	Token     string
	HTTP      *http.Client
}

func (g *GitHub) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// ---------------------------------------------------------------- device flow

// DeviceCode is what the user needs to approve sign-in.
type DeviceCode struct {
	DeviceCode      string `json:"-"`
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
	ExpiresIn       int    `json:"expiresIn"`
	Interval        int    `json:"interval"`
}

func (g *GitHub) oauthPost(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.Endpoints.Web+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := g.client().Do(req)
	if err != nil {
		return friendlyNetErr(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusNotFound {
		return &APIError{Code: CodeNotSetUp, Status: 404, Message: "GitHub didn't recognise the app's client ID; check it in Settings → Sync"}
	}
	if resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, Message: fmt.Sprintf("GitHub sign-in failed (%s)", resp.Status)}
	}
	return json.Unmarshal(body, out)
}

// StartDeviceFlow requests a code for the user to enter on github.com.
func (g *GitHub) StartDeviceFlow(ctx context.Context) (DeviceCode, error) {
	if g.ClientID == "" {
		return DeviceCode{}, &APIError{Code: CodeNotSetUp, Message: "set up the Flux GitHub app first"}
	}
	var r struct {
		DeviceCode       string `json:"device_code"`
		UserCode         string `json:"user_code"`
		VerificationURI  string `json:"verification_uri"`
		ExpiresIn        int    `json:"expires_in"`
		Interval         int    `json:"interval"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := g.oauthPost(ctx, "/login/device/code", url.Values{"client_id": {g.ClientID}}, &r); err != nil {
		return DeviceCode{}, err
	}
	if r.Error != "" {
		return DeviceCode{}, deviceErr(r.Error, r.ErrorDescription)
	}
	if r.Interval <= 0 {
		r.Interval = 5
	}
	return DeviceCode{DeviceCode: r.DeviceCode, UserCode: r.UserCode, VerificationURI: r.VerificationURI,
		ExpiresIn: r.ExpiresIn, Interval: r.Interval}, nil
}

func deviceErr(code, desc string) error {
	switch code {
	case "access_denied":
		return &APIError{Code: CodeDenied, Message: "sign-in was cancelled on GitHub"}
	case "expired_token", "token_expired":
		return &APIError{Code: CodeExpired, Message: "the sign-in code expired; start again"}
	case "device_flow_disabled":
		return &APIError{Code: CodeDeviceFlow, Message: "device flow is turned off for this GitHub app; enable it in the app's settings"}
	case "incorrect_client_credentials":
		return &APIError{Code: CodeNotSetUp, Message: "GitHub didn't recognise the app's client ID; check it in Settings → Sync"}
	}
	if desc == "" {
		desc = code
	}
	return &APIError{Message: "GitHub sign-in failed: " + desc}
}

// PollDeviceToken waits until the user approves (or declines) sign-in and
// returns the access token.
func (g *GitHub) PollDeviceToken(ctx context.Context, dc DeviceCode, unit time.Duration) (string, error) {
	if unit == 0 {
		unit = time.Second
	}
	interval := time.Duration(dc.Interval) * unit
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
		if dc.ExpiresIn > 0 && time.Now().After(deadline) {
			return "", deviceErr("expired_token", "")
		}
		var r struct {
			AccessToken      string `json:"access_token"`
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
			Interval         int    `json:"interval"`
		}
		err := g.oauthPost(ctx, "/login/oauth/access_token", url.Values{
			"client_id":   {g.ClientID},
			"device_code": {dc.DeviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}, &r)
		if err != nil {
			return "", err
		}
		switch r.Error {
		case "":
			if r.AccessToken == "" {
				return "", errors.New("GitHub returned no access token")
			}
			return r.AccessToken, nil
		case "authorization_pending":
		case "slow_down":
			if r.Interval > 0 {
				interval = time.Duration(r.Interval) * unit
			} else {
				interval += 5 * unit
			}
		default:
			return "", deviceErr(r.Error, r.ErrorDescription)
		}
	}
}

// ---------------------------------------------------------------- REST

func (g *GitHub) do(ctx context.Context, method, path string, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.Endpoints.API+path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.client().Do(req)
	if err != nil {
		return 0, friendlyNetErr(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		msg := e.Message
		if msg == "" {
			msg = resp.Status
		}
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			return resp.StatusCode, &APIError{Code: CodeAuth, Status: 401, Message: "GitHub sign-in expired or was revoked; sign in again"}
		case (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) && resp.Header.Get("X-RateLimit-Remaining") == "0":
			return resp.StatusCode, &APIError{Code: CodeRateLimited, Status: resp.StatusCode, Message: "GitHub rate limit reached; sync will retry later"}
		}
		return resp.StatusCode, &APIError{Status: resp.StatusCode, Message: "GitHub: " + msg}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, fmt.Errorf("unexpected GitHub response: %w", err)
		}
	}
	return resp.StatusCode, nil
}

// User returns the signed-in user's login.
func (g *GitHub) User(ctx context.Context) (string, error) {
	var u struct {
		Login string `json:"login"`
	}
	_, err := g.do(ctx, http.MethodGet, "/user", nil, &u)
	return u.Login, err
}

// Repo describes a repository the app can access.
type Repo struct {
	FullName      string `json:"fullName"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"defaultBranch"`
	HTMLURL       string `json:"htmlUrl"`
}

type apiRepo struct {
	FullName      string `json:"full_name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	HTMLURL       string `json:"html_url"`
}

func (r apiRepo) repo() Repo {
	return Repo{FullName: r.FullName, Private: r.Private, DefaultBranch: r.DefaultBranch, HTMLURL: r.HTMLURL}
}

// AccessibleRepos lists repositories the app is installed on that the user
// can reach. installed is false when the app isn't installed anywhere.
func (g *GitHub) AccessibleRepos(ctx context.Context) (repos []Repo, installed bool, err error) {
	var inst struct {
		Installations []struct {
			ID int64 `json:"id"`
		} `json:"installations"`
	}
	if _, err := g.do(ctx, http.MethodGet, "/user/installations?per_page=100", nil, &inst); err != nil {
		return nil, false, err
	}
	repos = []Repo{}
	for _, in := range inst.Installations {
		for page := 1; page <= 10; page++ {
			var r struct {
				Repositories []apiRepo `json:"repositories"`
			}
			if _, err := g.do(ctx, http.MethodGet, fmt.Sprintf("/user/installations/%d/repositories?per_page=100&page=%d", in.ID, page), nil, &r); err != nil {
				return nil, true, err
			}
			for _, x := range r.Repositories {
				repos = append(repos, x.repo())
			}
			if len(r.Repositories) < 100 {
				break
			}
		}
	}
	return repos, len(inst.Installations) > 0, nil
}

// GetRepo loads one repository.
func (g *GitHub) GetRepo(ctx context.Context, fullName string) (Repo, error) {
	var r apiRepo
	status, err := g.do(ctx, http.MethodGet, "/repos/"+fullName, nil, &r)
	if status == http.StatusNotFound {
		return Repo{}, &APIError{Code: CodeNoAccess, Status: 404, Message: fmt.Sprintf("Flux can't see %s; make sure the Flux GitHub app has access to it", fullName)}
	}
	return r.repo(), err
}

func friendlyNetErr(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("can't reach GitHub (%v)", err)
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
