package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
)

var (
	_ Runner = RunnerFunc(nil)
	_ Runner = &Command{}
)

func TestCallWithContext(t *testing.T) {
	origCtx := context.Background()
	replacedCtx := context.WithValue(context.Background(), struct{}{}, "replaced")
	stdin := bytes.NewBufferString("input")
	call := &Call{
		ctx:   origCtx,
		Rest:  []string{"run", "arg"},
		Stdin: stdin,
		Env: func(k string) (string, bool) {
			if k == "HOME" {
				return "/tmp/home", true
			}
			return "", false
		},
		Flags:   FlagSet{"verbose": true, "force": true},
		Options: OptionSet{"host": {"global-host"}, "name": {"option-name"}},
		Args:          ArgSet{"name": "arg-name"},
	}

	derived := call.WithContext(replacedCtx)
	if derived.Context() != replacedCtx {
		t.Fatal("expected context replacement")
	}
	if got := derived.Flags["verbose"]; !got {
		t.Fatalf("got %t", got)
	}
	if got := derived.Options.Get("host"); got != "global-host" {
		t.Fatalf("got %q", got)
	}
	if got := derived.Flags["force"]; !got {
		t.Fatalf("got %t", got)
	}
	if got := derived.Options.Get("name"); got != "option-name" {
		t.Fatalf("got %q", got)
	}
	if got := derived.Args["name"]; got != "arg-name" {
		t.Fatalf("got %q", got)
	}

	derived.Flags["verbose"] = false
	derived.Options.Set("host", "changed-host")
	derived.Flags["force"] = false
	derived.Options.Set("name", "changed-option-name")
	derived.Args["name"] = "changed-arg-name"

	if got := call.Flags["verbose"]; !got {
		t.Fatalf("original flag mutated: got %t", got)
	}
	if got := call.Options.Get("host"); got != "global-host" {
		t.Fatalf("original option mutated: got %q", got)
	}
	if got := call.Flags["force"]; !got {
		t.Fatalf("original flag mutated: got %t", got)
	}
	if got := call.Options.Get("name"); got != "option-name" {
		t.Fatalf("original option mutated: got %q", got)
	}
	if got := call.Args["name"]; got != "arg-name" {
		t.Fatalf("original arg mutated: got %q", got)
	}
}

func TestCallWithContextDeepCopiesOptionSlices(t *testing.T) {
	call := &Call{
		ctx:     context.Background(),
		Options: OptionSet{"host": {"a", "b"}, "tag": {"x", "y"}},
	}

	derived := call.WithContext(context.Background())
	derived.Options["host"][0] = "changed-host"
	derived.Options["tag"][0] = "changed-option"

	if got := call.Options.Values("host"); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("original options mutated: got %v", got)
	}
	if got := call.Options.Values("tag"); !slices.Equal(got, []string{"x", "y"}) {
		t.Fatalf("original options mutated: got %v", got)
	}
}

func TestCallWithContextPanicsOnNilContext(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewCall(context.Background(), nil).WithContext(nil) //nolint:staticcheck // intentional nil context to test panic
}

func TestCommandRejectsFlagOptionNameCollision(t *testing.T) {
	cmd := &Command{}
	cmd.Flag("name", "", false, "flag")

	defer func() {
		got := recover()
		if got == nil || !strings.Contains(fmt.Sprint(got), `duplicate input "name"`) {
			t.Fatalf("got panic %v", got)
		}
	}()
	cmd.Option("name", "", "", "option")
}

func TestCommandRejectsReservedHelpNames(t *testing.T) {
	t.Run("long", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic")
			}
		}()
		cmd := &Command{}
		cmd.Flag("help", "", false, "reserved")
	})
	t.Run("short", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic")
			}
		}()
		cmd := &Command{}
		cmd.Option("host", "h", "", "reserved")
	})
}

func TestCommandNilInput(t *testing.T) {
	cmd := &Command{Run: func(out *Output, call *Call) error { return nil }}
	if fs, os, as := commandInputs(cmd); fs != nil || os != nil || as != nil {
		t.Fatal("expected nil inputs")
	}
}

func TestCommandInputsAreValidated(t *testing.T) {
	cmd := &Command{
		Run: func(*Output, *Call) error { return nil },
	}
	cmd.Flag("verbose", "", false, "verbose output")
	cmd.Arg("name", "user name")
	fs, _, as := commandInputs(cmd)
	if got := fs.names(); len(got) != 1 || got[0] != "verbose" {
		t.Fatalf("got %v", got)
	}
	if got := as.helpArguments(); len(got) != 1 || got[0].Name != "<name>" {
		t.Fatalf("got %v", got)
	}
}

