package argv_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"mz.attahri.com/code/argv"
	"mz.attahri.com/code/argv/argvtest"
)

type authContextKey struct{}

func Example() {
	greet := &argv.Command{
		Description: "Print a greeting",
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprintf(out.Stdout, "hello %s\n", call.Args.Get("name"))
			return err
		},
	}
	greet.Arg("name", "Who to greet")

	mux := &argv.Mux{}
	mux.Flag("verbose", "v", false, "Verbose output")
	mux.Handle("greet", "Say hello", greet)

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"A demo CLI",
		Stdout: &stdout,
		Stderr: &stderr,
	}
	// In a real program: program.Run(ctx, mux, os.Args)
	_ = program.Invoke(context.Background(), mux, []string{"app", "greet", "gopher"})
	fmt.Print(stdout.String())
	// Output: hello gopher
}

func ExampleCommand() {
	cmd := &argv.Command{
		Run: func(out *argv.Output, call *argv.Call) error {
			detach := call.Flags.Get("detach")
			_, err := fmt.Fprintf(out.Stdout, "image=%s detach=%t command=%v", call.Args.Get("image"), detach, call.Tail)
			return err
		},
	}
	cmd.Flag("detach", "", false, "Run in background")
	cmd.Arg("image", "Image reference")
	cmd.Tail("command", "Command and arguments to run")

	mux := &argv.Mux{}
	mux.Handle("run", "Run a container", cmd)

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"Run a container",
		Stdout: &stdout,
		Stderr: &stderr,
	}
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

	mux := &argv.Mux{}
	mux.Handle("up", "Connect", cmd)

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"Manage network state",
		Stdout: &stdout,
		Stderr: &stderr,
	}
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

	mux := &argv.Mux{}
	mux.Handle("greet", "Print a greeting", cmd)

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"Greet someone",
		Stdout: &stdout,
		Stderr: &stderr,
	}
	_ = program.Invoke(context.Background(), mux, []string{"app", "greet", "gopher"})
	fmt.Print(stdout.String())
	// Output: hello gopher
}

func ExampleProgram_Invoke_errorHandling() {
	mux := &argv.Mux{}
	mux.Handle("fail", "Always fails", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		return argv.Errorf(7, "something went wrong")
	}))

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"Demonstrate exit codes",
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := program.Invoke(context.Background(), mux, []string{"app", "fail"})
	var exitErr *argv.ExitError
	if errors.As(err, &exitErr) {
		fmt.Printf("code=%d err=%s", exitErr.Code, exitErr.Err)
	}
	// Output: code=7 err=something went wrong
}

func ExampleProgram_Invoke_helpDetection() {
	mux := &argv.Mux{}
	mux.Handle("run", "Run something", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		return nil
	}))

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"Demonstrate help detection",
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := program.Invoke(context.Background(), mux, []string{"app", "nope"})
	if err != nil && errors.Is(err, argv.ErrHelp) {
		fmt.Print("help was shown")
	}
	// Output: help was shown
}

func ExampleNewMiddleware() {
	// A Runner wrapping another Runner is the middleware pattern.
	// Use argv.NewMiddleware so Helper/Walker/Completer on inner
	// survive the wrap.
	withLog := argv.NewMiddleware(func(out *argv.Output, call *argv.Call, next argv.Runner) error {
		fmt.Fprintln(out.Stderr, "running")
		return next.RunArgv(out, call)
	})

	inner := argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprint(out.Stdout, "done")
		return err
	})

	mux := &argv.Mux{}
	mux.Handle("deploy", "Deploy the app", withLog(inner))

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"Deploy with logging",
		Stdout: &stdout,
		Stderr: &stderr,
	}
	_ = program.Invoke(context.Background(), mux, []string{"app", "deploy"})
	fmt.Printf("stdout=%s stderr=%s", stdout.String(), stderr.String())
	// Output: stdout=done stderr=running
}

