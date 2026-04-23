package argv

import (
	"errors"
	"fmt"
	"os"
)

// ErrHelp indicates that help output was displayed instead of running
// a command.
var ErrHelp = errors.New("argv: help requested")

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
	// Code is the process exit code.
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
// written to stderr. An err wrapping an [*ExitError] exits with the
// wrapped Code. Any other non-nil err exits with [ExitFailure]. In
// the last two cases, Exit writes err to [os.Stderr] before exiting.
//
// Exit calls [os.Exit] directly; deferred functions do not run.
// Most callers want [Program.InvokeAndExit] instead, which composes
// [Program.Invoke] with Exit.
func Exit(err error) {
	// A typed-nil *ExitError — commonly returned by [Program.Invoke]
	// on success — is not nil when assigned to an error interface.
	// Normalize before downstream checks.
	if e, ok := err.(*ExitError); ok && e == nil {
		err = nil
	}
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
