package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mzattahri/cli"
	"github.com/mzattahri/cli/clitest"
)

type authContextKey struct{}

func ExampleMux_Handle_subtree() {
	repo := cli.NewMux("repo")
	repo.Handle("init", "Initialize a repository", cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		_, err := fmt.Fprint(out.Stdout, "initialized")
		return err
	}))

	mux := cli.NewMux("app")
	mux.Handle("repo", "Manage repositories", repo)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	program := &cli.Program{Stdout: &stdout, Stderr: &stderr}
	_ = program.Invoke(context.Background(), mux, []string{"app", "repo", "init"})
	fmt.Print(stdout.String())
	// Output: initialized
}

func ExampleCommand() {
	cmd := &cli.Command{
		CaptureRest: true,
		Run: func(out *cli.Output, call *cli.Call) error {
			detach := call.Flags["detach"]
			_, err := fmt.Fprintf(out.Stdout, "image=%s detach=%t command=%v", call.Args["image"], detach, call.Rest)
			return err
		},
	}
	cmd.Flag("detach", "", false, "Run in background")
	cmd.Arg("image", "Image reference")

	mux := cli.NewMux("app")
	mux.Handle("run", "Run a container", cmd)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	program := &cli.Program{Stdout: &stdout, Stderr: &stderr}
	_ = program.Invoke(context.Background(), mux, []string{"app", "run", "--detach", "alpine", "sh", "-c", "echo hi"})
	fmt.Print(stdout.String())
	// Output: image=alpine detach=true command=[sh -c echo hi]
}

func ExampleCommand_negateFlags() {
	cmd := &cli.Command{
		NegateFlags: true,
		Run: func(out *cli.Output, call *cli.Call) error {
			_, err := fmt.Fprintf(out.Stdout, "dns=%t cache=%t",
				call.Flags["accept-dns"], call.Flags["no-cache"])
			return err
		},
	}
	cmd.Flag("accept-dns", "", true, "Accept DNS")
	cmd.Flag("no-cache", "", true, "Disable cache")

	mux := cli.NewMux("app")
	mux.Handle("up", "Connect", cmd)

	var stdout bytes.Buffer
	program := &cli.Program{Stdout: &stdout, Stderr: &bytes.Buffer{}}
	_ = program.Invoke(context.Background(), mux, []string{"app", "up", "--no-accept-dns", "--cache"})
	fmt.Print(stdout.String())
	// Output: dns=false cache=false
}

func ExampleProgram_Invoke() {
	cmd := &cli.Command{
		Run: func(out *cli.Output, call *cli.Call) error {
			_, err := fmt.Fprintf(out.Stdout, "hello %s", call.Args["name"])
			return err
		},
	}
	cmd.Arg("name", "Person to greet")

	mux := cli.NewMux("app")
	mux.Handle("greet", "Print a greeting", cmd)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	program := &cli.Program{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	_ = program.Invoke(context.Background(), mux, []string{"app", "greet", "gopher"})
	fmt.Print(stdout.String())
	// Output: hello gopher
}

func ExampleProgram_Invoke_errorHandling() {
	mux := cli.NewMux("app")
	mux.Handle("fail", "Always fails", cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		return fmt.Errorf("something went wrong")
	}))

	program := &cli.Program{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
	err := program.Invoke(context.Background(), mux, []string{"app", "fail"})
	if err != nil {
		fmt.Printf("code=%d err=%s", err.Code, err.Err)
	}
	// Output: code=1 err=something went wrong
}

func ExampleProgram_Invoke_helpDetection() {
	mux := cli.NewMux("app")
	mux.Handle("run", "Run something", cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		return nil
	}))

	program := &cli.Program{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
	err := program.Invoke(context.Background(), mux, []string{"app", "nope"})
	if err != nil && errors.Is(err, cli.ErrHelp) {
		fmt.Print("help was shown")
	}
	// Output: help was shown
}

