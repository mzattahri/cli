package argv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
)

func runMux(ctx context.Context, mux *Mux, stdout io.Writer, stderr io.Writer, args []string) error {
	program := &Program{Stdout: stdout, Stderr: stderr}
	return program.Invoke(ctx, mux, append([]string{"app"}, args...))
}

func TestBasicDispatch(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{Run: func(out *Output, call *Call) error {
		_, err := fmt.Fprintf(out.Stdout, "hello %s", call.Args.Get("name"))
		return err
	}}
	cmd.Arg("name", "Name to greet")
	mux.Handle("greet", "Say hello", cmd)
	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"greet", "Go"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "hello Go" {
		t.Fatalf("got %q", got)
	}
}

func TestCommandFlagsAndOptions(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			repository := call.Options.Get("repository")
			verbose := call.Flags.Get("verbose")
			_, err := fmt.Fprintf(out.Stdout, "%s|%t", repository, verbose)
			return err
		},
	}
	cmd.Option("repository", "r", "", "repo path")
	cmd.Flag("verbose", "v", false, "verbose")
	mux.Handle("track", "", cmd)
	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"track", "--repository", "/tmp/repo", "--verbose"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "/tmp/repo|true" {
		t.Fatalf("got %q", got)
	}
}

func TestShortFlagsAndOptions(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "%t|%t|%s", call.Flags.Get("verbose"), call.Flags.Get("force"), call.Options.Get("repository"))
			return err
		},
	}
	cmd.Flag("verbose", "v", false, "verbose")
	cmd.Flag("force", "f", false, "force")
	cmd.Option("repository", "r", "", "repo path")
	mux.Handle("track", "", cmd)

	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"track", "-vf", "-r", "/tmp/repo"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "true|true|/tmp/repo" {
		t.Fatalf("got %q", got)
	}
}

func TestCommandRunnerField(t *testing.T) {
	mux := &Mux{}
	mux.Handle("version", "", &Command{
		Run: func(out *Output, call *Call) error {
			_, err := io.WriteString(out.Stdout, "v1.0.0")
			return err
		},
	})
	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"version"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "v1.0.0" {
		t.Fatalf("got %q", got)
	}
}

func TestHandlePointerCommandUsesDescription(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Description: "Print the current version.",
		Run: func(*Output, *Call) error {
			return nil
		},
	}
	mux.Handle("version", "Show version", cmd)

	var gotHelp *Help
	program := &Program{
		Stdout:   io.Discard,
		Stderr:   io.Discard,
		HelpFunc: func(_ io.Writer, help *Help) error { gotHelp = help; return nil },
	}

	err := program.Invoke(context.Background(), mux, []string{"app", "version", "--help"})
	if err != nil {
		t.Fatal(err)
	}
	if gotHelp == nil {
		t.Fatal("expected help to be rendered")
	}
	if gotHelp.Description != "Print the current version." {
		t.Fatalf("got description %q", gotHelp.Description)
	}
}

func TestPositionalArgsAreStrings(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "%s|%s", call.Args.Get("repo"), call.Args.Get("path"))
			return err
		},
	}
	cmd.Arg("repo", "Repository name")
	cmd.Arg("path", "File path")
	mux.Handle("open", "", cmd)
	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"open", "terminal", "README.md"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "terminal|README.md" {
		t.Fatalf("got %q", got)
	}
}

func TestVariadicPreservesTrailingArgs(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "%v", call.Tail)
			return err
		},
	}
	cmd.Tail("patterns", "")
	mux.Handle("match", "", cmd)
	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"match", "a*", "b*", "c*"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "[a* b* c*]" {
		t.Fatalf("got %q", got)
	}
}

func TestDoubleDashCanBePositionalArgument(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "%q", call.Args.Get("value"))
			return err
		},
	}
	cmd.Arg("value", "Value to echo")
	mux.Handle("echo", "", cmd)

	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"echo", "--", "--"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != `"--"` {
		t.Fatalf("got %q", got)
	}
}

func TestVariadicPreservesLiteralDoubleDash(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "value=%q tail=%q", call.Args.Get("value"), call.Tail)
			return err
		},
	}
	cmd.Arg("value", "Leading value")
	cmd.Tail("rest", "")
	mux.Handle("echo", "", cmd)

	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"echo", "--", "--", "tail"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != `value="--" tail=["tail"]` {
		t.Fatalf("got %q", got)
	}
}

