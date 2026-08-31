package tool

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// StdlibSearch is find/grep's always-available fallback searcher
// (filepath.WalkDir + regexp/bufio.Scanner), used when fff-mcp isn't on
// $PATH (ticket #18's resolution). It's deliberately naive next to
// fff-mcp: no index, no frecency, a full tree walk per call.
type StdlibSearch struct {
	// Dir is the root to search under. Empty defaults to the process's
	// current working directory.
	Dir string
}

func (s StdlibSearch) root() string {
	if s.Dir != "" {
		return s.Dir
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// walk calls visit for every regular file under root, skipping .git
// directories entirely and silently skipping unreadable entries rather
// than aborting the whole search.
func walk(root string, visit func(path, rel string) error) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		return visit(path, rel)
	})
}

// Find implements FindSearcher: a case-insensitive substring match of
// query against each file's path relative to root, in walk order. An
// empty query matches every file — the "also covers directory listing"
// case from issue #41's spec.
func (s StdlibSearch) Find(_ context.Context, query string) ([]string, int, error) {
	query = strings.ToLower(query)

	var all []string
	err := walk(s.root(), func(_, rel string) error {
		if query == "" || strings.Contains(strings.ToLower(rel), query) {
			all = append(all, rel)
		}
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("find: %w", err)
	}

	total := len(all)
	if len(all) > MaxSearchResults {
		all = all[:MaxSearchResults]
	}
	return all, total, nil
}

// Grep implements GrepSearcher: query is a regular expression, matched
// line by line against every file's content under root.
func (s StdlibSearch) Grep(_ context.Context, query string) ([]GrepMatch, int, error) {
	re, err := regexp.Compile(query)
	if err != nil {
		return nil, 0, fmt.Errorf("grep: invalid regular expression: %w", err)
	}

	var all []GrepMatch
	walkErr := walk(s.root(), func(path, rel string) error {
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		line := 0
		for scanner.Scan() {
			line++
			text := scanner.Text()
			if re.MatchString(text) {
				all = append(all, GrepMatch{File: rel, Line: line, Text: text})
			}
		}
		// A scan error (binary content, an over-long line) just stops that
		// one file's scan early rather than failing the whole search.
		return nil
	})
	if walkErr != nil {
		return nil, 0, fmt.Errorf("grep: %w", walkErr)
	}

	total := len(all)
	if len(all) > MaxSearchResults {
		all = all[:MaxSearchResults]
	}
	return all, total, nil
}
