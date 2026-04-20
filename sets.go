package cli

import (
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"
)

// checkCrossCollision panics if name or short collide with a different
// input kind (e.g. a flag name matching an existing option name).
func checkCrossCollision(name, short string, hasName func(string) bool, hasShort func(string) bool) {
	if hasName(name) {
		panic("cli: duplicate input " + `"` + name + `"`)
	}
	if short != "" && hasShort(short) {
		panic("cli: duplicate short input " + `"` + short + `"`)
	}
}

// validateInputSpec checks name and short for common input-declaration
// constraints. It panics on empty names, invalid characters, reserved
// names, and duplicates reported by hasName/hasShort. It returns the
// validated short name.
func validateInputSpec(kind, name, short string, hasName func(string) bool, hasShort func(string) bool) string {
	if name == "" {
		panic("cli: empty " + kind + " name")
	}
	if strings.ContainsAny(name, "= \t") || strings.HasPrefix(name, "-") {
		panic("cli: invalid " + kind + " name " + `"` + name + `"`)
	}
	if name == "help" {
		panic(`cli: reserved ` + kind + ` name "help"`)
	}
	if short != "" {
		short = validateShortName(short)
	}
	if hasName(name) {
		panic("cli: duplicate " + kind + " " + `"` + name + `"`)
	}
	if short != "" && hasShort(short) {
		panic("cli: duplicate short " + kind + " " + `"` + short + `"`)
	}
	return short
}

type flagSpecs struct {
	specs []flagSpec
}

func (s *flagSpecs) add(name, short string, value bool, usage string) {
	short = validateInputSpec("flag", name, short, s.hasName, s.hasShort)
	s.specs = append(s.specs, flagSpec{Name: name, Short: short, Usage: usage, Default: value})
}

func (s *flagSpecs) names() []string {
	if s == nil {
		return nil
	}
	names := make([]string, 0, len(s.specs))
	for _, spec := range s.specs {
		names = append(names, spec.Name)
	}
	return names
}

func (s *flagSpecs) hasName(name string) bool {
	if s == nil {
		return false
	}
	for _, spec := range s.specs {
		if spec.Name == name {
			return true
		}
	}
	return false
}

func (s *flagSpecs) hasShort(short string) bool {
	if s == nil || short == "" {
		return false
	}
	for _, spec := range s.specs {
		if spec.Short == short {
			return true
		}
	}
	return false
}

func (s *flagSpecs) helpEntries() []HelpFlag {
	return s.helpEntriesNegatable(false)
}

func (s *flagSpecs) helpEntriesNegatable(negatable bool) []HelpFlag {
	if s == nil {
		return nil
	}
	out := make([]HelpFlag, 0, len(s.specs))
	for _, spec := range s.specs {
		out = append(out, HelpFlag{
			Name:      spec.Name,
			Short:     spec.Short,
			Usage:     spec.Usage,
			Default:   spec.Default,
			Negatable: negatable,
		})
	}
	return out
}

type optionSpecs struct {
	specs []optionSpec
}

func (s *optionSpecs) add(name, short, value, usage string) {
	short = validateInputSpec("option", name, short, s.hasName, s.hasShort)
	s.specs = append(s.specs, optionSpec{Name: name, Short: short, Usage: usage, Default: value})
}

func (s *optionSpecs) names() []string {
	if s == nil {
		return nil
	}
	names := make([]string, 0, len(s.specs))
	for _, spec := range s.specs {
		names = append(names, spec.Name)
	}
	return names
}

func (s *optionSpecs) hasName(name string) bool {
	if s == nil {
		return false
	}
	for _, spec := range s.specs {
		if spec.Name == name {
			return true
		}
	}
	return false
}

func (s *optionSpecs) hasShort(short string) bool {
	if s == nil || short == "" {
		return false
	}
	for _, spec := range s.specs {
		if spec.Short == short {
			return true
		}
	}
	return false
}

func (s *optionSpecs) helpEntries() []HelpOption {
	if s == nil {
		return nil
	}
	out := make([]HelpOption, 0, len(s.specs))
	for _, spec := range s.specs {
		out = append(out, HelpOption{
			Name:    spec.Name,
			Short:   spec.Short,
			Usage:   spec.Usage,
			Default: spec.Default,
		})
	}
	return out
}

type argSpecs struct {
	specs []argSpec
}

func (s *argSpecs) add(name, usage string) {
	if name == "" {
		panic("cli: empty argument name")
	}
	for _, existing := range s.specs {
		if existing.Name == name {
			panic("cli: duplicate argument " + `"` + name + `"`)
		}
	}
	s.specs = append(s.specs, argSpec{Name: name, Usage: usage})
}

