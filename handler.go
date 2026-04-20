package cli

import "io"

// Output holds the standard output and error streams for a command.
type Output struct {
	// Stdout is the standard output stream. If it implements [Flusher],
	// [Output.Flush] flushes it after the runner returns.
	Stdout io.Writer

	// Stderr is the standard error stream. If it implements [Flusher],
	// [Output.Flush] flushes it after the runner returns.
	Stderr io.Writer
}

// Flusher is implemented by writers that support flushing buffered output.
type Flusher interface {
	Flush() error
}

// Write writes p to Stdout, implementing [io.Writer].
func (o *Output) Write(p []byte) (int, error) {
	return o.Stdout.Write(p)
}

// Flush flushes Stdout and Stderr if either implements [Flusher].
func (o *Output) Flush() error {
	if err := flushWriter(o.Stdout); err != nil {
		return err
	}
	if err := flushWriter(o.Stderr); err != nil {
		return err
	}
	return nil
}

// A Runner handles an invocation.
//
// RunCLI writes output to out and returns nil on success or a non-nil
// error on failure. A Runner should not retain out or call after
// RunCLI returns.
type Runner interface {
	RunCLI(out *Output, call *Call) error
}

// RunnerFunc adapts a plain function to the [Runner] interface.
type RunnerFunc func(out *Output, call *Call) error

// RunCLI calls f(out, call).
func (f RunnerFunc) RunCLI(out *Output, call *Call) error { return f(out, call) }

// A Command parses command-level input and runs a handler.
//
// Flags, options, and positional arguments are declared with the
// [Command.Flag], [Command.Option], and [Command.Arg] methods.
type Command struct {
	// Description is the longer help text shown by [HelpFunc].
	Description string

	// CaptureRest preserves unmatched trailing positional arguments
	// in [Call.Rest].
	CaptureRest bool

	// NegateFlags enables --no- prefix negation for boolean flags.
	// When true, --no-flagname sets a flag to false, and if a flag
	// is declared with a "no-" prefix, --flagname (without the prefix)
	// also sets it to false.
	NegateFlags bool

	// Run handles the command invocation.
	Run Runner

	// Completer, if non-nil, provides tab completions for this
	// command. See [Command.Complete] for delegation details.
	Completer Completer

	flags   flagSpecs
	options optionSpecs
	args    argSpecs
}

// Flag declares a boolean flag toggled by presence.
//
// short is an optional one-character short form (e.g. "v" for -v).
// An empty string means the flag has no short form.
// It panics on duplicate or reserved names.
func (c *Command) Flag(name, short string, value bool, usage string) {
	checkCrossCollision(name, short, c.options.hasName, c.options.hasShort)
	c.flags.add(name, short, value, usage)
}

// Option declares a named value option with a default.
//
// short is an optional one-character short form (e.g. "o" for -o).
// An empty string means the option has no short form.
// It panics on duplicate or reserved names.
func (c *Command) Option(name, short, value, usage string) {
	checkCrossCollision(name, short, c.flags.hasName, c.flags.hasShort)
	c.options.add(name, short, value, usage)
}

// Arg declares a required positional argument.
// It panics if name is empty or duplicated.
func (c *Command) Arg(name, usage string) {
	c.args.add(name, usage)
}

// RunCLI delegates to c.Run.
func (c *Command) RunCLI(out *Output, call *Call) error {
	return c.Run.RunCLI(out, call)
}

func (c *Command) inputs() (*flagSpecs, *optionSpecs, *argSpecs) {
	fs := &c.flags
	os := &c.options
	as := &c.args
	if len(fs.specs) == 0 {
		fs = nil
	}
	if len(os.specs) == 0 {
		os = nil
	}
	if len(as.specs) == 0 {
		as = nil
	}
	return fs, os, as
}

// A MiddlewareFunc wraps a [Runner] in another Runner.
type MiddlewareFunc func(Runner) Runner

// Chain composes middleware in the order given. The first middleware is
// the outermost wrapper.
//
//	stack := Chain(withLogging, withAuth)
//	mux.Handle("deploy", "Deploy", stack(deployCmd))
func Chain(mw ...MiddlewareFunc) MiddlewareFunc {
	return func(r Runner) Runner {
		for i := len(mw) - 1; i >= 0; i-- {
			r = mw[i](r)
		}
		return r
	}
}

func flushWriter(w io.Writer) error {
	if f, ok := w.(Flusher); ok {
		return f.Flush()
	}
	return nil
}
