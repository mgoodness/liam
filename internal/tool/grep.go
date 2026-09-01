package tool

import (
	"context"
	"fmt"
)

// Grep searches file contents line by line under the working directory,
// backed by a GrepSearcher — StdlibSearch as of issue #97's removal of the
// fff-mcp special-case.
type Grep struct {
	Searcher GrepSearcher
}

func (Grep) Name() string { return "grep" }
func (Grep) Description() string {
	return "Search file contents for a regular expression, matched line by line. Results are capped at 100 matches."
}

func (Grep) Parameters() Schema {
	return Schema{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Regular expression to search for within file contents.",
			},
		},
		"required": []string{"query"},
	}
}

func (Grep) Safety() Safety {
	return Safety{SideEffect: SideEffectRead}
}

func (g Grep) Run(ctx context.Context, args map[string]any) Result {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return errorResult(`grep: "query" argument is required`)
	}

	matches, total, err := g.Searcher.Grep(ctx, query)
	if err != nil {
		return errorResult(fmt.Sprintf("grep: %v", err))
	}
	return Result{Content: formatGrepResults(matches, total)}
}
