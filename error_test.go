package cli

import (
	"errors"
	"testing"
)

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", err: nil, want: ExitOK},
		{name: "help", err: ErrHelp, want: ExitHelp},
		{name: "default", err: errors.New("boom"), want: ExitFailure},
		{name: "exit error", err: &ExitError{Code: 42, Err: errors.New("nope")}, want: 42},
		{name: "wrapped exit error", err: errors.Join(errors.New("outer"), &ExitError{Code: 7, Err: errors.New("inner")}), want: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCode(tt.err); got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExitErrorMessage(t *testing.T) {
	t.Run("with error", func(t *testing.T) {
		e := &ExitError{Code: 1, Err: errors.New("boom")}
		if got := e.Error(); got != "boom" {
			t.Fatalf("got %q, want %q", got, "boom")
		}
	})
	t.Run("nil error", func(t *testing.T) {
		e := &ExitError{Code: 0}
		if got := e.Error(); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
	t.Run("unwrap", func(t *testing.T) {
		inner := errors.New("inner")
		e := &ExitError{Code: 1, Err: inner}
		if got := e.Unwrap(); got != inner {
			t.Fatalf("got %v, want %v", got, inner)
		}
	})
}

func TestExitCodeConstants(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{name: "ok", got: ExitOK, want: 0},
		{name: "failure", got: ExitFailure, want: 1},
		{name: "help", got: ExitHelp, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %d, want %d", tt.got, tt.want)
			}
		})
	}
}
