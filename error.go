package cli

import (
	"errors"
)

// ErrHelp is returned when help output was displayed instead of
// running a command. It is distinct from [ExitError].
var ErrHelp = errors.New("cli: help requested")

// Standard process exit codes used by [Program.Invoke].
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitHelp    = 2
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

// exitCode maps err to a process exit code: 0 for nil, [ExitHelp] for
// [ErrHelp], the wrapped code for [ExitError], and [ExitFailure] for
// everything else.
func exitCode(err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, ErrHelp):
		return ExitHelp
	default:
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			return exitErr.Code
		}
		return ExitFailure
	}
}
