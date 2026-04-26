package argv

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"
	"testing"
)

func newTestMux() *Mux {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error { return nil },
	}
	cmd.Option("repository", "r", "", "repo path")
	cmd.Flag("force", "f", false, "force init")
	mux.Handle("init", "Initialize a repository", cmd)
	mux.Handle("ls", "List entries", RunnerFunc(func(out *Output, call *Call) error { return nil }))
	mux.Handle("version", "Print version", RunnerFunc(func(out *Output, call *Call) error { return nil }))
	sub := &Mux{}
	sub.Handle("init", "Initialize a new repo", RunnerFunc(func(out *Output, call *Call) error { return nil }))
	sub.Handle("clone", "Clone a repo", RunnerFunc(func(out *Output, call *Call) error { return nil }))
	mux.Handle("repo", "Repository operations", sub)
	return mux
}

func addInheritedFlags(mux *Mux) {
	mux.Flag("verbose", "v", false, "verbose output")
	mux.Option("config", "c", "", "config file")
}

func runComplete(t *testing.T, mux *Mux, tokens ...string) []string {
	t.Helper()
	var buf bytes.Buffer
	var completed []string
	partial := ""
	if len(tokens) > 0 {
		completed = tokens[:len(tokens)-1]
		partial = tokens[len(tokens)-1]
	}
	tw := &TokenWriter{Writer: &buf}
	walkComplete(mux, tw, completed, partial)
	out := buf.String()
	if out == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(out, "\n"), "\n")
}

func completionValues(lines []string) []string {
	vals := make([]string, len(lines))
	for i, line := range lines {
		vals[i], _, _ = strings.Cut(line, "\t")
	}
	return vals
}

func assertContains(t *testing.T, vals []string, want string) {
	t.Helper()
	if !slices.Contains(vals, want) {
		t.Fatalf("missing %q in %v", want, vals)
	}
}

// --- Subcommand completion ---

func TestCompleteTopLevelSubcommands(t *testing.T) {
	mux := newTestMux()
	mux.Handle("complete", "Output completions", CompletionCommand(mux))
	lines := runComplete(t, mux, "")
	vals := completionValues(lines)
	for _, want := range []string{"init", "ls", "repo", "version"} {
		assertContains(t, vals, want)
	}
	if slices.Contains(vals, "complete") {
		t.Fatalf("complete should be hidden from candidates: %v", vals)
	}
}

func TestCompletePartialSubcommand(t *testing.T) {
	mux := newTestMux()
	lines := runComplete(t, mux, "i")
	vals := completionValues(lines)
	if len(vals) != 1 || vals[0] != "init" {
		t.Fatalf("got %v, want [init]", vals)
	}
}

func TestCompleteMountedMuxSubcommands(t *testing.T) {
	mux := newTestMux()
	lines := runComplete(t, mux, "repo", "")
	vals := completionValues(lines)
	if len(vals) != 2 || vals[0] != "clone" || vals[1] != "init" {
		t.Fatalf("got %v, want [clone init]", vals)
	}
}

func TestCompleteOmitsHiddenSubcommands(t *testing.T) {
	mux := &Mux{}
	mux.Handle("ls", "List entries", RunnerFunc(func(out *Output, call *Call) error { return nil }))
	mux.Handle("internal", "Internal tools", &Command{
		Hidden: true,
		Run:    func(out *Output, call *Call) error { return nil },
	})

	lines := runComplete(t, mux, "")
	vals := completionValues(lines)
	if !slices.Contains(vals, "ls") {
		t.Fatalf("expected ls in candidates, got %v", vals)
	}
	if slices.Contains(vals, "internal") {
		t.Fatalf("hidden command leaked into completion: %v", vals)
	}
}

// --- Flag completion ---

func TestCompleteFlags(t *testing.T) {
	mux := newTestMux()
	lines := runComplete(t, mux, "init", "--")
	vals := completionValues(lines)
	for _, want := range []string{"--force", "--repository", "--help"} {
		assertContains(t, vals, want)
	}
}

func TestCompletePartialFlag(t *testing.T) {
	mux := newTestMux()
	lines := runComplete(t, mux, "init", "--f")
	vals := completionValues(lines)
	if len(vals) != 1 || vals[0] != "--force" {
		t.Fatalf("got %v, want [--force]", vals)
	}
}

