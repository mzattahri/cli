package argvtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strconv"
	"strings"

	"mz.attahri.com/code/argv"
)

// NewCall returns a [*argv.Call] built from a shell-style argument
// string. Whitespace separates tokens; double and single quotes
// preserve spaces within a token. Inside double quotes, \" and \\
// are honored as escapes. Single quotes are literal.
//
// The call uses [context.TODO]. Set Stdin on the returned Call for
// stdin-dependent tests. NewCall panics on an unclosed quote.
//
// Use [NewCallArgs] when tokens are already split, for example when
// forwarding a slice through a table-driven test.
func NewCall(args string) *argv.Call {
	tk := NewTokenizer(args)
	tokens := slices.Collect(tk.Tokens())
	if err := tk.Err(); err != nil {
		panic(fmt.Sprintf("argvtest: %s in %s", err, strconv.Quote(args)))
	}
	return argv.NewCall(context.TODO(), tokens)
}

// NewCallArgs returns a [*argv.Call] from a pre-tokenized slice. Use
// it when the shell-string tokenization in [NewCall] is unnecessary
// (tokens already split, or values contain characters the tokenizer
// would interpret).
//
// The call uses [context.TODO]. Set Stdin on the returned Call for
// stdin-dependent tests.
func NewCallArgs(args []string) *argv.Call {
	return argv.NewCall(context.TODO(), args)
}

// NewLookupFunc returns an [argv.LookupFunc] backed by env, suitable
// for test injection via [argv.EnvMiddleware]. A nil env produces a
// lookup that always reports a miss.
func NewLookupFunc(env map[string]string) argv.LookupFunc {
	return func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}
}

// A Recorder captures stdout and stderr, analogous to
// [net/http/httptest.ResponseRecorder].
type Recorder struct {
	stdout, stderr bytes.Buffer
}

// NewRecorder returns a [Recorder] with empty buffers.
func NewRecorder() *Recorder {
	return &Recorder{}
}

// Output returns a [*argv.Output] backed by the recorder's buffers.
func (r *Recorder) Output() *argv.Output {
	return &argv.Output{Stdout: &r.stdout, Stderr: &r.stderr}
}

// Stdout returns the captured stdout contents.
func (r *Recorder) Stdout() string { return r.stdout.String() }

// Stderr returns the captured stderr contents.
func (r *Recorder) Stderr() string { return r.stderr.String() }

// Len returns the total number of bytes written to both buffers.
func (r *Recorder) Len() int {
	return r.stdout.Len() + r.stderr.Len()
}

// Reset clears both buffers so a single Recorder can be reused across
// table-driven subtests.
func (r *Recorder) Reset() {
	r.stdout.Reset()
	r.stderr.Reset()
}

// A Tokenizer splits a shell-style argument string into argv tokens.
// Construct one with [NewTokenizer], then range over [Tokenizer.Tokens]:
//
//	for tok := range argvtest.NewTokenizer(`echo "hello world"`).Tokens() {
//		fmt.Println(tok)
//	}
//
// Tokens are separated by ASCII whitespace. Single quotes preserve
// their contents literally. Double quotes preserve spaces and treat
// \" and \\ as escapes; other backslashes are kept verbatim.
type Tokenizer struct {
	src     string
	pos     int
	current string
	failure error
}

// NewTokenizer returns a tokenizer that scans src.
func NewTokenizer(src string) *Tokenizer {
	return &Tokenizer{src: src}
}

// Tokens returns an iterator over the argv tokens in the source.
// Iteration stops at end of input or on error; check [Tokenizer.Err]
// afterward to distinguish.
func (t *Tokenizer) Tokens() iter.Seq[string] {
	return func(yield func(string) bool) {
		for t.scan() {
			if !yield(t.token()) {
				return
			}
		}
	}
}

// scan advances to the next token, exposed via [Tokenizer.token]. It
// returns false at EOF or on error; check [Tokenizer.err] to
// distinguish.
func (t *Tokenizer) scan() bool {
	if t.failure != nil {
		return false
	}
	for t.pos < len(t.src) && isTokenSpace(t.src[t.pos]) {
		t.pos++
	}
	if t.pos >= len(t.src) {
		return false
	}

	var b strings.Builder
	quote := byte(0)
loop:
	for t.pos < len(t.src) {
		c := t.src[t.pos]
		switch {
		case quote != 0 && c == quote:
			quote = 0
			t.pos++
		case quote == '"' && c == '\\' && t.pos+1 < len(t.src) && (t.src[t.pos+1] == '"' || t.src[t.pos+1] == '\\'):
			b.WriteByte(t.src[t.pos+1])
			t.pos += 2
		case quote != 0:
			b.WriteByte(c)
			t.pos++
		case c == '"' || c == '\'':
			quote = c
			t.pos++
		case isTokenSpace(c):
			break loop
		default:
			b.WriteByte(c)
			t.pos++
		}
	}

	if quote != 0 {
		t.failure = errors.New("unclosed quote")
		return false
	}
	t.current = b.String()
	return true
}

// token returns the most recent token produced by [Tokenizer.scan].
func (t *Tokenizer) token() string { return t.current }

// Err returns the first error encountered while scanning. EOF is not
// an error; Err returns nil when iteration stops at end of input.
func (t *Tokenizer) Err() error { return t.failure }

func isTokenSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' }
