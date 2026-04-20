package clitest_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/mzattahri/cli"
	"github.com/mzattahri/cli/clitest"
)

func TestCallCapturesStdout(t *testing.T) {
	mux := cli.NewMux("app")
	cmd := &cli.Command{
		CaptureRest: true,
		Run: cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
			_, err := fmt.Fprint(out.Stdout, strings.Join(call.Rest, ","))
			return err
		}),
	}
	mux.Handle("echo", "", cmd)

	recorder := clitest.NewRecorder()
	call := clitest.NewCall("echo a b", nil)
	err := mux.RunCLI(recorder.Output(), call)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := recorder.Stdout.String(); got != "a,b" {
		t.Fatalf("got stdout %q, want %q", got, "a,b")
	}
	if got := recorder.Stderr.String(); got != "" {
		t.Fatalf("got stderr %q, want empty", got)
	}
}

func TestCallPassesStdin(t *testing.T) {
	mux := cli.NewMux("app")
	mux.Handle("cat", "", cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		_, err := io.Copy(out.Stdout, call.Stdin)
		return err
	}))

	recorder := clitest.NewRecorder()
	call := clitest.NewCall("cat", []byte("piped input"))
	err := mux.RunCLI(recorder.Output(), call)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := recorder.Stdout.String(); got != "piped input" {
		t.Fatalf("got stdout %q, want %q", got, "piped input")
	}
}

func TestCallMapsUsageToErrHelp(t *testing.T) {
	mux := cli.NewMux("app")
	mux.Handle("greet", "", cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		return nil
	}))

	recorder := clitest.NewRecorder()
	call := clitest.NewCall("", nil)
	err := mux.RunCLI(recorder.Output(), call)
	if !errors.Is(err, cli.ErrHelp) {
		t.Fatalf("got err=%v, want ErrHelp", err)
	}
	if got := recorder.Stdout.String(); got != "" {
		t.Fatalf("got stdout %q, want empty", got)
	}
	if got := recorder.Stderr.String(); !strings.Contains(got, "Usage:") {
		t.Fatalf("expected usage in stderr, got %q", got)
	}
}

func TestWrappedUsageIsHelp(t *testing.T) {
	runner := cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		return fmt.Errorf("wrapped: %w", cli.ErrHelp)
	})

	recorder := clitest.NewRecorder()
	err := runner.RunCLI(recorder.Output(), clitest.NewCall("", nil))
	if !errors.Is(err, cli.ErrHelp) {
		t.Fatalf("got err=%v, want wrapped ErrHelp", err)
	}
	if got := recorder.Stderr.String(); got != "" {
		t.Fatalf("got stderr %q, want empty", got)
	}
}

func TestDefaultErrorsAreReturned(t *testing.T) {
	mux := cli.NewMux("app")
	mux.Handle("fail", "", cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		return errors.New("boom")
	}))

	recorder := clitest.NewRecorder()
	err := mux.RunCLI(recorder.Output(), clitest.NewCall("fail", nil))
	if err == nil || err.Error() != "boom" {
		t.Fatalf("got err=%v, want %q", err, "boom")
	}
	if got := recorder.Stderr.String(); got != "" {
		t.Fatalf("got stderr %q, want empty", got)
	}
}

func TestExitErrorsAreReturned(t *testing.T) {
	mux := cli.NewMux("app")
	mux.Handle("fail", "", cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		return &cli.ExitError{Code: 9, Err: errors.New("denied")}
	}))

	recorder := clitest.NewRecorder()
	err := mux.RunCLI(recorder.Output(), clitest.NewCall("fail", nil))
	var exitErr *cli.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 9 || exitErr.Err.Error() != "denied" {
		t.Fatalf("got err=%v, want ExitError(code=9, err=denied)", err)
	}
}

func TestCallSupportsDirectInputs(t *testing.T) {
	type ctxKey struct{}

	call := clitest.NewCall("run --raw", []byte("stdin"))
	call = call.WithContext(context.WithValue(context.Background(), ctxKey{}, "ctx"))
	call.Flags.Set("verbose", true)
	call.Flags.Set("force", true)
	call.Options.Set("host", "unix:///tmp/docker.sock")
	call.Options.Set("output", "json")
	call.Args.Set("name", "alice")
	call.Args.Set("command", "sh -c echo hi")
	call.Env = func(key string) (string, bool) {
		if key == "HOME" {
			return "/tmp/home", true
		}
		return "", false
	}

	if got, ok := call.Env("HOME"); !ok || got != "/tmp/home" {
		t.Fatalf("got (%q, %t)", got, ok)
	}
	if got := call.Flags.Get("verbose"); !got {
		t.Fatalf("got %t", got)
	}
	if got := call.Options.Get("host"); got != "unix:///tmp/docker.sock" {
		t.Fatalf("got %q", got)
	}
	if got := call.Flags.Get("force"); !got {
		t.Fatalf("got %t", got)
	}
	if got := call.Options.Get("output"); got != "json" {
		t.Fatalf("got %q", got)
	}
	if got := call.Args.Get("name"); got != "alice" {
		t.Fatalf("got %q", got)
	}
	if got := call.Args.Get("command"); got != "sh -c echo hi" {
		t.Fatalf("got %q", got)
	}
	if got := call.Context().Value(ctxKey{}); got != "ctx" {
		t.Fatalf("got %v", got)
	}
}

func TestRecorderLen(t *testing.T) {
	r := clitest.NewRecorder()
	if r.Len() != 0 {
		t.Fatalf("got %d, want 0", r.Len())
	}
	r.Stdout.WriteString("hello")
	r.Stderr.WriteString("err")
	if got := r.Len(); got != 8 {
		t.Fatalf("got %d, want 8", got)
	}
}

func TestRecorderReset(t *testing.T) {
	r := clitest.NewRecorder()
	r.Stdout.WriteString("hello")
	r.Stderr.WriteString("err")
	r.Reset()
	if r.Len() != 0 {
		t.Fatalf("got %d after reset, want 0", r.Len())
	}
	if r.Stdout.String() != "" {
		t.Fatal("stdout not empty after reset")
	}
	if r.Stderr.String() != "" {
		t.Fatal("stderr not empty after reset")
	}
}
