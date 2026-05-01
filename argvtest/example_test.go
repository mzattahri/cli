package argvtest_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"mz.attahri.com/code/argv"
	"mz.attahri.com/code/argv/argvtest"
)

func ExampleNewCall() {
	cmd := &argv.Command{
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprint(out.Stdout, strings.Join(call.Tail, ","))
			return err
		},
	}
	cmd.Tail("words", "")
	mux := &argv.Mux{}
	mux.Handle("echo", "Echo arguments", cmd)

	recorder := argvtest.NewRecorder()
	call := argvtest.NewCall("echo a b")
	_ = mux.RunArgv(recorder.Output(), call)

	fmt.Printf("stdout=%q stderr=%q", recorder.Stdout(), recorder.Stderr())
	// Output: stdout="a,b" stderr=""
}

func ExampleNewCall_stdin() {
	mux := &argv.Mux{}
	mux.Handle("cat", "Copy stdin to stdout", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := io.Copy(out.Stdout, call.Stdin)
		return err
	}))

	recorder := argvtest.NewRecorder()
	call := argvtest.NewCall("cat")
	call.Stdin = bytes.NewReader([]byte("piped input"))
	_ = mux.RunArgv(recorder.Output(), call)

	fmt.Printf("stdout=%q stderr=%q", recorder.Stdout(), recorder.Stderr())
	// Output: stdout="piped input" stderr=""
}

func ExampleNewCall_context() {
	type authKey struct{}

	mux := &argv.Mux{}
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
	call := argvtest.NewCall("--verbose -H unix:///tmp/docker.sock whoami alice")
	call = call.WithContext(context.WithValue(context.Background(), authKey{}, "alice"))
	_ = mux.RunArgv(recorder.Output(), call)

	fmt.Print(recorder.Stdout())
	// Output: user=alice host=unix:///tmp/docker.sock verbose=true name=alice
}
