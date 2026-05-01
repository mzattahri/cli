package argv

import (
	"cmp"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
	"text/tabwriter"
)

// A HelpFunc renders help output to w for a resolved command path.
type HelpFunc func(w io.Writer, help *Help) error

// Help holds the data passed to a [HelpFunc] when rendering help
// output. Dispatchers own Name, FullPath, Summary, and Commands; Runners
// implementing [Helper] contribute Description, Flags, Options,
// Arguments, and Tail.
type Help struct {
	// Name is the final segment of the command path.
	Name string

	// FullPath is the complete command path (e.g. "app repo init").
	FullPath string

	// Summary is a short one-line summary.
	Summary string

	// Description is longer free-form help text.
	Description string

	// Flags lists boolean flags. Entries with Inherited set were declared
	// on a parent [Mux]; the rest were declared on the [Command].
	Flags []HelpFlag

	// Options lists value options. Entries with Inherited set were declared
	// on a parent [Mux]; the rest were declared on the [Command].
	Options []HelpOption

	// Commands lists immediate child commands. When Commands is
	// non-empty the node is a routing point and Arguments is empty.
	Commands []HelpCommand

	// Arguments lists positional arguments accepted by this command.
	// When a node has Commands, Arguments is empty.
	Arguments []HelpArg

	// Tail, if non-nil, describes a trailing variadic argument that
	// captures all positional tokens beyond those listed in Arguments
	// into [Call.Tail]. Name is rendered as "[<name>...]" in the
	// usage line; Usage carries the description shown in the
	// Arguments section. An empty Usage suppresses the Arguments row
	// but still names the tail in the usage line.
	Tail *HelpArg

	// Hidden marks the command as omitted from its parent's subcommand
	// listing and from completion candidates. The command remains
	// routable and renders --help when reached directly.
	Hidden bool

	// Annotations carry per-node metadata for renderers, walkers, and
	// other consumers. argv does not interpret the values.
	//
	// Use namespaced keys (e.g. "manpage/seealso") to avoid
	// collisions across packages. Annotations do not propagate to or
	// from ancestors.
	Annotations map[string]any
}

// A HelpFlag describes a boolean flag in help output.
type HelpFlag struct {
	// Name is the long flag name without the leading dashes (e.g. "verbose").
	Name string

	// Short is the one-character short name without the leading dash, or
	// empty if the flag has no short form.
	Short string

	// Usage is the one-line description shown alongside the flag. May
	// contain embedded newlines; the renderer aligns continuation lines.
	Usage string

	// Default is the value the flag carries when not supplied by the user.
	Default bool

	// Negatable reports whether the flag may be negated with the --no-
	// prefix at the same level it was declared. The renderer uses it to
	// emit the "--[no-]name" form, and completion offers the negated
	// variant as a candidate.
	Negatable bool

	// Inherited reports whether the flag was declared on a parent [Mux]
	// rather than locally on this [Command] or [Mux]. The renderer
	// surfaces inherited entries in a separate "Global Options" section,
	// and completion uses the bit to scope candidate emission to the
	// current level.
	Inherited bool
}

// A HelpOption describes a value option in help output.
type HelpOption struct {
	// Name is the long option name without the leading dashes (e.g. "host").
	Name string

	// Short is the one-character short name without the leading dash, or
	// empty if the option has no short form.
	Short string

	// Usage is the one-line description shown alongside the option. May
	// contain embedded newlines; the renderer aligns continuation lines.
	Usage string

	// Default is the value the option carries when not supplied by the user.
	Default string

	// Inherited reports whether the option was declared on a parent [Mux].
	// See [HelpFlag.Inherited] for renderer and completion semantics.
	Inherited bool
}

// A HelpCommand describes a subcommand in help output.
type HelpCommand struct {
	// Name is the subcommand path relative to the parent (e.g. "init"
	// or "repo init" for a multi-segment registration).
	Name string

	// Summary is the one-line summary shown in the parent's command
	// listing.
	Summary string

	// Description is the longer free-form text shown when help is
	// requested for the subcommand directly.
	Description string
}

// A HelpArg describes a positional argument in help output.
type HelpArg struct {
	// Name is the rendered argument label, including any decoration the
	// caller embeds (e.g. "<path>" or "[<files>...]").
	Name string

	// Usage is the one-line description shown alongside the argument.
	Usage string
}

