# argv

[![Go Reference](https://pkg.go.dev/badge/mz.attahri.com/code/argv.svg)](https://pkg.go.dev/mz.attahri.com/code/argv)

`argv` routes command-line arguments to handlers, the same way `net/http` routes
HTTP requests.

| net/http            | argv             |
| ------------------- | ---------------- |
| `Handler`           | `Runner`         |
| `Request`           | `Call`           |
| `ResponseWriter`    | `Output`         |
| `ServeMux`          | `Mux`            |
| `ServeMux.Handler`  | `Mux.Match`      |
| `Server`            | `Program`        |
| `HandlerFunc`       | `RunnerFunc`     |
| middleware wrapping | `MiddlewareFunc` |

A `Mux` matches command names to `Runner` values and dispatches. A `Command`
adds typed input declarations — flags, options, and positional arguments — to a
`Runner`. A `Program` ties a root runner to the process environment and handles
I/O and exit-code normalization.

## Quickstart

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"mz.attahri.com/code/argv"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	mux := argv.NewMux("app")
	mux.Flag("verbose", "v", false, "Verbose output")

	greet := &argv.Command{
		Description: "Print a greeting",
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprintf(out, "hello %s\n", call.Args.Get("name"))
			return err
		},
	}
	greet.Arg("name", "Who to greet")
	mux.Handle("greet", "Say hello", greet)

	(&argv.Program{Usage: "A demo CLI"}).InvokeAndExit(ctx, mux, os.Args)
}
```

```
$ app greet gopher
hello gopher

$ app greet --help
app greet - Say hello

Usage:
  app greet [arguments]

Global Flags:
  -v, --verbose  Verbose output

Arguments:
  <name>  Who to greet
```

## The four interfaces

Every extension point is a single-method interface. `*Mux` and `*Command`
implement all four; third-party types implement whichever roles they play.

| Interface   | Method                                         | Purpose                                 |
| ----------- | ---------------------------------------------- | --------------------------------------- |
| `Runner`    | `RunCLI(out, call) error`                      | Handle an invocation.                   |
| `Helper`    | `HelpCLI() Help`                               | Contribute flags/options/args to help.  |
| `Walker`    | `WalkCLI(path, base) iter.Seq2[string, *Help]` | Enumerate a subtree for `Program.Walk`. |
| `Completer` | `CompleteCLI(w, completed, partial) error`     | Provide tab completions.                |

Only `Runner` is required. The rest are opt-in.

## Writing your own Runner

The dispatch pipeline is interface-driven end to end. Any type implementing
`Runner` plugs in:

```go
type MyCommand struct{}

func (c *MyCommand) RunCLI(out *argv.Output, call *argv.Call) error {
	// Parse call.Argv however you like.
	for _, tok := range call.Argv {
		if tok == "--help" {
			// call.Help carries ancestor globals; call.HelpFunc renders.
			return c.renderHelp(out, call)
		}
	}
	_, err := fmt.Fprintln(out, "ran")
	return err
}

mux.Handle("mine", "Custom runner", &MyCommand{})
```

Registering your type with `Mux.Handle` makes it a first-class child — it
receives a handoff `Call` with the matched `Pattern`, unconsumed `Argv`, and a
`*Help` carrying the inherited ancestor context. Implement `Helper` to
contribute to `Program.Walk`; implement `Walker` to enumerate your own subtree;
implement `Completer` to participate in tab completion.

The `Call` is a plain data struct — no getters, no magic. A dispatcher derives a
child call by building a struct literal.

## Middleware

`MiddlewareFunc` is `func(Runner) Runner`. `Chain` composes, outermost first:

```go
stack := argv.Chain(withLogging, withAuth)
mux.Handle("deploy", "Deploy", stack(deployCmd))
```

Middleware runs inside the runner's `RunCLI`, so it sees parsed `call.Flags`,
`call.Options`, and `call.Args` — not raw argv.

## Environment fallback

`EnvMiddleware` reads flags and options from env vars when not provided on the
command line:

```go
mw := argv.EnvMiddleware(
	map[string]string{"verbose": "APP_VERBOSE"},
	map[string]string{"host":    "APP_HOST"},
	os.LookupEnv,
)
mux.Handle("deploy", "Deploy", mw(deployCmd))
```

CLI-provided values take precedence. Env values are a fallback, not an override.

## Completion

`CompletionRunner` emits tab-completion candidates for bash, zsh, or fish:

```go
mux.Handle("complete", "Output completions", argv.CompletionRunner(mux))
```

Shell scripts invoke `app complete -- <tokens>` on each TAB press. `*Command`
completes its flags and options; `*Mux` completes subcommands and mux-level
flags.

## Introspection

`Program.Walk` yields `(path, *Help)` for every reachable command. `Mux.Match`
is the analog of `http.ServeMux.Handler`:

```go
for path, help := range (&argv.Program{}).Walk(mux) {
	fmt.Printf("%s — %s\n", path, help.Usage)
}

runner, pattern := mux.Match([]string{"repo", "init"})
```

Use this to generate man pages, shell completion scripts, or a custom help
renderer. External dispatchers participate by implementing `Walker`.

## Testing

`argvtest` provides in-memory helpers — no process, no `os.Args`, no signal
handling:

```go
import "mz.attahri.com/code/argv/argvtest"

recorder := argvtest.NewRecorder()
call := argvtest.NewCall("greet gopher", nil)
err := mux.RunCLI(recorder.Output(), call)
// recorder.Stdout.String() == "hello gopher\n"
```

This is the `httptest.NewRequest` + `httptest.ResponseRecorder` pattern applied
to CLI.

## Scope

Values are strings. Typed conversion, validation, optional positionals,
config-file parsing, and shell-script generation are out of scope. Compose them
around the library.

Capabilities extend by wrapping runners (middleware) or by implementing one of
the four interfaces. Required inputs are declared as positional arguments; there
is no "required flag" form. Flags and options declared on a `Mux` cascade into
every runner mounted beneath it.

## The `Help` struct

`Help` is populated in two shapes. On `call.Help`, a dispatcher leaves a partial
`*Help` carrying `Usage`, `Description`, and accumulated global
`Flags`/`Options` for the receiver's render context. When a `Runner` renders
help, it extends that base with its own `Name`, `FullPath`, local
`Flags`/`Options`, `Commands`, `Arguments`, and `CaptureRest` and hands the full
struct to `call.HelpFunc`. The `Global` bit on each `HelpFlag`/`HelpOption`
distinguishes inherited from local entries; `Help.GlobalFlags()`,
`Help.LocalFlags()`, etc., iterate by discriminator.

See the [package documentation](https://pkg.go.dev/mz.attahri.com/code/argv) for
the full API and godoc examples.
