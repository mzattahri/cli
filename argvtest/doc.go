// Package argvtest provides test helpers for [argv] runners and muxes.
//
// It plays the same role as [net/http/httptest]: construct a [*argv.Call]
// with [NewCall], run a [argv.Runner] directly, and inspect captured
// output on a [Recorder]. No process, no os.Args, no signal handling —
// just the runner, its input, and its output.
package argvtest
