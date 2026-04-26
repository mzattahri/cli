package argvtest_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"mz.attahri.com/code/argv"
	"mz.attahri.com/code/argv/argvtest"
)

func TestCallCapturesStdout(t *testing.T) {
	mux := &argv.Mux{}
	cmd := &argv.Command{
		Variadic: true,
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprint(out.Stdout, strings.Join(call.Tail, ","))
			return err
		},
	}
	mux.Handle("echo", "", cmd)

	recorder := argvtest.NewRecorder()
	call := argvtest.NewCall("echo a b")
	err := mux.RunArgv(recorder.Output(), call)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := recorder.Stdout(); got != "a,b" {
		t.Fatalf("got stdout %q, want %q", got, "a,b")
	}
	if got := recorder.Stderr(); got != "" {
		t.Fatalf("got stderr %q, want empty", got)
	}
}

func TestCallPassesStdin(t *testing.T) {
	mux := &argv.Mux{}
	mux.Handle("cat", "", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := io.Copy(out.Stdout, call.Stdin)
		return err
	}))

	recorder := argvtest.NewRecorder()
	call := argvtest.NewCall("cat")
	call.Stdin = bytes.NewReader([]byte("piped input"))
	err := mux.RunArgv(recorder.Output(), call)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := recorder.Stdout(); got != "piped input" {
		t.Fatalf("got stdout %q, want %q", got, "piped input")
	}
}

func TestCallMapsUsageToErrHelp(t *testing.T) {
	mux := &argv.Mux{}
	mux.Handle("greet", "", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		return nil
	}))

	var stdout, stderr bytes.Buffer
	program := &argv.Program{Stdout: &stdout, Stderr: &stderr}
	err := program.Invoke(context.Background(), mux, []string{"app"})
	if !errors.Is(err, argv.ErrHelp) {
		t.Fatalf("got err=%v, want ErrHelp", err)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("got stdout %q, want empty", got)
	}
	if got := stderr.String(); !strings.Contains(got, "Usage:") {
		t.Fatalf("expected usage in stderr, got %q", got)
	}
}

func TestWrappedUsageIsHelp(t *testing.T) {
	runner := argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		return fmt.Errorf("wrapped: %w", argv.ErrHelp)
	})

	recorder := argvtest.NewRecorder()
	err := runner.RunArgv(recorder.Output(), argvtest.NewCall(""))
	if !errors.Is(err, argv.ErrHelp) {
		t.Fatalf("got err=%v, want wrapped ErrHelp", err)
	}
	if got := recorder.Stderr(); got != "" {
		t.Fatalf("got stderr %q, want empty", got)
	}
}

func TestDefaultErrorsAreReturned(t *testing.T) {
	mux := &argv.Mux{}
	mux.Handle("fail", "", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		return errors.New("boom")
	}))

	recorder := argvtest.NewRecorder()
	err := mux.RunArgv(recorder.Output(), argvtest.NewCall("fail"))
	if err == nil || err.Error() != "boom" {
		t.Fatalf("got err=%v, want %q", err, "boom")
	}
	if got := recorder.Stderr(); got != "" {
		t.Fatalf("got stderr %q, want empty", got)
	}
}

func TestExitErrorsAreReturned(t *testing.T) {
	mux := &argv.Mux{}
	mux.Handle("fail", "", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		return &argv.ExitError{Code: 9, Err: errors.New("denied")}
	}))

	recorder := argvtest.NewRecorder()
	err := mux.RunArgv(recorder.Output(), argvtest.NewCall("fail"))
	var exitErr *argv.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 9 || exitErr.Err.Error() != "denied" {
		t.Fatalf("got err=%v, want ExitError(code=9, err=denied)", err)
	}
}

func TestCallSupportsDirectInputs(t *testing.T) {
	type ctxKey struct{}

	call := argvtest.NewCall("run --raw")
	call.Stdin = bytes.NewReader([]byte("stdin"))
	call = call.WithContext(context.WithValue(context.Background(), ctxKey{}, "ctx"))
	call.Flags.Set("verbose", true)
	call.Flags.Set("force", true)
	call.Options.Set("host", "unix:///tmp/docker.sock")
	call.Options.Set("output", "json")
	call.Args.Set("name", "alice")
	call.Args.Set("command", "sh -c echo hi")

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

func TestNewCallQuoting(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"echo a b", []string{"echo", "a", "b"}},
		{`echo "hello world"`, []string{"echo", "hello world"}},
		{"echo 'hello world'", []string{"echo", "hello world"}},
		{`echo "a \"quoted\" word"`, []string{"echo", `a "quoted" word`}},
		{`echo "back\\slash"`, []string{"echo", `back\slash`}},
		{"echo '\\no escapes\\'", []string{"echo", `\no escapes\`}},
		{`a""b`, []string{"ab"}},
		{`  leading   "and trailing"  `, []string{"leading", "and trailing"}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			call := argvtest.NewCall(tc.input)
			got := call.Argv()
			if len(got) != len(tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("token %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestNewCallArgs(t *testing.T) {
	// Pre-tokenized; no shell parsing, values with internal whitespace
	// arrive verbatim.
	call := argvtest.NewCallArgs([]string{"echo", "hello world", "--flag"})
	if got := call.Argv(); len(got) != 3 || got[0] != "echo" || got[1] != "hello world" || got[2] != "--flag" {
		t.Fatalf("got %q", got)
	}
}

func TestNewCallUnclosedQuotePanics(t *testing.T) {
	defer func() {
		got := recover()
		if got == nil {
			t.Fatal("expected panic")
		}
		if s, ok := got.(string); !ok || !strings.Contains(s, "unclosed quote") {
			t.Fatalf("got panic %v", got)
		}
	}()
	argvtest.NewCall(`echo "oops`)
}

func TestRecorderLen(t *testing.T) {
	r := argvtest.NewRecorder()
	if r.Len() != 0 {
		t.Fatalf("got %d, want 0", r.Len())
	}
	out := r.Output()
	fmt.Fprint(out.Stdout, "hello")
	fmt.Fprint(out.Stderr, "err")
	if got := r.Len(); got != 8 {
		t.Fatalf("got %d, want 8", got)
	}
}

func TestNewLookupFuncNil(t *testing.T) {
	lookup := argvtest.NewLookupFunc(nil)
	if v, ok := lookup("ANYTHING"); ok || v != "" {
		t.Fatalf("got (%q, %v), want (\"\", false)", v, ok)
	}
}

func TestRecorderReset(t *testing.T) {
	r := argvtest.NewRecorder()
	out := r.Output()
	fmt.Fprint(out.Stdout, "hello")
	fmt.Fprint(out.Stderr, "err")
	r.Reset()
	if r.Len() != 0 {
		t.Fatalf("got %d after reset, want 0", r.Len())
	}
	if r.Stdout() != "" {
		t.Fatal("stdout not empty after reset")
	}
	if r.Stderr() != "" {
		t.Fatal("stderr not empty after reset")
	}
}