func TestProgramInheritedFlagsAndOptions(t *testing.T) {
	mux := &Mux{}
	mux.Option("host", "", "", "daemon socket")
	mux.Flag("verbose", "", false, "verbose")
	mux.Handle("run", "", RunnerFunc(func(out *Output, call *Call) error {
		host := call.Options.Get("host")
		verbose := call.Flags.Get("verbose")
		_, err := fmt.Fprintf(out.Stdout, "%s|%t", host, verbose)
		return err
	}))
	var out bytes.Buffer
	program := &Program{
		Stdout: &out,
		Stderr: io.Discard,
	}
	if err := program.Invoke(context.Background(), mux, []string{"app", "--host", "unix:///tmp/docker.sock", "--verbose", "run"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "unix:///tmp/docker.sock|true" {
		t.Fatalf("got %q", got)
	}
}

func TestMountedMuxHelpIncludesProgramGlobals(t *testing.T) {
	root := &Mux{}
	root.Flag("verbose", "v", false, "verbose")
	root.Option("config", "c", "", "config file")
	sub := &Mux{}
	var gotHelp *Help
	sub.Handle("init", "Initialize repo", &Command{
		Run: func(*Output, *Call) error { return nil },
	})
	root.Handle("repo", "Repository commands", sub)

	program := &Program{
		Stdout:   io.Discard,
		Stderr:   io.Discard,
		HelpFunc: func(_ io.Writer, help *Help) error { gotHelp = help; return nil },
	}

	if err := program.Invoke(context.Background(), root, []string{"app", "repo", "init", "--help"}); err != nil {
		t.Fatal(err)
	}
	if gotHelp == nil {
		t.Fatal("expected help to be rendered")
	}
	inheritedFlags := slices.Collect(gotHelp.InheritedFlags())
	if len(inheritedFlags) != 1 || inheritedFlags[0].Name != "verbose" {
		t.Fatalf("got global flags %#v", inheritedFlags)
	}
	inheritedOptions := slices.Collect(gotHelp.InheritedOptions())
	if len(inheritedOptions) != 1 || inheritedOptions[0].Name != "config" {
		t.Fatalf("got global options %#v", inheritedOptions)
	}
}

func TestNestedCommands(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{Run: func(out *Output, call *Call) error {
		_, err := io.WriteString(out.Stdout, call.Args.Get("name"))
		return err
	}}
	cmd.Arg("name", "repo name")
	mux.Handle("repo init", "", cmd)
	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"repo", "init", "demo"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "demo" {
		t.Fatalf("got %q", got)
	}
}

func TestMount(t *testing.T) {
	sub := &Mux{}
	sub.Handle("init", "Initialize", RunnerFunc(func(out *Output, call *Call) error {
		_, err := io.WriteString(out.Stdout, "repo-init")
		return err
	}))
	mux := &Mux{}
	mux.Handle("repo", "Manage repositories", sub)
	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"repo", "init"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "repo-init" {
		t.Fatalf("got %q", got)
	}
}

func TestUnknownCommandShowsHelp(t *testing.T) {
	mux := &Mux{}
	mux.Handle("greet", "Say hello", RunnerFunc(func(out *Output, call *Call) error { return nil }))
	var errout bytes.Buffer
	err := runMux(context.Background(), mux, io.Discard, &errout, []string{"nope"})
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("got err=%v", err)
	}
	if !strings.Contains(errout.String(), `unknown command "nope"`) {
		t.Fatalf("missing unknown command message:\n%s", errout.String())
	}
	if !strings.Contains(errout.String(), "greet") {
		t.Fatalf("help missing command:\n%s", errout.String())
	}
}

func TestNoSubcommandDoesNotSayUnknown(t *testing.T) {
	mux := &Mux{}
	mux.Handle("greet", "Say hello", RunnerFunc(func(out *Output, call *Call) error { return nil }))
	var errout bytes.Buffer
	err := runMux(context.Background(), mux, io.Discard, &errout, nil)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("got err=%v", err)
	}
	if strings.Contains(errout.String(), "unknown command") {
		t.Fatalf("should not say unknown command when no args given:\n%s", errout.String())
	}
}

func TestProgramHelpFunc(t *testing.T) {
	mux := &Mux{}
	mux.Handle("greet", "Say hello", RunnerFunc(func(out *Output, call *Call) error { return nil }))

	var stdout bytes.Buffer
	program := &Program{
		Stdout: &stdout,
		Stderr: io.Discard,
		HelpFunc: func(w io.Writer, help *Help) error {
			_, _ = io.WriteString(w, "custom help")
			return nil
		},
	}
	err := program.Invoke(context.Background(), mux, []string{"app", "--help"})
	if err != nil {
		t.Fatalf("got err=%v", err)
	}
	if got := stdout.String(); got != "custom help" {
		t.Fatalf("got %q", got)
	}
}

