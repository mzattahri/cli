package argv_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mzattahri/argv"
	"github.com/mzattahri/argv/argvtest"
)

type authContextKey struct{}

func ExampleMux_Handle_subtree() {
	repo := argv.NewMux("repo")
	repo.Handle("init", "Initialize a repository", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprint(out.Stdout, "initialized")
		return err
	}))

	mux := argv.NewMux("app")
	mux.Handle("repo", "Manage repositories", repo)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	program := &argv.Program{Stdout: &stdout, Stderr: &stderr}
	_ = program.Invoke(context.Background(), mux, []string{"app", "repo", "init"})
	fmt.Print(stdout.String())
	// Output: initialized
}

func ExampleCommand() {
	cmd := &argv.Command{
		CaptureRest: true,
		Run: func(out *argv.Output, call *argv.Call) error {
			detach := call.Flags.Get("detach")
			_, err := fmt.Fprintf(out.Stdout, "image=%s detach=%t command=%v", call.Args.Get("image"), detach, call.Rest)
			return err
		},
	}
	cmd.Flag("detach", "", false, "Run in background")
	cmd.Arg("image", "Image reference")

	mux := argv.NewMux("app")
	mux.Handle("run", "Run a container", cmd)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	program := &argv.Program{Stdout: &stdout, Stderr: &stderr}
	_ = program.Invoke(context.Background(), mux, []string{"app", "run", "--detach", "alpine", "sh", "-c", "echo hi"})
	fmt.Print(stdout.String())
	// Output: image=alpine detach=true command=[sh -c echo hi]
}

func ExampleCommand_negateFlags() {
	cmd := &argv.Command{
		NegateFlags: true,
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprintf(out.Stdout, "dns=%t cache=%t",
				call.Flags.Get("accept-dns"), call.Flags.Get("no-cache"))
			return err
		},
	}
	cmd.Flag("accept-dns", "", true, "Accept DNS")
	cmd.Flag("no-cache", "", true, "Disable cache")

	mux := argv.NewMux("app")
	mux.Handle("up", "Connect", cmd)

	var stdout bytes.Buffer
	program := &argv.Program{Stdout: &stdout, Stderr: &bytes.Buffer{}}
	_ = program.Invoke(context.Background(), mux, []string{"app", "up", "--no-accept-dns", "--cache"})
	fmt.Print(stdout.String())
	// Output: dns=false cache=false
}

func ExampleProgram_Invoke() {
	cmd := &argv.Command{
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprintf(out.Stdout, "hello %s", call.Args.Get("name"))
			return err
		},
	}
	cmd.Arg("name", "Person to greet")

	mux := argv.NewMux("app")
	mux.Handle("greet", "Print a greeting", cmd)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	program := &argv.Program{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	_ = program.Invoke(context.Background(), mux, []string{"app", "greet", "gopher"})
	fmt.Print(stdout.String())
	// Output: hello gopher
}

func ExampleProgram_Invoke_errorHandling() {
	mux := argv.NewMux("app")
	mux.Handle("fail", "Always fails", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		return argv.Errorf(7, "something went wrong")
	}))

	program := &argv.Program{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
	err := program.Invoke(context.Background(), mux, []string{"app", "fail"})
	if err != nil {
		fmt.Printf("code=%d err=%s", err.Code, err.Err)
	}
	// Output: code=7 err=something went wrong
}

func ExampleProgram_Invoke_helpDetection() {
	mux := argv.NewMux("app")
	mux.Handle("run", "Run something", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		return nil
	}))

	program := &argv.Program{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
	err := program.Invoke(context.Background(), mux, []string{"app", "nope"})
	if err != nil && errors.Is(err, argv.ErrHelp) {
		fmt.Print("help was shown")
	}
	// Output: help was shown
}

func ExampleRunnerFunc_middleware() {
	// A Runner wrapping another Runner is the middleware pattern.
	withLog := func(next argv.Runner) argv.Runner {
		return argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
			fmt.Fprintln(out.Stderr, "running")
			return next.RunCLI(out, call)
		})
	}

	inner := argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprint(out.Stdout, "done")
		return err
	})

	mux := argv.NewMux("app")
	mux.Handle("deploy", "Deploy the app", withLog(inner))

	recorder := argvtest.NewRecorder()
	call := argvtest.NewCall("deploy", nil)
	_ = mux.RunCLI(recorder.Output(), call)
	fmt.Printf("stdout=%s stderr=%s", recorder.Stdout.String(), recorder.Stderr.String())
	// Output: stdout=done stderr=running
}

