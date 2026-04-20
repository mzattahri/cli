package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestInvokeDefaultsNilTTYAndStdin(t *testing.T) {
	mux := NewMux("app")
	mux.Handle("noop", "", RunnerFunc(func(out *Output, call *Call) error {
		if out.Stdout == nil {
			t.Fatal("expected non-nil stdout from default Output")
		}
		if out.Stderr == nil {
			t.Fatal("expected non-nil stderr from default Output")
		}
		if call.Stdin == nil {
			t.Fatal("expected non-nil stdin from default")
		}
		return nil
	}))

	program := &Program{}
	if err := program.Invoke(context.Background(), mux, []string{"app", "noop"}); err != nil {
		t.Fatal(err)
	}
}

func TestInvokeSkipsArgv0(t *testing.T) {
	mux := NewMux("app")
	cmd := &Command{Run: func(out *Output, call *Call) error {
		value, _ := call.Env("TERMINAL_TEST_VALUE")
		_, err := out.Stdout.Write([]byte(call.Args.Get("msg") + ":" + value))
		return err
	}}
	cmd.Arg("msg", "message")
	mux.Handle("echo", "", cmd)

	t.Setenv("TERMINAL_TEST_VALUE", "ok")

	var out bytes.Buffer
	program := &Program{Stdout: &out, Stderr: &bytes.Buffer{}, Env: os.LookupEnv}
	err := program.Invoke(context.Background(), mux, []string{"app", "echo", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "hello:ok" {
		t.Fatalf("got %q, want %q", got, "hello:ok")
	}
}

func TestInvokeExplicitHelpReturnsSuccess(t *testing.T) {
	mux := NewMux("app")
	mux.Handle("echo", "Echo output", RunnerFunc(func(out *Output, call *Call) error { return nil }))

	var errout bytes.Buffer
	program := &Program{Stdout: io.Discard, Stderr: &errout}
	err := program.Invoke(context.Background(), mux, []string{"app", "--help"})
	if err != nil {
		t.Fatalf("got err=%v, want nil", err)
	}
	if got := errout.String(); got == "" {
		t.Fatal("expected help output")
	}
}

func TestInvokeWithPlainRunner(t *testing.T) {
	runner := RunnerFunc(func(out *Output, call *Call) error {
		_, err := io.WriteString(out.Stdout, "plain")
		return err
	})

	var stdout bytes.Buffer
	program := &Program{Stdout: &stdout, Stderr: &bytes.Buffer{}}
	if err := program.Invoke(context.Background(), runner, []string{"app"}); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "plain" {
		t.Fatalf("got %q, want %q", got, "plain")
	}
}

func TestInvokeWithPlainRunnerHelpFlag(t *testing.T) {
	runner := RunnerFunc(func(out *Output, call *Call) error {
		return nil
	})

	var stderr bytes.Buffer
	program := &Program{Stdout: &bytes.Buffer{}, Stderr: &stderr, Usage: "A test runner"}
	err := program.Invoke(context.Background(), runner, []string{"app", "--help"})
	if err != nil {
		t.Fatalf("got err=%v, want nil", err)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("expected help output, got %q", stderr.String())
	}
}

func TestInvokeEmptyArgs(t *testing.T) {
	mux := NewMux("app")
	mux.Handle("noop", "Do nothing", RunnerFunc(func(out *Output, call *Call) error {
		return nil
	}))

	program := &Program{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	err := program.Invoke(context.Background(), mux, nil)
	if err == nil || !errors.Is(err, ErrHelp) {
		t.Fatalf("got err=%v, want ErrHelp", err)
	}
}

func TestInvokeEmptyArgsFallbackToMuxName(t *testing.T) {
	var gotHelp *Help
	mux := NewMux("myapp")
	mux.Handle("noop", "Do nothing", RunnerFunc(func(out *Output, call *Call) error {
		return nil
	}))

	program := &Program{
		Stdout:   &bytes.Buffer{},
		Stderr:   &bytes.Buffer{},
		HelpFunc: func(_ io.Writer, help *Help) error { gotHelp = help; return nil },
	}
	err := program.Invoke(context.Background(), mux, nil)
	if err == nil || !errors.Is(err, ErrHelp) {
		t.Fatalf("got err=%v, want ErrHelp", err)
	}
	if gotHelp == nil {
		t.Fatal("expected help to be rendered")
	}
	if gotHelp.FullPath != "myapp" {
		t.Fatalf("got fullpath %q, want %q", gotHelp.FullPath, "myapp")
	}
}

func TestInvokeEmptyArgsFallbackToApp(t *testing.T) {
	runner := RunnerFunc(func(out *Output, call *Call) error {
		_, err := io.WriteString(out.Stdout, "ok")
		return err
	})

	var stdout bytes.Buffer
	program := &Program{Stdout: &stdout, Stderr: &bytes.Buffer{}}
	if err := program.Invoke(context.Background(), runner, nil); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "ok" {
		t.Fatalf("got %q, want %q", got, "ok")
	}
}

func TestWalkPlainRunner(t *testing.T) {
	runner := RunnerFunc(func(out *Output, call *Call) error { return nil })
	program := &Program{Name: "app", Usage: "A test app"}

	var paths []string
	for path, help := range program.Walk(runner) {
		paths = append(paths, path)
		if help.Name != "app" {
			t.Fatalf("got name %q", help.Name)
		}
		if help.Usage != "A test app" {
			t.Fatalf("got usage %q", help.Usage)
		}
	}
	if len(paths) != 1 || paths[0] != "app" {
		t.Fatalf("got paths %v", paths)
	}
}

func TestWalkMux(t *testing.T) {
	mux := NewMux("app")
	mux.Flag("verbose", "v", false, "verbose")

	deployCmd := &Command{
		Description: "Deploy the app",
		Run:         func(*Output, *Call) error { return nil },
	}
	deployCmd.Flag("force", "f", false, "force")
	deployCmd.Arg("target", "deploy target")
	mux.Handle("deploy", "Deploy", deployCmd)

	mux.Handle("version", "Print version", RunnerFunc(func(*Output, *Call) error { return nil }))

	program := &Program{Usage: "A CLI tool"}

	var paths []string
	helpByPath := map[string]*Help{}
	for path, help := range program.Walk(mux) {
		paths = append(paths, path)
		helpByPath[path] = help
	}

	wantPaths := []string{"app", "app deploy", "app version"}
	if !slices.Equal(paths, wantPaths) {
		t.Fatalf("got paths %v, want %v", paths, wantPaths)
	}

	// Root has usage and commands.
	root := helpByPath["app"]
	if root.Usage != "A CLI tool" {
		t.Fatalf("got root usage %q", root.Usage)
	}
	if len(root.Commands) != 2 {
		t.Fatalf("got %d commands, want 2", len(root.Commands))
	}

	// Deploy has global flag, local flag, and argument.
	deploy := helpByPath["app deploy"]
	if deploy.Description != "Deploy the app" {
		t.Fatalf("got description %q", deploy.Description)
	}
	globalFlags := filterFlags(deploy.Flags, true)
	localFlags := filterFlags(deploy.Flags, false)
	if len(globalFlags) != 1 || globalFlags[0].Name != "verbose" {
		t.Fatalf("got global flags %v", globalFlags)
	}
	if len(localFlags) != 1 || localFlags[0].Name != "force" {
		t.Fatalf("got local flags %v", localFlags)
	}
	if len(deploy.Arguments) != 1 || deploy.Arguments[0].Name != "<target>" {
		t.Fatalf("got arguments %v", deploy.Arguments)
	}
}

func TestWalkMountedMux(t *testing.T) {
	root := NewMux("app")
	root.Flag("verbose", "v", false, "verbose")

	sub := NewMux("repo")
	sub.Option("path", "p", ".", "repo path")
	sub.Handle("init", "Initialize", RunnerFunc(func(*Output, *Call) error { return nil }))
	sub.Handle("clone", "Clone", RunnerFunc(func(*Output, *Call) error { return nil }))
	root.Handle("repo", "Repository operations", sub)

	program := &Program{}

	var paths []string
	helpByPath := map[string]*Help{}
	for path, help := range program.Walk(root) {
		paths = append(paths, path)
		helpByPath[path] = help
	}

	wantPaths := []string{"app", "app repo", "app repo clone", "app repo init"}
	if !slices.Equal(paths, wantPaths) {
		t.Fatalf("got paths %v, want %v", paths, wantPaths)
	}

	// Sub-mux commands inherit root's global flags.
	init := helpByPath["app repo init"]
	globalFlags := filterFlags(init.Flags, true)
	globalOptions := filterOptions(init.Options, true)
	if len(globalFlags) != 1 || globalFlags[0].Name != "verbose" {
		t.Fatalf("got global flags %v", globalFlags)
	}
	if len(globalOptions) != 1 || globalOptions[0].Name != "path" {
		t.Fatalf("got global options %v", globalOptions)
	}
}

func TestWalkMultiSegmentPattern(t *testing.T) {
	mux := NewMux("app")
	mux.Handle("repo init", "Initialize a repository", RunnerFunc(func(*Output, *Call) error { return nil }))
	mux.Handle("repo clone", "Clone a repository", RunnerFunc(func(*Output, *Call) error { return nil }))

	program := &Program{}

	var paths []string
	for path := range program.Walk(mux) {
		paths = append(paths, path)
	}

	wantPaths := []string{"app", "app repo", "app repo clone", "app repo init"}
	if !slices.Equal(paths, wantPaths) {
		t.Fatalf("got paths %v, want %v", paths, wantPaths)
	}
}

func TestWalkEarlyTermination(t *testing.T) {
	mux := NewMux("app")
	mux.Handle("a", "First", RunnerFunc(func(*Output, *Call) error { return nil }))
	mux.Handle("b", "Second", RunnerFunc(func(*Output, *Call) error { return nil }))
	mux.Handle("c", "Third", RunnerFunc(func(*Output, *Call) error { return nil }))

	program := &Program{}
	count := 0
	for range program.Walk(mux) {
		count++
		if count == 2 {
			break
		}
	}
	if count != 2 {
		t.Fatalf("got %d iterations, want 2", count)
	}
}
