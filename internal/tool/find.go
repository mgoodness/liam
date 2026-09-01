package tool

import (
	"context"
	"fmt"
)

// Find searches file paths under the working directory by name, backed by
// a FindSearcher — StdlibSearch as of issue #97's removal of the fff-mcp
// special-case.
type Find struct {
	Searcher FindSearcher
}

func (Find) Name() string { return "find" }
func (Find) Description() string {
	return "Search file paths by name (also covers directory listing when \"query\" is empty). Results are capped at 100."
}

func (Find) Parameters() Schema {
	return Schema{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Fuzzy filename or path substring to search for. Omit or leave empty to list every file.",
			},
		},
	}
}

func (Find) Safety() Safety {
	return Safety{SideEffect: SideEffectRead}
}

func (f Find) Run(ctx context.Context, args map[string]any) Result {
	query, _ := args["query"].(string)

	paths, total, err := f.Searcher.Find(ctx, query)
	if err != nil {
		return errorResult(fmt.Sprintf("find: %v", err))
	}
	return Result{Content: formatFindResults(paths, total)}
}
