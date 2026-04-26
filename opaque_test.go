package argv_test

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"slices"
	"strings"
	"testing"

	"mz.attahri.com/code/argv"
	"mz.attahri.com/code/argv/argvtest"
)

// opaqueRepo composes a *argv.Mux but exposes only RunArgv. The parent
// mux that mounts it cannot see its subtree, flags, or descriptions.
// This mirrors http.Handler wrapping an http.ServeMux opaquely.
type opaqueRepo struct {
	inner *argv.Mux
}

func newOpaqueRepo() *opaqueRepo {
	m := &argv.Mux{Description: "Internal repo operations"}

	initCmd := &argv.Command{
		Description: "Initialize a repository",
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := out.Stdout.Write([]byte(
				"init pattern=" + call.Pattern() +
					" verbose=" + boolStr(call.Flags.Get("verbose")) +
					" bare=" + boolStr(call.Flags.Get("bare")) +
					"\n"))
			return err
		},
	}
	initCmd.Flag("bare", "b", false, "Create a bare repository")
	m.Handle("init", "Initialize a repo", initCmd)

	status := &argv.Command{
		Description: "Show repo status",
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := out.Stdout.Write([]byte("status pattern=" + call.Pattern() + "\n"))
			return err
		},
	}
	m.Handle("status", "Show status", status)

	return &opaqueRepo{inner: m}
}

// RunArgv is the ONE interface opaqueRepo implements. It is a pure Runner.
func (r *opaqueRepo) RunArgv(out *argv.Output, call *argv.Call) error {
	return r.inner.RunArgv(out, call)
}

// Compile-time: opaqueRepo is exactly a Runner, nothing else.
var _ argv.Runner = (*opaqueRepo)(nil)

// transparentRepo embeds opaqueRepo and adds Walker by delegating to
// the inner mux. One method is the entire cost of opting into
// introspection.
type transparentRepo struct{ *opaqueRepo }

func (r *transparentRepo) WalkArgv(path string, base *argv.Help) iter.Seq2[*argv.Help, argv.Runner] {
	return r.inner.WalkArgv(path, base)
}

var (
	_ argv.Runner = (*transparentRepo)(nil)
	_ argv.Walker = (*transparentRepo)(nil)
)

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// newOuter builds an outer mux with a global --verbose flag and the
// opaque repo mounted at "repo". A completion runner is mounted at
// "complete" so we can exercise walk-based completion.
func newOuter() (*argv.Mux, *opaqueRepo) {
	repo := newOpaqueRepo()
	outer := &argv.Mux{Description: "Outer app"}
	outer.Flag("verbose", "v", false, "Verbose output (global)")
	outer.Handle("repo", "Repository operations (opaque)", repo)
	outer.Handle("complete", "Emit completions", argv.CompletionCommand(outer))
	return outer, repo
}

// newOuterTransparent builds an identical outer mux but mounts a
// transparentRepo (Runner + Walker) instead of an opaqueRepo.
func newOuterTransparent() *argv.Mux {
	repo := &transparentRepo{opaqueRepo: newOpaqueRepo()}
	outer := &argv.Mux{Description: "Outer app"}
	outer.Flag("verbose", "v", false, "Verbose output (global)")
	outer.Handle("repo", "Repository operations (transparent)", repo)
	outer.Handle("complete", "Emit completions", argv.CompletionCommand(outer))
	return outer
}

// --- Dispatch tests ---------------------------------------------------------

func TestOpaque_DispatchReachesInnerLeaf(t *testing.T) {
	outer, _ := newOuter()
	rec := argvtest.NewRecorder()
	if err := outer.RunArgv(rec.Output(), argvtest.NewCall("repo init")); err != nil {
		t.Fatalf("RunArgv: %v", err)
	}
	got := rec.Stdout()
	if !strings.Contains(got, `pattern=repo init`) {
		t.Errorf("inner leaf did not receive full path; got %q", got)
	}
}

func TestOpaque_GlobalFlagCascadesThroughOpaque(t *testing.T) {
	outer, _ := newOuter()
	rec := argvtest.NewRecorder()
	if err := outer.RunArgv(rec.Output(), argvtest.NewCall("--verbose repo init")); err != nil {
		t.Fatalf("RunArgv: %v", err)
	}
	got := rec.Stdout()
	if !strings.Contains(got, "verbose=true") {
		t.Errorf("outer global --verbose did not cascade through opaque; got %q", got)
	}
}

