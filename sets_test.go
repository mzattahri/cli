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

	t.Run("Get", func(t *testing.T) {
		if !s.Get("verbose") {
			t.Error("expected true")
		}
		if s.Get("force") {
			t.Error("expected false")
		}
		if s.Get("missing") {
			t.Error("expected false")
		}
	})

	t.Run("Lookup", func(t *testing.T) {
		if v, ok := s.Lookup("verbose"); !ok || !v {
			t.Errorf("got (%v, %v)", v, ok)
		}
		if v, ok := s.Lookup("force"); !ok || v {
			t.Errorf("got (%v, %v) for force (explicitly set)", v, ok)
		}
		if _, ok := s.Lookup("missing"); ok {
			t.Error("expected ok=false")
		}
	})

	t.Run("String", func(t *testing.T) {
		got := s.String()
		want := "--quiet --verbose"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("Del", func(t *testing.T) {
		s := FlagSet{}
		s.Set("verbose", true)
		s.Del("verbose")
		if v, ok := s.Lookup("verbose"); ok || v {
			t.Errorf("after Del got (%v, %v), want (false, false)", v, ok)
		}
		s.Del("missing") // no-op
		var zero FlagSet
		zero.Del("anything") // no-op on nil map
	})
}

func TestOptionSet(t *testing.T) {
	var s OptionSet
	s.Set("host", "localhost")
	s.Set("user", "admin")

	t.Run("Get", func(t *testing.T) {
		if s.Get("host") != "localhost" {
			t.Errorf("got %q", s.Get("host"))
		}
		if s.Get("missing") != "" {
			t.Errorf("got %q", s.Get("missing"))
		}
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
		if v, ok := s.Lookup("host"); !ok || v != "localhost" {
			t.Errorf("got (%q, %v)", v, ok)
		}
		if _, ok := s.Lookup("missing"); ok {
			t.Error("expected ok=false")
		}
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

	t.Run("Del", func(t *testing.T) {
		s := OptionSet{}
		s.Add("tag", "a")
		s.Add("tag", "b")
		s.Del("tag")
		if got := s.Values("tag"); got != nil {
			t.Errorf("after Del got %v, want nil", got)
		}
		if _, ok := s.Lookup("tag"); ok {
			t.Error("expected ok=false after Del")
		}
		s.Del("missing") // no-op
		var zero OptionSet
		zero.Del("anything") // no-op on nil map
	})
}

func TestArgSet(t *testing.T) {
	var s ArgSet
	s.Set("path", "/tmp")
	s.Set("name", "test")

	t.Run("Get", func(t *testing.T) {
		if s.Get("path") != "/tmp" {
			t.Errorf("got %q", s.Get("path"))
		}
		if s.Get("missing") != "" {
			t.Errorf("got %q", s.Get("missing"))
		}
	})

	t.Run("Lookup", func(t *testing.T) {
		if v, ok := s.Lookup("path"); !ok || v != "/tmp" {
			t.Errorf("got (%q, %v)", v, ok)
		}
		if _, ok := s.Lookup("missing"); ok {
			t.Error("expected ok=false")
		}
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

	t.Run("Del", func(t *testing.T) {
		s := ArgSet{}
		s.Set("path", "/tmp")
		s.Del("path")
		if v, ok := s.Lookup("path"); ok || v != "" {
			t.Errorf("after Del got (%q, %v), want (\"\", false)", v, ok)
		}
		s.Del("missing") // no-op
		var zero ArgSet
		zero.Del("anything") // no-op on nil map
	})
}

func TestFlagSetMergeIsolation(t *testing.T) {
	var dst FlagSet
	var src FlagSet
	src.Set("verbose", true)

	dst.merge(src)
	src.Set("verbose", false)

	if v, _ := dst.Lookup("verbose"); !v {
		t.Fatalf("dst should retain true after src mutation, got %v", v)
	}
}

func TestOptionSetMergeIsolation(t *testing.T) {
	var dst OptionSet
	var src OptionSet
	src.Add("tag", "a")

	dst.merge(src)
	src.Add("tag", "b")

	if got := dst.Values("tag"); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("dst should retain [a] after src mutation, got %v", got)
	}
}

func TestFlagSetClone(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var src FlagSet
		c := src.Clone()
		c.Set("verbose", true)
		if _, ok := src.Lookup("verbose"); ok {
			t.Fatal("clone of zero FlagSet shared storage with src")
		}
	})
	t.Run("populated", func(t *testing.T) {
		var src FlagSet
		src.Set("verbose", true)
		src.Set("force", false)
		c := src.Clone()
		c.Set("verbose", false)
		c.Set("new", true)
		if v, _ := src.Lookup("verbose"); !v {
			t.Fatal("src mutated by clone change")
		}
		if _, ok := src.Lookup("new"); ok {
			t.Fatal("src observed clone-only entry")
		}
		if v, _ := c.Lookup("force"); v {
			t.Fatal("clone lost original entry")
		}
	})
}

func TestOptionSetClone(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var src OptionSet
		c := src.Clone()
		c.Set("host", "x")
		if _, ok := src.Lookup("host"); ok {
			t.Fatal("clone of zero OptionSet shared storage with src")
		}
	})
	t.Run("populated", func(t *testing.T) {
		var src OptionSet
		src.Add("tag", "a")
		src.Add("tag", "b")
		c := src.Clone()
		c.Add("tag", "c")
		if got := src.Values("tag"); !slices.Equal(got, []string{"a", "b"}) {
			t.Fatalf("src mutated through clone: %v", got)
		}
		if got := c.Values("tag"); !slices.Equal(got, []string{"a", "b", "c"}) {
			t.Fatalf("clone missing appended value: %v", got)
		}
	})
}