func ExampleNewMiddleware_nested() {
	withLog := argv.NewMiddleware(func(out *argv.Output, call *argv.Call, next argv.Runner) error {
		fmt.Fprintf(out.Stderr, "log ")
		return next.RunArgv(out, call)
	})
	withAuth := argv.NewMiddleware(func(out *argv.Output, call *argv.Call, next argv.Runner) error {
		fmt.Fprintf(out.Stderr, "auth ")
		return next.RunArgv(out, call)
	})

	handler := argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprint(out.Stdout, "done")
		return err
	})

	mux := &argv.Mux{}
	mux.Handle("deploy", "Deploy the app", withLog(withAuth(handler)))

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"Deploy with layered middleware",
		Stdout: &stdout,
		Stderr: &stderr,
	}
	_ = program.Invoke(context.Background(), mux, []string{"app", "deploy"})
	fmt.Printf("stdout=%s stderr=%s", stdout.String(), stderr.String())
	// Output: stdout=done stderr=log auth
}

func ExampleCall_WithContext() {
	// Middleware injects an authenticated user into the call's context;
	// the runner reads it back through call.Context().
	withAuth := argv.NewMiddleware(func(out *argv.Output, call *argv.Call, next argv.Runner) error {
		ctx := context.WithValue(call.Context(), authContextKey{}, "alice")
		return next.RunArgv(out, call.WithContext(ctx))
	})

	whoami := argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprintf(out.Stdout, "user=%v", call.Context().Value(authContextKey{}))
		return err
	})

	mux := &argv.Mux{}
	mux.Handle("whoami", "Show the authenticated user", withAuth(whoami))

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"Inject an auth identity via middleware",
		Stdout: &stdout,
		Stderr: &stderr,
	}
	_ = program.Invoke(context.Background(), mux, []string{"app", "whoami"})
	fmt.Print(stdout.String())
	// Output: user=alice
}

func ExampleMux_Flag_submux() {
	root := &argv.Mux{}
	root.Flag("verbose", "v", false, "Enable verbose output")

	sub := &argv.Mux{}
	sub.Option("path", "p", ".", "Repository path")
	sub.Handle("init", "Initialize a repository", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprintf(out.Stdout, "verbose=%t path=%s",
			call.Flags.Get("verbose"), call.Options.Get("path"))
		return err
	}))

	root.Handle("repo", "Manage repositories", sub)

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"Repository tools",
		Stdout: &stdout,
		Stderr: &stderr,
	}
	_ = program.Invoke(context.Background(), root, []string{"app", "--verbose", "repo", "--path", "/tmp", "init"})
	fmt.Print(stdout.String())
	// Output: verbose=true path=/tmp
}

func ExampleCompletionCommand() {
	mux := &argv.Mux{}
	mux.Handle("greet", "Print a greeting", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprint(out.Stdout, "hello")
		return err
	}))
	mux.Handle("complete", "Output completions", argv.CompletionCommand(mux))

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"Print greetings with shell completion",
		Stdout: &stdout,
		Stderr: &stderr,
	}
	_ = program.Invoke(context.Background(), mux, []string{"app", "complete", "--", "gr"})
	fmt.Print(stdout.String())
	// Output:
	// greet	Print a greeting
}

// deployCmd embeds [*argv.Command] for Run/Help and implements
// [argv.Completer] to provide dynamic --host value suggestions.
// Non-value positions materialize the embedded Command's [argv.Help]
// and delegate to its CompleteArgv, which emits flag, option, and
// argument candidates.
type deployCmd struct {
	*argv.Command
	hosts []string
}

func (d *deployCmd) CompleteArgv(w *argv.TokenWriter, completed []string, partial string) error {
	if len(completed) > 0 && completed[len(completed)-1] == "--host" {
		for _, h := range d.hosts {
			if strings.HasPrefix(h, partial) {
				w.WriteToken(h, "")
			}
		}
		return nil
	}
	var help argv.Help
	d.HelpArgv(&help)
	return help.CompleteArgv(w, completed, partial)
}

