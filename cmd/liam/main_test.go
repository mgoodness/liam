package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresPromptFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "-p") {
		t.Errorf("stderr = %q, want a mention of -p", stderr.String())
	}
}

func TestRunRequiresAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-p", "hello"}, &stdout, &stderr)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "OPENROUTER_API_KEY") {
		t.Errorf("stderr = %q, want a mention of OPENROUTER_API_KEY", stderr.String())
	}
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-bogus"}, &stdout, &stderr)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}
