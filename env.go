package argv

import (
	"fmt"
	"strings"
)

// EnvMiddleware returns middleware that reads flags and options from
// environment variables when they were not provided on the command
// line. Keys are flag or option names; values are environment variable
// names.
//
// For flags, the environment value is parsed as a boolean
// (case-insensitive):
//
//	true:  1, t, true, y, yes, on
//	false: 0, f, false, n, no, off
//
// An empty value is treated as "not set" for both flags and options.
// A non-empty flag value that does not parse as a boolean returns an
// error; option values are used as-is.
//
//	mw := argv.EnvMiddleware(
//		map[string]string{"verbose": "VERBOSE"},
//		map[string]string{"host": "API_HOST"},
//		os.LookupEnv,
//	)
func EnvMiddleware(flags, options map[string]string, lookupEnv LookupFunc) MiddlewareFunc {
	return func(next Runner) Runner {
		return RunnerFunc(func(out *Output, call *Call) error {
			for name, envVar := range flags {
				if _, ok := call.Flags.Lookup(name); ok {
					continue
				}
				val, ok := lookupEnv(envVar)
				if !ok || val == "" {
					continue
				}
				b, err := parseEnvBool(val)
				if err != nil {
					return Errorf(ExitUsage, "argv: env var %s: %w", envVar, err)
				}
				call.Flags.Set(name, b)
			}
			for name, envVar := range options {
				if _, ok := call.Options.Lookup(name); ok {
					continue
				}
				val, ok := lookupEnv(envVar)
				if !ok || val == "" {
					continue
				}
				call.Options.Set(name, val)
			}
			return next.RunCLI(out, call)
		})
	}
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
