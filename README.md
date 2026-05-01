# argv

[![Go Reference](https://pkg.go.dev/badge/mz.attahri.com/code/argv.svg)](https://pkg.go.dev/mz.attahri.com/code/argv)

**`argv` treats the command line as a transport protocol.**

POSIX argv is a wire format: tokens in, structured input out. `argv` handles the
transport. The pipeline mirrors `net/http`; values are strings.

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

	mux := &argv.Mux{}
	mux.Flag("verbose", "v", false, "Verbose output")

	greet := &argv.Command{
		Description: "Print a greeting",
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprintf(out.Stdout, "hello %s\n", call.Args.Get("name"))
			return err
		},
	}
	greet.Arg("name", "Who to greet")
	mux.Handle("greet", "Say hello", greet)

	(&argv.Program{Summary: "A demo CLI"}).Run(ctx, mux, os.Args)
}
```

```
$ app greet gopher
hello gopher
```

## Subcommands

```go
repo := &argv.Mux{}
repo.Handle("init", "Initialize a repository", initCmd)
repo.Handle("clone", "Clone a repository", cloneCmd)
mux.Handle("repo", "Repository operations", repo)
```

## Middleware

```go
var WithAuth = argv.NewMiddleware(func(out *argv.Output, call *argv.Call, next argv.Runner) error {
	if err := checkAuth(call.Context()); err != nil {
		return err
	}
	return next.RunArgv(out, call)
})

mux.Handle("deploy", "Deploy", WithAuth(WithLogging(deployCmd)))
```

## Environment fallback

```go
envMW := argv.EnvMiddleware(map[string]string{
	"verbose": "APP_VERBOSE",
	"host":    "APP_HOST",
}, nil)
mux.Handle("deploy", "Deploy", envMW(deployCmd))
```

## Tab completion

```go
mux.Handle("complete", "Output completions", argv.CompletionCommand(mux))
```

## Introspection

```go
for help, _ := range (&argv.Program{}).Walk("app", mux) {
	fmt.Printf("%s: %s\n", help.FullPath, help.Summary)
}
```

## Testing

```go
import "mz.attahri.com/code/argv/argvtest"

recorder := argvtest.NewRecorder()
call := argvtest.NewCall("greet gopher")
err := mux.RunArgv(recorder.Output(), call)
// recorder.Stdout() == "hello gopher\n"
```

See the [package documentation](https://pkg.go.dev/mz.attahri.com/code/argv) for
the full API and godoc examples.
