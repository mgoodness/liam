// Command liam is a minimal Go coding-agent harness.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/mgoodness/liam/internal/provider"
	"github.com/mgoodness/liam/internal/provider/openrouter"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("liam", flag.ContinueOnError)
	fs.SetOutput(stderr)
	prompt := fs.String("p", "", "send a single prompt headlessly and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *prompt == "" {
		fmt.Fprintln(stderr, "liam: interactive mode is not implemented yet; pass -p \"<prompt>\" for headless mode")
		return 2
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(stderr, "liam: OPENROUTER_API_KEY is not set")
		return 1
	}

	p := openrouter.New(apiKey)
	req := provider.Request{
		Messages: []provider.Message{{Role: "user", Content: *prompt}},
	}

	for ev, err := range p.Stream(context.Background(), req) {
		if err != nil {
			fmt.Fprintf(stderr, "liam: %v\n", err)
			return 1
		}
		switch e := ev.(type) {
		case provider.TextDeltaEvent:
			fmt.Fprint(stdout, e.Text)
		case provider.DoneEvent:
			fmt.Fprintln(stdout)
			fmt.Fprintf(stderr, "liam: model=%s\n", e.ModelUsed)
		}
	}

	return 0
}
