package argv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"slices"
)

// A Program binds a [Runner] to the process environment. The zero
// value is a valid Program that reads [os.Stdin] and writes to
// [os.Stdout] and [os.Stderr].
type Program struct {
	// Stdout is the standard output writer. A nil Stdout selects [os.Stdout].
	Stdout io.Writer

	// Stderr is the standard error writer. A nil Stderr selects [os.Stderr].
	Stderr io.Writer

	// Stdin is the standard input reader. A nil Stdin selects [os.Stdin].
	Stdin io.Reader

	// Summary is the short one-line description shown in top-level help output.
	Summary string

	// Description is the longer free-form help text.
	Description string

	// HelpFunc overrides the default help renderer.
	HelpFunc HelpFunc
}

func (p *Program) output() *Output {
	stdout := p.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := p.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	return &Output{Stdout: stdout, Stderr: stderr}
}

// Run is the convenience wrapper most main() functions want: it
// calls [Program.Invoke] and passes the result to [Exit], which
// prints a non-help error to [os.Stderr] and calls [os.Exit] with
// the mapped code. It never returns.
//
// Use [Program.Invoke] directly when you need the error value, in
// tests, or when embedding argv in a program that manages its own
// exit lifecycle.
func (p *Program) Run(ctx context.Context, runner Runner, args []string) {
	Exit(p.Invoke(ctx, runner, args))
}

// Invoke runs runner with args[1:]. The program name used in help
// output and command paths is [path/filepath.Base] of args[0], so
// passing [os.Args] directly yields the binary name.
//
// An explicit --help request returns nil after rendering help.
// A non-nil return wraps the underlying error with an [*ExitError]
// carrying a process exit code. Callers can recover the code with
// [errors.As] or pass the result to [Exit]. Invoke panics if ctx
// or runner is nil.
func (p *Program) Invoke(ctx context.Context, runner Runner, args []string) error {
	if ctx == nil {
		panic("argv: nil context")
	}
	if p == nil {
		panic("argv: nil program")
	}
	if runner == nil {
		panic("argv: nil runner")
	}
	stdin := p.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	if len(args) == 0 {
		panic("argv: Program.Invoke requires args with args[0] as the program name")
	}
	programName := filepath.Base(args[0])
	call := &Call{
		ctx:     ctx,
		pattern: programName,
		argv:    args[1:],
		Stdin:   stdin,
	}

	out := p.output()
	err := runner.RunArgv(out, call)

	var helpErr *HelpError
	if errors.As(err, &helpErr) {
		if !helpErr.Explicit && helpErr.Reason != "" {
			fmt.Fprintf(out.Stderr, "%s\n\n", helpErr.Reason)
		}
		rErr := p.renderHelp(out, runner, programName, helpErr.Path, helpErr.Explicit)
		if helpErr.Explicit {
			err = nil
		}
		// Renderer failures must surface even when err already carries a
		// HelpError (the implicit-help path).
		if rErr != nil {
			if err == nil {
				err = rErr
			} else {
				err = errors.Join(err, rErr)
			}
		}
	}

	if fErr := flushWriter(out.Stdout); fErr != nil && err == nil {
		err = fErr
	}
	if fErr := flushWriter(out.Stderr); fErr != nil && err == nil {
		err = fErr
	}

	if err == nil {
		return nil
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr
	}
	return &ExitError{Code: exitCode(err), Err: err}
}