func TestHelpIncludesOptionsAndArgs(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error { return nil },
	}
	cmd.Option("repository", "", "", "repo path")
	cmd.Arg("path", "Path to open")
	mux.Handle("open", "Open files", cmd)
	var stdout bytes.Buffer
	if err := runMux(context.Background(), mux, &stdout, io.Discard, []string{"open", "--help"}); err != nil {
		t.Fatalf("got err=%v", err)
	}
	help := stdout.String()
	for _, want := range []string{"--repository", "<path>"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

func TestHiddenCommandOmittedFromParentHelp(t *testing.T) {
	mux := &Mux{}
	mux.Handle("ls", "List entries", RunnerFunc(func(out *Output, call *Call) error { return nil }))
	mux.Handle("internal", "Internal tools", &Command{
		Hidden: true,
		Run:    func(out *Output, call *Call) error { return nil },
	})

	var stdout bytes.Buffer
	if err := runMux(context.Background(), mux, &stdout, io.Discard, []string{"--help"}); err != nil {
		t.Fatal(err)
	}
	help := stdout.String()
	if !strings.Contains(help, "ls") {
		t.Fatalf("expected ls in parent help:\n%s", help)
	}
	if strings.Contains(help, "internal") {
		t.Fatalf("hidden command leaked into parent help:\n%s", help)
	}
}

func TestHiddenCommandStillRoutable(t *testing.T) {
	mux := &Mux{}
	mux.Handle("internal", "Internal tools", &Command{
		Hidden: true,
		Run: func(out *Output, call *Call) error {
			_, err := io.WriteString(out.Stdout, "ran")
			return err
		},
	})

	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"internal"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "ran" {
		t.Fatalf("got %q", got)
	}
}

func TestHiddenSubMuxOmittedFromParentHelp(t *testing.T) {
	hiddenMux := &Mux{Hidden: true, Description: "internal subcommands"}
	hiddenMux.Handle("dump", "Dump state", RunnerFunc(func(out *Output, call *Call) error { return nil }))

	root := &Mux{}
	root.Handle("ls", "List entries", RunnerFunc(func(out *Output, call *Call) error { return nil }))
	root.Handle("internal", "Internal tools", hiddenMux)

	var stdout bytes.Buffer
	if err := runMux(context.Background(), root, &stdout, io.Discard, []string{"--help"}); err != nil {
		t.Fatal(err)
	}
	help := stdout.String()
	if !strings.Contains(help, "ls") {
		t.Fatalf("expected ls in parent help:\n%s", help)
	}
	if strings.Contains(help, "internal") {
		t.Fatalf("hidden sub-mux leaked into parent help:\n%s", help)
	}
}

func TestHiddenViaCustomHelper(t *testing.T) {
	// Custom Helper that sets Hidden directly. Verifies the framework
	// honours any Helper that flips the bit, not only *Command.
	helper := &annotatedHelper{description: "Hidden via Helper"}

	root := &Mux{}
	root.Handle("ls", "List entries", RunnerFunc(func(out *Output, call *Call) error { return nil }))
	root.Handle("internal", "", helper)

	var stdout bytes.Buffer
	if err := runMux(context.Background(), root, &stdout, io.Discard, []string{"--help"}); err != nil {
		t.Fatal(err)
	}
	help := stdout.String()
	if !strings.Contains(help, "ls") {
		t.Fatalf("expected ls in parent help:\n%s", help)
	}
	if strings.Contains(help, "internal") {
		t.Fatalf("Helper-marked-hidden runner leaked into parent help:\n%s", help)
	}
}

type annotatedHelper struct{ description string }

func (h *annotatedHelper) RunArgv(*Output, *Call) error { return nil }
func (h *annotatedHelper) HelpArgv(out *Help) {
	out.Description = h.description
	out.Hidden = true
}

func TestHiddenCommandRendersOwnHelp(t *testing.T) {
	mux := &Mux{}
	mux.Handle("internal", "Internal tools", &Command{
		Hidden:      true,
		Description: "Hidden but reachable",
		Run:         func(out *Output, call *Call) error { return nil },
	})

	var stdout bytes.Buffer
	if err := runMux(context.Background(), mux, &stdout, io.Discard, []string{"internal", "--help"}); err != nil {
		t.Fatal(err)
	}
	help := stdout.String()
	if !strings.Contains(help, "Hidden but reachable") {
		t.Fatalf("expected hidden command's help to render:\n%s", help)
	}
}

func TestCommandAnnotationsCopiedToHelp(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error { return nil },
		Annotations: map[string]any{
			"manpage.seealso": []string{"foo(1)", "bar(1)"},
			"deprecated":      true,
		},
	}
	mux.Handle("ship", "Ship", cmd)

	var gotHelp *Help
	program := &Program{
		Stdout:   io.Discard,
		Stderr:   io.Discard,
		HelpFunc: func(_ io.Writer, h *Help) error { gotHelp = h; return nil },
	}
	if err := program.Invoke(context.Background(), mux, []string{"app", "ship", "--help"}); err != nil {
		t.Fatal(err)
	}
	if gotHelp == nil {
		t.Fatal("expected help to be rendered")
	}
	if refs, ok := gotHelp.Annotations["manpage.seealso"].([]string); !ok || len(refs) != 2 {
		t.Fatalf("got annotations %#v", gotHelp.Annotations)
	}
	if dep, _ := gotHelp.Annotations["deprecated"].(bool); !dep {
		t.Fatalf("got deprecated %#v", gotHelp.Annotations["deprecated"])
	}
}