func ExampleCompleter() {
	inner := &argv.Command{
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprintf(out.Stdout, "host=%s", call.Options.Get("host"))
			return err
		},
	}
	inner.Option("host", "H", "", "Target host")

	cmd := &deployCmd{
		Command: inner,
		hosts:   []string{"prod.example.com", "staging.example.com", "dev.example.com"},
	}

	mux := &argv.Mux{}
	mux.Handle("deploy", "Deploy the app", cmd)
	mux.Handle("complete", "Output completions", argv.CompletionCommand(mux))

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"Deploy to a host",
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// "--host <TAB>" with partial "sta" completes to staging.
	_ = program.Invoke(context.Background(), mux, []string{"app", "complete", "--", "deploy", "--host", "sta"})
	fmt.Print(stdout.String())
	// Output:
	// staging.example.com
}

// catCmd embeds [*argv.Command] and implements [argv.Completer] to
// suggest filenames from a directory for the <file> positional. It
// uses [Help.PositionalIndex] to dispatch only when the next token
// fills the first positional, falling back to the default Help-driven
// completion for flags, options, and other positions.
type catCmd struct {
	*argv.Command
	dir string
}

func (c *catCmd) CompleteArgv(w *argv.TokenWriter, completed []string, partial string) error {
	var help argv.Help
	c.HelpArgv(&help)
	if help.PositionalIndex(completed) == 0 {
		entries, err := os.ReadDir(c.dir)
		if err != nil {
			return nil
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), partial) {
				if _, err := w.WriteToken(e.Name(), ""); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return help.CompleteArgv(w, completed, partial)
}

func ExampleCompleter_positional() {
	dir, err := os.MkdirTemp("", "argv-cat-")
	if err != nil {
		return
	}
	defer os.RemoveAll(dir)
	for _, name := range []string{"alpha.txt", "beta.txt", "gamma.log"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			return
		}
	}

	inner := &argv.Command{
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprintf(out.Stdout, "file=%s", call.Args.Get("file"))
			return err
		},
	}
	inner.Arg("file", "File to print")

	mux := &argv.Mux{}
	mux.Handle("cat", "Print a file", &catCmd{Command: inner, dir: dir})
	mux.Handle("complete", "Output completions", argv.CompletionCommand(mux))

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"Print files with completion",
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// "cat alp<TAB>" completes to alpha.txt.
	_ = program.Invoke(context.Background(), mux, []string{"app", "complete", "--", "cat", "alp"})
	fmt.Print(stdout.String())
	// Output:
	// alpha.txt
}

func ExampleEnvMiddleware() {
	// EnvMiddleware resolves environment variables for flags and
	// options not set on the command line. CLI values take precedence.
	envMW := argv.EnvMiddleware(
		map[string]string{
			"host":  "API_HOST",
			"token": "API_TOKEN",
		},
		argvtest.NewLookupFunc(map[string]string{
			"API_HOST":  "env.example.com",
			"API_TOKEN": "secret",
		}),
	)

	deploy := &argv.Command{
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprintf(out.Stdout, "host=%s token=%s\n",
				call.Options.Get("host"), call.Options.Get("token"))
			return err
		},
	}
	deploy.Option("host", "", "", "API host")
	deploy.Option("token", "", "", "API token")

	mux := &argv.Mux{}
	mux.Handle("deploy", "Deploy the app", envMW(deploy))

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"Deploy via API",
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// Env-only: no CLI flags.
	_ = program.Invoke(context.Background(), mux, []string{"app", "deploy"})

	// CLI overrides env for --host; token still comes from env.
	_ = program.Invoke(context.Background(), mux, []string{"app", "deploy", "--host", "argv.example.com"})

	fmt.Print(stdout.String())
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

	mux := &argv.Mux{}
	mux.Handle("fetch", "Fetch data", cmd)

	// Middleware that enforces a timeout via context.
	withTimeout := argv.NewMiddleware(func(out *argv.Output, call *argv.Call, next argv.Runner) error {
		ctx, cancel := context.WithTimeout(call.Context(), 0) // immediate timeout
		defer cancel()
		return next.RunArgv(out, call.WithContext(ctx))
	})

	mux.Handle("slow", "Fetch with timeout", withTimeout(cmd))

	var stdout, stderr bytes.Buffer
	program := &argv.Program{
		Summary:"Fetch data, optionally with timeout",
		Stdout: &stdout,
		Stderr: &stderr,
	}

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

	mux := &argv.Mux{}
	mux.Handle("greet", "Print a greeting", cmd)

	recorder := argvtest.NewRecorder()
	call := argvtest.NewCall("greet gopher")
	_ = mux.RunArgv(recorder.Output(), call)
	fmt.Print(recorder.Stdout())
	// Output: hello gopher
}

