package render

import (
	"os"
	"strings"
)

// CollapseHome shortens path to "~" (or "~" plus the remainder) when it
// falls inside the user's home directory, the convention shared by every
// shell and CLI tool for displaying filesystem paths. It's a display-only
// transform — callers that pass a path to git, os.Open, or any other
// filesystem/exec call must use the original, uncollapsed path, since
// os/exec never expands "~" itself. os.UserHomeDir failing (no $HOME set,
// etc.) leaves path unchanged rather than erroring, since this only ever
// feeds display text.
func CollapseHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(os.PathSeparator)); ok {
		return "~" + string(os.PathSeparator) + rest
	}
	return path
}
