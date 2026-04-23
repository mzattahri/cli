// Command todo is a worked example of a non-trivial CLI built with argv.
//
// It demonstrates:
//   - a root Mux with a global flag (verbose) and global option (store)
//   - multiple subcommands, each declared as a *argv.Command with its
//     own flags, options, and positional arguments
//   - a mounted sub-mux (todo list ...) with nested commands
//   - EnvMiddleware for env-var fallback (TODO_STORE)
//   - CompletionRunner wired at `todo complete`
//
// The backing store is an in-memory map keyed by a store path, so
// running `todo add "write docs" --due today` then `todo ls` behaves
// within a single process. A real implementation would persist to disk.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mz.attahri.com/code/argv"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	store := newStore()

	mux := argv.NewMux("todo")
	mux.Description = "A small todo CLI — demonstrates the argv extension model."
	mux.Flag("verbose", "v", false, "Print verbose diagnostics to stderr")
	mux.Option("store", "s", "todos.db", "Store location (also settable via TODO_STORE)")

	mux.Handle("add", "Add an item", addCommand(store))
	mux.Handle("done", "Mark an item done", doneCommand(store))
	mux.Handle("rm", "Remove an item", removeCommand(store))
	mux.Handle("ls", "List items", listCommand(store))

	listMux := argv.NewMux("list")
	listMux.Description = "Commands that operate on the todo list."
	listMux.Handle("clear", "Remove every item", clearCommand(store))
	listMux.Handle("count", "Print the number of items", countCommand(store))
	mux.Handle("list", "Manage the full list", listMux)

	mux.Handle("complete", "Emit shell completions", argv.CompletionRunner(mux))

	// EnvMiddleware fills in --store from TODO_STORE when not given on
	// the CLI. CLI values always win.
	envMW := argv.EnvMiddleware(
		nil,
		map[string]string{"store": "TODO_STORE"},
		os.LookupEnv,
	)

	program := &argv.Program{
		Name:  "todo",
		Usage: "Manage a list of todos",
		Description: "todo is a small worked example that ships with argv. " +
			"It demonstrates mux-level globals, mounted sub-muxes, " +
			"EnvMiddleware fallback, and shell-completion wiring.\n\n" +
			"The backing store is in-memory; items do not persist across runs.",
	}
	program.InvokeAndExit(ctx, envMW(mux), os.Args)
}

// ---- backing store ----------------------------------------------------------

type item struct {
	ID    int
	Title string
	Done  bool
	Due   time.Time
}

type store struct {
	mu    sync.Mutex
	next  int
	items map[int]item
}

func newStore() *store {
	return &store{items: map[int]item{}}
}

func (s *store) add(title string, due time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	s.items[s.next] = item{ID: s.next, Title: title, Due: due}
	return s.next
}

func (s *store) mark(id int, done bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[id]
	if !ok {
		return false
	}
	it.Done = done
	s.items[id] = it
	return true
}

func (s *store) remove(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return false
	}
	delete(s.items, id)
	return true
}

func (s *store) list() []item {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]item, 0, len(s.items))
	for _, it := range s.items {
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *store) clear() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.items)
	s.items = map[int]item{}
	return n
}

// ---- commands ---------------------------------------------------------------

func addCommand(s *store) *argv.Command {
	cmd := &argv.Command{
		Description: "Add a todo item. Multi-word titles must be quoted.",
		Run: func(out *argv.Output, call *argv.Call) error {
			due := parseDue(call.Options.Get("due"))
			id := s.add(call.Args.Get("title"), due)
			logVerbose(out, call, "store=%s", call.Options.Get("store"))
			_, err := fmt.Fprintf(out, "added #%d\n", id)
			return err
		},
	}
	cmd.Arg("title", "Item title")
	cmd.Option("due", "d", "", "Due date: today, tomorrow, or YYYY-MM-DD")
	return cmd
}

func doneCommand(s *store) *argv.Command {
	cmd := &argv.Command{
		Description: "Mark an item as done.",
		Run: func(out *argv.Output, call *argv.Call) error {
			id, err := strconv.Atoi(call.Args.Get("id"))
			if err != nil {
				return argv.Errorf(argv.ExitUsage, "invalid id: %s", call.Args.Get("id"))
			}
			if !s.mark(id, true) {
				return argv.Errorf(argv.ExitFailure, "no item with id %d", id)
			}
			_, err = fmt.Fprintf(out, "done #%d\n", id)
			return err
		},
	}
	cmd.Arg("id", "Item id")
	return cmd
}

func removeCommand(s *store) *argv.Command {
	cmd := &argv.Command{
		Description: "Remove an item.",
		Run: func(out *argv.Output, call *argv.Call) error {
			id, err := strconv.Atoi(call.Args.Get("id"))
			if err != nil {
				return argv.Errorf(argv.ExitUsage, "invalid id: %s", call.Args.Get("id"))
			}
			if !s.remove(id) {
				return argv.Errorf(argv.ExitFailure, "no item with id %d", id)
			}
			_, err = fmt.Fprintf(out, "removed #%d\n", id)
			return err
		},
	}
	cmd.Arg("id", "Item id")
	return cmd
}

func listCommand(s *store) *argv.Command {
	cmd := &argv.Command{
		Description: "List items, optionally filtered.",
		Run: func(out *argv.Output, call *argv.Call) error {
			showDone := call.Flags.Get("all")
			for _, it := range s.list() {
				if !showDone && it.Done {
					continue
				}
				mark := " "
				if it.Done {
					mark = "x"
				}
				due := ""
				if !it.Due.IsZero() {
					due = " (" + it.Due.Format("2006-01-02") + ")"
				}
				if _, err := fmt.Fprintf(out, "[%s] #%d %s%s\n", mark, it.ID, it.Title, due); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flag("all", "a", false, "Include completed items")
	return cmd
}

func clearCommand(s *store) *argv.Command {
	return &argv.Command{
		Description: "Remove every item from the list.",
		Run: func(out *argv.Output, call *argv.Call) error {
			n := s.clear()
			_, err := fmt.Fprintf(out, "cleared %d item(s)\n", n)
			return err
		},
	}
}

func countCommand(s *store) *argv.Command {
	return &argv.Command{
		Description: "Print the number of items currently in the list.",
		Run: func(out *argv.Output, call *argv.Call) error {
			_, err := fmt.Fprintln(out, len(s.list()))
			return err
		},
	}
}

// ---- helpers ----------------------------------------------------------------

func parseDue(s string) time.Time {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return time.Time{}
	case "today":
		return time.Now().Truncate(24 * time.Hour)
	case "tomorrow":
		return time.Now().Add(24 * time.Hour).Truncate(24 * time.Hour)
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func logVerbose(out *argv.Output, call *argv.Call, format string, args ...any) {
	if !call.Flags.Get("verbose") {
		return
	}
	fmt.Fprintf(out.Stderr, "%s: ", call.Pattern)
	fmt.Fprintf(out.Stderr, format, args...)
	io.WriteString(out.Stderr, "\n")
}