func TestCompleteShortFlags(t *testing.T) {
	mux := newTestMux()
	lines := runComplete(t, mux, "init", "-")
	vals := completionValues(lines)
	for _, want := range []string{"-f", "-r", "-h"} {
		assertContains(t, vals, want)
	}
}

func TestCompleteFlagDescriptions(t *testing.T) {
	mux := newTestMux()
	lines := runComplete(t, mux, "init", "--f")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	if !strings.Contains(lines[0], "\t") || !strings.Contains(lines[0], "force init") {
		t.Fatalf("bad description: %q", lines[0])
	}
}

func TestCompleteHelpAlwaysPresent(t *testing.T) {
	mux := newTestMux()
	lines := runComplete(t, mux, "init", "--h")
	vals := completionValues(lines)
	if len(vals) != 1 || vals[0] != "--help" {
		t.Fatalf("got %v, want [--help]", vals)
	}
}

// --- Inherited flags ---

func TestCompleteInheritedFlagsAtMuxLevel(t *testing.T) {
	mux := newTestMux()
	addInheritedFlags(mux)
	// Mux flags are offered at the mux level, not at the command level.
	lines := runComplete(t, mux, "--v")
	vals := completionValues(lines)
	if len(vals) != 1 || vals[0] != "--verbose" {
		t.Fatalf("got %v, want [--verbose]", vals)
	}
	// At command level, mux flags are NOT offered.
	lines = runComplete(t, mux, "init", "--v")
	if len(lines) != 0 {
		t.Fatalf("mux flags should not appear at command level, got %v", completionValues(lines))
	}
}

func TestCompleteInheritedFlagsAtRoot(t *testing.T) {
	mux := newTestMux()
	addInheritedFlags(mux)
	lines := runComplete(t, mux, "--")
	vals := completionValues(lines)
	for _, want := range []string{"--verbose", "--config", "--help"} {
		assertContains(t, vals, want)
	}
}

// --- Option value suppression ---

func TestCompleteOptionValueSuppression(t *testing.T) {
	mux := newTestMux()
	lines := runComplete(t, mux, "init", "--repository", "")
	if len(lines) != 0 {
		t.Fatalf("expected no completions after value-taking option, got %v", lines)
	}
}

func TestCompleteOptionValueSuppressionShort(t *testing.T) {
	mux := newTestMux()
	lines := runComplete(t, mux, "init", "-r", "")
	if len(lines) != 0 {
		t.Fatalf("expected no completions after short value-taking option, got %v", lines)
	}
}

func TestCompleteGlobalOptionValueSuppression(t *testing.T) {
	mux := newTestMux()
	addInheritedFlags(mux)
	lines := runComplete(t, mux, "--config", "")
	if len(lines) != 0 {
		t.Fatalf("expected no completions after global value-taking option, got %v", lines)
	}
}

// --- Double dash ---

func TestCompleteAfterDoubleDash(t *testing.T) {
	mux := newTestMux()
	lines := runComplete(t, mux, "init", "--", "any")
	if len(lines) != 0 {
		t.Fatalf("expected no completions after --, got %v", lines)
	}
}

// --- End to end via Mux ---

func TestCompleteEndToEndViaMux(t *testing.T) {
	mux := newTestMux()
	mux.Handle("complete", "Output completions", CompletionCommand(mux))
	var out bytes.Buffer
	call := NewCall(context.Background(), []string{"complete", "--", "init", "--f"})
	if err := mux.RunArgv(&Output{Stdout: &out, Stderr: io.Discard}, call); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "--force") {
		t.Fatalf("expected --force in output:\n%s", out.String())
	}
}

func TestCompleteNoArgs(t *testing.T) {
	mux := newTestMux()
	lines := runComplete(t, mux)
	vals := completionValues(lines)
	for _, want := range []string{"init", "ls", "repo", "version"} {
		assertContains(t, vals, want)
	}
}

// --- Negated flag completion ---

