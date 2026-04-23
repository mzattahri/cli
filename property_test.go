package argv

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"
)

// TestPropertyMatchMatchesRunCLI asserts that the runner returned by
// Mux.Match for a token sequence is the same runner that Mux.RunCLI
// would dispatch to. Match is meant to be a read-only preview of
// dispatch; any divergence is a correctness bug.
func TestPropertyMatchMatchesRunCLI(t *testing.T) {
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

	mux := NewMux("app")
	dispatched := map[string]string{}
	for _, l := range leaves {
		name := l.name
		mux.HandleFunc(joinSegments(l.tokens), "", func(_ *Output, call *Call) error {
			dispatched[call.Pattern] = name
			return nil
		})
	}

	for _, l := range leaves {
		runner, path := mux.Match(l.tokens)
		if runner == nil {
			t.Fatalf("Match(%v): got nil", l.tokens)
		}

		call := NewCall(context.Background(), l.tokens)
		if err := mux.RunCLI(&Output{Stdout: io.Discard, Stderr: io.Discard}, call); err != nil {
			t.Fatalf("RunCLI(%v): %v", l.tokens, err)
		}
		if dispatched[path] != l.name {
			t.Fatalf("Match(%v)=%q but RunCLI reached %q", l.tokens, path, dispatched[path])
		}
	}
}

// TestPropertyWalkCoversAllRunners asserts that Program.Walk yields an
// entry for every registered runner in a Mux tree (plus intermediate
// nodes). No runner should be silently invisible to introspection.
func TestPropertyWalkCoversAllRunners(t *testing.T) {
	root := NewMux("app")
	root.Flag("verbose", "v", false, "verbose")

	registered := []string{"a", "b", "c d", "c e"}
	for _, pat := range registered {
		root.HandleFunc(pat, "", func(*Output, *Call) error { return nil })
	}

	sub := NewMux("sub")
	sub.HandleFunc("x", "", func(*Output, *Call) error { return nil })
	sub.HandleFunc("y", "", func(*Output, *Call) error { return nil })
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
	for path := range (&Program{}).Walk(root) {
		got = append(got, path)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestPropertyEnrichPreservesIdentity asserts that enrichCall preserves
// user-set Flags, Options, and Args across routing levels — a parent's
// values should not be lost when descending into a child.
func TestPropertyEnrichPreservesIdentity(t *testing.T) {
	root := NewMux("app")
	root.Option("tag", "t", "default-tag", "tag")

	sub := NewMux("sub")
	var got string
	sub.HandleFunc("show", "", func(out *Output, call *Call) error {
		got = call.Options.Get("tag")
		return nil
	})
	root.Handle("nested", "", sub)

	call := NewCall(context.Background(), []string{"--tag", "mine", "nested", "show"})
	if err := root.RunCLI(&Output{Stdout: io.Discard, Stderr: io.Discard}, call); err != nil {
		t.Fatal(err)
	}
	if got != "mine" {
		t.Fatalf("got %q, want %q", got, "mine")
	}
}

func joinSegments(tokens []string) string {
	return strings.Join(tokens, " ")
}

// Compile-time assertions that the four core interfaces are
// implemented by the expected types.
var (
	_ Runner    = (*Mux)(nil)
	_ Runner    = (*Command)(nil)
	_ Helper    = (*Mux)(nil)
	_ Helper    = (*Command)(nil)
	_ Walker    = (*Mux)(nil)
	_ Walker    = (*Command)(nil)
	_ Completer = (*Mux)(nil)
	_ Completer = (*Command)(nil)
)
