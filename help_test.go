package argv

import (
	"bytes"
	"strings"
	"testing"
)

func TestDefaultHelpRender(t *testing.T) {
	help := &Help{
		Name:     "app",
		FullPath: "app",
		Summary:  "A test application.",
		Flags: []HelpFlag{
			{Name: "verbose", Short: "v", Usage: "Enable verbose output", Default: false, Inherited: true},
			{Name: "force", Short: "f", Usage: "Force operation", Default: false},
		},
		Options: []HelpOption{
			{Name: "repository", Short: "r", Usage: "Repository path", Default: "", Inherited: true},
		},
		Commands: []HelpCommand{
			{Name: "init", Summary: "Initialize"},
			{Name: "ls", Summary: "List entries"},
		},
		Arguments: []HelpArg{
			{Name: "<name>", Usage: "Repository name"},
		},
	}

	var buf bytes.Buffer
	DefaultHelpFunc(&buf, help)

	out := buf.String()
	for _, want := range []string{"app", "init", "ls", "<name>", "-v, --verbose", "-r, --repository <repository>", "-f, --force", "Options:", "Global Options:", "A test application."} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestDefaultHelpRenderOmitsArgumentsWhenAbsent(t *testing.T) {
	help := &Help{
		Name:     "app",
		FullPath: "app status",
		Summary:  "Show status.",
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
		Summary:     "Show status.",
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
		Summary:  "Show status.",
		Flags: []HelpFlag{
			{Name: "verbose", Short: "v", Usage: "Enable verbose output\nAlso prints debug events.", Default: false, Inherited: true},
			{Name: "force", Short: "f", Usage: "Force operation\nSkips checks.", Default: false},
		},
		Options: []HelpOption{
			{Name: "repository", Short: "r", Usage: "Repository path\nCan be relative.", Default: "", Inherited: true},
			{Name: "config", Short: "c", Usage: "Configuration file\nCan be relative.", Default: ""},
		},
		Arguments: []HelpArg{
			{Name: "<name>", Usage: "Repository name\nMust already exist."},
		},
		Commands: []HelpCommand{
			{Name: "status", Summary: "Show status\nIncludes workspace checks."},
		},
	}

	var buf bytes.Buffer
	DefaultHelpFunc(&buf, help)

	out := buf.String()
	for _, want := range []string{
		"  -v, --verbose                  Enable verbose output\n                                 Also prints debug events.",
		"  -r, --repository <repository>  Repository path\n                                 Can be relative.",
		"  -f, --force            Force operation\n                         Skips checks.",
		"  -c, --config <config>  Configuration file\n                         Can be relative.",
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
		Summary:  "Copy files.",
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

func TestPositionalIndex(t *testing.T) {
	h := &Help{
		Options: []HelpOption{
			{Name: "host", Short: "H"},
			{Name: "config"},
		},
	}
	tests := []struct {
		name      string
		completed []string
		want      int
	}{
		{"empty", nil, 0},
		{"one positional", []string{"foo"}, 1},
		{"two positionals", []string{"foo", "bar"}, 2},
		{"flag only", []string{"--verbose"}, 0},
		{"option with value (long)", []string{"--host", "x"}, 0},
		{"option with value (short)", []string{"-H", "x"}, 0},
		{"flag then positional", []string{"--verbose", "foo"}, 1},
		{"option then positional", []string{"--host", "x", "foo"}, 1},
		{"positional then option", []string{"foo", "--host", "x"}, 1},
		{"double dash suppresses", []string{"--"}, -1},
		{"double dash mid-stream suppresses", []string{"foo", "--", "bar"}, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.PositionalIndex(tt.completed); got != tt.want {
				t.Fatalf("PositionalIndex(%v) = %d, want %d", tt.completed, got, tt.want)
			}
		})
	}
}