// renderHelp locates the help entry for path in runner's [Walker]
// enumeration and writes it via [Program.HelpFunc] (or
// [DefaultHelpFunc]). Explicit help requests write to stdout;
// implicit ones (shown in lieu of running) write to stderr. It falls
// back to a minimal Help built from [Program.Summary] and
// [Program.Description] when runner does not implement Walker. The
// returned error is whatever the renderer returned, propagated up by
// [Program.Invoke] so a failed write surfaces.
func (p *Program) renderHelp(out *Output, runner Runner, programName, path string, explicit bool) error {
	renderer := p.HelpFunc
	if renderer == nil {
		renderer = DefaultHelpFunc
	}
	w := out.Stderr
	if explicit {
		w = out.Stdout
	}
	if walker, ok := runner.(Walker); ok {
		for help := range walker.WalkArgv(programName, &Help{Summary: p.Summary, Description: p.Description}) {
			if help.FullPath == path {
				return renderer(w, help)
			}
		}
		// Walker exists but the path lives past an opaque boundary
		// (a Runner that does not implement Walker). Render a minimal
		// help for the requested path rather than calling runner.HelpArgv,
		// which would paint root-level metadata under the wrong path.
		return renderer(w, &Help{
			Name:     lastPathSegment(path),
			FullPath: path,
		})
	}
	help := &Help{
		Name:        lastPathSegment(path),
		FullPath:    path,
		Summary:     p.Summary,
		Description: p.Description,
	}
	if h, ok := runner.(Helper); ok {
		h.HelpArgv(help)
	}
	return renderer(w, help)
}

// Walk returns an iterator over every command reachable from runner,
// rooted at name (typically os.Args[0] or another user-visible label).
// Each step yields the accumulated [*Help] for the node, including
// full command path, cascaded global flags and options, and
// subcommand listings, and the raw [Runner] at the node. Walk visits
// nodes depth-first, sorted alphabetically at each level.
//
// Walk delegates to runner's [Walker] implementation. A Runner that
// does not implement Walker yields a single entry using [Program.Summary]
// and [Program.Description]. Walk panics if name is empty.
func (p *Program) Walk(name string, runner Runner) iter.Seq2[*Help, Runner] {
	if name == "" {
		panic("argv: Program.Walk requires a non-empty name")
	}
	return func(yield func(*Help, Runner) bool) {
		base := &Help{Summary: p.Summary, Description: p.Description}

		if w, ok := runner.(Walker); ok {
			first := true
			for help, r := range w.WalkArgv(name, base) {
				if first {
					r = runner
					first = false
				}
				if !yield(help, r) {
					return
				}
			}
			return
		}

		yield(&Help{
			Name:        name,
			FullPath:    name,
			Summary:     p.Summary,
			Description: p.Description,
		}, runner)
	}
}

func walkChildren(n *node, basePath string, ancestorFlags []HelpFlag, ancestorOptions []HelpOption, yield func(*Help, Runner) bool) bool {
	for _, name := range n.childNames() {
		cn := n.children[name]
		childPath := joinedPath(basePath, name)

		if w, ok := cn.commandRunner().(Walker); ok {
			childBase := &Help{
				Summary:     cn.summary(),
				Description: cn.description(),
				Flags:       ancestorFlags,
				Options:     ancestorOptions,
			}
			registered := cn.commandRunner()
			first := true
			for h, r := range w.WalkArgv(childPath, childBase) {
				// The first yield identifies the subtree root; replace
				// with the runner as registered so embedding wrappers
				// (which re-expose an inner type via WalkArgv) still
				// surface to consumers for interface detection.
				if first {
					r = registered
					first = false
				}
				if !yield(h, r) {
					return false
				}
			}
			continue
		}

		childHelp := buildNodeHelp(cn, name, childPath, ancestorFlags, ancestorOptions)
		if !yield(childHelp, cn.commandRunner()) {
			return false
		}

		if len(cn.children) > 0 {
			if !walkChildren(cn, childPath, ancestorFlags, ancestorOptions, yield) {
				return false
			}
		}
	}
	return true
}

func buildNodeHelp(n *node, name, fullPath string, inheritedFlags []HelpFlag, inheritedOptions []HelpOption) *Help {
	help := &Help{
		Name:        name,
		FullPath:    fullPath,
		Summary:     n.summary(),
		Description: n.description(),
		Commands:    n.summaryCommands(""),
		Flags:       slices.Clone(inheritedFlags),
		Options:     slices.Clone(inheritedOptions),
	}
	if h, ok := n.runner.(Helper); ok {
		h.HelpArgv(help)
	}
	return help
}
