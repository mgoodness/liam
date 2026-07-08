package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// defaultClientID is the GitHub App client ID VS Code uses for its
	// Copilot Chat extension. Third-party device-flow clients (copilot.vim,
	// avante.nvim, etc.) use the same ID since GitHub has not published one
	// for external tools; see ADR 0001. It's fixed across every known
	// third-party client, not a per-deployment setting.
	defaultClientID = "Iv1.b507a08c87ecfe98"
	defaultScope    = "read:user"

	grantTypeDeviceCode = "urn:ietf:params:oauth:grant-type:device_code"
)

// Endpoints holds the GitHub URLs used during device-flow login and Copilot
// token exchange. Overridable so tests can point them at an httptest.Server.
type Endpoints struct {
	DeviceCode   string
	AccessToken  string
	CopilotToken string
}

var defaultEndpoints = Endpoints{
	DeviceCode:   "https://github.com/login/device/code",
	AccessToken:  "https://github.com/login/oauth/access_token",
	CopilotToken: "https://api.github.com/copilot_internal/v2/token",
}

// ErrTokenInvalid indicates the stored GitHub OAuth token was rejected by
// GitHub (revoked or otherwise invalid), as opposed to a transient failure
// or an unrelated API error (e.g. no active Copilot seat).
var ErrTokenInvalid = errors.New("github oauth token invalid or revoked")

// Prompt presents the device-flow verification URL and user code to the
// person completing the login.
type Prompt func(verificationURI, userCode string)

func defaultPrompt(verificationURI, userCode string) {
	fmt.Fprintf(os.Stderr, "First, copy your one-time code: %s\nThen open %s in your browser to authorize liam.\n", userCode, verificationURI)
}

// Authenticator performs GitHub device-flow login and Copilot token
// exchange, and persists the result to the XDG credentials file.
//
// The zero value is not usable; construct one with NewAuthenticator.
type Authenticator struct {
	HTTPClient      *http.Client
	Endpoints       Endpoints
	CredentialsPath string // overrides the resolved XDG path; mainly for tests
	Prompt          Prompt
	Now             func() time.Time
	Sleep           func(time.Duration)
}

// NewAuthenticator returns an Authenticator configured against the real
// GitHub endpoints.
func NewAuthenticator() *Authenticator {
	return &Authenticator{
		HTTPClient: http.DefaultClient,
		Endpoints:  defaultEndpoints,
		Prompt:     defaultPrompt,
		Now:        time.Now,
		Sleep:      time.Sleep,
	}
}

// Authenticate returns valid Credentials, loading and refreshing the stored
// ones where possible:
//
//   - No credentials file: runs the full device-flow login.
//   - Copilot token still valid: returns the stored credentials as-is.
//   - Copilot token expired: re-exchanges the stored GitHub token for a new
//     Copilot token.
//   - GitHub token invalid/revoked (the re-exchange fails with
//     ErrTokenInvalid): runs the full device-flow login again.
//
// Any other refresh failure (network error, non-auth API error) is returned
// to the caller as-is rather than triggering a re-login.
//
// In every success case, the result is persisted before it's returned.
func (a *Authenticator) Authenticate(ctx context.Context) (*Credentials, error) {
	path, err := a.path()
	if err != nil {
		return nil, err
	}

	creds, err := loadCredentials(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return a.login(ctx, path)
	case err != nil:
		return nil, err
	}

	if creds.copilotTokenValid(a.Now()) {
		return creds, nil
	}

	if err := a.refreshCopilotToken(ctx, creds); err != nil {
		if errors.Is(err, ErrTokenInvalid) {
			return a.login(ctx, path)
		}
		return nil, err
	}

	if err := saveCredentials(path, creds); err != nil {
		return nil, err
	}
	return creds, nil
}

// Login forces a full device-flow login, bypassing any cached
// credentials, and persists the result. Callers reach for this when a
// Copilot API request itself is rejected with an auth error despite the
// Authenticator's own cache believing the token was still valid — e.g.
// the Copilot token was revoked out from under it — as opposed to
// Authenticate, which trusts a not-yet-expired cached token.
func (a *Authenticator) Login(ctx context.Context) (*Credentials, error) {
	path, err := a.path()
	if err != nil {
		return nil, err
	}
	return a.login(ctx, path)
}