func TestMuxAnnotationsDoNotCascade(t *testing.T) {
	root := &Mux{
		Annotations: map[string]any{"category": "root"},
	}
	leaf := &Command{
		Run: func(out *Output, call *Call) error { return nil },
	}
	root.Handle("ship", "Ship", leaf)

	var gotHelp *Help
	program := &Program{
		Stdout:   io.Discard,
		Stderr:   io.Discard,
		HelpFunc: func(_ io.Writer, h *Help) error { gotHelp = h; return nil },
	}
	if err := program.Invoke(context.Background(), root, []string{"app", "ship", "--help"}); err != nil {
		t.Fatal(err)
	}
	if _, present := gotHelp.Annotations["category"]; present {
		t.Fatalf("annotations leaked from parent: %#v", gotHelp.Annotations)
	}
}

func TestMuxAnnotationsVisibleAtSelfYield(t *testing.T) {
	root := &Mux{
		Annotations: map[string]any{"category": "root", "version": 1},
	}
	root.Handle("ship", "Ship", RunnerFunc(func(*Output, *Call) error { return nil }))

	var rootHelp *Help
	for help := range (&Program{}).Walk("app", root) {
		if help.FullPath == "app" {
			rootHelp = help
			break
		}
	}
	if rootHelp == nil {
		t.Fatal("expected root entry from Walk")
	}
	if cat, _ := rootHelp.Annotations["category"].(string); cat != "root" {
		t.Fatalf("got annotations %#v", rootHelp.Annotations)
	}
	if v, _ := rootHelp.Annotations["version"].(int); v != 1 {
		t.Fatalf("got version %#v", rootHelp.Annotations["version"])
	}
}

func TestCommandRestHoldsUnparsedTokens(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "repo=%s tail=%v", call.Options.Get("repository"), call.Tail)
			return err
		},
	}
	cmd.Option("repository", "", "", "repo path")
	cmd.Tail("paths", "")
	mux.Handle("open", "", cmd)

	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"open", "--repository", "/tmp/repo", "README.md"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "repo=/tmp/repo tail=[README.md]" {
		t.Fatalf("got %q", got)
	}
}

func TestCustomHelpGetsRootName(t *testing.T) {
	mux := &Mux{}

	var stdout bytes.Buffer
	program := &Program{
		Stdout: &stdout,
		Stderr: io.Discard,
		HelpFunc: func(w io.Writer, help *Help) error {
			_, _ = fmt.Fprintf(w, "%s|%s", help.Name, help.FullPath)
			return nil
		},
	}
	if err := program.Invoke(context.Background(), mux, []string{"app", "--help"}); err != nil {
		t.Fatalf("got err=%v", err)
	}
	if got := stdout.String(); got != "app|app" {
		t.Fatalf("got %q", got)
	}
}

