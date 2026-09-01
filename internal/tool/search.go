package tool

import "context"

// MaxSearchResults caps how many results find/grep surface to the model
// before appending a truncation notice, matching OpenCode's own 100-result
// cap (docs/research/fff-integration.md) — the reference "proven shape"
// ticket #18's resolution cites for liam's output convention. Exported so
// callers outside this package (e.g. internal/tui's "@" mention popup) can
// reference the same cap.
const MaxSearchResults = 100

// GrepMatch is one line match from a Grep search, already relative to the
// search root.
type GrepMatch struct {
	File string
	Line int
	Text string
}

// GrepSearcher performs Grep's content search — StdlibSearch is currently
// the only implementer, after issue #97 removed the fff-mcp special-case
// that once backed it as an alternative. Not called "GrepBackend" to avoid
// colliding with CONTEXT.md's Provider entry, which reserves "backend" for
// that unrelated concept. matches is already capped at MaxSearchResults and
// grouped by file in encounter order; total is the full match count found,
// which may exceed len(matches).
type GrepSearcher interface {
	Grep(ctx context.Context, query string) (matches []GrepMatch, total int, err error)
}

// FindSearcher performs Find's path search, implemented the same way as
// GrepSearcher. paths is already capped at MaxSearchResults; total may
// exceed len(paths).
type FindSearcher interface {
	Find(ctx context.Context, query string) (paths []string, total int, err error)
}
