package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// fakeClock lets tests control time deterministically: Sleep advances it,
// so device-flow polling deadlines are exercised without real waiting.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(d time.Duration) { c.now = c.now.Add(d) }

func jsonServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	if handler == nil {
		handler = func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected request to %s", r.URL)
		}
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

// authTestCase exercises Authenticate against a fixed scenario: whatever
// credentials (if any) are on disk beforehand, and how each of the three
// GitHub endpoints responds.
type authTestCase struct {
	name string

	// initialCreds, given the fake clock's current time, returns the
	// credentials to persist before Authenticate runs. Nil means no
	// credentials file exists (first-run login).
	initialCreds func(now time.Time) *Credentials

	deviceCode   http.HandlerFunc
	accessToken  http.HandlerFunc
	copilotToken http.HandlerFunc

	wantErr bool
	check   func(t *testing.T, a *Authenticator, got *Credentials)
}

func TestAuthenticate(t *testing.T) {
	cases := []authTestCase{
		{
			name: "first run performs device flow login and persists",
			deviceCode: func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Fatalf("parse form: %v", err)
				}
				if got := r.FormValue("client_id"); got != defaultClientID {
					t.Errorf("device code client_id = %q, want %q", got, defaultClientID)
				}
				if got := r.FormValue("scope"); got != defaultScope {
					t.Errorf("device code scope = %q, want %q", got, defaultScope)
				}
				writeJSON(t, w, deviceCodeResponse{
					DeviceCode:      "device-123",
					UserCode:        "USER-CODE",
					VerificationURI: "https://github.com/login/device",
					ExpiresIn:       900,
					Interval:        1,
				})
			},
			accessToken: func() http.HandlerFunc {
				calls := 0
				return func(w http.ResponseWriter, r *http.Request) {
					calls++
					if err := r.ParseForm(); err != nil {
						t.Fatalf("parse form: %v", err)
					}
					if got := r.FormValue("client_id"); got != defaultClientID {
						t.Errorf("access token client_id = %q, want %q", got, defaultClientID)
					}
					if got := r.FormValue("device_code"); got != "device-123" {
						t.Errorf("access token device_code = %q, want device-123", got)
					}
					if got := r.FormValue("grant_type"); got != grantTypeDeviceCode {
						t.Errorf("access token grant_type = %q, want %q", got, grantTypeDeviceCode)
					}
					// Pending on the first poll, then success, exercising the
					// polling loop rather than a single-shot success.
					if calls == 1 {
						writeJSON(t, w, accessTokenResponse{Error: "authorization_pending"})
						return
					}
					writeJSON(t, w, accessTokenResponse{AccessToken: "gho_test123"})
				}
			}(),
			copilotToken: func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "token gho_test123" {
					t.Errorf("Authorization header = %q, want %q", got, "token gho_test123")
				}
				// GitHub's real backend 403s "Please only use approved
				// clients for Copilot" without these — the mock can't
				// enforce that itself, but it can lock down that we always
				// send them.
				if got := r.Header.Get("Editor-Version"); got != editorVersion {
					t.Errorf("Editor-Version header = %q, want %q", got, editorVersion)
				}
				if got := r.Header.Get("Editor-Plugin-Version"); got != editorPluginVersion {
					t.Errorf("Editor-Plugin-Version header = %q, want %q", got, editorPluginVersion)
				}
				if got := r.Header.Get("User-Agent"); got != copilotUserAgent {
					t.Errorf("User-Agent header = %q, want %q", got, copilotUserAgent)
				}
				writeJSON(t, w, copilotTokenResponse{Token: "tid=copilot123", ExpiresAt: time.Now().Add(time.Hour).Unix()})
			},
			check: func(t *testing.T, a *Authenticator, got *Credentials) {
				if got.GitHubToken != "gho_test123" {
					t.Errorf("GitHubToken = %q, want gho_test123", got.GitHubToken)
				}
				if got.CopilotToken != "tid=copilot123" {
					t.Errorf("CopilotToken = %q, want tid=copilot123", got.CopilotToken)
				}
				onDisk, err := loadCredentials(a.CredentialsPath)
				if err != nil {
					t.Fatalf("loadCredentials: %v", err)
				}
				if onDisk.GitHubToken != got.GitHubToken || onDisk.CopilotToken != got.CopilotToken {
					t.Errorf("persisted credentials = %+v, want %+v", onDisk, got)
				}
			},
		},
		{
			name: "reuses stored credentials without network calls",
			initialCreds: func(now time.Time) *Credentials {
				return &Credentials{
					GitHubToken:        "gho_stored",
					CopilotToken:       "tid=stored",
					CopilotTokenExpiry: now.Add(time.Hour),
				}
			},
			check: func(t *testing.T, a *Authenticator, got *Credentials) {
				if got.GitHubToken != "gho_stored" || got.CopilotToken != "tid=stored" {
					t.Errorf("Authenticate = %+v, want stored credentials unchanged", got)
				}
			},
		},
		{
			name: "silently refreshes expired copilot token",
			initialCreds: func(now time.Time) *Credentials {
				return &Credentials{
					GitHubToken:        "gho_stored",
					CopilotToken:       "tid=old",
					CopilotTokenExpiry: now.Add(-time.Minute),
				}
			},
			copilotToken: func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "token gho_stored" {
					t.Errorf("Authorization header = %q, want %q", got, "token gho_stored")
				}
				writeJSON(t, w, copilotTokenResponse{Token: "tid=refreshed", ExpiresAt: time.Now().Add(time.Hour).Unix()})
			},
			check: func(t *testing.T, a *Authenticator, got *Credentials) {
				if got.GitHubToken != "gho_stored" {
					t.Errorf("GitHubToken changed on refresh: got %q, want gho_stored", got.GitHubToken)
				}
				if got.CopilotToken != "tid=refreshed" {
					t.Errorf("CopilotToken = %q, want tid=refreshed", got.CopilotToken)
				}
				onDisk, err := loadCredentials(a.CredentialsPath)
				if err != nil {
					t.Fatalf("loadCredentials: %v", err)
				}
				if onDisk.CopilotToken != "tid=refreshed" {
					t.Errorf("persisted CopilotToken = %q, want tid=refreshed", onDisk.CopilotToken)
				}
			},
		},
		{
			name: "re-runs device flow login when github token is revoked",
			initialCreds: func(now time.Time) *Credentials {
				return &Credentials{
					GitHubToken:        "gho_revoked",
					CopilotToken:       "tid=old",
					CopilotTokenExpiry: now.Add(-time.Minute),
				}
			},
			deviceCode: func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, deviceCodeResponse{
					DeviceCode:      "device-456",
					UserCode:        "NEW-CODE",
					VerificationURI: "https://github.com/login/device",
					ExpiresIn:       900,
					Interval:        1,
				})
			},
			accessToken: func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, accessTokenResponse{AccessToken: "gho_new"})
			},
			copilotToken: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") == "token gho_revoked" {
					w.WriteHeader(http.StatusUnauthorized)
					writeJSON(t, w, map[string]string{"error": "bad_credentials"})
					return
				}
				writeJSON(t, w, copilotTokenResponse{Token: "tid=fresh", ExpiresAt: time.Now().Add(time.Hour).Unix()})
			},
			check: func(t *testing.T, a *Authenticator, got *Credentials) {
				if got.GitHubToken != "gho_new" {
					t.Errorf("GitHubToken = %q, want gho_new (full re-login)", got.GitHubToken)
				}
				if got.CopilotToken != "tid=fresh" {
					t.Errorf("CopilotToken = %q, want tid=fresh", got.CopilotToken)
				}
				onDisk, err := loadCredentials(a.CredentialsPath)
				if err != nil {
					t.Fatalf("loadCredentials: %v", err)
				}
				if onDisk.GitHubToken != "gho_new" {
					t.Errorf("persisted GitHubToken = %q, want gho_new", onDisk.GitHubToken)
				}
			},
		},
		{
			name: "propagates a transient error without re-login",
			initialCreds: func(now time.Time) *Credentials {
				return &Credentials{
					GitHubToken:        "gho_stored",
					CopilotToken:       "tid=old",
					CopilotTokenExpiry: now.Add(-time.Minute),
				}
			},
			copilotToken: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: true,
		},
		{
			// A 403 (e.g. no active Copilot seat) is a valid-token, non-auth
			// failure — it must not be classified as ErrTokenInvalid, which
			// would otherwise send the user through a pointless re-login
			// that fails the same way again.
			name: "propagates a 403 without re-login",
			initialCreds: func(now time.Time) *Credentials {
				return &Credentials{
					GitHubToken:        "gho_no_seat",
					CopilotToken:       "tid=old",
					CopilotTokenExpiry: now.Add(-time.Minute),
				}
			},
			copilotToken: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deviceCodeSrv := jsonServer(t, tc.deviceCode)
			accessTokenSrv := jsonServer(t, tc.accessToken)
			copilotTokenSrv := jsonServer(t, tc.copilotToken)

			clock := &fakeClock{now: time.Now()}
			a := NewAuthenticator()
			a.Endpoints = Endpoints{
				DeviceCode:   deviceCodeSrv.URL,
				AccessToken:  accessTokenSrv.URL,
				CopilotToken: copilotTokenSrv.URL,
			}
			a.CredentialsPath = filepath.Join(t.TempDir(), "credentials.json")
			a.Prompt = func(string, string) {}
			a.Now = clock.Now
			a.Sleep = clock.Sleep

			if tc.initialCreds != nil {
				if err := saveCredentials(a.CredentialsPath, tc.initialCreds(clock.Now())); err != nil {
					t.Fatalf("saveCredentials: %v", err)
				}
			}

			got, err := a.Authenticate(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("Authenticate: want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Authenticate: %v", err)
			}
			tc.check(t, a, got)
		})
	}
}

