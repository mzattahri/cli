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
type Call struct {
	ctx context.Context

	// Pattern is the matched command path, e.g. "app deploy". A
	// dispatcher sets it before invoking a child [Runner] so errors
	// and help output report the path the user typed.
	Pattern string

	// Argv holds the unconsumed argument tokens. A leaf [Runner]
	// parses Argv to populate Flags, Options, and Args. A dispatcher
	// updates Argv on the handoff call to reflect what remains for
	// the child.
	Argv []string

	// Help carries ancestor-aware help data inherited from parent
	// dispatchers: Usage and Description for this routing level, and
	// accumulated global Flags and Options. A leaf [Runner] extends
	// it with its own contribution before rendering. Nil at the root.
	Help *Help

	// HelpFunc renders help output. A nil HelpFunc selects
	// [DefaultHelpFunc].
	HelpFunc HelpFunc

	// Stdin is the standard input stream.
	Stdin io.Reader

	// Env resolves environment variables.
	Env LookupFunc

	// Flags holds boolean flags from all levels, mux and command.
	Flags FlagSet

	// Options holds option values from all levels, mux and command.
	Options OptionSet

	// Args holds bound positional arguments.
	Args ArgSet

	// Rest holds trailing positional arguments when
	// [Command.CaptureRest] is set.
	Rest []string

	// argNames preserves declared argument order for [Call.String].
	argNames []string
}

// A LookupFunc resolves a name to a value. It matches the signature
// of [os.LookupEnv].
type LookupFunc func(string) (string, bool)

// NewLookupFunc returns a [LookupFunc] backed by env. A nil env
// produces a function that always reports a miss.
func NewLookupFunc(env map[string]string) LookupFunc {
	if env == nil {
		return func(string) (string, bool) { return "", false }
	}
	return func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}
}

// NewCall returns a new [Call] for argv. It panics if ctx is nil.
func NewCall(ctx context.Context, argv []string) *Call {
	if ctx == nil {
		panic("argv: nil context")
	}
	return &Call{
		ctx:     ctx,
		Argv:    slices.Clone(argv),
		Env:     NewLookupFunc(nil),
		Flags:   FlagSet{},
		Options: OptionSet{},
		Args:    ArgSet{},
	}
}

// WithContext returns a copy of c with ctx replacing the context.
// Sets and slices are deep-copied. It panics if ctx is nil.
func (c *Call) WithContext(ctx context.Context) *Call {
	if ctx == nil {
		panic("argv: nil context")
	}
	c2 := *c
	c2.ctx = ctx
	c2.Argv = slices.Clone(c.Argv)
	c2.Flags = c.Flags.Clone()
	c2.Options = c.Options.Clone()
	c2.Args = c.Args.Clone()
	c2.Rest = slices.Clone(c.Rest)
	c2.argNames = slices.Clone(c.argNames)
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
//	<pattern> flag:<name>=<bool> opt:<name>=<value> arg:<name>=<value> rest:<value>
//
// Flags and options are sorted by name. Arguments preserve declaration
// order when available, otherwise they are sorted by name.
// Values containing spaces or special characters are quoted.
func (c *Call) String() string {
	tokens := make([]string, 0)
	if c.Pattern != "" {
		tokens = append(tokens, c.Pattern)
	}
	tokens = append(tokens, canonicalFlagTokens("flag", c.Flags)...)
	tokens = append(tokens, canonicalOptionTokens("opt", c.Options)...)
	tokens = append(tokens, canonicalArgTokens(c.Args, c.argNames)...)

	for _, token := range c.Rest {
		tokens = append(tokens, "rest:"+quoteToken(token))
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
		if !args.Has(name) {
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
