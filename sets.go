package argv

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
		panic(fmt.Sprintf("argv: duplicate input %q", name))
	}
	if short != "" && hasShort(short) {
		panic(fmt.Sprintf("argv: duplicate short input %q", short))
	}
}

func validateInputName(name string) {
	if name == "" {
		panic("argv: empty name")
	}
	first := name[0]
	if !isLetter(first) {
		panic(fmt.Sprintf("argv: invalid name %q", name))
	}
	for i := 1; i < len(name); i++ {
		b := name[i]
		if !isLetter(b) && !isDigit(b) && b != '-' {
			panic(fmt.Sprintf("argv: invalid name %q", name))
		}
	}
}

func isLetter(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }
func isDigit(b byte) bool  { return b >= '0' && b <= '9' }

// validateInputSpec checks name and short for common input-declaration
// constraints. It panics on empty names, invalid characters, reserved
// names, and duplicates reported by hasName/hasShort. It returns the
// validated short name.
func validateInputSpec(kind, name, short string, hasName func(string) bool, hasShort func(string) bool) string {
	validateInputName(name)
	if name == "help" {
		panic(fmt.Sprintf("argv: reserved %s name %q", kind, name))
	}
	if short != "" {
		short = validateShortName(short)
	}
	if hasName(name) {
		panic(fmt.Sprintf("argv: duplicate %s %q", kind, name))
	}
	if short != "" && hasShort(short) {
		panic(fmt.Sprintf("argv: duplicate short %s %q", kind, short))
	}
	return short
}

type flagSpecs struct {
	specs []flagSpec
}

func (s *flagSpecs) add(name, short string, value bool, usage string) {
	short = validateInputSpec("flag", name, short, s.hasName, s.hasShort)
	if other, ok := negatedCounterpart(name); ok && s.hasName(other) {
		panic(fmt.Sprintf("argv: flag %q collides with negation of %q", name, other))
	}
	s.specs = append(s.specs, flagSpec{Name: name, Short: short, Usage: usage, Default: value})
}

// negatedCounterpart returns the flag name that would be reached by
// bidirectional "no-" negation. For "cache" it returns "no-cache";
// for "no-cache" it returns "cache". Declaring both is rejected
// because --cache and --no-cache would otherwise target different
// flags depending on declaration order.
func negatedCounterpart(name string) (string, bool) {
	if rest, ok := strings.CutPrefix(name, "no-"); ok {
		if rest == "" {
			return "", false
		}
		return rest, true
	}
	return "no-" + name, true
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
	return slices.ContainsFunc(s.specs, func(spec flagSpec) bool { return spec.Name == name })
}

func (s *flagSpecs) hasShort(short string) bool {
	if s == nil || short == "" {
		return false
	}
	return slices.ContainsFunc(s.specs, func(spec flagSpec) bool { return spec.Short == short })
}

func (s *flagSpecs) lookupName(name string) (flagSpec, bool) {
	if s == nil {
		return flagSpec{}, false
	}
	i := slices.IndexFunc(s.specs, func(spec flagSpec) bool { return spec.Name == name })
	if i < 0 {
		return flagSpec{}, false
	}
	return s.specs[i], true
}

func (s *flagSpecs) lookupShort(b byte) (flagSpec, bool) {
	if s == nil {
		return flagSpec{}, false
	}
	i := slices.IndexFunc(s.specs, func(spec flagSpec) bool {
		return len(spec.Short) == 1 && spec.Short[0] == b
	})
	if i < 0 {
		return flagSpec{}, false
	}
	return s.specs[i], true
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
	return slices.ContainsFunc(s.specs, func(spec optionSpec) bool { return spec.Name == name })
}

func (s *optionSpecs) hasShort(short string) bool {
	if s == nil || short == "" {
		return false
	}
	return slices.ContainsFunc(s.specs, func(spec optionSpec) bool { return spec.Short == short })
}

func (s *optionSpecs) lookupName(name string) (optionSpec, bool) {
	if s == nil {
		return optionSpec{}, false
	}
	i := slices.IndexFunc(s.specs, func(spec optionSpec) bool { return spec.Name == name })
	if i < 0 {
		return optionSpec{}, false
	}
	return s.specs[i], true
}

