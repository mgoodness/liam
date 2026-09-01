package main

import (
	"testing"

	"github.com/mgoodness/liam/internal/tool"
)

// TestFindGrepSearchersReturnsStdlibUnconditionally covers issue #97's
// removal of the fff-mcp special-case: find/grep are backed by
// tool.StdlibSearch regardless of the environment, with no detection or
// spawn attempt of any kind.
func TestFindGrepSearchersReturnsStdlibUnconditionally(t *testing.T) {
	findSearcher, grepSearcher := findGrepSearchers(t.TempDir())

	if _, ok := findSearcher.(tool.StdlibSearch); !ok {
		t.Errorf("findSearcher = %T, want tool.StdlibSearch", findSearcher)
	}
	if _, ok := grepSearcher.(tool.StdlibSearch); !ok {
		t.Errorf("grepSearcher = %T, want tool.StdlibSearch", grepSearcher)
	}
}
