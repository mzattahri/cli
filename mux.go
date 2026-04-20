package cli

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// A Mux is a command multiplexer. It matches argv tokens against
// registered command names and dispatches to the corresponding [Runner].
type Mux struct {
	// Name is the mux identifier used in help output and command paths.
	Name string

	// NegateFlags enables --no- prefix negation for mux-level boolean
	// flags. When true, --no-flagname sets a flag to false. If a flag
	// is declared with a "no-" prefix, the bare form (--flagname)
	// also sets it to false. See [Command.NegateFlags].
	NegateFlags bool

	root    node
	flags   flagSpecs
	options optionSpecs
}

type ancestorHelp struct {
	flags   []HelpFlag
	options []HelpOption
}

// node is an internal trie node for command routing.
type node struct {
	segment         string
	parent          *node
	command         *Command
	runner          Runner
	usageText       string
	descriptionText string
	children        map[string]*node
}

type nodeChild struct {
	name        string
	path        string
	usage       string
	description string
	node        *node
}

func (n *node) getOrCreate(name string) *node {
	if n.children == nil {
		n.children = map[string]*node{}
	}
	child, ok := n.children[name]
	if !ok {
		child = &node{segment: name, parent: n}
		n.children[name] = child
	}
	return child
}

func (n *node) childInfos(prefix string) []nodeChild {
	names := slices.Sorted(maps.Keys(n.children))
	children := make([]nodeChild, 0, len(names))
	for _, name := range names {
		child := n.children[name]
		path := name
		if prefix != "" {
			path = prefix + " " + name
		}
		children = append(children, nodeChild{
			name:        name,
			path:        path,
			usage:       child.usage(),
			description: child.description(),
			node:        child,
		})
	}
	return children
}

func (n *node) usageCommands(prefix string) []HelpCommand {
	children := n.childInfos(prefix)
	cmds := make([]HelpCommand, 0, len(children))
	for _, child := range children {
		cmds = append(cmds, HelpCommand{
			Name:        child.path,
			Usage:       child.usage,
			Description: child.description,
		})
	}
	return cmds
}

func (n *node) path() string {
	if n == nil {
		return ""
	}
	var segments []string
	for cur := n; cur != nil; cur = cur.parent {
		if cur.segment != "" {
			segments = append(segments, cur.segment)
		}
	}
	slices.Reverse(segments)
	return strings.Join(segments, " ")
}

func (n *node) usage() string       { return n.usageText }
func (n *node) description() string { return n.descriptionText }

func validateRunner(runner Runner) {
	if runner == nil {
		panic("cli: nil command runner")
	}
	if cmd, ok := runner.(*Command); ok && cmd.Run == nil {
		panic("cli: nil command handler")
	}
}

func (n *node) setCommand(cmd *Command, runner Runner, usage, description string) {
	validateRunner(runner)
	n.command = cmd
	n.runner = runner
	n.usageText = usage
	n.descriptionText = description
}

func (n *node) commandRunner() Runner { return n.runner }
func (n *node) hasRunner() bool       { return n.runner != nil }

// NewMux returns a new [Mux] with the given program name.
// It panics if name is empty.
func NewMux(name string) *Mux {
	if name == "" {
		panic("cli: empty mux name")
	}
	return &Mux{Name: name}
}

// Flag declares a mux-level boolean flag that is parsed before subcommand
// routing. Parsed values accumulate in [Call.Flags].
//
// short is an optional one-character short form (e.g. "v" for -v).
// An empty string means the flag has no short form.
// It panics on duplicate or reserved names.
func (m *Mux) Flag(name, short string, value bool, usage string) {
	checkCrossCollision(name, short, m.options.hasName, m.options.hasShort)
	m.flags.add(name, short, value, usage)
}

// Option declares a mux-level named value option that is parsed before
// subcommand routing. Parsed values accumulate in [Call.Options].
//
// short is an optional one-character short form (e.g. "c" for -c).
// An empty string means the option has no short form.
// It panics on duplicate or reserved names.
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

// Handle registers runner for the given command pattern with a short usage
// summary shown in help output.
//
// Pattern segments are split on whitespace. Multi-segment patterns create
// nested command paths (e.g. "repo init"). An empty pattern registers a
// root handler invoked when no subcommand matches. If runner is a [*Mux],
// it is mounted as a sub-mux at pattern. It panics on conflicting
// registrations or a nil runner.
func (m *Mux) Handle(pattern string, usage string, runner Runner) {
	if sub, ok := runner.(*Mux); ok {
		m.mount(pattern, usage, sub)
		return
	}
	cmd, _ := runner.(*Command)
	var description string
	if cmd != nil {
		description = cmd.Description
	}
	n := &m.root
	for _, seg := range strings.Fields(pattern) {
		n = n.getOrCreate(seg)
	}
	if n.hasRunner() {
		panic("cli: command conflict at " + `"` + pattern + `"`)
	}
	n.setCommand(cmd, runner, usage, description)
}

