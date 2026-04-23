package argv

import (
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"
)

// A Mux is a command multiplexer. It matches argv tokens against
// registered command names and dispatches to the corresponding
// [Runner]. The zero value is not usable; build one with [NewMux].
type Mux struct {
	// Name is the mux identifier used in help output and command paths.
	Name string

	// Description is the longer help text rendered by [HelpFunc]
	// before the subcommand list.
	Description string

	// NegateFlags enables --no- prefix negation for mux-level boolean
	// flags. See [Command.NegateFlags] for semantics.
	NegateFlags bool

	root    node
	flags   flagSpecs
	options optionSpecs
}

// node is an internal trie node for command routing.
type node struct {
	runner          Runner
	usageText       string
	descriptionText string
	children        map[string]*node
}

func (n *node) getOrCreate(name string) *node {
	if n.children == nil {
		n.children = map[string]*node{}
	}
	child, ok := n.children[name]
	if !ok {
		child = &node{}
		n.children[name] = child
	}
	return child
}

// childNames returns the node's child keys in alphabetical order.
func (n *node) childNames() []string {
	return slices.Sorted(maps.Keys(n.children))
}

func (n *node) usageCommands(prefix string) []HelpCommand {
	names := n.childNames()
	cmds := make([]HelpCommand, 0, len(names))
	for _, name := range names {
		child := n.children[name]
		path := name
		if prefix != "" {
			path = prefix + " " + name
		}
		cmds = append(cmds, HelpCommand{
			Name:        path,
			Usage:       child.usage(),
			Description: child.description(),
		})
	}
	return cmds
}

func (n *node) usage() string       { return n.usageText }
func (n *node) description() string { return n.descriptionText }

func validateRunner(runner Runner) {
	if runner == nil {
		panic("argv: nil command runner")
	}
	if cmd, ok := runner.(*Command); ok && cmd.Run == nil {
		panic("argv: nil command handler")
	}
}

func (n *node) setRunner(runner Runner, usage, description string) {
	validateRunner(runner)
	n.runner = runner
	n.usageText = usage
	n.descriptionText = description
}

func (n *node) commandRunner() Runner { return n.runner }
func (n *node) hasRunner() bool       { return n.runner != nil }

// NewMux returns a new [Mux] named name. It panics if name is empty.
func NewMux(name string) *Mux {
	if name == "" {
		panic("argv: empty mux name")
	}
	return &Mux{Name: name}
}

// Flag declares a mux-level boolean flag parsed before subcommand
// routing. Parsed values accumulate in [Call.Flags]. short is an
// optional one-character short form; an empty short means the flag
// has no short form. It panics on duplicate or reserved names.
func (m *Mux) Flag(name, short string, value bool, usage string) {
	checkCrossCollision(name, short, m.options.hasName, m.options.hasShort)
	m.flags.add(name, short, value, usage)
}

// Option declares a mux-level named value option parsed before
// subcommand routing. Parsed values accumulate in [Call.Options].
// short is an optional one-character short form; an empty short means
// the option has no short form. It panics on duplicate or reserved
// names.
func (m *Mux) Option(name, short, value, usage string) {
	checkCrossCollision(name, short, m.flags.hasName, m.flags.hasShort)
	m.options.add(name, short, value, usage)
}

func (m *Mux) muxInputs() (*flagSpecs, *optionSpecs) {
	fs := &m.flags
	os := &m.options
	if len(fs.specs) == 0 {
		fs = nil
	}
	if len(os.specs) == 0 {
		os = nil
	}
	return fs, os
}

// Handle registers runner at pattern with a short usage summary.
// Pattern segments are split on whitespace; multi-segment patterns
// create nested command paths such as "repo init". An empty pattern
// registers a root handler invoked when no subcommand matches. A
// [*Mux] passed as runner becomes a mounted sub-mux at pattern. It
// panics on conflicting registrations or a nil runner.
func (m *Mux) Handle(pattern string, usage string, runner Runner) {
	n := &m.root
	for _, seg := range strings.Fields(pattern) {
		n = n.getOrCreate(seg)
	}
	if n.hasRunner() {
		panic("argv: command conflict at " + `"` + pattern + `"`)
	}
	var description string
	if h, ok := runner.(Helper); ok {
		description = h.HelpCLI().Description
	}
	n.setRunner(runner, usage, description)
}

