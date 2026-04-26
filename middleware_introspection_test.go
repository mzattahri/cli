package argv_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"mz.attahri.com/code/argv"
)

// TestMiddleware_RunnerFuncStripsMetadata documents the reason
// [argv.NewMiddleware] exists: a middleware that returns a bare
// RunnerFunc silently drops Helper/Walker/Completer from the wrapped
// Runner.
func TestMiddleware_RunnerFuncStripsMetadata(t *testing.T) {
	inner := buildInnerCommand()
	passthrough := func(next argv.Runner) argv.Runner {
		return argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
			return next.RunArgv(out, call)
		})
	}

	bareOut := renderCmdHelp(t, inner)
	wrappedOut := renderCmdHelp(t, passthrough(inner))

	t.Logf("--- bare ---\n%s", bareOut)
	t.Logf("--- wrapped (RunnerFunc) ---\n%s", wrappedOut)

	if strings.Contains(wrappedOut, "inner-flag") {
		t.Errorf("RunnerFunc middleware unexpectedly preserved inner-flag")
	}
	if strings.Contains(wrappedOut, "Inner description") {
		t.Errorf("RunnerFunc middleware unexpectedly preserved description")
	}
}

// TestMiddleware_PreservesMetadata proves Middleware-based middleware
// forwards Helper/Walker/Completer to the inner Runner. Output is
// byte-identical to invoking the inner Runner directly.
func TestMiddleware_PreservesMetadata(t *testing.T) {
	inner := buildInnerCommand()
	passthrough := argv.NewMiddleware(func(out *argv.Output, call *argv.Call, next argv.Runner) error {
		return next.RunArgv(out, call)
	})

	bareOut := renderCmdHelp(t, inner)
	wrappedOut := renderCmdHelp(t, passthrough(inner))

	t.Logf("--- bare ---\n%s", bareOut)
	t.Logf("--- wrapped (Middleware) ---\n%s", wrappedOut)

	if !strings.Contains(wrappedOut, "inner-flag") {
		t.Errorf("Middleware lost inner-flag: %s", wrappedOut)
	}
	if !strings.Contains(wrappedOut, "Inner description") {
		t.Errorf("Middleware lost description: %s", wrappedOut)
	}
	if bareOut != wrappedOut {
		t.Errorf("Middleware output differs from bare:\nbare:\n%s\nwrapped:\n%s", bareOut, wrappedOut)
	}
}

// TestMiddleware_ChainsPreserveMetadata verifies two layers of
// Middleware still surface inner metadata.
func TestMiddleware_ChainsPreserveMetadata(t *testing.T) {
	inner := buildInnerCommand()
	mw := argv.NewMiddleware(func(out *argv.Output, call *argv.Call, next argv.Runner) error {
		return next.RunArgv(out, call)
	})
	doubled := mw(mw(inner))

	out := renderCmdHelp(t, doubled)
	t.Logf("--- doubled ---\n%s", out)
	if !strings.Contains(out, "inner-flag") {
		t.Errorf("chained Middleware lost inner-flag: %s", out)
	}
}

func buildInnerCommand() *argv.Command {
	cmd := &argv.Command{
		Description: "Inner description",
		Run:         func(out *argv.Output, call *argv.Call) error { return nil },
	}
	cmd.Flag("inner-flag", "", false, "A flag declared on the inner command")
	return cmd
}

// TestMiddleware_FallbackWhenInnerIsBare covers the case where the
// wrapped Runner implements only Runner (no Helper/Walker/Completer).
// The wrapper produced by NewMiddleware must expose all four
// interfaces with sensible defaults.
func TestMiddleware_FallbackWhenInnerIsBare(t *testing.T) {
	bare := argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error { return nil })
	mw := argv.NewMiddleware(func(out *argv.Output, call *argv.Call, next argv.Runner) error {
		return next.RunArgv(out, call)
	})
	wrapped := mw(bare)

	// All three optional interfaces should be satisfied by the wrapper.
	var (
		_ argv.Helper    = wrapped.(argv.Helper)
		_ argv.Walker    = wrapped.(argv.Walker)
		_ argv.Completer = wrapped.(argv.Completer)
	)

	// HelpArgv: should be a no-op when inner isn't a Helper.
	var h argv.Help
	h.Description = "preserved"
	wrapped.(argv.Helper).HelpArgv(&h)
	if h.Description != "preserved" {
		t.Errorf("HelpArgv with non-Helper inner should not modify Help; got %q", h.Description)
	}

	// WalkArgv: should yield a single synthesized entry.
	var paths []string
	for entry := range wrapped.(argv.Walker).WalkArgv("app cmd", nil) {
		paths = append(paths, entry.FullPath)
	}
	if len(paths) != 1 || paths[0] != "app cmd" {
		t.Errorf("WalkArgv fallback should yield one entry 'app cmd'; got %v", paths)
	}

	// CompleteArgv: should return nil when inner isn't a Completer.
	tw := &argv.TokenWriter{Writer: &bytes.Buffer{}}
	if err := wrapped.(argv.Completer).CompleteArgv(tw, nil, ""); err != nil {
		t.Errorf("CompleteArgv fallback should not error; got %v", err)
	}
}

func TestMiddleware_PanicsOnNilAround(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil around")
		}
	}()
	argv.NewMiddleware(nil)
}

func TestMiddleware_PanicsOnNilRunner(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil runner")
		}
	}()
	mw := argv.NewMiddleware(func(out *argv.Output, call *argv.Call, next argv.Runner) error {
		return next.RunArgv(out, call)
	})
	mw(nil)
}

func renderCmdHelp(t *testing.T, mounted argv.Runner) string {
	t.Helper()
	m := &argv.Mux{}
	m.Handle("cmd", "Mounted command", mounted)
	var buf bytes.Buffer
	p := &argv.Program{Stdout: &buf, Stderr: &bytes.Buffer{}}
	if err := p.Invoke(context.Background(), m, []string{"app", "cmd", "--help"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	return buf.String()
}
