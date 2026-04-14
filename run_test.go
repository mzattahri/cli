package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
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
		_, err := out.Stdout.Write([]byte(call.Args["msg"] + ":" + value))
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