// HandleFunc registers fn as the handler for pattern. It is shorthand
// for [Mux.Handle] with a [RunnerFunc].
func (m *Mux) HandleFunc(pattern string, usage string, fn func(*Output, *Call) error) {
	m.Handle(pattern, usage, RunnerFunc(fn))
}

// Match walks the command trie with tokens and returns the matched
// [Runner] and its full command path. Match does not interpret
// flag-like tokens; callers pass positional tokens.
//
// Match returns nil, "" when no Runner is reachable. With empty
// tokens, Match returns the root handler if one is registered at "".
// Match is analogous to [net/http.ServeMux.Handler].
func (m *Mux) Match(tokens []string) (Runner, string) {
	n := &m.root
	path := m.Name
	for _, tok := range tokens {
		child, ok := n.children[tok]
		if !ok {
			break
		}
		n = child
		path = joinedPath(path, tok)
	}
	if !n.hasRunner() {
		return nil, ""
	}
	return n.commandRunner(), path
}

// HelpCLI returns the mux's Description, subcommand list, and
// mux-level Flags and Options.
func (m *Mux) HelpCLI() Help {
	return Help{
		Description: m.Description,
		Commands:    m.root.usageCommands(""),
		Flags:       m.flags.helpEntriesNegatable(m.NegateFlags),
		Options:     m.options.helpEntries(),
	}
}

// WalkCLI yields (path, help) for the Mux and every command
// reachable from its trie. The Mux extends base.Flags and
// base.Options with its own flags and options before yielding its
// children.
func (m *Mux) WalkCLI(path string, base *Help) iter.Seq2[string, *Help] {
	return func(yield func(string, *Help) bool) {
		if base == nil {
			base = &Help{}
		}
		muxFlags, muxOptions := m.muxInputs()
		globalFlags, globalOptions := accumulateHelp(
			base.Flags, base.Options,
			muxFlags, muxOptions, m.NegateFlags,
		)

		help := &Help{
			Name:        lastPathSegment(path),
			FullPath:    path,
			Usage:       base.Usage,
			Description: firstNonEmpty(base.Description, m.Description),
			Commands:    m.root.usageCommands(""),
			Flags:       slices.Clone(globalFlags),
			Options:     slices.Clone(globalOptions),
		}
		if !yield(path, help) {
			return
		}
		walkChildren(&m.root, path, globalFlags, globalOptions, yield)
	}
}

// RunCLI routes call.Argv through the command trie and dispatches to
// the matched handler. It panics if call is nil.
func (m *Mux) RunCLI(out *Output, call *Call) error {
	if call == nil {
		panic("argv: nil call")
	}
	if call.Pattern == "" {
		call.Pattern = m.Name
	}
	base := call.Help
	if base == nil {
		base = &Help{}
	}
	description := firstNonEmpty(base.Description, m.Description)
	helpFunc := resolveHelpFunc(call.HelpFunc)
	muxFlags, muxOptions := m.muxInputs()
	globalFlags, globalOptions := accumulateHelp(base.Flags, base.Options, muxFlags, muxOptions, m.NegateFlags)

	parsed, err := parseInput(muxFlags, muxOptions, slices.Clone(call.Argv), m.NegateFlags)
	if err != nil {
		if errors.Is(err, errFlagHelp) {
			return m.renderNodeHelp(out, &m.root, call.Pattern, base.Usage, description, globalFlags, globalOptions, helpFunc, true)
		}
		return Errorf(ExitUsage, "%s: %w", call.Pattern, err)
	}

	newCall := enrichCall(call, parsed, muxFlags, muxOptions)
	return m.route(out, newCall, &m.root, &tokenCursor{tokens: parsed.args},
		call.Pattern, base.Usage, description, globalFlags, globalOptions, helpFunc)
}