func TestOpaque_InnerCommandFlagIsLocal(t *testing.T) {
	outer, _ := newOuter()
	rec := argvtest.NewRecorder()
	if err := outer.RunArgv(rec.Output(), argvtest.NewCall("repo init --bare")); err != nil {
		t.Fatalf("RunArgv: %v", err)
	}
	got := rec.Stdout()
	if !strings.Contains(got, "bare=true") {
		t.Errorf("inner --bare not parsed; got %q", got)
	}
}

func TestOpaque_UnknownSubcommandBecomesHelpError(t *testing.T) {
	outer, _ := newOuter()
	rec := argvtest.NewRecorder()
	err := outer.RunArgv(rec.Output(), argvtest.NewCall("repo frobnicate"))
	if err == nil {
		t.Fatalf("expected error for unknown inner subcommand")
	}
	var he *argv.HelpError
	if !errors.As(err, &he) {
		t.Fatalf("expected *HelpError, got %T: %v", err, err)
	}
	if he.Path != "repo" {
		t.Errorf("HelpError.Path = %q, want %q", he.Path, "repo")
	}
	if he.Reason == "" {
		t.Errorf("expected a reason string on implicit HelpError")
	}
	t.Logf("HelpError: path=%q reason=%q explicit=%v", he.Path, he.Reason, he.Explicit)
}

// --- Help tests: the interesting ones ---------------------------------------