func (s *argSpecs) parse(args []string, captureRest bool) (ArgSet, []string, error) {
	var parsed ArgSet
	i := 0
	for _, spec := range s.specs {
		if i >= len(args) {
			return ArgSet{}, nil, fmt.Errorf("missing argument %q", spec.Name)
		}
		parsed.Set(spec.Name, args[i])
		i++
	}
	if captureRest {
		return parsed, slices.Clone(args[i:]), nil
	}
	if i < len(args) {
		return ArgSet{}, nil, fmt.Errorf("unexpected argument %q", args[i])
	}
	return parsed, nil, nil
}

func (s *argSpecs) HelpArguments() []HelpArg {
	if s == nil {
		return nil
	}
	out := make([]HelpArg, 0, len(s.specs))
	for _, spec := range s.specs {
		out = append(out, HelpArg{Name: "<" + spec.Name + ">", Usage: spec.Usage})
	}
	return out
}

func (s *argSpecs) names() []string {
	if s == nil {
		return nil
	}
	names := make([]string, 0, len(s.specs))
	for _, spec := range s.specs {
		names = append(names, spec.Name)
	}
	return names
}

// Entry types carry a value and its provenance.

type flagEntry struct {
	value    bool
	explicit bool
}

type optionEntry struct {
	values   []string
	explicit bool
}

type argEntry struct {
	value    string
	explicit bool
}

// A FlagSet holds boolean flag values with provenance tracking.
// The zero value is an empty, usable set.
type FlagSet struct{ m map[string]flagEntry }

// String returns a space-separated list of present flags (e.g. "--verbose").
func (s FlagSet) String() string {
	var names []string
	for k, e := range s.m {
		if e.value {
			names = append(names, "--"+k)
		}
	}
	slices.Sort(names)
	return strings.Join(names, " ")
}

// Has reports whether name exists in the set (explicit or default).
func (s FlagSet) Has(name string) bool {
	if s.m == nil {
		return false
	}
	_, ok := s.m[name]
	return ok
}

// Get returns the value associated with name, or false if not present.
func (s FlagSet) Get(name string) bool {
	if s.m == nil {
		return false
	}
	return s.m[name].value
}

// Explicit reports whether name was set on the command line rather than
// applied as a default.
func (s FlagSet) Explicit(name string) bool {
	if s.m == nil {
		return false
	}
	e, ok := s.m[name]
	return ok && e.explicit
}

// Set associates name with value, marking it as explicitly provided.
func (s *FlagSet) Set(name string, value bool) {
	if s.m == nil {
		s.m = make(map[string]flagEntry)
	}
	s.m[name] = flagEntry{value: value, explicit: true}
}

func (s *FlagSet) setDefault(name string, value bool) {
	if s.m == nil {
		s.m = make(map[string]flagEntry)
	}
	if _, ok := s.m[name]; !ok {
		s.m[name] = flagEntry{value: value}
	}
}

func (s *FlagSet) merge(other FlagSet) {
	if len(other.m) == 0 {
		return
	}
	if s.m == nil {
		s.m = make(map[string]flagEntry, len(other.m))
	}
	maps.Copy(s.m, other.m)
}

// Clone returns a deep copy of s.
func (s FlagSet) Clone() FlagSet {
	if s.m == nil {
		return FlagSet{}
	}
	return FlagSet{m: maps.Clone(s.m)}
}

// Len returns the number of entries in s.
func (s FlagSet) Len() int { return len(s.m) }

// All returns an iterator over flag names and values.
func (s FlagSet) All() iter.Seq2[string, bool] {
	return func(yield func(string, bool) bool) {
		for k, e := range s.m {
			if !yield(k, e.value) {
				return
			}
		}
	}
}

// An OptionSet holds named value options with provenance tracking.
// Each option may carry multiple values when repeated on the command
// line (e.g. --tag foo --tag bar). [OptionSet.Get] returns the last
// value; [OptionSet.Values] returns all values.
type OptionSet struct{ m map[string]optionEntry }

// String returns a space-separated list of options (e.g. "--host localhost").
// Multi-valued options are expanded into separate entries.
func (s OptionSet) String() string {
	var pairs []string
	for k, e := range s.m {
		for _, v := range e.values {
			pairs = append(pairs, fmt.Sprintf("--%s %s", k, quoteToken(v)))
		}
	}
	slices.Sort(pairs)
	return strings.Join(pairs, " ")
}

// Has reports whether name exists in the set (explicit or default).
func (s OptionSet) Has(name string) bool {
	if s.m == nil {
		return false
	}
	_, ok := s.m[name]
	return ok
}

