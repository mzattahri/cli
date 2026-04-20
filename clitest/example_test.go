package clitest_test

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mzattahri/cli"
	"github.com/mzattahri/cli/clitest"
)

func ExampleNewCall() {
	cmd := &cli.Command{
		CaptureRest: true,
		Run: func(out *cli.Output, call *cli.Call) error {
			_, err := fmt.Fprint(out.Stdout, strings.Join(call.Rest, ","))
			return err
		},
	}
	mux := cli.NewMux("app")
	mux.Handle("echo", "Echo arguments", cmd)

	recorder := clitest.NewRecorder()
	call := clitest.NewCall("echo a b", nil)
	_ = mux.RunCLI(recorder.Output(), call)

	fmt.Printf("stdout=%q stderr=%q", recorder.Stdout.String(), recorder.Stderr.String())
	// Output: stdout="a,b" stderr=""
}

func ExampleNewCall_stdin() {
	mux := cli.NewMux("app")
	mux.Handle("cat", "Copy stdin to stdout", cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		_, err := io.Copy(out.Stdout, call.Stdin)
		return err
	}))

	recorder := clitest.NewRecorder()
	call := clitest.NewCall("cat", []byte("piped input"))
	_ = mux.RunCLI(recorder.Output(), call)

	fmt.Printf("stdout=%q stderr=%q", recorder.Stdout.String(), recorder.Stderr.String())
	// Output: stdout="piped input" stderr=""
}

func ExampleNewCall_context() {
	type authKey struct{}

	mux := cli.NewMux("app")
	mux.Flag("verbose", "v", false, "verbose")
	mux.Option("host", "H", "", "host")

	cmd := &cli.Command{
		Run: func(out *cli.Output, call *cli.Call) error {
			user := call.Context().Value(authKey{})
			_, err := fmt.Fprintf(out.Stdout, "user=%v host=%s verbose=%t name=%s",
				user, call.Options.Get("host"), call.Flags.Get("verbose"), call.Args.Get("name"))
			return err
		},
	}
	cmd.Arg("name", "user name")
	mux.Handle("whoami", "", cmd)

	recorder := clitest.NewRecorder()
	call := clitest.NewCall("--verbose -H unix:///tmp/docker.sock whoami alice", nil)
	call = call.WithContext(context.WithValue(context.Background(), authKey{}, "alice"))
	_ = mux.RunCLI(recorder.Output(), call)

	fmt.Print(recorder.Stdout.String())
	// Output: user=alice host=unix:///tmp/docker.sock verbose=true name=alice
}
