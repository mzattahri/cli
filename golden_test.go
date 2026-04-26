package argv

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/help/*.golden fixtures")

// goldenCases captures representative *Help shapes that exercise every
// DefaultHelpFunc rendering branch: usage line composition, global vs
// local sections, arguments, subcommands, captureRest, negatable
// flags, and multi-line usage strings.
var goldenCases = []struct {
	name string
	help *Help
}{
	{
		name: "bare",
		help: &Help{
			Name:     "app",
			FullPath: "app",
		},
	},
	{
		name: "usage-and-description",
		help: &Help{
			Name:        "app",
			FullPath:    "app",
			Usage:       "A demo CLI",
			Description: "Longer-form description\nwith two lines.",
		},
	},
	{
		name: "command-with-all-inputs",
		help: &Help{
			Name:        "deploy",
			FullPath:    "app deploy",
			Usage:       "Deploy the app",
			Description: "Deploys the application to the given target.",
			Flags: []HelpFlag{
				{Name: "verbose", Short: "v", Usage: "Verbose output", Inherited: true},
				{Name: "force", Short: "f", Usage: "Skip confirmation"},
				{Name: "dry-run", Usage: "Print plan without executing", Default: true},
			},
			Options: []HelpOption{
				{Name: "config", Short: "c", Usage: "Config file", Inherited: true},
				{Name: "region", Short: "r", Usage: "Target region", Default: "us-east"},
			},
			Arguments: []HelpArg{
				{Name: "<target>", Usage: "Deploy target name"},
			},
			Variadic: true,
		},
	},
	{
		name: "mux-with-subcommands",
		help: &Help{
			Name:        "repo",
			FullPath:    "app repo",
			Usage:       "Manage repositories",
			Description: "Create, clone, and manage git repositories.",
			Flags: []HelpFlag{
				{Name: "verbose", Short: "v", Usage: "Verbose output", Inherited: true},
			},
			Commands: []HelpCommand{
				{Name: "init", Usage: "Initialize a repository"},
				{Name: "clone", Usage: "Clone an existing repository"},
				{Name: "status", Usage: "Show repository status"},
			},
		},
	},
	{
		name: "negatable-flags",
		help: &Help{
			Name:     "up",
			FullPath: "tailscale up",
			Usage:    "Bring tailscaled up",
			Flags: []HelpFlag{
				{Name: "accept-dns", Usage: "Accept DNS configuration", Default: true, Negatable: true},
				{Name: "no-cache", Usage: "Disable cache", Negatable: true},
			},
		},
	},
	{
		name: "multiline-usage",
		help: &Help{
			Name:     "arg",
			FullPath: "app arg",
			Usage:    "Short",
			Flags: []HelpFlag{
				{Name: "verbose", Short: "v", Usage: "Line one of usage.\nLine two of usage."},
			},
		},
	},
}

func TestGoldenDefaultHelpFunc(t *testing.T) {
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := DefaultHelpFunc(&buf, tc.help); err != nil {
				t.Fatal(err)
			}
			got := buf.Bytes()
			path := filepath.Join("testdata", "help", tc.name+".golden")

			if *updateGolden {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v (run `go test -update` to create)", path, err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s",
					tc.name, got, want)
			}
		})
	}
}