func (s *optionSpecs) lookupShort(b byte) (optionSpec, bool) {
	if s == nil {
		return optionSpec{}, false
	}
	i := slices.IndexFunc(s.specs, func(spec optionSpec) bool {
		return len(spec.Short) == 1 && spec.Short[0] == b
	})
	if i < 0 {
		return optionSpec{}, false
	}
	return s.specs[i], true
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
	specs     []argSpec
	nameCache []string // mirrors specs[i].Name; amortizes names() allocations.
}

func (s *argSpecs) add(name, usage string) {
	validateInputName(name)
	if s.hasName(name) {
		panic(fmt.Sprintf("argv: duplicate argument %q", name))
	}
	s.specs = append(s.specs, argSpec{Name: name, Usage: usage})
	s.nameCache = append(s.nameCache, name)
}

func (s *argSpecs) hasName(name string) bool {
	if s == nil {
		return false
	}
	return slices.ContainsFunc(s.specs, func(spec argSpec) bool { return spec.Name == name })
}

func (s *argSpecs) parse(args []string, variadic bool) (ArgSet, []string, error) {
	var parsed ArgSet
	i := 0
	for _, spec := range s.specs {
		if i >= len(args) {
			return ArgSet{}, nil, fmt.Errorf("missing argument %q", spec.Name)
		}
		parsed.Set(spec.Name, args[i])
		i++
	}
	if variadic {
		return parsed, slices.Clone(args[i:]), nil
	}
	if i < len(args) {
		return ArgSet{}, nil, fmt.Errorf("unexpected argument %q", args[i])
	}
	return parsed, nil, nil
}

func (s *argSpecs) helpArguments() []HelpArg {
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
	return s.nameCache
}

// Entry types carry a value and whether a caller set it.

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

// A FlagSet holds boolean flag values. The zero value is an empty,
// usable set.
type FlagSet struct{ m map[string]flagEntry }

// String returns a space-separated list of true flags (e.g. "--verbose").
// False flags are omitted.
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

// Get returns the value associated with name, or false if not present.
func (s FlagSet) Get(name string) bool {
	if s.m == nil {
		return false
	}
	return s.m[name].value
}

// Lookup returns the value of name. The ok result is true if the
// value was set by a caller and false for spec defaults or missing
// entries.
func (s FlagSet) Lookup(name string) (value bool, ok bool) {
	if s.m == nil {
		return false, false
	}
	e, has := s.m[name]
	if !has {
		return false, false
	}
	return e.value, e.explicit
}

// Set associates name with value. [FlagSet.Lookup] reports the entry
// as caller-set. It panics if name is not a valid input name.
func (s *FlagSet) Set(name string, value bool) {
	validateInputName(name)
	if s.m == nil {
		s.m = make(map[string]flagEntry)
	}
	s.m[name] = flagEntry{value: value, explicit: true}
}

// Del removes name from s. It is a no-op if name is not present.
// Spec defaults are not reapplied; the next [FlagSet.Lookup] reports
// ok=false until the entry is set again.
func (s *FlagSet) Del(name string) {
	delete(s.m, name)
}

func (s *FlagSet) setDefault(name string, value bool) {
	if s.m == nil {
		s.m = make(map[string]flagEntry)
	}
	if _, ok := s.m[name]; !ok {
		s.m[name] = flagEntry{value: value}
	}
}

// setParsed records a parser-driven value without re-validating name.
// Parser callers source names from spec slices that have already been
// validated by [validateInputName].
func (s *FlagSet) setParsed(name string, value bool) {
	if s.m == nil {
		s.m = make(map[string]flagEntry)
	}
	s.m[name] = flagEntry{value: value, explicit: true}
}

// merge copies other's entries into s.
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

// All returns an iterator over flag names and values in ascending
// name order.
func (s FlagSet) All() iter.Seq2[string, bool] {
	return func(yield func(string, bool) bool) {
		for _, k := range slices.Sorted(maps.Keys(s.m)) {
			if !yield(k, s.m[k].value) {
				return
			}
		}
	}
}

// An OptionSet holds named value options. An option may carry multiple
// values when repeated, such as --tag foo --tag bar. Get returns the
// last value; Values returns all values.
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

// Values returns all values associated with name in insertion order.
// It returns nil if name is not present.
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