func TestCompleteNegatedFlags(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		NegateFlags: true,
		Run:         func(out *Output, call *Call) error { return nil },
	}
	cmd.Flag("verbose", "v", false, "verbose output")
	cmd.Flag("no-cache", "", true, "disable cache")
	mux.Handle("build", "Build", cmd)

	t.Run("--no- prefix completes negated form", func(t *testing.T) {
		lines := runComplete(t, mux, "build", "--no-")
		vals := completionValues(lines)
		assertContains(t, vals, "--no-verbose")
		assertContains(t, vals, "--no-cache")
	})

	t.Run("bare form of no- flag completes", func(t *testing.T) {
		lines := runComplete(t, mux, "build", "--ca")
		vals := completionValues(lines)
		assertContains(t, vals, "--cache")
	})

	t.Run("all forms present in full listing", func(t *testing.T) {
		lines := runComplete(t, mux, "build", "--")
		vals := completionValues(lines)
		assertContains(t, vals, "--verbose")
		assertContains(t, vals, "--no-verbose")
		assertContains(t, vals, "--no-cache")
		assertContains(t, vals, "--cache")
		assertContains(t, vals, "--help")
	})
}

func TestCompleteNegatedFlagsMux(t *testing.T) {
	mux := &Mux{}
	mux.NegateFlags = true
	mux.Flag("verbose", "v", false, "verbose output")
	mux.Handle("run", "Run", RunnerFunc(func(out *Output, call *Call) error { return nil }))

	lines := runComplete(t, mux, "--no")
	vals := completionValues(lines)
	assertContains(t, vals, "--no-verbose")
}

// --- Mounted mux scoped flags ---

func TestCompleteMountedMuxScopedFlags(t *testing.T) {
	root := &Mux{}
	root.Flag("verbose", "v", false, "verbose output")
	sub := &Mux{}
	sub.Option("repository", "r", "", "repo path")
	sub.Handle("init", "Initialize", RunnerFunc(func(out *Output, call *Call) error { return nil }))
	root.Handle("repo", "Repository operations", sub)

	// At "repo" level, should see sub-mux flags.
	lines := runComplete(t, root, "repo", "--")
	vals := completionValues(lines)
	assertContains(t, vals, "--repository")
	assertContains(t, vals, "--help")
	// Should NOT see root-level --verbose at this position.
	for _, v := range vals {
		if v == "--verbose" {
			t.Fatal("should not see root --verbose at sub-mux level")
		}
	}

	// At root level, should see root-level flags.
	lines = runComplete(t, root, "--")
	vals = completionValues(lines)
	assertContains(t, vals, "--verbose")
	assertContains(t, vals, "--help")
}

// --- Argument hints ---

func TestCompleteArgHint(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error { return nil },
	}
	cmd.Arg("image", "Image reference")
	cmd.Arg("tag", "Image tag")
	mux.Handle("pull", "Pull an image", cmd)

	// No positional args consumed yet; hint first arg.
	lines := runComplete(t, mux, "pull", "")
	vals := completionValues(lines)
	assertContains(t, vals, "<image>")

	// One positional consumed; hint second arg.
	lines = runComplete(t, mux, "pull", "alpine", "")
	vals = completionValues(lines)
	assertContains(t, vals, "<tag>")

	// Both consumed; no hint.
	lines = runComplete(t, mux, "pull", "alpine", "latest", "")
	if len(lines) != 0 {
		t.Fatalf("expected no completions, got %v", completionValues(lines))
	}
}

func TestCompleteArgHintSkipsFlags(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error { return nil },
	}
	cmd.Flag("verbose", "v", false, "verbose")
	cmd.Option("output", "o", "", "output path")
	cmd.Arg("file", "File to process")
	mux.Handle("run", "Run something", cmd)

	// Flags and option values don't count as positional args.
	lines := runComplete(t, mux, "run", "--verbose", "--output", "/tmp/out", "")
	vals := completionValues(lines)
	assertContains(t, vals, "<file>")
}

func TestCompleteNoArgHintWhenTypingFlag(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error { return nil },
	}
	cmd.Flag("verbose", "", false, "verbose")
	cmd.Arg("name", "Name")
	mux.Handle("greet", "Greet", cmd)

	// Typing a flag; should get flag completions, not arg hint.
	lines := runComplete(t, mux, "greet", "--")
	vals := completionValues(lines)
	assertContains(t, vals, "--verbose")
	for _, v := range vals {
		if v == "<name>" {
			t.Fatal("should not see arg hint when typing a flag")
		}
	}
}

// --- Equals-form option value ---

