package argv

import (
	"cmp"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"
)

// A Mux is a command multiplexer. It matches argv tokens against
// registered command names and dispatches to the corresponding
// [Runner].
//
// The zero value is ready for use.
type Mux struct {
	// Description is the longer help text rendered by [HelpFunc]
	// before the subcommand list.
	Description string

	// Hidden omits the mux from its parent's subcommand listing and
	// from completion candidates. The mux remains routable: an
	// explicit invocation routes through normally, and --help still
	// renders.
	Hidden bool

	// NegateFlags enables --no- prefix negation for the boolean flags
	// declared on this Mux. See [Command.NegateFlags] for parsing and
	// per-level semantics.
	NegateFlags bool

	// Annotations carry per-node metadata copied into
	// [Help.Annotations] by [Mux.HelpArgv].
	//
	// argv does not interpret the values. Use namespaced keys
	// (e.g. "manpage/seealso") to avoid collisions across packages.
	// Annotations do not propagate to descendants.
	Annotations map[string]any

	root    node
	flags   flagSpecs
	options optionSpecs
}

// node is an internal trie node for command routing.
type node struct {
	runner    Runner
	usageText string
	children  map[string]*node
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
		if child.hidden() {
			continue
		}
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

func (n *node) usage() string { return n.usageText }

// description reads the runner's [Helper] description live, so dynamic
// Helpers reflect their current state in subcommand listings.
func (n *node) description() string {
	if h, ok := n.runner.(Helper); ok {
		var tmp Help
		h.HelpArgv(&tmp)
		return tmp.Description
	}
	return ""
}

// hidden reports whether the runner declares itself hidden via
// [Helper]. It reads the bit live so dynamic Helpers can toggle
// visibility.
func (n *node) hidden() bool {
	if h, ok := n.runner.(Helper); ok {
		var tmp Help
		h.HelpArgv(&tmp)
		return tmp.Hidden
	}
	return false
}

func validateRunner(runner Runner) {
	if runner == nil {
		panic("argv: nil command runner")
	}
	if cmd, ok := runner.(*Command); ok && cmd.Run == nil {
		panic("argv: nil command handler")
	}
}

func (n *node) setRunner(runner Runner, usage string) {
	validateRunner(runner)
	n.runner = runner
	n.usageText = usage
}

func (n *node) commandRunner() Runner { return n.runner }
func (n *node) hasRunner() bool       { return n.runner != nil }

// Flag declares a mux-level boolean flag parsed before subcommand
// routing. Parsed values accumulate in [Call.Flags]. short is an
// optional one-character short form. It panics on duplicate names
// or on a name already declared locally by a descendant runner.
func (m *Mux) Flag(name, short string, value bool, usage string) {
	checkCrossCollision(name, short, m.options.hasName, m.options.hasShort)
	m.checkDescendantShadow(name)
	m.flags.add(name, short, value, usage)
}

// Option declares a mux-level named value option parsed before
// subcommand routing. Parsed values accumulate in [Call.Options].
// short is an optional one-character short form. It panics on
// duplicate names or on a name already declared locally by a
// descendant runner.
func (m *Mux) Option(name, short, value, usage string) {
	checkCrossCollision(name, short, m.flags.hasName, m.flags.hasShort)
	m.checkDescendantShadow(name)
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
// create nested command paths such as "repo init".
//
// usage is the one-line shown in parent command listings; the
// runner's [Helper]-supplied Description carries longer prose.
//
// An empty pattern registers a root handler invoked when no
// subcommand matches. A [*Mux] passed as runner becomes a mounted
// sub-mux at pattern.
//
// It panics on conflicting registrations, a nil runner, or a local
// flag or option in runner's subtree whose name collides with one
// declared on this mux.
func (m *Mux) Handle(pattern string, usage string, runner Runner) {
	n := &m.root
	for _, seg := range strings.Fields(pattern) {
		n = n.getOrCreate(seg)
	}
	if n.hasRunner() {
		panic(fmt.Sprintf("argv: command conflict at %q", pattern))
	}
	m.checkRunnerShadow(pattern, runner)
	n.setRunner(runner, usage)
}

func (m *Mux) checkDescendantShadow(name string) {
	first := true
	for help := range m.WalkArgv("", nil) {
		if first {
			first = false
			continue
		}
		for _, f := range help.Flags {
			if !f.Inherited && f.Name == name {
				panic(fmt.Sprintf("argv: mux input %q shadows local input at %q", name, help.FullPath))
			}
		}
		for _, o := range help.Options {
			if !o.Inherited && o.Name == name {
				panic(fmt.Sprintf("argv: mux input %q shadows local input at %q", name, help.FullPath))
			}
		}
	}
}

func (m *Mux) checkRunnerShadow(pattern string, runner Runner) {
	assert := func(path, name string) {
		if m.flags.hasName(name) || m.options.hasName(name) {
			panic(fmt.Sprintf("argv: input %q at %q shadowed by mux input", name, path))
		}
	}
	emit := func(path string, flags []HelpFlag, options []HelpOption) {
		for _, f := range flags {
			if !f.Inherited {
				assert(path, f.Name)
			}
		}
		for _, o := range options {
			if !o.Inherited {
				assert(path, o.Name)
			}
		}
	}
	if w, ok := runner.(Walker); ok {
		for help := range w.WalkArgv(pattern, nil) {
			emit(help.FullPath, help.Flags, help.Options)
		}
		return
	}
	if h, ok := runner.(Helper); ok {
		var help Help
		h.HelpArgv(&help)
		emit(pattern, help.Flags, help.Options)
	}
}

// Match walks the command trie with tokens and returns the matched
// [Runner] and the matched command path (the tokens consumed during
// the walk, joined by spaces). Match does not interpret flag-like
// tokens; callers pass positional tokens.
//
// Match returns nil, "" when no Runner is reachable. With empty
// tokens, Match returns the root handler (path "") if one is
// registered.
func (m *Mux) Match(tokens []string) (Runner, string) {
	n := &m.root
	var path string
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

// HelpArgv contributes the mux's Description, mux-level Flags and
// Options, Hidden state, and subcommand list to h.
//
// Flags and Options are appended so ancestor globals set by the
// dispatcher are preserved. Commands is only filled when empty, so
// dispatcher pre-population wins.
func (m *Mux) HelpArgv(h *Help) {
	if h.Description == "" {
		h.Description = m.Description
	}
	h.Flags = append(h.Flags, m.flags.helpEntriesNegatable(m.NegateFlags)...)
	h.Options = append(h.Options, m.options.helpEntries()...)
	if len(h.Commands) == 0 {
		h.Commands = m.root.usageCommands("")
	}
	h.Hidden = m.Hidden
	h.Annotations = maps.Clone(m.Annotations)
}

// WalkArgv yields (help, runner) for the Mux and every command
// reachable from its trie. The Mux extends base.Flags and
// base.Options with its own flags and options before yielding its
// children.
func (m *Mux) WalkArgv(path string, base *Help) iter.Seq2[*Help, Runner] {
	return func(yield func(*Help, Runner) bool) {
		if base == nil {
			base = &Help{}
		}
		muxFlags, muxOptions := m.muxInputs()
		ownFlags := muxFlags.helpEntriesNegatable(m.NegateFlags)
		ownOptions := muxOptions.helpEntries()

		// Help at this level: ancestors (already Inherited=true) + own (Inherited=false).
		help := &Help{
			Name:        lastPathSegment(path),
			FullPath:    path,
			Usage:       base.Usage,
			Description: cmp.Or(base.Description, m.Description),
			Commands:    m.root.usageCommands(""),
			Flags:       slices.Concat(base.Flags, ownFlags),
			Options:     slices.Concat(base.Options, ownOptions),
			Hidden:      m.Hidden,
			Annotations: maps.Clone(m.Annotations),
		}
		if !yield(help, m) {
			return
		}

		// Children see everything visible here as globals.
		inheritedFlags, inheritedOptions := accumulateHelp(base.Flags, base.Options, muxFlags, muxOptions, m.NegateFlags)
		walkChildren(&m.root, path, inheritedFlags, inheritedOptions, yield)
	}
}

// RunArgv routes the call's argv through the command trie and
// dispatches to the matched handler. --help or -h at the mux level
// returns a [*HelpError]; unknown or partial paths likewise return a
// HelpError so [Program.Invoke] can render help. It panics if call
// is nil.
func (m *Mux) RunArgv(out *Output, call *Call) error {
	if call == nil {
		panic("argv: nil call")
	}
	pattern := call.pattern
	muxFlags, muxOptions := m.muxInputs()

	parsed, err := parseInput(muxFlags, muxOptions, call.argv, m.NegateFlags)
	if err != nil {
		if errors.Is(err, errFlagHelp) {
			return &HelpError{Path: pattern, Explicit: true}
		}
		return Errorf(ExitUsage, "%s: %w", pattern, err)
	}

	applyParse(call, parsed, muxFlags, muxOptions)
	return m.route(out, call, &m.root, parsed.args, pattern)
}

func (m *Mux) route(out *Output, call *Call, n *node, tokens []string, path string) error {
	pos := 0
	for pos < len(tokens) {
		child, ok := n.children[tokens[pos]]
		if !ok {
			break
		}
		path = joinedPath(path, tokens[pos])
		pos++
		n = child
	}

	if !n.hasRunner() {
		he := &HelpError{Path: path, Explicit: false}
		if pos < len(tokens) && len(n.children) > 0 {
			he.Reason = fmt.Sprintf("unknown command %q", tokens[pos])
		}
		return he
	}

	return n.commandRunner().RunArgv(out, call.WithArgv(path, tokens[pos:]))
}

// accumulateHelp merges ancestor help entries with the current mux's
// flag and option entries, marking all as global.
func accumulateHelp(ancestorFlags []HelpFlag, ancestorOptions []HelpOption, fs *flagSpecs, os *optionSpecs, negateFlags bool) ([]HelpFlag, []HelpOption) {
	flags := slices.Concat(ancestorFlags, fs.helpEntriesNegatable(negateFlags))
	for i := range flags {
		flags[i].Inherited = true
	}
	options := slices.Concat(ancestorOptions, os.helpEntries())
	for i := range options {
		options[i].Inherited = true
	}
	return flags, options
}

func applyParse(call *Call, parsed parsedInput, fs *flagSpecs, os *optionSpecs) {
	call.argv = parsed.args
	call.Flags.merge(parsed.flags)
	if fs != nil {
		for _, spec := range fs.specs {
			call.Flags.setDefault(spec.Name, spec.Default)
		}
	}
	call.Options.merge(parsed.options)
	if os != nil {
		for _, spec := range os.specs {
			call.Options.setDefault(spec.Name, spec.Default)
		}
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
	if i := strings.LastIndex(path, " "); i >= 0 {
		return path[i+1:]
	}
	return path
}
