package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mgoodness/liam/internal/tool"
)

// TestFindGrepSearchersReturnsStdlibUnconditionally covers issue #97's
// removal of the fff-mcp special-case: find/grep are backed by
// tool.StdlibSearch regardless of the environment, with no detection or
// spawn attempt of any kind.
func TestFindGrepSearchersReturnsStdlibUnconditionally(t *testing.T) {
	var stderr bytes.Buffer
	findSearcher, grepSearcher := findGrepSearchers(t.TempDir(), &stderr)

	if _, ok := findSearcher.(tool.StdlibSearch); !ok {
		t.Errorf("findSearcher = %T, want tool.StdlibSearch", findSearcher)
	}
	if _, ok := grepSearcher.(tool.StdlibSearch); !ok {
		t.Errorf("grepSearcher = %T, want tool.StdlibSearch", grepSearcher)
	}
	if !strings.Contains(stderr.String(), "searcher=stdlib") {
		t.Errorf("stderr = %q, want a mention of searcher=stdlib", stderr.String())
	}
}