// InheritedFlags returns an iterator over entries in h.Flags where
// Inherited is true.
func (h *Help) InheritedFlags() iter.Seq[HelpFlag] {
	return filterHelpFlags(h.Flags, true)
}

// LocalFlags returns an iterator over entries in h.Flags where
// Inherited is false.
func (h *Help) LocalFlags() iter.Seq[HelpFlag] {
	return filterHelpFlags(h.Flags, false)
}

// InheritedOptions returns an iterator over entries in h.Options where
// Inherited is true.
func (h *Help) InheritedOptions() iter.Seq[HelpOption] {
	return filterHelpOptions(h.Options, true)
}

// LocalOptions returns an iterator over entries in h.Options where
// Inherited is false.
func (h *Help) LocalOptions() iter.Seq[HelpOption] {
	return filterHelpOptions(h.Options, false)
}

// PositionalIndex reports the index of the positional argument the
// next non-flag token would fill, given the tokens already typed at
// this command level. Flag tokens are skipped, and options declared
// in h consume their value tokens. PositionalIndex returns -1 when
// "--" appears in completed, since tokens past "--" are literal and
// not subject to completion.
//
// Custom [Completer] implementations use PositionalIndex to dispatch
// dynamic value suggestions for a specific positional argument:
//
//	if help.PositionalIndex(completed) == 0 {
//		return suggestFiles(w, dir, partial)
//	}
//	return help.CompleteArgv(w, completed, partial)
func (h *Help) PositionalIndex(completed []string) int {
	return positionalIndex(completed, h.Options)
}

func positionalIndex(completed []string, options []HelpOption) int {
	if slices.Contains(completed, "--") {
		return -1
	}
	pos := 0
	skipNext := false
	for _, tok := range completed {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(tok, "-") {
			if isValueOption(tok, options) {
				skipNext = true
			}
			continue
		}
		pos++
	}
	return pos
}

// CompleteArgv emits default completion candidates derived from h:
// subcommand names, flag and option tokens (with short forms and
// negation variants), and argument placeholders. Flag and option
// emission is scoped to entries declared at this level (Inherited=false);
// globals from ancestor levels are visible for flag-skip detection
// but not emitted as candidates. Output is suppressed at option-value
// position (where values aren't knowable from Help) and after "--".
//
// *Help implements [Completer] through this method: a [Runner] that
// wants custom completion implements [Completer] itself and delegates
// to help.CompleteArgv for the non-custom cases:
//
//	func (c *MyCmd) CompleteArgv(w *argv.TokenWriter, completed []string, partial string) error {
//		if ... { return c.emitCustomValues(w, partial) }
//		var help argv.Help
//		c.HelpArgv(&help)
//		return help.CompleteArgv(w, completed, partial)
//	}
func (h *Help) CompleteArgv(w *TokenWriter, completed []string, partial string) error {
	if slices.Contains(completed, "--") {
		return nil
	}
	if len(completed) > 0 && isValueOption(completed[len(completed)-1], h.Options) {
		return nil
	}
	if isPartialOptionValue(partial, h.Flags, h.Options) {
		return nil
	}
	if strings.HasPrefix(partial, "-") {
		return writeFlagEntries(w, slices.Collect(h.LocalFlags()), slices.Collect(h.LocalOptions()), partial)
	}
	if len(h.Commands) > 0 {
		return writeSubcommands(w, h.Commands, partial)
	}
	return writeArgHint(w, h.Arguments, completed, h.Options)
}

func filterHelpFlags(flags []HelpFlag, global bool) iter.Seq[HelpFlag] {
	return func(yield func(HelpFlag) bool) {
		for _, f := range flags {
			if f.Inherited != global {
				continue
			}
			if !yield(f) {
				return
			}
		}
	}
}

func filterHelpOptions(options []HelpOption, global bool) iter.Seq[HelpOption] {
	return func(yield func(HelpOption) bool) {
		for _, o := range options {
			if o.Inherited != global {
				continue
			}
			if !yield(o) {
				return
			}
		}
	}
}