func TestHelpDoesNotShadowOptionValue(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			value := call.Options.Get("template")
			_, err := io.WriteString(out.Stdout, value)
			return err
		},
	}
	cmd.Option("template", "", "", "template name")
	mux.Handle("render", "", cmd)

	var out bytes.Buffer
	var errout bytes.Buffer
	if err := runMux(context.Background(), mux, &out, &errout, []string{"render", "--template", "--help"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "--help" {
		t.Fatalf("got %q", got)
	}
	if got := errout.String(); got != "" {
		t.Fatalf("got unexpected stderr %q", got)
	}
}

func TestMuxFlagsAreScopedToLevel(t *testing.T) {
	root := &Mux{}
	root.Option("host", "", "", "daemon socket")
	sub := &Mux{}
	sub.Handle("init", "", RunnerFunc(func(out *Output, call *Call) error {
		_, err := io.WriteString(out.Stdout, "run")
		return err
	}))
	root.Handle("repo", "", sub)

	// Root-level option placed at the root position works.
	var out bytes.Buffer
	program := &Program{
		Stdout: &out,
		Stderr: io.Discard,
	}
	if err := program.Invoke(context.Background(), root, []string{"app", "--host", "unix:///tmp/docker.sock", "repo", "init"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "run" {
		t.Fatalf("got %q", got)
	}
}

func TestProgramMuxRootHandlerWithInheritedOptions(t *testing.T) {
	mux := &Mux{}
	mux.Option("host", "", "", "daemon socket")
	mux.Handle("", "Run the root command", RunnerFunc(func(out *Output, call *Call) error {
		host := call.Options.Get("host")
		_, err := fmt.Fprintf(out.Stdout, "%s", host)
		return err
	}))

	var out bytes.Buffer
	var errout bytes.Buffer
	program := &Program{
		Stdout:  &out,
		Stderr:  &errout,
		Summary: "Run the root command",
	}
	if err := program.Invoke(context.Background(), mux, []string{"app", "--host", "unix:///tmp/docker.sock"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "unix:///tmp/docker.sock" {
		t.Fatalf("got %q", got)
	}

	out.Reset()
	errout.Reset()
	if err := program.Invoke(context.Background(), mux, []string{"app", "--help"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "--host") {
		t.Fatalf("help missing global option:\n%s", out.String())
	}
}

func TestMuxRejectsFlagOptionNameCollision(t *testing.T) {
	mux := &Mux{}
	mux.Flag("name", "", false, "flag")
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	mux.Option("name", "", "", "option")
}

func TestMuxRejectsCrossLevelNegationShadow(t *testing.T) {
	t.Run("mux flag added after child declares its negation counterpart", func(t *testing.T) {
		mux := &Mux{}
		cmd := &Command{Run: func(*Output, *Call) error { return nil }}
		cmd.Flag("no-verbose", "", false, "")
		mux.Handle("run", "", cmd)

		defer func() {
			got := recover()
			if got == nil {
				t.Fatal("expected panic for negation cross-level shadow")
			}
			if s, ok := got.(string); !ok || !strings.Contains(s, "negation") {
				t.Fatalf("got panic %v", got)
			}
		}()
		mux.Flag("verbose", "", false, "")
	})

	t.Run("child declares negation counterpart of mux flag at mount time", func(t *testing.T) {
		mux := &Mux{}
		mux.Flag("verbose", "", false, "")
		cmd := &Command{Run: func(*Output, *Call) error { return nil }}
		cmd.Flag("no-verbose", "", false, "")

		defer func() {
			got := recover()
			if got == nil {
				t.Fatal("expected panic for negation cross-level shadow")
			}
			if s, ok := got.(string); !ok || !strings.Contains(s, "negation") {
				t.Fatalf("got panic %v", got)
			}
		}()
		mux.Handle("run", "", cmd)
	})

	t.Run("option side: mux declares no-cache, child declares cache", func(t *testing.T) {
		mux := &Mux{}
		cmd := &Command{Run: func(*Output, *Call) error { return nil }}
		cmd.Option("cache", "", "", "")
		mux.Handle("run", "", cmd)

		defer func() {
			got := recover()
			if got == nil {
				t.Fatal("expected panic")
			}
			if s, ok := got.(string); !ok || !strings.Contains(s, "negation") {
				t.Fatalf("got panic %v", got)
			}
		}()
		mux.Flag("no-cache", "", false, "")
	})
}

func TestMuxRejectsCrossLevelFlagCollision(t *testing.T) {
	t.Run("mux flag added after child declares it", func(t *testing.T) {
		mux := &Mux{}
		cmd := &Command{Run: func(*Output, *Call) error { return nil }}
		cmd.Flag("verbose", "", false, "")
		mux.Handle("run", "", cmd)

		defer func() {
			got := recover()
			if got == nil {
				t.Fatal("expected panic for cross-level collision")
			}
			if s, ok := got.(string); !ok || !strings.Contains(s, "shadows") {
				t.Fatalf("got panic %v", got)
			}
		}()
		mux.Flag("verbose", "", false, "")
	})

	t.Run("child mounted after mux declares flag", func(t *testing.T) {
		mux := &Mux{}
		mux.Flag("verbose", "", false, "")
		cmd := &Command{Run: func(*Output, *Call) error { return nil }}
		cmd.Option("verbose", "", "", "")

		defer func() {
			got := recover()
			if got == nil {
				t.Fatal("expected panic for cross-level collision")
			}
			if s, ok := got.(string); !ok || !strings.Contains(s, "shadowed") {
				t.Fatalf("got panic %v", got)
			}
		}()
		mux.Handle("run", "", cmd)
	})

	t.Run("sub-mux mount collides with root mux", func(t *testing.T) {
		root := &Mux{}
		root.Flag("verbose", "", false, "")
		sub := &Mux{}
		sub.Flag("verbose", "", false, "")

		defer func() {
			if recover() == nil {
				t.Fatal("expected panic")
			}
		}()
		root.Handle("repo", "", sub)
	})
}

func TestMuxFlagAndOption(t *testing.T) {
	mux := &Mux{}
	mux.Flag("verbose", "v", false, "verbose")
	mux.Option("host", "", "", "daemon socket")
	mux.Handle("run", "", RunnerFunc(func(out *Output, call *Call) error {
		_, err := fmt.Fprintf(out.Stdout, "%s|%t", call.Options.Get("host"), call.Flags.Get("verbose"))
		return err
	}))
	var out bytes.Buffer
	call := NewCall(context.Background(), []string{"--host", "localhost", "--verbose", "run"})
	if err := mux.RunArgv(&Output{Stdout: &out, Stderr: io.Discard}, call); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "localhost|true" {
		t.Fatalf("got %q", got)
	}
}

func TestMountedMuxScopedFlags(t *testing.T) {
	root := &Mux{}
	root.Flag("verbose", "v", false, "verbose")
	sub := &Mux{}
	sub.Flag("dry-run", "n", false, "dry run")
	sub.Handle("init", "", RunnerFunc(func(out *Output, call *Call) error {
		_, err := fmt.Fprintf(out.Stdout, "verbose=%t dry-run=%t",
			call.Flags.Get("verbose"), call.Flags.Get("dry-run"))
		return err
	}))
	root.Handle("repo", "Repository commands", sub)

	var out bytes.Buffer
	program := &Program{Stdout: &out, Stderr: io.Discard}
	if err := program.Invoke(context.Background(), root, []string{"app", "--verbose", "repo", "--dry-run", "init"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "verbose=true dry-run=true" {
		t.Fatalf("got %q", got)
	}
}

func TestMountedMuxHelpShowsAllAncestorFlags(t *testing.T) {
	root := &Mux{}
	root.Flag("verbose", "v", false, "verbose")
	sub := &Mux{}
	sub.Option("repository", "r", ".", "repo path")
	var gotHelp *Help
	sub.Handle("init", "Initialize", &Command{Run: func(*Output, *Call) error { return nil }})
	root.Handle("repo", "Repository commands", sub)

	program := &Program{
		Stdout:   io.Discard,
		Stderr:   io.Discard,
		HelpFunc: func(_ io.Writer, help *Help) error { gotHelp = help; return nil },
	}
	if err := program.Invoke(context.Background(), root, []string{"app", "repo", "init", "--help"}); err != nil {
		t.Fatal(err)
	}
	if gotHelp == nil {
		t.Fatal("expected help to be rendered")
	}
	// Should include both root mux flag and repo mux option.
	inheritedFlags := slices.Collect(gotHelp.InheritedFlags())
	if len(inheritedFlags) != 1 || inheritedFlags[0].Name != "verbose" {
		t.Fatalf("got global flags %#v", inheritedFlags)
	}
	inheritedOptions := slices.Collect(gotHelp.InheritedOptions())
	if len(inheritedOptions) != 1 || inheritedOptions[0].Name != "repository" {
		t.Fatalf("got global options %#v", inheritedOptions)
	}
}

func TestNegateFlagsCommand(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		NegateFlags: true,
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "verbose=%t force=%t",
				call.Flags.Get("verbose"), call.Flags.Get("force"))
			return err
		},
	}
	cmd.Flag("verbose", "v", false, "verbose")
	cmd.Flag("force", "f", false, "force")
	mux.Handle("run", "", cmd)

	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"run", "--verbose", "--no-force"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "verbose=true force=false" {
		t.Fatalf("got %q", got)
	}
}

func TestNegateFlagsTrueDefault(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		NegateFlags: true,
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "accept-dns=%t", call.Flags.Get("accept-dns"))
			return err
		},
	}
	cmd.Flag("accept-dns", "", true, "accept DNS")
	mux.Handle("up", "", cmd)

	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"up", "--no-accept-dns"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "accept-dns=false" {
		t.Fatalf("got %q", got)
	}
}

func TestNegateFlagsBidirectional(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		NegateFlags: true,
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "no-cache=%t", call.Flags.Get("no-cache"))
			return err
		},
	}
	cmd.Flag("no-cache", "", true, "disable cache")
	mux.Handle("build", "", cmd)

	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"build", "--cache"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "no-cache=false" {
		t.Fatalf("got %q", got)
	}
}

