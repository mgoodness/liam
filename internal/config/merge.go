package config

// mergeMaps deep-merges src onto dst, with src winning on conflicting keys,
// and returns a new map — neither dst nor src is mutated. Two maps at the
// same key are merged recursively; any other conflicting value pair (or a
// map on one side and a non-map on the other) resolves to src's value.
func mergeMaps(dst, src map[string]any) map[string]any {
	out := make(map[string]any, len(dst)+len(src))
	for k, v := range dst {
		out[k] = v
	}
	for k, sv := range src {
		if sm, ok := sv.(map[string]any); ok {
			if dm, ok := out[k].(map[string]any); ok {
				out[k] = mergeMaps(dm, sm)
				continue
			}
		}
		out[k] = sv
	}
	return out
}
