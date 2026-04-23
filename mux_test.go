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
	call := NewCall(ctx, args)
	return mux.RunCLI(&Output{Stdout: stdout, Stderr: stderr}, call)
}

func TestBasicDispatch(t *testing.T) {
	mux := NewMux("app")
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

func TestHandleFunc(t *testing.T) {
	mux := NewMux("app")
	mux.HandleFunc("greet", "Say hello", func(out *Output, call *Call) error {
		_, err := fmt.Fprint(out.Stdout, "hello")
		return err
	})
	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"greet"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestCommandFlagsAndOptions(t *testing.T) {
	mux := NewMux("app")
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
	mux := NewMux("app")
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
	mux := NewMux("app")
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
	mux := NewMux("app")
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
	mux := NewMux("app")
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

func TestCaptureRestPreservesTrailingArgs(t *testing.T) {
	mux := NewMux("app")
	cmd := &Command{
		CaptureRest: true,
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "%v", call.Rest)
			return err
		},
	}
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
	mux := NewMux("app")
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

func TestCaptureRestPreservesLiteralDoubleDash(t *testing.T) {
	mux := NewMux("app")
	cmd := &Command{
		CaptureRest: true,
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "value=%q rest=%q", call.Args.Get("value"), call.Rest)
			return err
		},
	}
	cmd.Arg("value", "Leading value")
	mux.Handle("echo", "", cmd)

	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"echo", "--", "--", "tail"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != `value="--" rest=["tail"]` {
		t.Fatalf("got %q", got)
	}
}

func TestProgramGlobalFlagsAndOptions(t *testing.T) {
	mux := NewMux("app")
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
	root := NewMux("app")
	root.Flag("verbose", "v", false, "verbose")
	root.Option("config", "c", "", "config file")
	sub := NewMux("repo")
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
	globalFlags := slices.Collect(gotHelp.GlobalFlags())
	if len(globalFlags) != 1 || globalFlags[0].Name != "verbose" {
		t.Fatalf("got global flags %#v", globalFlags)
	}
	globalOptions := slices.Collect(gotHelp.GlobalOptions())
	if len(globalOptions) != 1 || globalOptions[0].Name != "config" {
		t.Fatalf("got global options %#v", globalOptions)
	}
}

func TestNestedCommands(t *testing.T) {
	mux := NewMux("app")
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
	sub := NewMux("repo")
	sub.Handle("init", "Initialize", RunnerFunc(func(out *Output, call *Call) error {
		_, err := io.WriteString(out.Stdout, "repo-init")
		return err
	}))
	mux := NewMux("app")
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
	mux := NewMux("app")
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
	mux := NewMux("app")
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
	mux := NewMux("app")
	mux.Handle("greet", "Say hello", RunnerFunc(func(out *Output, call *Call) error { return nil }))

	var errout bytes.Buffer
	program := &Program{
		Stdout: io.Discard,
		Stderr: &errout,
		HelpFunc: func(w io.Writer, help *Help) error {
			_, _ = io.WriteString(w, "custom help")
			return nil
		},
	}
	err := program.Invoke(context.Background(), mux, []string{"app", "--help"})
	if err != nil {
		t.Fatalf("got err=%v", err)
	}
	if got := errout.String(); got != "custom help" {
		t.Fatalf("got %q", got)
	}
}