// route descends the trie consuming positional tokens, then hands off
// to the matched runner (or renders help if none is found).
func (m *Mux) route(out *Output, call *Call, n *node, cur *tokenCursor,
	path, usage, description string,
	globalFlags []HelpFlag, globalOptions []HelpOption, helpFunc HelpFunc) error {

	for !cur.done() {
		child, ok := n.children[cur.peek()]
		if !ok {
			break
		}
		path = joinedPath(path, cur.next())
		n = child
	}

	if !n.hasRunner() {
		if !cur.done() && len(n.children) > 0 {
			fmt.Fprintf(out.Stderr, "unknown command %q\n\n", cur.peek())
		}
		return m.renderNodeHelp(out, n, path, usage, description, globalFlags, globalOptions, helpFunc, false)
	}

	// Hand off to the Runner. Stamp help context onto the call so
	// help-aware runners (Mux, Command, or external equivalents) can
	// render --help with ancestor-aware output. Plain runners that
	// don't parse --help themselves receive it as raw argv.
	h := n.commandRunner()
	handoff := &Call{
		ctx:     call.Context(),
		Pattern: path,
		Argv:    cur.rest(),
		Help: &Help{
			Usage:       n.usage(),
			Description: n.description(),
			Flags:       slices.Clone(globalFlags),
			Options:     slices.Clone(globalOptions),
		},
		HelpFunc: helpFunc,
		Stdin:    call.Stdin,
		Env:      call.Env,
		Flags:    call.Flags.Clone(),
		Options:  call.Options.Clone(),
		Args:     call.Args.Clone(),
		Rest:     slices.Clone(call.Rest),
	}
	return h.RunCLI(out, handoff)
}

// renderNodeHelp renders help for a node, respecting Mux-root overrides
// for usage and description. explicit reports whether help was asked
// for (returns nil) vs. shown in lieu of running (returns [ErrHelp]).
func (m *Mux) renderNodeHelp(out *Output, n *node, path, usage, description string,
	globalFlags []HelpFlag, globalOptions []HelpOption, helpFunc HelpFunc, explicit bool) error {

	usageText := n.usage()
	desc := n.description()
	if n == &m.root && (usage != "" || description != "") {
		usageText, desc = usage, description
	}

	help := Help{
		Name:        lastPathSegment(path),
		FullPath:    path,
		Usage:       usageText,
		Description: desc,
		Commands:    n.usageCommands(""),
		Flags:       slices.Clone(globalFlags),
		Options:     slices.Clone(globalOptions),
	}
	if err := helpFunc(out.Stderr, &help); err != nil {
		return err
	}
	if explicit {
		return nil
	}
	return ErrHelp
}

// accumulateHelp merges ancestor help entries with the current mux's
// flag and option entries, marking all as global.
func accumulateHelp(ancestorFlags []HelpFlag, ancestorOptions []HelpOption, fs *flagSpecs, os *optionSpecs, negateFlags bool) ([]HelpFlag, []HelpOption) {
	flags := slices.Concat(ancestorFlags, fs.helpEntriesNegatable(negateFlags))
	for i := range flags {
		flags[i].Global = true
	}
	options := slices.Concat(ancestorOptions, os.helpEntries())
	for i := range options {
		options[i].Global = true
	}
	return flags, options
}

// enrichCall returns a new Call that merges parsed flags and options
// from the current routing level and applies defaults from specs.
// Defaults are applied eagerly, so middleware that needs to distinguish
// caller-set values from defaults should use [FlagSet.Lookup] or
// [OptionSet.Lookup].
func enrichCall(call *Call, parsed *parsedInput, fs *flagSpecs, os *optionSpecs) *Call {
	flags := call.Flags.Clone()
	flags.merge(parsed.flags)
	if fs != nil {
		for _, spec := range fs.specs {
			flags.setDefault(spec.Name, spec.Default)
		}
	}

	options := call.Options.Clone()
	options.merge(parsed.options)
	if os != nil {
		for _, spec := range os.specs {
			options.setDefault(spec.Name, spec.Default)
		}
	}

	return &Call{
		ctx:      call.Context(),
		Pattern:  call.Pattern,
		Argv:     slices.Clone(parsed.args),
		Help:     call.Help,
		HelpFunc: call.HelpFunc,
		Stdin:    call.Stdin,
		Env:      call.Env,
		Flags:    flags,
		Options:  options,
		Args:     call.Args.Clone(),
		Rest:     slices.Clone(call.Rest),
		argNames: call.argNames,
	}
}

func joinedPath(base string, suffix string) string {
	if suffix == "" {
		return base
	}
	if base == "" {
		return suffix
	}
	return base + " " + suffix
}

func lastPathSegment(path string) string {
	if path == "" {
		return ""
	}
	parts := strings.Fields(path)
	return parts[len(parts)-1]
}

func resolveHelpFunc(help HelpFunc) HelpFunc {
	if help != nil {
		return help
	}
	return DefaultHelpFunc
}