func TestNegateFlagsUnknownErrors(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		NegateFlags: true,
		Run:         func(*Output, *Call) error { return nil },
	}
	cmd.Flag("verbose", "", false, "verbose")
	mux.Handle("run", "", cmd)

	err := runMux(context.Background(), mux, io.Discard, io.Discard, []string{"run", "--no-unknown"})
	if err == nil {
		t.Fatal("expected error for --no-unknown")
	}
}

func TestNegateFlagsDisabledByDefault(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(*Output, *Call) error { return nil },
	}
	cmd.Flag("verbose", "", false, "verbose")
	mux.Handle("run", "", cmd)

	err := runMux(context.Background(), mux, io.Discard, io.Discard, []string{"run", "--no-verbose"})
	if err == nil {
		t.Fatal("expected error for --no-verbose when NegateFlags is false")
	}
}

func TestNegateFlagsMux(t *testing.T) {
	mux := &Mux{}
	mux.NegateFlags = true
	mux.Flag("verbose", "v", false, "verbose")
	mux.Handle("run", "", RunnerFunc(func(out *Output, call *Call) error {
		_, err := fmt.Fprintf(out.Stdout, "verbose=%t", call.Flags.Get("verbose"))
		return err
	}))

	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"--no-verbose", "run"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "verbose=false" {
		t.Fatalf("got %q", got)
	}
}

func TestNegateFlagsHelpRendersBracketed(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		NegateFlags: true,
		Run:         func(*Output, *Call) error { return nil },
	}
	cmd.Flag("accept-dns", "", true, "accept DNS")
	cmd.Flag("no-cache", "", true, "disable cache")
	mux.Handle("up", "Connect", cmd)

	var stdout bytes.Buffer
	if err := runMux(context.Background(), mux, &stdout, io.Discard, []string{"up", "--help"}); err != nil {
		t.Fatalf("got err=%v", err)
	}
	help := stdout.String()
	if !strings.Contains(help, "--[no-]accept-dns") {
		t.Fatalf("help missing --[no-]accept-dns:\n%s", help)
	}
	if !strings.Contains(help, "--[no-]cache") {
		t.Fatalf("help missing --[no-]cache (negation of --no-cache):\n%s", help)
	}
}

func TestRepeatedOptionAccumulates(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "last=%s all=%v",
				call.Options.Get("tag"), call.Options.Values("tag"))
			return err
		},
	}
	cmd.Option("tag", "t", "", "tags")
	mux.Handle("run", "", cmd)

	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"run", "--tag", "a", "--tag", "b", "-t", "c"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "last=c all=[a b c]" {
		t.Fatalf("got %q", got)
	}
}