func TestCommandInputsReturnPointersToFields(t *testing.T) {
	cmd := &Command{
		Run: func(*Output, *Call) error { return nil },
	}
	cmd.Flag("verbose", "", false, "verbose output")
	cmd.Arg("name", "user name")

	fs1, _, as1 := commandInputs(cmd)
	fs2, _, as2 := commandInputs(cmd)
	// inputs() returns pointers to the command's fields, so both calls return
	// the same underlying data.
	if fs1 == nil || fs2 == nil || as1 == nil || as2 == nil {
		t.Fatal("expected non-nil inputs")
	}
	if got := fs1.names(); len(got) != 1 || got[0] != "verbose" {
		t.Fatalf("got %v", got)
	}
	if got := as1.helpArguments(); len(got) != 1 || got[0].Name != "<name>" {
		t.Fatalf("got %v", got)
	}
}

func TestCommandWithAllInputTypes(t *testing.T) {
	cmd := &Command{
		Run: func(*Output, *Call) error { return nil },
		CaptureRest: true,
	}
	cmd.Flag("verbose", "", false, "verbose output")
	cmd.Option("host", "", "", "daemon socket")
	cmd.Arg("name", "user name")

	fs, os, as := commandInputs(cmd)
	if got := fs.names(); len(got) != 1 || got[0] != "verbose" {
		t.Fatalf("got %v", got)
	}
	if got := os.names(); len(got) != 1 || got[0] != "host" {
		t.Fatalf("got %v", got)
	}
	if got := as.helpArguments(); len(got) != 1 || got[0].Name != "<name>" {
		t.Fatalf("got %v", got)
	}
	if !commandCaptureRest(cmd) {
		t.Fatal("expected capture rest")
	}
}

func TestFlushWriter(t *testing.T) {
	type flushRecorder struct {
		flushed bool
		err     error
	}
	fr := &flushRecorder{}

	type writerFlusher struct {
		*bytes.Buffer
		recorder *flushRecorder
	}
	wf := &writerFlusher{Buffer: &bytes.Buffer{}, recorder: fr}
	// Test that flushWriter handles a non-flusher gracefully.
	if err := flushWriter(wf); err != nil {
		t.Fatal(err)
	}
}

func TestFlushWriterError(t *testing.T) {
	fr := &flusherRecorder{err: fmt.Errorf("flush failed")}
	if err := flushWriter(fr); err == nil || err.Error() != "flush failed" {
		t.Fatalf("got err=%v", err)
	}
}

type flusherRecorder struct {
	flushed bool
	err     error
}

func (f *flusherRecorder) Write(p []byte) (int, error) { return len(p), nil }
func (f *flusherRecorder) Flush() error {
	f.flushed = true
	return f.err
}

func TestChain(t *testing.T) {
	var seen []string
	mw1 := func(next Runner) Runner {
		return RunnerFunc(func(out *Output, call *Call) error {
			seen = append(seen, "mw1")
			return next.RunCLI(out, call)
		})
	}
	mw2 := func(next Runner) Runner {
		return RunnerFunc(func(out *Output, call *Call) error {
			seen = append(seen, "mw2")
			return next.RunCLI(out, call)
		})
	}
	handler := RunnerFunc(func(out *Output, call *Call) error {
		seen = append(seen, "handler")
		return nil
	})

	chain := Chain(mw1, mw2)
	_ = chain(handler).RunCLI(nil, nil)

	want := []string{"mw1", "mw2", "handler"}
	if !slices.Equal(seen, want) {
		t.Fatalf("got %v, want %v", seen, want)
	}
}

func TestOutputWrite(t *testing.T) {
	var buf bytes.Buffer
	out := &Output{Stdout: &buf, Stderr: io.Discard}
	n, err := out.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("got %d, want 5", n)
	}
	if got := buf.String(); got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateRunnerPanicsOnNilRunner(t *testing.T) {
	defer func() {
		got := recover()
		if got == nil {
			t.Fatal("expected panic")
		}
		if s, ok := got.(string); !ok || !strings.Contains(s, "nil command runner") {
			t.Fatalf("got panic %v", got)
		}
	}()
	validateRunner(nil)
}

func TestValidateRunnerPanicsOnNilRun(t *testing.T) {
	defer func() {
		got := recover()
		if got == nil {
			t.Fatal("expected panic")
		}
		if s, ok := got.(string); !ok || !strings.Contains(s, "nil command handler") {
			t.Fatalf("got panic %v", got)
		}
	}()
	validateRunner(&Command{})
}