// Lookup returns the last value associated with name. The ok result
// is true if the value was set by a caller and false for spec defaults
// or missing entries.
//
// For all values of a repeated option, use [OptionSet.Values].
func (s OptionSet) Lookup(name string) (value string, ok bool) {
	if s.m == nil {
		return "", false
	}
	e, has := s.m[name]
	if !has || len(e.values) == 0 {
		return "", false
	}
	return e.values[len(e.values)-1], e.explicit
}

// Set replaces name with a single value. [OptionSet.Lookup] reports
// the entry as caller-set. It panics if name is not a valid input
// name.
func (s *OptionSet) Set(name string, value string) {
	validateInputName(name)
	if s.m == nil {
		s.m = make(map[string]optionEntry)
	}
	s.m[name] = optionEntry{values: []string{value}, explicit: true}
}

// Add appends value to the values associated with name.
// [OptionSet.Lookup] reports the entry as caller-set. It panics if
// name is not a valid input name.
func (s *OptionSet) Add(name string, value string) {
	validateInputName(name)
	if s.m == nil {
		s.m = make(map[string]optionEntry)
	}
	e := s.m[name]
	e.values = append(e.values, value)
	e.explicit = true
	s.m[name] = e
}

// Del removes name from s, discarding all values. It is a no-op if
// name is not present. Spec defaults are not reapplied; the next
// [OptionSet.Lookup] reports ok=false until the entry is set again.
func (s *OptionSet) Del(name string) {
	delete(s.m, name)
}

func (s *OptionSet) setDefault(name, value string) {
	if s.m == nil {
		s.m = make(map[string]optionEntry)
	}
	if _, ok := s.m[name]; !ok {
		s.m[name] = optionEntry{values: []string{value}}
	}
}

// addParsed appends a parser-driven value without re-validating name.
// Append (rather than replace) is intentional: a repeated CLI option
// such as "--tag a --tag b" must accumulate so [OptionSet.Values]
// returns both. See [FlagSet.setParsed] for the validation-skip
// rationale; the verb difference from setParsed mirrors [OptionSet.Add]
// versus [OptionSet.Set].
func (s *OptionSet) addParsed(name, value string) {
	if s.m == nil {
		s.m = make(map[string]optionEntry)
	}
	e := s.m[name]
	e.values = append(e.values, value)
	e.explicit = true
	s.m[name] = e
}

// merge copies other's entries into s. Value slices are cloned so the
// receiver and other share no storage.
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

// All returns an iterator over option names and value slices in
// ascending name order. Yielded slices are cloned; callers cannot
// mutate internal state.
func (s OptionSet) All() iter.Seq2[string, []string] {
	return func(yield func(string, []string) bool) {
		for _, k := range slices.Sorted(maps.Keys(s.m)) {
			if !yield(k, slices.Clone(s.m[k].values)) {
				return
			}
		}
	}
}

// An ArgSet holds bound positional arguments.
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

// Get returns the value associated with name, or the empty string if
// not present.
func (s ArgSet) Get(name string) string {
	if s.m == nil {
		return ""
	}
	return s.m[name].value
}

// Lookup returns the value of name. The ok result is true if the
// value was set by a caller and false for spec defaults or missing
// entries.
func (s ArgSet) Lookup(name string) (value string, ok bool) {
	if s.m == nil {
		return "", false
	}
	e, has := s.m[name]
	if !has {
		return "", false
	}
	return e.value, e.explicit
}

// Set associates name with value. [ArgSet.Lookup] reports the entry
// as caller-set. It panics if name is not a valid input name.
func (s *ArgSet) Set(name string, value string) {
	validateInputName(name)
	if s.m == nil {
		s.m = make(map[string]argEntry)
	}
	s.m[name] = argEntry{value: value, explicit: true}
}

// Del removes name from s. It is a no-op if name is not present. The
// next [ArgSet.Lookup] reports ok=false until the entry is set again.
func (s *ArgSet) Del(name string) {
	delete(s.m, name)
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

// All returns an iterator over argument names and values in
// ascending name order.
func (s ArgSet) All() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for _, k := range slices.Sorted(maps.Keys(s.m)) {
			if !yield(k, s.m[k].value) {
				return
			}
		}
	}
}