// HandleFunc registers fn as the handler for pattern.
// It is a shorthand for Handle(pattern, usage, [RunnerFunc](fn)).
func (m *Mux) HandleFunc(pattern string, usage string, fn func(*Output, *Call) error) {
	m.Handle(pattern, usage, RunnerFunc(fn))
}

func (m *Mux) mount(prefix string, usage string, sub *Mux) {
	if sub == nil {
		panic("cli: nil mount mux")
	}
	n := &m.root
	for _, seg := range strings.Fields(prefix) {
		n = n.getOrCreate(seg)
	}
	if n.hasRunner() {
		panic("cli: mount conflict at " + `"` + prefix + `"`)
	}
	n.setCommand(nil, sub, usage, "")
}

// RunCLI routes the call's arguments through the command trie and
// dispatches to the matched handler. It panics if call is nil.
func (m *Mux) RunCLI(out *Output, call *Call) error {
	if call == nil {
		panic("cli: nil call")
	}
	return m.runWithPath(out, call, m.Name, "", "", nil, DefaultHelpFunc)
}

func (m *Mux) runWithPath(out *Output, call *Call, fullPath string, usage string, description string, ancestors *ancestorHelp, helpRenderer HelpFunc) error {
	if ancestors == nil {
		ancestors = &ancestorHelp{}
	}
	helpRenderer = resolveHelpFunc(helpRenderer)
	muxFlags, muxOptions := m.muxInputs()
	accFlags, accOptions := accumulateHelp(ancestors, muxFlags, muxOptions, m.NegateFlags)

	parsed, err := parseInput(muxFlags, muxOptions, slices.Clone(getState(call.ctx).argv), m.NegateFlags)
	if err != nil {
		if errors.Is(err, errFlagHelp) {
			d := &dispatch{
				out:           out,
				call:          call,
				mux:           m,
				path:          fullPath,
				usage:         usage,
				description:   description,
				globalFlags:   accFlags,
				globalOptions: accOptions,
				helpFunc:      helpRenderer,
			}
			return d.renderHelp(helpCall{node: &m.root, explicit: true})
		}
		return fmt.Errorf("%s: %w", fullPath, err)
	}

	newCall := enrichCall(call, parsed, muxFlags, muxOptions)

	d := &dispatch{
		out:           out,
		call:          newCall,
		mux:           m,
		path:          fullPath,
		usage:         usage,
		description:   description,
		globalFlags:   accFlags,
		globalOptions: accOptions,
		helpFunc:      helpRenderer,
	}

	return d.route(&m.root, &tokenCursor{tokens: parsed.args})
}

// accumulateHelp merges ancestor help entries with the current mux's
// flag and option entries, marking all as global.
func accumulateHelp(ancestors *ancestorHelp, fs *flagSpecs, os *optionSpecs, negateFlags bool) ([]HelpFlag, []HelpOption) {
	flags := append(append([]HelpFlag(nil), ancestors.flags...), fs.helpEntriesNegatable(negateFlags)...)
	for i := range flags {
		flags[i].Global = true
	}
	options := append(append([]HelpOption(nil), ancestors.options...), os.helpEntries()...)
	for i := range options {
		options[i].Global = true
	}
	return flags, options
}

// enrichCall returns a new Call that merges parsed flags and options
// from the current routing level and applies defaults from specs.
// Defaults are applied eagerly with explicit=false, so middleware
// that needs to distinguish user input from defaults should check
// [FlagSet.Explicit] or [OptionSet.Explicit].
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

	s := getState(call.ctx)
	if s == nil {
		s = &callState{}
	}
	ns := &callState{argv: slices.Clone(parsed.args), argNames: s.argNames}

	return &Call{
		ctx:     setState(call.Context(), ns),
		Stdin:   call.Stdin,
		Env:     call.Env,
		Flags:   flags,
		Options: options,
		Args:    call.Args.Clone(),
		Rest:    slices.Clone(call.Rest),
	}
}

// dispatch bundles the state threaded through command routing.
type dispatch struct {
	out           *Output
	call          *Call
	mux           *Mux
	path          string
	usage         string
	description   string
	globalFlags   []HelpFlag
	globalOptions []HelpOption
	helpFunc      HelpFunc
}