func ExampleRunnerFunc_middleware() {
	// A Runner wrapping another Runner is the middleware pattern.
	withLog := func(next cli.Runner) cli.Runner {
		return cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
			fmt.Fprintln(out.Stderr, "running")
			return next.RunCLI(out, call)
		})
	}

	inner := cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		_, err := fmt.Fprint(out.Stdout, "done")
		return err
	})

	mux := cli.NewMux("app")
	mux.Handle("deploy", "Deploy the app", withLog(inner))

	recorder := clitest.NewRecorder()
	call := clitest.NewCall("deploy", nil)
	_ = mux.RunCLI(recorder.Output(), call)
	fmt.Printf("stdout=%s stderr=%s", recorder.Stdout.String(), recorder.Stderr.String())
	// Output: stdout=done stderr=running
}

func ExampleChain() {
	withLog := func(next cli.Runner) cli.Runner {
		return cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
			fmt.Fprintf(out.Stderr, "log ")
			return next.RunCLI(out, call)
		})
	}
	withAuth := func(next cli.Runner) cli.Runner {
		return cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
			fmt.Fprintf(out.Stderr, "auth ")
			return next.RunCLI(out, call)
		})
	}

	handler := cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		_, err := fmt.Fprint(out.Stdout, "done")
		return err
	})

	stack := cli.Chain(withLog, withAuth)
	mux := cli.NewMux("app")
	mux.Handle("deploy", "Deploy the app", stack(handler))

	recorder := clitest.NewRecorder()
	call := clitest.NewCall("deploy", nil)
	_ = mux.RunCLI(recorder.Output(), call)
	fmt.Printf("stdout=%s stderr=%s", recorder.Stdout.String(), recorder.Stderr.String())
	// Output: stdout=done stderr=log auth
}

func ExampleCall_WithContext() {
	call := clitest.NewCall("whoami", nil)
	ctx := context.WithValue(context.Background(), authContextKey{}, "alice")
	derived := call.WithContext(ctx)

	fmt.Print(derived.Context().Value(authContextKey{}))
	// Output: alice
}

func ExampleMux_Handle_optionsOnly() {
	cmd := &cli.Command{
		Run: func(out *cli.Output, call *cli.Call) error {
			host := call.Options.Get("host")
			_, err := fmt.Fprint(out.Stdout, host)
			return err
		},
	}
	cmd.Option("host", "", "", "daemon socket")

	mux := cli.NewMux("app")
	mux.Handle("status", "Print status", cmd)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	program := &cli.Program{Stdout: &stdout, Stderr: &stderr}
	_ = program.Invoke(context.Background(), mux, []string{"app", "status", "--host", "unix:///tmp/docker.sock"})
	fmt.Print(stdout.String())
	// Output: unix:///tmp/docker.sock
}

func ExampleMux_HandleFunc() {
	mux := cli.NewMux("app")
	mux.HandleFunc("version", "Print version", func(out *cli.Output, call *cli.Call) error {
		_, err := fmt.Fprint(out.Stdout, "v1.0.0")
		return err
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	program := &cli.Program{Stdout: &stdout, Stderr: &stderr}
	_ = program.Invoke(context.Background(), mux, []string{"app", "version"})
	fmt.Print(stdout.String())
	// Output: v1.0.0
}

func ExampleMux_Flag() {
	mux := cli.NewMux("app")
	mux.Flag("verbose", "v", false, "Enable verbose output")
	mux.Handle("status", "Print status", cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		_, err := fmt.Fprintf(out.Stdout, "verbose=%t", call.Flags["verbose"])
		return err
	}))

	var stdout bytes.Buffer
	program := &cli.Program{Stdout: &stdout, Stderr: &bytes.Buffer{}}
	_ = program.Invoke(context.Background(), mux, []string{"app", "--verbose", "status"})
	fmt.Print(stdout.String())
	// Output: verbose=true
}

func ExampleMux_Flag_submux() {
	root := cli.NewMux("app")
	root.Flag("verbose", "v", false, "Enable verbose output")

	sub := cli.NewMux("repo")
	sub.Option("path", "p", ".", "Repository path")
	sub.Handle("init", "Initialize a repository", cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		_, err := fmt.Fprintf(out.Stdout, "verbose=%t path=%s",
			call.Flags["verbose"], call.Options.Get("path"))
		return err
	}))

	root.Handle("repo", "Manage repositories", sub)

	var stdout bytes.Buffer
	program := &cli.Program{Stdout: &stdout, Stderr: &bytes.Buffer{}}
	_ = program.Invoke(context.Background(), root, []string{"app", "--verbose", "repo", "--path", "/tmp", "init"})
	fmt.Print(stdout.String())
	// Output: verbose=true path=/tmp
}

