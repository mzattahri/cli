package argv

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"
)

// TestPropertyMatchMatchesRunArgv verifies Mux.Match returns the same
// runner that Mux.RunArgv dispatches to.
func TestPropertyMatchMatchesRunArgv(t *testing.T) {
	type leaf struct {
		name   string
		tokens []string
	}
	leaves := []leaf{
		{"a", []string{"a"}},
		{"a-b", []string{"a", "b"}},
		{"c", []string{"c"}},
		{"d-e-f", []string{"d", "e", "f"}},
	}

	mux := &Mux{}
	dispatched := map[string]string{}
	for _, l := range leaves {
		name := l.name
		mux.Handle(joinSegments(l.tokens), "", RunnerFunc(func(_ *Output, call *Call) error {
			dispatched[call.Pattern()] = name
			return nil
		}))
	}

	for _, l := range leaves {
		runner, path := mux.Match(l.tokens)
		if runner == nil {
			t.Fatalf("Match(%v): got nil", l.tokens)
		}

		call := NewCall(context.Background(), l.tokens)
		if err := mux.RunArgv(&Output{Stdout: io.Discard, Stderr: io.Discard}, call); err != nil {
			t.Fatalf("RunArgv(%v): %v", l.tokens, err)
		}
		if dispatched[path] != l.name {
			t.Fatalf("Match(%v)=%q but RunArgv reached %q", l.tokens, path, dispatched[path])
		}
	}
}

// TestPropertyWalkCoversAllRunners verifies Program.Walk yields every
// registered runner plus intermediate nodes.
func TestPropertyWalkCoversAllRunners(t *testing.T) {
	root := &Mux{}
	root.Flag("verbose", "v", false, "verbose")

	noop := RunnerFunc(func(*Output, *Call) error { return nil })
	registered := []string{"a", "b", "c d", "c e"}
	for _, pat := range registered {
		root.Handle(pat, "", noop)
	}

	sub := &Mux{}
	sub.Handle("x", "", noop)
	sub.Handle("y", "", noop)
	root.Handle("nested", "", sub)

	want := []string{
		"app",
		"app a",
		"app b",
		"app c",
		"app c d",
		"app c e",
		"app nested",
		"app nested x",
		"app nested y",
	}

	var got []string
	for help := range (&Program{}).Walk("app", root) {
		got = append(got, help.FullPath)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestPropertyEnrichPreservesIdentity verifies parent-level options
// remain visible when descending into a child mux.
func TestPropertyEnrichPreservesIdentity(t *testing.T) {
	root := &Mux{}
	root.Option("tag", "t", "default-tag", "tag")

	sub := &Mux{}
	var got string
	sub.Handle("show", "", RunnerFunc(func(out *Output, call *Call) error {
		got = call.Options.Get("tag")
		return nil
	}))
	root.Handle("nested", "", sub)

	call := NewCall(context.Background(), []string{"--tag", "mine", "nested", "show"})
	if err := root.RunArgv(&Output{Stdout: io.Discard, Stderr: io.Discard}, call); err != nil {
		t.Fatal(err)
	}
	if got != "mine" {
		t.Fatalf("got %q, want %q", got, "mine")
	}
}

func joinSegments(tokens []string) string {
	return strings.Join(tokens, " ")
}

// Compile-time assertions that the core interfaces are implemented
// by the expected built-in types. Completer is deliberately absent:
// it is the optional customization hook, not a baseline contract.
var (
	_ Runner = (*Mux)(nil)
	_ Runner = (*Command)(nil)
	_ Helper = (*Mux)(nil)
	_ Helper = (*Command)(nil)
	_ Walker = (*Mux)(nil)
	_ Walker = (*Command)(nil)
)
