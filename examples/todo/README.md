# todo

A worked example CLI built with
[`mz.attahri.com/code/argv`](https://pkg.go.dev/mz.attahri.com/code/argv).

Demonstrates, in ~250 LOC:

- Root [`*argv.Mux`](https://pkg.go.dev/mz.attahri.com/code/argv#Mux) with a
  global flag and option.
- Leaf [`*argv.Command`](https://pkg.go.dev/mz.attahri.com/code/argv#Command)
  handlers with typed flags, options, and positional arguments.
- Mounted sub-mux (`todo list ...`).
- [`argv.EnvMiddleware`](https://pkg.go.dev/mz.attahri.com/code/argv#EnvMiddleware)
  for env-var fallback (`TODO_STORE`).
- [`argv.CompletionRunner`](https://pkg.go.dev/mz.attahri.com/code/argv#CompletionRunner)
  wired at `todo complete` for shell completion.

## Build and try

```sh
go run .

# Top-level help.
go run . --help

# Add and list (per-process — the store is in-memory; see note below).
go run . add --due tomorrow "write tests"
go run . ls

# Sub-mux help.
go run . list --help

# Env fallback.
TODO_STORE=/tmp/demo go run . -v add "from env"

# Shell completion.
go run . complete -- add --
# → --due, --help, -h
```

## Note on the store

The backing store is a `map[int]Item` held in memory. Each `go run .` spawns a
fresh process, so items added in one invocation don't persist to the next. The
example focuses on wiring argv features together; persistence is intentionally
out of scope. A real CLI would load and save through the `--store` path.

## Layout

- [`main.go`](main.go) — the whole CLI. Command definitions, a tiny in-memory
  store, `EnvMiddleware` wiring, and the `CompletionRunner` hookup all in one
  file.

## The module boundary

`examples/todo` is its own Go module with a `replace` directive pointing back at
`../..`. This keeps the top-level module (`mz.attahri.com/code/argv`) free of
example-only dependencies and lets the example be built independently.
