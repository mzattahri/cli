package argv

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

// A TokenWriter provides methods for writing tab-separated completion entries.
type TokenWriter struct {
	io.Writer
}

// WriteToken writes a completion candidate with an optional description.
// It returns the number of bytes written and any write error.
func (w *TokenWriter) WriteToken(value, desc string) (int, error) {
	if desc != "" {
		return fmt.Fprintf(w.Writer, "%s\t%s\n", value, desc)
	}
	return fmt.Fprintln(w.Writer, value)
}

// A Completer writes tab completions for a partial command line to w.
//
// completed holds the tokens before the cursor that have already been
// fully typed. partial is the token currently being typed (it may be
// empty). [*Mux] and [*Command] both implement Completer. A Mux
// completes subcommands and mux-level flags; a Command completes
// command-level flags and options.
type Completer interface {
	CompleteCLI(w *TokenWriter, completed []string, partial string) error
}

// CompleterFunc adapts a plain function to the [Completer] interface.
type CompleterFunc func(w *TokenWriter, completed []string, partial string) error

// CompleteCLI calls f(w, completed, partial).
func (f CompleterFunc) CompleteCLI(w *TokenWriter, completed []string, partial string) error {
	return f(w, completed, partial)
}

// CompletionRunner returns a [Runner] that outputs tab completions for the
// current command line.
//
// Shell integration scripts invoke this runner on each TAB press, passing
// the current tokens (shell tokens) as positional arguments after "--":
//
//	myapp complete -- repo init --f
//
// It panics if c is nil.
func CompletionRunner(c Completer) Runner {
	if c == nil {
		panic("argv: nil completer")
	}
	return &Command{
		CaptureRest: true,
		Run: func(out *Output, call *Call) error {
			args := call.Rest
			var completed []string
			partial := ""
			if len(args) > 0 {
				completed = args[:len(args)-1]
				partial = args[len(args)-1]
			}
			tw := &TokenWriter{Writer: out.Stdout}
			return c.CompleteCLI(tw, completed, partial)
		},
	}
}

// CompleteCLI writes tab-completion candidates for the given command
// line tokens to w, implementing the [Completer] interface.
//
// The trie walk matches completed tokens against registered subcommands.
// When a child node implements [Completer] (a mounted [*Mux] or a
// [*Command]), the remaining tokens are delegated to that node's
// CompleteCLI method. Otherwise, the mux completes its own subcommands
// and mux-level flags.
func (m *Mux) CompleteCLI(w *TokenWriter, completed []string, partial string) error {
	n := &m.root
	skipNext := false
	for i, tok := range completed {
		if tok == "--" {
			return nil
		}
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(tok, "-") {
			if isValueOption(tok, m.options.specs) {
				skipNext = true
			}
			continue
		}
		child, ok := n.children[tok]
		if !ok {
			break
		}
		n = child
		if completer, ok := n.commandRunner().(Completer); ok {
			return completer.CompleteCLI(w, completed[i+1:], partial)
		}
	}

	// Check if previous completed word was a value-taking option.
	if len(completed) > 0 {
		prev := completed[len(completed)-1]
		if isValueOption(prev, m.options.specs) {
			return nil
		}
	}

	// Suppress completions for --option=<TAB> (value position).
	if isPartialOptionValue(partial, &m.flags, &m.options) {
		return nil
	}

	if strings.HasPrefix(partial, "-") {
		return writeFlagEntries(w, &m.flags, &m.options, partial, m.NegateFlags)
	}
	return writeSubcommands(w, n, partial)
}

// CompleteCLI writes tab-completion candidates for command-level
// flags and options to w, implementing the [Completer] interface.
//
// At option value position, CompleteCLI delegates to [Command.Completer]
// (if set) instead of emitting flag or argument completions. Two
// invocation shapes reach the completer:
//
//   - Space-separated value (e.g. "--host <TAB>"): the completer is
//     called with completed ending in the option token ("--host" or
//     its short form) and partial as the partial value.
//   - Equals-separated value (e.g. "--host=loc<TAB>"): the completer
//     is called with a synthesized completed ending in "--<name>" and
//     partial as the value portion after "=".
//
// When no [Command.Completer] is set, value position yields no
// completions.
func (c *Command) CompleteCLI(w *TokenWriter, completed []string, partial string) error {
	if slices.Contains(completed, "--") {
		return nil
	}
	if len(completed) > 0 && isValueOption(completed[len(completed)-1], c.options.specs) {
		if c.Completer != nil {
			return c.Completer.CompleteCLI(w, completed, partial)
		}
		return nil
	}
	if name, value, ok := splitOptionValuePartial(partial, &c.flags, &c.options); ok {
		if c.Completer != nil {
			synth := append(slices.Clone(completed), "--"+name)
			return c.Completer.CompleteCLI(w, synth, value)
		}
		return nil
	}

	if c.Completer != nil {
		if err := c.Completer.CompleteCLI(w, completed, partial); err != nil {
			return err
		}
	}

	if !strings.HasPrefix(partial, "-") {
		return writeArgHint(w, &c.args, completed, &c.options)
	}

	return writeFlagEntries(w, &c.flags, &c.options, partial, c.NegateFlags)
}