// Get returns the last value associated with name, or the empty string
// if the option is not present. For options specified multiple times,
// use [OptionSet.Values] to retrieve all values.
func (s OptionSet) Get(name string) string {
	if s.m == nil {
		return ""
	}
	e, ok := s.m[name]
	if !ok || len(e.values) == 0 {
		return ""
	}
	return e.values[len(e.values)-1]
}

// Values returns all values associated with name in the order they
// appeared on the command line. It returns nil if name is not present.
func (s OptionSet) Values(name string) []string {
	if s.m == nil {
		return nil
	}
	e, ok := s.m[name]
	if !ok {
		return nil
	}
	return slices.Clone(e.values)
}

// Explicit reports whether name was set on the command line rather than
// applied as a default.
func (s OptionSet) Explicit(name string) bool {
	if s.m == nil {
		return false
	}
	e, ok := s.m[name]
	return ok && e.explicit
}

// Set replaces name with a single value, marking it as explicitly
// provided.
func (s *OptionSet) Set(name string, value string) {
	if s.m == nil {
		s.m = make(map[string]optionEntry)
	}
	s.m[name] = optionEntry{values: []string{value}, explicit: true}
}

// Add appends value to the values associated with name, marking it as
// explicitly provided.
func (s *OptionSet) Add(name string, value string) {
	if s.m == nil {
		s.m = make(map[string]optionEntry)
	}
	e := s.m[name]
	e.values = append(e.values, value)
	e.explicit = true
	s.m[name] = e
}

func (s *OptionSet) setDefault(name, value string) {
	if s.m == nil {
		s.m = make(map[string]optionEntry)
	}
	if _, ok := s.m[name]; !ok {
		s.m[name] = optionEntry{values: []string{value}}
	}
}

func (s *OptionSet) merge(other OptionSet) {
	if len(other.m) == 0 {
		return
	}
	if s.m == nil {
		s.m = make(map[string]optionEntry, len(other.m))
	}
	for k, e := range other.m {
		s.m[k] = optionEntry{values: slices.Clone(e.values), explicit: e.explicit}
	}
}

// Clone returns a deep copy of s. The returned OptionSet shares no
// slice storage with s, so either can be mutated independently.
func (s OptionSet) Clone() OptionSet {
	if s.m == nil {
		return OptionSet{}
	}
	out := make(map[string]optionEntry, len(s.m))
	for k, e := range s.m {
		out[k] = optionEntry{values: slices.Clone(e.values), explicit: e.explicit}
	}
	return OptionSet{m: out}
}

// Len returns the number of entries in s.
func (s OptionSet) Len() int { return len(s.m) }

// All returns an iterator over option names and value slices.
// Yielded slices are cloned; callers cannot mutate internal state.
func (s OptionSet) All() iter.Seq2[string, []string] {
	return func(yield func(string, []string) bool) {
		for k, e := range s.m {
			if !yield(k, slices.Clone(e.values)) {
				return
			}
		}
	}
}

// An ArgSet holds bound positional arguments with provenance tracking.
type ArgSet struct{ m map[string]argEntry }

// String returns a space-separated list of arguments (e.g. "<path> /tmp").
func (s ArgSet) String() string {
	var pairs []string
	for k, e := range s.m {
		pairs = append(pairs, fmt.Sprintf("<%s> %s", k, quoteToken(e.value)))
	}
	slices.Sort(pairs)
	return strings.Join(pairs, " ")
}

// Has reports whether name exists in the set.
func (s ArgSet) Has(name string) bool {
	if s.m == nil {
		return false
	}
	_, ok := s.m[name]
	return ok
}

// Get returns the value associated with name, or the empty string if
// not present.
func (s ArgSet) Get(name string) string {
	if s.m == nil {
		return ""
	}
	return s.m[name].value
}

// Explicit reports whether name was set on the command line rather than
// applied as a default.
func (s ArgSet) Explicit(name string) bool {
	if s.m == nil {
		return false
	}
	e, ok := s.m[name]
	return ok && e.explicit
}

// Set associates name with value, marking it as explicitly provided.
func (s *ArgSet) Set(name string, value string) {
	if s.m == nil {
		s.m = make(map[string]argEntry)
	}
	s.m[name] = argEntry{value: value, explicit: true}
}

// Clone returns a deep copy of s.
func (s ArgSet) Clone() ArgSet {
	if s.m == nil {
		return ArgSet{}
	}
	return ArgSet{m: maps.Clone(s.m)}
}

// Len returns the number of entries in s.
func (s ArgSet) Len() int { return len(s.m) }

// All returns an iterator over argument names and values.
func (s ArgSet) All() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for k, e := range s.m {
			if !yield(k, e.value) {
				return
			}
		}
	}
}
