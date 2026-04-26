package argv

import (
	"errors"
	"fmt"
	"os"
)

// ErrHelp indicates that help output was displayed instead of running
// a command.
var ErrHelp = errors.New("argv: help requested")

// A HelpError reports that the user requested help, or that help was
// shown in lieu of running because a command could not be resolved.
// A [Runner] returns HelpError and [Program.Invoke] catches it,
// locates the matching node via [Walker], and renders help using the
// [Program.HelpFunc].
//
// Path identifies the command path where help applies, e.g.
// "app repo init". Explicit is true when the user typed --help; it
// is false when help was shown because a token did not match any
// registered subcommand. Reason is set only on implicit HelpErrors
// produced by navigation failures (e.g. an unknown subcommand) and
// is printed to stderr before the help body. Parse errors bypass
// HelpError entirely and return an [*ExitError] instead. Explicit
// requests exit zero; implicit ones exit [ExitUsage]. HelpError
// satisfies [errors.Is] against [ErrHelp].
type HelpError struct {
	Path     string
	Explicit bool
	Reason   string
}

// Error returns a stable diagnostic string.
func (e *HelpError) Error() string {
	if e == nil {
		return ""
	}
	return "argv: help requested for " + e.Path
}

// Is reports whether target is [ErrHelp], letting callers match with
// [errors.Is] without referencing [*HelpError] directly.
func (e *HelpError) Is(target error) bool { return target == ErrHelp }

// Standard process exit codes used by [Program.Invoke]. ExitUsage
// covers both help output and usage errors such as an unknown flag
// or missing argument, following the POSIX convention of reserving
// 2 for guidance or correction.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// An ExitError is an error carrying an explicit process exit code.
type ExitError struct {
	// Code is the process exit code. A zero Code with a non-nil Err
	// is coerced to [ExitFailure] by [Exit] and [Program.Run] so a
	// real failure never silently exits zero.
	Code int

	// Err is the underlying error, or nil.
	Err error
}

// Error returns the underlying error message. When Err is nil but
// Code is non-zero, Error returns "argv: exit code N" so a silent
// non-zero exit produces a visible message. A nil receiver or a zero
// Code with no Err returns the empty string.
func (e *ExitError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		if e.Code == 0 {
			return ""
		}
		return fmt.Sprintf("argv: exit code %d", e.Code)
	}
	return e.Err.Error()
}

// Unwrap returns the underlying error. It returns nil when the
// receiver is nil.
func (e *ExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Errorf returns an [*ExitError] with the given code and a formatted
// underlying error. It follows [fmt.Errorf].
func Errorf(code int, format string, args ...any) *ExitError {
	return &ExitError{Code: code, Err: fmt.Errorf(format, args...)}
}

// Exit terminates the program with an exit code derived from err.
// A nil err exits zero. An err wrapping [ErrHelp] exits with
// [ExitUsage] and prints nothing; the help renderer has already
// written to stderr. An err wrapping an [*ExitError] exits with
// the wrapped Code, printing err to [os.Stderr]. Any other non-nil
// err exits with [ExitFailure], also printing err to [os.Stderr].
//
// Exit calls [os.Exit] directly; deferred functions do not run.
// Most callers want [Program.Run] instead, which composes
// [Program.Invoke] with Exit.
func Exit(err error) {
	// Defensive: callers handing us a typed-nil *ExitError (direct or
	// wrapped) get the same treatment as an untyped nil.
	var e *ExitError
	if errors.As(err, &e) && e == nil {
		err = nil
	}
	if err != nil && !errors.Is(err, ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(exitCode(err))
}

// exitCode maps err to a process exit code: 0 for nil, [ExitUsage] for
// [ErrHelp], the wrapped code for [ExitError], and [ExitFailure] for
// everything else. A wrapped [*ExitError] with a zero Code and a
// non-nil Err is coerced to [ExitFailure] so a real failure never
// silently exits zero.
func exitCode(err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, ErrHelp):
		return ExitUsage
	default:
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			if exitErr.Code == 0 && exitErr.Err != nil {
				return ExitFailure
			}
			return exitErr.Code
		}
		return ExitFailure
	}
}