func ExampleCompletionRunner() {
	mux := cli.NewMux("app")
	mux.Handle("greet", "Print a greeting", cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		_, err := fmt.Fprint(out.Stdout, "hello")
		return err
	}))
	mux.Handle("complete", "Output completions", cli.CompletionRunner(mux))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	program := &cli.Program{Stdout: &stdout, Stderr: &stderr}
	_ = program.Invoke(context.Background(), mux, []string{"app", "complete", "--", "gr"})
	fmt.Print(stdout.String())
	// Output:
	// greet	Print a greeting
}

func ExampleCommand_Completer() {
	hosts := []string{"prod.example.com", "staging.example.com", "dev.example.com"}

	cmd := &cli.Command{
		Run: func(out *cli.Output, call *cli.Call) error {
			_, err := fmt.Fprintf(out.Stdout, "host=%s", call.Options.Get("host"))
			return err
		},
		// Completer provides tab completions for option values.
		// At value position (e.g. "--host <TAB>"), Command.Complete
		// delegates here with completed ending in the option token.
		Completer: cli.CompleterFunc(func(w *cli.TokenWriter, completed []string, partial string) error {
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

	mux := cli.NewMux("app")
	mux.Handle("deploy", "Deploy the app", cmd)
	mux.Handle("complete", "Output completions", cli.CompletionRunner(mux))

	var stdout bytes.Buffer
	program := &cli.Program{Stdout: &stdout, Stderr: &bytes.Buffer{}}

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
	env := cli.NewLookupFunc(map[string]string{
		"API_HOST":  "env.example.com",
		"API_TOKEN": "secret",
	})
	middleware := cli.EnvMiddleware(
		nil,
		map[string]string{"host": "API_HOST", "token": "API_TOKEN"},
		env,
	)

	handler := cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
		_, err := fmt.Fprintf(out.Stdout, "host=%s token=%s",
			call.Options.Get("host"), call.Options.Get("token"))
		return err
	})

	recorder := clitest.NewRecorder()
	call := clitest.NewCall("status", nil)
	_ = middleware(handler).RunCLI(recorder.Output(), call)
	fmt.Println(recorder.Stdout.String())

	// CLI-provided values take precedence over env.
	recorder.Reset()
	call = clitest.NewCall("status", nil)
	call.Options.Set("host", "cli.example.com")
	_ = middleware(handler).RunCLI(recorder.Output(), call)
	fmt.Print(recorder.Stdout.String())
	// Output:
	// host=env.example.com token=secret
	// host=cli.example.com token=secret
}

func ExampleCall_WithContext_timeout() {
	cmd := &cli.Command{
		Run: func(out *cli.Output, call *cli.Call) error {
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

	mux := cli.NewMux("app")
	mux.Handle("fetch", "Fetch data", cmd)

	// Middleware that enforces a timeout via context.
	withTimeout := func(next cli.Runner) cli.Runner {
		return cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
			ctx, cancel := context.WithTimeout(call.Context(), 0) // immediate timeout
			defer cancel()
			return next.RunCLI(out, call.WithContext(ctx))
		})
	}

	mux.Handle("slow", "Fetch with timeout", withTimeout(cmd))

	var stdout bytes.Buffer
	program := &cli.Program{Stdout: &stdout, Stderr: &bytes.Buffer{}}

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
	cmd := &cli.Command{
		Run: func(out *cli.Output, call *cli.Call) error {
			_, err := fmt.Fprintf(out.Stdout, "hello %s", call.Args["name"])
			return err
		},
	}
	cmd.Arg("name", "Person to greet")

	mux := cli.NewMux("app")
	mux.Handle("greet", "Print a greeting", cmd)

	recorder := clitest.NewRecorder()
	call := clitest.NewCall("greet gopher", nil)
	_ = mux.RunCLI(recorder.Output(), call)
	fmt.Print(recorder.Stdout.String())
	// Output: hello gopher
}
