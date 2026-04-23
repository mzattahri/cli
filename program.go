package argv

import (
	"context"
	"errors"
	"io"
	"iter"
	"os"
	"slices"
)

// A Program binds a [Runner] to the process environment. The zero
// value is a valid Program that reads [os.Stdin] and writes to
// [os.Stdout] and [os.Stderr].
type Program struct {
	// Name is shown in usage output. An empty Name falls back to args[0].
	Name string

	// Stdout is the standard output writer. A nil Stdout selects [os.Stdout].
	Stdout io.Writer

	// Stderr is the standard error writer. A nil Stderr selects [os.Stderr].
	Stderr io.Writer

	// Stdin is the standard input reader. A nil Stdin selects [os.Stdin].
	Stdin io.Reader

	// Env resolves environment variables. A nil Env selects [os.LookupEnv].
	Env LookupFunc

	// Usage is the short summary shown in top-level help output.
	Usage string

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

// InvokeAndExit is the convenience wrapper most main() functions
// want: it calls [Program.Invoke] and passes the result to [Exit],
// which prints a non-help error to [os.Stderr] and calls [os.Exit]
// with the mapped code. It never returns.
//
// Use [Program.Invoke] directly when you need the error value — in
// tests, or when embedding argv in a program that manages its own
// exit lifecycle.
func (p *Program) InvokeAndExit(ctx context.Context, runner Runner, args []string) {
	Exit(p.Invoke(ctx, runner, args))
}

// Invoke strips args[0] as the program name, runs runner, and
// reports the result as an [*ExitError]. An explicit --help request
// returns nil after rendering help. Invoke panics if ctx or runner is
// nil.
func (p *Program) Invoke(ctx context.Context, runner Runner, args []string) *ExitError {
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
	lookupEnv := p.Env
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	programName := p.Name
	if len(args) > 0 {
		if programName == "" {
			programName = args[0]
		}
		args = args[1:]
	} else if programName == "" {
		if mux, ok := runner.(*Mux); ok && mux.Name != "" {
			programName = mux.Name
		} else {
			programName = "app"
		}
	}
	call := &Call{
		ctx:     ctx,
		Pattern: programName,
		Argv:    slices.Clone(args),
		Help: &Help{
			Usage:       p.Usage,
			Description: p.Description,
		},
		HelpFunc: p.HelpFunc,
		Stdin:    stdin,
		Env:      lookupEnv,
	}

	out := p.output()
	err := runner.RunCLI(out, call)

	// Flush stdout/stderr after the runner returns.
	if fErr := out.Flush(); fErr != nil && err == nil {
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

func (p *Program) programName(runner Runner) string {
	if p.Name != "" {
		return p.Name
	}
	if mux, ok := runner.(*Mux); ok && mux.Name != "" {
		return mux.Name
	}
	return "app"
}

// Walk returns an iterator over every command path reachable from
// runner. Each entry yields the full command path and a [*Help] with
// accumulated flags and options from all routing levels. Walk visits
// nodes depth-first, sorted alphabetically at each level.
//
// Walk delegates to runner's [Walker] implementation. A Runner that
// does not implement Walker yields a single entry using [Program.Usage]
// and [Program.Description].
func (p *Program) Walk(runner Runner) iter.Seq2[string, *Help] {
	return func(yield func(string, *Help) bool) {
		name := p.programName(runner)
		base := &Help{Usage: p.Usage, Description: p.Description}

		if w, ok := runner.(Walker); ok {
			for path, help := range w.WalkCLI(name, base) {
				if !yield(path, help) {
					return
				}
			}
			return
		}

		yield(name, &Help{
			Name:        name,
			FullPath:    name,
			Usage:       p.Usage,
			Description: p.Description,
		})
	}
}

func walkChildren(n *node, basePath string, ancestorFlags []HelpFlag, ancestorOptions []HelpOption, yield func(string, *Help) bool) bool {
	for _, name := range n.childNames() {
		cn := n.children[name]
		childPath := joinedPath(basePath, name)

		if w, ok := cn.commandRunner().(Walker); ok {
			childBase := &Help{
				Usage:       cn.usage(),
				Description: cn.description(),
				Flags:       ancestorFlags,
				Options:     ancestorOptions,
			}
			for p, h := range w.WalkCLI(childPath, childBase) {
				if !yield(p, h) {
					return false
				}
			}
			continue
		}

		childHelp := buildNodeHelp(cn, name, childPath, ancestorFlags, ancestorOptions)
		if !yield(childPath, childHelp) {
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

func buildNodeHelp(n *node, name, fullPath string, globalFlags []HelpFlag, globalOptions []HelpOption) *Help {
	help := &Help{
		Name:        name,
		FullPath:    fullPath,
		Usage:       n.usage(),
		Description: n.description(),
		Commands:    n.usageCommands(""),
		Flags:       slices.Clone(globalFlags),
		Options:     slices.Clone(globalOptions),
	}
	if h, ok := n.runner.(Helper); ok {
		contrib := h.HelpCLI()
		if help.Description == "" {
			help.Description = contrib.Description
		}
		help.Flags = append(help.Flags, contrib.Flags...)
		help.Options = append(help.Options, contrib.Options...)
		help.Arguments = contrib.Arguments
		help.CaptureRest = contrib.CaptureRest
	}
	return help
}

