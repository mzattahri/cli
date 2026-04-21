# argv

[![Go Reference](https://pkg.go.dev/badge/github.com/mzattahri/argv.svg)](https://pkg.go.dev/github.com/mzattahri/argv)

`argv` routes command-line arguments to handlers, the same way `net/http` routes
HTTP requests.

| net/http            | argv             |
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

The example below scaffolds a CLI shaped like `tailscale` — a root mux with a
global flag, flat subcommands (`up`, `down`, `status`), a nested `debug` mux,
and an `ssh` passthrough.

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/mzattahri/argv"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Root mux with a global flag available to every subcommand.
	mux := argv.NewMux("tailscale")
	mux.Flag("verbose", "v", false, "Verbose output")

	// `tailscale up` — negatable flags and an option with a default.
	up := &argv.Command{
		NegateFlags: true,
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprintf(out, "up hostname=%s dns=%t routes=%t\n",
				call.Options.Get("hostname"),
				call.Flags.Get("accept-dns"),
				call.Flags.Get("accept-routes"))
			return err
		},
	}
	up.Flag("accept-dns", "", true, "Accept DNS configuration")
	up.Flag("accept-routes", "", false, "Accept subnet routes")
	up.Option("hostname", "", "", "Tailnet hostname")
	mux.Handle("up", "Connect to Tailscale", up)

	mux.HandleFunc("down", "Disconnect", func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprintln(out, "disconnected")
		return err
	})
	mux.HandleFunc("status", "Show status", func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprintln(out, "connected")
		return err
	})

	// `tailscale debug ...` — a nested mux mounted as a subcommand.
	debug := argv.NewMux("debug")
	debug.HandleFunc("prefs", "Print current preferences", func(out *argv.Output, call *argv.Call) error {
		_, err := fmt.Fprintln(out, "{...prefs...}")
		return err
	})
	logs := &argv.Command{
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprintf(out, "logs for %s\n", call.Args.Get("component"))
			return err
		},
	}
	logs.Arg("component", "Component name")
	debug.Handle("component-logs", "Stream logs for a component", logs)
	mux.Handle("debug", "Debugging helpers", debug)

	// `tailscale ssh <host> -- cmd...` — passthrough via CaptureRest.
	ssh := &argv.Command{
		CaptureRest: true,
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprintf(out, "ssh %s -- %v\n",
				call.Args.Get("host"), call.Rest)
			return err
		},
	}
	ssh.Arg("host", "Target machine")
	mux.Handle("ssh", "SSH to a tailnet host", ssh)

	argv.Exit((&argv.Program{}).Invoke(ctx, mux, os.Args))
}
```

Running `tailscale up --help` renders:

```text
tailscale up - Connect to Tailscale

Usage:
  tailscale up [options]

Global Flags:
  -v, --verbose  Verbose output

Flags:
  --accept-dns, --no-accept-dns        Accept DNS configuration (default: true)
  --accept-routes, --no-accept-routes  Accept subnet routes

Options:
  --hostname  Tailnet hostname
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
- **Testing** — `argvtest` sub-package provides in-memory `Call` and `Recorder`
  helpers, no process or signal handling needed
- **CaptureRest** — passthrough commands like `exec` or `ssh` can capture
  trailing arguments
- **Help rendering** — pluggable `HelpFunc` with a built-in tabular renderer
- **Introspection** — `Program.Walk` iterates the command tree with full `Help`
  structs for doc generation, man pages, or custom completion scripts

## Design

`argv` models command-line parsing on `net/http` because CLIs and HTTP servers
solve the same problem — route input to a handler — and the patterns that work
for one work for the other. If you know how to write a middleware, a handler,
and a test against `httptest`, you already know how to use this library.

The scope is deliberately narrow. Values are strings; typed conversion,
validation, optional positionals, config-file parsing, and shell-script
generation are out of scope. Compose them around the library if you need them.

Capabilities compose by wrapping runners (middleware), not by registering hooks,
tags, or interceptor interfaces. Required inputs are declared as positional
arguments; there is no "required flag" form. Flags and options declared on a
`Mux` cascade into every runner mounted beneath it.

External tooling composes by walking the command tree. `Program.Walk` yields
every command's full `Help` value — enough to generate documentation, man pages,
or shell integration scripts without reaching into internals.

## Testing

`argvtest` provides in-memory helpers — no process, no `os.Args`, no signal
handling. Construct a call, run the handler, inspect the output:

```go
recorder := argvtest.NewRecorder()
call := argvtest.NewCall("up --hostname laptop", nil)
err := mux.RunCLI(recorder.Output(), call)
// recorder.Stdout.String() == "up hostname=laptop dns=true routes=false\n"
```

This is the `httptest.NewRequest` + `httptest.ResponseRecorder` pattern applied
to CLI.

See the [package documentation](https://pkg.go.dev/github.com/mzattahri/argv) for
the full API and examples.
