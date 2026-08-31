package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

// assertJSONEqual unmarshals got as JSON and compares it against want,
// avoiding brittle byte-for-byte comparisons of stripComments' whitespace
// preservation.
func assertJSONEqual(t *testing.T, got []byte, want any) {
	t.Helper()
	var parsed any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("stripComments output is not valid JSON: %v\noutput: %s", err, got)
	}
	if !reflect.DeepEqual(parsed, want) {
		t.Errorf("parsed = %#v, want %#v", parsed, want)
	}
}
