package cli

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

var errFlagHelp = errors.New("cli: help flag")

type parsedInput struct {
	flags   FlagSet
	options OptionSet
	args    []string
}

func parseInput(flags *flagSpecs, options *optionSpecs, args []string, negateFlags bool) (*parsedInput, error) {
	cur := &tokenCursor{tokens: args}
	return parseInputCursor(flags, options, cur, negateFlags)
}

type tokenCursor struct {
	tokens []string
	pos    int
}

func (c *tokenCursor) done() bool {
	return c.pos >= len(c.tokens)
}

// peek returns the current token without advancing. Callers must
// check [tokenCursor.done] first; the empty string returned for an
// exhausted cursor is a convenience, not a distinct sentinel.
func (c *tokenCursor) peek() string {
	if c.done() {
		return ""
	}
	return c.tokens[c.pos]
}

func (c *tokenCursor) next() string {
	token := c.peek()
	if !c.done() {
		c.pos++
	}
	return token
}

func (c *tokenCursor) rest() []string {
	if c.done() {
		return nil
	}
	return slices.Clone(c.tokens[c.pos:])
}

func parseInputCursor(flags *flagSpecs, options *optionSpecs, cur *tokenCursor, negateFlags bool) (*parsedInput, error) {
	parsed := &parsedInput{
		flags:   FlagSet{},
		options: OptionSet{},
	}

	flagByName := map[string]flagSpec{}
	flagByShort := map[string]flagSpec{}
	if flags != nil {
		for _, spec := range flags.specs {
			flagByName[spec.Name] = spec
			if spec.Short != "" {
				flagByShort[spec.Short] = spec
			}
		}
	}

	optionByName := map[string]optionSpec{}
	optionByShort := map[string]optionSpec{}
	if options != nil {
		for _, spec := range options.specs {
			optionByName[spec.Name] = spec
			if spec.Short != "" {
				optionByShort[spec.Short] = spec
			}
		}
	}

	for !cur.done() {
		arg := cur.peek()
		if arg == "--" {
			cur.next()
			parsed.args = cur.rest()
			return parsed, nil
		}
		if arg == "--help" || arg == "-h" {
			return nil, errFlagHelp
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			parsed.args = cur.rest()
			return parsed, nil
		}
		if !strings.HasPrefix(arg, "--") {
			cur.next()
			shorts := arg[1:]
			for i := 0; i < len(shorts); i++ {
				short := string(shorts[i])
				if short == "h" {
					return nil, errFlagHelp
				}
				if spec, ok := flagByShort[short]; ok {
					parsed.flags[spec.Name] = true
					continue
				}
				if spec, ok := optionByShort[short]; ok {
					if i != len(shorts)-1 {
						return nil, fmt.Errorf("option -%s must be final in %q", short, arg)
					}
					if cur.done() {
						return nil, fmt.Errorf("missing value for -%s", short)
					}
					parsed.options.Add(spec.Name, cur.next())
					continue
				}
				return nil, fmt.Errorf("unknown flag %q", "-"+short)
			}
			continue
		}

		cur.next()
		name, rawValue, hasValue := splitFlagToken(arg[2:])

		if spec, ok := flagByName[name]; ok {
			value := true
			if hasValue {
				boolValue, err := strconv.ParseBool(rawValue)
				if err != nil {
					return nil, fmt.Errorf("invalid boolean value %q for --%s", rawValue, name)
				}
				value = boolValue
			}
			parsed.flags[spec.Name] = value
			continue
		}

		if negateFlags {
			if negated, ok := negateFlagName(name, flagByName); ok {
				if hasValue {
					return nil, fmt.Errorf("--%s does not accept a value", "no-"+negated)
				}
				parsed.flags[negated] = false
				continue
			}
		}

		if spec, ok := optionByName[name]; ok {
			if !hasValue {
				if cur.done() {
					return nil, fmt.Errorf("missing value for --%s", name)
				}
				rawValue = cur.next()
			}
			parsed.options.Add(spec.Name, rawValue)
			continue
		}

		return nil, fmt.Errorf("unknown flag %q", "--"+name)
	}

	return parsed, nil
}

// negateFlagName checks whether name is a negation of a known flag.
// It returns the target flag name and true if a match is found.
//
// Negation is bidirectional:
//   - --no-verbose negates a flag named "verbose"
//   - --cache negates a flag named "no-cache"
func negateFlagName(name string, flagByName map[string]flagSpec) (string, bool) {
	if strings.HasPrefix(name, "no-") {
		target := name[3:]
		if spec, ok := flagByName[target]; ok {
			return spec.Name, true
		}
	} else {
		target := "no-" + name
		if spec, ok := flagByName[target]; ok {
			return spec.Name, true
		}
	}
	return "", false
}

func splitFlagToken(token string) (name, value string, hasValue bool) {
	name = token
	if i := strings.IndexByte(token, '='); i >= 0 {
		name = token[:i]
		value = token[i+1:]
		hasValue = true
	}
	return name, value, hasValue
}

func validateShortName(short string) string {
	if len(short) != 1 {
		panic("cli: short name must be one character")
	}
	if short == "-" || short == "=" {
		panic("cli: invalid short name " + `"` + short + `"`)
	}
	if short == "h" {
		panic("cli: short name " + `"h"` + " is reserved for help")
	}
	return short
}