func TestCompleteEqualsFormSuppression(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error { return nil },
	}
	cmd.Option("output", "", "", "output path")
	cmd.Flag("force", "", false, "force")
	mux.Handle("build", "Build", cmd)

	// --output= should suppress completions (awaiting value).
	lines := runComplete(t, mux, "build", "--output=")
	if len(lines) != 0 {
		t.Fatalf("expected no completions for --output=, got %v", completionValues(lines))
	}
}

func TestCompleteGlobalEqualsFormSuppression(t *testing.T) {
	mux := &Mux{}
	addInheritedFlags(mux)
	mux.Handle("run", "Run", RunnerFunc(func(out *Output, call *Call) error { return nil }))

	// --config= should suppress completions.
	lines := runComplete(t, mux, "--config=")
	if len(lines) != 0 {
		t.Fatalf("expected no completions for --config=, got %v", completionValues(lines))
	}
}

// --- Custom Completer via embedding Command ---

// hostCompleter embeds Command and implements Completer to provide
// dynamic host-value suggestions at --host value position.
type hostCompleter struct {
	*Command
}

func (h *hostCompleter) CompleteArgv(w *TokenWriter, completed []string, partial string) error {
	// Space-separated: "--host <TAB>" or "-H <TAB>".
	if n := len(completed); n > 0 {
		switch completed[n-1] {
		case "--host", "-H":
			return h.emitHosts(w, partial)
		}
	}
	// Equals-separated: "--host=value<TAB>" or "-H=value<TAB>".
	for _, prefix := range []string{"--host=", "-H="} {
		if strings.HasPrefix(partial, prefix) {
			return h.emitHosts(w, partial[len(prefix):])
		}
	}
	// Otherwise: fall back to default Help-driven completion.
	var help Help
	h.HelpArgv(&help)
	return help.CompleteArgv(w, completed, partial)
}

func (h *hostCompleter) emitHosts(w *TokenWriter, partial string) error {
	for _, host := range []string{"alpha", "beta", "gamma"} {
		if strings.HasPrefix(host, partial) {
			if _, err := w.WriteToken(host, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestCompleteDelegatesToCustomCompleterAtValuePosition(t *testing.T) {
	cmd := &Command{
		Run: func(out *Output, call *Call) error { return nil },
	}
	cmd.Option("host", "H", "", "host to connect to")

	mux := &Mux{}
	mux.Handle("run", "", &hostCompleter{Command: cmd})

	t.Run("space-separated long", func(t *testing.T) {
		lines := runComplete(t, mux, "run", "--host", "")
		vals := completionValues(lines)
		if len(vals) != 3 || vals[0] != "alpha" || vals[1] != "beta" || vals[2] != "gamma" {
			t.Fatalf("got %v", vals)
		}
	})

	t.Run("space-separated long with prefix", func(t *testing.T) {
		lines := runComplete(t, mux, "run", "--host", "g")
		vals := completionValues(lines)
		if len(vals) != 1 || vals[0] != "gamma" {
			t.Fatalf("got %v", vals)
		}
	})

	t.Run("space-separated short", func(t *testing.T) {
		lines := runComplete(t, mux, "run", "-H", "")
		vals := completionValues(lines)
		if len(vals) != 3 || vals[0] != "alpha" || vals[1] != "beta" || vals[2] != "gamma" {
			t.Fatalf("got %v", vals)
		}
	})

	t.Run("equals-separated", func(t *testing.T) {
		lines := runComplete(t, mux, "run", "--host=be")
		vals := completionValues(lines)
		if len(vals) != 1 || vals[0] != "beta" {
			t.Fatalf("got %v", vals)
		}
	})

	t.Run("equals-separated empty value", func(t *testing.T) {
		lines := runComplete(t, mux, "run", "--host=")
		vals := completionValues(lines)
		if len(vals) != 3 {
			t.Fatalf("got %v", vals)
		}
	})
}

func TestCompleteValuePositionNoCompleter(t *testing.T) {
	cmd := &Command{
		Run: func(out *Output, call *Call) error { return nil },
	}
	cmd.Option("host", "H", "", "host")

	mux := &Mux{}
	mux.Handle("run", "", cmd)

	lines := runComplete(t, mux, "run", "--host", "")
	if len(lines) != 0 {
		t.Fatalf("expected no completions without Completer, got %v", lines)
	}
	lines = runComplete(t, mux, "run", "--host=x")
	if len(lines) != 0 {
		t.Fatalf("expected no completions without Completer, got %v", lines)
	}
}