func TestOpaque_ExplicitHelpAtOpaqueBoundary(t *testing.T) {
	outer, _ := newOuter()
	var stdout, stderr bytes.Buffer
	p := &argv.Program{Stdout: &stdout, Stderr: &stderr, Usage: "test app"}
	err := p.Invoke(context.Background(), outer, []string{"app", "repo", "--help"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	t.Logf("--- stdout ---\n%s", stdout.String())
	t.Logf("--- stderr ---\n%s", stderr.String())
}

func TestOpaque_ExplicitHelpPastOpaqueBoundary(t *testing.T) {
	outer, _ := newOuter()
	var stdout, stderr bytes.Buffer
	p := &argv.Program{Stdout: &stdout, Stderr: &stderr, Usage: "test app"}
	err := p.Invoke(context.Background(), outer, []string{"app", "repo", "init", "--help"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	out := stdout.String()
	t.Logf("--- stdout ---\n%s", out)
	t.Logf("--- stderr ---\n%s", stderr.String())

	// Path is past the opaque boundary; the render should be minimal,
	// not painted with outer-mux metadata.
	if !strings.Contains(out, "app repo init") {
		t.Errorf("minimal help should mention the requested path: %s", out)
	}
	if strings.Contains(out, "complete  Emit completions") ||
		strings.Contains(out, "repo      Repository operations") {
		t.Errorf("opaque-boundary help leaked outer-mux commands: %s", out)
	}
	if strings.Contains(out, "Outer app") {
		t.Errorf("opaque-boundary help leaked outer-mux description: %s", out)
	}
}

func TestOpaque_OuterHelpListsOpaqueButNotItsSubtree(t *testing.T) {
	outer, _ := newOuter()
	var stdout, stderr bytes.Buffer
	p := &argv.Program{Stdout: &stdout, Stderr: &stderr, Usage: "test app"}
	err := p.Invoke(context.Background(), outer, []string{"app", "--help"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	out := stdout.String()
	t.Logf("--- stdout ---\n%s", out)
	if !strings.Contains(out, "repo") {
		t.Errorf("outer help should list 'repo', got %q", out)
	}
	if strings.Contains(out, "init") || strings.Contains(out, "status") {
		t.Errorf("outer help leaked inner subtree into outer listing: %q", out)
	}
}

// --- Completion tests -------------------------------------------------------

func TestOpaque_CompletionAtTopLevelListsOpaque(t *testing.T) {
	outer, _ := newOuter()
	rec := argvtest.NewRecorder()
	// User typed: `app complete -- <TAB>` → complete with empty partial.
	call := argv.NewCall(context.Background(), []string{"complete", "--", ""})
	if err := outer.RunArgv(rec.Output(), call); err != nil {
		t.Fatalf("RunArgv: %v", err)
	}
	got := rec.Stdout()
	t.Logf("--- completions ---\n%s", got)
	if !strings.Contains(got, "repo") {
		t.Errorf("top-level completion missing 'repo': %q", got)
	}
}

func TestOpaque_CompletionPastOpaqueBoundaryEmpty(t *testing.T) {
	outer, _ := newOuter()
	rec := argvtest.NewRecorder()
	// User typed: `app complete -- repo <TAB>`
	call := argv.NewCall(context.Background(), []string{"complete", "--", "repo", ""})
	if err := outer.RunArgv(rec.Output(), call); err != nil {
		t.Fatalf("RunArgv: %v", err)
	}
	got := rec.Stdout()
	t.Logf("--- completions after opaque boundary ---\n%q", got)
	// Hypothesis: without Walker on opaqueRepo, nothing is offered here.
	if strings.Contains(got, "init") || strings.Contains(got, "status") {
		t.Errorf("completion leaked past opaque boundary (shouldn't without Walker): %q", got)
	}
}

// --- Transparent (Runner + Walker) variant ---------------------------------
//
// Runner-only: opaque sub-handler (dispatch works, introspection doesn't).
// Runner + Walker: mountable mux (full introspection).

func TestTransparent_OuterHelpStillListsNode(t *testing.T) {
	outer := newOuterTransparent()
	var stdout bytes.Buffer
	p := &argv.Program{Stdout: &stdout, Stderr: &bytes.Buffer{}, Usage: "test app"}
	if err := p.Invoke(context.Background(), outer, []string{"app", "--help"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got := stdout.String()
	t.Logf("--- stdout ---\n%s", got)
	if !strings.Contains(got, "repo") {
		t.Errorf("outer help missing 'repo': %q", got)
	}
}

func TestTransparent_BoundaryHelpListsInnerSubcommands(t *testing.T) {
	outer := newOuterTransparent()
	var stdout bytes.Buffer
	p := &argv.Program{Stdout: &stdout, Stderr: &bytes.Buffer{}, Usage: "test app"}
	if err := p.Invoke(context.Background(), outer, []string{"app", "repo", "--help"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got := stdout.String()
	t.Logf("--- stdout ---\n%s", got)
	for _, want := range []string{"init", "status"} {
		if !strings.Contains(got, want) {
			t.Errorf("repo --help should list %q, got %q", want, got)
		}
	}
}

func TestTransparent_DeepHelpRendersCorrectly(t *testing.T) {
	outer := newOuterTransparent()
	var stdout bytes.Buffer
	p := &argv.Program{Stdout: &stdout, Stderr: &bytes.Buffer{}, Usage: "test app"}
	if err := p.Invoke(context.Background(), outer, []string{"app", "repo", "init", "--help"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got := stdout.String()
	t.Logf("--- stdout ---\n%s", got)
	if !strings.Contains(got, "app repo init") {
		t.Errorf("deep help missing correct path: %q", got)
	}
	if !strings.Contains(got, "Initialize a repository") {
		t.Errorf("deep help missing inner command description: %q", got)
	}
	if !strings.Contains(got, "--bare") {
		t.Errorf("deep help missing inner --bare flag: %q", got)
	}
	// Negative: the outer mux's subcommand listing should NOT appear here.
	if strings.Contains(got, "complete  Emit completions") {
		t.Errorf("deep help leaked outer mux commands list: %q", got)
	}
}

func TestTransparent_CompletionDescendsIntoInnerSubtree(t *testing.T) {
	outer := newOuterTransparent()
	rec := argvtest.NewRecorder()
	// `app complete -- repo <TAB>`
	call := argv.NewCall(context.Background(), []string{"complete", "--", "repo", ""})
	if err := outer.RunArgv(rec.Output(), call); err != nil {
		t.Fatalf("RunArgv: %v", err)
	}
	got := rec.Stdout()
	t.Logf("--- completions past transparent boundary ---\n%s", got)
	for _, want := range []string{"init", "status"} {
		if !strings.Contains(got, want) {
			t.Errorf("completion past transparent boundary missing %q: %q", want, got)
		}
	}
}

func TestTransparent_ProgramWalkEnumeratesFullTree(t *testing.T) {
	outer := newOuterTransparent()
	p := &argv.Program{Usage: "test app"}
	var paths []string
	for h := range p.Walk("app", outer) {
		paths = append(paths, h.FullPath)
	}
	t.Logf("walk paths: %v", paths)
	for _, want := range []string{"app", "app repo", "app repo init", "app repo status", "app complete"} {
		if !slices.Contains(paths, want) {
			t.Errorf("Walk missing %q; got %v", want, paths)
		}
	}
}