func TestArgSetClone(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var src ArgSet
		c := src.Clone()
		c.Set("path", "/tmp")
		if _, ok := src.Lookup("path"); ok {
			t.Fatal("clone of zero ArgSet shared storage with src")
		}
	})
	t.Run("populated", func(t *testing.T) {
		var src ArgSet
		src.Set("path", "/tmp")
		src.Set("name", "alice")
		c := src.Clone()
		c.Set("path", "/var")
		c.Set("new", "added")
		if got := src.Get("path"); got != "/tmp" {
			t.Fatalf("src mutated: got %q", got)
		}
		if _, ok := src.Lookup("new"); ok {
			t.Fatal("src observed clone-only entry")
		}
		if got := c.Get("name"); got != "alice" {
			t.Fatalf("clone lost original entry: got %q", got)
		}
	})
}

func TestArgSetAll(t *testing.T) {
	var s ArgSet
	s.Set("path", "/tmp")
	s.Set("name", "alice")
	s.Set("kind", "regular")

	collected := map[string]string{}
	var order []string
	for k, v := range s.All() {
		collected[k] = v
		order = append(order, k)
	}
	if len(collected) != 3 || collected["path"] != "/tmp" || collected["name"] != "alice" || collected["kind"] != "regular" {
		t.Fatalf("got %v", collected)
	}
	if !slices.IsSorted(order) {
		t.Fatalf("All did not yield in sorted order: %v", order)
	}

	// Early termination must stop iteration.
	var seen int
	for range s.All() {
		seen++
		if seen == 1 {
			break
		}
	}
	if seen != 1 {
		t.Fatalf("expected break to stop after 1 yield, got %d", seen)
	}
}

func TestSetValidatesName(t *testing.T) {
	cases := []struct {
		name     string
		badName  string
		mutation func(string)
	}{
		{"FlagSet empty", "", func(n string) { (&FlagSet{}).Set(n, true) }},
		{"FlagSet space", "has space", func(n string) { (&FlagSet{}).Set(n, true) }},
		{"FlagSet equals", "has=equals", func(n string) { (&FlagSet{}).Set(n, true) }},
		{"FlagSet dash", "-dash", func(n string) { (&FlagSet{}).Set(n, true) }},
		{"FlagSet tab", "has\ttab", func(n string) { (&FlagSet{}).Set(n, true) }},
		{"OptionSet.Set empty", "", func(n string) { (&OptionSet{}).Set(n, "v") }},
		{"OptionSet.Set space", "has space", func(n string) { (&OptionSet{}).Set(n, "v") }},
		{"OptionSet.Add empty", "", func(n string) { (&OptionSet{}).Add(n, "v") }},
		{"OptionSet.Add dash", "-dash", func(n string) { (&OptionSet{}).Add(n, "v") }},
		{"ArgSet empty", "", func(n string) { (&ArgSet{}).Set(n, "v") }},
		{"ArgSet equals", "has=equals", func(n string) { (&ArgSet{}).Set(n, "v") }},
		{"ArgSet angle open", "<path>", func(n string) { (&ArgSet{}).Set(n, "v") }},
		{"argSpecs angle", "<path>", func(n string) { (&argSpecs{}).add(n, "") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("expected panic for %q", tc.badName)
				}
			}()
			tc.mutation(tc.badName)
		})
	}
}
