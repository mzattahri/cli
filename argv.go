// Package argv parses command-line arguments and dispatches to
// handlers.
//
// A [Mux] matches argv tokens against registered command names and
// routes to a [Runner]. A [Command] parses flags, options, and
// positional arguments before invoking a handler. A [Call] carries
// the parsed input across the dispatch. A [Program] binds a root
// Runner to the process environment and normalizes exit codes.
//
// Values are strings. Typed conversion and validation are the
// caller's responsibility.
//
// # Inputs
//
// Flags are booleans set by presence. Options carry string values
// and may be repeated. Positional arguments are required and
// ordered.
//
// Flags and options appear before positional arguments. Parsing
// stops at the first non-flag token or after "--".
//
// Long options accept either "--name value" or "--name=value".
// Short options require space-separated values: "-H localhost", not
// "-Hlocalhost" or "-H=localhost". Short flags may be bundled
// ("-vfx"); a value-taking short option must be the last in its
// bundle.
//
// On the command line a flag value uses Go's [strconv.ParseBool]
// format ("1", "0", "true", "false"). [EnvMiddleware] additionally
// accepts shell and systemd conventions ("yes", "no", "on", "off").
//
// Flags and options declared on a [Mux] are parsed before subcommand
// routing and cascade into every Runner mounted beneath it. Parsed
// values accumulate in [Call.Flags] and [Call.Options]; defaults
// from each routing level are applied during dispatch.
// [FlagSet.Lookup] and [OptionSet.Lookup] distinguish caller-set
// values from defaults.
//
// # Extension model
//
// Every extension point is a single-method interface. [*Mux] and
// [*Command] implement the baseline set.
//
//   - [Runner] handles an invocation. Required.
//   - [Helper] contributes [Help] metadata. Optional.
//   - [Walker] enumerates a subtree. Optional.
//   - [Completer] provides tab completion. Optional; completion for
//     Runners that do not implement Completer is derived from
//     [Helper] and [Walker] output by [CompletionCommand].
//
// # Middleware
//
// A [Middleware] is a function of type func([Runner]) [Runner].
// Construct one with [NewMiddleware], which takes an "around" function
// mirroring [Runner.RunArgv] with a trailing next:
//
//	var WithAuth = argv.NewMiddleware(func(out *argv.Output, call *argv.Call, next argv.Runner) error {
//		if err := checkAuth(call.Context()); err != nil {
//			return err
//		}
//		return next.RunArgv(out, call)
//	})
//
//	mux.Handle("deploy", "Deploy", WithAuth(deployCmd))
//
// The Middleware produced by NewMiddleware forwards [Helper], [Walker],
// and [Completer] to the inner Runner, so help, subtree enumeration,
// and tab completion survive the wrap. A middleware that returns a
// bare [RunnerFunc] silently strips those interfaces.
//
// Compose middleware by nesting at the mount site; the outermost
// wrapper is applied first. Middleware wraps the entire invocation,
// including routing and input parsing.
//
// # Completion
//
// [CompletionCommand] returns a [*Command] that emits tab completions
// at runtime. Shell integration scripts invoke it on each TAB press
// with the shell-visible tokens after "--". Implement [Completer] on
// a Runner to provide dynamic, context-aware candidates (e.g. option
// values from a service).
//
// Each candidate is emitted as a newline-terminated line; a candidate
// with a description is "<value>\t<description>". Shells consume this
// format directly.
//
// For zsh, install the following as _myapp somewhere in your fpath
// (or source it at shell startup):
//
//	#compdef myapp
//
//	_myapp() {
//		local -a completions
//		local value desc
//		while IFS=$'\t' read -r value desc; do
//			if [[ -n "$desc" ]]; then
//				completions+=("${value}:${desc}")
//			else
//				completions+=("$value")
//			fi
//		done < <(${words[1]} complete -- "${words[@]:1}")
//		_describe 'command' completions
//	}
//
//	compdef _myapp myapp
//
// The same pattern adapts to bash (via COMPREPLY/compgen) and fish
// (via "complete -c myapp -f -a ..."); the CLI side is the same.
//
// # Introspection
//
// [Program.Walk] returns an iterator over every command reachable
// from a root Runner. [Mux.Match] returns the Runner that would
// handle a given token sequence along with its full command path.
// Both are read-only and suitable for generating documentation,
// man pages, or shell-completion scripts.
//
// # Testing
//
// The argvtest sub-package provides in-memory helpers for testing
// Runners without a process, os.Args, or signal handling:
//
//	recorder := argvtest.NewRecorder()
//	call := argvtest.NewCall("greet gopher")
//	err := mux.RunArgv(recorder.Output(), call)
//	// recorder.Stdout() == "hello gopher\n"
package argv // import "mz.attahri.com/code/argv"
