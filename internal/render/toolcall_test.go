package render

import "testing"

func TestToolCall(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		argsJSON string
		content  string
		isError  bool
		want     string
	}{
		{
			name:     "sorted args and short result",
			toolName: "grep",
			argsJSON: `{"path":"internal/csv","pattern":"func ParseCSV"}`,
			content:  "internal/csv/parser.go:14",
			want:     `grep(path: "internal/csv", pattern: "func ParseCSV") → internal/csv/parser.go:14`,
		},
		{
			name:     "multi-line content truncates to first line",
			toolName: "read",
			argsJSON: `{"path":"internal/csv/parser.go"}`,
			content:  "line one\nline two\nline three",
			want:     `read(path: "internal/csv/parser.go") → line one…`,
		},
		{
			name:     "error result is prefixed",
			toolName: "bash",
			argsJSON: `{"command":"go test ./..."}`,
			content:  "exit status 1",
			isError:  true,
			want:     `bash(command: "go test ./...") → error: exit status 1`,
		},
		{
			name:     "no args",
			toolName: "list",
			argsJSON: "",
			content:  "3 files",
			want:     `list() → 3 files`,
		},
		{
			name:     "long single-line content is truncated",
			toolName: "web_fetch",
			argsJSON: `{"url":"https://example.com"}`,
			content:  "0123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789",
			want:     `web_fetch(url: "https://example.com") → 0123456789012345678901234567890123456789012345678901234567890123456789012345678…`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ToolCall(tc.toolName, tc.argsJSON, tc.content, tc.isError)
			if got != tc.want {
				t.Errorf("ToolCall() = %q, want %q", got, tc.want)
			}
		})
	}
}
