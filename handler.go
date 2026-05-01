package argv

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"slices"
)

// Output holds the standard output and error streams for a command.
type Output struct {
	// Stdout is the standard output stream. If it implements
	// [Flusher], [Program.Invoke] flushes it after the runner returns.
	Stdout io.Writer

	// Stderr is the standard error stream. If it implements
	// [Flusher], [Program.Invoke] flushes it after the runner returns.
	Stderr io.Writer
}

// Flusher is implemented by writers that support flushing buffered output.
type Flusher interface {
	Flush() error
}

// A Runner handles a command invocation. A Runner should not retain
// out or call after RunArgv returns.
type Runner interface {
	RunArgv(out *Output, call *Call) error
}

// RunnerFunc adapts a function to the [Runner] interface.
type RunnerFunc func(out *Output, call *Call) error

// RunArgv calls f(out, call).
func (f RunnerFunc) RunArgv(out *Output, call *Call) error { return f(out, call) }

// A Command parses command-level input and runs a handler. The zero
// value is a valid empty command; Run must be set before invocation.
type Command struct {
	// Description is the longer help text shown by [HelpFunc].
	Description string

	// Hidden omits the command from its parent's subcommand listing and
	// from completion candidates. The command remains routable: an
	// explicit invocation runs the handler, and --help still renders.
	Hidden bool

	// NegateFlags enables --no- prefix negation for the boolean flags
	// declared on this Command. When set:
	//
	//   - --no-verbose sets a flag named "verbose" to false.
	//   - A flag declared as "no-cache" is also reached by --cache,
	//     which sets it to false (bidirectional).
	//
	// Declaring both "cache" and "no-cache" at the same level is
	// rejected at declaration time, because --cache and --no-cache
	// would otherwise resolve to different flags depending on
	// declaration order.
	//
	// NegateFlags is scoped to this Command. It does not inherit from
	// an enclosing Mux's NegateFlags, nor propagate to descendants.
	// Each level parses the flags it declares and applies negation
	// only when its own NegateFlags is set, so "app --no-verbose
	// deploy --no-cache" requires the Mux that declared "verbose" to
	// have NegateFlags set AND the Command that declared "cache" to
	// have NegateFlags set independently.
	NegateFlags bool

	// Run handles the command invocation.
	Run RunnerFunc

	// Annotations carry per-node metadata copied into
	// [Help.Annotations] by [Command.HelpArgv].
	//
	// argv does not interpret the values. Use namespaced keys
	// (e.g. "manpage/seealso") to avoid collisions across packages.
	// Annotations do not inherit from or propagate to ancestors.
	Annotations map[string]any

	flags   flagSpecs
	options optionSpecs
	args    argSpecs
	tail    *argSpec
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

// Tail declares a trailing variadic argument. It captures all
// positional tokens after declared [Command.Arg]s into [Call.Tail],
// and surfaces in help as "[<name>...]". usage is the short
// description shown alongside in the Arguments section; an empty
// usage suppresses that row but still names the tail in the usage
// line.
//
// It panics if Tail is called twice, if name is empty or invalid,
// or if name duplicates a declared positional argument.
func (c *Command) Tail(name, usage string) {
	if c.tail != nil {
		panic("argv: Tail already declared")
	}
	validateInputName(name)
	if c.args.hasName(name) {
		panic(fmt.Sprintf("argv: tail name %q duplicates a positional argument", name))
	}
	c.tail = &argSpec{Name: name, Usage: usage}
}

// RunArgv parses command-level inputs from the call's argv, binds
// positional arguments, applies defaults, and invokes Run. --help or
// -h returns a [*HelpError] for the current path, which
// [Program.Invoke] turns into rendered help. A parse error returns
// an [*ExitError] with code [ExitUsage].
func (c *Command) RunArgv(out *Output, call *Call) error {
	if c.Run == nil {
		panic("argv: nil command handler")
	}
	fs, os, as := c.inputs()

	parsed, err := parseInput(fs, os, call.argv, c.NegateFlags)
	if err != nil {
		if errors.Is(err, errFlagHelp) {
			return &HelpError{Path: call.pattern, Explicit: true}
		}
		return Errorf(ExitUsage, "%s: %w", call.pattern, err)
	}

	var argState ArgSet
	var tailState []string
	if as != nil {
		argState, tailState, err = as.parse(parsed.args, c.tail != nil)
		if err != nil {
			return Errorf(ExitUsage, "%s: %w", call.pattern, err)
		}
	} else if c.tail != nil {
		tailState = slices.Clone(parsed.args)
	} else if len(parsed.args) > 0 {
		return Errorf(ExitUsage, "%s: unexpected argument %q", call.pattern, parsed.args[0])
	}

	applyParse(call, parsed, fs, os)
	call.Args = argState
	call.Tail = tailState
	call.argNames = as.names()
	return c.Run(out, call)
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

// Helper is implemented by [Runner] types that contribute help
// metadata. HelpArgv receives an [*Help] with routing-level fields
// (Name, FullPath, Summary, Commands) pre-populated by the dispatcher
// and fills in Description, Flags, Options, Arguments, and Tail.
// HelpArgv appends to Flags and Options rather than replacing them
// so that ancestor globals are preserved.
//
// Implementations must not retain h past the call.
type Helper interface {
	Runner
	HelpArgv(h *Help)
}

// Walker is implemented by [Runner] types that expose a subtree of
// commands. WalkArgv yields (help, runner) for itself and every
// reachable descendant.
//
// help carries the accumulated [Help] for each node (Name, FullPath,
// Summary, Description, cascaded inherited Flags and Options, and
// Commands), so consumers never need to recompute ancestor context.
// runner is the raw [Runner] at the node, exposed so consumers can
// type-assert for [Completer] or other optional interfaces. base
// carries the ancestor-derived partial [Help] that this subtree
// should build on; a nil base is equivalent to an empty one.
//
// Implementations must:
//   - Yield a fresh *Help per node. Reusing pointers across yields
//     breaks consumers that index by FullPath (e.g. completion).
//   - Treat base as read-only. Clone base.Flags and base.Options
//     before appending; do not mutate them in place.
type Walker interface {
	WalkArgv(path string, base *Help) iter.Seq2[*Help, Runner]
}

// HelpArgv contributes the command's declared Description, Flags,
// Options, Arguments, and Tail to h. It appends to h.Flags and
// h.Options so ancestor globals set by the dispatcher are preserved.
func (c *Command) HelpArgv(h *Help) {
	fs, os, as := c.inputs()
	if h.Description == "" {
		h.Description = c.Description
	}
	h.Flags = append(h.Flags, fs.helpEntriesNegatable(c.NegateFlags)...)
	h.Options = append(h.Options, os.helpEntries()...)
	if as != nil {
		h.Arguments = as.helpArguments()
	}
	if c.tail != nil {
		h.Tail = &HelpArg{
			Name:  "[<" + c.tail.Name + ">...]",
			Usage: c.tail.Usage,
		}
	}
	h.Hidden = c.Hidden
	h.Annotations = maps.Clone(c.Annotations)
}

// WalkArgv yields a single (help, runner) entry for the command. It
// merges ancestor globals from base with the command's own inputs.
func (c *Command) WalkArgv(path string, base *Help) iter.Seq2[*Help, Runner] {
	return func(yield func(*Help, Runner) bool) {
		h := &Help{
			Name:     lastPathSegment(path),
			FullPath: path,
		}
		if base != nil {
			h.Summary = base.Summary
			h.Description = base.Description
			h.Flags = slices.Clone(base.Flags)
			h.Options = slices.Clone(base.Options)
		}
		c.HelpArgv(h)
		yield(h, c)
	}
}

// A Middleware wraps a [Runner] in another Runner. Construct one with
// [NewMiddleware] from an "around" function; use the named type in
// signatures for readability.
//
//	func WithLogging(logger *slog.Logger) argv.Middleware { ... }
//
// A plain func([Runner]) [Runner] literal is assignable to Middleware
// and vice versa; callers do not need explicit conversions.
type Middleware func(Runner) Runner

// NewMiddleware returns a [Middleware] that wraps its input with
// around. The returned wrapper dispatches via around while delegating
// [Helper], [Walker], and [Completer] to the inner Runner, so
// introspection metadata survives the wrap.
//
// around mirrors [Runner.RunArgv] with a trailing next; invoke
// next.RunArgv to continue the chain, or return an error to
// short-circuit. See the package overview for an example.
//
// The wrapper implements Helper, Walker, and Completer iff the wrapped
// Runner does. Callers should not type-assert the wrapper; its
// concrete type is unexported by design.
func NewMiddleware(around func(out *Output, call *Call, next Runner) error) Middleware {
	if around == nil {
		panic("argv: nil around func")
	}
	return func(next Runner) Runner {
		if next == nil {
			panic("argv: nil runner")
		}
		return &wrappedRunner{
			inner: next,
			run: func(out *Output, call *Call) error {
				return around(out, call, next)
			},
		}
	}
}

type wrappedRunner struct {
	inner Runner
	run   RunnerFunc
}

func (w *wrappedRunner) RunArgv(out *Output, call *Call) error {
	return w.run(out, call)
}

func (w *wrappedRunner) HelpArgv(h *Help) {
	if inner, ok := w.inner.(Helper); ok {
		inner.HelpArgv(h)
	}
}

func (w *wrappedRunner) WalkArgv(path string, base *Help) iter.Seq2[*Help, Runner] {
	if inner, ok := w.inner.(Walker); ok {
		return inner.WalkArgv(path, base)
	}
	return func(yield func(*Help, Runner) bool) {
		h := &Help{Name: lastPathSegment(path), FullPath: path}
		if base != nil {
			h.Summary = base.Summary
			h.Description = base.Description
			h.Flags = slices.Clone(base.Flags)
			h.Options = slices.Clone(base.Options)
		}
		if inner, ok := w.inner.(Helper); ok {
			inner.HelpArgv(h)
		}
		yield(h, w)
	}
}

func (w *wrappedRunner) CompleteArgv(tw *TokenWriter, completed []string, partial string) error {
	if inner, ok := w.inner.(Completer); ok {
		return inner.CompleteArgv(tw, completed, partial)
	}
	return nil
}

func flushWriter(w io.Writer) error {
	if f, ok := w.(Flusher); ok {
		return f.Flush()
	}
	return nil
}
