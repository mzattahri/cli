package argv

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func TestCallStringEmptyWhenUnparsed(t *testing.T) {
	call := NewCall(context.Background(), []string{"app", "say", "hello"})

	if got := call.String(); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestCallStringCanonicalizesParsedCall(t *testing.T) {
	mux := &Mux{}
	mux.Flag("verbose", "v", false, "verbose")
	mux.Option("config", "c", "", "config")

	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			_, err := out.Stdout.Write([]byte(call.String()))
			return err
		},
	}
	cmd.Flag("force", "f", false, "force")
	cmd.Flag("cache", "", true, "cache")
	cmd.Option("tag", "t", "", "tag")
	cmd.Arg("image", "image")
	cmd.Arg("target", "target")
	cmd.Tail("rest", "")
	mux.Handle("deploy", "", cmd)

	var out bytes.Buffer
	err := runMux(context.Background(), mux, &out, io.Discard, []string{
		"--config", "prod.toml",
		"--verbose",
		"deploy",
		"--force",
		"--tag", "b",
		"--cache=false",
		"--tag", "a",
		"alpine",
		"prod",
		"tail",
		"-f",
	})
	if err != nil {
		t.Fatal(err)
	}

	want := `app deploy flag:cache=false flag:force=true flag:verbose=true opt:config=prod.toml opt:tag=b opt:tag=a arg:image=alpine arg:target=prod tail:tail tail:-f`
	if got := out.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCallStringNormalizesEquivalentInvocations(t *testing.T) {
	mux := &Mux{}
	mux.Flag("verbose", "v", false, "verbose")

	cmd := &Command{
		Run: func(out *Output, call *Call) error {
			_, err := out.Stdout.Write([]byte(call.String()))
			return err
		},
	}
	cmd.Flag("force", "f", false, "force")
	cmd.Option("tag", "t", "", "tag")
	cmd.Arg("image", "image")
	mux.Handle("deploy", "", cmd)

	run := func(argv []string) string {
		t.Helper()
		var out bytes.Buffer
		if err := runMux(context.Background(), mux, &out, io.Discard, argv); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}

	got1 := run([]string{"--verbose", "deploy", "--force", "--tag", "a", "alpine"})
	got2 := run([]string{"-v", "deploy", "-t", "a", "-f", "alpine"})

	if got1 != got2 {
		t.Fatalf("canonical strings differ: %q != %q", got1, got2)
	}
}
