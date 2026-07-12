package main

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	capturedPreDash  []string
	capturedPostDash []string
	isCaptured       bool
)

const doc = `Usage: %s [flags...] [task...]

Runs the specified task(s). Falls back to the "default" task if no task name
was specified, or lists all tasks if an unknown task name was specified.

Options:
`

func toKebabCase(str string) string {
	var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

	kebab := matchFirstCap.ReplaceAllString(str, "${1}-${2}")
	kebab = matchAllCap.ReplaceAllString(kebab, "${1}-${2}")
	return strings.ToLower(kebab)
}

func getEnvName(appName string, flagName string) string {
	name := strings.ToUpper(appName) + "_" + strings.ToUpper(flagName)
	return strings.ReplaceAll(name, "-", "_")
}

func usage(cfg interface{}, appName string) {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	_, _ = fmt.Fprintf(flag.CommandLine.Output(), doc, appName)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		flagName := field.Tag.Get("flag")
		if flagName == "" {
			flagName = toKebabCase(field.Name)
		}
		flagDefault := field.Tag.Get("default")
		flagShort := field.Tag.Get("sflag")
		flagDoc := field.Tag.Get("doc")
		flagEnv := field.Tag.Get("noenv") != "true"

		if flagDoc == "" {
			continue
		}

		var flagSyntax string
		if flagShort != "" {
			flagSyntax = fmt.Sprintf("  -%s, --%s", flagShort, flagName)
		} else {
			flagSyntax = fmt.Sprintf("      --%s", flagName)
		}
		switch v.Field(i).Kind() {
		case reflect.String:
			flagSyntax += " string"
		case reflect.Int:
			flagSyntax += " int"
		}
		envText := ""
		if flagEnv {
			envText = fmt.Sprintf(" (Env: %s)", getEnvName(appName, flagName))
		}
		defaultText := ""
		if flagDefault != "" {
			defaultText = fmt.Sprintf(" (default %q)", flagDefault)
		}

		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "%-30s\t%s%s%s\n",
			flagSyntax, flagDoc, envText, defaultText)
	}
}

