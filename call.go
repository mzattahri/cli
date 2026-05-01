package argv

import (
	"context"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
)

type flagSpec struct {
	Name    string
	Short   string
	Usage   string
	Default bool
}

type optionSpec struct {
	Name    string
	Short   string
	Usage   string
	Default string
}

type argSpec struct {
	Name  string
	Usage string
}

// A Call holds the parsed input for a single command invocation. The
// zero value is not usable; build one through [NewCall] or
// [Program.Invoke].
//
// Pattern and the unconsumed token list are unexported; read them
// via [Call.Pattern] and [Call.Argv]. Dispatchers advance both by
// calling [Call.WithArgv] when handing off to a nested runner.
type Call struct {
	ctx     context.Context
	pattern string
	argv    []string

	// Stdin is the standard input stream.
	Stdin io.Reader

	// Flags holds boolean flags from all levels, mux and command.
	Flags FlagSet

	// Options holds option values from all levels, mux and command.
	Options OptionSet

	// Args holds bound positional arguments.
	Args ArgSet

	// Tail holds trailing positional arguments captured by
	// [Command.Variadic].
	Tail []string

	// argNames preserves declared argument order for [Call.String].
	argNames []string
}

// A LookupFunc resolves a name to a value. It matches the signature
// of [os.LookupEnv].
type LookupFunc func(string) (string, bool)

// NewCall returns a new [Call] for args. The returned call takes
// ownership of args; the caller must not mutate it afterward. It
// panics if ctx is nil.
func NewCall(ctx context.Context, args []string) *Call {
	if ctx == nil {
		panic("argv: nil context")
	}
	return &Call{
		ctx:  ctx,
		argv: args,
	}
}

// Pattern returns the matched command path, e.g. "app deploy". A
// dispatcher updates it via [Call.WithArgv] before invoking a child
// [Runner] so errors and help output report the path the user typed.
func (c *Call) Pattern() string { return c.pattern }

// Argv returns the unconsumed argument tokens. A leaf [Runner] parses
// them to populate Flags, Options, and Args. A dispatcher updates
// Argv via [Call.WithArgv] on the handoff call to reflect what
// remains for the child.
func (c *Call) Argv() []string { return c.argv }

// WithArgv returns a shallow copy of c with name as Pattern and
// argv as the unconsumed token list. Dispatchers use it to hand off
// to a nested runner.
//
// The copy shares Flags, Options, and Args map storage with the
// receiver, which is intentional: parsed inputs cascade naturally
// into nested runners. A child runner that mutates these maps
// affects the parent's view.
func (c *Call) WithArgv(name string, argv []string) *Call {
	c2 := *c
	c2.pattern = name
	c2.argv = argv
	return &c2
}

// WithContext returns a shallow copy of c with its context changed
// to ctx. The provided ctx must be non-nil.
func (c *Call) WithContext(ctx context.Context) *Call {
	if ctx == nil {
		panic("argv: nil context")
	}
	c2 := *c
	c2.ctx = ctx
	return &c2
}

// Context returns the call's context. A nil context returns
// [context.Background].
func (c *Call) Context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

// String returns a canonical representation of the call.
//
// The output starts with [Call.Pattern] followed by parsed fields as
// space-separated tokens in a stable format:
//
//	<pattern> flag:<name>=<bool> opt:<name>=<value> arg:<name>=<value> tail:<value>
//
// Flags and options are sorted by name. Arguments preserve declaration
// order when available, otherwise they are sorted by name. Tail
// tokens preserve insertion order (positional). Values containing
// spaces or special characters are quoted.
func (c *Call) String() string {
	tokens := make([]string, 0)
	if c.pattern != "" {
		tokens = append(tokens, c.pattern)
	}
	tokens = append(tokens, canonicalFlagTokens("flag", c.Flags)...)
	tokens = append(tokens, canonicalOptionTokens("opt", c.Options)...)
	tokens = append(tokens, canonicalArgTokens(c.Args, c.argNames)...)

	for _, token := range c.Tail {
		tokens = append(tokens, "tail:"+quoteToken(token))
	}
	if len(tokens) == 0 {
		return ""
	}
	return strings.Join(tokens, " ")
}

func canonicalFlagTokens(prefix string, flags FlagSet) []string {
	if flags.Len() == 0 {
		return nil
	}
	tokens := make([]string, 0, flags.Len())
	for name, value := range flags.All() {
		tokens = append(tokens, prefix+":"+name+"="+strconv.FormatBool(value))
	}
	slices.Sort(tokens)
	return tokens
}

func canonicalOptionTokens(prefix string, opts OptionSet) []string {
	if opts.Len() == 0 {
		return nil
	}
	// Collect sorted keys, then iterate values in insertion order per key.
	keys := slices.Sorted(maps.Keys(opts.m))
	var tokens []string
	for _, name := range keys {
		for _, value := range opts.Values(name) {
			tokens = append(tokens, prefix+":"+name+"="+quoteToken(value))
		}
	}
	return tokens
}

func canonicalArgTokens(args ArgSet, argNames []string) []string {
	if args.Len() == 0 {
		return nil
	}
	names := argNames
	if len(names) == 0 {
		names = slices.Sorted(maps.Keys(args.m))
	}
	tokens := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := args.m[name]; !ok {
			continue
		}
		tokens = append(tokens, "arg:"+name+"="+quoteToken(args.Get(name)))
	}
	return tokens
}

func quoteToken(token string) string {
	if token == "" {
		return `""`
	}
	if strings.ContainsAny(token, " \t\n\r\"'\\`$&|;()<>[]{}*?!#~") {
		return strconv.Quote(token)
	}
	return token
}
