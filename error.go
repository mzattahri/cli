package argv

import (
	"errors"
	"fmt"
	"os"
)

// ErrHelp is returned when help output was displayed instead of
// running a command. It is distinct from [ExitError].
var ErrHelp = errors.New("argv: help requested")

// Standard process exit codes used by [Program.Invoke]. ExitUsage
// covers both help output and usage errors (unknown flag, missing
// argument, etc.), following the POSIX convention of reserving exit
// code 2 for guidance or correction responses.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// An ExitError is an error with an explicit process exit code.
type ExitError struct {
	// Code is the process exit code.
	Code int

	// Err is the underlying error, if any.
	Err error
}

// Error returns the underlying error message, or the empty string if
// the underlying error is nil.
func (e *ExitError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *ExitError) Unwrap() error { return e.Err }

// Errorf returns an [*ExitError] with the given exit code and an
// underlying error formatted per [fmt.Errorf].
func Errorf(code int, format string, args ...any) *ExitError {
	return &ExitError{Code: code, Err: fmt.Errorf(format, args...)}
}

// Exit terminates the program with an exit code derived from err.
// A nil err exits zero. An err wrapping [ErrHelp] exits with
// [ExitUsage]; the help renderer has already written its output to
// stderr, so Exit prints nothing. An err wrapping an [*ExitError]
// exits with the wrapped Code. Any other non-nil err exits with
// [ExitFailure]. In the last two cases, Exit writes err to
// [os.Stderr] before exiting.
//
// Exit calls [os.Exit] directly, so deferred functions do not run.
// Callers that need deferred cleanup should use [Program.Invoke]
// and handle its return value inline.
func Exit(err error) {
	if err != nil && !errors.Is(err, ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(exitCode(err))
}

// exitCode maps err to a process exit code: 0 for nil, [ExitUsage] for
// [ErrHelp], the wrapped code for [ExitError], and [ExitFailure] for
// everything else.
func exitCode(err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, ErrHelp):
		return ExitUsage
	default:
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			return exitErr.Code
		}
		return ExitFailure
	}
}
