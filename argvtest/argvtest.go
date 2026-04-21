package argvtest

import (
	"bytes"
	"context"
	"strings"

	"github.com/mzattahri/argv"
)

// NewCall returns a [*argv.Call] from a space-separated argument string and
// optional raw stdin bytes. The call uses [context.Background].
func NewCall(arg string, stdin []byte) *argv.Call {
	call := argv.NewCall(context.Background(), strings.Fields(arg))
	if stdin != nil {
		call.Stdin = bytes.NewReader(stdin)
	}
	return call
}

// A Recorder captures stdout and stderr, analogous to
// [net/http/httptest.ResponseRecorder].
type Recorder struct {
	Stdout bytes.Buffer
	Stderr bytes.Buffer
}

// NewRecorder returns a [Recorder] with empty buffers.
func NewRecorder() *Recorder {
	return &Recorder{}
}

// Output returns a [*argv.Output] backed by the recorder's buffers.
func (r *Recorder) Output() *argv.Output {
	return &argv.Output{Stdout: &r.Stdout, Stderr: &r.Stderr}
}

// Len returns the total number of bytes written to both buffers.
func (r *Recorder) Len() int {
	return r.Stdout.Len() + r.Stderr.Len()
}

// Reset clears both buffers.
func (r *Recorder) Reset() {
	r.Stdout.Reset()
	r.Stderr.Reset()
}
