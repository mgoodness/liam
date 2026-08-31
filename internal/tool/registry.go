package tool

// Registry is a lookup of Tools by name, keyed by each Tool's own Name().
type Registry map[string]Tool

// NewRegistry builds a Registry from tools, keyed by each Tool's Name().
// A later tool with the same Name() overwrites an earlier one.
func NewRegistry(tools ...Tool) Registry {
	r := make(Registry, len(tools))
	for _, t := range tools {
		r[t.Name()] = t
	}
	return r
}
