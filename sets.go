package cli

import (
	"fmt"
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
	if strings.Contains(name, "=") {
		panic("cli: invalid " + kind + " name")
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

func (s *flagSpecs) helpEntries() []helpFlag {
	return s.helpEntriesNegatable(false)
}

func (s *flagSpecs) helpEntriesNegatable(negatable bool) []helpFlag {
	if s == nil {
		return nil
	}
	out := make([]helpFlag, 0, len(s.specs))
	for _, spec := range s.specs {
		out = append(out, helpFlag{
			Name:      spec.Name,
			Short:     spec.Short,
			Usage:     spec.Usage,
			Default:   spec.Default,
			Negatable: negatable,
		})
	}
	return out
}

func (s *flagSpecs) defaultMap() map[string]bool {
	if s == nil {
		return nil
	}
	out := make(map[string]bool, len(s.specs))
	for _, spec := range s.specs {
		out[spec.Name] = spec.Default
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

func (s *optionSpecs) helpEntries() []helpOption {
	if s == nil {
		return nil
	}
	out := make([]helpOption, 0, len(s.specs))
	for _, spec := range s.specs {
		out = append(out, helpOption{
			Name:    spec.Name,
			Short:   spec.Short,
			Usage:   spec.Usage,
			Default: spec.Default,
		})
	}
	return out
}

func (s *optionSpecs) defaultMap() map[string]string {
	if s == nil {
		return nil
	}
	out := make(map[string]string, len(s.specs))
	for _, spec := range s.specs {
		out[spec.Name] = spec.Default
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
	parsed := ArgSet{}
	i := 0
	for _, spec := range s.specs {
		if i >= len(args) {
			return nil, nil, fmt.Errorf("missing argument %q", spec.Name)
		}
		parsed[spec.Name] = args[i]
		i++
	}
	if captureRest {
		return parsed, slices.Clone(args[i:]), nil
	}
	if i < len(args) {
		return nil, nil, fmt.Errorf("unexpected argument %q", args[i])
	}
	return parsed, nil, nil
}

func (s *argSpecs) helpArguments() []helpArg {
	if s == nil {
		return nil
	}
	out := make([]helpArg, 0, len(s.specs))
	for _, spec := range s.specs {
		out = append(out, helpArg{Name: "<" + spec.Name + ">", Usage: spec.Usage})
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

// A FlagSet holds boolean flag values.
type FlagSet map[string]bool

// String returns a space-separated list of present flags (e.g. "--verbose").
func (s FlagSet) String() string {
	var names []string
	for k, v := range s {
		if v {
			names = append(names, "--"+k)
		}
	}
	slices.Sort(names)
	return strings.Join(names, " ")
}

// Has reports whether name exists in the set.
func (s FlagSet) Has(name string) bool {
	if s == nil {
		return false
	}
	_, ok := s[name]
	return ok
}

// Get returns the value associated with name, or false if not present.
func (s FlagSet) Get(name string) bool {
	if s == nil {
		return false
	}
	return s[name]
}

// Set associates name with value.
func (s FlagSet) Set(name string, value bool) { s[name] = value }

// An OptionSet holds named value options. Each option may carry multiple
// values when repeated on the command line (e.g. --tag foo --tag bar).
// The underlying type follows the [net/http.Header] convention:
// [OptionSet.Get] returns the last value, direct map access returns the
// full []string, and [OptionSet.Values] returns all values.
type OptionSet map[string][]string

// String returns a space-separated list of options (e.g. "--host localhost").
// Multi-valued options are expanded into separate entries.
func (s OptionSet) String() string {
	var pairs []string
	for k, vals := range s {
		for _, v := range vals {
			pairs = append(pairs, fmt.Sprintf("--%s %s", k, quoteToken(v)))
		}
	}
	slices.Sort(pairs)
	return strings.Join(pairs, " ")
}

// Has reports whether name exists in the set.
func (s OptionSet) Has(name string) bool {
	if s == nil {
		return false
	}
	_, ok := s[name]
	return ok
}

// Get returns the last value associated with name, or the empty string
// if the option is not present. For options specified multiple times,
// use [OptionSet.Values] to retrieve all values.
func (s OptionSet) Get(name string) string {
	if s == nil {
		return ""
	}
	v := s[name]
	if len(v) == 0 {
		return ""
	}
	return v[len(v)-1]
}

// Values returns all values associated with name in the order they
// appeared on the command line. It returns nil if name is not present.
func (s OptionSet) Values(name string) []string {
	if s == nil {
		return nil
	}
	return slices.Clone(s[name])
}

// Set replaces name with a single value.
func (s OptionSet) Set(name string, value string) {
	s[name] = []string{value}
}

// Add appends value to the values associated with name.
func (s OptionSet) Add(name string, value string) {
	s[name] = append(s[name], value)
}

// Clone returns a deep copy of s. The returned OptionSet shares no
// slice storage with s, so either can be mutated independently.
func (s OptionSet) Clone() OptionSet {
	if s == nil {
		return nil
	}
	out := make(OptionSet, len(s))
	for k, vals := range s {
		out[k] = slices.Clone(vals)
	}
	return out
}

// An ArgSet holds bound positional arguments.
type ArgSet map[string]string

// String returns a space-separated list of arguments (e.g. "<path> /tmp").
func (s ArgSet) String() string {
	var pairs []string
	for k, v := range s {
		pairs = append(pairs, fmt.Sprintf("<%s> %s", k, quoteToken(v)))
	}
	slices.Sort(pairs)
	return strings.Join(pairs, " ")
}

// Has reports whether name exists in the set.
func (s ArgSet) Has(name string) bool {
	if s == nil {
		return false
	}
	_, ok := s[name]
	return ok
}

// Get returns the value associated with name, or the empty string if
// not present.
func (s ArgSet) Get(name string) string {
	if s == nil {
		return ""
	}
	return s[name]
}

// Set associates name with value.
func (s ArgSet) Set(name string, value string) { s[name] = value }
