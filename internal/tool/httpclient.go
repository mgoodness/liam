package tool

import (
	"net/http"
	"time"
)

// defaultHTTPClient returns override unchanged when set — tests use this to
// point a tool at an httptest.Server-backed client with no timeout — or a
// new *http.Client{Timeout: timeout} otherwise. Shared by WebFetch and
// WebSearch so each network tool's "optional client override, else a
// timeout-bound default" isn't reimplemented per tool.
func defaultHTTPClient(override *http.Client, timeout time.Duration) *http.Client {
	if override != nil {
		return override
	}
	return &http.Client{Timeout: timeout}
}