func (d *dispatch) route(n *node, cur *tokenCursor) error {
	if !cur.done() {
		if child, ok := n.children[cur.peek()]; ok {
			token := cur.next()
			d.path = joinedPath(d.path, token)
			d.usage = ""
			d.description = ""
			return d.route(child, cur)
		}
	}

	if !n.hasRunner() {
		if !cur.done() && len(n.children) > 0 {
			fmt.Fprintf(d.out.Stderr, "unknown command %q\n\n", cur.peek())
		}
		return d.renderHelp(helpCall{node: n})
	}

	h := n.commandRunner()

	if sub, ok := h.(*Mux); ok {
		// Carry forward accumulated state; update argv for the sub-mux.
		ns := &callState{argv: slices.Clone(cur.rest())}
		mountCall := &Call{
			ctx:     setState(d.call.Context(), ns),
			Stdin:   d.call.Stdin,
			Env:     d.call.Env,
			Flags:   d.call.Flags.Clone(),
			Options: d.call.Options.Clone(),
			Args:    d.call.Args.Clone(),
			Rest:    slices.Clone(d.call.Rest),
		}
		return sub.runWithPath(d.out, mountCall, d.path, n.usage(), n.description(), &ancestorHelp{
			flags:   d.globalFlags,
			options: d.globalOptions,
		}, d.helpFunc)
	}

	cmd := n.command
	fs, os, as := commandInputs(cmd)
	captureRest := commandCaptureRest(cmd)
	negateFlags := commandNegateFlags(cmd)
	return d.runCommand(n, h, fs, os, as, captureRest, negateFlags, cur.rest())
}

func (d *dispatch) runCommand(n *node, h Runner, fs *flagSpecs, os *optionSpecs, as *argSpecs, captureRest bool, negateFlags bool, rest []string) error {
	parsed, err := parseInput(fs, os, rest, negateFlags)
	if err != nil {
		if errors.Is(err, errFlagHelp) {
			return d.renderHelp(helpCall{node: n, flags: fs, options: os, args: as, explicit: true, negateFlags: negateFlags})
		}
		return fmt.Errorf("%s: %w", d.path, err)
	}

	argState := ArgSet{}
	var restState []string
	if as != nil {
		argState, restState, err = as.parse(parsed.args, captureRest)
		if err != nil {
			return fmt.Errorf("%s: %w", d.path, err)
		}
	} else if captureRest {
		restState = slices.Clone(parsed.args)
	} else if len(parsed.args) > 0 {
		return fmt.Errorf("%s: unexpected argument %q", d.path, parsed.args[0])
	}

	runCall := enrichCall(d.call, parsed, fs, os)
	runCall.Pattern = d.path
	runCall.Args = argState
	runCall.Rest = restState
	getState(runCall.ctx).argNames = as.names()
	return h.RunCLI(d.out, runCall)
}

type helpCall struct {
	node        *node
	flags       *flagSpecs
	options     *optionSpecs
	args        *argSpecs
	explicit    bool
	negateFlags bool
}

func (d *dispatch) renderHelp(h helpCall) error {
	n := h.node
	fullPath := n.path()
	if d.path != "" {
		fullPath = d.path
	}
	usageText := n.usage()
	desc := n.description()
	if n == &d.mux.root && (d.usage != "" || d.description != "") {
		usageText, desc = d.usage, d.description
	}

	name := n.segment
	if name == "" {
		name = lastPathSegment(fullPath)
	}
	flags := append(slices.Clone(d.globalFlags), h.flags.helpEntriesNegatable(h.negateFlags)...)
	options := append(slices.Clone(d.globalOptions), h.options.helpEntries()...)
	help := Help{
		Name:        name,
		FullPath:    fullPath,
		Usage:       usageText,
		Description: desc,
		Commands:    n.usageCommands(""),
		Flags:       flags,
		Options:     options,
	}
	if h.args != nil {
		help.Arguments = h.args.HelpArguments()
	}
	if n.command != nil {
		help.CaptureRest = n.command.CaptureRest
	}
	if err := d.helpFunc(d.out.Stderr, &help); err != nil {
		return err
	}
	if h.explicit {
		return nil
	}
	return ErrHelp
}

func joinedPath(base string, suffix string) string {
	if suffix == "" {
		return base
	}
	if base == "" {
		return suffix
	}
	return strings.TrimSpace(base + " " + suffix)
}

func lastPathSegment(path string) string {
	if path == "" {
		return ""
	}
	parts := strings.Fields(path)
	return parts[len(parts)-1]
}

func commandInputs(cmd *Command) (*flagSpecs, *optionSpecs, *argSpecs) {
	if cmd != nil {
		return cmd.inputs()
	}
	return nil, nil, nil
}

func commandCaptureRest(cmd *Command) bool {
	return cmd != nil && cmd.CaptureRest
}

func commandNegateFlags(cmd *Command) bool {
	return cmd != nil && cmd.NegateFlags
}

func resolveHelpFunc(help HelpFunc) HelpFunc {
	if help != nil {
		return help
	}
	return DefaultHelpFunc
}
