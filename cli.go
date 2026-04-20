// Package cli parses command lines and dispatches to runners.
//
// A [Mux] matches command names to [Runner] values. A [Command] parses
// flags, options, and positional arguments before invoking its runner.
// A [Call] holds the parsed input for a single invocation. A [Program]
// binds a root runner to the process environment.
//
// # Inputs
//
// Flags are boolean values set by presence. Options carry string
// values and may be repeated. Positional arguments are required and
// ordered. All values are strings.
//
// Flags and options appear before positional arguments; parsing stops
// at the first non-flag token or after "--".
//
// Required inputs are declared as positional arguments.
//
// Flags and options declared on a [Mux] are parsed before subcommand
// routing and cascade into every runner mounted beneath it. Parsed
// values accumulate in [Call.Flags] and [Call.Options]; defaults from
// each routing level are applied during dispatch. [FlagSet.Explicit]
// and [OptionSet.Explicit] distinguish command-line input from
// defaults.
//
// # Middleware
//
// A [MiddlewareFunc] is a function of type func([Runner]) [Runner].
// [Chain] composes middleware in the order given; the first element
// is the outermost wrapper. Middleware wraps the entire invocation,
// including routing and input parsing.
//
// # Completion
//
// [CompletionRunner] returns a [Runner] that emits tab completions at
// runtime from a registered [Completer]. Shell integration scripts
// invoke the runner on each TAB press.
//
// # Introspection
//
// [Program.Walk] returns an iterator over every command reachable
// from the root runner. Walk visits nodes depth-first, sorted
// alphabetically at each level. Each iteration yields the full
// command path and a [Help] value describing that node's flags,
// options, positional arguments, and subcommands. Flags and options
// from ancestor routing levels are included and marked Global.
//
// # Testing
//
// The clitest sub-package provides in-memory helpers for testing
// runners without a process, os.Args, or signal handling:
//
//	recorder := clitest.NewRecorder()
//	call := clitest.NewCall("greet gopher", nil)
//	err := mux.RunCLI(recorder.Output(), call)
//	// recorder.Stdout.String() == "hello gopher\n"
package cli
