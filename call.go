package cli

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

// A Call holds the parsed input for a single command invocation.
type Call struct {
	ctx context.Context

	// Pattern is the matched command path (e.g. "app deploy").
	Pattern string

	// Stdin is the standard input stream.
	Stdin io.Reader

	// Env resolves environment variables.
	Env LookupFunc

	// Flags holds boolean flags from all levels (mux and command).
	Flags FlagSet

	// Options holds option values from all levels (mux and command).
	Options OptionSet

	// Args holds bound positional arguments.
	Args ArgSet

	// Rest holds trailing positional arguments when
	// [Command.CaptureRest] is set.
	Rest []string
}

// callState carries dispatch metadata through the context.
// It holds only routing state consumed by the dispatch machinery.
type callState struct {
	argv     []string
	argNames []string
}

type callStateKey struct{}

func getState(ctx context.Context) *callState {
	s, _ := ctx.Value(callStateKey{}).(*callState)
	return s
}

func setState(ctx context.Context, s *callState) context.Context {
	return context.WithValue(ctx, callStateKey{}, s)
}

// A LookupFunc resolves a name to a value. It follows the signature
// of [os.LookupEnv].
type LookupFunc func(string) (string, bool)

// NewLookupFunc returns a [LookupFunc] backed by env.
// When env is nil the returned function always reports a miss.
func NewLookupFunc(env map[string]string) LookupFunc {
	if env == nil {
		return func(string) (string, bool) { return "", false }
	}
	return func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}
}

// NewCall returns a new [Call] for the given argument tokens.
// It panics if ctx is nil.
func NewCall(ctx context.Context, argv []string) *Call {
	if ctx == nil {
		panic("cli: nil context")
	}
	return &Call{
		ctx:     setState(ctx, &callState{argv: slices.Clone(argv)}),
		Env:     NewLookupFunc(nil),
		Flags:   FlagSet{},
		Options: OptionSet{},
		Args:    ArgSet{},
	}
}

// WithContext returns a copy of c with ctx replacing the original
// context. Sets and slices are deep-copied.
// It panics if ctx is nil.
func (c *Call) WithContext(ctx context.Context) *Call {
	if ctx == nil {
		panic("cli: nil context")
	}
	if s := getState(c.ctx); s != nil {
		ctx = setState(ctx, s)
	}
	c2 := *c
	c2.ctx = ctx
	c2.Flags = c.Flags.Clone()
	c2.Options = c.Options.Clone()
	c2.Args = c.Args.Clone()
	c2.Rest = slices.Clone(c.Rest)
	return &c2
}

// Context returns the call's context, defaulting to [context.Background]
// if the context is nil.
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

	var argNames []string
	if s := getState(c.ctx); s != nil {
		argNames = s.argNames
	}
	tokens = append(tokens, canonicalArgTokens(c.Args, argNames)...)

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
