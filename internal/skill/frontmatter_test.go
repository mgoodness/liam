package skill

import (
	"reflect"
	"testing"
)

func TestParseFrontmatterBasic(t *testing.T) {
	data := []byte("---\nname: my-skill\ndescription: Does a thing, and: another thing.\n---\n# Body\n\nHello.\n")

	fields, body, err := parseFrontmatter(data)
	if err != nil {
		t.Fatalf("parseFrontmatter() error = %v", err)
	}

	want := map[string]string{
		"name":        "my-skill",
		"description": "Does a thing, and: another thing.",
	}
	if !reflect.DeepEqual(fields, want) {
		t.Errorf("fields = %#v, want %#v", fields, want)
	}
	if wantBody := "# Body\n\nHello."; body != wantBody {
		t.Errorf("body = %q, want %q", body, wantBody)
	}
}

func TestParseFrontmatterQuotedValues(t *testing.T) {
	data := []byte(`---
name: 'single-quoted'
description: "double-quoted description"
---
body
`)
	fields, _, err := parseFrontmatter(data)
	if err != nil {
		t.Fatalf("parseFrontmatter() error = %v", err)
	}
	if fields["name"] != "single-quoted" {
		t.Errorf(`fields["name"] = %q, want "single-quoted"`, fields["name"])
	}
	if fields["description"] != "double-quoted description" {
		t.Errorf(`fields["description"] = %q, want "double-quoted description"`, fields["description"])
	}
}

func TestParseFrontmatterSkipsNestedLines(t *testing.T) {
	data := []byte(`---
name: my-skill
description: A skill.
metadata:
  author: someone
  version: "1"
---
body
`)
	fields, _, err := parseFrontmatter(data)
	if err != nil {
		t.Fatalf("parseFrontmatter() error = %v", err)
	}
	if _, ok := fields["author"]; ok {
		t.Errorf("fields contains nested key \"author\", want it skipped")
	}
	if fields["name"] != "my-skill" {
		t.Errorf(`fields["name"] = %q, want "my-skill"`, fields["name"])
	}
}

func TestParseFrontmatterDisableModelInvocation(t *testing.T) {
	data := []byte("---\nname: s\ndescription: d\ndisable-model-invocation: true\n---\nbody\n")
	fields, _, err := parseFrontmatter(data)
	if err != nil {
		t.Fatalf("parseFrontmatter() error = %v", err)
	}
	if fields["disable-model-invocation"] != "true" {
		t.Errorf(`fields["disable-model-invocation"] = %q, want "true"`, fields["disable-model-invocation"])
	}
}

func TestParseFrontmatterMissingOpeningDelimiter(t *testing.T) {
	if _, _, err := parseFrontmatter([]byte("name: x\n---\nbody\n")); err == nil {
		t.Fatal("parseFrontmatter() error = nil, want an error for a missing opening delimiter")
	}
}

func TestParseFrontmatterMissingClosingDelimiter(t *testing.T) {
	if _, _, err := parseFrontmatter([]byte("---\nname: x\ndescription: d\n")); err == nil {
		t.Fatal("parseFrontmatter() error = nil, want an error for a missing closing delimiter")
	}
}

func TestParseFrontmatterEmptyFile(t *testing.T) {
	if _, _, err := parseFrontmatter([]byte("")); err == nil {
		t.Fatal("parseFrontmatter() error = nil, want an error for an empty file")
	}
}
