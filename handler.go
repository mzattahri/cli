package argv

import (
	"errors"
	"io"
	"iter"
	"slices"
)

// Output holds the standard output and error streams for a command.
type Output struct {
	// Stdout is the standard output stream. If it implements
	// [Flusher], [Output.Flush] flushes it after the runner returns.
	Stdout io.Writer

	// Stderr is the standard error stream. If it implements
	// [Flusher], [Output.Flush] flushes it after the runner returns.
	Stderr io.Writer
}

// Flusher is implemented by writers that support flushing buffered output.
type Flusher interface {
	Flush() error
}

// Write writes p to Stdout. It implements [io.Writer].
func (o *Output) Write(p []byte) (int, error) {
	return o.Stdout.Write(p)
}

// Flush flushes Stdout and Stderr. Writers that do not implement
// [Flusher] are skipped.
func (o *Output) Flush() error {
	if err := flushWriter(o.Stdout); err != nil {
		return err
	}
	return flushWriter(o.Stderr)
}

// A Runner handles a command invocation. A Runner should not retain
// out or call after RunCLI returns.
type Runner interface {
	RunCLI(out *Output, call *Call) error
}

// RunnerFunc adapts a function to the [Runner] interface.
type RunnerFunc func(out *Output, call *Call) error

// RunCLI calls f(out, call).
func (f RunnerFunc) RunCLI(out *Output, call *Call) error { return f(out, call) }

// A Command parses command-level input and runs a handler. The zero
// value is a valid empty command; Run must be set before invocation.
type Command struct {
	// Description is the longer help text shown by [HelpFunc].
	Description string

	// CaptureRest preserves unmatched trailing positional arguments in [Call.Rest].
	CaptureRest bool

	// NegateFlags enables --no- prefix negation for boolean flags.
	// When set, --no-flagname sets a flag to false, and a flag
	// declared with a "no-" prefix is also set to false by the bare
	// form.
	NegateFlags bool

	// Run handles the command invocation.
	Run RunnerFunc

	// Completer, if non-nil, provides tab completions for flag and
	// option values.
	Completer Completer

	flags   flagSpecs
	options optionSpecs
	args    argSpecs
}

// Flag declares a boolean flag toggled by presence. short is an
// optional one-character short form; an empty short means the flag
// has no short form. It panics on duplicate or reserved names.
func (c *Command) Flag(name, short string, value bool, usage string) {
	checkCrossCollision(name, short, c.options.hasName, c.options.hasShort)
	c.flags.add(name, short, value, usage)
}

// Option declares a named value option with a default. short is an
// optional one-character short form; an empty short means the option
// has no short form. It panics on duplicate or reserved names.
func (c *Command) Option(name, short, value, usage string) {
	checkCrossCollision(name, short, c.flags.hasName, c.flags.hasShort)
	c.options.add(name, short, value, usage)
}

// Arg declares a required positional argument. It panics if name is
// empty or duplicated.
func (c *Command) Arg(name, usage string) {
	c.args.add(name, usage)
}

// RunCLI parses command-level inputs from call.Argv, binds positional
// arguments, applies defaults, and invokes Run. --help or -h renders
// help to out.Stderr and returns nil. A parse error returns an
// [*ExitError] with code [ExitUsage].
func (c *Command) RunCLI(out *Output, call *Call) error {
	if c.Run == nil {
		panic("argv: nil command handler")
	}
	fs, os, as := c.inputs()

	parsed, err := parseInput(fs, os, slices.Clone(call.Argv), c.NegateFlags)
	if err != nil {
		if errors.Is(err, errFlagHelp) {
			return c.renderHelp(out, call, true)
		}
		return Errorf(ExitUsage, "%s: %w", call.Pattern, err)
	}

	var argState ArgSet
	var restState []string
	if as != nil {
		argState, restState, err = as.parse(parsed.args, c.CaptureRest)
		if err != nil {
			return Errorf(ExitUsage, "%s: %w", call.Pattern, err)
		}
	} else if c.CaptureRest {
		restState = slices.Clone(parsed.args)
	} else if len(parsed.args) > 0 {
		return Errorf(ExitUsage, "%s: unexpected argument %q", call.Pattern, parsed.args[0])
	}

	runCall := enrichCall(call, parsed, fs, os)
	runCall.Args = argState
	runCall.Rest = restState
	runCall.argNames = as.names()
	return c.Run(out, runCall)
}

// renderHelp renders the command's help to out.Stderr using
// [Call.Help] for ancestor context. It returns nil when explicit (the
// caller requested help), or [ErrHelp] when implicit (help shown in
// lieu of running).
func (c *Command) renderHelp(out *Output, call *Call, explicit bool) error {
	fs, os, as := c.inputs()
	base := call.Help
	if base == nil {
		base = &Help{}
	}
	help := &Help{
		Name:        lastPathSegment(call.Pattern),
		FullPath:    call.Pattern,
		Usage:       base.Usage,
		Description: firstNonEmpty(base.Description, c.Description),
		Flags:       append(slices.Clone(base.Flags), fs.helpEntriesNegatable(c.NegateFlags)...),
		Options:     append(slices.Clone(base.Options), os.helpEntries()...),
		CaptureRest: c.CaptureRest,
	}
	if as != nil {
		help.Arguments = as.HelpArguments()
	}
	if err := resolveHelpFunc(call.HelpFunc)(out.Stderr, help); err != nil {
		return err
	}
	if explicit {
		return nil
	}
	return ErrHelp
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
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

// Helper is implemented by Runners that contribute help metadata.
// The returned [Help] is partial; the caller fills in Name, FullPath,
// Usage, and Commands.
type Helper interface {
	HelpCLI() Help
}

// Walker is implemented by Runners that expose a subtree of commands.
// WalkCLI yields (path, help) for itself and every reachable
// descendant. base carries the ancestor-derived partial [Help]
// (Usage, Description, and accumulated global Flags and Options). A
// nil base is equivalent to an empty one.
type Walker interface {
	WalkCLI(path string, base *Help) iter.Seq2[string, *Help]
}

// HelpCLI returns the command's declared Description, Flags, Options,
// Arguments, and CaptureRest.
func (c *Command) HelpCLI() Help {
	fs, os, as := c.inputs()
	help := Help{
		Description: c.Description,
		CaptureRest: c.CaptureRest,
		Flags:       fs.helpEntriesNegatable(c.NegateFlags),
		Options:     os.helpEntries(),
	}
	if as != nil {
		help.Arguments = as.HelpArguments()
	}
	return help
}

// WalkCLI yields a single (path, help) entry for the command. It
// merges ancestor globals from base with the command's own inputs.
func (c *Command) WalkCLI(path string, base *Help) iter.Seq2[string, *Help] {
	return func(yield func(string, *Help) bool) {
		h := c.HelpCLI()
		h.Name = lastPathSegment(path)
		h.FullPath = path
		if base != nil {
			h.Usage = base.Usage
			h.Description = firstNonEmpty(base.Description, h.Description)
			h.Flags = append(slices.Clone(base.Flags), h.Flags...)
			h.Options = append(slices.Clone(base.Options), h.Options...)
		}
		yield(path, &h)
	}
}

// A MiddlewareFunc wraps a [Runner] in another Runner.
type MiddlewareFunc func(Runner) Runner

// Chain composes middleware. The first argument is the outermost
// wrapper.
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
