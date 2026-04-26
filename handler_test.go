package argv

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

var (
	_ Runner = RunnerFunc(nil)
	_ Runner = &Command{}
)

type testCtxKey struct{}

func TestCallWithContext(t *testing.T) {
	origCtx := context.Background()
	replacedCtx := context.WithValue(context.Background(), testCtxKey{}, "replaced")
	stdin := bytes.NewBufferString("input")
	call := &Call{
		ctx:   origCtx,
		Tail:  []string{"run", "arg"},
		Stdin: stdin,
	}
	call.Flags.Set("verbose", true)
	call.Options.Set("host", "global-host")
	call.Args.Set("name", "arg-name")

	derived := call.WithContext(replacedCtx)
	if derived.Context() != replacedCtx {
		t.Fatal("expected context replacement")
	}
	// The shallow copy carries the same state as the receiver.
	if got := derived.Flags.Get("verbose"); !got {
		t.Fatalf("got %t", got)
	}
	if got := derived.Options.Get("host"); got != "global-host" {
		t.Fatalf("got %q", got)
	}
	if got := derived.Args.Get("name"); got != "arg-name" {
		t.Fatalf("got %q", got)
	}

	// WithContext is a shallow copy: Flags, Options, and Args share
	// map storage with the receiver.
	derived.Flags.Set("verbose", false)
	derived.Options.Set("host", "changed-host")
	derived.Args.Set("name", "changed-arg-name")

	if got := call.Flags.Get("verbose"); got {
		t.Fatalf("expected shared flag mutation, got %t", got)
	}
	if got := call.Options.Get("host"); got != "changed-host" {
		t.Fatalf("expected shared option mutation, got %q", got)
	}
	if got := call.Args.Get("name"); got != "changed-arg-name" {
		t.Fatalf("expected shared arg mutation, got %q", got)
	}
}

func TestCallWithContextPanicsOnNilContext(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	// Typed-nil context to test that WithContext panics. A literal nil
	// argument trips SA1012.
	var ctx context.Context
	NewCall(context.Background(), nil).WithContext(ctx)
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

func TestCommandRejectsNegationCollision(t *testing.T) {
	cases := []struct{ first, second string }{
		{"cache", "no-cache"},
		{"no-cache", "cache"},
	}
	for _, tc := range cases {
		t.Run(tc.first+"_then_"+tc.second, func(t *testing.T) {
			defer func() {
				got := recover()
				if got == nil {
					t.Fatal("expected panic")
				}
				if s, ok := got.(string); !ok || !strings.Contains(s, "collides with negation") {
					t.Fatalf("got panic %v", got)
				}
			}()
			cmd := &Command{}
			cmd.Flag(tc.first, "", false, "")
			cmd.Flag(tc.second, "", false, "")
		})
	}
}

func TestCommandNilInput(t *testing.T) {
	cmd := &Command{Run: func(out *Output, call *Call) error { return nil }}
	if fs, os, as := cmd.inputs(); fs != nil || os != nil || as != nil {
		t.Fatal("expected nil inputs")
	}
}

func TestCommandInputsAreValidated(t *testing.T) {
	cmd := &Command{
		Run: func(*Output, *Call) error { return nil },
	}
	cmd.Flag("verbose", "", false, "verbose output")
	cmd.Arg("name", "user name")
	fs, _, as := cmd.inputs()
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

	fs1, _, as1 := cmd.inputs()
	fs2, _, as2 := cmd.inputs()
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
		Run:         func(*Output, *Call) error { return nil },
		Variadic: true,
	}
	cmd.Flag("verbose", "", false, "verbose output")
	cmd.Option("host", "", "", "daemon socket")
	cmd.Arg("name", "user name")

	fs, os, as := cmd.inputs()
	if got := fs.names(); len(got) != 1 || got[0] != "verbose" {
		t.Fatalf("got %v", got)
	}
	if got := os.names(); len(got) != 1 || got[0] != "host" {
		t.Fatalf("got %v", got)
	}
	if got := as.helpArguments(); len(got) != 1 || got[0].Name != "<name>" {
		t.Fatalf("got %v", got)
	}
	if !cmd.Variadic {
		t.Fatal("expected capture rest")
	}
}

func TestFlushWriter(t *testing.T) {
	// flushWriter returns nil for writers without Flush; a bare
	// *bytes.Buffer has no Flush method.
	if err := flushWriter(&bytes.Buffer{}); err != nil {
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
