package cli

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
)

// A HelpFunc renders help output to w for a resolved command path.
type HelpFunc func(w io.Writer, help *Help) error

// Help holds the data passed to a [HelpFunc] when rendering help output.
type Help struct {
	// Name is the final segment of the command path.
	Name string

	// FullPath is the complete command path (e.g. "app repo init").
	FullPath string

	// Usage is a short one-line summary.
	Usage string

	// Description is longer free-form help text.
	Description string

	// Flags lists boolean flags. Entries with Global set were declared
	// on a parent [Mux]; the rest were declared on the [Command].
	Flags []struct {
		Name      string
		Short     string
		Usage     string
		Default   bool
		Negatable bool
		Global    bool
	}

	// Options lists value options. Entries with Global set were declared
	// on a parent [Mux]; the rest were declared on the [Command].
	Options []struct {
		Name    string
		Short   string
		Usage   string
		Default string
		Global  bool
	}

	// Commands lists immediate child commands. When Commands is
	// non-empty the node is a routing point and Arguments is empty.
	Commands []struct {
		Name        string
		Usage       string
		Description string
	}

	// Arguments lists positional arguments accepted by this command.
	// When a node has Commands, Arguments is empty.
	Arguments []struct {
		Name  string
		Usage string
	}

	// CaptureRest indicates that the command accepts trailing
	// arguments beyond those listed in Arguments.
	CaptureRest bool
}

type helpFlag = struct {
	Name      string
	Short     string
	Usage     string
	Default   bool
	Negatable bool
	Global    bool
}
type helpOption = struct {
	Name    string
	Short   string
	Usage   string
	Default string
	Global  bool
}
type helpArg = struct {
	Name  string
	Usage string
}
type helpCommand = struct {
	Name        string
	Usage       string
	Description string
}

// DefaultHelpFunc is the built-in [HelpFunc] used when no override is set.
// It renders a tabular summary to w.
func DefaultHelpFunc(w io.Writer, help *Help) error {
	if help == nil {
		panic("cli: nil help")
	}
	commands := slices.Clone(help.Commands)
	slices.SortFunc(commands, func(a, b helpCommand) int {
		return cmp.Compare(a.Name, b.Name)
	})
	if help.Usage != "" {
		if _, err := fmt.Fprintf(w, "%s - %s\n", help.FullPath, help.Usage); err != nil {
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

	line := "  " + help.FullPath
	if len(commands) > 0 {
		line += " [command]"
	}
	if len(help.Flags) > 0 || len(help.Options) > 0 {
		line += " [options]"
	}
	if len(help.Arguments) > 0 {
		line += " [arguments]"
	}
	if help.CaptureRest {
		line += " [args...]"
	}
	line += "\n"
	if _, err := io.WriteString(w, line); err != nil {
		return err
	}

	globalFlags := filterFlags(help.Flags, true)
	localFlags := filterFlags(help.Flags, false)
	globalOptions := filterOptions(help.Options, true)
	localOptions := filterOptions(help.Options, false)

	if err := renderFlagSection(w, "Global Flags", globalFlags); err != nil {
		return err
	}
	if err := renderOptionSection(w, "Global Options", globalOptions); err != nil {
		return err
	}
	if err := renderFlagSection(w, "Flags", localFlags); err != nil {
		return err
	}
	if err := renderOptionSection(w, "Options", localOptions); err != nil {
		return err
	}

	if len(help.Arguments) > 0 {
		if _, err := io.WriteString(w, "\nArguments:\n"); err != nil {
			return err
		}
		rows := make([]helpRow, 0, len(help.Arguments))
		for _, argument := range help.Arguments {
			rows = append(rows, helpRow{Name: argument.Name, Usage: argument.Usage})
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
			rows = append(rows, helpRow{Name: cmd.Name, Usage: cmd.Usage})
		}
		if err := renderHelpTable(w, rows); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Use %q for more information.\n", help.FullPath+" [command] --help"); err != nil {
			return err
		}
	}
	return nil
}

func filterFlags(flags []helpFlag, global bool) []helpFlag {
	var out []helpFlag
	for _, f := range flags {
		if f.Global == global {
			out = append(out, f)
		}
	}
	return out
}

func filterOptions(options []helpOption, global bool) []helpOption {
	var out []helpOption
	for _, o := range options {
		if o.Global == global {
			out = append(out, o)
		}
	}
	return out
}

func renderFlagSection(w io.Writer, title string, entries []helpFlag) error {
	if len(entries) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\n%s:\n", title); err != nil {
		return err
	}
	rows := make([]helpRow, 0, len(entries))
	for _, e := range entries {
		usage := e.Usage
		usage += fmt.Sprintf(" (default: %t)", e.Default)
		rows = append(rows, helpRow{Name: formatInputName(e.Name, e.Short, e.Negatable), Usage: usage})
	}
	return renderHelpTable(w, rows)
}

func renderOptionSection(w io.Writer, title string, entries []helpOption) error {
	if len(entries) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\n%s:\n", title); err != nil {
		return err
	}
	rows := make([]helpRow, 0, len(entries))
	for _, e := range entries {
		usage := e.Usage
		if e.Default != "" {
			usage += fmt.Sprintf(" (default: %s)", e.Default)
		}
		rows = append(rows, helpRow{Name: formatInputName(e.Name, e.Short, false), Usage: usage})
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
		if len(lines) == 0 {
			lines = []string{""}
		}
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

func formatInputName(name, short string, negatable bool) string {
	var b strings.Builder
	if short != "" {
		b.WriteString("-")
		b.WriteString(short)
		b.WriteString(", ")
	}
	b.WriteString("--")
	b.WriteString(name)
	if negatable {
		b.WriteString(", --")
		if strings.HasPrefix(name, "no-") {
			b.WriteString(name[3:])
		} else {
			b.WriteString("no-")
			b.WriteString(name)
		}
	}
	return b.String()
}
