package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"slices"
)

// Signal returns a [context.Context] that is canceled when the process
// receives an interrupt signal (SIGINT).
//
//	program.Invoke(cli.Signal(), runner, os.Args)
//
// The stop function from [signal.NotifyContext] is intentionally
// discarded: the signal registration lives for the process lifetime.
// In tests, use [context.TODO] instead. For other signals, use
// [signal.NotifyContext] directly.
func Signal() context.Context {
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt)
	return ctx
}

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
	if p == nil {
		return &Output{Stdout: os.Stdout, Stderr: os.Stderr}
	}
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
		panic("cli: nil context")
	}
	if p == nil {
		panic("cli: nil program")
	}
	if runner == nil {
		panic("cli: nil runner")
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
		return err
	}

	newCall := &Call{
		ctx:   call.ctx,
		Stdin: call.Stdin,
		Env:   call.Env,
	}
	return runner.RunCLI(out, newCall)
}
