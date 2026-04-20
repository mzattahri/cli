# cli

[![Go Reference](https://pkg.go.dev/badge/github.com/mzattahri/cli.svg)](https://pkg.go.dev/github.com/mzattahri/cli)

`cli` routes command-line arguments to handlers, the same way `net/http` routes
HTTP requests.

| net/http            | cli              |
| ------------------- | ---------------- |
| `Handler`           | `Runner`         |
| `Request`           | `Call`           |
| `ResponseWriter`    | `Output`         |
| `ServeMux`          | `Mux`            |
| `Server`            | `Program`        |
| `HandlerFunc`       | `RunnerFunc`     |
| middleware wrapping | `MiddlewareFunc` |

A `Mux` matches command names to `Runner` values and dispatches. A `Command`
adds typed input declarations — flags, options, and positional arguments — to a
`Runner`. A `Program` ties a root runner to the process environment and handles
I/O and exit-code normalization.

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/mzattahri/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cmd := &cli.Command{
		Run: cli.RunnerFunc(func(out *cli.Output, call *cli.Call) error {
			_, err := fmt.Fprintf(out.Stdout, "hello %s\n", call.Args.Get("name"))
			return err
		}),
	}
	cmd.Arg("name", "Name to greet")

	mux := cli.NewMux("app")
	mux.Handle("greet", "Print a greeting", cmd)

	if err := (&cli.Program{}).Invoke(ctx, mux, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(err.Code)
	}
}
```

## Features

- **Routing** — nested `Mux` trees with subcommand mounting
- **Three input kinds** — boolean flags (`--verbose`), string options
  (`--host localhost`), and required positional arguments
- **Flag negation** — `--no-verbose` / `--cache` for `no-cache`
- **Middleware** — `MiddlewareFunc` and `Chain` compose runners the same way
  `net/http` middleware wraps handlers
- **Context** — `context.Context` flows from `Program.Invoke` through routing
  into every handler; middleware can derive or replace it via `Call.WithContext`
- **Environment fallback** — `EnvMiddleware` reads flags and options from env
  vars when not provided on the command line
- **Shell completion** — `CompletionRunner` emits tab completions for bash, zsh,
  and fish
- **Testing** — `clitest` sub-package provides in-memory `Call` and `Recorder`
  helpers, no process or signal handling needed
- **CaptureRest** — passthrough commands like `exec` or `ssh` can capture
  trailing arguments
- **Help rendering** — pluggable `HelpFunc` with a built-in tabular renderer
- **Introspection** — `Program.Walk` iterates the command tree with full `Help`
  structs for doc generation, man pages, or custom completion scripts

## Testing

`clitest` provides in-memory helpers — no process, no `os.Args`, no signal
handling. Construct a call, run the handler, inspect the output:

```go
recorder := clitest.NewRecorder()
call := clitest.NewCall("greet gopher", nil)
err := mux.RunCLI(recorder.Output(), call)
// recorder.Stdout.String() == "hello gopher\n"
```

This is the `httptest.NewRequest` + `httptest.ResponseRecorder` pattern applied
to CLI.

See the [package documentation](https://pkg.go.dev/github.com/mzattahri/cli) for
the full API and examples.