func writeFlagEntries(w *TokenWriter, flags *flagSpecs, options *optionSpecs, partial string, negateFlags bool) error {
	for _, f := range flags.helpEntries() {
		if err := writeEntry(w, "--"+f.Name, f.Usage, partial); err != nil {
			return err
		}
		if f.Short != "" {
			if err := writeEntry(w, "-"+f.Short, f.Usage, partial); err != nil {
				return err
			}
		}
		if negateFlags {
			var negName string
			if strings.HasPrefix(f.Name, "no-") {
				negName = f.Name[3:]
			} else {
				negName = "no-" + f.Name
			}
			if err := writeEntry(w, "--"+negName, f.Usage, partial); err != nil {
				return err
			}
		}
	}
	for _, o := range options.helpEntries() {
		if err := writeEntry(w, "--"+o.Name, o.Usage, partial); err != nil {
			return err
		}
		if o.Short != "" {
			if err := writeEntry(w, "-"+o.Short, o.Usage, partial); err != nil {
				return err
			}
		}
	}
	if err := writeEntry(w, "--help", "Show help", partial); err != nil {
		return err
	}
	return writeEntry(w, "-h", "Show help", partial)
}

func writeSubcommands(w *TokenWriter, n *node, partial string) error {
	names := slices.Sorted(maps.Keys(n.children))

	for _, name := range names {
		if err := writeEntry(w, name, n.children[name].usage(), partial); err != nil {
			return err
		}
	}
	return nil
}

func isValueOption(word string, specs []optionSpec) bool {
	for _, o := range specs {
		if word == "--"+o.Name || (o.Short != "" && word == "-"+o.Short) {
			return true
		}
	}
	return false
}

// isPartialOptionValue reports whether partial is a --option= prefix
// awaiting a value.
func isPartialOptionValue(partial string, flags *flagSpecs, options *optionSpecs) bool {
	_, _, ok := splitOptionValuePartial(partial, flags, options)
	return ok
}

// splitOptionValuePartial reports whether partial is a "--name=value"
// prefix awaiting completion of the value portion. When it returns
// true, name is the option name and value is the current partial
// value. It returns false for boolean flags and for names that are
// not registered as value-taking options.
func splitOptionValuePartial(partial string, flags *flagSpecs, options *optionSpecs) (name, value string, ok bool) {
	if !strings.HasPrefix(partial, "--") {
		return "", "", false
	}
	n, v, hasEquals := strings.Cut(partial[2:], "=")
	if !hasEquals {
		return "", "", false
	}
	if flags.hasName(n) {
		return "", "", false
	}
	if !options.hasName(n) {
		return "", "", false
	}
	return n, v, true
}

// writeArgHint emits the next expected positional argument name as a
// completion hint, if any remain.
func writeArgHint(w *TokenWriter, args *argSpecs, completed []string, options *optionSpecs) error {
	if args == nil || len(args.specs) == 0 {
		return nil
	}
	// Count positional tokens already consumed (skip flags and option values).
	pos := 0
	skipNext := false
	for _, tok := range completed {
		if skipNext {
			skipNext = false
			continue
		}
		if tok == "--" {
			break
		}
		if strings.HasPrefix(tok, "-") {
			if isValueOption(tok, options.specs) {
				skipNext = true
			}
			continue
		}
		pos++
	}
	if pos < len(args.specs) {
		spec := args.specs[pos]
		_, err := w.WriteToken("<"+spec.Name+">", spec.Usage)
		return err
	}
	return nil
}

func writeEntry(w *TokenWriter, value, desc, partial string) error {
	if strings.HasPrefix(value, partial) {
		_, err := w.WriteToken(value, desc)
		return err
	}
	return nil
}
