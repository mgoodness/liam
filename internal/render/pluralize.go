package render

// Pluralize returns singular when n == 1, plural otherwise. Shared here
// (rather than duplicated per package) since more than one caller needs
// count-driven noun forms that aren't a plain "+s" — e.g. "skill"/"skills"
// for /skills' header, "match"/"matches" for tool's search-result headers.
func Pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