func ParseFlags(cfg interface{}, appName string) error {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := v.Field(i)

		flagName := field.Tag.Get("flag")
		if flagName == "" {
			flagName = toKebabCase(field.Name)
		}
		flagDefault := field.Tag.Get("default")
		flagShort := field.Tag.Get("sflag")
		flagEnv := field.Tag.Get("noenv") != "true"

		// Combine configuration layers: default tag -> environment variable
		value := flagDefault
		if flagEnv {
			if envVal, exists := os.LookupEnv(getEnvName(appName, flagName)); exists {
				value = envVal
			}
		}

		switch fieldVal.Kind() {
		case reflect.String:
			ptr := fieldVal.Addr().Interface().(*string)
			if value != "" && *ptr == "" {
				*ptr = value
			}
			// Use 'new' to allocate to the heap so pointers stay alive out of loop scope
			tmpVal := new(string)
			*tmpVal = value
			flag.StringVar(tmpVal, flagName, *tmpVal, "")
			if len(flagShort) == 1 {
				flag.StringVar(tmpVal, flagShort, *tmpVal, "") // same pointer
			}

		case reflect.Int:
			ptr := fieldVal.Addr().Interface().(*int)
			if value != "" && *ptr == 0 {
				parsedInt, err := strconv.Atoi(value)
				if err != nil {
					return fmt.Errorf("invalid int value for flag %s: %w", flagName, err)
				}
				*ptr = parsedInt
			}
			tmpVal := new(int)
			*tmpVal = *ptr
			flag.IntVar(tmpVal, flagName, *tmpVal, "")
			if len(flagShort) == 1 {
				flag.IntVar(tmpVal, flagShort, *tmpVal, "")
			}

		case reflect.Int64:
			if fieldVal.Type().String() == "time.Duration" {
				ptr := fieldVal.Addr().Interface().(*time.Duration)
				if value != "" && *ptr == 0 {
					parsedDuration, err := time.ParseDuration(value)
					if err != nil {
						return fmt.Errorf("invalid duration value for flag %s: %w", flagName, err)
					}
					*ptr = parsedDuration
				}
				tmpVal := new(time.Duration)
				*tmpVal = *ptr
				flag.DurationVar(tmpVal, flagName, *tmpVal, "")
				if len(flagShort) == 1 {
					flag.DurationVar(tmpVal, flagShort, *tmpVal, "")
				}
			}

		case reflect.Bool:
			ptr := fieldVal.Addr().Interface().(*bool)
			if value != "" {
				parsedBool, err := strconv.ParseBool(value)
				if err != nil {
					return fmt.Errorf("invalid boolean value for flag %s: %w", flagName, err)
				}
				if !*ptr {
					*ptr = parsedBool
				}
			}
			tmpVal := new(bool)
			*tmpVal = *ptr
			flag.BoolVar(tmpVal, flagName, *tmpVal, "")
			if len(flagShort) == 1 {
				flag.BoolVar(tmpVal, flagShort, *tmpVal, "")
			}

		default:
			return fmt.Errorf("unsupported configuration type %s: %s", flagName, fieldVal.Kind())
		}
	}

	flag.Usage = func() {
		usage(cfg, appName)
	}

	// Parse command line arguments into our isolated heap tmp pointers
	flag.Parse()
	CaptureCliArgs()

	// Track exactly what flags the user physically typed
	providedFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		providedFlags[f.Name] = true
	})

	// Step 4: Second Pass. Resolve conflicts if BOTH flags were provided
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := v.Field(i)

		flagName := field.Tag.Get("flag")
		if flagName == "" {
			flagName = toKebabCase(field.Name)
		}
		flagShort := field.Tag.Get("sflag")

		hasLong := providedFlags[flagName]
		hasShort := len(flagShort) == 1 && providedFlags[flagShort]

		var finalFlagName string

		if hasLong && hasShort {
			// Precedence check: Parse through os.Args to find out which flag was typed last
			longIdx, shortIdx := -1, -1
			for idx, arg := range os.Args {
				if arg == "--"+flagName || strings.HasPrefix(arg, "--"+flagName+"=") {
					longIdx = idx
				}
				if arg == "-"+flagShort || strings.HasPrefix(arg, "-"+flagShort+"=") {
					shortIdx = idx
				}
			}
			// Whichever flag was listed furthest right on the CLI wins
			if longIdx > shortIdx {
				finalFlagName = flagName
			} else {
				finalFlagName = flagShort
			}
		} else if hasLong {
			finalFlagName = flagName
		} else if hasShort {
			finalFlagName = flagShort
		}

		// Apply the winning flag string to the configuration struct field
		if finalFlagName != "" {
			f := flag.Lookup(finalFlagName)
			if f != nil {
				getter := f.Value.(flag.Getter)
				switch fieldVal.Kind() {
				case reflect.String:
					fieldVal.SetString(getter.Get().(string))
				case reflect.Int:
					fieldVal.SetInt(int64(getter.Get().(int)))
				case reflect.Int64:
					if fieldVal.Type().String() == "time.Duration" {
						fieldVal.Set(reflect.ValueOf(getter.Get().(time.Duration)))
					}
				case reflect.Bool:
					fieldVal.SetBool(getter.Get().(bool))
				}
			}
		}
	}

	return nil
}

// Get extracts arguments cleanly without relying on the fragile flag.Args() pool.
func CaptureCliArgs() {
	if isCaptured {
		return
	}

	remaining := flag.Args()

	// Find the index of the literal "--" inside the remaining positional arguments
	dashIndex := -1
	for i, arg := range remaining {
		if arg == "--" {
			dashIndex = i
			break
		}
	}

	// Case 1: No explicit "--" was found in the positional pool
	if dashIndex == -1 {
		capturedPreDash = remaining
		capturedPostDash = []string{}
		isCaptured = true
		return
	}

	// Case 2: "--" exists. Slice everything before it, and everything after it.
	// This explicitly throws away the literal "--" token itself.
	capturedPreDash = remaining[:dashIndex]
	capturedPostDash = remaining[dashIndex+1:]
	isCaptured = true
}

func GetCliArgs() ([]string, []string) {
	return capturedPreDash, capturedPostDash
}