func ExampleProgram_Walk() {
	mux := &argv.Mux{}
	mux.Flag("verbose", "v", false, "Enable verbose output")
	mux.Handle("deploy", "Deploy the app", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		return nil
	}))

	sub := &argv.Mux{}
	sub.Handle("init", "Initialize a repository", argv.RunnerFunc(func(out *argv.Output, call *argv.Call) error {
		return nil
	}))
	mux.Handle("repo", "Manage repositories", sub)

	program := &argv.Program{}
	for help := range program.Walk("app", mux) {
		if help.Summary != "" {
			fmt.Printf("%s: %s\n", help.FullPath, help.Summary)
		} else {
			fmt.Println(help.FullPath)
		}
	}
	// Output:
	// app
	// app deploy: Deploy the app
	// app repo: Manage repositories
	// app repo init: Initialize a repository
}

func ExampleMux_Match() {
	mux := &argv.Mux{}
	deploy := argv.RunnerFunc(func(*argv.Output, *argv.Call) error { return nil })
	mux.Handle("deploy", "Deploy the app", deploy)

	runner, path := mux.Match([]string{"deploy", "production"})
	fmt.Printf("matched=%t path=%q\n", runner != nil, path)

	runner, path = mux.Match([]string{"unknown"})
	fmt.Printf("matched=%t path=%q\n", runner != nil, path)
	// Output:
	// matched=true path="deploy"
	// matched=false path=""
}

// greetingHelper is a custom Runner that implements [argv.Helper] so
// that [argv.Mux] extracts its Description at registration time and
// [argv.Program.Walk] enumerates it with full help metadata.
type greetingHelper struct{}

func (greetingHelper) RunArgv(out *argv.Output, call *argv.Call) error {
	_, err := fmt.Fprintln(out.Stdout, "hi")
	return err
}

func (greetingHelper) HelpArgv(h *argv.Help) {
	h.Description = "Print a fixed greeting"
	h.Flags = append(h.Flags, argv.HelpFlag{Name: "loud", Short: "l", Usage: "Shout the greeting"})
}

func ExampleHelper() {
	mux := &argv.Mux{}
	mux.Handle("greet", "Say hi", greetingHelper{})

	for help := range (&argv.Program{}).Walk("app", mux) {
		if help.FullPath != "app greet" {
			continue
		}
		fmt.Printf("desc=%q flags=%d\n", help.Description, len(help.Flags))
	}
	// Output: desc="Print a fixed greeting" flags=1
}

// staticWalker is a third-party dispatcher that yields a synthetic
// subtree. Implementing [argv.Walker] lets it participate in
// [argv.Program.Walk] as a first-class node.
type staticWalker struct{ name string }

func (s staticWalker) RunArgv(*argv.Output, *argv.Call) error { return nil }

func (s staticWalker) WalkArgv(path string, base *argv.Help) iter.Seq2[*argv.Help, argv.Runner] {
	return func(yield func(*argv.Help, argv.Runner) bool) {
		if !yield(&argv.Help{Name: s.name, FullPath: path, Summary: "Synthetic root"}, s) {
			return
		}
		child := path + " child"
		yield(&argv.Help{Name: "child", FullPath: child, Summary: "Synthetic child"}, s)
	}
}

func ExampleWalker() {
	mux := &argv.Mux{}
	mux.Handle("plug", "External subtree", staticWalker{name: "plug"})

	for help := range (&argv.Program{}).Walk("app", mux) {
		if help.Summary == "" {
			fmt.Println(help.FullPath)
		} else {
			fmt.Printf("%s: %s\n", help.FullPath, help.Summary)
		}
	}
	// Output:
	// app
	// app plug: Synthetic root
	// app plug child: Synthetic child
}
