package trace

import (
	"path/filepath"

	"github.com/mgoodness/liam/internal/xdg"
)

// tracesDir resolves $XDG_STATE_HOME/liam/traces, the ticket's spec'd
// directory for every session's JSONL trace file.
func tracesDir() (string, error) {
	base, err := xdg.StateHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "liam", "traces"), nil
}