func TestHelpIncludesOptionsAndArgs(t *testing.T) {
	mux := NewMux("app")
	cmd := &Command{
		Run: func(out *Output, call *Call) error { return nil },
	}
	cmd.Option("repository", "", "", "repo path")
	cmd.Arg("path", "Path to open")
	mux.Handle("open", "Open files", cmd)
	var errout bytes.Buffer
	if err := runMux(context.Background(), mux, io.Discard, &errout, []string{"open", "--help"}); err != nil {
		t.Fatalf("got err=%v", err)
	}
	help := errout.String()
	for _, want := range []string{"--repository", "<path>"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

func TestCommandRestHoldsUnparsedTokens(t *testing.T) {
	mux := NewMux("app")
	cmd := &Command{
		CaptureRest: true,
		Run: func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "repo=%s rest=%v", call.Options.Get("repository"), call.Rest)
			return err
		},
	}
	cmd.Option("repository", "", "", "repo path")
	mux.Handle("open", "", cmd)

	var out bytes.Buffer
	if err := runMux(context.Background(), mux, &out, io.Discard, []string{"open", "--repository", "/tmp/repo", "README.md"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "repo=/tmp/repo rest=[README.md]" {
		t.Fatalf("got %q", got)
	}
}

func TestCustomHelpGetsRootName(t *testing.T) {
	mux := NewMux("app")

	var errout bytes.Buffer
	program := &Program{
		Stdout: io.Discard,
		Stderr: &errout,
		HelpFunc: func(w io.Writer, help *Help) error {
			_, _ = fmt.Fprintf(w, "%s|%s", help.Name, help.FullPath)
			return nil
		},
	}
	if err := program.Invoke(context.Background(), mux, []string{"app", "--help"}); err != nil {
		t.Fatalf("got err=%v", err)
	}
	if got := errout.String(); got != "app|app" {
		t.Fatalf("got %q", got)
	}
}

func TestHelpDoesNotShadowOptionValue(t *testing.T) {
	mux := NewMux("app")
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
	root := NewMux("app")
	root.Option("host", "", "", "daemon socket")
	sub := NewMux("repo")
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


func TestProgramMuxRootHandlerWithGlobalOptions(t *testing.T) {
	mux := NewMux("app")
	mux.Option("host", "", "", "daemon socket")
	mux.HandleFunc("", "Run the root command", func(out *Output, call *Call) error {
		host := call.Options.Get("host")
		_, err := fmt.Fprintf(out.Stdout, "%s", host)
		return err
	})

	var out bytes.Buffer
	var errout bytes.Buffer
	program := &Program{
		Stdout: &out,
		Stderr: &errout,
		Usage:  "Run the root command",
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
	if !strings.Contains(errout.String(), "--host") {
		t.Fatalf("help missing global option:\n%s", errout.String())
	}
}

func TestMuxRejectsFlagOptionNameCollision(t *testing.T) {
	mux := NewMux("app")
	mux.Flag("name", "", false, "flag")
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	mux.Option("name", "", "", "option")
}

func TestProgramMuxRejectsFlagOptionNameCollision(t *testing.T) {
	mux := NewMux("app")
	mux.Flag("name", "", false, "flag")

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	mux.Option("name", "", "", "option")
}

func TestMuxFlagAndOption(t *testing.T) {
	mux := NewMux("app")
	mux.Flag("verbose", "v", false, "verbose")
	mux.Option("host", "", "", "daemon socket")
	mux.Handle("run", "", RunnerFunc(func(out *Output, call *Call) error {
		_, err := fmt.Fprintf(out.Stdout, "%s|%t", call.Options.Get("host"), call.Flags.Get("verbose"))
		return err
	}))
	var out bytes.Buffer
	call := NewCall(context.Background(), []string{"--host", "localhost", "--verbose", "run"})
	if err := mux.RunCLI(&Output{Stdout: &out, Stderr: io.Discard}, call); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "localhost|true" {
		t.Fatalf("got %q", got)
	}
}

func TestMountedMuxScopedFlags(t *testing.T) {
	root := NewMux("app")
	root.Flag("verbose", "v", false, "verbose")
	sub := NewMux("repo")
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
	root := NewMux("app")
	root.Flag("verbose", "v", false, "verbose")
	sub := NewMux("repo")
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
	globalFlags := slices.Collect(gotHelp.GlobalFlags())
	if len(globalFlags) != 1 || globalFlags[0].Name != "verbose" {
		t.Fatalf("got global flags %#v", globalFlags)
	}
	globalOptions := slices.Collect(gotHelp.GlobalOptions())
	if len(globalOptions) != 1 || globalOptions[0].Name != "repository" {
		t.Fatalf("got global options %#v", globalOptions)
	}
}

func TestNegateFlagsCommand(t *testing.T) {
	mux := NewMux("app")
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
	mux := NewMux("app")
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
	mux := NewMux("app")
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
	mux := NewMux("app")
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
	mux := NewMux("app")
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
	mux := NewMux("app")
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

func TestNegateFlagsHelpShowsBothForms(t *testing.T) {
	mux := NewMux("app")
	cmd := &Command{
		NegateFlags: true,
		Run:         func(*Output, *Call) error { return nil },
	}
	cmd.Flag("accept-dns", "", true, "accept DNS")
	cmd.Flag("no-cache", "", true, "disable cache")
	mux.Handle("up", "Connect", cmd)

	var errout bytes.Buffer
	if err := runMux(context.Background(), mux, io.Discard, &errout, []string{"up", "--help"}); err != nil {
		t.Fatalf("got err=%v", err)
	}
	help := errout.String()
	if !strings.Contains(help, "--no-accept-dns") {
		t.Fatalf("help missing --no-accept-dns:\n%s", help)
	}
	if !strings.Contains(help, "--cache") {
		t.Fatalf("help missing --cache (negation of --no-cache):\n%s", help)
	}
}

func TestRepeatedOptionAccumulates(t *testing.T) {
	mux := NewMux("app")
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
	mux := NewMux("app")
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
	mux := NewMux("app")
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
	mux := NewMux("app")
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

func TestEnvMap(t *testing.T) {
	env := map[string]string{
		"APP_HOST": "env-host",
		"VERBOSE":  "1",
	}
	middleware := EnvMiddleware(
		map[string]string{"verbose": "VERBOSE"},
		map[string]string{"host": "APP_HOST"},
		NewLookupFunc(env),
	)

	t.Run("fills missing values", func(t *testing.T) {
		call := NewCall(context.Background(), nil)

		inner := RunnerFunc(func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "host=%s verbose=%t",
				call.Options.Get("host"), call.Flags.Get("verbose"))
			return err
		})

		var out bytes.Buffer
		err := middleware(inner).RunCLI(&Output{Stdout: &out, Stderr: io.Discard}, call)
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

		inner := RunnerFunc(func(out *Output, call *Call) error {
			_, err := fmt.Fprintf(out.Stdout, "host=%s verbose=%t",
				call.Options.Get("host"), call.Flags.Get("verbose"))
			return err
		})

		var out bytes.Buffer
		err := middleware(inner).RunCLI(&Output{Stdout: &out, Stderr: io.Discard}, call)
		if err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != "host=cli-host verbose=false" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestEnvMapParsesBooleanValues(t *testing.T) {
	noop := RunnerFunc(func(out *Output, call *Call) error { return nil })

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
				nil,
				NewLookupFunc(map[string]string{"DEBUG": tc.val}),
			)
			call := NewCall(context.Background(), nil)
			if err := mw(noop).RunCLI(&Output{Stdout: io.Discard, Stderr: io.Discard}, call); err != nil {
				t.Fatal(err)
			}
			if got := call.Flags.Get("debug"); got != tc.want {
				t.Fatalf("got %t, want %t", got, tc.want)
			}
		})
	}
}

func TestEnvMapEmptyStringSkipsFlag(t *testing.T) {
	noop := RunnerFunc(func(out *Output, call *Call) error { return nil })
	mw := EnvMiddleware(
		map[string]string{"debug": "DEBUG"},
		nil,
		NewLookupFunc(map[string]string{"DEBUG": ""}),
	)
	call := NewCall(context.Background(), nil)

	if err := mw(noop).RunCLI(&Output{Stdout: io.Discard, Stderr: io.Discard}, call); err != nil {
		t.Fatal(err)
	}
	if call.Flags.Has("debug") {
		t.Fatalf("empty env value should leave flag unset, got Flags=%v", call.Flags)
	}
}

func TestEnvMapInvalidBooleanReturnsError(t *testing.T) {
	noop := RunnerFunc(func(out *Output, call *Call) error { return nil })
	mw := EnvMiddleware(
		map[string]string{"debug": "DEBUG"},
		nil,
		NewLookupFunc(map[string]string{"DEBUG": "maybe"}),
	)
	call := NewCall(context.Background(), nil)

	err := mw(noop).RunCLI(&Output{Stdout: io.Discard, Stderr: io.Discard}, call)
	if err == nil {
		t.Fatal("expected error for unparseable boolean")
	}
	if !strings.Contains(err.Error(), "DEBUG") {
		t.Fatalf("error should mention env var name, got %q", err)
	}
}

func TestMuxMatch(t *testing.T) {
	mux := NewMux("app")
	deploy := &Command{Run: func(*Output, *Call) error { return nil }}
	mux.Handle("deploy", "Deploy", deploy)

	sub := NewMux("repo")
	initRunner := RunnerFunc(func(*Output, *Call) error { return nil })
	sub.Handle("init", "", initRunner)
	mux.Handle("repo", "", sub)

	cases := []struct {
		name     string
		tokens   []string
		wantRun  Runner
		wantPath string
	}{
		{"direct command", []string{"deploy"}, deploy, "app deploy"},
		{"sub-mux matched shallowly", []string{"repo"}, sub, "app repo"},
		{"sub-mux with extra tokens stays shallow", []string{"repo", "init"}, sub, "app repo"},
		{"extra tokens past command", []string{"deploy", "extra"}, deploy, "app deploy"},
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
	mux := NewMux("app")
	root := &Command{Run: func(*Output, *Call) error { return nil }}
	mux.Handle("", "", root)

	got, path := mux.Match(nil)
	if got != root || path != "app" {
		t.Fatalf("got (%v, %q), want (root, %q)", got, path, "app")
	}
}

func TestRoutingCallDeepCopiesOptionSlices(t *testing.T) {
	root := NewMux("app")
	root.Option("tag", "", "", "tag")
	sub := NewMux("repo")
	sub.Handle("show", "", RunnerFunc(func(out *Output, call *Call) error {
		call.Options.Set("tag", "mutated")
		_, err := fmt.Fprint(out.Stdout, call.Options.Get("tag"))
		return err
	}))
	root.Handle("repo", "", sub)

	call := NewCall(context.Background(), []string{"--tag", "original", "repo", "show"})
	call.Options.Set("tag", "caller")

	var out bytes.Buffer
	if err := root.RunCLI(&Output{Stdout: &out, Stderr: io.Discard}, call); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "mutated" {
		t.Fatalf("got %q", got)
	}
	if got := call.Options.Get("tag"); got != "caller" {
		t.Fatalf("caller options mutated: got %q", got)
	}
}