func TestRepeatedOptionReplacesDefault(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "get=%s values=%v",
				call.Options.Get("host"), call.Options.Values("host"))
			return err
		},
	}
	cmd.Option("host", "", "localhost", "target host")
	mux.Handle("run", "", cmd)

	t.Run("default", func(t *testing.T) {
		var out bytes.Buffer
		if err := runMux(context.Background(), mux, &out, io.Discard, []string{"run"}); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != "get=localhost values=[localhost]" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("override", func(t *testing.T) {
		var out bytes.Buffer
		if err := runMux(context.Background(), mux, &out, io.Discard, []string{"run", "--host", "example.com"}); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != "get=example.com values=[example.com]" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestApplyDefaultsIdempotent(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "host=%s verbose=%t",
				call.Options.Get("host"), call.Flags.Get("verbose"))
			return err
		},
	}
	cmd.Option("host", "", "localhost", "target host")
	cmd.Flag("verbose", "", false, "verbose")
	mux.Handle("run", "", cmd)

	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"run"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "host=localhost verbose=false" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyDefaultsSparseBeforeComplete(t *testing.T) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "host=%s verbose=%t",
				call.Options.Get("host"), call.Flags.Get("verbose"))
			return err
		},
	}
	cmd.Option("host", "", "localhost", "target host")
	cmd.Flag("verbose", "", false, "verbose")
	mux.Handle("run", "", cmd)

	t.Run("defaults applied", func(t *testing.T) {
		var out bytes.Buffer
		if err := runMux(context.Background(), mux, &out, io.Discard, []string{"run"}); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != "host=localhost verbose=false" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("cli values override defaults", func(t *testing.T) {
		var out bytes.Buffer
		if err := runMux(context.Background(), mux, &out, io.Discard, []string{"run", "--host", "example.com", "--verbose"}); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != "host=example.com verbose=true" {
			t.Fatalf("got %q", got)
		}
	})
}

// mapLookup is a test-local helper: map-backed [LookupFunc] for
// EnvMiddleware tests. The public variant lives in argvtest.
func mapLookup(env map[string]string) LookupFunc {
	return func(k string) (string, bool) { v, ok := env[k]; return v, ok }
}

// newEnvTestCommand builds a Command that declares a "verbose" flag
// and "host" option, and prints their values.
func newEnvTestCommand() *Command {
	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "host=%s verbose=%t",
				call.Options.Get("host"), call.Flags.Get("verbose"))
			return err
		},
	}
	cmd.Flag("verbose", "", false, "")
	cmd.Option("host", "", "", "")
	return cmd
}

