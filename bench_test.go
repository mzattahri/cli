package argv

import (
	"context"
	"io"
	"testing"
)

func benchMuxFixture() *Mux {
	root := &Mux{}
	root.Flag("verbose", "v", false, "verbose")
	root.Option("config", "c", "", "config file")

	sub := &Mux{}
	sub.Option("path", "p", ".", "repo path")

	cmd := &Command{
		Run: func(*Output, *Call) error { return nil },
	}
	cmd.Flag("force", "f", false, "force")
	cmd.Flag("dry-run", "n", false, "dry run")
	cmd.Option("message", "m", "", "commit message")
	cmd.Arg("target", "deploy target")
	sub.Handle("init", "Initialize", cmd)
	root.Handle("repo", "Repository operations", sub)
	return root
}

func BenchmarkProgramInvokeLeaf(b *testing.B) {
	mux := benchMuxFixture()
	program := &Program{Stdout: io.Discard, Stderr: io.Discard}
	args := []string{"app", "--verbose", "--config", "prod.toml", "repo", "--path", "/tmp", "init", "--force", "--message", "hi", "production"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = program.Invoke(ctx, mux, args)
	}
}

func BenchmarkMuxRunArgvLeaf(b *testing.B) {
	mux := benchMuxFixture()
	out := &Output{Stdout: io.Discard, Stderr: io.Discard}
	argv := []string{"--verbose", "--config", "prod.toml", "repo", "--path", "/tmp", "init", "--force", "--message", "hi", "production"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		call := NewCall(ctx, argv)
		_ = mux.RunArgv(out, call)
	}
}

func BenchmarkParseInput(b *testing.B) {
	flags := &flagSpecs{}
	flags.add("verbose", "v", false, "verbose")
	flags.add("force", "f", false, "force")
	flags.add("dry-run", "n", false, "dry run")
	options := &optionSpecs{}
	options.add("config", "c", "", "config")
	options.add("message", "m", "", "message")
	args := []string{"--verbose", "--config", "prod.toml", "--force", "--message", "hi", "production"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parseInput(flags, options, args, false)
	}
}

func BenchmarkMuxRunArgvSimple(b *testing.B) {
	mux := &Mux{}
	cmd := &Command{
		Run: func(*Output, *Call) error { return nil },
	}
	cmd.Flag("force", "f", false, "force")
	cmd.Arg("name", "name")
	mux.Handle("run", "", cmd)

	out := &Output{Stdout: io.Discard, Stderr: io.Discard}
	argv := []string{"run", "--force", "production"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mux.RunArgv(out, NewCall(ctx, argv))
	}
}