func (a *Authenticator) path() (string, error) {
	if a.CredentialsPath != "" {
		return a.CredentialsPath, nil
	}
	return credentialsPath()
}

// login runs the full device-flow exchange from scratch and persists the
// resulting credentials.
func (a *Authenticator) login(ctx context.Context, path string) (*Credentials, error) {
	dc, err := a.requestDeviceCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("requesting device code: %w", err)
	}

	a.Prompt(dc.VerificationURI, dc.UserCode)

	githubToken, err := a.pollAccessToken(ctx, dc)
	if err != nil {
		return nil, fmt.Errorf("polling for access token: %w", err)
	}

	creds := &Credentials{GitHubToken: githubToken}
	if err := a.refreshCopilotToken(ctx, creds); err != nil {
		return nil, fmt.Errorf("exchanging copilot token: %w", err)
	}

	if err := saveCredentials(path, creds); err != nil {
		return nil, err
	}
	return creds, nil
}

// refreshCopilotToken re-exchanges creds.GitHubToken for a new Copilot
// token, updating creds in place.
func (a *Authenticator) refreshCopilotToken(ctx context.Context, creds *Credentials) error {
	ct, err := a.exchangeCopilotToken(ctx, creds.GitHubToken)
	if err != nil {
		return err
	}
	creds.CopilotToken = ct.Token
	creds.CopilotTokenExpiry = time.Unix(ct.ExpiresAt, 0)
	return nil
}

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

func (a *Authenticator) requestDeviceCode(ctx context.Context) (*deviceCodeResponse, error) {
	form := url.Values{"client_id": {defaultClientID}, "scope": {defaultScope}}
	var out deviceCodeResponse
	if err := postForm(ctx, a.HTTPClient, a.Endpoints.DeviceCode, form, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type accessTokenResponse struct {
	AccessToken      string `json:"access_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// pollAccessToken polls the access-token endpoint per the device-flow spec
// until the user completes authorization, the device code expires, or an
// unrecoverable error is returned.
func (a *Authenticator) pollAccessToken(ctx context.Context, dc *deviceCodeResponse) (string, error) {
	interval := time.Duration(dc.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := a.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)

	for {
		a.Sleep(interval)

		if a.Now().After(deadline) {
			return "", errors.New("device flow login timed out")
		}

		out, err := a.fetchAccessToken(ctx, dc.DeviceCode)
		if err != nil {
			return "", err
		}

		switch out.Error {
		case "":
			if out.AccessToken == "" {
				return "", errors.New("empty access token in response")
			}
			return out.AccessToken, nil
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		default:
			return "", fmt.Errorf("%s: %s", out.Error, out.ErrorDescription)
		}
	}
}

func (a *Authenticator) fetchAccessToken(ctx context.Context, deviceCode string) (*accessTokenResponse, error) {
	form := url.Values{
		"client_id":   {defaultClientID},
		"device_code": {deviceCode},
		"grant_type":  {grantTypeDeviceCode},
	}
	var out accessTokenResponse
	if err := postForm(ctx, a.HTTPClient, a.Endpoints.AccessToken, form, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type copilotTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

// exchangeCopilotToken trades a GitHub OAuth token for a short-lived
// Copilot token. A 401 means the GitHub token itself was rejected
// (ErrTokenInvalid, which should trigger a full re-login); any other
// non-200 status (e.g. a 403 for an account with no active Copilot seat) is
// a distinct, non-auth failure that's returned as-is.
func (a *Authenticator) exchangeCopilotToken(ctx context.Context, githubToken string) (*copilotTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.Endpoints.CopilotToken, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Accept", "application/json")

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrTokenInvalid
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %s: %s", resp.Status, body)
	}

	var out copilotTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// postForm POSTs a URL-encoded form and decodes a JSON response into out.
// It does not itself distinguish status codes beyond 200 vs. not-200;
// callers that need to special-case a particular status (as
// exchangeCopilotToken does for 401) handle the request directly instead.
func postForm(ctx context.Context, client *http.Client, rawURL string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %s: %s", resp.Status, body)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
