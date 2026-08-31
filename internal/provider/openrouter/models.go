package openrouter

import (
	"context"
	"fmt"
	"strings"
)

// MaxContextLength returns model's maximum context length in tokens,
// fetched from OpenRouter's GET /models/{author}/{slug} endpoint and
// memoized per model id on p — necessary since "openrouter/auto" can
// route to a different underlying model turn to turn, and repeated
// lookups for the same model shouldn't re-hit the network. It implements
// session.ContextLookup.
func (p *Provider) MaxContextLength(ctx context.Context, model string) (int, error) {
	p.contextMu.Lock()
	n, ok := p.contextCache[model]
	p.contextMu.Unlock()
	if ok {
		return n, nil
	}

	author, slug, ok := splitModelID(model)
	if !ok {
		return 0, fmt.Errorf("openrouter: model id %q is not in author/slug form", model)
	}

	resp, err := p.client.Models.Get(ctx, author, slug)
	if err != nil {
		return 0, classifyError(err)
	}
	cl := resp.Data.ContextLength
	if cl == nil || *cl <= 0 {
		return 0, fmt.Errorf("openrouter: model %q reported no context_length", model)
	}
	n = int(*cl)

	p.contextMu.Lock()
	p.contextCache[model] = n
	p.contextMu.Unlock()
	return n, nil
}

// splitModelID splits model on its first "/" into author and slug, as
// OpenRouter's get-model-by-slug endpoint requires (e.g.
// "anthropic/claude-3.7-sonnet" -> "anthropic", "claude-3.7-sonnet"). The
// slug half may itself carry a variant suffix (e.g. "openai/gpt-4:free"),
// which the endpoint resolves on its own.
func splitModelID(model string) (author, slug string, ok bool) {
	i := strings.IndexByte(model, '/')
	if i <= 0 || i == len(model)-1 {
		return "", "", false
	}
	return model[:i], model[i+1:], true
}
