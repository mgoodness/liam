package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mgoodness/liam/internal/tool"
)

func TestFindGrepSearchersFallsBackToStdlibWithoutFFF(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // fff-mcp isn't resolvable

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