// DefaultHelpFunc is the built-in [HelpFunc] used when no override is set.
// It renders a tabular summary to w.
func DefaultHelpFunc(w io.Writer, help *Help) error {
	if help == nil {
		panic("argv: nil help")
	}
	commands := slices.Clone(help.Commands)
	slices.SortFunc(commands, func(a, b HelpCommand) int {
		return cmp.Compare(a.Name, b.Name)
	})
	if help.Summary != "" {
		if _, err := fmt.Fprintf(w, "%s - %s\n", help.FullPath, help.Summary); err != nil {
			return err
		}
	}
	if help.Description != "" {
		if _, err := fmt.Fprintf(w, "\n%s\n", help.Description); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w, "\nUsage:\n"); err != nil {
		return err
	}

	var line strings.Builder
	line.WriteString("  ")
	line.WriteString(help.FullPath)
	if len(commands) > 0 {
		line.WriteString(" [command]")
	}
	if len(help.Flags) > 0 || len(help.Options) > 0 {
		line.WriteString(" [options]")
	}
	for _, arg := range help.Arguments {
		line.WriteString(" ")
		line.WriteString(arg.Name)
	}
	if help.Tail != nil {
		line.WriteString(" ")
		line.WriteString(help.Tail.Name)
	}
	line.WriteString("\n")
	if _, err := io.WriteString(w, line.String()); err != nil {
		return err
	}

	if err := renderInputSection(w, "Options", help.LocalFlags(), help.LocalOptions()); err != nil {
		return err
	}
	if err := renderInputSection(w, "Global Options", help.InheritedFlags(), help.InheritedOptions()); err != nil {
		return err
	}

	showTailRow := help.Tail != nil && help.Tail.Usage != ""
	if len(help.Arguments) > 0 || showTailRow {
		if _, err := io.WriteString(w, "\nArguments:\n"); err != nil {
			return err
		}
		rows := make([]helpRow, 0, len(help.Arguments)+1)
		for _, argument := range help.Arguments {
			rows = append(rows, helpRow(argument))
		}
		if showTailRow {
			rows = append(rows, helpRow{Name: help.Tail.Name, Usage: help.Tail.Usage})
		}
		if err := renderHelpTable(w, rows); err != nil {
			return err
		}
	}

	if len(commands) > 0 {
		if _, err := io.WriteString(w, "\nCommands:\n"); err != nil {
			return err
		}
		rows := make([]helpRow, 0, len(commands))
		for _, cmd := range commands {
			rows = append(rows, helpRow{Name: cmd.Name, Usage: cmd.Summary})
		}
		if err := renderHelpTable(w, rows); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Use `%s [command] --help` for more information.\n", help.FullPath); err != nil {
			return err
		}
	}
	return nil
}

func renderInputSection(w io.Writer, title string, flags iter.Seq[HelpFlag], options iter.Seq[HelpOption]) error {
	var rows []helpRow
	for f := range flags {
		usage := f.Usage
		if f.Default {
			usage += " (default: true)"
		}
		rows = append(rows, helpRow{Name: formatFlagName(f.Name, f.Short, f.Negatable), Usage: usage})
	}
	for o := range options {
		usage := o.Usage
		if o.Default != "" {
			usage += fmt.Sprintf(" (default: %s)", o.Default)
		}
		rows = append(rows, helpRow{Name: formatOptionName(o.Name, o.Short), Usage: usage})
	}
	if len(rows) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\n%s:\n", title); err != nil {
		return err
	}
	return renderHelpTable(w, rows)
}

type helpRow struct {
	Name  string
	Usage string
}

func renderHelpTable(w io.Writer, rows []helpRow) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, row := range rows {
		lines := strings.Split(row.Usage, "\n")
		if _, err := fmt.Fprintf(tw, "  %s\t%s\n", row.Name, lines[0]); err != nil {
			return err
		}
		for _, line := range lines[1:] {
			if _, err := fmt.Fprintf(tw, "  \t%s\n", line); err != nil {
				return err
			}
		}
	}
	return tw.Flush()
}

func formatFlagName(name, short string, negatable bool) string {
	var b strings.Builder
	if short != "" {
		b.WriteString("-")
		b.WriteString(short)
		b.WriteString(", ")
	}
	if negatable {
		base, _ := strings.CutPrefix(name, "no-")
		b.WriteString("--[no-]")
		b.WriteString(base)
	} else {
		b.WriteString("--")
		b.WriteString(name)
	}
	return b.String()
}

func formatOptionName(name, short string) string {
	var b strings.Builder
	if short != "" {
		b.WriteString("-")
		b.WriteString(short)
		b.WriteString(", ")
	}
	b.WriteString("--")
	b.WriteString(name)
	b.WriteString(" <")
	b.WriteString(name)
	b.WriteString(">")
	return b.String()
}
