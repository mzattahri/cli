package argv

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var errFlagHelp = errors.New("argv: help flag")

type parsedInput struct {
	flags   FlagSet
	options OptionSet
	args    []string
}

// parseInput walks args, populating parsed flags and options against
// the supplied spec slices. The unconsumed positional tail is
// returned as a subslice of args (no clone); callers that retain it
// past their own scope must clone.
func parseInput(flags *flagSpecs, options *optionSpecs, args []string, negateFlags bool) (parsedInput, error) {
	var parsed parsedInput
	pos := 0
	for pos < len(args) {
		arg := args[pos]
		if arg == "--" {
			pos++
			parsed.args = args[pos:]
			return parsed, nil
		}
		if arg == "--help" || arg == "-h" {
			return parsedInput{}, errFlagHelp
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			parsed.args = args[pos:]
			return parsed, nil
		}
		if !strings.HasPrefix(arg, "--") {
			pos++
			shorts := arg[1:]
			for i := 0; i < len(shorts); i++ {
				b := shorts[i]
				if b == 'h' {
					return parsedInput{}, errFlagHelp
				}
				if spec, ok := flags.lookupShort(b); ok {
					parsed.flags.setParsed(spec.Name, true)
					continue
				}
				if spec, ok := options.lookupShort(b); ok {
					if i != len(shorts)-1 {
						return parsedInput{}, fmt.Errorf("option -%c must be final in %q", b, arg)
					}
					if pos >= len(args) {
						return parsedInput{}, fmt.Errorf("missing value for -%c", b)
					}
					parsed.options.addParsed(spec.Name, args[pos])
					pos++
					continue
				}
				return parsedInput{}, fmt.Errorf("unknown flag %q", "-"+string(b))
			}
			continue
		}

		pos++
		name, rawValue, hasValue := strings.Cut(arg[2:], "=")

		if spec, ok := flags.lookupName(name); ok {
			value := true
			if hasValue {
				boolValue, err := strconv.ParseBool(rawValue)
				if err != nil {
					return parsedInput{}, fmt.Errorf("invalid boolean value %q for --%s", rawValue, name)
				}
				value = boolValue
			}
			parsed.flags.setParsed(spec.Name, value)
			continue
		}

		if negateFlags {
			if negated, ok := negateFlagName(name, flags); ok {
				if hasValue {
					return parsedInput{}, fmt.Errorf("--%s does not accept a value", "no-"+negated)
				}
				parsed.flags.setParsed(negated, false)
				continue
			}
		}

		if spec, ok := options.lookupName(name); ok {
			if !hasValue {
				if pos >= len(args) {
					return parsedInput{}, fmt.Errorf("missing value for --%s", name)
				}
				rawValue = args[pos]
				pos++
			}
			parsed.options.addParsed(spec.Name, rawValue)
			continue
		}

		return parsedInput{}, fmt.Errorf("unknown flag %q", "--"+name)
	}

	return parsed, nil
}

// negateFlagName checks whether name is a negation of a known flag.
// It returns the target flag name and true if a match is found.
//
// Negation is bidirectional:
//   - --no-verbose negates a flag named "verbose"
//   - --cache negates a flag named "no-cache"
func negateFlagName(name string, flags *flagSpecs) (string, bool) {
	if target, ok := strings.CutPrefix(name, "no-"); ok {
		if spec, ok := flags.lookupName(target); ok {
			return spec.Name, true
		}
	} else {
		if spec, ok := flags.lookupName("no-" + name); ok {
			return spec.Name, true
		}
	}
	return "", false
}

func validateShortName(short string) string {
	if len(short) != 1 {
		panic("argv: short name must be one character")
	}
	b := short[0]
	if !isLetter(b) && !isDigit(b) {
		panic(fmt.Sprintf("argv: invalid short name %q", short))
	}
	if short == "h" {
		panic(`argv: short name "h" is reserved for help`)
	}
	return short
}
