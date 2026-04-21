package argv

import (
	"context"
	"errors"
	"io"
	"iter"
	"os"
	"slices"
)

// A Program describes the process environment for a CLI invocation.
type Program struct {
	// Name is shown in usage output. When empty, [Program.Invoke]
	// uses args[0].
	Name string

	// Stdout is the standard output writer. When nil, [Program.Invoke]
	// uses [os.Stdout].
	Stdout io.Writer

	// Stderr is the standard error writer. When nil, [Program.Invoke]
	// uses [os.Stderr].
	Stderr io.Writer

	// Stdin is the standard input reader. When nil, [Program.Invoke]
	// uses [os.Stdin].
	Stdin io.Reader

	// Env resolves environment variables. When nil,
	// [Program.Invoke] uses [os.LookupEnv].
	Env LookupFunc

	// Usage is the short summary shown in top-level help output.
	Usage string

	// Description is longer free-form help text.
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

// Invoke strips args[0] as the program name, runs the given runner, and
// reports the result as an [*ExitError]. An explicit --help request
// returns nil after rendering help. Invoke panics if ctx is nil.
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
		ctx:   setState(ctx, &callState{argv: slices.Clone(args)}),
		Stdin: stdin,
		Env:   lookupEnv,
	}

	out := p.output()
	err := invokeRunnerWithProgram(p, runner, out, call, programName)

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
// runner. Each entry yields the full command path and a [*Help]
// struct with accumulated flags and options from all routing levels.
//
// Walk visits nodes depth-first, sorted alphabetically at each level.
func (p *Program) Walk(runner Runner) iter.Seq2[string, *Help] {
	return func(yield func(string, *Help) bool) {
		name := p.programName(runner)

		mux, ok := runner.(*Mux)
		if !ok {
			yield(name, &Help{
				Name:        name,
				FullPath:    name,
				Usage:       p.Usage,
				Description: p.Description,
			})
			return
		}

		walkMux(mux, name, p.Usage, p.Description, nil, nil, yield)
	}
}

func walkMux(m *Mux, path, usage, description string, ancestorFlags []HelpFlag, ancestorOptions []HelpOption, yield func(string, *Help) bool) bool {
	muxFlags, muxOptions := m.muxInputs()
	globalFlags, globalOptions := accumulateHelp(
		&ancestorHelp{flags: ancestorFlags, options: ancestorOptions},
		muxFlags, muxOptions, m.NegateFlags,
	)

	name := lastPathSegment(path)
	help := &Help{
		Name:        name,
		FullPath:    path,
		Usage:       usage,
		Description: description,
		Commands:    m.root.usageCommands(""),
		Flags:       slices.Clone(globalFlags),
		Options:     slices.Clone(globalOptions),
	}
	if !yield(path, help) {
		return false
	}

	return walkChildren(&m.root, path, globalFlags, globalOptions, yield)
}

func walkChildren(n *node, basePath string, globalFlags []HelpFlag, globalOptions []HelpOption, yield func(string, *Help) bool) bool {
	for _, info := range n.childInfos("") {
		childPath := joinedPath(basePath, info.name)
		cn := info.node

		if sub, ok := cn.commandRunner().(*Mux); ok {
			if !walkMux(sub, childPath, cn.usage(), cn.description(), globalFlags, globalOptions, yield) {
				return false
			}
			continue
		}

		childHelp := buildNodeHelp(cn, info.name, childPath, globalFlags, globalOptions)
		if !yield(childPath, childHelp) {
			return false
		}

		if len(cn.children) > 0 {
			if !walkChildren(cn, childPath, globalFlags, globalOptions, yield) {
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
	if cmd := n.command; cmd != nil {
		fs, os, as := cmd.inputs()
		help.Flags = append(help.Flags, fs.helpEntriesNegatable(cmd.NegateFlags)...)
		help.Options = append(help.Options, os.helpEntries()...)
		if as != nil {
			help.Arguments = as.HelpArguments()
		}
		help.CaptureRest = cmd.CaptureRest
	}
	return help
}

func invokeRunnerWithProgram(program *Program, runner Runner, out *Output, call *Call, fullPath string) error {
	if mux, ok := runner.(*Mux); ok {
		name := fullPath
		if name == "" {
			name = mux.Name
		}
		return mux.runWithPath(out, call, name, program.Usage, program.Description, nil, program.HelpFunc)
	}

	// Non-mux runner: parse for --help handling only.
	argv := getState(call.ctx).argv
	_, err := parseInput(nil, nil, argv, false)
	if err != nil {
		if errors.Is(err, errFlagHelp) {
			return resolveHelpFunc(program.HelpFunc)(out.Stderr, &Help{
				Name:        lastPathSegment(fullPath),
				FullPath:    fullPath,
				Usage:       program.Usage,
				Description: program.Description,
			})
		}
		return Errorf(ExitUsage, "%s: %w", fullPath, err)
	}

	newCall := &Call{
		ctx:   call.ctx,
		Stdin: call.Stdin,
		Env:   call.Env,
	}
	return runner.RunCLI(out, newCall)
}