func ExampleChain() {
	withLog := func(next argv.Runner) argv.Runner {
		return argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
			fmt.Fprintf(out.Stderr, "log ")
			return next.RunCLI(out, call)
		})
	}
	withAuth := func(next argv.Runner) argv.Runner {
		return argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
			fmt.Fprintf(out.Stderr, "auth ")
			return next.RunCLI(out, call)
		})
	}

	handler := argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprint(out.Stdout, "done")
		return err
	})

	stack := argv.Chain(withLog, withAuth)
	mux := argv.NewMux("app")
	mux.Handle("deploy", "Deploy the app", stack(handler))

	recorder := argvtest.NewRecorder()
	call := argvtest.NewCall("deploy", nil)
	_ = mux.RunCLI(recorder.Output(), call)
	fmt.Printf("stdout=%s stderr=%s", recorder.Stdout.String(), recorder.Stderr.String())
	// Output: stdout=done stderr=log auth
}

func ExampleCall_WithContext() {
	call := argvtest.NewCall("whoami", nil)
	ctx := context.WithValue(context.Background(), authContextKey{}, "alice")
	derived := call.WithContext(ctx)

	fmt.Print(derived.Context().Value(authContextKey{}))
	// Output: alice
}

func ExampleMux_Handle_optionsOnly() {
	cmd := &argv.Command{
		Run: func(out *argv.Output, call *argv.Call) error {
			host := call.Options.Get("host")
			_, err := fmt.Fprint(out.Stdout, host)
			return err
		},
	}
	cmd.Option("host", "", "", "daemon socket")

	mux := argv.NewMux("app")
	mux.Handle("status", "Print status", cmd)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	program := &argv.Program{Stdout: &stdout, Stderr: &stderr}
	_ = program.Invoke(context.Background(), mux, []string{"app", "status", "--host", "unix:///tmp/docker.sock"})
	fmt.Print(stdout.String())
	// Output: unix:///tmp/docker.sock
}

func ExampleMux_HandleFunc() {
	mux := argv.NewMux("app")
	mux.HandleFunc("version", "Print version", func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprint(out.Stdout, "v1.0.0")
		return err
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	program := &argv.Program{Stdout: &stdout, Stderr: &stderr}
	_ = program.Invoke(context.Background(), mux, []string{"app", "version"})
	fmt.Print(stdout.String())
	// Output: v1.0.0
}

func ExampleMux_Flag() {
	mux := argv.NewMux("app")
	mux.Flag("verbose", "v", false, "Enable verbose output")
	mux.Handle("status", "Print status", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprintf(out.Stdout, "verbose=%t", call.Flags.Get("verbose"))
		return err
	}))

	var stdout bytes.Buffer
	program := &argv.Program{Stdout: &stdout, Stderr: &bytes.Buffer{}}
	_ = program.Invoke(context.Background(), mux, []string{"app", "--verbose", "status"})
	fmt.Print(stdout.String())
	// Output: verbose=true
}

func ExampleMux_Flag_submux() {
	root := argv.NewMux("app")
	root.Flag("verbose", "v", false, "Enable verbose output")

	sub := argv.NewMux("repo")
	sub.Option("path", "p", ".", "Repository path")
	sub.Handle("init", "Initialize a repository", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprintf(out.Stdout, "verbose=%t path=%s",
			call.Flags.Get("verbose"), call.Options.Get("path"))
		return err
	}))

	root.Handle("repo", "Manage repositories", sub)

	var stdout bytes.Buffer
	program := &argv.Program{Stdout: &stdout, Stderr: &bytes.Buffer{}}
	_ = program.Invoke(context.Background(), root, []string{"app", "--verbose", "repo", "--path", "/tmp", "init"})
	fmt.Print(stdout.String())
	// Output: verbose=true path=/tmp
}

func ExampleCompletionRunner() {
	mux := argv.NewMux("app")
	mux.Handle("greet", "Print a greeting", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprint(out.Stdout, "hello")
		return err
	}))
	mux.Handle("complete", "Output completions", argv.CompletionRunner(mux))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	program := &argv.Program{Stdout: &stdout, Stderr: &stderr}
	_ = program.Invoke(context.Background(), mux, []string{"app", "complete", "--", "gr"})
	fmt.Print(stdout.String())
	// Output:
	// greet	Print a greeting
}