// TestAuthenticatorLogin_ForcesFullFlowIgnoringCache confirms Login
// always runs the device-flow login even when an unexpired, cache-valid
// credential is already on disk — the behavior Copilot.Complete depends
// on to recover when the API rejects a token the cache still trusts.
func TestAuthenticatorLogin_ForcesFullFlowIgnoringCache(t *testing.T) {
	deviceCodeSrv := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, deviceCodeResponse{
			DeviceCode:      "device-forced",
			UserCode:        "FORCED-CODE",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       900,
			Interval:        0,
		})
	})
	accessTokenSrv := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, accessTokenResponse{AccessToken: "gho_forced"})
	})
	copilotTokenSrv := jsonServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, copilotTokenResponse{Token: "tid=forced", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	})

	a := NewAuthenticator()
	a.Endpoints = Endpoints{
		DeviceCode:   deviceCodeSrv.URL,
		AccessToken:  accessTokenSrv.URL,
		CopilotToken: copilotTokenSrv.URL,
	}
	a.CredentialsPath = filepath.Join(t.TempDir(), "credentials.json")
	a.Prompt = func(string, string) {}

	if err := saveCredentials(a.CredentialsPath, &Credentials{
		GitHubToken:        "gho_still_valid_looking",
		CopilotToken:       "tid=still_valid_looking",
		CopilotTokenExpiry: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("saveCredentials: %v", err)
	}

	got, err := a.Login(context.Background())
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if got.GitHubToken != "gho_forced" || got.CopilotToken != "tid=forced" {
		t.Errorf("Login = %+v, want freshly issued tokens despite valid cache", got)
	}

	onDisk, err := loadCredentials(a.CredentialsPath)
	if err != nil {
		t.Fatalf("loadCredentials: %v", err)
	}
	if onDisk.GitHubToken != "gho_forced" {
		t.Errorf("persisted GitHubToken = %q, want gho_forced", onDisk.GitHubToken)
	}
}
