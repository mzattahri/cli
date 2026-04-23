package argv

import (
	"slices"
	"testing"
)

func TestFlagSet(t *testing.T) {
	var s FlagSet
	s.Set("verbose", true)
	s.Set("force", false)
	s.Set("quiet", true)
	
	t.Run("Has", func(t *testing.T) {
		if !s.Has("verbose") { t.Error("expected true") }
		if !s.Has("force") { t.Error("expected true") }
		if s.Has("missing") { t.Error("expected false") }
	})

	t.Run("Get", func(t *testing.T) {
		if !s.Get("verbose") { t.Error("expected true") }
		if s.Get("force") { t.Error("expected false") }
		if s.Get("missing") { t.Error("expected false") }
	})

	t.Run("Lookup", func(t *testing.T) {
		if v, ok := s.Lookup("verbose"); !ok || !v { t.Errorf("got (%v, %v)", v, ok) }
		if v, ok := s.Lookup("force"); !ok || v { t.Errorf("got (%v, %v) for force (explicitly set)", v, ok) }
		if _, ok := s.Lookup("missing"); ok { t.Error("expected ok=false") }
	})

	t.Run("String", func(t *testing.T) {
		got := s.String()
		want := "--quiet --verbose"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestOptionSet(t *testing.T) {
	var s OptionSet
	s.Set("host", "localhost")
	s.Set("user", "admin")

	t.Run("Has", func(t *testing.T) {
		if !s.Has("host") { t.Error("expected true") }
		if s.Has("missing") { t.Error("expected false") }
	})

	t.Run("Get", func(t *testing.T) {
		if s.Get("host") != "localhost" { t.Errorf("got %q", s.Get("host")) }
		if s.Get("missing") != "" { t.Errorf("got %q", s.Get("missing")) }
	})

	t.Run("GetReturnsLast", func(t *testing.T) {
		var multi OptionSet
		multi.Add("tag", "a")
		multi.Add("tag", "b")
		multi.Add("tag", "c")
		if got := multi.Get("tag"); got != "c" {
			t.Errorf("got %q, want last value", got)
		}
	})

	t.Run("Values", func(t *testing.T) {
		var multi OptionSet
		multi.Add("tag", "a")
		multi.Add("tag", "b")
		got := multi.Values("tag")
		if !slices.Equal(got, []string{"a", "b"}) {
			t.Errorf("got %v", got)
		}
		got[0] = "changed"
		if got := multi.Values("tag"); !slices.Equal(got, []string{"a", "b"}) {
			t.Errorf("values should be cloned, got %v", multi.Values("tag"))
		}
		if multi.Values("missing") != nil {
			t.Error("expected nil for missing key")
		}
	})

	t.Run("Add", func(t *testing.T) {
		s := OptionSet{}
		s.Add("tag", "a")
		s.Add("tag", "b")
		got := s.Values("tag")
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("Lookup", func(t *testing.T) {
		if v, ok := s.Lookup("host"); !ok || v != "localhost" { t.Errorf("got (%q, %v)", v, ok) }
		if _, ok := s.Lookup("missing"); ok { t.Error("expected ok=false") }
	})

	t.Run("String", func(t *testing.T) {
		got := s.String()
		want := "--host localhost --user admin"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("StringQuotesSpecialValues", func(t *testing.T) {
		var s OptionSet
		s.Set("name", "hello world")
		got := s.String()
		want := `--name "hello world"`
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestArgSet(t *testing.T) {
	var s ArgSet
	s.Set("path", "/tmp")
	s.Set("name", "test")

	t.Run("Has", func(t *testing.T) {
		if !s.Has("path") { t.Error("expected true") }
		if s.Has("missing") { t.Error("expected false") }
	})

	t.Run("Get", func(t *testing.T) {
		if s.Get("path") != "/tmp" { t.Errorf("got %q", s.Get("path")) }
		if s.Get("missing") != "" { t.Errorf("got %q", s.Get("missing")) }
	})

	t.Run("Lookup", func(t *testing.T) {
		if v, ok := s.Lookup("path"); !ok || v != "/tmp" { t.Errorf("got (%q, %v)", v, ok) }
		if _, ok := s.Lookup("missing"); ok { t.Error("expected ok=false") }
	})

	t.Run("Set", func(t *testing.T) {
		s := ArgSet{}
		s.Set("name", "alice")
		if got := s.Get("name"); got != "alice" {
			t.Fatalf("got %q, want %q", got, "alice")
		}
		s.Set("name", "bob")
		if got := s.Get("name"); got != "bob" {
			t.Fatalf("got %q after overwrite, want %q", got, "bob")
		}
	})

	t.Run("String", func(t *testing.T) {
		got := s.String()
		// Alphabetical order because it's a map
		want := "<name> test <path> /tmp"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("StringQuotesSpecialValues", func(t *testing.T) {
		var s ArgSet
		s.Set("path", "/my dir/file")
		got := s.String()
		want := `<path> "/my dir/file"`
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