func TestEnvMap(t *testing.T) {
	env := map[string]string{
		"APP_HOST": "env-host",
		"VERBOSE":  "1",
	}
	mw := EnvMiddleware(map[string]string{
		"verbose": "VERBOSE",
		"host":    "APP_HOST",
	}, mapLookup(env))

	t.Run("fills missing values", func(t *testing.T) {
		call := NewCall(context.Background(), nil)

		var out bytes.Buffer
		err := mw(newEnvTestCommand()).RunArgv(&Output{Stdout: &out, Stderr: io.Discard}, call)
		if err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != "host=env-host verbose=true" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("cli overrides env", func(t *testing.T) {
		call := NewCall(context.Background(), nil)
		call.Options.Set("host", "cli-host")
		call.Flags.Set("verbose", false)

		var out bytes.Buffer
		err := mw(newEnvTestCommand()).RunArgv(&Output{Stdout: &out, Stderr: io.Discard}, call)
		if err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != "host=cli-host verbose=false" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestEnvMap_PanicsOnUndeclaredBinding(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on undeclared binding")
		}
	}()
	noop := &Command{Run: func(*Output, *Call) error { return nil }}
	EnvMiddleware(map[string]string{"not-declared": "NOPE"}, nil)(noop)
}

func TestEnvMap_PanicsOnUndeclaredBindingDeterministic(t *testing.T) {
	// With multiple unknown names, the panic message must always
	// surface the alphabetically first one regardless of map iteration.
	noop := &Command{Run: func(*Output, *Call) error { return nil }}
	want := `"alpha"`
	for i := range 16 {
		got := func() (got string) {
			defer func() {
				if r := recover(); r != nil {
					got, _ = r.(string)
				}
			}()
			EnvMiddleware(map[string]string{
				"zeta":  "Z",
				"alpha": "A",
				"mike":  "M",
			}, nil)(noop)
			return ""
		}()
		if !strings.Contains(got, want) {
			t.Fatalf("iteration %d: panic should name %s first; got %q", i, want, got)
		}
	}
}

// newDebugFlagCommand builds a Command that declares a "debug" flag.
func newDebugFlagCommand() *Command {
	cmd := &Command{Run: func(*Output, *Call) error { return nil }}
	cmd.Flag("debug", "", false, "")
	return cmd
}

func TestEnvMapParsesBooleanValues(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want bool
	}{
		{"one", "1", true},
		{"true lower", "true", true},
		{"true upper", "TRUE", true},
		{"true mixed", "True", true},
		{"yes", "yes", true},
		{"on", "on", true},
		{"y", "y", true},
		{"t", "t", true},
		{"zero", "0", false},
		{"false", "false", false},
		{"FALSE", "FALSE", false},
		{"no", "no", false},
		{"off", "off", false},
		{"padded", "  YES  ", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mw := EnvMiddleware(
				map[string]string{"debug": "DEBUG"},
				mapLookup(map[string]string{"DEBUG": tc.val}),
			)
			call := NewCall(context.Background(), nil)
			if err := mw(newDebugFlagCommand()).RunArgv(&Output{Stdout: io.Discard, Stderr: io.Discard}, call); err != nil {
				t.Fatal(err)
			}
			if got := call.Flags.Get("debug"); got != tc.want {
				t.Fatalf("got %t, want %t", got, tc.want)
			}
		})
	}
}

func TestEnvMapEmptyStringSkipsFlag(t *testing.T) {
	mw := EnvMiddleware(
		map[string]string{"debug": "DEBUG"},
		mapLookup(map[string]string{"DEBUG": ""}),
	)
	call := NewCall(context.Background(), nil)

	if err := mw(newDebugFlagCommand()).RunArgv(&Output{Stdout: io.Discard, Stderr: io.Discard}, call); err != nil {
		t.Fatal(err)
	}
	// The command declares debug with default=false, so after its
	// RunArgv applies defaults the entry exists but is not explicit.
	if _, ok := call.Flags.Lookup("debug"); ok {
		t.Fatalf("empty env value should leave flag non-explicit, got Flags=%v", call.Flags)
	}
}

func TestEnvMapInvalidBooleanReturnsError(t *testing.T) {
	mw := EnvMiddleware(
		map[string]string{"debug": "DEBUG"},
		mapLookup(map[string]string{"DEBUG": "maybe"}),
	)
	call := NewCall(context.Background(), nil)

	err := mw(newDebugFlagCommand()).RunArgv(&Output{Stdout: io.Discard, Stderr: io.Discard}, call)
	if err == nil {
		t.Fatal("expected error for unparseable boolean")
	}
	if !strings.Contains(err.Error(), "DEBUG") {
		t.Fatalf("error should mention env var name, got %q", err)
	}
}

func TestMuxMatch(t *testing.T) {
	mux := &Mux{}
	deploy := &Command{Run: func(*Output, *Call) error { return nil }}
	mux.Handle("deploy", "Deploy", deploy)

	sub := &Mux{}
	initRunner := RunnerFunc(func(*Output, *Call) error { return nil })
	sub.Handle("init", "", initRunner)
	mux.Handle("repo", "", sub)

	cases := []struct {
		name     string
		tokens   []string
		wantRun  Runner
		wantPath string
	}{
		{"direct command", []string{"deploy"}, deploy, "deploy"},
		{"sub-mux matched shallowly", []string{"repo"}, sub, "repo"},
		{"sub-mux with extra tokens stays shallow", []string{"repo", "init"}, sub, "repo"},
		{"extra tokens past command", []string{"deploy", "extra"}, deploy, "deploy"},
		{"no match at root", []string{"nope"}, nil, ""},
		{"empty tokens with no root handler", nil, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, gotPath := mux.Match(tc.tokens)
			if got != tc.wantRun {
				t.Fatalf("got runner %v, want %v", got, tc.wantRun)
			}
			if gotPath != tc.wantPath {
				t.Fatalf("got path %q, want %q", gotPath, tc.wantPath)
			}
		})
	}
}

func TestMuxMatchRootHandler(t *testing.T) {
	mux := &Mux{}
	root := &Command{Run: func(*Output, *Call) error { return nil }}
	mux.Handle("", "", root)

	got, path := mux.Match(nil)
	if got != root || path != "" {
		t.Fatalf("got (%v, %q), want (root, %q)", got, path, "")
	}
}

func TestRoutingSharesCallState(t *testing.T) {
	// Dispatch mutates the caller's Call in place: parsed flags and
	// options are merged into the same maps, and handler mutations
	// are visible after RunArgv returns. Callers that need isolation
	// construct a fresh Call per invocation.
	root := &Mux{}
	root.Option("tag", "", "", "tag")
	sub := &Mux{}
	sub.Handle("show", "", RunnerFunc(func(out *Output, call *Call) error {
		call.Options.Set("tag", "mutated")
		_, err := fmt.Fprint(out.Stdout, call.Options.Get("tag"))
		return err
	}))
	root.Handle("repo", "", sub)

	call := NewCall(context.Background(), []string{"--tag", "original", "repo", "show"})

	var out bytes.Buffer
	if err := root.RunArgv(&Output{Stdout: &out, Stderr: io.Discard}, call); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "mutated" {
		t.Fatalf("got %q", got)
	}
	if got := call.Options.Get("tag"); got != "mutated" {
		t.Fatalf("expected shared mutation, got %q", got)
	}
}
