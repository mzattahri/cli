package argv

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type envKey struct{}

// withEnv stashes lookup on ctx for downstream retrieval by [LookupEnv].
func withEnv(ctx context.Context, lookup LookupFunc) context.Context {
	return context.WithValue(ctx, envKey{}, lookup)
}

// LookupEnv returns the value of the environment variable named name.
// It uses the [LookupFunc] attached to ctx by [EnvMiddleware], falling
// back to [os.LookupEnv] when no middleware has attached one.
func LookupEnv(ctx context.Context, name string) (value string, ok bool) {
	if l, ok := ctx.Value(envKey{}).(LookupFunc); ok {
		return l(name)
	}
	return os.LookupEnv(name)
}

// EnvMiddleware returns a [Middleware] that populates flags and
// options from environment variables when the command line did not
// provide them.
//
// env maps an input name to an environment variable name. The input
// name must be declared as either a flag or an option in the wrapped
// [Runner]'s command tree; classification is performed at wrap time
// by walking the Runner via [Walker]. The wrapped runner must
// implement Walker. Binding a name that is not declared anywhere in
// the tree panics at wrap time.
//
// A nil lookup selects [os.LookupEnv]; non-nil values override it,
// primarily for test injection.
//
// CLI values always override environment values. When multiple
// EnvMiddleware layers are chained, the outer wrapper's bindings take
// precedence.
//
// For flags, the environment value is parsed as a boolean
// (case-insensitive):
//
//	true:  1, t, true, y, yes, on
//	false: 0, f, false, n, no, off
//
// An empty value is treated as "not set" for both flags and options.
// A non-empty flag value that does not parse as a boolean returns a
// usage error; option values are used as-is.
//
//	envMW := argv.EnvMiddleware(map[string]string{
//		"verbose": "APP_VERBOSE",
//		"host":    "APP_HOST",
//	}, nil)
//	mux.Handle("deploy", "Deploy", envMW(deployCmd))
func EnvMiddleware(env map[string]string, lookup LookupFunc) Middleware {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	return func(next Runner) Runner {
		if next == nil {
			panic("argv: nil runner")
		}
		walker, ok := next.(Walker)
		if !ok {
			panic("argv: EnvMiddleware: wrapped runner must implement Walker")
		}
		flagNames, optionNames := classifyInputs(walker)
		for name := range env {
			if !flagNames[name] && !optionNames[name] {
				panic(fmt.Sprintf("argv: EnvMiddleware: %q is not a declared flag or option", name))
			}
		}
		return NewMiddleware(func(out *Output, call *Call, _ Runner) error {
			for name, envVar := range env {
				val, ok := lookup(envVar)
				if !ok || val == "" {
					continue
				}
				if flagNames[name] {
					if _, ok := call.Flags.Lookup(name); ok {
						continue
					}
					b, err := parseEnvBool(val)
					if err != nil {
						return Errorf(ExitUsage, "argv: env var %s: %w", envVar, err)
					}
					call.Flags.Set(name, b)
					continue
				}
				if _, ok := call.Options.Lookup(name); ok {
					continue
				}
				call.Options.Set(name, val)
			}
			return next.RunArgv(out, call.WithContext(withEnv(call.Context(), lookup)))
		})(next)
	}
}

// classifyInputs walks w and returns the sets of declared flag and
// option names across the subtree.
func classifyInputs(w Walker) (flags, options map[string]bool) {
	flags = map[string]bool{}
	options = map[string]bool{}
	for help := range w.WalkArgv("", nil) {
		for _, f := range help.Flags {
			flags[f.Name] = true
		}
		for _, o := range help.Options {
			options[o.Name] = true
		}
	}
	return flags, options
}

// parseEnvBool parses an environment-variable string as a boolean.
// It accepts common shell and systemd conventions.
func parseEnvBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "y", "yes", "on":
		return true, nil
	case "0", "f", "false", "n", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean %q", s)
}