func ExampleCommand_Completer() {
	hosts := []string{"prod.example.com", "staging.example.com", "dev.example.com"}

	cmd := &argv.Command{
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprintf(out.Stdout, "host=%s", call.Options.Get("host"))
			return err
		},
		// Completer provides tab completions for option values.
		// At value position (e.g. "--host <TAB>"), Command.Complete
		// delegates here with completed ending in the option token.
		Completer: argv.CompleterFunc(func(w *argv.TokenWriter, completed []string, partial string) error {
			if len(completed) == 0 || completed[len(completed)-1] != "--host" {
				return nil
			}
			for _, h := range hosts {
				if strings.HasPrefix(h, partial) {
					w.WriteToken(h, "")
				}
			}
			return nil
		}),
	}
	cmd.Option("host", "H", "", "Target host")

	mux := argv.NewMux("app")
	mux.Handle("deploy", "Deploy the app", cmd)
	mux.Handle("complete", "Output completions", argv.CompletionRunner(mux))

	var stdout bytes.Buffer
	program := &argv.Program{Stdout: &stdout, Stderr: &bytes.Buffer{}}

	// "--host <TAB>" with partial "sta" completes to staging.
	_ = program.Invoke(context.Background(), mux, []string{"app", "complete", "--", "deploy", "--host", "sta"})
	fmt.Print(stdout.String())
	// Output:
	// staging.example.com
}

func ExampleEnvMiddleware() {
	// EnvMiddleware resolves environment variables for flags/options
	// not provided on the command line. It runs before ApplyDefaults,
	// so Has reports only CLI-provided values.
	env := argv.NewLookupFunc(map[string]string{
		"API_HOST":  "env.example.com",
		"API_TOKEN": "secret",
	})
	middleware := argv.EnvMiddleware(
		nil,
		map[string]string{"host": "API_HOST", "token": "API_TOKEN"},
		env,
	)

	handler := argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprintf(out.Stdout, "host=%s token=%s",
			call.Options.Get("host"), call.Options.Get("token"))
		return err
	})

	recorder := argvtest.NewRecorder()
	call := argvtest.NewCall("status", nil)
	_ = middleware(handler).RunCLI(recorder.Output(), call)
	fmt.Println(recorder.Stdout.String())

	// CLI-provided values take precedence over env.
	recorder.Reset()
	call = argvtest.NewCall("status", nil)
	call.Options.Set("host", "argv.example.com")
	_ = middleware(handler).RunCLI(recorder.Output(), call)
	fmt.Print(recorder.Stdout.String())
	// Output:
	// host=env.example.com token=secret
	// host=argv.example.com token=secret
}

func ExampleCall_WithContext_timeout() {
	cmd := &argv.Command{
		Run: func(out *argv.Output, call *argv.Call) error {
			ctx := call.Context()
			// Simulate work that respects the context deadline.
			select {
			case <-ctx.Done():
				_, err := fmt.Fprint(out.Stdout, "timed out")
				return err
			default:
				_, err := fmt.Fprint(out.Stdout, "done")
				return err
			}
		},
	}

	mux := argv.NewMux("app")
	mux.Handle("fetch", "Fetch data", cmd)

	// Middleware that enforces a timeout via context.
	withTimeout := func(next argv.Runner) argv.Runner {
		return argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
			ctx, cancel := context.WithTimeout(call.Context(), 0) // immediate timeout
			defer cancel()
			return next.RunCLI(out, call.WithContext(ctx))
		})
	}

	mux.Handle("slow", "Fetch with timeout", withTimeout(cmd))

	var stdout bytes.Buffer
	program := &argv.Program{Stdout: &stdout, Stderr: &bytes.Buffer{}}

	_ = program.Invoke(context.Background(), mux, []string{"app", "fetch"})
	fmt.Println(stdout.String())

	stdout.Reset()
	_ = program.Invoke(context.Background(), mux, []string{"app", "slow"})
	fmt.Print(stdout.String())
	// Output:
	// done
	// timed out
}

func ExampleRecorder() {
	cmd := &argv.Command{
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprintf(out.Stdout, "hello %s", call.Args.Get("name"))
			return err
		},
	}
	cmd.Arg("name", "Person to greet")

	mux := argv.NewMux("app")
	mux.Handle("greet", "Print a greeting", cmd)

	recorder := argvtest.NewRecorder()
	call := argvtest.NewCall("greet gopher", nil)
	_ = mux.RunCLI(recorder.Output(), call)
	fmt.Print(recorder.Stdout.String())
	// Output: hello gopher
}

func ExampleProgram_Walk() {
	mux := argv.NewMux("app")
	mux.Flag("verbose", "v", false, "Enable verbose output")
	mux.Handle("deploy", "Deploy the app", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		return nil
	}))

	sub := argv.NewMux("repo")
	sub.Handle("init", "Initialize a repository", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		return nil
	}))
	mux.Handle("repo", "Manage repositories", sub)

	program := &argv.Program{}
	for path, help := range program.Walk(mux) {
		if help.Usage != "" {
			fmt.Printf("%s — %s\n", path, help.Usage)
		} else {
			fmt.Println(path)
		}
	}
	// Output:
	// app
	// app deploy — Deploy the app
	// app repo — Manage repositories
	// app repo init — Initialize a repository
}
