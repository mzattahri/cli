package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestDefaultHelpRender(t *testing.T) {
	help := &Help{
		Name:     "app",
		FullPath: "app",
		Usage:    "A test application.",
		Flags: []HelpFlag{
			{Name: "verbose", Short: "v", Usage: "Enable verbose output", Default: false, Global: true},
			{Name: "force", Short: "f", Usage: "Force operation", Default: false},
		},
		Options: []HelpOption{
			{Name: "repository", Short: "r", Usage: "Repository path", Default: "", Global: true},
		},
		Commands: []HelpCommand{
			{Name: "init", Usage: "Initialize"},
			{Name: "ls", Usage: "List entries"},
		},
		Arguments: []HelpArg{
			{Name: "<name>", Usage: "Repository name"},
		},
	}

	var buf bytes.Buffer
	DefaultHelpFunc(&buf, help)

	out := buf.String()
	for _, want := range []string{"app", "init", "ls", "<name>", "-v, --verbose", "-r, --repository", "-f, --force", "Global Flags:", "Global Options:", "Flags:", "A test application."} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestDefaultHelpRenderOmitsArgumentsWhenAbsent(t *testing.T) {
	help := &Help{
		Name:     "app",
		FullPath: "app status",
		Usage:    "Show status.",
	}

	var buf bytes.Buffer
	DefaultHelpFunc(&buf, help)

	out := buf.String()
	if strings.Contains(out, "[arguments]") {
		t.Fatalf("usage line should omit [arguments] when no arguments exist:\n%s", out)
	}
}

func TestDefaultHelpPrintsDescription(t *testing.T) {
	desc := "This is a description that the help renderer prints as-is."
	help := &Help{
		Name:        "app",
		FullPath:    "app",
		Usage:       "Show status.",
		Description: desc,
	}

	var buf bytes.Buffer
	DefaultHelpFunc(&buf, help)

	out := buf.String()
	if !strings.Contains(out, desc) {
		t.Fatalf("expected description in output, got:\n%s", out)
	}
}

func TestDefaultHelpAlignsMultilineUsage(t *testing.T) {
	help := &Help{
		Name:     "app",
		FullPath: "app",
		Usage:    "Show status.",
		Flags: []HelpFlag{
			{Name: "verbose", Short: "v", Usage: "Enable verbose output\nAlso prints debug events.", Default: false, Global: true},
			{Name: "force", Short: "f", Usage: "Force operation\nSkips checks.", Default: false},
		},
		Options: []HelpOption{
			{Name: "repository", Short: "r", Usage: "Repository path\nCan be relative.", Default: "", Global: true},
			{Name: "config", Short: "c", Usage: "Configuration file\nCan be relative.", Default: ""},
		},
		Arguments: []HelpArg{
			{Name: "<name>", Usage: "Repository name\nMust already exist."},
		},
		Commands: []HelpCommand{
			{Name: "status", Usage: "Show status\nIncludes workspace checks."},
		},
	}

	var buf bytes.Buffer
	DefaultHelpFunc(&buf, help)

	out := buf.String()
	for _, want := range []string{
		"  -v, --verbose  Enable verbose output\n                 Also prints debug events.",
		"  -r, --repository  Repository path\n                    Can be relative.",
		"  -f, --force  Force operation\n               Skips checks.",
		"  -c, --config  Configuration file\n                Can be relative.",
		"  <name>  Repository name\n          Must already exist.",
		"  status  Show status\n          Includes workspace checks.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestDefaultHelpPreservesArgumentOrder(t *testing.T) {
	help := &Help{
		Name:     "app",
		FullPath: "app copy",
		Usage:    "Copy files.",
		Arguments: []HelpArg{
			{Name: "<src>", Usage: "Source path"},
			{Name: "<dst>", Usage: "Destination path"},
		},
	}

	var buf bytes.Buffer
	DefaultHelpFunc(&buf, help)

	out := buf.String()
	src := strings.Index(out, "<src>")
	dst := strings.Index(out, "<dst>")
	if src == -1 || dst == -1 {
		t.Fatalf("expected both arguments in help output:\n%s", out)
	}
	if src > dst {
		t.Fatalf("argument order should be preserved, got:\n%s", out)
	}
}
