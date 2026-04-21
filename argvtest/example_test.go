package argvtest_test

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mzattahri/argv"
	"github.com/mzattahri/argv/argvtest"
)

func ExampleNewCall() {
	cmd := &argv.Command{
		CaptureRest: true,
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprint(out.Stdout, strings.Join(call.Rest, ","))
			return err
		},
	}
	mux := argv.NewMux("app")
	mux.Handle("echo", "Echo arguments", cmd)

	recorder := argvtest.NewRecorder()
	call := argvtest.NewCall("echo a b", nil)
	_ = mux.RunCLI(recorder.Output(), call)

	fmt.Printf("stdout=%q stderr=%q", recorder.Stdout.String(), recorder.Stderr.String())
	// Output: stdout="a,b" stderr=""
}

func ExampleNewCall_stdin() {
	mux := argv.NewMux("app")
	mux.Handle("cat", "Copy stdin to stdout", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := io.Copy(out.Stdout, call.Stdin)
		return err
	}))

	recorder := argvtest.NewRecorder()
	call := argvtest.NewCall("cat", []byte("piped input"))
	_ = mux.RunCLI(recorder.Output(), call)

	fmt.Printf("stdout=%q stderr=%q", recorder.Stdout.String(), recorder.Stderr.String())
	// Output: stdout="piped input" stderr=""
}

func ExampleNewCall_context() {
	type authKey struct{}

	mux := argv.NewMux("app")
	mux.Flag("verbose", "v", false, "verbose")
	mux.Option("host", "H", "", "host")

	cmd := &argv.Command{
		Run: func(out *argv.Output, call *argv.Call) error {
			user := call.Context().Value(authKey{})
			_, err := fmt.Fprintf(out.Stdout, "user=%v host=%s verbose=%t name=%s",
				user, call.Options.Get("host"), call.Flags.Get("verbose"), call.Args.Get("name"))
			return err
		},
	}
	cmd.Arg("name", "user name")
	mux.Handle("whoami", "", cmd)

	recorder := argvtest.NewRecorder()
	call := argvtest.NewCall("--verbose -H unix:///tmp/docker.sock whoami alice", nil)
	call = call.WithContext(context.WithValue(context.Background(), authKey{}, "alice"))
	_ = mux.RunCLI(recorder.Output(), call)

	fmt.Print(recorder.Stdout.String())
	// Output: user=alice host=unix:///tmp/docker.sock verbose=true name=alice
}
